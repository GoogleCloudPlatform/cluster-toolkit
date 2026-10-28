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
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

	if !strings.Contains(gpuManifest, "cloud.google.com/placement-policy-name: my-gpu-group") {
		t.Errorf("Expected cloud.google.com/placement-policy-name for static GPU, got:\n%s", gpuManifest)
	}
	if strings.Contains(gpuManifest, "cloud.google.com/gke-placement-group") {
		t.Errorf("Did not expect cloud.google.com/gke-placement-group for static GPU, got:\n%s", gpuManifest)
	}

	// Under NAP the same policy is also used as the GKE placement group identifier.
	orc.napEnabled = true
	orc.napLimits = map[string]int64{"nvidia.com/gpu": 100}
	napJob := gpuJob
	napJob.GKENAPProvisioning = "auto"
	napProfile, isDyn, isStat, err := orc.resolveHardwareRequirements(&napJob)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed for NAP GPU: %v", err)
	}
	napOpts, err := orc.PrepareManifestOptions(napJob, "test-image:latest", napProfile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed for NAP GPU: %v", err)
	}
	napManifest, err := orc.GenerateGKEManifest(napOpts, napProfile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed for NAP GPU: %v", err)
	}
	for _, want := range []string{"cloud.google.com/placement-policy-name: my-gpu-group", "cloud.google.com/gke-placement-group: my-gpu-group"} {
		if !strings.Contains(napManifest, want) {
			t.Errorf("Expected %q for NAP GPU, got:\n%s", want, napManifest)
		}
	}
}

func TestGenerateGKEManifest_A4X_WorkloadPolicy(t *testing.T) {
	setupMockMachineConfig(t)
	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes -o jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}": {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe a4x-highgpu-4g --zone=us-central1-a --format=json":                               {{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "nvidia-gb200"}]}`}},
	}
	render := func(t *testing.T, orc *GKEOrchestrator, job orchestrator.JobDefinition) string {
		t.Helper()
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
		return manifest
	}
	baseJob := orchestrator.JobDefinition{
		WorkloadName:    "a4x-job",
		CommandToRun:    "echo hello",
		ComputeType:     "a4x-highgpu-4g",
		ClusterLocation: "us-central1-a",
		ProjectID:       "mock-project",
	}

	t.Run("static cluster inherits node pool policy without flag", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
		orc.projectID = "mock-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.clusterDesc.NodePools = []gkeJobNodePool{{
			Name:            "a4x-pool",
			Config:          gkeNodePoolConfig{MachineType: "a4x-highgpu-4g"},
			PlacementPolicy: &gkePlacementPolicy{PolicyName: "a4x-workload-policy"},
		}}
		manifest := render(t, orc, baseJob)
		if !strings.Contains(manifest, "cloud.google.com/placement-policy-name: a4x-workload-policy") {
			t.Errorf("Expected node pool workload policy in manifest, got:\n%s", manifest)
		}
		if strings.Contains(manifest, "cloud.google.com/gke-placement-group") {
			t.Errorf("A4X must not carry gke-placement-group, got:\n%s", manifest)
		}
	})

	t.Run("NAP resolves canonical policy and never emits placement group", func(t *testing.T) {
		responses := map[string][]shell.CommandResult{
			"gcloud compute resource-policies describe gb200-4-1x72-placement-policy --region=us-central1 --project=mock-project --format=json": {
				{ExitCode: 0, Stdout: `{"name":"gb200-4-1x72-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"1x72"}}`},
			},
		}
		for k, v := range mockResponses {
			responses[k] = v
		}
		orc := newTestGKEOrchestrator(NewMockExecutor(responses))
		orc.projectID = "mock-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.napEnabled = true
		orc.napLimits = map[string]int64{"nvidia.com/gpu": 100}
		napJob := baseJob
		napJob.GKENAPProvisioning = "auto"
		manifest := render(t, orc, napJob)
		if !strings.Contains(manifest, "cloud.google.com/placement-policy-name: gb200-4-1x72-placement-policy") {
			t.Errorf("Expected canonical workload policy in manifest, got:\n%s", manifest)
		}
		if strings.Contains(manifest, "cloud.google.com/gke-placement-group") {
			t.Errorf("A4X must not carry gke-placement-group under NAP, got:\n%s", manifest)
		}
	})
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

// renderPathwaysTestManifest renders a Pathways manifest with the given head
// mode, colocated Python sidecar image, MTC ramdisk directory (empty disables
// MTC) and --mount values, failing the test on error.
func renderPathwaysTestManifest(t *testing.T, headless bool, sidecarImage, ramdiskDir string, mounts []string) string {
	t.Helper()
	manifest, err := generatePathwaysTestManifest(t, "", headless, sidecarImage, ramdiskDir, mounts)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}
	return manifest
}

