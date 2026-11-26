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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"path"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// gpuWorkloadPolicyTopologies maps GPU machine-type prefixes that require a GCE
// HIGH_THROUGHPUT workload policy to the accelerator topology that policy must carry.
// 1x72 (one full NVLink domain) is assumed for every a4x-* shape; revisit if GKE adds
// other A4X topologies or a --topology override for GPUs.
var gpuWorkloadPolicyTopologies = map[string]string{
	"a4x-": "1x72",
}

// gpuWorkloadPolicyTopology returns the required workload policy topology for GPU
// machine types that cannot be scheduled without one, or "" for all other machines.
func gpuWorkloadPolicyTopology(machineType string) string {
	lower := strings.ToLower(machineType)
	for prefix, topology := range gpuWorkloadPolicyTopologies {
		if strings.HasPrefix(lower, prefix) {
			return topology
		}
	}
	return ""
}

// requiresWorkloadPolicy reports whether the machine type is only schedulable through a
// GCE workload policy. GCE rejects dynamic compact placement groups for these machines.
func requiresWorkloadPolicy(machineType string) bool {
	return strings.Contains(strings.ToLower(machineType), "tpu7x") || gpuWorkloadPolicyTopology(machineType) != ""
}

type SchedulingOptions struct {
	PlacementPolicy     string
	Topology            string
	Scheduler           string
	MachineType         string
	IsTPU               bool
	IsNAP               bool
	ClusterNAPEnabled   bool
	PinCPUMachine       bool
	ExcludeAccelerators bool
	NodeAffinityLabels  map[string]string
	ExcludedNodePools   []string
	IsDynamicSlicing    bool
	IsStaticSlicing     bool
	OmitGKENodePool     bool
}

func getNodeSelector(opts SchedulingOptions) (map[string]string, error) {
	nodeSelector := make(map[string]string)

	if opts.PlacementPolicy != "" {
		// GKE labels nodes with "cloud.google.com/placement-policy-name=<policy>" for every node pool
		// created from a pre-existing GCE resource policy, whether it is a compact placement policy or
		// a workload policy, and NAP attaches an existing policy by that same selector. So the policy
		// name always goes there. "cloud.google.com/gke-placement-group" is a GKE-managed compact group
		// identifier: on existing nodes it holds the node pool name, and under NAP it asks GKE to create
		// a group named after it. It is only added for NAP on machines that accept group placement.
		// Slashes from full URIs/paths are stripped so the label value conforms to Kubernetes RFC 1123 format.
		policyName := path.Base(strings.TrimSuffix(strings.TrimSpace(opts.PlacementPolicy), "/"))
		nodeSelector[placementPolicyNameLabel] = policyName
		if opts.IsNAP && !opts.IsTPU && !requiresWorkloadPolicy(opts.MachineType) {
			nodeSelector[placementGroupLabel] = policyName
		}
	}

	for k, v := range opts.NodeAffinityLabels {
		// Skip if it has a pipe (will go to affinity)
		if strings.Contains(v, "|") {
			continue
		}
		// Validate topology format
		if k == tpuTopologyLabel {
			trimmed := strings.ToLower(strings.TrimSpace(v))
			if !config.TopologyRegex.MatchString(trimmed) {
				return nil, fmt.Errorf("invalid topology format %q for key %s", v, k)
			}
			nodeSelector[k] = trimmed
			continue
		}
		nodeSelector[k] = v
	}

	if opts.PinCPUMachine {
		addCPUMachineSelectors(nodeSelector, opts.NodeAffinityLabels, opts.IsNAP || opts.ClusterNAPEnabled, opts.MachineType)
	}

	if len(nodeSelector) == 0 {
		return nil, nil
	}
	return nodeSelector, nil
}

func excludedNodePoolValues(extra []string) []string {
	excluded := []string{"default-pool"}
	for _, pool := range extra {
		if pool != "" && !slices.Contains(excluded, pool) {
			excluded = append(excluded, pool)
		}
	}
	return excluded
}

