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
	"regexp"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

const defaultAcceleratorsPerVM = 4

var tpuFamilyDefaults = map[string]int{
	"ct5lp":     8, // Default for TPU v5e when suffix is missing
	"v5litepod": 8, // Legacy string literal default
}

var tpuRegex = regexp.MustCompile(`^v[4-6][ep]?(-\d+)?$`)

var valid2DTPUFamilies = []string{"ct6e", "ct5lp", "v5litepod", "v5-lite", "v5e", "v6e"}
var valid3DTPUFamilies = []string{"v4", "v5p", "ct4p", "ct5p", "tpu7"}

// AcceleratorShorthandMap maps shorthand names to full machine types.
var AcceleratorShorthandMap = map[string]string{
	// GPU mappings
	"l4-1":             "g2-standard-12",
	"l4-2":             "g2-standard-24",
	"l4-4":             "g2-standard-48",
	"l4-8":             "g2-standard-96",
	"rtx-6000-1":       "g4-standard-48",
	"rtx-6000-2":       "g4-standard-96",
	"rtx-6000-4":       "g4-standard-192",
	"rtx-6000-8":       "g4-standard-384",
	"a100-40gb-1":      "a2-highgpu-1g",
	"a100-40gb-2":      "a2-highgpu-2g",
	"a100-40gb-4":      "a2-highgpu-4g",
	"a100-40gb-8":      "a2-highgpu-8g",
	"a2-megagpu-16g":   "a2-megagpu-16g",
	"a100-80gb-1":      "a2-ultragpu-1g",
	"a100-80gb-2":      "a2-ultragpu-2g",
	"a100-80gb-4":      "a2-ultragpu-4g",
	"a100-80gb-8":      "a2-ultragpu-8g",
	"h100-80gb-1":      "a3-highgpu-1g",
	"h100-80gb-2":      "a3-highgpu-2g",
	"h100-80gb-4":      "a3-highgpu-4g",
	"h100-80gb-8":      "a3-highgpu-8g",
	"h100-mega-80gb-8": "a3-megagpu-8g",
	"h200-141gb-8":     "a3-ultragpu-8g",
	"b200-8":           "a4-highgpu-8g",
	"gb200-4":          "a4x-highgpu-4g",

	// TPU mappings
	"v4-8":    "ct4p-hightpu-4t",
	"v5p-1":   "ct5p-hightpu-1t",
	"v5p-2":   "ct5p-hightpu-2t",
	"v5p-4":   "ct5p-hightpu-4t",
	"v5e-1":   "ct5lp-hightpu-1t",
	"v5e-4":   "ct5lp-hightpu-4t",
	"v5e-8":   "ct5lp-hightpu-8t",
	"v6e-1":   "ct6e-standard-1t",
	"v6e-4":   "ct6e-standard-4t",
	"v6e-8":   "ct6e-standard-8t",
	"tpu7x-1": "tpu7x-standard-1t",
	"tpu7x-4": "tpu7x-standard-4t",
}

// ValidGPUAccelerators lists valid GPU accelerator types.
var ValidGPUAccelerators = map[string]bool{
	"nvidia-l4":             true,
	"nvidia-tesla-a100":     true,
	"nvidia-gb200":          true,
	"nvidia-b200":           true,
	"nvidia-h200-141gb":     true,
	"nvidia-h100-80gb":      true,
	"nvidia-h100-mega-80gb": true,
}

// 3D topologies for v4, v5p, tpu7x
var allowed3DTopologies = map[int]string{
	4:    "2x2x1",
	8:    "2x2x2",
	16:   "2x2x4",
	32:   "2x4x4",
	64:   "4x4x4",
	128:  "4x4x8",
	256:  "4x8x8",
	512:  "8x8x8",
	1024: "8x8x16",
	2048: "8x16x16",
}

// 2D topologies for v5e and v6e
var allowed2DTopologies = map[int]string{
	1:   "1x1",
	4:   "2x2",
	8:   "2x4",
	16:  "4x4",
	32:  "4x8",
	64:  "8x8",
	128: "8x16",
	256: "16x16",
}

var valid3DShapes = make(map[string]bool)
var valid2DShapes = make(map[string]bool)

