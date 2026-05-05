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
			nodes, err := calculateAcceleratorNodes(tc.machineType, tc.topology)
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
