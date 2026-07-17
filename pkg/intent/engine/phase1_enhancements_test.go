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
	"strings"
	"testing"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/intent/engine"
)

func findModByID(groups []ast.DeploymentGroup, id string) *ast.ModuleSpec {
	for gi := range groups {
		for mi := range groups[gi].Modules {
			if groups[gi].Modules[mi].ID == id {
				return &groups[gi].Modules[mi]
			}
		}
	}
	return nil
}

func TestInlineConfigBaseSettings(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	rawYAML := []byte(`
config_base:
  type: slurm
  settings:
    enable_login_public_ips: false
    enable_cleanup_compute: true
vars:
  project_id: test-proj
  deployment_name: test-cluster
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: cpu-pool
    machine_type: n2-standard-4
`)
	cfg, err := ast.UnmarshalClusterConfigStrict(rawYAML)
	if err != nil {
		t.Fatalf("failed to parse inline config_base mapping: %v", err)
	}
	if cfg.ConfigBase.Type != "slurm" {
		t.Fatalf("expected ConfigBase.Type='slurm', got %q", cfg.ConfigBase.Type)
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	login := findModByID(groups, "slurm_login")
	if login == nil {
		t.Fatal("slurm_login module not found")
	}
	if login.Settings["enable_login_public_ips"] != false {
		t.Errorf("expected slurm_login.enable_login_public_ips=false, got %v", login.Settings["enable_login_public_ips"])
	}

	ctrl := findModByID(groups, "slurm_controller")
	if ctrl == nil {
		t.Fatal("slurm_controller module not found")
	}
	if ctrl.Settings["enable_cleanup_compute"] != true {
		t.Errorf("expected slurm_controller.enable_cleanup_compute=true, got %v", ctrl.Settings["enable_cleanup_compute"])
	}
}

func TestSingletonProtectionFromArchetypeSettings(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "test-cluster",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "train-pool",
				MachineType: "a3-ultragpu-8g",
				Settings: map[string]any{
					"node_count_static": 8,
					"enable_public_ips": false,
				},
			},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	nodeset := findModByID(groups, "train-pool_nodeset")
	if nodeset == nil {
		t.Fatal("train-pool_nodeset not found")
	}
	if nodeset.Settings["enable_public_ips"] != false {
		t.Errorf("expected train-pool_nodeset.enable_public_ips=false, got %v", nodeset.Settings["enable_public_ips"])
	}

	ctrl := findModByID(groups, "slurm_controller")
	if ctrl == nil {
		t.Fatal("slurm_controller not found")
	}
	if _, leaked := ctrl.Settings["enable_public_ips"]; leaked {
		t.Errorf("per-pool setting enable_public_ips leaked into singleton slurm_controller: %v", ctrl.Settings)
	}
}

func TestMultiPoolControllerStartupRunnersAppend(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "test-cluster",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "train-pool",
				MachineType: "a4x-highgpu-4g",
			},
			{
				Name:        "eval-pool",
				MachineType: "a4x-highgpu-4g",
			},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	ctrlStartup := findModByID(groups, "controller_startup")
	if ctrlStartup == nil {
		t.Fatal("controller_startup module not found")
	}

	runnersRaw := fmt.Sprintf("%v", ctrlStartup.Settings["runners"])
	if !strings.Contains(runnersRaw, `PARTITION_NAME=$(train-pool_partition.partitions[0].partition_name)`) {
		t.Errorf("controller_startup.runners missing train-pool stage_scripts.sh:\n%s", runnersRaw)
	}
	if !strings.Contains(runnersRaw, `PARTITION_NAME=$(eval-pool_partition.partitions[0].partition_name)`) {
		t.Errorf("controller_startup.runners missing eval-pool stage_scripts.sh:\n%s", runnersRaw)
	}
	if !strings.Contains(runnersRaw, `stage_scripts_2.sh`) {
		t.Errorf("expected second stage_scripts runner destination to be disambiguated as stage_scripts_2.sh:\n%s", runnersRaw)
	}
}

func TestMultiPoolScopedVarsIsolation(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "test-cluster",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "train-pool",
				MachineType: "a3-ultragpu-8g",
				Settings: map[string]any{
					"a3u_reservation_name": "res-train-16",
					"disk_size_gb":         500,
				},
			},
			{
				Name:        "eval-pool",
				MachineType: "a3-ultragpu-8g",
				Settings: map[string]any{
					"a3u_reservation_name": "res-eval-4",
					"disk_size_gb":         200,
				},
			},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	if cfg.Vars["train_pool_a3u_reservation_name"] != "res-train-16" {
		t.Errorf("expected Vars[train_pool_a3u_reservation_name]='res-train-16', got %v", cfg.Vars["train_pool_a3u_reservation_name"])
	}
	if cfg.Vars["eval_pool_a3u_reservation_name"] != "res-eval-4" {
		t.Errorf("expected Vars[eval_pool_a3u_reservation_name]='res-eval-4', got %v", cfg.Vars["eval_pool_a3u_reservation_name"])
	}

	trainMod := findModByID(groups, "train-pool_nodeset")
	evalMod := findModByID(groups, "eval-pool_nodeset")
	if trainMod == nil || evalMod == nil {
		t.Fatal("expected both train-pool_nodeset and eval-pool_nodeset")
	}
	if trainMod.Settings["reservation_name"] != "$(vars.train_pool_a3u_reservation_name)" {
		t.Errorf("expected train-pool reservation_name='$(vars.train_pool_a3u_reservation_name)', got %v", trainMod.Settings["reservation_name"])
	}
	if evalMod.Settings["reservation_name"] != "$(vars.eval_pool_a3u_reservation_name)" {
		t.Errorf("expected eval-pool reservation_name='$(vars.eval_pool_a3u_reservation_name)', got %v", evalMod.Settings["reservation_name"])
	}
}

func TestProducerAttachToScopingInAutowire(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "test-cluster",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "train-pool",
				MachineType: "n2-standard-4",
			},
			{
				Name:        "eval-pool",
				MachineType: "n2-standard-4",
			},
		},
		Features: []ast.FeatureReference{
			{
				Name:     "scratchfs",
				Type:     "filestore",
				AttachTo: []string{"train-pool"},
				Settings: map[string]any{
					"local_mount": "/scratch",
				},
			},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	trainMod := findModByID(groups, "train-pool_nodeset")
	evalMod := findModByID(groups, "eval-pool_nodeset")
	if trainMod == nil || evalMod == nil {
		t.Fatal("expected both train-pool_nodeset and eval-pool_nodeset")
	}

	trainUsesScratch := false
	for _, u := range trainMod.Use {
		if u == "train-pool_scratchfs" || u == "scratchfs" {
			trainUsesScratch = true
		}
	}
	if !trainUsesScratch {
		t.Errorf("expected train-pool_nodeset to use train-pool_scratchfs, got use=%v", trainMod.Use)
	}

	for _, u := range evalMod.Use {
		if u == "train-pool_scratchfs" || u == "scratchfs" {
			t.Errorf("eval-pool_nodeset must NOT use scratchfs when attach_to=[train-pool], got use=%v", evalMod.Use)
		}
	}
}

func TestMultiPoolSubsetAttachToScoping(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "my-project",
			"deployment_name": "test-cluster",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "pool-a", MachineType: "n2-standard-4"},
			{Name: "pool-b", MachineType: "n2-standard-4"},
			{Name: "pool-c", MachineType: "n2-standard-4"},
		},
		Features: []ast.FeatureReference{
			{
				Name:     "sharedfs",
				Type:     "filestore",
				AttachTo: []string{"pool-a", "pool-b"},
				Settings: map[string]any{"local_mount": "/shared"},
			},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	poolA := findModByID(groups, "pool-a_nodeset")
	poolB := findModByID(groups, "pool-b_nodeset")
	poolC := findModByID(groups, "pool-c_nodeset")
	if poolA == nil || poolB == nil || poolC == nil {
		t.Fatal("missing expected nodeset modules")
	}

	hasShared := func(use []string) bool {
		for _, u := range use {
			if strings.Contains(u, "sharedfs") {
				return true
			}
		}
		return false
	}
	if !hasShared(poolA.Use) {
		t.Errorf("expected pool-a_nodeset to use sharedfs, got %v", poolA.Use)
	}
	if !hasShared(poolB.Use) {
		t.Errorf("expected pool-b_nodeset to use sharedfs, got %v", poolB.Use)
	}
	if hasShared(poolC.Use) {
		t.Errorf("expected pool-c_nodeset NOT to use sharedfs, got %v", poolC.Use)
	}
}

func TestSettingsAutoRoutingOnArchetypeAndFeature(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "my-project",
			"deployment_name": "test-cluster",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "train-pool",
				MachineType: "a3-ultragpu-8g",
				Settings: map[string]any{
					"exclusive": false,
				},
			},
		},
		Features: []ast.FeatureReference{
			{
				Name: "homefs",
				Type: "filestore",
				Settings: map[string]any{
					"filestore_tier": "BASIC_HDD",
				},
			},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	part := findModByID(groups, "train-pool_partition")
	if part == nil {
		t.Fatal("train-pool_partition not found")
	}
	if part.Settings["exclusive"] != false {
		t.Errorf("expected train-pool_partition.exclusive=false via settings auto-routing, got %v", part.Settings["exclusive"])
	}

	fs := findModByID(groups, "homefs")
	if fs == nil {
		t.Fatal("homefs not found")
	}
	if fs.Settings["filestore_tier"] != "BASIC_HDD" {
		t.Errorf("expected homefs.filestore_tier=BASIC_HDD via settings auto-routing, got %v", fs.Settings["filestore_tier"])
	}
}

func TestPackerKindPreservationInSerializer(t *testing.T) {
	groups := []ast.DeploymentGroup{
		{
			Group: "custom-image",
			Modules: []ast.ModuleSpec{
				{
					ID:     "slurm-image-packer",
					Source: "modules/packer/custom-image",
					Kind:   "packer",
					Settings: map[string]any{
						"source_image_family": "slurm-gcp-6-12-debian-12",
					},
				},
			},
		},
	}

	out, err := engine.Serialize("slurm", map[string]any{"project_id": "test"}, groups)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	if !strings.Contains(out, "kind: packer") {
		t.Errorf("expected serialized YAML to preserve 'kind: packer', got:\n%s", out)
	}
}

func TestInlineConfigBaseOnGKEAndJBVM(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	rawGKE := []byte(`
config_base:
  type: gke
  settings:
    enable_private_endpoint: false
vars:
  project_id: test-proj
  deployment_name: test-gke
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: cpu-pool
    machine_type: n2-standard-8
`)
	cfgGKE, err := ast.UnmarshalClusterConfigStrict(rawGKE)
	if err != nil {
		t.Fatalf("unmarshal GKE inline config_base failed: %v", err)
	}
	groupsGKE, err := engine.NewCompiler(repoRoot).Compile(cfgGKE)
	if err != nil {
		t.Fatalf("compile GKE inline config_base failed: %v", err)
	}
	clusterMod := findModByID(groupsGKE, "cluster")
	if clusterMod == nil || clusterMod.Settings["enable_private_endpoint"] != false {
		t.Errorf("expected cluster.enable_private_endpoint=false, got %v", clusterMod)
	}
}

func TestRunnerDeduplicationAndThreePoolDisambiguation(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "my-project",
			"deployment_name": "test-cluster",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "pool-1", MachineType: "a4x-highgpu-4g"},
			{Name: "pool-2", MachineType: "a4x-highgpu-4g"},
			{Name: "pool-3", MachineType: "a4x-highgpu-4g"},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	ctrlStartup := findModByID(groups, "controller_startup")
	if ctrlStartup == nil {
		t.Fatal("controller_startup not found")
	}
	runnersRaw := fmt.Sprintf("%v", ctrlStartup.Settings["runners"])
	for _, want := range []string{
		`PARTITION_NAME=$(pool-1_partition.partitions[0].partition_name)`,
		`PARTITION_NAME=$(pool-2_partition.partitions[0].partition_name)`,
		`PARTITION_NAME=$(pool-3_partition.partitions[0].partition_name)`,
		`stage_scripts.sh`,
		`stage_scripts_2.sh`,
		`stage_scripts_3.sh`,
	} {
		if !strings.Contains(runnersRaw, want) {
			t.Errorf("expected controller_startup.runners to contain %s, got:\n%s", want, runnersRaw)
		}
	}
}

