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
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

const defaultAcceleratorsPerVM = 4

var tpuFamilyDefaults = map[string]int{
	"ct5lp":     8, // Default for TPU v5e when suffix is missing
	"v5litepod": 8, // Legacy string literal default
}

func evalString(bp Blueprint, val cty.Value) (string, bool) {
	ev, err := bp.Eval(val)
	if err == nil && ev.Type() == cty.String && !ev.IsNull() && ev.IsKnown() {
		return ev.AsString(), true
	}
	return "", false
}

func extractTopology(bp Blueprint, mod *Module) (string, bool) {
	if mod.Settings.Has("tpu_topology") {
		if str, ok := evalString(bp, mod.Settings.Get("tpu_topology")); ok {
			return str, true
		}
	}
	if mod.Settings.Has("placement_policy") {
		ppVal, err := bp.Eval(mod.Settings.Get("placement_policy"))
		if err == nil && ppVal.Type().IsObjectType() && ppVal.IsKnown() {
			if ppVal.Type().HasAttribute("tpu_topology") {
				topoVal := ppVal.GetAttr("tpu_topology")
				if topoVal.Type() == cty.String && !topoVal.IsNull() && topoVal.IsKnown() {
					return topoVal.AsString(), true
				}
			}
		}
	}
	if topo, ok := extractTopologyFromWorkloadPolicy(bp, mod); ok {
		return topo, true
	}

	return "", false
}

func extractTopologyFromWorkloadPolicy(bp Blueprint, mod *Module) (string, bool) {
	for _, u := range mod.Use {
		usedMod, err := bp.Module(u)
		if err != nil {
			continue
		}
		if usedMod.Settings.Has("workload_policy") {
			wpVal, err := bp.Eval(usedMod.Settings.Get("workload_policy"))
			if err == nil && wpVal.Type().IsObjectType() && wpVal.IsKnown() {
				if wpVal.Type().HasAttribute("accelerator_topology") {
					topoVal := wpVal.GetAttr("accelerator_topology")
					if topoVal.Type() == cty.String && !topoVal.IsNull() && topoVal.IsKnown() {
						return topoVal.AsString(), true
					}
				}
			}
		}
	}
	return "", false
}

func injectCompactPlacementPolicy(mod *Module, tpuTopologyStr string) {
	var ppMap map[string]cty.Value
	if mod.Settings.Has("placement_policy") {
		ppRaw := mod.Settings.Get("placement_policy")
		if ppRaw.Type().IsObjectType() {
			ppMap = ppRaw.AsValueMap()
		} else {
			ppMap = make(map[string]cty.Value)
		}
	} else {
		ppMap = make(map[string]cty.Value)
	}
	ppMap["type"] = cty.StringVal("COMPACT")
	if tpuTopologyStr != "" {
		ppMap["tpu_topology"] = cty.StringVal(tpuTopologyStr)
	}
	mod.Settings = mod.Settings.With("placement_policy", cty.ObjectVal(ppMap))
}

// expandHardwareSettings automatically infers missing hardware settings
// such as static_node_count for TPUs based on machine_type and tpu_topology.
func expandHardwareSettings(bp Blueprint, mod *Module) error {
	// Only auto-calculate if static_node_count is missing.
	if mod.Settings.Has("static_node_count") {
		return nil
	}

	tpuTopologyStr, hasTopology := extractTopology(bp, mod)
	if !hasTopology || !mod.Settings.Has("machine_type") {
		return nil
	}

	mtStr, ok := evalString(bp, mod.Settings.Get("machine_type"))
	if !ok {
		return nil
	}

	if !isTPUMachineType(mtStr) {
		return nil
	}

	nodes, err := calculateAcceleratorNodes(mtStr, tpuTopologyStr)
	if err != nil {
		return fmt.Errorf("failed to automatically calculate static_node_count for module %q: %w", mod.ID, err)
	}

	mod.Settings = mod.Settings.With("static_node_count", cty.NumberIntVal(int64(nodes)))

	if nodes > 1 {
		injectCompactPlacementPolicy(mod, tpuTopologyStr)
	}

	return nil
}

// calculateAcceleratorNodes derives the node count from topology and machine type.
func calculateAcceleratorNodes(machineType, topology string) (int, error) {
	// 1. Calculate Total Accelerators from topology
	dims := strings.Split(topology, "x")
	totalAccelerators := 1
	for _, dim := range dims {
		val, err := strconv.Atoi(dim)
		if err != nil {
			return 0, fmt.Errorf("invalid tpu_topology format %q: %w", topology, err)
		}
		totalAccelerators *= val
	}

	// 2. Identify Accelerators per VM from machine_type
	acceleratorsPerVM := defaultAcceleratorsPerVM // Default for most TPU families (e.g., v4, v5p, v6e)

	// Explicitly check if the machine_type defines chips per VM, e.g., "-1t", "-4t", "-8t"
	hasExplicitAccelerators := false
	if idx := strings.LastIndex(machineType, "-"); idx != -1 && strings.HasSuffix(machineType, "t") {
		if val, err := strconv.Atoi(machineType[idx+1 : len(machineType)-1]); err == nil {
			acceleratorsPerVM = val
			hasExplicitAccelerators = true
		}
	}

	// Fallback to known machine family defaults if no explicit "-Nt" suffix
	if !hasExplicitAccelerators {
		for family, defaultAccs := range tpuFamilyDefaults {
			if strings.Contains(machineType, family) {
				acceleratorsPerVM = defaultAccs
				break
			}
		}
	}

	// 3. Calculate Nodes
	if totalAccelerators%acceleratorsPerVM != 0 {
		return 0, fmt.Errorf("topology %q (%d accelerators) is not divisible by machine_type %q capacity (%d accelerators/VM). "+
			"We assumed a default of %d accelerators/VM; if this is incorrect for a new machine type, please report a bug to the toolkit maintainers.",
			topology, totalAccelerators, machineType, acceleratorsPerVM, acceleratorsPerVM)
	}

	return totalAccelerators / acceleratorsPerVM, nil
}

func isTPUMachineType(machineType string) bool {
	return parseTPUCount(machineType) > 0
}
