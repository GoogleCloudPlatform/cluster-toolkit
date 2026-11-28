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
	"reflect"
	"slices"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"

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
			name: "tpu placement policy label",
			opts: SchedulingOptions{
				PlacementPolicy: "tpu7x-16-2x2x2-placement-policy",
				IsTPU:           true,
			},
			wantKey:   "cloud.google.com/placement-policy-name",
			wantValue: "tpu7x-16-2x2x2-placement-policy",
		},
		{
			name: "non-tpu placement policy label",
			opts: SchedulingOptions{
				PlacementPolicy: "compact-placement",
				MachineType:     "a3-highgpu-8g",
				IsTPU:           false,
			},
			wantKey:   "cloud.google.com/placement-policy-name",
			wantValue: "compact-placement",
		},
		{
			name: "a4x workload policy label",
			opts: SchedulingOptions{
				PlacementPolicy: "gb200-4-1x72-placement-policy",
				MachineType:     "a4x-highgpu-4g",
			},
			wantKey:   "cloud.google.com/placement-policy-name",
			wantValue: "gb200-4-1x72-placement-policy",
		},
		{
			name: "tpu placement policy with relative URI is sanitized to bare name",
			opts: SchedulingOptions{
				PlacementPolicy: "projects/my-proj/regions/us-central1/resourcePolicies/tpu-policy",
				IsTPU:           true,
			},
			wantKey:   "cloud.google.com/placement-policy-name",
			wantValue: "tpu-policy",
		},
		{
			name: "non-tpu placement policy with HTTPS URL is sanitized to bare name",
			opts: SchedulingOptions{
				PlacementPolicy: "https://www.googleapis.com/compute/v1/projects/my-proj/regions/us-central1/resourcePolicies/gpu-policy",
				MachineType:     "a3-ultragpu-8g",
				IsTPU:           false,
			},
			wantKey:   "cloud.google.com/placement-policy-name",
			wantValue: "gpu-policy",
		},
		{
			name: "placement policy with trailing slash is sanitized to bare name",
			opts: SchedulingOptions{
				PlacementPolicy: "projects/my-proj/regions/us-central1/resourcePolicies/trailing-policy/",
				IsTPU:           true,
			},
			wantKey:   "cloud.google.com/placement-policy-name",
			wantValue: "trailing-policy",
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

func TestGetNodeSelector_PlacementLabelsByProvisioning(t *testing.T) {
	tests := []struct {
		name        string
		opts        SchedulingOptions
		wantLabels  map[string]string
		absentLabel string
	}{
		{
			name: "static GPU: policy name only",
			opts: SchedulingOptions{PlacementPolicy: "h100-compact", MachineType: "a3-highgpu-8g"},
			wantLabels: map[string]string{
				placementPolicyNameLabel: "h100-compact",
			},
			absentLabel: placementGroupLabel,
		},
		{
			name: "NAP GPU: policy name plus placement group",
			opts: SchedulingOptions{PlacementPolicy: "h100-compact", MachineType: "a3-highgpu-8g", IsNAP: true},
			wantLabels: map[string]string{
				placementPolicyNameLabel: "h100-compact",
				placementGroupLabel:      "h100-compact",
			},
		},
		{
			name: "NAP CPU: policy name plus placement group",
			opts: SchedulingOptions{PlacementPolicy: "c2-compact", MachineType: "c2-standard-60", IsNAP: true},
			wantLabels: map[string]string{
				placementPolicyNameLabel: "c2-compact",
				placementGroupLabel:      "c2-compact",
			},
		},
		{
			name: "NAP A4X: workload policy only, no placement group",
			opts: SchedulingOptions{PlacementPolicy: "gb200-4-1x72-placement-policy", MachineType: "a4x-highgpu-4g", IsNAP: true},
			wantLabels: map[string]string{
				placementPolicyNameLabel: "gb200-4-1x72-placement-policy",
			},
			absentLabel: placementGroupLabel,
		},
		{
			name: "NAP TPU 7x: workload policy only, no placement group",
			opts: SchedulingOptions{PlacementPolicy: "tpu7x-16-2x2x2-placement-policy", MachineType: "tpu7x-standard-4t", IsTPU: true, IsNAP: true},
			wantLabels: map[string]string{
				placementPolicyNameLabel: "tpu7x-16-2x2x2-placement-policy",
			},
			absentLabel: placementGroupLabel,
		},
		{
			name: "NAP TPU v5e: policy name only",
			opts: SchedulingOptions{PlacementPolicy: "v5e-compact", MachineType: "ct5lp-hightpu-4t", IsTPU: true, IsNAP: true},
			wantLabels: map[string]string{
				placementPolicyNameLabel: "v5e-compact",
			},
			absentLabel: placementGroupLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, err := getNodeSelector(tt.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.wantLabels {
				if got := selector[k]; got != v {
					t.Errorf("selector[%s] = %q, want %q", k, got, v)
				}
			}
			if tt.absentLabel != "" {
				if _, ok := selector[tt.absentLabel]; ok {
					t.Errorf("selector must not contain %s: %v", tt.absentLabel, selector)
				}
			}
		})
	}
}

func TestRequiresWorkloadPolicy(t *testing.T) {
	tests := map[string]bool{
		"a4x-highgpu-4g":        true,
		"a4x-highgpu-4g-nolssd": true,
		"a4x-maxgpu-4g-metal":   true,
		"tpu7x-standard-4t":     true,
		"a4-highgpu-8g":         false,
		"a3-ultragpu-8g":        false,
		"a3-highgpu-8g":         false,
		"ct6e-standard-4t":      false,
		"ct5lp-hightpu-4t":      false,
		"n2-standard-8":         false,
		"":                      false,
	}
	for machineType, want := range tests {
		if got := requiresWorkloadPolicy(machineType); got != want {
			t.Errorf("requiresWorkloadPolicy(%q) = %v, want %v", machineType, got, want)
		}
	}
	if got := gpuWorkloadPolicyTopology("a4x-highgpu-4g"); got != "1x72" {
		t.Errorf("gpuWorkloadPolicyTopology(a4x-highgpu-4g) = %q, want 1x72", got)
	}
	if got := gpuWorkloadPolicyTopology("tpu7x-standard-4t"); got != "" {
		t.Errorf("gpuWorkloadPolicyTopology(tpu7x) = %q, want empty", got)
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

func TestBuildNodeSelector_PlacementPolicy(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.machineCapCache["tpu7x-standard-4t:"] = MachineTypeCap{}
	g.machineCapCache["a3-highgpu-8g:"] = MachineTypeCap{}

	// 1. TPU workload uses cloud.google.com/placement-policy-name
	tpuJob := orchestrator.JobDefinition{
		MachineType: "tpu7x-standard-4t",
	}
	tpuOpts := SchedulingOptions{
		PlacementPolicy: "tpu7x-16-2x2x2-placement-policy",
		IsTPU:           true,
	}
	tpuSelectorStr, err := g.buildNodeSelectorWithProfile(tpuOpts, tpuJob, JobProfile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(tpuSelectorStr, "cloud.google.com/placement-policy-name: tpu7x-16-2x2x2-placement-policy") {
		t.Errorf("Expected cloud.google.com/placement-policy-name in %s", tpuSelectorStr)
	}
	if strings.Contains(tpuSelectorStr, "cloud.google.com/gke-placement-group") {
		t.Errorf("Did not expect cloud.google.com/gke-placement-group in %s", tpuSelectorStr)
	}

	// 2. Non-TPU workload on a static cluster also uses cloud.google.com/placement-policy-name
	nonTpuJob := orchestrator.JobDefinition{
		MachineType: "a3-highgpu-8g",
	}
	nonTpuOpts := SchedulingOptions{
		PlacementPolicy: "compact-placement-group",
		MachineType:     "a3-highgpu-8g",
	}
	nonTpuSelectorStr, err := g.buildNodeSelectorWithProfile(nonTpuOpts, nonTpuJob, JobProfile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(nonTpuSelectorStr, "cloud.google.com/placement-policy-name: compact-placement-group") {
		t.Errorf("Expected cloud.google.com/placement-policy-name in %s", nonTpuSelectorStr)
	}
	if strings.Contains(nonTpuSelectorStr, "cloud.google.com/gke-placement-group") {
		t.Errorf("Did not expect cloud.google.com/gke-placement-group in %s", nonTpuSelectorStr)
	}

	// 2b. Non-TPU workload under NAP additionally gets cloud.google.com/gke-placement-group
	napOpts := nonTpuOpts
	napOpts.IsNAP = true
	napSelectorStr, err := g.buildNodeSelectorWithProfile(napOpts, nonTpuJob, JobProfile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(napSelectorStr, "cloud.google.com/gke-placement-group: compact-placement-group") {
		t.Errorf("Expected cloud.google.com/gke-placement-group in %s", napSelectorStr)
	}
	if !strings.Contains(napSelectorStr, "cloud.google.com/placement-policy-name: compact-placement-group") {
		t.Errorf("Expected cloud.google.com/placement-policy-name in %s", napSelectorStr)
	}

	// 3. Placement policy passed as URI with trailing slash is sanitized into bare name
	uriJob := orchestrator.JobDefinition{
		MachineType: "tpu7x-standard-4t",
	}
	uriOpts := SchedulingOptions{
		PlacementPolicy: "projects/my-proj/regions/us-central1/resourcePolicies/tpu7x-16-2x2x2-placement-policy/",
		IsTPU:           true,
	}
	uriSelectorStr, err := g.buildNodeSelectorWithProfile(uriOpts, uriJob, JobProfile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(uriSelectorStr, "cloud.google.com/placement-policy-name: tpu7x-16-2x2x2-placement-policy") {
		t.Errorf("Expected sanitized placement policy name in %s", uriSelectorStr)
	}
}

func TestGetNodeSelectorAndAffinity_CPUPinningAndExclusions(t *testing.T) {
	t.Parallel()

	sel, err := getNodeSelector(SchedulingOptions{
		MachineType:       "C3-Standard-44",
		PinCPUMachine:     true,
		ClusterNAPEnabled: true,
		NodeAffinityLabels: map[string]string{
			"topology.kubernetes.io/zone": "us-central1-a",
		},
	})
	if err != nil {
		t.Fatalf("getNodeSelector unexpected err: %v", err)
	}
	wantSel := map[string]string{
		corev1.LabelInstanceTypeStable: "c3-standard-44",
		machineFamilyLabel:             "c3",
		"topology.kubernetes.io/zone":  "us-central1-a",
	}
	if !reflect.DeepEqual(sel, wantSel) {
		t.Errorf("getNodeSelector = %v, want %v", sel, wantSel)
	}

	aff, err := GetAffinity(SchedulingOptions{
		ExcludeAccelerators: true,
		ExcludedNodePools:   []string{"", "default-pool", "cpu-np", "cpu-np"},
	})
	if err != nil {
		t.Fatalf("GetAffinity unexpected err: %v", err)
	}
	exprs := aff.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
	wantExprs := []corev1.NodeSelectorRequirement{
		{Key: nodePoolLabel, Operator: corev1.NodeSelectorOpNotIn, Values: []string{"default-pool", "cpu-np"}},
		{Key: "cloud.google.com/gke-accelerator", Operator: corev1.NodeSelectorOpDoesNotExist},
		{Key: "cloud.google.com/gke-tpu-accelerator", Operator: corev1.NodeSelectorOpDoesNotExist},
	}
	if !reflect.DeepEqual(exprs, wantExprs) {
		t.Errorf("GetAffinity MatchExpressions = %+v, want %+v", exprs, wantExprs)
	}

	affWithUserGPU, err := GetAffinity(SchedulingOptions{
		ExcludeAccelerators: true,
		NodeAffinityLabels:  map[string]string{"cloud.google.com/gke-accelerator": "nvidia-l4"},
	})
	if err != nil {
		t.Fatalf("GetAffinity(with user GPU constraint) unexpected err: %v", err)
	}
	gpuExprs := affWithUserGPU.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
	wantGPUExprs := []corev1.NodeSelectorRequirement{
		{Key: nodePoolLabel, Operator: corev1.NodeSelectorOpNotIn, Values: []string{"default-pool"}},
		{Key: "cloud.google.com/gke-tpu-accelerator", Operator: corev1.NodeSelectorOpDoesNotExist},
	}
	if !reflect.DeepEqual(gpuExprs, wantGPUExprs) {
		t.Errorf("GetAffinity(user GPU label) MatchExpressions = %+v, want %+v", gpuExprs, wantGPUExprs)
	}
}

func TestApplyCPUHeadPoolAvoidance_ConstraintsTaintsAndSpot(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		headNP        gkeJobNodePool
		machineType   string
		pinCPUMachine bool
		provisioning  string
		reservation   string
		constraints   map[string]string
		wantExcluded  bool
	}{
		{
			name:          "pipe instance-type excluding cpu-np still excludes head pool",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			machineType:   "c3-standard-44",
			pinCPUMachine: true,
			constraints:   map[string]string{corev1.LabelInstanceTypeStable: "c3-standard-44|c3-highmem-44"},
			wantExcluded:  true,
		},
		{
			name:          "pipe instance-type including cpu-np keeps head pool",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			constraints:   map[string]string{corev1.LabelInstanceTypeStable: "n2d-standard-64|n2d-highmem-96"},
			wantExcluded:  false,
		},
		{
			name:          "machine-family mismatching cpu-np excludes head pool",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			constraints:   map[string]string{machineFamilyLabel: "c3"},
			wantExcluded:  true,
		},
		{
			name: "tainted matching cpu-np is excluded",
			headNP: gkeJobNodePool{
				Name:             "cpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType: "n2d-standard-64",
					Taints:      []gkeTaint{{Key: "dedicated", Value: "pathways", Effect: "NO_SCHEDULE"}},
				},
			},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			wantExcluded:  true,
		},
		{
			name: "spot request keeps matching spot cpu-np with tolerated GKE spot taint",
			headNP: gkeJobNodePool{
				Name:             "cpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType: "n2d-standard-64",
					Spot:        true,
					Taints:      []gkeTaint{{Key: "cloud.google.com/gke-provisioning", Value: "spot", Effect: "NO_SCHEDULE"}},
				},
			},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "spot",
			wantExcluded:  false,
		},
		{
			name: "spot request still excludes spot cpu-np with additional untolerated taint",
			headNP: gkeJobNodePool{
				Name:             "cpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType: "n2d-standard-64",
					Spot:        true,
					Taints: []gkeTaint{
						{Key: "cloud.google.com/gke-provisioning", Value: "spot", Effect: "NO_SCHEDULE"},
						{Key: "dedicated", Value: "pathways", Effect: "NO_SCHEDULE"},
					},
				},
			},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "spot",
			wantExcluded:  true,
		},
		{
			name:          "0-node matching cpu-np without autoscaling is excluded",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 0, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			wantExcluded:  true,
		},
		{
			name:          "on-demand request excludes spot cpu-np",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64", Spot: true}},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "on-demand",
			wantExcluded:  true,
		},
		{
			name:          "on-demand request keeps on-demand cpu-np",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "on-demand",
			wantExcluded:  false,
		},
		{
			name:          "reservation request excludes non-reservation cpu-np",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "reservation",
			reservation:   "my-res",
			wantExcluded:  true,
		},
		{
			name: "reservation request keeps cpu-np matching reservation label and tolerated taint",
			headNP: gkeJobNodePool{
				Name:             "cpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType: "n2d-standard-64",
					Labels: map[string]string{
						"cloud.google.com/reservation-name":    "my-res",
						"cloud.google.com/reservation-project": "p",
					},
					Taints: []gkeTaint{{Key: "cloud.google.com/reservation-name", Value: "my-res", Effect: "NO_SCHEDULE"}},
				},
			},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "reservation",
			reservation:   "projects/p/zones/us-central1-a/reservations/my-res",
			wantExcluded:  false,
		},
		{
			name: "reservation request keeps cpu-np matching ReservationAffinity without config labels",
			headNP: gkeJobNodePool{
				Name:             "cpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType: "n2d-standard-64",
					ReservationAffinity: &gkeReservationAffinity{
						ConsumeReservationType: "SPECIFIC_RESERVATION",
						Key:                    "compute.googleapis.com/reservation-name",
						Values:                 []string{"projects/p/zones/us-central1-a/reservations/my-res"},
					},
					Taints: []gkeTaint{{Key: "cloud.google.com/reservation-name", Value: "my-res", Effect: "NO_SCHEDULE"}},
				},
			},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "reservation",
			reservation:   "projects/p/zones/us-central1-a/reservations/my-res",
			wantExcluded:  false,
		},
		{
			name: "reservation request excludes cpu-np when shared project mismatches",
			headNP: gkeJobNodePool{
				Name:             "cpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType: "n2d-standard-64",
					Labels: map[string]string{
						"cloud.google.com/reservation-name":    "my-res",
						"cloud.google.com/reservation-project": "other-proj",
					},
				},
			},
			machineType:   "n2d-standard-64",
			pinCPUMachine: true,
			provisioning:  "reservation",
			reservation:   "projects/owner-proj/zones/us-central1-a/reservations/my-res",
			wantExcluded:  true,
		},
		{
			name: "reservation request with pipe constraint excludes cpu-np carrying untolerated extra taint",
			headNP: gkeJobNodePool{
				Name:             "cpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType: "n2d-standard-64",
					Labels:      map[string]string{"cloud.google.com/reservation-name": "my-res"},
					Taints: []gkeTaint{
						{Key: "cloud.google.com/reservation-name", Value: "my-res", Effect: "NO_SCHEDULE"},
						{Key: "dedicated", Value: "pathways", Effect: "NO_SCHEDULE"},
					},
				},
			},
			machineType:   "c3-standard-44",
			pinCPUMachine: true,
			constraints:   map[string]string{corev1.LabelInstanceTypeStable: "n2d-standard-64|c3-standard-44"},
			provisioning:  "reservation",
			reservation:   "my-res",
			wantExcluded:  true,
		},
		{
			name:          "PinCPUMachine false skips head pool avoidance",
			headNP:        gkeJobNodePool{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			machineType:   "c3-standard-44",
			pinCPUMachine: false,
			wantExcluded:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orc := newTestGKEOrchestrator(nil)
			orc.clusterDesc.NodePools = []gkeJobNodePool{tc.headNP}
			schedOpts := SchedulingOptions{MachineType: tc.machineType, PinCPUMachine: tc.pinCPUMachine, NodeAffinityLabels: tc.constraints}
			orc.applyCPUHeadPoolAvoidance(&schedOpts, tc.provisioning, tc.reservation)
			gotExcluded := len(schedOpts.ExcludedNodePools) > 0 && schedOpts.ExcludedNodePools[0] == tc.headNP.Name
			if gotExcluded != tc.wantExcluded {
				t.Errorf("ExcludedNodePools = %v, wantExcluded = %v", schedOpts.ExcludedNodePools, tc.wantExcluded)
			}
		})
	}
}

