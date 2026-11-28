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
	"errors"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
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
			acceleratorType: "v5p-8",
			wantMachineName: "ct5p-hightpu-4t",
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

// A TPU shorthand names a slice size; the cluster's node pools decide the host shape.
func TestResolveJobMachineType_TPUShorthandFollowsCluster(t *testing.T) {
	tests := []struct {
		name        string
		computeType string
		nodePools   []string
		want        string
		wantErr     string
	}{
		{
			name:        "mapped machine type present in cluster is kept",
			computeType: "v5litepod-8",
			nodePools:   []string{"ct5lp-hightpu-8t", "n2-standard-8"},
			want:        "ct5lp-hightpu-8t",
		},
		{
			name:        "mapped machine type present next to another of its family is kept",
			computeType: "v5litepod-8",
			nodePools:   []string{"ct5lp-hightpu-4t", "ct5lp-hightpu-8t"},
			want:        "ct5lp-hightpu-8t",
		},
		{
			name:        "cluster's machine type of the same family wins",
			computeType: "v5litepod-8",
			nodePools:   []string{"ct5lp-hightpu-4t", "n2-standard-8"},
			want:        "ct5lp-hightpu-4t",
		},
		{
			name:        "v5e alias follows the cluster too",
			computeType: "v5e-8",
			nodePools:   []string{"ct5lp-hightpu-4t"},
			want:        "ct5lp-hightpu-4t",
		},
		{
			name:        "shorthand is case-insensitive",
			computeType: "V5LitePod-8",
			nodePools:   []string{"ct5lp-hightpu-4t"},
			want:        "ct5lp-hightpu-4t",
		},
		{
			name:        "v6e-8 on a 4t cluster",
			computeType: "v6e-8",
			nodePools:   []string{"ct6e-standard-4t"},
			want:        "ct6e-standard-4t",
		},
		{
			name:        "no pools of the family keeps the mapping for node auto-provisioning",
			computeType: "v5litepod-8",
			nodePools:   []string{"n2-standard-8"},
			want:        "ct5lp-hightpu-8t",
		},
		{
			name:        "v5litepod-4 ignores 1t pool and falls back to default 4t for NAP",
			computeType: "v5litepod-4",
			nodePools:   []string{"ct5lp-hightpu-1t"},
			want:        "ct5lp-hightpu-4t",
		},
		{
			name:        "v5litepod-8 ignores 1t pool and falls back to default 8t for NAP",
			computeType: "v5litepod-8",
			nodePools:   []string{"ct5lp-hightpu-1t"},
			want:        "ct5lp-hightpu-8t",
		},
		{
			name:        "v5litepod-8 ignores 1t pool and selects 4t pool",
			computeType: "v5litepod-8",
			nodePools:   []string{"ct5lp-hightpu-1t", "ct5lp-hightpu-4t"},
			want:        "ct5lp-hightpu-4t",
		},
		{
			name:        "v5e-16 cannot run on 8t pool and returns error",
			computeType: "v5e-16",
			nodePools:   []string{"ct5lp-hightpu-8t"},
			wantErr:     "no matching machine types found in cluster",
		},
		{
			name:        "explicit machine type is never substituted",
			computeType: "ct5lp-hightpu-8t",
			nodePools:   []string{"ct5lp-hightpu-4t"},
			want:        "ct5lp-hightpu-8t",
		},
		{
			name:        "GPU shorthand is unaffected",
			computeType: "l4-1",
			nodePools:   []string{"g2-standard-48"},
			want:        "g2-standard-12",
		},
		{
			name:        "unmapped multi-host shorthand resolves to cluster pool",
			computeType: "v5e-16",
			nodePools:   []string{"ct5lp-hightpu-4t"},
			want:        "ct5lp-hightpu-4t",
		},
		{
			name:        "unmapped shorthand with no matching pool returns error",
			computeType: "v5e-16",
			nodePools:   []string{"n2-standard-8"},
			wantErr:     "no matching machine types found in cluster",
		},
		{
			name:        "larger host shape in cluster is not substituted for smaller slice",
			computeType: "v5e-4",
			nodePools:   []string{"ct5lp-hightpu-8t"},
			want:        "ct5lp-hightpu-4t",
		},
		{
			name:        "1-chip slice ignores 4t and 8t pools in cluster",
			computeType: "v5e-1",
			nodePools:   []string{"ct5lp-hightpu-4t", "ct5lp-hightpu-8t"},
			want:        "ct5lp-hightpu-1t",
		},
		{
			name:        "unknown shorthand returns error",
			computeType: "unknown-8",
			nodePools:   []string{"n2-standard-8"},
			wantErr:     "is not a known shorthand",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestGKEOrchestrator(nil)
			for _, mt := range tt.nodePools {
				g.clusterDesc.NodePools = append(g.clusterDesc.NodePools, gkeJobNodePool{Config: gkeNodePoolConfig{MachineType: mt}})
			}

			got, err := g.resolveJobMachineType(tt.computeType, "", "", false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveJobMachineType() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveJobMachineType() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveJobMachineType() = %q, want %q", got, tt.want)
			}
		})
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

	expectedErrStr := "failed to fetch machine capabilities for tpu7x-1: tried in all candidate zones [europe-west2-a europe-west2-c europe-west2-b] but did not find machine type in any of them: resource not found"
	if err.Error() != expectedErrStr {
		t.Errorf("Expected error %q, got %q", expectedErrStr, err.Error())
	}
}

type funcMachineTypeClient func(project, zone, machineType string) (*compute.MachineType, error)

func (f funcMachineTypeClient) GetMachineType(project, zone, machineType string) (*compute.MachineType, error) {
	return f(project, zone, machineType)
}

func TestFetchMachineCapabilities_NilGuardsAndWrappedErr(t *testing.T) {
	t.Parallel()
	t.Run("nil machineTypeClient returns error without panic", func(t *testing.T) {
		t.Parallel()
		g := newTestGKEOrchestrator(nil)
		g.machineTypeClient = nil
		if _, err := g.FetchMachineCapabilities("n2-standard-32", "us-central1-a"); err == nil || !strings.Contains(err.Error(), "machine type client is not initialized") {
			t.Fatalf("FetchMachineCapabilities(nil client) err = %v, want client not initialized", err)
		}
	})
	t.Run("nil MachineType response returns error without panic", func(t *testing.T) {
		t.Parallel()
		g := newTestGKEOrchestrator(nil)
		g.machineTypeClient = funcMachineTypeClient(func(string, string, string) (*compute.MachineType, error) {
			return nil, nil
		})
		if _, err := g.FetchMachineCapabilities("n2-standard-32", "us-central1-a"); err == nil || !strings.Contains(err.Error(), "empty machine type response") {
			t.Fatalf("FetchMachineCapabilities(nil MT) err = %v, want empty response error", err)
		}
	})
	t.Run("regional all-zones failure wraps underlying client error", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("403 quota exceeded")
		g := newTestGKEOrchestrator(nil)
		g.clusterZones = []string{"us-central1-a", "us-central1-b"}
		g.clusterDesc.Autoscaling.EnableNodeAutoprovisioning = true
		g.machineTypeClient = funcMachineTypeClient(func(string, string, string) (*compute.MachineType, error) {
			return nil, sentinel
		})
		_, err := g.FetchMachineCapabilities("n2-standard-32", "us-central1")
		if !errors.Is(err, sentinel) {
			t.Fatalf("FetchMachineCapabilities(regional failure) err = %v, want wrapped %v", err, sentinel)
		}
	})
	t.Run("regional lookup honors napEnabled even when EnableNodeAutoprovisioning is unset", func(t *testing.T) {
		t.Parallel()
		g := newTestGKEOrchestrator(nil)
		g.napEnabled = true
		g.clusterZones = []string{"us-central1-a", "us-central1-b"}
		g.machineTypeClient = funcMachineTypeClient(func(_, zone, mt string) (*compute.MachineType, error) {
			if zone == "us-central1-b" {
				return &compute.MachineType{Name: mt, GuestCpus: 32, MemoryMb: 131072}, nil
			}
			return nil, errors.New("not in zone a")
		})
		cap, err := g.FetchMachineCapabilities("n2-standard-32", "us-central1")
		if err != nil || cap.GuestCpus != 32 || cap.MemoryMb != 131072 {
			t.Fatalf("FetchMachineCapabilities(regional napEnabled) = (%+v, %v), want 32 vCPU / 131072 MiB", cap, err)
		}
	})
}

