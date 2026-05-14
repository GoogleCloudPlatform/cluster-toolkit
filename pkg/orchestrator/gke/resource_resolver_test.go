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
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"testing"

	compute "google.golang.org/api/compute/v1"
)

func TestResolveMachineName(t *testing.T) {
	tests := []struct {
		name            string
		acceleratorType string
		wantMachineName string
		wantErr         bool
	}{
		{
			name:            "Direct shorthand mapping",
			acceleratorType: "l4-1",
			wantMachineName: "g2-standard-12",
		},
		{
			name:            "TPU shorthand mapping",
			acceleratorType: "v5p-1",
			wantMachineName: "ct5p-hightpu-1t",
		},
		{
			name:            "Unknown type falls back to input",
			acceleratorType: "unknown-machine",
			wantMachineName: "unknown-machine",
			wantErr:         true,
		},
		{
			name:            "GKE label string resolution for unknown shorthand",
			acceleratorType: "nvidia-l4",
			wantMachineName: "nvidia-l4", // Default fallthrough if neither matches
			wantErr:         true,
		},
		{
			name:            "TPU7x shorthand mapping",
			acceleratorType: "tpu7x",
			wantMachineName: "tpu7x-standard-4t",
		},
	}

	g := newTestGKEOrchestrator(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.resolveMachineName(tt.acceleratorType)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveMachineName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantMachineName {
				t.Errorf("resolveMachineName() = %v, want %v", got, tt.wantMachineName)
			}
		})
	}
}

func TestResolveMachineName_Dynamic(t *testing.T) {
	g := NewGKEOrchestrator()
	g.acceleratorToMachineType["nvidia-gb300"] = "a4-highgpu-8g"

	got, err := g.resolveMachineName("nvidia-gb300")
	if err != nil {
		t.Errorf("resolveMachineName() returned error: %v", err)
	}
	if got != "a4-highgpu-8g" {
		t.Errorf("resolveMachineName() = %v, want %v", got, "a4-highgpu-8g")
	}

	// Test case insensitivity
	got, err = g.resolveMachineName("Nvidia-GB300")
	if err != nil {
		t.Errorf("resolveMachineName() returned error: %v", err)
	}
	if got != "a4-highgpu-8g" {
		t.Errorf("resolveMachineName() case insensitive = %v, want %v", got, "a4-highgpu-8g")
	}
}

func TestFetchMachineCapacity_AllZonesFail(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.machineTypeClient = &MockMachineTypeClient{FailAll: true}
	g.clusterZones = []string{"europe-west2-a", "europe-west2-c", "europe-west2-b"}

	_, err := g.FetchMachineCapacity("tpu7x-1", "europe-west2")

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	expectedErrStr := "failed to fetch machine capabilities for tpu7x-1: tried in all candidate zones [europe-west2-a europe-west2-c europe-west2-b] but did not find machine type in any of them"
	if err.Error() != expectedErrStr {
		t.Errorf("Expected error %q, got %q", expectedErrStr, err.Error())
	}
}

func TestFetchMachineCapabilities_Caching(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.machineTypeClient = &MockMachineTypeClient{
		MT: &compute.MachineType{
			GuestCpus: 32,
			MemoryMb:  131072,
		},
	}

	// First call
	cap1, err := g.FetchMachineCapabilities("n2-standard-32", "us-east5-b")
	if err != nil {
		t.Fatalf("FetchMachineCapabilities failed: %v", err)
	}
	if cap1.GuestCpus != 32 {
		t.Errorf("cap1.GuestCpus = %d, want 32", cap1.GuestCpus)
	}

	// Second call - should hit cache
	cap2, err := g.FetchMachineCapabilities("n2-standard-32", "us-east5-b")
	if err != nil {
		t.Fatalf("FetchMachineCapabilities failed on second call: %v", err)
	}
	if cap2.GuestCpus != 32 {
		t.Errorf("cap2.GuestCpus = %d, want 32", cap2.GuestCpus)
	}

}

func TestCalculateResourceLimits_CPU(t *testing.T) {
	tests := []struct {
		name          string
		capacityCount int
		wantCPU       string
	}{
		{
			name:          "Large capacity",
			capacityCount: 32,
			wantCPU:       "30",
		},
		{
			name:          "Small capacity (rounds down to 1)",
			capacityCount: 2,
			wantCPU:       "1",
		},
		{
			name:          "Capacity 1 (offset < 1, fallback to 1)",
			capacityCount: 1,
			wantCPU:       "1",
		},
		{
			name:          "Capacity 0 (fallback to 1)",
			capacityCount: 0,
			wantCPU:       "1",
		},
	}

	g := newTestGKEOrchestrator(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ManifestOptions{ComputeType: "n2-standard-32", MachineType: "n2-standard-32"}
			profile := JobProfile{
				IsCPUMachine:  true,
				CapacityCount: tt.capacityCount,
			}

			cpu, mem, gpu, tpu, err := g.calculateResourceLimits(opts, profile)
			if err != nil {
				t.Fatalf("calculateResourceLimits failed: %v", err)
			}
			if cpu != tt.wantCPU {
				t.Errorf("cpu = %v, want %v", cpu, tt.wantCPU)
			}
			if mem != "" || gpu != "" || tpu != "" {
				t.Errorf("mem, gpu, tpu = %q, %q, %q; want empty strings", mem, gpu, tpu)
			}
		})
	}
}

