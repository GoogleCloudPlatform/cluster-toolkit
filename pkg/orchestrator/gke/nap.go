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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/orchestrator"
	"sort"
	"strings"
)

func (g *GKEOrchestrator) isNAPEnabledForMachineType(machineType, zone string) (bool, error) {
	if !g.napEnabled {
		return false, nil
	}

	resolvedType := config.ResolveMachineType(machineType)

	if config.IsTPU(resolvedType) {
		return g.validateTPUNAPLimit(resolvedType)
	}

	cap, err := g.FetchMachineCapabilities(resolvedType, zone)
	if err != nil {
		return false, err
	}
	if len(cap.Accelerators) > 0 {
		return g.validateGPUNAPLimit(resolvedType, cap)
	}

	return g.napLimits["cpu"] > 0, nil
}

func (g *GKEOrchestrator) validateTPUNAPLimit(resolvedType string) (bool, error) {
	key := strings.ToLower(g.GenerateGKENodeSelectorLabel(resolvedType))
	if limit, exists := g.napLimits[key]; exists {
		return limit > 0, nil
	}
	// Fallback to generic TPU limit ONLY if no specific TPU limits are configured.
	for k := range g.napLimits {
		if isSpecificTPUKey(k) {
			return false, nil
		}
	}
	return g.napLimits["google.com/tpu"] > 0, nil
}

func (g *GKEOrchestrator) validateGPUNAPLimit(resolvedType string, cap MachineTypeCap) (bool, error) {
	key := strings.ToLower(g.GenerateGKENodeSelectorLabel(resolvedType))
	if strings.EqualFold(key, resolvedType) {
		key = strings.ToLower(g.GenerateGKENodeSelectorLabel(cap.Accelerators[0].Type))
	}
	if !isKnownGKEAccelerator(key) {
		return false, fmt.Errorf("unknown accelerator label: %q", cap.Accelerators[0].Type)
	}
	if limit, exists := g.napLimits[key]; exists {
		return limit > 0, nil
	}
	// Fallback to generic GPU limit ONLY if no specific GPU limits are configured.
	for k := range g.napLimits {
		if isSpecificGPUKey(k) {
			return false, nil
		}
	}
	return g.napLimits["nvidia.com/gpu"] > 0, nil
}

func (g *GKEOrchestrator) checkNAPFlagsSupported(hasNAPFlags bool, job *orchestrator.JobDefinition) error {
	if !g.napEnabled && hasNAPFlags {
		return fmt.Errorf("GKE NAP provisioning options (--gke-nap-provisioning %q, --gke-nap-reservation %q) are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled. The current cluster does not have NAP enabled.\nRemediation: Enable Node Auto-Provisioning on your cluster to use these options, or submit your job without them", job.GKENAPProvisioning, job.GKENAPReservation)
	}
	return nil
}

func (g *GKEOrchestrator) getConfiguredLimitsError(computeType string) error {
	var configuredLimits []string
	for k, v := range g.napLimits {
		if v > 0 {
			configuredLimits = append(configuredLimits, k)
		}
	}
	sort.Strings(configuredLimits)
	return fmt.Errorf("workload submission rejected. Compute type %q is not configured within your cluster's Node Auto-Provisioning (NAP) limits. Configured limits on cluster: %s", computeType, strings.Join(configuredLimits, ", "))
}

func (g *GKEOrchestrator) validateConsumptionForStaticCluster(job *orchestrator.JobDefinition) error {
	hasNAPFlags := job.GKENAPProvisioning != ""

	if err := g.checkNAPFlagsSupported(hasNAPFlags, job); err != nil {
		return err
	}

	if !g.napEnabled || !hasNAPFlags {
		return nil
	}

	// NAP flags were requested. Validate strictly against GKE NAP limits.
	isNAP, err := g.isNAPEnabledForMachineType(job.MachineType, job.ClusterLocation)
	if err != nil {
		return err
	}
	if !isNAP {
		return g.getConfiguredLimitsError(job.ComputeType)
	}

	return nil
}

// resolveFallbackName returns a fallback reservation name from the URI parts.
func resolveFallbackName(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	// If it has reservationBlocks, the name is the one before it
	for i, part := range parts {
		if part == "reservationBlocks" && i > 0 {
			return parts[i-1]
		}
	}
	// Fallback to last element
	return parts[len(parts)-1]
}

