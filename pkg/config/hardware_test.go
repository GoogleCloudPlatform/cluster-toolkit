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

package config

import (
	"fmt"
	"slices"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestCalculateAcceleratorNodes(t *testing.T) {
	tests := []struct {
		name          string
		machineType   string
		topology      string
		expectedNodes int
		expectErr     bool
	}{
		{
			name:          "v4 standard 4 chips per VM",
			machineType:   "ct4p-hightpu-4t",
			topology:      "4x4x4",
			expectedNodes: 16, // 64 / 4
			expectErr:     false,
		},
		{
			name:          "v5p standard 4 chips per VM",
			machineType:   "ct5p-hightpu-4t",
			topology:      "4x4x4",
			expectedNodes: 16, // 64 / 4
			expectErr:     false,
		},
		{
			name:          "v5litepod 8 chips per VM",
			machineType:   "ct5lp-hightpu-8t",
			topology:      "8x16",
			expectedNodes: 16, // 128 / 8
			expectErr:     false,
		},
		{
			name:          "v5litepod string literal 8 chips per VM",
			machineType:   "v5litepod-16",
			topology:      "4x4",
			expectedNodes: 2, // 16 / 8
			expectErr:     false,
		},
		{
			name:          "v6e 4 chips per VM",
			machineType:   "ct6e-standard-4t",
			topology:      "2x2",
			expectedNodes: 1, // 4 / 4
			expectErr:     false,
		},
		{
			name:          "v7x 1 chip per VM test",
			machineType:   "tpu7x-standard-1t",
			topology:      "1x1x1",
			expectedNodes: 1, // 1 / 1
			expectErr:     false,
		},
		{
			name:          "v7x 4 chip per VM test",
			machineType:   "tpu7x-standard-4t",
			topology:      "4x4x4",
			expectedNodes: 16, // 64 / 4
			expectErr:     false,
		},
		{
			name:          "not divisible error",
			machineType:   "ct5p-hightpu-4t",
			topology:      "2x1x1", // 2 chips
			expectedNodes: 0,
			expectErr:     true,
		},
		{
			name:          "invalid topology format",
			machineType:   "ct5lp-hightpu-8t",
			topology:      "8x16xfoo",
			expectedNodes: 0,
			expectErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes, err := CalculateAcceleratorNodes(tc.machineType, tc.topology, 0)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if nodes != tc.expectedNodes {
					t.Errorf("expected %d nodes, got %d", tc.expectedNodes, nodes)
				}
			}
		})
	}
}

