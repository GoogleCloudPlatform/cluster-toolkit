/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package clusterdirector

import (
	"reflect"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

func res(mode tfjson.ResourceMode, typ string, attrs map[string]any) *tfjson.StateResource {
	return &tfjson.StateResource{Mode: mode, Type: typ, AttributeValues: attrs}
}

func managed(typ string, attrs map[string]any) *tfjson.StateResource {
	return res(tfjson.ManagedResourceMode, typ, attrs)
}

func data(typ string, attrs map[string]any) *tfjson.StateResource {
	return res(tfjson.DataResourceMode, typ, attrs)
}

func state(root *tfjson.StateModule) *tfjson.State {
	return &tfjson.State{Values: &tfjson.StateValues{RootModule: root}}
}

func TestDiscoverFindsResourcesInNestedModules(t *testing.T) {
	// Cluster Toolkit always nests: the root module calls a module per
	// blueprint module, which itself may call upstream modules.
	st := state(&tfjson.StateModule{
		ChildModules: []*tfjson.StateModule{
			{
				ChildModules: []*tfjson.StateModule{
					{
						Resources: []*tfjson.StateResource{
							managed(tfSubnetwork, map[string]any{
								"self_link": "https://www.googleapis.com/compute/v1/projects/p/regions/us-central1/subnetworks/ctk-subnet",
								"region":    "us-central1",
								"network":   "https://www.googleapis.com/compute/v1/projects/p/global/networks/ctk-net",
							}),
						},
					},
				},
			},
			{
				Resources: []*tfjson.StateResource{
					managed(tfBucket, map[string]any{"name": "bucket-a"}),
					managed(tfFilestore, map[string]any{"id": "projects/p/locations/us-central1-a/instances/fs"}),
					managed(tfLustre, map[string]any{"id": "projects/p/locations/us-central1-a/instances/lu"}),
					managed(tfMIG, map[string]any{"self_link": "https://example/zones/us-central1-a/instanceGroupManagers/mig"}),
					managed(tfRegionMIG, map[string]any{"self_link": "https://example/regions/us-central1/instanceGroupManagers/rmig"}),
				},
			},
		},
	})

	got := Discover([]*tfjson.State{st})

	wantPairs := []NetworkPair{{Network: "projects/p/global/networks/ctk-net", Subnetwork: "projects/p/regions/us-central1/subnetworks/ctk-subnet"}}
	if got := got.NetworkPairs("us-central1"); !reflect.DeepEqual(got, wantPairs) {
		t.Errorf("NetworkPairs() = %v, want %v", got, wantPairs)
	}
	if want := []string{"bucket-a"}; !reflect.DeepEqual(got.Buckets, want) {
		t.Errorf("Buckets = %v, want %v", got.Buckets, want)
	}
	if want := []string{"projects/p/locations/us-central1-a/instances/fs"}; !reflect.DeepEqual(got.Filestores, want) {
		t.Errorf("Filestores = %v, want %v", got.Filestores, want)
	}
	if want := []string{"projects/p/locations/us-central1-a/instances/lu"}; !reflect.DeepEqual(got.Lustres, want) {
		t.Errorf("Lustres = %v, want %v", got.Lustres, want)
	}
	if len(got.MIGs) != 2 {
		t.Errorf("MIGs = %v, want both the zonal and the regional group", got.MIGs)
	}
}

func TestDiscoverIgnoresLookalikeResourceTypes(t *testing.T) {
	// google_storage_bucket_iam_binding must not be read as a bucket.
	st := state(&tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			managed("google_storage_bucket_iam_binding", map[string]any{"name": "not-a-bucket"}),
			managed("google_storage_bucket_iam_member", map[string]any{"name": "also-not"}),
			managed("google_compute_network_peering", map[string]any{"name": "not-a-network"}),
		},
	})

	got := Discover([]*tfjson.State{st})
	if len(got.Buckets) != 0 || len(got.NetworkPairs("us-central1")) != 0 {
		t.Errorf("Discover() = %+v, want nothing matched", got)
	}
}

