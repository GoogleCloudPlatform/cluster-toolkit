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

package gke

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
)

func (g *GKEOrchestrator) isNAPEnabledForMachineType(machineType, zone string) (bool, error) {
	if !g.napEnabled {
		return false, nil
	}

	resolvedType := config.ResolveMachineType(machineType)

	if config.IsTPU(resolvedType) {
		return g.validateTPUNAPLimit(resolvedType)
	}

	cap, err := g.FetchMachineCapabilities(resolvedType, zone)
	if err != nil {
		return false, err
	}
	if len(cap.Accelerators) > 0 {
		return g.validateGPUNAPLimit(resolvedType, cap)
	}

	return g.napLimits["cpu"] > 0, nil
}

func (g *GKEOrchestrator) validateTPUNAPLimit(resolvedType string) (bool, error) {
	key := strings.ToLower(g.GenerateGKENodeSelectorLabel(resolvedType))
	if limit, exists := g.napLimits[key]; exists {
		return limit > 0, nil
	}
	// Fallback to generic TPU limit ONLY if no specific TPU limits are configured.
	for k := range g.napLimits {
		if isSpecificTPUKey(k) {
			return false, nil
		}
	}
	return g.napLimits["google.com/tpu"] > 0, nil
}

func (g *GKEOrchestrator) validateGPUNAPLimit(resolvedType string, cap MachineTypeCap) (bool, error) {
	key := strings.ToLower(g.GenerateGKENodeSelectorLabel(resolvedType))
	if strings.EqualFold(key, resolvedType) {
		key = strings.ToLower(g.GenerateGKENodeSelectorLabel(cap.Accelerators[0].Type))
	}
	if !isKnownGKEAccelerator(key) {
		return false, fmt.Errorf("unknown accelerator label: %q", cap.Accelerators[0].Type)
	}
	if limit, exists := g.napLimits[key]; exists {
		return limit > 0, nil
	}
	// Fallback to generic GPU limit ONLY if no specific GPU limits are configured.
	for k := range g.napLimits {
		if isSpecificGPUKey(k) {
			return false, nil
		}
	}
	return g.napLimits["nvidia.com/gpu"] > 0, nil
}

func (g *GKEOrchestrator) getConfiguredLimitsError(computeType string) error {
	var configuredLimits []string
	for k, v := range g.napLimits {
		if v > 0 {
			configuredLimits = append(configuredLimits, k)
		}
	}
	sort.Strings(configuredLimits)
	return fmt.Errorf("workload submission rejected. Compute type %q is not configured within your cluster's Node Auto-Provisioning (NAP) limits. Configured limits on cluster: %s", computeType, strings.Join(configuredLimits, ", "))
}

func (g *GKEOrchestrator) validateConsumptionForStaticCluster(job *orchestrator.JobDefinition) error {
	if job.GKENAPProvisioning == "" {
		return nil
	}

	if !g.napEnabled {
		return fmt.Errorf("GKE NAP provisioning options (--gke-nap-provisioning %q, --gke-nap-reservation %q) are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled. The current cluster does not have NAP enabled.\nRemediation: Enable Node Auto-Provisioning on your cluster to use these options, or submit your job without them", job.GKENAPProvisioning, job.GKENAPReservation)
	}

	// NAP flags were requested. Validate strictly against GKE NAP limits.
	isNAP, err := g.isNAPEnabledForMachineType(job.MachineType, job.ClusterLocation)
	if err != nil {
		return err
	}
	if !isNAP {
		return g.getConfiguredLimitsError(job.ComputeType)
	}

	return g.validateNAPReservation(job)
}