func GetAffinity(opts SchedulingOptions) (*corev1.Affinity, error) {
	term := corev1.NodeSelectorTerm{}
	if !opts.OmitGKENodePool {
		term.MatchExpressions = append(term.MatchExpressions, corev1.NodeSelectorRequirement{
			Key:      nodePoolLabel,
			Operator: corev1.NodeSelectorOpNotIn,
			Values:   excludedNodePoolValues(opts.ExcludedNodePools),
		})
	}
	if opts.ExcludeAccelerators {
		appendAcceleratorExclusions(&term, opts.NodeAffinityLabels)
	}
	// Collect and sort keys of NodeAffinityLabels to ensure deterministic output
	keys := make([]string, 0, len(opts.NodeAffinityLabels))
	for k := range opts.NodeAffinityLabels {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		v := opts.NodeAffinityLabels[k]
		if !strings.Contains(v, "|") {
			continue
		}

		values, err := parseAffinityValues(k, v)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			continue
		}
		term.MatchExpressions = append(
			term.MatchExpressions,
			corev1.NodeSelectorRequirement{
				Key:      k,
				Operator: corev1.NodeSelectorOpIn,
				Values:   values,
			},
		)
	}

	if len(term.MatchExpressions) == 0 {
		return nil, nil
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{term},
			},
		},
	}, nil
}

func parseAffinityValues(k string, v string) ([]string, error) {
	if v == "" {
		return nil, nil
	}

	if k == tpuTopologyLabel && strings.Contains(v, "|") {
		return nil, fmt.Errorf("multiple topologies in node constraints are not supported for key %s: got %q. GKE Warden validation rejects templates targeting multiple topologies", k, v)
	}

	var values []string
	for _, val := range strings.Split(v, "|") {
		trimmed := strings.TrimSpace(val)
		if trimmed == "" {
			return nil, fmt.Errorf("invalid node constraint for key %s: empty element in %q", k, v)
		}
		if !slices.Contains(values, trimmed) {
			values = append(values, trimmed)
		}
	}

	return values, nil
}

// GetTopologyAnnotation returns the GKE and Kueue topology annotations for the given topology and slice count.
// It dynamically selects the appropriate Kueue annotation key based on whether it is a single-slice or multi-slice job.
func GetTopologyAnnotation(topology string, machineType string, numSlices int, nodesPerSlice int) map[string]string {
	if topology == "" {
		return nil
	}

	annotationKey := "kueue.x-k8s.io/podset-required-topology"
	annotations := map[string]string{
		"cloud.google.com/gke-tpu-slice-topology": topology,
	}
	partitionValue := fmt.Sprintf("cloud.google.com/gke-tpu-slice-%s-id", topology)
	if strings.Contains(machineType, "tpu7x") {
		partitionValue = fmt.Sprintf("cloud.google.com/gke-tpu-partition-%s-id", topology)
	}
	if numSlices > 1 {
		annotationKey = "kueue.x-k8s.io/podset-slice-required-topology"
		annotations["kueue.x-k8s.io/podset-slice-size"] = strconv.Itoa(nodesPerSlice)
	}
	annotations[annotationKey] = partitionValue
	return annotations
}

