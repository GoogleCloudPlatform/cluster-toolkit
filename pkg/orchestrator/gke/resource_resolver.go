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
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"
	"path"
	"slices"
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

var defaultTPUHardwareResolver = gceTPUHardwareResolver{}

func (g *GKEOrchestrator) FetchMachineCapacity(machineType, zone string) (int, error) {
	cap, err := g.FetchMachineCapabilities(machineType, zone)
	if err != nil {
		return 0, err
	}

	if len(cap.Accelerators) > 0 {
		return cap.Accelerators[0].Count, nil
	}
	if cap.GuestCpus > 0 {
		return g.getEffectiveCPUs(machineType, cap.GuestCpus), nil
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
	if g.mode == orchestrator.TypeK8s {
		return g.resolveK8sMachineCapabilities(machineType)
	}

	cacheKey := machineType + ":" + zone
	if g.machineCapCache != nil {
		if cap, ok := g.machineCapCache[cacheKey]; ok {
			return cap, nil
		}
	}

	isRegion := len(strings.Split(zone, "-")) < 3
	zonesToTry, err := g.resolveCandidateZonesForMachine(machineType, zone, isRegion)
	if err != nil {
		return MachineTypeCap{}, err
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

func (g *GKEOrchestrator) resolveCandidateZonesForMachine(machineType, zone string, isRegion bool) ([]string, error) {
	if !isRegion {
		return []string{zone}, nil
	}
	if npZones := g.getZonesForMachineType(machineType); len(npZones) > 0 {
		return npZones, nil
	}
	if !g.clusterDesc.Autoscaling.EnableNodeAutoprovisioning {
		return nil, fmt.Errorf("failed to fetch machine capabilities for %s: no node pool matching machine type found and GKE Node Auto-Provisioning is disabled", machineType)
	}
	if len(g.clusterZones) > 0 {
		return g.clusterZones, nil
	}
	return []string{zone}, nil
}

func (g *GKEOrchestrator) verifyDynamicSlicingActive(opts ManifestOptions) (bool, error) {
	if g.mode == orchestrator.TypeK8s {
		return false, nil
	}
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

	active, err := g.checkNodePoolsDynamicSlicing(requestedMachineName, opts)
	if err != nil {
		return active, err
	}
	g.dynamicSlicingCache[cacheKey] = active
	return active, nil
}

func (g *GKEOrchestrator) verifyStaticSlicingActive(machineType, topology string) (bool, error) {
	if g.mode == orchestrator.TypeK8s {
		return false, nil
	}
	if !config.IsTPU(machineType) {
		return false, nil
	}

	// Static sub-slicing (logical partitioning) is strictly unsupported for 3D Torus TPUs (v4 and v5p)
	if config.Is3DTorusTPU(machineType) {
		return false, nil
	}

	if g.staticSlicingCache == nil {
		g.staticSlicingCache = make(map[string]bool)
	}

	cacheKey := fmt.Sprintf("%s-%s", machineType, topology)
	if val, ok := g.staticSlicingCache[cacheKey]; ok {
		return val, nil
	}

	if !g.hasSlicingTopologies() {
		g.staticSlicingCache[cacheKey] = false
		return false, nil
	}

	accelLabel := g.GenerateGKENodeSelectorLabel(machineType)
	output, err := g.queryDiscoveredTopologies(accelLabel, machineType)
	if err != nil {
		return false, fmt.Errorf("failed to discover topologies for static sub-slicing check: %w", err)
	}
	discoveredTopologies := g.parseTopologies(output)

	for t := range discoveredTopologies {
		fits, err := config.CheckTopologyContainment(topology, t, machineType)
		if err != nil {
			return false, err
		}
		if fits {
			logging.Info("Static sub-slicing/TAS active: requested topology %s fits inside discovered physical topology %s.", topology, t)
			g.staticSlicingCache[cacheKey] = true
			return true, nil
		}
	}

	g.staticSlicingCache[cacheKey] = false
	return false, nil
}

func (g *GKEOrchestrator) checkNodePoolsDynamicSlicing(requestedMachineName string, opts ManifestOptions) (bool, error) {
	projID := opts.ProjectID
	if projID == "" {
		projID = g.projectID
	}

	for _, np := range g.clusterDesc.NodePools {
		if !strings.EqualFold(np.Config.MachineType, requestedMachineName) || np.PlacementPolicy == nil {
			continue
		}

		mode := np.PlacementPolicy.AcceleratorTopologyMode
		if mode == "" && np.PlacementPolicy.PolicyName != "" {
			policy, err := g.describeResourcePolicyCached(np.PlacementPolicy.PolicyName, opts.ClusterLocation, projID)
			if err != nil {
				if errors.Is(err, ErrResourcePolicyPermissionDenied) {
					logging.Warn("Permission denied querying resource policy %q for node pool %q: %v. Assuming non-dynamic slicing.", np.PlacementPolicy.PolicyName, np.Name, err)
				} else {
					return false, fmt.Errorf("failed to fetch compute resource policy %q for node pool %q: %w", np.PlacementPolicy.PolicyName, np.Name, err)
				}
			} else if policy != nil {
				mode = policy.AcceleratorTopologyMode
			}
		}

		if strings.EqualFold(mode, "PROVISION_ONLY") {
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

func (g *GKEOrchestrator) checkDynamicSlicingViaGKE() bool {
	for _, np := range g.clusterDesc.NodePools {
		if np.PlacementPolicy != nil {
			mode := np.PlacementPolicy.AcceleratorTopologyMode
			if strings.EqualFold(mode, "PROVISION_ONLY") {
				return true
			}
		}
	}
	return false
}

// ErrResourcePolicyPermissionDenied indicates the caller lacks GCP IAM permissions to read the compute resource policy.
var ErrResourcePolicyPermissionDenied = errors.New("permission denied querying compute resource policy")

// describeResourcePolicyCached retrieves GCE resource policy metadata using gcloud compute resource-policies describe,
// utilizing a local cache to prevent redundant invocations across dynamic slicing and NAP policy resolution.
func (g *GKEOrchestrator) describeResourcePolicyCached(policyName, location, projectID string) (*GCEWorkloadPolicy, error) {
	if g.resourcePolicyCache == nil {
		g.resourcePolicyCache = make(map[string]*GCEWorkloadPolicy)
	}

	shortPolicyName := path.Base(policyName)
	if policy, found := g.resourcePolicyCache[shortPolicyName]; found {
		return policy, nil
	}

	region := shell.ExtractRegion(location)
	res := g.executor.ExecuteCommand("gcloud", "compute", "resource-policies", "describe", shortPolicyName, "--region="+region, "--project="+projectID, "--format=json")
	if res.ExitCode != 0 {
		stderrLower := strings.ToLower(res.Stderr)
		if strings.Contains(stderrLower, "not found") || strings.Contains(stderrLower, "404") {
			return nil, nil // Policy does not exist
		}
		if isPermissionDenied(res.Stderr) {
			return nil, fmt.Errorf("%w: %s", ErrResourcePolicyPermissionDenied, res.Stderr)
		}
		return nil, fmt.Errorf("gcloud compute resource-policies describe failed: %s\n"+
			"To resolve this issue:\n"+
			"  1. Check IAM: Ensure your GCP credentials have 'compute.resourcePolicies.get' permission (e.g., 'roles/compute.viewer').\n"+
			"  2. Verify Policy: Test manual access by running:\n"+
			"     gcloud compute resource-policies describe %s --region=%s --project=%s",
			res.Stderr, shortPolicyName, region, projectID)
	}

	var raw gceResourcePolicyRaw
	if err := json.Unmarshal([]byte(res.Stdout), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse resource policy json: %w", err)
	}

	policy := &GCEWorkloadPolicy{
		Name:                    raw.Name,
		Region:                  raw.Region,
		AcceleratorTopology:     raw.WorkloadPolicy.AcceleratorTopology,
		AcceleratorTopologyMode: raw.WorkloadPolicy.AcceleratorTopologyMode,
		Type:                    raw.WorkloadPolicy.Type,
	}
	g.resourcePolicyCache[shortPolicyName] = policy
	return policy, nil
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
	if !g.kueueEnabled() {
		return false
	}
	acResult := g.kubectl("get", "admissioncheck", "-o", "json")
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
	if !g.kueueEnabled() {
		return false
	}
	if g.slicingTopologiesChecked {
		return g.slicingTopologiesDetected
	}

	defer func() {
		g.slicingTopologiesChecked = true
	}()

	tResult := g.kubectl("get", "topologies.kueue.x-k8s.io", "-o", "json")
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
		if g.mode == orchestrator.TypeK8s {
			return calculateK8sCPUResourceLimit(profile, opts.GCSFuseEnabled), "", "", "", nil
		}
		machineType := opts.MachineType
		if machineType == "" {
			machineType = opts.ComputeType
		}
		vCPUs := g.getEffectiveCPUs(machineType, profile.CapacityCount)
		logging.Info("Using cached capacity for CPU machine %s during limits calculation: %d", opts.ComputeType, vCPUs)
		cpuMilli := calculateFullNodeCPURequestMilli(vCPUs, opts.GCSFuseEnabled)
		return fmt.Sprintf("%dm", cpuMilli), "", "", "", nil
	}

	mapped := g.GenerateGKENodeSelectorLabel(opts.ComputeType)
	var cpuLim, memLim, gpuLim, tpuLim string
	if g.mode == orchestrator.TypeK8s {
		cpuLim, memLim, gpuLim, tpuLim, err = g.calculateK8sMachineResourceLimits(opts, profile)
	} else {
		cpuLim, memLim, gpuLim, tpuLim, err = g.calculateGCPMachineResourceLimits(opts, mapped)
	}
	if err != nil {
		return "", "", "", "", fmt.Errorf("cluster resolution failed for %s: %w", opts.ComputeType, err)
	}
	if opts.ParallelContainers > 1 && tpuLim != "" {
		tpuInt, err := strconv.Atoi(tpuLim)
		if err != nil {
			return "", "", "", "", fmt.Errorf("failed to parse tpu limit %q: %w", tpuLim, err)
		}
		if tpuInt < opts.ParallelContainers || tpuInt%opts.ParallelContainers != 0 {
			return "", "", "", "", fmt.Errorf("TPU chip count %d cannot be evenly split across %d parallel containers", tpuInt, opts.ParallelContainers)
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
	if err == nil && slices.ContainsFunc(clusterMachineTypes, func(cmt string) bool { return strings.EqualFold(acceleratorType, cmt) }) {
		return acceleratorType, nil
	}

	// 3. Fail fast
	return "", fmt.Errorf("machine type %q could not be resolved from static maps or cluster state", acceleratorType)
}

// resolveJobMachineType maps --compute-type to a machine type, preferring the host shape used by the cluster's node pools for TPU shorthands.
func (g *GKEOrchestrator) resolveJobMachineType(computeType string) (string, error) {
	if mapped, isShorthand := config.AcceleratorShorthandMap[strings.ToLower(computeType)]; isShorthand {
		if config.IsTPU(mapped) {
			return g.resolveShorthandFromCluster(computeType, mapped)
		}
		return mapped, nil
	}

	machineName, err := g.resolveMachineName(computeType)
	if err != nil {
		return g.resolveShorthandFromCluster(computeType, "")
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
		isStaticSlicing, err = g.verifyStaticSlicingActive(job.MachineType, job.Topology)
		if err != nil {
			return false, false, err
		}
	}

	if err = g.dynamicallyCalculateNodesPerSlice(job); err != nil {
		return false, false, err
	}

	if err = g.resolveTPUPlacementPolicy(job, isTPU7x, isDynamicSlicing); err != nil {
		return false, false, err
	}

	return isDynamicSlicing, isStaticSlicing, nil
}

func (g *GKEOrchestrator) resolveTPUPlacementPolicy(job *orchestrator.JobDefinition, isTPU7x, isDynamicSlicing bool) error {
	if job.PlacementPolicy != "" {
		return g.resolveWorkloadPlacementPolicy(job, isTPU7x, isDynamicSlicing)
	}
	if isTPU7x && !isDynamicSlicing && job.NodesPerSlice > 1 {
		policy, err := g.resolveWorkloadPolicy(job.MachineType, job.Topology, job.ClusterLocation, job.ProjectID, job.DryRunManifest != "")
		if err != nil {
			return err
		}
		job.PlacementPolicy = policy
	}
	return nil
}

// parseResourcePolicyURI parses a GCE resource policy URI, URL, or plain name into its components.
// E.g.:
// - "my-policy" -> Name: "my-policy"
// - "projects/my-proj/regions/us-central1/resourcePolicies/my-policy" -> Project: "my-proj", Region: "us-central1", Name: "my-policy"
// - "https://www.googleapis.com/compute/v1/projects/my-proj/regions/us-central1/resourcePolicies/my-policy" -> Project: "my-proj", Region: "us-central1", Name: "my-policy"
func parseResourcePolicyURI(policyStr string) parsedResourcePolicy {
	policyStr = strings.TrimSuffix(strings.TrimSpace(policyStr), "/")
	if !strings.Contains(policyStr, "/") {
		return parsedResourcePolicy{Name: policyStr}
	}

	return parsedResourcePolicy{
		Project: extractURIPart(policyStr, "projects"),
		Region:  extractURIPart(policyStr, "regions"),
		Name:    extractURIPart(policyStr, "resourcepolicies"),
	}
}

// resolveWorkloadPlacementPolicy sanitizes user-specified placement policy URIs into bare resource names
// and validates their existence, project/region locality, topology, and static mode for TPU 7x workloads.
func (g *GKEOrchestrator) resolveWorkloadPlacementPolicy(job *orchestrator.JobDefinition, isTPU7x, isDynamicSlicing bool) error {
	if job.PlacementPolicy == "" {
		return nil
	}

	parsed := parseResourcePolicyURI(job.PlacementPolicy)
	if parsed.Name == "" {
		return fmt.Errorf("invalid placement policy %q: unable to determine policy name", job.PlacementPolicy)
	}
	job.PlacementPolicy = parsed.Name

	if err := validateResourcePolicyLocality(parsed, job.ProjectID, job.ClusterLocation); err != nil {
		return err
	}

	// GCE workload policies are queried and validated only where the hardware requires one:
	// static multi-host TPU 7x and GPU families listed in gpuWorkloadPolicyTopologies.
	// Other GPUs/CPUs may use either a compact placement or a workload policy, so only locality is checked.
	expectedTopology := ""
	if isTPU7x && !isDynamicSlicing && job.NodesPerSlice > 1 {
		expectedTopology = job.Topology
	} else if !isTPU7x {
		expectedTopology = gpuWorkloadPolicyTopology(job.MachineType)
	}
	if expectedTopology != "" && job.DryRunManifest == "" {
		clusterRegion := shell.ExtractRegion(job.ClusterLocation)
		return g.validateWorkloadPlacementPolicy(parsed.Name, clusterRegion, job.ProjectID, expectedTopology)
	}

	return nil
}

// resolveGPUPlacementPolicy validates a user-specified placement policy for GPU/CPU jobs and,
// for GPU families that cannot schedule without a GCE workload policy (e.g. A4X), discovers one
// when none was given: from the matching node pool on static clusters, or from GCE (creating the
// canonical policy if missing) under NAP. A policy is never created for a static cluster because
// nodes only carry the policy their node pool was built with.
func (g *GKEOrchestrator) resolveGPUPlacementPolicy(job *orchestrator.JobDefinition) error {
	if job.PlacementPolicy != "" {
		return g.resolveWorkloadPlacementPolicy(job, false, false)
	}
	topology := gpuWorkloadPolicyTopology(job.MachineType)
	if topology == "" {
		return nil
	}
	if job.GKENAPProvisioning != "" {
		policy, err := g.resolveWorkloadPolicy(job.MachineType, topology, job.ClusterLocation, job.ProjectID, job.DryRunManifest != "")
		if err != nil {
			return err
		}
		job.PlacementPolicy = policy
		return nil
	}
	if policy := g.findPolicyInClusterNodePools(job.MachineType, topology); policy != "" {
		job.PlacementPolicy = policy
		return nil
	}
	logging.Warn("No node pool for %s carries a placement policy. %s requires a HIGH_THROUGHPUT workload policy with topology %s attached to its node pool; pods may fail to schedule or run with degraded performance. Create a node pool with the workload policy attached (it will then be discovered automatically), or use --gke-nap-provisioning.", job.MachineType, job.MachineType, topology)
	return nil
}

func validateResourcePolicyLocality(parsed parsedResourcePolicy, projectID, clusterLocation string) error {
	if parsed.Project != "" && projectID != "" && !strings.EqualFold(parsed.Project, projectID) {
		return fmt.Errorf("placement policy %q belongs to project %q, but cluster is in project %q", parsed.Name, parsed.Project, projectID)
	}
	clusterRegion := shell.ExtractRegion(clusterLocation)
	if parsed.Region != "" && clusterRegion != "" && !strings.EqualFold(parsed.Region, clusterRegion) {
		return fmt.Errorf("placement policy %q belongs to region %q, but cluster is in region %q", parsed.Name, parsed.Region, clusterRegion)
	}
	return nil
}

func (g *GKEOrchestrator) validateWorkloadPlacementPolicy(name, region, projectID, expectedTopology string) error {
	policy, err := g.describeResourcePolicyCached(name, region, projectID)
	if err != nil {
		if errors.Is(err, ErrResourcePolicyPermissionDenied) {
			logging.Warn("Permission denied querying placement policy %q: %v. Proceeding without policy validation.", name, err)
			return nil
		}
		return fmt.Errorf("failed to describe placement policy %q: %w", name, err)
	}
	if policy == nil {
		return fmt.Errorf("placement policy %q does not exist in region %s (project %s)", name, region, projectID)
	}
	if policy.Type != "" && !strings.EqualFold(policy.Type, "HIGH_THROUGHPUT") {
		return fmt.Errorf("placement policy %q has type %q; expected %q", name, policy.Type, "HIGH_THROUGHPUT")
	}
	if !isStaticWorkloadPolicyMode(policy.AcceleratorTopologyMode) {
		return fmt.Errorf("placement policy %q has mode %q, which is incompatible with static multi-host workloads (expected AUTO_CONNECT or default static placement)", name, policy.AcceleratorTopologyMode)
	}
	if expectedTopology != "" && policy.AcceleratorTopology != "" && !strings.EqualFold(policy.AcceleratorTopology, expectedTopology) {
		return fmt.Errorf("placement policy %q has topology %q; expected %q", name, policy.AcceleratorTopology, expectedTopology)
	}
	return nil
}

func (g *GKEOrchestrator) resolveHardwareRequirements(job *orchestrator.JobDefinition) (profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool, err error) {
	if g.mode == orchestrator.TypeK8s {
		return g.resolveK8sHardwareRequirements(job)
	}
	if job.ComputeType == "" {
		return JobProfile{}, false, false, nil
	}

	machineName, err := g.resolveJobMachineType(job.ComputeType)
	if err != nil {
		return JobProfile{}, false, false, err
	}
	job.MachineType = machineName
	if config.IsTPU(machineName) {
		isDynamicSlicing, isStaticSlicing, err = g.resolveGKETPUHardware(job)
		if err != nil {
			return JobProfile{}, false, false, err
		}
	} else {
		if err := g.resolveGPUPlacementPolicy(job); err != nil {
			return JobProfile{}, isDynamicSlicing, isStaticSlicing, err
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

func (g *GKEOrchestrator) resolveGKETPUHardware(job *orchestrator.JobDefinition) (isDynamicSlicing, isStaticSlicing bool, err error) {
	if job.NumNodesSpecified {
		return false, false, fmt.Errorf("--num-nodes cannot be used with TPU jobs (it is calculated automatically from topology)")
	}
	isDynamicSlicing, isStaticSlicing, err = g.resolveTPURequirements(job)
	if err != nil {
		return false, false, err
	}
	if job.GKENAPProvisioning != "" {
		if isDynamicSlicing {
			return false, false, fmt.Errorf("TPU Dynamic Slicing is not supported on GKE Node Auto-Provisioning (NAP) workloads")
		}
		if isStaticSlicing {
			return false, false, fmt.Errorf("TPU Static Sub-slicing is not supported on GKE Node Auto-Provisioning (NAP) workloads")
		}
		if job.GKEScheduler == "gke.io/tpu-provisioning-request" {
			return false, false, fmt.Errorf("TPU ProvisioningRequest (DWS Flex) is not supported on GKE Node Auto-Provisioning (NAP) workloads")
		}
	}
	return isDynamicSlicing, isStaticSlicing, nil
}

func isCandidateCompatibleForComputeType(computeType, candidate string) bool {
	if !config.IsTPU(candidate) {
		return true
	}
	topo, err := config.ResolveTopologyForChips(strings.ToLower(computeType), candidate)
	if err != nil {
		return true
	}
	nodes, err := config.CalculateAcceleratorNodes(candidate, topo, 0)
	if err != nil {
		return false
	}
	hostChips := config.AcceleratorsPerVM(candidate)
	return nodes == 1 || hostChips == 4
}

func (g *GKEOrchestrator) resolveShorthandFromCluster(computeType, defaultMachineType string) (string, error) {
	prefix := strings.Split(computeType, "-")[0]
	candidates := config.GetCandidatesForShorthand(prefix)
	if len(candidates) == 0 {
		return "", fmt.Errorf("compute type %q is not a known shorthand and could not be resolved", prefix)
	}

	clusterMachineTypes, err := g.queryAllMachineTypes()
	if err != nil {
		return "", err
	}

	var inCluster []string
	for _, c := range candidates {
		if !isCandidateCompatibleForComputeType(computeType, c) {
			continue
		}
		if slices.ContainsFunc(clusterMachineTypes, func(cmt string) bool { return strings.EqualFold(cmt, c) }) {
			inCluster = append(inCluster, c)
		}
	}

	if defaultMachineType != "" && (len(inCluster) == 0 || slices.Contains(inCluster, defaultMachineType)) {
		// Present, or absent along with the rest of its family: node auto-provisioning
		// may still create it, and otherwise the capacity lookup reports the gap.
		return defaultMachineType, nil
	}

	switch len(inCluster) {
	case 1:
		logging.Info("Disambiguated %q to %q based on cluster state.", prefix, inCluster[0])
		return inCluster[0], nil
	case 0:
		return "", fmt.Errorf("no matching machine types found in cluster for shorthand %q. Available candidates: %v", prefix, candidates)
	default:
		return "", fmt.Errorf("multiple matching machine types found in cluster for shorthand %q: %v. Please pass the required machine type with --compute-type to disambiguate", prefix, inCluster)
	}
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
	if g.mode == orchestrator.TypeK8s {
		return nil
	}
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

func (g *GKEOrchestrator) resolveResourcesAndGates(opts *ManifestOptions, profile JobProfile, job orchestrator.JobDefinition) (JobProfile, error) {
	isTPU := profile.IsTPU || (g.mode != orchestrator.TypeK8s && config.IsTPU(job.MachineType))
	isGPU := !profile.IsCPUMachine && !isTPU
	if isGPU && job.GKEScheduler == "gke.io/topology-aware-auto" {
		opts.SchedulingGates = indentYaml("schedulingGates:\n  - name: \"gke.io/topology-aware-auto-"+job.WorkloadName+"\"", 14)
		opts.SchedulerName = ""
	}

	opts.ParallelContainers = g.resolveParallelContainerCount(job)

	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(*opts, profile)
	if err != nil {
		logging.Warn("Warning: failed to calculate resource limits: %v", err)
	} else {
		if opts.ComputeType != "" && gpuLimit == "" && tpuLimit == "" {
			logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.ComputeType)
			opts.ComputeType = ""
		}
		resStr, err := g.buildResourcesStringWithGPUKey(cpuLimit, memoryLimit, gpuLimit, g.resolveGPUResourceName(*opts, profile), tpuLimit, 16)
		if err != nil {
			return profile, err
		}
		opts.ResourcesString = resStr
	}

	return profile, nil
}

func (g *GKEOrchestrator) resolveParallelContainerCount(job orchestrator.JobDefinition) int {
	mt := strings.ToLower(strings.TrimSpace(job.MachineType))
	if g.mode == orchestrator.TypeK8s || !job.UseParallelContainers || job.IsPathwaysJob || !strings.Contains(mt, "tpu7x") {
		return 1
	}
	if chips := config.AcceleratorsPerVM(mt); chips >= 2 && chips%2 == 0 {
		return 2
	}
	return 1
}

// calculateAllocatableCPUMilli mirrors modules/compute/gke-node-pool/outputs.tf
func calculateAllocatableCPUMilli(vCPUs int) int64 {
	if vCPUs <= 0 {
		return 0
	}
	// GKE CPU reservations: 60m (1st core), 10m (2nd), 10m (3rd & 4th), 2.5m/core (>4)
	var reserveMilli int64 = 60
	if vCPUs > 1 {
		reserveMilli += 10
	}
	if vCPUs > 2 {
		thirdFourth := min(vCPUs-2, 2)
		reserveMilli += int64(thirdFourth) * 5
	}
	if vCPUs > 4 {
		reserveMilli += int64(vCPUs-4) * 5 / 2 // 2.5m per core
	}
	return int64(vCPUs)*1000 - reserveMilli
}

func calculateK8sCPUResourceLimit(profile JobProfile, hasGCSFuse bool) string {
	if profile.AllocatableCPUMilli > 0 {
		return fmt.Sprintf("%dm", calculateFullNodeCPURequestFromAllocatableMilli(profile.AllocatableCPUMilli, hasGCSFuse))
	}
	if profile.CapacityCount <= 0 {
		return ""
	}
	return fmt.Sprintf("%dm", calculateFullNodeCPURequestMilli(profile.CapacityCount, hasGCSFuse))
}

func calculateFullNodeCPURequestFromAllocatableMilli(allocatable int64, hasGCSFuse bool) int64 {
	var request int64
	if allocatable > 2000 {
		// Leave headroom for 1 CPU of system DaemonSets
		request = allocatable - 1000
	} else {
		// On small nodes, take just over half to guarantee scheduling
		request = allocatable/2 + 100
	}
	if hasGCSFuse {
		request -= 250 // gcsfuse sidecar
	}
	return max(100, request)
}

// calculateFullNodeCPURequestMilli mirrors modules/compute/gke-job-template/main.tf
func calculateFullNodeCPURequestMilli(vCPUs int, hasGCSFuse bool) int64 {
	return calculateFullNodeCPURequestFromAllocatableMilli(calculateAllocatableCPUMilli(vCPUs), hasGCSFuse)
}

// gceTPUFamilies centralizes GCE/GKE/TPU-slice family tokens and observed cluster-label shorthands
// (such as ironwood, v7x, tpu7x-podslice, v5lite-*, and tpu-v*-slice) locally to gceTPUHardwareResolver
// without expanding user-facing config.AcceleratorShorthandMap.
var gceTPUFamilies = []gceTPUFamilySpec{
	{
		canonical:      "tpu7x",
		normTokens:     []string{"tpu7x", "tpu-v7x", "tpu-7x", "ironwood"},
		shorthandBases: []string{"tpu7", "tpu7x", "tpu-7x", "tpu-v7x", "v7x", "ironwood"},
		tpu7xChipRules: true,
	},
	{
		canonical:      "v6e",
		normTokens:     []string{"ct6e", "v6e", "trillium"},
		shorthandBases: []string{"v6e", "tpu-v6e", "trillium"},
	},
	{
		canonical:      "v5p",
		normTokens:     []string{"ct5p", "v5p"},
		shorthandBases: []string{"v5p", "tpu-v5p"},
	},
	{
		canonical:      "v5e",
		normTokens:     []string{"ct5lp", "ct5l", "v5e", "v5lite", "v5-lite"},
		shorthandBases: []string{"v5e", "v5lite", "v5-lite", "v5litepod", "tpu-v5e", "tpu-v5-lite", "tpu-v5-lite-podslice", "tpu-v5-lite-device"},
	},
	{
		canonical:      "v4",
		normTokens:     []string{"ct4p", "tpu-v4"},
		shorthandBases: []string{"v4", "tpu-v4"},
	},
}

func (gceTPUHardwareResolver) IsTPUMachine(machineType string) bool {
	s := strings.ToLower(strings.TrimSpace(machineType))
	if s == "" {
		return false
	}
	return isKnownGCETPUMachine(s) || isGCEAcceleratorShorthand(s)
}

func (gceTPUHardwareResolver) NormalizeTPUType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	norm := strings.ReplaceAll(s, "_", "-")
	for _, fam := range gceTPUFamilies {
		if matchesNormalizedTPUFamily(norm, fam) {
			return fam.canonical
		}
	}
	return s
}

func (r gceTPUHardwareResolver) InferTPUMachine(tpuType, rawMachine string, chipsPerNode int) string {
	trimmedMachine := strings.TrimSpace(rawMachine)
	if trimmedMachine != "" && !isGCEAcceleratorShorthand(trimmedMachine) {
		return trimmedMachine
	}
	chips := chipsPerNode
	if chips <= 0 && trimmedMachine != "" {
		chips = r.ChipsPerNode(trimmedMachine)
	}
	normType := r.NormalizeTPUType(tpuType)
	if normType == "" && trimmedMachine != "" {
		normType = r.NormalizeTPUType(trimmedMachine)
	}
	return defaultGCETPUMachineType(normType, chips)
}

func (r gceTPUHardwareResolver) ChipsPerNode(machineType string) int {
	trimmed := strings.TrimSpace(machineType)
	if trimmed == "" || !r.IsTPUMachine(trimmed) {
		return 0
	}
	lower := strings.ToLower(trimmed)
	resolved := resolveGCETPUShorthand(lower)
	if isKnownGCETPUMachine(resolved) {
		if n, ok := parseStrictChipSuffix(resolved); ok {
			return n
		}
		return config.AcceleratorsPerVM(resolved)
	}
	return extraTPUShorthandChips(lower)
}

func matchesNormalizedTPUFamily(norm string, fam gceTPUFamilySpec) bool {
	for _, tok := range fam.normTokens {
		if strings.Contains(norm, tok) {
			return true
		}
	}
	for _, base := range fam.shorthandBases {
		if norm == base || strings.HasPrefix(norm, base+"-") {
			return true
		}
	}
	return false
}

// resolveGCETPUShorthand delegates official shorthands to config.ResolveMachineType
// and normalizes cluster-label family aliases (e.g. v5lite-*, tpu-v6e-*, tpu-v5-lite-*)
// via gceTPUFamilies before lookup.
func resolveGCETPUShorthand(lower string) string {
	candidate := lower
	if fam, rest, ok := matchGCEFamilyShorthand(lower); ok && rest != "" && isAllDigits(rest) {
		candidate = fam.canonical + "-" + rest
	}
	if resolved := config.ResolveMachineType(candidate); resolved != candidate && config.IsTPU(resolved) {
		return strings.ToLower(resolved)
	}
	return lower
}

func parseStrictChipSuffix(machineType string) (int, bool) {
	lower := strings.ToLower(strings.TrimSpace(machineType))
	idx := strings.LastIndex(lower, "-")
	if idx < 0 || !strings.HasSuffix(lower, "t") {
		return 0, false
	}
	digits := lower[idx+1 : len(lower)-1]
	if digits == "" || !isAllDigits(digits) {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil && n > 0
}

func extraTPUShorthandChips(lower string) int {
	fam, rest, ok := matchGCEFamilyShorthand(lower)
	if !ok || !fam.tpu7xChipRules {
		return 0
	}
	if rest == "1" {
		return 1
	}
	return 4
}

func isKnownGCETPUMachine(lower string) bool {
	switch lower {
	case "tpu7x-standard", "ct6e-standard", "ct5p-hightpu", "ct5lp-hightpu", "ct5l-hightpu", "ct4p-hightpu":
		return true
	}
	if _, hasChipSuffix := parseStrictChipSuffix(lower); !hasChipSuffix {
		return false
	}
	for _, prefix := range []string{"tpu7x-", "ct6e-", "ct5p-", "ct5lp-", "ct5l-", "ct4p-"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isGCEAcceleratorShorthand(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || isKnownGCETPUMachine(s) {
		return false
	}
	if resolved := resolveGCETPUShorthand(s); resolved != s {
		return true
	}
	_, _, ok := matchGCEFamilyShorthand(s)
	return ok
}

func matchGCEFamilyShorthand(s string) (gceTPUFamilySpec, string, bool) {
	for _, fam := range gceTPUFamilies {
		for _, base := range fam.shorthandBases {
			if s == base {
				return fam, "", true
			}
			if rest, hasPrefix := strings.CutPrefix(s, base+"-"); hasPrefix && isValidShorthandSuffix(rest) {
				return fam, rest, true
			}
		}
	}
	return gceTPUFamilySpec{}, "", false
}

func isValidShorthandSuffix(rest string) bool {
	return (rest != "" && isAllDigits(rest)) || rest == "slice" || rest == "podslice"
}

func defaultGCETPUMachineType(tpuType string, chipsPerNode int) string {
	switch tpuType {
	case "tpu7x":
		if chipsPerNode == 1 {
			return "tpu7x-standard-1t"
		}
		return config.ResolveMachineType("tpu7x")
	case "v6e":
		return config.ResolveMachineType(fmt.Sprintf("v6e-%d", normalizeGCEChipCount(chipsPerNode)))
	case "v5p":
		return config.ResolveMachineType("v5p-8")
	case "v5e":
		return config.ResolveMachineType(fmt.Sprintf("v5e-%d", normalizeGCEChipCount(chipsPerNode)))
	case "v4":
		return config.ResolveMachineType("v4-8")
	default:
		return ""
	}
}

func normalizeGCEChipCount(chips int) int {
	switch chips {
	case 1, 4, 8:
		return chips
	default:
		return 4
	}
}

func allocatableMilliToWholeCPUs(milli int64) int {
	if milli <= 0 {
		return 0
	}
	return int((milli + 999) / 1000)
}

func (g *GKEOrchestrator) resolveGPUResourceName(opts ManifestOptions, profile JobProfile) string {
	if profile.GPUResourceName != "" {
		return profile.GPUResourceName
	}
	if g.mode == orchestrator.TypeK8s {
		if profile.K8sPool != nil && profile.K8sPool.GPUResourceName != "" {
			return profile.K8sPool.GPUResourceName
		}
		if mt := strings.TrimSpace(opts.MachineType); k8s.IsGPUResourceName(mt) {
			return mt
		}
		if ct := strings.TrimSpace(opts.ComputeType); k8s.IsGPUResourceName(ct) {
			return ct
		}
		if pool, ok := g.findK8sPoolForRequest(opts.ComputeType, opts.MachineType); ok && pool.GPUResourceName != "" {
			return pool.GPUResourceName
		}
	}
	return "nvidia.com/gpu"
}

func (g *GKEOrchestrator) calculateK8sMachineResourceLimits(opts ManifestOptions, profile JobProfile) (cpu, mem, gpu, tpu string, err error) {
	if profile.CapacityCount > 0 {
		if profile.IsTPU {
			return "", "", "", strconv.Itoa(profile.CapacityCount), nil
		}
		return "", "", strconv.Itoa(profile.CapacityCount), "", nil
	}
	if profile.K8sPool != nil {
		return limitsFromK8sPool(*profile.K8sPool)
	}
	if pool, ok := g.findK8sPoolForRequest(opts.ComputeType, opts.MachineType); ok {
		return limitsFromK8sPool(pool)
	}
	machineName := cmp.Or(strings.TrimSpace(opts.MachineType), strings.TrimSpace(opts.ComputeType))
	cap, err := g.resolveK8sMachineCapabilities(machineName)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to resolve machine type %s: %w", machineName, err)
	}
	if len(cap.Accelerators) == 0 || cap.Accelerators[0].Count <= 0 {
		return "", "", "", "", fmt.Errorf("failed to determine accelerator capacity for machine type %s", machineName)
	}
	count := cap.Accelerators[0].Count
	if defaultTPUHardwareResolver.IsTPUMachine(machineName) {
		return "", "", "", strconv.Itoa(count), nil
	}
	return "", "", strconv.Itoa(count), "", nil
}

func limitsFromK8sPool(pool k8s.PoolCapacity) (cpu, mem, gpu, tpu string, err error) {
	if pool.IsTPU {
		chips := pool.TPUsPerNode
		if chips <= 0 {
			chips = config.AcceleratorsPerVM(pool.MachineType)
		}
		if chips <= 0 {
			chips = 4
		}
		return "", "", "", strconv.Itoa(chips), nil
	}
	if pool.IsGPU {
		if pool.GPUsPerNode <= 0 {
			return "", "", "", "", fmt.Errorf("matched GPU pool %q reports 0 allocatable GPUs per node", cmp.Or(pool.MachineType, pool.GPUType))
		}
		return "", "", strconv.Itoa(pool.GPUsPerNode), "", nil
	}
	return "", "", "", "", fmt.Errorf("pool %q has no accelerator capacity", pool.MachineType)
}

func (g *GKEOrchestrator) resolveK8sMachineCapabilities(machineType string) (MachineTypeCap, error) {
	if pool, ok := g.findK8sPoolForMachineType(machineType); ok {
		return machineTypeCapFromPool(pool, g.GenerateGKENodeSelectorLabel(machineType))
	}
	if g.inventory != nil && g.inventory.TPU != nil && defaultTPUHardwareResolver.IsTPUMachine(machineType) {
		wantType := defaultTPUHardwareResolver.NormalizeTPUType(machineType)
		if g.inventory.TPU.TPUType == "" || wantType == "" || strings.EqualFold(g.inventory.TPU.TPUType, wantType) {
			return g.machineTypeCapFromTPUInventory(machineType), nil
		}
	}
	return fallbackK8sMachineTypeCap(machineType, g.GenerateGKENodeSelectorLabel(machineType)), nil
}

func poolPreferenceScore(pool k8s.PoolCapacity) int {
	score := 0
	if (!pool.IsGPU || pool.GPUsPerNode > 0) && (!pool.IsTPU || pool.TPUsPerNode > 0) {
		score += 8
	}
	if pool.ReadyNodes > 0 {
		score += 4
	}
	if !pool.IsControlPlane {
		score += 2
	}
	if !pool.IsSystemPool {
		score++
	}
	return score
}

func selectBestK8sPool(candidates []k8s.PoolCapacity) (k8s.PoolCapacity, bool) {
	if len(candidates) == 0 {
		return k8s.PoolCapacity{}, false
	}
	best := candidates[0]
	bestScore := poolPreferenceScore(best)
	for _, c := range candidates[1:] {
		if s := poolPreferenceScore(c); s > bestScore {
			best = c
			bestScore = s
		}
	}
	return best, true
}

func (g *GKEOrchestrator) findK8sPoolForMachineType(machineType string) (k8s.PoolCapacity, bool) {
	return g.findK8sPoolForRequest("", machineType)
}

func (g *GKEOrchestrator) findK8sPoolForRequest(computeType, machineType string) (k8s.PoolCapacity, bool) {
	if g.inventory == nil {
		return k8s.PoolCapacity{}, false
	}
	ct := strings.TrimSpace(computeType)
	mt := strings.TrimSpace(machineType)
	if ct != "" {
		if pool, ok := g.findK8sGPUPoolByQuery(ct); ok {
			return pool, true
		}
	}
	if mt != "" {
		if pool, ok := g.findK8sPoolByMachineType(mt); ok {
			return pool, true
		}
		if pool, ok := g.findK8sGPUPoolByQuery(mt); ok {
			return pool, true
		}
		if defaultTPUHardwareResolver.IsTPUMachine(mt) {
			return g.findK8sTPUPoolForMachineType(mt)
		}
	}
	if ct != "" && ct != mt {
		if pool, ok := g.findK8sPoolByMachineType(ct); ok {
			return pool, true
		}
		if defaultTPUHardwareResolver.IsTPUMachine(ct) {
			return g.findK8sTPUPoolForMachineType(ct)
		}
	}
	return k8s.PoolCapacity{}, false
}

func (g *GKEOrchestrator) findK8sPoolByMachineType(machineType string) (k8s.PoolCapacity, bool) {
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if strings.EqualFold(pool.MachineType, machineType) {
			matches = append(matches, pool)
		}
	}
	return selectBestK8sPool(matches)
}

func (g *GKEOrchestrator) findK8sGPUPoolByQuery(query string) (k8s.PoolCapacity, bool) {
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if !pool.IsGPU {
			continue
		}
		if strings.EqualFold(pool.GPUType, query) || strings.EqualFold(pool.GPUResourceName, query) || strings.EqualFold(name, query) {
			matches = append(matches, pool)
		}
	}
	return selectBestK8sPool(matches)
}

func (g *GKEOrchestrator) findK8sTPUPoolForMachineType(machineType string) (k8s.PoolCapacity, bool) {
	wantType := defaultTPUHardwareResolver.NormalizeTPUType(machineType)
	wantChips, _ := parseStrictChipSuffix(machineType)
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if !pool.IsTPU || wantType == "" || !strings.EqualFold(pool.TPUType, wantType) {
			continue
		}
		if wantChips > 0 && pool.TPUsPerNode > 0 && pool.TPUsPerNode != wantChips {
			continue
		}
		matches = append(matches, pool)
	}
	return selectBestK8sPool(matches)
}

func machineTypeCapFromPool(pool k8s.PoolCapacity, tpuAccelLabel string) (MachineTypeCap, error) {
	cap := MachineTypeCap{
		GuestCpus: allocatableMilliToWholeCPUs(pool.AllocatableCPUMilli),
		MemoryMb:  pool.MemMiBPerNode,
	}
	if pool.IsTPU {
		chips := pool.TPUsPerNode
		if chips <= 0 {
			chips = config.AcceleratorsPerVM(pool.MachineType)
		}
		if chips <= 0 {
			chips = 4
		}
		cap.Accelerators = append(cap.Accelerators, struct {
			Count int    `json:"guestAcceleratorCount"`
			Type  string `json:"guestAcceleratorType"`
		}{Count: chips, Type: cmp.Or(strings.TrimSpace(tpuAccelLabel), "tpu")})
	} else if pool.IsGPU {
		if pool.GPUsPerNode <= 0 {
			return MachineTypeCap{}, fmt.Errorf("matched GPU pool %q reports 0 allocatable GPUs per node", cmp.Or(pool.MachineType, pool.GPUType))
		}
		cap.Accelerators = append(cap.Accelerators, struct {
			Count int    `json:"guestAcceleratorCount"`
			Type  string `json:"guestAcceleratorType"`
		}{Count: pool.GPUsPerNode, Type: cmp.Or(strings.TrimSpace(pool.GPUType), "nvidia-gpu")})
	}
	return cap, nil
}

func (g *GKEOrchestrator) machineTypeCapFromTPUInventory(machineType string) MachineTypeCap {
	cap := MachineTypeCap{}
	if g.inventory.TPUPoolCap != nil {
		cap.GuestCpus = allocatableMilliToWholeCPUs(g.inventory.TPUPoolCap.AllocatableCPUMilli)
		cap.MemoryMb = g.inventory.TPUPoolCap.MemMiBPerNode
	}
	chips, hasExplicitChips := parseStrictChipSuffix(machineType)
	if !hasExplicitChips {
		chips = g.inventory.TPU.ChipsPerNode
	}
	if chips <= 0 {
		chips = config.AcceleratorsPerVM(machineType)
	}
	if chips <= 0 {
		chips = 4
	}
	cap.Accelerators = append(cap.Accelerators, struct {
		Count int    `json:"guestAcceleratorCount"`
		Type  string `json:"guestAcceleratorType"`
	}{Count: chips, Type: cmp.Or(strings.TrimSpace(g.GenerateGKENodeSelectorLabel(machineType)), "tpu")})
	return cap
}

func fallbackK8sMachineTypeCap(machineType, accelLabel string) MachineTypeCap {
	if defaultTPUHardwareResolver.IsTPUMachine(machineType) {
		chips := config.AcceleratorsPerVM(machineType)
		if chips <= 0 {
			chips = 4
		}
		return MachineTypeCap{
			Accelerators: []struct {
				Count int    `json:"guestAcceleratorCount"`
				Type  string `json:"guestAcceleratorType"`
			}{{Count: chips, Type: cmp.Or(strings.TrimSpace(accelLabel), "tpu")}},
		}
	}
	if count, isGPU := inferFallbackGPUCount(machineType); isGPU {
		return MachineTypeCap{
			Accelerators: []struct {
				Count int    `json:"guestAcceleratorCount"`
				Type  string `json:"guestAcceleratorType"`
			}{{Count: count, Type: cmp.Or(strings.TrimSpace(accelLabel), strings.TrimSpace(machineType))}},
		}
	}
	if vCPUs := inferFallbackGCECPUCount(machineType); vCPUs > 0 {
		return MachineTypeCap{GuestCpus: vCPUs}
	}
	if strings.TrimSpace(machineType) != "" {
		logging.Warn("Unable to infer vCPU count for machine type %q without cluster node inventory; pod will omit CPU requests/limits (BestEffort QoS), which ResourceQuota or Kueue may reject.", machineType)
	}
	return MachineTypeCap{}
}

func stripGCEMachineVariantSuffixes(lower string) string {
	for {
		trimmed := lower
		for _, suffix := range []string{"-nolocalssd", "-nolssd", "-lssd", "-metal", "-ext"} {
			if cut, ok := strings.CutSuffix(trimmed, suffix); ok {
				trimmed = cut
			}
		}
		if trimmed == lower {
			return trimmed
		}
		lower = trimmed
	}
}

func hasKnownGPUFamilyPrefix(lower string) bool {
	for _, prefix := range []string{"a2-", "a3-", "a4-", "a4x-", "a5x-", "g2-", "g4-"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return strings.Contains(lower, "gpu")
}

func inferG2OrG4GPUCount(lower string) (int, bool) {
	if rest, ok := strings.CutPrefix(lower, "g2-standard-"); ok {
		return g2GPUCountFromVCPUs(rest)
	}
	if rest, ok := strings.CutPrefix(lower, "g2-custom-"); ok {
		parts := strings.SplitN(rest, "-", 2)
		return g2GPUCountFromVCPUs(parts[0])
	}
	if rest, ok := strings.CutPrefix(lower, "g4-standard-"); ok {
		vcpus, err := strconv.Atoi(rest)
		if err != nil || vcpus <= 0 {
			return 0, false
		}
		switch vcpus {
		case 384:
			return 8, true
		case 192:
			return 4, true
		case 96:
			return 2, true
		default:
			return 1, true
		}
	}
	return 0, false
}

func g2GPUCountFromVCPUs(vcpuStr string) (int, bool) {
	vcpus, err := strconv.Atoi(vcpuStr)
	if err != nil || vcpus <= 0 {
		return 0, false
	}
	switch vcpus {
	case 96:
		return 8, true
	case 48:
		return 4, true
	case 24:
		return 2, true
	default:
		return 1, true
	}
}

func inferFallbackGPUCount(machineType string) (int, bool) {
	trimmed := strings.TrimSpace(machineType)
	if k8s.IsGPUResourceName(trimmed) {
		return 1, true
	}
	lower := stripGCEMachineVariantSuffixes(strings.ToLower(trimmed))
	if count, ok := inferG2OrG4GPUCount(lower); ok {
		return count, true
	}
	if hasKnownGPUFamilyPrefix(lower) {
		if idx := strings.LastIndex(lower, "-"); idx != -1 && strings.HasSuffix(lower, "g") {
			if n, err := strconv.Atoi(lower[idx+1 : len(lower)-1]); err == nil && n > 0 {
				return n, true
			}
		}
	}
	if strings.Contains(lower, "nvidia") || strings.Contains(lower, "gpu") {
		return 1, true
	}
	return 0, false
}

func inferFallbackGCECPUCount(machineType string) int {
	lower := stripGCEMachineVariantSuffixes(strings.ToLower(strings.TrimSpace(machineType)))
	if rest, ok := strings.CutPrefix(lower, "custom-"); ok {
		return parsePositiveInt(strings.SplitN(rest, "-", 2)[0])
	}
	if !hasGCECPUPrefix(lower) {
		return 0
	}
	parts := strings.Split(lower, "-")
	if len(parts) < 3 {
		return 0
	}
	if parts[1] == "custom" {
		return parsePositiveInt(parts[2])
	}
	return parsePositiveInt(parts[len(parts)-1])
}

func hasGCECPUPrefix(lower string) bool {
	for _, p := range []string{
		"e2-", "n1-", "n2-", "n2d-", "n4-", "c2-", "c2d-", "c3-", "c3d-", "c4-", "c4a-", "c4d-",
		"h3-", "h4d-", "m1-", "m2-", "m3-", "m4-", "t2a-", "t2d-", "z3-",
	} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func parsePositiveInt(s string) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return 0
}

func (g *GKEOrchestrator) isK8sTPURequest(computeType, machineType string) bool {
	if pool, ok := g.findK8sPoolForRequest(computeType, machineType); ok {
		return pool.IsTPU
	}
	if g.mode != orchestrator.TypeK8s {
		return config.IsTPU(machineType) || config.IsTPU(computeType)
	}
	resolver := defaultTPUHardwareResolver
	return resolver.IsTPUMachine(machineType) || resolver.IsTPUMachine(computeType)
}

func (g *GKEOrchestrator) resolveK8sHardwareRequirements(job *orchestrator.JobDefinition) (JobProfile, bool, bool, error) {
	unlabeledCPU, err := g.resolveK8sComputeType(job)
	if err != nil {
		return JobProfile{}, false, false, err
	}
	if unlabeledCPU {
		return g.buildUnlabeledCPUProfile(job)
	}

	job.MachineType = g.resolveK8sMachineType(job.ComputeType, job.NodeConstraint)
	matchedPool, hasPool := g.findK8sPoolForRequest(job.ComputeType, job.MachineType)
	isTPU := (hasPool && matchedPool.IsTPU) || (!hasPool && (defaultTPUHardwareResolver.IsTPUMachine(job.MachineType) || defaultTPUHardwareResolver.IsTPUMachine(job.ComputeType)))
	if isTPU {
		if job.NumNodesSpecified {
			return JobProfile{}, false, false, fmt.Errorf("--num-nodes cannot be used with TPU jobs (it is calculated automatically from topology)")
		}
		if err := g.resolveK8sTPURequirements(job, matchedPool, hasPool); err != nil {
			return JobProfile{}, false, false, err
		}
	} else if job.Topology != "" {
		return JobProfile{}, false, false, fmt.Errorf("topology is not allowed for GPU and CPU jobs")
	}

	profile, err := g.buildK8sJobProfile(job, matchedPool, hasPool, isTPU)
	if err != nil {
		return JobProfile{}, false, false, err
	}
	return profile, false, false, nil
}

func (g *GKEOrchestrator) buildUnlabeledCPUProfile(job *orchestrator.JobDefinition) (JobProfile, bool, bool, error) {
	if job.Topology != "" {
		return JobProfile{}, false, false, fmt.Errorf("topology is not allowed for GPU and CPU jobs")
	}
	if job.NodesPerSlice <= 0 {
		job.NodesPerSlice = 1
	}
	cp := *g.inventory.CPUPoolCap
	return JobProfile{
		IsCPUMachine:        true,
		CapacityCount:       allocatableMilliToWholeCPUs(cp.AllocatableCPUMilli),
		AllocatableCPUMilli: cp.AllocatableCPUMilli,
		K8sPool:             &cp,
	}, false, false, nil
}

func (g *GKEOrchestrator) resolveK8sComputeType(job *orchestrator.JobDefinition) (unlabeledCPU bool, err error) {
	if strings.TrimSpace(job.ComputeType) != "" {
		return false, nil
	}
	if g.inventory != nil && g.inventory.ExplicitMode {
		return false, fmt.Errorf("--compute-type is required because listing cluster nodes was denied (403 Forbidden; explicit mode)")
	}
	if ct, source := g.discoverK8sDefaultComputeType(job.NodeConstraint); ct != "" {
		job.ComputeType = ct
		logging.Info("Auto-discovered compute type %q from %s.", ct, source)
		return false, nil
	}
	if g.inventory != nil && g.inventory.CPUPoolCap != nil {
		logging.Info("Auto-discovered unlabeled Kubernetes CPU worker pool.")
		return true, nil
	}
	return false, fmt.Errorf("--compute-type is required: no schedulable worker nodes were discovered in Kubernetes inventory")
}

func (g *GKEOrchestrator) findTargetSlices(nodeConstraint map[string]string) (names []string, found []k8s.SliceInfo, missing string, err error) {
	names, err = parseTargetSliceNames(nodeConstraint[k8s.TPUSliceNameLabel])
	if err != nil || len(names) == 0 {
		return names, nil, "", err
	}
	for _, name := range names {
		s, ok := g.findSliceByName(name)
		if !ok {
			if missing == "" {
				missing = name
			}
			continue
		}
		found = append(found, s)
	}
	return names, found, missing, nil
}

func (g *GKEOrchestrator) discoverK8sDefaultComputeType(nodeConstraint map[string]string) (string, string) {
	if _, slices, _, err := g.findTargetSlices(nodeConstraint); err == nil && len(slices) > 0 {
		for _, s := range slices {
			if ct := cmp.Or(strings.TrimSpace(s.MachineType), strings.TrimSpace(s.TPUType)); ct != "" {
				return ct, fmt.Sprintf("TPU slice %q", s.Name)
			}
		}
	}
	if g.inventory == nil {
		return "", ""
	}
	if g.inventory.TPU != nil {
		if ct := cmp.Or(strings.TrimSpace(g.inventory.TPU.TPUType), strings.TrimSpace(g.inventory.TPU.MachineType)); ct != "" {
			return ct, "Kubernetes TPU inventory"
		}
	}
	if gpuCT := g.firstK8sGPUComputeType(); gpuCT != "" {
		return gpuCT, "Kubernetes GPU inventory"
	}
	if g.inventory.CPUPoolCap != nil && g.inventory.CPUPoolCap.MachineType != "" {
		return g.inventory.CPUPoolCap.MachineType, "Kubernetes CPU inventory"
	}
	return "", ""
}

func (g *GKEOrchestrator) firstK8sGPUComputeType() string {
	if g.inventory == nil {
		return ""
	}
	var gpuPools []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if pool.IsGPU && pool.GPUsPerNode > 0 {
			gpuPools = append(gpuPools, pool)
		}
	}
	best, ok := selectBestK8sPool(gpuPools)
	if !ok {
		return ""
	}
	return cmp.Or(strings.TrimSpace(best.MachineType), strings.TrimSpace(best.GPUType), strings.TrimSpace(best.GPUResourceName))
}

func (g *GKEOrchestrator) preferK8sPoolMachineType(computeType, candidate string) string {
	if pool, found := g.findK8sPoolForRequest(computeType, candidate); found && pool.MachineType != "" {
		return pool.MachineType
	}
	return candidate
}

func (g *GKEOrchestrator) resolveK8sTPUShorthandFromPools(computeType, defaultCandidate string) string {
	if pool, found := g.findK8sPoolForMachineType(defaultCandidate); found && pool.MachineType != "" {
		return pool.MachineType
	}
	if g.inventory == nil {
		return defaultCandidate
	}
	wantType := defaultTPUHardwareResolver.NormalizeTPUType(defaultCandidate)
	normalizedShorthand := normalizeTPUShorthandForTopology(computeType)
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if !pool.IsTPU || pool.MachineType == "" || (wantType != "" && !strings.EqualFold(pool.TPUType, wantType)) {
			continue
		}
		if isCandidateCompatibleForComputeType(normalizedShorthand, pool.MachineType) {
			matches = append(matches, pool)
		}
	}
	if best, ok := selectBestK8sPool(matches); ok {
		return best.MachineType
	}
	if candidate := g.resolveTPUShorthandFromInventoryTPU(wantType, normalizedShorthand); candidate != "" {
		return candidate
	}
	return defaultCandidate
}

func (g *GKEOrchestrator) resolveTPUShorthandFromInventoryTPU(wantType, normalizedShorthand string) string {
	if g.inventory == nil || len(g.inventory.PoolNames) > 0 || g.inventory.TPU == nil {
		return ""
	}
	invTPU := g.inventory.TPU
	if wantType != "" && !strings.EqualFold(invTPU.TPUType, wantType) {
		return ""
	}
	candidate := cmp.Or(strings.TrimSpace(invTPU.MachineType), defaultTPUHardwareResolver.InferTPUMachine(invTPU.TPUType, "", invTPU.ChipsPerNode))
	if candidate != "" && isCandidateCompatibleForComputeType(normalizedShorthand, candidate) {
		return candidate
	}
	return ""
}

func sliceCandidateMachineType(s k8s.SliceInfo, wantType string) string {
	if s.TPUType != "" && wantType != "" && !strings.EqualFold(wantType, s.TPUType) {
		return ""
	}
	if s.MachineType != "" {
		return s.MachineType
	}
	if s.ChipsPerNode > 0 {
		return defaultTPUHardwareResolver.InferTPUMachine(cmp.Or(s.TPUType, wantType), "", s.ChipsPerNode)
	}
	return ""
}

func (g *GKEOrchestrator) resolveMachineFromNamedSlices(lower, wantType string, nodeConstraint map[string]string, isChipShorthand bool) string {
	_, slices, _, err := g.findTargetSlices(nodeConstraint)
	if err != nil || len(slices) == 0 {
		return ""
	}
	normalized := normalizeTPUShorthandForTopology(lower)
	for _, s := range slices {
		cand := sliceCandidateMachineType(s, wantType)
		if cand == "" {
			continue
		}
		if !isChipShorthand || isCandidateCompatibleForComputeType(normalized, cand) {
			return cand
		}
	}
	return ""
}

func (g *GKEOrchestrator) resolveK8sMachineType(computeType string, nodeConstraint map[string]string) string {
	trimmed := strings.TrimSpace(computeType)
	lower := strings.ToLower(trimmed)
	if k8s.IsGPUResourceName(trimmed) {
		return g.preferK8sPoolMachineType(trimmed, trimmed)
	}
	if mapped, ok := config.AcceleratorShorthandMap[lower]; ok {
		if defaultTPUHardwareResolver.IsTPUMachine(mapped) {
			if fromSlice := g.resolveShorthandFromNamedSlices(lower, nodeConstraint); fromSlice != "" {
				return fromSlice
			}
			return g.resolveK8sTPUShorthandFromPools(lower, mapped)
		}
		return g.preferK8sPoolMachineType(trimmed, mapped)
	}
	if resolved := g.resolveK8sFamilyShorthand(trimmed, lower, nodeConstraint); resolved != "" {
		return resolved
	}
	return g.preferK8sPoolMachineType(trimmed, trimmed)
}

func (g *GKEOrchestrator) resolveShorthandFromNamedSlices(lower string, nodeConstraint map[string]string) string {
	fam, rest, isFam := matchGCEFamilyShorthand(lower)
	if !isFam {
		return ""
	}
	isChipShorthand := rest != "" && isAllDigits(rest)
	return g.resolveMachineFromNamedSlices(lower, fam.canonical, nodeConstraint, isChipShorthand)
}

func (g *GKEOrchestrator) resolveK8sFamilyShorthand(computeType, lower string, nodeConstraint map[string]string) string {
	fam, rest, isFam := matchGCEFamilyShorthand(lower)
	if !isFam {
		return ""
	}
	isChipShorthand := rest != "" && isAllDigits(rest)
	if fromSlice := g.resolveMachineFromNamedSlices(lower, fam.canonical, nodeConstraint, isChipShorthand); fromSlice != "" {
		return fromSlice
	}
	chipsHint := 0
	if !isChipShorthand && g.inventory != nil && g.inventory.TPU != nil {
		if g.inventory.TPU.TPUType == "" || strings.EqualFold(g.inventory.TPU.TPUType, fam.canonical) {
			chipsHint = g.inventory.TPU.ChipsPerNode
		}
	}
	if inferred := defaultTPUHardwareResolver.InferTPUMachine("", computeType, chipsHint); defaultTPUHardwareResolver.IsTPUMachine(inferred) {
		return g.resolveK8sTPUShorthandFromPools(lower, inferred)
	}
	return ""
}

func (g *GKEOrchestrator) resolveK8sTPURequirements(job *orchestrator.JobDefinition, matchedPool k8s.PoolCapacity, hasPool bool) error {
	if done, err := g.applyExplicitSliceTPURequirements(job); err != nil || done {
		return err
	}
	if job.Topology == "" {
		topo, err := g.resolveK8sTPUTopology(job)
		if err != nil {
			return err
		}
		job.Topology = topo
	}
	if err := validateK8sTPUTopology(job.MachineType, job.Topology); err != nil {
		return err
	}
	chipsPerNode := g.resolveK8sTPUChipsPerNode(job.MachineType, matchedPool, hasPool, g.resolveTargetSliceChips(job.NodeConstraint))
	return calculateK8sTPUNodesPerSlice(job, chipsPerNode)
}

func (g *GKEOrchestrator) resolveTargetSliceChips(nodeConstraint map[string]string) int {
	_, slices, _, err := g.findTargetSlices(nodeConstraint)
	if err != nil {
		return 0
	}
	for _, s := range slices {
		if s.ChipsPerNode > 0 {
			return s.ChipsPerNode
		}
	}
	return 0
}

func validateK8sTPUTopology(machineType, topology string) error {
	if !config.TopologyRegex.MatchString(topology) {
		return fmt.Errorf("provided topology has invalid format %q", topology)
	}
	mt := strings.ToLower(strings.TrimSpace(machineType))
	if topology == "1x1x1" && strings.HasPrefix(mt, "tpu7x") && defaultTPUHardwareResolver.ChipsPerNode(mt) == 1 {
		return nil
	}
	if defaultTPUHardwareResolver.IsTPUMachine(mt) {
		normMT := mt
		if strings.HasPrefix(normMT, "ct5l-") {
			normMT = "ct5lp-" + strings.TrimPrefix(normMT, "ct5l-")
		}
		if err := config.ValidateHardwareRequest(normMT, topology); err != nil {
			// Newer GCE TPU families not yet in config's static 2D/3D shape tables still pass TopologyRegex.
			if !strings.Contains(err.Error(), "topology family is unknown") {
				return err
			}
		}
	}
	return nil
}

func (g *GKEOrchestrator) resolveK8sTPUChipsPerNode(machineType string, matchedPool k8s.PoolCapacity, hasPool bool, sliceChips int) int {
	if hasPool && matchedPool.TPUsPerNode > 0 {
		return matchedPool.TPUsPerNode
	}
	if sliceChips > 0 {
		return sliceChips
	}
	if chips, hasExplicit := parseStrictChipSuffix(machineType); hasExplicit {
		return chips
	}
	if g.inventory != nil && g.inventory.TPU != nil && g.inventory.TPU.ChipsPerNode > 0 {
		wantType := defaultTPUHardwareResolver.NormalizeTPUType(machineType)
		if g.inventory.TPU.TPUType == "" || wantType == "" || strings.EqualFold(g.inventory.TPU.TPUType, wantType) {
			return g.inventory.TPU.ChipsPerNode
		}
	}
	if chips := config.AcceleratorsPerVM(strings.ToLower(strings.TrimSpace(machineType))); chips > 0 {
		return chips
	}
	return 4
}

func calculateK8sTPUNodesPerSlice(job *orchestrator.JobDefinition, chipsPerNode int) error {
	if job.Topology == "" {
		return fmt.Errorf("could not resolve TPU topology for the provided machine type: %q", job.MachineType)
	}
	nodes, err := config.CalculateAcceleratorNodes(job.MachineType, job.Topology, chipsPerNode)
	if err != nil {
		return fmt.Errorf("failed to calculate nodes from topology: %w", err)
	}
	if nodes <= 0 {
		return fmt.Errorf("invalid nodes_per_slice (%d) for topology %s", nodes, job.Topology)
	}
	job.NodesPerSlice = nodes
	logging.Info("Dynamically determined nodes_per_slice for %s: %d", job.Topology, job.NodesPerSlice)
	return nil
}

func (g *GKEOrchestrator) validateSliceHardwareMatch(job *orchestrator.JobDefinition, sliceName string, s k8s.SliceInfo) error {
	if err := g.validateSliceTypeAndChipsMatch(job, sliceName, s); err != nil {
		return err
	}
	if job.Topology != "" && s.Topology != "" && !strings.EqualFold(job.Topology, s.Topology) {
		return fmt.Errorf("requested topology %q does not match slice %q topology %q", job.Topology, sliceName, s.Topology)
	}
	if job.Topology != "" && s.Topology == "" && s.NodesPerSlice > 0 {
		chips := cmp.Or(s.ChipsPerNode, defaultTPUHardwareResolver.ChipsPerNode(job.MachineType))
		if chips > 0 {
			if nodes, err := config.CalculateAcceleratorNodes(job.MachineType, job.Topology, chips); err == nil && nodes != s.NodesPerSlice {
				return fmt.Errorf("requested topology %q (%d nodes) does not match slice %q (%d nodes)", job.Topology, nodes, sliceName, s.NodesPerSlice)
			}
		}
	}
	return nil
}

func (g *GKEOrchestrator) validateSliceTypeAndChipsMatch(job *orchestrator.JobDefinition, sliceName string, s k8s.SliceInfo) error {
	resolver := defaultTPUHardwareResolver
	sameMachine := s.MachineType != "" && strings.EqualFold(strings.TrimSpace(job.MachineType), strings.TrimSpace(s.MachineType))
	if !sameMachine && s.TPUType != "" {
		jobTPUType := ""
		if pool, ok := g.findK8sPoolForRequest(job.ComputeType, job.MachineType); ok && pool.IsTPU && pool.TPUType != "" {
			jobTPUType = pool.TPUType
		} else {
			jobTPUType = resolver.NormalizeTPUType(cmp.Or(strings.TrimSpace(job.MachineType), strings.TrimSpace(job.ComputeType)))
		}
		if jobTPUType != "" && !strings.EqualFold(jobTPUType, s.TPUType) {
			return fmt.Errorf("requested TPU type %q does not match slice %q type %q", jobTPUType, sliceName, s.TPUType)
		}
	}
	if wantChips := resolver.ChipsPerNode(job.MachineType); wantChips > 0 && s.ChipsPerNode > 0 && wantChips != s.ChipsPerNode {
		return fmt.Errorf("requested TPU host shape %q (%d chips/node) does not match slice %q (%d chips/node)", job.MachineType, wantChips, sliceName, s.ChipsPerNode)
	}
	return nil
}

func deriveSliceTopologyFromNodes(job *orchestrator.JobDefinition, s k8s.SliceInfo) string {
	if s.NodesPerSlice <= 0 {
		return ""
	}
	resolver := defaultTPUHardwareResolver
	chips := cmp.Or(s.ChipsPerNode, resolver.ChipsPerNode(job.MachineType))
	if chips <= 0 {
		return ""
	}
	tpuType := cmp.Or(strings.TrimSpace(s.TPUType), resolver.NormalizeTPUType(cmp.Or(job.MachineType, job.ComputeType)))
	if tpuType == "" {
		return ""
	}
	shorthandCount := s.NodesPerSlice * chips
	// v4 and v5p shorthands express TensorCore counts (2 cores per chip).
	if strings.EqualFold(tpuType, "v4") || strings.EqualFold(tpuType, "v5p") {
		shorthandCount *= 2
	}
	topo, err := config.ResolveTopologyForChips(fmt.Sprintf("%s-%d", tpuType, shorthandCount), job.MachineType)
	if err != nil {
		return ""
	}
	return topo
}

func (g *GKEOrchestrator) applyExplicitSliceTPURequirements(job *orchestrator.JobDefinition) (bool, error) {
	rawSlice := strings.TrimSpace(job.NodeConstraint[k8s.TPUSliceNameLabel])
	if rawSlice == "" {
		return false, nil
	}
	if !g.jobSetEnabled() && strings.Contains(rawSlice, "|") {
		return false, fmt.Errorf("standalone batch/v1 Indexed Job without JobSet cannot target multiple TPU slices (%q); specify a single existing READY slice via --node-constraint %s=<ready-slice-name>",
			rawSlice, k8s.TPUSliceNameLabel)
	}
	_, slices, missing, err := g.findTargetSlices(job.NodeConstraint)
	if err != nil {
		return false, err
	}
	if missing != "" {
		if g.inventory != nil && !g.inventory.SliceInventoryUnavailable {
			return false, fmt.Errorf("TPU slice %q does not exist; target an existing READY slice via --node-constraint %s=<ready-slice-name>", missing, k8s.TPUSliceNameLabel)
		}
		if len(slices) == 0 {
			return false, nil
		}
	}
	if len(slices) == 0 {
		return false, nil
	}
	return g.applyResolvedTargetSlices(job, slices)
}

func (g *GKEOrchestrator) applyResolvedTargetSlices(job *orchestrator.JobDefinition, slices []k8s.SliceInfo) (bool, error) {
	first := slices[0]
	if err := g.populateSliceTopology(job, first.Name, first); err != nil {
		return false, err
	}
	for _, s := range slices[1:] {
		if err := g.validateSliceHardwareMatch(job, s.Name, s); err != nil {
			return false, err
		}
		if first.NodesPerSlice > 0 && s.NodesPerSlice > 0 && s.NodesPerSlice != first.NodesPerSlice {
			return false, fmt.Errorf("target TPU slice %q (%d nodes) does not match slice %q (%d nodes)", s.Name, s.NodesPerSlice, first.Name, first.NodesPerSlice)
		}
	}
	if first.NodesPerSlice > 0 && job.Topology != "" {
		job.NodesPerSlice = first.NodesPerSlice
		return true, nil
	}
	return false, nil
}

func (g *GKEOrchestrator) populateSliceTopology(job *orchestrator.JobDefinition, sliceName string, s k8s.SliceInfo) error {
	if job.Topology == "" {
		if shorthandTopo, err := resolveShorthandTopologyIfPresent(job.ComputeType, job.MachineType); err != nil {
			return err
		} else if shorthandTopo != "" {
			job.Topology = shorthandTopo
		}
	}
	if err := g.validateSliceHardwareMatch(job, sliceName, s); err != nil {
		return err
	}
	if job.Topology == "" {
		if s.Topology != "" {
			job.Topology = s.Topology
			logging.Info("Auto-discovered topology %q from TPU slice %q.", job.Topology, sliceName)
		} else if derived := deriveSliceTopologyFromNodes(job, s); derived != "" {
			job.Topology = derived
			logging.Info("Derived topology %q from TPU slice %q (%d nodes).", job.Topology, sliceName, s.NodesPerSlice)
		}
	}
	if job.Topology != "" {
		return validateK8sTPUTopology(job.MachineType, job.Topology)
	}
	return nil
}

func resolveShorthandTopologyIfPresent(computeType, machineType string) (string, error) {
	fam, rest, isFam := matchGCEFamilyShorthand(strings.ToLower(strings.TrimSpace(computeType)))
	if !isFam || rest == "" || !isAllDigits(rest) {
		return "", nil
	}
	return config.ResolveTopologyForChips(fam.canonical+"-"+rest, machineType)
}

func normalizeTPUShorthandForTopology(computeType string) string {
	lower := strings.ToLower(strings.TrimSpace(computeType))
	if fam, rest, ok := matchGCEFamilyShorthand(lower); ok && rest != "" && isAllDigits(rest) {
		return fam.canonical + "-" + rest
	}
	return lower
}

func (g *GKEOrchestrator) resolveK8sTPUTopology(job *orchestrator.JobDefinition) (string, error) {
	topo, err := resolveShorthandTopologyIfPresent(job.ComputeType, job.MachineType)
	if err != nil {
		return "", err
	}
	if topo != "" {
		logging.Info("Auto-resolved topology %s for shorthand %s", topo, job.ComputeType)
		return topo, nil
	}
	if g.inventory != nil && g.inventory.ExplicitMode {
		return "", fmt.Errorf("--topology is required for TPU workloads because listing cluster nodes was denied (403 Forbidden; explicit mode)")
	}
	if g.labelScheme() != k8s.SchemeTPUSlice && g.inventory != nil && g.inventory.TPU != nil && g.inventory.TPU.Topology != "" {
		logging.Info("Auto-discovered topology %q from Kubernetes inventory.", g.inventory.TPU.Topology)
		return g.inventory.TPU.Topology, nil
	}
	return "", fmt.Errorf("--topology is required for TPU machine type %s", job.MachineType)
}

func (g *GKEOrchestrator) buildK8sJobProfile(job *orchestrator.JobDefinition, matchedPool k8s.PoolCapacity, hasPool, isTPU bool) (JobProfile, error) {
	var poolPtr *k8s.PoolCapacity
	if hasPool {
		poolCopy := matchedPool
		poolPtr = &poolCopy
	}
	if isTPU {
		chips := g.resolveK8sTPUChipsPerNode(job.MachineType, matchedPool, hasPool, g.resolveTargetSliceChips(job.NodeConstraint))
		return JobProfile{
			IsCPUMachine:  false,
			IsTPU:         true,
			CapacityCount: chips,
			K8sPool:       poolPtr,
		}, nil
	}
	if job.NodesPerSlice <= 0 {
		job.NodesPerSlice = 1
	}
	if hasPool {
		if matchedPool.IsGPU {
			if matchedPool.GPUsPerNode <= 0 {
				return JobProfile{}, fmt.Errorf("matched GPU pool %q reports 0 allocatable GPUs per node", cmp.Or(matchedPool.MachineType, matchedPool.GPUType))
			}
			return JobProfile{
				IsCPUMachine:    false,
				CapacityCount:   matchedPool.GPUsPerNode,
				GPUResourceName: matchedPool.GPUResourceName,
				K8sPool:         poolPtr,
			}, nil
		}
		return JobProfile{
			IsCPUMachine:        true,
			CapacityCount:       allocatableMilliToWholeCPUs(matchedPool.AllocatableCPUMilli),
			AllocatableCPUMilli: matchedPool.AllocatableCPUMilli,
			K8sPool:             poolPtr,
		}, nil
	}
	gpuKey := ""
	if k8s.IsGPUResourceName(job.MachineType) {
		gpuKey = strings.TrimSpace(job.MachineType)
	} else if k8s.IsGPUResourceName(job.ComputeType) {
		gpuKey = strings.TrimSpace(job.ComputeType)
	}
	cap := fallbackK8sMachineTypeCap(job.MachineType, g.GenerateGKENodeSelectorLabel(job.MachineType))
	if len(cap.Accelerators) > 0 {
		return JobProfile{
			IsCPUMachine:    false,
			CapacityCount:   cap.Accelerators[0].Count,
			GPUResourceName: gpuKey,
		}, nil
	}
	return JobProfile{
		IsCPUMachine:  true,
		CapacityCount: cap.GuestCpus,
	}, nil
}