// validateNAPReservation verifies that the requested GCE reservation exists and is compatible.
func (g *GKEOrchestrator) validateNAPReservation(job *orchestrator.JobDefinition) error {
	if job.GKENAPProvisioning != "reservation" || job.GKENAPReservation == "" {
		return nil
	}

	res := parseReservationURI(job.GKENAPReservation)
	if res.Name == "" {
		return fmt.Errorf("invalid reservation URI %q: unable to determine reservation name", job.GKENAPReservation)
	}

	if job.DryRunManifest != "" {
		return nil
	}

	clusterRegion := shell.ExtractRegion(job.ClusterLocation)
	if res.Zone != "" && clusterRegion != "" {
		resRegion := shell.ExtractRegion(res.Zone)
		if resRegion != "" && !strings.EqualFold(resRegion, clusterRegion) {
			return fmt.Errorf("reservation %q belongs to zone %q (region %q), but cluster is in region %q", res.Name, res.Zone, resRegion, clusterRegion)
		}
	}

	projectID := res.Project
	if projectID == "" {
		projectID = job.ProjectID
	}

	zone := res.Zone
	if zone == "" && len(strings.Split(job.ClusterLocation, "-")) == 3 {
		zone = job.ClusterLocation
	}

	if zone != "" {
		return g.validateZonalReservation(res.Name, zone, projectID, job.MachineType)
	}

	return g.validateRegionalReservation(res.Name, job.ClusterLocation, projectID, job.MachineType)
}

func (g *GKEOrchestrator) validateZonalReservation(name, zone, projectID, expectedMachineType string) error {
	resCmd := g.executor.ExecuteCommand("gcloud", "compute", "reservations", "describe", name, "--zone="+zone, "--project="+projectID, "--format=json")
	if resCmd.ExitCode != 0 {
		stderrLower := strings.ToLower(resCmd.Stderr)
		if strings.Contains(stderrLower, "not found") || strings.Contains(stderrLower, "404") {
			return fmt.Errorf("reservation %q does not exist in zone %s (project %s)", name, zone, projectID)
		}
		if isPermissionDenied(stderrLower) {
			logging.Warn("Permission denied querying reservation %q in zone %s (project %s): %s. Proceeding without reservation validation.", name, zone, projectID, resCmd.Stderr)
			return nil
		}
		return fmt.Errorf("failed to describe reservation %q: %s", name, resCmd.Stderr)
	}

	var rData struct {
		SpecificReservation struct {
			InstanceProperties struct {
				MachineType string `json:"machineType"`
			} `json:"instanceProperties"`
		} `json:"specificReservation"`
	}
	if err := json.Unmarshal([]byte(resCmd.Stdout), &rData); err != nil {
		return fmt.Errorf("failed to parse reservation describe json: %w", err)
	}
	var resMachineType string
	if rawMT := rData.SpecificReservation.InstanceProperties.MachineType; rawMT != "" {
		resMachineType = path.Base(rawMT)
	}
	var expected string
	if expectedMachineType != "" {
		expected = path.Base(expectedMachineType)
	}
	if resMachineType != "" && expected != "" && resMachineType != expected {
		return fmt.Errorf("reservation %q is configured for machine type %q, which does not match requested machine type %q", name, resMachineType, expectedMachineType)
	}
	return nil
}

func (g *GKEOrchestrator) validateRegionalReservation(name, clusterLocation, projectID, expectedMachineType string) error {
	filter := fmt.Sprintf("name=( '%s' )", name)
	listCmd := g.executor.ExecuteCommand("gcloud", "compute", "reservations", "list", "--project="+projectID, "--filter="+filter, "--format=json")
	if listCmd.ExitCode != 0 {
		if isPermissionDenied(strings.ToLower(listCmd.Stderr)) {
			logging.Warn("Permission denied listing reservations for %q in project %s: %s. Proceeding without reservation validation.", name, projectID, listCmd.Stderr)
			return nil
		}
		return fmt.Errorf("failed to list reservations for %q: %s", name, listCmd.Stderr)
	}

	var listData []reservationListItem
	if err := json.Unmarshal([]byte(listCmd.Stdout), &listData); err != nil {
		return fmt.Errorf("failed to parse reservations list json: %w", err)
	}

	if len(listData) == 0 {
		return fmt.Errorf("reservation %q does not exist in project %s", name, projectID)
	}

	return checkRegionalReservationMatch(name, clusterLocation, expectedMachineType, listData)
}