// parseReservationURI parses a GCE reservation resource URI or reservation path into its components.
// E.g.,
// - "my-res" -> Name: "my-res"
// - "projects/my-project/reservations/my-res" -> Project: "my-project", Name: "my-res"
// - "projects/my-project/reservations/my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2" -> Project: "my-project", Name: "my-res", Block: "block-1", Subblock: "subblock-2"
func parseReservationURI(resName string) parsedReservation {
	resName = strings.TrimSuffix(resName, "/")
	var parsed parsedReservation
	if !strings.Contains(resName, "/") {
		parsed.Name = strings.ToLower(resName)
		return parsed
	}

	parts := strings.Split(resName, "/")
	for i := 0; i < len(parts)-1; i++ {
		switch parts[i] {
		case "projects":
			parsed.Project = strings.ToLower(parts[i+1])
		case "reservations":
			parsed.Name = strings.ToLower(parts[i+1])
		case "reservationBlocks":
			parsed.Block = strings.ToLower(parts[i+1])
		case "reservationSubBlocks":
			parsed.Subblock = strings.ToLower(parts[i+1])
		}
	}

	if parsed.Name == "" {
		parsed.Name = strings.ToLower(resolveFallbackName(parts))
	}

	return parsed
}

func isSpecificGPUKey(key string) bool {
	return strings.HasPrefix(key, "nvidia-")
}

func isSpecificTPUKey(key string) bool {
	return strings.HasPrefix(key, "tpu-") || strings.HasPrefix(key, "tpu7")
}

func isKnownGKEAccelerator(key string) bool {
	switch key {
	case "nvidia-tesla-t4", "nvidia-tesla-v100":
		return true
	}
	for _, val := range config.GetMachineMappings().MachineFamilyToLabelMap {
		if val == key {
			return true
		}
	}
	return false
}

func parseNAPLimits(autoscaling gkeClusterAutoscaling) map[string]int64 {
	limits := make(map[string]int64)
	for _, rl := range autoscaling.ResourceLimits {
		limits[rl.ResourceType] = rl.Maximum
	}

	for _, rl := range autoscaling.ResourceLimits {
		resName := rl.ResourceType
		maxVal := rl.Maximum
		if resName == "gpu" || strings.Contains(resName, "nvidia") {
			if maxVal > limits["nvidia.com/gpu"] {
				limits["nvidia.com/gpu"] = maxVal
			}
		} else if strings.Contains(resName, "tpu") {
			if maxVal > limits["google.com/tpu"] {
				limits["google.com/tpu"] = maxVal
			}
		}
	}
	return limits
}

func isNonAcceleratorResource(resName string) bool {
	switch resName {
	case "cpu", "memory", "gpu", "tpu", "pods", "storage", "ephemeral-storage":
		return true
	default:
		return false
	}
}

func resolveFlavorFromResource(resName string) (string, map[string]string, error) {
	var flavorName string
	var nodeLabels map[string]string

	switch {
	case resName == "google.com/tpu":
		flavorName = "flavor-tpu-generic"
	case resName == "nvidia.com/gpu":
		flavorName = "flavor-nvidia-generic"
	case strings.HasPrefix(resName, "nvidia-"):
		flavorName = "flavor-" + resName
		nodeLabels = map[string]string{
			"cloud.google.com/gke-accelerator": resName,
		}
	case strings.HasPrefix(resName, "tpu-") || strings.HasPrefix(resName, "tpu7"):
		flavorName = "flavor-" + resName
		nodeLabels = map[string]string{
			"cloud.google.com/gke-tpu-accelerator": resName,
		}
	default:
		return "", nil, fmt.Errorf("unknown accelerator label %q", resName)
	}

	return flavorName, nodeLabels, nil
}

func (g *GKEOrchestrator) populateNAPFlavors(flavors map[string]FlavorCapacity) error {
	if !g.napEnabled {
		return nil
	}

	for resName, maxLimit := range g.napLimits {
		if maxLimit <= 0 {
			continue
		}
		if isNonAcceleratorResource(resName) {
			continue
		}

		flavorName, nodeLabels, err := resolveFlavorFromResource(resName)
		if err != nil {
			return err
		}

		if _, ok := flavors[flavorName]; !ok {
			flavors[flavorName] = FlavorCapacity{
				NodeLabels: nodeLabels,
			}
		}
	}
	return nil
}