func TestCalculateAcceleratorNodes_WithExplicitCount(t *testing.T) {
	nodes, err := CalculateAcceleratorNodes("ct5p-hightpu-4t", "4x4x4", 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodes != 8 {
		t.Errorf("expected 8 nodes, got %d", nodes)
	}
}

func TestExtractTopology(t *testing.T) {
	bp := Blueprint{}

	mod1 := &Module{
		Settings: Dict{}.With("tpu_topology", cty.StringVal("4x4x4")),
	}
	if topo, ok := extractTopology(bp, mod1); !ok || topo != "4x4x4" {
		t.Errorf("expected 4x4x4, got %v (ok=%v)", topo, ok)
	}

	pp := cty.ObjectVal(map[string]cty.Value{"tpu_topology": cty.StringVal("2x2x2")})
	mod2 := &Module{
		Settings: Dict{}.With("placement_policy", pp),
	}
	if topo, ok := extractTopology(bp, mod2); !ok || topo != "2x2x2" {
		t.Errorf("expected 2x2x2, got %v (ok=%v)", topo, ok)
	}

	mod3 := &Module{
		Settings: Dict{},
	}
	if topo, ok := extractTopology(bp, mod3); ok {
		t.Errorf("expected false, got %v", topo)
	}
}

func TestInjectCompactPlacementPolicy(t *testing.T) {
	mod1 := &Module{
		Settings: Dict{},
	}
	injectCompactPlacementPolicy(mod1, "4x4x4")
	if !mod1.Settings.Has("placement_policy") {
		t.Fatal("expected placement_policy to be injected")
	}
	pp1 := mod1.Settings.Get("placement_policy").AsValueMap()
	if pp1["type"].AsString() != "COMPACT" {
		t.Errorf("expected pp type COMPACT, got %v", pp1["type"])
	}
	if pp1["tpu_topology"].AsString() != "4x4x4" {
		t.Errorf("expected pp topology 4x4x4, got %v", pp1["tpu_topology"])
	}

	ppOrig := cty.ObjectVal(map[string]cty.Value{"foo": cty.StringVal("bar")})
	mod2 := &Module{
		Settings: Dict{}.With("placement_policy", ppOrig),
	}
	injectCompactPlacementPolicy(mod2, "2x2x2")
	pp2 := mod2.Settings.Get("placement_policy").AsValueMap()
	if pp2["type"].AsString() != "COMPACT" || pp2["tpu_topology"].AsString() != "2x2x2" || pp2["foo"].AsString() != "bar" {
		t.Errorf("mod2 placement policy incorrect: %v", pp2)
	}
}

func TestExpandHardwareSettings(t *testing.T) {
	bp := Blueprint{}

	mod1 := &Module{
		Settings: Dict{}.
			With("static_node_count", cty.NumberIntVal(10)).
			With("machine_type", cty.StringVal("ct6e-standard-4t")).
			With("tpu_topology", cty.StringVal("2x2")),
	}
	err := expandHardwareSettings(bp, mod1)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	count, _ := mod1.Settings.Get("static_node_count").AsBigFloat().Int64()
	if count != 10 {
		t.Errorf("expected static_node_count 10, got %d", count)
	}

	mod2 := &Module{
		Settings: Dict{}.
			With("machine_type", cty.StringVal("ct6e-standard-4t")).
			With("tpu_topology", cty.StringVal("2x2")),
	}
	err = expandHardwareSettings(bp, mod2)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	count2, _ := mod2.Settings.Get("static_node_count").AsBigFloat().Int64()
	if count2 != 1 {
		t.Errorf("expected static_node_count 1, got %d", count2)
	}

	mod3 := &Module{
		Settings: Dict{}.
			With("machine_type", cty.StringVal("ct5lp-hightpu-8t")).
			With("tpu_topology", cty.StringVal("8x16")),
	}
	err = expandHardwareSettings(bp, mod3)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	count3, _ := mod3.Settings.Get("static_node_count").AsBigFloat().Int64()
	if count3 != 16 {
		t.Errorf("expected static_node_count 16, got %d", count3)
	}
	if !mod3.Settings.Has("placement_policy") {
		t.Errorf("expected placement_policy to be injected for multi-node setups")
	}
	// Test that it skips non-TPU machine types
	mod4 := &Module{
		Settings: Dict{}.
			With("machine_type", cty.StringVal("n2-standard-2")).
			With("tpu_topology", cty.StringVal("2x2")),
	}
	err = expandHardwareSettings(bp, mod4)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if mod4.Settings.Has("static_node_count") {
		t.Errorf("expected static_node_count NOT to be set for non-TPU machine type")
	}
}

func TestResolveTopologyForChips(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		totalChips int
		wantShape  string
		wantErr    bool
	}{
		{
			name:       "v4 8 cores (4 chips)",
			prefix:     "v4",
			totalChips: 8,
			wantShape:  "2x2x1",
			wantErr:    false,
		},
		{
			name:       "tpu7x 2048 chips",
			prefix:     "tpu7x-4",
			totalChips: 2048,
			wantShape:  "8x16x16",
			wantErr:    false,
		},
		{
			name:       "v6e 1 chip",
			prefix:     "v6e",
			totalChips: 1,
			wantShape:  "1x1",
			wantErr:    false,
		},
		{
			name:       "v6e 256 chips",
			prefix:     "v6e",
			totalChips: 256,
			wantShape:  "16x16",
			wantErr:    false,
		},
		{
			name:       "tpu7x 1 chip (Fail)",
			prefix:     "tpu7x-4",
			totalChips: 1,
			wantShape:  "",
			wantErr:    true,
		},
		{
			name:       "v4 3 chips (Fail)",
			prefix:     "v4",
			totalChips: 3,
			wantShape:  "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTopologyForChips(fmt.Sprintf("%s-%d", tt.prefix, tt.totalChips))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveTopologyForChips() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantShape {
				t.Errorf("ResolveTopologyForChips() got = %v, want %v", got, tt.wantShape)
			}
		})
	}
}

func TestIsTPU(t *testing.T) {
	tests := []struct {
		accelType string
		want      bool
	}{
		{"v4-8", true},
		{"v5e-8", true},
		{"v6e-8", true},
		{"tpu7x-1", true},
		{"ct4p-hightpu-4t", true},
		{"ct5lp-hightpu-8t", true},
		{"v5litepod-16", true},
		{"l4-1", false},
		{"nvidia-tesla-a100", false},
		{"g2-standard-12", false},
		{"n2-standard-2", false},
		{"v1-standard-2", false},
		{"v5x", false},
	}
	for _, tt := range tests {
		if got := IsTPU(tt.accelType); got != tt.want {
			t.Errorf("IsTPU(%q) = %v, want %v", tt.accelType, got, tt.want)
		}
	}
}

