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
		key := config.GetTPULimitKey(resolvedType)
		return g.napLimits[key] > 0 || g.napLimits["google.com/tpu"] > 0, nil
	}

	cap, err := g.FetchMachineCapabilities(resolvedType, zone)
	if err != nil {
		return false, err
	}
	if len(cap.Accelerators) > 0 {
		key, err := config.GetGPULimitKey(resolvedType, cap.Accelerators[0].Type)
		if err != nil {
			return false, err
		}
		return g.napLimits[key] > 0 || g.napLimits["nvidia.com/gpu"] > 0, nil
	}

	return g.napLimits["cpu"] > 0, nil
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
	return fmt.Errorf("workload submission rejected. Compute type %q is not configured within your cluster's Node Auto-Provisioning (NAP) limits. Configured limits on cluster: %s", computeType, strings.Join(configuredLimits, ", "))
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
	if !isNAP {
		return g.getConfiguredLimitsError(job.ComputeType)
	}

	return nil
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
		if strings.EqualFold(np.Config.MachineType, machineType) && strings.EqualFold(extractShortReservationName(lblVal), shortResName) {
			tolerations[0].Value = extractShortReservationName(lblVal)
			for _, t := range np.Config.Taints {
				// Avoid duplicate tolerations
				if seenTaints[t.Key] {
					continue
				}
				seenTaints[t.Key] = true
				var effect corev1.TaintEffect
				switch strings.ToUpper(t.Effect) {
				case "NO_SCHEDULE":
					effect = corev1.TaintEffectNoSchedule
				case "PREFER_NO_SCHEDULE":
					effect = corev1.TaintEffectPreferNoSchedule
				case "NO_EXECUTE":
					effect = corev1.TaintEffectNoExecute
				default:
					effect = corev1.TaintEffect(t.Effect)
				}
				tolerations = append(tolerations, corev1.Toleration{
					Key:      t.Key,
					Value:    t.Value,
					Effect:   effect,
					Operator: corev1.TolerationOpEqual,
				})
			}
		}
	}
	return tolerations
}

func (g *GKEOrchestrator) resolveTolerations(acceleratorType string, consumptionModel string, reservationName string, indent int) (string, error) {
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
	return g.indentYaml(string(b), indent), nil
}

// extractShortReservationName extracts the reservation name from a GCE reservation resource URI or reservation path.
// It handles standard URIs, simple names, and paths containing reservationBlocks/reservationSubBlocks.
// E.g.,
// - "my-res" -> "my-res"
// - "projects/my-project/reservations/my-res" -> "my-res"
// - "projects/my-project/reservations/my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2" -> "my-res"
// - "my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2" -> "my-res"
func extractShortReservationName(resName string) string {
	if !strings.Contains(resName, "/") {
		return resName
	}

	parts := strings.Split(resName, "/")

	// Case 1: Full GCP URI containing ".../reservations/RESERVATION_NAME/..."
	for i, part := range parts {
		if part == "reservations" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	// Case 2: Path containing ".../reservationBlocks/..." but not "reservations" prefix
	for i, part := range parts {
		if part == "reservationBlocks" && i > 0 {
			return parts[i-1]
		}
	}

	// Fallback: Return the last segment
	return parts[len(parts)-1]
}
