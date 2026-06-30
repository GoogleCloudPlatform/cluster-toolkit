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
	"bytes"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"strings"

	"github.com/google/safetext/yamltemplate"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8syaml "sigs.k8s.io/yaml"
)

func (g *GKEOrchestrator) GenerateGKEManifest(opts ManifestOptions, profile JobProfile) (string, error) {
	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(opts, profile)
	if err != nil {
		return "", fmt.Errorf("failed to calculate resource limits: %w", err)
	}

	if opts.ComputeType != "" && gpuLimit == "" && tpuLimit == "" {
		logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.ComputeType)
		opts.ComputeType = ""
	}

	resourcesString, err := g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit, 16)
	if err != nil {
		return "", err
	}

	cmdSlice := []string{"/bin/bash", "-c", opts.CommandToRun}

	tmpl, err := yamltemplate.New("jobset.tmpl").ParseFS(templatesFS, "templates/jobset.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse jobset template: %w", err)
	}

	isTPU := tpuLimit != ""
	isGPU := gpuLimit != ""
	data := g.prepareJobSetTemplateData(opts, cmdSlice, resourcesString, isTPU, isGPU)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute jobset template: %w", err)
	}
	return assembleManifest(buf.String(), opts.AdditionalManifests), nil
}

func (g *GKEOrchestrator) buildResourcesString(cpu, mem, gpu, tpu string, indent int) (string, error) {
	limits := corev1.ResourceList{}
	if cpu != "" {
		q, err := resource.ParseQuantity(cpu)
		if err != nil {
			return "", fmt.Errorf("failed to parse CPU quantity %q: %w", cpu, err)
		}
		limits[corev1.ResourceCPU] = q
	}
	if mem != "" {
		q, err := resource.ParseQuantity(mem)
		if err != nil {
			return "", fmt.Errorf("failed to parse memory quantity %q: %w", mem, err)
		}
		limits[corev1.ResourceMemory] = q
	}
	if gpu != "" {
		q, err := resource.ParseQuantity(gpu)
		if err != nil {
			return "", fmt.Errorf("failed to parse GPU quantity %q: %w", gpu, err)
		}
		limits[corev1.ResourceName("nvidia.com/gpu")] = q
	}
	if tpu != "" {
		q, err := resource.ParseQuantity(tpu)
		if err != nil {
			return "", fmt.Errorf("failed to parse TPU quantity %q: %w", tpu, err)
		}
		limits[corev1.ResourceName("google.com/tpu")] = q
	}

	if len(limits) == 0 {
		return "", nil
	}

	resources := corev1.ResourceRequirements{
		Limits: limits,
	}

	b, err := k8syaml.Marshal(resources)
	if err != nil {
		return "", fmt.Errorf("failed to marshal resources: %w", err)
	}

	resourcesStr := "resources:\n" + indentYaml(string(b), 2)
	return indentYaml(resourcesStr, indent), nil
}

