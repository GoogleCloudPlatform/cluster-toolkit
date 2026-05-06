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
	"hpc-toolkit/pkg/config"

	corev1 "k8s.io/api/core/v1"
)

type SchedulingOptions struct {
	PlacementPolicy    string
	Topology           string
	Scheduler          string
	NodeAffinityLabels map[string]string
}

func GetNodeSelector(opts SchedulingOptions) map[string]string {
	nodeSelector := make(map[string]string)

	if opts.PlacementPolicy != "" {
		nodeSelector["cloud.google.com/gke-placement-group"] = opts.PlacementPolicy
	}

	for k, v := range opts.NodeAffinityLabels {
		nodeSelector[k] = v
	}

	if len(nodeSelector) == 0 {
		return nil
	}
	return nodeSelector
}

func GetAffinity(opts SchedulingOptions) *corev1.Affinity {
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "cloud.google.com/gke-nodepool",
								Operator: corev1.NodeSelectorOpNotIn,
								Values:   []string{"default-pool"},
							},
						},
					},
				},
			},
		},
	}
}

func GetTopologyAnnotation(topology string) map[string]string {
	if topology == "" {
		return nil
	}
	return map[string]string{
		"cloud.google.com/gke-tpu-slice-topology": topology,
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
