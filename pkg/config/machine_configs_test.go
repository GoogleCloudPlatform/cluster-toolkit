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

package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/zclconf/go-cty/cty"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

func TestParseTPUCount(t *testing.T) {
	tests := []struct {
		machineType string
		want        int
	}{
		{"n1-standard-1", 0},
		{"ct5p-hbm-2t", 2},
		{"tpu-v4-podslice", 0},
		{"ct4p-hbm-4t", 4},
		{"", 0},
		{"invalid-t", 0},
	}

	for _, tt := range tests {
		t.Run(tt.machineType, func(t *testing.T) {
			if got := ParseTPUCount(tt.machineType); got != tt.want {
				t.Errorf("parseTPUCount(%q) = %v, want %v", tt.machineType, got, tt.want)
			}
		})
	}
}

func TestExtractStringSetting(t *testing.T) {
	bp := Blueprint{}
	m := Module{
		Settings: NewDict(map[string]cty.Value{
			"test_key": cty.StringVal("test_value"),
		}),
	}

	if got := extractStringSetting(&m, bp, "test_key"); got != "test_value" {
		t.Errorf("extractStringSetting() = %v, want %v", got, "test_value")
	}

	if got := extractStringSetting(&m, bp, "non_existent"); got != "" {
		t.Errorf("extractStringSetting() = %v, want %v", got, "")
	}
}

func TestExtractZone(t *testing.T) {
	// Case 1: Zone in module settings
	bp := Blueprint{}
	m1 := Module{
		Settings: NewDict(map[string]cty.Value{
			"zone": cty.StringVal("us-central1-a"),
		}),
	}
	if got := extractZone(&m1, bp); got != "us-central1-a" {
		t.Errorf("extractZone (module setting) = %v, want %v", got, "us-central1-a")
	}

	// Case 2: Zone in bp vars
	bp2 := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"zone": cty.StringVal("us-central1-b"),
		}),
	}
	m2 := Module{}
	if got := extractZone(&m2, bp2); got != "us-central1-b" {
		t.Errorf("extractZone (bp var) = %v, want %v", got, "us-central1-b")
	}

	// Case 3: Zones (list) in bp vars
	bp3 := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"zones": cty.ListVal([]cty.Value{cty.StringVal("us-central1-c")}),
		}),
	}
	m3 := Module{}
	if got := extractZone(&m3, bp3); got != "us-central1-c" {
		t.Errorf("extractZone (bp vars list) = %v, want %v", got, "us-central1-c")
	}
}

func TestExtractProject(t *testing.T) {
	bp := Blueprint{}
	m1 := Module{
		Settings: NewDict(map[string]cty.Value{
			"project_id": cty.StringVal("my-project"),
		}),
	}
	if got := extractProject(&m1, bp); got != "my-project" {
		t.Errorf("extractProject (module setting) = %v, want %v", got, "my-project")
	}

	bp2 := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"project_id": cty.StringVal("my-project-bp"),
		}),
	}
	m2 := Module{}
	if got := extractProject(&m2, bp2); got != "my-project-bp" {
		t.Errorf("extractProject (bp var) = %v, want %v", got, "my-project-bp")
	}
}

func TestBuildOutputConfigStruct_CPU(t *testing.T) {
	mt1 := &compute.MachineType{GuestCpus: 4}
	out1 := buildOutputConfigStruct("n1-standard-4", mt1)
	if out1.CPUs["n1-standard-4"].Count != 4 {
		t.Errorf("expected 4 CPUs, got %v", out1.CPUs["n1-standard-4"].Count)
	}
}

func TestBuildOutputConfigStruct_GPU(t *testing.T) {
	mt2 := &compute.MachineType{
		GuestCpus: 8,
		Accelerators: []*compute.MachineTypeAccelerators{
			{GuestAcceleratorCount: 2, GuestAcceleratorType: "nvidia-tesla-t4"},
		},
	}
	out2 := buildOutputConfigStruct("n1-standard-8", mt2)
	if out2.GPUs["n1-standard-8"].Count != 2 || out2.GPUs["n1-standard-8"].Type != "nvidia-tesla-t4" {
		t.Errorf("expected 2 nvidia-tesla-t4 GPUs, got %+v", out2.GPUs["n1-standard-8"])
	}
}

