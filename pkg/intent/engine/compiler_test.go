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

package engine_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/intent/engine"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"
)

// realFS adapts an os.DirFS to the ReadDir/ReadFile surface sourcereader.ModuleFS expects,
// letting a test serve the repository's genuine module sources in place of the embedded FS.
type realFS struct {
	fs.FS
}

func (r realFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(r.FS, name)
}

func (r realFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(r.FS, name)
}

func TestCompiler(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "compiler-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Setup Mock Workspace Directories
	os.MkdirAll(filepath.Join(tempDir, "v2", "config-base"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "v2", "compute-archetypes"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "modules", "vpc"), 0755)     // For validator
	os.MkdirAll(filepath.Join(tempDir, "modules", "nodeset"), 0755) // For validator

	// Write mock variables.tf for validator to succeed
	os.WriteFile(filepath.Join(tempDir, "modules", "vpc", "variables.tf"), []byte(`
variable "network_name" {}
`), 0644)
	os.WriteFile(filepath.Join(tempDir, "modules", "nodeset", "variables.tf"), []byte(`
variable "machine_type" {}
variable "node_count" {}
`), 0644)

	// Write mock Base Skeleton (Tier 2)
	baseYaml := `
name: slurm
aliases:
  network: base_network
deployment_groups:
  - group: primary
    modules:
      - id: base_network
        source: ` + filepath.Join(tempDir, "modules", "vpc") + `
        settings:
          network_name: "base-slurm-net"
`
	os.WriteFile(filepath.Join(tempDir, "v2", "config-base", "slurm.yaml"), []byte(baseYaml), 0644)

	// Write mock Archetype (Tier 3)
	archYaml := `
name: a3-ultragpu-8g
config_base:
  slurm:
    deployment_groups:
      - group: primary
        modules:
          - id: "{name}_rdma_net"
            source: ` + filepath.Join(tempDir, "modules", "vpc") + `
          - id: "{name}_nodeset"
            source: ` + filepath.Join(tempDir, "modules", "nodeset") + `
            use: [network, "{name}_rdma_net"]
            settings:
              machine_type: a3-ultragpu-8g
`
	os.WriteFile(filepath.Join(tempDir, "v2", "compute-archetypes", "a3ultra.yaml"), []byte(archYaml), 0644)

	// 2. Execute Compiler
	compiler := engine.NewCompiler(tempDir)

	userConfig := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		// project_id is a mandatory input enforced by Compile's PHASE 0 gate.
		Vars: map[string]any{"project_id": "test-project"},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "a3u-training",
				MachineType: "a3-ultragpu-8g",
				Settings: map[string]any{
					"node_count": 32, // Validated against variables.tf
				},
			},
		},
	}

	finalGroups, err := compiler.Compile(userConfig)
	if err != nil {
		t.Fatalf("Compiler failed: %v", err)
	}

	// 3. Assert Results
	if len(finalGroups) != 1 {
		t.Fatalf("Expected 1 deployment group, got %d", len(finalGroups))
	}

	assertCompiledSlurmPrimaryGroup(t, finalGroups[0])

	// 4. Test Validation Failure Catching
	badUserConfig := userConfig
	badUserConfig.ComputeArchetypes[0].Settings = map[string]any{
		"typo_count": 32,
	}

	_, err = compiler.Compile(badUserConfig)
	if err == nil {
		t.Fatal("Expected compiler to fail validation due to typo in settings, got nil")
	}
}

