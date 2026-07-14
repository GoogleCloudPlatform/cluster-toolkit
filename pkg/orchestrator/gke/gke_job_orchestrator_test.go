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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"os"
	"strings"
	"testing"
)

func setupMockMachineConfig(t *testing.T) {
	mockJSON := `{
	  "cpus": {
	    "custom-c2-60": {"count": 60, "memoryMb": 240000},
	    "t2a-custom-16": {"count": 16, "memoryMb": 64000},
	    "c2-standard-60": {"count": 60, "memoryMb": 240000},
	    "nvidia-tesla-a100": {"count": 12, "memoryMb": 48000},
	    "nvidia-h100-mega-80gb": {"count": 80, "memoryMb": 320000},
	    "nvidia-gb200": {"count": 96, "memoryMb": 384000},
	    "nvidia-l4": {"count": 12, "memoryMb": 48000},
	    "tpu-v6e-slice": {"count": 8, "memoryMb": 32768},
	    "nvidia-unknown-new-gpu": {"count": 8, "memoryMb": 32000},
	    "n2-standard-2": {"count": 2, "memoryMb": 8000},
	    "n2-standard-4": {"count": 4, "memoryMb": 16000},
	    "g2-standard-48": {"count": 48, "memoryMb": 192000},
	    "ct6e-standard-8t": {"count": 8, "memoryMb": 32768}
	  },
	  "gpus": {
	    "nvidia-tesla-a100": {"count": 1, "type": "nvidia-tesla-a100"},
	    "nvidia-h100-mega-80gb": {"count": 1, "type": "nvidia-h100-mega-80gb"},
	    "nvidia-gb200": {"count": 1, "type": "nvidia-gb200"},
	    "nvidia-l4": {"count": 1, "type": "nvidia-l4"},
	    "nvidia-unknown-new-gpu": {"count": 1, "type": "nvidia-unknown-new-gpu"},
	    "g2-standard-48": {"count": 4, "type": "nvidia-l4"}
	  },
	  "tpus": {
	    "tpu-v6e-slice": {"count": 4},
	    "ct6e-standard-8t": {"count": 8}
	  }
	}`
	config.ClearMachineTypeCache()
	os.Setenv("GHPC_MOCK_MACHINE_CONFIG", mockJSON)
	t.Cleanup(func() {
		os.Unsetenv("GHPC_MOCK_MACHINE_CONFIG")
	})
}

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

func newTestGKEOrchestrator(executor Executor) *GKEOrchestrator {
	return &GKEOrchestrator{
		executor:                 executor,
		machineTypeClient:        &MockMachineTypeClient{Executor: executor},
		acceleratorToMachineType: make(map[string]string),
		machineCapCache:          make(map[string]MachineTypeCap),
		topologyCache:            make(map[string]string),
		dynamicSlicingCache:      make(map[string]bool),
		staticSlicingCache:       make(map[string]bool),
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

	return shell.CommandResult{
		ExitCode: 1,
		Stderr:   fmt.Sprintf("mock error: unexpected command: %s", cmdKey),
	}
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

func (m *MockKubeClient) ListJobSets(labelSelector string) ([]orchestrator.JobStatus, error) {
	return []orchestrator.JobStatus{}, m.Err
}

func TestGenerateGKEManifest_Accelerators(t *testing.T) {
	setupMockMachineConfig(t)

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
				ComputeType:     tt.acceleratorType,
				ClusterLocation: "us-central1-a",
			}

			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: "16x16"}},
				"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json":                                {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
				"gcloud compute machine-types describe nvidia-h100-mega-80gb --zone=us-central1-a --format=json":                        {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-h100-mega-80gb"}]}`}},
				"gcloud compute machine-types describe nvidia-gb200 --zone=us-central1-a --format=json":                                 {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-gb200"}]}`}},
				"gcloud compute machine-types describe nvidia-l4 --zone=us-central1-a --format=json":                                    {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-l4"}]}`}},
				"gcloud compute machine-types describe tpu-v6e-slice --zone=us-central1-a --format=json":                                {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v6e-slice"}]}`}},
				"gcloud compute machine-types describe nvidia-unknown-new-gpu --zone=us-central1-a --format=json":                       {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-unknown-new-gpu"}]}`}},
			}
			mockExec := NewMockExecutor(mockResponses)
			orc := newTestGKEOrchestrator(mockExec)
			orc.projectID = "mock-project"
			orc.clusterDesc.NodePools = []gkeJobNodePool{
				{Config: gkeNodePoolConfig{MachineType: "nvidia-h100-mega-80gb"}},
				{Config: gkeNodePoolConfig{MachineType: "nvidia-gb200"}},
				{Config: gkeNodePoolConfig{MachineType: "nvidia-l4"}},
				{Config: gkeNodePoolConfig{MachineType: "tpu-v6e-slice"}},
				{Config: gkeNodePoolConfig{MachineType: "nvidia-unknown-new-gpu"}},
				{Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
			}

			profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
			var manifest string
			if err == nil {
				var opts ManifestOptions
				opts, err = orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
				if err == nil {
					manifest, err = orc.GenerateGKEManifest(opts, profile)
				}
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
	setupMockMachineConfig(t)
	orc := NewGKEOrchestrator()
	orc.projectID = "mock-project"
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-4 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"guestCpus": 4, "memoryMb": 16384}`},
		},
	})
	orc.SetExecutor(mockExec)
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Config: gkeNodePoolConfig{MachineType: "n2-standard-4"}},
	}
	job := orchestrator.JobDefinition{
		WorkloadName:    "volume-test",
		CommandToRun:    "echo hello",
		ClusterLocation: "us-central1-a",
		ComputeType:     "n2-standard-4",
		RawMounts: []string{
			"gs://my-bucket:/data",
			"/host/path:/host",
			"my-pvc:/pvc",
		},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("prepareManifestOptions failed: %v", err)
	}

	opts.ComputeType = "n2-standard-4"
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
	setupMockMachineConfig(t)
	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe nvidia-l4 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1}]}`},
		},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Config: gkeNodePoolConfig{MachineType: "nvidia-l4"}},
	}
	opts := ManifestOptions{
		WorkloadName:    "test-workload",
		FullImageName:   "test-image:latest",
		CommandToRun:    `python -c "print('hello')"` + " && echo \"world\"",
		ComputeType:     "nvidia-l4",
		MachineType:     "nvidia-l4",
		ClusterLocation: "us-central1-a",
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
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-test",
		CommandToRun:    "echo hello",
		NumSlices:       2,
		ClusterLocation: "us-central1",
		ComputeType:     "n2-standard-2",
		Pathways: orchestrator.PathwaysJobDefinition{
			ProxyServerImage: "proxy:latest",
			ServerImage:      "server:latest",
			WorkerImage:      "worker:latest",
			GCSLocation:      "gs://my-bucket",
			HeadNodePool:     "pathways-np",
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
	}
	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("generatePathwaysManifest failed: %v", err)
	}

	err = os.WriteFile("gcluster_pathways_manifest.yaml", []byte(manifest), 0644)
	if err != nil {
		t.Fatalf("failed to write manifest to file: %v", err)
	}
	defer os.Remove("gcluster_pathways_manifest.yaml")

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
		"restartStrategy: BlockingRecreate",
		"privileged: true",
		"alpha.jobset.sigs.k8s.io/exclusive-topology: cloud.google.com/gke-nodepool",
		`cpu: "24"`,
		`cpu: "2"`,
		`memory: "8Gi"`,
		"kill -SIGTERM $PID",
		"echo \"Exit code: $EXIT_CODE\"",
		"name: shared-tmp",
		"hostPath:",
		"path: /tmp",
		"type: DirectoryOrCreate",
		"mountPath: /tmp",
		"jobset.sigs.k8s.io/hack: \"true\"",
		"kueue.x-k8s.io/safe-to-forcefully-delete: \"true\"",
		"cloud.google.com/skip-tpu-webhook-check: \"true\"",
		"backoffLimitPerIndex: 4000",
		"podReplacementPolicy: Failed",
		"maxFailedIndexes: 0",
	}

	for _, substr := range expectedSubstrs {
		if !strings.Contains(manifest, substr) {
			t.Errorf("manifest missing expected substring %q", substr)
		}
	}
}

