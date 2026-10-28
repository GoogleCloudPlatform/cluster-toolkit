// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"os"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

func (s *MySuite) TestMergeDeploymentSettings(c *C) {
	ds1 := config.DeploymentSettings{
		Vars: config.Dict{}.
			With("project_id", cty.StringVal("ds_test_project_id")).
			With("deployment_name", cty.StringVal("ds_deployment_name"))}

	bp1 := config.Blueprint{
		Vars: config.Dict{}.
			With("project_id", cty.StringVal("bp_test_project_id")).
			With("example_var", cty.StringVal("bp_example_value"))}

	// test priority-based merging of deployment variables
	mergeDeploymentSettings(&bp1, ds1)
	c.Check(bp1.Vars.Items(), DeepEquals, map[string]cty.Value{
		"project_id":      cty.StringVal("ds_test_project_id"),
		"deployment_name": cty.StringVal("ds_deployment_name"),
		"example_var":     cty.StringVal("bp_example_value"),
	})

	// check merging zero-value backends
	ds2 := config.DeploymentSettings{
		TerraformBackendDefaults: config.TerraformBackend{},
	}
	bp2 := config.Blueprint{
		TerraformBackendDefaults: config.TerraformBackend{},
	}
	mergeDeploymentSettings(&bp2, ds2)
	c.Check(bp2.TerraformBackendDefaults, DeepEquals, config.TerraformBackend{})

	// check keeping blueprint defined backend with no backend in deployment file
	bp3 := config.Blueprint{
		TerraformBackendDefaults: config.TerraformBackend{
			Type: "gsc",
			Configuration: config.NewDict(map[string]cty.Value{
				"bucket": cty.StringVal("bp_bucket"),
			}),
		},
	}
	mergeDeploymentSettings(&bp3, ds2)
	c.Check(bp3.TerraformBackendDefaults, DeepEquals, config.TerraformBackend{
		Type: "gsc",
		Configuration: config.NewDict(map[string]cty.Value{
			"bucket": cty.StringVal("bp_bucket"),
		}),
	})

	// check overriding blueprint defined backend with deployment file
	ds3 := config.DeploymentSettings{
		TerraformBackendDefaults: config.TerraformBackend{
			Type: "gsc",
			Configuration: config.NewDict(map[string]cty.Value{
				"bucket": cty.StringVal("ds_bucket"),
			}),
		},
	}
	mergeDeploymentSettings(&bp3, ds3)
	c.Check(bp3.TerraformBackendDefaults, DeepEquals, config.TerraformBackend{
		Type: "gsc",
		Configuration: config.NewDict(map[string]cty.Value{
			"bucket": cty.StringVal("ds_bucket"),
		}),
	})
}

