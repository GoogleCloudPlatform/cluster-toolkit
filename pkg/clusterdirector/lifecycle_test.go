/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusterdirector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/sourcereader"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

func withFakeCdClient(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := newCdClient
	newCdClient = func(ctx context.Context, _ ...Option) (*Client, error) {
		return NewClient(ctx, WithEndpoint(srv.URL), WithHTTPClient(srv.Client()))
	}
	t.Cleanup(func() {
		newCdClient = prev
		srv.Close()
	})
}

func withFakeTerraformState(t *testing.T, fn func(groupDir string) (*tfjson.State, error)) {
	t.Helper()
	prev := readGroupTerraformState
	readGroupTerraformState = func(_ context.Context, groupDir string) (*tfjson.State, error) {
		return fn(groupDir)
	}
	t.Cleanup(func() { readGroupTerraformState = prev })
}

func withFakeRemoteInit(t *testing.T, fn func(groupDir string) (*tfjson.State, error)) {
	t.Helper()
	prev := initRemoteGroupState
	initRemoteGroupState = func(_ context.Context, groupDir string) (*tfjson.State, error) {
		return fn(groupDir)
	}
	t.Cleanup(func() { initRemoteGroupState = prev })
}

func withFastPolling(t *testing.T) {
	t.Helper()
	prev := cdPollInterval
	cdPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { cdPollInterval = prev })
}

func tfGroup(name string) config.Group {
	return config.Group{
		Name:    config.GroupName(name),
		Modules: []config.Module{{Kind: config.TerraformKind}},
	}
}

func testBlueprint(zone string) config.Blueprint {
	enabled := true
	bp := config.Blueprint{
		Groups:                  []config.Group{tfGroup("network"), tfGroup("compute")},
		ExportToClusterDirector: &enabled,
	}
	vars := map[string]cty.Value{
		"project_id":      cty.StringVal("my-project"),
		"region":          cty.StringVal("us-central1"),
		"deployment_name": cty.StringVal("ctkdemo"),
	}
	if zone != "" {
		vars["zone"] = cty.StringVal(zone)
	}
	bp.Vars = config.NewDict(vars)
	return bp
}

func groupSelector(only, skip []string) func(config.GroupName) bool {
	return func(n config.GroupName) bool {
		if len(only) > 0 {
			return slices.Contains(only, string(n))
		}
		if len(skip) > 0 {
			return !slices.Contains(skip, string(n))
		}
		return true
	}
}

func TestMarkerRoundTripAndRemove(t *testing.T) {
	dir := t.TempDir()
	want := Marker{
		ClusterName: "ctkdemo",
		ProjectID:   "my-project",
		Region:      "us-central1",
	}
	if err := writeMarker(dir, want); err != nil {
		t.Fatalf("writeMarker() failed: %v", err)
	}

	got, ok, err := readMarker(dir)
	if err != nil {
		t.Fatalf("readMarker() failed: %v", err)
	}
	if !ok {
		t.Fatal("readMarker() reported no marker, want the one just written")
	}
	if got != want {
		t.Errorf("readMarker() = %+v, want %+v", got, want)
	}

	if err := removeMarker(dir); err != nil {
		t.Fatalf("removeMarker() failed: %v", err)
	}
	if err := removeMarker(dir); err != nil {
		t.Fatalf("removeMarker() on absent file failed: %v", err)
	}
}

func TestMarkerAbsentIsNotAnError(t *testing.T) {
	_, ok, err := readMarker(t.TempDir())
	if err != nil {
		t.Errorf("readMarker() on a fresh dir failed: %v", err)
	}
	if ok {
		t.Error("readMarker() found a marker in a fresh dir, want none")
	}
}

func TestMarkerMalformedIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, cdMarkerFile), []byte("{not json"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, _, err := readMarker(dir); err == nil {
		t.Error("readMarker() succeeded on a corrupt marker, want an error")
	}
}

func TestStringVar(t *testing.T) {
	bp := config.Blueprint{}
	bp.Vars = config.NewDict(map[string]cty.Value{
		"project_id": cty.StringVal("my-project"),
		"count":      cty.NumberIntVal(3),
		"nothing":    cty.NullVal(cty.String),
	})

	for _, tc := range []struct {
		key  string
		want string
	}{
		{"project_id", "my-project"},
		{"count", ""},
		{"nothing", ""},
		{"absent", ""},
	} {
		if got := stringVar(bp, tc.key); got != tc.want {
			t.Errorf("stringVar(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestBuildSpecSkipsWhenBlueprintCannotBeExported(t *testing.T) {
	bp := config.Blueprint{}
	bp.Vars = config.NewDict(map[string]cty.Value{
		"deployment_name": cty.StringVal("depl"),
	})

	if _, ok, err := buildSpecFromDeployment(context.Background(), t.TempDir(), bp, allGroups); err != nil || ok {
		t.Errorf("buildSpecFromDeployment() = (_, %t, %v), want (spec, false, nil)", ok, err)
	}
}

func TestBuildSpecSkipsGKEContainerClusterDeployments(t *testing.T) {
	bp := testBlueprint("us-central1-a")
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
			Resources: []*tfjson.StateResource{
				{Mode: tfjson.ManagedResourceMode, Type: "google_container_cluster", AttributeValues: map[string]any{"name": "gke-cluster"}},
				{Mode: tfjson.ManagedResourceMode, Type: "google_container_node_pool", AttributeValues: map[string]any{
					"name": "pool-1",
					"managed_instance_group_urls": []any{
						"https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instanceGroups/gke-pool-1-grp",
					},
				}},
			},
		}}}, nil
	})

	if _, ok, err := buildSpecFromDeployment(context.Background(), t.TempDir(), bp, allGroups); err != nil || ok {
		t.Errorf("buildSpecFromDeployment() on GKE deployment = (_, %t, %v), want skipped (false, nil)", ok, err)
	}
}

func TestBuildSpecUsesDeploymentNameAndDiscoversAmbiguousNetworks(t *testing.T) {
	bp := config.Blueprint{Groups: []config.Group{tfGroup("primary")}}
	bp.Vars = config.NewDict(map[string]cty.Value{
		"project_id":      cty.StringVal("my-project"),
		"region":          cty.StringVal("us-central1"),
		"zone":            cty.StringVal("us-central1-a"),
		"deployment_name": cty.StringVal("My_Deployment"),
	})

	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
			Resources: []*tfjson.StateResource{
				{Mode: tfjson.ManagedResourceMode, Type: "google_compute_network", AttributeValues: map[string]any{"name": "net-b"}},
				{Mode: tfjson.ManagedResourceMode, Type: "google_compute_network", AttributeValues: map[string]any{"name": "net-a"}},
				{Mode: tfjson.ManagedResourceMode, Type: "google_compute_subnetwork", AttributeValues: map[string]any{"project": "p", "name": "sub-b", "region": "us-central1", "network": "net-b"}},
				{Mode: tfjson.ManagedResourceMode, Type: "google_compute_subnetwork", AttributeValues: map[string]any{"project": "p", "name": "sub-a", "region": "us-central1", "network": "net-a"}},
				{Mode: tfjson.ManagedResourceMode, Type: "google_storage_bucket", AttributeValues: map[string]any{"name": "bkt-1"}},
			},
		}}}, nil
	})

	spec, ok, err := buildSpecFromDeployment(context.Background(), t.TempDir(), bp, allGroups)
	if err != nil {
		t.Fatalf("buildSpecFromDeployment() failed: %v", err)
	}
	if !ok {
		t.Fatal("buildSpecFromDeployment() skipped an exportable deployment")
	}
	if spec.DeploymentName != "My_Deployment" {
		t.Errorf("DeploymentName = %q, want %q", spec.DeploymentName, "My_Deployment")
	}
	if spec.ClusterName == "" || strings.ContainsAny(spec.ClusterName, "_ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Errorf("ClusterName = %q, want it sanitized from the deployment name", spec.ClusterName)
	}
	wantFirst := NetworkPair{Network: "projects/p/global/networks/net-a", Subnetwork: "projects/p/regions/us-central1/subnetworks/sub-a"}
	if len(spec.Networks) != 2 || spec.Networks[0] != wantFirst {
		t.Errorf("Networks = %+v, want 2 pairs starting with %+v", spec.Networks, wantFirst)
	}
	if !slices.Equal(spec.Buckets, []string{"bkt-1"}) {
		t.Errorf("Buckets = %v, want [bkt-1]", spec.Buckets)
	}
}

