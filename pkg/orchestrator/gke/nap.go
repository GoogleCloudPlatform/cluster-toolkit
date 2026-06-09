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

	corev1 "k8s.io/api/core/v1"
	k8syaml "sigs.k8s.io/yaml"
)

func (g *GKEOrchestrator) isNAPEnabledForMachineType(machineType, zone string) (bool, error) {
	if !g.napEnabled {
		return false, nil
	}

	resolvedType := config.ResolveMachineType(machineType)

	if config.IsTPU(resolvedType) {
		key := getTPULimitKey(resolvedType)
		return g.napLimits[key] > 0 || g.napLimits["google.com/tpu"] > 0, nil
	}

	cap, err := g.FetchMachineCapabilities(resolvedType, zone)
	if err != nil {
		return false, err
	}
	if len(cap.Accelerators) > 0 {
		key, err := getGPULimitKey(resolvedType, cap.Accelerators[0].Type)
		if err != nil {
			return false, err
		}
		return g.napLimits[key] > 0 || g.napLimits["nvidia.com/gpu"] > 0, nil
	}

	return g.napLimits["cpu"] > 0, nil
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

func getGPULimitKey(machineType string, accelLabel string) (string, error) {
	m := strings.ToLower(machineType)
	accel := strings.ToLower(accelLabel)
	if strings.Contains(m, "g2-standard") || strings.Contains(accel, "l4") {
		return "nvidia-l4", nil
	}
	if strings.Contains(m, "a3-highgpu") {
		return "nvidia-h100-80gb", nil
	}
	if strings.Contains(m, "a3-megagpu") {
		return "nvidia-h100-mega-80gb", nil
	}
	if strings.Contains(m, "a3-ultragpu") {
		return "nvidia-h200-141gb", nil
	}
	if strings.Contains(m, "a4-highgpu") {
		return "nvidia-b200", nil
	}
	if strings.Contains(m, "a4x-highgpu") {
		return "nvidia-gb200", nil
	}
	if strings.Contains(m, "a2-high") || strings.Contains(m, "a2-mega") || strings.Contains(m, "a2-ultra") || strings.Contains(accel, "a100") {
		return "nvidia-tesla-a100", nil
	}
	if accel != "" {
		return "", fmt.Errorf("unknown accelerator label: %q", accelLabel)
	}
	return "nvidia.com/gpu", nil
}

func matchStaticNodePool(np gkeJobNodePool, job *orchestrator.JobDefinition) bool {
	if !strings.EqualFold(np.Config.MachineType, job.MachineType) {
		return false
	}
	switch job.GKENAPProvisioning {
	case "spot":
		return np.Config.Labels["cloud.google.com/gke-provisioning"] == "spot"
	case "on-demand":
		val := np.Config.Labels["cloud.google.com/gke-provisioning"]
		return val == "standard" || val == ""
	case "reservation":
		shortResName := extractShortReservationName(job.GKENAPReservation)
		lblVal := np.Config.Labels["cloud.google.com/reservation-name"]
		return lblVal != "" && extractShortReservationName(lblVal) == shortResName
	default:
		return false
	}
}

func (g *GKEOrchestrator) checkNAPFlagsSupported(hasNAPFlags bool, job *orchestrator.JobDefinition) error {
	if !g.napEnabled && hasNAPFlags {
		return fmt.Errorf("GKE NAP provisioning options (--gke-nap-provisioning=%q, --gke-nap-reservation=%q) are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled. The current cluster does not have NAP enabled.\nRemediation: Enable Node Auto-Provisioning on your cluster to use these options, or submit your job without them", job.GKENAPProvisioning, job.GKENAPReservation)
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
	return fmt.Errorf("workload submission rejected. Compute type %q is not configured within your cluster's Node Auto-Provisioning (NAP) limits, and no matching static node pools exist. Configured limits on cluster: %s", computeType, strings.Join(configuredLimits, ", "))
}

func getSubmissionRejectError(job *orchestrator.JobDefinition) error {
	switch job.GKENAPProvisioning {
	case "spot":
		return fmt.Errorf("workload submission rejected. You requested the '--gke-nap-provisioning=spot' option for compute type %q, but the cluster's static node pools for this hardware are configured exclusively as Standard/On-Demand.\nRemediation: Please re-submit your job without the '--gke-nap-provisioning=spot' setting, or enable Node Auto-Provisioning (NAP) limits for this hardware on your cluster to allow dynamic scale-up of Spot resources", job.ComputeType)
	case "on-demand":
		return fmt.Errorf("workload submission rejected. You requested the '--gke-nap-provisioning=on-demand' option for compute type %q, but the cluster's static node pools for this hardware are configured exclusively as Spot.\nRemediation: Please re-submit your job with the '--gke-nap-provisioning=spot' setting, or enable Node Auto-Provisioning (NAP) limits for this hardware on your cluster to allow dynamic scale-up of On-Demand resources", job.ComputeType)
	case "reservation":
		return fmt.Errorf("workload submission rejected. You requested the '--gke-nap-provisioning=reservation' with reservation name %q for compute type %q, but no static node pools matching this hardware are configured to consume this reservation", job.GKENAPReservation, job.ComputeType)
	default:
		return fmt.Errorf("workload submission rejected. No active node pools found matching your consumption model constraints")
	}
}

func (g *GKEOrchestrator) validateConsumptionForStaticCluster(job *orchestrator.JobDefinition) error {
	hasNAPFlags := job.GKENAPProvisioning != "" || job.GKENAPReservation != ""

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
	if isNAP {
		return nil // Validated: GKE NAP can dynamically autoprovision this machine type
	}

	// Fallback: Check if there is a matching static node pool that satisfies the requested consumption model.
	matchedNodePoolFound := false
	for _, np := range g.clusterDesc.NodePools {
		if strings.EqualFold(np.Config.MachineType, job.MachineType) {
			matchedNodePoolFound = true
			if matchStaticNodePool(np, job) {
				return nil // Valid static node pool path exists
			}
		}
	}

	if !matchedNodePoolFound {
		return g.getConfiguredLimitsError(job.ComputeType)
	}

	return getSubmissionRejectError(job)
}

func (g *GKEOrchestrator) resolveReservationTolerations(machineType, reservationName string) []corev1.Toleration {
	var tolerations []corev1.Toleration
	// Always add the standard GKE reservation toleration to support NAP where the node pool may not exist yet
	tolerations = append(tolerations, corev1.Toleration{
		Key:      "cloud.google.com/reservation-name",
		Operator: corev1.TolerationOpEqual,
		Value:    extractShortReservationName(reservationName),
		Effect:   corev1.TaintEffectNoSchedule,
	})

	seenTaints := map[string]bool{
		"cloud.google.com/reservation-name": true,
	}

	shortResName := extractShortReservationName(reservationName)
	for _, np := range g.clusterDesc.NodePools {
		lblVal := np.Config.Labels["cloud.google.com/reservation-name"]
		if lblVal == "" {
			continue
		}
		if strings.EqualFold(np.Config.MachineType, machineType) && extractShortReservationName(lblVal) == shortResName {
			for _, t := range np.Config.Taints {
				// Avoid duplicate tolerations
				if seenTaints[t.Key] {
					continue
				}
				seenTaints[t.Key] = true
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
	// Copy the slice to avoid mutating any shared underlying array returned by GetTolerations
	tolerations := append([]corev1.Toleration(nil), GetTolerations(acceleratorType)...)

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

func extractShortReservationName(resName string) string {
	if strings.Contains(resName, "/") {
		parts := strings.Split(resName, "/")
		return parts[len(parts)-1]
	}
	return resName
}
