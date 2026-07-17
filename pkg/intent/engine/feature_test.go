// Copyright 2026 "Google LLC"
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

package engine

import (
	"os"
	"strings"
	"testing"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/sourcereader"
)

func TestEvaluateFeature(t *testing.T) {
	sourcereader.ModuleFS = mockFS{os.DirFS("../../../")}

	manifest := ast.FeatureManifest{
		Name: "managed-lustre",
		BlueprintBlock: ast.BlueprintBlock{
			DeploymentGroups: []ast.DeploymentGroup{
				{
					Group: "storage",
					Modules: []ast.ModuleSpec{
						{
							ID:     "{name}",
							Source: "modules/file-system/managed-lustre",
							Export: true,
							Settings: map[string]any{
								"base_setting": "base",
							},
						},
						{
							ID:        "{name}_network_peering",
							Source:    "modules/network/peering",
							Condition: "connect_mode == 'PRIVATE'",
							Use:       []string{"{name}"},
						},
					},
				},
			},
		},
	}

	vars := map[string]any{
		"project_id": "test-project",
	}

	t.Run("Hydrates and passes condition", func(t *testing.T) {
		ref := ast.FeatureReference{
			Name: "lustre1",
			Type: "managed-lustre",
			Settings: map[string]any{
				"connect_mode": "PRIVATE",
				"size_gib":     36000,
			},
		}

		groups, err := EvaluateFeature(ref, manifest, vars, "slurm")
		if err != nil {
			t.Fatalf("EvaluateFeature failed: %v", err)
		}

		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}

		mods := groups[0].Modules
		if len(mods) != 2 {
			t.Fatalf("expected 2 modules, got %d", len(mods))
		}

		// Check primary module hydration and injection
		fs := mods[0]
		if fs.ID != "lustre1" {
			t.Errorf("expected lustre1, got %s", fs.ID)
		}
		if fs.Settings["size_gib"] != 36000 {
			t.Errorf("expected size_gib 36000, got %v", fs.Settings["size_gib"])
		}
		if fs.Settings["base_setting"] != "base" {
			t.Errorf("expected base_setting base, got %v", fs.Settings["base_setting"])
		}

		// Check conditional module hydration
		peer := mods[1]
		if peer.ID != "lustre1_network_peering" {
			t.Errorf("expected lustre1_network_peering, got %s", peer.ID)
		}
		if len(peer.Use) != 1 || peer.Use[0] != "lustre1" {
			t.Errorf("expected use: [lustre1_fs], got %v", peer.Use)
		}
	})

	testEvaluateFeatureConditionAndBaseGroups(t, manifest, vars)
	testEvaluateFeatureErrors(t, manifest, vars)
}

func testEvaluateFeatureConditionAndBaseGroups(t *testing.T, manifest ast.FeatureManifest, vars map[string]any) {
	t.Helper()
	t.Run("Fails condition and drops module", func(t *testing.T) {
		ref := ast.FeatureReference{
			Name: "lustre2",
			Type: "managed-lustre",
			Settings: map[string]any{
				"connect_mode": "PUBLIC",
				"size_gib":     12000,
			},
		}

		groups, err := EvaluateFeature(ref, manifest, vars, "slurm")
		if err != nil {
			t.Fatalf("EvaluateFeature failed: %v", err)
		}

		if len(groups) != 1 {
			t.Fatalf("expected 1 group, got %d", len(groups))
		}

		mods := groups[0].Modules
		if len(mods) != 1 {
			t.Fatalf("expected 1 module (peering dropped), got %d", len(mods))
		}

		if mods[0].ID != "lustre2" {
			t.Errorf("expected lustre2, got %s", mods[0].ID)
		}
	})

	t.Run("ConfigBase groups are appended correctly", func(t *testing.T) {
		// Create a manifest that has a generic group and a base-specific group
		manifestWithBase := manifest
		manifestWithBase.ConfigBase = map[string]ast.BlueprintBlock{
			"slurm": {
				DeploymentGroups: []ast.DeploymentGroup{
					{
						Group: "slurm-specific",
						Modules: []ast.ModuleSpec{
							{ID: "slurm-extra-mod"},
						},
					},
				},
			},
		}

		ref := ast.FeatureReference{Name: "lustre3", Settings: map[string]any{"connect_mode": "PUBLIC"}}
		groups, err := EvaluateFeature(ref, manifestWithBase, vars, "slurm")
		if err != nil {
			t.Fatalf("EvaluateFeature failed: %v", err)
		}

		// Should have the generic "storage" group AND the "slurm-specific" group
		if len(groups) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(groups))
		}
		if groups[1].Group != "slurm-specific" {
			t.Errorf("expected slurm-specific group, got %s", groups[1].Group)
		}
	})
}