func TestInternalModuleBucketsAreSkippedInBuildSpec(t *testing.T) {
	bp := testBlueprint("us-central1-a")
	bp.Groups = []config.Group{{
		Name: "primary",
		Modules: []config.Module{
			{ID: "slurm_controller", Kind: config.TerraformKind, Source: "community/modules/scheduler/schedmd-slurm-gcp-v6-controller"},
			{ID: "startup", Kind: config.TerraformKind, Source: "modules/scripts/startup-script"},
			{ID: "vm", Kind: config.TerraformKind, Source: "github.com/GoogleCloudPlatform/cluster-toolkit//modules/compute/vm-instance?ref=v1.109.0"},
			{ID: "gcs_data", Kind: config.TerraformKind, Source: "modules/file-system/cloud-storage-bucket"},
			{ID: "local_user_mod", Kind: config.TerraformKind, Source: "./modules/storage/my-bucket"},
			{ID: "third_party_mod", Kind: config.TerraformKind, Source: "github.com/acme/infra//modules/gcs/bucket"},
		},
	}}
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
			ChildModules: []*tfjson.StateModule{
				{
					Address: "module.slurm_controller",
					ChildModules: []*tfjson.StateModule{{
						Address:   "module.slurm_controller.module.slurm_files",
						Resources: []*tfjson.StateResource{{Mode: tfjson.ManagedResourceMode, Type: tfBucket, AttributeValues: map[string]any{"name": "slurm-devel-cfg"}}},
					}},
				},
				{
					Address:   "module.startup",
					Resources: []*tfjson.StateResource{{Mode: tfjson.ManagedResourceMode, Type: tfBucket, AttributeValues: map[string]any{"name": "startup-cfg"}}},
				},
				{
					Address:   "module.vm",
					Resources: []*tfjson.StateResource{{Mode: tfjson.ManagedResourceMode, Type: tfBucket, AttributeValues: map[string]any{"name": "vm-scripts-cfg"}}},
				},
				{
					Address:   "module.gcs_data",
					Resources: []*tfjson.StateResource{{Mode: tfjson.ManagedResourceMode, Type: tfBucket, AttributeValues: map[string]any{"name": "user-dataset"}}},
				},
				{
					Address:   "module.local_user_mod",
					Resources: []*tfjson.StateResource{{Mode: tfjson.ManagedResourceMode, Type: tfBucket, AttributeValues: map[string]any{"name": "local-user-bucket"}}},
				},
				{
					Address:   "module.third_party_mod",
					Resources: []*tfjson.StateResource{{Mode: tfjson.ManagedResourceMode, Type: tfBucket, AttributeValues: map[string]any{"name": "third-party-bucket"}}},
				},
			},
		}}}, nil
	})

	spec, ok, err := buildSpecFromDeployment(context.Background(), t.TempDir(), bp, allGroups)
	if err != nil || !ok {
		t.Fatalf("buildSpecFromDeployment() = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	want := []string{"local-user-bucket", "third-party-bucket", "user-dataset"}
	if !slices.Equal(spec.Buckets, want) {
		t.Errorf("spec.Buckets = %v, want %v (built-in non-storage buckets skipped, user/third-party modules preserved)", spec.Buckets, want)
	}
}

func fakeDeployedState() *tfjson.State {
	return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			{Mode: tfjson.ManagedResourceMode, Type: "google_compute_instance", AttributeValues: map[string]any{"name": "vm-1"}},
		},
	}}}
}

func TestExportClusterDirectorLifecycle(t *testing.T) {
	t.Run("blueprint export_to_cluster_director=false skips export on fresh deploy and deletes existing record when previously exported", testExportBlueprintFalseSkipsAndDeletesExisting)
	t.Run("unset export_to_cluster_director preserves and updates existing marker instead of deleting", testExportUnsetSettingPreservesAndUpdatesExistingMarker)
	t.Run("unexportable blueprint skips quietly", testExportUnexportableBlueprintSkipsQuietly)
	t.Run("no deployed Terraform resources skips export", testExportNoDeployedTerraformResources)
	t.Run("unreadable Terraform state fails export instead of silently dropping resources", testExportUnreadableTerraformStateFails)
	t.Run("invalid spec fails with guidance", testExportInvalidSpecFailsWithGuidance)
	t.Run("create succeeds and writes marker", testExportCreateSucceedsAndWritesMarker)
	t.Run("create writes marker before waiting on LRO so interrupted export is cleaned up on destroy", testCreateWritesMarkerBeforeWaitingOnOperation)
	t.Run("already exists triggers UpdateCluster when owned by same deployment", testExportAlreadyExistsTriggersUpdateCluster)
	t.Run("already exists refuses to overwrite cluster owned by colliding deployment name", testExportAlreadyExistsRefusesCollidingOwner)
	t.Run("already exists with identical resources skips UpdateCluster", testExportAlreadyExistsUnchangedSkipsUpdate)
	t.Run("already exists with failing UpdateCluster returns error and still writes marker", testExportAlreadyExistsFailingUpdateWritesMarker)
	t.Run("create API error fails without writing marker", testExportCreateAPIErrorFails)
}

func TestDeleteClusterDirectorLifecycle(t *testing.T) {
	t.Run("absent marker is a no-op", testDeleteAbsentMarkerIsANoOp)
	t.Run("corrupt marker fails", testDeleteCorruptMarkerFails)
	t.Run("full destroy deletes cluster and removes marker", testDeleteFullDestroyDeletesClusterAndRemovesMarker)
	t.Run("full destroy tolerates NOT_FOUND and removes marker", testDeleteFullDestroyToleratesNOTFOUNDAndRemoves)
	t.Run("full destroy API error fails and preserves marker", testDeleteFullDestroyAPIErrorFailsAndPreserves)
	t.Run("partial destroy shrinks cluster and keeps marker when surviving group has resources", testDeletePartialDestroyShrinksClusterAndKeepsMarker)
	t.Run("partial destroy shrinks even when export_to_cluster_director is unset in blueprint", testDeletePartialDestroyUnsetSettingStillShrinks)
	t.Run("partial destroy fails when surviving group state is unreadable instead of deleting cluster", testDeletePartialDestroyFailsWhenSurvivingGroupState)
	t.Run("partial destroy with identical surviving resources skips UpdateCluster", testDeletePartialDestroyWithIdenticalSurvivingResourcesSkips)
	t.Run("partial destroy deletes cluster and removes marker when unselected groups have no resources left", testDeletePartialDestroyDeletesClusterAndRemovesMarker)
	t.Run("partial destroy tolerates NOT_FOUND", testDeletePartialDestroyToleratesNOTFOUND)
	t.Run("partial destroy API error fails", testDeletePartialDestroyAPIErrorFails)
	t.Run("refuses to delete cluster owned by another deployment", testDeleteRefusesToDeleteClusterOwnedByAnother)
	t.Run("all selected groups skipped preserves export untouched", testDeleteAllSelectedGroupsSkippedPreservesExport)
	t.Run("one skipped group shrinks export to that group", testDeleteOneSkippedGroupShrinksClusterToSkippedGroup)
}

