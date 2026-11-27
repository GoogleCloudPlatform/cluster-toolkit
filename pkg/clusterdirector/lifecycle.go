// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clusterdirector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"hpc-toolkit/pkg/sourcereader"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

// Overall timeout for Cluster Director operations.
const cdWaitTimeout = 30 * time.Minute

// Artifact filenames written under `<deplRoot>/.ghpc/artifacts/`.
const (
	cdMarkerFile  = modulewriter.ClusterDirectorMarkerName
	cdRequestFile = modulewriter.ClusterDirectorRequestName
)

// errGroupNotDeployed is returned by readGroupTerraformState when a group
// directory has neither a .terraform directory nor a local terraform.tfstate
// file (for example, a group skipped via --only / --skip on initial deploy).
var errGroupNotDeployed = errors.New("group has not been initialized or deployed")

var (
	cdPollInterval          = 10 * time.Second
	newCdClient             = NewClient
	readGroupTerraformState = defaultReadGroupTerraformState
	initRemoteGroupState    = defaultInitAndReadRemoteGroupState
)

// ExportOptions configures an Export run at the end of `gcluster deploy`.
type ExportOptions struct {
	DeplRoot     string
	ArtifactsDir string
	Blueprint    config.Blueprint
	// Override is non-nil when --export-to-cluster-director was explicitly
	// passed on the CLI.
	Override *bool
}

// DeleteOptions configures a Delete (or partial-destroy shrink) run at the end
// of `gcluster destroy`.
type DeleteOptions struct {
	DeplRoot        string
	ArtifactsDir    string
	Blueprint       config.Blueprint
	Override        *bool
	IsGroupSelected func(config.GroupName) bool
	SkippedGroups   map[config.GroupName]bool
}

// SupersededCluster records a previously exported cluster whose ID, region, or
// project changed across `-w`. Persisting it in Marker until cleanup completes
// ensures an interrupted supersede never orphans the old cluster record.
type SupersededCluster struct {
	ClusterName    string `json:"cluster_name"`
	DeploymentName string `json:"deployment_name,omitempty"`
	ProjectID      string `json:"project_id"`
	Region         string `json:"region"`
}

// Marker is the on-disk record of an exported cluster. `gcluster destroy`
// reads it instead of recomputing values because the blueprint may have been
// edited since the export.
type Marker struct {
	ClusterName              string             `json:"cluster_name"`
	DeploymentName           string             `json:"deployment_name,omitempty"`
	ProjectID                string             `json:"project_id"`
	Region                   string             `json:"region"`
	SkipCrossProjectNetworks bool               `json:"skip_cross_project_networks,omitempty"`
	Superseded               *SupersededCluster `json:"superseded,omitempty"`
}

// ApplyOverrideToArtifacts persists an explicit --export-to-cluster-director
// CLI override into `<artDir>/expanded_blueprint.yaml`.
func ApplyOverrideToArtifacts(artDir string, enabled bool) error {
	expPath := filepath.Join(artDir, modulewriter.ExpandedBlueprintName)
	bp, _, err := config.NewBlueprint(expPath)
	if err != nil {
		return err
	}
	bp.SetExportToClusterDirector(enabled)
	return bp.Export(expPath)
}

// PreviousExportSetting returns the export_to_cluster_director decision saved
// in an existing deployment folder so `gcluster create -w` and `gcluster deploy
// <bp.yaml> -w` preserve a previous `--export-to-cluster-director` choice when
// neither the blueprint, deployment file, nor CLI flag sets one explicitly.
func PreviousExportSetting(deplDir string) (bool, bool) {
	artDir := modulewriter.ArtifactsDir(deplDir)
	expPath := filepath.Join(artDir, modulewriter.ExpandedBlueprintName)
	if _, err := os.Stat(expPath); err == nil {
		if prev, _, readErr := config.NewBlueprint(expPath); readErr == nil && prev.ExportToClusterDirector != nil {
			return *prev.ExportToClusterDirector, true
		}
	}
	if _, hasMarker, err := readMarker(artDir); err == nil && hasMarker {
		return true, true
	}
	return false, false
}

