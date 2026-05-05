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
	"text/template"

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

	tmpl, err := template.ParseFS(templatesFS, "templates/jobset.tmpl")
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
	return buf.String(), nil
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

	resourcesStr := "resources:\n" + g.indentYaml(string(b), 2)
	return g.indentYaml(resourcesStr, indent), nil
}

func (g *GKEOrchestrator) PrepareManifestOptions(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool) (ManifestOptions, error) {
	originalAccelType := job.ComputeType

	schedOpts := SchedulingOptions{
		PlacementPolicy:    job.PlacementPolicy,
		NodeAffinityLabels: job.NodeConstraint,
		Topology:           job.Topology,
		Scheduler:          job.GKEScheduler,
	}

	parts := strings.Split(originalAccelType, "-")
	instanceType := parts[0]
	pathwaysInstanceType := fmt.Sprintf("%s:%s", instanceType, schedOpts.Topology)

	opts := ManifestOptions{
		IsDynamicSlicing:              isDynamicSlicing,
		WorkloadName:                  job.WorkloadName,
		FullImageName:                 fullImageName,
		CommandToRun:                  job.CommandToRun,
		ComputeType:                   job.ComputeType,
		PathwaysInstanceType:          pathwaysInstanceType,
		ProjectID:                     job.ProjectID,
		ClusterName:                   job.ClusterName,
		ClusterLocation:               job.ClusterLocation,
		KueueQueueName:                job.KueueQueueName,
		NumSlices:                     job.NumSlices,
		VmsPerSlice:                   job.VmsPerSlice,
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

	if err := g.fillManifestStrings(&opts, schedOpts, job, isDynamicSlicing, profile.IsCPUMachine); err != nil {
		return ManifestOptions{}, err
	}

	g.addVolumeOptions(&opts, job.Volumes)

	_, err := g.resolveResourcesAndGates(&opts, profile.IsCPUMachine, profile.CapacityCount, job)
	if err != nil {
		return ManifestOptions{}, err
	}

	return opts, nil
}

func (g *GKEOrchestrator) fillManifestStrings(opts *ManifestOptions, schedOpts SchedulingOptions, job orchestrator.JobDefinition, isDynamicSlicing, isCPUMachine bool) error {
	nodeSelectorStr, err := g.buildNodeSelector(schedOpts, job, isDynamicSlicing, isCPUMachine)
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
	opts.PodFailurePolicy = g.indentYaml(podFailurePolicyStr, 12)

	imagePullSecretsStr := g.generateImagePullSecrets(job.ImagePullSecrets)
	if imagePullSecretsStr != "" {
		opts.ImagePullSecrets = g.indentYaml(imagePullSecretsStr, 16)
	}

	opts.TopologyAnnotation = g.buildTopologyAnnotation(schedOpts.Topology)

	tolerationsStr, err := g.resolveTolerations(job.MachineType)
	if err != nil {
		return err
	}
	opts.Tolerations = tolerationsStr

	return nil
}