func TestInvalidConfigBaseMappingAndStrictUnknownFieldsRejected(t *testing.T) {
	// 1. Missing `type` in config_base mapping must fail fast
	_, err := ast.UnmarshalClusterConfigStrict([]byte(`
config_base:
  settings:
    enable_cleanup_compute: true
`))
	if err == nil || !strings.Contains(err.Error(), "must specify a non-empty 'type' field") {
		t.Errorf("expected error for missing config_base.type, got: %v", err)
	}

	// 2. Sequence config_base (`[slurm]`) must fail fast
	_, err = ast.UnmarshalClusterConfigStrict([]byte(`
config_base:
  - slurm
`))
	if err == nil || !strings.Contains(err.Error(), "must be a string") {
		t.Errorf("expected error for sequence config_base, got: %v", err)
	}

	// 3. Stale `addons:` keyword must be rejected by strict unmarshaler
	_, err = ast.UnmarshalClusterConfigStrict([]byte(`
config_base: gke
addons:
  - name: smi
    type: run-nvidia-smi
`))
	if err == nil {
		t.Errorf("expected strict unmarshaler to reject legacy 'addons:' field")
	}
}

// T0.13: Multiple unscoped `cloud-storage` + `aiml-gcsfuse` features on Slurm
// (including a4x-highgpu-4g with `controller_startup`) and JBVM (with `{name}_startup_script`)
// compile without triggering a scalar collision on `startup-script.gcs_bucket_path` (Law 4c `export_suppress`).
func TestMultipleUnscopedCloudStorageBucketsOnSlurm(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	for _, tc := range []struct {
		name        string
		configBase  string
		machineType string
		startupID   string
	}{
		{"slurm-a4x-controller-startup", "slurm", "a4x-highgpu-4g", "controller_startup"},
		{"jbvm-vm-startup-script", "jbvm", "a3-ultragpu-8g", "test-pool_startup_script"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawYAML := []byte(fmt.Sprintf(`
config_base: %s
vars:
  project_id: my-project
  deployment_name: multi-gcs-test
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: test-pool
    machine_type: %s
features:
  - name: data-bkt
    type: cloud-storage
    settings:
      local_mount: /mnt/data
  - name: ckpt-bkt
    type: cloud-storage
    settings:
      local_mount: /mnt/ckpt
  - name: ml-data
    type: aiml-gcsfuse
`, tc.configBase, tc.machineType))
			cfg, err := ast.UnmarshalClusterConfigStrict(rawYAML)
			if err != nil {
				t.Fatalf("failed to unmarshal config: %v", err)
			}

			groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
			if err != nil {
				t.Fatalf("Compile failed for multiple unscoped cloud-storage + aiml-gcsfuse on %s: %v", tc.name, err)
			}
			startupMod := findModByID(groups, tc.startupID)
			if startupMod == nil {
				t.Fatalf("expected %s module in blueprint", tc.startupID)
			}
			startupUse := strings.Join(startupMod.Use, ",")
			for _, bkt := range []string{"data-bkt_bucket", "ckpt-bkt_bucket", "ml-data_bucket"} {
				if strings.Contains(startupUse, bkt) {
					t.Errorf("expected %s.Use NOT to include user storage bucket %s, got: %v", tc.startupID, bkt, startupMod.Use)
				}
			}
		})
	}
}

// T0.14 (Diff 16): Overriding an archetype/feature `vars:` handle inside `settings:`
// promotes the scoped variable (`<pool>_<var>`) and prunes the un-prefixed fallback variable
// from `userConfig.Vars` when no module references the un-prefixed variable.
func TestScopedVarsDoNotLeaveUnreferencedFallbackVarsInUserConfig(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	rawYAML := []byte(`
config_base: jbvm
vars:
  project_id: my-project
  deployment_name: test-jbvm-vars
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: gpu-vm
    machine_type: a3-ultragpu-8g
    settings:
      a3u_reservation_name: my-custom-res
`)
	cfg, err := ast.UnmarshalClusterConfigStrict(rawYAML)
	if err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if _, err := engine.NewCompiler(repoRoot).Compile(cfg); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if got := cfg.Vars["gpu_vm_a3u_reservation_name"]; got != "my-custom-res" {
		t.Errorf("expected scoped var gpu_vm_a3u_reservation_name='my-custom-res', got %v", got)
	}
	if _, leaked := cfg.Vars["a3u_reservation_name"]; leaked {
		t.Errorf("expected un-prefixed fallback var a3u_reservation_name to be pruned from cfg.Vars, but it remained: %v", cfg.Vars)
	}
}

// T0.15: Verify submodule disambiguation inside the single `settings:` map across:
//
//	a. `config_base.settings`: flat `machine_type` routes ONLY to `slurm_controller` (preserving `slurm_login` default `n2-standard-4`),
//	   `enable_login_public_ips` routes to `slurm_login`, and `slurm_login: { machine_type: n2-standard-8 }` overrides `slurm_login.machine_type`.
//	b. `aiml-gcsfuse` feature `settings: { local_mount: /gcs, checkpoints: { local_mount: /custom-ckpt } }` sets `/gcs` on `ml-data_bucket`
//	   and `/custom-ckpt` on `ml-data_checkpoints` while preserving `/gcs-training-data` and `/gcs-model-serving`.
//	c. Strict unmarshaler rejects `module_settings:` on `compute_archetypes` and `features` as an unknown field.
func TestSubmoduleDisambiguationInsideSingleSettingsMap(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	testSubmoduleDisambiguationBasePrimary(t, repoRoot)
	testSubmoduleDisambiguationSubmodMap(t, repoRoot)
	testSubmoduleDisambiguationRejectsModuleSettings(t)
}

func testSubmoduleDisambiguationBasePrimary(t *testing.T, repoRoot string) {
	t.Helper()
	// (a) Flat config_base.settings routes machine_type ONLY to slurm_controller
	rawBasePrimary := []byte(`
config_base:
  type: slurm
  settings:
    machine_type: n2-standard-64
    enable_login_public_ips: false
vars:
  project_id: test-proj
  deployment_name: test-base-primary
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: cpu-pool
    machine_type: n2-standard-4
`)
	cfg1, err := ast.UnmarshalClusterConfigStrict(rawBasePrimary)
	if err != nil {
		t.Fatalf("failed to unmarshal rawBasePrimary: %v", err)
	}
	groups1, err := engine.NewCompiler(repoRoot).Compile(cfg1)
	if err != nil {
		t.Fatalf("compile rawBasePrimary failed: %v", err)
	}
	ctrl1 := findModByID(groups1, "slurm_controller")
	login1 := findModByID(groups1, "slurm_login")
	if ctrl1 == nil || login1 == nil {
		t.Fatal("expected slurm_controller and slurm_login modules")
	}
	if ctrl1.Settings["machine_type"] != "n2-standard-64" {
		t.Errorf("expected slurm_controller.machine_type='n2-standard-64', got %v", ctrl1.Settings["machine_type"])
	}
	if login1.Settings["machine_type"] != "n2-standard-8" {
		t.Errorf("expected slurm_login.machine_type to remain default 'n2-standard-8', got %v", login1.Settings["machine_type"])
	}
	if login1.Settings["enable_login_public_ips"] != false {
		t.Errorf("expected slurm_login.enable_login_public_ips=false, got %v", login1.Settings["enable_login_public_ips"])
	}
}

func testSubmoduleDisambiguationSubmodMap(t *testing.T, repoRoot string) {
	t.Helper()
	// (a.2 + b) Explicit submodule sub-map inside settings (`slurm_login:` and `checkpoints:`)
	rawSubmodMap := []byte(`
config_base:
  type: slurm
  settings:
    machine_type: n2-standard-64
    slurm_login:
      machine_type: n2-standard-16
vars:
  project_id: test-proj
  deployment_name: test-submod
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: train-pool
    machine_type: a3-ultragpu-8g
    settings:
      exclusive: false
features:
  - name: ml-data
    type: aiml-gcsfuse
    settings:
      local_mount: /gcs
      checkpoints:
        local_mount: /custom-ckpt
`)
	cfg2, err := ast.UnmarshalClusterConfigStrict(rawSubmodMap)
	if err != nil {
		t.Fatalf("failed to unmarshal rawSubmodMap: %v", err)
	}
	groups2, err := engine.NewCompiler(repoRoot).Compile(cfg2)
	if err != nil {
		t.Fatalf("compile rawSubmodMap failed: %v", err)
	}
	login2 := findModByID(groups2, "slurm_login")
	if login2 == nil || login2.Settings["machine_type"] != "n2-standard-16" {
		t.Errorf("expected slurm_login.machine_type='n2-standard-16', got %v", login2)
	}
	part2 := findModByID(groups2, "train-pool_partition")
	if part2 == nil || part2.Settings["exclusive"] != false {
		t.Errorf("expected train-pool_partition.exclusive=false, got %v", part2)
	}
	assertDisambiguatedGCSFuseBuckets(t, groups2)
}

func assertDisambiguatedGCSFuseBuckets(t *testing.T, groups2 []ast.DeploymentGroup) {
	t.Helper()
	bkt := findModByID(groups2, "ml-data_bucket")
	ckpt := findModByID(groups2, "ml-data_checkpoints")
	td := findModByID(groups2, "ml-data_training_data")
	ms := findModByID(groups2, "ml-data_model_serving")
	if bkt == nil || ckpt == nil || td == nil || ms == nil {
		t.Fatal("expected all 4 aiml-gcsfuse bucket modules")
	}
	if bkt.Settings["local_mount"] != "/gcs" {
		t.Errorf("expected ml-data_bucket.local_mount='/gcs', got %v", bkt.Settings["local_mount"])
	}
	if ckpt.Settings["local_mount"] != "/custom-ckpt" {
		t.Errorf("expected ml-data_checkpoints.local_mount='/custom-ckpt', got %v", ckpt.Settings["local_mount"])
	}
	if td.Settings["local_mount"] != "/gcs-training-data" {
		t.Errorf("expected ml-data_training_data.local_mount='/gcs-training-data', got %v", td.Settings["local_mount"])
	}
	if ms.Settings["local_mount"] != "/gcs-model-serving" {
		t.Errorf("expected ml-data_model_serving.local_mount='/gcs-model-serving', got %v", ms.Settings["local_mount"])
	}
}

func testSubmoduleDisambiguationRejectsModuleSettings(t *testing.T) {
	t.Helper()
	// (c) Strict unmarshaler rejects `module_settings:` on compute_archetypes and features
	for _, badYAML := range []string{
		`
config_base: slurm
compute_archetypes:
  - name: pool
    machine_type: n2-standard-4
    module_settings:
      pool_partition:
        exclusive: false
`,
		`
config_base: slurm
compute_archetypes:
  - name: pool
    machine_type: n2-standard-4
features:
  - name: homefs
    type: filestore
    module_settings:
      homefs:
        filestore_tier: BASIC_HDD
`,
	} {
		if _, err := ast.UnmarshalClusterConfigStrict([]byte(badYAML)); err == nil || !strings.Contains(err.Error(), "field module_settings not found") {
			t.Errorf("expected strict unmarshaler to reject module_settings with 'field module_settings not found', got: %v", err)
		}
	}
}

// T0.16: Verify unified `name:` + `BlueprintBlock` on FeatureManifest and strictly additive
// Phase 1 `OverlayManifest` (adding pools, adding features, adding new/nil vars, and rejecting
// non-additive overrides of config_base.settings, existing vars, existing pools, or existing features,
// as well as inline overlays[].settings in cluster-config.yaml).
func TestUnifiedManifestSchemasAndCompositeOverlay(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	overlayPath := filepath.Join(repoRoot, "v2", "overlays", "hft-production-profile.yaml")
	overlayYAML := []byte(`
name: hft-production-profile
description: "Strictly additive composite overlay: adds a new debug-cpu-pool, adds monitoring, and sets default vars."
config_base:
  type: slurm
vars:
  custom_profile_tag: hft-v1
compute_archetypes:
  - name: debug-cpu-pool
    machine_type: n2-standard-8
    settings:
      node_count_dynamic_max: 2
      instance_properties:
        profile: $(vars.custom_profile_tag)
features:
  - name: prod-monitoring
    type: monitoring-dashboard
`)
	if err := os.WriteFile(overlayPath, overlayYAML, 0644); err != nil {
		t.Fatalf("failed to write test overlay file: %v", err)
	}
	defer os.Remove(overlayPath)

	rawClusterConfig := []byte(`
config_base: slurm
vars:
  project_id: test-proj
  deployment_name: hft-slurm
  region: us-central1
  zone: us-central1-a
  custom_profile_tag:
compute_archetypes:
  - name: train-pool
    machine_type: n2-standard-4
    settings:
      node_count_static: 4
overlays:
  - name: hft-production-profile
`)
	cfg, err := ast.UnmarshalClusterConfigStrict(rawClusterConfig)
	if err != nil {
		t.Fatalf("failed to unmarshal cluster config: %v", err)
	}

	compiler := engine.NewCompiler(repoRoot)
	groups, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatalf("compile with strictly additive composite overlay failed: %v", err)
	}
	assertCompositeOverlayCompiled(t, cfg, groups)
	assertCompositeOverlayNegatives(t, repoRoot, compiler, cfg)
}

