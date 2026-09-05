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
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
)

func (g *GKEOrchestrator) isNAPEnabledForMachineType(machineType, zone string) (bool, error) {
	if !g.napEnabled {
		return false, nil
	}

	resolvedType := config.ResolveMachineType(machineType)

	if config.IsTPU(resolvedType) {
		return g.validateTPUNAPLimit(resolvedType)
	}

	cap, err := g.FetchMachineCapabilities(resolvedType, zone)
	if err != nil {
		return false, err
	}
	if len(cap.Accelerators) > 0 {
		return g.validateGPUNAPLimit(resolvedType, cap)
	}

	return g.napLimits["cpu"] > 0, nil
}

func (g *GKEOrchestrator) validateTPUNAPLimit(resolvedType string) (bool, error) {
	key := strings.ToLower(g.GenerateGKENodeSelectorLabel(resolvedType))
	if limit, exists := g.napLimits[key]; exists {
		return limit > 0, nil
	}
	// Fallback to generic TPU limit ONLY if no specific TPU limits are configured.
	for k := range g.napLimits {
		if isSpecificTPUKey(k) {
			return false, nil
		}
	}
	return g.napLimits["google.com/tpu"] > 0, nil
}

func (g *GKEOrchestrator) validateGPUNAPLimit(resolvedType string, cap MachineTypeCap) (bool, error) {
	key := strings.ToLower(g.GenerateGKENodeSelectorLabel(resolvedType))
	if strings.EqualFold(key, resolvedType) {
		key = strings.ToLower(g.GenerateGKENodeSelectorLabel(cap.Accelerators[0].Type))
	}
	if !isKnownGKEAccelerator(key) {
		return false, fmt.Errorf("unknown accelerator label: %q", cap.Accelerators[0].Type)
	}
	if limit, exists := g.napLimits[key]; exists {
		return limit > 0, nil
	}
	// Fallback to generic GPU limit ONLY if no specific GPU limits are configured.
	for k := range g.napLimits {
		if isSpecificGPUKey(k) {
			return false, nil
		}
	}
	return g.napLimits["nvidia.com/gpu"] > 0, nil
}