func TestFetchMachineCapabilities_RegionalAutoprovisioningLocations(t *testing.T) {
	t.Parallel()
	g := newTestGKEOrchestrator(nil)
	g.napEnabled = true
	g.clusterZones = []string{"us-central1-a"}
	g.clusterDesc.Autoscaling.AutoprovisioningLocations = []string{"us-central1-c"}
	g.machineTypeClient = funcMachineTypeClient(func(_, zone, mt string) (*compute.MachineType, error) {
		if zone == "us-central1-c" {
			return &compute.MachineType{Name: mt, GuestCpus: 32, MemoryMb: 131072}, nil
		}
		return nil, errors.New("not in zone")
	})
	cap, err := g.FetchMachineCapabilities("n2-standard-32", "us-central1")
	if err != nil || cap.GuestCpus != 32 {
		t.Fatalf("FetchMachineCapabilities(autoprovisioningLocations) = (%+v, %v), want 32 vCPU", cap, err)
	}

	emptyG := newTestGKEOrchestrator(nil)
	emptyG.napEnabled = true
	if _, err := emptyG.FetchMachineCapabilities("n2-standard-32", "us-central1"); err == nil || !strings.Contains(err.Error(), "no candidate zones") {
		t.Fatalf("FetchMachineCapabilities(empty regional zones) err = %v, want no candidate zones error", err)
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
		name           string
		machineType    string
		threadsPerCore string
		capacityCount  int
		gcsFuse        bool
		wantAlloc      int64
		wantCPU        string
	}{
		{
			name:          "Large capacity (32 vCPU)",
			capacityCount: 32,
			wantAlloc:     31850, // 32000 - (60 + 10 + 10 + 28*2.5)
			wantCPU:       "30850m",
		},
		{
			name:          "Large capacity (32 vCPU) with GCSFuse",
			capacityCount: 32,
			gcsFuse:       true,
			wantAlloc:     31850,
			wantCPU:       "30600m",
		},
		{
			name:          "Medium capacity (4 vCPU)",
			capacityCount: 4,
			wantAlloc:     3920, // 4000 - (60 + 10 + 10)
			wantCPU:       "2920m",
		},
		{
			name:          "Medium capacity (4 vCPU) with GCSFuse",
			capacityCount: 4,
			gcsFuse:       true,
			wantAlloc:     3920,
			wantCPU:       "2670m",
		},
		{
			name:          "3 vCPU capacity",
			capacityCount: 3,
			wantAlloc:     2925, // 3000 - (60 + 10 + 5)
			wantCPU:       "1925m",
		},
		{
			name:          "Small capacity (2 vCPU)",
			capacityCount: 2,
			wantAlloc:     1930,    // 2000 - (60 + 10)
			wantCPU:       "1065m", // 1930/2 + 100
		},
		{
			name:          "Small capacity (2 vCPU) with GCSFuse",
			capacityCount: 2,
			gcsFuse:       true,
			wantAlloc:     1930,
			wantCPU:       "815m", // 1065 - 250
		},
		{
			name:          "Shared-core machine type (e2-medium, raw guestCpus=2 -> 1 effective vCPU)",
			machineType:   "e2-medium",
			capacityCount: 2,
			wantAlloc:     1930,
			wantCPU:       "570m", // 940/2 + 100
		},
		{
			name:          "Shared-core machine type (e2-medium) with GCSFuse",
			machineType:   "e2-medium",
			capacityCount: 2,
			gcsFuse:       true,
			wantAlloc:     1930,
			wantCPU:       "320m", // 570 - 250
		},
		{
			name:           "SMT disabled (threads_per_core=1 on n2-standard-4, 4 -> 2 effective vCPUs)",
			machineType:    "n2-standard-4",
			threadsPerCore: "1",
			capacityCount:  4,
			wantAlloc:      3920,
			wantCPU:        "1065m", // 2 effective vCPUs -> 1930/2 + 100
		},
		{
			name:           "SMT disabled (threads_per_core=1 on n2-standard-4) with GCSFuse",
			machineType:    "n2-standard-4",
			threadsPerCore: "1",
			capacityCount:  4,
			gcsFuse:        true,
			wantAlloc:      3920,
			wantCPU:        "815m", // 1065 - 250
		},
		{
			name:           "Non-SMT-capable family (t2d-standard-4) ignores threads_per_core=1",
			machineType:    "t2d-standard-4",
			threadsPerCore: "1",
			capacityCount:  4,
			wantAlloc:      3920,
			wantCPU:        "2920m",
		},
		{
			name:          "Capacity 1",
			capacityCount: 1,
			wantAlloc:     940,    // 1000 - 60
			wantCPU:       "570m", // 940/2 + 100
		},
		{
			name:          "Capacity 1 with GCSFuse",
			capacityCount: 1,
			gcsFuse:       true,
			wantAlloc:     940,
			wantCPU:       "320m", // 570 - 250
		},
		{
			name:          "Capacity 0 (floors to 100m)",
			capacityCount: 0,
			wantAlloc:     0,
			wantCPU:       "100m",
		},
		{
			name:          "Capacity 0 with GCSFuse (floors to 100m)",
			capacityCount: 0,
			gcsFuse:       true,
			wantAlloc:     0,
			wantCPU:       "100m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotAlloc := calculateAllocatableCPUMilli(tt.capacityCount); gotAlloc != tt.wantAlloc {
				t.Errorf("calculateAllocatableCPUMilli(%d) = %d, want %d", tt.capacityCount, gotAlloc, tt.wantAlloc)
			}

			g := newTestGKEOrchestrator(nil)
			mt := tt.machineType
			if mt == "" {
				mt = "n2-standard-32"
			}
			if tt.threadsPerCore != "" {
				g.machineTypeToThreadsPerCore = map[string]string{mt: tt.threadsPerCore}
			}

			opts := ManifestOptions{
				ComputeType:    mt,
				MachineType:    mt,
				GCSFuseEnabled: tt.gcsFuse,
			}
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

func injectDefaultMocksForShorthand(mockResponses map[string][]shell.CommandResult) {
	if _, ok := mockResponses["kubectl get resourceflavors.kueue.x-k8s.io"]; !ok {
		mockResponses["kubectl get resourceflavors.kueue.x-k8s.io"] = []shell.CommandResult{
			{ExitCode: 0, Stdout: ""},
			{ExitCode: 0, Stdout: ""},
		}
	}
	if _, ok := mockResponses["kubectl get nodes -o jsonpath"]; !ok {
		mockResponses["kubectl get nodes -o jsonpath"] = []shell.CommandResult{
			{ExitCode: 0, Stdout: "16x16\n"},
			{ExitCode: 0, Stdout: "16x16\n"},
		}
	}
	if _, ok := mockResponses["kubectl get topologies.kueue.x-k8s.io -o json"]; !ok {
		mockResponses["kubectl get topologies.kueue.x-k8s.io -o json"] = []shell.CommandResult{
			{ExitCode: 0, Stdout: `{"items":[{"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-4x8-id"}]}}]}`},
			{ExitCode: 0, Stdout: `{"items":[{"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-4x8-id"}]}}]}`},
		}
	}
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
			nodePools:       []string{"ct6e-standard-4t"},
			wantType:        "ct6e-standard-4t", // Resolves to full machine type
			wantTopology:    "16x16",
			wantErr:         false,
		},
		{
			name:            "v5litepod-8 uses the cluster's 4t hosts for a 2x4 slice",
			acceleratorType: "v5litepod-8",
			nodePools:       []string{"ct5lp-hightpu-4t"},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get nodes -o jsonpath": {{ExitCode: 0, Stdout: "2x4\n"}, {ExitCode: 0, Stdout: "2x4\n"}},
			},
			wantType:     "ct5lp-hightpu-4t",
			wantTopology: "2x4",
		},
		{
			name:            "v5e-8 alias resolves like v5litepod-8",
			acceleratorType: "v5e-8",
			nodePools:       []string{"ct5lp-hightpu-4t"},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get nodes -o jsonpath": {{ExitCode: 0, Stdout: "2x4\n"}, {ExitCode: 0, Stdout: "2x4\n"}},
			},
			wantType:     "ct5lp-hightpu-4t",
			wantTopology: "2x4",
		},
		{
			name:            "v5e-16 resolves through the cluster's machine type",
			acceleratorType: "v5e-16",
			nodePools:       []string{"ct5lp-hightpu-4t"},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get nodes -o jsonpath": {{ExitCode: 0, Stdout: "4x4\n"}, {ExitCode: 0, Stdout: "4x4\n"}},
			},
			wantType:     "ct5lp-hightpu-4t",
			wantTopology: "4x4",
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
			nodePools:       []string{"ct6e-standard-4t"},
			wantType:        "ct6e-standard-4t",
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
			injectDefaultMocksForShorthand(mockResponses)

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
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-2x2-id"}]}}]}`},
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
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-4x4-id"}]}}]}`},
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
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-4x4-id"}]}}]}`},
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
					{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-2x2-id"}]}}]}`},
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

			got, err := orc.verifyStaticSlicingActive(tt.machineType, tt.requestedTopo)
			if (err != nil) != tt.wantErr {
				t.Fatalf("verifyStaticSlicingActive() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantActive {
				t.Errorf("verifyStaticSlicingActive() = %v, want %v", got, tt.wantActive)
			}

			if tt.verifyCacheHit && err == nil && got == tt.wantActive {
				// Clear mock executor to ensure subsequent call is satisfied entirely from cache
				orc.executor = NewMockExecutor(nil)
				got2, err2 := orc.verifyStaticSlicingActive(tt.machineType, tt.requestedTopo)
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
				"kubectl get topologies.kueue.x-k8s.io": {
					{ExitCode: 0, Stdout: `{"items": [{"metadata":{"name":"tpu-v6e-slice"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-2x4-id"}]}}]}`},
					{ExitCode: 0, Stdout: `{"items": [{"metadata":{"name":"tpu-v6e-slice"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-2x4-id"}]}}]}`},
					{ExitCode: 0, Stdout: `{"items": [{"metadata":{"name":"tpu-v6e-slice"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-2x4-id"}]}}]}`},
				},
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes":           {{ExitCode: 0, Stdout: "4x8\n"}},
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

func TestCheckNodePoolsDynamicSlicing_PolicyLookupAndCaching(t *testing.T) {
	tests := []struct {
		name          string
		opts          ManifestOptions
		nodePools     []gkeJobNodePool
		mockResponses map[string][]shell.CommandResult
		wantResult    bool
		wantErr       bool
	}{
		{
			name: "Success - Dynamic slicing active via PolicyName lookup",
			opts: ManifestOptions{
				ClusterLocation: "us-central1-a",
				ProjectID:       "cloud-tpu-dev",
				Topology:        "2x2x2",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "np-0",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName: "projects/12345/regions/us-central1/resourcePolicies/tpu7x-policy",
					},
				},
				{
					Name: "np-1",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName: "projects/12345/regions/us-central1/resourcePolicies/tpu7x-policy",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-policy --region=us-central1 --project=cloud-tpu-dev --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-policy","workloadPolicy":{"acceleratorTopologyMode":"PROVISION_ONLY"}}`},
				},
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "Success - Dynamic slicing active with mixed policyName formats (full path vs short name)",
			opts: ManifestOptions{
				ClusterLocation: "us-central1-a",
				ProjectID:       "cloud-tpu-dev",
				Topology:        "2x2x2",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "np-0",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName: "projects/12345/regions/us-central1/resourcePolicies/tpu7x-policy",
					},
				},
				{
					Name: "np-1",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName: "tpu7x-policy",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-policy --region=us-central1 --project=cloud-tpu-dev --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-policy","workloadPolicy":{"acceleratorTopologyMode":"PROVISION_ONLY"}}`},
				},
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "Success - Permission denied reading policy logs warning and assumes non-dynamic slicing without failing",
			opts: ManifestOptions{
				ClusterLocation: "us-central1-a",
				ProjectID:       "cloud-tpu-dev",
				Topology:        "2x2x2",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "np-0",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName: "restricted-policy",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe restricted-policy --region=us-central1 --project=cloud-tpu-dev --format=json": {
					{ExitCode: 1, Stderr: "ERROR: (gcloud.compute.resource-policies.describe) Some requests did not succeed: - Required 'compute.resourcePolicies.get' permission for '...'"},
				},
			},
			wantResult: false,
			wantErr:    false,
		},
		{
			name: "Failure - Policy lookup command fails with actionable error",
			opts: ManifestOptions{
				ClusterLocation: "us-central1-a",
				ProjectID:       "cloud-tpu-dev",
				Topology:        "2x2x2",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "np-0",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName: "invalid-policy",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe invalid-policy --region=us-central1 --project=cloud-tpu-dev --format=json": {
					{ExitCode: 1, Stderr: "ERROR: (gcloud.compute.resource-policies.describe) Internal error occurred"},
				},
			},
			wantResult: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			g := newTestGKEOrchestrator(mockExecutor)
			g.projectID = tt.opts.ProjectID
			g.clusterDesc.NodePools = tt.nodePools

			got, err := g.checkNodePoolsDynamicSlicing("tpu7x-standard-4t", tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkNodePoolsDynamicSlicing() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantResult {
				t.Errorf("checkNodePoolsDynamicSlicing() = %v, want %v", got, tt.wantResult)
			}
			if tt.wantResult && len(g.resourcePolicyCache) == 0 {
				t.Errorf("expected resourcePolicyCache to be populated")
			}
		})
	}
}

func TestParseResourcePolicyURI(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPolicy  string
		wantProject string
		wantRegion  string
	}{
		{
			name:       "Plain policy name",
			input:      "my-policy",
			wantPolicy: "my-policy",
		},
		{
			name:        "Relative resource URI",
			input:       "projects/my-project/regions/us-central1/resourcePolicies/my-policy",
			wantPolicy:  "my-policy",
			wantProject: "my-project",
			wantRegion:  "us-central1",
		},
		{
			name:        "Full HTTPS URL with compute/v1",
			input:       "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/resourcePolicies/my-policy",
			wantPolicy:  "my-policy",
			wantProject: "my-project",
			wantRegion:  "us-central1",
		},
		{
			name:        "Relative resource URI with trailing slash",
			input:       "projects/my-project/regions/us-central1/resourcePolicies/my-policy/",
			wantPolicy:  "my-policy",
			wantProject: "my-project",
			wantRegion:  "us-central1",
		},
		{
			name:        "Full HTTPS URL with subdomain compute.googleapis.com",
			input:       "https://compute.googleapis.com/compute/v1/projects/prod-project/regions/europe-west4/resourcePolicies/prod-policy",
			wantPolicy:  "prod-policy",
			wantProject: "prod-project",
			wantRegion:  "europe-west4",
		},
		{
			name:        "Incomplete resource URI ending at resourcePolicies with trailing slash",
			input:       "projects/my-project/regions/us-central1/resourcePolicies/",
			wantPolicy:  "",
			wantProject: "my-project",
			wantRegion:  "us-central1",
		},
		{
			name:        "Incomplete resource URI ending at resourcePolicies without trailing slash",
			input:       "projects/my-project/regions/us-central1/resourcePolicies",
			wantPolicy:  "",
			wantProject: "my-project",
			wantRegion:  "us-central1",
		},
		{
			name:        "Resource policy named resourcepolicies-xyz in full URI",
			input:       "projects/my-project/regions/us-central1/resourcePolicies/resourcepolicies-xyz",
			wantPolicy:  "resourcepolicies-xyz",
			wantProject: "my-project",
			wantRegion:  "us-central1",
		},
		{
			name:        "Non-canonical path without resourcePolicies collection returns empty policy name",
			input:       "custom/resourcepolicies-xyz",
			wantPolicy:  "",
			wantProject: "",
			wantRegion:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResourcePolicyURI(tt.input)
			if got.Name != tt.wantPolicy {
				t.Errorf("parseResourcePolicyURI(%q).Name = %q, want %q", tt.input, got.Name, tt.wantPolicy)
			}
			if got.Project != tt.wantProject {
				t.Errorf("parseResourcePolicyURI(%q).Project = %q, want %q", tt.input, got.Project, tt.wantProject)
			}
			if got.Region != tt.wantRegion {
				t.Errorf("parseResourcePolicyURI(%q).Region = %q, want %q", tt.input, got.Region, tt.wantRegion)
			}
		})
	}
}

