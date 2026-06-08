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
	"strings"
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
			name:            "Valid shorthand in map",
			acceleratorType: "v4-8",
			nodePools:       []string{"ct4p-hightpu-4t"},
			wantType:        "ct4p-hightpu-4t",
			wantTopology:    "2x2x1",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get nodes -o jsonpath": {{ExitCode: 0, Stdout: "2x2x1\n"}},
			},
			wantErr: false,
		},
		{
			name:            "Valid shorthand in map fails with conflicting cluster topology",
			acceleratorType: "v4-8",
			nodePools:       []string{"ct4p-hightpu-4t"},
			wantType:        "ct4p-hightpu-4t",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get nodes -o jsonpath": {{ExitCode: 0, Stdout: "16x16\n"}},
			},
			wantErr: true,
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
		{
			name:            "TPU shorthand with invalid size (not power of 2)",
			acceleratorType: "v6e-12",
			wantErr:         true,
		},
		{
			name:            "TPU shorthand with topology suffix fails",
			acceleratorType: "v6e-4x4",
			wantErr:         true,
		},
		{
			name:            "TPU shorthand with valid size >= 16",
			acceleratorType: "v6e-32",
			nodePools:       []string{"ct6e-standard-8t"},
			wantType:        "ct6e-standard-8t",
			wantTopology:    "4x8",
			wantErr:         false,
		},
		{
			name:            "TPU7x shorthand fails with explicit topology requirement",
			acceleratorType: "tpu7x-32",
			wantErr:         true,
		},
		{
			name:            "TPU7x full machine type fails with empty topology",
			acceleratorType: "tpu7x-standard-4t",
			wantErr:         true,
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

			_, _, _, err := orc.resolveHardwareRequirements(job)

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

func TestVerifyStaticSlicingActive(t *testing.T) {
	tests := []struct {
		name           string
		machineType    string
		requestedTopo  string
		mockResponses  map[string][]shell.CommandResult
		wantActive     bool
		wantErr        bool
		verifyCacheHit bool
	}{
		{
			name:          "Non-TPU machine type",
			machineType:   "n2-standard-8",
			requestedTopo: "2x2",
			wantActive:    false,
		},
		{
			name:          "TPU v4 (3D Torus) bypasses static sub-slicing",
			machineType:   "ct4p-hightpu-4t",
			requestedTopo: "2x2x1",
			wantActive:    false,
		},
		{
			name:          "TPU v5p (3D Torus) bypasses static sub-slicing",
			machineType:   "ct5p-hightpu-4t",
			requestedTopo: "2x2x2",
			wantActive:    false,
		},
		{
			name:          "No Kueue topologies configured",
			machineType:   "ct6e-standard-8t",
			requestedTopo: "2x2",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io -o json": {
					{ExitCode: 1, Stderr: "NotFound"},
				},
			},
			wantActive: false,
		},
		{
			name:          "Empty topologies configured",
			machineType:   "ct6e-standard-8t",
			requestedTopo: "2x2",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io -o json": {
					{ExitCode: 0, Stdout: `{"items":[]}`},
				},
			},
			wantActive: false,
		},
		{
			name:          "Static sub-slicing active (requested shape fits physical)",
			machineType:   "ct6e-standard-8t",
			requestedTopo: "2x2",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`},
				},
				"kubectl get resourceflavors.kueue.x-k8s.io -o jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end} -l cloud.google.com/gke-tpu-accelerator=tpu-v6e-slice": {
					{ExitCode: 0, Stdout: "4x4\n"},
				},
			},
			wantActive:     true,
			verifyCacheHit: true,
		},
		{
			name:          "Full-slice topology matches physical (Still TAS active)",
			machineType:   "ct6e-standard-8t",
			requestedTopo: "4x4",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`},
				},
				"kubectl get resourceflavors.kueue.x-k8s.io -o jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end} -l cloud.google.com/gke-tpu-accelerator=tpu-v6e-slice": {
					{ExitCode: 0, Stdout: "4x4\n"},
				},
			},
			wantActive: true,
		},
		{
			name:          "Requested shape too large for physical",
			machineType:   "ct6e-standard-8t",
			requestedTopo: "8x8",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`},
				},
				"kubectl get resourceflavors.kueue.x-k8s.io -o jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end} -l cloud.google.com/gke-tpu-accelerator=tpu-v6e-slice": {
					{ExitCode: 0, Stdout: "4x4\n"},
				},
			},
			wantActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := NewMockExecutor(tt.mockResponses)
			orc := newTestGKEOrchestrator(mockExec)

			job := &orchestrator.JobDefinition{
				MachineType: tt.machineType,
				Topology:    tt.requestedTopo,
			}

			got, err := orc.verifyStaticSlicingActive(job)
			if (err != nil) != tt.wantErr {
				t.Fatalf("verifyStaticSlicingActive() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantActive {
				t.Errorf("verifyStaticSlicingActive() = %v, want %v", got, tt.wantActive)
			}

			if tt.verifyCacheHit && err == nil && got == tt.wantActive {
				// Clear mock executor to ensure subsequent call is satisfied entirely from cache
				orc.executor = NewMockExecutor(nil)
				got2, err2 := orc.verifyStaticSlicingActive(job)
				if err2 != nil || got2 != tt.wantActive {
					t.Errorf("cache hit failed: got %v, err %v", got2, err2)
				}
			}
		})
	}
}

func TestValidateConsumptionForStaticCluster(t *testing.T) {
	tests := []struct {
		name        string
		napEnabled  bool
		napLimits   map[string]int64
		nodePools   []gkeJobNodePool
		job         orchestrator.JobDefinition
		wantErr     bool
		expectedErr string
	}{
		{
			name:       "Static Cluster - No flags set",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "on-demand",
			},
			wantErr: false,
		},
		{
			name:       "Static Cluster - Empty string consumption model",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "",
			},
			wantErr: false,
		},
		{
			name:       "Static Cluster - Consumption model flag set to spot",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning=\"spot\", --gke-nap-reservation=\"\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
		},
		{
			name:       "Static Cluster - Reservation name flag set",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "on-demand",
				GKENAPReservation:  "my-res",
			},
			wantErr:     true,
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning=\"on-demand\", --gke-nap-reservation=\"my-res\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
		},
		{
			name:       "NAP Cluster - Machine type in NAP limits",
			napEnabled: true,
			napLimits: map[string]int64{
				"tpu-v6e-slice": 100,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "ct6e-standard-8t", // TPU
				GKENAPProvisioning: "spot",
			},
			wantErr: false,
		},
		{
			name:       "NAP Cluster - Machine type not in limits, but matches static pool",
			napEnabled: true,
			napLimits:  map[string]int64{},
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "n2-standard-4",
						Labels: map[string]string{
							"cloud.google.com/gke-provisioning": "spot",
						},
					},
				},
			},
			job: orchestrator.JobDefinition{
				MachineType:        "n2-standard-4",
				GKENAPProvisioning: "spot",
			},
			wantErr: false,
		},
		{
			name:       "NAP Cluster - Machine type not in limits, and mismatches static pool",
			napEnabled: true,
			napLimits:  map[string]int64{},
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "n2-standard-4",
						Labels: map[string]string{
							"cloud.google.com/gke-provisioning": "standard",
						},
					},
				},
			},
			job: orchestrator.JobDefinition{
				MachineType:        "n2-standard-4",
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "but the cluster's static node pools for this hardware are configured exclusively as Standard/On-Demand",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := newTestGKEOrchestrator(nil)
			orc.napEnabled = tt.napEnabled
			orc.napLimits = tt.napLimits
			orc.clusterDesc.NodePools = tt.nodePools

			err := orc.validateConsumptionForStaticCluster(&tt.job)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("expected error containing %q, got: %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
