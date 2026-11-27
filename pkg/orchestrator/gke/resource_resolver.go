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

	"k8s.io/apimachinery/pkg/api/resource"
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
		return g.getEffectiveCPUs(machineType, cap.GuestCpus), nil
	}
	return 0, fmt.Errorf("no accelerators or guestCpus found for machine type %s in zone %s", machineType, zone)
}

// getZonesForMachineType finds all candidate zones hosting the specified machineType in the cluster's node pools.
func (g *GKEOrchestrator) getZonesForMachineType(machineType string) []string {
	if strings.TrimSpace(machineType) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	for _, np := range g.clusterDesc.NodePools {
		if !strings.EqualFold(np.Config.MachineType, machineType) {
			continue
		}
		locs := np.Locations
		if len(locs) == 0 {
			locs = g.clusterZones
		}
		for _, loc := range locs {
			if trimmed := strings.TrimSpace(loc); trimmed != "" {
				seen[trimmed] = struct{}{}
			}
		}
	}

	zones := make([]string, 0, len(seen))
	for loc := range seen {
		zones = append(zones, loc)
	}
	slices.Sort(zones)
	return zones
}

func (g *GKEOrchestrator) napCandidateZones() []string {
	var zones []string
	for _, loc := range slices.Concat(g.clusterZones, g.clusterDesc.Autoscaling.AutoprovisioningLocations) {
		if trimmed := strings.TrimSpace(loc); trimmed != "" && !slices.Contains(zones, trimmed) {
			zones = append(zones, trimmed)
		}
	}
	return zones
}

func (g *GKEOrchestrator) resolveCandidateZonesForMachineType(machineType, zone string) ([]string, bool, error) {
	isRegion := len(strings.Split(zone, "-")) < 3
	if !isRegion {
		return []string{zone}, false, nil
	}
	if npZones := g.getZonesForMachineType(machineType); len(npZones) > 0 {
		return npZones, true, nil
	}
	if !g.napEnabled && !g.clusterDesc.Autoscaling.EnableNodeAutoprovisioning {
		return nil, true, fmt.Errorf("failed to fetch machine capabilities for %s: no node pool matching machine type found and GKE Node Auto-Provisioning is disabled", machineType)
	}
	if candidates := g.napCandidateZones(); len(candidates) > 0 {
		return candidates, true, nil
	}
	return nil, true, fmt.Errorf("failed to fetch machine capabilities for %s: regional cluster %q reported no candidate zones", machineType, zone)
}