// generatePathwaysTestManifest is renderPathwaysTestManifest with an optional
// custom templates directory, returning the GeneratePathwaysManifest error.
func generatePathwaysTestManifest(t *testing.T, customTemplatesPath string, headless bool, sidecarImage, ramdiskDir string, mounts []string) (string, error) {
	t.Helper()
	setupMockMachineConfig(t)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pathways-test",
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
		GKEMTCEnabled:          ramdiskDir != "",
		GKEMTCRamdiskDirectory: ramdiskDir,
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
	orc.gkeCustomTemplatesPath = customTemplatesPath
	profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	return orc.GeneratePathwaysManifest(job, image, profile, isDynamicSlicing, isStaticSlicing)
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
			manifest := renderPathwaysTestManifest(t, tc.headless, tc.sidecar, "", tc.mounts)
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
			if rj.Template.Spec.Template.Spec.RestartPolicy != "Never" {
				t.Errorf("expected worker restartPolicy to be Never, got %q", rj.Template.Spec.Template.Spec.RestartPolicy)
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
			wantWorkerRestart:  "Never",
		},
		{
			name:               "pathways, with restart-on-exit-codes",
			isPathways:         true,
			restartOnExitCodes: []int{137, 143},
			wantHeadPFP:        true,
			wantHeadRestart:    "Never",
			wantWorkerPFP:      false,
			wantWorkerRestart:  "Never",
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
		"backoffLimitPerIndex: 0",
		"podReplacementPolicy: Failed",
		"maxFailedIndexes: 0",
	}

	for _, substr := range expectedSubstrs {
		if !strings.Contains(manifest, substr) {
			t.Errorf("manifest missing expected substring %q", substr)
		}
	}

	gotReplicatedAnnotations := jobSetReplicatedJobAnnotations(t, manifest)
	if got := gotReplicatedAnnotations["pathways-head"]["alpha.jobset.sigs.k8s.io/exclusive-topology"]; got != "kubernetes.io/hostname" {
		t.Errorf("pathways-head exclusive-topology = %q, want %q", got, "kubernetes.io/hostname")
	}
	if got := gotReplicatedAnnotations["worker"]["alpha.jobset.sigs.k8s.io/exclusive-topology"]; got != "kubernetes.io/hostname" {
		t.Errorf("single-node worker exclusive-topology = %q, want %q", got, "kubernetes.io/hostname")
	}
}

func TestGeneratePathwaysManifest_HeadContainerResources(t *testing.T) {
	type wantResources struct {
		limits   corev1.ResourceList
		requests corev1.ResourceList
	}

	tests := []struct {
		name           string
		headless       bool
		wantContainers map[string]wantResources
	}{
		{
			name:     "default with workload-container",
			headless: false,
			wantContainers: map[string]wantResources{
				"pathways-proxy": {
					limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("16"),
						corev1.ResourceMemory: resource.MustParse("100Gi"),
					},
					requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
					},
				},
				"pathways-rm": {
					limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
					},
					requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
				"workload-container": {
					limits: nil,
					requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
		},
		{
			name:     "headless",
			headless: true,
			wantContainers: map[string]wantResources{
				"pathways-proxy": {
					limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("16"),
						corev1.ResourceMemory: resource.MustParse("100Gi"),
					},
					requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
					},
				},
				"pathways-rm": {
					limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
					},
					requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
		},
	}

	quantityOpt := cmp.Comparer(func(x, y resource.Quantity) bool {
		return x.Cmp(y) == 0
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := renderPathwaysTestManifest(t, tc.headless, "", "", nil)
			specs := pathwaysPodSpecs(t, manifest)
			head, ok := specs["pathways-head"]
			if !ok {
				t.Fatalf("pathways-head replicatedJob not found in manifest:\n%s", manifest)
			}

			containers := podContainers(head)
			if len(containers) != len(tc.wantContainers) {
				t.Fatalf("pathways-head container count = %d, want %d", len(containers), len(tc.wantContainers))
			}

			for _, c := range containers {
				want, ok := tc.wantContainers[c.Name]
				if !ok {
					t.Errorf("unexpected container %q in pathways-head", c.Name)
					continue
				}
				if diff := cmp.Diff(want.limits, c.Resources.Limits, quantityOpt, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("container %q limits mismatch (-want +got):\n%s", c.Name, diff)
				}
				if diff := cmp.Diff(want.requests, c.Resources.Requests, quantityOpt, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("container %q requests mismatch (-want +got):\n%s", c.Name, diff)
				}
			}
		})
	}
}