func assertCompositeOverlayCompiled(t *testing.T, cfg ast.ClusterConfig, groups []ast.DeploymentGroup) {
	t.Helper()
	if cfg.Vars["custom_profile_tag"] != "hft-v1" {
		t.Errorf("expected nil var custom_profile_tag to be hydrated by overlay to 'hft-v1', got %v", cfg.Vars["custom_profile_tag"])
	}

	trainNodeset := findModByID(groups, "train-pool_nodeset")
	if trainNodeset == nil || trainNodeset.Settings["node_count_static"] != 4 {
		t.Errorf("expected train-pool_nodeset.node_count_static=4 preserved from cluster-config, got %v", trainNodeset)
	}

	debugNodeset := findModByID(groups, "debug-cpu-pool_nodeset")
	if debugNodeset == nil {
		t.Fatal("expected new debug-cpu-pool_nodeset added by composite overlay")
	}
	if debugNodeset.Settings["node_count_dynamic_max"] != 2 {
		t.Errorf("expected debug-cpu-pool_nodeset.node_count_dynamic_max=2, got %v", debugNodeset.Settings["node_count_dynamic_max"])
	}

	mon := findModByID(groups, "prod-monitoring")
	if mon == nil {
		t.Fatal("expected prod-monitoring feature module added by composite overlay")
	}
}

func assertCompositeOverlayNegatives(t *testing.T, repoRoot string, compiler *engine.Compiler, cfg ast.ClusterConfig) {
	t.Helper()
	// Negative 1: inline overlays[].settings in cluster-config.yaml is rejected
	cfgInlineSettings := cfg
	cfgInlineSettings.Overlays = []ast.FeatureReference{{
		Name:     "hft-production-profile",
		Settings: map[string]any{"job_nodes": 4},
	}}
	if _, err := compiler.Compile(cfgInlineSettings); err == nil || !strings.Contains(err.Error(), "does not accept inline settings in cluster config") {
		t.Errorf("expected inline overlays[].settings to be rejected, got: %v", err)
	}

	// Negative 2: overlay overriding a non-nil var in cluster-config.yaml is rejected
	cfgVarConflict := cfg
	cfgVarConflict.Vars = map[string]any{
		"project_id":         "test-proj",
		"deployment_name":    "hft-slurm",
		"region":             "us-central1",
		"zone":               "us-central1-a",
		"custom_profile_tag": "user-v2",
	}
	if _, err := compiler.Compile(cfgVarConflict); err == nil || !strings.Contains(err.Error(), "cannot override existing var \"custom_profile_tag\"") {
		t.Errorf("expected overlay var override to be rejected, got: %v", err)
	}

	// Negative 3: overlay setting config_base.settings is rejected
	badBasePath := filepath.Join(repoRoot, "v2", "overlays", "bad-base-overlay.yaml")
	_ = os.WriteFile(badBasePath, []byte(`
name: bad-base-overlay
config_base:
  type: slurm
  settings:
    compute_startup_scripts_timeout: 3600
`), 0644)
	defer os.Remove(badBasePath)
	cfgBadBase := cfg
	cfgBadBase.Vars = map[string]any{"project_id": "p", "deployment_name": "d", "region": "r", "zone": "z"}
	cfgBadBase.Overlays = []ast.FeatureReference{{Name: "bad-base-overlay"}}
	if _, err := engine.NewCompiler(repoRoot).Compile(cfgBadBase); err == nil || !strings.Contains(err.Error(), "cannot modify config_base settings") {
		t.Errorf("expected overlay config_base.settings to be rejected, got: %v", err)
	}

	// Negative 4: overlay patching an existing compute_archetypes pool is rejected
	badPoolPath := filepath.Join(repoRoot, "v2", "overlays", "bad-pool-overlay.yaml")
	_ = os.WriteFile(badPoolPath, []byte(`
name: bad-pool-overlay
compute_archetypes:
  - name: train-pool
    settings:
      exclusive: true
`), 0644)
	defer os.Remove(badPoolPath)
	cfgBadPool := cfgBadBase
	cfgBadPool.Overlays = []ast.FeatureReference{{Name: "bad-pool-overlay"}}
	if _, err := engine.NewCompiler(repoRoot).Compile(cfgBadPool); err == nil || !strings.Contains(err.Error(), "conflicts with existing compute archetype \"train-pool\"") {
		t.Errorf("expected overlay modifying existing pool to be rejected, got: %v", err)
	}
}

func TestAutomaticStartupScriptMergingAndRoleTargeting(t *testing.T) {
	repoRoot := withRealModuleFS(t)

	cfg := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "startup-merge-test",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "gpu-pool",
				MachineType: "a3-ultragpu-8g",
			},
			{
				Name:        "cpu-pool",
				MachineType: "n2-standard-4",
			},
		},
		Features: []ast.FeatureReference{
			{
				Name: "all-compute-init",
				Type: "startup-script",
				Settings: map[string]any{
					"runners": []any{
						map[string]any{
							"type":        "shell",
							"destination": "all_compute.sh",
							"content":     "#!/bin/bash\necho all-compute\n",
						},
					},
				},
			},
			{
				Name:     "cpu-extra-init",
				Type:     "startup-script",
				AttachTo: []string{"cpu-pool"},
				Settings: map[string]any{
					"runners": []any{
						map[string]any{
							"type":        "shell",
							"destination": "cpu_only.sh",
							"content":     "#!/bin/bash\necho cpu-only\n",
						},
					},
				},
			},
			{
				Name:     "ctrl-init",
				Type:     "startup-script",
				AttachTo: []string{"controller"},
				Settings: map[string]any{
					"runners": []any{
						map[string]any{
							"type":        "shell",
							"destination": "ctrl_only.sh",
							"content":     "#!/bin/bash\necho ctrl-only\n",
						},
					},
				},
			},
			{
				Name:     "login-init",
				Type:     "startup-script",
				AttachTo: []string{"login"},
				Settings: map[string]any{
					"runners": []any{
						map[string]any{
							"type":        "shell",
							"destination": "login_only.sh",
							"content":     "#!/bin/bash\necho login-only\n",
						},
					},
				},
			},
			{
				Name:     "login-fs",
				Type:     "filestore",
				AttachTo: []string{"login"},
				Settings: map[string]any{
					"local_mount": "/login_scratch",
				},
			},
		},
	}

	groups, err := engine.NewCompiler(repoRoot).Compile(cfg)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	assertStartupMergingComputePools(t, groups)
	assertStartupMergingControllerAndLogin(t, groups)
}

func assertStartupMergingComputePools(t *testing.T, groups []ast.DeploymentGroup) {
	t.Helper()
	// 1. gpu-pool_a3u_startup must contain the archetype's runner AND all_compute.sh (but NOT cpu_only.sh)
	gpuStartup := findModByID(groups, "gpu-pool_a3u_startup")
	if gpuStartup == nil {
		t.Fatal("expected gpu-pool_a3u_startup module")
	}
	gpuStartupStr := fmt.Sprintf("%#v", gpuStartup.Settings["runners"])
	if !strings.Contains(gpuStartupStr, "all_compute.sh") {
		t.Errorf("expected gpu-pool_a3u_startup to contain all_compute.sh, got: %s", gpuStartupStr)
	}
	if strings.Contains(gpuStartupStr, "cpu_only.sh") {
		t.Errorf("gpu-pool_a3u_startup must NOT contain cpu_only.sh, got: %s", gpuStartupStr)
	}

	// 2. cpu-pool_startup must be created with both all_compute.sh AND cpu_only.sh, and wired to cpu-pool_nodeset
	cpuStartup := findModByID(groups, "cpu-pool_startup")
	if cpuStartup == nil {
		t.Fatal("expected cpu-pool_startup module to be created for generic-cpu pool")
	}
	cpuStartupStr := fmt.Sprintf("%#v", cpuStartup.Settings["runners"])
	if !strings.Contains(cpuStartupStr, "all_compute.sh") || !strings.Contains(cpuStartupStr, "cpu_only.sh") {
		t.Errorf("expected cpu-pool_startup to contain both all_compute.sh and cpu_only.sh, got: %s", cpuStartupStr)
	}
	cpuNodeset := findModByID(groups, "cpu-pool_nodeset")
	if cpuNodeset == nil || !containsStr(cpuNodeset.Use, "cpu-pool_startup") {
		t.Errorf("expected cpu-pool_nodeset to use cpu-pool_startup, got use=%v", cpuNodeset.Use)
	}
}

func assertStartupMergingControllerAndLogin(t *testing.T, groups []ast.DeploymentGroup) {
	t.Helper()
	// 3. controller_startup must contain ctrl_only.sh
	ctrlStartup := findModByID(groups, "controller_startup")
	if ctrlStartup == nil || !strings.Contains(fmt.Sprintf("%#v", ctrlStartup.Settings["runners"]), "ctrl_only.sh") {
		t.Errorf("expected controller_startup to contain ctrl_only.sh, got: %#v", ctrlStartup)
	}

	// 4. login_startup must be created with login_only.sh and wired to slurm_login
	loginStartup := findModByID(groups, "login_startup")
	if loginStartup == nil || !strings.Contains(fmt.Sprintf("%#v", loginStartup.Settings["runners"]), "login_only.sh") {
		t.Errorf("expected login_startup to contain login_only.sh, got: %#v", loginStartup)
	}
	slurmLogin := findModByID(groups, "slurm_login")
	if slurmLogin == nil || !containsStr(slurmLogin.Use, "login_startup") {
		t.Errorf("expected slurm_login to use login_startup, got use=%v", slurmLogin.Use)
	}

	// 5. login-fs (`attach_to: [login]`) must be wired into slurm_controller.Settings["login_network_storage"]
	// and NOT wired into gpu-pool_nodeset or cpu-pool_nodeset
	slurmCtrl := findModByID(groups, "slurm_controller")
	if slurmCtrl == nil || !strings.Contains(fmt.Sprintf("%#v", slurmCtrl.Settings["login_network_storage"]), "$(login-fs.network_storage)") {
		t.Errorf("expected slurm_controller.login_network_storage to contain $(login-fs.network_storage), got: %#v", slurmCtrl.Settings["login_network_storage"])
	}
	cpuNodeset := findModByID(groups, "cpu-pool_nodeset")
	if containsStr(gpuNodesetUse(groups, "gpu-pool_nodeset"), "login-fs") || (cpuNodeset != nil && containsStr(cpuNodeset.Use, "login-fs")) {
		t.Errorf("login-fs must not be wired to compute nodesets when attach_to=[login]")
	}
}