func TestResolveWorkloadPlacementPolicy(t *testing.T) {
	tests := []struct {
		name          string
		job           *orchestrator.JobDefinition
		isTPU7x       bool
		mockResponses map[string][]shell.CommandResult
		wantPolicy    string
		wantErr       bool
	}{
		{
			name: "Placement policy already specified on job is verified and preserved",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "user-specified-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe user-specified-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"user-specified-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
				},
			},
			wantPolicy: "user-specified-policy",
		},
		{
			name: "Placement policy specified as relative URI is sanitized and verified",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "projects/my-project/regions/us-central1/resourcePolicies/user-specified-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe user-specified-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"user-specified-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
				},
			},
			wantPolicy: "user-specified-policy",
		},
		{
			name: "Placement policy specified as HTTPS URL is sanitized and verified",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "https://www.googleapis.com/compute/v1/projects/my-project/regions/us-central1/resourcePolicies/user-specified-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe user-specified-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"user-specified-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
				},
			},
			wantPolicy: "user-specified-policy",
		},
		{
			name: "Placement policy with URI in dry-run mode is sanitized without GCE call",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				DryRunManifest:  "/tmp/job.yaml",
				PlacementPolicy: "projects/my-project/regions/us-central1/resourcePolicies/user-specified-policy",
			},
			isTPU7x:    true,
			wantPolicy: "user-specified-policy",
		},
		{
			name: "Non-TPU workload sanitizes placement policy URI without GCE query",
			job: &orchestrator.JobDefinition{
				MachineType:     "a3-highgpu-8g",
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "projects/my-project/regions/us-central1/resourcePolicies/gpu-policy",
			},
			isTPU7x:    false,
			wantPolicy: "gpu-policy",
		},
		{
			name: "Placement policy project mismatch returns error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "projects/other-project/regions/us-central1/resourcePolicies/user-specified-policy",
			},
			isTPU7x: true,
			wantErr: true,
		},
		{
			name: "Placement policy region mismatch returns error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "projects/my-project/regions/europe-west4/resourcePolicies/user-specified-policy",
			},
			isTPU7x: true,
			wantErr: true,
		},
		{
			name: "Placement policy not found (404) returns error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "missing-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe missing-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 1, Stderr: "ERROR: (gcloud.compute.resource-policies.describe) The resource 'projects/my-project/regions/us-central1/resourcePolicies/missing-policy' was not found"},
				},
			},
			wantErr: true,
		},
		{
			name: "Placement policy topology mismatch returns error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "mismatched-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe mismatched-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"mismatched-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x4"}}`},
				},
			},
			wantErr: true,
		},
		{
			name: "Placement policy PROVISION_ONLY mode on static workload returns error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "dynamic-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe dynamic-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"dynamic-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2","acceleratorTopologyMode":"PROVISION_ONLY"}}`},
				},
			},
			wantErr: true,
		},
		{
			name: "Placement policy invalid type returns error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "group-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe group-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"group-policy","region":"us-central1","workloadPolicy":{"type":"GROUP_PLACEMENT","acceleratorTopology":"2x2x2"}}`},
				},
			},
			wantErr: true,
		},
		{
			name: "Placement policy permission denied (403) warns and proceeds",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "forbidden-policy",
			},
			isTPU7x: true,
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe forbidden-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 1, Stderr: "ERROR: (gcloud.compute.resource-policies.describe) 403 Forbidden: Required 'compute.resourcePolicies.get' permission"},
				},
			},
			wantPolicy: "forbidden-policy",
		},
		{
			name: "Incomplete placement policy URI missing name returns error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				PlacementPolicy: "projects/my-project/regions/us-central1/resourcePolicies/",
			},
			isTPU7x: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			g := newTestGKEOrchestrator(mockExecutor)
			g.projectID = tt.job.ProjectID

			err := g.resolveWorkloadPlacementPolicy(tt.job, tt.isTPU7x, false)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveWorkloadPlacementPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.job.PlacementPolicy != tt.wantPolicy {
				t.Errorf("resolveWorkloadPlacementPolicy() placementPolicy = %q, want %q", tt.job.PlacementPolicy, tt.wantPolicy)
			}
		})
	}
}

func TestResolveGPUPlacementPolicy(t *testing.T) {
	a4xPool := gkeJobNodePool{
		Name:            "a4x-pool",
		Config:          gkeNodePoolConfig{MachineType: "a4x-highgpu-4g"},
		PlacementPolicy: &gkePlacementPolicy{PolicyName: "projects/my-project/regions/us-central1/resourcePolicies/a4x-workload-policy"},
	}
	a4xDescribe := "gcloud compute resource-policies describe a4x-workload-policy --region=us-central1 --project=my-project --format=json"

	tests := []struct {
		name          string
		job           *orchestrator.JobDefinition
		nodePools     []gkeJobNodePool
		mockResponses map[string][]shell.CommandResult
		wantPolicy    string
		wantErr       bool
		wantCreated   bool
	}{
		{
			name:       "non-workload-policy GPU without flag resolves nothing",
			job:        &orchestrator.JobDefinition{MachineType: "a3-ultragpu-8g", ClusterLocation: "us-central1-c", ProjectID: "my-project"},
			nodePools:  []gkeJobNodePool{a4xPool},
			wantPolicy: "",
		},
		{
			name:       "non-workload-policy GPU with explicit policy is sanitized but not described",
			job:        &orchestrator.JobDefinition{MachineType: "a3-highgpu-8g", ClusterLocation: "us-central1-c", ProjectID: "my-project", PlacementPolicy: "projects/my-project/regions/us-central1/resourcePolicies/h100-compact"},
			wantPolicy: "h100-compact",
		},
		{
			name:       "A4X static cluster picks the policy attached to its node pool",
			job:        &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project"},
			nodePools:  []gkeJobNodePool{{Name: "cpu", Config: gkeNodePoolConfig{MachineType: "n2-standard-8"}}, a4xPool},
			wantPolicy: "a4x-workload-policy",
		},
		{
			name:       "A4X static cluster without a policy-bearing node pool warns and resolves nothing",
			job:        &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project"},
			nodePools:  []gkeJobNodePool{{Name: "a4x-nopolicy", Config: gkeNodePoolConfig{MachineType: "a4x-highgpu-4g"}}},
			wantPolicy: "",
		},
		{
			name: "A4X explicit policy is validated for HIGH_THROUGHPUT 1x72",
			job:  &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project", PlacementPolicy: "a4x-workload-policy"},
			mockResponses: map[string][]shell.CommandResult{
				a4xDescribe: {{ExitCode: 0, Stdout: `{"name":"a4x-workload-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"1x72"}}`}},
			},
			wantPolicy: "a4x-workload-policy",
		},
		{
			name: "A4X explicit policy with wrong topology is rejected",
			job:  &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project", PlacementPolicy: "a4x-workload-policy"},
			mockResponses: map[string][]shell.CommandResult{
				a4xDescribe: {{ExitCode: 0, Stdout: `{"name":"a4x-workload-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"1x4"}}`}},
			},
			wantErr: true,
		},
		{
			name: "A4X explicit policy missing in GCE is rejected",
			job:  &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project", PlacementPolicy: "a4x-workload-policy"},
			mockResponses: map[string][]shell.CommandResult{
				a4xDescribe: {{ExitCode: 1, Stderr: "ERROR: The resource 'a4x-workload-policy' was not found"}},
			},
			wantErr: true,
		},
		{
			name:       "A4X explicit policy in dry-run skips GCE validation",
			job:        &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project", PlacementPolicy: "a4x-workload-policy", DryRunManifest: "out.yaml"},
			wantPolicy: "a4x-workload-policy",
		},
		{
			name: "A4X NAP discovers the canonical regional policy",
			job:  &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project", GKENAPProvisioning: "auto"},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe gb200-4-1x72-placement-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"gb200-4-1x72-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"1x72"}}`},
				},
			},
			wantPolicy: "gb200-4-1x72-placement-policy",
		},
		{
			name: "A4X NAP creates the canonical policy when missing",
			job:  &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project", GKENAPProvisioning: "auto"},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe gb200-4-1x72-placement-policy --region=us-central1 --project=my-project --format=json":                                                                                                            {{ExitCode: 1, Stderr: "not found"}},
				"gcloud compute resource-policies list --project=my-project --filter=region:( us-central1 ) AND workloadPolicy.acceleratorTopology=1x72 AND workloadPolicy.type=HIGH_THROUGHPUT --format=value(name,workloadPolicy.acceleratorTopologyMode)": {{ExitCode: 0, Stdout: ""}},
				"gcloud compute resource-policies create workload-policy gb200-4-1x72-placement-policy --region=us-central1 --project=my-project --type=HIGH_THROUGHPUT --accelerator-topology=1x72":                                                         {{ExitCode: 0}},
			},
			wantPolicy:  "gb200-4-1x72-placement-policy",
			wantCreated: true,
		},
		{
			name:       "A4X NAP reuses the policy of an existing A4X node pool before touching GCE",
			job:        &orchestrator.JobDefinition{MachineType: "a4x-highgpu-4g", ClusterLocation: "us-central1-c", ProjectID: "my-project", GKENAPProvisioning: "auto"},
			nodePools:  []gkeJobNodePool{a4xPool},
			wantPolicy: "a4x-workload-policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			g := newTestGKEOrchestrator(mockExecutor)
			g.projectID = tt.job.ProjectID
			g.clusterDesc.NodePools = tt.nodePools

			err := g.resolveGPUPlacementPolicy(tt.job)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveGPUPlacementPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.job.PlacementPolicy != tt.wantPolicy {
				t.Errorf("PlacementPolicy = %q, want %q", tt.job.PlacementPolicy, tt.wantPolicy)
			}
			created := false
			for key, n := range mockExecutor.callCount {
				if n > 0 && strings.Contains(key, "resource-policies create workload-policy") {
					created = true
				}
			}
			if created != tt.wantCreated {
				t.Errorf("workload policy created = %v, want %v (calls: %v)", created, tt.wantCreated, mockExecutor.callCount)
			}
		})
	}
}

func TestVerifyDynamicSlicingActive(t *testing.T) {
	tests := []struct {
		name          string
		opts          ManifestOptions
		nodePools     []gkeJobNodePool
		mockResponses map[string][]shell.CommandResult
		wantResult    bool
		wantErr       bool
	}{
		{
			name: "Success - TPU7x Dynamic-slicing active via PROVISION_ONLY",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        "4x4x4",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"spec":{"controllerName":"accelerator.gke.io/slice"}}]}`},
				},
			},
			wantResult: true,
		},
		{
			name: "Failure - Dynamic-slicing inactive for non-TPU7x via topology subset",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu-v6e-slice",
				Topology:        "2x4",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu-v6e-slice",
						Labels: map[string]string{
							"cloud.google.com/gke-tpu-topology": "4x4",
						},
					},
				},
			},
			mockResponses: nil,
			wantResult:    false,
		},
		{
			name: "Success - TPU7x Dynamic-slicing active via topology subset (static reservation)",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        "4x4x4",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
						Labels: map[string]string{
							"cloud.google.com/gke-tpu-topology": "8x8x8",
						},
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"spec":{"controllerName":"accelerator.gke.io/slice"}}]}`},
				},
			},
			wantResult: false,
		},
		{
			name: "Success - TPU7x Dynamic-slicing requested subslice topology (2x2x1)",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        "2x2x1",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"spec":{"controllerName":"accelerator.gke.io/slice"}}]}`},
				},
			},
			wantResult: true,
			wantErr:    false,
		},
		{
			name: "Failure - TPU7x Dynamic-slicing requested topology 2x4x8 (product 64 but a < 4)",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        "2x4x8",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"spec":{"controllerName":"accelerator.gke.io/slice"}}]}`},
				},
			},
			wantResult: true,
			wantErr:    true,
		},
		{
			name: "Failure - TPU7x Dynamic-slicing requested topology empty",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        "",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"spec":{"controllerName":"accelerator.gke.io/slice"}}]}`},
				},
			},
			wantResult: true,
			wantErr:    true,
		},
		{
			name: "Failure - TPU7x Dynamic-slicing requested topology 2D (4x4)",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        "4x4",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"spec":{"controllerName":"accelerator.gke.io/slice"}}]}`},
				},
			},
			wantResult: true,
			wantErr:    true,
		},
		{
			name: "Failure - Requested topology matches physical topology (not dynamic topology subset)",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu-v6e-slice",
				Topology:        "4x4",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu-v6e-slice",
						Labels: map[string]string{
							"cloud.google.com/gke-tpu-topology": "4x4",
						},
					},
				},
			},
			mockResponses: nil,
			wantResult:    false,
		},
		{
			name: "Failure - No TPU",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "nvidia-l4",
			},
			nodePools:     nil,
			mockResponses: nil,
			wantResult:    false,
		},
		{
			name: "Failure - No matching node pool",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu-v6e-slice",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "other-pool",
					Config: gkeNodePoolConfig{
						MachineType: "other-machine",
					},
				},
			},
			mockResponses: nil,
			wantResult:    false,
			wantErr:       true,
		},
		{
			name: "Failure - CRD not found",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get topologies.kueue.x-k8s.io -o json": {
					{ExitCode: 1},
				},
			},
			wantResult: false,
		},
		{
			name: "Failure - AdmissionCheck missing",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 0, Stdout: `{"items":[{"spec":{"controllerName":"other-controller"}}]}`},
				},
			},
			wantResult: false,
		},
		{
			name: "Failure - AdmissionCheck command fails",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "test-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
					},
					PlacementPolicy: &gkePlacementPolicy{
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl get admissioncheck -o json": {
					{ExitCode: 1, Stderr: "error"},
				},
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockResponses != nil {
				if _, ok := tt.mockResponses["kubectl get topologies.kueue.x-k8s.io -o json"]; !ok {
					tt.mockResponses["kubectl get topologies.kueue.x-k8s.io -o json"] = []shell.CommandResult{
						{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-slice-2x2-id"}]}}]}`},
					}
				}
			}
			mockExecutor := NewMockExecutor(tt.mockResponses)
			orc := newTestGKEOrchestrator(mockExecutor)
			orc.clusterDesc.NodePools = tt.nodePools

			got, err := orc.verifyDynamicSlicingActive(tt.opts)

			if (err != nil) != tt.wantErr {
				t.Errorf("verifyDynamicSlicingActive() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantResult {
				t.Errorf("Expected %t, got %t", tt.wantResult, got)
			}
		})
	}
}