func checkRegionalReservationMatch(name, clusterLocation, expectedMachineType string, listData []reservationListItem) error {
	clusterRegion := shell.ExtractRegion(clusterLocation)
	var expected string
	if expectedMachineType != "" {
		expected = path.Base(expectedMachineType)
	}

	foundInRegion, mismatched := findRegionalReservationMismatches(listData, clusterRegion, expected)
	if foundInRegion {
		if len(mismatched) == 0 {
			return nil
		}
		return fmt.Errorf("reservation %q is configured for machine type(s) %v, which does not match requested machine type %q", name, mismatched, expectedMachineType)
	}

	if clusterRegion != "" {
		return fmt.Errorf("reservation %q was found in zones %v, which do not match cluster region %q", name, collectItemZones(listData), clusterRegion)
	}
	return nil
}

func findRegionalReservationMismatches(listData []reservationListItem, clusterRegion, expected string) (bool, []string) {
	var foundInRegion bool
	var mismatched []string
	for _, item := range listData {
		if item.Zone == "" || (clusterRegion != "" && !strings.HasPrefix(path.Base(item.Zone), clusterRegion)) {
			continue
		}
		foundInRegion = true
		var resMT string
		if rawMT := item.SpecificReservation.InstanceProperties.MachineType; rawMT != "" {
			resMT = path.Base(rawMT)
		}
		if resMT != "" && expected != "" && resMT == expected {
			return true, nil
		}
		if resMT != "" {
			mismatched = append(mismatched, resMT)
		}
	}
	return foundInRegion, mismatched
}

func collectItemZones(listData []reservationListItem) []string {
	var zones []string
	for _, item := range listData {
		if item.Zone != "" {
			zones = append(zones, path.Base(item.Zone))
		}
	}
	return zones
}

// resolveFallbackName returns a fallback reservation name from the URI parts.
func resolveFallbackName(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	// If it has reservationBlocks, the name is the one before it
	for i, part := range parts {
		if strings.EqualFold(part, "reservationBlocks") && i > 0 {
			return parts[i-1]
		}
	}
	// Fallback to last element, unless it's the collection name itself
	last := parts[len(parts)-1]
	if strings.EqualFold(last, "reservations") {
		return ""
	}
	return last
}

// parseReservationURI parses a GCE reservation resource URI or reservation path into its components.
// E.g.,
// - "my-res" -> Name: "my-res"
// - "projects/my-project/reservations/my-res" -> Project: "my-project", Name: "my-res"
// - "projects/my-project/zones/us-central1-a/reservations/my-res" -> Project: "my-project", Zone: "us-central1-a", Name: "my-res"
// - "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/reservations/my-res" -> Project: "my-project", Zone: "us-central1-a", Name: "my-res"
// - "projects/my-project/reservations/my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2" -> Project: "my-project", Name: "my-res", Block: "block-1", Subblock: "subblock-2"
func parseReservationURI(resName string) parsedReservation {
	resName = strings.TrimSuffix(strings.TrimSpace(resName), "/")
	if !strings.Contains(resName, "/") {
		return parsedReservation{Name: strings.ToLower(resName)}
	}

	name := extractURIPart(resName, "reservations")
	if name == "" {
		name = resolveFallbackName(strings.Split(resName, "/"))
	}
	subblock := extractURIPart(resName, "reservationsubblocks")
	if subblock == "" {
		subblock = extractURIPart(resName, "subblocks")
	}

	return parsedReservation{
		Project:  strings.ToLower(extractURIPart(resName, "projects")),
		Zone:     strings.ToLower(extractURIPart(resName, "zones")),
		Name:     strings.ToLower(name),
		Block:    strings.ToLower(extractURIPart(resName, "reservationblocks")),
		Subblock: strings.ToLower(subblock),
	}
}