func TestGatherTerraformStatesConsultsTheGroupFilter(t *testing.T) {
	bp := config.Blueprint{Groups: []config.Group{
		{Name: "packer", Modules: []config.Module{{Kind: config.PackerKind}}},
		tfGroup("network"),
		tfGroup("compute"),
	}}

	var asked []string
	states, err := gatherTerraformStates(context.Background(), t.TempDir(), bp, func(n config.GroupName) bool {
		asked = append(asked, string(n))
		return false
	})
	if err != nil {
		t.Fatalf("gatherTerraformStates() failed: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("gatherTerraformStates() read %d states, want none when every group is filtered out", len(states))
	}
	if want := []string{"network", "compute"}; !slices.Equal(asked, want) {
		t.Errorf("filter consulted for %v, want %v", asked, want)
	}
}

func TestHasSurvivingTerraformGroup(t *testing.T) {
	bp := config.Blueprint{Groups: []config.Group{tfGroup("network"), tfGroup("compute")}}

	for _, tc := range []struct {
		name string
		only []string
		skip []string
		want bool
	}{
		{"a full destroy leaves nothing", nil, nil, false},
		{"--only one group leaves the other", []string{"compute"}, nil, true},
		{"--only every group is a full destroy", []string{"network", "compute"}, nil, false},
		{"--skip one group leaves that one", nil, []string{"network"}, true},
		{"--skip every group destroys nothing", nil, []string{"network", "compute"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sel := groupSelector(tc.only, tc.skip)
			surviving := func(n config.GroupName) bool { return !sel(n) }
			if got := hasTerraformGroupMatching(bp, surviving); got != tc.want {
				t.Errorf("hasTerraformGroupMatching() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasSurvivingTerraformGroupIgnoresNonTerraformGroups(t *testing.T) {
	bp := config.Blueprint{Groups: []config.Group{{
		Name:    "image",
		Modules: []config.Module{{Kind: config.PackerKind}},
	}}}
	if hasTerraformGroupMatching(bp, func(config.GroupName) bool { return true }) {
		t.Error("hasTerraformGroupMatching() = true for a Packer-only blueprint, want false")
	}
}

func TestDefaultReadGroupTerraformState(t *testing.T) {
	t.Run("uninitialized group without state returns errGroupNotDeployed", func(t *testing.T) {
		groupDir := t.TempDir()
		if _, err := defaultReadGroupTerraformState(context.Background(), groupDir); !errors.Is(err, errGroupNotDeployed) {
			t.Fatalf("defaultReadGroupTerraformState() on empty dir = %v, want errGroupNotDeployed", err)
		}
	})

	t.Run("corrupt local terraform.tfstate returns non-skipped error", func(t *testing.T) {
		groupDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(groupDir, "terraform.tfstate"), []byte(`{not valid json`), 0644)
		_, err := defaultReadGroupTerraformState(context.Background(), groupDir)
		if err == nil || errors.Is(err, errGroupNotDeployed) {
			t.Fatalf("defaultReadGroupTerraformState() on corrupt state = %v, want non-skipped error", err)
		}
	})

	t.Run("reads local v4 terraform.tfstate preserving module addresses", func(t *testing.T) {
		groupDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(groupDir, "main.tf"), []byte(`module "network" { source = "./missing" }`), 0644)
		_ = os.WriteFile(filepath.Join(groupDir, "terraform.tfstate"), []byte(`{
			"version": 4,
			"resources": [
				{
					"module": "module.network",
					"mode": "managed",
					"type": "google_compute_subnetwork",
					"name": "subnet",
					"instances": [{"attributes": {
						"project": "p",
						"name": "ctkjbvm-primary-subnet",
						"region": "us-central1",
						"network": "https://www.googleapis.com/compute/v1/projects/p/global/networks/ctkjbvm-net"
					}}]
				},
				{
					"module": "module.slurm_controller.module.slurm_files",
					"mode": "managed",
					"type": "google_storage_bucket",
					"name": "cfg",
					"instances": [{"attributes": {"name": "internal-slurm-bkt"}}]
				},
				{
					"module": "module.user_bkt",
					"mode": "managed",
					"type": "google_storage_bucket",
					"name": "bkt",
					"instances": [{"attributes": {"name": "user-bkt"}}]
				}
			]
		}`), 0644)

		st, err := defaultReadGroupTerraformState(context.Background(), groupDir)
		if err != nil {
			t.Fatalf("defaultReadGroupTerraformState() failed: %v", err)
		}
		d := DiscoverWithOptions([]*tfjson.State{st}, DiscoverOptions{
			IgnoreBucketsFromModules: map[string]bool{"slurm_controller": true},
		})
		pairs := d.NetworkPairs("us-central1")
		want := NetworkPair{Network: "projects/p/global/networks/ctkjbvm-net", Subnetwork: "projects/p/regions/us-central1/subnetworks/ctkjbvm-primary-subnet"}
		if len(pairs) != 1 || pairs[0] != want {
			t.Errorf("Discover() pairs = %+v, want [%+v]", pairs, want)
		}
		if !slices.Equal(d.Buckets, []string{"user-bkt"}) {
			t.Errorf("Discover() Buckets = %v, want [user-bkt] (internal-slurm-bkt skipped)", d.Buckets)
		}
	})
}

func TestStagedGKEBlueprintSkipsExportEvenBeforeGKEGroupApplied(t *testing.T) {
	bp := testBlueprint("us-central1-a")
	bp.Groups = []config.Group{
		tfGroup("network"),
		{
			Name: "gke",
			Modules: []config.Module{{
				ID:     "gke_cluster",
				Kind:   config.TerraformKind,
				Source: "modules/scheduler/gke-cluster",
			}},
		},
	}
	withFakeTerraformState(t, func(groupDir string) (*tfjson.State, error) {
		if strings.HasSuffix(groupDir, "/gke") {
			return nil, errGroupNotDeployed
		}
		return fakeDeployedState(), nil
	})
	if _, ok, err := buildSpecFromDeployment(context.Background(), t.TempDir(), bp, allGroups); err != nil || ok {
		t.Errorf("buildSpecFromDeployment() on staged GKE blueprint = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

func TestGatherTerraformStatesRemoteBackendInit(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return nil, errGroupNotDeployed
	})

	bp := testBlueprint("us-central1-a")
	bp.TerraformBackendDefaults = config.TerraformBackend{Type: "gcs"}
	withFakeRemoteInit(t, func(string) (*tfjson.State, error) {
		return &tfjson.State{}, nil
	})
	if states, err := gatherTerraformStates(context.Background(), t.TempDir(), bp, allGroups); err != nil || len(states) != 0 {
		t.Errorf("gatherTerraformStates() on empty remote state = (%d states, %v), want (0, nil)", len(states), err)
	}

	withFakeRemoteInit(t, func(groupDir string) (*tfjson.State, error) {
		if strings.HasSuffix(groupDir, "/network") {
			return fakeDeployedState(), nil
		}
		return &tfjson.State{}, nil
	})
	if states, err := gatherTerraformStates(context.Background(), t.TempDir(), bp, allGroups); err != nil || len(states) != 1 {
		t.Errorf("gatherTerraformStates() on populated remote state = (%d states, %v), want (1, nil)", len(states), err)
	}

	withFakeRemoteInit(t, func(string) (*tfjson.State, error) {
		return nil, errors.New("bucket does not exist")
	})
	if _, err := gatherTerraformStates(context.Background(), t.TempDir(), bp, allGroups); err == nil || !strings.Contains(err.Error(), "could not read remote Terraform state") {
		t.Errorf("gatherTerraformStates() on failing remote init = %v, want remote Terraform state error", err)
	}

	initCalled := false
	withFakeRemoteInit(t, func(string) (*tfjson.State, error) {
		initCalled = true
		return nil, errors.New("unexpected remote init")
	})
	bpLocalOverride := testBlueprint("us-central1-a")
	bpLocalOverride.TerraformBackendDefaults = config.TerraformBackend{Type: "gcs"}
	for i := range bpLocalOverride.Groups {
		bpLocalOverride.Groups[i].TerraformBackend = config.TerraformBackend{Type: "local"}
	}
	if states, err := gatherTerraformStates(context.Background(), t.TempDir(), bpLocalOverride, allGroups); err != nil || len(states) != 0 || initCalled {
		t.Errorf("gatherTerraformStates() on local override = (%d, %v, initCalled=%v), want (0, nil, false)", len(states), err, initCalled)
	}
}

func TestRequestPayloadArtifactAndListNodesPostExport(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	artDir := t.TempDir()
	listNodesCalled := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/clusters"):
			_, _ = io.WriteString(w, `{"name": "operations/op-1", "done": true}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/nodes"):
			listNodesCalled = true
			_, _ = io.WriteString(w, `{"nodes": [{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo/nodes/node-1"}]}`)
		}
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Export() failed: %v", err)
	}
	if !listNodesCalled {
		t.Error("expected ListNodes to be called after CreateCluster")
	}
	data, err := os.ReadFile(requestPath(artDir))
	if err != nil {
		t.Fatalf("expected %s to be written, got error: %v", requestPath(artDir), err)
	}
	if !strings.Contains(string(data), `"ghpc_deployment": "ctkdemo"`) || !strings.Contains(string(data), `"imported_from_cluster_toolkit": "true"`) {
		t.Errorf("unexpected %s content: %s", cdRequestFile, string(data))
	}
}

func TestExampleBlueprintsExportLabels(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"mocked": true}`)
	sourcereader.ModuleFS = os.DirFS("../..").(sourcereader.BaseFS)
	cases := []string{
		"../../examples/hpc-slurm.yaml",
		"../../examples/storage-vm.yaml",
		"../../examples/pfs-managed-lustre-vm.yaml",
	}
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	for _, bpPath := range cases {
		bp, _, err := config.NewBlueprint(bpPath)
		if err != nil {
			t.Fatalf("NewBlueprint(%q) failed: %v", bpPath, err)
		}
		bp.Vars = bp.Vars.With("project_id", cty.StringVal("my-project"))
		if err := bp.Expand(); err != nil && !strings.Contains(err.Error(), "failed to get machine type info") {
			t.Errorf("Expand(%q) failed: %v", bpPath, err)
		}
		assertExampleClusterLabels(t, bpPath, bp)
	}
}

func assertExampleClusterLabels(t *testing.T, bpPath string, bp config.Blueprint) {
	t.Helper()
	if blueprintHasGKE(bp) {
		return
	}
	spec, ok, err := buildSpecFromDeployment(context.Background(), t.TempDir(), bp, allGroups)
	if err != nil || !ok {
		t.Fatalf("buildSpecFromDeployment(%q) = (ok=%v, err=%v), want (true, nil)", bpPath, ok, err)
	}
	cluster, err := spec.Cluster()
	if err != nil {
		t.Fatalf("spec.Cluster(%q) failed: %v", bpPath, err)
	}
	if cluster.Labels[ImportedFromToolkitLabel] != "true" {
		t.Errorf("Cluster(%q).Labels[%q] = %q, want \"true\"", bpPath, ImportedFromToolkitLabel, cluster.Labels[ImportedFromToolkitLabel])
	}
	if cluster.Labels[BlueprintLabel] != bp.BlueprintName {
		t.Errorf("Cluster(%q).Labels[%q] = %q, want %q", bpPath, BlueprintLabel, cluster.Labels[BlueprintLabel], bp.BlueprintName)
	}
	if cluster.Labels[DeploymentLabel] == "" {
		t.Errorf("Cluster(%q).Labels[%q] is empty", bpPath, DeploymentLabel)
	}
}

func clusterLabelsTestBlueprint(t *testing.T) config.Blueprint {
	t.Helper()
	bp := config.Blueprint{
		BlueprintName:   "slurm-demo-bp",
		AddCreatorLabel: true,
		CreatorUsername: "alice",
		Groups: []config.Group{{
			Name: "primary",
			Modules: []config.Module{{
				ID:     "bkt",
				Kind:   config.TerraformKind,
				Source: "modules/file-system/cloud-storage-bucket",
			}},
		}},
	}
	bp.Vars = config.NewDict(map[string]cty.Value{
		"project_id":      cty.StringVal("my-project"),
		"region":          cty.StringVal("us-central1"),
		"zone":            cty.StringVal("us-central1-a"),
		"deployment_name": cty.StringVal("ctk-demo-1"),
		"labels": cty.ObjectVal(map[string]cty.Value{
			"env":                           cty.StringVal("demo"),
			"team":                          cty.StringVal("hpc"),
			"imported_from_cluster_toolkit": cty.StringVal("false"),
		}),
	})
	sourcereader.ModuleFS = os.DirFS("../..").(sourcereader.BaseFS)
	if err := bp.Expand(); err != nil {
		t.Fatalf("bp.Expand() failed: %v", err)
	}
	return bp
}

func checkLabels(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	for k, wantVal := range want {
		if gotVal := got[k]; gotVal != wantVal {
			t.Errorf("%s[%q] = %q, want %q (all: %v)", what, k, gotVal, wantVal, got)
		}
	}
}

func TestClusterLabelsFromBlueprint(t *testing.T) {
	bp := clusterLabelsTestBlueprint(t)

	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	spec, ok, err := buildSpecFromDeployment(context.Background(), t.TempDir(), bp, allGroups)
	if err != nil || !ok {
		t.Fatalf("buildSpecFromDeployment() = (_, %v, %v), want ok=true, err=nil", ok, err)
	}

	wantClusterLabels := map[string]string{
		"ghpc_blueprint":                "slurm-demo-bp",
		"ghpc_deployment":               "ctk-demo-1",
		"ghpc_creator":                  "alice",
		"imported_from_cluster_toolkit": "true",
		"env":                           "demo",
		"team":                          "hpc",
	}
	checkLabels(t, "spec.ClusterLabels", spec.ClusterLabels, wantClusterLabels)

	cluster, err := spec.Cluster()
	if err != nil {
		t.Fatalf("spec.Cluster() failed: %v", err)
	}
	if wantDesc := "Cluster Toolkit deployment: ctk-demo-1"; cluster.Description != wantDesc {
		t.Errorf("cluster.Description = %q, want %q", cluster.Description, wantDesc)
	}
	checkLabels(t, "cluster.Labels", cluster.Labels, wantClusterLabels)

	instLabels := cluster.Orchestrator.ComputeEngine.ExistingInstances["ctk-nodes"].Labels
	if len(instLabels) != 1 || instLabels[DeploymentLabel] != "ctk-demo-1" {
		t.Errorf("ExistingInstances[\"ctk-nodes\"].Labels = %v, want only {%q: %q}", instLabels, DeploymentLabel, "ctk-demo-1")
	}
}

func TestExportPatchesMissingImportedFromToolkitLabel(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	var patchBody, patchMask string
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "message": "exists", "status": "ALREADY_EXISTS"}}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{
				"name": "projects/my-project/locations/us-central1/clusters/ctkdemo",
				"labels": {"ghpc_deployment": "ctkdemo"},
				"orchestrator": {"computeEngine": {"existingInstances": {"ctk-nodes": {"project": "projects/my-project", "labels": {"ghpc_deployment": "ctkdemo"}}}}}
			}`)
		case http.MethodPatch:
			patchMask = r.URL.Query().Get("updateMask")
			b, _ := io.ReadAll(r.Body)
			patchBody = string(b)
			_, _ = io.WriteString(w, `{"name": "operations/op-patch", "done": true}`)
		}
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Export() on legacy cluster failed: %v", err)
	}
	if !strings.Contains(patchMask, "labels") || !strings.Contains(patchBody, `"imported_from_cluster_toolkit":"true"`) {
		t.Errorf("UpdateCluster mask=%q body=%s, want labels mask and imported_from_cluster_toolkit:true", patchMask, patchBody)
	}
	if !strings.Contains(patchMask, "description") || !strings.Contains(patchBody, `"description":"Cluster Toolkit deployment: ctkdemo"`) {
		t.Errorf("UpdateCluster mask=%q body=%s, want description mask and Cluster.description populated", patchMask, patchBody)
	}
}

func TestExportNeverStampsForeignCluster(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	patched := false
	artDir := t.TempDir()
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "message": "exists", "status": "ALREADY_EXISTS"}}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"team": "ml"}}`)
		case http.MethodPatch:
			patched = true
		}
	})
	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), `missing "ghpc_deployment" ownership label`) {
		t.Errorf("Export() on foreign cluster = %v, want missing ownership label error", err)
	}
	if patched {
		t.Error("UpdateCluster must not stamp imported_from_cluster_toolkit onto a cluster gcluster does not own")
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("marker must not be written when existing cluster belongs to another owner")
	}
}

func testExportBlueprintFalseSkipsAndDeletesExisting(t *testing.T) {
	bp := testBlueprint("us-central1-a")
	bp.SetExportToClusterDirector(false)
	artDir := t.TempDir()
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bp}); err != nil {
		t.Fatalf("Export() with export_to_cluster_director=false failed: %v", err)
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("expected marker not to be written when export_to_cluster_director=false")
	}

	if err := writeMarker(artDir, Marker{
		ClusterName:    "ctkdemo",
		DeploymentName: "ctkdemo",
		ProjectID:      "my-project",
		Region:         "us-central1",
	}); err != nil {
		t.Fatalf("writeMarker() failed: %v", err)
	}
	deleteCalled := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name":"projects/my-project/locations/us-central1/clusters/ctkdemo","labels":{"ghpc_deployment":"ctkdemo"}}`)
		case http.MethodDelete:
			deleteCalled = true
			_, _ = io.WriteString(w, `{"name":"operations/del-1","done":true}`)
		}
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bp}); err != nil {
		t.Fatalf("Export() on disable failed: %v", err)
	}
	if !deleteCalled {
		t.Error("expected DeleteCluster to be called when export_to_cluster_director=false on previously exported deployment")
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("expected marker to be removed after deleting disabled Cluster Director record")
	}
}