func TestGCETPUHardwareResolver(t *testing.T) {
	t.Parallel()
	r := gceTPUHardwareResolver{}
	for _, tc := range [][2]string{
		{"tpu7", "tpu7x"},
		{"tpu7-8", "tpu7x"},
		{"tpu7-podslice", "tpu7x"},
		{"tpu7x", "tpu7x"},
		{"tpu-7x", "tpu7x"},
		{"tpu-v7x-slice", "tpu7x"},
		{"TPU_V7X", "tpu7x"},
		{"ironwood", "tpu7x"},
		{"tpu7custom", "tpu7custom"},
		{"ct5l-hightpu-4t", "v5e"},
		{"TPU_V5_LITE", "v5e"},
		{"TPU_V4", "v4"},
		{"v6e-8", "v6e"},
		{"TRILLIUM", "v6e"},
		{"custom_accel", "custom_accel"},
	} {
		if got := r.NormalizeTPUType(tc[0]); got != tc[1] {
			t.Errorf("NormalizeTPUType(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"v7x", true},
		{"v7x-1", true},
		{"ironwood", true},
		{"ironwood-1", true},
		{"tpu-7x-1", true},
		{"tpu-v7x-1", true},
		{"tpu7x-standard-4t", true},
		{"v5lite-4", true},
		{"v5-lite-4", true},
		{"custom-node-2t", false},
		{"ctl-plane-0", false},
		{"output-worker", false},
		{"my-tpu-baremetal-host", false},
		{"my-tpu7x-baremetal-host", false},
		{"node-ironwood-01", false},
		{"v4-standard-8", false},
		{"v5e-cpu-worker", false},
		{"v6e-login", false},
		{"tpu7-head", false},
		{"tpu7x-login", false},
		{"ironwood-mgmt", false},
	} {
		if got := r.IsTPUMachine(tc.in); got != tc.want {
			t.Errorf("IsTPUMachine(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	assertGCETPUInferAndChips(t, r)
}

func assertGCETPUInferAndChips(t *testing.T, r gceTPUHardwareResolver) {
	t.Helper()
	for _, tc := range []struct {
		tpuType, rawMachine string
		chips               int
		want                string
	}{
		{"tpu7x", "", 8, "tpu7x-standard-4t"},
		{"tpu7x", "", 1, "tpu7x-standard-1t"},
		{"TPU7X", "", 4, "tpu7x-standard-4t"},
		{"tpu-v7x-slice", "tpu-v7x-slice", 4, "tpu7x-standard-4t"},
		{"tpu7x", "tpu7x-8", 0, "tpu7x-standard-4t"},
		{"tpu7x", "tpu7x-16", 0, "tpu7x-standard-4t"},
		{"tpu7x", "tpu7x-podslice", 0, "tpu7x-standard-4t"},
		{"tpu7x", "tpu7x-standard-1t", 4, "tpu7x-standard-1t"},
		{"", "v7x-1", 0, "tpu7x-standard-1t"},
		{"", "v7x-8", 0, "tpu7x-standard-4t"},
		{"", "ironwood-1", 0, "tpu7x-standard-1t"},
		{"", "tpu-7x-1", 0, "tpu7x-standard-1t"},
		{"", "tpu-v7x-1", 0, "tpu7x-standard-1t"},
		{"", "v6e-8", 0, "ct6e-standard-8t"},
		{"", "tpu-v6e-1", 0, "ct6e-standard-1t"},
		{"", "tpu-v6e-8", 0, "ct6e-standard-8t"},
		{"v5e", "v5e-1", 0, "ct5lp-hightpu-1t"},
		{"v5e", "v5lite-1", 0, "ct5lp-hightpu-1t"},
		{"v5e", "v5lite-4", 0, "ct5lp-hightpu-4t"},
		{"v5e", "v5lite-8", 0, "ct5lp-hightpu-8t"},
		{"v5e", "v5-lite-1", 0, "ct5lp-hightpu-1t"},
		{"v5e", "v5-lite-8", 0, "ct5lp-hightpu-8t"},
		{"", "tpu-v5-lite-1", 0, "ct5lp-hightpu-1t"},
		{"", "tpu-v5-lite-8", 0, "ct5lp-hightpu-8t"},
		{"v5p", "", 4, "ct5p-hightpu-4t"},
		{"v4", "", 4, "ct4p-hightpu-4t"},
		{"", "my-baremetal-host", 0, "my-baremetal-host"},
		{"", "my-tpu-baremetal-host", 0, "my-tpu-baremetal-host"},
		{"", "my-tpu7x-baremetal-host", 0, "my-tpu7x-baremetal-host"},
		{"", "node-ironwood-01", 0, "node-ironwood-01"},
		{"", "v4-standard-8", 0, "v4-standard-8"},
		{"", "v5e-cpu-worker", 0, "v5e-cpu-worker"},
		{"", "v6e-login", 0, "v6e-login"},
		{"", "tpu7-head", 0, "tpu7-head"},
		{"", "tpu7x-login", 0, "tpu7x-login"},
		{"", "ironwood-mgmt", 0, "ironwood-mgmt"},
	} {
		if got := r.InferTPUMachine(tc.tpuType, tc.rawMachine, tc.chips); got != tc.want {
			t.Errorf("InferTPUMachine(%q, %q, %d) = %q, want %q", tc.tpuType, tc.rawMachine, tc.chips, got, tc.want)
		}
	}
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"ct6e-standard-8T", 8},
		{"ct5lp-hightpu", 8},
		{"ct6e-standard", 4},
		{"tpu-4test", 0},
		{"tpu-v7x-2t", 0},
		{"custom-node-2t", 0},
		{"ctl-plane-0", 0},
		{"output-worker", 0},
		{"my-baremetal-host", 0},
		{"my-tpu-baremetal-host", 0},
		{"my-tpu7x-baremetal-host", 0},
		{"node-ironwood-01", 0},
		{"v4-standard-8", 0},
		{"v5e-cpu-worker", 0},
		{"v6e-login", 0},
		{"tpu7-head", 0},
		{"tpu7x-login", 0},
		{"ironwood-mgmt", 0},
		{"v6e-8", 8},
		{"tpu-v6e-1", 1},
		{"tpu-v6e-8", 8},
		{"v5e-1", 1},
		{"v5lite-1", 1},
		{"v5lite-8", 8},
		{"v5-lite-8", 8},
		{"tpu-v5-lite-1", 1},
		{"tpu-v5-lite-8", 8},
		{"v7x-1", 1},
		{"v7x-8", 4},
		{"ironwood-1", 1},
		{"tpu-7x-1", 1},
		{"tpu-v7x-1", 1},
		{"tpu7x-1", 1},
		{"tpu7-8", 4},
		{"tpu7x-8", 4},
		{"tpu7x-16", 4},
		{"tpu7-podslice", 4},
		{"tpu7x-podslice", 4},
	} {
		if got := r.ChipsPerNode(tc.in); got != tc.want {
			t.Errorf("ChipsPerNode(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestK8sResourceResolution_TPUShorthandDisambiguation(t *testing.T) {
	orc8t := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeTPUSlice,
				TPU:         &k8s.TPUInventory{TPUType: "v6e", MachineType: "ct6e-standard-8t", ChipsPerNode: 8},
			},
		},
	}
	if got := orc8t.resolveK8sMachineType("v6e-16", nil); got != "ct6e-standard-4t" {
		t.Errorf("resolveK8sMachineType(v6e-16 on 8t primary) = %q, want ct6e-standard-4t", got)
	}

	orc4t := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeTPUSlice,
				PoolNames:   []string{"v6e-4t"},
				Pools: map[string]k8s.PoolCapacity{
					"v6e-4t": {MachineType: "ct6e-standard-4t", IsTPU: true, TPUType: "v6e", TPUsPerNode: 4, ReadyNodes: 2},
				},
				TPU: &k8s.TPUInventory{TPUType: "v6e", MachineType: "ct6e-standard-4t", ChipsPerNode: 4},
			},
		},
	}
	if got := orc4t.resolveK8sMachineType("v6e-8", nil); got != "ct6e-standard-4t" {
		t.Errorf("resolveK8sMachineType(v6e-8 on 4t pool) = %q, want ct6e-standard-4t", got)
	}
	if chips := orc4t.resolveK8sTPUChipsPerNode("ct6e-standard-1t", k8s.PoolCapacity{}, false, 0); chips != 1 {
		t.Errorf("resolveK8sTPUChipsPerNode(ct6e-standard-1t) = %d, want 1 chip", chips)
	}
}

func TestK8sResourceResolution_SliceChipsAndMultiHyphenTopology(t *testing.T) {
	job := &orchestrator.JobDefinition{ComputeType: "ct6e-standard-8t", MachineType: "ct6e-standard-8t", Topology: "2x4"}
	slice4t := k8s.SliceInfo{Name: "s-4t", TPUType: "v6e", Topology: "2x4", ChipsPerNode: 4, Ready: true, NodesPerSlice: 2}
	if err := (&GKEOrchestrator{}).validateSliceHardwareMatch(job, "s-4t", slice4t); err == nil || !strings.Contains(err.Error(), "chips/node") {
		t.Errorf("validateSliceHardwareMatch(8t vs 4t slice) err = %v, want chips/node mismatch", err)
	}
	orc := &GKEOrchestrator{mode: orchestrator.TypeK8s}
	for _, tc := range []struct {
		ct, mt, want string
	}{
		{"tpu-v6e-8", "ct6e-standard-4t", "2x4"},
		{"v5-lite-8", "ct5lp-hightpu-4t", "2x4"},
	} {
		got, err := orc.resolveK8sTPUTopology(&orchestrator.JobDefinition{ComputeType: tc.ct, MachineType: tc.mt})
		if err != nil || got != tc.want {
			t.Errorf("resolveK8sTPUTopology(%s, %s) = (%q, %v), want (%q, nil)", tc.ct, tc.mt, got, err, tc.want)
		}
	}
	if _, err := orc.resolveK8sTPUTopology(&orchestrator.JobDefinition{ComputeType: "tpu-v6e-7", MachineType: "ct6e-standard-4t"}); err == nil {
		t.Errorf("resolveK8sTPUTopology(invalid shorthand tpu-v6e-7) = nil, want error")
	}
}

func TestK8sResourceResolution_SuffixesAndExplicitCPUFallback(t *testing.T) {
	for _, tc := range []struct {
		mt   string
		want int
	}{
		{"a3-ultragpu-8g-nolocalssd", 8},
		{"a4-highgpu-8g-nolocalssd", 8},
		{"a3-highgpu-8g-metal", 8},
		{"a4x-highgpu-4g-nolssd", 4},
		{"g4-standard-384-nolocalssd", 8},
		{"g2-custom-48-196608", 4},
	} {
		got, ok := inferFallbackGPUCount(tc.mt)
		if !ok || got != tc.want {
			t.Errorf("inferFallbackGPUCount(%q) = (%d, %v), want (%d, true)", tc.mt, got, ok, tc.want)
		}
	}
	for _, tc := range []struct {
		mt   string
		want int
	}{
		{"n2-standard-32", 32},
		{"n2-custom-16-65536", 16},
		{"kind-worker-1", 0},
	} {
		if got := inferFallbackGCECPUCount(tc.mt); got != tc.want {
			t.Errorf("inferFallbackGCECPUCount(%q) = %d, want %d", tc.mt, got, tc.want)
		}
	}
}

func TestK8sResourceResolution_GPUResourceNameAndTPUNumNodesGuard(t *testing.T) {
	orc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"intel-pool"},
				Pools: map[string]k8s.PoolCapacity{
					"intel-pool": {MachineType: "intel-gpu", IsGPU: true, GPUsPerNode: 2, GPUType: "gpu.intel.com/i915", GPUResourceName: "gpu.intel.com/i915", ReadyNodes: 1},
				},
			},
		},
	}
	gpuJob := orchestrator.JobDefinition{ComputeType: "gpu.intel.com/i915", NumSlices: 1}
	prof, _, _, err := orc.resolveK8sHardwareRequirements(&gpuJob)
	if err != nil || prof.GPUResourceName != "gpu.intel.com/i915" || prof.CapacityCount != 2 {
		t.Fatalf("intel GPU profile = (%+v, %v), want 2 of gpu.intel.com/i915", prof, err)
	}

	tpuOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeTPUSlice,
				TPU:         &k8s.TPUInventory{TPUType: "v6e", MachineType: "ct6e-standard-4t", ChipsPerNode: 4, Topology: "2x2"},
			},
		},
	}
	for _, ct := range []string{"", "trillium"} {
		job := orchestrator.JobDefinition{ComputeType: ct, NumSlices: 1, NodesPerSlice: 2, NumNodesSpecified: true}
		if _, _, _, err := tpuOrc.resolveK8sHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), "--num-nodes cannot be used with TPU jobs") {
			t.Fatalf("TPU computeType=%q with --num-nodes err = %v, want rejection", ct, err)
		}
	}
}