func (g *GKEOrchestrator) checkNAPFlagsSupported(hasNAPFlags bool, job *orchestrator.JobDefinition) error {
	if !g.napEnabled && hasNAPFlags {
		return fmt.Errorf("GKE NAP provisioning options (--gke-nap-provisioning %q, --gke-nap-reservation %q) are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled. The current cluster does not have NAP enabled.\nRemediation: Enable Node Auto-Provisioning on your cluster to use these options, or submit your job without them", job.GKENAPProvisioning, job.GKENAPReservation)
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
	hasNAPFlags := job.GKENAPProvisioning != ""

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

// resolveFallbackName returns a fallback reservation name from the URI parts.
func resolveFallbackName(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	// If it has reservationBlocks, the name is the one before it
	for i, part := range parts {
		if part == "reservationBlocks" && i > 0 {
			return parts[i-1]
		}
	}
	// Fallback to last element
	return parts[len(parts)-1]
}

// parseReservationURI parses a GCE reservation resource URI or reservation path into its components.
// E.g.,
// - "my-res" -> Name: "my-res"
// - "projects/my-project/reservations/my-res" -> Project: "my-project", Name: "my-res"
// - "projects/my-project/reservations/my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2" -> Project: "my-project", Name: "my-res", Block: "block-1", Subblock: "subblock-2"
func parseReservationURI(resName string) parsedReservation {
	resName = strings.TrimSuffix(resName, "/")
	var parsed parsedReservation
	if !strings.Contains(resName, "/") {
		parsed.Name = strings.ToLower(resName)
		return parsed
	}

	parts := strings.Split(resName, "/")
	for i := 0; i < len(parts)-1; i++ {
		switch parts[i] {
		case "projects":
			parsed.Project = strings.ToLower(parts[i+1])
		case "reservations":
			parsed.Name = strings.ToLower(parts[i+1])
		case "reservationBlocks":
			parsed.Block = strings.ToLower(parts[i+1])
		case "reservationSubBlocks":
			parsed.Subblock = strings.ToLower(parts[i+1])
		}
	}

	if parsed.Name == "" {
		parsed.Name = strings.ToLower(resolveFallbackName(parts))
	}

	return parsed
}

func isSpecificGPUKey(key string) bool {
	return strings.HasPrefix(key, "nvidia-")
}

func isSpecificTPUKey(key string) bool {
	return strings.HasPrefix(key, "tpu-") || strings.HasPrefix(key, "tpu7")
}

func isKnownGKEAccelerator(key string) bool {
	switch key {
	case "nvidia-tesla-t4", "nvidia-tesla-v100":
		return true
	}
	for _, val := range config.GetMachineMappings().MachineFamilyToLabelMap {
		if val == key {
			return true
		}
	}
	return false
}

func parseNAPLimits(autoscaling gkeClusterAutoscaling) map[string]int64 {
	limits := make(map[string]int64)
	for _, rl := range autoscaling.ResourceLimits {
		limits[rl.ResourceType] = rl.Maximum
	}

	for _, rl := range autoscaling.ResourceLimits {
		resName := rl.ResourceType
		maxVal := rl.Maximum
		if resName == "gpu" || strings.Contains(resName, "nvidia") {
			if maxVal > limits["nvidia.com/gpu"] {
				limits["nvidia.com/gpu"] = maxVal
			}
		} else if strings.Contains(resName, "tpu") {
			if maxVal > limits["google.com/tpu"] {
				limits["google.com/tpu"] = maxVal
			}
		}
	}
	return limits
}

func isNonAcceleratorResource(resName string) bool {
	switch resName {
	case "cpu", "memory", "gpu", "tpu", "pods", "storage", "ephemeral-storage":
		return true
	default:
		return false
	}
}

func resolveFlavorFromResource(resName string) (string, map[string]string, error) {
	var flavorName string
	var nodeLabels map[string]string

	switch {
	case resName == "google.com/tpu":
		flavorName = "flavor-tpu-generic"
	case resName == "nvidia.com/gpu":
		flavorName = "flavor-nvidia-generic"
	case strings.HasPrefix(resName, "nvidia-"):
		flavorName = "flavor-" + resName
		nodeLabels = map[string]string{
			"cloud.google.com/gke-accelerator": resName,
		}
	case strings.HasPrefix(resName, "tpu-") || strings.HasPrefix(resName, "tpu7"):
		flavorName = "flavor-" + resName
		nodeLabels = map[string]string{
			"cloud.google.com/gke-tpu-accelerator": resName,
		}
	default:
		return "", nil, fmt.Errorf("unknown accelerator label %q", resName)
	}

	return flavorName, nodeLabels, nil
}

func (g *GKEOrchestrator) populateNAPFlavors(flavors map[string]FlavorCapacity) error {
	if !g.napEnabled {
		return nil
	}

	for resName, maxLimit := range g.napLimits {
		if maxLimit <= 0 {
			continue
		}
		if isNonAcceleratorResource(resName) {
			continue
		}

		flavorName, nodeLabels, err := resolveFlavorFromResource(resName)
		if err != nil {
			return err
		}

		if _, ok := flavors[flavorName]; !ok {
			flavors[flavorName] = FlavorCapacity{
				NodeLabels: nodeLabels,
			}
		}
	}
	return nil
}

func (g *GKEOrchestrator) resolveTPUWorkloadPolicy(machineType, topology, clusterLocation, projectID string, isDryRun bool) (string, error) {
	if topology == "" {
		return "", fmt.Errorf("cannot resolve TPU workload policy: topology is empty")
	}

	if policy := g.findPolicyInClusterNodePools(machineType, topology); policy != "" {
		return policy, nil
	}

	canonicalPolicyName := getCanonicalTPUWorkloadPolicyName(topology)
	if g.resourcePolicyCache != nil && g.resourcePolicyCache[canonicalPolicyName] != nil {
		return canonicalPolicyName, nil
	}

	region, resolvedProjID := g.resolvePolicyRegionAndProject(clusterLocation, projectID)
	if region == "" || resolvedProjID == "" {
		if isDryRun {
			logging.Info("Dry-run: Could not determine cluster region or project. Assuming workload policy %q for topology %s. Ensure it exists in your target cluster's region.", canonicalPolicyName, topology)
		}
		return canonicalPolicyName, nil
	}

	policy, err := g.discoverRegionalWorkloadPolicy(canonicalPolicyName, region, resolvedProjID, topology)
	if err != nil {
		if errors.Is(err, ErrResourcePolicyPermissionDenied) {
			logging.Warn("Permission denied reading workload policy %q in region %s. Assuming canonical policy name %q. If job fails to schedule, verify with an administrator that the policy exists in region %s.", canonicalPolicyName, region, canonicalPolicyName, region)
			return canonicalPolicyName, nil
		}
		return "", err
	}
	if policy != "" {
		return policy, nil
	}

	if !isDryRun {
		if err := g.createTPUWorkloadPolicy(canonicalPolicyName, region, resolvedProjID, topology); err != nil {
			return "", err
		}
	} else {
		logging.Info("Dry-run: Workload policy %q for topology %s was not found. Please ensure it exists before applying the manifest, or create it using:\n  gcloud compute resource-policies create workload-policy %s --region=%s --project=%s --type=HIGH_THROUGHPUT --accelerator-topology=%s", canonicalPolicyName, topology, canonicalPolicyName, region, resolvedProjID, topology)
	}

	return canonicalPolicyName, nil
}

func (g *GKEOrchestrator) findPolicyInClusterNodePools(machineType, topology string) string {
	for _, np := range g.clusterDesc.NodePools {
		if strings.EqualFold(np.Config.MachineType, machineType) && np.PlacementPolicy != nil && np.PlacementPolicy.PolicyName != "" {
			mode := np.PlacementPolicy.AcceleratorTopologyMode
			if mode == "" && g.resourcePolicyCache != nil {
				if cached := g.resourcePolicyCache[path.Base(np.PlacementPolicy.PolicyName)]; cached != nil {
					mode = cached.AcceleratorTopologyMode
				}
			}
			if strings.EqualFold(mode, "PROVISION_ONLY") {
				continue // Skip dynamic slicing policies for static workloads
			}
			topo := np.PlacementPolicy.TpuTopology
			if topo == "" && np.Config.Labels != nil {
				topo = np.Config.Labels[tpuTopologyLabel]
			}
			if topo != "" && topo == topology {
				policy := path.Base(np.PlacementPolicy.PolicyName)
				logging.Info("Discovered matching placement policy %q from existing node pool %q", policy, np.Name)
				return policy
			}
		}
	}
	return ""
}

func getCanonicalTPUWorkloadPolicyName(topology string) string {
	chips := calculateChipsFromTopology(topology)
	tensorcores := chips * 2
	return fmt.Sprintf("tpu7x-%d-%s-placement-policy", tensorcores, topology)
}

func (g *GKEOrchestrator) resolvePolicyRegionAndProject(clusterLocation, projectID string) (string, string) {
	region := ""
	if clusterLocation != "" {
		region = shell.ExtractRegion(clusterLocation)
	} else if len(g.clusterDesc.Locations) > 0 {
		region = shell.ExtractRegion(g.clusterDesc.Locations[0])
	} else if len(g.clusterZones) > 0 {
		region = shell.ExtractRegion(g.clusterZones[0])
	}
	if projectID == "" {
		projectID = g.projectID
	}
	return region, projectID
}

func (g *GKEOrchestrator) discoverRegionalWorkloadPolicy(canonicalPolicyName, region, projectID, topology string) (string, error) {
	policy, err := g.describeResourcePolicyCached(canonicalPolicyName, region, projectID)
	if err != nil {
		return "", err
	}
	if policy != nil {
		logging.Info("Discovered existing workload policy %q in region %s", canonicalPolicyName, region)
		return canonicalPolicyName, nil
	}

	filter := fmt.Sprintf("region:( %s ) AND workloadPolicy.acceleratorTopology=%s AND workloadPolicy.type=HIGH_THROUGHPUT", region, topology)
	listRes := g.executor.ExecuteCommand("gcloud", "compute", "resource-policies", "list", "--project="+projectID, "--filter="+filter, "--format=value(name)")
	if listRes.ExitCode == 0 && strings.TrimSpace(listRes.Stdout) != "" {
		names := strings.Fields(listRes.Stdout)
		if len(names) > 0 {
			if g.resourcePolicyCache == nil {
				g.resourcePolicyCache = make(map[string]*GCEWorkloadPolicy)
			}
			g.resourcePolicyCache[names[0]] = &GCEWorkloadPolicy{
				Name:                names[0],
				Region:              region,
				AcceleratorTopology: topology,
				Type:                "HIGH_THROUGHPUT",
			}
			logging.Info("Discovered matching workload policy %q for topology %s in region %s", names[0], topology, region)
			return names[0], nil
		}
	}
	return "", nil
}

func (g *GKEOrchestrator) createTPUWorkloadPolicy(canonicalPolicyName, region, projectID, topology string) error {
	logging.Info("Workload policy for topology %s not found. Creating workload policy %q...", topology, canonicalPolicyName)
	createRes := g.executor.ExecuteCommand("gcloud", "compute", "resource-policies", "create", "workload-policy", canonicalPolicyName, "--region="+region, "--project="+projectID, "--type=HIGH_THROUGHPUT", "--accelerator-topology="+topology)
	if createRes.ExitCode != 0 {
		stderrLower := strings.ToLower(createRes.Stderr)
		if strings.Contains(stderrLower, "already exists") || strings.Contains(stderrLower, "409") {
			logging.Info("Workload policy %q was concurrently created.", canonicalPolicyName)
			if g.resourcePolicyCache == nil {
				g.resourcePolicyCache = make(map[string]*GCEWorkloadPolicy)
			}
			g.resourcePolicyCache[canonicalPolicyName] = &GCEWorkloadPolicy{
				Name:                canonicalPolicyName,
				Region:              region,
				AcceleratorTopology: topology,
				Type:                "HIGH_THROUGHPUT",
			}
			return nil
		}
		return fmt.Errorf("failed to create required workload policy %q in region %s: %s\nRemediation: Ensure your GCP credentials have 'compute.resourcePolicies.create' permission (e.g. 'roles/compute.admin') or create the policy manually:\n  gcloud compute resource-policies create workload-policy %s --region=%s --project=%s --type=HIGH_THROUGHPUT --accelerator-topology=%s",
			canonicalPolicyName, region, createRes.Stderr, canonicalPolicyName, region, projectID, topology)
	}
	if g.resourcePolicyCache == nil {
		g.resourcePolicyCache = make(map[string]*GCEWorkloadPolicy)
	}
	g.resourcePolicyCache[canonicalPolicyName] = &GCEWorkloadPolicy{
		Name:                canonicalPolicyName,
		Region:              region,
		AcceleratorTopology: topology,
		Type:                "HIGH_THROUGHPUT",
	}
	logging.Info("Successfully created workload policy %q in region %s", canonicalPolicyName, region)
	return nil
}