func init() {
	for _, v := range allowed3DTopologies {
		valid3DShapes[v] = true
	}
	for _, v := range allowed2DTopologies {
		valid2DShapes[v] = true
	}
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

	nodes, err := CalculateAcceleratorNodes(mtStr, tpuTopologyStr)
	if err != nil {
		return fmt.Errorf("failed to automatically calculate static_node_count for module %q: %w", mod.ID, err)
	}

	mod.Settings = mod.Settings.With("static_node_count", cty.NumberIntVal(int64(nodes)))

	if nodes > 1 {
		injectCompactPlacementPolicy(mod, tpuTopologyStr)
	}

	return nil
}

// CalculateAcceleratorNodes derives the node count from topology and machine type.
func CalculateAcceleratorNodes(machineType, topology string) (int, error) {
	// 1. Calculate Total Accelerators from topology
	totalAccelerators := 1
	for _, dim := range strings.Split(topology, "x") {
		val, err := strconv.Atoi(dim)
		if err != nil {
			return 0, fmt.Errorf("invalid tpu_topology format %q: %w", topology, err)
		}
		totalAccelerators *= val
	}

	// 2. Identify Accelerators per VM from machine_type
	acceleratorsPerVM := func() int {
		// Check for explicit accelerators count in machine_type (e.g., "-4t")
		if idx := strings.LastIndex(machineType, "-"); idx != -1 && strings.HasSuffix(machineType, "t") {
			if val, err := strconv.Atoi(machineType[idx+1 : len(machineType)-1]); err == nil && val > 0 {
				return val
			}
		}
		// Fallback to known machine family defaults
		for family, defaultAccs := range tpuFamilyDefaults {
			if strings.Contains(machineType, family) {
				return defaultAccs
			}
		}
		// Final fallback to global default
		return defaultAcceleratorsPerVM
	}()

	// 3. Calculate Nodes
	if totalAccelerators%acceleratorsPerVM != 0 {
		return 0, fmt.Errorf("topology %q (%d accelerators) is not divisible by machine_type %q capacity (%d accelerators/VM). "+
			"We assumed a default of %d accelerators/VM; if this is incorrect for a new machine type, please report a bug to the toolkit maintainers.",
			topology, totalAccelerators, machineType, acceleratorsPerVM, acceleratorsPerVM)
	}

	return totalAccelerators / acceleratorsPerVM, nil
}

// ResolveMachineType returns the full machine type for a given accelerator shorthand.
// If not found in map, it returns the input string.
func ResolveMachineType(acceleratorType string) string {
	if machineType, exists := AcceleratorShorthandMap[strings.ToLower(acceleratorType)]; exists {
		return machineType
	}
	return acceleratorType
}

// matchesTPUFamily returns true if the accelerator type matches any of the given families.
func matchesTPUFamily(acceleratorType string, families []string) bool {
	resolved := ResolveMachineType(acceleratorType)
	resolvedLower := strings.ToLower(resolved)
	for _, f := range families {
		if strings.Contains(resolvedLower, f) {
			return true
		}
	}
	return false
}

// IsTPU returns true if the accelerator type is a TPU.
// It first resolves shorthand to machine type, then checks for 'ct' or 'tpu' prefixes.
func IsTPU(acceleratorType string) bool {
	resolved := ResolveMachineType(acceleratorType)
	resolvedLower := strings.ToLower(resolved)

	for k := range tpuFamilyDefaults {
		if strings.HasPrefix(resolvedLower, k) {
			return true
		}
	}

	if strings.HasPrefix(resolvedLower, "ct") || strings.Contains(resolvedLower, "tpu") {
		return true
	}
	// Fallback for shorthands not in map that match known TPU versions (v4, v5e, v5p, v6e).
	// We use a strict regex to avoid matching arbitrary machine types starting with 'v'.
	return tpuRegex.MatchString(resolvedLower)
}

// GetCandidatesForShorthand returns all full machine types that match the given shorthand prefix.
func GetCandidatesForShorthand(shorthand string) []string {
	var candidates []string
	shorthandLower := strings.ToLower(shorthand)
	for k, v := range AcceleratorShorthandMap {
		if strings.HasPrefix(strings.ToLower(k), shorthandLower) {
			candidates = append(candidates, v)
		}
	}
	return candidates
}

