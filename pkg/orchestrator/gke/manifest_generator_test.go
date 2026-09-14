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

func TestBuildResourcesString(t *testing.T) {
	g := &GKEOrchestrator{}

	tests := []struct {
		name        string
		cpu         string
		mem         string
		gpu         string
		tpu         string
		wantContain string
		wantErr     bool
	}{
		{
			name:        "valid cpu",
			cpu:         "100m",
			wantContain: "cpu: 100m",
			wantErr:     false,
		},
		{
			name:    "invalid cpu",
			cpu:     "invalid",
			wantErr: true,
		},
		{
			name:        "valid gpu",
			gpu:         "1",
			wantContain: "nvidia.com/gpu",
			wantErr:     false,
		},
		{
			name:    "invalid gpu",
			gpu:     "invalid",
			wantErr: true,
		},
		{
			name:        "valid tpu",
			tpu:         "4",
			wantContain: "google.com/tpu",
			wantErr:     false,
		},
		{
			name:        "empty limits",
			wantErr:     false,
			wantContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.buildResourcesString(tt.cpu, tt.mem, tt.gpu, tt.tpu, 16)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildResourcesString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("buildResourcesString() = %v, want contain %v", got, tt.wantContain)
			}
		})
	}
}

func TestAssembleManifest(t *testing.T) {
	tests := []struct {
		name                string
		mainManifest        string
		additionalManifests []string
		want                string
	}{
		{
			name:                "no additional manifests",
			mainManifest:        "main: content",
			additionalManifests: nil,
			want:                "main: content",
		},
		{
			name:                "one additional manifest",
			mainManifest:        "main: content",
			additionalManifests: []string{"add1: content"},
			want:                "add1: content\n---\nmain: content",
		},
		{
			name:                "multiple additional manifests",
			mainManifest:        "main: content",
			additionalManifests: []string{"add1: content", "add2: content"},
			want:                "add1: content\n---\nadd2: content\n---\nmain: content",
		},
		{
			name:                "empty and whitespace additional manifests",
			mainManifest:        "main: content",
			additionalManifests: []string{"", "  ", "add1: content", "\n"},
			want:                "add1: content\n---\nmain: content",
		},
		{
			name:                "whitespace main manifest",
			mainManifest:        "  main: content  ",
			additionalManifests: []string{"add1: content"},
			want:                "add1: content\n---\nmain: content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assembleManifest(tt.mainManifest, tt.additionalManifests)
			if got != tt.want {
				t.Errorf("assembleManifest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateGKEManifest_MLDiagnosticsEnabled(t *testing.T) {
	setupMockMachineConfig(t)
	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n1-standard-4 --zone=test-location-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 4}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"test-location-a"}
	orc.gkeCustomTemplatesPath = ""
	orc.acceleratorToMachineType = make(map[string]string)

	opts := ManifestOptions{
		WorkloadName:         "test-workload",
		FullImageName:        "test-image",
		CommandToRun:         "test-command",
		ComputeType:          "n1-standard-4",
		ClusterName:          "test-cluster",
		ClusterLocation:      "test-location",
		ProjectID:            "test-project",
		MLDiagnosticsEnabled: true,
	}

	profile := JobProfile{
		IsCPUMachine:  true,
		CapacityCount: 1,
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("Failed to generate manifest: %v", err)
	}

	if !strings.Contains(manifest, "managed-mldiagnostics-gke: \"true\"") {
		t.Errorf("Expected manifest to contain ML Diagnostics label, got:\n%s", manifest)
	}
}

func TestGeneratePathwaysManifest_MLDiagnosticsEnabled(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:         "pathways-test",
		CommandToRun:         "echo hello",
		NumSlices:            2,
		ClusterLocation:      "us-central1",
		ComputeType:          "n2-standard-2",
		MLDiagnosticsEnabled: true,
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
	profile, _, _, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	manifest, err := orc.GeneratePathwaysManifest(job, "test-image", profile, false, false)
	if err != nil {
		t.Fatalf("Failed to generate pathways manifest: %v", err)
	}

	if !strings.Contains(manifest, "managed-mldiagnostics-gke: \"true\"") {
		t.Errorf("Expected manifest to contain ML Diagnostics label, got:\n%s", manifest)
	}
}

func TestGenerateGKEManifest_MLDiagnosticsDisabled(t *testing.T) {
	setupMockMachineConfig(t)
	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n1-standard-4 --zone=test-location-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 4}`}},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"test-location-a"}
	orc.gkeCustomTemplatesPath = ""
	orc.acceleratorToMachineType = make(map[string]string)

	opts := ManifestOptions{
		WorkloadName:         "test-workload",
		FullImageName:        "test-image",
		CommandToRun:         "test-command",
		ComputeType:          "n1-standard-4",
		ClusterName:          "test-cluster",
		ClusterLocation:      "test-location",
		ProjectID:            "test-project",
		MLDiagnosticsEnabled: false,
	}

	profile := JobProfile{
		IsCPUMachine:  true,
		CapacityCount: 1,
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("Failed to generate manifest: %v", err)
	}

	if strings.Contains(manifest, "managed-mldiagnostics-gke: \"true\"") {
		t.Errorf("Expected manifest to NOT contain ML Diagnostics label when disabled, got:\n%s", manifest)
	}
}

func TestGeneratePathwaysManifest_MLDiagnosticsDisabled(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:         "pathways-test",
		CommandToRun:         "echo hello",
		NumSlices:            2,
		ClusterLocation:      "us-central1",
		ComputeType:          "n2-standard-2",
		MLDiagnosticsEnabled: false,
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
	profile, _, _, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	manifest, err := orc.GeneratePathwaysManifest(job, "test-image", profile, false, false)
	if err != nil {
		t.Fatalf("Failed to generate pathways manifest: %v", err)
	}

	if strings.Contains(manifest, "managed-mldiagnostics-gke: \"true\"") {
		t.Errorf("Expected manifest to NOT contain ML Diagnostics label when disabled, got:\n%s", manifest)
	}
}

func TestGenerateGKEManifest_OlderTPU_Default(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "tpu-v4-job",
		CommandToRun:    "echo hello",
		ComputeType:     "ct4p-hightpu-4t",
		Topology:        "2x2x2",
		NumSlices:       1,
		ClusterLocation: "us-central1-a",
		ProjectID:       "mock-project",
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: "2x2x2"}},
		"gcloud compute machine-types describe ct4p-hightpu-4t --zone=us-central1-a --format=json":                              {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v4-podslice"}]}`}},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "v4-pool", Config: gkeNodePoolConfig{MachineType: "ct4p-hightpu-4t"}},
	}

	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	if job.NodesPerSlice != 2 {
		t.Fatalf("expected NodesPerSlice to be 2 for 2x2x2 topology on ct4p-hightpu-4t, got %d", job.NodesPerSlice)
	}
	if job.PlacementPolicy != "" {
		t.Errorf("expected PlacementPolicy to be empty for TPU v4, got %q", job.PlacementPolicy)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	if strings.Contains(manifest, "cloud.google.com/placement-policy-name") {
		t.Errorf("Did not expect cloud.google.com/placement-policy-name in TPU v4 manifest:\n%s", manifest)
	}
	if strings.Contains(manifest, "cloud.google.com/gke-placement-group") {
		t.Errorf("Did not expect cloud.google.com/gke-placement-group in TPU v4 manifest:\n%s", manifest)
	}
	if !strings.Contains(manifest, "cloud.google.com/gke-tpu-accelerator: tpu-v4-podslice") {
		t.Errorf("Expected cloud.google.com/gke-tpu-accelerator: tpu-v4-podslice in manifest:\n%s", manifest)
	}
	if !strings.Contains(manifest, "cloud.google.com/gke-tpu-topology: 2x2x2") {
		t.Errorf("Expected cloud.google.com/gke-tpu-topology: 2x2x2 in manifest:\n%s", manifest)
	}
}