func TestApplyCPUHeadPoolAvoidance_MultipleHeadPoolsAndKueueDisabled(t *testing.T) {
	t.Parallel()
	pools := []gkeJobNodePool{
		{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
		{Name: "pathways-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n4-standard-64"}},
	}

	orc := newTestGKEOrchestrator(nil)
	orc.clusterDesc.NodePools = pools
	schedOpts := SchedulingOptions{MachineType: "c3-standard-44", PinCPUMachine: true}
	orc.applyCPUHeadPoolAvoidance(&schedOpts, "", "")
	if !slices.Equal(schedOpts.ExcludedNodePools, []string{"cpu-np", "pathways-np"}) {
		t.Errorf("ExcludedNodePools = %v, want [cpu-np pathways-np]", schedOpts.ExcludedNodePools)
	}

	orcNoKueue := newTestGKEOrchestrator(nil)
	orcNoKueue.clusterDesc.NodePools = pools
	orcNoKueue.addons = addonPlan{resolved: true, jobSetEnabled: true, kueueEnabled: false}
	noKueueOpts := SchedulingOptions{MachineType: "c3-standard-44", PinCPUMachine: true}
	orcNoKueue.applyCPUHeadPoolAvoidance(&noKueueOpts, "", "")
	if len(noKueueOpts.ExcludedNodePools) != 0 {
		t.Errorf("expected no exclusions when Kueue is disabled, got %v", noKueueOpts.ExcludedNodePools)
	}

	orcTaintedNAP := newTestGKEOrchestrator(nil)
	orcTaintedNAP.napEnabled = true
	orcTaintedNAP.clusterDesc.NodePools = []gkeJobNodePool{{
		Name:             "cpu-np",
		InitialNodeCount: 1,
		Config: gkeNodePoolConfig{
			MachineType: "n2d-standard-64",
			Taints:      []gkeTaint{{Key: "dedicated", Value: "pathways", Effect: "NO_SCHEDULE"}},
		},
	}}
	if !orcTaintedNAP.shouldPinCPUMachine(true, "", "", "", "", "n2d-standard-64") {
		t.Errorf("expected NAP full-node CPU job to pin when only matching static pool is tainted")
	}
}