func (s *MySuite) TestIsOverwriteAllowed_Absent(c *C) {
	testDir := c.MkDir()
	depDir := filepath.Join(testDir, "casper")

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/), IsNil)
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_NotGHPC(c *C) {
	depDir := c.MkDir() // empty deployment folder considered malformed

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* not a valid GHPC deployment folder.*")
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* not a valid GHPC deployment folder.*")

	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_NoExpanded(c *C) {
	depDir := c.MkDir() // empty deployment folder considered malformed
	if err := os.MkdirAll(modulewriter.HiddenGhpcDir(depDir), 0755); err != nil {
		c.Fatal(err)
	}

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* changing GHPC version.*")
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* changing GHPC version.*")

	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_Malformed(c *C) {
	depDir := c.MkDir() // empty deployment folder considered malformed
	if err := os.MkdirAll(modulewriter.ArtifactsDir(depDir), 0755); err != nil {
		c.Fatal(err)
	}
	expPath := filepath.Join(modulewriter.ArtifactsDir(depDir), "expanded_blueprint.yaml")
	if err := os.WriteFile(expPath, []byte("humus"), 0644); err != nil {
		c.Fatal(err)
	}

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/), NotNil)
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/), NotNil)
	// force
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_Present(c *C) {
	p := c.MkDir()
	artDir := modulewriter.ArtifactsDir(p)
	if err := os.MkdirAll(artDir, 0755); err != nil {
		c.Fatal(err)
	}

	prev := config.Blueprint{
		GhpcVersion: "TaleOfBygoneYears",
		Groups: []config.Group{
			{Name: "isildur"}}}
	if err := prev.Export(filepath.Join(artDir, "expanded_blueprint.yaml")); err != nil {
		c.Fatal(err)
	}
	noW, yesW, noForce, yesForce := false, true, false, true

	{ // Superset
		bp := config.Blueprint{
			GhpcVersion: "TaleOfBygoneYears",
			Groups: []config.Group{
				{Name: "isildur"},
				{Name: "elendil"}}}
		c.Check(checkOverwriteAllowed(p, bp, noW, noForce), ErrorMatches, ".* already exists.*")
		c.Check(checkOverwriteAllowed(p, bp, yesW, noForce), IsNil)
	}

	{ // Version mismatch
		bp := config.Blueprint{
			GhpcVersion: "TheAlloyOfLaw",
			Groups: []config.Group{
				{Name: "isildur"}}}
		c.Check(checkOverwriteAllowed(p, bp, noW, noForce), ErrorMatches, ".*ghpc_version has changed.*")
		c.Check(checkOverwriteAllowed(p, bp, yesW, noForce), ErrorMatches, ".*ghpc_version has changed.*")
		c.Check(checkOverwriteAllowed(p, bp, noW, yesForce), IsNil)
	}

	{ // Subset
		bp := config.Blueprint{
			GhpcVersion: "TaleOfBygoneYears",
			Groups: []config.Group{
				{Name: "aragorn"}}}
		c.Check(checkOverwriteAllowed(p, bp, noW, noForce), ErrorMatches, `.* already exists.*`)
		c.Check(checkOverwriteAllowed(p, bp, yesW, noForce), ErrorMatches, `.*remove a deployment group "isildur".*`)
		c.Check(checkOverwriteAllowed(p, bp, noW, yesForce), IsNil)
	}
}

func (s *MySuite) TestVerifyGcsBucketsEmptyBucket(c *C) {
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id": cty.StringVal("my-project"),
		}),
		Groups: []config.Group{
			{
				Name: "group1",
				TerraformBackend: config.TerraformBackend{
					Type: "gcs",
					Configuration: config.NewDict(map[string]cty.Value{
						"bucket": cty.NullVal(cty.String),
					}),
				},
			},
		},
	}

	err := verifyGcsBuckets(context.Background(), bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name for group \"group1\" cannot be empty or unknown")

	bp.Groups[0].TerraformBackend.Configuration = config.NewDict(map[string]cty.Value{
		"bucket": cty.StringVal(""),
	})
	err = verifyGcsBuckets(context.Background(), bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name for group \"group1\" cannot be empty")
}

