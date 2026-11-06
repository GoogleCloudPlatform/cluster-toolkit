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

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestMachineTypesExcludeControlPlane guards the catalog against advertising
// login/controller/system-pool/image-builder VMs as a blueprint's workload
// machine type: a control-plane var's value may only appear in machine_types
// when a workload var (any other *machine_type key) carries the same value.
func TestMachineTypesExcludeControlPlane(t *testing.T) {
	_, blueprints := cachedScan(t)
	byPath := make(map[string]BlueprintEntry, len(blueprints))
	for _, bp := range blueprints {
		byPath[bp.Path] = bp
		// Convention applies to official examples/ only; community blueprints
		// may legitimately reuse a login/image-builder shape for a workload VM.
		if strings.HasPrefix(bp.Path, "examples/") {
			checkNoControlPlaneLeak(t, bp)
		}
	}
	verifyPinnedMachineTypes(t, byPath)
}

func checkNoControlPlaneLeak(t *testing.T, bp BlueprintEntry) {
	t.Helper()
	workload := map[string]bool{}
	for k, v := range bp.Vars {
		if s, ok := v.(string); ok && strings.HasSuffix(k, "machine_type") && !controlPlaneMachineTypeVars[k] {
			workload[s] = true
		}
	}
	for k := range controlPlaneMachineTypeVars {
		s, ok := bp.Vars[k].(string)
		if ok && !workload[s] && slices.Contains(bp.MachineTypes, s) {
			t.Errorf("%s: control-plane var %s=%q leaked into machine_types %v", bp.Path, k, s, bp.MachineTypes)
		}
	}
}

func verifyPinnedMachineTypes(t *testing.T, byPath map[string]BlueprintEntry) {
	t.Helper()
	for path, want := range map[string][]string{
		"examples/gke-a4/gke-a4.yaml":                 {"a4-highgpu-8g"},
		"examples/gke-tpu-v6e/gke-tpu-v6e.yaml":       {"ct6e-standard-4t"},
		"examples/gke-g4/gke-g4.yaml":                 {"g4-standard-48"},
		"examples/hpc-gke.yaml":                       {"c2-standard-60"},
		"examples/image-builder.yaml":                 {"c2-standard-60"},
		"examples/hpc-slurm.yaml":                     {"n2-standard-2", "c2-standard-60", "h3-standard-88"},
		"examples/ml-slurm-g4.yaml":                   {"g4-standard-48"},
		"examples/pfs-managed-lustre-slurm.yaml":      {"c2-standard-4"},
		"community/examples/slurm-gke/slurm-gke.yaml": {"n2d-standard-4", "c2-standard-16", "n2-standard-2"},
	} {
		got := byPath[path].MachineTypes
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: machine_types = %v, want %v", path, got, want)
		}
	}
	// Control-plane vars must still be present in vars so deployment.yaml can override them.
	for path, key := range map[string]string{
		"examples/gke-a4/gke-a4.yaml": "system_node_pool_machine_type",
		"examples/hpc-slurm.yaml":     "login_machine_type",
		"examples/ml-slurm-g4.yaml":   "image_build_machine_type",
		"examples/storage-slurm.yaml": "controller_machine_type",
	} {
		if _, ok := byPath[path].Vars[key]; !ok {
			t.Errorf("%s: expected %s to remain in vars", path, key)
		}
	}
}

// TestRolePrefixedVarsInOfficialExamples pins the naming contract for the
// official examples: no legacy machine-type var names, and no bare shared
// inputs that applyGlobalVarsInModule would broadcast to every module.
func TestRolePrefixedVarsInOfficialExamples(t *testing.T) {
	_, blueprints := cachedScan(t)
	legacy := map[string]bool{
		"compute_node_machine_type":     true,
		"compute_nodepool_machine_type": true,
		"g4_machine_type":               true,
		"g4_enable_spot_vm":             true,
		"g4_reservation_name":           true,
		"g4_dws_flex_enabled":           true,
		"system_nodepool_machine_type":  true,
		"data_disk_size_gb":             true,
	}
	// Bare module inputs that must never appear unprefixed in examples/*.yaml
	// single-file templates (they would be injected into every module).
	bare := map[string]bool{"machine_type": true, "enable_spot_vm": true, "node_count_dynamic_max": true, "node_count_static": true, "cluster_size": true}
	// Blueprints whose bare disk_size_gb broadcast is documented as intentional.
	allowBareDisk := map[string]bool{"examples/ml-slurm.yaml": true}
	for _, bp := range blueprints {
		if !strings.HasPrefix(bp.Path, "examples/") {
			continue
		}
		singleFile := !strings.Contains(strings.TrimPrefix(bp.Path, "examples/"), "/")
		for k := range bp.Vars {
			if legacy[k] {
				t.Errorf("%s: legacy var %q must be renamed to the <role>_<input> convention", bp.Path, k)
			}
			if singleFile && bare[k] {
				t.Errorf("%s: bare var %q would be injected into every module; prefix it by role", bp.Path, k)
			}
			if singleFile && k == "disk_size_gb" && !allowBareDisk[bp.Path] {
				t.Errorf("%s: bare disk_size_gb broadcasts to login/controller; prefix it by role", bp.Path)
			}
		}
	}
}