func TestGeneratePathwaysManifest_MTC(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-mtc-test",
		CommandToRun:    "echo hello",
		NumSlices:       2,
		ClusterLocation: "us-central1",
		ComputeType:     "n2-standard-2",
		Pathways: orchestrator.PathwaysJobDefinition{
			ProxyServerImage:            "proxy:latest",
			ServerImage:                 "server:latest",
			WorkerImage:                 "worker:latest",
			ColocatedPythonSidecarImage: "sidecar:latest",
			GCSLocation:                 "gs://my-bucket",
			HeadNodePool:                "pathways-np",
			MTCEnabled:                  true,
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
	}
	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("generatePathwaysManifest failed: %v", err)
	}

	expectedSubstrs := []string{
		"name: pathways-mtc-test",
		"colocated-python-sidecar",
		"restartPolicy: Always",
		"driver: multitier-checkpoint.csi.storage.gke.io",
		"name: sidecar-shared-memory",
		"medium: Memory",
		"--cloud_pathways_sidecar_shm_directory=/tmp/sidecar",
		"mountPath: /tmp/mtc_checkpoints",
	}

	for _, substr := range expectedSubstrs {
		if !strings.Contains(manifest, substr) {
			t.Errorf("manifest missing expected substring %q", substr)
		}
	}

	parts := strings.Split(manifest, "name: worker")
	if len(parts) < 2 {
		t.Fatalf("failed to split manifest into head and worker parts")
	}
	headPart := parts[0]
	workerPart := parts[1]

	if strings.Contains(headPart, "colocated-python-sidecar") {
		t.Errorf("headPart should not contain colocated-python-sidecar container")
	}
	if !strings.Contains(workerPart, "colocated-python-sidecar") {
		t.Errorf("workerPart should contain colocated-python-sidecar container")
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
			orc := newTestGKEOrchestrator(mockExecutor)
			orc.kubeClient = mockKube

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
	setupMockMachineConfig(t)
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
			orc := newTestGKEOrchestrator(mockExecutor)
			orc.projectID = "mock-project"

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
	setupMockMachineConfig(t)

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
					MachineType: "custom-c2-60",
					AdvancedMachineFeatures: &gkeAdvancedMachineFeatures{
						ThreadsPerCore: "1",
					},
				},
				InitialNodeCount: 1,
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe custom-c2-60 --zone=us-central1-a --format=json": {
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
					MachineType: "custom-c2-60",
					AdvancedMachineFeatures: &gkeAdvancedMachineFeatures{
						ThreadsPerCore: "2",
					},
				},
				InitialNodeCount: 1,
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe custom-c2-60 --zone=us-central1-a --format=json": {
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
					MachineType: "t2a-custom-16",
					AdvancedMachineFeatures: &gkeAdvancedMachineFeatures{
						ThreadsPerCore: "1",
					},
				},
				InitialNodeCount: 1,
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute machine-types describe t2a-custom-16 --zone=us-central1-a --format=json": {
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
			orc := newTestGKEOrchestrator(mockExecutor)
			orc.projectID = "mock-project"
			orc.machineTypeToThreadsPerCore = make(map[string]string)
			if tt.np.Config.AdvancedMachineFeatures != nil {
				orc.machineTypeToThreadsPerCore[tt.np.Config.MachineType] = tt.np.Config.AdvancedMachineFeatures.ThreadsPerCore
			}

			cpus, _, _, _, _, _, _, err := orc.processNodePoolCapacity(tt.np, "us-central1-a")
			t.Logf("Test case %s: cpus=%d, err=%v", tt.name, cpus, err)

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
			orc := newTestGKEOrchestrator(nil)
			orc.clusterDesc.NodePools = tt.nodePools

			got := orc.autoDetectCPUNodePool()
			if got != tt.wantPool {
				t.Errorf("autoDetectCPUNodePool() = %v, want %v", got, tt.wantPool)
			}
		})
	}
}

