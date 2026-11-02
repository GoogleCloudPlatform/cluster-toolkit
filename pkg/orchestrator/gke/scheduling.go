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
	"fmt"
	"hpc-toolkit/pkg/config"
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
	PlacementPolicy    string
	Topology           string
	Scheduler          string
	MachineType        string
	IsTPU              bool
	IsNAP              bool
	NodeAffinityLabels map[string]string
	IsDynamicSlicing   bool
	IsStaticSlicing    bool
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

	if len(nodeSelector) == 0 {
		return nil, nil
	}
	return nodeSelector, nil
}

func GetAffinity(opts SchedulingOptions) (*corev1.Affinity, error) {
	// Build the inner term first to reduce nesting
	defaultPoolExclusion := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      nodePoolLabel,
				Operator: corev1.NodeSelectorOpNotIn,
				Values:   []string{"default-pool"},
			},
		},
	}

	affinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{defaultPoolExclusion},
			},
		},
	}

	term := &affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0]

	// Collect and sort keys of NodeAffinityLabels to ensure deterministic output
	keys := make([]string, 0, len(opts.NodeAffinityLabels))
	for k := range opts.NodeAffinityLabels {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		v := opts.NodeAffinityLabels[k]
		hasPipe := strings.Contains(v, "|")

		if !hasPipe {
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

	return affinity, nil
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