func isSpecificGPUKey(key string) bool {
	return strings.HasPrefix(key, "nvidia-")
}

func isSpecificTPUKey(key string) bool {
	return strings.HasPrefix(key, "tpu-") || strings.HasPrefix(key, "tpu7")
}

func isKnownGKEAccelerator(key string) bool {
	switch key {
	case "nvidia-tesla-t4", "nvidia-tesla-v100":
		return true
	}
	for _, val := range config.GetMachineMappings().MachineFamilyToLabelMap {
		if val == key {
			return true
		}
	}
	return false
}

func parseNAPLimits(autoscaling gkeClusterAutoscaling) map[string]int64 {
	limits := make(map[string]int64)
	for _, rl := range autoscaling.ResourceLimits {
		limits[rl.ResourceType] = rl.Maximum
	}

	for _, rl := range autoscaling.ResourceLimits {
		resName := rl.ResourceType
		maxVal := rl.Maximum
		if resName == "gpu" || strings.Contains(resName, "nvidia") {
			if maxVal > limits["nvidia.com/gpu"] {
				limits["nvidia.com/gpu"] = maxVal
			}
		} else if strings.Contains(resName, "tpu") {
			if maxVal > limits["google.com/tpu"] {
				limits["google.com/tpu"] = maxVal
			}
		}
	}
	return limits
}

func isNonAcceleratorResource(resName string) bool {
	switch resName {
	case "cpu", "memory", "gpu", "tpu", "pods", "storage", "ephemeral-storage":
		return true
	default:
		return false
	}
}

func resolveFlavorFromResource(resName string) (string, map[string]string, error) {
	var flavorName string
	var nodeLabels map[string]string

	switch {
	case resName == "google.com/tpu":
		flavorName = "flavor-tpu-generic"
	case resName == "nvidia.com/gpu":
		flavorName = "flavor-nvidia-generic"
	case strings.HasPrefix(resName, "nvidia-"):
		flavorName = "flavor-" + resName
		nodeLabels = map[string]string{
			"cloud.google.com/gke-accelerator": resName,
		}
	case strings.HasPrefix(resName, "tpu-") || strings.HasPrefix(resName, "tpu7"):
		flavorName = "flavor-" + resName
		nodeLabels = map[string]string{
			"cloud.google.com/gke-tpu-accelerator": resName,
		}
	default:
		return "", nil, fmt.Errorf("unknown accelerator label %q", resName)
	}

	return flavorName, nodeLabels, nil
}

func (g *GKEOrchestrator) populateNAPFlavors(flavors map[string]FlavorCapacity) error {
	if !g.napEnabled {
		return nil
	}

	for resName, maxLimit := range g.napLimits {
		if maxLimit <= 0 {
			continue
		}
		if isNonAcceleratorResource(resName) {
			continue
		}

		flavorName, nodeLabels, err := resolveFlavorFromResource(resName)
		if err != nil {
			return err
		}

		if _, ok := flavors[flavorName]; !ok {
			flavors[flavorName] = FlavorCapacity{
				NodeLabels: nodeLabels,
			}
		}
	}
	return nil
}

