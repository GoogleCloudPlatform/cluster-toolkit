// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gke

import (
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"os"
	"strings"
	"testing"
)

type MockExecutor struct {
	responses map[string][]shell.CommandResult
	callCount map[string]int
}

func NewMockExecutor(responses map[string][]shell.CommandResult) *MockExecutor {
	return &MockExecutor{
		responses: responses,
		callCount: make(map[string]int),
	}
}

func (m *MockExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	cmdKey := name + " " + strings.Join(args, " ")

	for key, results := range m.responses {
		if strings.HasPrefix(cmdKey, key) {
			count := m.callCount[key]
			if count < len(results) {
				m.callCount[key]++
				return results[count]
			}
		}
	}

	return shell.CommandResult{ExitCode: 1, Stderr: "mock error: unexpected command"}
}

func (m *MockExecutor) ExecuteCommandStream(name string, args ...string) error {
	// Mock implementation: just return nil to satisfy interface
	return nil
}

type MockKubeClient struct {
	Namespace string
	Workloads []string
	Err       error
}

func (m *MockKubeClient) GetJobNamespace(workloadName string) (string, error) {
	return m.Namespace, m.Err
}

func (m *MockKubeClient) ListWorkloads(namespace string, workloadName string) ([]string, error) {
	return m.Workloads, m.Err
}

func (m *MockKubeClient) DeleteJobSet(namespace string, name string) error {
	return m.Err
}

func TestGenerateGKEManifest_Accelerators(t *testing.T) {

	tests := []struct {
		name            string
		acceleratorType string
		cpuLimit        string
		memoryLimit     string
		gpuLimit        string
		tpuLimit        string
		wantLabels      []string
		wantLimits      []string
		dontWantLimits  []string
		wantErr         bool
	}{
		{
			name:            "A3 Mega (H100)",
			acceleratorType: "nvidia-h100-mega-80gb",
			cpuLimit:        "",
			memoryLimit:     "",
			gpuLimit:        "1",
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-h100-mega-80gb"},
			wantLimits:      []string{`nvidia.com/gpu: "1"`},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "A4X Max (GB200)",
			acceleratorType: "nvidia-gb200",
			cpuLimit:        "",
			memoryLimit:     "",
			gpuLimit:        "1",
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-gb200"},
			wantLimits:      []string{`nvidia.com/gpu: "1"`},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "G2 (L4)",
			acceleratorType: "nvidia-l4",
			cpuLimit:        "",
			memoryLimit:     "",
			gpuLimit:        "1",
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-l4"},
			wantLimits:      []string{`nvidia.com/gpu: "1"`},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "TPU v6e slice",
			acceleratorType: "tpu-v6e-slice",
			cpuLimit:        "",
			memoryLimit:     "",
			tpuLimit:        "4",
			wantLabels:      []string{"cloud.google.com/gke-tpu-accelerator: tpu-v6e-slice"},
			wantLimits:      []string{`google.com/tpu: "4"`},
			dontWantLimits:  []string{"nvidia.com/gpu", "cpu:", "memory:"},
		},
		{
			name:            "CPU Only (Default)",
			acceleratorType: "",
			wantErr:         true,
		},
		{
			name:            "Fallback NVIDIA",
			acceleratorType: "nvidia-unknown-new-gpu",
			cpuLimit:        "",
			memoryLimit:     "",
			gpuLimit:        "1",
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-unknown-new-gpu"},
			wantLimits:      []string{`nvidia.com/gpu: "1"`},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "Uniform CPU Machine via Accelerator Flag",
			acceleratorType: "n2-standard-2",
			cpuLimit:        "",
			memoryLimit:     "",
			wantLabels:      []string{},
			wantLimits:      []string{`cpu: "1"`},
			dontWantLimits:  []string{"nvidia.com/gpu", "google.com/tpu"},
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := orchestrator.JobDefinition{
				WorkloadName:    "test-workload",
				CommandToRun:    "echo hello",
				AcceleratorType: tt.acceleratorType,
				ClusterLocation: "us-central1",
			}

			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
				"gcloud compute machine-types describe n2-standard-2 --zone=us-central1 --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
			}
			orc := &GKEOrchestrator{executor: NewMockExecutor(mockResponses)}

			opts, profile, err := orc.PrepareManifestOptions(job, "test-image:latest")
			var manifest string
			if err == nil {
				manifest, err = orc.GenerateGKEManifest(opts, profile)
			}

			if (err != nil) != tt.wantErr {
				t.Fatalf("Manifest generation failed with error %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			for _, want := range tt.wantLabels {
				if !strings.Contains(manifest, want) {
					t.Errorf("manifest missing expected label %q\nManifest: %s", want, manifest)
				}
			}

			for _, want := range tt.wantLimits {
				if !strings.Contains(manifest, want) {
					t.Errorf("manifest missing expected limit %q\nManifest: %s", want, manifest)
				}
			}

			for _, dontWant := range tt.dontWantLimits {
				if strings.Contains(manifest, dontWant) {
					t.Errorf("manifest contains unexpected limit %q", dontWant)
				}
			}
		})
	}
}