func TestK8sParallelContainersDisabledOnTypeK8sAndSingleChip(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		mode         orchestrator.Type
		machineType  string
		chips        int
		wantParallel int
		wantTPU      string
	}{
		{"k8s_tpu7x_1t", orchestrator.TypeK8s, "tpu7x-standard-1t", 1, 1, "1"},
		{"k8s_tpu7x_4t", orchestrator.TypeK8s, "tpu7x-standard-4t", 4, 1, "4"},
		{"gke_tpu7x_1t_odd_chip_guard", orchestrator.TypeGKE, "tpu7x-standard-1t", 1, 1, "1"},
		{"gke_tpu7x_4t_parallel_enabled", orchestrator.TypeGKE, "tpu7x-standard-4t", 4, 2, "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orc := &GKEOrchestrator{mode: tc.mode}
			if tc.mode == orchestrator.TypeGKE {
				orc.machineCapCache = map[string]MachineTypeCap{
					tc.machineType + ":": {
						Accelerators: []struct {
							Count int    `json:"guestAcceleratorCount"`
							Type  string `json:"guestAcceleratorType"`
						}{{Count: tc.chips, Type: "tpu7x"}},
					},
				}
			}
			job := orchestrator.JobDefinition{
				MachineType:           tc.machineType,
				UseParallelContainers: true,
			}
			opts := ManifestOptions{
				ComputeType:        tc.machineType,
				MachineType:        tc.machineType,
				ParallelContainers: 1,
			}
			profile := JobProfile{IsTPU: true, CapacityCount: tc.chips}
			orc.applySchedulingGatesAndParallelism(&opts, profile, job)
			resStr, _, _, err := orc.resolveWorkloadResourcesYAML(&opts, profile, 16)
			if err != nil {
				t.Fatalf("resolveWorkloadResourcesYAML() err = %v", err)
			}
			wantTPU := fmt.Sprintf("google.com/tpu: %q", tc.wantTPU)
			if opts.ParallelContainers != tc.wantParallel || !strings.Contains(resStr, wantTPU) {
				t.Errorf("got (ParallelContainers=%d, ResourcesString=%q), want (%d, containing %s)",
					opts.ParallelContainers, resStr, tc.wantParallel, wantTPU)
			}
		})
	}
}

func TestK8sResourceResolution_NonTPUSubstringAndCustomTPUTypes(t *testing.T) {
	t.Parallel()
	orc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"cpu-output", "bm-tpu", "custom-tpu-pool"},
				Pools: map[string]k8s.PoolCapacity{
					"cpu-output":      {MachineType: "output-worker", IsTPU: false, IsGPU: false, ReadyNodes: 2, AllocatableCPUMilli: 16000},
					"bm-tpu":          {MachineType: "bm-host", IsTPU: true, TPUType: "tpu7x", TPUsPerNode: 4, ReadyNodes: 2},
					"custom-tpu-pool": {MachineType: "custom-tpu-node", IsTPU: true, TPUType: "tpu-custom-a", TPUsPerNode: 4, ReadyNodes: 2},
				},
			},
		},
	}

	cpuJob := orchestrator.JobDefinition{ComputeType: "output-worker", NumSlices: 1, NodesPerSlice: 2, NumNodesSpecified: true}
	cpuProf, _, _, err := orc.resolveK8sHardwareRequirements(&cpuJob)
	if err != nil {
		t.Fatalf("output-worker CPU pool rejected as TPU: %v", err)
	}
	if !cpuProf.IsCPUMachine || cpuProf.IsTPU || cpuJob.NodesPerSlice != 2 {
		t.Errorf("output-worker profile = %+v nodes=%d, want CPU with 2 nodes", cpuProf, cpuJob.NodesPerSlice)
	}
	if len(orc.machineCapCache) != 0 {
		t.Errorf("TypeK8s must not write machineCapCache, got %v", orc.machineCapCache)
	}

	bmJob := orchestrator.JobDefinition{ComputeType: "bm-host", Topology: "2x2x2", NumSlices: 1}
	bmProf, _, _, err := orc.resolveK8sHardwareRequirements(&bmJob)
	if err != nil {
		t.Fatalf("bm-host TPU pool err = %v", err)
	}
	if !bmProf.IsTPU || bmProf.CapacityCount != 4 || bmJob.NodesPerSlice != 2 {
		t.Errorf("bm-host profile = %+v nodes=%d, want TPU 4 chips/node, 2 nodes", bmProf, bmJob.NodesPerSlice)
	}

	customTPUJob := orchestrator.JobDefinition{ComputeType: "custom-tpu-node", Topology: "2x2x2", NumSlices: 1}
	customTPUProf, _, _, err := orc.resolveK8sHardwareRequirements(&customTPUJob)
	if err != nil {
		t.Fatalf("custom-tpu-node custom TPU pool err = %v", err)
	}
	if !customTPUProf.IsTPU || customTPUJob.NodesPerSlice != 2 {
		t.Errorf("custom-tpu-node profile = %+v nodes=%d, want TPU with 2 nodes", customTPUProf, customTPUJob.NodesPerSlice)
	}
}

func TestK8sResourceResolution_UnlabeledCPUAndPoolRanking(t *testing.T) {
	t.Parallel()
	// Unlabeled CPU worker (MachineType == "") auto-discovers cleanly without emitting an instance-type selector.
	unlabeledOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				CPUPoolCap:  &k8s.PoolCapacity{MachineType: "", ReadyNodes: 2, AllocatableCPUMilli: 8000},
				PoolNames:   []string{"cpu"},
				Pools: map[string]k8s.PoolCapacity{
					"cpu": {MachineType: "", ReadyNodes: 2, AllocatableCPUMilli: 8000},
				},
			},
		},
	}
	job := orchestrator.JobDefinition{NumSlices: 1}
	prof, _, _, err := unlabeledOrc.resolveK8sHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("unlabeled CPU auto-discovery err = %v", err)
	}
	if !prof.IsCPUMachine || prof.K8sPool == nil {
		t.Errorf("unlabeled CPU profile = %+v, want IsCPUMachine=true with K8sPool", prof)
	}
	sel, err := unlabeledOrc.buildNodeSelectorWithProfile(SchedulingOptions{}, job, prof)
	if err != nil || strings.Contains(sel, k8s.InstanceTypeLabel) {
		t.Errorf("unlabeled CPU nodeSelector = (%q, %v), must omit %s", sel, err, k8s.InstanceTypeLabel)
	}

	// Ready worker pool beats control-plane and unready pools for the same machine type.
	rankOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"a-ctrl", "b-unready", "c-worker"},
				Pools: map[string]k8s.PoolCapacity{
					"a-ctrl":    {MachineType: "n2-standard-8", NodePoolName: "ctrl", IsControlPlane: true, ReadyNodes: 1},
					"b-unready": {MachineType: "n2-standard-8", NodePoolName: "down", ReadyNodes: 0},
					"c-worker":  {MachineType: "n2-standard-8", NodePoolName: "workers", ReadyNodes: 3},
				},
			},
		},
	}
	picked, ok := rankOrc.findK8sPoolForRequest("", "n2-standard-8")
	if !ok || picked.NodePoolName != "workers" {
		t.Errorf("findK8sPoolForRequest = (%+v, %v), want workers pool", picked, ok)
	}

	// Empty PoolNames with inventory.TPU still resolves TPU shorthands.
	tpuFallbackOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeTPUSlice,
				TPU:         &k8s.TPUInventory{TPUType: "v6e", MachineType: "ct6e-standard-4t", ChipsPerNode: 4},
			},
		},
	}
	if got := tpuFallbackOrc.resolveK8sMachineType("v6e-8", nil); got != "ct6e-standard-4t" {
		t.Errorf("resolveK8sMachineType(v6e-8 with empty PoolNames) = %q, want ct6e-standard-4t", got)
	}
}

func TestK8sResourceResolution_ExplicitSliceBareFamilyAndDerivedTopology(t *testing.T) {
	t.Parallel()
	orc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeTPUSlice,
				TPU:         &k8s.TPUInventory{TPUType: "v5p", MachineType: "ct5p-hightpu-4t", ChipsPerNode: 4},
				Slices: []k8s.SliceInfo{
					{Name: "s-v6e-1t", TPUType: "v6e", MachineType: "ct6e-standard-1t", Topology: "1x1", ChipsPerNode: 1, NodesPerSlice: 1, Ready: true},
					{Name: "s-v6e-8t", TPUType: "v6e", Topology: "2x4", ChipsPerNode: 8, NodesPerSlice: 1, Ready: true},
					{Name: "s-derived", TPUType: "v6e", MachineType: "ct6e-standard-4t", ChipsPerNode: 4, NodesPerSlice: 2, Ready: true},
				},
			},
		},
	}

	// Bare family "v6e" adopts the targeted 1t slice's machine type instead of defaulting to 4t.
	job1t := orchestrator.JobDefinition{ComputeType: "v6e", NumSlices: 1, NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-v6e-1t"}}
	prof1t, _, _, err := orc.resolveK8sHardwareRequirements(&job1t)
	if err != nil || job1t.MachineType != "ct6e-standard-1t" || prof1t.CapacityCount != 1 || job1t.Topology != "1x1" {
		t.Fatalf("bare v6e on 1t slice = (mt=%q topo=%q prof=%+v err=%v), want ct6e-standard-1t / 1x1 / 1 chip", job1t.MachineType, job1t.Topology, prof1t, err)
	}

	// Bare family "v6e" on 8t slice (empty MachineType on CRD) infers ct6e-standard-8t.
	job8t := orchestrator.JobDefinition{ComputeType: "v6e", NumSlices: 1, NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-v6e-8t"}}
	prof8t, _, _, err := orc.resolveK8sHardwareRequirements(&job8t)
	if err != nil || job8t.MachineType != "ct6e-standard-8t" || prof8t.CapacityCount != 8 {
		t.Fatalf("bare v6e on 8t slice = (mt=%q prof=%+v err=%v), want ct6e-standard-8t / 8 chips", job8t.MachineType, prof8t, err)
	}

	// Shorthand v6e-16 (4x4) against 2x4 slice must fail with topology mismatch.
	mismatchJob := orchestrator.JobDefinition{ComputeType: "v6e-16", NumSlices: 1, NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-v6e-8t"}}
	if _, _, _, err := orc.resolveK8sHardwareRequirements(&mismatchJob); err == nil || !strings.Contains(err.Error(), "does not match slice") {
		t.Errorf("shorthand v6e-16 vs 2x4 slice err = %v, want mismatch error", err)
	}

	// Slice with empty Topology + NodesPerSlice=2 derives topology ("2x4") instead of leaving job.Topology empty.
	derivedJob := orchestrator.JobDefinition{ComputeType: "v6e", NumSlices: 1, NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-derived"}}
	if _, _, _, err := orc.resolveK8sHardwareRequirements(&derivedJob); err != nil || derivedJob.Topology != "2x4" || derivedJob.NodesPerSlice != 2 {
		t.Fatalf("derived slice topology = (topo=%q nodes=%d err=%v), want (2x4, 2, nil)", derivedJob.Topology, derivedJob.NodesPerSlice, err)
	}
}

func TestK8sResourceResolution_ZeroGPUPoolAndExplicitModeNonNvidiaGPUs(t *testing.T) {
	t.Parallel()
	zeroGPUOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"broken-gpu"},
				Pools: map[string]k8s.PoolCapacity{
					"broken-gpu": {MachineType: "gpu-broken", IsGPU: true, GPUsPerNode: 0, GPUType: "nvidia-l4", ReadyNodes: 1},
				},
			},
		},
	}
	zeroJob := orchestrator.JobDefinition{ComputeType: "gpu-broken", NumSlices: 1}
	if _, _, _, err := zeroGPUOrc.resolveK8sHardwareRequirements(&zeroJob); err == nil || !strings.Contains(err.Error(), "0 allocatable GPUs") {
		t.Fatalf("0-GPU pool err = %v, want 0 allocatable GPUs error", err)
	}

	explicitOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{LabelScheme: k8s.SchemeGeneric, ExplicitMode: true},
		},
	}
	for _, resKey := range []string{"amd.com/gpu", "gpu.intel.com/i915", "nvidia.com/mig-1g.5gb"} {
		job := orchestrator.JobDefinition{ComputeType: resKey, NumSlices: 1}
		prof, _, _, err := explicitOrc.resolveK8sHardwareRequirements(&job)
		if err != nil {
			t.Fatalf("ExplicitMode %s err = %v", resKey, err)
		}
		if prof.IsCPUMachine || prof.GPUResourceName != resKey || prof.CapacityCount != 1 {
			t.Errorf("ExplicitMode %s profile = %+v, want GPU resource=%s count=1", resKey, prof, resKey)
		}
	}
}

