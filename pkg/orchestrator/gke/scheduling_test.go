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
	tests := []struct {
		name      string
		opts      SchedulingOptions
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{
			name: "basic labels inclusion",
			opts: SchedulingOptions{
				PlacementPolicy: "compact-placement",
				NodeAffinityLabels: map[string]string{
					"key": "value",
				},
			},
			wantKey:   "key",
			wantValue: "value",
		},
		{
			name: "skip pipe separated values (goes to affinity)",
			opts: SchedulingOptions{
				NodeAffinityLabels: map[string]string{
					"pipe-key": "val1|val2",
				},
			},
			wantKey: "pipe-key",
		},
		{
			name: "single value topology inclusion",
			opts: SchedulingOptions{
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "4x4",
				},
			},
			wantKey:   "cloud.google.com/gke-tpu-topology",
			wantValue: "4x4",
		},
		{
			name: "case insensitivity canonicalization",
			opts: SchedulingOptions{
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "4X4",
				},
			},
			wantKey:   "cloud.google.com/gke-tpu-topology",
			wantValue: "4x4",
		},
		{
			name: "invalid topology format fails",
			opts: SchedulingOptions{
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, err := getNodeSelector(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantValue == "" {
				if selector != nil {
					if _, exists := selector[tt.wantKey]; exists {
						t.Errorf("Expected key %s to be excluded from nodeSelector", tt.wantKey)
					}
				}
			} else {
				if selector == nil {
					t.Fatal("Expected selector, got nil")
				}
				if val := selector[tt.wantKey]; val != tt.wantValue {
					t.Errorf("Expected key %s value to be %q, got %q", tt.wantKey, tt.wantValue, val)
				}
			}
		})
	}
}

func TestGetAffinity(t *testing.T) {
	opts := SchedulingOptions{}
	affinity, err := GetAffinity(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
		name          string
		topology      string
		machineType   string
		numSlices     int
		nodesPerSlice int
		wantKey       string
		wantVal       string
		wantSize      string
	}{
		{
			name:          "single slice - tpu7x",
			topology:      "2x2x1",
			machineType:   "tpu7x-standard-4t",
			numSlices:     1,
			nodesPerSlice: 1,
			wantKey:       "kueue.x-k8s.io/podset-required-topology",
			wantVal:       "cloud.google.com/gke-tpu-partition-2x2x1-id",
		},
		{
			name:          "multislice - tpu7x",
			topology:      "2x2x1",
			machineType:   "tpu7x-standard-4t",
			numSlices:     2,
			nodesPerSlice: 1,
			wantKey:       "kueue.x-k8s.io/podset-slice-required-topology",
			wantVal:       "cloud.google.com/gke-tpu-partition-2x2x1-id",
			wantSize:      "1",
		},
		{
			name:          "single slice - v6e",
			topology:      "2x2",
			machineType:   "v6e-standard-8t",
			numSlices:     1,
			nodesPerSlice: 1,
			wantKey:       "kueue.x-k8s.io/podset-required-topology",
			wantVal:       "cloud.google.com/gke-tpu-slice-2x2-id",
		},
		{
			name:          "multislice - v6e",
			topology:      "2x2",
			machineType:   "v6e-standard-8t",
			numSlices:     4,
			nodesPerSlice: 1,
			wantKey:       "kueue.x-k8s.io/podset-slice-required-topology",
			wantVal:       "cloud.google.com/gke-tpu-slice-2x2-id",
			wantSize:      "1",
		},
		{
			name:          "empty topology",
			topology:      "",
			machineType:   "v6e-standard-8t",
			numSlices:     1,
			nodesPerSlice: 1,
			wantKey:       "",
			wantVal:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTopologyAnnotation(tt.topology, tt.machineType, tt.numSlices, tt.nodesPerSlice)
			if tt.topology == "" {
				if got != nil {
					t.Errorf("Expected nil for empty topology, got %v", got)
				}
				return
			}
			if got["cloud.google.com/gke-tpu-slice-topology"] != tt.topology {
				t.Errorf("Expected topology %s, got %v", tt.topology, got["cloud.google.com/gke-tpu-slice-topology"])
			}
			if got[tt.wantKey] != tt.wantVal {
				t.Errorf("Expected %s = %s, got %v", tt.wantKey, tt.wantVal, got[tt.wantKey])
			}
			if tt.wantSize != "" && got["kueue.x-k8s.io/podset-slice-size"] != tt.wantSize {
				t.Errorf("Expected slice size %s, got %s", tt.wantSize, got["kueue.x-k8s.io/podset-slice-size"])
			}
		})
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
	selector, err := getNodeSelector(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if selector["normal-key"] != "value" {
		t.Errorf("Expected normal-key to be 'value', got %v", selector["normal-key"])
	}
	if _, exists := selector["pipe-key"]; exists {
		t.Errorf("Expected pipe-key to be excluded from nodeSelector")
	}
	if selector["cloud.google.com/gke-tpu-topology"] != "2x2" {
		t.Errorf("Expected cloud.google.com/gke-tpu-topology to be '2x2', got %v", selector["cloud.google.com/gke-tpu-topology"])
	}
}

func TestGetAffinity_ConstraintsAndMerging(t *testing.T) {
	tests := []struct {
		name       string
		opts       SchedulingOptions
		wantKey    string
		wantValues []string
		wantErr    bool
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
			name: "multiple topologies in constraint fails",
			opts: SchedulingOptions{
				Topology: "2x2x1",
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": "2x2x2|2x2x1",
				},
			},
			wantErr: true,
		},
		{
			name: "null safety with whitespace (now fails)",
			opts: SchedulingOptions{
				Topology: "2x2x1",
				NodeAffinityLabels: map[string]string{
					"cloud.google.com/gke-tpu-topology": " | ",
				},
			},
			wantErr: true,
		},
		{
			name: "empty element in list",
			opts: SchedulingOptions{
				NodeAffinityLabels: map[string]string{
					"pipe-key": "val1||val2",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affinity, err := GetAffinity(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
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