// resolveTPUWorkloadPolicy resolves or auto-creates a GCE HIGH_THROUGHPUT workload policy
// required for multi-host TPU 7x Node Auto-Provisioning (NAP) to successfully scale from 0.
// Resolution order:
//  1. Check existing TPU 7x node pools in the cluster for an attached placement policy.
//  2. Check the regional resource policy cache or query GCE for a policy matching the topology.
//  3. If missing and not in dry-run mode, auto-create the canonical workload policy.
func (g *GKEOrchestrator) resolveTPUWorkloadPolicy(machineType, topology, clusterLocation, projectID string, isDryRun bool) (string, error) {
	if topology == "" {
		return "", fmt.Errorf("cannot resolve TPU workload policy: topology is empty")
	}

	if policy := g.findPolicyInClusterNodePools(machineType, topology); policy != "" {
		return policy, nil
	}

	canonicalPolicyName := getCanonicalTPUWorkloadPolicyName(topology)
	if policy := g.getCachedTPUWorkloadPolicy(canonicalPolicyName, topology); policy != "" {
		return policy, nil
	}

	region, resolvedProjID := g.resolvePolicyRegionAndProject(clusterLocation, projectID)
	if region == "" || resolvedProjID == "" {
		if isDryRun {
			logging.Info("Dry-run: Could not determine cluster region or project. Assuming workload policy %q for topology %s. Ensure it exists in your target cluster's region.", canonicalPolicyName, topology)
		}
		return canonicalPolicyName, nil
	}

	if isDryRun {
		logging.Info("Dry-run: Assuming workload policy %q for topology %s. Ensure it exists before applying the manifest, or create it using:\n  gcloud compute resource-policies create workload-policy %s --region=%s --project=%s --type=HIGH_THROUGHPUT --accelerator-topology=%s", canonicalPolicyName, topology, canonicalPolicyName, region, resolvedProjID, topology)
		return canonicalPolicyName, nil
	}

	policy, err := g.discoverRegionalWorkloadPolicy(canonicalPolicyName, region, resolvedProjID, topology)
	if err != nil {
		if errors.Is(err, ErrResourcePolicyPermissionDenied) {
			logging.Warn("Permission denied reading workload policy %q in region %s. Assuming canonical policy name %q. If job fails to schedule, verify with an administrator that the policy exists in region %s.", canonicalPolicyName, region, canonicalPolicyName, region)
			return canonicalPolicyName, nil
		}
		return "", err
	}
	if policy != "" {
		return policy, nil
	}

	if !isDryRun {
		if err := g.createTPUWorkloadPolicy(canonicalPolicyName, region, resolvedProjID, topology); err != nil {
			return "", err
		}
	} else {
		logging.Info("Dry-run: Workload policy %q for topology %s was not found. Please ensure it exists before applying the manifest, or create it using:\n  gcloud compute resource-policies create workload-policy %s --region=%s --project=%s --type=HIGH_THROUGHPUT --accelerator-topology=%s", canonicalPolicyName, topology, canonicalPolicyName, region, resolvedProjID, topology)
	}

	return canonicalPolicyName, nil
}

// isStaticWorkloadPolicyMode checks whether the accelerator topology mode provides a
// static, pre-connected accelerator topology suitable for non-dynamic workloads.
// In GCE, standard static workload policies omit this field (defaulting to auto-connected)
// or explicitly specify "AUTO_CONNECT".
func isStaticWorkloadPolicyMode(mode string) bool {
	trimmed := strings.TrimSpace(mode)
	return trimmed == "" || strings.EqualFold(trimmed, "AUTO_CONNECT")
}

func (g *GKEOrchestrator) getCachedTPUWorkloadPolicy(canonicalPolicyName, topology string) string {
	if g.resourcePolicyCache == nil {
		return ""
	}
	cached := g.resourcePolicyCache[canonicalPolicyName]
	if cached != nil && strings.EqualFold(cached.Type, "HIGH_THROUGHPUT") && cached.AcceleratorTopology == topology && isStaticWorkloadPolicyMode(cached.AcceleratorTopologyMode) {
		return canonicalPolicyName
	}
	return ""
}