func (g *GKEOrchestrator) FetchMachineCapabilities(machineType, zone string) (MachineTypeCap, error) {
	cacheKey := machineType + ":" + zone
	if g.machineCapCache != nil {
		if cap, ok := g.machineCapCache[cacheKey]; ok {
			return cap, nil
		}
	}

	zonesToTry, isRegion, err := g.resolveCandidateZonesForMachineType(machineType, zone)
	if err != nil {
		return MachineTypeCap{}, err
	}

	if g.machineTypeClient == nil {
		return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s: machine type client is not initialized", machineType)
	}

	var lastErr error
	for _, z := range zonesToTry {

		mt, err := g.machineTypeClient.GetMachineType(g.projectID, z, machineType)
		if err != nil {
			lastErr = err
			continue
		}
		if mt == nil {
			lastErr = fmt.Errorf("empty machine type response in zone %s", z)
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
		return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s: tried in all candidate zones %v but did not find machine type in any of them: %w", machineType, zonesToTry, lastErr)
	}
	return MachineTypeCap{}, fmt.Errorf("failed to fetch machine capabilities for %s in zone %s: %w", machineType, zone, lastErr)
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

	var cpuLim, memLim, gpuLim, tpuLim string
	if g.mode == orchestrator.TypeK8s {
		cpuLim, memLim, gpuLim, tpuLim, err = g.calculateK8sMachineResourceLimits(opts, profile)
	} else {
		mapped := g.GenerateGKENodeSelectorLabel(opts.ComputeType)
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
			return v, nil
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
		if idx := slices.IndexFunc(clusterMachineTypes, func(cmt string) bool { return strings.EqualFold(acceleratorType, cmt) }); idx >= 0 {
			return clusterMachineTypes[idx], nil
		}
	}

	// 3. Fail fast
	return "", fmt.Errorf("machine type %q could not be resolved from static maps or cluster state", acceleratorType)
}

func isNAPUnsupportedMachineType(lower string, hasComputeClass bool) bool {
	if lower == "f1-micro" || lower == "g1-small" {
		return true
	}
	return !hasComputeClass && isCustomMachineType(lower)
}

func napUnsupportedMachineTypeError(computeType string) error {
	return fmt.Errorf("machine type %q is not present in any eligible static node pool and cannot be auto-provisioned directly by GKE Node Auto-Provisioning; specify a predefined non-legacy GCE machine type with --compute-type (for example, n2-standard-32) or create a static node pool for %s", computeType, computeType)
}

// resolveJobMachineType maps --compute-type to a machine type, preferring the host shape used by the cluster's node pools for TPU shorthands.
func (g *GKEOrchestrator) resolveJobMachineType(computeType, napProvisioning, napReservation string, hasComputeClass bool) (string, error) {
	if mapped, isShorthand := config.AcceleratorShorthandMap[strings.ToLower(computeType)]; isShorthand {
		if config.IsTPU(mapped) {
			return g.resolveShorthandFromCluster(computeType, mapped)
		}
		return mapped, nil
	}

	machineName, err := g.resolveMachineName(computeType)
	if err != nil {
		resolved, shortErr := g.resolveShorthandFromCluster(computeType, "")
		if shortErr != nil && g.napEnabled && !config.IsKnownAccelerator(computeType) && strings.Contains(computeType, "-") {
			lower := strings.ToLower(strings.TrimSpace(computeType))
			if isNAPUnsupportedMachineType(lower, hasComputeClass) {
				return "", napUnsupportedMachineTypeError(computeType)
			}
			return lower, nil
		}
		return resolved, shortErr
	}
	if g.napEnabled && !config.IsKnownAccelerator(computeType) &&
		isNAPUnsupportedMachineType(strings.ToLower(strings.TrimSpace(machineName)), hasComputeClass) &&
		!g.hasStaticNodePoolForMachineType(machineName, napProvisioning, napReservation) {
		return "", napUnsupportedMachineTypeError(computeType)
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

func (g *GKEOrchestrator) resolveK8sHardwareWithCPUValidation(job *orchestrator.JobDefinition) (JobProfile, bool, bool, error) {
	profile, isDynamicSlicing, isStaticSlicing, err := g.resolveK8sHardwareRequirements(job)
	if err != nil {
		return JobProfile{}, false, false, err
	}
	if err := validateK8sCPUJobResources(cmp.Or(job.MachineType, job.ComputeType), job.CPURequest, job.MemoryRequest, profile); err != nil {
		return JobProfile{}, false, false, err
	}
	return profile, isDynamicSlicing, isStaticSlicing, nil
}

func (g *GKEOrchestrator) resolveHardwareRequirements(job *orchestrator.JobDefinition) (profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool, err error) {
	if g.mode == orchestrator.TypeK8s {
		return g.resolveK8sHardwareWithCPUValidation(job)
	}
	if job.ComputeType == "" {
		return JobProfile{}, false, false, nil
	}

	hasComputeClass := strings.TrimSpace(job.NodeConstraint[computeClassLabel]) != ""
	machineName, err := g.resolveJobMachineType(job.ComputeType, job.GKENAPProvisioning, job.GKENAPReservation, hasComputeClass)
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
	profile, err = g.determineIfCPUMachine(job.ComputeType, job.MachineType, job.ClusterLocation)
	if err != nil {
		return JobProfile{}, isDynamicSlicing, isStaticSlicing, err
	}
	profile.IsTPU = !profile.IsCPUMachine && config.IsTPU(machineName)
	if err := validateCPUJobResources(job.MachineType, job.CPURequest, job.MemoryRequest, profile); err != nil {
		return JobProfile{}, isDynamicSlicing, isStaticSlicing, err
	}
	if !profile.IsCPUMachine {
		if err := g.checkStaticAttachedGPUMachineError(job.ComputeType, job.MachineType); err != nil {
			return JobProfile{}, isDynamicSlicing, isStaticSlicing, err
		}
	}

	if err := g.validateConsumptionForStaticCluster(job); err != nil {
		return JobProfile{}, isDynamicSlicing, isStaticSlicing, err
	}

	return profile, isDynamicSlicing, isStaticSlicing, nil
}

// isSharedCoreMachineType returns true for GCE shared-core shapes (e.g., e2-micro, f1-micro)
// that expose 1 allocatable vCPU in GKE even when guestCpus is 2.
func isSharedCoreMachineType(machineType string) bool {
	trimmed := strings.TrimSpace(machineType)
	return trimmed != "" && strings.Count(trimmed, "-") <= 1
}

func (g *GKEOrchestrator) getThreadsPerCore(machineType string) string {
	if tpc, ok := g.machineTypeToThreadsPerCore[machineType]; ok {
		return tpc
	}
	for _, np := range g.clusterDesc.NodePools {
		if g.isSystemPool(np) {
			continue
		}
		if strings.EqualFold(np.Config.MachineType, machineType) && np.Config.AdvancedMachineFeatures != nil {
			return np.Config.AdvancedMachineFeatures.ThreadsPerCore
		}
	}
	return ""
}

func (g *GKEOrchestrator) getEffectiveCPUs(machineType string, guestCpus int) int {
	if guestCpus <= 0 {
		return 0
	}
	if isSharedCoreMachineType(machineType) {
		return 1
	}
	if g.getThreadsPerCore(machineType) == "1" && !strings.HasPrefix(machineType, "t2a") && !strings.HasPrefix(machineType, "t2d") {
		for k, cap := range g.machineCapCache {
			if strings.HasPrefix(k, machineType+":") && cap.GuestCpus > 0 && guestCpus == cap.GuestCpus/2 && cap.GuestCpus != guestCpus {
				return guestCpus
			}
		}
		return guestCpus / 2
	}
	return guestCpus
}

func appendPoolAcceleratorTypes(accels []string, poolAccels []gkeAccelerator) []string {
	for _, a := range poolAccels {
		if t := strings.TrimSpace(a.AcceleratorType); t != "" && !slices.Contains(accels, t) {
			accels = append(accels, t)
		}
	}
	return accels
}

// staticAttachedGPUAccelerators returns the unique attached GPU types when static non-system pools
// for machineType only provide attached GPUs (no active CPU-only pool exists).
func (g *GKEOrchestrator) staticAttachedGPUAccelerators(machineType string) []string {
	if g.napEnabled || strings.TrimSpace(machineType) == "" {
		return nil
	}
	var accels []string
	for _, np := range g.clusterDesc.NodePools {
		if np.Name == "default-pool" || g.isSystemPool(np) || !strings.EqualFold(np.Config.MachineType, machineType) {
			continue
		}
		if len(np.Config.Accelerators) == 0 {
			if g.getNodeCount(np) > 0 {
				return nil
			}
			continue
		}
		accels = appendPoolAcceleratorTypes(accels, np.Config.Accelerators)
	}
	slices.Sort(accels)
	return accels
}

func (g *GKEOrchestrator) checkStaticAttachedGPUMachineError(computeType, machineType string) error {
	if config.IsKnownAccelerator(computeType) {
		return nil
	}
	if _, isAttachedAccel := g.acceleratorToMachineType[strings.ToLower(strings.TrimSpace(computeType))]; isAttachedAccel {
		return nil
	}
	if accels := g.staticAttachedGPUAccelerators(machineType); len(accels) > 0 {
		return fmt.Errorf("static node pools for machine type %s only provide attached GPUs (%s); pass the accelerator label (e.g., --compute-type %s) or add a CPU-only node pool", machineType, strings.Join(accels, ", "), accels[0])
	}
	return nil
}

func (g *GKEOrchestrator) determineIfCPUMachine(computeType, machineType, clusterLocation string) (JobProfile, error) {
	if config.IsKnownAccelerator(computeType) || config.IsKnownAccelerator(machineType) {
		return JobProfile{}, nil
	}
	if _, isAttachedAccel := g.acceleratorToMachineType[strings.ToLower(strings.TrimSpace(computeType))]; isAttachedAccel {
		return JobProfile{}, nil
	}
	if len(g.staticAttachedGPUAccelerators(machineType)) > 0 {
		return JobProfile{}, nil
	}

	cap, err := g.FetchMachineCapabilities(machineType, clusterLocation)
	if err != nil {
		return JobProfile{}, fmt.Errorf("failed to fetch machine capabilities for %s: %w", machineType, err)
	}

	if len(cap.Accelerators) > 0 {
		return JobProfile{}, nil
	}

	logging.Info("Dynamically determined %s is a CPU-only machine during manifest preparation", machineType)
	return JobProfile{
		IsCPUMachine:  true,
		CapacityCount: g.getEffectiveCPUs(machineType, cap.GuestCpus),
		MemoryMb:      cap.MemoryMb,
	}, nil
}

// errTPUNumNodes is shared by the GKE preflight and the k8s post-discovery TPU --num-nodes checks.
var errTPUNumNodes = errors.New("--num-nodes cannot be used with TPU jobs (it is calculated automatically from topology)")

// validateGKETPUNumNodes rejects --num-nodes on GKE TPU jobs before any cluster access or add-on prompts. In k8s
// mode the equivalent check runs after node discovery, since custom TPU pools are only identifiable from inventory.
func validateGKETPUNumNodes(job orchestrator.JobDefinition) error {
	if !job.NumNodesSpecified {
		return nil
	}
	if config.IsTPU(job.ComputeType) {
		return errTPUNumNodes
	}
	return nil
}

func (g *GKEOrchestrator) resolveGKETPUHardware(job *orchestrator.JobDefinition) (isDynamicSlicing, isStaticSlicing bool, err error) {
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

const (
	mibBytes             int64 = 1024 * 1024
	gkeEvictionMemoryMiB int64 = 100
	gcsFuseCPUMilli      int64 = 250
	gcsFuseMemoryMiB     int64 = 256
	gcsFuseMemoryBytes   int64 = gcsFuseMemoryMiB * mibBytes
)

// formatAllocatableMemory floors bytes to MiB (~GiB when >= 1 GiB) so remediation hints never exceed allocatable.
func formatAllocatableMemory(bytes int64) string {
	if bytes < mibBytes {
		return fmt.Sprintf("%d bytes", bytes)
	}
	mib := bytes / mibBytes
	if mib >= 1024 {
		return fmt.Sprintf("%dMi (~%dGi)", mib, mib/1024)
	}
	return fmt.Sprintf("%dMi", mib)
}

func warnFractionalCPUOptions(machineType, cpuReq, memReq string, gcsFuseEnabled bool, effectiveCPUs, memoryMb int) {
	if cpuReq == "" {
		logging.Warn("--memory is set without --cpu; pods on shared CPU nodes receive minimal CPU share when sibling pods are busy; consider setting --cpu as well")
	} else if memReq == "" {
		logging.Warn("--cpu is set without --memory; pods on shared CPU nodes have no memory reservation and are prioritized for OOM eviction; consider setting --memory as well")
	}
	if !gcsFuseEnabled {
		return
	}
	warnGCSFuseCPUHeadroom(machineType, cpuReq, effectiveCPUs)
	warnGCSFuseMemoryHeadroom(machineType, memReq, memoryMb)
}

func warnGCSFuseCPUHeadroom(machineType, cpuReq string, effectiveCPUs int) {
	if cpuReq == "" || effectiveCPUs <= 0 {
		return
	}
	qty, err := k8s.ParseCPURequest(cpuReq)
	if err != nil {
		return
	}
	allocMilli := calculateAllocatableCPUMilli(effectiveCPUs)
	allocQty := resource.NewMilliQuantity(allocMilli, resource.DecimalSI)
	headroomMilli := max(0, allocMilli-gcsFuseCPUMilli)
	headroomQty := resource.NewMilliQuantity(headroomMilli, resource.DecimalSI)
	if qty.Cmp(*headroomQty) > 0 {
		logging.Warn("--cpu %s leaves less than %dm headroom on machine type %s for the GKE GCS FUSE CSI sidecar (pre-GKE 1.37.2 tiered allocatable CPU estimate: %s); pods may stay Pending unless you lower --cpu to at most %s or add the gke-gcsfuse/cpu-request pod annotation under pod_template in a custom base.tmpl (--gke-custom-templates-path)", cpuReq, gcsFuseCPUMilli, machineType, allocQty.String(), headroomQty.String())
	}
}

func warnGCSFuseMemoryHeadroom(machineType, memReq string, memoryMb int) {
	if memReq == "" || memoryMb <= 0 {
		return
	}
	qty, err := k8s.ParseMemoryRequest(memReq)
	if err != nil {
		return
	}
	allocBytes := calculateAllocatableMemoryBytes(memoryMb)
	if allocBytes <= gcsFuseMemoryBytes {
		logging.Warn("--memory %s on machine type %s (pre-GKE 1.37.2 tiered allocatable memory estimate %s) leaves less than %dMi for the GKE GCS FUSE CSI sidecar; pods may stay Pending unless you use a larger --compute-type or add the gke-gcsfuse/memory-request pod annotation under pod_template in a custom base.tmpl (--gke-custom-templates-path)", memReq, machineType, formatAllocatableMemory(allocBytes), gcsFuseMemoryMiB)
		return
	}
	headroomBytes := allocBytes - gcsFuseMemoryBytes
	headroomQty := resource.NewQuantity(headroomBytes, resource.BinarySI)
	if qty.Cmp(*headroomQty) > 0 {
		logging.Warn("--memory %s leaves less than %dMi headroom on machine type %s for the GKE GCS FUSE CSI sidecar (pre-GKE 1.37.2 tiered allocatable memory estimate: %s); pods may stay Pending on clusters older than GKE 1.37.2 unless you lower --memory to at most %s or add the gke-gcsfuse/memory-request pod annotation under pod_template in a custom base.tmpl (--gke-custom-templates-path)", memReq, gcsFuseMemoryMiB, machineType, formatAllocatableMemory(allocBytes), formatAllocatableMemory(headroomBytes))
	}
}

// calculateAllocatableMemoryBytes estimates GKE allocatable memory using tiered system reservations plus 100 MiB eviction headroom.
func calculateAllocatableMemoryBytes(memoryMb int) int64 {
	if memoryMb <= 0 {
		return 0
	}
	totalBytes := int64(memoryMb) * mibBytes
	evictionBytes := gkeEvictionMemoryMiB * mibBytes
	if memoryMb < 1024 {
		return max(0, totalBytes-255*mibBytes-evictionBytes)
	}
	var reserveBytes int64
	tiers := []struct {
		spanMiB int64
		pct     int64
	}{
		{4096, 25},   // first 4 GiB: 25%
		{4096, 20},   // next 4 GiB (4..8 GiB): 20%
		{8192, 10},   // next 8 GiB (8..16 GiB): 10%
		{114688, 6},  // next 112 GiB (16..128 GiB): 6%
		{1 << 60, 2}, // above 128 GiB: 2%
	}
	remaining := int64(memoryMb)
	for _, tier := range tiers {
		if remaining <= 0 {
			break
		}
		chunk := min(remaining, tier.spanMiB)
		reserveBytes += (chunk * mibBytes * tier.pct) / 100
		remaining -= chunk
	}
	return max(0, totalBytes-reserveBytes-evictionBytes)
}

// calculateMaxAllocatableMemoryBytes returns the GKE 1.37.2+ upper-bound allocatable memory.
func calculateMaxAllocatableMemoryBytes(memoryMb int) int64 {
	legacy := calculateAllocatableMemoryBytes(memoryMb)
	if memoryMb < 1024 {
		return legacy
	}
	// Min GKE 1.37.2+ reserve: 15Mi*8 (min max-pods) + 500Mi (COS) + 100Mi eviction.
	const minGKE137ReserveMiB = 15*8 + 500 + gkeEvictionMemoryMiB
	totalBytes := int64(memoryMb) * mibBytes
	return max(legacy, totalBytes-minGKE137ReserveMiB*mibBytes)
}

// calculateMaxAllocatableCPUMilli returns the GKE 1.37.2+ upper-bound allocatable CPU (1000m reserve cap).
func calculateMaxAllocatableCPUMilli(vCPUs int) int64 {
	if vCPUs <= 0 {
		return 0
	}
	legacy := calculateAllocatableCPUMilli(vCPUs)
	if vCPUs <= 1 {
		return legacy
	}
	return max(legacy, int64(vCPUs)*1000-1000)
}

func validateCPUJobResources(machineType, cpuReq, memReq string, profile JobProfile) error {
	cpuReq = strings.TrimSpace(cpuReq)
	memReq = strings.TrimSpace(memReq)
	if cpuReq == "" && memReq == "" {
		return nil
	}
	if !profile.IsCPUMachine {
		return fmt.Errorf("--cpu and --memory are only supported for CPU workloads, not accelerator machine type %q", machineType)
	}
	if err := validateCPURequestBound(machineType, cpuReq, profile.CapacityCount); err != nil {
		return err
	}
	return validateMemoryRequestBound(machineType, memReq, profile.MemoryMb)
}

func validateK8sCPUJobResources(machineType, cpuReq, memReq string, profile JobProfile) error {
	cpuReq = strings.TrimSpace(cpuReq)
	memReq = strings.TrimSpace(memReq)
	if cpuReq == "" && memReq == "" {
		return nil
	}
	if !profile.IsCPUMachine {
		return fmt.Errorf("--cpu and --memory are only supported for CPU workloads, not accelerator machine type %q", machineType)
	}
	target := cmp.Or(machineType, "discovered CPU node pool")
	if err := validateK8sCPURequestBound(target, cpuReq, profile.AllocatableCPUMilli); err != nil {
		return err
	}
	memMiB := 0
	if profile.K8sPool != nil {
		memMiB = profile.K8sPool.MemMiBPerNode
	}
	return validateK8sMemoryRequestBound(target, memReq, memMiB)
}

func validateK8sCPURequestBound(target, cpuReq string, allocatableMilli int64) error {
	if cpuReq == "" {
		return nil
	}
	qty, err := k8s.ParseCPURequest(cpuReq)
	if err != nil {
		return err
	}
	if allocatableMilli <= 0 {
		return nil
	}
	allocQty := resource.NewMilliQuantity(allocatableMilli, resource.DecimalSI)
	if qty.Cmp(*allocQty) > 0 {
		return fmt.Errorf("requested --cpu %s exceeds node allocatable CPU (%s) for %s; lower --cpu to at most %s, choose a larger --compute-type, or omit --cpu/--memory for full-node sizing", cpuReq, allocQty.String(), target, allocQty.String())
	}
	return nil
}

func validateK8sMemoryRequestBound(target, memReq string, memMiB int) error {
	if memReq == "" {
		return nil
	}
	qty, err := k8s.ParseMemoryRequest(memReq)
	if err != nil {
		return err
	}
	if memMiB <= 0 {
		return nil
	}
	allocBytes := int64(memMiB) * mibBytes
	allocQty := resource.NewQuantity(allocBytes, resource.BinarySI)
	if qty.Cmp(*allocQty) > 0 {
		allocDesc := formatAllocatableMemory(allocBytes)
		return fmt.Errorf("requested --memory %s exceeds node allocatable memory (%s) for %s; lower --memory to at most %s, choose a larger --compute-type, or omit --cpu/--memory for full-node sizing", memReq, allocDesc, target, allocDesc)
	}
	return nil
}

func validateCPURequestBound(machineType, cpuReq string, effectiveCPUs int) error {
	if cpuReq == "" {
		return nil
	}
	qty, err := k8s.ParseCPURequest(cpuReq)
	if err != nil {
		return err
	}
	if effectiveCPUs <= 0 {
		return nil
	}
	legacyMilli := calculateAllocatableCPUMilli(effectiveCPUs)
	maxMilli := calculateMaxAllocatableCPUMilli(effectiveCPUs)
	maxQty := resource.NewMilliQuantity(maxMilli, resource.DecimalSI)
	unit := "effective vCPUs"
	if effectiveCPUs == 1 {
		unit = "effective vCPU"
	}
	if qty.Cmp(*maxQty) > 0 {
		boundLabel := "estimated allocatable CPU"
		if maxMilli > legacyMilli {
			boundLabel = "estimated GKE 1.37.2+ upper-bound allocatable CPU"
		}
		return fmt.Errorf("requested --cpu %s exceeds %s (%s from %d %s) for machine type %s; lower --cpu to at most %s, choose a larger --compute-type, or omit --cpu/--memory for full-node sizing", cpuReq, boundLabel, maxQty.String(), effectiveCPUs, unit, machineType, maxQty.String())
	}
	legacyQty := resource.NewMilliQuantity(legacyMilli, resource.DecimalSI)
	if qty.Cmp(*legacyQty) > 0 {
		logging.Warn("Requested --cpu %s exceeds the pre-GKE 1.37.2 tiered allocatable estimate (%s from %d %s) for %s; pods may remain Pending on clusters older than GKE 1.37.2.", cpuReq, legacyQty.String(), effectiveCPUs, unit, machineType)
	}
	return nil
}

func validateMemoryRequestBound(machineType, memReq string, memoryMb int) error {
	if memReq == "" {
		return nil
	}
	qty, err := k8s.ParseMemoryRequest(memReq)
	if err != nil {
		return err
	}
	if memoryMb <= 0 {
		return nil
	}
	legacyBytes := calculateAllocatableMemoryBytes(memoryMb)
	maxBytes := calculateMaxAllocatableMemoryBytes(memoryMb)
	maxQty := resource.NewQuantity(maxBytes, resource.BinarySI)
	if qty.Cmp(*maxQty) > 0 {
		allocDesc := formatAllocatableMemory(maxBytes)
		boundLabel := "estimated allocatable memory"
		if maxBytes > legacyBytes {
			boundLabel = "estimated GKE 1.37.2+ upper-bound allocatable memory"
		}
		return fmt.Errorf("requested --memory %s exceeds %s (%s) for machine type %s; lower --memory to at most %s, choose a larger --compute-type, or omit --cpu/--memory for full-node sizing", memReq, boundLabel, allocDesc, machineType, allocDesc)
	}
	legacyQty := resource.NewQuantity(legacyBytes, resource.BinarySI)
	if qty.Cmp(*legacyQty) > 0 {
		const defaultMaxPodsReserveMiB = 15*110 + 500 + gkeEvictionMemoryMiB
		defaultPodsBytes := max(legacyBytes, int64(memoryMb)*mibBytes-defaultMaxPodsReserveMiB*mibBytes)
		podDensityHint := "higher max-pods-per-node"
		if qty.Cmp(*resource.NewQuantity(defaultPodsBytes, resource.BinarySI)) > 0 {
			podDensityHint = "default or higher max-pods-per-node"
		}
		logging.Warn("Requested --memory %s exceeds the pre-GKE 1.37.2 tiered allocatable estimate (%s) for %s; pods may remain Pending on clusters older than GKE 1.37.2 or with %s.", memReq, formatAllocatableMemory(legacyBytes), machineType, podDensityHint)
	}
	return nil
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
		request -= gcsFuseCPUMilli
	}
	return max(100, request)
}

// calculateFullNodeCPURequestMilli mirrors modules/compute/gke-job-template/main.tf
func calculateFullNodeCPURequestMilli(vCPUs int, hasGCSFuse bool) int64 {
	return calculateFullNodeCPURequestFromAllocatableMilli(calculateAllocatableCPUMilli(vCPUs), hasGCSFuse)
}