func testExportUnsetSettingPreservesAndUpdatesExistingMarker(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	bp := testBlueprint("us-central1-a")
	bp.ExportToClusterDirector = nil // Unset in blueprint (e.g. after deploy bp.yaml -w)

	// Without a marker, unset remains off and makes zero API calls.
	emptyArtDir := t.TempDir()
	called := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: emptyArtDir, Blueprint: bp}); err != nil {
		t.Fatalf("Export() with unset setting and no marker failed: %v", err)
	}
	if called {
		t.Error("expected zero API calls when export_to_cluster_director is unset and no marker exists")
	}

	// With an existing marker, unset keeps the export alive rather than deleting it.
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	deleted := false
	updated := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "message": "exists", "status": "ALREADY_EXISTS"}}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case http.MethodPatch:
			updated = true
			_, _ = io.WriteString(w, `{"name": "operations/op-patch", "done": true}`)
		case http.MethodDelete:
			deleted = true
		}
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bp}); err != nil {
		t.Fatalf("Export() with unset setting and existing marker failed: %v", err)
	}
	if deleted || !updated {
		t.Errorf("Export() with unset setting + marker: deleted=%v updated=%v, want deleted=false updated=true", deleted, updated)
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("expected marker to remain present")
	}
}

func testExportUnexportableBlueprintSkipsQuietly(t *testing.T) {
	enabled := true
	bp := config.Blueprint{ExportToClusterDirector: &enabled}
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: bp}); err != nil {
		t.Fatalf("Export() on empty blueprint failed: %v", err)
	}
}