func TestDetermineIfCPUMachine_Hyperthreading(t *testing.T) {
	setupMockMachineConfig(t)

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
				ComputeType:     "c2-standard-60",
				MachineType:     "c2-standard-60",
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
				ComputeType:     "c2-standard-60",
				MachineType:     "c2-standard-60",
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
			orc := newTestGKEOrchestrator(mockExecutor)

			orc.machineTypeToThreadsPerCore = make(map[string]string)
			if tt.threadsPerCore != "" {
				orc.machineTypeToThreadsPerCore[tt.job.ComputeType] = tt.threadsPerCore
			}
			orc.machineCapCache = make(map[string]MachineTypeCap)

			isCPU, capacity, err := orc.determineIfCPUMachine(&tt.job)

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
						{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`},
					}
				}
			}
			mockExecutor := NewMockExecutor(tt.mockResponses)
			orc := newTestGKEOrchestrator(mockExecutor)
			orc.clusterDesc.NodePools = tt.nodePools

			got, err := orc.verifyDynamicSlicingActive(tt.opts)

			if (err != nil) != tt.wantErr {
				t.Errorf("verifySuperSlicingActive() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantResult {
				t.Errorf("Expected %t, got %t", tt.wantResult, got)
			}
		})
	}
}

func TestGenerateGKEManifest_Verbose_GPU(t *testing.T) {
	setupMockMachineConfig(t)

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe nvidia-l4 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1}]}`},
		},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Config: gkeNodePoolConfig{MachineType: "nvidia-l4"}},
	}
	opts := ManifestOptions{
		WorkloadName:    "test-workload",
		FullImageName:   "test-image:latest",
		CommandToRun:    "python app.py",
		ComputeType:     "nvidia-l4",
		MachineType:     "nvidia-l4",
		ClusterLocation: "us-central1-a",
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
	setupMockMachineConfig(t)

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe tpu-v6e-slice --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4}]}`},
		},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Config: gkeNodePoolConfig{MachineType: "tpu-v6e-slice"}},
	}
	opts := ManifestOptions{
		WorkloadName:    "test-workload",
		FullImageName:   "test-image:latest",
		CommandToRun:    "python app.py",
		ComputeType:     "tpu-v6e-slice",
		MachineType:     "tpu-v6e-slice",
		ClusterLocation: "us-central1-a",
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
			gotStatus, gotCompletionTime := parseJobStatus(tt.obj)
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
	g := newTestGKEOrchestrator(nil)

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
	setupMockMachineConfig(t)

	orc := NewGKEOrchestrator()
	orc.projectID = "mock-project"
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe ct6e-standard-8t --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 8, "guestAcceleratorType": "tpu-v6e-slice"}]}`},
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 8, "guestAcceleratorType": "tpu-v6e-slice"}]}`},
		},
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
	})
	orc.SetExecutor(mockExec)
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}

	job := orchestrator.JobDefinition{
		WorkloadName:    "dynamic-vms-test",
		CommandToRun:    "echo hello",
		ClusterLocation: "us-central1-a",
		ComputeType:     "v6e-8",
		Topology:        "16x16",
		NodesPerSlice:   0,
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
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

func TestGenerateGKEManifest_RespectUserNumNodes(t *testing.T) {
	setupMockMachineConfig(t)

	orc := NewGKEOrchestrator()
	orc.projectID = "mock-project"
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe g2-standard-12 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-l4"}]}`},
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-l4"}]}`},
		},
	})
	orc.SetExecutor(mockExec)
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}

	job := orchestrator.JobDefinition{
		WorkloadName:    "user-nodes-test",
		CommandToRun:    "echo hello",
		ClusterLocation: "us-central1-a",
		ComputeType:     "l4-1", // Non-TPU job (resolves to g2-standard-12)
		NodesPerSlice:   32,     // Explicitly set to 32 (representing --num-nodes)
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
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

func TestGenerateGKEManifest_ParallelContainers(t *testing.T) {
	setupMockMachineConfig(t)

	orc := NewGKEOrchestrator()
	orc.projectID = "mock-project"
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v7x-slice"}]}`},
		},
	})
	orc.SetExecutor(mockExec)
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}

	job := orchestrator.JobDefinition{
		WorkloadName:          "parallel-container-test",
		CommandToRun:          "echo hello",
		ClusterLocation:       "us-central1-a",
		ComputeType:           "tpu7x", // Resolves to tpu7x-standard-4t
		Topology:              "2x2x1", // 4 chips
		UseParallelContainers: true,    // Enable parallel containers
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	// Verify that we have two containers!
	if !strings.Contains(manifest, "name: workload-container-1") {
		t.Errorf("manifest missing expected container name %q\nManifest: %s", "workload-container-1", manifest)
	}
	if !strings.Contains(manifest, "name: workload-container-2") {
		t.Errorf("manifest missing expected container name %q\nManifest: %s", "workload-container-2", manifest)
	}

	// Verify that resources are split!
	// Total chips = 4. Parallel containers = 2. So each should get 2 chips!
	expectedResources := "google.com/tpu: \"2\""
	if strings.Count(manifest, expectedResources) != 2 {
		t.Errorf("expected %d occurrences of %q, got %d\nManifest: %s", 2, expectedResources, strings.Count(manifest, expectedResources), manifest)
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
	orc := newTestGKEOrchestrator(mockExec)
	orc.capacity = ClusterCapacity{
		Flavors: map[string]FlavorCapacity{
			"flavor-default": {CPUs: 30},
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
			orc := newTestGKEOrchestrator(mockExec)

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
	setupMockMachineConfig(t)

	job := orchestrator.JobDefinition{
		WorkloadName:    "gpu-test-job",
		ComputeType:     "nvidia-tesla-a100",
		GKEScheduler:    "gke.io/topology-aware-auto",
		NumSlices:       1,
		NodesPerSlice:   1,
		ClusterLocation: "us-central1-a",
	}

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe nvidia-tesla-a100 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1}]}`},
		},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "nvidia-tesla-a100"}},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
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

func TestGetNodeCount(t *testing.T) {
	tests := []struct {
		name      string
		locations []string
		np        gkeJobNodePool
		expected  int
	}{
		{
			name:      "Autoscaling disabled, zonal",
			locations: []string{"zone-1"},
			np: gkeJobNodePool{
				InitialNodeCount: 5,
				Autoscaling: gkeAutoscaling{
					Enabled: false,
				},
			},
			expected: 5,
		},
		{
			name:      "Autoscaling disabled, regional",
			locations: []string{"zone-1", "zone-2", "zone-3"},
			np: gkeJobNodePool{
				InitialNodeCount: 5,
				Autoscaling: gkeAutoscaling{
					Enabled: false,
				},
			},
			expected: 15,
		},
		{
			name:      "Autoscaling enabled, uses TotalMaxNodeCount",
			locations: []string{"zone-1", "zone-2", "zone-3"},
			np: gkeJobNodePool{
				InitialNodeCount: 5,
				Autoscaling: gkeAutoscaling{
					Enabled:           true,
					MaxNodeCount:      10,
					TotalMaxNodeCount: 20,
				},
			},
			expected: 20,
		},
		{
			name:      "Autoscaling enabled, falls back to MaxNodeCount (regional)",
			locations: []string{"zone-1", "zone-2"},
			np: gkeJobNodePool{
				InitialNodeCount: 5,
				Autoscaling: gkeAutoscaling{
					Enabled:           true,
					MaxNodeCount:      10,
					TotalMaxNodeCount: 0,
				},
			},
			expected: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				clusterZones: tt.locations,
			}
			result := orc.getNodeCount(tt.np)
			if result != tt.expected {
				t.Errorf("getNodeCount() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestGenerateGKEManifest_DynamicSlicingActive_TPU7x(t *testing.T) {
	setupMockMachineConfig(t)

	job := orchestrator.JobDefinition{
		WorkloadName:    "test-tas-tpu7x-job",
		CommandToRun:    "echo hello",
		ComputeType:     "tpu7x-standard-4t",
		Topology:        "4x4x4",
		ClusterLocation: "us-central1-a",
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors":                   {{ExitCode: 0, Stdout: ""}},
		"kubectl get topologies.kueue.x-k8s.io -o json": {{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`}},
		"kubectl get admissioncheck":                    {{ExitCode: 0, Stdout: `{"items": [{"spec": {"controllerName": "accelerator.gke.io/slice"}}]}`}},
		"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: "8x8x8"}},
		"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json":                            {{ExitCode: 0, Stdout: `{"guestCpus": 8, "memoryMb": 32768, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu7x-standard-4t"}]}`}},
	}

	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{
			Name: "tpu-pool",
			Config: gkeNodePoolConfig{
				MachineType: "tpu7x-standard-4t",
			},
			PlacementPolicy: &gkePlacementPolicy{
				AcceleratorTopologyMode: "PROVISION_ONLY",
			},
		},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	// Crucial check: The CLI resolved it as dynamic slicing because it's a 4x4x4 subset of 8x8x8
	if !isDynamicSlicing {
		t.Fatalf("Expected resolveHardwareRequirements to flag isDynamicSlicing as true")
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	// Because dynamic slicing is true:
	// 1. NodeSelector MUST NOT include the topology strictly (Fix A)
	if strings.Contains(manifest, "cloud.google.com/gke-tpu-topology: 4x4x4") {
		t.Errorf("Expected manifest to NOT contain strict nodeSelector topology, but it was found\nManifest: %s", manifest)
	}

	// 2. Kueue TAS annotations MUST be injected (Fix B)
	if !strings.Contains(manifest, "kueue.x-k8s.io/podset-required-topology: cloud.google.com/gke-tpu-partition-4x4x4-id") {
		t.Errorf("Expected manifest to contain kueue TAS annotation, but it was missing or incorrect\nManifest: %s", manifest)
	}
	if !strings.Contains(manifest, "cloud.google.com/gke-tpu-slice-topology: 4x4x4") {
		t.Errorf("Expected manifest to contain gke-tpu-slice-topology annotation, but it was missing\nManifest: %s", manifest)
	}

	// 3. Exclusive-topology annotation MUST NOT be set for dynamic slicing
	if strings.Contains(manifest, "alpha.jobset.sigs.k8s.io/exclusive-topology") {
		t.Errorf("Expected manifest to NOT contain exclusive-topology annotation for dynamic slicing, but it was found\nManifest: %s", manifest)
	}
}

