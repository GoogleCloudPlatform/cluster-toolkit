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
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestGetNodeSelector(t *testing.T) {
	opts := SchedulingOptions{
		PlacementPolicy:    "compact-placement",
		NodeAffinityLabels: map[string]string{"key": "value"},
	}
	selector := GetNodeSelector(opts)
	if selector["cloud.google.com/gke-placement-group"] != "compact-placement" {
		t.Errorf("Expected placement policy label, got %v", selector["cloud.google.com/gke-placement-group"])
	}
	if selector["key"] != "value" {
		t.Errorf("Expected user label, got %v", selector["key"])
	}
}

func TestGetAffinity(t *testing.T) {
	opts := SchedulingOptions{}
	affinity := GetAffinity(opts)
	if affinity == nil {
		t.Fatal("Expected affinity, got nil")
	}
	if affinity.NodeAffinity == nil {
		t.Fatal("Expected NodeAffinity, got nil")
	}
	terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) == 0 {
		t.Fatal("Expected NodeSelectorTerms")
	}
	match := terms[0].MatchExpressions[0]
	if match.Key != "cloud.google.com/gke-nodepool" || match.Operator != corev1.NodeSelectorOpNotIn || match.Values[0] != "default-pool" {
		t.Errorf("Expected default-pool exclusion, got %v", match)
	}
}

func TestGetTopologyAnnotation(t *testing.T) {
	tests := []struct {
		topology string
		want     string
	}{
		{"2x2x1", "2x2x1"},
		{"", ""},
	}
	for _, tt := range tests {
		got := GetTopologyAnnotation(tt.topology)
		if tt.want == "" {
			if got != nil {
				t.Errorf("Expected nil for empty topology, got %v", got)
			}
		} else {
			if got["cloud.google.com/gke-tpu-slice-topology"] != tt.want {
				t.Errorf("Expected %s, got %v", tt.want, got)
			}
		}
	}
}

func TestGetTolerations(t *testing.T) {
	tests := []struct {
		acceleratorType string
		wantToleration  bool
	}{
		{"", false},
		{"nvidia-tesla-t4", false},
		{"tpu-v4-8", true},
		{"v4-8", true},
		{"v5p-8", true},
		{"a100-80gb", false},
	}

	for _, tt := range tests {
		got := GetTolerations(tt.acceleratorType)
		if tt.wantToleration {
			if got == nil {
				t.Errorf("expected toleration for %s, got nil", tt.acceleratorType)
			} else if got[0].Key != "google.com/tpu" {
				t.Errorf("expected toleration key to be google.com/tpu, got %s", got[0].Key)
			}
		} else {
			if got != nil {
				t.Errorf("expected nil toleration for %s, got %v", tt.acceleratorType, got)
			}
		}
	}
}

func TestGetNodeSelector_DynamicTopology(t *testing.T) {
	opts := SchedulingOptions{
		NodeAffinityLabels: map[string]string{
			"normal-key":                        "value",
			"pipe-key":                          "val1|val2",
			"cloud.google.com/gke-tpu-topology": "2x2",
		},
		Topology: "2x2",
	}
	selector := GetNodeSelector(opts)

	if selector["normal-key"] != "value" {
		t.Errorf("Expected normal-key to be 'value', got %v", selector["normal-key"])
	}
	if _, exists := selector["pipe-key"]; exists {
		t.Errorf("Expected pipe-key to be excluded from nodeSelector")
	}
	if _, exists := selector["cloud.google.com/gke-tpu-topology"]; exists {
		t.Errorf("Expected cloud.google.com/gke-tpu-topology to be excluded from nodeSelector due to smart merging")
	}
}

func TestGetAffinity_ConstraintsAndMerging(t *testing.T) {
	tests := []struct {
		name       string
		opts       SchedulingOptions
		wantKey    string
		wantValues []string
	}{
		{
			name: "pipe separated values (non-topology)",
			opts: SchedulingOptions{
				NodeAffinityLabels: map[string]string{
					"pipe-key": "val1|val2",
				},
			},
			wantKey:    "pipe-key",
			wantValues: []string{"val1", "val2"},
		},
		{
			name: "smart merging (legacy case)",
			opts: SchedulingOptions{
				Topology: "2x2",
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "4x4",
				},
			},
			wantKey:    "cloud.google.com/gke-tpu-topology",
			wantValues: []string{"2x2", "4x4"},
		},
		{
			name: "prioritize baseline and deduplicate",
			opts: SchedulingOptions{
				Topology: "2x2x1",
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "2x2x2|2x2x1",
				},
			},
			wantKey:    "cloud.google.com/gke-tpu-topology",
			wantValues: []string{"2x2x1", "2x2x2"},
		},
		{
			name: "null safety with whitespace",
			opts: SchedulingOptions{
				Topology: "2x2x1",
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": " | ",
				},
			},
			wantKey:    "cloud.google.com/gke-tpu-topology",
			wantValues: []string{"2x2x1"},
		},
		{
			name: "null safety with empty string",
			opts: SchedulingOptions{
				Topology: "2x2x1",
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "",
				},
			},
			wantKey:    "cloud.google.com/gke-tpu-topology",
			wantValues: []string{"2x2x1"},
		},
		{
			name: "skip baseline for dynamic slicing",
			opts: SchedulingOptions{
				Topology: "2x2x1",
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "2x2x2",
				},
				IsDynamicSlicing: true,
			},
			wantKey:    "cloud.google.com/gke-tpu-topology",
			wantValues: []string{"2x2x2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affinity := GetAffinity(tt.opts)
			if affinity == nil {
				t.Fatal("Expected affinity, got nil")
			}
			terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
			if len(terms) == 0 {
				t.Fatal("Expected NodeSelectorTerms")
			}
			var found bool
			for _, req := range terms[0].MatchExpressions {
				if req.Key == tt.wantKey {
					found = true
					if !slices.Equal(req.Values, tt.wantValues) {
						t.Errorf("Expected values %v, got %v", tt.wantValues, req.Values)
					}
					if req.Operator != corev1.NodeSelectorOpIn {
						t.Errorf("Expected operator In, got %v", req.Operator)
					}
				}
			}
			if !found {
				t.Errorf("Expected to find requirement for key %s", tt.wantKey)
			}
		})
	}
}
