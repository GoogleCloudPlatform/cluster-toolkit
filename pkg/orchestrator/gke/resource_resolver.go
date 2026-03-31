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
	"os"
	"strings"
	"time"
)

type MachineTypeCap struct {
	Accelerators []struct {
		Count int    `json:"guestAcceleratorCount"`
		Type  string `json:"guestAcceleratorType"`
	} `json:"accelerators"`
	GuestCpus int `json:"guestCpus"` // Parse vCPUs for CPU-only machines
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

	// TPUs mappings
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
	"tpu-v7":  "tpu7-standard-1t",
	"tpu-v7x": "tpu7x-standard-4t",
}

func (g *GKEOrchestrator) FetchMachineCapacity(machineType, zone string) (int, error) {
	if zone == "" {
		return 0, fmt.Errorf("zone is required for machine capacity lookup")
	}

	maxRetries := 3
	var result shell.CommandResult

	for i := 0; i < maxRetries; i++ {
		result = g.executor.ExecuteCommand("gcloud", "compute", "machine-types", "describe", machineType, "--zone="+zone, "--format=json")
		if result.ExitCode == 0 {
			break
		}
		logging.Info("gcloud compute machine-types describe failed (attempt %d/%d): %s. Retrying...", i+1, maxRetries, result.Stderr)
		time.Sleep(time.Duration(1<<i) * time.Second) // Exponential backoff
	}

	if result.ExitCode != 0 {
		return 0, fmt.Errorf("gcloud compute machine-types describe failed after %d retries: %s", maxRetries, result.Stderr)
	}

	var cap MachineTypeCap
	if err := json.Unmarshal([]byte(result.Stdout), &cap); err != nil {
		return 0, fmt.Errorf("failed to unmarshal machine capacity JSON: %w", err)
	}

	if len(cap.Accelerators) > 0 {
		return cap.Accelerators[0].Count, nil
	}

	// For CPU-only machines, return the vCPUs count as "capacity"
	if cap.GuestCpus > 0 {
		return cap.GuestCpus, nil
	}

	return 0, fmt.Errorf("no accelerators or guestCpus found for machine type %s", machineType)
}

func (g *GKEOrchestrator) verifySuperSlicingActive(opts ManifestOptions) (bool, error) {
	// 1. TPU Focus: Return false immediately if not using TPUs.
	if opts.AcceleratorType == "" || !strings.Contains(strings.ToLower(opts.AcceleratorType), "tpu") {
		return false, nil
	}

	// 2. Machine Type/Profile Guard: Describe node pool to see if it uses PROVISION_ONLY!
	poolName := os.Getenv("GKE_NODE_POOL_NAME")
	if poolName == "" {
		logging.Warn("GKE_NODE_POOL_NAME is not set. Assuming Super-slicing is not active for this node pool.")
		return false, nil
	}

	result := g.executor.ExecuteCommand("gcloud", "container", "node-pools", "describe", poolName, "--cluster="+opts.ClusterName, "--zone="+opts.ClusterLocation, "--format=json(placementPolicy)")
	if result.ExitCode != 0 {
		logging.Warn("gcloud container node-pools describe failed: %s. Proceeding assuming no Super-slicing.", result.Stderr)
		return false, nil
	}

	var policy map[string]interface{}
	if err := json.Unmarshal([]byte(result.Stdout), &policy); err == nil {
		if placement, ok := policy["placementPolicy"].(map[string]interface{}); ok {
			if mode, ok := placement["acceleratorTopologyMode"].(string); ok && mode == "PROVISION_ONLY" {
				logging.Info("Super-slicing PROVISION_ONLY mode detected for node pool %s.", poolName)
				return true, nil
			}
		}
	}

	// 3. Kueue CRD Checks: Check for topologies.kueue.x-k8s.io and AdmissionChecks (simulated via shell commands)
	cResult := g.executor.ExecuteCommand("kubectl", "get", "crd", "topologies.kueue.x-k8s.io")
	if cResult.ExitCode != 0 {
		logging.Warn("Topology CRD not found. Kueue Super-slicing not active.")
		return false, nil
	}

	return true, nil
}

func (g *GKEOrchestrator) calculateResourceLimits(opts ManifestOptions, profile JobProfile) (cpu, mem, gpu, tpu string, err error) {
	mapped := g.GenerateGKENodeSelectorLabel(opts.AcceleratorType)

	if opts.AcceleratorType != "" {
		if opts.ClusterLocation == "" {
			if !strings.Contains(strings.ToLower(mapped), "nvidia") && !isTPUFallback(mapped) {
				return "", "", "", "", fmt.Errorf("cluster location (zone/region) is required to determine if %s is a CPU machine", opts.AcceleratorType)
			}
			// Let it fall through to the hardcoded NVIDIA/TPU fallbacks below!
		} else {
			cpuLim, memLim, gpuLim, tpuLim, err := g.calculateGCPMachineResourceLimits(opts, profile, mapped)
			if err != nil {
				return "", "", "", "", err
			}
			if cpuLim != "" || memLim != "" || gpuLim != "" || tpuLim != "" {
				return cpuLim, memLim, gpuLim, tpuLim, nil
			}
		}

	}

	if strings.Contains(strings.ToLower(mapped), "nvidia") {
		return "", "", "1", "", nil
	}
	if isTPUFallback(mapped) {
		return "", "", "", "4", nil
	}

	if opts.AcceleratorType == "" {
		return "", "", "", "", fmt.Errorf("--accelerator (machine type) is required for submission to determine resource limits (XPK strict enforcement)")
	}

	return "", "", "", "", fmt.Errorf("could not determine resource limits for %s", opts.AcceleratorType)
}

func (g *GKEOrchestrator) calculateGCPMachineResourceLimits(opts ManifestOptions, profile JobProfile, mapped string) (cpu, mem, gpu, tpu string, err error) {
	if profile.IsCPUMachine {
		cpuLim, err := g.calculateCPUMachineResourceLimits(opts, profile)
		if err != nil {
			return "", "", "", "", err
		}
		return cpuLim, "", "", "", nil
	}

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

func (g *GKEOrchestrator) calculateCPUMachineResourceLimits(opts ManifestOptions, profile JobProfile) (string, error) {
	count := profile.CapacityCount
	logging.Info("Using cached capacity for CPU machine %s during limits calculation: %d", opts.AcceleratorType, count)

	offsetVCPUs := int(float64(count) * 0.95)
	if offsetVCPUs < 1 {
		offsetVCPUs = 1
	}
	return fmt.Sprintf("%d", offsetVCPUs), nil
}

func (g *GKEOrchestrator) resolveMachineName(acceleratorType string) string {
	if mappedName, exists := acceleratorShorthandMap[acceleratorType]; exists {
		return mappedName
	}

	mapped := g.GenerateGKENodeSelectorLabel(acceleratorType)
	if mappedName, exists := acceleratorShorthandMap[mapped]; exists {
		return mappedName
	}
	return acceleratorType
}