func TestK8sRebuttalFixes_V4V5pTopoCustomSliceZeroGPUPoolAndCaseValidation(t *testing.T) {
	t.Parallel()
	// C3: v4 and v5p deriveSliceTopologyFromNodes must multiply chips by 2 (TensorCores).
	for _, tc := range []struct {
		tpuType, mt string
		nodes       int
		want        string
	}{
		{"v5p", "ct5p-hightpu-4t", 1, "2x2x1"},
		{"v5p", "ct5p-hightpu-4t", 2, "2x2x2"},
		{"v4", "ct4p-hightpu-4t", 1, "2x2x1"},
		{"v4", "ct4p-hightpu-4t", 2, "2x2x2"},
	} {
		got := deriveSliceTopologyFromNodes(
			&orchestrator.JobDefinition{MachineType: tc.mt},
			k8s.SliceInfo{TPUType: tc.tpuType, MachineType: tc.mt, ChipsPerNode: 4, NodesPerSlice: tc.nodes},
		)
		if got != tc.want {
			t.Errorf("deriveSliceTopologyFromNodes(%s, %d nodes) = %q, want %q", tc.tpuType, tc.nodes, got, tc.want)
		}
	}

	// C4 + C12 + C14: Custom bm-host TPU slice matches; s.Topology=="" validates against s.NodesPerSlice;
	// ExplicitMode slice ChipsPerNode honored; resolveK8sTPUChipsPerNode respects TPU family guard.
	bmOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeTPUSlice,
				PoolNames:   []string{"bm-tpu"},
				Pools: map[string]k8s.PoolCapacity{
					"bm-tpu": {MachineType: "bm-host", IsTPU: true, TPUType: "tpu7x", TPUsPerNode: 4, ReadyNodes: 2},
				},
				TPU: &k8s.TPUInventory{TPUType: "v6e", MachineType: "ct6e-standard-8t", ChipsPerNode: 8},
				Slices: []k8s.SliceInfo{
					{Name: "s-bm", TPUType: "tpu7x", MachineType: "bm-host", Topology: "2x2x2", ChipsPerNode: 4, NodesPerSlice: 2, Ready: true},
					{Name: "s-no-topo", TPUType: "v6e", MachineType: "ct6e-standard-4t", ChipsPerNode: 4, NodesPerSlice: 2, Ready: true},
					{Name: "s-explicit-8", TPUType: "tpu-custom-a", MachineType: "custom-8", Topology: "2x2x2", ChipsPerNode: 8, NodesPerSlice: 1, Ready: true},
				},
			},
		},
	}
	bmJob := orchestrator.JobDefinition{NumSlices: 1, NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-bm"}}
	if prof, _, _, err := bmOrc.resolveK8sHardwareRequirements(&bmJob); err != nil || !prof.IsTPU || bmJob.NodesPerSlice != 2 {
		t.Fatalf("custom bm-host slice resolution = (prof=%+v nodes=%d err=%v), want TPU 2 nodes", prof, bmJob.NodesPerSlice, err)
	}
	mismatchNoTopo := orchestrator.JobDefinition{ComputeType: "v6e-16", NumSlices: 1, NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-no-topo"}}
	if _, _, _, err := bmOrc.resolveK8sHardwareRequirements(&mismatchNoTopo); err == nil || !strings.Contains(err.Error(), "does not match slice") {
		t.Errorf("v6e-16 (4 nodes) on 2-node empty-topology slice err = %v, want mismatch", err)
	}
	if chips := bmOrc.resolveK8sTPUChipsPerNode("custom-8", k8s.PoolCapacity{}, false, 8); chips != 8 {
		t.Errorf("resolveK8sTPUChipsPerNode(custom-8, sliceChips=8) = %d, want 8", chips)
	}
	if chips := bmOrc.resolveK8sTPUChipsPerNode("ct5p-hightpu", k8s.PoolCapacity{}, false, 0); chips == 8 {
		t.Errorf("resolveK8sTPUChipsPerNode(ct5p-hightpu) leaked v6e 8t inventory: got %d", chips)
	}
	assertZeroGPURankingAndCaseValidation(t)
}

func assertZeroGPURankingAndCaseValidation(t *testing.T) {
	t.Helper()
	// C5 + C6: Healthy 8-GPU pool beats restarting 0-GPU pool; calculateK8sMachineResourceLimits handles empty JobProfile.
	gpuOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"a-restarting-0gpu", "b-healthy-8gpu", "c-bm-tpu", "d-mig-3g"},
				Pools: map[string]k8s.PoolCapacity{
					"a-restarting-0gpu": {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 0, GPUType: "nvidia-h100-80gb", ReadyNodes: 1},
					"b-healthy-8gpu":    {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 8, GPUType: "nvidia-h100-80gb", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 7},
					"c-bm-tpu":          {MachineType: "bm-host", IsTPU: true, TPUType: "tpu7x", TPUsPerNode: 4, ReadyNodes: 2},
					"d-mig-3g":          {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 2, GPUResourceName: "nvidia.com/mig-3g.40gb", ReadyNodes: 2},
				},
			},
		},
	}
	picked, ok := gpuOrc.findK8sPoolForRequest("a3-highgpu-8g", "a3-highgpu-8g")
	if !ok || picked.GPUsPerNode != 8 {
		t.Errorf("findK8sPoolForRequest(a3-highgpu-8g) picked %+v (ok=%v), want 8-GPU pool over 0-GPU restarting pool", picked, ok)
	}
	if _, _, gpu, tpu, err := gpuOrc.calculateK8sMachineResourceLimits(ManifestOptions{MachineType: "bm-host"}, JobProfile{}); err != nil || tpu != "4" || gpu != "" {
		t.Errorf("calculateK8sMachineResourceLimits(bm-host, empty profile) = (gpu=%q tpu=%q err=%v), want tpu=4", gpu, tpu, err)
	}
	if prof, err := gpuOrc.buildK8sJobProfile(&orchestrator.JobDefinition{MachineType: "output-gpu-worker"}, k8s.PoolCapacity{}, false, false); err != nil || prof.IsCPUMachine || prof.CapacityCount != 1 {
		t.Errorf("buildK8sJobProfile(output-gpu-worker) = (%+v, %v), want gpu CapacityCount=1", prof, err)
	}
	if _, _, gpu, _, err := gpuOrc.calculateK8sMachineResourceLimits(ManifestOptions{ComputeType: "nvidia.com/mig-3g.40gb", MachineType: "a3-highgpu-8g"}, JobProfile{}); err != nil || gpu != "2" {
		t.Errorf("calculateK8sMachineResourceLimits(mig-3g.40gb) = (gpu=%q err=%v), want gpu=2", gpu, err)
	}
	assertCaseAndNamedSliceShorthandValidation(t)
}

func assertCaseAndNamedSliceShorthandValidation(t *testing.T) {
	t.Helper()
	// C13: ct5l-hightpu-* and uppercase machine types validate properly.
	for _, tc := range []struct{ mt, topo string }{
		{"ct5l-hightpu-4t", "2x2"},
		{"CT6E-STANDARD-4T", "2x2"},
		{"TPU7X-STANDARD-1T", "1x1x1"},
	} {
		if err := validateK8sTPUTopology(tc.mt, tc.topo); err != nil {
			t.Errorf("validateK8sTPUTopology(%q, %q) unexpected err = %v", tc.mt, tc.topo, err)
		}
	}

	// C15: tpu-v6e-slice on 1t slice and v6e-8 on 4t slice adopt slice host shape.
	sliceOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeTPUSlice,
				Slices: []k8s.SliceInfo{
					{Name: "s-1t", TPUType: "v6e", MachineType: "ct6e-standard-1t", Topology: "1x1", ChipsPerNode: 1, NodesPerSlice: 1, Ready: true},
					{Name: "s-4t", TPUType: "v6e", MachineType: "ct6e-standard-4t", Topology: "2x4", ChipsPerNode: 4, NodesPerSlice: 2, Ready: true},
				},
			},
		},
	}
	if got := sliceOrc.resolveK8sMachineType("tpu-v6e-slice", map[string]string{k8s.TPUSliceNameLabel: "s-1t"}); got != "ct6e-standard-1t" {
		t.Errorf("resolveK8sMachineType(tpu-v6e-slice on s-1t) = %q, want ct6e-standard-1t", got)
	}
	if got := sliceOrc.resolveK8sMachineType("v6e-8", map[string]string{k8s.TPUSliceNameLabel: "s-4t"}); got != "ct6e-standard-4t" {
		t.Errorf("resolveK8sMachineType(v6e-8 on s-4t) = %q, want ct6e-standard-4t", got)
	}
}

func TestResolveParallelContainerCount_GKEAndK8s(t *testing.T) {
	tests := []struct {
		name string
		mode orchestrator.Type
		job  orchestrator.JobDefinition
		want int
	}{
		{
			name: "gke_tpu7x_4t_uses_2_containers",
			mode: orchestrator.TypeGKE,
			job:  orchestrator.JobDefinition{MachineType: "tpu7x-standard-4t", UseParallelContainers: true},
			want: 2,
		},
		{
			name: "gke_tpu7x_1t_uses_1_container",
			mode: orchestrator.TypeGKE,
			job:  orchestrator.JobDefinition{MachineType: "tpu7x-standard-1t", UseParallelContainers: true},
			want: 1,
		},
		{
			name: "gke_tpu7x_4t_disabled_uses_1_container",
			mode: orchestrator.TypeGKE,
			job:  orchestrator.JobDefinition{MachineType: "tpu7x-standard-4t", UseParallelContainers: false},
			want: 1,
		},
		{
			name: "gke_tpu7x_pathways_uses_1_container",
			mode: orchestrator.TypeGKE,
			job:  orchestrator.JobDefinition{MachineType: "tpu7x-standard-4t", UseParallelContainers: true, IsPathwaysJob: true},
			want: 1,
		},
		{
			name: "gke_non_tpu7x_uses_1_container",
			mode: orchestrator.TypeGKE,
			job:  orchestrator.JobDefinition{MachineType: "ct6e-standard-4t", UseParallelContainers: true},
			want: 1,
		},
		{
			name: "k8s_tpu7x_4t_uses_1_container",
			mode: orchestrator.TypeK8s,
			job:  orchestrator.JobDefinition{MachineType: "tpu7x-standard-4t", UseParallelContainers: true},
			want: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc := &GKEOrchestrator{mode: tc.mode}
			if got := orc.resolveParallelContainerCount(tc.job); got != tc.want {
				t.Errorf("resolveParallelContainerCount() = %d, want %d", got, tc.want)
			}
		})
	}

	// Verify defensive divisibility check in calculateResourceLimits rejects indivisible parallel splits.
	k8sOrc := &GKEOrchestrator{mode: orchestrator.TypeK8s}
	_, _, _, _, err := k8sOrc.calculateResourceLimits(
		ManifestOptions{ComputeType: "tpu7x-standard-1t", MachineType: "tpu7x-standard-1t", ParallelContainers: 2},
		JobProfile{IsTPU: true, CapacityCount: 1},
	)
	if err == nil || !strings.Contains(err.Error(), "cannot be evenly split") {
		t.Errorf("calculateResourceLimits(1 chip / 2 containers) err = %v, want divisibility error", err)
	}
}

func TestCalculateAllocatableMemoryBytes(t *testing.T) {
	t.Parallel()
	const mib int64 = 1024 * 1024
	for _, tc := range []struct {
		name     string
		memoryMb int
		want     int64
	}{
		{name: "zero", memoryMb: 0, want: 0},
		{name: "tiny clamps to zero", memoryMb: 256, want: 0},
		{name: "sub-1GiB uses 255MiB + 100MiB", memoryMb: 512, want: (512 - 255 - 100) * mib},
		{name: "1GiB tier-1 boundary", memoryMb: 1024, want: (1024 - 256 - 100) * mib},
		{name: "4GiB tier-1", memoryMb: 4096, want: (4096 - 1024 - 100) * mib},
		{name: "8GiB tier-2", memoryMb: 8192, want: 8192*mib - (1024*mib + (4096*mib*20)/100) - 100*mib},
		{name: "16GiB tier-3", memoryMb: 16384, want: 16384*mib - (1024*mib + (4096*mib*20)/100 + (8192*mib*10)/100) - 100*mib},
		{name: "64GiB tier-4", memoryMb: 65536, want: 65536*mib - (1024*mib + (4096*mib*20)/100 + (8192*mib*10)/100 + (49152*mib*6)/100) - 100*mib},
		{name: "128GiB tier-4 boundary", memoryMb: 131072, want: 131072*mib - (1024*mib + (4096*mib*20)/100 + (8192*mib*10)/100 + (114688*mib*6)/100) - 100*mib},
		{name: "256GiB n2d-standard-64 tier-5", memoryMb: 262144, want: 262144*mib - (1024*mib + (4096*mib*20)/100 + (8192*mib*10)/100 + (114688*mib*6)/100 + (131072*mib*2)/100) - 100*mib},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := calculateAllocatableMemoryBytes(tc.memoryMb); got != tc.want {
				t.Errorf("calculateAllocatableMemoryBytes(%d) = %d, want %d", tc.memoryMb, got, tc.want)
			}
		})
	}
}