func GetTolerations(acceleratorType string) []corev1.Toleration {
	if acceleratorType == "" {
		return nil
	}
	if config.IsTPU(acceleratorType) {
		return []corev1.Toleration{
			{
				Key:      "google.com/tpu",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		}
	}
	return nil
}

func appendAcceleratorExclusions(term *corev1.NodeSelectorTerm, userConstraints map[string]string) {
	for _, key := range []string{k8s.GKEAcceleratorLabel, k8s.GKETPUAcceleratorLabel} {
		if strings.TrimSpace(userConstraints[key]) == "" {
			term.MatchExpressions = append(term.MatchExpressions, corev1.NodeSelectorRequirement{
				Key:      key,
				Operator: corev1.NodeSelectorOpDoesNotExist,
			})
		}
	}
}

func hasUserCPUPoolConstraint(constraints map[string]string) bool {
	for _, key := range []string{corev1.LabelInstanceTypeStable, nodePoolLabel, machineFamilyLabel, computeClassLabel} {
		if strings.TrimSpace(constraints[key]) != "" {
			return true
		}
	}
	return false
}

func isCustomMachineType(lower string) bool {
	return strings.HasPrefix(lower, "custom-") || strings.Contains(lower, "-custom-")
}

func predefinedMachineFamily(machineType string) string {
	lower := strings.ToLower(strings.TrimSpace(machineType))
	switch lower {
	case "", "custom":
		return ""
	case "f1-micro", "g1-small":
		return "n1"
	}
	if isCustomMachineType(lower) {
		return ""
	}
	family, _, ok := strings.Cut(lower, "-")
	if !ok || family == "" {
		return ""
	}
	return family
}

func addCPUMachineSelectors(nodeSelector, userConstraints map[string]string, napEnabled bool, machineType string) {
	normalized := strings.ToLower(strings.TrimSpace(machineType))
	if normalized == "" || hasUserCPUPoolConstraint(userConstraints) {
		return
	}
	nodeSelector[corev1.LabelInstanceTypeStable] = normalized
	if napEnabled {
		if family := predefinedMachineFamily(normalized); family != "" {
			nodeSelector[machineFamilyLabel] = family
		}
	}
}

func (g *GKEOrchestrator) hasStaticNodePoolForMachineType(machineType, provisioning, reservationName string) bool {
	if strings.TrimSpace(machineType) == "" {
		return false
	}
	prov := strings.ToLower(strings.TrimSpace(provisioning))
	return slices.ContainsFunc(g.clusterDesc.NodePools, func(np gkeJobNodePool) bool {
		return g.isEligibleStaticCPUPool(np, machineType, prov, reservationName)
	})
}

func (g *GKEOrchestrator) isEligibleStaticCPUPool(np gkeJobNodePool, machineType, prov, reservationName string) bool {
	if np.Name == "default-pool" || g.isSystemPool(np) || len(np.Config.Accelerators) > 0 ||
		g.getNodeCount(np) == 0 || !strings.EqualFold(np.Config.MachineType, machineType) {
		return false
	}
	if provisioningMismatchesPool(prov, reservationName, np) {
		return false
	}
	copiesAllTaints := prov == "reservation" && poolMatchesReservation(np.Config, reservationName)
	return !hasBlockingTaint(np.Config.Taints, prov, reservationName, copiesAllTaints)
}

func (g *GKEOrchestrator) shouldPinCPUMachine(isCPUMachine bool, cpuReq, memReq, napProvisioning, napReservation, machineType string) bool {
	if !isCPUMachine {
		return false
	}
	if cpuReq != "" || memReq != "" || strings.TrimSpace(napProvisioning) != "" {
		return true
	}
	return g.napEnabled && !g.hasStaticNodePoolForMachineType(machineType, napProvisioning, napReservation)
}

func pipeConstraintContains(constraintVal, target string) bool {
	for part := range strings.SplitSeq(constraintVal, "|") {
		if strings.EqualFold(strings.TrimSpace(part), target) {
			return true
		}
	}
	return false
}

func headPoolMatchesUserMachine(headMachine, targetMachine string, constraints map[string]string) bool {
	if strings.TrimSpace(headMachine) == "" {
		return false
	}
	if inst := strings.TrimSpace(constraints[corev1.LabelInstanceTypeStable]); inst != "" {
		return pipeConstraintContains(inst, headMachine)
	}
	if fam := strings.TrimSpace(constraints[machineFamilyLabel]); fam != "" {
		headFamily := predefinedMachineFamily(headMachine)
		return headFamily != "" && pipeConstraintContains(fam, headFamily)
	}
	return strings.EqualFold(headMachine, targetMachine)
}

func isToleratedProvisioningTaint(t gkeTaint, effect, prov, reservationName string) bool {
	if effect != "NO_SCHEDULE" && effect != "NOSCHEDULE" {
		return false
	}
	if prov == "spot" && t.Key == "cloud.google.com/gke-provisioning" && t.Value == "spot" {
		return true
	}
	if prov == "reservation" && t.Key == "cloud.google.com/reservation-name" {
		reqRes := parseReservationURI(reservationName)
		taintRes := parseReservationURI(t.Value)
		return reqRes.Name != "" && taintRes.Name == reqRes.Name
	}
	return false
}

func hasBlockingTaint(taints []gkeTaint, prov, reservationName string, copiesAllPoolTaints bool) bool {
	if copiesAllPoolTaints {
		return false
	}
	return slices.ContainsFunc(taints, func(t gkeTaint) bool {
		effect := strings.ToUpper(strings.TrimSpace(t.Effect))
		if isToleratedProvisioningTaint(t, effect, prov, reservationName) {
			return false
		}
		switch effect {
		case "NO_SCHEDULE", "NOSCHEDULE", "NO_EXECUTE", "NOEXECUTE":
			return true
		}
		return false
	})
}

func matchParsedReservation(poolRes parsedReservation, poolLabels map[string]string, reqRes parsedReservation) bool {
	if poolRes.Name == "" || !strings.EqualFold(poolRes.Name, reqRes.Name) {
		return false
	}
	if reqRes.Project != "" {
		poolProject := cmp.Or(poolRes.Project, strings.TrimSpace(poolLabels["cloud.google.com/reservation-project"]))
		if !strings.EqualFold(poolProject, reqRes.Project) {
			return false
		}
	}
	if reqRes.Block != "" {
		poolBlock := cmp.Or(poolRes.Block, strings.TrimSpace(poolLabels["cloud.google.com/reservation-blocks"]))
		if !strings.EqualFold(poolBlock, reqRes.Block) {
			return false
		}
	}
	if reqRes.Subblock != "" {
		poolSubblock := cmp.Or(poolRes.Subblock, strings.TrimSpace(poolLabels["cloud.google.com/reservation-subblocks"]))
		if !strings.EqualFold(poolSubblock, reqRes.Subblock) {
			return false
		}
	}
	return true
}

func poolMatchesReservation(cfg gkeNodePoolConfig, reservationName string) bool {
	reqRes := parseReservationURI(reservationName)
	if reqRes.Name == "" {
		return false
	}
	if lblVal := strings.TrimSpace(cfg.Labels["cloud.google.com/reservation-name"]); lblVal != "" {
		if matchParsedReservation(parseReservationURI(lblVal), cfg.Labels, reqRes) {
			return true
		}
	}
	if ra := cfg.ReservationAffinity; ra != nil &&
		strings.EqualFold(ra.ConsumeReservationType, "SPECIFIC_RESERVATION") &&
		ra.Key == "compute.googleapis.com/reservation-name" {
		for _, v := range ra.Values {
			if matchParsedReservation(parseReservationURI(v), cfg.Labels, reqRes) {
				return true
			}
		}
	}
	return false
}

func provisioningMismatchesPool(prov, reservationName string, np gkeJobNodePool) bool {
	switch prov {
	case "reservation":
		return !poolMatchesReservation(np.Config, reservationName)
	case "spot":
		return !np.Config.Spot
	case "on-demand":
		return np.Config.Spot
	default:
		return false
	}
}

func (g *GKEOrchestrator) isCandidatePathwaysHeadPool(np gkeJobNodePool) bool {
	return (np.Name == "cpu-np" || np.Name == "pathways-np") &&
		len(np.Config.Accelerators) == 0 &&
		!config.IsTPU(np.Config.MachineType) &&
		!g.isSystemPool(np)
}

func (g *GKEOrchestrator) shouldExcludeHeadPool(headNP gkeJobNodePool, schedOpts *SchedulingOptions, prov, reservationName string) bool {
	if g.getNodeCount(headNP) == 0 ||
		!headPoolMatchesUserMachine(headNP.Config.MachineType, schedOpts.MachineType, schedOpts.NodeAffinityLabels) ||
		provisioningMismatchesPool(prov, reservationName, headNP) {
		return true
	}
	// resolveReservationTolerations only copies extra pool taints when the pool's machine type matches schedOpts.MachineType.
	copiesAllPoolTaints := prov == "reservation" && strings.EqualFold(headNP.Config.MachineType, schedOpts.MachineType)
	return hasBlockingTaint(headNP.Config.Taints, prov, reservationName, copiesAllPoolTaints)
}

// applyCPUHeadPoolAvoidance excludes incompatible Pathways head pools via NotIn so
// Kueue skips pathways-flavor and falls through to flavor-default on static and NAP clusters.
func (g *GKEOrchestrator) applyCPUHeadPoolAvoidance(schedOpts *SchedulingOptions, provisioning, reservationName string) {
	if schedOpts == nil ||
		!g.kueueEnabled() ||
		!schedOpts.PinCPUMachine ||
		strings.TrimSpace(schedOpts.NodeAffinityLabels[nodePoolLabel]) != "" ||
		strings.TrimSpace(schedOpts.NodeAffinityLabels[computeClassLabel]) != "" {
		return
	}
	prov := strings.ToLower(strings.TrimSpace(provisioning))
	for _, headNP := range g.clusterDesc.NodePools {
		if g.isCandidatePathwaysHeadPool(headNP) && g.shouldExcludeHeadPool(headNP, schedOpts, prov, reservationName) {
			schedOpts.ExcludedNodePools = append(schedOpts.ExcludedNodePools, headNP.Name)
		}
	}
}