// findPolicyInClusterNodePools inspects the cluster's existing node pools for an active
// placement policy matching the requested machine type and TPU topology.
// Non-static policies (such as dynamic slicing PROVISION_ONLY) are skipped for static workloads.
func (g *GKEOrchestrator) findPolicyInClusterNodePools(machineType, topology string) string {
	for _, np := range g.clusterDesc.NodePools {
		if strings.EqualFold(np.Config.MachineType, machineType) && np.PlacementPolicy != nil && np.PlacementPolicy.PolicyName != "" {
			mode := np.PlacementPolicy.AcceleratorTopologyMode
			if mode == "" && g.resourcePolicyCache != nil {
				if cached := g.resourcePolicyCache[path.Base(np.PlacementPolicy.PolicyName)]; cached != nil {
					mode = cached.AcceleratorTopologyMode
				}
			}
			if !isStaticWorkloadPolicyMode(mode) {
				continue // Skip non-static (e.g. PROVISION_ONLY) policies for static workloads
			}
			topo := np.PlacementPolicy.TpuTopology
			if topo == "" && np.Config.Labels != nil {
				topo = np.Config.Labels[tpuTopologyLabel]
			}
			if topo != "" && topo == topology {
				policy := path.Base(np.PlacementPolicy.PolicyName)
				logging.Info("Discovered matching placement policy %q from existing node pool %q", policy, np.Name)
				return policy
			}
		}
	}
	return ""
}

// getCanonicalTPUWorkloadPolicyName generates the deterministic GCE resource policy name
// for a given TPU 7x topology (e.g., "tpu7x-16-2x2x2-placement-policy").
func getCanonicalTPUWorkloadPolicyName(topology string) string {
	chips := calculateChipsFromTopology(topology)
	tensorcores := chips * 2
	return fmt.Sprintf("tpu7x-%d-%s-placement-policy", tensorcores, topology)
}

// resolvePolicyRegionAndProject determines the target GCP region and project ID
// from the cluster location, cluster description, or orchestrator state.
func (g *GKEOrchestrator) resolvePolicyRegionAndProject(clusterLocation, projectID string) (string, string) {
	region := ""
	if clusterLocation != "" {
		region = shell.ExtractRegion(clusterLocation)
	} else if len(g.clusterDesc.Locations) > 0 {
		region = shell.ExtractRegion(g.clusterDesc.Locations[0])
	} else if len(g.clusterZones) > 0 {
		region = shell.ExtractRegion(g.clusterZones[0])
	}
	if projectID == "" {
		projectID = g.projectID
	}
	return region, projectID
}

// discoverRegionalWorkloadPolicy searches for an existing HIGH_THROUGHPUT workload policy
// in the specified GCP region and project. It first checks for the canonical policy name,
// and if not found or incompatible, falls back to querying regional policies matching the topology.
// Only policies with static auto-connected topology modes (empty or AUTO_CONNECT) are matched.
func (g *GKEOrchestrator) discoverRegionalWorkloadPolicy(canonicalPolicyName, region, projectID, topology string) (string, error) {
	policy, err := g.describeResourcePolicyCached(canonicalPolicyName, region, projectID)
	if err != nil {
		return "", err
	}
	if policy != nil {
		if strings.EqualFold(policy.Type, "HIGH_THROUGHPUT") && policy.AcceleratorTopology == topology && isStaticWorkloadPolicyMode(policy.AcceleratorTopologyMode) {
			logging.Info("Discovered existing workload policy %q in region %s", canonicalPolicyName, region)
			return canonicalPolicyName, nil
		}
		logging.Warn("Found policy %q in region %s, but its type (%q), mode (%q), or topology (%q) does not match expected (HIGH_THROUGHPUT, static auto-connected, %s). Skipping it.",
			canonicalPolicyName, region, policy.Type, policy.AcceleratorTopologyMode, policy.AcceleratorTopology, topology)
	}

	filter := fmt.Sprintf("region:( %s ) AND workloadPolicy.acceleratorTopology=%s AND workloadPolicy.type=HIGH_THROUGHPUT", region, topology)
	listRes := g.executor.ExecuteCommand("gcloud", "compute", "resource-policies", "list", "--project="+projectID, "--filter="+filter, "--format=value(name,workloadPolicy.acceleratorTopologyMode)")
	if listRes.ExitCode == 0 && strings.TrimSpace(listRes.Stdout) != "" {
		for _, line := range strings.Split(strings.TrimSpace(listRes.Stdout), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			name := fields[0]
			mode := ""
			if len(fields) > 1 {
				mode = fields[1]
			}
			if isStaticWorkloadPolicyMode(mode) {
				if g.resourcePolicyCache == nil {
					g.resourcePolicyCache = make(map[string]*GCEWorkloadPolicy)
				}
				g.resourcePolicyCache[name] = &GCEWorkloadPolicy{
					Name:                    name,
					Region:                  region,
					AcceleratorTopology:     topology,
					AcceleratorTopologyMode: mode,
					Type:                    "HIGH_THROUGHPUT",
				}
				logging.Info("Discovered matching workload policy %q for topology %s in region %s", name, topology, region)
				return name, nil
			}
		}
	}
	return "", nil
}

