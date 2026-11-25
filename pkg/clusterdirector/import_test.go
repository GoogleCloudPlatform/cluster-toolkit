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
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var rfc1034 = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$`)

// basicSpec is the equivalent of the simple Cluster Toolkit demo deployment:
// one VPC, one bucket and label selected VM instances.
func basicSpec() Spec {
	return Spec{
		ProjectID:      "my-project",
		Region:         "us-central1",
		ClusterName:    "ctktest",
		DeploymentName: "ctk-cluster-director-demo",
		Networks:       []NetworkPair{{Network: "ctk-net", Subnetwork: "ctk-subnet"}},
		Buckets:        []string{"ctk-bucket-1a2b"},
	}
}

func TestClusterBasic(t *testing.T) {
	got, err := basicSpec().Cluster()
	if err != nil {
		t.Fatalf("Cluster() returned an unexpected error: %v", err)
	}

	want := &Cluster{
		Name:        "ctktest",
		Description: "Cluster Toolkit deployment: ctk-cluster-director-demo",
		Labels: map[string]string{
			DeploymentLabel:          "ctk-cluster-director-demo",
			ImportedFromToolkitLabel: "true",
		},
		NetworkResources: map[string]NetworkResource{
			"ctk-net-ctk-net-ctk-subnet": {Config: NetworkResourceConfig{ExistingNetwork: &ExistingNetworkConfig{
				Network:    "projects/my-project/global/networks/ctk-net",
				Subnetwork: "projects/my-project/regions/us-central1/subnetworks/ctk-subnet",
			}}},
		},
		StorageResources: map[string]StorageResource{
			"ctk-bucket-ctk-bucket-1a2b": {Config: StorageResourceConfig{ExistingBucket: &ExistingBucketConfig{Bucket: "ctk-bucket-1a2b"}}},
		},
		Orchestrator: &Orchestrator{ComputeEngine: &ComputeEngineOrchestrator{
			ExistingInstances: map[string]ExistingInstances{
				NodesResourceKey: {
					Project: "projects/my-project",
					Labels:  map[string]string{DeploymentLabel: "ctk-cluster-director-demo"},
				},
			},
		}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Cluster() returned diff (-want +got):\n%s", diff)
	}
}

func TestClusterComprehensive(t *testing.T) {
	spec := basicSpec()
	spec.Buckets = []string{"gs://bucket-one", "bucket-two"}
	spec.Filestores = []string{"projects/my-project/locations/us-central1-a/instances/ctk-filestore"}
	spec.Lustres = []string{"https://lustre.googleapis.com/v1/projects/my-project/locations/us-central1-a/instances/ctk-lustre"}
	spec.MIGs = []string{
		"https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instanceGroupManagers/mig-pool",
		"projects/my-project/regions/us-central1/instanceGroupManagers/regional-pool",
	}
	spec.ClusterLabels = map[string]string{"env": "dev"}

	got, err := spec.Cluster()
	if err != nil {
		t.Fatalf("Cluster() returned an unexpected error: %v", err)
	}

	wantStorage := map[string]StorageResource{
		"ctk-bucket-bucket-one":                     {Config: StorageResourceConfig{ExistingBucket: &ExistingBucketConfig{Bucket: "bucket-one"}}},
		"ctk-bucket-bucket-two":                     {Config: StorageResourceConfig{ExistingBucket: &ExistingBucketConfig{Bucket: "bucket-two"}}},
		"ctk-filestore-us-central1-a-ctk-filestore": {Config: StorageResourceConfig{ExistingFilestore: &ExistingFilestoreConfig{Filestore: "projects/my-project/locations/us-central1-a/instances/ctk-filestore"}}},
		"ctk-lustre-us-central1-a-ctk-lustre":       {Config: StorageResourceConfig{ExistingLustre: &ExistingLustreConfig{Lustre: "projects/my-project/locations/us-central1-a/instances/ctk-lustre"}}},
	}
	if diff := cmp.Diff(wantStorage, got.StorageResources); diff != "" {
		t.Errorf("Cluster() storage resources diff (-want +got):\n%s", diff)
	}

	deplLabels := map[string]string{DeploymentLabel: "ctk-cluster-director-demo"}
	wantInstances := map[string]ExistingInstances{
		NodesResourceKey: {
			Project: "projects/my-project",
			Labels:  deplLabels,
		},
		"ctk-mig-us-central1-a-mig-pool":    {InstanceGroupManager: "projects/my-project/zones/us-central1-a/instanceGroupManagers/mig-pool"},
		"ctk-mig-us-central1-regional-pool": {RegionInstanceGroupManager: "projects/my-project/regions/us-central1/instanceGroupManagers/regional-pool"},
	}
	if diff := cmp.Diff(wantInstances, got.Orchestrator.ComputeEngine.ExistingInstances); diff != "" {
		t.Errorf("Cluster() existing instances diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(map[string]string{
		"env":                    "dev",
		DeploymentLabel:          "ctk-cluster-director-demo",
		ImportedFromToolkitLabel: "true",
	}, got.Labels); diff != "" {
		t.Errorf("Cluster() labels diff (-want +got):\n%s", diff)
	}
}

func TestClusterImportedFromToolkitLabelAlwaysStamped(t *testing.T) {
	t.Run("mig-only spec without deployment name still stamps imported_from_cluster_toolkit", func(t *testing.T) {
		spec := basicSpec()
		spec.DeploymentName = ""
		spec.MIGs = []string{"projects/my-project/zones/us-central1-a/instanceGroupManagers/mig-0"}
		got, err := spec.Cluster()
		if err != nil {
			t.Fatalf("Cluster() failed: %v", err)
		}
		want := map[string]string{ImportedFromToolkitLabel: "true"}
		if diff := cmp.Diff(want, got.Labels); diff != "" {
			t.Errorf("Cluster() labels diff (-want +got):\n%s", diff)
		}
		if got.Description != "" {
			t.Errorf("Cluster() description = %q, want empty when DeploymentName is unset", got.Description)
		}
	})

	t.Run("cannot override imported_from_cluster_toolkit or ghpc_deployment", func(t *testing.T) {
		spec := basicSpec()
		spec.ClusterLabels = map[string]string{
			ImportedFromToolkitLabel: "false",
			DeploymentLabel:          "wrong-deployment",
		}
		got, err := spec.Cluster()
		if err != nil {
			t.Fatalf("Cluster() failed: %v", err)
		}
		if got.Labels[ImportedFromToolkitLabel] != "true" {
			t.Errorf("Labels[%q] = %q, want %q", ImportedFromToolkitLabel, got.Labels[ImportedFromToolkitLabel], "true")
		}
		if got.Labels[DeploymentLabel] != spec.DeploymentName {
			t.Errorf("Labels[%q] = %q, want %q", DeploymentLabel, got.Labels[DeploymentLabel], spec.DeploymentName)
		}
	})
}

func TestClusterWithoutNetwork(t *testing.T) {
	spec := basicSpec()
	spec.Networks = nil

	got, err := spec.Cluster()
	if err != nil {
		t.Fatalf("Cluster() returned an unexpected error: %v", err)
	}
	if got.NetworkResources != nil {
		t.Errorf("Cluster() network resources = %v, want nil", got.NetworkResources)
	}
}

func TestClusterErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Spec)
		wantErr string
	}{
		{"no project", func(s *Spec) { s.ProjectID = "" }, "missing required value(s): project"},
		{"no region", func(s *Spec) { s.Region = "" }, "missing required value(s): region"},
		{"no cluster name", func(s *Spec) { s.ClusterName = "" }, "missing required value(s): cluster name"},
		{"invalid cluster name", func(s *Spec) { s.ClusterName = "CTK_Test" }, "must start with a lowercase letter"},
		{"hyphenated cluster name", func(s *Spec) { s.ClusterName = "ctk-test" }, "no hyphens"},
		{"long cluster name", func(s *Spec) { s.ClusterName = "ctkclusterdemo" }, "at most 10 characters"},
		{"subnet without network", func(s *Spec) { s.Networks = []NetworkPair{{Subnetwork: "ctk-subnet"}} }, "must be provided together"},
		{"no compute", func(s *Spec) { s.DeploymentName = "" }, "no compute to export"},
		{"empty bucket", func(s *Spec) { s.Buckets = []string{"gs://"} }, "empty bucket name"},
		{"bucket with path", func(s *Spec) { s.Buckets = []string{"bucket/with/path"} }, "must be a bare bucket name"},
		{"empty filestore", func(s *Spec) { s.Filestores = []string{" "} }, "empty filestore instance ID"},
		{"short filestore", func(s *Spec) { s.Filestores = []string{"my-filestore"} }, "must be of the form"},
		{"short lustre", func(s *Spec) { s.Lustres = []string{"my-lustre"} }, "must be of the form"},
		{"empty mig", func(s *Spec) { s.MIGs = []string{" "} }, "empty managed instance group"},
		{"bad mig", func(s *Spec) { s.MIGs = []string{"projects/p/zones/z/instances/i"} }, "must be of the form"},
		{"bad mig scope", func(s *Spec) { s.MIGs = []string{"projects/p/global/loc/instanceGroupManagers/m"} }, "must be scoped by `zones` or `regions`"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := basicSpec()
			tc.mutate(&spec)
			_, err := spec.Cluster()
			if err == nil {
				t.Fatalf("Cluster() succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Cluster() error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestSanitizeClusterID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ctkdemo", "ctkdemo"},
		{"ctk-demo", "ctkdemo"},
		{"CTK_Demo-1", "ctkdemo1"},
		{"0demo", "c0demo"},
		{"", "ctk"},
		{"---", "ctk"},
		{"ctk-cluster-director-demo", "ctkclua77e"},
	}
	for _, tc := range tests {
		got := SanitizeClusterID(tc.in)
		if got != tc.want {
			t.Errorf("SanitizeClusterID(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if !clusterIDPattern.MatchString(got) {
			t.Errorf("SanitizeClusterID(%q) = %q, which fails clusterIDPattern", tc.in, got)
		}
	}
}

func TestQualifiedLeafAcceptsSelfLinks(t *testing.T) {
	spec := basicSpec()
	spec.Networks = []NetworkPair{{
		Network:    "https://www.googleapis.com/compute/v1/projects/my-project/global/networks/ctk-net",
		Subnetwork: "projects/my-project/regions/us-central1/subnetworks/ctk-subnet",
	}}

	got, err := spec.Cluster()
	if err != nil {
		t.Fatalf("Cluster() returned an unexpected error: %v", err)
	}
	want := &ExistingNetworkConfig{
		Network:    "projects/my-project/global/networks/ctk-net",
		Subnetwork: "projects/my-project/regions/us-central1/subnetworks/ctk-subnet",
	}
	if diff := cmp.Diff(want, got.NetworkResources["ctk-net-ctk-net-ctk-subnet"].Config.ExistingNetwork); diff != "" {
		t.Errorf("Cluster() network diff (-want +got):\n%s", diff)
	}
}

func TestClusterKeepsSharedVPCHostProject(t *testing.T) {
	spec := basicSpec()
	spec.Networks = []NetworkPair{{
		Network:    "projects/host/global/networks/shared-vpc",
		Subnetwork: "projects/host/regions/us-central1/subnetworks/shared-sub",
	}}

	got, err := spec.Cluster()
	if err != nil {
		t.Fatalf("Cluster() returned an unexpected error: %v", err)
	}
	want := &ExistingNetworkConfig{
		Network:    "projects/host/global/networks/shared-vpc",
		Subnetwork: "projects/host/regions/us-central1/subnetworks/shared-sub",
	}
	if diff := cmp.Diff(want, got.NetworkResources["ctk-net-host-shared-vpc-shared-sub"].Config.ExistingNetwork); diff != "" {
		t.Errorf("Cluster() network diff (-want +got):\n%s", diff)
	}
}

func TestClusterMultiZoneAndCrossProjectKeysDoNotCollide(t *testing.T) {
	spec := basicSpec()
	spec.Networks = []NetworkPair{
		{Network: "projects/host/global/networks/default", Subnetwork: "projects/host/regions/us-central1/subnetworks/default"},
		{Network: "projects/my-project/global/networks/default", Subnetwork: "projects/my-project/regions/us-central1/subnetworks/default"},
	}
	spec.Buckets = nil
	spec.Filestores = []string{
		"projects/my-project/locations/us-central1-a/instances/home",
		"projects/my-project/locations/us-central1-b/instances/home",
	}
	spec.Lustres = []string{
		"projects/my-project/locations/us-central1-a/instances/scratch",
		"projects/my-project/locations/us-central1-b/instances/scratch",
	}
	spec.MIGs = []string{
		"projects/my-project/zones/us-central1-a/instanceGroupManagers/pool",
		"projects/my-project/zones/us-central1-b/instanceGroupManagers/pool",
	}

	got, err := spec.Cluster()
	if err != nil {
		t.Fatalf("Cluster() failed: %v", err)
	}
	if len(got.NetworkResources) != 2 {
		t.Errorf("len(NetworkResources) = %d, want 2 (host + service default VPCs)", len(got.NetworkResources))
	}
	if len(got.StorageResources) != 4 {
		t.Errorf("len(StorageResources) = %d, want 4 (2 zonal Filestores + 2 zonal Lustres)", len(got.StorageResources))
	}
	if len(got.Orchestrator.ComputeEngine.ExistingInstances) != 3 {
		t.Errorf("len(ExistingInstances) = %d, want 3 (ctk-nodes + 2 zonal MIGs)", len(got.Orchestrator.ComputeEngine.ExistingInstances))
	}
}

func TestClusterMultiNetworkAndStorageKeyStability(t *testing.T) {
	spec := basicSpec()
	spec.Networks = []NetworkPair{
		{Network: "ctk-net", Subnetwork: "ctk-subnet"},
		{Network: "rdma-net", Subnetwork: "rdma-subnet"},
	}
	spec.Buckets = []string{"z-existing-bucket"}

	before, err := spec.Cluster()
	if err != nil {
		t.Fatalf("Cluster() before update failed: %v", err)
	}

	// Simulate adding a bucket that sorts earlier on a `-w` redeploy.
	spec.Buckets = []string{"a-new-bucket", "z-existing-bucket"}
	after, err := spec.Cluster()
	if err != nil {
		t.Fatalf("Cluster() after update failed: %v", err)
	}

	// The map key for "z-existing-bucket" must remain identical and point to
	// the exact same config so Cluster Director's validateStorageResourcesUpdate
	// accepts the PATCH.
	const zKey = "ctk-bucket-z-existing-bucket"
	if diff := cmp.Diff(before.StorageResources[zKey], after.StorageResources[zKey]); diff != "" {
		t.Errorf("StorageResources[%q] changed across update (-before +after):\n%s", zKey, diff)
	}
	if len(after.NetworkResources) != 2 {
		t.Errorf("len(NetworkResources) = %d, want 2: %v", len(after.NetworkResources), after.NetworkResources)
	}
}

func TestResourceKeyLongNamesDoNotCollide(t *testing.T) {
	prefix := strings.Repeat("a", 60)
	k1 := ResourceKey("ctk-bucket", prefix+"-one")
	k2 := ResourceKey("ctk-bucket", prefix+"-two")
	if k1 == k2 {
		t.Errorf("ResourceKey collided for distinct long names: %q", k1)
	}
	if len(k1) > maxKeyLength || !rfc1034.MatchString(k1) {
		t.Errorf("ResourceKey() = %q (len=%d), want valid RFC-1034 key <= %d chars", k1, len(k1), maxKeyLength)
	}
}

func TestSameResources(t *testing.T) {
	spec := Spec{
		ProjectID:      "my-project",
		Region:         "us-central1",
		ClusterName:    "ctkjbvm",
		DeploymentName: "ctkjbvm",
		Networks:       []NetworkPair{{Network: "ctkjbvm-net", Subnetwork: "ctkjbvm-primary-subnet"}},
		Buckets:        []string{"shared-ctkjbvm", "scratch-ctkjbvm"},
	}
	base, err := spec.Cluster()
	if err != nil {
		t.Fatalf("spec.Cluster() failed: %v", err)
	}

	// Server-returned GET payload carries fully qualified Name and may omit
	// empty maps; SameResources must still treat it as identical.
	fromServer, err := spec.Cluster()
	if err != nil {
		t.Fatalf("spec.Cluster() failed: %v", err)
	}
	fromServer.Name = "projects/my-project/locations/us-central1/clusters/ctkjbvm"
	if !SameResources(fromServer, base) {
		t.Error("SameResources() = false for identical resource sets, want true")
	}

	// Adding or removing a bucket changes the resource set.
	specModified := spec
	specModified.Buckets = []string{"shared-ctkjbvm"}
	modified, err := specModified.Cluster()
	if err != nil {
		t.Fatalf("specModified.Cluster() failed: %v", err)
	}
	if SameResources(fromServer, modified) {
		t.Error("SameResources() = true when storageResources differ, want false")
	}

	// Changing Cluster.labels also marks the cluster as needing UpdateCluster.
	specLabels := spec
	specLabels.ClusterLabels = map[string]string{
		DeploymentLabel: "ctkjbvm",
		BlueprintLabel:  "jbvm-blueprint",
		"ghpc_creator":  "alice",
	}
	withLabels, err := specLabels.Cluster()
	if err != nil {
		t.Fatalf("specLabels.Cluster() failed: %v", err)
	}
	if SameResources(fromServer, withLabels) {
		t.Error("SameResources() = true when Cluster.labels differ, want false")
	}

	// A pre-existing cluster exported before imported_from_cluster_toolkit was
	// introduced must be detected as needing UpdateCluster so redeploying
	// patches the label in place.
	legacyServer, err := spec.Cluster()
	if err != nil {
		t.Fatalf("spec.Cluster() failed: %v", err)
	}
	delete(legacyServer.Labels, ImportedFromToolkitLabel)
	if SameResources(legacyServer, base) {
		t.Error("SameResources() = true when existing cluster is missing imported_from_cluster_toolkit, want false")
	}

	// A pre-existing cluster exported before Cluster.description was populated
	// must also be detected as needing UpdateCluster so redeploying backfills
	// the full deployment name in place.
	missingDescription, err := spec.Cluster()
	if err != nil {
		t.Fatalf("spec.Cluster() failed: %v", err)
	}
	missingDescription.Description = ""
	if SameResources(missingDescription, base) {
		t.Error("SameResources() = true when existing cluster is missing description, want false")
	}
}

func TestResolveClusterID(t *testing.T) {
	// Without colliding MIGs, ResolveClusterID returns SanitizeClusterID unchanged.
	if got := ResolveClusterID("demoslurm", nil); got != "demoslurm" {
		t.Errorf("ResolveClusterID(no MIGs) = %q, want %q", got, "demoslurm")
	}
	// When a discovered MIG starts with "<clusterID>-", ResolveClusterID disambiguates
	// the cluster ID so Cluster Director's validate.go does not reject the MIG.
	migs := []string{"https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/instanceGroupManagers/demoslurm-debugnodeset-mig-0"}
	got := ResolveClusterID("demoslurm", migs)
	if got == "demoslurm" {
		t.Fatalf("ResolveClusterID() returned %q which collides with MIG prefix demoslurm-", got)
	}
	if !clusterIDPattern.MatchString(got) {
		t.Errorf("ResolveClusterID() = %q, does not match clusterIDPattern", got)
	}
	if MIGHasClusterIDPrefix(got, migs) {
		t.Errorf("ResolveClusterID() = %q still collides with MIG %v", got, migs)
	}
}

func TestConsoleURL(t *testing.T) {
	got := ConsoleURL("my-project", "us-central1", "demovm")
	want := "https://console.cloud.google.com/cluster-director/clusters/details/us-central1/demovm/overview?project=my-project"
	if got != want {
		t.Errorf("ConsoleURL() = %q, want %q", got, want)
	}
}