func (s *MySuite) TestGetModifiedIgnoredSettingsWarnings(c *C) {
	bpPrev := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "np-1",
						Source: "modules/compute/gke-node-pool",
						Settings: config.NewDict(map[string]cty.Value{
							"labels": cty.ObjectVal(map[string]cty.Value{
								"env": cty.StringVal("prod"),
							}),
						}),
					},
					{
						ID:     "cluster",
						Source: "modules/scheduler/gke-cluster",
						Settings: config.NewDict(map[string]cty.Value{
							"version_prefix": cty.StringVal("1.35."),
							"labels": cty.ObjectVal(map[string]cty.Value{
								"env": cty.StringVal("prod"),
							}),
							"node_locations": cty.ListVal([]cty.Value{cty.StringVal("us-central1-a")}),
						}),
					},
				},
			},
		},
	}

	// Case 1: Identical blueprints produce no warnings
	c.Check(getModifiedIgnoredSettingsWarnings(bpPrev, bpPrev), HasLen, 0)

	// Case 2: New module added produces no warning
	bpNewModule := bpPrev.Clone()
	bpNewModule.Groups[0].Modules = append(bpNewModule.Groups[0].Modules, config.Module{
		ID:     "np-2",
		Source: "modules/compute/gke-node-pool",
		Settings: config.NewDict(map[string]cty.Value{
			"labels": cty.ObjectVal(map[string]cty.Value{
				"env": cty.StringVal("staging"),
			}),
		}),
	})
	c.Check(getModifiedIgnoredSettingsWarnings(bpPrev, bpNewModule), HasLen, 0)

	// Case 3: Modified gke-node-pool labels produces warning
	bpModNodePoolLabels := bpPrev.Clone()
	bpModNodePoolLabels.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
		"labels": cty.ObjectVal(map[string]cty.Value{
			"env": cty.StringVal("staging"),
		}),
	})
	npWarns := getModifiedIgnoredSettingsWarnings(bpPrev, bpModNodePoolLabels)
	c.Assert(npWarns, HasLen, 1)
	c.Check(npWarns[0], Matches, "(?s).*module \"np-1\".*setting 'labels' was modified.*gcloud container node-pools update.*")

	// Case 4: Modified gke-cluster version_prefix produces warning
	bpModClusterVersion := bpPrev.Clone()
	bpModClusterVersion.Groups[0].Modules[1].Settings = config.NewDict(map[string]cty.Value{
		"version_prefix": cty.StringVal("1.36."),
		"labels": cty.ObjectVal(map[string]cty.Value{
			"env": cty.StringVal("prod"),
		}),
		"node_locations": cty.ListVal([]cty.Value{cty.StringVal("us-central1-a")}),
	})
	verWarns := getModifiedIgnoredSettingsWarnings(bpPrev, bpModClusterVersion)
	c.Assert(verWarns, HasLen, 1)
	c.Check(verWarns[0], Matches, "(?s).*module \"cluster\".*control-plane version setting was modified.*gcloud container clusters upgrade.*")

	// Case 5: Modified gke-cluster node_locations produces warning
	bpModClusterLocations := bpPrev.Clone()
	bpModClusterLocations.Groups[0].Modules[1].Settings = config.NewDict(map[string]cty.Value{
		"version_prefix": cty.StringVal("1.35."),
		"labels": cty.ObjectVal(map[string]cty.Value{
			"env": cty.StringVal("prod"),
		}),
		"node_locations": cty.ListVal([]cty.Value{cty.StringVal("us-central1-b")}),
	})
	locWarns := getModifiedIgnoredSettingsWarnings(bpPrev, bpModClusterLocations)
	c.Assert(locWarns, HasLen, 1)
	c.Check(locWarns[0], Matches, "(?s).*module \"cluster\".*cluster zone settings.*node_locations.*")

	// Case 6: Modified gke-cluster labels produces warning
	bpModClusterLabels := bpPrev.Clone()
	bpModClusterLabels.Groups[0].Modules[1].Settings = config.NewDict(map[string]cty.Value{
		"version_prefix": cty.StringVal("1.35."),
		"labels": cty.ObjectVal(map[string]cty.Value{
			"env": cty.StringVal("staging"),
		}),
		"node_locations": cty.ListVal([]cty.Value{cty.StringVal("us-central1-a")}),
	})
	clLabelsWarns := getModifiedIgnoredSettingsWarnings(bpPrev, bpModClusterLabels)
	c.Assert(clLabelsWarns, HasLen, 1)
	c.Check(clLabelsWarns[0], Matches, "(?s).*module \"cluster\".*setting 'labels' was modified.*system node pools.*")

	// Case 7: Modified gke-node-pool initial_node_count produces warning
	bpModNodeCount := bpPrev.Clone()
	bpModNodeCount.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
		"labels": cty.ObjectVal(map[string]cty.Value{
			"env": cty.StringVal("prod"),
		}),
		"initial_node_count": cty.NumberIntVal(5),
	})
	countWarns := getModifiedIgnoredSettingsWarnings(bpPrev, bpModNodeCount)
	c.Assert(countWarns, HasLen, 1)
	c.Check(countWarns[0], Matches, "(?s).*module \"np-1\".*setting 'initial_node_count' was modified.*gcloud container clusters resize.*")

	// Case 8: Modified vm-instance ssh-keys in metadata produces warning
	bpVMPrev := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "vm",
						Source: "modules/compute/vm-instance",
						Settings: config.NewDict(map[string]cty.Value{
							"metadata": cty.ObjectVal(map[string]cty.Value{
								"ssh-keys": cty.StringVal("user:ssh-rsa AAAAB3..."),
								"foo":      cty.StringVal("bar"),
							}),
						}),
					},
				},
			},
		},
	}
	bpVMCurr := bpVMPrev.Clone()
	bpVMCurr.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
		"metadata": cty.ObjectVal(map[string]cty.Value{
			"ssh-keys": cty.StringVal("user:ssh-rsa NEWKEY..."),
			"foo":      cty.StringVal("bar"),
		}),
	})
	vmWarns := getModifiedIgnoredSettingsWarnings(bpVMPrev, bpVMCurr)
	c.Assert(vmWarns, HasLen, 1)
	c.Check(vmWarns[0], Matches, "(?s).*module \"vm\".*'ssh-keys' within 'metadata' was modified.*OS Login.*")

	// Case 9: Modified netapp-storage-pool scale_type produces warning
	bpNetAppPrev := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "netapp",
						Source: "modules/file-system/netapp-storage-pool",
						Settings: config.NewDict(map[string]cty.Value{
							"scale_type": cty.StringVal("SCALE_TYPE_DEFAULT"),
						}),
					},
				},
			},
		},
	}
	bpNetAppCurr := bpNetAppPrev.Clone()
	bpNetAppCurr.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
		"scale_type": cty.StringVal("SCALE_TYPE_SCALEOUT"),
	})
	netappWarns := getModifiedIgnoredSettingsWarnings(bpNetAppPrev, bpNetAppCurr)
	c.Assert(netappWarns, HasLen, 1)
	c.Check(netappWarns[0], Matches, "(?s).*module \"netapp\".*setting \"scale_type\" was modified.*NetApp Volumes.*")

	// Case 10: Modified slurm controller nodeset/partitions produces warning
	bpSlurmPrev := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "slurm-controller",
						Source: "community/modules/scheduler/schedmd-slurm-gcp-v6-controller",
						Settings: config.NewDict(map[string]cty.Value{
							"nodeset_dyn": cty.ListVal([]cty.Value{cty.StringVal("dyn1")}),
						}),
					},
				},
			},
		},
	}
	bpSlurmCurr := bpSlurmPrev.Clone()
	bpSlurmCurr.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
		"nodeset_dyn": cty.ListVal([]cty.Value{cty.StringVal("dyn2")}),
	})
	slurmWarns := getModifiedIgnoredSettingsWarnings(bpSlurmPrev, bpSlurmCurr)
	c.Assert(slurmWarns, HasLen, 1)
	c.Check(slurmWarns[0], Matches, "(?s).*module \"slurm-controller\".*partition or nodeset definitions were modified.*target_size.*")

	// Case 11: Explicit null vs absent setting produces no warning
	bpNullPrev := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "np-null",
						Source: "modules/compute/gke-node-pool",
						Settings: config.NewDict(map[string]cty.Value{
							"initial_node_count": cty.NullVal(cty.Number),
						}),
					},
				},
			},
		},
	}
	bpNullCurr := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:       "np-null",
						Source:   "modules/compute/gke-node-pool",
						Settings: config.NewDict(map[string]cty.Value{}),
					},
				},
			},
		},
	}
	c.Check(getModifiedIgnoredSettingsWarnings(bpNullPrev, bpNullCurr), HasLen, 0)
	c.Check(getModifiedIgnoredSettingsWarnings(bpNullCurr, bpNullPrev), HasLen, 0)

	// Case 12: Explicit null on primary setting falls back to alias
	bpAliasPrev := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "cluster-alias",
						Source: "modules/scheduler/gke-cluster",
						Settings: config.NewDict(map[string]cty.Value{
							"version_prefix":     cty.NullVal(cty.String),
							"min_master_version": cty.StringVal("1.35."),
						}),
					},
				},
			},
		},
	}
	bpAliasCurr := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "cluster-alias",
						Source: "modules/scheduler/gke-cluster",
						Settings: config.NewDict(map[string]cty.Value{
							"version_prefix":     cty.NullVal(cty.String),
							"min_master_version": cty.StringVal("1.36."),
						}),
					},
				},
			},
		},
	}
	aliasWarns := getModifiedIgnoredSettingsWarnings(bpAliasPrev, bpAliasCurr)
	c.Assert(aliasWarns, HasLen, 1)
	c.Check(aliasWarns[0], Matches, "(?s).*module \"cluster-alias\".*control-plane version setting was modified.*gcloud container clusters upgrade.*")
}