func gpuNodesetUse(groups []ast.DeploymentGroup, id string) []string {
	m := findModByID(groups, id)
	if m == nil {
		return nil
	}
	return m.Use
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func TestBugBashFixes(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	testBugBashSlurmFixes(t, repoRoot)
	testBugBashGKEFixes(t, repoRoot)
	testBugBashJBVMFixes(t, repoRoot)
}

func testBugBashSlurmFixes(t *testing.T, repoRoot string) {
	t.Helper()
	// 1. Multi-pool Slurm with conflicting archetype default vars (generic-cpu disk_size_gb=100 vs a4-highgpu-8g disk_size_gb=200),
	// blank top-level var (enable_login_public_ips: ), controller-only startup-script without pre-existing controller_startup,
	// disjoint pool mounts on /shared, {name} template hydration in settings, and single default partition.
	rawSlurm := []byte(`
config_base: slurm
vars:
  project_id: test-proj
  deployment_name: bugbash-slurm
  region: us-central1
  zone: us-central1-a
  a4_dws_flex_enabled:
compute_archetypes:
  - name: cpu-pool
    machine_type: n2-standard-4
  - name: a4h-pool
    machine_type: a4-highgpu-8g
    settings:
      instance_properties:
        pool_tag: "{name}-{machine_type}"
features:
  - name: ctrl-init
    type: startup-script
    attach_to: [controller]
    settings:
      runners:
        - type: shell
          destination: ctrl.sh
          content: "#!/bin/bash\necho ctrl\n"
  - name: fs-a
    type: filestore
    attach_to: [cpu-pool]
    settings:
      local_mount: /shared
  - name: fs-b
    type: filestore
    attach_to: [a4h-pool]
    settings:
      local_mount: /shared
`)
	cfgSlurm, err := ast.UnmarshalClusterConfigStrict(rawSlurm)
	if err != nil {
		t.Fatalf("unmarshal rawSlurm failed: %v", err)
	}
	groupsSlurm, err := engine.NewCompiler(repoRoot).Compile(cfgSlurm)
	if err != nil {
		t.Fatalf("compile rawSlurm failed: %v", err)
	}

	if cfgSlurm.Vars["a4_dws_flex_enabled"] != false {
		t.Errorf("expected blank a4_dws_flex_enabled to preserve default false, got %v", cfgSlurm.Vars["a4_dws_flex_enabled"])
	}
	if cfgSlurm.Vars["cpu_pool_disk_size_gb"] != 100 || cfgSlurm.Vars["a4h_pool_disk_size_gb"] != 300 {
		t.Errorf("expected cpu_pool_disk_size_gb=100 and a4h_pool_disk_size_gb=300, got cpu=%v a4h=%v", cfgSlurm.Vars["cpu_pool_disk_size_gb"], cfgSlurm.Vars["a4h_pool_disk_size_gb"])
	}
	assertBugBashSlurmModules(t, cfgSlurm, groupsSlurm)
}

func assertBugBashSlurmModules(t *testing.T, cfgSlurm ast.ClusterConfig, groupsSlurm []ast.DeploymentGroup) {
	t.Helper()
	slurmCtrl := findModByID(groupsSlurm, "slurm_controller")
	slurmLogin := findModByID(groupsSlurm, "slurm_login")
	cpuNodeset := findModByID(groupsSlurm, "cpu-pool_nodeset")
	ctrlStartup := findModByID(groupsSlurm, "controller_startup")
	if slurmCtrl == nil || slurmLogin == nil || cpuNodeset == nil || ctrlStartup == nil {
		t.Fatal("missing slurm modules")
	}
	if !strings.Contains(fmt.Sprintf("%v", ctrlStartup.Settings["runners"]), "ctrl.sh") {
		t.Errorf("expected controller_startup.runners to contain ctrl.sh, got %v", ctrlStartup.Settings["runners"])
	}
	if containsStr(slurmLogin.Use, "controller_startup") || containsStr(cpuNodeset.Use, "controller_startup") {
		t.Errorf("controller_startup leaked to login (%v) or nodeset (%v)", slurmLogin.Use, cpuNodeset.Use)
	}
	if cpuNodeset.Settings["enable_placement"] != false || cpuNodeset.Settings["on_host_maintenance"] != "MIGRATE" {
		t.Errorf("expected generic-cpu slurm nodeset to have enable_placement=false and on_host_maintenance=MIGRATE, got %v / %v", cpuNodeset.Settings["enable_placement"], cpuNodeset.Settings["on_host_maintenance"])
	}
	assertBugBashSlurmPartitionsAndSerialize(t, slurmCtrl, cfgSlurm, groupsSlurm)
}

func assertBugBashSlurmPartitionsAndSerialize(t *testing.T, slurmCtrl *ast.ModuleSpec, cfgSlurm ast.ClusterConfig, groupsSlurm []ast.DeploymentGroup) {
	t.Helper()
	for _, u := range slurmCtrl.Use {
		if strings.HasSuffix(u, "_nodeset") {
			t.Errorf("slurm_controller.Use must not directly contain %s (duplicate nodeset wiring): %v", u, slurmCtrl.Use)
		}
	}
	cpuPart := findModByID(groupsSlurm, "cpu-pool_partition")
	a4hPart := findModByID(groupsSlurm, "a4h-pool_partition")
	if cpuPart == nil || a4hPart == nil {
		t.Fatal("missing partition modules")
	}
	if cpuPart.Settings["is_default"] != true || a4hPart.Settings["is_default"] != false {
		t.Errorf("expected cpu_partition.is_default=true and a4h_partition.is_default=false, got %v and %v", cpuPart.Settings["is_default"], a4hPart.Settings["is_default"])
	}
	a4hNodeset := findModByID(groupsSlurm, "a4h-pool_nodeset")
	if a4hNodeset == nil || !strings.Contains(fmt.Sprintf("%v", a4hNodeset.Settings["instance_properties"]), "a4h-pool-a4-highgpu-8g") {
		t.Errorf("expected {name}-{machine_type} hydrated in settings, got %v", a4hNodeset)
	}
	serializedSlurm, err := engine.Serialize("slurm", cfgSlurm.Vars, groupsSlurm)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	if !strings.Contains(serializedSlurm, "- instructions") {
		t.Errorf("expected serialized Slurm YAML to contain outputs: [instructions], got:\n%s", serializedSlurm)
	}
}

func testBugBashGKEFixes(t *testing.T, repoRoot string) {
	t.Helper()
	// 2. GKE DWS Flex (#7 / G-01), TPU v6e machine_type override (#6 / C-01), and multiple same-type overlays (#12 / G-05)
	rawGKE := []byte(`
config_base: gke
vars:
  project_id: test-proj
  deployment_name: bugbash-gke
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: tpu-pool-a
    machine_type: tpu-v6e
    settings:
      enable_flex_start: true
  - name: tpu-pool-b
    machine_type: ct6e-standard-8t
    settings:
      enable_flex_start: true
overlays:
  - name: tpu-a-diag
    type: run-nvidia-smi
    attach_to: [tpu-pool-a]
  - name: tpu-b-diag
    type: run-nvidia-smi
    attach_to: [tpu-pool-b]
`)
	cfgGKE, err := ast.UnmarshalClusterConfigStrict(rawGKE)
	if err != nil {
		t.Fatalf("unmarshal rawGKE failed: %v", err)
	}
	groupsGKE, err := engine.NewCompiler(repoRoot).Compile(cfgGKE)
	if err != nil {
		t.Fatalf("compile rawGKE failed: %v", err)
	}
	tpuA := findModByID(groupsGKE, "tpu-pool-a_pool")
	tpuB := findModByID(groupsGKE, "tpu-pool-b_pool")
	if tpuA == nil || tpuB == nil {
		t.Fatal("missing GKE pool modules")
	}
	for _, np := range []*ast.ModuleSpec{tpuA, tpuB} {
		if _, hasStatic := np.Settings["static_node_count"]; hasStatic {
			t.Errorf("expected DWS Flex node pool %s to remove static_node_count, got %v", np.ID, np.Settings)
		}
		if np.Settings["auto_repair"] != false {
			t.Errorf("expected DWS Flex node pool %s to set auto_repair=false, got %v", np.ID, np.Settings["auto_repair"])
		}
	}
	if tpuA.Settings["machine_type"] != "ct6e-standard-4t" {
		t.Errorf("expected tpu-pool-a_pool.machine_type='ct6e-standard-4t', got %v", tpuA.Settings["machine_type"])
	}
	if tpuB.Settings["machine_type"] != "ct6e-standard-8t" {
		t.Errorf("expected tpu-pool-b_pool.machine_type='ct6e-standard-8t', got %v", tpuB.Settings["machine_type"])
	}
}

func testBugBashJBVMFixes(t *testing.T, repoRoot string) {
	t.Helper()
	// 3. Multi-pool JBVM name_prefix disambiguation (#16 / A-08) and conflicting number_of_vms defaults (#14a / V-01)
	rawJBVM := []byte(`
config_base: jbvm
vars:
  project_id: test-proj
  deployment_name: bugbash-jbvm
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: vm-a
    machine_type: a4-highgpu-8g
  - name: vm-b
    machine_type: a4-highgpu-8g
  - name: vm-cpu
    machine_type: n2-standard-4
`)
	cfgJBVM, err := ast.UnmarshalClusterConfigStrict(rawJBVM)
	if err != nil {
		t.Fatalf("unmarshal rawJBVM failed: %v", err)
	}
	groupsJBVM, err := engine.NewCompiler(repoRoot).Compile(cfgJBVM)
	if err != nil {
		t.Fatalf("compile rawJBVM failed: %v", err)
	}
	if cfgJBVM.Vars["vm_cpu_number_of_vms"] != 1 || cfgJBVM.Vars["vm_a_number_of_vms"] != 2 || cfgJBVM.Vars["vm_b_number_of_vms"] != 2 {
		t.Errorf("expected scoped JBVM number_of_vms vars (1, 2, 2), got cpu=%v vm_a=%v vm_b=%v", cfgJBVM.Vars["vm_cpu_number_of_vms"], cfgJBVM.Vars["vm_a_number_of_vms"], cfgJBVM.Vars["vm_b_number_of_vms"])
	}
	vmA := findModByID(groupsJBVM, "vm-a_instance")
	vmB := findModByID(groupsJBVM, "vm-b_instance")
	if vmA == nil || vmB == nil {
		t.Fatal("missing JBVM instance modules")
	}
	if vmA.Settings["name_prefix"] == vmB.Settings["name_prefix"] {
		t.Errorf("expected disambiguated JBVM name_prefix across two a4-highgpu-8g pools, got %v and %v", vmA.Settings["name_prefix"], vmB.Settings["name_prefix"])
	}
}

func TestGKEAndCatalogParityFixes(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	testGKEParityFixes(t, repoRoot)
	testCatalogParityConfigs(t, repoRoot)
}

func testGKEParityFixes(t *testing.T, repoRoot string) {
	t.Helper()
	// 1. GKE-1, GKE-2, GKE-3, GKE-4, GKE-5 on GKE
	rawGKE := []byte(`
config_base: gke
vars:
  project_id: test-proj
  deployment_name: gke-bugfixes
  region: us-central1
  zone: us-central1-a
  tpu_cluster_size: 2
compute_archetypes:
  - name: tpu_pool
    machine_type: tpu-v6e
    settings:
      autoscaling_total_min_nodes: 0
      autoscaling_total_max_nodes: 2
features:
  - name: ml-fuse
    type: aiml-gcsfuse
    attach_to: [tpu_pool]
  - name: shared-bkt
    type: cloud-storage
    attach_to: [tpu_pool]
  - name: ext_bkt
    type: pre-existing-network-storage
    attach_to: [tpu_pool]
    settings:
      gcs_bucket_name: some-bucket
      local_mount: /data
      fs_type: gcsfuse
  - name: job-tpl
    type: gke-job-template
    attach_to: [tpu_pool]
    settings:
      image: busybox
      command: ["echo", "ok"]
      node_count: 1
  - name: kueue
    type: kueue-jobset
`)
	cfgGKE, err := ast.UnmarshalClusterConfigStrict(rawGKE)
	if err != nil {
		t.Fatalf("unmarshal rawGKE failed: %v", err)
	}
	groupsGKE, err := engine.NewCompiler(repoRoot).Compile(cfgGKE)
	if err != nil {
		t.Fatalf("compile rawGKE failed: %v", err)
	}
	assertGKEStorageAndJobDefaults(t, groupsGKE)
	assertGKEKueueAutoscalingAndPV(t, cfgGKE, groupsGKE)
}

func assertGKEStorageAndJobDefaults(t *testing.T, groupsGKE []ast.DeploymentGroup) {
	t.Helper()
	// GKE-1: aiml-gcsfuse & cloud-storage on GKE use CSI-compatible mount_options
	for _, bktID := range []string{"ml-fuse_bucket", "shared-bkt_bucket"} {
		bktMod := findModByID(groupsGKE, bktID)
		if bktMod == nil {
			t.Fatalf("missing module %s", bktID)
		}
		if bktMod.Settings["mount_options"] != "defaults,_netdev,implicit-dirs" {
			t.Errorf("expected %s mount_options='defaults,_netdev,implicit-dirs' on GKE, got %v", bktID, bktMod.Settings["mount_options"])
		}
	}
	for _, bktID := range []string{"ml-fuse_checkpoints", "ml-fuse_training_data", "ml-fuse_model_serving"} {
		bktMod := findModByID(groupsGKE, bktID)
		if bktMod == nil {
			t.Fatalf("missing module %s", bktID)
		}
		if bktMod.Settings["mount_options"] != "defaults,_netdev,implicit_dirs" {
			t.Errorf("expected %s mount_options='defaults,_netdev,implicit_dirs' on GKE, got %v", bktID, bktMod.Settings["mount_options"])
		}
	}

	// GKE-2: gke-job-template has default k8s_service_account_name
	jobMod := findModByID(groupsGKE, "tpu_pool_job-tpl")
	if jobMod == nil {
		t.Fatal("missing tpu_pool_job-tpl module")
	}
	if jobMod.Settings["k8s_service_account_name"] != "workload-identity-k8s-sa" {
		t.Errorf("expected default k8s_service_account_name='workload-identity-k8s-sa', got %v", jobMod.Settings["k8s_service_account_name"])
	}
}

func assertGKEKueueAutoscalingAndPV(t *testing.T, cfgGKE ast.ClusterConfig, groupsGKE []ast.DeploymentGroup) {
	t.Helper()
	// GKE-3: kueue on TPU Pathways cluster defaults accelerator_type: tpu-v6e-slice
	kueueMod := findModByID(groupsGKE, "kueue_manager")
	if kueueMod == nil {
		t.Fatal("missing kueue_manager module")
	}
	kueueMap, _ := kueueMod.Settings["kueue"].(map[string]any)
	tmplVars, _ := kueueMap["config_template_vars"].(map[string]any)
	if tmplVars["accelerator_type"] != "tpu-v6e-slice" {
		t.Errorf("expected kueue.config_template_vars.accelerator_type='tpu-v6e-slice', got %v", tmplVars)
	}

	// GKE-4: autoscaling removes static_node_count and prunes unused tpu_cluster_size
	tpuPool := findModByID(groupsGKE, "tpu_pool_pool")
	if tpuPool == nil {
		t.Fatal("missing tpu_pool_pool module")
	}
	if _, hasSNC := tpuPool.Settings["static_node_count"]; hasSNC {
		t.Errorf("expected static_node_count to be removed when autoscaling is configured, got %v", tpuPool.Settings["static_node_count"])
	}
	if _, hasUnusedVar := cfgGKE.Vars["tpu_cluster_size"]; hasUnusedVar {
		t.Errorf("expected unused tpu_cluster_size var to be pruned after static_node_count removal")
	}

	// GKE-5: pre-existing-network-storage PV/PVC names are RFC-1123 compliant (no underscores) and distinct
	extPV := findModByID(groupsGKE, "ext_bkt_volume_mapping")
	if extPV == nil {
		t.Fatal("missing ext_bkt_volume_mapping module")
	}
	if pvName, _ := extPV.Settings["pv_name"].(string); strings.Contains(pvName, "_") || pvName != "ext-bkt-pv" {
		t.Errorf("expected RFC-1123 pv_name='ext-bkt-pv', got %q", pvName)
	}
	if pvcName, _ := extPV.Settings["pvc_name"].(string); strings.Contains(pvcName, "_") || pvcName != "ext-bkt-pvc" {
		t.Errorf("expected RFC-1123 pvc_name='ext-bkt-pvc', got %q", pvcName)
	}
}

func testCatalogParityConfigs(t *testing.T, repoRoot string) {
	t.Helper()
	// 2. Catalog parity checks (a3ultra-slurm.yaml Lustre, slurm enable_controller_public_ips, a4high-jbvm firewall_rules)
	a3uBytes, err := os.ReadFile(filepath.Join(repoRoot, "v2/cluster-configs/a3ultra-slurm.yaml"))
	if err != nil {
		t.Fatalf("failed to read a3ultra-slurm.yaml: %v", err)
	}
	cfgA3U, err := ast.UnmarshalClusterConfigStrict(a3uBytes)
	if err != nil {
		t.Fatalf("unmarshal a3ultra-slurm.yaml failed: %v", err)
	}
	cfgA3U.Vars["project_id"] = "test-proj"
	cfgA3U.Vars["region"] = "us-central1"
	cfgA3U.Vars["zone"] = "us-central1-a"
	groupsA3U, err := engine.NewCompiler(repoRoot).Compile(cfgA3U)
	if err != nil {
		t.Fatalf("compile a3ultra-slurm.yaml failed: %v", err)
	}
	lustreMod := findModByID(groupsA3U, "my-lustre")
	if lustreMod == nil || lustreMod.Settings["size_gib"] != 36000 || lustreMod.Settings["per_unit_storage_throughput"] != 500 || lustreMod.Settings["local_mount"] != "/home" {
		t.Errorf("unexpected a3ultra-slurm lustre settings: %v", lustreMod)
	}
	slurmCtrl := findModByID(groupsA3U, "slurm_controller")
	if slurmCtrl == nil || slurmCtrl.Settings["enable_controller_public_ips"] != true {
		t.Errorf("expected slurm_controller.enable_controller_public_ips=true, got %v", slurmCtrl)
	}
	testCatalogParityA4HighJBVM(t, repoRoot)
}

func testCatalogParityA4HighJBVM(t *testing.T, repoRoot string) {
	t.Helper()
	a4hBytes, err := os.ReadFile(filepath.Join(repoRoot, "v2/cluster-configs/a4high-jbvm.yaml"))
	if err != nil {
		t.Fatalf("failed to read a4high-jbvm.yaml: %v", err)
	}
	cfgA4H, err := ast.UnmarshalClusterConfigStrict(a4hBytes)
	if err != nil {
		t.Fatalf("unmarshal a4high-jbvm.yaml failed: %v", err)
	}
	cfgA4H.Vars["project_id"] = "test-proj"
	cfgA4H.Vars["region"] = "us-central1"
	cfgA4H.Vars["zone"] = "us-central1-a"
	groupsA4H, err := engine.NewCompiler(repoRoot).Compile(cfgA4H)
	if err != nil {
		t.Fatalf("compile a4high-jbvm.yaml failed: %v", err)
	}
	net1Mod := findModByID(groupsA4H, "net-1")
	if net1Mod == nil || net1Mod.Settings["firewall_rules"] == nil {
		t.Errorf("expected net-1 in a4high-jbvm to include firewall_rules, got %v", net1Mod)
	}
}

func TestEngineZeroHardcodingGeneralizations(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	testZeroHardcodingGKE(t, repoRoot)
	testZeroHardcodingSlurm(t, repoRoot)
}

func testZeroHardcodingGKE(t *testing.T, repoRoot string) {
	t.Helper()
	// 1. GKE multi-pool with workload_manager deduplication and cross-feature PVC wiring to pool-fanned-out jobs
	rawGKE := []byte(`
config_base: gke
vars:
  project_id: test-proj
  deployment_name: zero-hardcoding-gke
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: a4-pool-1
    machine_type: a4-highgpu-8g
    settings:
      static_node_count: 2
  - name: a4-pool-2
    machine_type: a4-highgpu-8g
    settings:
      static_node_count: 4
  - name: cpu-pool
    machine_type: n2-standard-8
features:
  - name: shared-fs
    type: filestore
    attach_to: [a4-pool-1, a4-pool-2]
    settings:
      local_mount: /shared
overlays:
  - name: smi
    type: run-nvidia-smi
    attach_to: [a4-pool-1, a4-pool-2]
  - name: fio
    type: fio-bench-job
    attach_to: [cpu-pool]
`)
	cfgGKE, err := ast.UnmarshalClusterConfigStrict(rawGKE)
	if err != nil {
		t.Fatalf("unmarshal rawGKE failed: %v", err)
	}
	groupsGKE, err := engine.NewCompiler(repoRoot).Compile(cfgGKE)
	if err != nil {
		t.Fatalf("compile rawGKE failed: %v", err)
	}

	wmCount := countModulesContaining(groupsGKE, "workload_manager")
	if wmCount != 1 {
		t.Errorf("expected exactly 1 deduplicated workload_manager module, got %d", wmCount)
	}

	smi1 := findModByID(groupsGKE, "a4-pool-1_smi")
	smi2 := findModByID(groupsGKE, "a4-pool-2_smi")
	fio := findModByID(groupsGKE, "cpu-pool_fio")
	if smi1 == nil || smi2 == nil || fio == nil {
		t.Fatal("missing expected GKE job modules")
	}
	if !containsStr(smi1.Use, "shared-fs_volume_mapping") || !containsStr(smi2.Use, "shared-fs_volume_mapping") {
		t.Errorf("expected shared-fs_volume_mapping wired to both smi jobs, got smi1=%v smi2=%v", smi1.Use, smi2.Use)
	}
	if containsStr(fio.Use, "shared-fs_volume_mapping") {
		t.Errorf("expected shared-fs_volume_mapping NOT wired to cpu-pool_fio, got %v", fio.Use)
	}
}

func countModulesContaining(groups []ast.DeploymentGroup, substr string) int {
	count := 0
	for _, g := range groups {
		for _, m := range g.Modules {
			if strings.Contains(m.ID, substr) {
				count++
			}
		}
	}
	return count
}

func testZeroHardcodingSlurm(t *testing.T, repoRoot string) {
	t.Helper()
	// 2. Slurm role-targeted login startup script + login filestore:
	// login-fs bridges to slurm_controller.login_network_storage, while login_startup wires directly to slurm_login
	// without polluting slurm_controller.login_startup_script.
	rawSlurm := []byte(`
config_base: slurm
vars:
  project_id: test-proj
  deployment_name: zero-hardcoding-slurm
  region: us-central1
  zone: us-central1-a
compute_archetypes:
  - name: pool-a
    machine_type: n2-standard-4
features:
  - name: login-fs
    type: filestore
    attach_to: [login]
    settings:
      local_mount: /login_only
  - name: login-init
    type: startup-script
    attach_to: [login]
    settings:
      runners:
        - type: shell
          destination: l.sh
          content: "#!/bin/bash\necho login\n"
`)
	cfgSlurm, err := ast.UnmarshalClusterConfigStrict(rawSlurm)
	if err != nil {
		t.Fatalf("unmarshal rawSlurm failed: %v", err)
	}
	groupsSlurm, err := engine.NewCompiler(repoRoot).Compile(cfgSlurm)
	if err != nil {
		t.Fatalf("compile rawSlurm failed: %v", err)
	}
	ctrl := findModByID(groupsSlurm, "slurm_controller")
	login := findModByID(groupsSlurm, "slurm_login")
	if ctrl == nil || login == nil {
		t.Fatal("missing slurm_controller or slurm_login")
	}
	if !strings.Contains(fmt.Sprintf("%v", ctrl.Settings["login_network_storage"]), "$(login-fs.network_storage)") {
		t.Errorf("expected slurm_controller.login_network_storage to bridge $(login-fs.network_storage), got %v", ctrl.Settings["login_network_storage"])
	}
	if _, polluted := ctrl.Settings["login_startup_script"]; polluted {
		t.Errorf("expected slurm_controller.login_startup_script NOT to be bridged when slurm_login accepts startup_script directly, got %v", ctrl.Settings["login_startup_script"])
	}
	if !containsStr(login.Use, "login_startup") {
		t.Errorf("expected slurm_login.Use to contain login_startup, got %v", login.Use)
	}
}

func TestHotlist8922304Phase1Fixes(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)
	baseVars := func() map[string]any {
		return map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "hotlist-test",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		}
	}
	testHotlistValidationDiagnostics(t, compiler, baseVars)
	testHotlistVarPromotionAndScoping(t, compiler, baseVars)
}

