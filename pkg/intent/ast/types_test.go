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

package ast_test

import (
	"testing"

	"hpc-toolkit/pkg/intent/ast"

	"gopkg.in/yaml.v3"
)

func TestUnmarshalClusterConfig(t *testing.T) {
	yamlInput := `
config_base: slurm
vars:
  deployment_name: a3u-test
  project_id: test-project
compute_archetypes:
  - name: a3u-pool
    machine_type: a3-ultragpu-8g
    settings:
      node_count_static: 32
features:
  - name: homefs
    type: managed-lustre
    settings:
      capacity_gb: 10240
`
	cfg, err := ast.UnmarshalClusterConfigStrict([]byte(yamlInput))
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if cfg.ConfigBase.Type != "slurm" {
		t.Errorf("got ConfigBase %q, want %q", cfg.ConfigBase.Type, "slurm")
	}

	if len(cfg.ComputeArchetypes) != 1 {
		t.Fatalf("got %d compute archetypes, want 1", len(cfg.ComputeArchetypes))
	}

	arch := cfg.ComputeArchetypes[0]
	if arch.Name != "a3u-pool" || arch.MachineType != "a3-ultragpu-8g" {
		t.Errorf("unexpected archetype parsing: %+v", arch)
	}
	if v, ok := arch.Settings["node_count_static"]; !ok || v.(int) != 32 {
		t.Errorf("unexpected archetype settings: %+v", arch.Settings)
	}

	if len(cfg.Features) != 1 {
		t.Fatalf("got %d features, want 1", len(cfg.Features))
	}

	feat := cfg.Features[0]
	if feat.Name != "homefs" || feat.Type != "managed-lustre" {
		t.Errorf("unexpected feature parsing: %+v", feat)
	}
}

func TestUnmarshalOverlayManifest(t *testing.T) {
	yamlInput := `
name: run-nvidia-smi
description: GPU visibility check
config_base: gke
features:
  - name: run-nvidia-smi
    type: gke-job-template
    settings:
      image: nvidia/cuda:12.8.0-runtime-ubuntu24.04
`
	var ov ast.OverlayManifest
	if err := yaml.Unmarshal([]byte(yamlInput), &ov); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if ov.Name != "run-nvidia-smi" || ov.ConfigBase.Type != "gke" || len(ov.Features) != 1 {
		t.Errorf("unexpected overlay manifest: %+v", ov)
	}
}

func TestUnmarshalClusterConfigVariantsAndHelpers(t *testing.T) {
	// Empty input
	if cfg, err := ast.UnmarshalClusterConfigStrict([]byte("")); err != nil || cfg.ConfigBase.Type != "" {
		t.Fatalf("expected empty ClusterConfig on EOF, got %+v, err=%v", cfg, err)
	}

	// Lenient vs strict unknown field
	if _, err := ast.UnmarshalClusterConfig([]byte("config_base: gke\nunknown_field: 1\n")); err != nil {
		t.Fatalf("expected lenient UnmarshalClusterConfig to allow unknown field, got: %v", err)
	}
	if _, err := ast.UnmarshalClusterConfigStrict([]byte("config_base: gke\nunknown_field: 1\n")); err == nil {
		t.Fatal("expected strict UnmarshalClusterConfigStrict to reject unknown field")
	}
	if _, err := ast.UnmarshalClusterConfig([]byte("[\n")); err == nil {
		t.Fatal("expected syntax error on invalid yaml in lenient mode")
	}

	// ConfigBase mapping with settings, default Feature/Overlay Type from Name, and MarshalYAML
	mappedYAML := `
config_base:
  type: slurm
  settings:
    machine_type: n2-standard-16
features:
  - name: filestore
overlays:
  - name: run-nvidia-smi
`
	cfg, err := ast.UnmarshalClusterConfigStrict([]byte(mappedYAML))
	if err != nil {
		t.Fatalf("unexpected error for mapping config_base: %v", err)
	}
	if cfg.Features[0].Type != "filestore" || cfg.Overlays[0].Type != "run-nvidia-smi" {
		t.Fatalf("expected Type to default from Name, got feat=%+v ov=%+v", cfg.Features[0], cfg.Overlays[0])
	}
	outBytes, err := yaml.Marshal(cfg.ConfigBase)
	if err != nil || len(outBytes) == 0 {
		t.Fatalf("MarshalYAML mapping failed: %v", err)
	}
	scalarOut, err := yaml.Marshal(ast.ConfigBaseReference{Type: "gke"})
	if err != nil || string(scalarOut) != "gke\n" {
		t.Fatalf("MarshalYAML scalar got %q, err=%v", string(scalarOut), err)
	}
	testUnmarshalInvalidConfigBaseAndDeploymentGroup(t)
}

func testUnmarshalInvalidConfigBaseAndDeploymentGroup(t *testing.T) {
	t.Helper()
	// Invalid config_base cases
	if _, err := ast.UnmarshalClusterConfigStrict([]byte("config_base:\n  settings:\n    a: 1\n")); err == nil {
		t.Fatal("expected error when config_base mapping omits type")
	}
	if _, err := ast.UnmarshalClusterConfigStrict([]byte("config_base:\n  type: slurm\n  bogus: 1\n")); err == nil {
		t.Fatal("expected error when config_base mapping has unknown key")
	}
	if _, err := ast.UnmarshalClusterConfigStrict([]byte("config_base:\n  - list_item\n")); err == nil {
		t.Fatal("expected error when config_base is a sequence")
	}

	// DeploymentGroup UnmarshalYAML & IsComputePool detection
	dgYAML := `
group: cluster
modules:
  - id: "{name}_nodeset"
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    settings:
      machine_type: a3-ultragpu-8g
  - id: "{name}_partition"
    source: community/modules/compute/schedmd-slurm-gcp-v6-partition
`
	var dg ast.DeploymentGroup
	if err := yaml.Unmarshal([]byte(dgYAML), &dg); err != nil {
		t.Fatalf("DeploymentGroup UnmarshalYAML failed: %v", err)
	}
	if len(dg.Modules) != 2 || !dg.Modules[0].IsComputePool || dg.Modules[1].IsComputePool {
		t.Fatalf("unexpected IsComputePool flags: %+v", dg.Modules)
	}
	if err := yaml.Unmarshal([]byte("group: cluster\nunknown_dg_key: 1\n"), &dg); err == nil {
		t.Fatal("expected strict DeploymentGroup unmarshal to reject unknown key")
	}

	// UnmarshalClusterConfigFile (missing and valid)
	tmpFile := t.TempDir() + "/cfg.yaml"
	if _, err := ast.UnmarshalClusterConfigFile(tmpFile); err == nil {
		t.Fatal("expected error for missing file")
	}
	var emptyCB ast.ConfigBaseReference
	_ = emptyCB.UnmarshalYAML(&yaml.Node{Kind: 0})
}