func TestValidateCPUJobResources(t *testing.T) {
	t.Parallel()
	n2d64 := JobProfile{IsCPUMachine: true, CapacityCount: 64, MemoryMb: 262144}
	for _, tc := range []struct {
		name        string
		machineType string
		cpuReq      string
		memReq      string
		profile     JobProfile
		wantErr     string
	}{
		{name: "no overrides passes even for accelerator", machineType: "a3-highgpu-8g", profile: JobProfile{IsCPUMachine: false}},
		{name: "valid fractional CPU and memory", machineType: "n2d-standard-64", cpuReq: "8", memReq: "64Gi", profile: n2d64},
		{name: "valid memory only", machineType: "n2d-standard-64", memReq: "200G", profile: n2d64},
		{name: "valid CPU at exact allocatable ceiling", machineType: "n2d-standard-64", cpuReq: "63770m", profile: n2d64},
		{name: "valid memory at exact pre-1.37.2 allocatable ceiling", machineType: "n2d-standard-64", memReq: fmt.Sprintf("%d", calculateAllocatableMemoryBytes(262144)), profile: n2d64},
		{name: "memory 1 byte over 1.37.2+ max allocatable fails with human-readable Mi/Gi", machineType: "n2d-standard-64", memReq: fmt.Sprintf("%d", calculateMaxAllocatableMemoryBytes(262144)+1), profile: n2d64, wantErr: "Mi (~"},
		{name: "CPU exceeding allocatable fails with effective vCPUs", machineType: "n2d-standard-64", cpuReq: "64", profile: n2d64, wantErr: "from 64 effective vCPUs"},
		{name: "large node CPU exceeding 1.37.2+ max allocatable fails", machineType: "m2-hypermem-416", cpuReq: "415001m", profile: JobProfile{IsCPUMachine: true, CapacityCount: 416, MemoryMb: 12288000}, wantErr: "exceeds estimated GKE 1.37.2+ upper-bound allocatable CPU (415 from 416 effective vCPUs)"},
		{name: "memory exceeding 1.37.2+ max allocatable fails with human-readable units", machineType: "n2d-standard-64", memReq: "256Gi", profile: n2d64, wantErr: "exceeds estimated GKE 1.37.2+ upper-bound allocatable memory (261424Mi (~255Gi))"},
		{name: "overflow-scale CPU rejected via Cmp", machineType: "n2d-standard-64", cpuReq: "10000000000000000", profile: n2d64, wantErr: "exceeds estimated allocatable CPU"},
		{name: "overflow-scale memory rejected via Cmp", machineType: "n2d-standard-64", memReq: "10000000000Ei", profile: n2d64, wantErr: "exceeds estimated GKE 1.37.2+ upper-bound allocatable memory"},
		{name: "shared-core e2-medium uses 1 effective vCPU", machineType: "e2-medium", cpuReq: "1", profile: JobProfile{IsCPUMachine: true, CapacityCount: 1, MemoryMb: 4096}, wantErr: "exceeds estimated allocatable CPU"},
		{name: "threads_per_core=1 effective vCPU is not halved twice", machineType: "n2-standard-32", cpuReq: "15", profile: JobProfile{IsCPUMachine: true, CapacityCount: 16, MemoryMb: 131072}},
		{name: "GPU machine rejected", machineType: "g2-standard-12", cpuReq: "4", profile: JobProfile{IsCPUMachine: false}, wantErr: "not accelerator machine type"},
		{name: "TPU machine rejected", machineType: "ct6e-standard-4t", memReq: "32Gi", profile: JobProfile{IsCPUMachine: false}, wantErr: "not accelerator machine type"},
		{name: "zero CapacityCount and MemoryMb skip bound checks cleanly", machineType: "n2d-standard-64", cpuReq: "8", memReq: "64Gi", profile: JobProfile{IsCPUMachine: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateCPUJobResources(tc.machineType, tc.cpuReq, tc.memReq, tc.profile)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("validateCPUJobResources() err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateCPUJobResources() unexpected err: %v", err)
			}
		})
	}
}

func TestValidateCPUJobResources_GKE137WarningBand(t *testing.T) {
	for _, tc := range []struct {
		name        string
		machineType string
		cpuReq      string
		memReq      string
		profile     JobProfile
		wantWarn    string
	}{
		{
			name:        "n2-standard-32 124Gi warns above pre-1.37.2 estimate without citing default max-pods",
			machineType: "n2-standard-32",
			memReq:      "124Gi",
			profile:     JobProfile{IsCPUMachine: true, CapacityCount: 32, MemoryMb: 131072},
			wantWarn:    "or with higher max-pods-per-node",
		},
		{
			name:        "n2-standard-32 126Gi warns above GKE 1.37.2 default-110-pod estimate",
			machineType: "n2-standard-32",
			memReq:      "126Gi",
			profile:     JobProfile{IsCPUMachine: true, CapacityCount: 32, MemoryMb: 131072},
			wantWarn:    "default or higher max-pods-per-node",
		},
		{
			name:        "n2d-standard-64 250Gi warns above pre-1.37.2 estimate without rejecting",
			machineType: "n2d-standard-64",
			memReq:      "250Gi",
			profile:     JobProfile{IsCPUMachine: true, CapacityCount: 64, MemoryMb: 262144},
			wantWarn:    "pre-GKE 1.37.2 tiered allocatable estimate (249878Mi (~244Gi))",
		},
		{
			name:        "m2-hypermem-416 414950m warns above pre-1.37.2 estimate without rejecting",
			machineType: "m2-hypermem-416",
			cpuReq:      "414950m",
			profile:     JobProfile{IsCPUMachine: true, CapacityCount: 416, MemoryMb: 12288000},
			wantWarn:    "pre-GKE 1.37.2 tiered allocatable estimate (414890m from 416 effective vCPUs)",
		},
		{
			name:        "requests within pre-1.37.2 estimate emit no warning",
			machineType: "n2d-standard-64",
			cpuReq:      "63770m",
			memReq:      "240Gi",
			profile:     JobProfile{IsCPUMachine: true, CapacityCount: 64, MemoryMb: 262144},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, restore := logging.CaptureForTesting(t)
			defer restore()
			if err := validateCPUJobResources(tc.machineType, tc.cpuReq, tc.memReq, tc.profile); err != nil {
				t.Fatalf("validateCPUJobResources() unexpected err: %v", err)
			}
			restore()
			_, warnLogs := snapshot()
			if tc.wantWarn == "" {
				if warnLogs != "" {
					t.Errorf("expected no warnings, got:\n%s", warnLogs)
				}
				return
			}
			if !strings.Contains(warnLogs, tc.wantWarn) {
				t.Errorf("expected warning containing %q, got:\n%s", tc.wantWarn, warnLogs)
			}
		})
	}
}

func TestResolveHardwareRequirements_CPUMemoryValidation(t *testing.T) {
	t.Parallel()
	newOrc := func(machineType string, guestCpus, memMb int, threadsPerCore string) *GKEOrchestrator {
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
		orc.projectID = "test-project"
		orc.clusterZones = []string{"us-central1-a"}
		np := gkeJobNodePool{Name: "cpu-np", Config: gkeNodePoolConfig{MachineType: machineType}}
		if threadsPerCore != "" {
			np.Config.AdvancedMachineFeatures = &gkeAdvancedMachineFeatures{ThreadsPerCore: threadsPerCore}
		}
		orc.clusterDesc.NodePools = []gkeJobNodePool{np}
		orc.machineTypeClient = &MockMachineTypeClient{
			MT: &compute.MachineType{Name: machineType, GuestCpus: int64(guestCpus), MemoryMb: int64(memMb)},
		}
		return orc
	}

	t.Run("threads_per_core=1 allows 15 CPU on 32-vCPU machine (single halving) and rejects 16", func(t *testing.T) {
		t.Parallel()
		orc := newOrc("n2-standard-32", 32, 131072, "1")
		job := orchestrator.JobDefinition{
			ComputeType: "n2-standard-32", ClusterLocation: "us-central1-a", CPURequest: "15", MemoryRequest: "64Gi",
		}
		profile, _, _, err := orc.resolveHardwareRequirements(&job)
		if err != nil {
			t.Fatalf("expected 15 CPU / 64Gi to fit on threads_per_core=1 n2-standard-32, got: %v", err)
		}
		if !profile.IsCPUMachine || profile.CapacityCount != 16 || profile.MemoryMb != 131072 {
			t.Errorf("unexpected profile: %+v", profile)
		}

		tooLarge := job
		tooLarge.CPURequest = "16"
		if _, _, _, err := orc.resolveHardwareRequirements(&tooLarge); err == nil || !strings.Contains(err.Error(), "exceeds estimated allocatable CPU") {
			t.Fatalf("expected 16 CPU to exceed allocatable on threads_per_core=1 n2-standard-32, got: %v", err)
		}
	})

	t.Run("rejects memory above allocatable before cluster mounts or build", func(t *testing.T) {
		t.Parallel()
		orc := newOrc("n2d-standard-64", 64, 262144, "")
		job := orchestrator.JobDefinition{
			ComputeType: "n2d-standard-64", ClusterLocation: "us-central1-a", MemoryRequest: "256Gi",
		}
		if _, _, _, err := orc.resolveHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), "exceeds estimated GKE 1.37.2+ upper-bound allocatable memory") {
			t.Fatalf("got err = %v, want allocatable memory error", err)
		}
	})

	t.Run("rejects dynamically discovered GPU machine type with --cpu", func(t *testing.T) {
		t.Parallel()
		orc := newOrc("custom-gpu-box", 16, 65536, "")
		orc.machineTypeClient = &MockMachineTypeClient{
			MT: &compute.MachineType{
				Name:         "custom-gpu-box",
				GuestCpus:    16,
				MemoryMb:     65536,
				Accelerators: []*compute.MachineTypeAccelerators{{GuestAcceleratorCount: 1, GuestAcceleratorType: "nvidia-l4"}},
			},
		}
		job := orchestrator.JobDefinition{
			ComputeType: "custom-gpu-box", ClusterLocation: "us-central1-a", CPURequest: "4",
		}
		if _, _, _, err := orc.resolveHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), "not accelerator machine type") {
			t.Fatalf("got err = %v, want accelerator machine rejection", err)
		}
	})
}

func TestFormatAllocatableMemory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		bytes int64
		want  string
	}{
		{bytes: 0, want: "0 bytes"},
		{bytes: 512, want: "512 bytes"},
		{bytes: mibBytes, want: "1Mi"},
		{bytes: 157 * mibBytes, want: "157Mi"},
		{bytes: 1024 * mibBytes, want: "1024Mi (~1Gi)"},
		{bytes: 121428 * mibBytes, want: "121428Mi (~118Gi)"},
		{bytes: calculateAllocatableMemoryBytes(262144), want: "249878Mi (~244Gi)"},
		{bytes: calculateAllocatableMemoryBytes(262144) - gcsFuseMemoryBytes, want: "249622Mi (~243Gi)"},
	} {
		if got := formatAllocatableMemory(tc.bytes); got != tc.want {
			t.Errorf("formatAllocatableMemory(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestResolveHardwareRequirements_AttachedGPURejectsCPUMemory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		computeType string
	}{
		{name: "known nvidia-tesla-t4 label", computeType: "nvidia-tesla-t4"},
		{name: "non-nvidia attached accelerator exercises acceleratorToMachineType guard", computeType: "custom-attached-accel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
			orc.projectID = "test-project"
			orc.clusterZones = []string{"us-central1-a"}
			orc.clusterDesc.NodePools = []gkeJobNodePool{
				{Name: "gpu-np", Config: gkeNodePoolConfig{MachineType: "n1-standard-8"}},
			}
			orc.machineTypeClient = &MockMachineTypeClient{
				MT: &compute.MachineType{Name: "n1-standard-8", GuestCpus: 8, MemoryMb: 30720},
			}
			orc.acceleratorToMachineType = map[string]string{tc.computeType: "n1-standard-8"}
			job := orchestrator.JobDefinition{
				ComputeType: tc.computeType, ClusterLocation: "us-central1-a", CPURequest: "4",
			}
			if _, _, _, err := orc.resolveHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), "not accelerator machine type") {
				t.Fatalf("got err = %v, want attached-GPU accelerator rejection", err)
			}
		})
	}

	t.Run("static cluster rejects n1-standard-8 when all matching pools have attached GPUs", func(t *testing.T) {
		t.Parallel()
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
		orc.projectID = "test-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.clusterDesc.NodePools = []gkeJobNodePool{
			{
				Name:             "gpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType:  "n1-standard-8",
					Accelerators: []gkeAccelerator{{AcceleratorType: "nvidia-tesla-t4", AcceleratorCount: "1"}},
				},
			},
		}
		orc.machineTypeClient = &MockMachineTypeClient{
			MT: &compute.MachineType{Name: "n1-standard-8", GuestCpus: 8, MemoryMb: 30720},
		}
		orc.acceleratorToMachineType = map[string]string{"nvidia-tesla-t4": "n1-standard-8"}
		job := orchestrator.JobDefinition{
			ComputeType: "n1-standard-8", ClusterLocation: "us-central1-a", CPURequest: "4",
		}
		if _, _, _, err := orc.resolveHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), "not accelerator machine type") {
			t.Fatalf("got err = %v, want rejection when all matching static pools have attached GPUs", err)
		}
	})

	t.Run("static cluster allows n1-standard-8 when a CPU-only matching pool also exists", func(t *testing.T) {
		t.Parallel()
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
		orc.projectID = "test-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.clusterDesc.NodePools = []gkeJobNodePool{
			{
				Name:             "gpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType:  "n1-standard-8",
					Accelerators: []gkeAccelerator{{AcceleratorType: "nvidia-tesla-t4", AcceleratorCount: "1"}},
				},
			},
			{Name: "cpu-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n1-standard-8"}},
		}
		orc.machineTypeClient = &MockMachineTypeClient{
			MT: &compute.MachineType{Name: "n1-standard-8", GuestCpus: 8, MemoryMb: 30720},
		}
		job := orchestrator.JobDefinition{
			ComputeType: "n1-standard-8", ClusterLocation: "us-central1-a", CPURequest: "4",
		}
		profile, _, _, err := orc.resolveHardwareRequirements(&job)
		if err != nil || !profile.IsCPUMachine {
			t.Fatalf("resolveHardwareRequirements(mixed static n1-standard-8) = (profile=%+v, err=%v), want CPU profile", profile, err)
		}
	})

	t.Run("NAP cluster allows n1-standard-8 even when existing static pool has attached GPUs", func(t *testing.T) {
		t.Parallel()
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
		orc.projectID = "test-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.napEnabled = true
		orc.napLimits = map[string]int64{"cpu": 64}
		orc.clusterDesc.NodePools = []gkeJobNodePool{
			{
				Name:             "gpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType:  "n1-standard-8",
					Accelerators: []gkeAccelerator{{AcceleratorType: "nvidia-tesla-t4", AcceleratorCount: "1"}},
				},
			},
		}
		orc.machineTypeClient = &MockMachineTypeClient{
			MT: &compute.MachineType{Name: "n1-standard-8", GuestCpus: 8, MemoryMb: 30720},
		}
		job := orchestrator.JobDefinition{
			ComputeType: "n1-standard-8", ClusterLocation: "us-central1-a", CPURequest: "4",
		}
		profile, _, _, err := orc.resolveHardwareRequirements(&job)
		if err != nil || !profile.IsCPUMachine {
			t.Fatalf("resolveHardwareRequirements(NAP n1-standard-8) = (profile=%+v, err=%v), want CPU profile", profile, err)
		}
	})
}

