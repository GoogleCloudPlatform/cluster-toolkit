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
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"strconv"
	"strings"
	"text/template"

	k8syaml "sigs.k8s.io/yaml"
)

func (g *GKEOrchestrator) GenerateGKEManifest(opts ManifestOptions, profile JobProfile) (string, error) {
	g.setManifestDefaults(&opts)

	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(opts, profile)
	if err != nil {
		return "", fmt.Errorf("failed to calculate resource limits: %w", err)
	}

	if opts.AcceleratorType != "" && gpuLimit == "" && tpuLimit == "" {
		logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.AcceleratorType)
		opts.AcceleratorType = ""
	}

	resourcesString := g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit)

	if opts.Verbose {
		if tpuLimit != "" {
			opts.CommandToRun = "export TPU_STDERR_LOG_LEVEL=0 && export TPU_MIN_LOG_LEVEL=0 && export TF_CPP_MIN_LOG_LEVEL=0 && export TPU_VMODULE=real_program_continuator=1 && " + opts.CommandToRun
		} else if gpuLimit != "" {
			opts.CommandToRun = "export NCCL_DEBUG=INFO && " + opts.CommandToRun
		}
	}

	cmdSlice := []string{"/bin/bash", "-c", opts.CommandToRun}
	var jsonBuf bytes.Buffer
	enc := json.NewEncoder(&jsonBuf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cmdSlice); err != nil {
		return "", fmt.Errorf("failed to marshal command: %w", err)
	}
	trimmedCmdBytes := bytes.TrimSpace(jsonBuf.Bytes())
	updatedCommand := fmt.Sprintf("                command: %s\n%s", string(trimmedCmdBytes), resourcesString)

	tmpl, err := template.ParseFS(templatesFS, "templates/jobset.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse jobset template: %w", err)
	}

	data := g.prepareJobSetTemplateData(opts, updatedCommand)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute jobset template: %w", err)
	}
	return buf.String(), nil
}

func (g *GKEOrchestrator) setManifestDefaults(opts *ManifestOptions) {
	if opts.WorkloadName == "" {
		opts.WorkloadName = "gcluster-workload-" + shell.RandomString(8)
	}
	if opts.KueueQueueName == "" {
		opts.KueueQueueName = "default-queue"
	}
	if opts.NumSlices == 0 {
		opts.NumSlices = 1
	}
	if opts.VmsPerSlice == 0 {
		opts.VmsPerSlice = 1
	}
	if opts.MaxRestarts == 0 {
		opts.MaxRestarts = 1
	}
	if opts.TtlSecondsAfterFinished == 0 {
		opts.TtlSecondsAfterFinished = 3600
	}
}

func (g *GKEOrchestrator) buildResourcesString(cpu, mem, gpu, tpu string) string {
	var limits []string
	if cpu != "" {
		limits = append(limits, fmt.Sprintf("                    cpu: %s", cpu))
	}
	if mem != "" {
		limits = append(limits, fmt.Sprintf("                    memory: %s", mem))
	}
	if gpu != "" {
		limits = append(limits, fmt.Sprintf("                    nvidia.com/gpu: %s", gpu))
	}
	if tpu != "" {
		limits = append(limits, fmt.Sprintf("                    google.com/tpu: %s", tpu))
	}

	if len(limits) > 0 {
		return "                resources:\n                  limits:\n" + strings.Join(limits, "\n")
	}
	return ""
}