func TestGenerateGKEManifest_OlderTPU_UserSpecifiedPlacement(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "tpu-v4-job",
		CommandToRun:    "echo hello",
		ComputeType:     "ct4p-hightpu-4t",
		Topology:        "2x2x2",
		NumSlices:       1,
		ClusterLocation: "us-central1-a",
		ProjectID:       "mock-project",
		PlacementPolicy: "my-custom-tpu-policy",
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: "2x2x2"}},
		"gcloud compute machine-types describe ct4p-hightpu-4t --zone=us-central1-a --format=json":                              {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v4-podslice"}]}`}},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "v4-pool", Config: gkeNodePoolConfig{MachineType: "ct4p-hightpu-4t"}},
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

	if !strings.Contains(manifest, "cloud.google.com/placement-policy-name: my-custom-tpu-policy") {
		t.Errorf("Expected cloud.google.com/placement-policy-name for TPU when specified, got:\n%s", manifest)
	}
	if strings.Contains(manifest, "cloud.google.com/gke-placement-group") {
		t.Errorf("Did not expect cloud.google.com/gke-placement-group on TPU, got:\n%s", manifest)
	}
}

func TestGenerateGKEManifest_GPU_Placement(t *testing.T) {
	setupMockMachineConfig(t)
	gpuJob := orchestrator.JobDefinition{
		WorkloadName:    "gpu-job",
		CommandToRun:    "echo hello",
		ComputeType:     "a3-highgpu-8g",
		ClusterLocation: "us-central1-a",
		ProjectID:       "mock-project",
		PlacementPolicy: "my-gpu-group",
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe a3-highgpu-8g --zone=us-central1-a --format=json":                                {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 8, "guestAcceleratorType": "nvidia-h100-80gb"}]}`}},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "gpu-pool", Config: gkeNodePoolConfig{MachineType: "a3-highgpu-8g"}},
	}

	gpuProfile, isDyn, isStat, err := orc.resolveHardwareRequirements(&gpuJob)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed for GPU: %v", err)
	}

	gpuOpts, err := orc.PrepareManifestOptions(gpuJob, "test-image:latest", gpuProfile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed for GPU: %v", err)
	}

	gpuManifest, err := orc.GenerateGKEManifest(gpuOpts, gpuProfile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed for GPU: %v", err)
	}

	if !strings.Contains(gpuManifest, "cloud.google.com/gke-placement-group: my-gpu-group") {
		t.Errorf("Expected cloud.google.com/gke-placement-group for GPU, got:\n%s", gpuManifest)
	}
	if strings.Contains(gpuManifest, "cloud.google.com/placement-policy-name") {
		t.Errorf("Did not expect cloud.google.com/placement-policy-name for GPU, got:\n%s", gpuManifest)
	}
}
