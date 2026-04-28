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

package gke

import (
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"
	"strings"
	"time"
)

type MachineTypeCap struct {
	Accelerators []struct {
		Count int    `json:"guestAcceleratorCount"`
		Type  string `json:"guestAcceleratorType"`
	} `json:"accelerators"`
	GuestCpus int `json:"guestCpus"`
	MemoryMb  int `json:"memoryMb"`
}

var acceleratorShorthandMap = map[string]string{
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
	"v4-8":  "ct4p-hightpu-4t",
	"v5p-1": "ct5p-hightpu-1t",
	"v5p-2": "ct5p-hightpu-2t",
	"v5p-4": "ct5p-hightpu-4t",
	"v5e-1": "ct5lp-hightpu-1t",
	"v5e-4": "ct5lp-hightpu-4t",
	"v5e-8": "ct5lp-hightpu-8t",
	"v6e-1": "ct6e-standard-1t",
	"v6e-4": "ct6e-standard-4t",
	"v6e-8": "ct6e-standard-8t",
	"tpu7":  "tpu7-standard-1t",
	"tpu7x": "tpu7x-standard-4t",
}

func (g *GKEOrchestrator) FetchMachineCapacity(machineType, zone string) (int, error) {
	cap, err := g.FetchMachineCapabilities(machineType, zone)
	if err != nil {
		return 0, err
	}

	if len(cap.Accelerators) > 0 {
		return cap.Accelerators[0].Count, nil
	}
	if cap.GuestCpus > 0 {
		return cap.GuestCpus, nil
	}
	return 0, fmt.Errorf("no accelerators or guestCpus found for machine type %s in zone %s", machineType, zone)
}

func (g *GKEOrchestrator) FetchMachineCapabilities(machineType, zone string) (MachineTypeCap, error) {
	if zone == "" {
		return MachineTypeCap{}, fmt.Errorf("zone is required for machine capacity lookup")
	}

	cacheKey := machineType + ":" + zone
	if g.machineCapCache != nil {
		if cap, ok := g.machineCapCache[cacheKey]; ok {
			return cap, nil
		}
	}

	isRegion := len(strings.Split(zone, "-")) < 3
	zonesToTry := []string{zone}

	if isRegion && len(g.clusterZones) > 0 {
		zonesToTry = g.clusterZones
	}

	var lastErr error
	for _, z := range zonesToTry {
		cap, err := g.fetchCapacityForZoneWithRetry(machineType, z, !isRegion)
		if err == nil {
			logging.Info("Discovered machine capabilities in zone %s", z)
			if g.machineCapCache == nil {
				g.machineCapCache = make(map[string]MachineTypeCap)
			}
			g.machineCapCache[cacheKey] = cap
			// Also cache for the specific zone that succeeded
			specificKey := machineType + ":" + z
			g.machineCapCache[specificKey] = cap
			return cap, nil
		}
		lastErr = err
	}

	if isRegion {
		return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s: tried in all candidate zones %v but did not find machine type in any of them", machineType, zonesToTry)
	}
	return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s in zone %s: %w", machineType, zone, lastErr)
}

func (g *GKEOrchestrator) fetchCapacityForZoneWithRetry(machineType, zone string, shouldRetry bool) (MachineTypeCap, error) {
	maxRetries := 1
	if shouldRetry {
		maxRetries = 3
	}

	var result shell.CommandResult
	var cap MachineTypeCap

	for i := 0; i < maxRetries; i++ {
		result = g.executor.ExecuteCommand("gcloud", "compute", "machine-types", "describe", machineType, "--zone="+zone, "--format=json")
		if result.ExitCode == 0 {
			break
		}
		if shouldRetry {
			logging.Info("gcloud compute machine-types describe failed (attempt %d/%d): %s. Retrying...", i+1, maxRetries, result.Stderr)
			time.Sleep(time.Duration(1<<i) * time.Second)
		}
	}

	if result.ExitCode != 0 {
		return cap, fmt.Errorf("gcloud compute machine-types describe failed for zone %s: %s", zone, result.Stderr)
	}

	if err := json.Unmarshal([]byte(result.Stdout), &cap); err != nil {
		return cap, fmt.Errorf("failed to unmarshal machine capacity JSON: %w", err)
	}

	return cap, nil
}