func assertCompiledSlurmPrimaryGroup(t *testing.T, primary ast.DeploymentGroup) {
	t.Helper()
	if primary.Group != "primary" {
		t.Errorf("Expected group 'primary', got %q", primary.Group)
	}

	// Check Base + Hydrated Compute Pool Modules
	if len(primary.Modules) != 3 {
		t.Fatalf("Expected 3 modules in primary group (base_network, rdma_net, nodeset), got %d", len(primary.Modules))
	}
	if primary.Modules[0].ID != "base_network" {
		t.Errorf("Expected module ID 'base_network', got %q", primary.Modules[0].ID)
	}

	// Check Hydrated Compute Pool
	var nodeset *ast.ModuleSpec
	for i := range primary.Modules {
		if primary.Modules[i].IsComputePool {
			nodeset = &primary.Modules[i]
			break
		}
	}
	if nodeset == nil {
		t.Fatalf("Expected compute pool module in primary.Modules")
	}
	if nodeset.ID != "a3u-training_nodeset" {
		t.Errorf("Expected module ID 'a3u-training_nodeset', got %q", nodeset.ID)
	}
	if nodeset.Settings["node_count"] != 32 {
		t.Errorf("Expected node_count 32, got %v", nodeset.Settings["node_count"])
	}

	// Check Alias Resolution
	if len(nodeset.Use) != 2 {
		t.Fatalf("Expected 2 use items, got %d", len(nodeset.Use))
	}
	if nodeset.Use[0] != "base_network" {
		t.Errorf("Expected alias 'network' to be resolved to 'base_network', got %q", nodeset.Use[0])
	}
	if nodeset.Use[1] != "a3u-training_rdma_net" {
		t.Errorf("Expected '{name}_rdma_net' to be hydrated, got %q", nodeset.Use[1])
	}
}

func TestCompiler_GenericCPUFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "compiler-test-cpu-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	os.MkdirAll(filepath.Join(tempDir, "v2", "config-base"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "v2", "compute-archetypes"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "modules", "nodeset"), 0755)

	os.WriteFile(filepath.Join(tempDir, "modules", "nodeset", "variables.tf"), []byte(`
variable "machine_type" {}
variable "node_count" {}
`), 0644)

	baseYaml := `
name: slurm
deployment_groups:
  - group: primary
`
	os.WriteFile(filepath.Join(tempDir, "v2", "config-base", "slurm.yaml"), []byte(baseYaml), 0644)

	genericCpuYaml := `
name: generic-cpu
match_patterns: ["*"]
config_base:
  slurm:
    deployment_groups:
      - group: primary
        modules:
          - id: "{name}_nodeset"
            source: ` + filepath.Join(tempDir, "modules", "nodeset") + `
            settings:
              machine_type: "{machine_type}"
`
	os.WriteFile(filepath.Join(tempDir, "v2", "compute-archetypes", "generic-cpu.yaml"), []byte(genericCpuYaml), 0644)

	compiler := engine.NewCompiler(tempDir)
	userConfig := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		// project_id is a mandatory input enforced by Compile's PHASE 0 gate.
		Vars: map[string]any{"project_id": "test-project"},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "cpu-pool",
				MachineType: "n2-standard-60",
				Settings: map[string]any{
					"node_count": 20,
				},
			},
		},
	}

	finalGroups, err := compiler.Compile(userConfig)
	if err != nil {
		t.Fatalf("Compiler failed for generic CPU fallback: %v", err)
	}
	if len(finalGroups) != 1 {
		t.Fatalf("Expected 1 deployment group, got %d", len(finalGroups))
	}
	if len(finalGroups[0].Modules) != 1 {
		t.Fatalf("Expected 1 compute pool module, got %d", len(finalGroups[0].Modules))
	}
	nodeset := finalGroups[0].Modules[0]
	if nodeset.ID != "cpu-pool_nodeset" {
		t.Errorf("Expected module ID 'cpu-pool_nodeset', got %q", nodeset.ID)
	}
	if nodeset.Settings["machine_type"] != "n2-standard-60" {
		t.Errorf("Expected machine_type 'n2-standard-60', got %v", nodeset.Settings["machine_type"])
	}
	if nodeset.Settings["node_count"] != 20 {
		t.Errorf("Expected node_count 20, got %v", nodeset.Settings["node_count"])
	}
}

