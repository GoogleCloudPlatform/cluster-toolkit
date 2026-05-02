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
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"strings"

	k8syaml "sigs.k8s.io/yaml"
)

type MachineTypeCap struct {
	Accelerators []struct {
		Count int    `json:"guestAcceleratorCount"`
		Type  string `json:"guestAcceleratorType"`
	} `json:"accelerators"`
	GuestCpus int `json:"guestCpus"`
	MemoryMb  int `json:"memoryMb"`
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

		mt, err := g.machineTypeClient.GetMachineType(g.projectID, z, machineType)
		if err != nil {
			lastErr = err
			continue
		}

		logging.Info("Discovered machine capabilities in zone %s", z)

		cap := MachineTypeCap{
			GuestCpus: int(mt.GuestCpus),
			MemoryMb:  int(mt.MemoryMb),
		}

		count, accelType, _ := config.ResolveAcceleratorInfo(mt, machineType)
		if count > 0 {
			cap.Accelerators = append(cap.Accelerators, struct {
				Count int    `json:"guestAcceleratorCount"`
				Type  string `json:"guestAcceleratorType"`
			}{Count: count, Type: accelType})
		}

		if g.machineCapCache == nil {
			g.machineCapCache = make(map[string]MachineTypeCap)
		}
		g.machineCapCache[cacheKey] = cap
		// Also cache for the specific zone that succeeded
		specificKey := machineType + ":" + z
		g.machineCapCache[specificKey] = cap
		return cap, nil
	}

	if isRegion {
		return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s: tried in all candidate zones %v but did not find machine type in any of them", machineType, zonesToTry)
	}
	return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s in zone %s: %w", machineType, zone, lastErr)
}