func (g *GKEOrchestrator) verifySuperSlicingActive(opts ManifestOptions) (bool, error) {
	// Return false immediately if not using TPUs.
	if !strings.Contains(strings.ToLower(opts.AcceleratorType), "tpu") {
		return false, nil
	}

	// Check for topologies.kueue.x-k8s.io CRD
	cResult := g.executor.ExecuteCommand("kubectl", "get", "crd", "topologies.kueue.x-k8s.io")
	if cResult.ExitCode != 0 {
		logging.Warn("Topology CRD not found. Kueue Super-slicing not active.")
		return false, nil
	}

	// Check for AdmissionCheck with controllerName: accelerator.gke.io/slice
	acResult := g.executor.ExecuteCommand("kubectl", "get", "admissioncheck", "-o", "json")
	if acResult.ExitCode != 0 {
		logging.Warn("Failed to query AdmissionChecks. Assuming super-slicing not active.")
		return false, nil
	}

	var acList struct {
		Items []struct {
			Spec struct {
				ControllerName string `json:"controllerName"`
			} `json:"spec"`
		} `json:"items"`
	}

	if err := json.Unmarshal([]byte(acResult.Stdout), &acList); err != nil {
		logging.Warn("Failed to parse AdmissionChecks JSON: %v. Assuming super-slicing not active.", err)
		return false, nil
	}

	hasSliceController := false
	for _, item := range acList.Items {
		if item.Spec.ControllerName == "accelerator.gke.io/slice" {
			hasSliceController = true
			break
		}
	}

	if !hasSliceController {
		logging.Info("No AdmissionCheck with controller 'accelerator.gke.io/slice' found. Super-slicing not active.")
		return false, nil
	}

	// Check discovered node pools for super-slicing
	requestedMachineName := g.resolveMachineName(opts.AcceleratorType)
	for _, np := range g.clusterDesc.NodePools {
		if np.Config.MachineType == requestedMachineName {
			if np.PlacementPolicy != nil && np.PlacementPolicy.AcceleratorTopologyMode == "PROVISION_ONLY" {
				logging.Info("Super-slicing PROVISION_ONLY mode detected for node pool %s.", np.Name)
				return true, nil
			}
		}
	}

	logging.Info("Node pool does not have PROVISION_ONLY mode. Super-slicing not active.")
	return false, nil
}

func (g *GKEOrchestrator) calculateResourceLimits(opts ManifestOptions, profile JobProfile) (cpu, mem, gpu, tpu string, err error) {
	if profile.IsCPUMachine {
		logging.Info("Using cached capacity for CPU machine %s during limits calculation: %d", opts.AcceleratorType, profile.CapacityCount)
		offsetVCPUs := max(1, int(float64(profile.CapacityCount)*0.95))
		return fmt.Sprintf("%d", offsetVCPUs), "", "", "", nil
	}

	mapped := g.GenerateGKENodeSelectorLabel(opts.AcceleratorType)

	cpuLim, memLim, gpuLim, tpuLim, err := g.calculateGCPMachineResourceLimits(opts, profile, mapped)
	if err != nil {
		return "", "", "", "", fmt.Errorf("cluster resolution failed for %s: %w", opts.AcceleratorType, err)
	}
	return cpuLim, memLim, gpuLim, tpuLim, nil
}

func (g *GKEOrchestrator) calculateGCPMachineResourceLimits(opts ManifestOptions, profile JobProfile, mapped string) (cpu, mem, gpu, tpu string, err error) {
	machineName := g.resolveMachineName(opts.AcceleratorType)

	count, err := g.FetchMachineCapacity(machineName, opts.ClusterLocation)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to resolve machine type %s: %w", machineName, err)
	}

	if count > 0 {
		logging.Info("Dynamically determined capacity for %s: %d", machineName, count)

		if strings.Contains(strings.ToLower(mapped), "nvidia") {
			return "", "", fmt.Sprintf("%d", count), "", nil
		}
		if strings.Contains(strings.ToLower(mapped), "tpu") {
			return "", "", "", fmt.Sprintf("%d", count), nil
		}
		return "", "", "", "", fmt.Errorf("machine type %s resolved to %d capacity but could not be classified as GPU or TPU (mapped label: %s)", machineName, count, mapped)
	}
	return "", "", "", "", fmt.Errorf("failed to determine capacity for machine type %s", machineName)
}

func (g *GKEOrchestrator) resolveMachineName(acceleratorType string) string {
	if g.acceleratorToMachineType != nil {
		if machineType, exists := g.acceleratorToMachineType[strings.ToLower(acceleratorType)]; exists {
			return machineType
		}
	}

	if mappedName, exists := acceleratorShorthandMap[acceleratorType]; exists {
		return mappedName
	}

	mapped := g.GenerateGKENodeSelectorLabel(acceleratorType)
	if mappedName, exists := acceleratorShorthandMap[mapped]; exists {
		return mappedName
	}
	return acceleratorType
}