func TestGenerateGKEManifest_Volumes(t *testing.T) {
	orc := NewGKEOrchestrator()
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-4 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"guestCpus": 4, "memoryMb": 16384}`},
		},
	})
	orc.SetExecutor(mockExec)
	job := orchestrator.JobDefinition{
		WorkloadName:    "volume-test",
		CommandToRun:    "echo hello",
		ClusterLocation: "us-central1-a",
		AcceleratorType: "n2-standard-4",
		Volumes: []orchestrator.VolumeDefinition{
			{Name: "vol-0", Source: "gs://my-bucket", MountPath: "/data", Type: "gcsfuse"},
			{Name: "vol-1", Source: "/host/path", MountPath: "/host", Type: "hostPath"},
			{Name: "vol-2", Source: "my-pvc", MountPath: "/pvc", Type: "pvc"},
		},
	}

	opts, profile, err := orc.PrepareManifestOptions(job, "test-image:latest")
	if err != nil {
		t.Fatalf("prepareManifestOptions failed: %v", err)
	}

	opts.AcceleratorType = "n2-standard-4"
	manifest, err := orc.GenerateGKEManifest(opts, profile)

	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	expectedSubStrs := []string{
		"gke-gcsfuse/volumes: \"true\"",
		"name: vol-0",
		"mountPath: /data",
		"name: vol-1",
		"mountPath: /host",
		"name: vol-2",
		"mountPath: /pvc",
		"csi:",
		"driver: gcsfuse.csi.storage.gke.io",
		"bucketName: my-bucket",
		"hostPath:",
		"path: /host/path",
		"persistentVolumeClaim:",
		"claimName: my-pvc",
	}

	for _, want := range expectedSubStrs {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest missing expected substring %q\nManifest: %s", want, manifest)
		}
	}
}

func TestGenerateGKEManifest_CommandEscaping(t *testing.T) {
	orc := NewGKEOrchestrator()
	opts := ManifestOptions{
		WorkloadName:    "test-workload",
		FullImageName:   "test-image:latest",
		CommandToRun:    `python -c "print('hello')"` + " && echo \"world\"",
		AcceleratorType: "nvidia-l4",
	}

	manifest, err := orc.GenerateGKEManifest(opts, JobProfile{})

	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	// We expect the command to be properly rendered as a YAML list
	expectedSubStr := `                command:
                - "/bin/bash"
                - "-c"
                - "python -c \"print('hello')\" && echo \"world\""`
	if !strings.Contains(manifest, expectedSubStr) {
		t.Errorf("manifest command string is not properly rendered as a YAML list.\nExpected substring:\n%s\nActual manifest:\n%s", expectedSubStr, manifest)
	}
}

func TestInjectTolerations(t *testing.T) {
	orc := NewGKEOrchestrator()

	// Sample Deployment YAML
	inputYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobset-controller-manager
  namespace: jobset-system
spec:
  template:
    metadata:
      labels:
        foo: bar
    spec:
      containers:
      - name: manager
        image: jobset:v0.1.0
`
	// Convert to byte array
	manifestBytes := []byte(inputYAML)

	// Clean and inject
	cleanedBytes, err := orc.cleanJobSetManifests(manifestBytes)
	if err != nil {
		t.Fatalf("cleanJobSetManifests failed: %v", err)
	}

	cleanedString := string(cleanedBytes)

	// Check for tolerations
	if !strings.Contains(cleanedString, "key: nvidia.com/gpu") {
		t.Errorf("Resulting manifest should contain nvidia.com/gpu toleration.\nGot:\n%s", cleanedString)
	}
	if !strings.Contains(cleanedString, "key: components.gke.io/gke-managed-components") {
		t.Errorf("Resulting manifest should contain components.gke.io/gke-managed-components toleration.\nGot:\n%s", cleanedString)
	}
	if !strings.Contains(cleanedString, "control-plane: controller-manager") {
		t.Errorf("Resulting manifest should contain control-plane: controller-manager label.\nGot:\n%s", cleanedString)
	}
}