func TestBuildOutputConfigStruct_TPU(t *testing.T) {
	mt3 := &compute.MachineType{
		GuestCpus: 8,
		Accelerators: []*compute.MachineTypeAccelerators{
			{GuestAcceleratorCount: 4, GuestAcceleratorType: "tpu-v4"},
		},
	}
	out3 := buildOutputConfigStruct("tpu-v4-8", mt3)
	if out3.TPUs["tpu-v4-8"].Count != 4 {
		t.Errorf("expected 4 TPUs, got %v", out3.TPUs["tpu-v4-8"].Count)
	}
}

func TestBuildOutputConfigStruct_TPUFallback(t *testing.T) {
	mt4 := &compute.MachineType{GuestCpus: 4}
	out4 := buildOutputConfigStruct("ct5p-hbm-2t", mt4)
	if out4.TPUs["ct5p-hbm-2t"].Count != 2 {
		t.Errorf("expected 2 TPUs from ct5p-hbm-2t fallback, got %v", out4.TPUs["ct5p-hbm-2t"].Count)
	}
}

func TestGetMachineConfigJSON(t *testing.T) {
	// Test mock data path
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"mocked": true}`)

	bp := Blueprint{}
	m := Module{}

	got, err := getMachineConfigJSON(&m, bp)
	if err != nil {
		t.Fatalf("getMachineConfigJSON failed: %v", err)
	}
	if got != `{"mocked": true}` {
		t.Errorf("expected mocked data, got %v", got)
	}
}

func TestGetMachineConfigJSON_EmptyParams(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", "")
	bp := Blueprint{}
	m := Module{}

	got, err := getMachineConfigJSON(&m, bp)
	if err != nil {
		t.Fatalf("getMachineConfigJSON failed: %v", err)
	}

	var out OutputConfig
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if len(out.CPUs) != 0 || len(out.GPUs) != 0 || len(out.TPUs) != 0 {
		t.Errorf("expected empty config, got %+v", out)
	}
}

func TestExtractFallbackMachineTypes(t *testing.T) {
	sel := func(attrs map[string]cty.Value) cty.Value { return cty.ObjectVal(attrs) }
	policy := func(sels ...cty.Value) cty.Value {
		return cty.ObjectVal(map[string]cty.Value{"instance_selections": cty.TupleVal(sels)})
	}

	tests := []struct {
		name     string
		settings map[string]cty.Value
		want     []string
	}{
		{"setting absent", map[string]cty.Value{}, nil},
		{"policy null", map[string]cty.Value{"instance_flexibility_policy": cty.NullVal(cty.DynamicPseudoType)}, nil},
		{"policy not an object", map[string]cty.Value{"instance_flexibility_policy": cty.StringVal("n2-standard-8")}, nil},
		{"selections missing", map[string]cty.Value{"instance_flexibility_policy": cty.EmptyObjectVal}, nil},
		{
			"tuple, list and set machine_types across selections",
			map[string]cty.Value{"instance_flexibility_policy": policy(
				sel(map[string]cty.Value{"rank": cty.NumberIntVal(1), "machine_types": cty.TupleVal([]cty.Value{cty.StringVal("n2d-standard-16")})}),
				sel(map[string]cty.Value{"machine_types": cty.ListVal([]cty.Value{cty.StringVal("c3-standard-22")})}),
				sel(map[string]cty.Value{"machine_types": cty.SetVal([]cty.Value{cty.StringVal("n2-standard-32")})}),
			)},
			[]string{"n2d-standard-16", "c3-standard-22", "n2-standard-32"},
		},
		{
			"selection without machine_types and non-string / unknown elements are skipped",
			map[string]cty.Value{"instance_flexibility_policy": policy(
				sel(map[string]cty.Value{"name": cty.StringVal("no-types")}),
				sel(map[string]cty.Value{"machine_types": cty.TupleVal([]cty.Value{
					cty.NumberIntVal(1), cty.UnknownVal(cty.String), cty.StringVal("n2d-standard-8"),
				})}),
				sel(map[string]cty.Value{"machine_types": cty.StringVal("not-a-collection")}),
			)},
			[]string{"n2d-standard-8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Module{Settings: NewDict(tt.settings)}
			got := extractFallbackMachineTypes(&m, Blueprint{})
			if len(got) != len(tt.want) {
				t.Fatalf("extractFallbackMachineTypes() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractFallbackMachineTypes()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAddMachineTypeToOutputConfig_MergesFallback(t *testing.T) {
	out := buildOutputConfigStruct("g2-standard-24", &compute.MachineType{
		GuestCpus: 24, MemoryMb: 98304,
		Accelerators: []*compute.MachineTypeAccelerators{{GuestAcceleratorCount: 2, GuestAcceleratorType: "nvidia-l4"}},
	})
	addMachineTypeToOutputConfig(&out, "g2-standard-48", &compute.MachineType{
		GuestCpus: 48, MemoryMb: 196608,
		Accelerators: []*compute.MachineTypeAccelerators{{GuestAcceleratorCount: 4, GuestAcceleratorType: "nvidia-l4"}},
	})

	if out.CPUs["g2-standard-24"].Count != 24 || out.CPUs["g2-standard-48"].Count != 48 {
		t.Errorf("expected CPU entries for primary and fallback, got %+v", out.CPUs)
	}
	if out.GPUs["g2-standard-24"].Count != 2 || out.GPUs["g2-standard-48"].Count != 4 {
		t.Errorf("expected GPU entries for primary and fallback, got %+v", out.GPUs)
	}
}

func TestGetOutputConfig_FallbackLookupErrors(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", "")
	apiErr := func(code int) error {
		// Mirror GetMachineType, which wraps the API error with %w.
		return fmt.Errorf("failed to get machine type info: %w", &googleapi.Error{Code: code})
	}
	mod := func() Module {
		return Module{Settings: NewDict(map[string]cty.Value{
			"machine_type": cty.StringVal("n2-standard-16"),
			"zone":         cty.StringVal("us-central1-a"),
			"project_id":   cty.StringVal("p"),
			"instance_flexibility_policy": cty.ObjectVal(map[string]cty.Value{
				"instance_selections": cty.TupleVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"machine_types": cty.TupleVal([]cty.Value{cty.StringVal("n2d-standard-16")}),
				})}),
			}),
		})}
	}

	tests := []struct {
		name       string
		fallbackEr error
		wantErr    bool
	}{
		{"fallback not in primary zone (404) is skipped", apiErr(http.StatusNotFound), false},
		{"fallback permission denied (403) fails", apiErr(http.StatusForbidden), true},
		{"fallback rate limited (429) fails", apiErr(http.StatusTooManyRequests), true},
		{"fallback non-API error fails", fmt.Errorf("dial tcp: i/o timeout"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := getMachineType
			t.Cleanup(func() { getMachineType = orig })
			getMachineType = func(_, _, mt string) (*compute.MachineType, error) {
				if mt == "n2-standard-16" {
					return &compute.MachineType{GuestCpus: 16, MemoryMb: 65536}, nil
				}
				return nil, tt.fallbackEr
			}

			m := mod()
			out, err := GetOutputConfig(&m, Blueprint{})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got config %+v", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, ok := out.CPUs["n2-standard-16"]; !ok {
				t.Errorf("primary machine type missing from %+v", out.CPUs)
			}
			if _, ok := out.CPUs["n2d-standard-16"]; ok {
				t.Errorf("skipped fallback should not be present in %+v", out.CPUs)
			}
		})
	}

	// Primary machine type 404 must still fail hard.
	orig := getMachineType
	t.Cleanup(func() { getMachineType = orig })
	getMachineType = func(_, _, _ string) (*compute.MachineType, error) { return nil, apiErr(http.StatusNotFound) }
	m := mod()
	if _, err := GetOutputConfig(&m, Blueprint{}); err == nil {
		t.Error("expected primary machine type 404 to fail")
	}
}