func testHotlistValidationDiagnostics(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 1. Missing machine_type on compute pool (b/567525394, b/566978451, b/566975515, b/566973284)
	_, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool"}},
	})
	if err == nil || !strings.Contains(err.Error(), "missing required field `machine_type`") {
		t.Errorf("expected missing machine_type error, got: %v", err)
	}

	// 2. Pool name containing spaces (b/566019042)
	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu 1", MachineType: "n2-standard-4"}},
	})
	if err == nil || !strings.Contains(err.Error(), `invalid compute archetype name "cpu 1"`) {
		t.Errorf("expected invalid compute archetype name error for 'cpu 1', got: %v", err)
	}

	// 3. Uppercase/malformed machine type (b/566025408)
	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "N2-STANDARD-2"}},
	})
	if err == nil || !strings.Contains(err.Error(), `machine type "N2-STANDARD-2" does not exist`) {
		t.Errorf("expected clean machine type error for N2-STANDARD-2, got: %v", err)
	}

	testHotlistAcceleratorAndTypoDiagnostics(t, compiler, baseVars)
}

func testHotlistAcceleratorAndTypoDiagnostics(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 4. Unknown accelerator machine type filters suggestions by config_base (b/566019222)
	_, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "gpu-pool", MachineType: "a3-ultragpu-4g"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown accelerator SKU a3-ultragpu-4g")
	}
	if !strings.Contains(err.Error(), "Available accelerator archetypes for slurm: a3-ultragpu-8g, a4-highgpu-8g, a4x-highgpu-4g") {
		t.Errorf("expected slurm-filtered accelerator list, got: %v", err)
	}
	if strings.Contains(err.Error(), "a3-megagpu-8g") || strings.Contains(err.Error(), "tpu-v6e") {
		t.Errorf("slurm error must not suggest GKE-only archetypes, got: %v", err)
	}

	// 5. Structured module settings validation error with Levenshtein suggestion (b/566011154)
	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "cpu-pool",
			MachineType: "n2-standard-4",
			Settings:    map[string]any{"node_count_statc": 4},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown setting "node_count_statc" for pool "cpu-pool" (did you mean "node_count_static"?)`) || strings.Contains(err.Error(), "Accepted settings:") {
		t.Errorf("expected concise Levenshtein suggestion without Accepted settings dump, got: %v", err)
	}
}

func testHotlistVarPromotionAndScoping(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 6. Single-pool TPU `tpu_num_slices` override in settings propagates to Kueue (b/565607914 Comment #4A)
	cfgTPU := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars: map[string]any{
			"project_id":       "test-proj",
			"deployment_name":  "tpu-slices-sync",
			"region":           "us-central1",
			"zone":             "us-central1-a",
			"authorized_cidr":  "0.0.0.0/0",
			"tpu_cluster_size": 2,
			"tpu_num_slices":   2,
		},
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "tpu-compute",
			MachineType: "ct6e-standard-4t",
			Settings:    map[string]any{"tpu_num_slices": 4},
		}},
		Features: []ast.FeatureReference{{
			Name: "kueue",
			Type: "kueue-jobset",
			Settings: map[string]any{
				"kueue": map[string]any{
					"config_template_vars": map[string]any{
						"accelerator_type": "tpu-v6e-slice",
						"tpu_quota":        "$(vars.tpu_num_slices * vars.tpu_cluster_size * tpu-compute_pool.tpu_chips_per_node)",
					},
				},
			},
		}},
	}
	if _, err := compiler.Compile(cfgTPU); err != nil {
		t.Fatalf("compile cfgTPU failed: %v", err)
	}
	if cfgTPU.Vars["tpu_num_slices"] != 4 {
		t.Errorf("expected Vars[tpu_num_slices]=4 for kueue feature, got %v", cfgTPU.Vars["tpu_num_slices"])
	}

	// 7. Multi-pool Slurm `local_ssd_mountpoint` override propagates to controller_startup (b/565607914 Comment #4B)
	cfgSSD := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{
			{
				Name:        "gpu-pool",
				MachineType: "a4-highgpu-8g",
				Settings:    map[string]any{"local_ssd_mountpoint": "/mnt/custom_nvme"},
			},
			{
				Name:        "cpu-pool",
				MachineType: "n2-standard-8",
			},
		},
	}
	if _, err := compiler.Compile(cfgSSD); err != nil {
		t.Fatalf("compile cfgSSD failed: %v", err)
	}
	if cfgSSD.Vars["local_ssd_mountpoint"] != "/mnt/custom_nvme" {
		t.Errorf("expected Vars[local_ssd_mountpoint]='/mnt/custom_nvme' for controller_startup, got %v", cfgSSD.Vars["local_ssd_mountpoint"])
	}
	testHotlistProjectAndDiskScoping(t, compiler, baseVars)
}

func testHotlistProjectAndDiskScoping(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 8. Invalid 3-char project_id ("abc") does not falsely report invalid machine type (b/566014125)
	_, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars: map[string]any{
			"project_id":      "abc",
			"deployment_name": "invalid-proj-test",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		},
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-2"}},
	})
	if err != nil {
		t.Errorf("expected Compile to defer invalid project 'abc' to project validator rather than failing on machine type, got: %v", err)
	}

	// 9. Multi-pool TPU + CPU with settings.disk_size_gb override prunes unscoped disk_size_gb (b/566025274 Comment #3)
	cfgBleed := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "tpu-pool", MachineType: "ct6e-standard-4t"},
			{Name: "cpu-pool", MachineType: "n2-standard-8", Settings: map[string]any{"disk_size_gb": 250}},
		},
	}
	if _, err := compiler.Compile(cfgBleed); err != nil {
		t.Fatalf("compile cfgBleed failed: %v", err)
	}
	if cfgBleed.Vars["cpu_pool_disk_size_gb"] != 250 {
		t.Errorf("expected Vars[cpu_pool_disk_size_gb]=250, got %v", cfgBleed.Vars["cpu_pool_disk_size_gb"])
	}
	if _, leaked := cfgBleed.Vars["disk_size_gb"]; leaked {
		t.Errorf("expected unscoped disk_size_gb to be pruned so it does not bleed into tpu-pool_pool, got %v", cfgBleed.Vars["disk_size_gb"])
	}

	// 10. Multi-pool TPU + CPU without settings.disk_size_gb also scopes cpu_pool_disk_size_gb (b/566025274 Comment #3)
	cfgBleedDefault := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "tpu-pool", MachineType: "ct6e-standard-4t"},
			{Name: "cpu-pool", MachineType: "n2-standard-8"},
		},
	}
	if _, err := compiler.Compile(cfgBleedDefault); err != nil {
		t.Fatalf("compile cfgBleedDefault failed: %v", err)
	}
	if cfgBleedDefault.Vars["cpu_pool_disk_size_gb"] != 100 {
		t.Errorf("expected Vars[cpu_pool_disk_size_gb]=100, got %v", cfgBleedDefault.Vars["cpu_pool_disk_size_gb"])
	}
	if _, leaked := cfgBleedDefault.Vars["disk_size_gb"]; leaked {
		t.Errorf("expected unscoped disk_size_gb to be scoped to cpu_pool_disk_size_gb so it does not auto-wire into tpu-pool_pool, got %v", cfgBleedDefault.Vars["disk_size_gb"])
	}
}

func TestBugBashPhase1GenuineFixes(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)
	baseVars := func() map[string]any {
		return map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "bugbash-test",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		}
	}
	testGenuineFixesStartupAndDeterminism(t, compiler, baseVars)
	testGenuineFixesStorageAssertions(t, compiler, baseVars)
	testGenuineFixesA4XAndOverlays(t, compiler, baseVars)
	testGenuineFixesRightSizingAndDeprecatedInputs(t, compiler, baseVars)
}

func testGenuineFixesStartupAndDeterminism(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 1. BB-17: startup-script feature with docker / local_ssd_filesystem (no explicit runners key) merges settings into {pool}_startup
	for _, baseType := range []string{"slurm", "jbvm"} {
		groups, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase: ast.ConfigBaseReference{Type: baseType},
			Vars:       baseVars(),
			ComputeArchetypes: []ast.ArchetypeReference{{
				Name:        "cpu-pool",
				MachineType: "n2-standard-4",
			}},
			Features: []ast.FeatureReference{
				{
					Name: "docker-setup",
					Type: "startup-script",
					Settings: map[string]any{
						"docker": map[string]any{"enabled": true},
					},
				},
				{
					Name: "ssd-setup",
					Type: "startup-script",
					Settings: map[string]any{
						"local_ssd_filesystem": map[string]any{"mountpoint": "/mnt/scratch"},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("compile startup-script docker+ssd on %s failed: %v", baseType, err)
		}
		startupMod := findModByID(groups, "cpu-pool_startup")
		if startupMod == nil {
			t.Fatalf("missing merged cpu-pool_startup on %s", baseType)
		}
		if startupMod.Settings["docker"] == nil || startupMod.Settings["local_ssd_filesystem"] == nil {
			t.Errorf("expected docker and local_ssd_filesystem settings merged on cpu-pool_startup (%s), got %v", baseType, startupMod.Settings)
		}
	}

	// 2. BB-22: splitSubmoduleSettings determinism across 25 iterations when multiple submodule aliases match
	for i := 0; i < 25; i++ {
		groups, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase: ast.ConfigBaseReference{
				Type: "slurm",
				Settings: map[string]any{
					"login":       map[string]any{"disk_size_gb": 180},
					"slurm_login": map[string]any{"machine_type": "n2-standard-8"},
				},
			},
			Vars:              baseVars(),
			ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-4"}},
		})
		if err != nil {
			t.Fatalf("compile determinism iteration %d failed: %v", i, err)
		}
		loginMod := findModByID(groups, "slurm_login")
		if loginMod == nil || fmt.Sprintf("%v", loginMod.Settings["disk_size_gb"]) != "180" || fmt.Sprintf("%v", loginMod.Settings["machine_type"]) != "n2-standard-8" {
			t.Fatalf("iteration %d: non-deterministic slurm_login settings: %v", i, loginMod)
		}
	}
}

func testGenuineFixesStorageAssertions(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 3. BB-23: pre-existing-network-storage rejects null/placeholder values on Slurm/JBVM and GKE
	for _, badSettings := range []map[string]any{
		{"server_ip": nil, "remote_mount": "/export", "local_mount": "/mnt/ext"},
		{"server_ip": "10.0.0.1", "remote_mount": "/mock-export", "local_mount": "/mnt/ext"},
		{"server_ip": "10.0.0.1", "remote_mount": nil, "local_mount": "/mnt/ext"},
	} {
		_, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
			Vars:              baseVars(),
			ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-4"}},
			Features: []ast.FeatureReference{{
				Name:     "ext-fs",
				Type:     "pre-existing-network-storage",
				Settings: badSettings,
			}},
		})
		if err == nil {
			t.Errorf("expected CEL assertion failure for bad Slurm pre-existing-network-storage settings %v", badSettings)
		}
	}
	for _, badGKESettings := range []map[string]any{
		{"fs_type": "gcs", "local_mount": "/data", "existing_storage_bucket_volume_id": nil},
		{"fs_type": "filestore", "local_mount": "/shared", "existing_filestore_instance_volume_id": nil},
	} {
		_, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase:        ast.ConfigBaseReference{Type: "gke"},
			Vars:              baseVars(),
			ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-4"}},
			Features: []ast.FeatureReference{{
				Name:     "ext-vol",
				Type:     "pre-existing-network-storage",
				Settings: badGKESettings,
			}},
		})
		if err == nil {
			t.Errorf("expected CEL assertion failure for bad GKE pre-existing-network-storage settings %v", badGKESettings)
		}
	}
}

func testGenuineFixesA4XAndOverlays(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 4. BB-07 & BB-08: A4X Slurm sets controller_state_disk=nil on t2a-standard-2 and substitutes local_ssd_mountpoint in a4x_startup
	cfgA4XSlurm := ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "a4x-compute",
			MachineType: "a4x-highgpu-4g",
			Settings:    map[string]any{"local_ssd_mountpoint": "/mnt/custom_nvme"},
		}},
	}
	groupsA4XSlurm, err := compiler.Compile(cfgA4XSlurm)
	if err != nil {
		t.Fatalf("compile a4x slurm failed: %v", err)
	}
	ctrlA4X := findModByID(groupsA4XSlurm, "slurm_controller")
	if ctrlA4X == nil {
		t.Fatal("missing slurm_controller on a4x")
	}
	if val, exists := ctrlA4X.Settings["controller_state_disk"]; !exists || val != nil {
		t.Errorf("expected controller_state_disk: nil on A4X slurm_controller, got exists=%v val=%v", exists, val)
	}
	a4xStartup := findModByID(groupsA4XSlurm, "a4x-compute_a4x_startup")
	if a4xStartup == nil || strings.Contains(fmt.Sprintf("%v", a4xStartup.Settings["runners"]), "mkdir -p /mnt/localssd") || cfgA4XSlurm.Vars["a4x_compute_local_ssd_mountpoint"] != "/mnt/custom_nvme" {
		t.Errorf("expected dynamic local_ssd_mountpoint in a4x-compute_a4x_startup runners, got %v", a4xStartup)
	}

	testGenuineFixesA4XJBVMAndGKE(t, compiler, baseVars)
}

func testGenuineFixesA4XJBVMAndGKE(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 5. BB-10: A4X JBVM startup_script writes /etc/ld.so.conf.d/000_nccl-gib.conf
	groupsA4XJBVM, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "jbvm"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "a4x-compute",
			MachineType: "a4x-highgpu-4g",
		}},
	})
	if err != nil {
		t.Fatalf("compile a4x jbvm failed: %v", err)
	}
	jbvmStartup := findModByID(groupsA4XJBVM, "a4x-compute_startup_script")
	if jbvmStartup == nil || !strings.Contains(fmt.Sprintf("%v", jbvmStartup.Settings["runners"]), "/etc/ld.so.conf.d/000_nccl-gib.conf") {
		t.Errorf("expected /etc/ld.so.conf.d/000_nccl-gib.conf in a4x JBVM startup_script, got %v", jbvmStartup)
	}

	// 6. BB-06: Arm64 tolerations present on all 3 GKE overlays
	groupsA4XGKE, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "a4x-compute",
			MachineType: "a4x-highgpu-4g",
		}},
		Overlays: []ast.FeatureReference{
			{Name: "smi", Type: "run-nvidia-smi"},
			{Name: "fio", Type: "fio-bench-job"},
			{Name: "nccl", Type: "nccl-jobset-test"},
		},
	})
	if err != nil {
		t.Fatalf("compile a4x gke overlays failed: %v", err)
	}
	for _, modID := range []string{"smi", "fio", "nccl"} {
		mod := findModByID(groupsA4XGKE, modID)
		if mod == nil || !strings.Contains(fmt.Sprintf("%v", mod.Settings), "arm64") {
			t.Errorf("expected arm64 toleration in %s settings, got %v", modID, mod)
		}
	}
}

func testGenuineFixesRightSizingAndDeprecatedInputs(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 7. BB-11 & BB-12: Slurm controller right-sizing and GKE network parity
	groupsA3USlurm, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "a3u-compute", MachineType: "a3-ultragpu-8g"}},
	})
	if err != nil {
		t.Fatalf("compile a3u slurm failed: %v", err)
	}
	if ctrl := findModByID(groupsA3USlurm, "slurm_controller"); ctrl == nil || ctrl.Settings["machine_type"] != "n2d-standard-16" {
		t.Errorf("expected n2d-standard-16 on a3u slurm_controller, got %v", ctrl)
	}

	groupsA3MGKE, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "gke"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "a3m-compute", MachineType: "a3-megagpu-8g"}},
	})
	if err != nil {
		t.Fatalf("compile a3m gke failed: %v", err)
	}
	if gpuNetMod := findModByID(groupsA3MGKE, "gpu-net"); gpuNetMod == nil || fmt.Sprintf("%v", gpuNetMod.Settings["mtu"]) != "8244" {
		t.Errorf("expected mtu=8244 on a3m GKE gpu-net, got %v", gpuNetMod)
	}
	if netMod := findModByID(groupsA3MGKE, "network"); netMod == nil || netMod.Settings["mtu"] != nil {
		t.Errorf("expected primary GKE network mtu to remain untouched by a3m archetype, got %v", netMod)
	}

	assertDeprecatedSlurmInputsNotAutowired(t, compiler, baseVars)
}

func assertDeprecatedSlurmInputsNotAutowired(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 8. Verify DEPRECATED Terraform inputs (network_storage on schedmd-slurm-gcp-v6-partition,
	// compute_startup_script on schedmd-slurm-gcp-v6-controller) are never auto-wired.
	groupsDeprecatedGuard, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-8"}},
		Features: []ast.FeatureReference{
			{Name: "sharedfs", Type: "filestore"},
			{Name: "bootscript", Type: "startup-script", Settings: map[string]any{
				"runners": []any{map[string]any{"type": "shell", "destination": "init.sh", "content": "echo ok"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("compile slurm with storage + startup-script failed: %v", err)
	}
	partMod := findModByID(groupsDeprecatedGuard, "cpu-pool_partition")
	if partMod == nil {
		t.Fatal("missing cpu-pool_partition")
	}
	for _, u := range partMod.Use {
		if u == "sharedfs" {
			t.Errorf("expected cpu-pool_partition NOT to auto-wire storage module 'sharedfs' via deprecated network_storage input, got Use=%v", partMod.Use)
		}
	}
	ctrlMod := findModByID(groupsDeprecatedGuard, "slurm_controller")
	if ctrlMod == nil || ctrlMod.Settings["compute_startup_script"] != nil {
		t.Errorf("expected slurm_controller.compute_startup_script to remain nil, got %v", ctrlMod)
	}
}

func TestPhase1Round2BugBashAndRenames(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	baseVars := func() map[string]any {
		return map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "r2-test",
			"region":          "us-central1",
			"zone":            "us-central1-a",
			"authorized_cidr": "0.0.0.0/0",
		}
	}

	testRound2ConsumptionAndMachineTypeGuards(t, compiler, baseVars)
	testRound2HeterogeneousGKEAutowire(t, compiler, baseVars)
	testRound2StorageSidecarRouting(t, compiler, baseVars)
	testRound2MultiPoolAndConflictChecks(t, compiler, baseVars)
}

func testRound2ConsumptionAndMachineTypeGuards(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 1. Option B: Reject literal machine_type: cpu and machine_type: generic-cpu with clear error
	for _, invalidMT := range []string{"cpu", "generic-cpu"} {
		_, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
			Vars:              baseVars(),
			ComputeArchetypes: []ast.ArchetypeReference{{Name: "pool-a", MachineType: invalidMT}},
		})
		if err == nil || !strings.Contains(err.Error(), "specify a concrete Google Cloud CPU machine type") {
			t.Errorf("expected concrete CPU machine type error for machine_type=%q, got: %v", invalidMT, err)
		}
	}

	// 2. b/566012866: Reject mutually exclusive GKE and Slurm consumption settings
	_, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "gpu-pool",
			MachineType: "a3-megagpu-8g",
			Settings: map[string]any{
				"spot": true,
				"reservation_affinity": map[string]any{
					"consume_reservation_type": "SPECIFIC_RESERVATION",
					"specific_reservations":    []any{map[string]any{"name": "my-res"}},
				},
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine spot VMs (spot: true) with a specific reservation") {
		t.Errorf("expected GKE spot + reservation error, got: %v", err)
	}

	testRound2SpotFlexAndSlurmReservationGuards(t, compiler, baseVars)
}

func testRound2SpotFlexAndSlurmReservationGuards(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	_, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "gpu-pool",
			MachineType: "a3-megagpu-8g",
			Settings: map[string]any{
				"spot":              true,
				"enable_flex_start": true,
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine spot VMs (spot: true) with DWS Flex-Start") {
		t.Errorf("expected GKE spot + flex-start error, got: %v", err)
	}

	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "gpu-pool",
			MachineType: "a3-ultragpu-8g",
			Settings: map[string]any{
				"enable_spot_vm":   true,
				"reservation_name": "my-slurm-res",
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine spot VMs (enable_spot_vm: true) with a reservation") {
		t.Errorf("expected Slurm spot + reservation error, got: %v", err)
	}
}

func testRound2HeterogeneousGKEAutowire(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 3. B1, B2, B3, B4: Heterogeneous GKE (GPU + CPU pool) with pool-attached storage and unattached overlays
	groupsHeteroGKE, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "gpu-pool", MachineType: "a4-highgpu-8g"},
			{Name: "cpu-pool", MachineType: "n2-standard-8"},
		},
		Features: []ast.FeatureReference{
			{Name: "gpu-fs", Type: "filestore", AttachTo: []string{"gpu-pool"}},
			{Name: "ai-workload", Type: "kueue-jobset"},
		},
		Overlays: []ast.FeatureReference{
			{Name: "smi-test", Type: "run-nvidia-smi"},
			{Name: "fio-test", Type: "fio-bench-job"},
			{Name: "nccl-test", Type: "nccl-jobset-test"},
		},
	})
	if err != nil {
		t.Fatalf("compile heterogeneous GKE failed: %v", err)
	}
	assertHeterogeneousGKEGPUAndFio(t, groupsHeteroGKE)
	testRound2HeterogeneousGKETPUAndCPUOnly(t, compiler, baseVars)
}

func assertHeterogeneousGKEGPUAndFio(t *testing.T, groupsHeteroGKE []ast.DeploymentGroup) {
	t.Helper()
	// B3: Unattached run-nvidia-smi in mixed GPU+CPU cluster wires only to GPU pool (gpu-pool_pool)
	smiMod := findModByID(groupsHeteroGKE, "gpu-pool_smi-test")
	if smiMod == nil {
		smiMod = findModByID(groupsHeteroGKE, "smi-test")
	}
	if smiMod == nil || containsStr(smiMod.Use, "cpu-pool_pool") {
		t.Errorf("expected unattached run-nvidia-smi NOT to wire to CPU pool cpu-pool_pool in heterogeneous GKE cluster, got %v", smiMod)
	}
	// B4 & B1: Unattached fio-bench-job wires pool-attached storage PV (gpu-fs_pv) and has no mandatory 1TB local-ssd ephemeral_volumes
	fioMod := findModByID(groupsHeteroGKE, "gpu-pool_fio-test")
	if fioMod == nil {
		fioMod = findModByID(groupsHeteroGKE, "fio-test")
	}
	if fioMod == nil || fioMod.Settings["ephemeral_volumes"] != nil || !containsStr(fioMod.Use, "gpu-fs_volume_mapping") {
		t.Errorf("expected unattached fio-bench-job to wire pool-attached gpu-fs_volume_mapping without ephemeral_volumes, got %v", fioMod)
	}
	// B2: nccl-jobset-test uses dynamic NUM_GPUS from nvidia-smi -L
	ncclMod := findModByID(groupsHeteroGKE, "nccl-test")
	if ncclMod == nil || !strings.Contains(fmt.Sprintf("%v", ncclMod.Settings["command"]), "NUM_GPUS") {
		t.Errorf("expected nccl-jobset-test to detect NUM_GPUS dynamically, got %v", ncclMod)
	}
}

func testRound2HeterogeneousGKETPUAndCPUOnly(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 3b. B3 (TPU + CPU heterogeneous GKE): Unattached workload targets TPU pool (placement_policy.tpu_topology)
	groupsTPUHetero, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "tpu-pool", MachineType: "ct6e-standard-4t"},
			{Name: "cpu-pool", MachineType: "n2-standard-8"},
		},
		Overlays: []ast.FeatureReference{{Name: "fio-tpu", Type: "fio-bench-job"}},
	})
	if err != nil {
		t.Fatalf("compile heterogeneous TPU+CPU GKE failed: %v", err)
	}
	fioTPU := findModByID(groupsTPUHetero, "fio-tpu")
	if fioTPU == nil || !containsStr(fioTPU.Use, "tpu-pool_pool") || containsStr(fioTPU.Use, "cpu-pool_pool") {
		t.Errorf("expected unattached fio-tpu in TPU+CPU GKE cluster to wire only to tpu-pool_pool, got %v", fioTPU)
	}

	// 3c. B3 (CPU-only multi-pool GKE): Unattached workload wires to ALL CPU pools when no accelerator pool exists
	groupsCPUOnlyGKE, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "cpu-a", MachineType: "n2-standard-8"},
			{Name: "cpu-b", MachineType: "c2-standard-60"},
		},
		Overlays: []ast.FeatureReference{{Name: "fio-cpu", Type: "fio-bench-job"}},
	})
	if err != nil {
		t.Fatalf("compile CPU-only multi-pool GKE failed: %v", err)
	}
	fioCPU := findModByID(groupsCPUOnlyGKE, "fio-cpu")
	if fioCPU == nil || !containsStr(fioCPU.Use, "cpu-a_pool") || !containsStr(fioCPU.Use, "cpu-b_pool") {
		t.Errorf("expected unattached fio-cpu in CPU-only GKE cluster to wire to both cpu-a_pool and cpu-b_pool, got %v", fioCPU)
	}
}

func testRound2StorageSidecarRouting(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 4. B5: aiml-gcsfuse top-level local_mount override (including trailing slash) derives unique sidecar mountpoints
	groupsAIML, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-8"}},
		Features: []ast.FeatureReference{
			{Name: "aiml-1", Type: "aiml-gcsfuse"},
			{Name: "aiml-2", Type: "aiml-gcsfuse", Settings: map[string]any{"local_mount": "/mnt/gcs2/"}},
		},
	})
	if err != nil {
		t.Fatalf("compile multi aiml-gcsfuse failed: %v", err)
	}
	for _, tc := range []struct{ id, want string }{
		{"aiml-2_checkpoints", "/mnt/gcs2-checkpoints"},
		{"aiml-2_training_data", "/mnt/gcs2-training-data"},
		{"aiml-2_model_serving", "/mnt/gcs2-model-serving"},
	} {
		m := findModByID(groupsAIML, tc.id)
		if m == nil || m.Settings["local_mount"] != tc.want {
			t.Errorf("expected %s local_mount=%s, got %v", tc.id, tc.want, m)
		}
	}

	// 5. B6: netapp-volume routes sidecar-produced keys (service_level, allow_auto_tiering, scale_type) to pool and avoids duplicating on volume
	groupsNetApp, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-8"}},
		Features: []ast.FeatureReference{{
			Name: "netapp-std",
			Type: "netapp-volume",
			Settings: map[string]any{
				"service_level":      "STANDARD",
				"allow_auto_tiering": true,
				"scale_type":         "SCALE_TYPE_DEFAULT",
				"capacity_gib":       1024,
			},
		}},
	})
	if err != nil {
		t.Fatalf("compile netapp-volume STANDARD failed: %v", err)
	}
	assertNetAppSidecarRouting(t, groupsNetApp)
}

func assertNetAppSidecarRouting(t *testing.T, groupsNetApp []ast.DeploymentGroup) {
	t.Helper()
	naPool := findModByID(groupsNetApp, "netapp-std_pool")
	naVol := findModByID(groupsNetApp, "netapp-std")
	if naPool == nil || naPool.Settings["service_level"] != "STANDARD" || naPool.Settings["allow_auto_tiering"] != true || naPool.Settings["scale_type"] != "SCALE_TYPE_DEFAULT" {
		t.Errorf("expected netapp-std_pool to receive service_level/allow_auto_tiering/scale_type, got %v", naPool)
	}
	if naVol == nil || naVol.Settings["service_level"] != nil || naVol.Settings["allow_auto_tiering"] != nil || naVol.Settings["scale_type"] != nil {
		t.Errorf("expected netapp-std volume not to duplicate pool-produced keys in settings, got %v", naVol)
	}
}

func testRound2MultiPoolAndConflictChecks(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 6. B7: Multi-pool a4x-highgpu-4g GKE has distinct {name}_workload_policy IDs and Slurm hydrates PARTITION_NAME
	groupsA4XGKE, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{
			{Name: "a4x-a", MachineType: "a4x-highgpu-4g"},
			{Name: "a4x-b", MachineType: "a4x-highgpu-4g"},
		},
	})
	if err != nil {
		t.Fatalf("compile multi-pool a4x GKE failed: %v", err)
	}
	if findModByID(groupsA4XGKE, "a4x-a_workload_policy") == nil || findModByID(groupsA4XGKE, "a4x-b_workload_policy") == nil {
		t.Errorf("expected distinct a4x-a_workload_policy and a4x-b_workload_policy modules")
	}

	groupsA4XSlurm, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "gb200", MachineType: "a4x-highgpu-4g"}},
	})
	if err != nil {
		t.Fatalf("compile a4x Slurm failed: %v", err)
	}
	ctrlStartup := findModByID(groupsA4XSlurm, "controller_startup")
	if ctrlStartup == nil || !strings.Contains(fmt.Sprintf("%v", ctrlStartup.Settings["runners"]), "$(gb200_partition.partitions[0].partition_name)") {
		t.Errorf("expected controller_startup to hydrate $(gb200_partition.partitions[0].partition_name), got %v", ctrlStartup)
	}

	testRound2NameAndMountConflicts(t, compiler, baseVars)
	testRound2SlurmDiskSizeOverrides(t, compiler, baseVars)
}

func testRound2NameAndMountConflicts(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 7. B8: Reject pool + feature same name conflict and feature + overlay different-type conflict
	_, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "gke"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "kueue", MachineType: "a3-megagpu-8g"}},
		Features:          []ast.FeatureReference{{Name: "kueue", Type: "kueue-jobset"}},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with compute archetype of the same name") {
		t.Errorf("expected pool/feature name conflict error, got: %v", err)
	}

	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "gke"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "gpu-pool", MachineType: "a3-megagpu-8g"}},
		Features:          []ast.FeatureReference{{Name: "shared-x", Type: "filestore"}},
		Overlays:          []ast.FeatureReference{{Name: "shared-x", Type: "run-nvidia-smi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with existing feature") {
		t.Errorf("expected overlay/feature conflict error, got: %v", err)
	}

	// 8. B9: gke-hyperdisk pv_mount_path duplicate mount detection & trailing-slash normalization
	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "gke"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "gpu-pool", MachineType: "a3-megagpu-8g"}},
		Features: []ast.FeatureReference{
			{Name: "hd-vol", Type: "gke-hyperdisk", Settings: map[string]any{"pv_mount_path": "/data"}},
			{Name: "gcs-vol", Type: "cloud-storage", Settings: map[string]any{"local_mount": "/data/"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate local_mount path \"/data\"") {
		t.Errorf("expected duplicate storage mount path error for gke-hyperdisk + cloud-storage on /data vs /data/, got: %v", err)
	}
}

func testRound2SlurmDiskSizeOverrides(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	// 9. B10: a3-ultragpu-8g and a4-highgpu-8g Slurm disk_size_gb override via vars and settings
	for _, mt := range []string{"a3-ultragpu-8g", "a4-highgpu-8g"} {
		varsOverride := baseVars()
		varsOverride["disk_size_gb"] = 500
		groupsVars, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
			Vars:              varsOverride,
			ComputeArchetypes: []ast.ArchetypeReference{{Name: "gpu", MachineType: mt}},
		})
		if err != nil {
			t.Fatalf("compile Slurm %s with vars.disk_size_gb=500 failed: %v", mt, err)
		}
		nsVars := findModByID(groupsVars, "gpu_nodeset")
		if nsVars == nil || nsVars.Settings["disk_size_gb"] != "$(vars.disk_size_gb)" || varsOverride["disk_size_gb"] != 500 {
			t.Errorf("expected Slurm %s gpu_nodeset.disk_size_gb='$(vars.disk_size_gb)' with vars.disk_size_gb=500, got mod=%v vars=%v", mt, nsVars, varsOverride["disk_size_gb"])
		}

		varsPool := baseVars()
		groupsPool, err := compiler.Compile(ast.ClusterConfig{
			ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
			Vars:       varsPool,
			ComputeArchetypes: []ast.ArchetypeReference{{
				Name:        "gpu",
				MachineType: mt,
				Settings:    map[string]any{"disk_size_gb": 750},
			}},
		})
		if err != nil {
			t.Fatalf("compile Slurm %s with settings.disk_size_gb=750 failed: %v", mt, err)
		}
		nsPool := findModByID(groupsPool, "gpu_nodeset")
		if nsPool == nil || varsPool["gpu_disk_size_gb"] != 750 {
			t.Errorf("expected Slurm %s settings.disk_size_gb=750 to promote gpu_disk_size_gb=750, got mod=%v vars=%v", mt, nsPool, varsPool)
		}
	}
}

func TestBugBashPS26Regressions(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)
	baseVars := func() map[string]any {
		return map[string]any{
			"project_id":      "test-proj",
			"deployment_name": "test-dep",
			"region":          "us-central1",
			"zone":            "us-central1-a",
		}
	}

	testPS26DoubleParenVarExpressions(t, compiler, baseVars)
	testPS26MultiRoleAttachBridgingAndBaseError(t, compiler, baseVars)
	testPS26LustreAndGCSStorageManifests(t, compiler, baseVars)
}

func testPS26DoubleParenVarExpressions(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	varsGKE := baseVars()
	varsGKE["my_flex"] = true
	groupsGKE, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "gke"},
		Vars:       varsGKE,
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "gpu-pool",
			MachineType: "a3-megagpu-8g",
			Settings:    map[string]any{"enable_flex_start": "((var.my_flex))"},
		}},
	})
	if err != nil {
		t.Fatalf("compile GKE with ((var.my_flex)) failed: %v", err)
	}
	poolMod := findModByID(groupsGKE, "gpu-pool_pool")
	if poolMod == nil || poolMod.Settings["auto_repair"] != false || poolMod.Settings["static_node_count"] != nil {
		t.Errorf("expected ((var.my_flex))=true to set auto_repair=false and drop static_node_count, got %v", poolMod)
	}

	varsSlurm := baseVars()
	varsSlurm["my_dws"] = true
	varsSlurm["my_res"] = "res-1"
	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{Type: "slurm"},
		Vars:       varsSlurm,
		ComputeArchetypes: []ast.ArchetypeReference{{
			Name:        "gpu-pool",
			MachineType: "a3-ultragpu-8g",
			Settings: map[string]any{
				"dws_flex":         map[string]any{"enabled": "((var.my_dws))"},
				"reservation_name": "((var.my_res))",
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot combine") {
		t.Errorf("expected ((var.my_dws)) and ((var.my_res)) mutual exclusion error, got: %v", err)
	}
}

func testPS26MultiRoleAttachBridgingAndBaseError(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	groupsSlurm, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "slurm"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-8"}},
		Features: []ast.FeatureReference{{
			Name:     "shared-fs",
			Type:     "filestore",
			AttachTo: []string{"controller", "login"},
		}},
	})
	if err != nil {
		t.Fatalf("compile Slurm with attach_to: [controller, login] failed: %v", err)
	}
	ctrlMod := findModByID(groupsSlurm, "slurm_controller")
	if ctrlMod == nil || !containsStr(ctrlMod.Use, "shared-fs") || ctrlMod.Settings["login_network_storage"] == nil {
		t.Errorf("expected slurm_controller to use shared-fs and receive bridged login_network_storage, got %v", ctrlMod)
	}

	_, err = compiler.Compile(ast.ClusterConfig{
		ConfigBase: ast.ConfigBaseReference{
			Type:     "slurm",
			Settings: map[string]any{"unknown_base_setting_xyz": true},
		},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "cpu-pool", MachineType: "n2-standard-8"}},
	})
	if err == nil || !strings.Contains(err.Error(), `"slurm"`) {
		t.Errorf("expected unknown config_base.settings error to mention \"slurm\", got: %v", err)
	}
}

func testPS26LustreAndGCSStorageManifests(t *testing.T, compiler *engine.Compiler, baseVars func() map[string]any) {
	t.Helper()
	groupsGKE, err := compiler.Compile(ast.ClusterConfig{
		ConfigBase:        ast.ConfigBaseReference{Type: "gke"},
		Vars:              baseVars(),
		ComputeArchetypes: []ast.ArchetypeReference{{Name: "gpu-pool", MachineType: "a3-megagpu-8g"}},
		Features: []ast.FeatureReference{
			{Name: "my-lustre", Type: "managed-lustre", Settings: map[string]any{"size_gib": 18000, "local_mount": "/lustre"}},
			{Name: "my-aiml", Type: "aiml-gcsfuse"},
		},
	})
	if err != nil {
		t.Fatalf("compile GKE with managed-lustre + aiml-gcsfuse failed: %v", err)
	}
	lustrePV := findModByID(groupsGKE, "my-lustre_volume_mapping")
	if lustrePV == nil || lustrePV.Settings["capacity_gib"] != nil {
		t.Errorf("expected my-lustre_volume_mapping not to hardcode capacity_gib=36000 in settings (auto-wired via use), got %v", lustrePV)
	}
	trainPV := findModByID(groupsGKE, "my-aiml_training_pv")
	servePV := findModByID(groupsGKE, "my-aiml_serving_pv")
	if trainPV == nil || trainPV.Settings["grant_gcsfuse_service_agent_role"] != false || servePV == nil || servePV.Settings["grant_gcsfuse_service_agent_role"] != false {
		t.Errorf("expected training_pv and serving_pv to set grant_gcsfuse_service_agent_role=false, got train=%v serve=%v", trainPV, servePV)
	}
}