func testExportNoDeployedTerraformResources(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return nil, errGroupNotDeployed })
	called := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	artDir := t.TempDir()
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Export() when no groups are deployed failed: %v", err)
	}
	if called {
		t.Error("expected Cluster Director API not to be called when no Terraform groups have deployed resources")
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("expected marker not to be written when no Terraform groups have deployed resources")
	}
}

func testExportUnreadableTerraformStateFails(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return nil, errors.New("corrupt state file")
	})
	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), "could not read Terraform state") {
		t.Errorf("Export() on unreadable state = %v, want state read error", err)
	}
}

func testExportInvalidSpecFailsWithGuidance(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
			Resources: []*tfjson.StateResource{{
				Mode:            tfjson.ManagedResourceMode,
				Type:            "google_compute_instance_group_manager",
				AttributeValues: map[string]any{"self_link": "malformed-mig-link"},
			}},
		}}}, nil
	})
	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: testBlueprint("")})
	if err == nil || !strings.Contains(err.Error(), "cannot export") {
		t.Errorf("Export() error = %v, want cannot export validation failure", err)
	}
}

func testExportCreateSucceedsAndWritesMarker(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"name": "operations/op-create", "done": true}`)
	})
	artDir := t.TempDir()
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Export() failed: %v", err)
	}
	m, ok, err := readMarker(artDir)
	if err != nil || !ok || m.ClusterName != "ctkdemo" || m.DeploymentName != "ctkdemo" {
		t.Errorf("readMarker() = (%+v, %t, %v), want ctkdemo marker", m, ok, err)
	}
}

func testCreateWritesMarkerBeforeWaitingOnOperation(t *testing.T) {
	withFastPolling(t)
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	artDir := t.TempDir()

	// POST accepts the create (done:false), then polling fails with a non-retryable 403.
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/operations/op-create", "done": false}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "operations.get denied"}}`)
	})

	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")})
	if err == nil {
		t.Fatal("Export() succeeded when operation polling failed, want error")
	}
	if _, ok, readErr := readMarker(artDir); readErr != nil || !ok {
		t.Fatalf("readMarker() after accepted CreateCluster + failed poll = (ok=%v, err=%v), want marker persisted before waiting", ok, readErr)
	}

	// Because the marker was written on acceptance, a subsequent Delete cleans up the cluster.
	deleteCalled := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case http.MethodDelete:
			deleteCalled = true
			_, _ = io.WriteString(w, `{"name": "operations/op-del", "done": true}`)
		}
	})
	if delErr := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); delErr != nil {
		t.Fatalf("Delete() after interrupted Export failed: %v", delErr)
	}
	if !deleteCalled {
		t.Error("expected DeleteCluster to be called for cluster whose CreateCluster LRO did not finish polling")
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("expected marker to be removed after Delete succeeded")
	}
}

func testExportAlreadyExistsTriggersUpdateCluster(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	patched := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "message": "exists", "status": "ALREADY_EXISTS"}}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case http.MethodPatch:
			patched = true
			_, _ = io.WriteString(w, `{"name": "operations/op-patch", "done": true}`)
		}
	})
	artDir := t.TempDir()
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Export() on ALREADY_EXISTS failed: %v", err)
	}
	if !patched {
		t.Error("expected UpdateCluster (PATCH) to be called after ALREADY_EXISTS")
	}
}

func testExportAlreadyExistsRefusesCollidingOwner(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	patched := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "message": "exists", "status": "ALREADY_EXISTS"}}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctk-demo"}}`)
		case http.MethodPatch:
			patched = true
		}
	})
	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), `belongs to deployment "ctk-demo", not "ctkdemo"`) {
		t.Errorf("Export() on colliding cluster = %v, want ownership mismatch error", err)
	}
	if patched {
		t.Error("expected UpdateCluster (PATCH) NOT to be called when cluster belongs to another deployment")
	}
}

func testExportAlreadyExistsUnchangedSkipsUpdate(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	patched := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "message": "exists", "status": "ALREADY_EXISTS"}}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{
				"name": "projects/my-project/locations/us-central1/clusters/ctkdemo",
				"description": "Cluster Toolkit deployment: ctkdemo",
				"labels": {
					"ghpc_deployment": "ctkdemo",
					"imported_from_cluster_toolkit": "true"
				},
				"orchestrator": {
					"computeEngine": {
						"existingInstances": {
							"ctk-nodes": {
								"project": "projects/my-project",
								"labels": {"ghpc_deployment": "ctkdemo"}
							}
						}
					}
				}
			}`)
		case http.MethodPatch:
			patched = true
			_, _ = io.WriteString(w, `{"name": "operations/op-patch", "done": true}`)
		}
	})
	artDir := t.TempDir()
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Export() on unchanged ALREADY_EXISTS failed: %v", err)
	}
	if patched {
		t.Error("expected UpdateCluster (PATCH) to be skipped when existing resources match")
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("expected marker to be present when cluster is already exported and up to date")
	}
}

func testExportAlreadyExistsFailingUpdateWritesMarker(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "message": "exists", "status": "ALREADY_EXISTS"}}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error": {"code": 403, "message": "patch denied", "status": "PERMISSION_DENIED"}}`)
		}
	})
	artDir := t.TempDir()
	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), "could not update the Cluster Director export") {
		t.Errorf("Export() error = %v, want update failure", err)
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("expected marker to be written on ALREADY_EXISTS once ownership is verified")
	}
}

