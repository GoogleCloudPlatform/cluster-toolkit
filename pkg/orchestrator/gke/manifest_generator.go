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
	"strconv"
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

	if opts.AcceleratorType != "" && gpuLimit == "" && tpuLimit == "" {
		logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.AcceleratorType)
		opts.AcceleratorType = ""
	}

	resourcesString, err := g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit)
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

func (g *GKEOrchestrator) buildResourcesString(cpu, mem, gpu, tpu string) (string, error) {
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

	return g.indentYaml("resources:\n"+string(b), 16), nil
}

func (g *GKEOrchestrator) PrepareManifestOptions(job orchestrator.JobDefinition, fullImageName string) (ManifestOptions, JobProfile, error) {
	originalAccelType := job.AcceleratorType
	if err := g.resolveAcceleratorShorthand(&job); err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	mappedLabel := g.GenerateGKENodeSelectorLabel(job.AcceleratorType)

	schedOpts, isSuperSlicing, err := g.resolveSchedulingAndTopology(&job, mappedLabel)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	isCPUMachine, capacity, err := g.determineIfCPUMachine(job)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	if job.IsPathwaysJob && originalAccelType == "" {
		return ManifestOptions{}, JobProfile{}, fmt.Errorf("accelerator type is required for Pathways workloads")
	}

	parts := strings.Split(originalAccelType, "-")
	instanceType := parts[0]
	pathwaysInstanceType := fmt.Sprintf("%s:%s", instanceType, schedOpts.Topology)

	opts := ManifestOptions{
		IsSuperSlicing:                isSuperSlicing,
		WorkloadName:                  job.WorkloadName,
		FullImageName:                 fullImageName,
		CommandToRun:                  job.CommandToRun,
		AcceleratorType:               job.AcceleratorType,
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

	if err := g.fillManifestStrings(&opts, schedOpts, job, isSuperSlicing, isCPUMachine); err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	g.addVolumeOptions(&opts, job.Volumes)

	profile, err := g.resolveResourcesAndGates(&opts, isCPUMachine, capacity, job)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	return opts, profile, nil
}

func (g *GKEOrchestrator) resolveAcceleratorShorthand(job *orchestrator.JobDefinition) error {
	if job.AcceleratorType == "" {
		return nil
	}

	// Prioritize shorthands over total-chip requests to avoid collisions (e.g., v6e-8)
	if _, exists := acceleratorShorthandMap[job.AcceleratorType]; exists {
		return nil
	}

	parts := strings.Split(job.AcceleratorType, "-")
	if len(parts) != 2 {
		return nil
	}

	prefix := parts[0]
	totalChipsStr := parts[1]

	totalChips, err := strconv.Atoi(totalChipsStr)
	if err != nil || totalChips <= 0 {
		return nil
	}

	logging.Info("Detected accelerator shorthand request: %s", job.AcceleratorType)

	machineType, err := g.queryMachineType()
	if err != nil {
		return err
	}
	if machineType == "" {
		return fmt.Errorf("could not auto-discover machine type because no active nodes were found. If this is an auto-provisioning (NAP) cluster, please specify the exact base accelerator unit (e.g., --accelerator v6e-8) instead of the total shape (%s)", job.AcceleratorType)
	}

	chipsPerVM, err := g.FetchMachineCapacity(machineType, job.ClusterLocation)
	if err != nil {
		return fmt.Errorf("failed to fetch capacity for machine type %s: %w", machineType, err)
	}

	job.VmsPerSlice = totalChips / chipsPerVM
	if job.VmsPerSlice == 0 {
		job.VmsPerSlice = 1
	}

	topology, err := g.resolveTopologyForChips(prefix, totalChips)
	if err != nil {
		return err
	}
	job.Topology = topology

	job.AcceleratorType = g.GenerateGKENodeSelectorLabel(prefix)

	return nil
}

func (g *GKEOrchestrator) dynamicallyCalculateVmsPerSlice(job *orchestrator.JobDefinition, topology, mappedLabel string) {
	if job.VmsPerSlice <= 0 && topology != "" && strings.Contains(strings.ToLower(mappedLabel), "tpu") {
		machineType := g.resolveMachineName(job.AcceleratorType)
		chipsPerVM, err := g.FetchMachineCapacity(machineType, job.ClusterLocation)
		if err != nil {
			logging.Warn("Failed to fetch machine capacity for %s: %v", machineType, err)
			return
		}
		if chipsPerVM > 0 {
			dims := strings.Split(topology, "x")
			totalChips := 1
			for _, dim := range dims {
				if val, err := strconv.Atoi(dim); err == nil {
					totalChips *= val
				}
			}
			job.VmsPerSlice = totalChips / chipsPerVM
			logging.Info("Dynamically determined vms_per_slice for %s: %d", topology, job.VmsPerSlice)
		}
	}
	if job.VmsPerSlice <= 0 {
		job.VmsPerSlice = 1
	}
}

func (g *GKEOrchestrator) resolveTolerations(acceleratorType string) (string, error) {
	tolerations := GetTolerations(acceleratorType)
	if len(tolerations) == 0 {
		return "", nil
	}
	b, err := k8syaml.Marshal(tolerations)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tolerations: %w", err)
	}
	return g.indentYaml(string(b), 16), nil
}