func TestAllExampleClusterConfigsCompile(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	// Point the module reader at the REAL repository, not at a mock.
	//
	// TestAutowire installs a mock ModuleFS containing stub modules at real paths
	// (e.g. modules/file-system/managed-lustre with an outputs.tf and no variables).
	// modInfoCache is global and survives that test's ModuleFS restore, so without this
	// reset every schema lookup below would return the stub's empty input set and this
	// end-to-end test would validate settings against fabricated schemas.
	oldFS := sourcereader.ModuleFS
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}
	sourcereader.ModuleFS = realFS{os.DirFS(absRoot)}
	modulereader.ClearModuleInfoCache()
	defer func() {
		sourcereader.ModuleFS = oldFS
		modulereader.ClearModuleInfoCache()
	}()

	// Both the shipped reference configs and the overlay coverage configs. The latter
	// exist only so that catalog entries absent from any real blueprint are still
	// compiled; see examples/cluster-configs/overlay-tests/.
	paths, err := clusterConfigPaths(repoRoot)
	if err != nil {
		t.Fatalf("Failed to enumerate cluster configs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("No cluster config files found under %s", filepath.Join(repoRoot, "v2", "cluster-configs"))
	}

	compiler := engine.NewCompiler(repoRoot)
	for _, cfgPath := range paths {
		name := filepath.Base(cfgPath)
		t.Run(name, func(t *testing.T) {
			compileAndVerifyExampleConfig(t, compiler, cfgPath, name)
		})
	}
}

func compileAndVerifyExampleConfig(t *testing.T, compiler *engine.Compiler, cfgPath, name string) {
	t.Helper()
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", name, err)
	}
	// Parse strictly: this test is also the guard that no shipped config
	// grows an unknown or misspelled top-level key, which the compiler
	// would otherwise silently discard.
	cfg, err := ast.UnmarshalClusterConfigStrict(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal %s: %v", name, err)
	}

	// The shipped cluster-configs deliberately leave deployment-specific
	// values blank: they are placeholders the user fills in, and committing
	// real ones would leak project identifiers into a public repository.
	// Supply them here so this test exercises compilation rather than
	// re-asserting that the placeholders are still blank.
	if cfg.Vars == nil {
		cfg.Vars = map[string]any{}
	}
	for k, v := range map[string]any{
		"project_id":      "test-project",
		"region":          "us-central1",
		"zone":            "us-central1-a",
		"authorized_cidr": "10.0.0.0/8",
	} {
		if cur, ok := cfg.Vars[k]; !ok || cur == nil || cur == "" {
			cfg.Vars[k] = v
		}
	}

	groups, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatalf("Compiler failed on %s: %v", name, err)
	}
	if len(groups) == 0 {
		t.Fatalf("Expected non-empty deployment groups for %s", name)
	}

	// Compiling is only half the journey. The compiler emits an AST; the
	// blueprint is what `gcluster` actually consumes, and it is parsed as HCL.
	// Literal `$(...)` in overlay shell payloads, malformed expressions and
	// structurally invalid groups all compile cleanly and only fail here.
	// This stays hermetic -- NewBlueprintFromYamlBytes makes no GCP calls.
	serialized, err := engine.Serialize(cfg.ConfigBase.Type, cfg.Vars, groups)
	if err != nil {
		t.Fatalf("Serialize failed on %s: %v", name, err)
	}
	if _, _, err := config.NewBlueprintFromYamlBytes([]byte(serialized)); err != nil {
		t.Fatalf("Blueprint construction failed on %s: %v", name, err)
	}
}

// withRealModuleFS points the module reader at the genuine repository sources for the
// duration of a test, and clears the global schema cache on both entry and exit so no
// mock schema leaks in or out. Returns the repo root.
func withRealModuleFS(t *testing.T) string {
	t.Helper()
	repoRoot := filepath.Join("..", "..", "..")
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}
	oldFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = realFS{os.DirFS(absRoot)}
	modulereader.ClearModuleInfoCache()
	t.Cleanup(func() {
		sourcereader.ModuleFS = oldFS
		modulereader.ClearModuleInfoCache()
	})
	return repoRoot
}