func testExportCreateAPIErrorFails(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "message": "denied", "status": "PERMISSION_DENIED"}}`)
	})
	artDir := t.TempDir()
	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), "could not export") {
		t.Errorf("Export() error = %v, want could not export", err)
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("expected no marker when CreateCluster fails synchronously")
	}
}

func testDeleteAbsentMarkerIsANoOp(t *testing.T) {
	if err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Delete() without marker failed: %v", err)
	}
}

func testDeleteCorruptMarkerFails(t *testing.T) {
	artDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(artDir, cdMarkerFile), []byte("{bad"), 0644)
	if err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err == nil {
		t.Error("Delete() on corrupt marker succeeded, want error")
	}
}

func testDeleteFullDestroyDeletesClusterAndRemovesMarker(t *testing.T) {
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	deleted := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case http.MethodDelete:
			deleted = true
			_, _ = io.WriteString(w, `{"name": "operations/op-del", "done": true}`)
		}
	})
	if err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}
	if !deleted {
		t.Error("expected DeleteCluster to be called")
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("marker still exists after successful delete")
	}
}

func testDeleteFullDestroyToleratesNOTFOUNDAndRemoves(t *testing.T) {
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error": {"code": 404, "message": "not found", "status": "NOT_FOUND"}}`)
	})
	if err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Delete() on NOT_FOUND failed: %v", err)
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("marker still exists after NOT_FOUND delete")
	}
}

func testDeleteFullDestroyAPIErrorFailsAndPreserves(t *testing.T) {
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "message": "denied", "status": "PERMISSION_DENIED"}}`)
	})
	err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), "could not delete") {
		t.Errorf("Delete() error = %v, want could not delete", err)
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("marker was removed after failed delete, want it preserved")
	}
}

func testDeletePartialDestroyShrinksClusterAndKeepsMarker(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	shrunk := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case http.MethodPatch:
			shrunk = true
			_, _ = io.WriteString(w, `{"name": "operations/op-shrink", "done": true}`)
		}
	})
	opts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       testBlueprint("us-central1-a"),
		IsGroupSelected: groupSelector([]string{"compute"}, nil),
	}
	if err := Delete(context.Background(), opts); err != nil {
		t.Fatalf("Delete() partial destroy failed: %v", err)
	}
	if !shrunk {
		t.Error("expected UpdateCluster (PATCH) on partial destroy")
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("marker was removed on partial destroy, want it kept")
	}
}

func testDeletePartialDestroyUnsetSettingStillShrinks(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	bp := testBlueprint("us-central1-a")
	bp.ExportToClusterDirector = nil

	shrunk := false
	deleted := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case http.MethodPatch:
			shrunk = true
			_, _ = io.WriteString(w, `{"name": "operations/op-shrink", "done": true}`)
		case http.MethodDelete:
			deleted = true
		}
	})
	opts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       bp,
		IsGroupSelected: groupSelector([]string{"compute"}, nil),
	}
	if err := Delete(context.Background(), opts); err != nil {
		t.Fatalf("Delete() partial destroy with unset setting failed: %v", err)
	}
	if deleted || !shrunk {
		t.Errorf("Delete() partial destroy with unset setting: deleted=%v shrunk=%v, want deleted=false shrunk=true", deleted, shrunk)
	}
}

func testDeletePartialDestroyFailsWhenSurvivingGroupState(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return nil, errors.New("corrupt state file")
	})
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	deleted := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
		}
	})
	opts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       testBlueprint("us-central1-a"),
		IsGroupSelected: groupSelector([]string{"compute"}, nil),
	}
	err := Delete(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "cannot verify surviving Terraform state") {
		t.Errorf("Delete() on unreadable surviving state = %v, want state verification error", err)
	}
	if deleted {
		t.Error("expected DeleteCluster NOT to be called when surviving group state is unreadable")
	}
}

func testDeletePartialDestroyWithIdenticalSurvivingResourcesSkips(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	shrunk := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{
				"name": "projects/my-project/locations/us-central1/clusters/ctkdemo",
				"description": "Cluster Toolkit deployment: ctkdemo",
				"labels": {
					"ghpc_deployment": "ctkdemo",
					"imported_from_cluster_toolkit": "true"
				},
				"orchestrator": {
					"computeEngine": {
						"existingInstances": {
							"ctk-nodes": {
								"project": "projects/my-project",
								"labels": {"ghpc_deployment": "ctkdemo"}
							}
						}
					}
				}
			}`)
		case http.MethodPatch:
			shrunk = true
			_, _ = io.WriteString(w, `{"name": "operations/op-shrink", "done": true}`)
		}
	})
	opts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       testBlueprint("us-central1-a"),
		IsGroupSelected: groupSelector([]string{"compute"}, nil),
	}
	if err := Delete(context.Background(), opts); err != nil {
		t.Fatalf("Delete() partial destroy with identical resources failed: %v", err)
	}
	if shrunk {
		t.Error("expected UpdateCluster (PATCH) to be skipped when surviving resources match existing cluster")
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("marker was removed on partial destroy, want it kept")
	}
}

func testDeletePartialDestroyDeletesClusterAndRemovesMarker(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{}}}, nil
	})
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	deleted := false
	shrunk := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case http.MethodDelete:
			deleted = true
			_, _ = io.WriteString(w, `{"name": "operations/op-del", "done": true}`)
		case http.MethodPatch:
			shrunk = true
		}
	})
	opts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       testBlueprint("us-central1-a"),
		IsGroupSelected: groupSelector([]string{"network"}, nil),
	}
	if err := Delete(context.Background(), opts); err != nil {
		t.Fatalf("Delete() when last surviving group was destroyed failed: %v", err)
	}
	if shrunk {
		t.Error("expected DeleteCluster (DELETE), not UpdateCluster (PATCH), when no unselected group has resources left")
	}
	if !deleted {
		t.Error("expected DeleteCluster (DELETE) when no unselected group has resources left")
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("expected marker to be removed when all groups have been destroyed")
	}
}

func testDeletePartialDestroyToleratesNOTFOUND(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error": {"code": 404, "message": "not found", "status": "NOT_FOUND"}}`)
	})
	opts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       testBlueprint("us-central1-a"),
		IsGroupSelected: groupSelector([]string{"compute"}, nil),
	}
	if err := Delete(context.Background(), opts); err != nil {
		t.Fatalf("Delete() partial destroy on NOT_FOUND failed: %v", err)
	}
}

func testDeletePartialDestroyAPIErrorFails(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "message": "denied", "status": "PERMISSION_DENIED"}}`)
	})
	opts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       testBlueprint("us-central1-a"),
		IsGroupSelected: groupSelector([]string{"compute"}, nil),
	}
	err := Delete(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "could not update") {
		t.Errorf("Delete() partial destroy error = %v, want could not update", err)
	}
}

func testDeleteRefusesToDeleteClusterOwnedByAnother(t *testing.T) {
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	deleted := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctk-demo"}}`)
		case http.MethodDelete:
			deleted = true
		}
	})
	err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), `belongs to deployment "ctk-demo", not "ctkdemo"`) {
		t.Errorf("Delete() on colliding cluster = %v, want ownership mismatch error", err)
	}
	if deleted {
		t.Error("expected DeleteCluster NOT to be called when cluster belongs to another deployment")
	}
}

func TestExportFailsOnMalformedMarker(t *testing.T) {
	artDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(artDir, cdMarkerFile), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		bp := testBlueprint("us-central1-a")
		bp.SetExportToClusterDirector(enabled)
		if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bp}); err == nil {
			t.Errorf("Export(export=%v) with a malformed marker = nil, want an error", enabled)
		}
	}
}

func TestCleanupSupersededClusterWarnsWhenDeleteFails(t *testing.T) {
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name":"projects/my-project/locations/us-central1/clusters/oldid","labels":{"ghpc_deployment":"ctkdemo"}}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "boom"}}`)
		}
	})
	snapshot, restore := logging.CaptureForTesting(t)
	defer restore()
	prev := SupersededCluster{ClusterName: "oldid", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"}
	cleaned := cleanupSupersededCluster(context.Background(), prev, testBlueprint("us-central1-a"))
	_, errOut := snapshot()
	if cleaned {
		t.Error("cleanupSupersededCluster() = true on failing delete, want false")
	}
	if !strings.Contains(errOut, "gcloud alpha cluster-director clusters delete oldid") {
		t.Errorf("cleanupSupersededCluster() output = %q, want a warning with the manual delete command", errOut)
	}
}