func (g *GKEOrchestrator) resolveSchedulingAndTopology(job *orchestrator.JobDefinition, mappedLabel string) (SchedulingOptions, bool, error) {
	schedOpts := SchedulingOptions{
		PlacementPolicy:    job.PlacementPolicy,
		NodeAffinityLabels: job.NodeConstraint,
		Topology:           job.Topology,
		Scheduler:          job.GKEScheduler,
	}

	topology, err := g.resolveTopology(job.Topology, mappedLabel, job.ClusterName, job.ClusterLocation)
	if err != nil {
		return SchedulingOptions{}, false, err
	}
	schedOpts.Topology = topology

	g.dynamicallyCalculateVmsPerSlice(job, topology, mappedLabel)

	isSuperSlicing, err := g.verifySuperSlicingActive(ManifestOptions{
		ClusterName:     job.ClusterName,
		ClusterLocation: job.ClusterLocation,
		AcceleratorType: job.AcceleratorType,
	})
	if err != nil {
		logging.Warn("Failed to verify if Super-slicing is active: %v. Assuming not active.", err)
	}

	return schedOpts, isSuperSlicing, nil
}

func (g *GKEOrchestrator) fillManifestStrings(opts *ManifestOptions, schedOpts SchedulingOptions, job orchestrator.JobDefinition, isSuperSlicing, isCPUMachine bool) error {
	nodeSelectorStr, err := g.buildNodeSelector(schedOpts, job, isSuperSlicing, isCPUMachine)
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

	tolerationsStr, err := g.resolveTolerations(job.AcceleratorType)
	if err != nil {
		return err
	}
	opts.Tolerations = tolerationsStr

	return nil
}

func (g *GKEOrchestrator) resolveResourcesAndGates(opts *ManifestOptions, isCPUMachine bool, capacity int, job orchestrator.JobDefinition) (JobProfile, error) {
	isGPU := !isCPUMachine && !IsTPU(job.AcceleratorType)
	if isGPU && job.GKEScheduler == "gke.io/topology-aware-auto" {
		opts.SchedulingGates = g.indentYaml("schedulingGates:\n  - name: \"gke.io/topology-aware-auto-"+job.WorkloadName+"\"", 14)
		opts.SchedulerName = ""
	}

	profile := JobProfile{
		IsCPUMachine:  isCPUMachine,
		CapacityCount: capacity,
	}

	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(*opts, profile)
	if err != nil {
		logging.Warn("Warning: failed to calculate resource limits: %v", err)
	} else {
		if opts.AcceleratorType != "" && gpuLimit == "" && tpuLimit == "" {
			logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.AcceleratorType)
			opts.AcceleratorType = ""
		}
		resStr, err := g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit)
		if err != nil {
			return profile, err
		}
		opts.ResourcesString = resStr
	}

	return profile, nil
}