func TestGeneratePathwaysManifest_DynamicSlicing(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-test",
		CommandToRun:    "echo hello",
		NumSlices:       2,
		ClusterLocation: "us-central1-a",
		ComputeType:     "tpu7x-standard-4t",
		Topology:        "4x4x4",
		Pathways: orchestrator.PathwaysJobDefinition{
			ProxyServerImage: "proxy:latest",
			ServerImage:      "server:latest",
			WorkerImage:      "worker:latest",
			GCSLocation:      "gs://my-bucket",
			HeadNodePool:     "pathways-np",
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors":                   {{ExitCode: 0, Stdout: ""}},
		"kubectl get topologies.kueue.x-k8s.io -o json": {{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`}},
		"kubectl get admissioncheck":                    {{ExitCode: 0, Stdout: `{"items": [{"spec": {"controllerName": "accelerator.gke.io/slice"}}]}`}},
		"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end} -l cloud.google.com/gke-tpu-accelerator=tpu7x": {{ExitCode: 0, Stdout: "8x8x8"}},
		"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json":                                                                          {{ExitCode: 0, Stdout: `{"guestCpus": 8, "memoryMb": 32768, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu7x-standard-4t"}]}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{
			Name: "tpu-pool",
			Config: gkeNodePoolConfig{
				MachineType: "tpu7x-standard-4t",
			},
			PlacementPolicy: &gkePlacementPolicy{
				AcceleratorTopologyMode: "PROVISION_ONLY",
			},
		},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	if !isDynamicSlicing {
		t.Fatalf("Expected isDynamicSlicing to be true")
	}

	manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}

	// 1. Manifest must NOT contain strict NodeSelector topology
	if strings.Contains(manifest, "cloud.google.com/gke-tpu-topology: 4x4x4") {
		t.Errorf("Expected manifest to NOT contain strict nodeSelector topology, but it was found\nManifest: %s", manifest)
	}

	// 2. Kueue TAS annotations must be present under pathways worker replicatedJob template annotations
	expectedSubstrs := []string{
		"kueue.x-k8s.io/podset-slice-required-topology: cloud.google.com/gke-tpu-partition-4x4x4-id",
		"cloud.google.com/gke-tpu-slice-topology: 4x4x4",
	}
	for _, substr := range expectedSubstrs {
		if !strings.Contains(manifest, substr) {
			t.Errorf("manifest missing expected substring %q\nManifest: %s", substr, manifest)
		}
	}
}

func TestGenerateGKEManifest_StaticSlicingActive_v6e(t *testing.T) {
	setupMockMachineConfig(t)

	job := orchestrator.JobDefinition{
		WorkloadName:    "test-tas-v6e-job",
		CommandToRun:    "echo hello",
		ComputeType:     "v6e-8",
		Topology:        "2x2",
		ClusterLocation: "us-central1-a",
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors":                   {{ExitCode: 0, Stdout: ""}, {ExitCode: 0, Stdout: ""}},
		"kubectl get topologies.kueue.x-k8s.io -o json": {{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`}, {ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"}}]}`}},
		"kubectl get nodes":                             {{ExitCode: 0, Stdout: "4x4"}, {ExitCode: 0, Stdout: "4x4"}},
		"gcloud compute machine-types describe ct6e-standard-8t --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 8, "memoryMb": 32768, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v6e-slice"}]}`}},
	}

	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{
			Name: "tpu-pool",
			Config: gkeNodePoolConfig{
				MachineType: "tpu-v6e-slice",
			},
		},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	if isDynamicSlicing {
		t.Fatalf("Expected isDynamicSlicing to be false for v6e")
	}
	if !isStaticSlicing {
		t.Fatalf("Expected resolveHardwareRequirements to flag isStaticSlicing as true")
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	// Because static sub-slicing is true:
	// 1. NodeSelector MUST NOT include the topology strictly (Fix A)
	if strings.Contains(manifest, "cloud.google.com/gke-tpu-topology: 2x2") {
		t.Errorf("Expected manifest to NOT contain strict nodeSelector topology, but it was found\nManifest: %s", manifest)
	}

	// 2. Kueue TAS annotations MUST be injected with slice-based format (Fix B)
	if !strings.Contains(manifest, "kueue.x-k8s.io/podset-required-topology: cloud.google.com/gke-tpu-slice-2x2-id") {
		t.Errorf("Expected manifest to contain kueue TAS slice-based annotation, but it was missing or incorrect\nManifest: %s", manifest)
	}
	if !strings.Contains(manifest, "cloud.google.com/gke-tpu-slice-topology: 2x2") {
		t.Errorf("Expected manifest to contain gke-tpu-slice-topology annotation, but it was missing\nManifest: %s", manifest)
	}
}

func TestPopulateClusterMetadata_NAPLimitsLoopOrder(t *testing.T) {
	setupMockMachineConfig(t)

	// Mock JSON with specific limit (nvidia-l4: 16) and generic limit (nvidia.com/gpu: 4).
	// The order doesn't matter now since we compute generic limits in a second pass.
	mockDescribeOutput := `{
		"locations": ["us-central1-a"],
		"nodePools": [],
		"autoscaling": {
			"enableNodeAutoprovisioning": true,
			"resourceLimits": [
				{
					"resourceType": "nvidia-l4",
					"maximum": "16"
				},
				{
					"resourceType": "nvidia.com/gpu",
					"maximum": "4"
				}
			]
		}
	}`

	mockResponses := map[string][]shell.CommandResult{
		"gcloud container clusters describe": {
			{
				ExitCode: 0,
				Stdout:   mockDescribeOutput,
			},
		},
	}

	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	job := &orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterName:     "my-cluster",
		ClusterLocation: "us-central1-a",
	}

	err := orc.populateClusterMetadata(job)
	if err != nil {
		t.Fatalf("populateClusterMetadata failed: %v", err)
	}

	// The generic limit "nvidia.com/gpu" must be the MAXIMUM of specific limits (16), NOT overwritten by the lower generic limit (4)
	expectedGPULimit := int64(16)
	if limit := orc.napLimits["nvidia.com/gpu"]; limit != expectedGPULimit {
		t.Errorf("expected napLimits[nvidia.com/gpu] to be %d, got %d", expectedGPULimit, limit)
	}
}

func TestPopulateClusterMetadata_LocationFallback(t *testing.T) {
	setupMockMachineConfig(t)

	mockDescribeOutput := `{
		"locations": ["us-central1-a", "us-central1-b"],
		"nodePools": [],
		"autoscaling": {}
	}`

	mockResponses := map[string][]shell.CommandResult{
		"gcloud container clusters describe my-cluster --location us-central1-a --project my-project --format=json": {
			{
				ExitCode: 1,
				Stderr:   "Resource my-cluster was not found in us-central1-a",
			},
		},
		"gcloud container clusters describe my-cluster --location us-central1 --project my-project --format=json": {
			{
				ExitCode: 0,
				Stdout:   mockDescribeOutput,
			},
		},
	}

	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	job := &orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterName:     "my-cluster",
		ClusterLocation: "us-central1-a",
	}

	err := orc.populateClusterMetadata(job)
	if err != nil {
		t.Fatalf("populateClusterMetadata failed: %v", err)
	}

	if job.ClusterLocation != "us-central1" {
		t.Errorf("Expected job.ClusterLocation to fall back to 'us-central1', got %q", job.ClusterLocation)
	}
}

func TestPopulateNAPFlavors(t *testing.T) {
	tests := []struct {
		name        string
		napLimits   map[string]int64
		wantFlavors map[string]FlavorCapacity
		wantErr     bool
	}{
		{
			name: "Skip cpu and memory, populate generic tpu and model specific gpu",
			napLimits: map[string]int64{
				"cpu":            100,
				"memory":         1024,
				"google.com/tpu": 4,
				"nvidia-l4":      8,
			},
			wantFlavors: map[string]FlavorCapacity{
				"flavor-tpu-generic": {
					NodeLabels: nil,
				},
				"flavor-nvidia-l4": {
					NodeLabels: map[string]string{
						"cloud.google.com/gke-accelerator": "nvidia-l4",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Populate tpu7x",
			napLimits: map[string]int64{
				"tpu7x": 4,
			},
			wantFlavors: map[string]FlavorCapacity{
				"flavor-tpu7x": {
					NodeLabels: map[string]string{
						"cloud.google.com/gke-tpu-accelerator": "tpu7x",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Unknown accelerator label returns error",
			napLimits: map[string]int64{
				"unknown-gpu": 2,
			},
			wantFlavors: nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				napEnabled: true,
				napLimits:  tt.napLimits,
			}
			flavors := make(map[string]FlavorCapacity)
			err := orc.populateNAPFlavors(flavors)
			if (err != nil) != tt.wantErr {
				t.Fatalf("populateNAPFlavors() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(flavors) != len(tt.wantFlavors) {
				t.Errorf("expected %d flavors, got %d", len(tt.wantFlavors), len(flavors))
			}
			for fName, expectedCapacity := range tt.wantFlavors {
				gotCapacity, ok := flavors[fName]
				if !ok {
					t.Errorf("missing expected flavor %s", fName)
					continue
				}
				if len(gotCapacity.NodeLabels) != len(expectedCapacity.NodeLabels) {
					t.Errorf("expected %d node labels, got %d", len(expectedCapacity.NodeLabels), len(gotCapacity.NodeLabels))
				}
				for k, v := range expectedCapacity.NodeLabels {
					if gotCapacity.NodeLabels[k] != v {
						t.Errorf("expected label %s=%s, got %s", k, v, gotCapacity.NodeLabels[k])
					}
				}
			}
		})
	}
}

func TestGenerateGKENodeSelectorLabel(t *testing.T) {
	orc := &GKEOrchestrator{}
	tests := []struct {
		input string
		want  string
	}{
		// Mapped families (2 parts)
		{"g2-standard-48", "nvidia-l4"},
		{"a3-highgpu-8g", "nvidia-h100-80gb"},
		{"a2-highgpu-1g", "nvidia-tesla-a100"},
		{"g4-standard-4", "nvidia-rtx-pro-6000"},
		{"ct6e-standard-8t", "tpu-v6e-slice"},

		// Mapped families (1 part)
		{"v6e-standard", "tpu-v6e-slice"},
		{"v5litepod-slice", "tpu-v5-lite-podslice"},
		{"v4-podslice", "tpu-v4-podslice"},
		{"l4-standard", "nvidia-l4"},

		// Old switch fallbacks (unmapped directly, but should return as is)
		{"nvidia-tesla-a100", "nvidia-tesla-a100"},
		{"tpu-v4-podslice", "tpu-v4-podslice"},
		{"tpu-v6e-slice", "tpu-v6e-slice"},
		{"tpu-v5p-slice", "tpu-v5p-slice"},
		{"tpu-v5-lite-podslice", "tpu-v5-lite-podslice"},

		// Unmapped values
		{"unknown-accelerator", "unknown-accelerator"},
		{"h100", "h100"},

		// Case insensitivity
		{"G2-STANDARD-48", "nvidia-l4"},
		{"NVIDIA-TESLA-A100", "NVIDIA-TESLA-A100"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := orc.GenerateGKENodeSelectorLabel(tc.input)
			if got != tc.want {
				t.Errorf("GenerateGKENodeSelectorLabel(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGenerateGKEManifest_CustomEnv(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "env-test-job",
		CommandToRun:    "echo hello",
		ComputeType:     "n2-standard-2",
		ClusterLocation: "us-central1-a",
		Env: map[string]string{
			"MY_CUSTOM_VAR": "some-value",
			"ANOTHER_VAR":   "another=value",
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	expectedLines := []string{
		`- name: MY_CUSTOM_VAR`,
		`  value: "some-value"`,
		`- name: ANOTHER_VAR`,
		`  value: "another=value"`,
	}
	for _, line := range expectedLines {
		if !strings.Contains(manifest, line) {
			t.Errorf("manifest does not contain expected line %q. Got manifest:\n%s", line, manifest)
		}
	}
}

func TestGeneratePathwaysManifest_CustomEnv(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-env-job",
		CommandToRun:    "echo hello",
		ComputeType:     "tpu-v5-lite-podslice",
		ClusterLocation: "us-central1-a",
		IsPathwaysJob:   true,
		Pathways: orchestrator.PathwaysJobDefinition{
			ProxyServerImage: "proxy:latest",
			ServerImage:      "server:latest",
			WorkerImage:      "worker:latest",
			GCSLocation:      "gs://my-bucket",
		},
		Env: map[string]string{
			"PATHWAYS_UNSAFE_UNSAFE_OVERRIDE_GRPC_CREDENTIALS": "grpc_insecure_override",
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: "16x16"}},
		"gcloud compute machine-types describe tpu-v5-lite-podslice --zone=us-central1-a --format=json":                         {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v5-lite-podslice"}]}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Config: gkeNodePoolConfig{MachineType: "tpu-v5-lite-podslice"}},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}

	// The custom environment variables must be present only in the workload-container container spec
	expectedLines := []string{
		`- name: PATHWAYS_UNSAFE_UNSAFE_OVERRIDE_GRPC_CREDENTIALS`,
		`  value: "grpc_insecure_override"`,
	}
	for _, line := range expectedLines {
		count := strings.Count(manifest, line)
		// We expect it to appear exactly once: in the workload-container
		if count != 1 {
			t.Errorf("expected line %q to appear exactly 1 time in manifest, got %d. Manifest:\n%s", line, count, manifest)
		}
	}
}

func TestPrepareManifestOptions_PathwaysPlatform(t *testing.T) {
	setupMockMachineConfig(t)
	tests := []struct {
		computeType  string
		topology     string
		wantPlatform string
	}{
		{
			computeType:  "tpu-v5-lite-podslice",
			topology:     "16x16",
			wantPlatform: "tpuv5e:16x16",
		},
		{
			computeType:  "v5litepod",
			topology:     "8x8",
			wantPlatform: "tpuv5e:8x8",
		},
		{
			computeType:  "tpu-v6e-slice",
			topology:     "4x4",
			wantPlatform: "tpuv6e:4x4",
		},
		{
			computeType:  "v6e",
			topology:     "4x4",
			wantPlatform: "tpuv6e:4x4",
		},
		{
			computeType:  "tpu-v5p-slice",
			topology:     "2x2x2",
			wantPlatform: "tpuv5:2x2x2",
		},
		{
			computeType:  "v5p",
			topology:     "2x2x2",
			wantPlatform: "tpuv5:2x2x2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.computeType, func(t *testing.T) {
			job := orchestrator.JobDefinition{
				WorkloadName:    "pathways-test-job",
				CommandToRun:    "echo hello",
				ComputeType:     tc.computeType,
				ClusterLocation: "us-central1-a",
				IsPathwaysJob:   true,
				Topology:        tc.topology,
			}

			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: tc.topology}},
				"gcloud compute machine-types describe " + tc.computeType + " --zone=us-central1-a --format=json":                       {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "` + tc.computeType + `"}]}`}},
			}

			mockExec := NewMockExecutor(mockResponses)
			orc := newTestGKEOrchestrator(mockExec)
			orc.projectID = "mock-project"
			orc.clusterDesc.NodePools = []gkeJobNodePool{
				{Config: gkeNodePoolConfig{MachineType: tc.computeType}},
			}

			profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveHardwareRequirements failed: %v", err)
			}

			opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
			if err != nil {
				t.Fatalf("PrepareManifestOptions failed: %v", err)
			}

			if opts.PathwaysInstanceType != tc.wantPlatform {
				t.Errorf("PathwaysInstanceType = %q, want %q", opts.PathwaysInstanceType, tc.wantPlatform)
			}
		})
	}
}