func (g *GKEOrchestrator) prepareManifestOptions(job orchestrator.JobDefinition, fullImageName string) (ManifestOptions, JobProfile, error) {
	if err := g.resolveXPKStyleAccelerator(&job); err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	schedOpts := SchedulingOptions{
		PlacementPolicy:    job.PlacementPolicy,
		NodeAffinityLabels: job.NodeSelector,
		Topology:           job.Topology,
		Scheduler:          job.Scheduler,
	}

	mappedLabel := g.GenerateGKENodeSelectorLabel(job.AcceleratorType)
	topology, err := g.resolveTopology(job.Topology, mappedLabel, job.ClusterName, job.ClusterLocation)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}
	schedOpts.Topology = topology

	// Calculate VmsPerSlice dynamically for TPUs if not provided and topology is present.
	if job.VmsPerSlice <= 1 && topology != "" && strings.Contains(strings.ToLower(mappedLabel), "tpu") {
		machineType := g.resolveMachineName(job.AcceleratorType)
		chipsPerVM, err := g.FetchMachineCapacity(machineType, job.ClusterLocation)
		if err == nil && chipsPerVM > 0 {
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

	isSuperSlicing, _ := g.verifySuperSlicingActive(ManifestOptions{
		ClusterName:     job.ClusterName,
		ClusterLocation: job.ClusterLocation,
		AcceleratorType: job.AcceleratorType,
	})

	isCPUMachine, capacity, err := g.determineIfCPUMachine(job)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	nodeSelectorStr, err := g.buildNodeSelector(schedOpts, job, isSuperSlicing, isCPUMachine)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	affinityStr, err := g.buildAffinity(schedOpts)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}

	podFailurePolicyStr, err := g.generatePodFailurePolicy(job.RestartOnExitCodes)
	if err != nil {
		return ManifestOptions{}, JobProfile{}, err
	}
	podFailurePolicyStr = g.indentYaml(podFailurePolicyStr, 12)

	imagePullSecretsStr := g.generateImagePullSecrets(job.ImagePullSecrets)
	if imagePullSecretsStr != "" {
		imagePullSecretsStr = g.indentYaml(imagePullSecretsStr, 16)
	}

	topologyAnnotationStr := g.buildTopologyAnnotation(schedOpts.Topology)

	tolerations := GetTolerations(job.AcceleratorType)
	var tolerationsStr string
	if len(tolerations) > 0 {
		b, err := k8syaml.Marshal(tolerations)
		if err != nil {
			return ManifestOptions{}, JobProfile{}, fmt.Errorf("failed to marshal tolerations: %w", err)
		}
		tolerationsStr = g.indentYaml(string(b), 16)
	}

	opts := ManifestOptions{
		WorkloadName:            job.WorkloadName,
		FullImageName:           fullImageName,
		CommandToRun:            job.CommandToRun,
		AcceleratorType:         job.AcceleratorType,
		ProjectID:               job.ProjectID,
		ClusterName:             job.ClusterName,
		ClusterLocation:         job.ClusterLocation,
		KueueQueueName:          job.KueueQueueName,
		NumSlices:               job.NumSlices,
		VmsPerSlice:             job.VmsPerSlice,
		MaxRestarts:             job.MaxRestarts,
		TtlSecondsAfterFinished: job.TtlSecondsAfterFinished,
		NodeSelector:            nodeSelectorStr,
		Affinity:                affinityStr,
		PodFailurePolicy:        podFailurePolicyStr,
		ImagePullSecrets:        imagePullSecretsStr,
		ServiceAccountName:      job.ServiceAccountName,
		TopologyAnnotation:      topologyAnnotationStr,
		SchedulerName:           job.Scheduler,
		Tolerations:             tolerationsStr,
		AwaitJobCompletion:      job.AwaitJobCompletion,
		PriorityClassName:       job.PriorityClassName,
		Topology:                schedOpts.Topology,
		Verbose:                 job.Verbose,
	}

	g.addVolumeOptions(&opts, job.Volumes)

	profile := JobProfile{
		IsCPUMachine:  isCPUMachine,
		CapacityCount: capacity,
	}

	return opts, profile, nil
}

func (g *GKEOrchestrator) resolveXPKStyleAccelerator(job *orchestrator.JobDefinition) error {
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

	logging.Info("Detected XPK-style accelerator request: %s", job.AcceleratorType)

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