type MockMachineTypeClient struct {
	FailAll  bool
	MT       *compute.MachineType
	Executor Executor
}

func (m *MockMachineTypeClient) GetMachineType(project, zone, machineType string) (*compute.MachineType, error) {
	if m.FailAll {
		return nil, fmt.Errorf("resource not found")
	}
	if m.MT != nil {
		return m.MT, nil
	}
	if m.Executor != nil {
		res := m.Executor.ExecuteCommand("gcloud", "compute", "machine-types", "describe", machineType, "--zone="+zone, "--format=json")
		if res.ExitCode != 0 {
			return nil, fmt.Errorf("failed to get machine type: %s", res.Stderr)
		}
		var mt compute.MachineType
		if err := json.Unmarshal([]byte(res.Stdout), &mt); err != nil {
			return nil, err
		}
		return &mt, nil
	}
	return nil, fmt.Errorf("mock not configured")
}

func TestResolveAcceleratorShorthand(t *testing.T) {
	setupMockMachineConfig(t)

	tests := []struct {
		name            string
		acceleratorType string
		mockResponses   map[string][]shell.CommandResult
		nodePools       []string
		wantType        string
		wantTopology    string
		wantErr         bool
	}{
		{
			name:            "Valid shorthand in map",
			acceleratorType: "v4-8",
			wantType:        "ct4p-hightpu-4t", // Resolves to full machine type
			wantErr:         false,
		},
		{
			name:            "Valid full type in map values",
			acceleratorType: "ct4p-hightpu-4t",
			wantType:        "ct4p-hightpu-4t",
			wantErr:         false,
		},
		{
			name:            "Valid full type in cluster",
			acceleratorType: "custom-c2-60",
			nodePools:       []string{"custom-c2-60"},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe custom-c2-60": {{ExitCode: 0, Stdout: `{"guestCpus": 60, "memoryMb": 240000}`}},
			},
			wantType: "custom-c2-60",
			wantErr:  false,
		},
		{
			name:            "Invalid full type not in cluster",
			acceleratorType: "custom-invalid-type",
			nodePools:       []string{"other-type"},
			wantErr:         true,
		},
		{
			name:            "Ambiguous shorthand resolved",
			acceleratorType: "v6e",
			nodePools:       []string{"ct6e-standard-8t"},
			wantType:        "ct6e-standard-8t",
			wantErr:         false,
		},
		{
			name:            "Unknown shorthand with less than 2 hyphens",
			acceleratorType: "unknown",
			wantErr:         true,
		},
		{
			name:            "TPU shorthand with chip count resolved",
			acceleratorType: "v6e-256",
			nodePools:       []string{"ct6e-standard-8t"},
			wantType:        "ct6e-standard-8t", // Resolves to full machine type
			wantTopology:    "16x16",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResponses := tt.mockResponses
			if mockResponses == nil {
				mockResponses = make(map[string][]shell.CommandResult)
			}
			// Add default mocks for topology discovery if not provided
			if _, ok := mockResponses["kubectl get resourceflavors"]; !ok {
				mockResponses["kubectl get resourceflavors"] = []shell.CommandResult{{ExitCode: 0, Stdout: ""}}
			}
			if _, ok := mockResponses["kubectl get nodes -o jsonpath"]; !ok {
				mockResponses["kubectl get nodes -o jsonpath"] = []shell.CommandResult{{ExitCode: 0, Stdout: "16x16\n"}}
			}

			mockExecutor := NewMockExecutor(mockResponses)
			orc := newTestGKEOrchestrator(mockExecutor)
			orc.projectID = "mock-project"
			if len(tt.nodePools) > 0 {
				for _, mt := range tt.nodePools {
					orc.clusterDesc.NodePools = append(orc.clusterDesc.NodePools, gkeJobNodePool{
						Config: gkeNodePoolConfig{MachineType: mt},
					})
				}
			}

			job := &orchestrator.JobDefinition{
				ComputeType: tt.acceleratorType,
			}

			_, _, err := orc.resolveHardwareRequirements(job)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if job.MachineType != tt.wantType {
					t.Errorf("Expected job.MachineType %q, got %q", tt.wantType, job.MachineType)
				}
				if tt.wantTopology != "" && job.Topology != tt.wantTopology {
					t.Errorf("Expected job.Topology %q, got %q", tt.wantTopology, job.Topology)
				}
			}
		})
	}
}