func TestSharedReservationManifestGeneration(t *testing.T) {
	orc := NewGKEOrchestrator()
	orc.projectID = "my-consumer-project"
	orc.clusterDesc = gkeCluster{
		Locations: []string{"us-central1-a"},
	}
	orc.napEnabled = true
	orc.napLimits = map[string]int64{
		"nvidia.com/gpu": 100,
	}
	// Mock machine capabilities fetcher to avoid actual API calls
	orc.machineCapCache = map[string]MachineTypeCap{
		"a3-highgpu-8g:us-central1-a": {
			GuestCpus: 208,
			MemoryMb:  1884160,
			Accelerators: []struct {
				Count int    `json:"guestAcceleratorCount"`
				Type  string `json:"guestAcceleratorType"`
			}{
				{Count: 8, Type: "nvidia-h100-80gb"},
			},
		},
	}

	job := orchestrator.JobDefinition{
		WorkloadName:       "shared-res-job",
		MachineType:        "a3-highgpu-8g",
		ComputeType:        "a3-highgpu-8g",
		ClusterLocation:    "us-central1-a",
		GKENAPProvisioning: "reservation",
		GKENAPReservation:  "projects/my-owner-project/reservations/my-shared-res",
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	// Verify that the manifest contains the correct node selector labels
	expectedLabels := []string{
		"cloud.google.com/reservation-name: my-shared-res",
		"cloud.google.com/reservation-project: my-owner-project",
		"cloud.google.com/reservation-affinity: specific",
	}

	for _, label := range expectedLabels {
		if !strings.Contains(manifest, label) {
			t.Errorf("Expected manifest to contain nodeSelector label %q, but it was missing.\nManifest:\n%s", label, manifest)
		}
	}
}

func TestSharedReservationSubblockManifestGeneration(t *testing.T) {
	orc := NewGKEOrchestrator()
	orc.projectID = "my-consumer-project"
	orc.clusterDesc = gkeCluster{
		Locations: []string{"us-central1-a"},
	}
	orc.napEnabled = true
	orc.napLimits = map[string]int64{
		"nvidia.com/gpu": 100,
	}
	// Mock machine capabilities fetcher to avoid actual API calls
	orc.machineCapCache = map[string]MachineTypeCap{
		"a3-highgpu-8g:us-central1-a": {
			GuestCpus: 208,
			MemoryMb:  1884160,
			Accelerators: []struct {
				Count int    `json:"guestAcceleratorCount"`
				Type  string `json:"guestAcceleratorType"`
			}{
				{Count: 8, Type: "nvidia-h100-80gb"},
			},
		},
	}

	job := orchestrator.JobDefinition{
		WorkloadName:       "shared-res-subblock-job",
		MachineType:        "a3-highgpu-8g",
		ComputeType:        "a3-highgpu-8g",
		ClusterLocation:    "us-central1-a",
		GKENAPProvisioning: "reservation",
		GKENAPReservation:  "projects/my-owner-project/reservations/my-shared-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2",
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	// Verify that the manifest contains the correct node selector labels, including subblock
	expectedLabels := []string{
		"cloud.google.com/reservation-name: my-shared-res",
		"cloud.google.com/reservation-project: my-owner-project",
		"cloud.google.com/reservation-blocks: block-1",
		"cloud.google.com/reservation-subblocks: subblock-2",
		"cloud.google.com/reservation-affinity: specific",
	}

	for _, label := range expectedLabels {
		if !strings.Contains(manifest, label) {
			t.Errorf("Expected manifest to contain nodeSelector label %q, but it was missing.\nManifest:\n%s", label, manifest)
		}
	}
}
func TestGetJobLogs(t *testing.T) {
	jobName := "test-job"
	trueVal := true
	falseVal := false

	tests := []struct {
		desc               string
		mainOnly           *bool
		mockGetPods1Stdout string // response for total count check
		mockGetPods2Stdout string // response for filtered count check
		expectedCmdLogsKey string
		expectErrorContain string
	}{
		{
			desc:               "explicit MainOnly=true uses coordinator-only selector (1 pod, succeeds)",
			mainOnly:           &trueVal,
			mockGetPods2Stdout: "pod-main-0-0\n",
			expectedCmdLogsKey: "kubectl logs -n default -l jobset.sigs.k8s.io/jobset-name=test-job,jobset.sigs.k8s.io/job-index=0,batch.kubernetes.io/job-completion-index=0 --all-containers --max-log-requests=10",
		},
		{
			desc:               "explicit MainOnly=false with pods <= 10 uses all-job selector (succeeds)",
			mainOnly:           &falseVal,
			mockGetPods2Stdout: "pod-1\npod-2\npod-3\npod-4\npod-5\npod-6\npod-7\npod-8\n", // 8 pods
			expectedCmdLogsKey: "kubectl logs -n default -l jobset.sigs.k8s.io/jobset-name=test-job --all-containers --max-log-requests=10",
		},
		{
			desc:               "explicit MainOnly=false with pods > 10 fails proactively with Console URL",
			mainOnly:           &falseVal,
			mockGetPods2Stdout: "pod-1\npod-2\npod-3\npod-4\npod-5\npod-6\npod-7\npod-8\npod-9\npod-10\npod-11\npod-12\n", // 12 pods
			expectErrorContain: "exceeds the max fetch limit (10). Please view logs directly in the Google Cloud Console",
		},
		{
			desc:               "implicit MainOnly (nil) with pods <= 5 defaults to all pods (succeeds)",
			mainOnly:           nil,
			mockGetPods1Stdout: "pod-1\npod-2\n", // 2 pods (total)
			mockGetPods2Stdout: "pod-1\npod-2\n", // 2 pods (filtered)
			expectedCmdLogsKey: "kubectl logs -n default -l jobset.sigs.k8s.io/jobset-name=test-job --all-containers --max-log-requests=10",
		},
		{
			desc:               "implicit MainOnly (nil) with pods > 5 defaults to coordinator-only (succeeds)",
			mainOnly:           nil,
			mockGetPods1Stdout: "pod-1\npod-2\npod-3\npod-4\npod-5\npod-6\npod-7\npod-8\n", // 8 pods total
			mockGetPods2Stdout: "pod-main-0-0\n",                                           // 1 pod coordinator
			expectedCmdLogsKey: "kubectl logs -n default -l jobset.sigs.k8s.io/jobset-name=test-job,jobset.sigs.k8s.io/job-index=0,batch.kubernetes.io/job-completion-index=0 --all-containers --max-log-requests=10",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			totalQuery := "kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=test-job --no-headers"
			filteredQuery := "kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=test-job,jobset.sigs.k8s.io/job-index=0,batch.kubernetes.io/job-completion-index=0 --no-headers"
			if tc.mainOnly != nil && !*tc.mainOnly {
				filteredQuery = "kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=test-job --no-headers"
			}

			mockResponses := map[string][]shell.CommandResult{
				"gcloud container clusters get-credentials test-cluster --location us-central1-a --project test-project": {{ExitCode: 0, Stdout: ""}},
				"gcloud container clusters describe": {{ExitCode: 0, Stdout: "description"}},
			}
			if totalQuery == filteredQuery {
				mockResponses[totalQuery] = []shell.CommandResult{{ExitCode: 0, Stdout: tc.mockGetPods2Stdout}}
			} else {
				mockResponses[totalQuery] = []shell.CommandResult{{ExitCode: 0, Stdout: tc.mockGetPods1Stdout}, {ExitCode: 0, Stdout: tc.mockGetPods1Stdout}}
				mockResponses[filteredQuery] = []shell.CommandResult{{ExitCode: 0, Stdout: tc.mockGetPods2Stdout}}
			}
			if tc.expectedCmdLogsKey != "" {
				mockResponses[tc.expectedCmdLogsKey] = []shell.CommandResult{{ExitCode: 0, Stdout: "mock-logs-content"}}
			}

			mockExec := NewMockExecutor(mockResponses)
			orc := newTestGKEOrchestrator(mockExec)
			orc.kubeClient = &MockKubeClient{Namespace: "default"}

			opts := orchestrator.LogsOptions{
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ProjectID:       "test-project",
				Follow:          false,
				MainOnly:        tc.mainOnly,
			}

			logs, err := orc.GetJobLogs(jobName, opts)

			if tc.expectErrorContain != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectErrorContain)
				}
				if !strings.Contains(err.Error(), tc.expectErrorContain) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.expectErrorContain)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetJobLogs failed: %v", err)
			}

			if logs != "mock-logs-content" {
				t.Errorf("expected logs %q, got %q", "mock-logs-content", logs)
			}

			if mockExec.callCount[tc.expectedCmdLogsKey] != 1 {
				t.Errorf("expected command %q to be called exactly once, call count: %d", tc.expectedCmdLogsKey, mockExec.callCount[tc.expectedCmdLogsKey])
			}
		})

	}
}