type rawTerraformStateFile struct {
	Version   int `json:"version"`
	Resources []struct {
		Module    string `json:"module"`
		Mode      string `json:"mode"`
		Type      string `json:"type"`
		Name      string `json:"name"`
		Instances []struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"instances"`
	} `json:"resources"`
}

func formatLocalResourceAddress(moduleAddr, mode, typ, name string) string {
	res := typ + "." + name
	if mode == string(tfjson.DataResourceMode) {
		res = "data." + res
	}
	if moduleAddr != "" {
		return moduleAddr + "." + res
	}
	return res
}

func readLocalTerraformStateFile(path string) (*tfjson.State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw rawTerraformStateFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Version == 0 && len(raw.Resources) == 0 {
		return nil, fmt.Errorf("unrecognized state file %s", path)
	}
	root := &tfjson.StateModule{}
	for _, r := range raw.Resources {
		addr := formatLocalResourceAddress(r.Module, r.Mode, r.Type, r.Name)
		for _, inst := range r.Instances {
			root.Resources = append(root.Resources, &tfjson.StateResource{
				Address:         addr,
				Mode:            tfjson.ResourceMode(r.Mode),
				Type:            r.Type,
				Name:            r.Name,
				AttributeValues: inst.Attributes,
			})
		}
	}
	return &tfjson.State{Values: &tfjson.StateValues{RootModule: root}}, nil
}

func defaultReadGroupTerraformState(ctx context.Context, groupDir string) (*tfjson.State, error) {
	_, hasDotTerraformErr := os.Stat(filepath.Join(groupDir, ".terraform"))
	stateFile := filepath.Join(groupDir, "terraform.tfstate")
	_, hasStateFileErr := os.Stat(stateFile)

	if hasDotTerraformErr == nil {
		tf, err := shell.ConfigureTerraform(groupDir)
		if err != nil {
			return nil, fmt.Errorf("terraform is unavailable: %w", err)
		}
		st, err := tf.Show(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not read its state: %w", err)
		}
		return st, nil
	}

	if hasStateFileErr == nil {
		st, err := readLocalTerraformStateFile(stateFile)
		if err != nil {
			return nil, fmt.Errorf("could not read its state: %w", err)
		}
		return st, nil
	}

	return nil, errGroupNotDeployed
}

func defaultInitAndReadRemoteGroupState(ctx context.Context, groupDir string) (*tfjson.State, error) {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return nil, fmt.Errorf("terraform is unavailable: %w", err)
	}
	logging.Info("Cluster Director: initializing remote backend for %s to inspect state", groupDir)
	if err := shell.InitModule(ctx, tf); err != nil {
		return nil, err
	}
	st, err := tf.Show(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not read its state: %w", err)
	}
	return st, nil
}

func markerPath(artifactsDir string) string {
	return filepath.Join(artifactsDir, cdMarkerFile)
}

func requestPath(artifactsDir string) string {
	return filepath.Join(artifactsDir, cdRequestFile)
}

func writeRequestPayload(artifactsDir string, cluster *Cluster) error {
	if artifactsDir == "" || cluster == nil {
		return nil
	}
	data, err := json.MarshalIndent(cluster, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(requestPath(artifactsDir), append(data, '\n'), 0644)
}

func writeMarker(artifactsDir string, m Marker) error {
	if artifactsDir == "" {
		return nil
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath(artifactsDir), append(data, '\n'), 0644)
}

// readMarker returns the marker, or ok=false when the deployment was never
// exported. A malformed marker is an error: silently ignoring it would leak
// an exported cluster record.
func readMarker(artifactsDir string) (Marker, bool, error) {
	data, err := os.ReadFile(markerPath(artifactsDir))
	if os.IsNotExist(err) {
		return Marker{}, false, nil
	}
	if err != nil {
		return Marker{}, false, err
	}
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return Marker{}, false, fmt.Errorf("could not read %s: %w", markerPath(artifactsDir), err)
	}
	return m, true, nil
}

func removeMarker(artifactsDir string) error {
	if err := os.Remove(markerPath(artifactsDir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(requestPath(artifactsDir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// stringVar reads a global blueprint variable, returning "" when it is absent
// or is not a string.
func stringVar(bp config.Blueprint, key string) string {
	if !bp.Vars.Has(key) {
		return ""
	}
	v, _ := bp.Vars.Get(key).Unmark()
	if v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
		return ""
	}
	return v.AsString()
}

func cleanModuleSource(src string) string {
	s, _, _ := strings.Cut(src, "?")
	return strings.TrimRight(s, "/")
}

// hasModuleName reports whether the module source src is name or ends in
// "/<name>".
func hasModuleName(src, name string) bool {
	s := cleanModuleSource(src)
	return s == name || strings.HasSuffix(s, "/"+name)
}

func builtInToolkitModulePath(src string) (string, bool) {
	s := sourcereader.ToSlash(cleanModuleSource(src))
	if sourcereader.IsEmbeddedPath(s) {
		return s, true
	}
	lower := strings.ToLower(s)
	for _, repo := range []string{
		"github.com/googlecloudplatform/cluster-toolkit//",
		"github.com/googlecloudplatform/hpc-toolkit//",
	} {
		if idx := strings.Index(lower, repo); idx >= 0 {
			sub := s[idx+len(repo):]
			if sourcereader.IsEmbeddedPath(sub) {
				return sub, true
			}
		}
	}
	return "", false
}

// isBuiltInNonStorageModule reports whether src points to a built-in Cluster
// Toolkit module outside `file-system` (such as `compute/vm-instance`,
// `scripts/startup-script`, or `scheduler/schedmd-slurm-gcp-v6-controller`),
// whose `google_storage_bucket` resources are internal config/staging buckets
// rather than user workload storage. User local modules (e.g. `./modules/...`)
// and third-party remote modules are never skipped.
func isBuiltInNonStorageModule(src string) bool {
	embedded, ok := builtInToolkitModulePath(src)
	if !ok {
		return false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(embedded, "community/"), "modules/")
	cat, _, hasSlash := strings.Cut(rest, "/")
	return hasSlash && cat != "" && cat != "file-system" && cat != "files"
}

func internalBucketModules(bp config.Blueprint) map[string]bool {
	var ignored map[string]bool
	for _, g := range bp.Groups {
		for _, m := range g.Modules {
			if m.ID != "" && isBuiltInNonStorageModule(m.Source) {
				if ignored == nil {
					ignored = map[string]bool{}
				}
				ignored[string(m.ID)] = true
			}
		}
	}
	return ignored
}

// blueprintHasGKE reports whether any module in the blueprint is a GKE
// scheduler or node-pool module (`gke-cluster`, `gke-node-pool`, or
// `pre-existing-gke-cluster`).
func blueprintHasGKE(bp config.Blueprint) bool {
	for _, g := range bp.Groups {
		for _, m := range g.Modules {
			if hasModuleName(m.Source, "gke-cluster") || hasModuleName(m.Source, "gke-node-pool") || hasModuleName(m.Source, "pre-existing-gke-cluster") {
				return true
			}
		}
	}
	return false
}

// blueprintHasMIG reports whether any module in any group of the blueprint
// creates a Managed Instance Group (`mig`, `htcondor-execute-point`, or
// `schedmd-slurm-gcp-v6-nodeset` with `provisioning_engine: MIG`).
func blueprintHasMIG(bp config.Blueprint) bool {
	for _, g := range bp.Groups {
		for _, m := range g.Modules {
			if moduleCreatesMIG(m) {
				return true
			}
		}
	}
	return false
}

func moduleCreatesMIG(m config.Module) bool {
	if hasModuleName(m.Source, "mig") || hasModuleName(m.Source, "htcondor-execute-point") {
		return true
	}
	if !hasModuleName(m.Source, "schedmd-slurm-gcp-v6-nodeset") || !m.Settings.Has("provisioning_engine") {
		return false
	}
	v, _ := m.Settings.Get("provisioning_engine").Unmark()
	return !v.IsNull() && v.IsKnown() && v.Type() == cty.String && v.AsString() == "MIG"
}

func moduleHasResources(m *tfjson.StateModule) bool {
	if m == nil {
		return false
	}
	if len(m.Resources) > 0 {
		return true
	}
	for _, c := range m.ChildModules {
		if moduleHasResources(c) {
			return true
		}
	}
	return false
}

func stateHasResources(st *tfjson.State) bool {
	if st == nil || st.Values == nil {
		return false
	}
	return moduleHasResources(st.Values.RootModule)
}

func groupHasRemoteBackend(bp config.Blueprint, group config.Group) bool {
	t := group.TerraformBackend.Type
	if t == "" {
		t = bp.TerraformBackendDefaults.Type
	}
	return t != "" && t != "local"
}

// gatherTerraformStates reads the state of the Terraform groups selected by
// include. Uninitialized local-backend groups and groups with empty state
// (including remote-backend groups that have not been deployed yet) are
// skipped, whereas unreadable local state or a remote backend that fails to
// initialize or read returns an error so deployed resources are never silently
// dropped.
func gatherTerraformStates(ctx context.Context, deplRoot string, bp config.Blueprint, include func(config.GroupName) bool) ([]*tfjson.State, error) {
	var states []*tfjson.State
	for _, group := range bp.Groups {
		if group.Kind() != config.TerraformKind || !include(group.Name) {
			continue
		}
		groupDir := filepath.Join(deplRoot, string(group.Name))
		st, ok, err := readGroupStateForDiscovery(ctx, bp, group, groupDir)
		if err != nil {
			return nil, err
		}
		if ok {
			states = append(states, st)
		}
	}
	return states, nil
}

func readGroupStateForDiscovery(ctx context.Context, bp config.Blueprint, group config.Group, groupDir string) (*tfjson.State, bool, error) {
	st, err := readGroupTerraformState(ctx, groupDir)
	if errors.Is(err, errGroupNotDeployed) {
		if !groupHasRemoteBackend(bp, group) {
			logging.Info("Cluster Director: skipping group %q, %v", group.Name, err)
			return nil, false, nil
		}
		st, err = initRemoteGroupState(ctx, groupDir)
		if err != nil {
			return nil, false, fmt.Errorf("could not read remote Terraform state for group %q: %w", group.Name, err)
		}
	} else if err != nil {
		return nil, false, fmt.Errorf("could not read Terraform state for group %q: %w", group.Name, err)
	}
	if !stateHasResources(st) {
		logging.Info("Cluster Director: skipping group %q, state has no resources", group.Name)
		return nil, false, nil
	}
	return st, true, nil
}

func allGroups(config.GroupName) bool { return true }

// buildSpecFromDeployment assembles an export spec by inspecting the Terraform
// state of the groups selected by include.
//
// ok=false (with err=nil) means the deployment is not exportable at all (for
// example it declares no project or region, no Terraform group has any
// deployed resources, or it is a GKE deployment).
func buildSpecFromDeployment(ctx context.Context, deplRoot string, bp config.Blueprint, include func(config.GroupName) bool) (Spec, bool, error) {
	projectID := stringVar(bp, "project_id")
	region := stringVar(bp, "region")
	deploymentName := stringVar(bp, "deployment_name")

	if projectID == "" || region == "" || deploymentName == "" {
		logging.Warn("Cluster Director: not exporting, the blueprint must set project_id, region and deployment_name as literal values")
		return Spec{}, false, nil
	}
	if blueprintHasGKE(bp) {
		logging.Info("Cluster Director: skipping export for GKE deployment %q (exporting GKE clusters is not supported yet)", deploymentName)
		return Spec{}, false, nil
	}

	states, err := gatherTerraformStates(ctx, deplRoot, bp, include)
	if err != nil {
		return Spec{}, false, err
	}
	if len(states) == 0 {
		logging.Warn("Cluster Director: not exporting, no deployed Terraform resources found")
		return Spec{}, false, nil
	}

	d := DiscoverWithOptions(states, DiscoverOptions{IgnoreBucketsFromModules: internalBucketModules(bp)})
	if len(d.ContainerClusters) > 0 {
		logging.Info("Cluster Director: skipping export for GKE deployment %q (exporting GKE clusters is not supported yet)", deploymentName)
		return Spec{}, false, nil
	}

	return buildSpecFromDiscovered(bp, d), true, nil
}

func buildSpecFromStates(bp config.Blueprint, states []*tfjson.State) Spec {
	d := DiscoverWithOptions(states, DiscoverOptions{IgnoreBucketsFromModules: internalBucketModules(bp)})
	return buildSpecFromDiscovered(bp, d)
}

func buildSpecFromDiscovered(bp config.Blueprint, d Discovered) Spec {
	projectID := stringVar(bp, "project_id")
	region := stringVar(bp, "region")
	deploymentName := stringVar(bp, "deployment_name")

	if others := d.OtherRegionSubnetworks(region); len(others) > 0 {
		logging.Warn("Cluster Director: not exporting subnetworks outside region %s: %v", region, others)
	}

	return Spec{
		ProjectID:      projectID,
		Region:         region,
		ClusterName:    resolveBlueprintClusterID(bp, deploymentName, d.MIGs),
		DeploymentName: deploymentName,
		ClusterLabels:  clusterLabelsFromBlueprint(bp),
		Networks:       d.NetworkPairs(region),
		Buckets:        d.Buckets,
		Filestores:     d.Filestores,
		Lustres:        d.Lustres,
		MIGs:           d.MIGs,
	}
}

// clusterLabelsFromBlueprint returns the labels for the exported cluster: the
// blueprint's global labels (`vars.labels`) with `ghpc_blueprint`,
// `ghpc_deployment` and `imported_from_cluster_toolkit: "true"` enforced on top
// so custom `vars.labels` cannot override ownership metadata.
func clusterLabelsFromBlueprint(bp config.Blueprint) map[string]string {
	labels := map[string]string{}
	copyBlueprintVarLabels(bp, labels)
	if bp.BlueprintName != "" {
		labels[BlueprintLabel] = bp.BlueprintName
	}
	if depl := stringVar(bp, "deployment_name"); depl != "" {
		labels[DeploymentLabel] = depl
	}
	labels[ImportedFromToolkitLabel] = "true"
	return labels
}

func copyBlueprintVarLabels(bp config.Blueprint, dst map[string]string) {
	if !bp.Vars.Has("labels") {
		return
	}
	raw := bp.Vars.Get("labels")
	evaluated, err := bp.Eval(raw)
	if err != nil {
		evaluated = raw
	}
	evaluated, _ = evaluated.Unmark()
	if evaluated.IsNull() || !evaluated.IsKnown() || (!evaluated.Type().IsObjectType() && !evaluated.Type().IsMapType()) {
		return
	}
	for k, v := range evaluated.AsValueMap() {
		v, _ = v.Unmark()
		if v.IsNull() || !v.IsKnown() || v.Type() != cty.String || v.AsString() == "" {
			continue
		}
		dst[k] = v.AsString()
	}
}

func resolveBlueprintClusterID(bp config.Blueprint, deploymentName string, discoveredMIGs []string) string {
	migs := discoveredMIGs
	if len(migs) == 0 && blueprintHasMIG(bp) {
		migs = []string{SanitizeClusterID(deploymentName) + "-mig"}
	}
	return ResolveClusterID(deploymentName, migs)
}

// effectiveExportDecision resolves whether export is enabled and whether it was
// explicitly set to false. When neither Override nor Blueprint sets a value
// (both nil), an existing marker keeps the deployment exported rather than
// treating an unset field as an opt-out.
func effectiveExportDecision(bp config.Blueprint, override *bool, hasMarker bool) (enabled bool, explicitlyDisabled bool) {
	if override != nil {
		return *override, !*override
	}
	if bp.ExportToClusterDirector != nil {
		return *bp.ExportToClusterDirector, !*bp.ExportToClusterDirector
	}
	if hasMarker {
		return true, false
	}
	return false, false
}

// verifyClusterOwnership ensures that an existing Cluster Director record
// belongs to deploymentName before gcluster overwrites or deletes it.
func verifyClusterOwnership(existing *Cluster, clusterName, deploymentName string) error {
	if existing == nil || deploymentName == "" {
		return nil
	}
	if owner := ClusterOwner(existing); owner != "" {
		if owner != deploymentName {
			return fmt.Errorf("cluster %q in Cluster Director belongs to deployment %q, not %q; refusing to modify or delete it",
				clusterName, owner, deploymentName)
		}
		return nil
	}
	if hasClusterContent(existing) {
		return fmt.Errorf("cluster %q in Cluster Director was not created by deployment %q (missing %q ownership label); refusing to modify or delete it",
			clusterName, deploymentName, DeploymentLabel)
	}
	return nil
}

func hasClusterContent(c *Cluster) bool {
	return c.Name != "" || c.Description != "" || len(c.Labels) > 0 || c.Orchestrator != nil || len(c.NetworkResources) > 0 || len(c.StorageResources) > 0
}

// Export is called by `gcluster deploy` once the infrastructure is up.
// Failures fail the command: an export that did not happen must never look
// like one that did.
func Export(ctx context.Context, opts ExportOptions) error {
	prevMarker, hasPrevMarker, prevErr := readMarker(opts.ArtifactsDir)
	if prevErr != nil {
		return prevErr
	}
	enabled, explicitlyDisabled := effectiveExportDecision(opts.Blueprint, opts.Override, hasPrevMarker)
	if !enabled {
		if explicitlyDisabled && hasPrevMarker {
			logging.Info("Cluster Director disabled (export_to_cluster_director=false); deleting cluster from Cluster Director...")
			return deleteExportedCluster(ctx, opts.ArtifactsDir, opts.Blueprint, prevMarker)
		}
		return nil
	}
	// --only and --skip are not consulted. The exported cluster reflects the
	// current state of all deployed groups in the deployment, not just the
	// subset of groups applied by this invocation.
	spec, ok, err := buildSpecFromDeployment(ctx, opts.DeplRoot, opts.Blueprint, allGroups)
	if err != nil {
		return fmt.Errorf("cannot export to Cluster Director: %w", err)
	}
	if !ok {
		if hasPrevMarker {
			logging.Info("Deployment is no longer exportable to Cluster Director; deleting previous export %q...", prevMarker.ClusterName)
			return deleteExportedCluster(ctx, opts.ArtifactsDir, opts.Blueprint, prevMarker)
		}
		return nil
	}
	prev, skipCrossNets := reconcileWithPrevMarker(&spec, prevMarker, hasPrevMarker)

	cluster, err := spec.Cluster()
	if err != nil {
		return fmt.Errorf(`cannot export %q to Cluster Director: %w

The infrastructure was deployed successfully and has NOT been rolled back.
Correct the blueprint and retry, or set export_to_cluster_director: false in the
blueprint (or pass --export-to-cluster-director=false) to skip exporting`, spec.ClusterName, err)
	}
	if err := writeRequestPayload(opts.ArtifactsDir, cluster); err != nil {
		logging.Info("Cluster Director: could not write request payload artifact %s: %v", requestPath(opts.ArtifactsDir), err)
	}

	return exportCluster(ctx, opts.DeplRoot, opts.ArtifactsDir, spec, cluster, prev, skipCrossNets, opts.Blueprint)
}

func sameLocationAsSpec(m Marker, spec Spec) bool {
	return (m.ProjectID == "" || m.ProjectID == spec.ProjectID) &&
		(m.Region == "" || m.Region == spec.Region)
}

func reconcileWithPrevMarker(spec *Spec, prevMarker Marker, hasPrev bool) (*SupersededCluster, bool) {
	if !hasPrev {
		return nil, false
	}
	sameLoc := sameLocationAsSpec(prevMarker, *spec)
	skipCrossNets := prevMarker.SkipCrossProjectNetworks && (prevMarker.ProjectID == "" || prevMarker.ProjectID == spec.ProjectID)
	if skipCrossNets {
		_, spec.Networks = splitCrossProjectNetworks(spec.ProjectID, spec.Networks)
	}
	if sameLoc && prevMarker.ClusterName != "" && prevMarker.ClusterName != spec.ClusterName && !MIGHasClusterIDPrefix(prevMarker.ClusterName, spec.MIGs) {
		spec.ClusterName = prevMarker.ClusterName
	}
	if prevMarker.ClusterName != "" && (prevMarker.ClusterName != spec.ClusterName || !sameLoc) {
		return &SupersededCluster{
			ClusterName:    prevMarker.ClusterName,
			DeploymentName: prevMarker.DeploymentName,
			ProjectID:      prevMarker.ProjectID,
			Region:         prevMarker.Region,
		}, skipCrossNets
	}
	return prevMarker.Superseded, skipCrossNets
}

func exportCluster(ctx context.Context, deplRoot, artifactsDir string, spec Spec, cluster *Cluster, prev *SupersededCluster, skipCrossNets bool, bp config.Blueprint) error {
	client, err := cdClient(ctx, spec.ProjectID)
	if err != nil {
		return err
	}
	m := Marker{
		ClusterName:              spec.ClusterName,
		DeploymentName:           spec.DeploymentName,
		ProjectID:                spec.ProjectID,
		Region:                   spec.Region,
		SkipCrossProjectNetworks: skipCrossNets,
		Superseded:               prev,
	}
	if err := createOrUpdateCluster(ctx, client, deplRoot, artifactsDir, spec, cluster, m); err != nil {
		var retryErr error
		m, retryErr = retryWithoutCrossProjectNetworks(ctx, client, deplRoot, artifactsDir, spec, err, m)
		if retryErr != nil {
			return retryErr
		}
	}
	if m.Superseded != nil && cleanupSupersededCluster(ctx, *m.Superseded, bp) {
		m.Superseded = nil
		_ = writeMarker(artifactsDir, m)
	}

	logExportedNodeCount(ctx, client, spec)
	logging.Info("View cluster in Cluster Director: %s", ConsoleURL(spec.ProjectID, spec.Region, spec.ClusterName))
	return nil
}

func resourceProject(path string) string {
	parts := strings.Split(path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "projects" {
			return parts[i+1]
		}
	}
	return ""
}

func splitCrossProjectNetworks(projectID string, pairs []NetworkPair) ([]NetworkPair, []NetworkPair) {
	var cross, inProject []NetworkPair
	for _, p := range pairs {
		netProj := resourceProject(p.Network)
		subProj := resourceProject(p.Subnetwork)
		if (netProj != "" && netProj != projectID) || (subProj != "" && subProj != projectID) {
			cross = append(cross, p)
			continue
		}
		inProject = append(inProject, p)
	}
	return cross, inProject
}

// serverRejectionText returns only the server-authored error message/body from
// a 400/403 APIError or a failed OperationError, omitting client-constructed
// URLs, operation names, and local filesystem paths so project-prefix checks
// cannot match the deployment's own project ID in the request URL.
func serverRejectionText(err error) (string, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode != http.StatusBadRequest && apiErr.StatusCode != http.StatusForbidden {
			return "", false
		}
		if apiErr.Status != nil && apiErr.Status.Message != "" {
			return apiErr.Status.Message + "\n" + apiErr.Body, true
		}
		return apiErr.Body, true
	}
	var opErr *OperationError
	if errors.As(err, &opErr) && opErr.Status != nil {
		return opErr.Status.Message, true
	}
	return "", false
}

func isResourceIDContChar(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c == '-' || c == '_'
}

func containsBoundedToken(raw, token string) bool {
	if token == "" {
		return false
	}
	for start := 0; start < len(raw); {
		idx := strings.Index(raw[start:], token)
		if idx < 0 {
			return false
		}
		end := start + idx + len(token)
		if end == len(raw) || !isResourceIDContChar(raw[end]) {
			return true
		}
		start += idx + 1
	}
	return false
}

func mentionsCrossProjectNetwork(raw string, cross []NetworkPair) bool {
	for _, p := range cross {
		if mentionsNetworkPair(raw, p) {
			return true
		}
	}
	return false
}

func mentionsNetworkPair(raw string, p NetworkPair) bool {
	for _, res := range []string{p.Network, p.Subnetwork} {
		if containsBoundedToken(raw, res) {
			return true
		}
		if proj := resourceProject(res); proj != "" && containsBoundedToken(raw, "projects/"+proj) {
			return true
		}
	}
	return false
}

func isCrossProjectNetworkRejection(err error, cross []NetworkPair) bool {
	if err == nil || len(cross) == 0 {
		return false
	}
	raw, ok := serverRejectionText(err)
	if !ok {
		return false
	}
	return mentionsCrossProjectNetwork(raw, cross)
}

func retryWithoutCrossProjectNetworks(ctx context.Context, client *Client, deplRoot, artifactsDir string, spec Spec, origErr error, m Marker) (Marker, error) {
	cross, inProject := splitCrossProjectNetworks(spec.ProjectID, spec.Networks)
	if !isCrossProjectNetworkRejection(origErr, cross) {
		return m, origErr
	}
	cause := origErr
	if u := errors.Unwrap(origErr); u != nil {
		cause = u
	}
	logging.Warn("Cluster Director: cross-project Shared VPC network(s) %v were rejected (%v); retrying export without cross-project networks", cross, cause)
	fallbackSpec := spec
	fallbackSpec.Networks = inProject
	fallbackCluster, err := fallbackSpec.Cluster()
	if err != nil {
		return m, origErr
	}
	if writeErr := writeRequestPayload(artifactsDir, fallbackCluster); writeErr != nil {
		logging.Info("Cluster Director: could not write request payload artifact %s: %v", requestPath(artifactsDir), writeErr)
	}
	// Only persist SkipCrossProjectNetworks=true after the fallback succeeds so
	// a failing fallback does not permanently drop Shared VPC networks.
	if err := createOrUpdateCluster(ctx, client, deplRoot, artifactsDir, fallbackSpec, fallbackCluster, m); err != nil {
		return m, err
	}
	m.SkipCrossProjectNetworks = true
	if writeErr := writeMarker(artifactsDir, m); writeErr != nil {
		return m, fmt.Errorf("exported %q without cross-project networks, but could not update marker: %w", fallbackSpec.ClusterName, writeErr)
	}
	return m, nil
}

func cleanupSupersededCluster(ctx context.Context, prev SupersededCluster, bp config.Blueprint) bool {
	client, err := cdClient(ctx, prev.ProjectID)
	if err == nil {
		prevMarker := Marker{
			ClusterName:    prev.ClusterName,
			DeploymentName: prev.DeploymentName,
			ProjectID:      prev.ProjectID,
			Region:         prev.Region,
		}
		var exists bool
		exists, err = verifyRemoteClusterForDelete(ctx, client, prevMarker, bp)
		if err == nil && !exists {
			return true
		}
		if err == nil {
			logging.Info("Deleting previously exported Cluster Director cluster %q (project %q, region %q)...", prev.ClusterName, prev.ProjectID, prev.Region)
			var op *Operation
			if op, err = client.DeleteCluster(ctx, prev.ProjectID, prev.Region, prev.ClusterName); err == nil {
				err = waitClusterDirectorOp(ctx, client, op)
			}
		}
	}
	if err != nil && !IsNotFound(err) {
		logging.Warn(`could not delete the previously exported Cluster Director cluster %q: %v
Delete it by hand with:
  gcloud alpha cluster-director clusters delete %s --project=%s --location=%s`,
			prev.ClusterName, err, prev.ClusterName, prev.ProjectID, prev.Region)
		return false
	}
	return true
}

func logExportedNodeCount(ctx context.Context, client *Client, spec Spec) {
	nodes, err := client.ListNodes(ctx, spec.ProjectID, spec.Region, spec.ClusterName)
	if err != nil {
		logging.Info("Cluster Director: could not verify node count for cluster %q via ListNodes: %v", spec.ClusterName, err)
		return
	}
	if len(nodes) > 0 {
		logging.Info("Cluster Director: discovered %d compute node(s) in cluster %q.", len(nodes), spec.ClusterName)
		return
	}
	logging.Info("Cluster Director: cluster %q currently reports 0 compute nodes via ListNodes (newly created instances may take a few minutes to appear; otherwise verify that instances in %s are running and carry label %s=%s).",
		spec.ClusterName, spec.Region, DeploymentLabel, spec.DeploymentName)
}

func waitClusterDirectorOp(ctx context.Context, client *Client, op *Operation) error {
	if op != nil && op.Name != "" && !op.Done {
		logging.Info("Waiting for Cluster Director operation %s (this typically takes 1-3 minutes)...", op.Name)
	}
	_, err := client.WaitForOperation(ctx, op, cdPollInterval, cdWaitTimeout)
	return err
}

func formatExportError(spec Spec, deplRoot string, err error) error {
	return fmt.Errorf(`could not export %q to Cluster Director: %w%s

The infrastructure was deployed successfully and has NOT been rolled back.
Retry exporting with:
  gcluster deploy %s --export-to-cluster-director=true
or set export_to_cluster_director: false in the blueprint (or pass --export-to-cluster-director=false) to skip exporting`,
		spec.ClusterName, err, accessHintSection(err, spec.ProjectID), deplRoot)
}

func formatUpdateError(spec Spec, err error) error {
	return fmt.Errorf(`could not update the Cluster Director export for %q: %w%s

The infrastructure was deployed successfully and has NOT been rolled back.
The cluster is still exported, but with its previous resource set`,
		spec.ClusterName, err, accessHintSection(err, spec.ProjectID))
}

// createOrUpdateCluster creates the Cluster Director cluster or updates it on
// ALREADY_EXISTS. The marker is written as soon as CreateCluster is accepted
// (before polling the LRO) and on ALREADY_EXISTS once ownership is verified so
// an interrupted or timed-out operation can still be cleaned up by destroy.
func createOrUpdateCluster(ctx context.Context, client *Client, deplRoot, artifactsDir string, spec Spec, cluster *Cluster, m Marker) error {
	op, err := client.CreateCluster(ctx, spec.ProjectID, spec.Region, spec.ClusterName, cluster)
	switch {
	case err == nil:
		if writeErr := writeMarker(artifactsDir, m); writeErr != nil {
			return fmt.Errorf("started exporting %q to Cluster Director, but could not record marker: %w", spec.ClusterName, writeErr)
		}
		logging.Info("Exporting deployment %q to Cluster Director as cluster %q...", spec.DeploymentName, spec.ClusterName)
		if err := waitClusterDirectorOp(ctx, client, op); err != nil {
			return formatExportError(spec, deplRoot, fmt.Errorf("export of %q did not complete: %w", spec.ClusterName, err))
		}
		logging.Info("Exported cluster %q to Cluster Director.", spec.ClusterName)
		return nil
	case IsAlreadyExists(err):
		return updateExistingClusterOnDeploy(ctx, client, artifactsDir, spec, cluster, m)
	default:
		return formatExportError(spec, deplRoot, err)
	}
}

func updateExistingClusterOnDeploy(ctx context.Context, client *Client, artifactsDir string, spec Spec, cluster *Cluster, m Marker) error {
	existing, getErr := client.GetCluster(ctx, spec.ProjectID, spec.Region, spec.ClusterName)
	if getErr != nil {
		return fmt.Errorf("could not inspect existing Cluster Director cluster %q: %w%s",
			spec.ClusterName, getErr, accessHintSection(getErr, spec.ProjectID))
	}
	if err := verifyClusterOwnership(existing, spec.ClusterName, spec.DeploymentName); err != nil {
		return err
	}
	if writeErr := writeMarker(artifactsDir, m); writeErr != nil {
		return fmt.Errorf("verified existing export %q, but could not record marker: %w", spec.ClusterName, writeErr)
	}
	if SameResources(existing, cluster) {
		logging.Info("Cluster %q is already exported to Cluster Director and up to date.", spec.ClusterName)
		return nil
	}
	logging.Info("Cluster %q is already exported to Cluster Director, updating its resource set...", spec.ClusterName)
	op, err := client.UpdateCluster(ctx, spec.ProjectID, spec.Region, spec.ClusterName, cluster, UpdateResourceMask)
	if err != nil {
		return formatUpdateError(spec, err)
	}
	if err := waitClusterDirectorOp(ctx, client, op); err != nil {
		return formatUpdateError(spec, fmt.Errorf("update of %q did not complete: %w", spec.ClusterName, err))
	}
	logging.Info("Updated exported cluster %q in Cluster Director.", spec.ClusterName)
	return nil
}

func allSelectedTerraformSkipped(bp config.Blueprint, isSelected func(config.GroupName) bool, skippedGroups map[config.GroupName]bool) bool {
	if len(skippedGroups) == 0 {
		return false
	}
	selected := 0
	for _, g := range bp.Groups {
		if g.Kind() != config.TerraformKind || !isSelected(g.Name) {
			continue
		}
		selected++
		if !skippedGroups[g.Name] {
			return false
		}
	}
	return selected > 0
}

func hasTerraformGroupMatching(bp config.Blueprint, match func(config.GroupName) bool) bool {
	for _, g := range bp.Groups {
		if g.Kind() == config.TerraformKind && match(g.Name) {
			return true
		}
	}
	return false
}

func verifyRemoteClusterForDelete(ctx context.Context, client *Client, m Marker, bp config.Blueprint) (bool, error) {
	ownerDeployment := m.DeploymentName
	if ownerDeployment == "" {
		ownerDeployment = stringVar(bp, "deployment_name")
	}
	if ownerDeployment == "" {
		return true, nil
	}
	existing, getErr := client.GetCluster(ctx, m.ProjectID, m.Region, m.ClusterName)
	if IsNotFound(getErr) {
		logging.Info("Cluster %q is not found in Cluster Director, nothing to do.", m.ClusterName)
		return false, nil
	}
	if getErr != nil {
		return false, fmt.Errorf(`could not inspect %q in Cluster Director before deletion: %w%s

Delete the exported cluster by hand with:
  gcloud alpha cluster-director clusters delete %s --project=%s --location=%s`,
			m.ClusterName, getErr, accessHintSection(getErr, m.ProjectID), m.ClusterName, m.ProjectID, m.Region)
	}
	if err := verifyClusterOwnership(existing, m.ClusterName, ownerDeployment); err != nil {
		return false, err
	}
	return true, nil
}

// Delete removes or shrinks the Cluster Director export recorded in
// `<opts.ArtifactsDir>/cluster-director-export.json` after `gcluster destroy`
// has torn down the selected Terraform groups.
func Delete(ctx context.Context, opts DeleteOptions) error {
	isSelected := opts.IsGroupSelected
	if isSelected == nil {
		isSelected = allGroups
	}
	if allSelectedTerraformSkipped(opts.Blueprint, isSelected, opts.SkippedGroups) {
		return nil
	}
	m, ok, err := readMarker(opts.ArtifactsDir)
	if err != nil || !ok {
		return err
	}

	surviving := func(n config.GroupName) bool {
		return !isSelected(n) || opts.SkippedGroups[n]
	}
	enabled, _ := effectiveExportDecision(opts.Blueprint, opts.Override, true)
	if enabled && hasTerraformGroupMatching(opts.Blueprint, surviving) {
		survivingStates, err := gatherTerraformStates(ctx, opts.DeplRoot, opts.Blueprint, surviving)
		if err != nil {
			return fmt.Errorf(`cannot verify surviving Terraform state during partial destroy: %w

Delete the exported cluster by hand if all resources were removed:
  gcloud alpha cluster-director clusters delete %s --project=%s --location=%s`,
				err, m.ClusterName, m.ProjectID, m.Region)
		}
		if len(survivingStates) > 0 {
			return shrinkClusterDirectorExport(ctx, opts.ArtifactsDir, opts.Blueprint, m, survivingStates)
		}
	}

	return deleteExportedCluster(ctx, opts.ArtifactsDir, opts.Blueprint, m)
}

func formatDeleteError(m Marker, err error) error {
	return fmt.Errorf(`could not delete %q from Cluster Director: %w%s

Delete the exported cluster by hand with:
  gcloud alpha cluster-director clusters delete %s --project=%s --location=%s`,
		m.ClusterName, err, accessHintSection(err, m.ProjectID), m.ClusterName, m.ProjectID, m.Region)
}

func deleteExportedCluster(ctx context.Context, artifactsDir string, bp config.Blueprint, m Marker) error {
	client, err := cdClient(ctx, m.ProjectID)
	if err != nil {
		return formatDeleteError(m, err)
	}

	exists, err := verifyRemoteClusterForDelete(ctx, client, m, bp)
	if err != nil {
		return err
	}
	if !exists {
		if m.Superseded != nil {
			cleanupSupersededCluster(ctx, *m.Superseded, bp)
		}
		if err := removeMarker(artifactsDir); err != nil {
			return fmt.Errorf("could not clear Cluster Director marker for %q: %w", m.ClusterName, err)
		}
		return nil
	}

	logging.Info("Deleting exported cluster %q from Cluster Director (project %q, region %q)...", m.ClusterName, m.ProjectID, m.Region)
	op, err := client.DeleteCluster(ctx, m.ProjectID, m.Region, m.ClusterName)
	switch {
	case err == nil:
		if err := waitClusterDirectorOp(ctx, client, op); err != nil {
			return formatDeleteError(m, fmt.Errorf("deletion of %q from Cluster Director did not complete: %w", m.ClusterName, err))
		}
		logging.Info("Deleted exported cluster %q from Cluster Director.", m.ClusterName)
	case IsNotFound(err):
		logging.Info("Cluster %q is not found in Cluster Director, nothing to do.", m.ClusterName)
	default:
		return formatDeleteError(m, err)
	}

	if m.Superseded != nil {
		cleanupSupersededCluster(ctx, *m.Superseded, bp)
	}
	if err := removeMarker(artifactsDir); err != nil {
		return fmt.Errorf("deleted %q from Cluster Director but could not clear its record: %w", m.ClusterName, err)
	}
	return nil
}

func shrinkClusterDirectorExport(ctx context.Context, artifactsDir string, bp config.Blueprint, m Marker, survivingStates []*tfjson.State) error {
	if stringVar(bp, "project_id") == "" || stringVar(bp, "region") == "" || stringVar(bp, "deployment_name") == "" {
		return nil
	}
	spec := buildSpecFromStates(bp, survivingStates)
	if m.ClusterName != "" {
		spec.ClusterName = m.ClusterName
	}
	if m.SkipCrossProjectNetworks {
		_, spec.Networks = splitCrossProjectNetworks(spec.ProjectID, spec.Networks)
	}
	client, err := cdClient(ctx, m.ProjectID)
	if err != nil {
		return formatShrinkError(m, "could not update the Cluster Director export", "Resolve the error and retry", err)
	}
	if err := applyShrinkSpec(ctx, client, artifactsDir, bp, m, spec); err != nil {
		cross, inProject := splitCrossProjectNetworks(spec.ProjectID, spec.Networks)
		if !isCrossProjectNetworkRejection(err, cross) {
			return err
		}
		logging.Warn("Cluster Director: cross-project Shared VPC network(s) %v were rejected (%v); retrying shrink without cross-project networks", cross, err)
		spec.Networks = inProject
		if retryErr := applyShrinkSpec(ctx, client, artifactsDir, bp, m, spec); retryErr != nil {
			return retryErr
		}
		m.SkipCrossProjectNetworks = true
		_ = writeMarker(artifactsDir, m)
	}
	return nil
}

func applyShrinkSpec(ctx context.Context, client *Client, artifactsDir string, bp config.Blueprint, m Marker, spec Spec) error {
	cluster, err := spec.Cluster()
	if err != nil {
		return formatShrinkError(m, "cannot update the Cluster Director export", "Correct the blueprint and retry", err)
	}
	if err := writeRequestPayload(artifactsDir, cluster); err != nil {
		logging.Info("Cluster Director: could not write request payload artifact %s: %v", requestPath(artifactsDir), err)
	}

	unchanged, err := inspectClusterBeforeShrink(ctx, client, bp, m, cluster)
	if err != nil || unchanged {
		return err
	}

	logging.Info("Partial destroy: shrinking the Cluster Director export for %q to the surviving resources...", m.ClusterName)
	op, err := client.UpdateCluster(ctx, m.ProjectID, m.Region, m.ClusterName, cluster, UpdateResourceMask)
	switch {
	case err == nil:
		if err := waitClusterDirectorOp(ctx, client, op); err != nil {
			return formatShrinkError(m, "could not update the Cluster Director export", "Resolve the error and retry", fmt.Errorf("update of %q did not complete: %w", m.ClusterName, err))
		}
		logging.Info("Updated exported cluster %q in Cluster Director.", m.ClusterName)
		logging.Info("View cluster in Cluster Director: %s", ConsoleURL(m.ProjectID, m.Region, m.ClusterName))
	case IsNotFound(err):
		logging.Info("Cluster %q is not found in Cluster Director, nothing to update.", m.ClusterName)
	default:
		return formatShrinkError(m, "could not update the Cluster Director export", "Resolve the error and retry", err)
	}
	return nil
}

func inspectClusterBeforeShrink(ctx context.Context, client *Client, bp config.Blueprint, m Marker, desired *Cluster) (bool, error) {
	existing, getErr := client.GetCluster(ctx, m.ProjectID, m.Region, m.ClusterName)
	if IsNotFound(getErr) {
		logging.Info("Cluster %q is not found in Cluster Director, nothing to update.", m.ClusterName)
		return true, nil
	}
	if getErr != nil {
		return false, formatShrinkError(m, "could not update the Cluster Director export", "Resolve the error and retry", getErr)
	}
	ownerDeployment := m.DeploymentName
	if ownerDeployment == "" {
		ownerDeployment = stringVar(bp, "deployment_name")
	}
	if err := verifyClusterOwnership(existing, m.ClusterName, ownerDeployment); err != nil {
		return false, err
	}
	if SameResources(existing, desired) {
		logging.Info("Cluster %q in Cluster Director is already up to date with the surviving resources.", m.ClusterName)
		return true, nil
	}
	return false, nil
}

func formatShrinkError(m Marker, prefix, action string, err error) error {
	return fmt.Errorf(`%s for %q: %w%s

%s, or delete the exported cluster by hand with:
  gcloud alpha cluster-director clusters delete %s --project=%s --location=%s`,
		prefix, m.ClusterName, err, accessHintSection(err, m.ProjectID), action, m.ClusterName, m.ProjectID, m.Region)
}

func accessHintSection(err error, projectID string) string {
	if h := AccessHint(err, projectID); h != "" {
		return "\n\n" + h
	}
	return ""
}

func cdClient(ctx context.Context, projectID string) (*Client, error) {
	return newCdClient(ctx,
		WithQuotaProject(projectID),
		WithUserAgent("cluster-toolkit/"+config.GetToolkitVersion()))
}