func (g *GKEOrchestrator) verifyDynamicSlicingActive(opts ManifestOptions) (bool, error) {
	// Return false immediately if not using TPUs.
	if !strings.Contains(strings.ToLower(opts.AcceleratorType), "tpu") {
		return false, nil
	}

	// Check for topologies.kueue.x-k8s.io CRD
	cResult := g.executor.ExecuteCommand("kubectl", "get", "crd", "topologies.kueue.x-k8s.io")
	if cResult.ExitCode != 0 {
		logging.Warn("Topology CRD not found. Kueue Dynamic-slicing not active.")
		return false, nil
	}

	// Check for AdmissionCheck with controllerName: accelerator.gke.io/slice
	acResult := g.executor.ExecuteCommand("kubectl", "get", "admissioncheck", "-o", "json")
	if acResult.ExitCode != 0 {
		logging.Warn("Failed to query AdmissionChecks. Assuming dynamic-slicing not active.")
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
		logging.Warn("Failed to parse AdmissionChecks JSON: %v. Assuming dynamic-slicing not active.", err)
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
		logging.Info("No AdmissionCheck with controller 'accelerator.gke.io/slice' found. Dynamic-slicing not active.")
		return false, nil
	}

	// Check discovered node pools for dynamic-slicing
	requestedMachineName, err := g.resolveMachineName(opts.AcceleratorType)
	if err != nil {
		return false, err
	}
	for _, np := range g.clusterDesc.NodePools {
		if np.Config.MachineType == requestedMachineName {
			if np.PlacementPolicy != nil && np.PlacementPolicy.AcceleratorTopologyMode == "PROVISION_ONLY" {
				logging.Info("Dynamic-slicing PROVISION_ONLY mode detected for node pool %s.", np.Name)
				return true, nil
			}
		}
	}

	logging.Info("Node pool does not have PROVISION_ONLY mode. Dynamic-slicing not active.")
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
	machineName, err := g.resolveMachineName(opts.AcceleratorType)
	if err != nil {
		return "", "", "", "", err
	}

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

func (g *GKEOrchestrator) resolveMachineName(acceleratorType string) (string, error) {
	// Check if shorthand (key) existis in the static mao
	if fullType, exists := config.AcceleratorShorthandMap[strings.ToLower(acceleratorType)]; exists {
		return fullType, nil
	}
	// Check if the passed value is a full machine type (value) in the static map
	for _, v := range config.AcceleratorShorthandMap {
		if strings.EqualFold(v, acceleratorType) {
			return acceleratorType, nil
		}
	}

	// Check cluster state [Dynamic accelerator to machine type mapping from the cluster]
	if g.acceleratorToMachineType != nil {
		if machineType, exists := g.acceleratorToMachineType[strings.ToLower(acceleratorType)]; exists {
			return machineType, nil
		}
	}

	// Check if the input is a full machine type and present in the cluster (required for CPUs).
	clusterMachineTypes, err := g.queryAllMachineTypes()
	if err == nil {
		for _, cmt := range clusterMachineTypes {
			if strings.EqualFold(acceleratorType, cmt) {
				return acceleratorType, nil
			}
		}
	}

	// 3. Fail fast
	return "", fmt.Errorf("machine type %q could not be resolved from static maps or cluster state", acceleratorType)
}

func (g *GKEOrchestrator) resolveAcceleratorShorthand(job *orchestrator.JobDefinition) error {
	originalInput := job.AcceleratorType
	parts := strings.Split(originalInput, "-")

	machineName, err := g.resolveMachineName(job.AcceleratorType)
	if err == nil {
		job.AcceleratorType = machineName
		// If it is a TPU, we might still need to resolve topology!
		if config.IsTPU(machineName) && job.Topology == "" && len(parts) == 2 {
			topology, err := config.ResolveTopologyForChips(originalInput)
			if err != nil {
				return err
			}
			job.Topology = topology
			logging.Info("Auto-resolved topology for %s to %s", originalInput, topology)
		}
		return nil
	}

	// 2. If resolveMachineName failed, check if it's a multi-node TPU shorthand!
	if config.IsTPU(originalInput) && job.Topology == "" && len(parts) == 2 {
		topology, err := config.ResolveTopologyForChips(originalInput)
		if err != nil {
			return fmt.Errorf("failed to resolve topology for %s: %w", originalInput, err)
		}
		job.Topology = topology
		logging.Info("Auto-resolved topology for %s to %s", originalInput, topology)
		// Set AcceleratorType to prefix to be resolved via cluster state
		job.AcceleratorType = parts[0]
		return g.resolveAmbiguousShorthand(job)
	}
	return err
}

func (g *GKEOrchestrator) resolveFullMachineTypeOrValue(acceleratorType string) error {
	// Check if it is a defined machine type (value in map)
	for _, v := range config.AcceleratorShorthandMap {
		if v == acceleratorType {
			return nil // Valid defined machine type
		}
	}

	clusterMachineTypes, err := g.queryAllMachineTypes()
	if err != nil {
		return err
	}

	for _, cmt := range clusterMachineTypes {
		if acceleratorType == cmt {
			return nil // Valid full machine type found in cluster
		}
	}
	return fmt.Errorf("machine type %q not found in cluster and is not a known shorthand or defined machine type", acceleratorType)
}

func (g *GKEOrchestrator) resolveAmbiguousShorthand(job *orchestrator.JobDefinition) error {
	candidates := config.GetCandidatesForShorthand(job.AcceleratorType)
	if len(candidates) == 0 {
		return fmt.Errorf("accelerator type %q is not a known shorthand and could not be resolved", job.AcceleratorType)
	}

	logging.Info("Detected ambiguous accelerator shorthand %q, finding candidates...", job.AcceleratorType)

	clusterMachineTypes, err := g.queryAllMachineTypes()
	if err != nil {
		return err
	}

	cmtSet := make(map[string]bool, len(clusterMachineTypes))
	for _, cmt := range clusterMachineTypes {
		cmtSet[cmt] = true
	}

	var matchedCandidates []string
	for _, c := range candidates {
		if cmtSet[c] {
			matchedCandidates = append(matchedCandidates, c)
		}
	}

	if len(matchedCandidates) == 1 {
		logging.Info("Disambiguated %q to %q based on cluster state.", job.AcceleratorType, matchedCandidates[0])

		// If not found in map, fallback to candidate name
		job.AcceleratorType = matchedCandidates[0]
		return nil
	}

	if len(matchedCandidates) == 0 {
		return fmt.Errorf("no matching machine types found in cluster for shorthand %q. Available candidates: %v", job.AcceleratorType, candidates)
	}

	return fmt.Errorf("multiple matching machine types found in cluster for shorthand %q: %v. Please pass the required machine type directly to disambiguate.", job.AcceleratorType, matchedCandidates)
}

func (g *GKEOrchestrator) dynamicallyCalculateVmsPerSlice(job *orchestrator.JobDefinition, topology, mappedLabel string) error {
	if job.VmsPerSlice <= 0 && topology != "" && config.IsTPU(mappedLabel) {
		machineType, err := g.resolveMachineName(job.AcceleratorType)
		if err != nil {
			return err
		}
		nodes, err := config.CalculateAcceleratorNodes(machineType, topology)
		if err != nil {
			return fmt.Errorf("failed to calculate nodes from topology: %w", err)
		}
		job.VmsPerSlice = nodes
		logging.Info("Dynamically determined vms_per_slice for %s: %d", topology, job.VmsPerSlice)
	}
	if job.VmsPerSlice <= 0 {
		job.VmsPerSlice = 1
	}
	return nil
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

	if err := g.dynamicallyCalculateVmsPerSlice(job, topology, mappedLabel); err != nil {
		return SchedulingOptions{}, false, err
	}

	isDynamicSlicing, err := g.verifyDynamicSlicingActive(ManifestOptions{
		ClusterName:     job.ClusterName,
		ClusterLocation: job.ClusterLocation,
		AcceleratorType: job.AcceleratorType,
	})
	if err != nil {
		logging.Warn("Failed to verify if Dynamic-slicing is active: %v. Assuming not active.", err)
	}

	return schedOpts, isDynamicSlicing, nil
}

func (g *GKEOrchestrator) resolveResourcesAndGates(opts *ManifestOptions, isCPUMachine bool, capacity int, job orchestrator.JobDefinition) (JobProfile, error) {
	isGPU := !isCPUMachine && !config.IsTPU(job.AcceleratorType)
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
		resStr, err := g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit, 16)
		if err != nil {
			return profile, err
		}
		opts.ResourcesString = resStr
	}

	return profile, nil
}
