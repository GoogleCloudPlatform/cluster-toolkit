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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type SchedulingOptions struct {
	PlacementPolicy    string
	Topology           string
	Scheduler          string
	NodeAffinityLabels map[string]string
	IsDynamicSlicing   bool
}

func GetNodeSelector(opts SchedulingOptions) map[string]string {
	nodeSelector := make(map[string]string)

	if opts.PlacementPolicy != "" {
		nodeSelector["cloud.google.com/gke-placement-group"] = opts.PlacementPolicy
	}

	for k, v := range opts.NodeAffinityLabels {
		// Skip if it has a pipe (will go to affinity)
		if strings.Contains(v, "|") {
			continue
		}
		// Skip if it's topology (will go to affinity)
		if k == tpuTopologyLabel {
			continue
		}
		nodeSelector[k] = v
	}

	if len(nodeSelector) == 0 {
		return nil
	}
	return nodeSelector
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

	// Handle pipe-separated constraints and smart merging for topology
	for k, v := range opts.NodeAffinityLabels {
		// True if this is a topology label that needs to be merged with a baseline topology.
		isTopologyMerge := (k == tpuTopologyLabel) && (opts.Topology != "") && (!opts.IsDynamicSlicing)
		hasPipe := strings.Contains(v, "|")

		if !hasPipe && k != tpuTopologyLabel {
			continue
		}

		var values []string
		if isTopologyMerge {
			values = append(values, opts.Topology)
		}

		if v != "" {
			for _, val := range strings.Split(v, "|") {
				trimmed := strings.TrimSpace(val)
				if trimmed == "" {
					return nil, fmt.Errorf("invalid node constraint for key %s: empty element in %q", k, v)
				}
				if k == tpuTopologyLabel && !config.TopologyRegex.MatchString(trimmed) {
					return nil, fmt.Errorf("invalid topology format %q for key %s", trimmed, k)
				}
				if !slices.Contains(values, trimmed) {
					values = append(values, trimmed)
				}
			}
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

func GetTopologyAnnotation(topology string, numSlices int) map[string]string {
	if topology == "" {
		return nil
	}

	annotationKey := "kueue.x-k8s.io/podset-required-topology"
	if numSlices > 1 {
		annotationKey = "kueue.x-k8s.io/podset-slice-required-topology"
	}

	return map[string]string{
		"cloud.google.com/gke-tpu-slice-topology": topology,
		annotationKey: fmt.Sprintf("cloud.google.com/gke-tpu-partition-%s-id", topology),
	}
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
