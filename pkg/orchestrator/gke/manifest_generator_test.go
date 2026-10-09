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
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	corev1 "k8s.io/api/core/v1"
	k8syaml "sigs.k8s.io/yaml"
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

func TestGenerateGKEManifest_DefaultDevShm(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "shm-job",
		CommandToRun:    "echo test",
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

	profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	if !strings.Contains(manifest, "mountPath: /dev/shm") || !strings.Contains(manifest, "name: dshm") {
		t.Errorf("Expected /dev/shm volumeMount with name dshm in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "medium: Memory") {
		t.Errorf("Expected emptyDir with medium: Memory in manifest, got:\n%s", manifest)
	}
}

func TestGenerateGKEManifest_DefaultDevShm_ParallelContainers(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:          "shm-parallel-job",
		CommandToRun:          "echo test",
		ComputeType:           "tpu7x",
		Topology:              "2x2x1",
		NumSlices:             1,
		ClusterLocation:       "us-central1-a",
		ProjectID:             "mock-project",
		UseParallelContainers: true,
	}

	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v7x-slice"}]}`},
		},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "v7x-pool", Config: gkeNodePoolConfig{MachineType: "tpu7x-standard-4t"}},
	}

	profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}

	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}

	// Verify that both containers share the single dshm volume.
	dshmCount := strings.Count(manifest, "name: dshm")
	// Expected: 2 container volumeMounts + 1 pod volume definition = 3 occurrences
	if dshmCount != 3 {
		t.Errorf("Expected exactly 3 occurrences of 'name: dshm' (2 container mounts + 1 volume), got %d in manifest:\n%s", dshmCount, manifest)
	}
	if !strings.Contains(manifest, "mountPath: /dev/shm") {
		t.Errorf("Expected mountPath: /dev/shm in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "medium: Memory") {
		t.Errorf("Expected emptyDir with medium: Memory in manifest, got:\n%s", manifest)
	}
}

