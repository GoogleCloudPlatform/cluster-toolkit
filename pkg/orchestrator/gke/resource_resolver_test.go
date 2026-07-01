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
	g.clusterDesc.Autoscaling.EnableNodeAutoprovisioning = true

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
			orc.clusterZones = []string{"us-central1-a"}
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
		nodePools      []gkeJobNodePool
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
		{
			name:          "Static sub-slicing active (discovered from scaled-to-0 node pool)",
			machineType:   "ct6e-standard-8t",
			requestedTopo: "2x2",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`},
				},
				"kubectl get resourceflavors.kueue.x-k8s.io -o jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end} -l cloud.google.com/gke-tpu-accelerator=tpu-v6e-slice": {
					{ExitCode: 0, Stdout: ""},
				},
				"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end} -l cloud.google.com/gke-tpu-accelerator=tpu-v6e-slice": {
					{ExitCode: 0, Stdout: ""},
				},
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "tpu-pool-4x4",
					Config: gkeNodePoolConfig{
						MachineType: "ct6e-standard-8t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						TpuTopology: "4x4",
					},
				},
			},
			wantActive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := NewMockExecutor(tt.mockResponses)
			orc := newTestGKEOrchestrator(mockExec)
			if len(tt.nodePools) > 0 {
				orc.clusterDesc.NodePools = tt.nodePools
			}

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

func TestResolveHardwareRequirements_NAPIncompatibilities(t *testing.T) {
	setupMockMachineConfig(t)

	tests := []struct {
		name               string
		napEnabled         bool
		gkeNapProvisioning string
		scheduler          string
		topology           string
		computeType        string
		machineType        string
		dynamicSlicing     bool
		mockResponses      map[string][]shell.CommandResult
		wantErr            bool
		expectedErrMatch   string
	}{
		{
			name:        "NAP Cluster - Standard TPU allowed",
			napEnabled:  true,
			computeType: "v6e-8",
			wantErr:     false,
		},
		{
			name:               "NAP Cluster - TPU Dynamic Slicing disallowed",
			napEnabled:         true,
			gkeNapProvisioning: "spot",
			computeType:        "tpu7x-128",
			machineType:        "tpu7x-standard-4t",
			topology:           "4x4x8",
			dynamicSlicing:     true,
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get nodes -o jsonpath": {{ExitCode: 0, Stdout: "4x4x8\n"}},
			},
			wantErr:          true,
			expectedErrMatch: "TPU Dynamic Slicing is not supported on GKE Node Auto-Provisioning (NAP) workloads",
		},
		{
			name:               "NAP Cluster - TPU Static Sub-slicing disallowed",
			napEnabled:         true,
			gkeNapProvisioning: "spot",
			computeType:        "v6e-8",
			machineType:        "v6e-standard-8t",
			topology:           "2x4",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io": {{ExitCode: 0, Stdout: `{"items": [{"metadata":{"name":"tpu-v6e-slice"},"spec":{"topologies":["4x8"]}}]}`}},
			},
			wantErr:          true,
			expectedErrMatch: "TPU Static Sub-slicing is not supported on GKE Node Auto-Provisioning (NAP) workloads",
		},
		{
			name:               "NAP Cluster - DWS Flex Scheduler disallowed",
			napEnabled:         true,
			gkeNapProvisioning: "spot",
			computeType:        "v6e-8",
			scheduler:          "gke.io/tpu-provisioning-request",
			wantErr:            true,
			expectedErrMatch:   "TPU ProvisioningRequest (DWS Flex) is not supported on GKE Node Auto-Provisioning (NAP) workloads",
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
				mockResponses["kubectl get nodes -o jsonpath"] = []shell.CommandResult{{ExitCode: 0, Stdout: "4x8\n"}, {ExitCode: 0, Stdout: "4x8\n"}}
			}

			orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
			orc.napEnabled = tt.napEnabled
			orc.projectID = "mock-project"

			if tt.dynamicSlicing {
				orc.dynamicSlicingCache = map[string]bool{
					tt.machineType:                     true,
					tt.computeType:                     true,
					tt.computeType + ":" + tt.topology: true,
					tt.machineType + ":" + tt.topology: true,
				}
			}

			// Populate a node pool so the ambiguous shorthands 'v6e' and 'tpu7x' can be resolved
			orc.clusterDesc.NodePools = append(orc.clusterDesc.NodePools, gkeJobNodePool{
				Config: gkeNodePoolConfig{MachineType: "ct6e-standard-8t"},
			})
			orc.clusterDesc.NodePools = append(orc.clusterDesc.NodePools, gkeJobNodePool{
				Config: gkeNodePoolConfig{MachineType: "tpu7x-standard-4t"},
			})

			job := &orchestrator.JobDefinition{
				ComputeType:        tt.computeType,
				MachineType:        tt.machineType,
				Topology:           tt.topology,
				GKEScheduler:       tt.scheduler,
				GKENAPProvisioning: tt.gkeNapProvisioning,
			}

			_, _, _, err := orc.resolveHardwareRequirements(job)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectedErrMatch) {
					t.Errorf("expected error to contain %q, got: %v", tt.expectedErrMatch, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
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
			name:       "Static Cluster - Explicit on-demand flag fails",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "on-demand",
			},
			wantErr:     true,
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning \"on-demand\", --gke-nap-reservation \"\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
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
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning \"spot\", --gke-nap-reservation \"\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
		},
		{
			name:       "Static Cluster - Reservation name flag set",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "on-demand",
				GKENAPReservation:  "my-res",
			},
			wantErr:     true,
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning \"on-demand\", --gke-nap-reservation \"my-res\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
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
				ComputeType:        "n2-standard-4",
				MachineType:        "n2-standard-4",
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
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
				ComputeType:        "n2-standard-4",
				MachineType:        "n2-standard-4",
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
		},
		{
			name:       "NAP Cluster - Machine type covered by generic TPU limit fallback",
			napEnabled: true,
			napLimits: map[string]int64{
				"google.com/tpu": 100,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "ct6e-standard-8t",
				GKENAPProvisioning: "spot",
			},
			wantErr: false,
		},
		{
			name:       "NAP Cluster - Machine type with unknown GPU accelerator fails fast",
			napEnabled: true,
			napLimits:  map[string]int64{},
			job: orchestrator.JobDefinition{
				MachineType:        "my-unknown-gpu-machine",
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "unknown accelerator label: \"unknown-gpu\"",
		},
		{
			name:       "NAP Cluster - TPU: Specific limit configured, requesting different TPU (Should Fail)",
			napEnabled: true,
			napLimits: map[string]int64{
				"tpu-v6e-slice":  8,
				"google.com/tpu": 8,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "ct5lp-hightpu-4t", // TPU v5e (tpu-v5-lite-podslice)
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
		},
		{
			name:       "NAP Cluster - GPU: Specific limit configured, requesting different GPU (Should Fail)",
			napEnabled: true,
			napLimits: map[string]int64{
				"nvidia-h100-mega-80gb": 8,
				"nvidia.com/gpu":        8,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "g2-standard-12", // L4 GPU (nvidia-l4)
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
		},
		{
			name:       "NAP Cluster - GPU: Generic limit only, requesting GPU (Should Pass)",
			napEnabled: true,
			napLimits: map[string]int64{
				"nvidia.com/gpu": 8,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "g2-standard-12",
				GKENAPProvisioning: "spot",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := newTestGKEOrchestrator(nil)
			orc.napEnabled = tt.napEnabled
			orc.napLimits = tt.napLimits
			orc.clusterDesc.NodePools = tt.nodePools
			orc.machineCapCache = map[string]MachineTypeCap{
				"n2-standard-4:": {
					GuestCpus: 4,
					MemoryMb:  16000,
				},
				"my-unknown-gpu-machine:": {
					GuestCpus: 8,
					MemoryMb:  32000,
					Accelerators: []struct {
						Count int    `json:"guestAcceleratorCount"`
						Type  string `json:"guestAcceleratorType"`
					}{
						{
							Count: 1,
							Type:  "unknown-gpu",
						},
					},
				},
				"g2-standard-12:": {
					GuestCpus: 12,
					MemoryMb:  48000,
					Accelerators: []struct {
						Count int    `json:"guestAcceleratorCount"`
						Type  string `json:"guestAcceleratorType"`
					}{
						{
							Count: 1,
							Type:  "nvidia-l4",
						},
					},
				},
			}

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

type ZoneSelectiveMockMachineTypeClient struct {
	AllowedZone string
	MT          *compute.MachineType
}

func (m *ZoneSelectiveMockMachineTypeClient) GetMachineType(project, zone, machineType string) (*compute.MachineType, error) {
	if zone != m.AllowedZone {
		return nil, fmt.Errorf("machine type not found in zone %s", zone)
	}
	return m.MT, nil
}

func TestFetchMachineCapabilities_NodePoolSpecificZones(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.projectID = "test-project"
	g.clusterZones = []string{"us-central1-a", "us-central1-b"}
	g.clusterDesc.NodePools = []gkeJobNodePool{
		{
			Name: "tpu-np-0",
			Config: gkeNodePoolConfig{
				MachineType: "tpu-v5-lite-podslice",
			},
			Locations: []string{"us-central1-c"},
		},
	}

	g.machineTypeClient = &ZoneSelectiveMockMachineTypeClient{
		AllowedZone: "us-central1-c",
		MT: &compute.MachineType{
			GuestCpus: 4,
			MemoryMb:  16384,
		},
	}

	cap, err := g.FetchMachineCapabilities("tpu-v5-lite-podslice", "us-central1")
	if err != nil {
		t.Fatalf("FetchMachineCapabilities failed: %v", err)
	}

	if cap.GuestCpus != 4 {
		t.Errorf("cap.GuestCpus = %d, want 4", cap.GuestCpus)
	}
}

func TestFetchMachineCapabilities_NoPools_NAPDisabled_Fails(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.projectID = "test-project"
	g.clusterZones = []string{"us-central1-a", "us-central1-b"}
	g.clusterDesc.Autoscaling.EnableNodeAutoprovisioning = false

	_, err := g.FetchMachineCapabilities("tpu-v5-lite-podslice", "us-central1")
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	expectedErr := "no node pool matching machine type found and GKE Node Auto-Provisioning is disabled"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error containing %q, got: %v", expectedErr, err)
	}
}

func TestFetchMachineCapabilities_NoPools_NAPEnabled_Fallback(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.projectID = "test-project"
	g.clusterZones = []string{"us-central1-a", "us-central1-b"}
	g.clusterDesc.Autoscaling.EnableNodeAutoprovisioning = true

	g.machineTypeClient = &ZoneSelectiveMockMachineTypeClient{
		AllowedZone: "us-central1-b",
		MT: &compute.MachineType{
			GuestCpus: 8,
			MemoryMb:  32768,
		},
	}

	cap, err := g.FetchMachineCapabilities("tpu-v5-lite-podslice", "us-central1")
	if err != nil {
		t.Fatalf("FetchMachineCapabilities failed: %v", err)
	}

	if cap.GuestCpus != 8 {
		t.Errorf("cap.GuestCpus = %d, want 8", cap.GuestCpus)
	}
}

func TestFetchMachineCapabilities_NodePoolEmptyLocations_ClusterZonesFallback(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.projectID = "test-project"
	g.clusterZones = []string{"us-central1-b"}
	g.clusterDesc.NodePools = []gkeJobNodePool{
		{
			Name: "tpu-np-0",
			Config: gkeNodePoolConfig{
				MachineType: "tpu-v5-lite-podslice",
			},
			Locations: []string{}, // Empty/inherited locations
		},
	}

	g.machineTypeClient = &ZoneSelectiveMockMachineTypeClient{
		AllowedZone: "us-central1-b",
		MT: &compute.MachineType{
			GuestCpus: 4,
			MemoryMb:  16384,
		},
	}

	cap, err := g.FetchMachineCapabilities("tpu-v5-lite-podslice", "us-central1")
	if err != nil {
		t.Fatalf("FetchMachineCapabilities failed: %v", err)
	}

	if cap.GuestCpus != 4 {
		t.Errorf("cap.GuestCpus = %d, want 4", cap.GuestCpus)
	}
}