func TestDiscoverDataSourcePolicy(t *testing.T) {
	// A pre-existing VPC is read, not created, and must still be imported.
	// An incidentally read bucket must not be.
	st := state(&tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			data(tfSubnetwork, map[string]any{
				"self_link": "https://www.googleapis.com/compute/v1/projects/p/regions/us-central1/subnetworks/shared-subnet",
				"region":    "us-central1",
				"network":   "https://www.googleapis.com/compute/v1/projects/p/global/networks/shared-net",
			}),
			data(tfBucket, map[string]any{"name": "someone-elses-bucket"}),
			data(tfFilestore, map[string]any{"id": "projects/p/locations/z/instances/other"}),
			data(tfMIG, map[string]any{"self_link": "https://example/other"}),
		},
	})

	got := Discover([]*tfjson.State{st})

	if pairs := got.NetworkPairs("us-central1"); len(pairs) != 1 || pairs[0].Network != "projects/p/global/networks/shared-net" {
		t.Errorf("NetworkPairs() = %v, want the pre-existing VPC and subnetwork", pairs)
	}
	if len(got.Buckets) != 0 || len(got.Filestores) != 0 || len(got.MIGs) != 0 {
		t.Errorf("Discover() picked up data sources it should ignore: %+v", got)
	}
}

func TestDiscoverDetectsGKEContainerClustersAndDoesNotTreatNodePoolsAsGCEMIGs(t *testing.T) {
	// GKE clusters use ExistingContainerClusterOrchestrator in Cluster Director,
	// and google_container_node_pool exports `/instanceGroups/` URLs rather than
	// `/instanceGroupManagers/`. Discover records GKE clusters in
	// ContainerClusters and leaves MIGs empty.
	st := state(&tfjson.StateModule{Resources: []*tfjson.StateResource{
		managed(tfContainerCluster, map[string]any{
			"id":   "projects/my-project/locations/us-central1/clusters/gke-cluster",
			"name": "gke-cluster",
		}),
		managed(tfNodePool, map[string]any{
			"name":    "e2-pool",
			"cluster": "projects/my-project/locations/us-central1/clusters/gke-cluster",
			"managed_instance_group_urls": []any{
				"https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instanceGroups/gke-pool-a-grp",
			},
			"instance_group_urls": []any{
				"https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/instanceGroups/gke-pool-a-grp",
			},
		}),
	}})

	got := Discover([]*tfjson.State{st})
	wantClusters := []string{"projects/my-project/locations/us-central1/clusters/gke-cluster"}
	if !reflect.DeepEqual(got.ContainerClusters, wantClusters) {
		t.Errorf("ContainerClusters = %v, want %v", got.ContainerClusters, wantClusters)
	}
	if len(got.MIGs) != 0 {
		t.Errorf("MIGs = %v, want none for GKE node pools", got.MIGs)
	}
}

func TestDiscoverDeduplicatesAcrossGroups(t *testing.T) {
	mk := func() *tfjson.State {
		return state(&tfjson.StateModule{Resources: []*tfjson.StateResource{
			managed(tfBucket, map[string]any{"name": "shared"}),
		}})
	}
	got := Discover([]*tfjson.State{mk(), mk()})
	if want := []string{"shared"}; !reflect.DeepEqual(got.Buckets, want) {
		t.Errorf("Buckets = %v, want %v", got.Buckets, want)
	}
}

func TestDiscoverToleratesEmptyAndMalformedStates(t *testing.T) {
	got := Discover([]*tfjson.State{
		nil,
		{},                           // no Values
		state(nil),                   // no root module
		state(&tfjson.StateModule{}), // no resources
		state(&tfjson.StateModule{Resources: []*tfjson.StateResource{
			nil,
			managed(tfBucket, map[string]any{}), // attribute missing
			managed(tfBucket, map[string]any{"name": 42}), // wrong type
			managed(tfBucket, map[string]any{"name": ""}), // empty
		}}),
	})
	if len(got.Buckets) != 0 {
		t.Errorf("Buckets = %v, want none", got.Buckets)
	}
}

func TestNetworkPairsMatchesEachSubnetworkToItsParentVPC(t *testing.T) {
	st := state(&tfjson.StateModule{Resources: []*tfjson.StateResource{
		managed(tfSubnetwork, map[string]any{
			"self_link": "https://www.googleapis.com/compute/v1/projects/p/regions/europe-west4/subnetworks/eu-sub",
			"region":    "europe-west4",
			"network":   "https://www.googleapis.com/compute/v1/projects/p/global/networks/eu-vpc",
		}),
		managed(tfSubnetwork, map[string]any{
			"self_link": "https://www.googleapis.com/compute/v1/projects/p/regions/us-central1/subnetworks/us-sub-1",
			"region":    "us-central1",
			"network":   "projects/p/global/networks/us-vpc-1",
		}),
		// No self link, and the network given by name only.
		managed(tfSubnetwork, map[string]any{
			"project": "p",
			"name":    "us-sub-2",
			"region":  "us-central1",
			"network": "us-vpc-2",
		}),
	}})

	d := Discover([]*tfjson.State{st})
	got := d.NetworkPairs("us-central1")
	want := []NetworkPair{
		{Network: "projects/p/global/networks/us-vpc-1", Subnetwork: "projects/p/regions/us-central1/subnetworks/us-sub-1"},
		{Network: "projects/p/global/networks/us-vpc-2", Subnetwork: "projects/p/regions/us-central1/subnetworks/us-sub-2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NetworkPairs(us-central1) = %+v, want %+v", got, want)
	}
	if got, want := d.OtherRegionSubnetworks("us-central1"), []string{"projects/p/regions/europe-west4/subnetworks/eu-sub"}; !reflect.DeepEqual(got, want) {
		t.Errorf("OtherRegionSubnetworks(us-central1) = %v, want %v", got, want)
	}
}