// findContainer returns the init or regular container with the given name, or nil.
func findContainer(spec *corev1.PodSpec, name string) *corev1.Container {
	for _, c := range podContainers(spec) {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// volumeMountPaths maps each volume a container mounts to its mount path.
func volumeMountPaths(c *corev1.Container) map[string]string {
	paths := make(map[string]string, len(c.VolumeMounts))
	for _, m := range c.VolumeMounts {
		paths[m.Name] = m.MountPath
	}
	return paths
}

// assertNoPodVolumes checks that a pod neither declares nor mounts the named volumes.
func assertNoPodVolumes(t *testing.T, podName string, spec *corev1.PodSpec, names ...string) {
	t.Helper()
	if spec == nil {
		t.Fatalf("%s replicatedJob not found in manifest", podName)
	}
	for _, name := range names {
		if v := findPodVolume(spec, name); v != nil {
			t.Errorf("%s pod declares volume %q (%+v), want none", podName, name, v)
		}
		for _, c := range podContainers(spec) {
			if _, ok := volumeMountPaths(c)[name]; ok {
				t.Errorf("%s container %q mounts volume %q, want none", podName, c.Name, name)
			}
		}
	}
}

// expectedWorkerMounts returns the mounts the built-in Pathways worker volumes add to a container.
func expectedWorkerMounts(ramdiskDir string, sidecarShm bool) map[string]string {
	want := map[string]string{"shared-tmp": "/tmp"}
	if ramdiskDir != "" {
		want["cache"] = ramdiskDir
	}
	if sidecarShm {
		want["sidecar-shared-memory"] = "/tmp/sidecar"
	}
	return want
}

// assertWorkerCacheVolume checks that the worker declares the MTC CSI cache volume only when ramdiskDir is set.
func assertWorkerCacheVolume(t *testing.T, worker *corev1.PodSpec, ramdiskDir string) {
	t.Helper()
	v := findPodVolume(worker, "cache")
	if ramdiskDir == "" {
		if v != nil {
			t.Errorf("worker declares volume cache (%+v) without MTC", v)
		}
		return
	}
	if v == nil || v.CSI == nil || v.CSI.Driver != multitierCheckpointCSIDriver {
		t.Errorf("worker volume cache = %+v, want a CSI volume with driver %s", v, multitierCheckpointCSIDriver)
	}
}

// assertWorkerMTCVolumes checks the worker's MTC cache and colocated-sidecar volumes, their mounts and the sidecar shm flag.
func assertWorkerMTCVolumes(t *testing.T, worker *corev1.PodSpec, ramdiskDir string, wantSidecar bool) {
	t.Helper()
	if worker == nil {
		t.Fatalf("worker replicatedJob not found in manifest")
	}
	assertWorkerCacheVolume(t, worker, ramdiskDir)
	if got := isMemoryEmptyDir(findPodVolume(worker, "sidecar-shared-memory")); got != wantSidecar {
		t.Errorf("worker declares in-memory sidecar-shared-memory volume = %v, want %v", got, wantSidecar)
	}
	sidecar := findContainer(worker, "colocated-python-sidecar")
	if got := sidecar != nil; got != wantSidecar {
		t.Errorf("worker has colocated-python-sidecar container = %v, want %v", got, wantSidecar)
	}
	if sidecar != nil {
		if diff := cmp.Diff(expectedWorkerMounts(ramdiskDir, true), volumeMountPaths(sidecar)); diff != "" {
			t.Errorf("colocated-python-sidecar volume mounts mismatch (-want +got):\n%s", diff)
		}
	}

	pw := findContainer(worker, "pathways-worker")
	if pw == nil {
		t.Fatalf("worker pod has no pathways-worker container")
	}
	if diff := cmp.Diff(expectedWorkerMounts(ramdiskDir, wantSidecar), volumeMountPaths(pw)); diff != "" {
		t.Errorf("pathways-worker volume mounts mismatch (-want +got):\n%s", diff)
	}
	const shmFlag = "--cloud_pathways_sidecar_shm_directory=/tmp/sidecar"
	if got := slices.Contains(pw.Args, shmFlag); got != wantSidecar {
		t.Errorf("pathways-worker args contain %s = %v, want %v", shmFlag, got, wantSidecar)
	}
}

// TestGeneratePathwaysManifest_MTC verifies that MTC volumes are only declared and mounted on Pathways worker pods.
func TestGeneratePathwaysManifest_MTC(t *testing.T) {
	tests := []struct {
		name       string
		headless   bool
		sidecar    string
		ramdiskDir string // empty disables MTC
	}{
		{name: "with colocated python sidecar", sidecar: "sidecar:latest", ramdiskDir: "/tmp/mtc_checkpoints"},
		{name: "without colocated python sidecar", ramdiskDir: "/tmp/mtc_checkpoints"},
		{name: "headless", headless: true, ramdiskDir: "/tmp/mtc_checkpoints"},
		{name: "ramdisk path with YAML comment character", sidecar: "sidecar:latest", ramdiskDir: "/tmp/mtc #1"},
		{name: "MTC disabled with colocated python sidecar", sidecar: "sidecar:latest"},
		{name: "MTC disabled without colocated python sidecar"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specs := pathwaysPodSpecs(t, renderPathwaysTestManifest(t, tc.headless, tc.sidecar, tc.ramdiskDir, nil))
			assertNoPodVolumes(t, "pathways-head", specs["pathways-head"], "cache", "sidecar-shared-memory")
			assertWorkerMTCVolumes(t, specs["worker"], tc.ramdiskDir, tc.sidecar != "")
			assertUniqueVolumeNames(t, "worker", specs["worker"])
		})
	}
}

func TestGeneratePathwaysManifest_MTCRejectsStaleCustomTemplate(t *testing.T) {
	dir := t.TempDir()
	// A custom pathways_jobset.tmpl from an older gcluster version has no MTC volume.
	stale := "apiVersion: jobset.x-k8s.io/v1alpha2\nkind: JobSet\nmetadata:\n  name: stale\n"
	if err := os.WriteFile(filepath.Join(dir, "pathways_jobset.tmpl"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := generatePathwaysTestManifest(t, dir, false, "", "/tmp/mtc_checkpoints", nil)
	if err == nil || !strings.Contains(err.Error(), "re-extract pathways_jobset.tmpl") {
		t.Fatalf("GeneratePathwaysManifest() error = %v, want a hint to re-extract pathways_jobset.tmpl", err)
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

	// 3. Worker replicatedJob must NOT set exclusive-topology when dynamic slicing is active
	gotReplicatedAnnotations := jobSetReplicatedJobAnnotations(t, manifest)
	if got, exists := gotReplicatedAnnotations["worker"]["alpha.jobset.sigs.k8s.io/exclusive-topology"]; exists {
		t.Errorf("dynamic-slicing worker exclusive-topology = %q, want omitted", got)
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

func TestGeneratePathwaysManifest_MaxSliceRestarts(t *testing.T) {
	tests := []struct {
		name                     string
		maxSliceRestarts         int
		wantBackoffLimit         string
		wantBackoffLimitPerIndex string
		wantRestartPolicy        string
	}{
		{
			name:                     "flag set to 5 on 2 x v6e-32",
			maxSliceRestarts:         5,
			wantBackoffLimit:         "backoffLimit: 40",
			wantBackoffLimitPerIndex: "backoffLimitPerIndex: 5",
			wantRestartPolicy:        "restartPolicy: OnFailure",
		},
		{
			name:                     "non-elastic default 0 on 2 x v6e-32",
			maxSliceRestarts:         0,
			wantBackoffLimit:         "backoffLimit: 0",
			wantBackoffLimitPerIndex: "backoffLimitPerIndex: 0",
			wantRestartPolicy:        "restartPolicy: Never",
		},
		{
			name:                     "elastic default 4000 on 2 x v6e-32",
			maxSliceRestarts:         4000,
			wantBackoffLimit:         "backoffLimit: 32000",
			wantBackoffLimitPerIndex: "backoffLimitPerIndex: 4000",
			wantRestartPolicy:        "restartPolicy: OnFailure",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupMockMachineConfig(t)
			job := orchestrator.JobDefinition{
				WorkloadName:    "pathways-restarts-test",
				CommandToRun:    "python3 train.py",
				NumSlices:       2,
				ClusterLocation: "us-central1-a",
				ComputeType:     "v6e-32",
				IsPathwaysJob:   true,
				Pathways: orchestrator.PathwaysJobDefinition{
					ProxyServerImage: "proxy:latest",
					ServerImage:      "server:latest",
					WorkerImage:      "worker:latest",
					GCSLocation:      "gs://my-bucket",
					HeadNodePool:     "pathways-np",
					MaxSliceRestarts: tc.maxSliceRestarts,
				},
			}

			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors":                   {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes":                             {{ExitCode: 0, Stdout: "4x8\n"}},
				"kubectl get topologies.kueue.x-k8s.io -o json": {{ExitCode: 0, Stdout: `{"items":[]}`}},
				"gcloud compute machine-types describe ct6e-standard-4t --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v6e-slice"}]}`},
				},
			}
			orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
			orc.projectID = "mock-project"
			orc.clusterZones = []string{"us-central1-a"}
			orc.clusterDesc.NodePools = []gkeJobNodePool{
				{
					Name:            "v6e-pool",
					Config:          gkeNodePoolConfig{MachineType: "ct6e-standard-4t"},
					PlacementPolicy: &gkePlacementPolicy{TpuTopology: "4x8"},
				},
			}

			profile, isDynamic, isStatic, err := orc.resolveHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveHardwareRequirements failed: %v", err)
			}
			if job.NodesPerSlice != 8 {
				t.Fatalf("expected NodesPerSlice = 8 for v6e-32, got %d", job.NodesPerSlice)
			}

			manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", profile, isDynamic, isStatic)
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}

			parts := strings.Split(manifest, "name: worker")
			if len(parts) < 2 {
				t.Fatalf("worker replicatedJob not found in manifest:\n%s", manifest)
			}
			workerManifest := parts[1]
			if !strings.Contains(workerManifest, tc.wantBackoffLimit) {
				t.Errorf("worker spec missing %q\nGot:\n%s", tc.wantBackoffLimit, workerManifest)
			}
			if !strings.Contains(workerManifest, tc.wantBackoffLimitPerIndex) {
				t.Errorf("worker spec missing %q\nGot:\n%s", tc.wantBackoffLimitPerIndex, workerManifest)
			}
			if !strings.Contains(workerManifest, tc.wantRestartPolicy) {
				t.Errorf("worker spec missing %q\nGot:\n%s", tc.wantRestartPolicy, workerManifest)
			}
		})
	}
}

func jobSetTopLevelAnnotations(t *testing.T, manifest string) map[string]string {
	t.Helper()
	docs, err := splitYAMLDocuments(manifest)
	if err != nil {
		t.Fatalf("failed to split manifest into YAML documents: %v", err)
	}
	for _, doc := range docs {
		var js struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		}
		if err := k8syaml.Unmarshal([]byte(doc), &js); err != nil {
			t.Fatalf("failed to unmarshal manifest document: %v\n%s", err, doc)
		}
		if js.Kind == "JobSet" {
			return js.Metadata.Annotations
		}
	}
	t.Fatalf("no JobSet document found in manifest:\n%s", manifest)
	return nil
}

func jobSetReplicatedJobAnnotations(t *testing.T, manifest string) map[string]map[string]string {
	t.Helper()
	docs, err := splitYAMLDocuments(manifest)
	if err != nil {
		t.Fatalf("failed to split manifest into YAML documents: %v", err)
	}
	for _, doc := range docs {
		var js struct {
			Kind string `json:"kind"`
			Spec struct {
				ReplicatedJobs []struct {
					Name     string `json:"name"`
					Template struct {
						Metadata struct {
							Annotations map[string]string `json:"annotations"`
						} `json:"metadata"`
					} `json:"template"`
				} `json:"replicatedJobs"`
			} `json:"spec"`
		}
		if err := k8syaml.Unmarshal([]byte(doc), &js); err != nil {
			t.Fatalf("failed to unmarshal manifest document: %v\n%s", err, doc)
		}
		if js.Kind != "JobSet" {
			continue
		}
		out := make(map[string]map[string]string, len(js.Spec.ReplicatedJobs))
		for _, rj := range js.Spec.ReplicatedJobs {
			out[rj.Name] = rj.Template.Metadata.Annotations
		}
		return out
	}
	t.Fatalf("no JobSet document found in manifest:\n%s", manifest)
	return nil
}

func TestGenerateGKEManifest_ExclusiveTopology(t *testing.T) {
	setupMockMachineConfig(t)
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}

	profile := JobProfile{
		IsCPUMachine:  true,
		CapacityCount: 4,
	}

	tests := []struct {
		name             string
		nodesPerSlice    int
		isDynamicSlicing bool
		isStaticSlicing  bool
		wantTopology     string
		wantExists       bool
	}{
		{
			name:          "single node slice (NodesPerSlice=1) uses hostname",
			nodesPerSlice: 1,
			wantTopology:  "kubernetes.io/hostname",
			wantExists:    true,
		},
		{
			name:          "unset nodesPerSlice (NodesPerSlice=0) uses hostname",
			nodesPerSlice: 0,
			wantTopology:  "kubernetes.io/hostname",
			wantExists:    true,
		},
		{
			name:          "multi-node slice (NodesPerSlice=2) uses gke-nodepool",
			nodesPerSlice: 2,
			wantTopology:  "cloud.google.com/gke-nodepool",
			wantExists:    true,
		},
		{
			name:             "dynamic slicing omits exclusive-topology",
			nodesPerSlice:    16,
			isDynamicSlicing: true,
			wantExists:       false,
		},
		{
			name:            "static slicing omits exclusive-topology",
			nodesPerSlice:   1,
			isStaticSlicing: true,
			wantExists:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := ManifestOptions{
				WorkloadName:     "test-exclusive-topo",
				FullImageName:    "test-image:latest",
				CommandToRun:     "echo hello",
				ComputeType:      "n2-standard-4",
				MachineType:      "n2-standard-4",
				ClusterLocation:  "us-central1-a",
				NumSlices:        1,
				NodesPerSlice:    tc.nodesPerSlice,
				IsDynamicSlicing: tc.isDynamicSlicing,
				IsStaticSlicing:  tc.isStaticSlicing,
			}

			manifest, err := orc.GenerateGKEManifest(opts, profile)
			if err != nil {
				t.Fatalf("GenerateGKEManifest failed: %v", err)
			}

			annotations := jobSetTopLevelAnnotations(t, manifest)
			got, exists := annotations["alpha.jobset.sigs.k8s.io/exclusive-topology"]
			if exists != tc.wantExists {
				t.Fatalf("exclusive-topology exists = %v (val=%q), want %v\nManifest:\n%s", exists, got, tc.wantExists, manifest)
			}
			if tc.wantExists && got != tc.wantTopology {
				t.Errorf("exclusive-topology = %q, want %q\nManifest:\n%s", got, tc.wantTopology, manifest)
			}
		})
	}

	tpuTests := []struct {
		name         string
		topology     string
		wantNodes    int
		wantTopology string
	}{
		{
			name:         "end-to-end tpu7x-standard-4t 2x2x1 derives NodesPerSlice=1 and uses hostname",
			topology:     "2x2x1",
			wantNodes:    1,
			wantTopology: "kubernetes.io/hostname",
		},
		{
			name:         "end-to-end tpu7x-standard-4t 2x2x2 derives NodesPerSlice=2 and uses gke-nodepool",
			topology:     "2x2x2",
			wantNodes:    2,
			wantTopology: "cloud.google.com/gke-nodepool",
		},
	}

	for _, tc := range tpuTests {
		t.Run(tc.name, func(t *testing.T) {
			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
				"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v7x-slice"}]}`},
				},
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=mock-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
				},
			}
			mockExec := NewMockExecutor(mockResponses)
			tpuOrc := newTestGKEOrchestrator(mockExec)
			tpuOrc.projectID = "mock-project"
			tpuOrc.clusterZones = []string{"us-central1-a"}
			tpuOrc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
			tpuOrc.clusterDesc.NodePools = []gkeJobNodePool{
				{Name: "v7x-pool", Config: gkeNodePoolConfig{MachineType: "tpu7x-standard-4t"}},
			}

			job := orchestrator.JobDefinition{
				WorkloadName:    "tpu7x-exclusive-topo",
				CommandToRun:    "echo hello",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        tc.topology,
				NumSlices:       1,
				ClusterLocation: "us-central1-a",
				ProjectID:       "mock-project",
			}

			tpuProfile, isDyn, isStat, err := tpuOrc.resolveHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveHardwareRequirements failed: %v", err)
			}
			if job.NodesPerSlice != tc.wantNodes {
				t.Fatalf("resolveHardwareRequirements derived NodesPerSlice = %d, want %d", job.NodesPerSlice, tc.wantNodes)
			}

			opts, err := tpuOrc.PrepareManifestOptions(job, "test-image:latest", tpuProfile, isDyn, isStat)
			if err != nil {
				t.Fatalf("PrepareManifestOptions failed: %v", err)
			}

			manifest, err := tpuOrc.GenerateGKEManifest(opts, tpuProfile)
			if err != nil {
				t.Fatalf("GenerateGKEManifest failed: %v", err)
			}

			annotations := jobSetTopLevelAnnotations(t, manifest)
			if got := annotations["alpha.jobset.sigs.k8s.io/exclusive-topology"]; got != tc.wantTopology {
				t.Errorf("exclusive-topology = %q, want %q\nManifest:\n%s", got, tc.wantTopology, manifest)
			}
		})
	}
}

