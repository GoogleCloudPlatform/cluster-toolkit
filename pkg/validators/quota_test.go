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

package validators

import (
	"fmt"
	"testing"

	"hpc-toolkit/pkg/config"

	"github.com/zclconf/go-cty/cty"
	"google.golang.org/api/compute/v1"
)

type MockQuotaClient struct {
	Projects     map[string]*compute.Project
	Regions      map[string]*compute.Region
	MachineTypes map[string]*compute.MachineType
}

func (m *MockQuotaClient) GetProject(projectID string) (*compute.Project, error) {
	if p, ok := m.Projects[projectID]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("project not found: %s", projectID)
}

func (m *MockQuotaClient) GetRegion(projectID, region string) (*compute.Region, error) {
	key := fmt.Sprintf("%s/%s", projectID, region)
	if r, ok := m.Regions[key]; ok {
		return r, nil
	}
	return &compute.Region{
		Zones:  []string{fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s-a", projectID, region)},
		Quotas: []*compute.Quota{},
	}, nil
}

func (m *MockQuotaClient) GetMachineType(projectID, zone, machineType string) (*compute.MachineType, error) {
	key := fmt.Sprintf("%s/%s/%s", projectID, zone, machineType)
	if mt, ok := m.MachineTypes[key]; ok {
		return mt, nil
	}
	return nil, fmt.Errorf("machine type not found: %s", key)
}

func TestCollectRequirements(t *testing.T) {
	client := &MockQuotaClient{
		MachineTypes: map[string]*compute.MachineType{
			"test-project/us-central1-a/n1-standard-4": {
				GuestCpus: 4,
			},
			"test-project/us-central1-a/a2-highgpu-1g": {
				GuestCpus: 12,
				Accelerators: []*compute.MachineTypeAccelerators{
					{GuestAcceleratorType: "nvidia-tesla-a100", GuestAcceleratorCount: 1},
				},
			},
			"test-project/us-central1-a/c3-standard-8": {
				GuestCpus: 8,
			},
			"test-project/us-central1-a/c4-standard-8": {
				GuestCpus: 8,
			},
			"test-project/us-central1-a/h100-vm": {
				GuestCpus: 96,
				Accelerators: []*compute.MachineTypeAccelerators{
					{GuestAcceleratorType: "nvidia-h100-80gb", GuestAcceleratorCount: 8},
				},
			},
			"test-project/us-central1-a/a3-megagpu-8g": {
				GuestCpus: 208,
				Accelerators: []*compute.MachineTypeAccelerators{
					{GuestAcceleratorType: "nvidia-h100-mega-80gb", GuestAcceleratorCount: 8},
				},
			},
			"test-project/us-central1-a/a2-ultragpu-1g": {
				GuestCpus: 12,
				Accelerators: []*compute.MachineTypeAccelerators{
					{GuestAcceleratorType: "nvidia-a100-80gb", GuestAcceleratorCount: 1},
				},
			},
			"test-project/us-east1-b/a2-highgpu-1g": {
				GuestCpus: 12,
				Accelerators: []*compute.MachineTypeAccelerators{
					{GuestAcceleratorType: "nvidia-a100", GuestAcceleratorCount: 1},
				},
			},
		},
	}

	tests := []struct {
		name     string
		modules  []config.Module
		expected []QuotaRequirement
	}{
		{
			name: "Simple VM",
			modules: []config.Module{
				{
					ID: "vm1",
					Settings: config.NewDict(map[string]cty.Value{
						"machine_type": cty.StringVal("n1-standard-4"),
						"zone":         cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "CPUS", Needed: 4},
			},
		},
		{
			name: "C3 Family VM",
			modules: []config.Module{
				{
					ID: "c3vm",
					Settings: config.NewDict(map[string]cty.Value{
						"machine_type": cty.StringVal("c3-standard-8"),
						"zone":         cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "C3_CPUS", Needed: 8},
			},
		},
		{
			name: "C4 Future Proof",
			modules: []config.Module{
				{
					ID: "c4vm",
					Settings: config.NewDict(map[string]cty.Value{
						"machine_type": cty.StringVal("c4-standard-8"),
						"zone":         cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "C4_CPUS", Needed: 8},
			},
		},
		{
			name: "H100 with Family Metric",
			modules: []config.Module{
				{
					ID: "h100vm",
					Settings: config.NewDict(map[string]cty.Value{
						"machine_type": cty.StringVal("h100-vm"),
						"zone":         cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "NVIDIA_H100_GPUS", Needed: 8},
				{ProjectID: "test-project", Region: "us-central1", Metric: "H100_CPUS", Needed: 96},
				{ProjectID: "test-project", Region: "global", Metric: "GPUS_ALL_REGIONS", Needed: 8}, 
			},
		},
		{
			name: "PD Extreme",
			modules: []config.Module{
				{
					ID: "pd-ext",
					Settings: config.NewDict(map[string]cty.Value{
						"disk_type":            cty.StringVal("pd-extreme"),
						"disk_size_gb":         cty.NumberIntVal(500),
						"zone":                 cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "EXTREME_TOTAL_GB", Needed: 500},
			},
		},
		{
			name: "A3 Mega (H100) - No CPU Check",
			modules: []config.Module{
				{
					ID: "a3mega",
					Settings: config.NewDict(map[string]cty.Value{
						"machine_type": cty.StringVal("a3-megagpu-8g"),
						"zone":         cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "NVIDIA_H100_GPUS", Needed: 8},
				{ProjectID: "test-project", Region: "global", Metric: "GPUS_ALL_REGIONS", Needed: 8}, 
			},
		},
		{
			name: "A100 80GB vs 40GB",
			modules: []config.Module{
				{
					ID: "a100-80",
					Settings: config.NewDict(map[string]cty.Value{
						"machine_type": cty.StringVal("a2-ultragpu-1g"),
						"zone":         cty.StringVal("us-central1-a"),
					}),
				},
				{
					ID: "a100-40",
					Settings: config.NewDict(map[string]cty.Value{
						"machine_type": cty.StringVal("a2-highgpu-1g"),
						"zone":         cty.StringVal("us-east1-b"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "NVIDIA_A100_80GB_GPUS", Needed: 1},
				{ProjectID: "test-project", Region: "us-central1", Metric: "A2_CPUS", Needed: 12},

				{ProjectID: "test-project", Region: "us-east1", Metric: "NVIDIA_A100_GPUS", Needed: 1},
				{ProjectID: "test-project", Region: "us-east1", Metric: "A2_CPUS", Needed: 12}, 
				
				{ProjectID: "test-project", Region: "global", Metric: "GPUS_ALL_REGIONS", Needed: 2}, 
			},
		},
		{
			name: "Hyperdisk Balance with IOPS",
			modules: []config.Module{
				{
					ID: "hdb",
					Settings: config.NewDict(map[string]cty.Value{
						"disk_type":            cty.StringVal("hyperdisk-balanced"),
						"disk_size_gb":         cty.NumberIntVal(100),
						"provisioned_iops":     cty.NumberIntVal(3000),
						"provisioned_throughput": cty.NumberIntVal(150),
						"zone":                 cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "HYPERDISK_BALANCED_TOTAL_GB", Needed: 100},
				{ProjectID: "test-project", Region: "us-central1", Metric: "HYPERDISK_BALANCED_IOPS", Needed: 3000},
				{ProjectID: "test-project", Region: "us-central1", Metric: "HYPERDISK_BALANCED_THROUGHPUT", Needed: 150},
			},
		},
		{
			name: "TPU v2-8",
			modules: []config.Module{
				{
					ID: "tpu",
					Settings: config.NewDict(map[string]cty.Value{
						"accelerator_type": cty.StringVal("v2-8"),
						"zone":             cty.StringVal("us-central1-a"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "V2_TPUS", Needed: 8},
			},
		},
		{
			name: "Filestore High Scale",
			modules: []config.Module{
				{
					ID:     "fs",
					Source: "modules/filestore",
					Settings: config.NewDict(map[string]cty.Value{
						"tier":        cty.StringVal("HIGH_SCALE_SSD"),
						"capacity_gb": cty.NumberIntVal(2560),
						"region":      cty.StringVal("us-central1"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "us-central1", Metric: "HighScaleSSDStorageGibPerRegion", Needed: 2560},
			},
		},
		{
			name: "Global Subnetworks",
			modules: []config.Module{
				{
					ID: "vpc",
					Settings: config.NewDict(map[string]cty.Value{
						"subnetworks": cty.ListVal([]cty.Value{
							cty.ObjectVal(map[string]cty.Value{
								"subnet_region": cty.StringVal("us-central1"),
							}),
						}),
						"region": cty.StringVal("us-central1"),
					}),
				},
			},
			expected: []QuotaRequirement{
				{ProjectID: "test-project", Region: "global", Metric: "SUBNETWORKS", Needed: 1},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bp := config.Blueprint{
				Vars: config.NewDict(nil),
				Groups: []config.Group{
					{Modules: tc.modules},
				},
			}
			reqs, err := collectRequirements(bp, client, "test-project", "us-central1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(reqs) != len(tc.expected) {
				t.Errorf("expected %d requirements, got %d. Got: %v", len(tc.expected), len(reqs), reqs)
			}
			
			for _, exp := range tc.expected {
				found := false
				for _, act := range reqs {
					if act.Metric == exp.Metric && act.Region == exp.Region {
						if act.Needed != exp.Needed {
							t.Errorf("for %s/%s expected %.0f, got %.0f", exp.Region, exp.Metric, exp.Needed, act.Needed)
						}
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing requirement: %v", exp)
				}
			}
		})
	}
}