func testEvaluateFeatureErrors(t *testing.T, manifest ast.FeatureManifest, vars map[string]any) {
	t.Helper()
	t.Run("Fails if condition references undefined variable", func(t *testing.T) {
		ref := ast.FeatureReference{
			Name: "lustre4",
			// We omit "connect_mode" from Settings
			Settings: map[string]any{},
		}

		_, err := EvaluateFeature(ref, manifest, vars, "slurm")
		if err == nil {
			t.Fatalf("expected EvaluateFeature to fail due to missing CEL variable")
		}
		if !strings.Contains(err.Error(), "undeclared reference to 'connect_mode'") {
			t.Errorf("expected error about missing key connect_mode, got %v", err)
		}
	})

	t.Run("Handles deeply nested settings injection", func(t *testing.T) {
		ref := ast.FeatureReference{
			Name: "lustre5",
			Settings: map[string]any{
				"connect_mode": "PRIVATE",
				"nested": map[string]any{
					"key1": "value1",
				},
			},
		}

		groups, err := EvaluateFeature(ref, manifest, vars, "slurm")
		if err != nil {
			t.Fatalf("EvaluateFeature failed: %v", err)
		}

		fs := groups[0].Modules[0]
		nestedMap, ok := fs.Settings["nested"].(map[string]any)
		if !ok || nestedMap["key1"] != "value1" {
			t.Errorf("deep merge failed for nested setting")
		}
	})

	t.Run("Rejects unknown setting when module has valid variables.tf schema", func(t *testing.T) {
		manifestNoPrimary := manifest
		manifestNoPrimary.DeploymentGroups = []ast.DeploymentGroup{
			{
				Group: "storage",
				Modules: []ast.ModuleSpec{
					{
						ID:       "{name}_fs",
						Source:   "modules/file-system/managed-lustre",
						Settings: map[string]any{},
					},
				},
			},
		}

		ref := ast.FeatureReference{
			Name: "lustre6",
			Settings: map[string]any{
				"should_not_inject": true,
			},
		}

		_, err := EvaluateFeature(ref, manifestNoPrimary, vars, "slurm")
		if err == nil || !strings.Contains(err.Error(), "sets unknown key(s)") {
			t.Errorf("expected unknown key error for should_not_inject, got: %v", err)
		}
	})

	t.Run("Fails if feature does not support requested config_base", func(t *testing.T) {
		manifestGkeOnly := ast.FeatureManifest{
			Name: "kueue",
			ConfigBase: map[string]ast.BlueprintBlock{
				"gke": {
					DeploymentGroups: []ast.DeploymentGroup{
						{Group: "cluster"},
					},
				},
			},
		}

		ref := ast.FeatureReference{Name: "kueue-test", Type: "kueue"}
		_, err := EvaluateFeature(ref, manifestGkeOnly, vars, "slurm")
		if err == nil {
			t.Fatalf("expected error for unsupported config_base, got nil")
		}
		if !strings.Contains(err.Error(), "feature \"kueue\" does not support config_base \"slurm\"") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