func testDeleteAllSelectedGroupsSkippedPreservesExport(t *testing.T) {
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	called := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	skipped := map[config.GroupName]bool{"network": true, "compute": true}
	if err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a"), SkippedGroups: skipped}); err != nil {
		t.Fatalf("Delete() when all groups skipped failed: %v", err)
	}
	if called {
		t.Error("expected Cluster Director API not to be called when all selected groups were skipped")
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("marker was removed when all selected groups were skipped")
	}
}

func testDeleteOneSkippedGroupShrinksClusterToSkippedGroup(t *testing.T) {
	withFakeTerraformState(t, func(groupDir string) (*tfjson.State, error) {
		if strings.HasSuffix(groupDir, "/network") {
			return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{{
					Mode: tfjson.ManagedResourceMode,
					Type: "google_compute_subnetwork",
					AttributeValues: map[string]any{
						"self_link": "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/subnetworks/net-sub",
						"network":   "https://www.googleapis.com/compute/v1/projects/my-project/global/networks/net",
					},
				}},
			}}}, nil
		}
		t.Fatalf("unexpected state read on destroyed group %q", groupDir)
		return nil, nil
	})
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	var patchBody string
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-central1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}, "storageResources": {"ctk-bucket-b": {"config": {"existingBucket": {"bucket": "b"}}}}}`)
		case http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			patchBody = string(b)
			_, _ = io.WriteString(w, `{"name": "operations/op-shrink", "done": true}`)
		}
	})
	skipped := map[config.GroupName]bool{"network": true}
	if err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a"), SkippedGroups: skipped}); err != nil {
		t.Fatalf("Delete() with one skipped group failed: %v", err)
	}
	if !strings.Contains(patchBody, "net-sub") || strings.Contains(patchBody, "ctk-bucket-b") {
		t.Errorf("shrink PATCH body = %s, want surviving network and no destroyed bucket", patchBody)
	}
	if _, ok, _ := readMarker(artDir); !ok {
		t.Error("expected marker to remain when skipped group survived")
	}
}

func sharedVPCTestState() *tfjson.State {
	return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			{
				Mode: tfjson.DataResourceMode,
				Type: "google_compute_subnetwork",
				AttributeValues: map[string]any{
					"self_link": "https://www.googleapis.com/compute/v1/projects/host-proj/regions/us-central1/subnetworks/shared-sub",
					"network":   "https://www.googleapis.com/compute/v1/projects/host-proj/global/networks/shared-vpc",
				},
			},
			{
				Mode: tfjson.ManagedResourceMode,
				Type: "google_storage_bucket",
				AttributeValues: map[string]any{
					"name": "my-project-bucket",
				},
			},
		},
	}}}
}

func TestExportClusterRetriesWithoutCrossProjectNetworksOnRejection(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return sharedVPCTestState(), nil })
	artDir := t.TempDir()
	var postBodies []string
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		postBodies = append(postBodies, body)
		if strings.Contains(body, "projects/host-proj/") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error": {"code": 400, "status": "INVALID_ARGUMENT", "message": "network projects/host-proj/global/networks/shared-vpc must belong to cluster project"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"name": "operations/op-1", "done": true}`)
	})

	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("Export() on rejected Shared VPC network failed: %v", err)
	}
	if len(postBodies) != 2 {
		t.Fatalf("CreateCluster attempts = %d, want 2 (initial + fallback without cross-project network)", len(postBodies))
	}
	if strings.Contains(postBodies[1], "host-proj") || !strings.Contains(postBodies[1], "my-project-bucket") {
		t.Errorf("fallback CreateCluster body = %s, want host-proj network stripped and bucket preserved", postBodies[1])
	}

	postBodies = nil
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")}); err != nil {
		t.Fatalf("second Export() failed: %v", err)
	}
	if len(postBodies) != 1 || strings.Contains(postBodies[0], "host-proj") {
		t.Errorf("redeploy postBodies = %v, want 1 request without host-proj", postBodies)
	}
}

func TestExportClusterDoesNotMaskUnrelatedErrorsOnSharedVPC(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return sharedVPCTestState(), nil })
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "permission denied on cluster project",
			status: http.StatusForbidden,
			body:   `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "Permission 'hypercomputecluster.clusters.create' denied on 'projects/my-project'"}}`,
		},
		{
			name:   "unrelated 400 mentioning network without host project path",
			status: http.StatusBadRequest,
			body:   `{"error": {"code": 400, "status": "INVALID_ARGUMENT", "message": "network_resources key exceeds maximum length in projects/my-project"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			artDir := t.TempDir()
			withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
				posts++
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: testBlueprint("us-central1-a")})
			if err == nil || posts != 1 {
				t.Errorf("Export() = %v (posts=%d), want immediate error on 1 attempt", err, posts)
			}
			if _, ok, _ := readMarker(artDir); ok {
				t.Error("expected no marker saved on failed export")
			}
		})
	}
}

func TestApplyOverrideToArtifactsRoundTripAndPreviousExportSetting(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"mocked": true}`)
	sourcereader.ModuleFS = os.DirFS("../..").(sourcereader.BaseFS)

	bp, _, err := config.NewBlueprint("../../examples/hpc-slurm.yaml")
	if err != nil {
		t.Fatalf("NewBlueprint(hpc-slurm.yaml) failed: %v", err)
	}
	bp.Vars = bp.Vars.With("project_id", cty.StringVal("my-project"))
	if err := bp.Expand(); err != nil {
		t.Fatalf("bp.Expand() failed: %v", err)
	}

	deplDir := t.TempDir()
	artDir := modulewriter.ArtifactsDir(deplDir)
	if err := os.MkdirAll(artDir, 0755); err != nil {
		t.Fatal(err)
	}
	expPath := filepath.Join(artDir, modulewriter.ExpandedBlueprintName)
	if err := bp.Export(expPath); err != nil {
		t.Fatalf("bp.Export() failed: %v", err)
	}
	origBytes, err := os.ReadFile(expPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := PreviousExportSetting(deplDir); ok {
		t.Error("PreviousExportSetting() = (_, true) when unset and no marker, want false")
	}

	if err := ApplyOverrideToArtifacts(artDir, true); err != nil {
		t.Fatalf("ApplyOverrideToArtifacts(true) failed: %v", err)
	}
	if prev, ok := PreviousExportSetting(deplDir); !ok || !prev {
		t.Errorf("PreviousExportSetting() after ApplyOverrideToArtifacts(true) = (%v, %v), want (true, true)", prev, ok)
	}

	trueBytes, err := os.ReadFile(expPath)
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Replace(string(trueBytes), "export_to_cluster_director: true\n", "", 1)
	if stripped != string(origBytes) {
		t.Errorf("ApplyOverrideToArtifacts(true) altered expanded_blueprint.yaml beyond export_to_cluster_director")
	}
}

func TestVarsLabelsCannotOverrideOwnershipLabels(t *testing.T) {
	bp := testBlueprint("us-central1-a")
	bp.BlueprintName = "real-bp"
	bp.Vars = bp.Vars.With("labels", cty.ObjectVal(map[string]cty.Value{
		"ghpc_deployment": cty.StringVal("spoofed-depl"),
		"ghpc_blueprint":  cty.StringVal("spoofed-bp"),
		"custom":          cty.StringVal("kept"),
	}))
	got := clusterLabelsFromBlueprint(bp)
	if got["ghpc_deployment"] != "ctkdemo" || got["ghpc_blueprint"] != "real-bp" || got["custom"] != "kept" {
		t.Errorf("clusterLabelsFromBlueprint() = %v, want ownership labels preserved", got)
	}
}

func TestUnexportableOverwriteDeletesPreviousExport(t *testing.T) {
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	deleted := false
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"name":"projects/my-project/locations/us-central1/clusters/ctkdemo","labels":{"ghpc_deployment":"ctkdemo"}}`)
		case http.MethodDelete:
			deleted = true
			_, _ = io.WriteString(w, `{"name":"operations/del-1","done":true}`)
		}
	})
	gkeBp := testBlueprint("us-central1-a")
	gkeBp.Groups = []config.Group{{
		Name: "gke",
		Modules: []config.Module{{
			ID:     "gke_cluster",
			Kind:   config.TerraformKind,
			Source: "modules/scheduler/gke-cluster",
		}},
	}}
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: gkeBp}); err != nil {
		t.Fatalf("Export() on GKE overwrite failed: %v", err)
	}
	if !deleted {
		t.Error("expected previous Cluster Director export to be deleted when deployment becomes unexportable")
	}
	if _, ok, _ := readMarker(artDir); ok {
		t.Error("expected marker to be removed after deleting superseded export")
	}
}

func TestCreateOperationFailureIncludesNonRollbackGuidance(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"name":"operations/op-fail","done":true,"error":{"code":3,"status":"INVALID_ARGUMENT","message":"bad resource"}}`)
	})
	err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: testBlueprint("us-central1-a")})
	if err == nil || !strings.Contains(err.Error(), "has NOT been rolled back") {
		t.Errorf("Export() LRO error = %v, want non-rollback guidance", err)
	}
}

