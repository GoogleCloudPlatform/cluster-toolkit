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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/zclconf/go-cty/cty"
	compute "google.golang.org/api/compute/v1"
)

var (
	machineTypeCache sync.Map // map[string]*compute.MachineType
	computeService   *compute.Service
	computeOnce      sync.Once
)

func extractStringSetting(m *Module, bp Blueprint, key string) string {
	if m.Settings.Has(key) {
		v, ok := attemptEvalModuleInput(m.Settings.Get(key), bp)
		if ok && v.Type() == cty.String {
			return v.AsString()
		}
	}
	return ""
}

func extractZone(m *Module, bp Blueprint) string {
	zone := extractStringSetting(m, bp, "zone")
	if zone != "" {
		return zone
	}
	if bp.Vars.Has("zone") {
		v, ok := attemptEvalModuleInput(bp.Vars.Get("zone"), bp)
		if ok && v.Type() == cty.String {
			return v.AsString()
		}
	}
	if bp.Vars.Has("zones") {
		v, ok := attemptEvalModuleInput(bp.Vars.Get("zones"), bp)
		if ok && (v.Type().IsTupleType() || v.Type().IsListType()) {
			iter := v.ElementIterator()
			if iter.Next() {
				_, val := iter.Element()
				if val.Type() == cty.String {
					return val.AsString()
				}
			}
		}
	}
	return ""
}

func extractProject(m *Module, bp Blueprint) string {
	project := extractStringSetting(m, bp, "project_id")
	if project != "" {
		return project
	}
	if bp.Vars.Has("project_id") {
		v, ok := attemptEvalModuleInput(bp.Vars.Get("project_id"), bp)
		if ok && v.Type() == cty.String {
			return v.AsString()
		}
	}
	return ""
}

func extractMachineParams(m *Module, bp Blueprint) (string, string, string) {
	machineType := extractStringSetting(m, bp, "machine_type")
	zone := extractZone(m, bp)
	project := extractProject(m, bp)
	return machineType, zone, project
}

func ParseTPUCount(machineType string) int {
	if !(strings.HasPrefix(machineType, "ct") || strings.HasPrefix(machineType, "tpu")) {
		return 0
	}
	parts := strings.Split(machineType, "-")
	if len(parts) == 0 {
		return 0
	}
	suffix := parts[len(parts)-1]
	if !strings.HasSuffix(suffix, "t") {
		return 0
	}
	var count int
	if _, err := fmt.Sscanf(suffix, "%dt", &count); err != nil {
		return 0
	}
	return count
}

// ResolveAcceleratorInfo returns the number and type of accelerators for a given machine type.
func ResolveAcceleratorInfo(mt *compute.MachineType, machineType string) (count int, accelType string, isTPU bool) {
	isTPU = strings.HasPrefix(machineType, "ct") || strings.HasPrefix(machineType, "tpu")

	if len(mt.Accelerators) > 0 {
		acc := mt.Accelerators[0]
		if isTPU || strings.Contains(strings.ToLower(acc.GuestAcceleratorType), "tpu") {
			return int(acc.GuestAcceleratorCount), "tpu", true
		}
		return int(acc.GuestAcceleratorCount), acc.GuestAcceleratorType, false
	}

	if count := ParseTPUCount(machineType); count > 0 {
		return count, "tpu", true
	}

	return 0, "", false
}

// GetMachineType fetches machine type information from GCP Compute API with caching.
func GetMachineType(project, zone, machineType string) (*compute.MachineType, error) {
	cacheKey := fmt.Sprintf("%s/%s/%s", project, zone, machineType)
	if cached, ok := machineTypeCache.Load(cacheKey); ok {
		return cached.(*compute.MachineType), nil
	}

	var initErr error
	computeOnce.Do(func() {
		s, err := compute.NewService(context.Background())
		if err != nil {
			initErr = fmt.Errorf("failed to initialize compute service: %w", err)
			return
		}
		computeService = s
	})

	if initErr != nil {
		return nil, initErr
	}

	res, err := computeService.MachineTypes.Get(project, zone, machineType).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get machine type info for machine_type=%s zone=%s project=%s: %w", machineType, zone, project, err)
	}
	machineTypeCache.Store(cacheKey, res)

	return res, nil
}

func getMachineConfigJSON(m *Module, bp Blueprint) (string, error) {
	if mockData := os.Getenv("GHPC_MOCK_MACHINE_CONFIG"); mockData != "" {
		return mockData, nil
	}

	machineType, zone, project := extractMachineParams(m, bp)

	if machineType == "" || zone == "" || project == "" {
		return `{"gpus": {}, "tpus": {}, "cpus": {}}`, nil
	}

	mt, err := GetMachineType(project, zone, machineType)
	if err != nil {
		return "", err
	}

	return buildOutputConfigJSON(machineType, mt)
}

type gpuConfig struct {
	Count int    `json:"count"`
	Type  string `json:"type"`
}
type tpuConfig struct {
	Count int `json:"count"`
}
type cpuConfig struct {
	Count    int `json:"count"`
	MemoryMb int `json:"memoryMb,omitempty"`
}

type outputConfig struct {
	GPUs map[string]gpuConfig `json:"gpus"`
	TPUs map[string]tpuConfig `json:"tpus"`
	CPUs map[string]cpuConfig `json:"cpus"`
}

func buildOutputConfigJSON(machineType string, mt *compute.MachineType) (string, error) {
	result := outputConfig{
		GPUs: make(map[string]gpuConfig),
		TPUs: make(map[string]tpuConfig),
		CPUs: make(map[string]cpuConfig),
	}

	result.CPUs[machineType] = cpuConfig{Count: int(mt.GuestCpus), MemoryMb: int(mt.MemoryMb)}

	count, accelType, isTPU := ResolveAcceleratorInfo(mt, machineType)
	if count > 0 {
		if isTPU {
			result.TPUs[machineType] = tpuConfig{Count: count}
		} else {
			result.GPUs[machineType] = gpuConfig{
				Count: count,
				Type:  accelType,
			}
		}
	}

	resBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal machine config object to JSON: %w", err)
	}

	return string(resBytes), nil
}

// ClearMachineTypeCache clears the machine type cache. Used for testing.
func ClearMachineTypeCache() {
	machineTypeCache.Range(func(key, value interface{}) bool {
		machineTypeCache.Delete(key)
		return true
	})
}
