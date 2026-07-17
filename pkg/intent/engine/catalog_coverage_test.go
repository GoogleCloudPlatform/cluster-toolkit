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

package engine_test

import (
	"testing"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/intent/engine"
)

// escapeHatchFeatures are features deliberately absent from every shipped cluster-config,
// and legitimately so: they exist to reference infrastructure the user ALREADY owns, which
// by definition no reference blueprint can create.
//
// The value is the settings needed to compile the feature on each base it supports.
// TestNoOrphanedCatalogEntries consults the key set; this test compiles the entries.
//
// Adding to this map is a deliberate act. The bar is "a blueprint could not possibly
// reference this because it depends on the user's pre-existing environment" -- NOT "we
// could not find a blueprint that uses it". Nine overlays failed that second, weaker bar and
// were deleted rather than parked here.
var escapeHatchFeatures = map[string]map[string]map[string]any{
	// modules/file-system/pre-existing-network-storage is used by three shipped blueprints
	// (a4high-, a4xhigh- and a4xmax- slurm), but always to mount a bucket the SAME blueprint
	// just created -- a job our aiml-gcsfuse feature already does end to end. The standalone
	// feature covers the other half: storage that predates the deployment.
	"pre-existing-network-storage": {
		"slurm": {
			"server_ip":    "10.0.0.2",
			"remote_mount": "/export/home",
			"local_mount":  "/home",
		},
		"jbvm": {
			"server_ip":    "10.0.0.2",
			"remote_mount": "/export/home",
			"local_mount":  "/home",
		},
		"gke": {
			"gcs_bucket_name": "an-existing-bucket",
			"local_mount":     "/data",
		},
	},
	"startup-script": {
		"slurm": {
			"runners": []any{
				map[string]any{
					"type":        "shell",
					"destination": "custom_init.sh",
					"content":     "#!/bin/bash\necho hello\n",
				},
			},
		},
		"jbvm": {
			"runners": []any{
				map[string]any{
					"type":        "shell",
					"destination": "custom_init.sh",
					"content":     "#!/bin/bash\necho hello\n",
				},
			},
		},
	},
}

// TestEscapeHatchFeaturesCompile compiles every escape-hatch feature on every base it
// declares support for.
//
// This replaces examples/cluster-configs/overlay-tests/gke-overlays.yaml. That file existed
// only to give unreferenced catalog entries a home, and it had to be stamped "NOT A
// REFERENCE ARCHITECTURE. DO NOT DEPLOY." A synthetic in-memory config does the same job
// without shipping a config nobody should use, and it states the required settings inline
// where a reader can see them.
func TestEscapeHatchFeaturesCompile(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	if len(escapeHatchFeatures) == 0 {
		t.Fatal("escapeHatchFeatures is empty; TestNoOrphanedCatalogEntries' exemption is then dead code")
	}

	for featureType, perBase := range escapeHatchFeatures {
		if len(perBase) == 0 {
			t.Errorf("escape hatch %q lists no bases to compile on", featureType)
			continue
		}
		for base, settings := range perBase {
			t.Run(featureType+"/"+base, func(t *testing.T) {
				cfg := ast.ClusterConfig{
					ConfigBase: ast.ConfigBaseReference{Type: base},
					Vars:       escapeHatchVars(base),
					ComputeArchetypes: []ast.ArchetypeReference{{
						Name:        "pool",
						MachineType: "a3-ultragpu-8g", // explicit archetype: no live GCP lookup
					}},
					Features: []ast.FeatureReference{{
						Name:     "existing",
						Type:     featureType,
						Settings: settings,
					}},
				}
				groups, err := compiler.Compile(cfg)
				if err != nil {
					t.Fatalf("compile failed: %v", err)
				}
				if len(groups) == 0 {
					t.Fatal("expected non-empty deployment groups")
				}
				serialized, err := engine.Serialize(cfg.ConfigBase.Type, cfg.Vars, groups)
				if err != nil {
					t.Fatalf("serialize failed: %v", err)
				}
				if _, _, err := config.NewBlueprintFromYamlBytes([]byte(serialized)); err != nil {
					t.Fatalf("blueprint construction failed: %v", err)
				}
			})
		}
	}
}

func escapeHatchVars(base string) map[string]any {
	vars := map[string]any{
		"deployment_name": "esc-hatch",
		"project_id":      "hpc-toolkit-dev",
		"region":          "us-central1",
		"zone":            "us-central1-a",
	}
	if base == "gke" {
		vars["authorized_cidr"] = "0.0.0.0/0"
		vars["version_prefix"] = "1.35."
	}
	return vars
}