func TestGeneratePathwaysManifest(t *testing.T) {
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-test",
		CommandToRun:    "echo hello",
		NumSlices:       2,
		ClusterLocation: "us-central1",
		AcceleratorType: "n2-standard-2",
		Pathways: orchestrator.PathwaysJobDefinition{
			ProxyServerImage: "proxy:latest",
			ServerImage:      "server:latest",
			WorkerImage:      "worker:latest",
			GCSLocation:      "gs://my-bucket",
			HeadNodePool:     "pathways-np",
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1 --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	}
	orc := &GKEOrchestrator{executor: NewMockExecutor(mockResponses)}
	orc.clusterDesc.NodePools = []gkeJobNodePool{{Name: "default-pool"}}
	manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest")
	if err != nil {
		t.Fatalf("generatePathwaysManifest failed: %v", err)
	}

	err = os.WriteFile("gcluster_pathways_manifest.yaml", []byte(manifest), 0644)
	if err != nil {
		t.Fatalf("failed to write manifest to file: %v", err)
	}

	expectedSubstrs := []string{
		"name: pathways-test",
		"replicas: 2",
		"image: proxy:latest",
		"--gcs_scratch_location=gs://my-bucket",
		"cloud.google.com/gke-nodepool: pathways-np",
		"completionMode: Indexed",
		"alpha.jobset.sigs.k8s.io/exclusive-topology: kubernetes.io/hostname",
		"MEGASCALE_GRPC_ENABLE_XOR_TRACER",
		`cpu: "16"`,
		`memory: "100Gi"`,
		`cpu: "8"`,
		`memory: "32Gi"`,
	}

	for _, substr := range expectedSubstrs {
		if !strings.Contains(manifest, substr) {
			t.Errorf("manifest missing expected substring %q", substr)
		}
	}
}

func TestAwaitJobCompletion(t *testing.T) {
	workloadName := "test-workload"
	clusterName := "test-cluster"
	clusterLocation := "us-central1-a"
	projectID := "test-project"

	tests := []struct {
		name          string
		mockNamespace string
		mockWorkloads []string
		mockResponses map[string][]shell.CommandResult
		expectedError string
	}{
		{
			name:          "Successful completion",
			mockNamespace: "default",
			mockWorkloads: []string{"jobset-test-workload-abc"},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for=condition=Finished workload jobset-test-workload-abc -n default --timeout=1h": {
					{ExitCode: 0, Stdout: "workload condition met"},
				},
				"kubectl get jobset test-workload -n default -o json": {
					{ExitCode: 0, Stdout: `{"status": {"conditions": [{"type": "Completed", "status": "True", "lastTransitionTime": "2026-04-12T12:00:00Z"}]}}`},
				},
			},
			expectedError: "",
		},
		{
			name:          "Job timeout",
			mockNamespace: "default",
			mockWorkloads: []string{"jobset-test-workload-abc"},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for=condition=Finished workload jobset-test-workload-abc -n default --timeout=1h": {
					{ExitCode: 1, Stderr: "timed out waiting for conditions to be met"},
				},
			},
			expectedError: "job timed out",
		},
		{
			name:          "Job finished but not completed",
			mockNamespace: "default",
			mockWorkloads: []string{"jobset-test-workload-abc"},
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for=condition=Finished workload jobset-test-workload-abc -n default --timeout=1h": {
					{ExitCode: 0, Stdout: "workload condition met"},
				},
				"kubectl get jobset test-workload -n default -o json": {
					{ExitCode: 0, Stdout: `{"status": {"conditions": [{"type": "Failed", "status": "True", "lastTransitionTime": "2026-04-12T12:00:00Z"}]}}`},
				},
			},
			expectedError: "job completed unsuccessfully with status: Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := &MockExecutor{responses: tt.mockResponses, callCount: make(map[string]int)}
			mockKube := &MockKubeClient{Namespace: tt.mockNamespace, Workloads: tt.mockWorkloads}
			orc := &GKEOrchestrator{executor: mockExecutor, kubeClient: mockKube}

			err := orc.awaitJobCompletion(workloadName, clusterName, clusterLocation, projectID, "1h")

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, but got: %v", tt.expectedError, err)
				}
			}
		})
	}
}

func TestFetchMachineCapacity(t *testing.T) {
	tests := []struct {
		name          string
		machineType   string
		zone          string
		mockResponses map[string][]shell.CommandResult
		wantCount     int
		wantErr       bool
	}{
		{
			name:        "Successful lookup",
			machineType: "g2-standard-48",
			zone:        "us-central1-a",
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe g2-standard-48 --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "nvidia-l4"}]}`},
				},
			},
			wantCount: 4,
			wantErr:   false,
		},
		{
			name:        "Lookup failure with retries succeeding",
			machineType: "g2-standard-48",
			zone:        "us-central1-a",
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe g2-standard-48 --zone=us-central1-a --format=json": {
					{ExitCode: 1, Stderr: "slow network"},
					{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "nvidia-l4"}]}`},
				},
			},
			wantCount: 4,
			wantErr:   false,
		},
		{
			name:        "Total failure after retries",
			machineType: "unknown-machine-type",
			zone:        "us-central1-a",
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe unknown-machine-type --zone=us-central1-a --format=json": {
					{ExitCode: 1, Stderr: "slow network"},
					{ExitCode: 1, Stderr: "slow network"},
					{ExitCode: 1, Stderr: "slow network"},
				},
			},
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			orc := &GKEOrchestrator{executor: mockExecutor}

			count, err := orc.FetchMachineCapacity(tt.machineType, tt.zone)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if count != tt.wantCount {
					t.Errorf("Expected count %d, got %d", tt.wantCount, count)
				}
			}
		})
	}
}