// TestUnknownSettingKeyIsRejected is the proof-of-life for validateSettingKeys.
//
// Without it, TestAllExampleClusterConfigsCompile passing is ambiguous: the check might
// simply be abstaining because no schema was readable. The positive control below pins
// that down by compiling the SAME config with the correctly spelled key.
func TestUnknownSettingKeyIsRejected(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	base := func(key string) ast.ClusterConfig {
		return ast.ClusterConfig{
			ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
			Vars: map[string]any{
				"project_id":      "test-project",
				"deployment_name": "test-dep",
				"region":          "us-central1",
				"zone":            "us-central1-a",
			},
			// Required by Phase 0. An explicit catalog archetype keeps the test
			// hermetic; an unrecognised machine type would trigger a live GCP lookup.
			ComputeArchetypes: []ast.ArchetypeReference{{
				Name:        "gpu-pool",
				MachineType: "a3-ultragpu-8g",
			}},
			Features: []ast.FeatureReference{{
				Name: "homefs",
				Type: "managed-lustre",
				Settings: map[string]any{
					key:            18000,
					"remote_mount": "lustrefs",
				},
			}},
		}
	}

	// Positive control: the real variables.tf declares size_gib, so this must compile.
	if _, err := compiler.Compile(base("size_gib")); err != nil {
		t.Fatalf("control config with valid key size_gib failed to compile: %v", err)
	}

	// Negative: one character off. Must be rejected, not silently dropped.
	_, err := compiler.Compile(base("size_gb"))
	if err == nil {
		t.Fatal("expected typo'd setting key \"size_gb\" to be rejected, but compilation succeeded")
	}
	if !strings.Contains(err.Error(), "size_gb") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
	if !strings.Contains(err.Error(), `did you mean "size_gib"?`) {
		t.Errorf("error should suggest the correct spelling, got: %v", err)
	}
}

// TestOverlayOnlyFeatureCannotBeSelectedDirectly guards the Tier 4/Tier 5 split: a feature
// marked `requires_overlay` carries no content, so naming one under `features:` would emit a
// billed no-op module.
func TestOverlayOnlyFeatureCannotBeSelectedDirectly(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars: map[string]any{
			"project_id":      "test-project",
			"deployment_name": "test-dep",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		// Required by Phase 0. An explicit catalog archetype keeps the test hermetic.
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "gpu-pool",
			MachineType: "a3-ultragpu-8g",
		}},
		Features: []ast.FeatureReference{{Name: "job", Type: "gke-job-template"}},
	}

	_, err := compiler.Compile(cfg)
	if err == nil {
		t.Fatal("expected overlay-only feature \"gke-job-template\" to be rejected under features:")
	}
	// The message must do more than state the rule: it must name a shipped overlay the user
	// can paste under `overlays:`. run-nvidia-smi includes `type: gke-job-template` in `features:`.
	if !strings.Contains(err.Error(), "run-nvidia-smi") {
		t.Errorf("error should name an overlay that supplies content for gke-job-template, got: %v", err)
	}
	if !strings.Contains(err.Error(), "overlays:") {
		t.Errorf("error should point the user at the `overlays:` list, got: %v", err)
	}
}

// TestMandatoryInputsAreEnforced pins the other half of the contract: the two values that
// genuinely have no default must fail at the intent layer, not at terraform plan.
func TestMandatoryInputsAreEnforced(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	pool := []ast.ArchetypeReference{{Name: "pool", MachineType: "a3-ultragpu-8g"}}

	if _, err := compiler.Compile(ast.ClusterConfig{
		Vars:              map[string]any{"project_id": "p"},
		ComputeArchetypes: pool,
	}); err == nil || !strings.Contains(err.Error(), "config_base") {
		t.Errorf("missing config_base should be rejected by name, got: %v", err)
	}

	for _, vars := range []map[string]any{nil, {}, {"project_id": nil}, {"project_id": ""}} {
		_, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase:        ast.ConfigBaseReference{Type: "gke"},
			Vars:              vars,
			ComputeArchetypes: pool,
		})
		if err == nil || !strings.Contains(err.Error(), "project_id") {
			t.Errorf("vars=%v: missing project_id should be rejected by name, got: %v", vars, err)
		}
	}
}

