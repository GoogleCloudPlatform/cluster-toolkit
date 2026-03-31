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

func TestGenerateGKEManifest_Accelerators(t *testing.T) {

	tests := []struct {
		name            string
		acceleratorType string
		cpuLimit        string
		memoryLimit     string
		gpuLimit        string
		tpuLimit        string
		wantLabels      []string // Labels that should be in the output
		wantLimits      []string // Limits that should be in the output,
		dontWantLimits  []string // Limits that should NOT be in the output
		wantErr         bool     // Whether GenerateGKEManifest should return an error
	}{
		{
			name:            "A3 Mega (H100)",
			acceleratorType: "nvidia-h100-mega-80gb",
			cpuLimit:        "",  // Omitted
			memoryLimit:     "",  // Omitted
			gpuLimit:        "1", // NVIDIA fallback default
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-h100-mega-80gb"},
			wantLimits:      []string{"nvidia.com/gpu: 1"},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "A4X Max (GB200)",
			acceleratorType: "nvidia-gb200",
			cpuLimit:        "",
			memoryLimit:     "",
			gpuLimit:        "1",
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-gb200"},
			wantLimits:      []string{"nvidia.com/gpu: 1"},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "G2 (L4)",
			acceleratorType: "nvidia-l4",
			cpuLimit:        "",
			memoryLimit:     "",
			gpuLimit:        "1",
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-l4"},
			wantLimits:      []string{"nvidia.com/gpu: 1"},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "TPU v6e slice",
			acceleratorType: "tpu-v6e-slice",
			cpuLimit:        "",
			memoryLimit:     "",
			tpuLimit:        "4",
			wantLabels:      []string{"cloud.google.com/gke-tpu-accelerator: tpu-v6e-slice"},
			wantLimits:      []string{"google.com/tpu: 4"},
			dontWantLimits:  []string{"nvidia.com/gpu", "cpu:", "memory:"},
		},
		{
			name:            "CPU Only (Default)",
			acceleratorType: "",
			wantErr:         true, // Empty accelerator is no longer allowed
		},
		{
			name:            "Fallback NVIDIA",
			acceleratorType: "nvidia-unknown-new-gpu",
			cpuLimit:        "",
			memoryLimit:     "",
			gpuLimit:        "1",
			wantLabels:      []string{"cloud.google.com/gke-accelerator: nvidia-unknown-new-gpu"},
			wantLimits:      []string{"nvidia.com/gpu: 1"},
			dontWantLimits:  []string{"google.com/tpu", "cpu:", "memory:"},
		},
		{
			name:            "Uniform CPU Machine via Accelerator Flag (Empty Zone / Strict Fail)",
			acceleratorType: "n2-standard-4",
			cpuLimit:        "",
			memoryLimit:     "",
			wantLabels:      []string{},
			wantLimits:      []string{},
			dontWantLimits:  []string{},
			wantErr:         true, // Expect failure if zone is empty!
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := orchestrator.JobDefinition{
				WorkloadName:    "test-workload",
				CommandToRun:    "echo hello",
				AcceleratorType: tt.acceleratorType,
			}

			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
			}
			orc := &GKEOrchestrator{executor: NewMockExecutor(mockResponses)}

			opts, profile, err := orc.prepareManifestOptions(job, "test-image:latest")
			if err != nil {
				t.Fatalf("prepareManifestOptions failed: %v", err)
			}
			// prepareManifestOptions doesn't set limits in opts (GenerateGKEManifest does),
			// but it sets NodeSelector string which is key for labels.

			manifest, err := orc.GenerateGKEManifest(opts, profile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateGKEManifest returned error %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return // Continue to next test case if error was expected and received!
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
	orc, _ := NewGKEOrchestrator()
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-4": {
			{ExitCode: 0, Stdout: `{"guestCpus": 4}`},
		},
	})
	orc.SetExecutor(mockExec)
	job := orchestrator.JobDefinition{
		WorkloadName:    "volume-test",
		CommandToRun:    "echo hello",
		ClusterLocation: "us-central1-a",
		AcceleratorType: "n2-standard-4", // Required for strict enforcement
		Volumes: []orchestrator.VolumeDefinition{
			{Name: "vol-0", Source: "gs://my-bucket", MountPath: "/data", Type: "gcsfuse"},
			{Name: "vol-1", Source: "/host/path", MountPath: "/host", Type: "hostPath"},
			{Name: "vol-2", Source: "my-pvc", MountPath: "/pvc", Type: "pvc"},
		},
	}

	opts, profile, err := orc.prepareManifestOptions(job, "test-image:latest")
	if err != nil {
		t.Fatalf("prepareManifestOptions failed: %v", err)
	}

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
	orc, _ := NewGKEOrchestrator()
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

	// We expect the command to be properly escaped in the JSON/YAML array syntax used in the manifest
	expectedSubStr := `command: ["/bin/bash","-c","python -c \"print('hello')\" && echo \"world\""]`
	if !strings.Contains(manifest, expectedSubStr) {
		t.Errorf("manifest command string is not properly escaped.\nExpected substring: %s\nActual manifest section:\n%s", expectedSubStr, manifest)
	}
}