var optionalOverrideLine = regexp.MustCompile(`^\s*#\s*([a-z][a-z0-9_]*):\s*(.*?)\s*$`)

// hardwareLockedPaths are blueprint path prefixes whose compute shape is tied
// to the blueprint's multi-NIC RDMA/GPUDirect wiring. They must hardcode
// machine_type in the module and never expose a compute machine-type var.
// gke-h4d is deliberately absent: both H4D shapes share the same RDMA wiring.
var hardwareLockedPaths = []string{
	"examples/gke-a3-",
	"examples/gke-a4/",
	"examples/gke-a4x",
	"examples/gke-consumption-options/dws-flex-start/gke-a3-",
	"examples/gke-consumption-options/dws-flex-start-queued-provisioning/gke-a3-",
	"examples/machine-learning/a3-",
	"examples/machine-learning/a4-",
	"examples/machine-learning/a4x-",
}

// TestHardwareLockedBlueprintsHideMachineType enforces Rule 4: hardware-locked
// blueprints advertise their shape in machine_types but offer no overridable
// compute machine-type var (control-plane vars are still allowed).
func TestHardwareLockedBlueprintsHideMachineType(t *testing.T) {
	_, blueprints := cachedScan(t)
	matched := 0
	for _, bp := range blueprints {
		locked := false
		for _, prefix := range hardwareLockedPaths {
			if strings.HasPrefix(bp.Path, prefix) {
				locked = true
			}
		}
		if !locked || !strings.HasSuffix(bp.Path, ".yaml") || strings.Contains(bp.Path, "build-service-images") {
			continue
		}
		matched++
		// Secondary CPU pools / debug partitions may keep their own
		// <role>_machine_type; only the accelerator pool's shape is locked.
		for _, k := range []string{"machine_type", "compute_machine_type", "compute_nodepool_machine_type"} {
			if _, ok := bp.Vars[k]; ok {
				t.Errorf("%s: hardware-locked blueprint must not expose %q; hardcode machine_type in the module", bp.Path, k)
			}
		}
		if len(bp.MachineTypes) == 0 {
			t.Errorf("%s: hardware-locked blueprint should still advertise its shape in machine_types", bp.Path)
		}
	}
	if matched < 10 {
		t.Fatalf("expected at least 10 hardware-locked blueprints, matched %d", matched)
	}
}

// TestOptionalOverrideCommentsMatchBlueprintDefaults verifies that every
// commented `# key: value` under the trailing "Optional overrides (defaults
// shown)" block of a companion *-deployment.yaml matches the paired
// blueprint's vars[key] default, so the comments cannot drift.
func TestOptionalOverrideCommentsMatchBlueprintDefaults(t *testing.T) {
	root, blueprints := cachedScan(t)
	checked := 0
	for _, bp := range blueprints {
		if bp.DeploymentPath == "" {
			continue
		}
		overrides, err := parseOptionalOverrides(filepath.Join(root, bp.DeploymentPath))
		if err != nil {
			t.Errorf("%s: %v", bp.DeploymentPath, err)
			continue
		}
		for key, commented := range overrides {
			checked++
			want, ok := bp.Vars[key]
			if !ok {
				t.Errorf("%s: optional override %q is not a var of %s", bp.DeploymentPath, key, bp.Path)
				continue
			}
			if !overrideValueEqual(commented, want) {
				t.Errorf("%s: optional override %s = %v, but %s vars.%s = %v", bp.DeploymentPath, key, commented, bp.Path, key, want)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("expected at least one Optional overrides comment to be checked")
	}
	t.Logf("checked %d optional override comments", checked)
}

// parseOptionalOverrides returns key -> YAML-decoded value for each commented
// `# key: value` line that follows the Optional overrides marker.
func parseOptionalOverrides(deployPath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(deployPath)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "Optional overrides (defaults shown)") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		m := optionalOverrideLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var v interface{}
		if err := yaml.Unmarshal([]byte(m[2]), &v); err != nil {
			return nil, fmt.Errorf("optional override %s: %w", m[1], err)
		}
		out[m[1]] = v
	}
	return out, nil
}

func overrideValueEqual(commented, want interface{}) bool {
	if reflect.DeepEqual(commented, want) {
		return true
	}
	// yaml.v3 decodes integers as int; the catalog stores int64/float64.
	return fmt.Sprint(commented) == fmt.Sprint(want)
}