func (g *GKEOrchestrator) PrepareManifestOptions(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) (ManifestOptions, error) {
	originalAccelType := job.ComputeType

	schedOpts := SchedulingOptions{
		PlacementPolicy:    job.PlacementPolicy,
		NodeAffinityLabels: job.NodeConstraint,
		Topology:           job.Topology,
		Scheduler:          job.GKEScheduler,
		IsDynamicSlicing:   isDynamicSlicing,
		IsStaticSlicing:    isStaticSlicing,
	}

	parts := strings.Split(originalAccelType, "-")
	instanceType := parts[0]

	// Reuse GCluster's existing GKE accelerator label mapping and algorithmically
	// derive the Pathways short platform key to avoid duplicating mapping tables.
	gkeLabel := g.GenerateGKENodeSelectorLabel(instanceType)
	// Normalize GKE node selector labels to match JAX/Pathways platform keys:
	// 1. Map GKE "v5-lite" (TPU v5e) to JAX standard "v5e" (deriving tpuv5e)
	// 2. Map GKE "v5p" (TPU v5p) to JAX standard "v5" (deriving tpuv5)
	normalizedLabel := gkeLabel
	if strings.Contains(gkeLabel, "v5-lite") {
		normalizedLabel = strings.ReplaceAll(gkeLabel, "v5-lite", "v5e")
	} else if strings.Contains(gkeLabel, "v5p") {
		normalizedLabel = strings.ReplaceAll(gkeLabel, "v5p", "v5")
	}
	pathwaysPlatform := strings.ReplaceAll(normalizedLabel, "-podslice", "")
	pathwaysPlatform = strings.ReplaceAll(pathwaysPlatform, "-slice", "")
	pathwaysPlatform = strings.ReplaceAll(pathwaysPlatform, "-", "")

	pathwaysInstanceType := fmt.Sprintf("%s:%s", pathwaysPlatform, schedOpts.Topology)

	opts := ManifestOptions{
		IsDynamicSlicing:              isDynamicSlicing,
		IsStaticSlicing:               isStaticSlicing,
		WorkloadName:                  job.WorkloadName,
		FullImageName:                 fullImageName,
		CommandToRun:                  job.CommandToRun,
		ComputeType:                   job.ComputeType,
		MachineType:                   job.MachineType,
		PathwaysInstanceType:          pathwaysInstanceType,
		ProjectID:                     job.ProjectID,
		ClusterName:                   job.ClusterName,
		ClusterLocation:               job.ClusterLocation,
		KueueQueueName:                job.KueueQueueName,
		NumSlices:                     job.NumSlices,
		NodesPerSlice:                 job.NodesPerSlice,
		MaxRestarts:                   job.MaxRestarts,
		TtlSecondsAfterFinished:       job.TtlSecondsAfterFinished,
		TerminationGracePeriodSeconds: job.TerminationGracePeriodSeconds,
		ServiceAccountName:            job.ServiceAccountName,
		SchedulerName:                 job.GKEScheduler,
		AwaitJobCompletion:            job.AwaitJobCompletion,
		PriorityClassName:             job.PriorityClassName,
		Topology:                      schedOpts.Topology,
		Verbose:                       job.Verbose,
	}

	if err := g.fillManifestStrings(&opts, schedOpts, job, isDynamicSlicing, isStaticSlicing, profile.IsCPUMachine); err != nil {
		return ManifestOptions{}, err
	}

	sm := &StorageManager{orchestrator: g}
	mountInfos, manifests, err := sm.ProcessMounts(job.RawMounts, job)
	if err != nil {
		return ManifestOptions{}, err
	}
	opts.AdditionalManifests = manifests

	sm.AddVolumeOptions(&opts, mountInfos)

	_, err = g.resolveResourcesAndGates(&opts, profile.IsCPUMachine, profile.CapacityCount, job)
	if err != nil {
		return ManifestOptions{}, err
	}

	return opts, nil
}

func (g *GKEOrchestrator) fillManifestStrings(opts *ManifestOptions, schedOpts SchedulingOptions, job orchestrator.JobDefinition, isDynamicSlicing bool, isStaticSlicing bool, isCPUMachine bool) error {
	nodeSelectorStr, err := g.buildNodeSelector(schedOpts, job, isCPUMachine)
	if err != nil {
		return err
	}
	opts.NodeSelector = nodeSelectorStr

	affinityStr, err := g.buildAffinity(schedOpts)
	if err != nil {
		return err
	}
	opts.Affinity = affinityStr

	podFailurePolicyStr, err := g.generatePodFailurePolicy(job.RestartOnExitCodes)
	if err != nil {
		return err
	}
	opts.PodFailurePolicy = indentYaml(podFailurePolicyStr, 12)

	imagePullSecretsStr := g.generateImagePullSecrets(job.ImagePullSecrets)
	if imagePullSecretsStr != "" {
		opts.ImagePullSecrets = indentYaml(imagePullSecretsStr, 16)
	}

	isSubSlicing := isDynamicSlicing || isStaticSlicing
	opts.TopologyAnnotation = g.buildTopologyAnnotation(schedOpts.Topology, job.MachineType, job.NumSlices, job.NodesPerSlice, isSubSlicing)

	tolerationsStr, err := g.resolveTolerations(job.MachineType, job.GKENAPProvisioning, job.GKENAPReservation, 16)
	if err != nil {
		return err
	}
	opts.Tolerations = tolerationsStr

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

	switch consumptionModel {
	case "spot":
		tolerations = append(tolerations, corev1.Toleration{
			Key:      "cloud.google.com/gke-provisioning",
			Operator: corev1.TolerationOpEqual,
			Value:    "spot",
			Effect:   corev1.TaintEffectNoSchedule,
		})
	case "reservation":
		tolerations = append(tolerations, g.resolveReservationTolerations(acceleratorType, reservationName)...)
	}

	if len(tolerations) == 0 {
		return "", nil
	}
	b, err := k8syaml.Marshal(tolerations)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tolerations: %w", err)
	}
	return indentYaml(string(b), indent), nil
}

func assembleManifest(mainManifest string, additionalManifests []string) string {
	var resManifest strings.Builder
	for _, m := range additionalManifests {
		trimmed := strings.TrimSpace(m)
		if trimmed == "" {
			continue
		}
		resManifest.WriteString(trimmed)
		resManifest.WriteString("\n---\n")
	}
	resManifest.WriteString(strings.TrimSpace(mainManifest))
	return resManifest.String()
}

func indentYaml(s string, indent int) string {
	lines := strings.Split(s, "\n")
	padding := strings.Repeat(" ", indent)
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, padding+line)
		}
	}
	return strings.Join(result, "\n")
}