// pathwaysPodSpecs parses a rendered Pathways manifest and returns the pod spec
// of each replicated job, keyed by replicated job name.
func pathwaysPodSpecs(t *testing.T, manifest string) map[string]*corev1.PodSpec {
	t.Helper()
	type replicatedJob struct {
		Name     string `json:"name"`
		Template struct {
			Spec struct {
				Template struct {
					Spec corev1.PodSpec `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		} `json:"template"`
	}
	docs, err := splitYAMLDocuments(manifest)
	if err != nil {
		t.Fatalf("failed to split manifest into YAML documents: %v", err)
	}
	for _, doc := range docs {
		var js struct {
			Kind string `json:"kind"`
			Spec struct {
				ReplicatedJobs []replicatedJob `json:"replicatedJobs"`
			} `json:"spec"`
		}
		if err := k8syaml.Unmarshal([]byte(doc), &js); err != nil {
			t.Fatalf("failed to unmarshal manifest document: %v\n%s", err, doc)
		}
		if js.Kind != "JobSet" {
			continue
		}
		specs := make(map[string]*corev1.PodSpec, len(js.Spec.ReplicatedJobs))
		for i := range js.Spec.ReplicatedJobs {
			rj := &js.Spec.ReplicatedJobs[i]
			specs[rj.Name] = &rj.Template.Spec.Template.Spec
		}
		return specs
	}
	t.Fatalf("no JobSet document found in manifest:\n%s", manifest)
	return nil
}

// devShmMountNames returns the names of the volumes a container mounts at /dev/shm.
func devShmMountNames(c *corev1.Container) []string {
	var names []string
	for _, m := range c.VolumeMounts {
		if m.MountPath == "/dev/shm" {
			names = append(names, m.Name)
		}
	}
	return names
}

// findPodVolume returns the pod volume with the given name, or nil.
func findPodVolume(spec *corev1.PodSpec, name string) *corev1.Volume {
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == name {
			return &spec.Volumes[i]
		}
	}
	return nil
}

// isMemoryEmptyDir reports whether v is an emptyDir backed by memory (tmpfs).
func isMemoryEmptyDir(v *corev1.Volume) bool {
	return v != nil && v.EmptyDir != nil && v.EmptyDir.Medium == corev1.StorageMediumMemory
}

// podContainers returns pointers to a pod's init containers followed by its
// containers.
func podContainers(spec *corev1.PodSpec) []*corev1.Container {
	containers := make([]*corev1.Container, 0, len(spec.InitContainers)+len(spec.Containers))
	for i := range spec.InitContainers {
		containers = append(containers, &spec.InitContainers[i])
	}
	for i := range spec.Containers {
		containers = append(containers, &spec.Containers[i])
	}
	return containers
}

// renderPathwaysShmTestManifest renders a Pathways manifest with the given
// head mode, colocated Python sidecar image and --mount values.
func renderPathwaysShmTestManifest(t *testing.T, headless bool, sidecarImage string, mounts []string) string {
	t.Helper()
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-shm-test",
		NumSlices:       2,
		ClusterLocation: "us-central1",
		ComputeType:     "n2-standard-2",
		IsPathwaysJob:   true,
		RawMounts:       mounts,
		Pathways: orchestrator.PathwaysJobDefinition{
			Headless:                    headless,
			ProxyServerImage:            "proxy:latest",
			ServerImage:                 "server:latest",
			WorkerImage:                 "worker:latest",
			ColocatedPythonSidecarImage: sidecarImage,
			GCSLocation:                 "gs://my-bucket",
			HeadNodePool:                "pathways-np",
		},
	}
	image := ""
	if !headless {
		job.CommandToRun = "python3 train.py"
		image = "test-image:latest"
	}

	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	})
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
	manifest, err := orc.GeneratePathwaysManifest(job, image, profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}
	return manifest
}

// assertHeadDevShm checks that the head pod declares the in-memory dshm volume
// and that only its workload-container mounts it at /dev/shm. When wantShm is
// false (headless mode, no workload-container), neither may exist.
func assertHeadDevShm(t *testing.T, head *corev1.PodSpec, wantShm bool) {
	t.Helper()
	dshm := findPodVolume(head, "dshm")
	if wantShm && !isMemoryEmptyDir(dshm) {
		t.Errorf("pathways-head volume dshm = %+v, want an emptyDir with medium Memory", dshm)
	}
	if !wantShm && dshm != nil {
		t.Errorf("pathways-head declares volume dshm %+v, want none in headless mode", dshm)
	}

	sawWorkloadContainer := false
	for _, c := range podContainers(head) {
		var want []string
		if c.Name == "workload-container" {
			sawWorkloadContainer = true
			want = []string{"dshm"}
		}
		if got := devShmMountNames(c); !reflect.DeepEqual(got, want) {
			t.Errorf("pathways-head container %q mounts %v at /dev/shm, want %v", c.Name, got, want)
		}
	}
	if sawWorkloadContainer != wantShm {
		t.Errorf("pathways-head has workload-container = %v, want %v", sawWorkloadContainer, wantShm)
	}
}

// assertNoDevShm checks that a pod neither declares the dshm volume nor mounts
// anything at /dev/shm.
func assertNoDevShm(t *testing.T, podName string, spec *corev1.PodSpec) {
	t.Helper()
	if v := findPodVolume(spec, "dshm"); v != nil {
		t.Errorf("%s pod declares volume dshm %+v, want none", podName, v)
	}
	for _, c := range podContainers(spec) {
		if got := devShmMountNames(c); len(got) > 0 {
			t.Errorf("%s container %q mounts %v at /dev/shm, want none", podName, c.Name, got)
		}
	}
}

// assertUniqueVolumeNames checks that a pod declares each volume name once.
func assertUniqueVolumeNames(t *testing.T, podName string, spec *corev1.PodSpec) {
	t.Helper()
	seen := make(map[string]bool)
	for i := range spec.Volumes {
		name := spec.Volumes[i].Name
		if seen[name] {
			t.Errorf("%s pod declares volume %q more than once", podName, name)
		}
		seen[name] = true
	}
}

// TestGeneratePathwaysManifest_HeadDevShm verifies that the Pathways head's
// workload-container, where the user's Python process runs, gets an in-memory
// /dev/shm instead of the container runtime's 64 MB default. Data loaders such
// as Grain pass batches between processes through /dev/shm and die with SIGBUS
// without it. The Pathways proxy, resource manager and workers run Pathways
// binaries rather than user code, so they must not get the mount.
func TestGeneratePathwaysManifest_HeadDevShm(t *testing.T) {
	tests := []struct {
		name        string
		headless    bool
		sidecar     string
		mounts      []string
		wantHeadShm bool
	}{
		{name: "default", wantHeadShm: true},
		{name: "with user mounts", mounts: []string{"gs://my-bucket;/data;ro", "/host/path;/host"}, wantHeadShm: true},
		{name: "with colocated python sidecar", sidecar: "sidecar:latest", wantHeadShm: true},
		{name: "headless", headless: true, wantHeadShm: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := renderPathwaysShmTestManifest(t, tc.headless, tc.sidecar, tc.mounts)
			specs := pathwaysPodSpecs(t, manifest)
			head, ok := specs["pathways-head"]
			if !ok {
				t.Fatalf("pathways-head replicatedJob not found in manifest:\n%s", manifest)
			}
			worker, ok := specs["worker"]
			if !ok {
				t.Fatalf("worker replicatedJob not found in manifest:\n%s", manifest)
			}

			assertHeadDevShm(t, head, tc.wantHeadShm)
			assertNoDevShm(t, "worker", worker)
			// dshm must not collide with volumes added by --mount, MTC or the sidecar.
			assertUniqueVolumeNames(t, "pathways-head", head)
			assertUniqueVolumeNames(t, "worker", worker)
		})
	}
}

func TestGeneratePathwaysManifest_RestartOnExitCodes(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:       "pathways-exit-code-test",
		CommandToRun:       "python train.py",
		NumSlices:          2,
		ClusterLocation:    "us-central1",
		ComputeType:        "n2-standard-2",
		RestartOnExitCodes: []int{137, 143},
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
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}

	var jsDoc struct {
		Kind string `json:"kind"`
		Spec struct {
			ReplicatedJobs []struct {
				Name     string `json:"name"`
				Template struct {
					Spec struct {
						PodFailurePolicy interface{} `json:"podFailurePolicy"`
						Template         struct {
							Spec struct {
								RestartPolicy string `json:"restartPolicy"`
							} `json:"spec"`
						} `json:"template"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"replicatedJobs"`
		} `json:"spec"`
	}

	if err := k8syaml.Unmarshal([]byte(manifest), &jsDoc); err != nil {
		t.Fatalf("Failed to unmarshal generated manifest: %v", err)
	}

	var headFound, workerFound bool
	for _, rj := range jsDoc.Spec.ReplicatedJobs {
		switch rj.Name {
		case "pathways-head":
			headFound = true
			if rj.Template.Spec.PodFailurePolicy == nil {
				t.Errorf("expected pathways-head to have podFailurePolicy, but was nil")
			}
			if rj.Template.Spec.Template.Spec.RestartPolicy != "Never" {
				t.Errorf("expected pathways-head restartPolicy to be Never, got %q", rj.Template.Spec.Template.Spec.RestartPolicy)
			}
		case "worker":
			workerFound = true
			if rj.Template.Spec.PodFailurePolicy != nil {
				t.Errorf("expected worker to NOT have podFailurePolicy, but got %+v", rj.Template.Spec.PodFailurePolicy)
			}
			if rj.Template.Spec.Template.Spec.RestartPolicy != "OnFailure" {
				t.Errorf("expected worker restartPolicy to be OnFailure, got %q", rj.Template.Spec.Template.Spec.RestartPolicy)
			}
		}
	}

	if !headFound {
		t.Errorf("pathways-head replicatedJob not found in manifest")
	}
	if !workerFound {
		t.Errorf("worker replicatedJob not found in manifest")
	}
}

func TestManifestGeneration_Matrix_Pathways_RestartOnExitCodes(t *testing.T) {
	setupMockMachineConfig(t)
	tests := []struct {
		name               string
		isPathways         bool
		restartOnExitCodes []int
		wantHeadPFP        bool
		wantHeadRestart    string
		wantWorkerPFP      bool
		wantWorkerRestart  string
	}{
		{
			name:               "no pathways, no restart-on-exit-codes",
			isPathways:         false,
			restartOnExitCodes: nil,
			wantHeadPFP:        false,
			wantHeadRestart:    "Never",
		},
		{
			name:               "no pathways, with restart-on-exit-codes",
			isPathways:         false,
			restartOnExitCodes: []int{137, 143},
			wantHeadPFP:        true,
			wantHeadRestart:    "Never",
		},
		{
			name:               "pathways, no restart-on-exit-codes",
			isPathways:         true,
			restartOnExitCodes: nil,
			wantHeadPFP:        false,
			wantHeadRestart:    "Never",
			wantWorkerPFP:      false,
			wantWorkerRestart:  "OnFailure",
		},
		{
			name:               "pathways, with restart-on-exit-codes",
			isPathways:         true,
			restartOnExitCodes: []int{137, 143},
			wantHeadPFP:        true,
			wantHeadRestart:    "Never",
			wantWorkerPFP:      false,
			wantWorkerRestart:  "OnFailure",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := orchestrator.JobDefinition{
				WorkloadName:       "matrix-test",
				CommandToRun:       "python train.py",
				NumSlices:          1,
				ClusterLocation:    "us-central1",
				ComputeType:        "n2-standard-2",
				RestartOnExitCodes: tc.restartOnExitCodes,
				IsPathwaysJob:      tc.isPathways,
			}
			if tc.isPathways {
				job.Pathways = orchestrator.PathwaysJobDefinition{
					ProxyServerImage: "proxy:latest",
					ServerImage:      "server:latest",
					WorkerImage:      "worker:latest",
					GCSLocation:      "gs://my-bucket",
					HeadNodePool:     "pathways-np",
				}
			}

			mockResponses := map[string][]shell.CommandResult{
				"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
			}
			orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
			orc.projectID = "mock-project"
			orc.clusterZones = []string{"us-central1-a"}
			orc.clusterDesc.NodePools = []gkeJobNodePool{
				{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
			}

			profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveHardwareRequirements failed: %v", err)
			}

			var manifest string
			if tc.isPathways {
				manifest, err = orc.GeneratePathwaysManifest(job, "test-image:latest", profile, isDyn, isStat)
			} else {
				opts, prepErr := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDyn, isStat)
				if prepErr != nil {
					t.Fatalf("PrepareManifestOptions failed: %v", prepErr)
				}
				manifest, err = orc.GenerateGKEManifest(opts, profile)
			}
			if err != nil {
				t.Fatalf("Failed to generate manifest: %v", err)
			}

			if valErr := ValidateJobSetManifest(manifest); valErr != nil {
				t.Fatalf("Generated manifest failed client-side validation: %v", valErr)
			}

			var jsDoc struct {
				Kind string `json:"kind"`
				Spec struct {
					ReplicatedJobs []struct {
						Name     string `json:"name"`
						Template struct {
							Spec struct {
								PodFailurePolicy interface{} `json:"podFailurePolicy"`
								Template         struct {
									Spec struct {
										RestartPolicy string `json:"restartPolicy"`
									} `json:"spec"`
								} `json:"template"`
							} `json:"spec"`
						} `json:"template"`
					} `json:"replicatedJobs"`
				} `json:"spec"`
			}
			if err := k8syaml.Unmarshal([]byte(manifest), &jsDoc); err != nil {
				t.Fatalf("Failed to unmarshal manifest: %v", err)
			}

			for _, rj := range jsDoc.Spec.ReplicatedJobs {
				hasPFP := rj.Template.Spec.PodFailurePolicy != nil
				gotRP := rj.Template.Spec.Template.Spec.RestartPolicy
				switch rj.Name {
				case "main-job", "pathways-head":
					verifyMatrixReplicatedJob(t, rj.Name, gotRP, hasPFP, tc.wantHeadPFP, tc.wantHeadRestart)
				case "worker":
					verifyMatrixReplicatedJob(t, rj.Name, gotRP, hasPFP, tc.wantWorkerPFP, tc.wantWorkerRestart)
				}
			}
		})
	}
}

func verifyMatrixReplicatedJob(t *testing.T, name, gotRP string, hasPFP, wantPFP bool, wantRP string) {
	t.Helper()
	if hasPFP != wantPFP {
		t.Errorf("%s: expected podFailurePolicy=%v, got=%v", name, wantPFP, hasPFP)
	}
	if gotRP != wantRP {
		t.Errorf("%s: expected restartPolicy=%q, got=%q", name, wantRP, gotRP)
	}
}

func TestValidateJobSetManifest(t *testing.T) {
	tests := []struct {
		name      string
		manifest  string
		expectErr bool
		errSubstr string
	}{
		{
			name: "valid JobSet with podFailurePolicy and Never restartPolicy",
			manifest: `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test-jobset
spec:
  replicatedJobs:
  - name: main-job
    template:
      spec:
        podFailurePolicy:
          rules:
          - action: FailJob
            onExitCodes:
              operator: NotIn
              values: [137, 143]
        template:
          spec:
            restartPolicy: Never
`,
			expectErr: false,
		},
		{
			name: "valid JobSet without podFailurePolicy and OnFailure restartPolicy",
			manifest: `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test-jobset
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        template:
          spec:
            restartPolicy: OnFailure
`,
			expectErr: false,
		},
		{
			name: "invalid JobSet with podFailurePolicy and OnFailure restartPolicy",
			manifest: `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test-jobset
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        podFailurePolicy:
          rules:
          - action: FailJob
            onExitCodes:
              operator: NotIn
              values: [137, 143]
        template:
          spec:
            restartPolicy: OnFailure
`,
			expectErr: true,
			errSubstr: "replicatedJob \"worker\" specifies podFailurePolicy with restartPolicy \"OnFailure\"",
		},
		{
			name: "invalid JobSet with podFailurePolicy and Always restartPolicy",
			manifest: `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test-jobset
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        podFailurePolicy:
          rules:
          - action: FailJob
            onExitCodes:
              operator: NotIn
              values: [137, 143]
        template:
          spec:
            restartPolicy: Always
`,
			expectErr: true,
			errSubstr: "replicatedJob \"worker\" specifies podFailurePolicy with restartPolicy \"Always\"",
		},
		{
			name: "invalid JobSet with podFailurePolicy and omitted restartPolicy",
			manifest: `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test-jobset
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        podFailurePolicy:
          rules:
          - action: FailJob
            onExitCodes:
              operator: NotIn
              values: [137, 143]
        template:
          spec: {}
`,
			expectErr: true,
			errSubstr: "replicatedJob \"worker\" specifies podFailurePolicy with restartPolicy \"\"",
		},
		{
			name: "invalid standalone Job with podFailurePolicy and OnFailure restartPolicy",
			manifest: `
apiVersion: batch/v1
kind: Job
metadata:
  name: test-job
spec:
  podFailurePolicy:
    rules:
    - action: FailJob
      onExitCodes:
        operator: NotIn
        values: [137, 143]
  template:
    spec:
      restartPolicy: OnFailure
`,
			expectErr: true,
			errSubstr: "specifies podFailurePolicy with restartPolicy \"OnFailure\"",
		},
		{
			name: "multi-document manifest with one invalid JobSet",
			manifest: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
---
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test-jobset
spec:
  replicatedJobs:
  - name: bad-worker
    template:
      spec:
        podFailurePolicy:
          rules:
          - action: FailJob
        template:
          spec:
            restartPolicy: OnFailure
`,
			expectErr: true,
			errSubstr: "replicatedJob \"bad-worker\" specifies podFailurePolicy with restartPolicy \"OnFailure\"",
		},
		{
			name:      "empty manifest string",
			manifest:  "",
			expectErr: false,
		},
		{
			name:      "whitespace and comment only manifest",
			manifest:  "  \n# just a comment\n  \n",
			expectErr: false,
		},
		{
			name:      "empty multi-document YAML separators",
			manifest:  "---\n---\n",
			expectErr: false,
		},
		{
			name: "valid standalone Job with podFailurePolicy and Never restartPolicy",
			manifest: `
apiVersion: batch/v1
kind: Job
metadata:
  name: test-valid-job
spec:
  podFailurePolicy:
    rules:
    - action: FailJob
      onExitCodes:
        operator: NotIn
        values: [137, 143]
  template:
    spec:
      restartPolicy: Never
`,
			expectErr: false,
		},
		{
			name: "valid multi-document with ConfigMap and valid JobSet",
			manifest: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value
---
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: test-jobset
spec:
  replicatedJobs:
  - name: pathways-head
    template:
      spec:
        podFailurePolicy:
          rules:
          - action: FailJob
            onExitCodes:
              operator: NotIn
              values: [137]
        template:
          spec:
            restartPolicy: Never
  - name: worker
    template:
      spec:
        template:
          spec:
            restartPolicy: OnFailure
`,
			expectErr: false,
		},
		{
			name: "malformed JobSet spec structure returns error",
			manifest: `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: malformed-jobset
spec:
  replicatedJobs:
    not: a-slice
`,
			expectErr: true,
			errSubstr: "failed to parse JobSet manifest spec",
		},
		{
			name: "malformed Job spec structure returns error",
			manifest: `
apiVersion: batch/v1
kind: Job
metadata:
  name: malformed-job
spec:
  template: "not-an-object"
`,
			expectErr: true,
			errSubstr: "failed to parse Job manifest spec",
		},
		{
			name: "non-JobSet document with invalid YAML syntax is skipped",
			manifest: `
apiVersion: v1
kind: ConfigMap
data: [unclosed bracket
`,
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateJobSetManifest(tc.manifest)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestSplitYAMLDocuments(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDocs  int
		expectErr bool
	}{
		{
			name:     "valid multi-doc stream",
			input:    "apiVersion: v1\nkind: ConfigMap\n---\napiVersion: batch/v1\nkind: Job\n",
			wantDocs: 2,
		},
		{
			name:     "leading and trailing separators",
			input:    "---\napiVersion: v1\nkind: ConfigMap\n---\napiVersion: batch/v1\nkind: Job\n---\n",
			wantDocs: 2,
		},
		{
			name:     "empty and whitespace documents between separators",
			input:    "apiVersion: v1\nkind: ConfigMap\n---\n   \n---\napiVersion: batch/v1\nkind: Job",
			wantDocs: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			docs, err := splitYAMLDocuments(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if len(docs) != tc.wantDocs {
				t.Errorf("splitYAMLDocuments() got %d docs, want %d", len(docs), tc.wantDocs)
			}
		})
	}
}

func TestWorkloadContainerCommand_PassesUserCommandVerbatim(t *testing.T) {
	userCommand := `python -c "print('hi')" && echo "$HOME" | tee out.log; exit ${PIPESTATUS[0]}`
	argv := workloadContainerCommand(userCommand)
	if len(argv) != 5 || argv[0] != "/bin/bash" || argv[1] != "-c" || argv[3] != "gcluster" {
		t.Fatalf("unexpected command structure: %q", argv)
	}
	if got := argv[4]; got != userCommand {
		t.Errorf("user command must be passed as its own argument, unmodified.\ngot:  %q\nwant: %q", got, userCommand)
	}
}

func runWorkloadCommand(t *testing.T, ctx context.Context, userCommand string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	argv := workloadContainerCommand(userCommand)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start workload command: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, &stdout, &stderr
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("unexpected wait error: %v", err)
	}
	return exitErr.ExitCode()
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestWorkloadContainerCommand_PreservesExitCodeAndOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd, stdout, stderr := runWorkloadCommand(t, ctx, `set -e; echo "hello from $0"; exit 7`)
	if got := exitCode(t, cmd.Wait()); got != 7 {
		t.Errorf("exit code = %d, want 7", got)
	}
	if !strings.Contains(stdout.String(), "hello from") {
		t.Errorf("stdout = %q, want it to contain the workload output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "GCluster Start:") {
		t.Errorf("stderr = %q, want it to contain GCluster Start banner", stderr.String())
	}
	if !strings.Contains(stderr.String(), "GCluster End:") {
		t.Errorf("stderr = %q, want it to contain GCluster End banner", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Exit code: 7") {
		t.Errorf("stderr = %q, want it to contain Exit code banner", stderr.String())
	}
}

func TestWorkloadContainerCommand_PreservesExitCode0(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd, stdout, stderr := runWorkloadCommand(t, ctx, `echo "success output"; exit 0`)
	if got := exitCode(t, cmd.Wait()); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
	if !strings.Contains(stdout.String(), "success output") {
		t.Errorf("stdout = %q, want it to contain workload output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Exit code: 0") {
		t.Errorf("stderr = %q, want Exit code: 0 banner", stderr.String())
	}
}

func TestWorkloadContainerCommand_ClearsPositionalParameters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd, stdout, _ := runWorkloadCommand(t, ctx, `echo "argc: $#; arg1: ${1:-empty}"`)
	if got := exitCode(t, cmd.Wait()); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
	if !strings.Contains(stdout.String(), "argc: 0; arg1: empty") {
		t.Errorf("stdout = %q, want positional parameters to be cleared", stdout.String())
	}
}

func TestWorkloadContainerCommand_SIGTERMRunsCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	marker := filepath.Join(dir, "cleanup")
	userCommand := `set -e && set -o pipefail && set +e; ` +
		`(touch '` + ready + `'; exec sleep 300) | cat; ` +
		`BENCHMARK_EXIT_CODE=${PIPESTATUS[0]}; ` +
		`echo "cleanup ran exit=${BENCHMARK_EXIT_CODE}" > '` + marker + `'; ` +
		`exit ${BENCHMARK_EXIT_CODE}`

	cmd, _, stderr := runWorkloadCommand(t, ctx, userCommand)
	waitForFile(t, ready, 10*time.Second)
	time.Sleep(300 * time.Millisecond)

	start := time.Now()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}
	code := exitCode(t, cmd.Wait())
	elapsed := time.Since(start)

	if ctx.Err() != nil {
		t.Fatalf("workload did not exit after SIGTERM; stderr: %s", stderr.String())
	}
	if elapsed > 10*time.Second {
		t.Errorf("workload took %v to exit after SIGTERM, want prompt reaction", elapsed)
	}
	if code != 143 {
		t.Errorf("exit code = %d, want 143, got %d", code, code)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("cleanup commands after benchmark did not run: %v; stderr: %s", err, stderr.String())
	}
	if want := "cleanup ran exit=143\n"; string(got) != want {
		t.Errorf("cleanup marker = %q, want %q", got, want)
	}
	if !strings.Contains(stderr.String(), "gcluster: SIGTERM received, running cleanup") {
		t.Errorf("stderr = %q, want SIGTERM notification", stderr.String())
	}
}

func TestWorkloadContainerCommand_SIGINTForwarding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	marker := filepath.Join(dir, "sigint_cleanup")
	userCommand := `set -e && set +e; ` +
		`(touch '` + ready + `'; exec sleep 300); ` +
		`echo "sigint cleanup ran exit=${PIPESTATUS[0]}" > '` + marker + `'; ` +
		`exit 130`

	cmd, _, stderr := runWorkloadCommand(t, ctx, userCommand)
	waitForFile(t, ready, 10*time.Second)
	time.Sleep(300 * time.Millisecond)

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}
	_ = exitCode(t, cmd.Wait())

	if !strings.Contains(stderr.String(), "gcluster: SIGTERM received, running cleanup") {
		t.Errorf("stderr = %q, want SIGTERM notification forwarded on SIGINT", stderr.String())
	}
}

func TestWorkloadContainerCommand_SIGTERMWithoutCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	userCommand := `touch '` + ready + `'; exec sleep 300`

	cmd, _, stderr := runWorkloadCommand(t, ctx, userCommand)
	waitForFile(t, ready, 10*time.Second)
	time.Sleep(300 * time.Millisecond)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}
	code := exitCode(t, cmd.Wait())

	if code != 143 {
		t.Errorf("exit code = %d, want 143 for default SIGTERM termination", code)
	}
	if !strings.Contains(stderr.String(), "Exit code: 143") {
		t.Errorf("stderr = %q, want Exit code: 143 banner", stderr.String())
	}
}

func TestWorkloadContainerCommand_SIGTERMCleanupExitsZeroOverriddenTo143(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	marker := filepath.Join(dir, "cleanup")
	userCommand := `set -e && set +e; ` +
		`(touch '` + ready + `'; exec sleep 300); ` +
		`echo "cleanup succeeded" > '` + marker + `'; ` +
		`exit 0`

	cmd, _, stderr := runWorkloadCommand(t, ctx, userCommand)
	waitForFile(t, ready, 10*time.Second)
	time.Sleep(300 * time.Millisecond)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}
	code := exitCode(t, cmd.Wait())

	if code != 143 {
		t.Errorf("exit code = %d, want 143 (overridden from 0 on SIGTERM), got %d", code, code)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("cleanup commands did not run: %v; stderr: %s", err, stderr.String())
	}
	if want := "cleanup succeeded\n"; string(got) != want {
		t.Errorf("cleanup marker = %q, want %q", got, want)
	}
	if !strings.Contains(stderr.String(), "Exit code: 143") {
		t.Errorf("stderr = %q, want Exit code: 143 banner", stderr.String())
	}
}

func containerCommands(t *testing.T, manifest string) [][]string {
	t.Helper()
	var out [][]string
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch n := v.(type) {
		case map[string]interface{}:
			for k, child := range n {
				if list, ok := child.([]interface{}); ok && k == "command" {
					var argv []string
					for _, a := range list {
						s, _ := a.(string)
						argv = append(argv, s)
					}
					out = append(out, argv)
					continue
				}
				walk(child)
			}
		case []interface{}:
			for _, child := range n {
				walk(child)
			}
		}
	}
	for _, doc := range strings.Split(manifest, "\n---\n") {
		var parsed interface{}
		if err := k8syaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Fatalf("manifest is not valid YAML: %v\n%s", err, doc)
		}
		walk(parsed)
	}
	return out
}

func assertRendersWorkloadCommand(t *testing.T, manifest, userCommand string) {
	t.Helper()
	want := workloadContainerCommand(userCommand)
	for _, argv := range containerCommands(t, manifest) {
		if reflect.DeepEqual(argv, want) {
			return
		}
	}
	t.Errorf("no container runs workloadContainerCommand(%q).\ncommands found: %q\nmanifest:\n%s", userCommand, containerCommands(t, manifest), manifest)
}

func TestGenerateGKEManifest_UsesWorkloadContainerCommand(t *testing.T) {
	setupMockMachineConfig(t)
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe nvidia-l4 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 1}]}`},
		},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterDesc.NodePools = []gkeJobNodePool{{Config: gkeNodePoolConfig{MachineType: "nvidia-l4"}}}

	userCommand := "set -e\npython3 -u run.py | tee benchmark.log\necho \"done: ${PIPESTATUS[0]}\""
	manifest, err := orc.GenerateGKEManifest(ManifestOptions{
		WorkloadName:    "test-workload",
		FullImageName:   "test-image:latest",
		CommandToRun:    userCommand,
		ComputeType:     "nvidia-l4",
		MachineType:     "nvidia-l4",
		ClusterLocation: "us-central1-a",
	}, JobProfile{})
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}
	assertRendersWorkloadCommand(t, manifest, userCommand)
}

func TestGeneratePathwaysManifest_UsesWorkloadContainerCommand(t *testing.T) {
	for name, userCommand := range map[string]string{
		"single line with quotes": `pip install pathwaysutils && python -c 'import jax; print("JAX Device count:", jax.device_count())'`,
		"multi-line":              "set -e\npython3 -u train.py | tee train.log\necho \"exit: ${PIPESTATUS[0]}\"",
	} {
		t.Run(name, func(t *testing.T) {
			setupMockMachineConfig(t)
			job := orchestrator.JobDefinition{
				WorkloadName:    "pathways-test",
				CommandToRun:    userCommand,
				NumSlices:       1,
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
			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
			})
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
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}
			assertRendersWorkloadCommand(t, manifest, userCommand)
		})
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
		`kill -TERM -- \"-$child\"`,
		`echo \"Exit code: $rc\"`,
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
		},
		IsPathwaysJob:          true,
		GKEMTCEnabled:          true,
		GKEMTCRamdiskDirectory: "/tmp/mtc_checkpoints",
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
		"kubectl get topologies.kueue.x-k8s.io -o json": {{ExitCode: 0, Stdout: `{"items":[{"metadata":{"name":"tpu-topology"},"spec":{"levels":[{"nodeLabel":"cloud.google.com/gke-tpu-partition-4x4x4-id"}]}}]}`}},
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

func TestGeneratePathwaysManifest_CommandWithQuotes(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-test",
		CommandToRun:    `pip install pathwaysutils && python -c 'import pathwaysutils; pathwaysutils.initialize(); import jax; print("JAX Device count:", jax.device_count())'`,
		NumSlices:       1,
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

	expectedCommand := `pip install pathwaysutils && python -c 'import pathwaysutils; pathwaysutils.initialize(); import jax; print(\"JAX Device count:\", jax.device_count())'`
	if !strings.Contains(manifest, expectedCommand) {
		t.Errorf("manifest does not contain expected command exactly.\nExpected to find: %q\nManifest: %s", expectedCommand, manifest)
	}
}

func TestGeneratePathwaysManifest_DefaultImages(t *testing.T) {
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-defaults-test",
		CommandToRun:    "echo test",
		NumSlices:       1,
		ClusterLocation: "us-central1",
		ComputeType:     "n2-standard-2",
		Pathways: orchestrator.PathwaysJobDefinition{
			// All image fields left empty to test fallback to defaults
			GCSLocation:  "gs://my-bucket",
			HeadNodePool: "pathways-np",
		},
	}

	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
	}
	profile, isDynamic, isStatic, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", profile, isDynamic, isStatic)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}

	if !strings.Contains(manifest, defaultPathwaysProxyImage) {
		t.Errorf("manifest missing default proxy image %q", defaultPathwaysProxyImage)
	}
	if !strings.Contains(manifest, defaultPathwaysServerImage) {
		t.Errorf("manifest missing default server image %q", defaultPathwaysServerImage)
	}
}
