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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/intent/engine"

	"gopkg.in/yaml.v3"
)

// matrixMachineType maps an archetype manifest file to a machine type that resolves to it.
//
// The manifest's own `machine_type` field is not always usable: `generic-tpu` and `h4d-rdma`
// carry a family label there, not a real SKU, and only their aliases name bookable machines.
//
// generic-cpu is deliberately absent. It is the catch-all, so an arbitrary CPU SKU fails
// HasExplicitArchetype and compiler PHASE 2 validates it against the live GCP API -- which
// would make this test non-hermetic and flaky offline. Its four bases are already compiled
// end-to-end by hpc-slurm, hpc-gke, batch-hpc, minimal and a3ultra-jbvm.
var matrixMachineType = map[string]string{
	"a3-megagpu-8g":  "a3-megagpu-8g",
	"a3-ultragpu-8g": "a3-ultragpu-8g",
	"a4-highgpu-8g":  "a4-highgpu-8g",
	"a4x-highgpu-4g": "a4x-highgpu-4g",
	"tpu-v6e":        "ct6e-standard-4t",
}

// TestArchetypeBaseMatrixCompiles compiles every (archetype, config_base) pair the catalog
// declares, using a synthetic minimal cluster-config held in memory.
//
// WHY THIS EXISTS: an archetype section that no shipped example happens to select is dead
// code that still ships. Four such sections were carrying real defects -- a reference to
// `multivpc.subnetwork_interfaces_gke`, an output that module does not have; three archetypes
// missing `kueue_configuration_path`; an unused `local_ssd_mountpoint` -- and none of it
// surfaced until a cluster-config was written that touched them.
//
// That coverage is fragile by construction: it disappears the moment an example is deleted.
// Removing the three Packer-dependent Slurm configs (a3high, a3mega, a4xmax) did exactly
// that. This test decouples archetype coverage from the example set entirely.
func TestArchetypeBaseMatrixCompiles(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	entries, err := os.ReadDir(filepath.Join(repoRoot, "v2", "compute-archetypes"))
	if err != nil {
		t.Fatalf("failed to list v2/compute-archetypes: %v", err)
	}

	compiler := engine.NewCompiler(repoRoot)
	seen := map[string]bool{}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		archetype := entry.Name()[:len(entry.Name())-len(".yaml")]
		machineType, covered := matrixMachineType[archetype]
		if !covered {
			// cpu is the documented exclusion. Anything else is a new archetype
			// whose author forgot this table, and it must not pass silently.
			if archetype == "cpu" {
				continue
			}
			t.Errorf("archetype %q has no entry in matrixMachineType; add one so its "+
				"config_base sections are compiled", archetype)
			continue
		}

		data, err := os.ReadFile(filepath.Join(repoRoot, "v2", "compute-archetypes", entry.Name()))
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		var manifest ast.ComputeArchetypeManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			t.Fatalf("failed to unmarshal %s: %v", entry.Name(), err)
		}
		if len(manifest.ConfigBase) == 0 {
			t.Errorf("archetype %q declares no config_base sections", archetype)
			continue
		}

		for base := range manifest.ConfigBase {
			seen[archetype+"/"+base] = true
			t.Run(archetype+"/"+base, func(t *testing.T) {
				compileArchetypeBasePair(t, compiler, base, archetype, machineType)
			})
		}
	}

	if len(seen) == 0 {
		t.Fatal("matrix compiled nothing; compute-archetypes/ enumeration is broken")
	}
	t.Logf("compiled %d archetype/config_base pairs", len(seen))
}

func compileArchetypeBasePair(t *testing.T, compiler *engine.Compiler, base, archetype, machineType string) {
	t.Helper()
	cfg := syntheticClusterConfig(base, archetype, machineType)
	groups, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if len(groups) == 0 {
		t.Fatalf("expected non-empty deployment groups")
	}
	// Same reasoning as TestAllExampleClusterConfigsCompile: the compiler
	// emits an AST, but gcluster consumes HCL. Malformed expressions only
	// fail here. NewBlueprintFromYamlBytes makes no GCP calls.
	serialized, err := engine.Serialize(cfg.ConfigBase.Type, cfg.Vars, groups)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}
	if _, _, err := config.NewBlueprintFromYamlBytes([]byte(serialized)); err != nil {
		t.Fatalf("blueprint construction failed: %v", err)
	}
}

// syntheticClusterConfig builds the smallest cluster-config that exercises one archetype on
// one base. No features and no overlays: this test is about the archetype section itself, and
// feature wiring is covered by the shipped examples.
func syntheticClusterConfig(base, archetype, machineType string) ast.ClusterConfig {
	vars := map[string]any{
		// Deployment names are validated, so keep it short and alphanumeric-plus-dash.
		"deployment_name": fmt.Sprintf("mtx-%d", len(archetype)+len(base)),
		"project_id":      "hpc-toolkit-dev",
		"region":          "us-central1",
		"zone":            "us-central1-a",
	}
	if base == "gke" {
		// gke.yaml declares these with no default; the compiler's PHASE 0 gate rejects
		// the config without them.
		vars["authorized_cidr"] = "0.0.0.0/0"
		vars["version_prefix"] = "1.35."
	}
	return ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: base},
		Vars:       vars,
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "mtxpool",
			MachineType: machineType,
		}},
	}
}