// ResolveTopologyForChips returns the default topology for a given accelerator and total chips.
func ResolveTopologyForChips(accelaratorType string) (string, error) {
	idx := strings.LastIndex(accelaratorType, "-")
	if idx == -1 {
		return "", fmt.Errorf("invalid accelerator type format for topology resolution: %s (expected prefix-suffix)", accelaratorType)
	}
	accelType := accelaratorType[:idx]
	suffix := accelaratorType[idx+1:]
	totalChips, err := strconv.Atoi(suffix)
	if err != nil {
		return "", fmt.Errorf("invalid chips value for accelerator type %s: %w", accelaratorType, err)
	}
	resolved := ResolveMachineType(accelType)
	resolvedLower := strings.ToLower(resolved)

	// 3D topologies for v4, v5p, tpu7, tpu7x
	if strings.HasPrefix(resolvedLower, "v4") || strings.HasPrefix(resolvedLower, "v5p") || strings.HasPrefix(resolvedLower, "ct4") || strings.HasPrefix(resolvedLower, "ct5p") || strings.HasPrefix(resolvedLower, "tpu7") {
		if shape, exists := allowed3DTopologies[totalChips]; exists {
			return shape, nil
		}
	} else {
		// 2D topologies for v5e and v6e (default)
		if shape, exists := allowed2DTopologies[totalChips]; exists {
			return shape, nil
		}
	}

	return "", fmt.Errorf("could not find a valid topology shape for %d chips with accelerator %s", totalChips, accelType)
}

// ValidateHardwareRequest validates hardware requests for TPUs.
func ValidateHardwareRequest(machineType, topology, placementPolicy string) error {
	if !IsTPU(machineType) {
		return nil // Skip for non-TPUs
	}

	if topology != "" {
		if err := validateTopologyMeshFormat(topology, machineType); err != nil {
			return err
		}

		if matchesTPUFamily(machineType, valid2DTPUFamilies) {
			if !valid2DShapes[topology] {
				return fmt.Errorf("requested carve footprint layout %s is not an authorized topology subset layout for %s", topology, machineType)
			}
		} else if matchesTPUFamily(machineType, valid3DTPUFamilies) {
			if !valid3DShapes[topology] {
				return fmt.Errorf("requested carve footprint layout %s is not an authorized topology subset layout for %s", topology, machineType)
			}
		} else {
			return fmt.Errorf("TPU type %q is recognized but its topology family is unknown; please report a bug to the toolkit maintainers.", machineType)
		}
	}
	return nil
}

func validateTopologyMeshFormat(requested string, accelType string) error {
	dims := strings.Split(requested, "x")
	if matchesTPUFamily(accelType, valid2DTPUFamilies) {
		if len(dims) != 2 {
			return fmt.Errorf("topology format invalid for %s: requested %s, expected AxB (2 dimensions)", accelType, requested)
		}
	} else {
		if len(dims) != 3 {
			return fmt.Errorf("topology format invalid for %s: requested %s, expected AxBxC (3 dimensions)", accelType, requested)
		}
	}
	for _, d := range dims {
		if _, err := strconv.Atoi(d); err != nil {
			return fmt.Errorf("invalid topology dimension footprint val: %s", d)
		}
	}
	return nil
}

// CheckTopologyContainment returns true if the requested topology fits within the container topology.
func CheckTopologyContainment(requested, container string, accelType string) (bool, error) {
	reqDims := strings.Split(requested, "x")
	conDims := strings.Split(container, "x")
	if len(reqDims) != len(conDims) {
		return false, nil
	}
	for i := 0; i < len(reqDims); i++ {
		r, err := strconv.Atoi(reqDims[i])
		if err != nil {
			return false, fmt.Errorf("invalid topology dimension val: %s", reqDims[i])
		}
		c, err := strconv.Atoi(conDims[i])
		if err != nil {
			return false, fmt.Errorf("invalid topology dimension val: %s", conDims[i])
		}
		if r > c {
			return false, nil
		}
	}

	if requested != container {
		if matchesTPUFamily(accelType, valid2DTPUFamilies) {
			if !valid2DShapes[requested] {
				return false, fmt.Errorf("requested carve footprint layout %s is not an authorized topology subset layout", requested)
			}
		} else if matchesTPUFamily(accelType, valid3DTPUFamilies) {
			if !valid3DShapes[requested] {
				return false, fmt.Errorf("requested carve footprint layout %s is not an authorized topology subset layout", requested)
			}
		}
	}
	return true, nil
}