func TestResolveHardwareRequirements_StaticAttachedGPUEdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("static cluster rejects n1-standard-8 when CPU-only matching pool has 0 nodes", func(t *testing.T) {
		t.Parallel()
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
		orc.projectID = "test-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.clusterDesc.NodePools = []gkeJobNodePool{
			{
				Name:             "gpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType:  "n1-standard-8",
					Accelerators: []gkeAccelerator{{AcceleratorType: "nvidia-tesla-t4", AcceleratorCount: "1"}},
				},
			},
			{Name: "zero-cpu-np", InitialNodeCount: 0, Config: gkeNodePoolConfig{MachineType: "n1-standard-8"}},
		}
		orc.machineTypeClient = &MockMachineTypeClient{
			MT: &compute.MachineType{Name: "n1-standard-8", GuestCpus: 8, MemoryMb: 30720},
		}
		job := orchestrator.JobDefinition{
			ComputeType: "n1-standard-8", ClusterLocation: "us-central1-a", CPURequest: "4",
		}
		if _, _, _, err := orc.resolveHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), "not accelerator machine type") {
			t.Fatalf("got err = %v, want rejection when CPU-only matching pool has 0 nodes", err)
		}
	})

	t.Run("static cluster gives actionable attached-GPU error for non-fractional n1-standard-8", func(t *testing.T) {
		t.Parallel()
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
		orc.projectID = "test-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.clusterDesc.NodePools = []gkeJobNodePool{
			{
				Name:             "gpu-np",
				InitialNodeCount: 1,
				Config: gkeNodePoolConfig{
					MachineType:  "n1-standard-8",
					Accelerators: []gkeAccelerator{{AcceleratorType: "nvidia-tesla-t4", AcceleratorCount: "1"}},
				},
			},
		}
		orc.machineTypeClient = &MockMachineTypeClient{
			MT: &compute.MachineType{Name: "n1-standard-8", GuestCpus: 8, MemoryMb: 30720},
		}
		job := orchestrator.JobDefinition{
			ComputeType: "n1-standard-8", ClusterLocation: "us-central1-a",
		}
		if _, _, _, err := orc.resolveHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), "--compute-type nvidia-tesla-t4") {
			t.Fatalf("got err = %v, want actionable --compute-type nvidia-tesla-t4 error", err)
		}
	})
}

func TestResolveHardwareRequirements_NAPCPUWithoutStaticPool(t *testing.T) {
	t.Parallel()
	orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{}))
	orc.projectID = "test-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.napEnabled = true
	orc.napLimits = map[string]int64{"cpu": 256}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "cpu-np", Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
	}
	orc.machineTypeClient = &MockMachineTypeClient{
		MT: &compute.MachineType{Name: "c3-standard-44", GuestCpus: 44, MemoryMb: 180224},
	}
	job := orchestrator.JobDefinition{
		ComputeType:        "c3-standard-44",
		ClusterLocation:    "us-central1-a",
		CPURequest:         "8",
		MemoryRequest:      "64Gi",
		GKENAPProvisioning: "spot",
	}
	profile, _, _, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("NAP CPU resolveHardwareRequirements without static pool failed: %v", err)
	}
	if !profile.IsCPUMachine || job.MachineType != "c3-standard-44" || profile.CapacityCount != 44 {
		t.Errorf("unexpected resolution: machineType=%q profile=%+v", job.MachineType, profile)
	}
}

func TestResolveJobMachineType_NAPCPUFallbackAndMixedCaseStatic(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		napEnabled  bool
		computeType string
		want        string
		wantErr     string
	}{
		{name: "NAP mixed-case CPU machine normalized", napEnabled: true, computeType: " C3-Standard-44 ", want: "c3-standard-44"},
		{name: "static mixed-case matching pool returns canonical lowercase", napEnabled: false, computeType: "N2D-Standard-64", want: "n2d-standard-64"},
		{name: "static unmatched CPU machine rejected", napEnabled: false, computeType: "c3-standard-44", wantErr: "not a known shorthand"},
		{name: "NAP TPU shorthand without pool rejected", napEnabled: true, computeType: "v5e-16", wantErr: "no matching machine types found in cluster"},
		{name: "NAP attached GPU label rejected", napEnabled: true, computeType: "nvidia-tesla-t4", wantErr: "not a known shorthand"},
		{name: "NAP unknown-count GPU shorthand rejected", napEnabled: true, computeType: "h100-16", wantErr: "no matching machine types found in cluster"},
		{name: "NAP v7x shorthand rejected", napEnabled: true, computeType: "v7x-8", wantErr: "not a known shorthand"},
		{name: "NAP non-hyphenated bogus compute type rejected", napEnabled: true, computeType: "bogus", wantErr: "not a known shorthand"},
		{name: "NAP family-prefixed custom machine rejected with predefined guidance", napEnabled: true, computeType: "n2-custom-8-32768", wantErr: "predefined non-legacy GCE machine type"},
		{name: "NAP legacy custom machine rejected with predefined guidance", napEnabled: true, computeType: "custom-8-32768", wantErr: "predefined non-legacy GCE machine type"},
		{name: "NAP f1-micro rejected with predefined guidance", napEnabled: true, computeType: "f1-micro", wantErr: "predefined non-legacy GCE machine type"},
		{name: "NAP g1-small rejected with predefined guidance", napEnabled: true, computeType: "g1-small", wantErr: "predefined non-legacy GCE machine type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orc := newTestGKEOrchestrator(nil)
			orc.napEnabled = tc.napEnabled
			orc.clusterDesc.NodePools = []gkeJobNodePool{
				{Name: "cpu-np", Config: gkeNodePoolConfig{MachineType: "n2d-standard-64"}},
			}
			got, err := orc.resolveJobMachineType(tc.computeType, "", "", false)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("resolveJobMachineType(%q) err = %v, want substring %q", tc.computeType, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveJobMachineType(%q) unexpected err: %v", tc.computeType, err)
			}
			if got != tc.want {
				t.Errorf("resolveJobMachineType(%q) = %q, want %q", tc.computeType, got, tc.want)
			}
		})
	}
}

func TestResolveJobMachineType_NAPUnsupportedPoolFiltering(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		pools           []gkeJobNodePool
		computeType     string
		provisioning    string
		hasComputeClass bool
		want            string
		wantErr         string
	}{
		{
			name:        "NAP f1-micro in default-pool only is still rejected",
			pools:       []gkeJobNodePool{{Name: "default-pool", InitialNodeCount: 2, Config: gkeNodePoolConfig{MachineType: "f1-micro"}}},
			computeType: "f1-micro",
			wantErr:     "predefined non-legacy GCE machine type",
		},
		{
			name:        "NAP custom shape in 0-node pool only is still rejected",
			pools:       []gkeJobNodePool{{Name: "custom-np", InitialNodeCount: 0, Config: gkeNodePoolConfig{MachineType: "n2-custom-8-32768"}}},
			computeType: "n2-custom-8-32768",
			wantErr:     "predefined non-legacy GCE machine type",
		},
		{
			name: "NAP custom shape in tainted system pool only is still rejected",
			pools: []gkeJobNodePool{{
				Name:             "sys-np",
				InitialNodeCount: 2,
				Config: gkeNodePoolConfig{
					MachineType: "n2-custom-8-32768",
					Taints:      []gkeTaint{{Key: k8s.GKESystemTaintKey, Value: "true", Effect: "NO_SCHEDULE"}},
				},
			}},
			computeType: "n2-custom-8-32768",
			wantErr:     "predefined non-legacy GCE machine type",
		},
		{
			name:         "NAP custom shape in on-demand static pool rejected for spot request",
			pools:        []gkeJobNodePool{{Name: "custom-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2-custom-8-32768"}}},
			computeType:  "n2-custom-8-32768",
			provisioning: "spot",
			wantErr:      "predefined non-legacy GCE machine type",
		},
		{
			name:            "NAP custom shape allowed when Custom ComputeClass is specified",
			computeType:     "n4-custom-8-20480",
			hasComputeClass: true,
			want:            "n4-custom-8-20480",
		},
		{
			name:        "NAP custom shape in eligible non-system static pool is allowed",
			pools:       []gkeJobNodePool{{Name: "custom-np", InitialNodeCount: 1, Config: gkeNodePoolConfig{MachineType: "n2-custom-8-32768"}}},
			computeType: "n2-custom-8-32768",
			want:        "n2-custom-8-32768",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orc := newTestGKEOrchestrator(nil)
			orc.napEnabled = true
			orc.clusterDesc.NodePools = tc.pools
			got, err := orc.resolveJobMachineType(tc.computeType, tc.provisioning, "", tc.hasComputeClass)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("resolveJobMachineType(%q) err = %v, want substring %q", tc.computeType, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("resolveJobMachineType(%q) = (%q, %v), want (%q, nil)", tc.computeType, got, err, tc.want)
			}
		})
	}
}

func TestResolveHardwareRequirements_TypeK8sCPUMemory(t *testing.T) {
	t.Parallel()
	cpuPool := k8s.PoolCapacity{
		AllocatableCPUMilli: 15800,
		MemMiBPerNode:       32768,
		ReadyNodes:          2,
	}
	gpuPool := k8s.PoolCapacity{
		MachineType:     "mi300x-node",
		IsGPU:           true,
		GPUsPerNode:     8,
		GPUResourceName: "amd.com/gpu",
		ReadyNodes:      1,
	}
	tpuPool := k8s.PoolCapacity{
		MachineType: "ct6e-standard-4t",
		IsTPU:       true,
		TPUType:     "v6e",
		TPUsPerNode: 4,
		Topologies:  []string{"2x2"},
		ReadyNodes:  1,
	}

	for _, tc := range []struct {
		name    string
		inv     *k8s.Inventory
		job     orchestrator.JobDefinition
		wantErr string
	}{
		{
			name: "unlabeled CPU pool accepts fractional --cpu and --memory without --compute-type",
			inv: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				CPUPoolCap:  &cpuPool,
				PoolNames:   []string{"cpu"},
				Pools:       map[string]k8s.PoolCapacity{"cpu": cpuPool},
			},
			job: orchestrator.JobDefinition{CPURequest: "4", MemoryRequest: "16Gi"},
		},
		{
			name: "unlabeled CPU pool rejects --cpu above node allocatable",
			inv: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				CPUPoolCap:  &cpuPool,
				PoolNames:   []string{"cpu"},
				Pools:       map[string]k8s.PoolCapacity{"cpu": cpuPool},
			},
			job:     orchestrator.JobDefinition{CPURequest: "16"},
			wantErr: "exceeds node allocatable CPU (15800m)",
		},
		{
			name: "unlabeled CPU pool rejects --memory above node allocatable",
			inv: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				CPUPoolCap:  &cpuPool,
				PoolNames:   []string{"cpu"},
				Pools:       map[string]k8s.PoolCapacity{"cpu": cpuPool},
			},
			job:     orchestrator.JobDefinition{MemoryRequest: "33Gi"},
			wantErr: "exceeds node allocatable memory (32768Mi (~32Gi))",
		},
		{
			name: "GPU pool rejects --cpu",
			inv: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"mi300x-node"},
				Pools:       map[string]k8s.PoolCapacity{"mi300x-node": gpuPool},
			},
			job:     orchestrator.JobDefinition{ComputeType: "mi300x-node", CPURequest: "4"},
			wantErr: "not accelerator machine type",
		},
		{
			name: "TPU pool rejects --memory",
			inv: &k8s.Inventory{
				LabelScheme: k8s.SchemeGKE,
				TPUPoolCap:  &tpuPool,
				PoolNames:   []string{"ct6e-standard-4t"},
				Pools:       map[string]k8s.PoolCapacity{"ct6e-standard-4t": tpuPool},
			},
			job:     orchestrator.JobDefinition{ComputeType: "ct6e-standard-4t", Topology: "2x2", MemoryRequest: "16Gi"},
			wantErr: "not accelerator machine type",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orc := newTestGKEOrchestrator(nil)
			orc.mode = orchestrator.TypeK8s
			orc.inventory = tc.inv
			job := tc.job
			profile, _, _, err := orc.resolveHardwareRequirements(&job)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("resolveHardwareRequirements() err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveHardwareRequirements() unexpected err: %v", err)
			}
			if !profile.IsCPUMachine || profile.AllocatableCPUMilli != 15800 {
				t.Errorf("got profile = %+v, want CPU profile with AllocatableCPUMilli=15800", profile)
			}
		})
	}
}