func TestGeneratePathwaysManifest_ExclusiveTopology(t *testing.T) {
	tests := []struct {
		name             string
		topology         string
		wantNodes        int
		isDynamicSlicing bool
		isStaticSlicing  bool
		wantWorkerTopo   string
		wantWorkerExists bool
	}{
		{
			name:             "single-host TPU slice (2x2x1, NodesPerSlice=1) uses hostname",
			topology:         "2x2x1",
			wantNodes:        1,
			wantWorkerTopo:   "kubernetes.io/hostname",
			wantWorkerExists: true,
		},
		{
			name:             "multi-host TPU slice (2x2x2, NodesPerSlice=2) uses gke-nodepool",
			topology:         "2x2x2",
			wantNodes:        2,
			wantWorkerTopo:   "cloud.google.com/gke-nodepool",
			wantWorkerExists: true,
		},
		{
			name:             "dynamic slicing omits worker exclusive-topology",
			topology:         "2x2x2",
			wantNodes:        2,
			isDynamicSlicing: true,
			wantWorkerExists: false,
		},
		{
			name:             "static slicing omits worker exclusive-topology",
			topology:         "2x2x1",
			wantNodes:        1,
			isStaticSlicing:  true,
			wantWorkerExists: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupMockMachineConfig(t)
			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
				"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v7x-slice"}]}`},
				},
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=mock-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
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

			job := orchestrator.JobDefinition{
				WorkloadName:    "pathways-topo-test",
				CommandToRun:    "python3 train.py",
				NumSlices:       2,
				ClusterLocation: "us-central1-a",
				ComputeType:     "tpu7x-standard-4t",
				Topology:        tc.topology,
				IsPathwaysJob:   true,
				Pathways: orchestrator.PathwaysJobDefinition{
					ProxyServerImage: "proxy:latest",
					ServerImage:      "server:latest",
					WorkerImage:      "worker:latest",
					GCSLocation:      "gs://my-bucket",
					HeadNodePool:     "pathways-np",
				},
			}

			profile, _, _, err := orc.resolveHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveHardwareRequirements failed: %v", err)
			}
			if job.NodesPerSlice != tc.wantNodes {
				t.Fatalf("resolveHardwareRequirements derived NodesPerSlice = %d, want %d", job.NodesPerSlice, tc.wantNodes)
			}

			manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", profile, tc.isDynamicSlicing, tc.isStaticSlicing)
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}

			replicatedAnnotations := jobSetReplicatedJobAnnotations(t, manifest)
			if gotHead := replicatedAnnotations["pathways-head"]["alpha.jobset.sigs.k8s.io/exclusive-topology"]; gotHead != "kubernetes.io/hostname" {
				t.Errorf("pathways-head exclusive-topology = %q, want %q", gotHead, "kubernetes.io/hostname")
			}

			gotWorker, exists := replicatedAnnotations["worker"]["alpha.jobset.sigs.k8s.io/exclusive-topology"]
			if exists != tc.wantWorkerExists {
				t.Fatalf("worker exclusive-topology exists = %v (val=%q), want %v\nManifest:\n%s", exists, gotWorker, tc.wantWorkerExists, manifest)
			}
			if tc.wantWorkerExists && gotWorker != tc.wantWorkerTopo {
				t.Errorf("worker exclusive-topology = %q, want %q\nManifest:\n%s", gotWorker, tc.wantWorkerTopo, manifest)
			}
		})
	}
}

func assertContainerEnvMap(t *testing.T, c corev1.Container, want map[string]string) {
	t.Helper()
	got := make(map[string]string, len(c.Env))
	for _, e := range c.Env {
		if _, dup := got[e.Name]; dup {
			t.Errorf("container %q has duplicate env var %q", c.Name, e.Name)
		}
		got[e.Name] = e.Value
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("container %q env mismatch (-want +got):\n%s", c.Name, diff)
	}
}

func assertJobSetDNSAndIndexed(t *testing.T, manifest string) {
	t.Helper()
	var js struct {
		Spec struct {
			Network struct {
				EnableDNSHostnames       bool `json:"enableDNSHostnames"`
				PublishNotReadyAddresses bool `json:"publishNotReadyAddresses"`
			} `json:"network"`
			ReplicatedJobs []struct {
				Name     string `json:"name"`
				Template struct {
					Spec struct {
						CompletionMode string `json:"completionMode"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"replicatedJobs"`
		} `json:"spec"`
	}
	if err := k8syaml.Unmarshal([]byte(manifest), &js); err != nil {
		t.Fatalf("failed to unmarshal JobSet manifest: %v\n%s", err, manifest)
	}
	if !js.Spec.Network.EnableDNSHostnames || !js.Spec.Network.PublishNotReadyAddresses {
		t.Errorf("JobSet spec.network = %+v, want enableDNSHostnames=true and publishNotReadyAddresses=true", js.Spec.Network)
	}
	if len(js.Spec.ReplicatedJobs) != 1 || js.Spec.ReplicatedJobs[0].Template.Spec.CompletionMode != "Indexed" {
		t.Errorf("main-job completionMode = %+v, want Indexed", js.Spec.ReplicatedJobs)
	}
}

func TestGenerateGKEManifest_CoordinatorEnv(t *testing.T) {
	setupMockMachineConfig(t)
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}

	tests := []struct {
		name         string
		workloadName string
		userEnv      map[string]string
		wantEnv      map[string]string
	}{
		{
			name:         "injects default MASTER_ADDR and WORKERS_0_HOSTNAME when unset",
			workloadName: "torch-job",
			userEnv:      nil,
			wantEnv: map[string]string{
				"MASTER_ADDR":        "torch-job-main-job-0-0.torch-job",
				"WORKERS_0_HOSTNAME": "torch-job-main-job-0-0.torch-job",
			},
		},
		{
			name:         "supports 28-character workload name without label truncation",
			workloadName: "max-len-28-char-workload-nam",
			userEnv:      map[string]string{"CUSTOM_KEY": "val"},
			wantEnv: map[string]string{
				"CUSTOM_KEY":         "val",
				"MASTER_ADDR":        "max-len-28-char-workload-nam-main-job-0-0.max-len-28-char-workload-nam",
				"WORKERS_0_HOSTNAME": "max-len-28-char-workload-nam-main-job-0-0.max-len-28-char-workload-nam",
			},
		},
		{
			name:         "user override for WORKERS_0_HOSTNAME preserves default MASTER_ADDR",
			workloadName: "torch-job",
			userEnv: map[string]string{
				"WORKERS_0_HOSTNAME": "custom-worker-0.custom-svc",
			},
			wantEnv: map[string]string{
				"MASTER_ADDR":        "torch-job-main-job-0-0.torch-job",
				"WORKERS_0_HOSTNAME": "custom-worker-0.custom-svc",
			},
		},
		{
			name:         "user override for both MASTER_ADDR and WORKERS_0_HOSTNAME",
			workloadName: "torch-job",
			userEnv: map[string]string{
				"MASTER_ADDR":        "10.0.0.1",
				"WORKERS_0_HOSTNAME": "rank0.local",
			},
			wantEnv: map[string]string{
				"MASTER_ADDR":        "10.0.0.1",
				"WORKERS_0_HOSTNAME": "rank0.local",
			},
		},
		{
			name:         "empty workload name does not inject coordinator env",
			workloadName: "",
			userEnv:      nil,
			wantEnv:      map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origLen := len(tc.userEnv)
			opts := ManifestOptions{
				WorkloadName:    tc.workloadName,
				FullImageName:   "test-image:latest",
				CommandToRun:    "torchrun train.py",
				ComputeType:     "n2-standard-4",
				MachineType:     "n2-standard-4",
				ClusterLocation: "us-central1-a",
				NumSlices:       1,
				NodesPerSlice:   2,
				Env:             tc.userEnv,
			}
			manifest, err := orc.GenerateGKEManifest(opts, profile)
			if err != nil {
				t.Fatalf("GenerateGKEManifest failed: %v", err)
			}
			if len(tc.userEnv) != origLen {
				t.Errorf("caller Env map was mutated: got len %d, want %d", len(tc.userEnv), origLen)
			}
			assertJobSetDNSAndIndexed(t, manifest)
			podSpec := pathwaysPodSpecs(t, manifest)["main-job"]
			if podSpec == nil || len(podSpec.Containers) != 1 {
				t.Fatalf("expected 1 container in main-job podSpec, got %+v", podSpec)
			}
			assertContainerEnvMap(t, podSpec.Containers[0], tc.wantEnv)
		})
	}

	t.Run("pathways manifest does not inject main-job coordinator env", func(t *testing.T) {
		manifest := renderPathwaysTestManifest(t, false, "", "", nil)
		if strings.Contains(manifest, "WORKERS_0_HOSTNAME") || strings.Contains(manifest, "MASTER_ADDR") {
			t.Errorf("Pathways manifest should not contain WORKERS_0_HOSTNAME or MASTER_ADDR, got:\n%s", manifest)
		}
	})
}

func TestGenerateGKEManifest_CoordinatorEnv_TPU7xParallelContainers(t *testing.T) {
	setupMockMachineConfig(t)
	mockResponses := map[string][]shell.CommandResult{
		"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
		"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
		"gcloud compute machine-types describe tpu7x-standard-4t --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v7x-slice"}]}`},
		},
		"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=mock-project --format=json": {
			{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
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

	job := orchestrator.JobDefinition{
		WorkloadName:          "tpu7x-torchrun",
		CommandToRun:          "torchrun train.py",
		ComputeType:           "tpu7x-standard-4t",
		Topology:              "2x2x2",
		NumSlices:             1,
		ClusterLocation:       "us-central1-a",
		ProjectID:             "mock-project",
		UseParallelContainers: true,
		Verbose:               true,
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

	podSpec := pathwaysPodSpecs(t, manifest)["main-job"]
	if podSpec == nil || len(podSpec.Containers) != 2 {
		t.Fatalf("expected 2 parallel containers in main-job podSpec, got %+v", podSpec)
	}
	wantEnv := map[string]string{
		"MASTER_ADDR":          "tpu7x-torchrun-main-job-0-0.tpu7x-torchrun",
		"WORKERS_0_HOSTNAME":   "tpu7x-torchrun-main-job-0-0.tpu7x-torchrun",
		"TPU_STDERR_LOG_LEVEL": "0",
		"TPU_MIN_LOG_LEVEL":    "0",
		"TF_CPP_MIN_LOG_LEVEL": "0",
		"TPU_VMODULE":          "real_program_continuator=1",
	}
	for _, c := range podSpec.Containers {
		assertContainerEnvMap(t, c, wantEnv)
	}
}