func TestSharedVPCHostPrefixAndFailedFallbackDoNotCorruptState(t *testing.T) {
	withFastPolling(t)
	// Host project "acme-hpc" is a prefix of service project "acme-hpc-dev".
	withFakeTerraformState(t, func(string) (*tfjson.State, error) {
		return &tfjson.State{Values: &tfjson.StateValues{RootModule: &tfjson.StateModule{
			Resources: []*tfjson.StateResource{{
				Mode: tfjson.DataResourceMode,
				Type: "google_compute_subnetwork",
				AttributeValues: map[string]any{
					"self_link": "https://www.googleapis.com/compute/v1/projects/acme-hpc/regions/us-central1/subnetworks/shared-sub",
					"network":   "https://www.googleapis.com/compute/v1/projects/acme-hpc/global/networks/shared-vpc",
				},
			}},
		}}}, nil
	})
	bp := testBlueprint("us-central1-a")
	bp.Vars = bp.Vars.With("project_id", cty.StringVal("acme-hpc-dev"))

	// 1. A 403 on "projects/acme-hpc-dev" must NOT match host project "acme-hpc".
	posts := 0
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "Permission denied on 'projects/acme-hpc-dev'"}}`)
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: filepath.Join(t.TempDir(), "projects", "acme-hpc"), ArtifactsDir: t.TempDir(), Blueprint: bp}); err == nil || posts != 1 {
		t.Errorf("Export() on service-project 403 = %v (posts=%d), want 1 attempt with no fallback", err, posts)
	}

	// 2. When host project IS rejected, if the fallback itself fails during polling,
	// SkipCrossProjectNetworks must NOT be persisted as true in the marker.
	artDir := t.TempDir()
	posts = 0
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			if posts == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error": {"code": 400, "status": "INVALID_ARGUMENT", "message": "network projects/acme-hpc/global/networks/shared-vpc rejected"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"name": "projects/acme-hpc-dev/locations/us-central1/operations/op-fallback", "done": false}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "operations.get denied"}}`)
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bp}); err == nil || posts != 2 {
		t.Fatalf("Export() with failing fallback = %v (posts=%d), want error after 2 attempts", err, posts)
	}
	m, ok, err := readMarker(artDir)
	if err != nil || !ok || m.SkipCrossProjectNetworks {
		t.Errorf("readMarker() after failing fallback = (%+v, ok=%v, err=%v), want marker present with SkipCrossProjectNetworks=false", m, ok, err)
	}
}

func TestSupersededMarkerRegionChangeAndInterruptedCreateCleanup(t *testing.T) {
	withFastPolling(t)
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })

	artDir := t.TempDir()
	// Previously exported in us-central1.
	_ = writeMarker(artDir, Marker{
		ClusterName:    "ctkdemo",
		DeploymentName: "ctkdemo",
		ProjectID:      "my-project",
		Region:         "us-central1",
	})

	// Move deployment to us-west1 (-w). First attempt: CreateCluster in us-west1
	// is accepted (done:false), then polling fails BEFORE superseded cleanup runs.
	bpWest := testBlueprint("us-west1-a")
	bpWest.Vars = bpWest.Vars.With("region", cty.StringVal("us-west1"))
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-west1/operations/op-west", "done": false}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "poll failed"}}`)
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bpWest}); err == nil {
		t.Fatal("expected first us-west1 export to fail on poll")
	}
	m, ok, err := readMarker(artDir)
	if err != nil || !ok || m.Region != "us-west1" || m.Superseded == nil || m.Superseded.Region != "us-central1" {
		t.Fatalf("marker after interrupted supersede = (%+v, ok=%v, err=%v), want us-west1 with Superseded=us-central1", m, ok, err)
	}

	assertRetryCleansUpSupersededRegion(t, artDir, bpWest)
}

func assertRetryCleansUpSupersededRegion(t *testing.T, artDir string, bpWest config.Blueprint) {
	t.Helper()
	var deletedPaths []string
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "status": "ALREADY_EXISTS", "message": "exists"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/clusters/ctkdemo"):
			_, _ = io.WriteString(w, `{"name": "projects/my-project/locations/us-west1/clusters/ctkdemo", "labels": {"ghpc_deployment": "ctkdemo"}}`)
		case r.Method == http.MethodPatch:
			_, _ = io.WriteString(w, `{"name": "operations/op-patch", "done": true}`)
		case r.Method == http.MethodDelete:
			deletedPaths = append(deletedPaths, r.URL.Path)
			_, _ = io.WriteString(w, `{"name": "operations/op-del", "done": true}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bpWest}); err != nil {
		t.Fatalf("retry Export() after interrupted supersede failed: %v", err)
	}
	if len(deletedPaths) != 1 || !strings.Contains(deletedPaths[0], "/locations/us-central1/clusters/ctkdemo") {
		t.Errorf("deletedPaths = %v, want superseded us-central1 cluster deleted", deletedPaths)
	}
	mAfter, ok, err := readMarker(artDir)
	if err != nil || !ok || mAfter.Superseded != nil {
		t.Errorf("marker after supersede cleanup = (%+v, ok=%v, err=%v), want Superseded cleared", mAfter, ok, err)
	}
}

func TestAccessHintsIncludedOnInspectAndShrinkErrors(t *testing.T) {
	withFakeTerraformState(t, func(string) (*tfjson.State, error) { return fakeDeployedState(), nil })
	bp := testBlueprint("us-central1-a")

	// 1. GetCluster failure on ALREADY_EXISTS during Export includes AccessHint.
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error": {"code": 409, "status": "ALREADY_EXISTS", "message": "exists"}}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error": {"code": 401, "status": "UNAUTHENTICATED", "message": "expired token"}}`)
	})
	if err := Export(context.Background(), ExportOptions{DeplRoot: t.TempDir(), ArtifactsDir: t.TempDir(), Blueprint: bp}); err == nil || !strings.Contains(err.Error(), "gcloud auth application-default login") {
		t.Errorf("Export() GetCluster 401 error = %v, want ADC login hint", err)
	}

	// 2. GetCluster failure in verifyRemoteClusterForDelete during full Delete includes AccessHint.
	artDir := t.TempDir()
	_ = writeMarker(artDir, Marker{ClusterName: "ctkdemo", DeploymentName: "ctkdemo", ProjectID: "my-project", Region: "us-central1"})
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "USER_PROJECT_DENIED: serviceusage.services.use"}}`)
	})
	if err := Delete(context.Background(), DeleteOptions{DeplRoot: t.TempDir(), ArtifactsDir: artDir, Blueprint: bp}); err == nil || !strings.Contains(err.Error(), "roles/serviceusage.serviceUsageConsumer") {
		t.Errorf("Delete() GetCluster USER_PROJECT_DENIED error = %v, want serviceUsageConsumer hint", err)
	}

	// 3. Partial-destroy shrink failure (formatShrinkError) includes AccessHint.
	withFakeCdClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "SERVICE_DISABLED: API is disabled"}}`)
	})
	shrinkOpts := DeleteOptions{
		DeplRoot:        t.TempDir(),
		ArtifactsDir:    artDir,
		Blueprint:       bp,
		IsGroupSelected: groupSelector([]string{"compute"}, nil),
	}
	if err := Delete(context.Background(), shrinkOpts); err == nil || !strings.Contains(err.Error(), "gcloud services enable") {
		t.Errorf("Delete() partial-destroy SERVICE_DISABLED error = %v, want gcloud services enable hint", err)
	}
}
