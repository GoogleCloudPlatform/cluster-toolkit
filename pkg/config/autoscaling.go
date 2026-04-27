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

package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// ExpandClusterAutoscaling intercepts the cluster_autoscaling variable,
// parses machine_type in Go, and injects internal variables for maximum chips and accelerator type.
type cachedAccInfo struct {
	count int
	t     string
}

func ExpandClusterAutoscaling(bp Blueprint, mod *Module) error {
	caMap, limitsVal, ok, err := validateAndGetLimits(bp, mod)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	it := limitsVal.ElementIterator()
	var newLimits []cty.Value
	accCache := make(map[string]cachedAccInfo)

	for it.Next() {
		_, resVal := it.Element()
		processedLimit, ok, err := processAutoscalingLimit(resVal, bp, mod, accCache)
		if err != nil {
			return err
		}
		if ok {
			newLimits = append(newLimits, processedLimit)
		}
	}

	if len(newLimits) > 0 {
		caMap["limits"] = cty.ListVal(newLimits)
	} else {
		caMap["limits"] = cty.ListValEmpty(limitsVal.Type().ElementType())
	}
	mod.Settings = mod.Settings.With("cluster_autoscaling", cty.ObjectVal(caMap))

	return nil
}

func validateAndGetLimits(bp Blueprint, mod *Module) (map[string]cty.Value, cty.Value, bool, error) {
	if !mod.Settings.Has("cluster_autoscaling") {
		return nil, cty.Value{}, false, nil
	}

	caVal, err := bp.Eval(mod.Settings.Get("cluster_autoscaling"))
	if err != nil || !caVal.IsKnown() || caVal.IsNull() {
		return nil, cty.Value{}, false, err
	}

	if !caVal.Type().IsObjectType() {
		return nil, cty.Value{}, false, nil
	}

	caMap := caVal.AsValueMap()

	enabledVal, ok := caMap["enabled"]
	if !ok || enabledVal.IsNull() || !enabledVal.IsKnown() || !enabledVal.True() {
		return nil, cty.Value{}, false, nil
	}

	limitsVal, ok := caMap["limits"]
	if !ok || limitsVal.IsNull() || !limitsVal.IsKnown() {
		return nil, cty.Value{}, false, nil
	}

	return caMap, limitsVal, true, nil
}

func processAutoscalingLimit(resVal cty.Value, bp Blueprint, mod *Module, accCache map[string]cachedAccInfo) (cty.Value, bool, error) {
	if !resVal.Type().IsObjectType() {
		return cty.Value{}, false, nil
	}
	resMap := resVal.AsValueMap()

	mtVal, ok := resMap["autoprovisioning_machine_type"]
	if !ok || mtVal.IsNull() || !mtVal.IsKnown() {
		return cty.Value{}, false, nil
	}
	machineType := mtVal.AsString()

	maxCount, maxCountPassed := extractMaxCount(resMap)
	var acceleratorsPerVM int
	var accType string

	if cached, exists := accCache[machineType]; exists {
		acceleratorsPerVM = cached.count
		accType = cached.t
	} else {
		var err error
		acceleratorsPerVM, accType, err = getAcceleratorCountAndType(machineType, bp, mod)
		if err != nil {
			return cty.Value{}, false, err
		}
		accCache[machineType] = cachedAccInfo{count: acceleratorsPerVM, t: accType}
	}

	if acceleratorsPerVM == 0 {
		return cty.Value{}, false, nil
	}

	totalAccelerators, err := validateAndExtractTotalAccelerators(maxCount, maxCountPassed, acceleratorsPerVM, machineType)
	if err != nil {
		return cty.Value{}, false, err
	}

	resMap["autoprovisioning_max_count"] = cty.NumberIntVal(int64(totalAccelerators))
	resMap["autoprovisioning_resource_type"] = cty.StringVal(accType)
	return cty.ObjectVal(resMap), true, nil
}

func extractMaxCount(resMap map[string]cty.Value) (int, bool) {
	if mcVal, ok := resMap["autoprovisioning_max_count"]; ok && !mcVal.IsNull() && mcVal.IsKnown() {
		if mcVal.Type() == cty.Number {
			f, _ := mcVal.AsBigFloat().Float64()
			return int(f), true
		}
	}
	return 1, false
}

func validateAndExtractTotalAccelerators(maxCount int, maxCountPassed bool, acceleratorsPerVM int, machineType string) (int, error) {
	if maxCountPassed {
		if maxCount <= 0 {
			return 0, fmt.Errorf("autoprovisioning_max_count must be greater than 0 for machine type %s, got %d", machineType, maxCount)
		}
		if maxCount%acceleratorsPerVM != 0 {
			return 0, fmt.Errorf("autoprovisioning_max_count (%d) for machine type %s must be a multiple of its native capacity (%d)", maxCount, machineType, acceleratorsPerVM)
		}
		return maxCount, nil
	}
	// Default to a large number (e.g., 1000) to allow Node Auto-Provisioning
	// to scale beyond a single VM capacity.
	return 1000, nil
}

func getAcceleratorCountAndType(machineType string, bp Blueprint, mod *Module) (int, string, error) {
	var origMt cty.Value
	hasMt := mod.Settings.Has("machine_type")
	if hasMt {
		origMt = mod.Settings.Get("machine_type")
	}
	mod.Settings = mod.Settings.With("machine_type", cty.StringVal(machineType))

	// Defensively restore the original machine_type setting in all circumstances
	defer func() {
		if hasMt {
			mod.Settings = mod.Settings.With("machine_type", origMt)
		}
	}()

	cfgJson, err := getMachineConfigJSON(mod, bp)
	if err != nil {
		return 0, "", err
	}

	var data struct {
		GPUs map[string]struct {
			Count int    `json:"count"`
			Type  string `json:"type"`
		} `json:"gpus"`
		TPUs map[string]struct {
			Count int `json:"count"`
		} `json:"tpus"`
	}
	if err := json.Unmarshal([]byte(cfgJson), &data); err != nil {
		return 0, "", fmt.Errorf("failed to unmarshal machine configurations: %w", err)
	}

	if acc, ok := data.GPUs[machineType]; ok {
		return acc.Count, acc.Type, nil
	} else if acc, ok := data.TPUs[machineType]; ok {
		// Map TPU machine type to GKE accelerator identifier
		tpuAccMap := map[string]string{
			"ct4p":  "tpu-v4-podslice",
			"ct5lp": "tpu-v5-lite-podslice",
			"ct5p":  "tpu-v5p-slice",
			"ct6e":  "tpu-v6e-slice",
			"tpu7x": "tpu7x",
		}
		accType := machineType
		// Robustly extract prefix (e.g., "ct6e" from "ct6e-standard-4t")
		parts := strings.Split(machineType, "-")
		if len(parts) > 0 {
			if mapped, exists := tpuAccMap[parts[0]]; exists {
				accType = mapped
			}
		}
		return acc.Count, accType, nil
	}

	return 0, "", nil
}
