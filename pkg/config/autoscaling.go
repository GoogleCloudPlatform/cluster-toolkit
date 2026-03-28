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
	"fmt"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// ExpandClusterAutoscaling intercepts the cluster_autoscaling variable,
// parses machine_type in Go, and injects internal variables for maximum chips and accelerator type.
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

	for it.Next() {
		_, resVal := it.Element()
		processedLimit, ok, err := processAutoscalingLimit(resVal)
		if err != nil {
			return err
		}
		if ok {
			newLimits = append(newLimits, processedLimit)
		}
	}

	caMap["limits"] = cty.ListVal(newLimits)
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

func processAutoscalingLimit(resVal cty.Value) (cty.Value, bool, error) {
	if !resVal.Type().IsObjectType() {
		return cty.Value{}, false, nil
	}
	resMap := resVal.AsValueMap()

	mtVal, ok := resMap["autoprovisioning_machine_type"]
	if !ok || mtVal.IsNull() || !mtVal.IsKnown() {
		return cty.Value{}, false, nil
	}
	machineType := mtVal.AsString()

	maxCount := 1
	maxCountPassed := false
	if mcVal, ok := resMap["autoprovisioning_max_accelerator_count"]; ok && !mcVal.IsNull() && mcVal.IsKnown() {
		if mcVal.Type() == cty.Number {
			f, _ := mcVal.AsBigFloat().Float64()
			maxCount = int(f)
			maxCountPassed = true
		}
	}

	acceleratorsPerVM, accType := extractAcceleratorCountAndType(machineType)
	if acceleratorsPerVM == 0 {
		return cty.Value{}, false, nil // Not an accelerator or unrecognized
	}

	var totalAccelerators int
	if maxCountPassed {
		if maxCount <= 0 {
			return cty.Value{}, false, fmt.Errorf("autoprovisioning_max_accelerator_count must be greater than 0 for machine type %s, got %d", machineType, maxCount)
		}
		if maxCount%acceleratorsPerVM != 0 {
			return cty.Value{}, false, fmt.Errorf("autoprovisioning_max_accelerator_count (%d) for machine type %s must be a multiple of its native accelerator count (%d)", maxCount, machineType, acceleratorsPerVM)
		}
		totalAccelerators = maxCount
	} else {
		totalAccelerators = acceleratorsPerVM // assume 1 VM worth by default
	}

	resMap["autoprovisioning_max_accelerator_count"] = cty.NumberIntVal(int64(totalAccelerators))
	resMap["autoprovisioning_machine_type"] = cty.StringVal(accType)
	return cty.ObjectVal(resMap), true, nil
}

func extractAcceleratorCountAndType(machineType string) (int, string) {
	switch {
	case strings.Contains(machineType, "tpu") || strings.HasPrefix(machineType, "v"):
		return extractTPUChipsPerVM(machineType), machineType // Returning exact machineType literal as corrected

	case strings.Contains(machineType, "highgpu") || strings.Contains(machineType, "megagpu") || strings.Contains(machineType, "ultragpu"):
		idx := strings.LastIndex(machineType, "-")
		if idx != -1 && strings.HasSuffix(machineType, "g") {
			if val, err := strconv.Atoi(machineType[idx+1 : len(machineType)-1]); err == nil {
				return val, machineType // Returning exact machineType literal
			}
		}
	}

	return 0, ""
}
