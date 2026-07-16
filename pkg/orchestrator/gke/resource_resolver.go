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
	"strconv"
	"strings"
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

// getZonesForMachineType finds all candidate zones hosting the specified machineType in the cluster's node pools.
func (g *GKEOrchestrator) getZonesForMachineType(machineType string) []string {
	seen := make(map[string]struct{})
	for _, np := range g.clusterDesc.NodePools {
		// Filter node pools that match the target machine type.
		if np.Config.MachineType != machineType {
			continue
		}
		// If the node pool doesn't define custom locations, it inherits cluster-wide locations.
		locs := np.Locations
		if len(locs) == 0 {
			locs = g.clusterZones
		}
		for _, loc := range locs {
			seen[loc] = struct{}{}
		}
	}

	// Return a deduplicated slice of all matched zones.
	var zones []string
	for loc := range seen {
		zones = append(zones, loc)
	}
	return zones
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

	if isRegion {
		npZones := g.getZonesForMachineType(machineType)
		if len(npZones) > 0 {
			zonesToTry = npZones
		} else {
			if !g.clusterDesc.Autoscaling.EnableNodeAutoprovisioning {
				return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s: no node pool matching machine type found and GKE Node Auto-Provisioning is disabled", machineType)
			}
			if len(g.clusterZones) > 0 {
				zonesToTry = g.clusterZones
			}
		}
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

		count, accelType, isTPU := config.ResolveAcceleratorInfo(mt, machineType)
		if count > 0 {
			// Fetch correct accelerator type for TPUs using map
			if isTPU {
				accelType = g.GenerateGKENodeSelectorLabel(machineType)
			}
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
	if !config.IsTPU(opts.ComputeType) {
		return false, nil
	}

	if g.dynamicSlicingCache == nil {
		g.dynamicSlicingCache = make(map[string]bool)
	}

	cacheKey := opts.ComputeType + ":" + opts.Topology
	if val, ok := g.dynamicSlicingCache[cacheKey]; ok {
		return val, nil
	}

	// Check discovered node pools for dynamic-slicing
	requestedMachineName := opts.MachineType
	if requestedMachineName == "" {
		var err error
		requestedMachineName, err = g.resolveMachineName(opts.ComputeType)
		if err != nil {
			return false, err
		}
	}

	isTPU7x := strings.Contains(strings.ToLower(requestedMachineName), "tpu7x")
	if !isTPU7x || !g.hasSlicingTopologies() || !g.hasSliceAdmissionCheck() {
		g.dynamicSlicingCache[cacheKey] = false
		return false, nil
	}

	active, err := g.checkNodePoolsDynamicSlicing(requestedMachineName, opts, isTPU7x)
	if err != nil {
		return active, err
	}
	g.dynamicSlicingCache[cacheKey] = active
	return active, nil
}

func (g *GKEOrchestrator) verifyStaticSlicingActive(job *orchestrator.JobDefinition) (bool, error) {
	if !config.IsTPU(job.MachineType) {
		return false, nil
	}

	// Static sub-slicing (logical partitioning) is strictly unsupported for 3D Torus TPUs (v4 and v5p)
	if config.Is3DTorusTPU(job.MachineType) {
		return false, nil
	}

	cacheKey := fmt.Sprintf("%s-%s", job.MachineType, job.Topology)
	if val, ok := g.staticSlicingCache[cacheKey]; ok {
		return val, nil
	}

	if !g.hasSlicingTopologies() {
		g.staticSlicingCache[cacheKey] = false
		return false, nil
	}

	accelLabel := g.GenerateGKENodeSelectorLabel(job.MachineType)
	output, err := g.queryDiscoveredTopologies(accelLabel, job.MachineType)
	if err != nil {
		return false, fmt.Errorf("failed to discover topologies for static sub-slicing check: %w", err)
	}
	discoveredTopologies := g.parseTopologies(output)

	for t := range discoveredTopologies {
		fits, err := config.CheckTopologyContainment(job.Topology, t, job.MachineType)
		if err != nil {
			return false, err
		}
		if fits {
			logging.Info("Static sub-slicing/TAS active: requested topology %s fits inside discovered physical topology %s.", job.Topology, t)
			g.staticSlicingCache[cacheKey] = true
			return true, nil
		}
	}

	g.staticSlicingCache[cacheKey] = false
	return false, nil
}

func (g *GKEOrchestrator) checkNodePoolsDynamicSlicing(requestedMachineName string, opts ManifestOptions, isTPU7x bool) (bool, error) {
	for _, np := range g.clusterDesc.NodePools {
		if !strings.EqualFold(np.Config.MachineType, requestedMachineName) {
			continue
		}

		isProvisionOnly := isTPU7x && np.PlacementPolicy != nil && np.PlacementPolicy.AcceleratorTopologyMode == "PROVISION_ONLY"

		if isProvisionOnly {
			if err := validateTPU7xTopology(opts.Topology, requestedMachineName); err != nil {
				return true, err
			}
			logging.Info("Dynamic-slicing PROVISION_ONLY mode validated for TPU7x node pool %s with topology %s.", np.Name, opts.Topology)
			return true, nil
		}
	}

	logging.Info("Node pool does not have dynamic topology subset requirement. Dynamic-slicing not active.")
	return false, nil
}

func validateTPU7xTopology(topology string, machineType string) error {
	if topology == "" {
		return fmt.Errorf("topology must be specified explicitly via --topology flag for TPU 7x dynamic slicing")
	}
	if !config.TopologyRegex.MatchString(topology) {
		return fmt.Errorf("invalid topology format %s", topology)
	}
	return config.Validate3DTopology(topology, machineType, true)
}

func (g *GKEOrchestrator) hasSliceAdmissionCheck() bool {
	acResult := g.executor.ExecuteCommand("kubectl", "get", "admissioncheck", "-o", "json")
	if acResult.ExitCode != 0 {
		logging.Warn("Failed to query AdmissionChecks. Assuming dynamic-slicing not active.")
		return false
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
		return false
	}

	for _, item := range acList.Items {
		if item.Spec.ControllerName == "accelerator.gke.io/slice" {
			return true
		}
	}

	logging.Info("No AdmissionCheck with controller 'accelerator.gke.io/slice' found. Dynamic-slicing not active.")
	return false
}

func (g *GKEOrchestrator) hasSlicingTopologies() bool {
	if g.slicingTopologiesChecked {
		return g.slicingTopologiesDetected
	}

	defer func() {
		g.slicingTopologiesChecked = true
	}()

	tResult := g.executor.ExecuteCommand("kubectl", "get", "topologies.kueue.x-k8s.io", "-o", "json")
	if tResult.ExitCode != 0 {
		logging.Warn("Failed to query Kueue topologies. Assuming dynamic-slicing not active.")
		return false
	}

	var tList struct {
		Items []struct {
			Spec struct {
				Levels []struct {
					NodeLabel string `json:"nodeLabel"`
				} `json:"levels"`
			} `json:"spec"`
		} `json:"items"`
	}

	if err := json.Unmarshal([]byte(tResult.Stdout), &tList); err != nil {
		logging.Warn("Failed to parse Kueue topologies JSON: %v. Assuming dynamic-slicing not active.", err)
		return false
	}

	if len(tList.Items) == 0 {
		logging.Info("No Kueue topology resources found. Dynamic-slicing not active.")
		return false
	}

	for _, t := range tList.Items {
		for _, l := range t.Spec.Levels {
			if (strings.HasPrefix(l.NodeLabel, "cloud.google.com/gke-tpu-slice-") || strings.HasPrefix(l.NodeLabel, "cloud.google.com/gke-tpu-partition-")) && strings.HasSuffix(l.NodeLabel, "-id") {
				g.slicingTopologiesDetected = true
				return true
			}
		}
	}

	logging.Info("Kueue topologies found but they do not contain slice/partition labels. Assuming dynamic-slicing not active.")
	return false
}

func (g *GKEOrchestrator) calculateResourceLimits(opts ManifestOptions, profile JobProfile) (cpu, mem, gpu, tpu string, err error) {
	if profile.IsCPUMachine {
		logging.Info("Using cached capacity for CPU machine %s during limits calculation: %d", opts.ComputeType, profile.CapacityCount)
		offsetVCPUs := max(1, int(float64(profile.CapacityCount)*0.95))
		return fmt.Sprintf("%d", offsetVCPUs), "", "", "", nil
	}

	mapped := g.GenerateGKENodeSelectorLabel(opts.ComputeType)

	cpuLim, memLim, gpuLim, tpuLim, err := g.calculateGCPMachineResourceLimits(opts, mapped)
	if err != nil {
		return "", "", "", "", fmt.Errorf("cluster resolution failed for %s: %w", opts.ComputeType, err)
	}
	if opts.ParallelContainers > 1 && tpuLim != "" {
		tpuInt, err := strconv.Atoi(tpuLim)
		if err != nil {
			return "", "", "", "", fmt.Errorf("failed to parse tpu limit %q: %w", tpuLim, err)
		}
		tpuLim = strconv.Itoa(tpuInt / opts.ParallelContainers)
	}
	return cpuLim, memLim, gpuLim, tpuLim, nil
}

func (g *GKEOrchestrator) calculateGCPMachineResourceLimits(opts ManifestOptions, mapped string) (cpu, mem, gpu, tpu string, err error) {
	machineName := opts.MachineType

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

func (g *GKEOrchestrator) resolveJobMachineType(computeType string) (string, error) {
	parts := strings.Split(computeType, "-")
	machineName, err := g.resolveMachineName(computeType)
	if err == nil {
		return machineName, nil
	}

	prefix := parts[0]
	candidates := config.GetCandidatesForShorthand(prefix)
	if len(candidates) == 0 {
		return "", fmt.Errorf("compute type %q is not a known shorthand and could not be resolved", prefix)
	}

	machineName, err = g.resolveAmbiguousComputeShorthand(prefix, candidates)
	if err != nil {
		return "", err
	}
	return machineName, nil
}

func (g *GKEOrchestrator) resolveTPURequirements(job *orchestrator.JobDefinition) (isDynamicSlicing bool, isStaticSlicing bool, err error) {
	isTPU7x := strings.Contains(strings.ToLower(job.MachineType), "tpu7x")
	if isTPU7x && job.Topology == "" {
		return false, false, fmt.Errorf("topology must be specified explicitly via --topology flag for TPU 7x machine type %s", job.MachineType)
	}

	// Validate topology shape before resolving/discovering
	if job.Topology != "" {
		if err := config.ValidateHardwareRequest(job.MachineType, job.Topology); err != nil {
			return false, false, err
		}
	}

	var topology string
	topology, isDynamicSlicing, err = g.resolveTopology(job)
	if err != nil {
		return false, false, err
	}
	job.Topology = topology

	if !isDynamicSlicing && job.Topology != "" {
		isStaticSlicing, err = g.verifyStaticSlicingActive(job)
		if err != nil {
			return false, false, err
		}
	}

	// Calculate VMs per slice
	err = g.dynamicallyCalculateNodesPerSlice(job)
	if err != nil {
		return false, false, err
	}

	return isDynamicSlicing, isStaticSlicing, nil
}

func (g *GKEOrchestrator) resolveHardwareRequirements(job *orchestrator.JobDefinition) (profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool, err error) {
	if job.ComputeType == "" {
		return JobProfile{}, false, false, nil
	}

	machineName, err := g.resolveJobMachineType(job.ComputeType)
	if err != nil {
		return JobProfile{}, false, false, err
	}
	job.MachineType = machineName
	if config.IsTPU(machineName) {
		isDynamicSlicing, isStaticSlicing, err = g.resolveTPURequirements(job)
		if err != nil {
			return JobProfile{}, false, false, err
		}
		if job.GKENAPProvisioning != "" {
			if isDynamicSlicing {
				return JobProfile{}, false, false, fmt.Errorf("TPU Dynamic Slicing is not supported on GKE Node Auto-Provisioning (NAP) workloads")
			}
			if isStaticSlicing {
				return JobProfile{}, false, false, fmt.Errorf("TPU Static Sub-slicing is not supported on GKE Node Auto-Provisioning (NAP) workloads")
			}
			if job.GKEScheduler == "gke.io/tpu-provisioning-request" {
				return JobProfile{}, false, false, fmt.Errorf("TPU ProvisioningRequest (DWS Flex) is not supported on GKE Node Auto-Provisioning (NAP) workloads")
			}
		}
	}
	isCPUMachine, capacity, err := g.determineIfCPUMachine(job)
	if err != nil {
		return JobProfile{}, isDynamicSlicing, isStaticSlicing, err
	}

	if err := g.validateConsumptionForStaticCluster(job); err != nil {
		return JobProfile{}, isDynamicSlicing, isStaticSlicing, err
	}

	return JobProfile{
		IsCPUMachine:  isCPUMachine,
		CapacityCount: capacity,
	}, isDynamicSlicing, isStaticSlicing, nil
}

func (g *GKEOrchestrator) resolveAmbiguousComputeShorthand(prefix string, candidates []string) (string, error) {
	logging.Info("Detected ambiguous compute shorthand %q, finding candidates...", prefix)

	clusterMachineTypes, err := g.queryAllMachineTypes()
	if err != nil {
		return "", err
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
		logging.Info("Disambiguated %q to %q based on cluster state.", prefix, matchedCandidates[0])
		return matchedCandidates[0], nil
	}

	if len(matchedCandidates) == 0 {
		return "", fmt.Errorf("no matching machine types found in cluster for shorthand %q. Available candidates: %v", prefix, candidates)
	}

	return "", fmt.Errorf("multiple matching machine types found in cluster for shorthand %q: %v. Please pass the required machine type directly to disambiguate.", prefix, matchedCandidates)
}

func (g *GKEOrchestrator) dynamicallyCalculateNodesPerSlice(job *orchestrator.JobDefinition) error {
	if !config.IsTPU(job.MachineType) {
		if job.NodesPerSlice <= 0 {
			job.NodesPerSlice = 1 // default to 1 for non-TPU jobs if not provided
		}
		return nil
	}
	if job.Topology == "" {
		return fmt.Errorf("could not resolve TPU topology for the provided machine type: %q", job.MachineType)
	}
	machineType := job.MachineType
	accelsPerVM, err := g.FetchMachineCapacity(machineType, job.ClusterLocation)
	if err != nil {
		logging.Warn("Failed to fetch machine capacity for %s: %v. Falling back to static defaults.", machineType, err)
		accelsPerVM = 0 // Fallback to static logic in CalculateAcceleratorNodes
	}
	nodes, err := config.CalculateAcceleratorNodes(machineType, job.Topology, accelsPerVM)
	if err != nil {
		return fmt.Errorf("failed to calculate nodes from topology: %w", err)
	}
	job.NodesPerSlice = nodes
	if job.NodesPerSlice <= 0 {
		return fmt.Errorf("invalid nodes_per_slice (%d) for topology %s", job.NodesPerSlice, job.Topology)
	}
	logging.Info("Dynamically determined nodes_per_slice for %s: %d", job.Topology, job.NodesPerSlice)
	return nil
}

func (g *GKEOrchestrator) fetchClusterState(job *orchestrator.JobDefinition) error {
	logging.Info("Eagerly fetching and caching machine capabilities...")
	machineTypes, err := g.queryAllMachineTypes()
	if err != nil {
		return err
	}

	for _, mt := range machineTypes {
		_, err := g.FetchMachineCapabilities(mt, job.ClusterLocation)
		if err != nil {
			logging.Warn("Failed to pre-fetch capabilities for machine type %s: %v", mt, err)
		}
	}
	return nil
}

func (g *GKEOrchestrator) resolveResourcesAndGates(opts *ManifestOptions, isCPUMachine bool, capacity int, job orchestrator.JobDefinition) (JobProfile, error) {
	isGPU := !isCPUMachine && !config.IsTPU(job.MachineType)
	if isGPU && job.GKEScheduler == "gke.io/topology-aware-auto" {
		opts.SchedulingGates = indentYaml("schedulingGates:\n  - name: \"gke.io/topology-aware-auto-"+job.WorkloadName+"\"", 14)
		opts.SchedulerName = ""
	}

	profile := JobProfile{
		IsCPUMachine:  isCPUMachine,
		CapacityCount: capacity,
	}

	opts.ParallelContainers = 1
	if job.UseParallelContainers && !job.IsPathwaysJob && strings.Contains(job.MachineType, "tpu7x") {
		opts.ParallelContainers = 2
	}

	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(*opts, profile)
	if err != nil {
		logging.Warn("Warning: failed to calculate resource limits: %v", err)
	} else {
		if opts.ComputeType != "" && gpuLimit == "" && tpuLimit == "" {
			logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.ComputeType)
			opts.ComputeType = ""
		}
		resStr, err := g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit, 16)
		if err != nil {
			return profile, err
		}
		opts.ResourcesString = resStr
	}

	return profile, nil
}
