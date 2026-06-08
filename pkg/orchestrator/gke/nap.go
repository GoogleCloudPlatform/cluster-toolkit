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
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8syaml "sigs.k8s.io/yaml"
)

func (g *GKEOrchestrator) isNAPEnabledForMachineType(machineType, zone string) bool {
	if !g.napEnabled {
		return false
	}

	resolvedType := config.ResolveMachineType(machineType)

	if config.IsTPU(resolvedType) {
		key := getTPULimitKey(resolvedType)
		if key != "google.com/tpu" {
			return g.napLimits[key] > 0
		}
		return g.napLimits["google.com/tpu"] > 0
	}

	cap, err := g.FetchMachineCapabilities(resolvedType, zone)
	if err == nil && len(cap.Accelerators) > 0 {
		key := getGPULimitKey(resolvedType, cap.Accelerators[0].Type)
		if key != "nvidia.com/gpu" {
			return g.napLimits[key] > 0
		}
		return g.napLimits["nvidia.com/gpu"] > 0
	}

	return g.napLimits["cpu"] > 0
}

func getTPULimitKey(machineType string) string {
	m := strings.ToLower(machineType)
	if strings.Contains(m, "v6e") || strings.Contains(m, "ct6e") {
		return "tpu-v6e-slice"
	}
	if strings.Contains(m, "v5litepod") || strings.Contains(m, "v5e") || strings.Contains(m, "ct5lp") {
		return "tpu-v5e-slice"
	}
	if strings.Contains(m, "v5p") || strings.Contains(m, "ct5p") {
		return "tpu-v5p-slice"
	}
	if strings.Contains(m, "v4") || strings.Contains(m, "ct4p") {
		return "tpu-v4-podslice"
	}
	return "google.com/tpu"
}

func getGPULimitKey(machineType string, accelLabel string) string {
	m := strings.ToLower(machineType)
	if strings.Contains(m, "g2-standard") || strings.Contains(accelLabel, "l4") {
		return "nvidia-l4"
	}
	if strings.Contains(m, "a3-highgpu") {
		return "nvidia-h100-80gb"
	}
	if strings.Contains(m, "a3-megagpu") {
		return "nvidia-h100-mega-80gb"
	}
	if strings.Contains(m, "a3-ultragpu") {
		return "nvidia-h200-141gb"
	}
	if strings.Contains(m, "a4-highgpu") {
		return "nvidia-b200"
	}
	if strings.Contains(m, "a4x-highgpu") {
		return "nvidia-gb200"
	}
	if strings.Contains(m, "a2-high") || strings.Contains(m, "a2-mega") || strings.Contains(m, "a2-ultra") || strings.Contains(accelLabel, "a100") {
		return "nvidia-tesla-a100"
	}
	return "nvidia.com/gpu"
}

func matchesProvisioningModel(np gkeJobNodePool, provisioning string, reservationName string) bool {
	labels := np.Config.Labels
	switch provisioning {
	case "spot":
		return labels["cloud.google.com/gke-provisioning"] == "spot"
	case "reservation":
		return labels["cloud.google.com/reservation-name"] == reservationName
	case "on-demand", "":
		val := labels["cloud.google.com/gke-provisioning"]
		return val == "standard" || val == ""
	}
	return false
}

func (g *GKEOrchestrator) validateConsumptionForStaticCluster(job *orchestrator.JobDefinition) error {
	if !g.napEnabled {
		if (job.GKENAPProvisioning != "" && job.GKENAPProvisioning != "on-demand") || job.GKENAPReservation != "" {
			return fmt.Errorf("GKE NAP provisioning options (--gke-nap-provisioning=%q, --gke-nap-reservation=%q) are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled. The current cluster does not have NAP enabled.\nRemediation: Enable Node Auto-Provisioning on your cluster to use these options, or submit your job without them", job.GKENAPProvisioning, job.GKENAPReservation)
		}
		return nil
	}

	if g.isNAPEnabledForMachineType(job.MachineType, job.ClusterLocation) {
		return nil // Skip check; GKE NAP can dynamically autoprovision this machine type
	}

	matchedNodePoolFound := false
	for _, np := range g.clusterDesc.NodePools {
		if strings.EqualFold(np.Config.MachineType, job.MachineType) {
			matchedNodePoolFound = true
			if matchesProvisioningModel(np, job.GKENAPProvisioning, job.GKENAPReservation) {
				return nil // Valid static node pool path exists
			}
		}
	}

	if !matchedNodePoolFound {
		if g.napEnabled {
			return fmt.Errorf("workload submission rejected. Compute type %q is not configured within your cluster's Node Auto-Provisioning (NAP) limits, and no matching static node pools exist", job.ComputeType)
		}
		return fmt.Errorf("no active node pools in cluster match requested compute-type %q", job.ComputeType)
	}

	// Print dynamic console warnings
	switch job.GKENAPProvisioning {
	case "spot":
		return fmt.Errorf("workload submission rejected. You requested the '--gke-nap-provisioning=spot' option for compute type %q, but the cluster's static node pools for this hardware are configured exclusively as Standard/On-Demand.\nRemediation: Please re-submit your job without the '--gke-nap-provisioning=spot' setting, or enable Node Auto-Provisioning (NAP) limits for this hardware on your cluster to allow dynamic scale-up of Spot resources", job.ComputeType)
	case "reservation":
		return fmt.Errorf("workload submission rejected. You requested the '--gke-nap-provisioning=reservation' with reservation name %q for compute type %q, but no static node pools matching this hardware are configured to consume this reservation", job.GKENAPReservation, job.ComputeType)
	default:
		return fmt.Errorf("workload submission rejected. No active node pools found matching your consumption model constraints")
	}
}

func (g *GKEOrchestrator) resolveReservationTolerations(machineType, reservationName string) []corev1.Toleration {
	var tolerations []corev1.Toleration
	for _, np := range g.clusterDesc.NodePools {
		if strings.EqualFold(np.Config.MachineType, machineType) && np.Config.Labels["cloud.google.com/reservation-name"] == reservationName {
			for _, t := range np.Config.Taints {
				tolerations = append(tolerations, corev1.Toleration{
					Key:      t.Key,
					Value:    t.Value,
					Effect:   corev1.TaintEffect(t.Effect),
					Operator: corev1.TolerationOpEqual,
				})
			}
		}
	}
	return tolerations
}

func (g *GKEOrchestrator) resolveTolerations(acceleratorType string, consumptionModel string, reservationName string) (string, error) {
	tolerations := GetTolerations(acceleratorType)

	if consumptionModel == "spot" {
		tolerations = append(tolerations, corev1.Toleration{
			Key:      "cloud.google.com/gke-provisioning",
			Operator: corev1.TolerationOpEqual,
			Value:    "spot",
			Effect:   corev1.TaintEffectNoSchedule,
		})
	} else if consumptionModel == "reservation" {
		tolerations = append(tolerations, g.resolveReservationTolerations(acceleratorType, reservationName)...)
	}

	if len(tolerations) == 0 {
		return "", nil
	}
	b, err := k8syaml.Marshal(tolerations)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tolerations: %w", err)
	}
	return g.indentYaml(string(b), 16), nil
}