// createTPUWorkloadPolicy executes 'gcloud compute resource-policies create workload-policy'
// to provision a new HIGH_THROUGHPUT workload policy with the requested accelerator topology.
// Handles concurrent creation (HTTP 409 / already exists) gracefully.
func (g *GKEOrchestrator) createTPUWorkloadPolicy(canonicalPolicyName, region, projectID, topology string) error {
	logging.Info("Workload policy for topology %s not found. Creating workload policy %q...", topology, canonicalPolicyName)
	createRes := g.executor.ExecuteCommand("gcloud", "compute", "resource-policies", "create", "workload-policy", canonicalPolicyName, "--region="+region, "--project="+projectID, "--type=HIGH_THROUGHPUT", "--accelerator-topology="+topology)
	if createRes.ExitCode != 0 {
		stderrLower := strings.ToLower(createRes.Stderr)
		if strings.Contains(stderrLower, "already exists") || strings.Contains(stderrLower, "409") {
			if cached := g.resourcePolicyCache[canonicalPolicyName]; cached != nil && (!strings.EqualFold(cached.Type, "HIGH_THROUGHPUT") || cached.AcceleratorTopology != topology || !isStaticWorkloadPolicyMode(cached.AcceleratorTopologyMode)) {
				return fmt.Errorf("a resource policy named %q already exists in region %s with incompatible settings (type=%q, topology=%q, mode=%q); expected HIGH_THROUGHPUT, static auto-connected, and %s. Please remove or rename the existing policy, or specify an existing valid placement policy via --placement-policy",
					canonicalPolicyName, region, cached.Type, cached.AcceleratorTopology, cached.AcceleratorTopologyMode, topology)
			}
			logging.Info("Workload policy %q was concurrently created.", canonicalPolicyName)
		} else {
			return fmt.Errorf("failed to create required workload policy %q in region %s: %s\nRemediation: Ensure your GCP credentials have 'compute.resourcePolicies.create' permission (e.g. 'roles/compute.admin') or create the policy manually:\n  gcloud compute resource-policies create workload-policy %s --region=%s --project=%s --type=HIGH_THROUGHPUT --accelerator-topology=%s",
				canonicalPolicyName, region, createRes.Stderr, canonicalPolicyName, region, projectID, topology)
		}
	} else {
		logging.Info("Successfully created workload policy %q in region %s", canonicalPolicyName, region)
	}

	if g.resourcePolicyCache == nil {
		g.resourcePolicyCache = make(map[string]*GCEWorkloadPolicy)
	}
	g.resourcePolicyCache[canonicalPolicyName] = &GCEWorkloadPolicy{
		Name:                canonicalPolicyName,
		Region:              region,
		AcceleratorTopology: topology,
		Type:                "HIGH_THROUGHPUT",
	}
	return nil
}