func TestInjectTolerations(t *testing.T) {
	orc, _ := NewGKEOrchestrator()

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
		WorkloadName: "pathways-test",
		CommandToRun: "echo hello",
		NumSlices:    2,
		Pathways: orchestrator.PathwaysJobDefinition{
			ProxyServerImage: "proxy:latest",
			ServerImage:      "server:latest",
			WorkerImage:      "worker:latest",
			GCSLocation:      "gs://my-bucket",
		},
	}

	orc, _ := NewGKEOrchestrator()
	manifest, err := orc.generatePathwaysManifest(job, "test-image:latest")
	if err != nil {
		t.Fatalf("generatePathwaysManifest failed: %v", err)
	}

	if !strings.Contains(manifest, "name: pathways-test") {
		t.Errorf("manifest does not contain correct workload name")
	}

	if !strings.Contains(manifest, "replicas: 2") {
		t.Errorf("manifest does not contain correct number of replicas")
	}

	if !strings.Contains(manifest, "image: proxy:latest") {
		t.Errorf("manifest does not contain correct proxy image")
	}

	if !strings.Contains(manifest, "--gcs_location=gs://my-bucket") {
		t.Errorf("manifest does not contain correct GCS location")
	}
}

func TestWaitForJobCompletion(t *testing.T) {
	workloadName := "test-workload"
	clusterName := "test-cluster"
	clusterLocation := "us-central1-a"
	projectID := "test-project"

	tests := []struct {
		name          string
		mockResponses map[string][]shell.CommandResult
		expectedError string
	}{
		{
			name: "Successful completion",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for jsonpath={.status.conditions[-1].type}=Finished jobset test-workload --timeout=1h": {
					{ExitCode: 0, Stdout: "jobset.jobset.x-k8s.io/test-workload condition met"},
				},
				"kubectl get jobset test-workload -o jsonpath={.status.conditions[-1].type}": {
					{ExitCode: 0, Stdout: "Completed"},
				},
			},
			expectedError: "",
		},
		{
			name: "Job timeout",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for jsonpath={.status.conditions[-1].type}=Finished jobset test-workload --timeout=1h": {
					{ExitCode: 1, Stderr: "timed out waiting for conditions to be met"},
				},
			},
			expectedError: "job timed out",
		},
		{
			name: "Job finished but not completed",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for jsonpath={.status.conditions[-1].type}=Finished jobset test-workload --timeout=1h": {
					{ExitCode: 0, Stdout: "jobset.jobset.x-k8s.io/test-workload condition met"},
				},
				"kubectl get jobset test-workload -o jsonpath={.status.conditions[-1].type}": {
					{ExitCode: 0, Stdout: "Failed"},
				},
			},
			expectedError: "job completed unsuccessfully with status: Failed",
		},
		{
			name: "Error during kubectl wait",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for jsonpath={.status.conditions[-1].type}=Finished jobset test-workload --timeout=1h": {
					{ExitCode: 1, Stderr: "some kubectl error"},
				},
			},
			expectedError: "error waiting for job completion: some kubectl error\n",
		},
		{
			name: "Error during kubectl get status",
			mockResponses: map[string][]shell.CommandResult{
				"kubectl wait --for jsonpath={.status.conditions[-1].type}=Finished jobset test-workload --timeout=1h": {
					{ExitCode: 0, Stdout: "jobset.jobset.x-k8s.io/test-workload condition met"},
				},
				"kubectl get jobset test-workload -o jsonpath={.status.conditions[-1].type}": {
					{ExitCode: 1, Stderr: "some get status error"},
				},
			},
			expectedError: "failed to get final job status: some get status error\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			orc := &GKEOrchestrator{executor: mockExecutor}

			err := orc.waitForJobCompletion(workloadName, clusterName, clusterLocation, projectID)

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

func TestParseAcceleratorOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantAccel string
		wantErr   bool
	}{
		{
			name:      "Single Accelerator (Success)",
			output:    "nvidia-l4",
			wantAccel: "nvidia-l4",
			wantErr:   false,
		},
		{
			name:      "Multiple Accelerators (Failure)",
			output:    "nvidia-l4\ntpu-v6e-slice",
			wantAccel: "",
			wantErr:   true,
		},
		{
			name:      "Empty Output (Success, CPU Default)",
			output:    "",
			wantAccel: "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{}
			got, err := orc.parseAcceleratorOutput(tt.output)

			if (err != nil) != tt.wantErr {
				t.Fatalf("parseAcceleratorOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantAccel {
				t.Errorf("parseAcceleratorOutput() got = %v, want %v", got, tt.wantAccel)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "Multiple Accelerator Types found") {
				t.Errorf("Expected error message to contain 'Multiple Accelerator Types found', got %v", err)
			}
		})
	}
}