func TestMatchesTPUFamily(t *testing.T) {
	tests := []struct {
		name      string
		accelType string
		families  []string
		want      bool
	}{
		{"v6e matches 2D", "v6e-8", valid2DTPUFamilies, true},
		{"v5e matches 2D", "v5e-8", valid2DTPUFamilies, true},
		{"v5litepod matches 2D", "v5litepod-16", valid2DTPUFamilies, true},
		{"l4 does not match 2D", "l4-1", valid2DTPUFamilies, false},
		{"v4 matches 3D", "v4-8", valid3DTPUFamilies, true},
		{"v5p matches 3D", "v5p-4", valid3DTPUFamilies, true},
		{"v6e does not match 3D", "v6e-8", valid3DTPUFamilies, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesTPUFamily(tt.accelType, tt.families); got != tt.want {
				t.Errorf("matchesTPUFamily(%q, %v) = %v, want %v", tt.accelType, tt.families, got, tt.want)
			}
		})
	}
}

func TestResolveMachineType(t *testing.T) {
	tests := []struct {
		accelType string
		want      string
	}{
		{"v4-8", "ct4p-hightpu-4t"},
		{"v5e-8", "ct5lp-hightpu-8t"},
		{"l4-1", "g2-standard-12"},
		{"unknown", "unknown"},
		{"ct4p-hightpu-4t", "ct4p-hightpu-4t"},
	}
	for _, tt := range tests {
		if got := ResolveMachineType(tt.accelType); got != tt.want {
			t.Errorf("ResolveMachineType(%q) = %q, want %q", tt.accelType, got, tt.want)
		}
	}
}

func TestGetCandidatesForShorthand(t *testing.T) {
	tests := []struct {
		shorthand string
		want      []string
	}{
		{"v5e", []string{"ct5lp-hightpu-1t", "ct5lp-hightpu-4t", "ct5lp-hightpu-8t"}},
		{"v4", []string{"ct4p-hightpu-4t"}},
		{"l4", []string{"g2-standard-12", "g2-standard-24", "g2-standard-48", "g2-standard-96"}},
		{"unknown", nil},
	}
	for _, tt := range tests {
		got := GetCandidatesForShorthand(tt.shorthand)
		// Sort for comparison because map iteration order is random
		slices.Sort(got)
		slices.Sort(tt.want)
		if !slices.Equal(got, tt.want) {
			t.Errorf("GetCandidatesForShorthand(%q) = %v, want %v", tt.shorthand, got, tt.want)
		}
	}
}

func TestValidateHardwareRequest(t *testing.T) {
	tests := []struct {
		name            string
		machineType     string
		topology        string
		placementPolicy string
		wantErr         bool
	}{
		{"Valid TPU v4", "v4-8", "2x2x1", "", false},
		{"Valid TPU v6e", "v6e-8", "2x2", "", false},
		{"Invalid TPU v6e shape", "v6e-8", "3x3", "", true},
		{"Valid TPU v5litepod", "v5litepod-16", "4x4", "", false},
		{"Invalid TPU v5litepod shape", "v5litepod-16", "3x3", "", true},
		{"Invalid TPU v4 dimensions", "v4-8", "2x2", "", true}, // Needs 3D
		{"Invalid TPU v4 shape", "v4-8", "3x3x3", "", true},
		{"Unknown TPU family fails", "tpu-v7-8", "2x2x2", "", true},
		{"Non-TPU skips validation", "l4-1", "invalid", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHardwareRequest(tt.machineType, tt.topology, tt.placementPolicy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHardwareRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckTopologyContainment(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		container string
		accelType string
		wantFit   bool
		wantErr   bool
	}{
		{
			name:      "Perfect fit",
			requested: "2x2",
			container: "2x2",
			accelType: "v6e",
			wantFit:   true,
			wantErr:   false,
		},
		{
			name:      "Fits inside",
			requested: "2x2",
			container: "4x4",
			accelType: "v6e",
			wantFit:   true,
			wantErr:   false,
		},
		{
			name:      "Doesn't fit (too large)",
			requested: "4x4",
			container: "2x2",
			accelType: "v6e",
			wantFit:   false,
			wantErr:   false,
		},
		{
			name:      "Different dimensions",
			requested: "2x2x1",
			container: "4x4",
			accelType: "v6e",
			wantFit:   false,
			wantErr:   false,
		},
		{
			name:      "Invalid requested topology (NaN)",
			requested: "2xfoo",
			container: "4x4",
			accelType: "v6e",
			wantFit:   false,
			wantErr:   true,
		},
		{
			name:      "Invalid container topology (NaN)",
			requested: "2x2",
			container: "4xbar",
			accelType: "v6e",
			wantFit:   false,
			wantErr:   true,
		},
		{
			name:      "V6e invalid subset shape",
			requested: "3x3",
			container: "4x4",
			accelType: "v6e",
			wantFit:   false,
			wantErr:   true,
		},
		{
			name:      "V4 invalid subset shape",
			requested: "3x3x3",
			container: "4x4x4",
			accelType: "v4",
			wantFit:   false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckTopologyContainment(tt.requested, tt.container, tt.accelType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckTopologyContainment() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantFit {
				t.Errorf("CheckTopologyContainment() got = %v, want %v", got, tt.wantFit)
			}
		})
	}
}