// clusterConfigPaths returns every cluster-config YAML: the shipped reference configs in
// v2/cluster-configs/ plus the overlay coverage configs in its overlay-tests/
// subdirectory.
//
// The split matters. A reference config carries only what its blueprint actually
// contains, so purging inauthentic overlays left ten catalog entries referenced by
// nothing. Rather than weaken the orphan rule, the coverage configs give those entries a
// home that is clearly labelled as not-a-reference-architecture.
func clusterConfigPaths(repoRoot string) ([]string, error) {
	base := filepath.Join(repoRoot, "v2", "cluster-configs")
	paths, err := filepath.Glob(filepath.Join(base, "*.yaml"))
	if err != nil {
		return nil, err
	}
	extra, err := filepath.Glob(filepath.Join(base, "overlay-tests", "*.yaml"))
	if err != nil {
		return nil, err
	}
	return append(paths, extra...), nil
}

// TestSharedPrivateServiceAccessAcrossFeatures verifies that:
//  1. Multiple managed-lustre features deduplicate into a single shared private_service_access module.
//  2. A default filestore (DIRECT_PEERING) drops private_service_access from its use list even when
//     managed-lustre creates private_service_access in the same deployment.
//  3. A filestore with connect_mode: PRIVATE_SERVICE_ACCESS shares that same single private_service_access module.
func TestSharedPrivateServiceAccessAcrossFeatures(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "test-project",
			"deployment_name": "test-psa",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "pool",
			MachineType: "a3-ultragpu-8g",
		}},
		Features: []ast.FeatureReference{
			{
				Name: "lustre1",
				Type: "managed-lustre",
				Settings: map[string]any{
					"size_gib":     18000,
					"local_mount":  "/mnt/l1",
					"remote_mount": "lustre1",
				},
			},
			{
				Name: "lustre2",
				Type: "managed-lustre",
				Settings: map[string]any{
					"size_gib":     18000,
					"local_mount":  "/mnt/l2",
					"remote_mount": "lustre2",
				},
			},
			{
				Name: "homefs",
				Type: "filestore",
				Settings: map[string]any{
					"local_mount": "/home",
				},
			},
			{
				Name: "psafs",
				Type: "filestore",
				Settings: map[string]any{
					"connect_mode": "PRIVATE_SERVICE_ACCESS",
					"local_mount":  "/mnt/psa",
				},
			},
		},
	}

	groups, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	psaCount := 0
	var homefsUses, psafsUses []string
	for _, g := range groups {
		for _, m := range g.Modules {
			if m.ID == "private_service_access" {
				psaCount++
			}
			if m.ID == "homefs" {
				homefsUses = m.Use
			}
			if m.ID == "psafs" {
				psafsUses = m.Use
			}
		}
	}

	if psaCount != 1 {
		t.Errorf("expected exactly 1 private_service_access module across 2 Lustre + 1 PSA Filestore, got %d", psaCount)
	}

	for _, u := range homefsUses {
		if u == "private_service_access" {
			t.Errorf("default DIRECT_PEERING filestore 'homefs' should NOT use private_service_access, got use: %v", homefsUses)
		}
	}

	foundPSA := false
	for _, u := range psafsUses {
		if u == "private_service_access" {
			foundPSA = true
		}
	}
	if !foundPSA {
		t.Errorf("PSA filestore 'psafs' should use private_service_access, got use: %v", psafsUses)
	}
}

// TestArchetypeVarOverrideIsPoolScoped verifies that setting an archetype variable
// inside `compute_archetypes[].settings` scopes the override to that pool only,
// leaving other pools to read the user's global var (or archetype default).
func TestArchetypeVarOverrideIsPoolScoped(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars: map[string]any{
			"project_id":      "test-project",
			"deployment_name": "test-dep",
			"region":          "us-central1",
			"zone":            "us-central1-a",
			"authorized_cidr": "10.0.0.0/8",
			"tpu_num_slices":  2, // user global
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "tpu-a", MachineType: "ct6e-standard-4t", Settings: map[string]any{"tpu_num_slices": 3}},
			{Name: "tpu-b", MachineType: "ct6e-standard-4t"},
			// A trailing pool re-applies archetype defaults; it must not clobber tpu-a.
			{Name: "tpu-c", MachineType: "ct6e-standard-4t"},
		},
	}

	groups, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got := map[string]any{}
	for _, g := range groups {
		for _, m := range g.Modules {
			if m.IsComputePool {
				got[m.ID] = m.Settings["num_slices"]
			}
		}
	}
	if got["tpu-a_pool"] != "$(vars.tpu_a_tpu_num_slices)" || cfg.Vars["tpu_a_tpu_num_slices"] != 3 {
		t.Errorf("tpu-a_pool num_slices = %v (var=%v), want $(vars.tpu_a_tpu_num_slices) with value 3", got["tpu-a_pool"], cfg.Vars["tpu_a_tpu_num_slices"])
	}
	for _, id := range []string{"tpu-b_pool", "tpu-c_pool"} {
		if got[id] != "$(vars.tpu_num_slices)" {
			t.Errorf("%s num_slices = %v, want it to read the global var $(vars.tpu_num_slices)", id, got[id])
		}
	}
	if cfg.Vars["tpu_num_slices"] != 2 {
		t.Errorf("global tpu_num_slices = %v, want user value 2 (pool override must not leak)", cfg.Vars["tpu_num_slices"])
	}
}