func TestProcessNodePoolCapacity_Hyperthreading(t *testing.T) {
	tests := []struct {
		name          string
		np            gkeJobNodePool
		mockResponses map[string][]shell.CommandResult
		wantCpus      int
		wantErr       bool
	}{
		{
			name: "x86 with hyperthreading disabled",
			np: gkeJobNodePool{
				Config: gkeNodePoolConfig{
					MachineType: "c2-standard-60",
					AdvancedMachineFeatures: &gkeAdvancedMachineFeatures{
						ThreadsPerCore: "1",
					},
				},
				InitialNodeCount: 1,
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe c2-standard-60 --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"guestCpus": 60, "memoryMb": 240000}`},
				},
			},
			wantCpus: 30, // Halved!
			wantErr:  false,
		},
		{
			name: "x86 with hyperthreading enabled",
			np: gkeJobNodePool{
				Config: gkeNodePoolConfig{
					MachineType: "c2-standard-60",
					AdvancedMachineFeatures: &gkeAdvancedMachineFeatures{
						ThreadsPerCore: "2",
					},
				},
				InitialNodeCount: 1,
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe c2-standard-60 --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"guestCpus": 60, "memoryMb": 240000}`},
				},
			},
			wantCpus: 60, // Not halved!
			wantErr:  false,
		},
		{
			name: "ARM64 with threadsPerCore=1",
			np: gkeJobNodePool{
				Config: gkeNodePoolConfig{
					MachineType: "t2a-standard-16",
					AdvancedMachineFeatures: &gkeAdvancedMachineFeatures{
						ThreadsPerCore: "1",
					},
				},
				InitialNodeCount: 1,
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe t2a-standard-16 --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"guestCpus": 16, "memoryMb": 64000}`},
				},
			},
			wantCpus: 16, // Not halved because it's ARM!
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			orc := &GKEOrchestrator{executor: mockExecutor}
			orc.machineTypeToThreadsPerCore = make(map[string]string)
			if tt.np.Config.AdvancedMachineFeatures != nil {
				orc.machineTypeToThreadsPerCore[tt.np.Config.MachineType] = tt.np.Config.AdvancedMachineFeatures.ThreadsPerCore
			}

			cpus, _, _, _, _, _, _, err := orc.processNodePoolCapacity(tt.np, "us-central1-a")

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if cpus != tt.wantCpus {
					t.Errorf("Expected cpus %d, got %d", tt.wantCpus, cpus)
				}
			}
		})
	}
}

func TestAutoDetectCPUNodePool(t *testing.T) {
	tests := []struct {
		name      string
		nodePools []gkeJobNodePool
		wantPool  string
	}{
		{
			name: "Single CPU pool (not matching expected names)",
			nodePools: []gkeJobNodePool{
				{Name: "system", Config: gkeNodePoolConfig{Taints: []gkeTaint{{Key: "components.gke.io/gke-managed-components", Value: "true"}}}},
				{Name: "cpu-pool", Config: gkeNodePoolConfig{}},
			},
			wantPool: "",
		},
		{
			name: "Single CPU pool (matching cpu-np)",
			nodePools: []gkeJobNodePool{
				{Name: "system", Config: gkeNodePoolConfig{Taints: []gkeTaint{{Key: "components.gke.io/gke-managed-components", Value: "true"}}}},
				{Name: "cpu-np", Config: gkeNodePoolConfig{}},
			},
			wantPool: "cpu-np",
		},
		{
			name: "Multiple CPU pools, prefer cpu-np",
			nodePools: []gkeJobNodePool{
				{Name: "system", Config: gkeNodePoolConfig{Taints: []gkeTaint{{Key: "components.gke.io/gke-managed-components", Value: "true"}}}},
				{Name: "my-cpu-pool", Config: gkeNodePoolConfig{}},
				{Name: "cpu-np", Config: gkeNodePoolConfig{}},
			},
			wantPool: "cpu-np",
		},
		{
			name: "Multiple CPU pools, prefer pathways-np",
			nodePools: []gkeJobNodePool{
				{Name: "system", Config: gkeNodePoolConfig{Taints: []gkeTaint{{Key: "components.gke.io/gke-managed-components", Value: "true"}}}},
				{Name: "my-cpu-pool", Config: gkeNodePoolConfig{}},
				{Name: "pathways-np", Config: gkeNodePoolConfig{}},
			},
			wantPool: "pathways-np",
		},
		{
			name: "Multiple CPU pools, return first matching",
			nodePools: []gkeJobNodePool{
				{Name: "system", Config: gkeNodePoolConfig{Taints: []gkeTaint{{Key: "components.gke.io/gke-managed-components", Value: "true"}}}},
				{Name: "pathways-np", Config: gkeNodePoolConfig{}},
				{Name: "cpu-np", Config: gkeNodePoolConfig{}},
			},
			wantPool: "pathways-np",
		},
		{
			name: "Ambiguous CPU pools (none matching preferred names)",
			nodePools: []gkeJobNodePool{
				{Name: "system", Config: gkeNodePoolConfig{Taints: []gkeTaint{{Key: "components.gke.io/gke-managed-components", Value: "true"}}}},
				{Name: "cpu-pool-1", Config: gkeNodePoolConfig{}},
				{Name: "cpu-pool-2", Config: gkeNodePoolConfig{}},
			},
			wantPool: "",
		},
		{
			name: "Exclude system pools by taint",
			nodePools: []gkeJobNodePool{
				{Name: "system", Config: gkeNodePoolConfig{Taints: []gkeTaint{{Key: "components.gke.io/gke-managed-components", Value: "true"}}}},
				{Name: "cpu-np", Config: gkeNodePoolConfig{}},
			},
			wantPool: "cpu-np",
		},
		{
			name: "Exclude pools with accelerators",
			nodePools: []gkeJobNodePool{
				{Name: "cpu-np", Config: gkeNodePoolConfig{}},
				{Name: "gpu-pool", Config: gkeNodePoolConfig{Accelerators: []gkeAccelerator{{AcceleratorType: "nvidia-l4"}}}},
			},
			wantPool: "cpu-np",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{}
			orc.clusterDesc.NodePools = tt.nodePools

			got := orc.autoDetectCPUNodePool()
			if got != tt.wantPool {
				t.Errorf("autoDetectCPUNodePool() = %v, want %v", got, tt.wantPool)
			}
		})
	}
}

func TestDetermineIfCPUMachine_Hyperthreading(t *testing.T) {
	tests := []struct {
		name           string
		job            orchestrator.JobDefinition
		threadsPerCore string
		mockResponses  map[string][]shell.CommandResult
		wantIsCPU      bool
		wantCapacity   int
		wantErr        bool
	}{
		{
			name: "x86 with hyperthreading disabled in map",
			job: orchestrator.JobDefinition{
				AcceleratorType: "c2-standard-60",
				ClusterLocation: "us-central1-a",
			},
			threadsPerCore: "1",
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe c2-standard-60 --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"guestCpus": 60, "memoryMb": 240000}`},
				},
			},
			wantIsCPU:    true,
			wantCapacity: 30, // Halved!
			wantErr:      false,
		},
		{
			name: "Fallback to Compute API when not in map",
			job: orchestrator.JobDefinition{
				AcceleratorType: "c2-standard-60",
				ClusterLocation: "us-central1-a",
			},
			threadsPerCore: "",
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe c2-standard-60 --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"guestCpus": 60, "memoryMb": 240000}`},
				},
			},
			wantIsCPU:    true,
			wantCapacity: 60, // Not halved!
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			orc := &GKEOrchestrator{
				executor: mockExecutor,
			}
			orc.machineTypeToThreadsPerCore = make(map[string]string)
			if tt.threadsPerCore != "" {
				orc.machineTypeToThreadsPerCore[tt.job.AcceleratorType] = tt.threadsPerCore
			}
			orc.machineCapCache = make(map[string]MachineTypeCap)

			isCPU, capacity, err := orc.determineIfCPUMachine(tt.job)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if isCPU != tt.wantIsCPU {
					t.Errorf("Expected isCPU %t, got %t", tt.wantIsCPU, isCPU)
				}
				if capacity != tt.wantCapacity {
					t.Errorf("Expected capacity %d, got %d", tt.wantCapacity, capacity)
				}
			}
		})
	}
}

func TestVerifySuperSlicingActive(t *testing.T) {
	tests := []struct {
		name          string
		opts          ManifestOptions
		mockResponses map[string][]shell.CommandResult
		envVars       map[string]string
		wantResult    bool
	}{
		{
			name: "Success - Super-slicing active",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				AcceleratorType: "tpu-v6e-slice",
			},
			envVars: map[string]string{"GKE_NODE_POOL_NAME": "test-pool"},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud container node-pools describe test-pool --cluster=test-cluster --location=us-central1-a --format=json(placementPolicy)": {
					{ExitCode: 0, Stdout: `{"placementPolicy": {"acceleratorTopologyMode": "PROVISION_ONLY"}}`},
				},
				"kubectl get crd topologies.kueue.x-k8s.io": {
					{ExitCode: 0},
				},
			},
			wantResult: true,
		},
		{
			name: "Failure - No TPU",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				AcceleratorType: "nvidia-l4",
			},
			envVars:       nil,
			mockResponses: nil,
			wantResult:    false,
		},
		{
			name: "Failure - No Node Pool set",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				AcceleratorType: "tpu-v6e-slice",
			},
			envVars:       nil,
			mockResponses: nil,
			wantResult:    false,
		},
		{
			name: "Failure - gcloud fails",
			opts: ManifestOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				AcceleratorType: "tpu-v6e-slice",
			},
			envVars: map[string]string{"GKE_NODE_POOL_NAME": "test-pool"},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud container node-pools describe test-pool --cluster=test-cluster --location=us-central1-a --format=json(placementPolicy)": {
					{ExitCode: 1, Stderr: "slow network"},
				},
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			mockExecutor := NewMockExecutor(tt.mockResponses)
			orc := &GKEOrchestrator{executor: mockExecutor}

			got, err := orc.verifySuperSlicingActive(tt.opts)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if got != tt.wantResult {
				t.Errorf("Expected %t, got %t", tt.wantResult, got)
			}
		})
	}
}

func TestGenerateGKEManifest_Verbose_GPU(t *testing.T) {
	orc := NewGKEOrchestrator()
	opts := ManifestOptions{
		WorkloadName:    "test-workload",
		FullImageName:   "test-image:latest",
		CommandToRun:    "python app.py",
		AcceleratorType: "nvidia-l4",
		Verbose:         true,
	}

	manifest, err := orc.GenerateGKEManifest(opts, JobProfile{})
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	if !strings.Contains(manifest, "name: NCCL_DEBUG") || !strings.Contains(manifest, "value: \"INFO\"") {
		t.Errorf("manifest missing expected GPU verbose env var.\nManifest: %s", manifest)
	}
}

func TestGenerateGKEManifest_Verbose_TPU(t *testing.T) {
	orc := NewGKEOrchestrator()
	opts := ManifestOptions{
		WorkloadName:    "test-workload",
		FullImageName:   "test-image:latest",
		CommandToRun:    "python app.py",
		AcceleratorType: "tpu-v6e-slice",
		Verbose:         true,
	}

	manifest, err := orc.GenerateGKEManifest(opts, JobProfile{})
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	if !strings.Contains(manifest, "name: TPU_STDERR_LOG_LEVEL") || !strings.Contains(manifest, "value: \"0\"") {
		t.Errorf("manifest missing expected TPU verbose env var.\nManifest: %s", manifest)
	}
}

func TestParseJobStatus_CompletionTime(t *testing.T) {
	orc := &GKEOrchestrator{}

	tests := []struct {
		name               string
		obj                map[string]interface{}
		wantStatus         string
		wantCompletionTime string
	}{
		{
			name: "Top-level completionTime",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"completionTime": "2026-04-03T12:00:00Z",
				},
			},
			wantStatus:         "Unknown",
			wantCompletionTime: "2026-04-03T12:00:00Z",
		},
		{
			name: "TransitionTime in Succeeded condition",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "Succeeded",
							"status":             "True",
							"lastTransitionTime": "2026-04-03T12:15:00Z",
						},
					},
				},
			},
			wantStatus:         "Succeeded",
			wantCompletionTime: "2026-04-03T12:15:00Z",
		},
		{
			name: "TransitionTime in Failed condition",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "Failed",
							"status":             "True",
							"lastTransitionTime": "2026-04-03T12:30:00Z",
						},
					},
				},
			},
			wantStatus:         "Failed",
			wantCompletionTime: "2026-04-03T12:30:00Z",
		},
		{
			name: "Top-level completionTime wins over condition",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"completionTime": "2026-04-03T12:00:00Z",
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "Succeeded",
							"status":             "True",
							"lastTransitionTime": "2026-04-03T12:15:00Z",
						},
					},
				},
			},
			wantStatus:         "Succeeded",
			wantCompletionTime: "2026-04-03T12:00:00Z",
		},
		{
			name: "Running (no completion time)",
			obj: map[string]interface{}{
				"spec": map[string]interface{}{
					"suspend": false,
				},
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Active",
							"status": "True",
						},
					},
				},
			},
			wantStatus:         "Running",
			wantCompletionTime: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotCompletionTime := orc.parseJobStatus(tt.obj)
			if gotStatus != tt.wantStatus {
				t.Errorf("parseJobStatus() gotStatus = %v, want %v", gotStatus, tt.wantStatus)
			}
			if gotCompletionTime != tt.wantCompletionTime {
				t.Errorf("parseJobStatus() gotCompletionTime = %v, want %v", gotCompletionTime, tt.wantCompletionTime)
			}
		})
	}
}

func TestParseKueueWorkloadStatus(t *testing.T) {
	g := &GKEOrchestrator{}

	tests := []struct {
		name       string
		obj        map[string]interface{}
		wantStatus string
	}{
		{
			name: "Admitted",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "Admitted",
							"status":             "True",
							"lastTransitionTime": "2026-04-13T07:00:00Z",
						},
					},
				},
			},
			wantStatus: "Admitted",
		},
		{
			name: "QuotaReserved",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "QuotaReserved",
							"status":             "True",
							"lastTransitionTime": "2026-04-13T07:00:00Z",
						},
					},
				},
			},
			wantStatus: "QuotaReserved",
		},
		{
			name: "Evicted",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "Evicted",
							"status":             "True",
							"lastTransitionTime": "2026-04-13T07:00:00Z",
						},
					},
				},
			},
			wantStatus: "Evicted",
		},
		{
			name: "LatestConditionTakesPrecedence",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "QuotaReserved",
							"status":             "True",
							"lastTransitionTime": "2026-04-13T07:00:00Z",
						},
						map[string]interface{}{
							"type":               "Admitted",
							"status":             "True",
							"lastTransitionTime": "2026-04-13T07:05:00Z",
						},
					},
				},
			},
			wantStatus: "Admitted",
		},
		{
			name: "UnknownIfNoTrueConditions",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":               "Admitted",
							"status":             "False",
							"lastTransitionTime": "2026-04-13T07:05:00Z",
						},
					},
				},
			},
			wantStatus: "Unknown",
		},
		{
			name: "UnknownIfNoConditions",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{},
				},
			},
			wantStatus: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.parseKueueWorkloadStatus(tt.obj)
			if got != tt.wantStatus {
				t.Errorf("parseKueueWorkloadStatus() = %v, want %v", got, tt.wantStatus)
			}
		})
	}
}

func TestGenerateGKEManifest_DynamicVmsPerSlice(t *testing.T) {
	orc := NewGKEOrchestrator()
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe ct6e-standard-8t": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 8, "guestAcceleratorType": "tpu-v6e-slice"}]}`},
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 8, "guestAcceleratorType": "tpu-v6e-slice"}]}`},
		},
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
	})
	orc.SetExecutor(mockExec)

	job := orchestrator.JobDefinition{
		WorkloadName:    "dynamic-vms-test",
		CommandToRun:    "echo hello",
		ClusterLocation: "us-central1-a",
		AcceleratorType: "v6e-8",
		Topology:        "16x16",
		VmsPerSlice:     0,
	}

	opts, profile, err := orc.PrepareManifestOptions(job, "test-image:latest")
	if err != nil {
		t.Fatalf("prepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)

	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	expectedParallelism := "parallelism: 32"
	expectedCompletions := "completions: 32"

	if !strings.Contains(manifest, expectedParallelism) {
		t.Errorf("manifest missing expected parallelism %q\nManifest: %s", expectedParallelism, manifest)
	}
	if !strings.Contains(manifest, expectedCompletions) {
		t.Errorf("manifest missing expected completions %q\nManifest: %s", expectedCompletions, manifest)
	}
}

func TestGenerateGKEManifest_RespectUserVmsPerSlice(t *testing.T) {
	orc := NewGKEOrchestrator()
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe ct6e-standard-8t": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 8, "guestAcceleratorType": "tpu-v6e-slice"}]}`},
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 8, "guestAcceleratorType": "tpu-v6e-slice"}]}`},
		},
	})
	orc.SetExecutor(mockExec)

	job := orchestrator.JobDefinition{
		WorkloadName:    "user-vms-test",
		CommandToRun:    "echo hello",
		ClusterLocation: "us-central1-a",
		AcceleratorType: "v6e-8",
		Topology:        "16x16",
		VmsPerSlice:     1, // Explicitly set to 1
	}

	opts, profile, err := orc.PrepareManifestOptions(job, "test-image:latest")
	if err != nil {
		t.Fatalf("prepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	expectedParallelism := "parallelism: 1"
	expectedCompletions := "completions: 1"

	if !strings.Contains(manifest, expectedParallelism) {
		t.Errorf("manifest missing expected parallelism %q\nManifest: %s", expectedParallelism, manifest)
	}
	if !strings.Contains(manifest, expectedCompletions) {
		t.Errorf("manifest missing expected completions %q\nManifest: %s", expectedCompletions, manifest)
	}
}

func TestResolveTopologyForChips(t *testing.T) {
	orc := &GKEOrchestrator{}

	tests := []struct {
		name       string
		prefix     string
		totalChips int
		wantShape  string
		wantErr    bool
	}{
		{
			name:       "v4 4 chips",
			prefix:     "v4",
			totalChips: 4,
			wantShape:  "2x2x1",
			wantErr:    false,
		},
		{
			name:       "tpu7x 2048 chips",
			prefix:     "tpu7x",
			totalChips: 2048,
			wantShape:  "8x16x16",
			wantErr:    false,
		},
		{
			name:       "v6e 1 chip",
			prefix:     "v6e",
			totalChips: 1,
			wantShape:  "1x1",
			wantErr:    false,
		},
		{
			name:       "v6e 256 chips",
			prefix:     "v6e",
			totalChips: 256,
			wantShape:  "16x16",
			wantErr:    false,
		},
		{
			name:       "tpu7x 1 chip (Fail)",
			prefix:     "tpu7x",
			totalChips: 1,
			wantShape:  "",
			wantErr:    true,
		},
		{
			name:       "v4 3 chips (Fail)",
			prefix:     "v4",
			totalChips: 3,
			wantShape:  "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := orc.resolveTopologyForChips(tt.prefix, tt.totalChips)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveTopologyForChips() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantShape {
				t.Errorf("resolveTopologyForChips() got = %v, want %v", got, tt.wantShape)
			}
		})
	}
}

func TestConfigureClusterEnvironment_AutoCreateQueues(t *testing.T) {
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipeRead.Close()
	defer pipeWrite.Close()

	origStdin := os.Stdin
	os.Stdin = pipeRead
	defer func() { os.Stdin = origStdin }()

	if _, err := pipeWrite.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}

	responses := map[string][]shell.CommandResult{
		"kubectl get localqueue default-queue -n default": {
			{ExitCode: 1, Stderr: "Error from server (NotFound): localqueues.kueue.x-k8s.io \"default-queue\" not found"},
		},
		"kubectl apply -f": {
			{ExitCode: 0, Stdout: "resourceflavor.kueue.x-k8s.io/flavor-default created"},
			{ExitCode: 0, Stdout: "clusterqueue.kueue.x-k8s.io/cluster-queue created"},
			{ExitCode: 0, Stdout: "localqueue.kueue.x-k8s.io/default-queue created"},
		},
		"kubectl get localqueue default-queue -n default -o jsonpath={.spec.clusterQueue}": {
			{ExitCode: 0, Stdout: "cluster-queue"},
		},
		"kubectl get clusterqueue cluster-queue -o json": {
			{ExitCode: 0, Stdout: "{\"apiVersion\":\"kueue.x-k8s.io/v1beta2\",\"kind\":\"ClusterQueue\",\"spec\":{\"resourceGroups\":[{\"coveredResources\":[\"cpu\"]}]}}"},
		},
		"kubectl patch clusterqueue cluster-queue": {
			{ExitCode: 0, Stdout: "clusterqueue.kueue.x-k8s.io/cluster-queue patched"},
		},
	}

	mockExec := NewMockExecutor(responses)
	orc := &GKEOrchestrator{
		executor: mockExec,
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-default": {CPUs: 30},
			},
		},
	}

	job := &orchestrator.JobDefinition{
		KueueQueueName: "default-queue",
	}

	err = orc.configureClusterEnvironment(job)
	if err != nil {
		t.Fatalf("configureClusterEnvironment failed: %v", err)
	}

	// Verify calls
	if mockExec.callCount["kubectl apply -f"] != 3 {
		t.Errorf("Expected 3 calls to kubectl apply -f, got %d", mockExec.callCount["kubectl apply -f"])
	}
}

func TestResolveKueueQueue(t *testing.T) {
	tests := []struct {
		name          string
		requestedName string
		kubectlOutput string
		wantName      string
		wantErr       bool
	}{
		{
			name:          "User requested name",
			requestedName: "custom-q",
			kubectlOutput: "",
			wantName:      "custom-q",
			wantErr:       false,
		},
		{
			name:          "No queues found, fallback to multislice-queue",
			requestedName: "",
			kubectlOutput: "",
			wantName:      "multislice-queue",
			wantErr:       false,
		},
		{
			name:          "Single queue found, auto-discover",
			requestedName: "",
			kubectlOutput: "queue-1",
			wantName:      "queue-1",
			wantErr:       false,
		},
		{
			name:          "Multiple queues found, hard fail",
			requestedName: "",
			kubectlOutput: "queue-1 queue-2",
			wantName:      "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses := map[string][]shell.CommandResult{
				"kubectl get localqueue -n default -o jsonpath={.items[*].metadata.name}": {
					{ExitCode: 0, Stdout: tt.kubectlOutput},
				},
			}
			mockExec := NewMockExecutor(responses)
			orc := &GKEOrchestrator{executor: mockExec}

			got, err := orc.resolveKueueQueue(tt.requestedName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveKueueQueue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantName {
				t.Errorf("resolveKueueQueue() got = %v, want %v", got, tt.wantName)
			}
		})
	}
}

func TestGPUTopologyAwareScheduling(t *testing.T) {
	job := orchestrator.JobDefinition{
		WorkloadName:    "gpu-test-job",
		AcceleratorType: "nvidia-tesla-a100",
		GKEScheduler:    "gke.io/topology-aware-auto",
		NumSlices:       1,
		VmsPerSlice:     1,
	}

	orc := NewGKEOrchestrator()
	orc.clusterDesc.NodePools = []gkeJobNodePool{{Name: "default-pool"}}

	opts, profile, err := orc.PrepareManifestOptions(job, "test-image:latest")
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	if opts.SchedulingGates == "" {
		t.Errorf("Expected SchedulingGates to be populated, got empty string")
	}

	if opts.SchedulerName != "" {
		t.Errorf("Expected SchedulerName to be empty, got %v", opts.SchedulerName)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	if !strings.Contains(manifest, "schedulingGates:") {
		t.Errorf("Rendered manifest does not contain schedulingGates")
	}
	if !strings.Contains(manifest, "gke.io/topology-aware-auto-gpu-test-job") {
		t.Errorf("Rendered manifest does not contain correct gate name")
	}
	if strings.Contains(manifest, "schedulerName: gke.io/topology-aware-auto") {
		t.Errorf("Rendered manifest unexpectedly contains schedulerName")
	}
}