func TestNetworkPairsKeepsSharedVPCHostProject(t *testing.T) {
	// modules/network/pre-existing-vpc reading a Shared VPC: the subnetwork
	// and network live in the host project, not the deployment project.
	st := state(&tfjson.StateModule{Resources: []*tfjson.StateResource{
		data(tfSubnetwork, map[string]any{
			"self_link": "https://www.googleapis.com/compute/v1/projects/host/regions/us-central1/subnetworks/shared-sub",
			"region":    "us-central1",
			"network":   "https://www.googleapis.com/compute/v1/projects/host/global/networks/shared-vpc",
		}),
	}})

	got := Discover([]*tfjson.State{st}).NetworkPairs("us-central1")
	want := []NetworkPair{{Network: "projects/host/global/networks/shared-vpc", Subnetwork: "projects/host/regions/us-central1/subnetworks/shared-sub"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NetworkPairs() = %+v, want %+v", got, want)
	}
}

func TestNetworkPairsSkipsSubnetworksWithoutEnoughDetail(t *testing.T) {
	st := state(&tfjson.StateModule{Resources: []*tfjson.StateResource{
		managed(tfSubnetwork, map[string]any{"name": "no-project", "region": "us-central1", "network": "net"}),
		managed(tfSubnetwork, map[string]any{
			"self_link": "https://www.googleapis.com/compute/v1/projects/p/regions/us-central1/subnetworks/no-network",
			"region":    "us-central1",
		}),
	}})
	if got := Discover([]*tfjson.State{st}).NetworkPairs("us-central1"); len(got) != 0 {
		t.Errorf("NetworkPairs() = %+v, want none", got)
	}
}

func TestDiscoverDataContainerCluster(t *testing.T) {
	st := state(&tfjson.StateModule{Resources: []*tfjson.StateResource{
		data(tfContainerCluster, map[string]any{
			"name": "pre-existing-gke",
		}),
	}})

	d := Discover([]*tfjson.State{st})
	if !reflect.DeepEqual(d.ContainerClusters, []string{"pre-existing-gke"}) {
		t.Errorf("ContainerClusters = %v, want [pre-existing-gke]", d.ContainerClusters)
	}
}

func TestDiscoverSkipsInternalModuleBuckets(t *testing.T) {
	st := state(&tfjson.StateModule{
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.slurm_controller",
				ChildModules: []*tfjson.StateModule{{
					Address: "module.slurm_controller.module.slurm_files",
					Resources: []*tfjson.StateResource{
						managed(tfBucket, map[string]any{"name": "slurm-internal-config"}),
					},
				}},
			},
			{
				Address: "module.startup",
				Resources: []*tfjson.StateResource{
					managed(tfBucket, map[string]any{"name": "vm-startup-scripts"}),
				},
			},
			{
				Address: "module.gcs_storage",
				Resources: []*tfjson.StateResource{
					managed(tfBucket, map[string]any{"name": "workload-data"}),
				},
			},
		},
		// Also verify resource-level Address fallback (e.g. local terraform.tfstate reader).
		Resources: []*tfjson.StateResource{{
			Address:         "module.compute_vm.google_storage_bucket.configs_bucket[0]",
			Mode:            tfjson.ManagedResourceMode,
			Type:            tfBucket,
			AttributeValues: map[string]any{"name": "vm-configs-bucket"},
		}},
	})

	got := DiscoverWithOptions([]*tfjson.State{st}, DiscoverOptions{
		IgnoreBucketsFromModules: map[string]bool{
			"slurm_controller": true,
			"startup":          true,
			"compute_vm":       true,
		},
	})
	if want := []string{"workload-data"}; !reflect.DeepEqual(got.Buckets, want) {
		t.Errorf("Buckets = %v, want %v (internal controller/startup buckets skipped)", got.Buckets, want)
	}
}