func TestGeneratePathwaysManifest_Headless(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-headless-test",
		NumSlices:       1,
		ClusterLocation: "us-central1",
		ComputeType:     "n2-standard-2",
		Pathways: orchestrator.PathwaysJobDefinition{
			Headless:         true,
			ProxyServerImage: "proxy:latest",
			ServerImage:      "server:latest",
			WorkerImage:      "worker:latest",
			GCSLocation:      "gs://my-bucket",
			HeadNodePool:     "pathways-np",
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
	}
	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	manifest, err := orc.GeneratePathwaysManifest(job, "", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("generatePathwaysManifest failed: %v", err)
	}

	expectedSubstrs := []string{
		"name: pathways-headless-test",
		"image: proxy:latest",
		"image: server:latest",
		"--gcs_scratch_location=gs://my-bucket",
		"cloud.google.com/gke-nodepool: pathways-np",
	}

	for _, substr := range expectedSubstrs {
		if !strings.Contains(manifest, substr) {
			t.Errorf("manifest missing expected substring %q", substr)
		}
	}

	// In headless mode, the workload-container must NOT be present.
	if strings.Contains(manifest, "workload-container") {
		t.Errorf("manifest contains 'workload-container', which is unexpected in headless mode")
	}

	// In headless mode, the command template block must NOT be present.
	if strings.Contains(manifest, "JAX_PLATFORMS") || strings.Contains(manifest, "kill -SIGTERM") {
		t.Errorf("manifest contains workload command envs/traps, which is unexpected in headless mode")
	}
}