func TestArchetypeVarExpressionOverride(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars: map[string]any{
			"project_id":      "test-project",
			"deployment_name": "test-dep",
			"region":          "us-central1",
			"zone":            "us-central1-a",
			"authorized_cidr": "10.0.0.0/8",
			"tpu_num_slices":  1,
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "tpu-v6e-pool",
				MachineType: "ct6e-standard-4t",
				Settings: map[string]any{
					"tpu_num_slices": "$(vars.tpu_num_slices)",
				},
			},
		},
	}

	groups, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if cfg.Vars["tpu_num_slices"] != 1 {
		t.Errorf("cfg.Vars[tpu_num_slices] = %v, want literal 1 (must not create var-to-var reference)", cfg.Vars["tpu_num_slices"])
	}
	var poolNumSlices any
	for _, g := range groups {
		for _, m := range g.Modules {
			if m.ID == "tpu-v6e-pool_pool" {
				poolNumSlices = m.Settings["num_slices"]
			}
		}
	}
	if poolNumSlices != "$(vars.tpu_num_slices)" {
		t.Errorf("tpu-v6e-pool_pool num_slices = %v, want $(vars.tpu_num_slices)", poolNumSlices)
	}
}

// TestMixedAcceleratorArchetypesRejected: one accelerator archetype per cluster in
// Phase 1. Two different ones (GPU+GPU, GPU+TPU) share cluster-level modules and vars
// that silently overwrite each other, so the combination must hard-error. The same
// accelerator archetype backing several pools, plus CPU pools, stays legal.
func TestMixedAcceleratorArchetypesRejected(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)
	vars := func() map[string]any {
		return map[string]any{
			"project_id": "test-project", "deployment_name": "test-dep",
			"region": "us-central1", "zone": "us-central1-a", "authorized_cidr": "10.0.0.0/8",
		}
	}

	for name, pools := range map[string][]ast.ArchetypeReference{
		"gpu-gpu": {{Name: "h200", MachineType: "a3-ultragpu-8g"}, {Name: "b200", MachineType: "a4-highgpu-8g"}},
		"gpu-tpu": {{Name: "b200", MachineType: "a4-highgpu-8g"}, {Name: "v6e", MachineType: "ct6e-standard-4t"}},
	} {
		_, err := compiler.Compile(ast.ClusterConfig{ConfigBase: ast.ConfigBaseReference{Type: "gke"}, Vars: vars(), ComputeArchetypes: pools})
		if err == nil || !strings.Contains(err.Error(), "combining different accelerator archetypes") {
			t.Errorf("%s: expected accelerator-mix error, got: %v", name, err)
		}
	}

	// Same accelerator archetype on two pools (+ CPU pool) is allowed.
	_, err := compiler.Compile(ast.ClusterConfig{ConfigBase: ast.ConfigBaseReference{Type: "gke"}, Vars: vars(), ComputeArchetypes: []ast.ArchetypeReference{
		{Name: "v6e-a", MachineType: "ct6e-standard-4t"}, {Name: "v6e-b", MachineType: "ct6e-standard-4t"},
	}})
	if err != nil && strings.Contains(err.Error(), "combining different accelerator archetypes") {
		t.Errorf("same archetype on two pools must be allowed, got: %v", err)
	}
}
