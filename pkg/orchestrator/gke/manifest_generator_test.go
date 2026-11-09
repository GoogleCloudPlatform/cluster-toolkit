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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safetext/yamltemplate"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			name: "document with invalid YAML syntax fails before apply",
			manifest: `
apiVersion: v1
kind: ConfigMap
data: [unclosed bracket
`,
			expectErr: true,
			errSubstr: "failed to parse manifest document",
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

// envValues returns every value set for the named env var on a container, in
// order, so callers can also detect duplicates.
func envValues(c *corev1.Container, name string) []string {
	var values []string
	for _, e := range c.Env {
		if e.Name == name {
			values = append(values, e.Value)
		}
	}
	return values
}

// TestGeneratePathwaysManifest_GoldenYAMLAlignment covers the fields aligned
// with the Cloud Pathways golden JobSet YAMLs (pathwaysutils PathwaysJobSet).
func TestGeneratePathwaysManifest_GoldenYAMLAlignment(t *testing.T) {
	tests := []struct {
		name     string
		headless bool
	}{
		{name: "batch"},
		{name: "headless", headless: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := renderPathwaysTestManifest(t, tc.headless, "", "", nil)
			specs := pathwaysPodSpecs(t, manifest)
			head := specs["pathways-head"]
			if head == nil {
				t.Fatalf("pathways-head replicatedJob not found in manifest:\n%s", manifest)
			}

			// The resource manager starts before the proxy that connects to it.
			headContainers := head.InitContainers
			if tc.headless {
				headContainers = head.Containers
			}
			var names []string
			for _, c := range headContainers {
				names = append(names, c.Name)
			}
			if want := []string{"pathways-rm", "pathways-proxy"}; !reflect.DeepEqual(names, want) {
				t.Errorf("pathways-head system containers = %v, want %v", names, want)
			}

			if !tc.headless {
				workload := findContainer(head, "workload-container")
				if workload == nil {
					t.Fatalf("workload-container not found in manifest:\n%s", manifest)
				}
				if got := envValues(workload, "JAX_BACKEND_TARGET"); !reflect.DeepEqual(got, []string{"grpc://localhost:29000"}) {
					t.Errorf("workload-container JAX_BACKEND_TARGET = %v, want [grpc://localhost:29000]", got)
				}
			}
		})
	}
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

// TestGeneratePathwaysManifest_ColocatedSidecarUserMounts verifies that user
// --mount volumes are mounted inside colocated-python-sidecar alongside
// pathways-worker and workload-container.
func TestGeneratePathwaysManifest_ColocatedSidecarUserMounts(t *testing.T) {
	mounts := []string{
		"gs://my-bucket;/data;ro",
		"/host/path;/host",
		"custom-pvc;/mnt/pvc;rw",
	}
	tests := []struct {
		name       string
		headless   bool
		ramdiskDir string
	}{
		{name: "batch without MTC"},
		{name: "batch with MTC", ramdiskDir: "/tmp/mtc_checkpoints"},
		{name: "headless with MTC", headless: true, ramdiskDir: "/tmp/mtc_checkpoints"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := renderPathwaysTestManifest(t, tc.headless, "sidecar:latest", tc.ramdiskDir, mounts)
			if err := ValidateJobSetManifest(manifest); err != nil {
				t.Fatalf("ValidateJobSetManifest failed: %v\n%s", err, manifest)
			}

			specs := pathwaysPodSpecs(t, manifest)
			worker := specs["worker"]
			if worker == nil {
				t.Fatalf("worker replicatedJob not found in manifest")
			}

			wantWorker := expectedWorkerMounts(tc.ramdiskDir, true)
			wantWorker["vol-0"] = "/data"
			wantWorker["vol-1"] = "/host"
			wantWorker["vol-2"] = "/mnt/pvc"

			for _, cName := range []string{"colocated-python-sidecar", "pathways-worker"} {
				c := findContainer(worker, cName)
				if c == nil {
					t.Fatalf("worker container %q not found", cName)
				}
				if diff := cmp.Diff(wantWorker, volumeMountPaths(c)); diff != "" {
					t.Errorf("%s volume mounts mismatch (-want +got):\n%s", cName, diff)
				}
			}

			if !tc.headless {
				head := specs["pathways-head"]
				wc := findContainer(head, "workload-container")
				if wc == nil {
					t.Fatalf("workload-container not found in pathways-head")
				}
				gotHeadMounts := volumeMountPaths(wc)
				for vol, wantPath := range map[string]string{"vol-0": "/data", "vol-1": "/host", "vol-2": "/mnt/pvc"} {
					if gotHeadMounts[vol] != wantPath {
						t.Errorf("workload-container mount %q = %q, want %q", vol, gotHeadMounts[vol], wantPath)
					}
				}
			}
		})
	}
}

func TestGenerateManifest_CustomGCSFusePVCAnnotation(t *testing.T) {
	setupMockMachineConfig(t)
	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"guestCpus": 2}`},
			{ExitCode: 0, Stdout: `{"guestCpus": 2}`},
		},
		fmt.Sprintf("kubectl get pvc ub-verify-dataset-pvc -n default --ignore-not-found --request-timeout=10s -o %s", pvcGCSFuseJSONPath): {
			{ExitCode: 0, Stdout: "ub-verify-dataset-pv\t\t\t"},
			{ExitCode: 0, Stdout: "ub-verify-dataset-pv\t\t\t"},
		},
		fmt.Sprintf("kubectl get pv ub-verify-dataset-pv --ignore-not-found --request-timeout=10s -o %s", pvCSIDriverTemplate): {
			{ExitCode: 0, Stdout: "gcsfuse.csi.storage.gke.io"},
			{ExitCode: 0, Stdout: "gcsfuse.csi.storage.gke.io"},
		},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
	}

	job := orchestrator.JobDefinition{
		WorkloadName:    "gcsfuse-pvc-job",
		CommandToRun:    "python3 train.py",
		NumSlices:       1,
		ClusterLocation: "us-central1-a",
		ComputeType:     "n2-standard-2",
		RawMounts:       []string{"ub-verify-dataset-pvc;/data;ro"},
		Pathways: orchestrator.PathwaysJobDefinition{
			ProxyServerImage:            "proxy:latest",
			ServerImage:                 "server:latest",
			WorkerImage:                 "worker:latest",
			ColocatedPythonSidecarImage: "sidecar:latest",
			GCSLocation:                 "gs://my-bucket",
			HeadNodePool:                "pathways-np",
		},
	}

	profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}

	opts, err := orc.PrepareManifestOptions(job, "test-image:latest", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	stdManifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}
	if !strings.Contains(stdManifest, `gke-gcsfuse/volumes: "true"`) {
		t.Errorf("standard JobSet manifest missing gke-gcsfuse/volumes annotation:\n%s", stdManifest)
	}

	pwJob := job
	pwJob.IsPathwaysJob = true
	pwManifest, err := orc.GeneratePathwaysManifest(pwJob, "test-image:latest", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}
	if got := strings.Count(pwManifest, `gke-gcsfuse/volumes: "true"`); got != 2 {
		t.Errorf("Pathways JobSet manifest has %d occurrences of gke-gcsfuse/volumes annotation, want 2 (head + worker):\n%s", got, pwManifest)
	}
}

func TestGeneratePathwaysManifest_MTCRejectsStaleCustomTemplate(t *testing.T) {
	tests := []struct {
		name        string
		tmplContent string
		sidecar     string
		wantErr     string
	}{
		{
			name:        "missing worker and MTC volume entirely",
			tmplContent: "apiVersion: jobset.x-k8s.io/v1alpha2\nkind: JobSet\nmetadata:\n  name: stale\n",
			wantErr:     "no worker replicatedJob",
		},
		{
			name: "driver string only in YAML comment",
			tmplContent: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: comment-only
# multitier-checkpoint.csi.storage.gke.io
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-worker
`,
			wantErr: "no multitier-checkpoint.csi.storage.gke.io volume on worker pods",
		},
		{
			name: "MTC volume declared on pathways-head instead of worker",
			tmplContent: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: head-mtc
spec:
  replicatedJobs:
  - name: pathways-head
    template:
      spec:
        template:
          spec:
            volumes:
            - name: cache
              csi:
                driver: multitier-checkpoint.csi.storage.gke.io
            containers:
            - name: workload-container
              volumeMounts:
              - name: cache
                mountPath: /tmp/mtc_checkpoints
  - name: worker
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-worker
`,
			wantErr: "pathways-head declares a multitier-checkpoint.csi.storage.gke.io volume",
		},
		{
			name: "worker declares MTC volume but pathways-worker does not mount it at ramdisk dir",
			tmplContent: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: worker-unmounted
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        template:
          spec:
            volumes:
            - name: cache
              csi:
                driver: multitier-checkpoint.csi.storage.gke.io
            containers:
            - name: pathways-worker
              volumeMounts:
              - name: cache
                mountPath: /wrong/path
`,
			wantErr: `container "pathways-worker" does not mount multitier-checkpoint.csi.storage.gke.io volume "cache" at "/tmp/mtc_checkpoints"`,
		},
		{
			name:    "colocated-python-sidecar does not mount MTC volume when sidecar is enabled",
			sidecar: "sidecar:latest",
			tmplContent: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: sidecar-unmounted
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        template:
          spec:
            volumes:
            - name: cache
              csi:
                driver: multitier-checkpoint.csi.storage.gke.io
            containers:
            - name: colocated-python-sidecar
            - name: pathways-worker
              volumeMounts:
              - name: cache
                mountPath: /tmp/mtc_checkpoints
`,
			wantErr: `container "colocated-python-sidecar" does not mount multitier-checkpoint.csi.storage.gke.io volume "cache" at "/tmp/mtc_checkpoints"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pathways_jobset.tmpl"), []byte(tc.tmplContent), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := generatePathwaysTestManifest(t, dir, false, tc.sidecar, "/tmp/mtc_checkpoints", nil)
			if err == nil {
				t.Fatalf("GeneratePathwaysManifest() succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "re-extract pathways_jobset.tmpl") {
				t.Fatalf("GeneratePathwaysManifest() error = %v, want substring %q and hint to re-extract pathways_jobset.tmpl", err, tc.wantErr)
			}
		})
	}
}

func TestValidateJobSetManifest_PodSpecVolumes(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		wantErr  string
	}{
		{
			name: "valid JobSet volumes and mounts",
			manifest: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: slice-job
    template:
      spec:
        template:
          spec:
            volumes:
            - name: dshm
              emptyDir:
                medium: Memory
            containers:
            - name: workload-container
              volumeMounts:
              - name: dshm
                mountPath: /dev/shm
`,
		},
		{
			name: "JobSet mounts undefined volume",
			manifest: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: slice-job
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              volumeMounts:
              - name: missing-vol
                mountPath: /mnt/data
`,
			wantErr: `replicatedJob "slice-job" container "workload-container" mounts undefined volume "missing-vol" at "/mnt/data"`,
		},
		{
			name: "JobSet declares duplicate volume name",
			manifest: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: slice-job
    template:
      spec:
        template:
          spec:
            volumes:
            - name: cache
              emptyDir: {}
            - name: cache
              emptyDir: {}
`,
			wantErr: `replicatedJob "slice-job" declares duplicate volume "cache"`,
		},
		{
			name: "JobSet container has duplicate volumeMount path",
			manifest: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: slice-job
    template:
      spec:
        template:
          spec:
            volumes:
            - name: vol-a
              emptyDir: {}
            - name: vol-b
              emptyDir: {}
            containers:
            - name: workload-container
              volumeMounts:
              - name: vol-a
                mountPath: /mnt/shared
              - name: vol-b
                mountPath: /mnt/shared
`,
			wantErr: `replicatedJob "slice-job" container "workload-container" has duplicate volumeMount path "/mnt/shared"`,
		},
		{
			name: "Job mounts undefined volume",
			manifest: `apiVersion: batch/v1
kind: Job
spec:
  template:
    spec:
      containers:
      - name: workload-container
        volumeMounts:
        - name: missing
          mountPath: /data
`,
			wantErr: `invalid Job manifest: container "workload-container" mounts undefined volume "missing" at "/data"`,
		},
		{
			name: "JobSet initContainer mounts undefined volume",
			manifest: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: slice-job
    template:
      spec:
        template:
          spec:
            initContainers:
            - name: init-setup
              volumeMounts:
              - name: missing-init-vol
                mountPath: /init
`,
			wantErr: `replicatedJob "slice-job" container "init-setup" mounts undefined volume "missing-init-vol" at "/init"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateJobSetManifest(tc.manifest)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateJobSetManifest() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateJobSetManifest() error = %v, want substring %q", err, tc.wantErr)
			}
		})
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
	if strings.Contains(manifest, "JAX_PLATFORMS") || strings.Contains(manifest, "kill -TERM") {
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

func TestValidatePathwaysMTCManifest_CleansMountPath(t *testing.T) {
	manifest := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: worker
    template:
      spec:
        template:
          spec:
            volumes:
            - name: cache
              csi:
                driver: multitier-checkpoint.csi.storage.gke.io
            containers:
            - name: pathways-worker
              volumeMounts:
              - name: cache
                mountPath: /tmp/mtc_checkpoints/
`
	opts := ManifestOptions{GKEMTCEnabled: true, GKEMTCRamdiskDirectory: "/tmp/mtc_checkpoints"}
	if err := validatePathwaysMTCManifest(manifest, opts); err != nil {
		t.Fatalf("validatePathwaysMTCManifest() error = %v, want nil for a mountPath that differs only by a trailing slash", err)
	}
}

func TestValidateJobSetManifest_DuplicateMountPathNormalized(t *testing.T) {
	manifest := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: dup-mount-norm
spec:
  replicatedJobs:
  - name: worker
    replicas: 1
    template:
      spec:
        parallelism: 1
        completions: 1
        template:
          spec:
            volumes:
            - name: vol-a
              emptyDir: {}
            - name: vol-b
              emptyDir: {}
            containers:
            - name: main
              image: busybox
              volumeMounts:
              - name: vol-a
                mountPath: /mnt/shared
              - name: vol-b
                mountPath: /mnt/shared/
`
	err := ValidateJobSetManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), `has duplicate volumeMount path "/mnt/shared/"`) {
		t.Fatalf("ValidateJobSetManifest() error = %v, want duplicate volumeMount path error", err)
	}
}

func TestGeneratePathwaysManifest_DeploySPS(t *testing.T) {
	tests := []struct {
		name       string
		sidecar    string
		ramdiskDir string // empty disables MTC
	}{
		{name: "without sidecar"},
		{name: "with sidecar", sidecar: "sidecar:20260831-jax0.11.1"},
		{name: "MTC without sidecar", ramdiskDir: "/tmp/mtc_checkpoints"},
		{name: "MTC with sidecar", sidecar: "sidecar:20260831-jax0.11.1", ramdiskDir: "/tmp/mtc_checkpoints"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupMockMachineConfig(t)
			mockResponses := map[string][]shell.CommandResult{
				"kubectl get resourceflavors": {{ExitCode: 0, Stdout: ""}},
				"kubectl get nodes":           {{ExitCode: 0, Stdout: ""}},
				"gcloud compute machine-types describe ct5p-hightpu-4t --zone=us-central1-a --format=json": {
					{ExitCode: 0, Stdout: `{"accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v5p-slice"}]}`},
				},
			}
			mockExec := NewMockExecutor(mockResponses)
			orc := newTestGKEOrchestrator(mockExec)
			orc.projectID = "mock-project"
			orc.clusterZones = []string{"us-central1-a"}
			orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
			orc.clusterDesc.NodePools = []gkeJobNodePool{
				{
					Name:            "v5p-pool",
					Config:          gkeNodePoolConfig{MachineType: "ct5p-hightpu-4t"},
					PlacementPolicy: &gkePlacementPolicy{TpuTopology: "2x2x2"},
				},
			}

			job := orchestrator.JobDefinition{
				WorkloadName:           "sps-v5p",
				NumSlices:              4,
				MaxRestarts:            30,
				ClusterLocation:        "us-central1-a",
				ComputeType:            "ct5p-hightpu-4t",
				Topology:               "2x2x2",
				IsPathwaysJob:          true,
				GKEMTCEnabled:          tc.ramdiskDir != "",
				GKEMTCRamdiskDirectory: tc.ramdiskDir,
				Pathways: orchestrator.PathwaysJobDefinition{
					DeploySPS:                   true,
					MaxSliceRestarts:            1000000,
					ServerImage:                 "server:jax-0.11.1",
					WorkerImage:                 "server:jax-0.11.1",
					ColocatedPythonSidecarImage: tc.sidecar,
					GCSLocation:                 "gs://my-bucket/sps",
					HeadNodePool:                "cpu-np",
				},
			}

			img, err := orc.BuildContainerImage(job)
			if err != nil || img != "" {
				t.Fatalf("BuildContainerImage() = (%q, %v), want (\"\", nil)", img, err)
			}

			profile, isDynamicSlicing, isStaticSlicing, err := orc.resolveHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveHardwareRequirements failed: %v", err)
			}
			manifest, err := orc.GeneratePathwaysManifest(job, "", profile, isDynamicSlicing, isStaticSlicing)
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}
			assertSPSManifest(t, manifest, tc.sidecar, tc.ramdiskDir)
			if got := fmt.Sprintf(spsEndpointFormat, job.WorkloadName); got != "sps-v5p-pathways-head-0-0.sps-v5p:29001" {
				t.Errorf("spsEndpointFormat gives %q, want sps-v5p-pathways-head-0-0.sps-v5p:29001", got)
			}
		})
	}
}

func assertSPSManifest(t *testing.T, manifest, sidecarImage, ramdiskDir string) {
	t.Helper()
	specs := pathwaysPodSpecs(t, manifest)
	head, ok := specs["pathways-head"]
	if !ok {
		t.Fatalf("pathways-head replicatedJob not found in manifest:\n%s", manifest)
	}
	worker, ok := specs["worker"]
	if !ok {
		t.Fatalf("worker replicatedJob not found in manifest:\n%s", manifest)
	}

	if len(head.InitContainers) != 0 {
		t.Errorf("pathways-head initContainers = %d, want 0", len(head.InitContainers))
	}
	if len(head.Containers) != 1 || head.Containers[0].Name != "pathways-rm" {
		t.Fatalf("pathways-head containers = %+v, want only [pathways-rm]", head.Containers)
	}
	if head.Containers[0].RestartPolicy != nil {
		t.Errorf("pathways-rm restartPolicy = %v, want unset on a regular container", *head.Containers[0].RestartPolicy)
	}
	if strings.Contains(manifest, "pathways-proxy") || strings.Contains(manifest, "workload-container") || strings.Contains(manifest, "successPolicy:") {
		t.Errorf("SPS manifest should not contain pathways-proxy, workload-container, or successPolicy:\n%s", manifest)
	}
	assertNoDevShm(t, "pathways-head", head)
	assertNoPodVolumes(t, "pathways-head", head, "cache", "sidecar-shared-memory")
	assertUniqueVolumeNames(t, "pathways-head", head)
	assertUniqueVolumeNames(t, "worker", worker)
	assertWorkerMTCVolumes(t, worker, ramdiskDir, sidecarImage != "")

	for _, want := range []string{
		"maxRestarts: 30",
		"restartStrategy: BlockingRecreate",
		"--instance_count=4",
		"--instance_type=tpuv5:2x2x2",
		"backoffLimit: 2000000",
		"backoffLimitPerIndex: 1000000",
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("SPS manifest missing %q:\n%s", want, manifest)
		}
	}
}

func TestValidatePathwaysSPSManifest(t *testing.T) {
	withHead := func(containers string) string {
		return "apiVersion: jobset.x-k8s.io/v1alpha2\nkind: JobSet\nspec:\n  replicatedJobs:\n  - name: pathways-head\n    template:\n      spec:\n        template:\n          spec:\n" + containers
	}
	tests := []struct {
		name      string
		deploySPS bool
		manifest  string
		wantErr   string
	}{
		{name: "resource manager only", deploySPS: true, manifest: withHead("            containers:\n            - name: pathways-rm\n")},
		{name: "stale template keeps the proxy", deploySPS: true, manifest: withHead("            initContainers:\n            - name: pathways-proxy\n            - name: pathways-rm\n"), wantErr: `runs container "pathways-proxy"`},
		{name: "stale template keeps the workload", deploySPS: true, manifest: withHead("            containers:\n            - name: pathways-rm\n            - name: workload-container-1\n"), wantErr: `runs container "workload-container-1"`},
		{name: "no pathways-head", deploySPS: true, manifest: "apiVersion: jobset.x-k8s.io/v1alpha2\nkind: JobSet\nmetadata:\n  name: stale\n", wantErr: "no pathways-head replicatedJob"},
		{name: "not an SPS job", manifest: withHead("            initContainers:\n            - name: pathways-proxy\n")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := ManifestOptions{Pathways: orchestrator.PathwaysJobDefinition{DeploySPS: tc.deploySPS}}
			err := validatePathwaysSPSManifest(tc.manifest, opts)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validatePathwaysSPSManifest() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "re-extract pathways_jobset.tmpl") {
				t.Fatalf("validatePathwaysSPSManifest() error = %v, want %q with a re-extract hint", err, tc.wantErr)
			}
		})
	}
}

func TestWithSidecarNameArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "nil args", args: nil, want: []string{"--sidecar_name=external"}},
		{name: "other args", args: []string{"--foo=1"}, want: []string{"--foo=1", "--sidecar_name=external"}},
		{name: "user value kept", args: []string{"--sidecar_name=custom"}, want: []string{"--sidecar_name=custom"}},
		{name: "user value without equals kept", args: []string{"--sidecar_name", "custom"}, want: []string{"--sidecar_name", "custom"}},
		{name: "user single-dash value kept", args: []string{"-sidecar_name=custom"}, want: []string{"-sidecar_name=custom"}},
		{name: "user empty value kept", args: []string{"--sidecar_name="}, want: []string{"--sidecar_name="}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := withSidecarNameArg(tc.args); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("withSidecarNameArg(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// TestGeneratePathwaysManifest_SidecarNameArg verifies that pathways-proxy is
// told to use the external sidecar only when the colocated Python sidecar is
// enabled.
func TestGeneratePathwaysManifest_SidecarNameArg(t *testing.T) {
	tests := []struct {
		name     string
		headless bool
		sidecar  string
		wantArg  bool
	}{
		{name: "no sidecar"},
		{name: "sidecar", sidecar: "sidecar:latest", wantArg: true},
		{name: "headless no sidecar", headless: true},
		{name: "headless sidecar", headless: true, sidecar: "sidecar:latest", wantArg: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := renderPathwaysTestManifest(t, tc.headless, tc.sidecar, "", nil)
			head := pathwaysPodSpecs(t, manifest)["pathways-head"]
			if head == nil {
				t.Fatalf("pathways-head replicatedJob not found in manifest:\n%s", manifest)
			}
			proxy := findContainer(head, "pathways-proxy")
			if proxy == nil {
				t.Fatalf("pathways-proxy not found in manifest:\n%s", manifest)
			}
			count := 0
			for _, arg := range proxy.Args {
				if arg == "--sidecar_name=external" {
					count++
				}
			}
			want := 0
			if tc.wantArg {
				want = 1
			}
			if count != want {
				t.Errorf("pathways-proxy has --sidecar_name=external %d times, want %d (args: %v)", count, want, proxy.Args)
			}
		})
	}
}

// TestGeneratePathwaysManifest_SidecarNameArgUserValue verifies that a
// --sidecar_name passed through --pathways-proxy-args, with or without "=",
// is kept and gcluster does not add a second one.
func TestGeneratePathwaysManifest_SidecarNameArgUserValue(t *testing.T) {
	for _, proxyArgs := range []string{"--sidecar_name=custom", "--sidecar_name custom"} {
		t.Run(proxyArgs, func(t *testing.T) {
			setupMockMachineConfig(t)
			job := orchestrator.JobDefinition{
				WorkloadName:    "pathways-test",
				CommandToRun:    "python3 train.py",
				NumSlices:       2,
				ClusterLocation: "us-central1",
				ComputeType:     "n2-standard-2",
				IsPathwaysJob:   true,
				Pathways: orchestrator.PathwaysJobDefinition{
					ProxyServerImage:            "proxy:latest",
					ServerImage:                 "server:latest",
					WorkerImage:                 "worker:latest",
					ColocatedPythonSidecarImage: "sidecar:latest",
					GCSLocation:                 "gs://my-bucket",
					HeadNodePool:                "pathways-np",
					ProxyArgs:                   proxyArgs,
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

			head := pathwaysPodSpecs(t, manifest)["pathways-head"]
			if head == nil {
				t.Fatalf("pathways-head replicatedJob not found in manifest:\n%s", manifest)
			}
			proxy := findContainer(head, "pathways-proxy")
			if proxy == nil {
				t.Fatalf("pathways-proxy not found in manifest:\n%s", manifest)
			}
			want := strings.Fields(proxyArgs)
			i := slices.Index(proxy.Args, want[0])
			if i < 0 || !slices.Equal(proxy.Args[i:min(i+len(want), len(proxy.Args))], want) || slices.Contains(proxy.Args, "--sidecar_name=external") {
				t.Errorf("pathways-proxy args = %v, want %v kept and no --sidecar_name=external", proxy.Args, want)
			}
		})
	}
}

func unmarshalStrictWorkloadDoc(t *testing.T, kind, doc string) (*corev1.Service, *batchv1.Job) {
	t.Helper()
	switch kind {
	case "PersistentVolume":
		var pv corev1.PersistentVolume
		if err := k8syaml.UnmarshalStrict([]byte(doc), &pv); err != nil {
			t.Fatalf("UnmarshalStrict PersistentVolume failed: %v\n%s", err, doc)
		}
	case "PersistentVolumeClaim":
		var pvc corev1.PersistentVolumeClaim
		if err := k8syaml.UnmarshalStrict([]byte(doc), &pvc); err != nil {
			t.Fatalf("UnmarshalStrict PersistentVolumeClaim failed: %v\n%s", err, doc)
		}
	case "Service":
		var s corev1.Service
		if err := k8syaml.UnmarshalStrict([]byte(doc), &s); err != nil {
			t.Fatalf("UnmarshalStrict Service failed: %v\n%s", err, doc)
		}
		return &s, nil
	case "Job":
		var j batchv1.Job
		if err := k8syaml.UnmarshalStrict([]byte(doc), &j); err != nil {
			t.Fatalf("UnmarshalStrict Job failed: %v\n%s", err, doc)
		}
		return nil, &j
	default:
		t.Fatalf("unexpected Kubernetes document kind %q in:\n%s", kind, doc)
	}
	return nil, nil
}

func parseServiceAndJobDocs(t *testing.T, manifest string) (*corev1.Service, *batchv1.Job, []string) {
	t.Helper()
	docs, err := splitYAMLDocuments(manifest)
	if err != nil {
		t.Fatalf("splitYAMLDocuments failed: %v\n%s", err, manifest)
	}
	var svc *corev1.Service
	var job *batchv1.Job
	var kinds []string
	for _, doc := range docs {
		var meta metav1.TypeMeta
		if err := k8syaml.Unmarshal([]byte(doc), &meta); err != nil {
			t.Fatalf("failed to unmarshal doc kind: %v\n%s", err, doc)
		}
		kinds = append(kinds, meta.Kind)
		s, j := unmarshalStrictWorkloadDoc(t, meta.Kind, doc)
		if s != nil {
			svc = s
		}
		if j != nil {
			job = j
		}
	}
	if svc == nil || job == nil {
		t.Fatalf("expected both Service and Job in manifest (got kinds %v):\n%s", kinds, manifest)
	}
	return svc, job, kinds
}

// assertHeadlessWorkloadService checks the headless Service and the two relations that make
// "<workload>-<index>.<workload>" resolve: the pod subdomain must equal the Service name, and the Service
// selector must be satisfied by the pod labels plus the batch.kubernetes.io/job-name label the Job controller adds.
func assertHeadlessWorkloadService(t *testing.T, svc *corev1.Service, k8sJob *batchv1.Job, workloadName string) {
	t.Helper()
	if svc.Name != workloadName || svc.Spec.ClusterIP != "None" || !svc.Spec.PublishNotReadyAddresses {
		t.Errorf("unexpected headless Service spec: %+v", svc)
	}
	wantLabels := map[string]string{"gcluster.google.com/workload": workloadName}
	if diff := cmp.Diff(wantLabels, svc.Labels); diff != "" {
		t.Errorf("Service labels should only carry workload label (-want +got):\n%s", diff)
	}
	wantSelector := map[string]string{
		"gcluster.google.com/workload": workloadName,
		"batch.kubernetes.io/job-name": workloadName,
	}
	if diff := cmp.Diff(wantSelector, svc.Spec.Selector); diff != "" {
		t.Errorf("Service selector mismatch (-want +got):\n%s", diff)
	}
	if got := k8sJob.Spec.Template.Spec.Subdomain; got != svc.Name {
		t.Errorf("pod subdomain = %q, want Service name %q", got, svc.Name)
	}
	effectivePodLabels := map[string]string{"batch.kubernetes.io/job-name": k8sJob.Name}
	for k, v := range k8sJob.Spec.Template.Labels {
		effectivePodLabels[k] = v
	}
	for k, v := range svc.Spec.Selector {
		if effectivePodLabels[k] != v {
			t.Errorf("Service selector %s=%s not satisfied by pod labels %v", k, v, effectivePodLabels)
		}
	}
}

// assertIndexedJobShape pins the standalone Job contract: Indexed, one completion per node, and backoffLimit 0
// (fail fast like the child Job in jobset.tmpl; --restarts is not mapped onto a standalone Job).
func assertIndexedJobShape(t *testing.T, k8sJob *batchv1.Job, wantParallelism int32) {
	t.Helper()
	if k8sJob.Spec.CompletionMode == nil || *k8sJob.Spec.CompletionMode != batchv1.IndexedCompletion {
		t.Errorf("CompletionMode = %v, want Indexed", k8sJob.Spec.CompletionMode)
	}
	if k8sJob.Spec.Parallelism == nil || *k8sJob.Spec.Parallelism != wantParallelism {
		t.Errorf("Parallelism = %v, want %d", k8sJob.Spec.Parallelism, wantParallelism)
	}
	if k8sJob.Spec.Completions == nil || *k8sJob.Spec.Completions != wantParallelism {
		t.Errorf("Completions = %v, want %d", k8sJob.Spec.Completions, wantParallelism)
	}
	if k8sJob.Spec.BackoffLimit == nil || *k8sJob.Spec.BackoffLimit != 0 {
		t.Errorf("BackoffLimit = %v, want 0", k8sJob.Spec.BackoffLimit)
	}
	if k8sJob.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("pod restartPolicy = %q, want Never", k8sJob.Spec.Template.Spec.RestartPolicy)
	}
}

func assertContainerHasMount(t *testing.T, c corev1.Container, mountPath string, wantReadOnly bool) {
	t.Helper()
	for _, vm := range c.VolumeMounts {
		if vm.MountPath == mountPath {
			if vm.ReadOnly != wantReadOnly {
				t.Errorf("mount %q ReadOnly = %v, want %v", mountPath, vm.ReadOnly, wantReadOnly)
			}
			return
		}
	}
	t.Errorf("container %q missing volumeMount %q (mounts: %+v)", c.Name, mountPath, c.VolumeMounts)
}

func assertGPUPodSpecFeatures(t *testing.T, podSpec corev1.PodSpec) {
	t.Helper()
	if podSpec.Subdomain != "gpu-idx-job" {
		t.Errorf("PodSpec.Subdomain = %q, want %q", podSpec.Subdomain, "gpu-idx-job")
	}
	if !podSpec.HostNetwork || podSpec.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
		t.Errorf("HostNetwork=%v DNSPolicy=%q, want true and ClusterFirstWithHostNet", podSpec.HostNetwork, podSpec.DNSPolicy)
	}
	if podSpec.PriorityClassName != "high" || podSpec.ServiceAccountName != "workload-ksa" {
		t.Errorf("PriorityClassName=%q ServiceAccountName=%q, want high / workload-ksa", podSpec.PriorityClassName, podSpec.ServiceAccountName)
	}
	if len(podSpec.ImagePullSecrets) != 2 {
		t.Errorf("ImagePullSecrets = %+v, want 2 secrets", podSpec.ImagePullSecrets)
	}
	if len(podSpec.SchedulingGates) != 1 || podSpec.SchedulingGates[0].Name != "gke.io/topology-aware-auto-gpu-idx-job" {
		t.Errorf("SchedulingGates = %+v, want gke.io/topology-aware-auto-gpu-idx-job", podSpec.SchedulingGates)
	}
	if podSpec.Affinity == nil || len(podSpec.Tolerations) == 0 {
		t.Errorf("expected Affinity and Tolerations to be populated, got Affinity=%+v Tolerations=%+v", podSpec.Affinity, podSpec.Tolerations)
	}
}

func assertGPUMountsAndTimers(t *testing.T, k8sJob *batchv1.Job) {
	t.Helper()
	if k8sJob.Spec.TTLSecondsAfterFinished == nil || *k8sJob.Spec.TTLSecondsAfterFinished != 3600 {
		t.Errorf("TTLSecondsAfterFinished = %v, want 3600", k8sJob.Spec.TTLSecondsAfterFinished)
	}
	podSpec := k8sJob.Spec.Template.Spec
	if podSpec.TerminationGracePeriodSeconds == nil || *podSpec.TerminationGracePeriodSeconds != 60 {
		t.Errorf("TerminationGracePeriodSeconds = %v, want 60", podSpec.TerminationGracePeriodSeconds)
	}
	if podSpec.NodeSelector["team"] != "ml" {
		t.Errorf("NodeSelector[team] = %q, want ml (got %+v)", podSpec.NodeSelector["team"], podSpec.NodeSelector)
	}
	assertContainerHasMount(t, podSpec.Containers[0], "/mnt/shared", true)
	assertContainerHasMount(t, podSpec.Containers[0], "/tmp/mtc", false)
}

// newGPUDerivationTestOrc returns an orchestrator wired for a NAP-provisioned g2-standard-48 (4x L4) job with a
// pre-existing "shared-data" PVC.
func newGPUDerivationTestOrc(t *testing.T) *GKEOrchestrator {
	t.Helper()
	setupMockMachineConfig(t)
	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe g2-standard-48 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"guestCpus": 48, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "nvidia-l4"}]}`},
		},
		"kubectl get pvc shared-data -o jsonpath={.metadata.name}": {
			{ExitCode: 0, Stdout: "shared-data"},
		},
	}
	mockExec := NewMockExecutor(mockResponses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.napEnabled = true
	orc.napLimits = map[string]int64{"nvidia-l4": 16, "cpu": 1000, "memory": 1000}
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "gpu-pool", Config: gkeNodePoolConfig{MachineType: "g2-standard-48"}},
	}
	return orc
}

// gpuDerivationJobDef returns a job exercising every optional manifest input (exit codes, env, priority class,
// SA, pull secrets, node constraints, MTC, GCS mounts) for the two-node GPU pipeline tests.
func gpuDerivationJobDef(workloadName string) orchestrator.JobDefinition {
	return orchestrator.JobDefinition{
		WorkloadName:                  workloadName,
		CommandToRun:                  "torchrun train.py",
		ComputeType:                   "g2-standard-48",
		NumSlices:                     1,
		NodesPerSlice:                 2,
		MaxRestarts:                   3,
		TtlSecondsAfterFinished:       3600,
		TerminationGracePeriodSeconds: 60,
		ClusterLocation:               "us-central1-a",
		ProjectID:                     "mock-project",
		KueueQueueName:                "user-queue",
		GKEScheduler:                  "gke.io/topology-aware-auto",
		PriorityClassName:             "high",
		ServiceAccountName:            "workload-ksa",
		ImagePullSecrets:              "regcred-a,regcred-b",
		RestartOnExitCodes:            []int{137, 143},
		NodeConstraint:                map[string]string{"team": "ml"},
		GKENAPProvisioning:            "spot",
		MLDiagnosticsEnabled:          true,
		GKEMTCEnabled:                 true,
		GKEMTCRamdiskDirectory:        "/tmp/mtc",
		Verbose:                       true,
		RawMounts:                     []string{"shared-data;/mnt/shared;ro"},
		Env:                           map[string]string{"USER_VAR": "hello"},
	}
}

func TestGenerateJobManifest_GPUDerivationPipeline(t *testing.T) {
	orc := newGPUDerivationTestOrc(t)
	jobDef := gpuDerivationJobDef("gpu-idx-job")

	profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&jobDef)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	opts, err := orc.PrepareManifestOptions(jobDef, "registry.local/trainer:v1", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	manifest, err := orc.GenerateJobManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateJobManifest failed: %v", err)
	}

	svc, k8sJob, kinds := parseServiceAndJobDocs(t, manifest)
	if diff := cmp.Diff([]string{"Service", "Job"}, kinds); diff != "" {
		t.Errorf("document order mismatch (-want +got):\n%s", diff)
	}
	assertHeadlessWorkloadService(t, svc, k8sJob, "gpu-idx-job")
	assertIndexedJobShape(t, k8sJob, 2)

	wantJobLabels := map[string]string{
		"gcluster.google.com/workload": "gpu-idx-job",
		"kueue.x-k8s.io/queue-name":    "user-queue",
		"managed-mldiagnostics-gke":    "true",
	}
	if diff := cmp.Diff(wantJobLabels, k8sJob.Labels); diff != "" {
		t.Errorf("Job labels mismatch (-want +got):\n%s", diff)
	}
	if k8sJob.Spec.Suspend == nil || !*k8sJob.Spec.Suspend {
		t.Errorf("expected Job.Spec.Suspend=true when KueueQueueName is set, got %v", k8sJob.Spec.Suspend)
	}
	if k8sJob.Spec.PodFailurePolicy == nil || len(k8sJob.Spec.PodFailurePolicy.Rules) != 1 || k8sJob.Spec.PodFailurePolicy.Rules[0].OnExitCodes == nil || k8sJob.Spec.PodFailurePolicy.Rules[0].OnExitCodes.ContainerName == nil || *k8sJob.Spec.PodFailurePolicy.Rules[0].OnExitCodes.ContainerName != "workload-container" {
		t.Errorf("expected PodFailurePolicy scoped to workload-container on Job, got %+v", k8sJob.Spec.PodFailurePolicy)
	}
	assertGPUPodSpecFeatures(t, k8sJob.Spec.Template.Spec)
	assertGPUMountsAndTimers(t, k8sJob)
	assertContainerEnvMap(t, k8sJob.Spec.Template.Spec.Containers[0], map[string]string{
		"MASTER_ADDR":        "gpu-idx-job-0.gpu-idx-job",
		"NCCL_DEBUG":         "INFO",
		"USER_VAR":           "hello",
		"WORKERS_0_HOSTNAME": "gpu-idx-job-0.gpu-idx-job",
	})
}

func assertTPUv7xDerivedJob(t *testing.T, jobDef orchestrator.JobDefinition, svc *corev1.Service, k8sJob *batchv1.Job) {
	t.Helper()
	if jobDef.NodesPerSlice != 2 || jobDef.PlacementPolicy != "tpu7x-16-2x2x2-placement-policy" {
		t.Errorf("derived NodesPerSlice=%d PlacementPolicy=%q, want 2 and tpu7x-16-2x2x2-placement-policy", jobDef.NodesPerSlice, jobDef.PlacementPolicy)
	}
	assertHeadlessWorkloadService(t, svc, k8sJob, "tpu7x-idx-job")
	assertIndexedJobShape(t, k8sJob, 2)
	podSpec := k8sJob.Spec.Template.Spec
	if !podSpec.HostNetwork || podSpec.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
		t.Errorf("TPU pod hostNetwork=%v dnsPolicy=%q, want true and ClusterFirstWithHostNet", podSpec.HostNetwork, podSpec.DNSPolicy)
	}
	if got := k8sJob.Spec.Template.Spec.NodeSelector["cloud.google.com/placement-policy-name"]; got != "tpu7x-16-2x2x2-placement-policy" {
		t.Errorf("NodeSelector placement-policy-name = %q, want tpu7x-16-2x2x2-placement-policy", got)
	}
	if got := k8sJob.Spec.Template.Annotations["cloud.google.com/gke-tpu-slice-topology"]; got != "2x2x2" {
		t.Errorf("pod topology annotation = %q, want 2x2x2", got)
	}
	if got := k8sJob.Spec.Template.Annotations["gke-gcsfuse/volumes"]; got != "true" {
		t.Errorf("pod gcsfuse annotation = %q, want true", got)
	}
}

func TestGenerateJobManifest_TPUv7xDerivationPipeline(t *testing.T) {
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
	orc.namespace = "ml-ns"
	orc.clusterZones = []string{"us-central1-a"}
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "v7x-pool", Config: gkeNodePoolConfig{MachineType: "tpu7x-standard-4t"}},
	}

	jobDef := orchestrator.JobDefinition{
		WorkloadName:          "tpu7x-idx-job",
		CommandToRun:          "python3 train.py",
		ComputeType:           "tpu7x-standard-4t",
		Topology:              "2x2x2",
		NumSlices:             1,
		ClusterLocation:       "us-central1-a",
		ProjectID:             "mock-project",
		UseParallelContainers: true,
		DryRunManifest:        "/tmp/dry.yaml",
		RawMounts:             []string{"gs://my-ckpt-bucket;/ckpt;rw;profile=training"},
	}
	profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&jobDef)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	opts, err := orc.PrepareManifestOptions(jobDef, "registry.local/jax:v7x", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	manifest, err := orc.GenerateJobManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateJobManifest failed: %v", err)
	}

	svc, k8sJob, kinds := parseServiceAndJobDocs(t, manifest)
	if diff := cmp.Diff([]string{"PersistentVolume", "PersistentVolumeClaim", "Service", "Job"}, kinds); diff != "" {
		t.Errorf("document order mismatch (-want +got):\n%s", diff)
	}
	assertTPUv7xDerivedJob(t, jobDef, svc, k8sJob)
	if len(k8sJob.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("expected 2 parallel containers, got %d", len(k8sJob.Spec.Template.Spec.Containers))
	}
	for _, c := range k8sJob.Spec.Template.Spec.Containers {
		assertContainerHasMount(t, c, "/ckpt", false)
		assertContainerEnvMap(t, c, map[string]string{
			"MASTER_ADDR":        "tpu7x-idx-job-0.tpu7x-idx-job",
			"WORKERS_0_HOSTNAME": "tpu7x-idx-job-0.tpu7x-idx-job",
		})
	}
}

func TestGenerateJobManifest_CoordinatorEnvAndOptions(t *testing.T) {
	setupMockMachineConfig(t)
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}

	t.Run("CPU job with empty KueueQueueName, custom SchedulerName, and caller env immutability", func(t *testing.T) {
		userEnv := map[string]string{"MASTER_ADDR": "custom-master:29500"}
		opts := ManifestOptions{
			WorkloadName:    "cpu-job",
			FullImageName:   "alpine:3.20",
			CommandToRun:    "echo ok",
			ComputeType:     "n2-standard-4",
			MachineType:     "n2-standard-4",
			ClusterLocation: "us-central1-a",
			KueueQueueName:  "",
			SchedulerName:   "custom-scheduler",
			NumSlices:       1,
			NodesPerSlice:   1,
			Env:             userEnv,
		}
		manifest, err := orc.GenerateJobManifest(opts, profile)
		if err != nil {
			t.Fatalf("GenerateJobManifest failed: %v", err)
		}
		if len(userEnv) != 1 {
			t.Errorf("caller userEnv map was mutated: %+v", userEnv)
		}
		_, k8sJob, _ := parseServiceAndJobDocs(t, manifest)
		if _, hasQueue := k8sJob.Labels["kueue.x-k8s.io/queue-name"]; hasQueue {
			t.Errorf("expected empty KueueQueueName to omit kueue.x-k8s.io/queue-name label, got %+v", k8sJob.Labels)
		}
		if k8sJob.Spec.Suspend != nil {
			t.Errorf("expected empty KueueQueueName to omit Job.Spec.Suspend, got %v", *k8sJob.Spec.Suspend)
		}
		assertIndexedJobShape(t, k8sJob, 1)
		if k8sJob.Spec.Template.Spec.HostNetwork || k8sJob.Spec.Template.Spec.SchedulerName != "custom-scheduler" {
			t.Errorf("HostNetwork=%v SchedulerName=%q, want false and custom-scheduler", k8sJob.Spec.Template.Spec.HostNetwork, k8sJob.Spec.Template.Spec.SchedulerName)
		}
		assertContainerEnvMap(t, k8sJob.Spec.Template.Spec.Containers[0], map[string]string{
			"MASTER_ADDR":        "custom-master:29500",
			"WORKERS_0_HOSTNAME": "cpu-job-0.cpu-job",
		})
	})

	t.Run("empty WorkloadName omits coordinator env and subdomain", func(t *testing.T) {
		opts := ManifestOptions{
			FullImageName:   "alpine:3.20",
			CommandToRun:    "echo ok",
			ComputeType:     "n2-standard-4",
			MachineType:     "n2-standard-4",
			ClusterLocation: "us-central1-a",
			NumSlices:       1,
			NodesPerSlice:   1,
		}
		manifest, err := orc.GenerateJobManifest(opts, profile)
		if err != nil {
			t.Fatalf("GenerateJobManifest failed: %v", err)
		}
		_, k8sJob, _ := parseServiceAndJobDocs(t, manifest)
		if k8sJob.Spec.Template.Spec.Subdomain != "" || len(k8sJob.Spec.Template.Spec.Containers[0].Env) != 0 {
			t.Errorf("expected empty Subdomain and Env, got Subdomain=%q Env=%+v", k8sJob.Spec.Template.Spec.Subdomain, k8sJob.Spec.Template.Spec.Containers[0].Env)
		}
	})

	t.Run("rejects multi-slice NumSlices > 1", func(t *testing.T) {
		opts := ManifestOptions{
			WorkloadName:    "multi-slice",
			FullImageName:   "alpine:3.20",
			CommandToRun:    "echo ok",
			ComputeType:     "n2-standard-4",
			MachineType:     "n2-standard-4",
			ClusterLocation: "us-central1-a",
			NumSlices:       2,
			NodesPerSlice:   2,
		}
		if _, err := orc.GenerateJobManifest(opts, profile); err == nil || !strings.Contains(err.Error(), "JobSet") {
			t.Errorf("expected multi-slice error mentioning JobSet, got: %v", err)
		}
	})
}

func parseStrictJobSetTemplates(t *testing.T, manifest string) (map[string]string, map[string]batchv1.JobTemplateSpec) {
	t.Helper()
	var raw struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
		Spec     struct {
			ReplicatedJobs []struct {
				Name     string          `json:"name"`
				Template json.RawMessage `json:"template"`
			} `json:"replicatedJobs"`
		} `json:"spec"`
	}
	if err := k8syaml.Unmarshal([]byte(manifest), &raw); err != nil {
		t.Fatalf("failed to unmarshal JobSet envelope: %v\n%s", err, manifest)
	}
	out := make(map[string]batchv1.JobTemplateSpec, len(raw.Spec.ReplicatedJobs))
	for _, rj := range raw.Spec.ReplicatedJobs {
		var jt batchv1.JobTemplateSpec
		if err := k8syaml.UnmarshalStrict(rj.Template, &jt); err != nil {
			t.Fatalf("UnmarshalStrict JobTemplateSpec for replicatedJob %q failed: %v\n%s", rj.Name, err, string(rj.Template))
		}
		out[rj.Name] = jt
	}
	return raw.Metadata.Labels, out
}

func assertPathwaysStrictPodAuth(t *testing.T, name string, spec corev1.PodSpec) {
	t.Helper()
	if spec.PriorityClassName != "high-pw" || spec.ServiceAccountName != "pw-ksa" || len(spec.ImagePullSecrets) != 1 {
		t.Errorf("%s pod_auth_and_priority mismatch: %+v", name, spec)
	}
}

func TestGenerateJobSetAndPathways_SharedBaseBlocksStrict(t *testing.T) {
	setupMockMachineConfig(t)
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe ct5lp-hightpu-4t --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"guestCpus": 112, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v5-lite-podslice"}]}`},
		},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.napEnabled = true
	orc.napLimits = map[string]int64{"tpu-v5-lite-podslice": 16, "cpu": 1000, "memory": 1000}
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
	profile := JobProfile{CapacityCount: 4}

	pwJob := orchestrator.JobDefinition{
		WorkloadName:         "pw-strict",
		CommandToRun:         "python3 train.py",
		ComputeType:          "v5litepod-4",
		MachineType:          "ct5lp-hightpu-4t",
		Topology:             "2x2",
		NumSlices:            1,
		NodesPerSlice:        1,
		ClusterLocation:      "us-central1-a",
		ProjectID:            "mock-project",
		KueueQueueName:       "pw-queue",
		PriorityClassName:    "high-pw",
		ServiceAccountName:   "pw-ksa",
		ImagePullSecrets:     "pw-secret",
		RestartOnExitCodes:   []int{42},
		NodeConstraint:       map[string]string{"env": "prod"},
		GKENAPProvisioning:   "spot",
		MLDiagnosticsEnabled: true,
		IsPathwaysJob:        true,
		Pathways: orchestrator.PathwaysJobDefinition{
			GCSLocation:  "gs://pw-scratch",
			HeadNodePool: "cpu-np",
		},
	}

	pwManifest, err := orc.GeneratePathwaysManifest(pwJob, "img:v1", profile, false, false)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}
	pwLabels, pwTemplates := parseStrictJobSetTemplates(t, pwManifest)
	if pwLabels["kueue.x-k8s.io/queue-name"] != "pw-queue" || pwLabels["managed-mldiagnostics-gke"] != "true" {
		t.Errorf("unexpected Pathways JobSet labels: %+v", pwLabels)
	}
	assertPathwaysStrictPodAuth(t, "pathways-head", pwTemplates["pathways-head"].Spec.Template.Spec)
	workerSpec := pwTemplates["worker"].Spec.Template.Spec
	assertPathwaysStrictPodAuth(t, "worker", workerSpec)
	if workerSpec.NodeSelector["env"] != "prod" || workerSpec.Affinity == nil || len(workerSpec.Tolerations) == 0 {
		t.Errorf("worker pod_placement mismatch: NodeSelector=%+v Affinity=%+v Tolerations=%+v", workerSpec.NodeSelector, workerSpec.Affinity, workerSpec.Tolerations)
	}

	opts, err := orc.PrepareManifestOptions(pwJob, "img:v1", profile, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	jsManifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}
	_, jsTemplates := parseStrictJobSetTemplates(t, jsManifest)
	mainJob := jsTemplates["main-job"]
	assertPathwaysStrictPodAuth(t, "main-job", mainJob.Spec.Template.Spec)
	if mainJob.Spec.PodFailurePolicy == nil || mainJob.Spec.Template.Spec.NodeSelector["env"] != "prod" {
		t.Errorf("main-job strict JobTemplateSpec mismatch: %+v", mainJob)
	}
}

func TestGenerateManifest_YAMLInjectionRejectedInBaseBlocks(t *testing.T) {
	setupMockMachineConfig(t)
	cpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 4}`}
	tpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 112, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v6e-slice"}]}`}
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-4 --zone=us-central1-a --format=json": {cpuResp, cpuResp, cpuResp, cpuResp, cpuResp, cpuResp, cpuResp, cpuResp, cpuResp, cpuResp, cpuResp, cpuResp},
		"gcloud compute machine-types describe tpu-v6e-slice --zone=us-central1-a --format=json": {tpuResp, tpuResp, tpuResp, tpuResp},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.machineTypeClient = &MockMachineTypeClient{Executor: mockExec}
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}

	cases := []struct {
		name   string
		useTPU bool
		mutate func(*ManifestOptions, *orchestrator.JobDefinition)
	}{
		{
			name: "WorkloadName injection in workload_labels",
			mutate: func(o *ManifestOptions, j *orchestrator.JobDefinition) {
				o.WorkloadName = "bad\n  injectedLabel: pwn"
				j.WorkloadName = o.WorkloadName
			},
		},
		{
			name: "KueueQueueName injection in workload_labels",
			mutate: func(o *ManifestOptions, j *orchestrator.JobDefinition) {
				o.KueueQueueName = "q\n  injectedLabel: pwn"
				j.KueueQueueName = o.KueueQueueName
			},
		},
		{
			name: "ServiceAccountName injection in pod_auth_and_priority",
			mutate: func(o *ManifestOptions, j *orchestrator.JobDefinition) {
				o.ServiceAccountName = "ksa\n  hostPID: true"
				j.ServiceAccountName = o.ServiceAccountName
			},
		},
		{
			name: "PriorityClassName injection in pod_auth_and_priority",
			mutate: func(o *ManifestOptions, j *orchestrator.JobDefinition) {
				o.PriorityClassName = "high\n  privileged: true"
				j.PriorityClassName = o.PriorityClassName
			},
		},
		{
			name:   "TelemetryCollectorImage injection in telemetry_sidecar",
			useTPU: true,
			mutate: func(o *ManifestOptions, j *orchestrator.JobDefinition) {
				o.MLDiagnosticsEnabled = true
				o.TelemetryCollectorImage = "registry.local/col:v1\n    securityContext:\n      privileged: true"
				o.TelemetryCollectorPort = 8889
				j.MLDiagnosticsEnabled = true
				j.MLDiagnosticsImage = o.TelemetryCollectorImage
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := profile
			mt := "n2-standard-4"
			ct := "n2-standard-4"
			topo := ""
			if tc.useTPU {
				p = JobProfile{CapacityCount: 4}
				mt = "tpu-v6e-slice"
				ct = "v6e-4"
				topo = "2x2"
			}
			opts := ManifestOptions{
				WorkloadName:    "safe-job",
				FullImageName:   "alpine:3.20",
				CommandToRun:    "echo ok",
				ComputeType:     mt,
				MachineType:     mt,
				ClusterLocation: "us-central1-a",
				NumSlices:       1,
				NodesPerSlice:   1,
			}
			pwJob := orchestrator.JobDefinition{
				WorkloadName:    "safe-job",
				CommandToRun:    "echo ok",
				ComputeType:     ct,
				MachineType:     mt,
				Topology:        topo,
				ClusterLocation: "us-central1-a",
				NumSlices:       1,
				NodesPerSlice:   1,
				Pathways:        orchestrator.PathwaysJobDefinition{GCSLocation: "gs://bkt", HeadNodePool: "head-np"},
			}
			tc.mutate(&opts, &pwJob)

			if _, err := orc.GenerateJobManifest(opts, p); !errors.Is(err, yamltemplate.ErrYAMLInjection) {
				t.Errorf("GenerateJobManifest error = %v, want wrap of ErrYAMLInjection", err)
			}
			if _, err := orc.GenerateGKEManifest(opts, p); !errors.Is(err, yamltemplate.ErrYAMLInjection) {
				t.Errorf("GenerateGKEManifest error = %v, want wrap of ErrYAMLInjection", err)
			}
			if _, err := orc.GeneratePathwaysManifest(pwJob, "alpine:3.20", p, false, false); !errors.Is(err, yamltemplate.ErrYAMLInjection) {
				t.Errorf("GeneratePathwaysManifest error = %v, want wrap of ErrYAMLInjection", err)
			}
		})
	}
}

func assertPodFailurePolicyContainers(t *testing.T, pfp *batchv1.PodFailurePolicy, wantContainers []string) {
	t.Helper()
	if pfp == nil {
		t.Fatalf("expected non-nil PodFailurePolicy, want containers %v", wantContainers)
	}
	var got []string
	for _, r := range pfp.Rules {
		if r.OnExitCodes != nil && r.OnExitCodes.ContainerName != nil {
			got = append(got, *r.OnExitCodes.ContainerName)
		}
	}
	if diff := cmp.Diff(wantContainers, got); diff != "" {
		t.Errorf("PodFailurePolicy containerNames mismatch (-want +got):\n%s", diff)
	}
}

func mustGenerateJobManifest(t *testing.T, orc *GKEOrchestrator, opts ManifestOptions, profile JobProfile) string {
	t.Helper()
	m, err := orc.GenerateJobManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateJobManifest failed: %v", err)
	}
	return m
}

func TestPodFailurePolicy_SidecarContainerScoping(t *testing.T) {
	setupMockMachineConfig(t)
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-4 --zone=us-central1-a --format=json": {
			{ExitCode: 0, Stdout: `{"guestCpus": 4, "memoryMb": 16384}`},
			{ExitCode: 0, Stdout: `{"guestCpus": 4, "memoryMb": 16384}`},
		},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.clusterZones = []string{"us-central1-a"}
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}

	pureJob := orchestrator.JobDefinition{
		WorkloadName:       "no-sidecar-pfp",
		CommandToRun:       "echo hi",
		ComputeType:        "n2-standard-4",
		MachineType:        "n2-standard-4",
		ClusterLocation:    "us-central1-a",
		NumSlices:          1,
		NodesPerSlice:      1,
		RestartOnExitCodes: []int{143, 42},
	}
	pureOpts, err := orc.PrepareManifestOptions(pureJob, "registry.local/app:v1", profile, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	_, pureK8sJob, _ := parseServiceAndJobDocs(t, mustGenerateJobManifest(t, orc, pureOpts, profile))
	// Always scoped, even without sidecars: a pod-wide rule would evaluate exit codes of containers gcluster
	// does not control (custom base.tmpl sidecars, cluster webhook injections).
	assertPodFailurePolicyContainers(t, pureK8sJob.Spec.PodFailurePolicy, []string{"workload-container"})

	parallelOpts := pureOpts
	parallelOpts.ParallelContainers = 2
	if diff := cmp.Diff([]string{"workload-container-1", "workload-container-2"}, podFailurePolicyContainers(parallelOpts)); diff != "" {
		t.Errorf("podFailurePolicyContainers with parallel containers (-want +got):\n%s", diff)
	}

	fuseJob := pureJob
	fuseJob.WorkloadName = "fuse-sidecar-pfp"
	fuseJob.RawMounts = []string{"gs://my-bkt;/mnt/gcs;ro"}
	fuseOpts, err := orc.PrepareManifestOptions(fuseJob, "registry.local/app:v1", profile, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions with GCSFuse failed: %v", err)
	}
	_, fuseK8sJob, _ := parseServiceAndJobDocs(t, mustGenerateJobManifest(t, orc, fuseOpts, profile))
	assertPodFailurePolicyContainers(t, fuseK8sJob.Spec.PodFailurePolicy, []string{"workload-container"})
}
func makeDockerConfigSecret(name string, authHosts ...string) unstructured.Unstructured {
	auths := make(map[string]map[string]string, len(authHosts))
	for _, h := range authHosts {
		auths[h] = map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte("user:pass"))}
	}
	raw, _ := json.Marshal(map[string]any{"auths": auths})
	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": name},
			"type":       string(corev1.SecretTypeDockerConfigJson),
			"data": map[string]any{
				corev1.DockerConfigJsonKey: base64.StdEncoding.EncodeToString(raw),
			},
		},
	}
}

func newCachedTestOrchestrator() *GKEOrchestrator {
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.machineCapCache = testMachineCapCache()
	return orc
}

func testMachineCapCache() map[string]MachineTypeCap {
	tpuCap := MachineTypeCap{GuestCpus: 112, MemoryMb: 196608}
	tpuCap.Accelerators = append(tpuCap.Accelerators, struct {
		Count int    `json:"guestAcceleratorCount"`
		Type  string `json:"guestAcceleratorType"`
	}{Count: 4, Type: "tpu-v6e-slice"})
	return map[string]MachineTypeCap{
		"n2-standard-4:us-central1-a": {GuestCpus: 4, MemoryMb: 16000},
		"tpu-v6e-slice:us-central1-a": tpuCap,
	}
}

func TestImagePullSecrets_MultiImageMatching(t *testing.T) {
	setupMockMachineConfig(t)

	malformed := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": "bad-json"},
			"type":       string(corev1.SecretTypeDockerConfigJson),
			"data":       map[string]any{corev1.DockerConfigJsonKey: base64.StdEncoding.EncodeToString([]byte("{not-json"))},
		},
	}
	orc := newCachedTestOrchestrator()
	orc.kubeClient = &MockKubeClient{
		Objects: map[string][]unstructured.Unstructured{
			"secrets": {
				malformed,
				makeDockerConfigSecret("dockerhub-sec", "https://index.docker.io/v1/", "https://index.docker.io/v1/access-token"),
				makeDockerConfigSecret("ipv6-sidecar-sec", "https://[fd00::1]:5000"),
				makeDockerConfigSecret("team-a-sec", "harbor.local:8443/team-a"),
				makeDockerConfigSecret("team-b-sec", "harbor.local:8443/team-b"),
			},
		},
	}

	tpuJob := orchestrator.JobDefinition{
		WorkloadName:         "multi-img-tpu",
		CommandToRun:         "python3 train.py",
		ComputeType:          "v6e-4",
		MachineType:          "tpu-v6e-slice",
		Topology:             "2x2",
		ClusterLocation:      "us-central1-a",
		NumSlices:            1,
		NodesPerSlice:        1,
		MLDiagnosticsEnabled: true,
		MLDiagnosticsImage:   "[fd00::1]:5000/obs/ai-telemetry-collector:v1.2.3",
	}
	opts, err := orc.PrepareManifestOptions(tpuJob, "alpine:3.20", JobProfile{CapacityCount: 4}, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	if !strings.Contains(opts.ImagePullSecrets, "dockerhub-sec") || !strings.Contains(opts.ImagePullSecrets, "ipv6-sidecar-sec") {
		t.Fatalf("expected dockerhub-sec and ipv6-sidecar-sec, got %q", opts.ImagePullSecrets)
	}

	pwJob := tpuJob
	pwJob.WorkloadName = "pw-secrets"
	pwJob.MLDiagnosticsImage = ""
	pwJob.MLDiagnosticsEnabled = false
	pwJob.IsPathwaysJob = true
	pwJob.Pathways = orchestrator.PathwaysJobDefinition{
		GCSLocation:                 "gs://bkt",
		HeadNodePool:                "cpu-np",
		ServerImage:                 "harbor.local:8443/team-a/pw-server:v1",
		ProxyServerImage:            "harbor.local:8443/team-a/pw-proxy:v1",
		WorkerImage:                 "harbor.local:8443/team-a/pw-worker:v1",
		ColocatedPythonSidecarImage: "[fd00::1]:5000/obs/colocated-sidecar:v1",
	}
	pwOpts, err := orc.PrepareManifestOptions(pwJob, "harbor.local:8443/team-a/trainer:v1", JobProfile{CapacityCount: 4}, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions for Pathways failed: %v", err)
	}
	if strings.Count(pwOpts.ImagePullSecrets, "team-a-sec") != 1 || strings.Count(pwOpts.ImagePullSecrets, "ipv6-sidecar-sec") != 1 || strings.Contains(pwOpts.ImagePullSecrets, "team-b-sec") {
		t.Fatalf("unexpected Pathways ImagePullSecrets %q", pwOpts.ImagePullSecrets)
	}
}

func TestImagePullSecrets_AmbiguityWarning(t *testing.T) {
	setupMockMachineConfig(t)

	secrets := []unstructured.Unstructured{
		makeDockerConfigSecret("z-dup", "dup.registry.local"),
		makeDockerConfigSecret("a-dup", "https://dup.registry.local"),
		makeDockerConfigSecret("unique-sidecar", "sidecar.registry.local"),
	}
	orc := newCachedTestOrchestrator()
	orc.kubeClient = &MockKubeClient{Objects: map[string][]unstructured.Unstructured{"secrets": secrets}}
	pwJob := orchestrator.JobDefinition{
		WorkloadName:    "ambig-job",
		CommandToRun:    "python3 train.py",
		ComputeType:     "v6e-4",
		MachineType:     "tpu-v6e-slice",
		Topology:        "2x2",
		ClusterLocation: "us-central1-a",
		NumSlices:       1,
		NodesPerSlice:   1,
		IsPathwaysJob:   true,
		Pathways: orchestrator.PathwaysJobDefinition{
			GCSLocation:                 "gs://bkt",
			HeadNodePool:                "cpu-np",
			ColocatedPythonSidecarImage: "sidecar.registry.local/py-sidecar:v1",
		},
	}
	opts, err := orc.PrepareManifestOptions(pwJob, "dup.registry.local/workload:v1", JobProfile{CapacityCount: 4}, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	if !strings.Contains(opts.ImagePullSecrets, "unique-sidecar") || strings.Contains(opts.ImagePullSecrets, "dup") {
		t.Fatalf("expected only unique-sidecar, got %q", opts.ImagePullSecrets)
	}
}

func TestImagePullSecrets_ExplicitBypassAndActiveImages(t *testing.T) {
	setupMockMachineConfig(t)
	orc := newCachedTestOrchestrator()
	orc.kubeClient = &MockKubeClient{
		ListErrs: map[string]error{"secrets": errors.New("must not list secrets when --image-pull-secret is explicit")},
	}
	explicitJob := orchestrator.JobDefinition{
		WorkloadName:     "explicit-sec",
		CommandToRun:     "echo hi",
		ComputeType:      "n2-standard-4",
		MachineType:      "n2-standard-4",
		ClusterLocation:  "us-central1-a",
		NumSlices:        1,
		NodesPerSlice:    1,
		ImagePullSecrets: "manual-secret",
	}
	opts, err := orc.PrepareManifestOptions(explicitJob, "registry.local/app:v1", JobProfile{IsCPUMachine: true, CapacityCount: 4}, false, false)
	if err != nil || !strings.Contains(opts.ImagePullSecrets, "manual-secret") {
		t.Fatalf("explicit ImagePullSecrets failed: opts=%q err=%v", opts.ImagePullSecrets, err)
	}

	// An explicitly empty --image-pull-secret leaves the pods without imagePullSecrets even when a secret matches.
	orc.kubeClient = &MockKubeClient{Objects: map[string][]unstructured.Unstructured{"secrets": {makeDockerConfigSecret("reg-sec", "registry.local")}}}
	disabledJob := explicitJob
	disabledJob.ImagePullSecrets, disabledJob.ImagePullSecretsSpecified = "", true
	if opts, err := orc.PrepareManifestOptions(disabledJob, "registry.local/app:v1", JobProfile{IsCPUMachine: true, CapacityCount: 4}, false, false); err != nil || opts.ImagePullSecrets != "" {
		t.Fatalf("empty --image-pull-secret: opts=%q err=%v, want no imagePullSecrets", opts.ImagePullSecrets, err)
	}

	// The --service-account ServiceAccount is the one whose pull secrets discovery defers to.
	saJob := explicitJob
	saJob.ImagePullSecrets, saJob.ServiceAccountName = "", "team-sa"
	orc.kubeClient = &MockKubeClient{Objects: map[string][]unstructured.Unstructured{
		"secrets": {makeDockerConfigSecret("reg-sec", "registry.local")},
		"serviceaccounts": {{Object: map[string]any{
			"metadata":         map[string]any{"name": "team-sa"},
			"imagePullSecrets": []any{map[string]any{"name": "sa-cred"}},
		}}},
	}}
	if opts, err := orc.PrepareManifestOptions(saJob, "registry.local/app:v1", JobProfile{IsCPUMachine: true, CapacityCount: 4}, false, false); err != nil || opts.ImagePullSecrets != "" {
		t.Fatalf("ServiceAccount with pull secrets: opts=%q err=%v, want discovery skipped", opts.ImagePullSecrets, err)
	}

	pw := orchestrator.PathwaysJobDefinition{
		DeploySPS:        true,
		ServerImage:      "reg.local/server:v1",
		ProxyServerImage: "reg.local/proxy:v1",
		WorkerImage:      "reg.local/server:v1",
	}
	spsImages := activeWorkloadImages(orchestrator.JobDefinition{IsPathwaysJob: true, Pathways: pw}, "reg.local/workload:v1", "reg.local/telem:v1")
	if diff := cmp.Diff([]string{"reg.local/server:v1", "reg.local/telem:v1"}, spsImages); diff != "" {
		t.Errorf("DeploySPS activeWorkloadImages mismatch (-want +got):\n%s", diff)
	}
	pw.DeploySPS, pw.Headless = false, true
	headlessImages := activeWorkloadImages(orchestrator.JobDefinition{IsPathwaysJob: true, Pathways: pw}, "reg.local/workload:v1", "")
	if diff := cmp.Diff([]string{"reg.local/server:v1", "reg.local/proxy:v1"}, headlessImages); diff != "" {
		t.Errorf("Headless activeWorkloadImages mismatch (-want +got):\n%s", diff)
	}
	pw.Headless = false
	fullImages := activeWorkloadImages(orchestrator.JobDefinition{IsPathwaysJob: true, Pathways: pw}, "reg.local/workload:v1", "")
	if diff := cmp.Diff([]string{"reg.local/workload:v1", "reg.local/server:v1", "reg.local/proxy:v1"}, fullImages); diff != "" {
		t.Errorf("Pathways activeWorkloadImages mismatch (-want +got):\n%s", diff)
	}
}

func newCustomTemplateBaseOpts() (ManifestOptions, JobProfile) {
	return ManifestOptions{
		WorkloadName:    "custom-tmpl-job",
		FullImageName:   "test-image:latest",
		CommandToRun:    "echo hi",
		ComputeType:     "n2-standard-4",
		MachineType:     "n2-standard-4",
		ClusterLocation: "us-central1-a",
		NumSlices:       1,
		NodesPerSlice:   1,
	}, JobProfile{IsCPUMachine: true, CapacityCount: 4}
}

func newCustomTemplateTestOrc(t *testing.T, customDir string) *GKEOrchestrator {
	t.Helper()
	setupMockMachineConfig(t)
	orc := newCachedTestOrchestrator()
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.gkeCustomTemplatesPath = customDir
	return orc
}

func TestCustomTemplateLoading_PartialOverrideFallback(t *testing.T) {
	baseOpts, profile := newCustomTemplateBaseOpts()
	customDir := t.TempDir()
	customJob := `apiVersion: batch/v1
kind: Job
metadata:
  name: {{.WorkloadName}}
  annotations:
    custom-wrapper: "true"
  labels:
{{(StructuralData (.RenderBlock "workload_labels" 4))}}
spec:
  completionMode: Indexed
  parallelism: {{.NodesPerSlice}}
  completions: {{.NodesPerSlice}}
  template:
{{(StructuralData (.RenderBlock "pod_template" 4))}}
`
	if err := os.WriteFile(filepath.Join(customDir, "job.tmpl"), []byte(customJob), 0644); err != nil {
		t.Fatal(err)
	}
	orc := newCustomTemplateTestOrc(t, customDir)
	manifest, err := orc.GenerateJobManifest(baseOpts, profile)
	if err != nil {
		t.Fatalf("GenerateJobManifest failed: %v", err)
	}
	if !strings.Contains(manifest, `custom-wrapper: "true"`) || !strings.Contains(manifest, "gcluster.google.com/workload: custom-tmpl-job") {
		t.Errorf("unexpected rendered manifest:\n%s", manifest)
	}
}

func TestCustomTemplateLoading_DynamicBlockDiscovery(t *testing.T) {
	baseOpts, profile := newCustomTemplateBaseOpts()
	customDir := t.TempDir()
	if err := k8s.ExtractDefaultTemplates(customDir, false); err != nil {
		t.Fatalf("ExtractDefaultTemplates failed: %v", err)
	}
	origBase, err := os.ReadFile(filepath.Join(customDir, "base.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	extraBlock := "\n{{- define \"custom_user_block\" -}}\ncustom.example.com/team: \"ml-infra\"\n{{- end -}}\n"
	if err := os.WriteFile(filepath.Join(customDir, "base.tmpl"), append(origBase, []byte(extraBlock)...), 0644); err != nil {
		t.Fatal(err)
	}
	customJob := `apiVersion: batch/v1
kind: Job
metadata:
  name: {{.WorkloadName}}
  annotations:
{{(StructuralData (.RenderBlock "custom_user_block" 4))}}
spec:
  template:
{{(StructuralData (.RenderBlock "pod_template" 4))}}
`
	if err := os.WriteFile(filepath.Join(customDir, "job.tmpl"), []byte(customJob), 0644); err != nil {
		t.Fatal(err)
	}
	orc := newCustomTemplateTestOrc(t, customDir)
	manifest, err := orc.GenerateJobManifest(baseOpts, profile)
	if err != nil {
		t.Fatalf("GenerateJobManifest failed with user-defined block: %v", err)
	}
	if !strings.Contains(manifest, `custom.example.com/team: "ml-infra"`) {
		t.Errorf("expected custom_user_block output in manifest, got:\n%s", manifest)
	}
}

// TestCustomTemplateLoading_ExtractedDefaultsAreNoOp pins that pointing --gke-custom-templates-path at a fresh
// gke-template-extract output (with and without base.tmpl) renders byte-identical JobSet, Job, and Pathways
// manifests to the embedded defaults, covering the read-custom -> overlay -> compile-per-block -> reindent path.
func TestCustomTemplateLoading_ExtractedDefaultsAreNoOp(t *testing.T) {
	baseOpts, profile := newCustomTemplateBaseOpts()
	// Exercise the telemetry_sidecar base block too, so extracted and embedded base.tmpl are compared on it.
	baseOpts.MLDiagnosticsEnabled = true
	baseOpts.TelemetryCollectorImage = "registry.local/ai-telemetry-collector:v1"
	baseOpts.TelemetryCollectorPort = 9100
	pwJob := newCPUPathwaysJob("pw-noop")
	pwJob.ComputeType = "v6e-4"
	pwJob.MachineType = "tpu-v6e-slice"
	pwJob.Topology = "2x2"
	pwJob.MLDiagnosticsEnabled = true
	pwJob.MLDiagnosticsImage = "registry.local/ai-telemetry-collector:v1"
	pwJob.MLDiagnosticsTelemetryPort = 9100
	render := func(t *testing.T, customDir string) map[string]string {
		t.Helper()
		orc := newCustomTemplateTestOrc(t, customDir)
		out := make(map[string]string, 3)
		var err error
		if out["jobset"], err = orc.GenerateGKEManifest(baseOpts, profile); err != nil {
			t.Fatalf("GenerateGKEManifest(customDir=%q) failed: %v", customDir, err)
		}
		if out["job"], err = orc.GenerateJobManifest(baseOpts, profile); err != nil {
			t.Fatalf("GenerateJobManifest(customDir=%q) failed: %v", customDir, err)
		}
		if out["pathways"], err = orc.GeneratePathwaysManifest(pwJob, "img:latest", JobProfile{CapacityCount: 4}, false, false); err != nil {
			t.Fatalf("GeneratePathwaysManifest(customDir=%q) failed: %v", customDir, err)
		}
		return out
	}
	want := render(t, "")

	extracted := t.TempDir()
	if err := k8s.ExtractDefaultTemplates(extracted, false); err != nil {
		t.Fatalf("ExtractDefaultTemplates failed: %v", err)
	}
	if diff := cmp.Diff(want, render(t, extracted)); diff != "" {
		t.Errorf("extracted defaults must render identically to embedded templates (-embedded +extracted):\n%s", diff)
	}
	if err := os.Remove(filepath.Join(extracted, "base.tmpl")); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, render(t, extracted)); diff != "" {
		t.Errorf("extracted workload templates with embedded base.tmpl must render identically (-embedded +extracted):\n%s", diff)
	}
}

// newCPUPathwaysJob returns a minimal Pathways job on a CPU head/worker machine type, the shape used by the
// custom-template, dispatch and podFailurePolicy tests that only need a renderable Pathways manifest.
func newCPUPathwaysJob(name string) orchestrator.JobDefinition {
	return orchestrator.JobDefinition{
		WorkloadName:    name,
		CommandToRun:    "echo hi",
		NumSlices:       1,
		NodesPerSlice:   1,
		ClusterLocation: "us-central1-a",
		ComputeType:     "n2-standard-4",
		MachineType:     "n2-standard-4",
		IsPathwaysJob:   true,
		Pathways:        orchestrator.PathwaysJobDefinition{GCSLocation: "gs://bkt", HeadNodePool: "head-np"},
	}
}

// TestSubmitWorkloadTemplate_DispatchesByJobKind pins that each job kind reaches its own generator (observed via
// the dry-run manifest) and that an unknown template name is an error rather than a silent JobSet fallback.
func TestSubmitWorkloadTemplate_DispatchesByJobKind(t *testing.T) {
	orc := newCustomTemplateTestOrc(t, "")
	_, profile := newCustomTemplateBaseOpts()
	pwJob := newCPUPathwaysJob("dispatch-pw")
	stdJob := newCPUPathwaysJob("dispatch-std")
	stdJob.IsPathwaysJob = false

	for _, tc := range []struct {
		name               string
		job                orchestrator.JobDefinition
		wantTemplate       string
		wantReplicatedJobs []string
	}{
		{name: "standard job renders jobset.tmpl", job: stdJob, wantTemplate: k8s.JobSetTemplate, wantReplicatedJobs: []string{"main-job"}},
		{name: "pathways job renders pathways_jobset.tmpl", job: pwJob, wantTemplate: k8s.PathwaysJobSetTemplate, wantReplicatedJobs: []string{"pathways-head", "worker"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := workloadTemplateName(tc.job); got != tc.wantTemplate {
				t.Fatalf("workloadTemplateName() = %q, want %q", got, tc.wantTemplate)
			}
			job := tc.job
			job.DryRunManifest = filepath.Join(t.TempDir(), "manifest.yaml")
			if err := orc.generateAndSubmitManifests(job, "img:v1", profile, false, false); err != nil {
				t.Fatalf("generateAndSubmitManifests failed: %v", err)
			}
			content, err := os.ReadFile(job.DryRunManifest)
			if err != nil {
				t.Fatal(err)
			}
			_, templates := parseStrictJobSetTemplates(t, string(content))
			if diff := cmp.Diff(tc.wantReplicatedJobs, slices.Sorted(maps.Keys(templates))); diff != "" {
				t.Errorf("dry-run manifest replicatedJobs mismatch (-want +got):\n%s", diff)
			}
		})
	}

	err := orc.submitWorkloadTemplate("pathways-jobset.tmpl", pwJob, "img:v1", profile, false, false)
	if err == nil || !strings.Contains(err.Error(), `no manifest generator is wired for workload template "pathways-jobset.tmpl"`) {
		t.Errorf("unknown template name: error = %v, want internal error naming the template", err)
	}
}

// TestGeneratePathwaysManifest_PodFailurePolicyScopingByMode pins podFailurePolicyContainers on the Pathways head:
// scoped to workload-container when the head runs the workload, pod-wide (no containerName) for headless and SPS
// heads that have no workload container, and valid in every mode.
func TestGeneratePathwaysManifest_PodFailurePolicyScopingByMode(t *testing.T) {
	orc := newCustomTemplateTestOrc(t, "")
	_, profile := newCustomTemplateBaseOpts()
	for _, tc := range []struct {
		mode          string
		headless      bool
		deploySPS     bool
		wantContainer []string // nil = pod-wide rule
	}{
		{mode: "default", wantContainer: []string{"workload-container"}},
		{mode: "headless", headless: true},
		{mode: "sps", deploySPS: true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			job := newCPUPathwaysJob("pw-pfp-" + tc.mode)
			job.RestartOnExitCodes = []int{143, 42}
			job.Pathways.Headless = tc.headless
			job.Pathways.DeploySPS = tc.deploySPS
			manifest, err := orc.GeneratePathwaysManifest(job, "img:v1", profile, false, false)
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}
			if err := ValidateJobSetManifest(manifest); err != nil {
				t.Fatalf("ValidateJobSetManifest failed: %v\n%s", err, manifest)
			}
			_, templates := parseStrictJobSetTemplates(t, manifest)
			head := templates["pathways-head"]
			if head.Spec.PodFailurePolicy == nil || len(head.Spec.PodFailurePolicy.Rules) != 1 {
				t.Fatalf("pathways-head podFailurePolicy = %+v, want exactly one rule", head.Spec.PodFailurePolicy)
			}
			assertPodFailurePolicyContainers(t, head.Spec.PodFailurePolicy, tc.wantContainer)
			if worker := templates["worker"]; worker.Spec.PodFailurePolicy != nil {
				t.Errorf("worker replicatedJob must not carry a podFailurePolicy, got %+v", worker.Spec.PodFailurePolicy)
			}
		})
	}
}

func TestCustomTemplateLoading_MalformedBaseFailsFast(t *testing.T) {
	baseOpts, profile := newCustomTemplateBaseOpts()
	customDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(customDir, "base.tmpl"), []byte(`{{- define "broken" }}{{ .Unclosed`), 0644); err != nil {
		t.Fatal(err)
	}
	orc := newCustomTemplateTestOrc(t, customDir)
	const wantErr = "failed to parse custom base.tmpl"

	if _, err := orc.GenerateGKEManifest(baseOpts, profile); err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("GenerateGKEManifest error = %v, want substring %q", err, wantErr)
	}
	if _, err := orc.GenerateJobManifest(baseOpts, profile); err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("GenerateJobManifest error = %v, want substring %q", err, wantErr)
	}
	pwJob := newCPUPathwaysJob("pw-broken")
	if _, err := orc.GeneratePathwaysManifest(pwJob, "img:latest", profile, false, false); err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("GeneratePathwaysManifest error = %v, want substring %q", err, wantErr)
	}
}

func TestCustomTemplateLoading_RenderBlockErrors(t *testing.T) {
	_, blocks, err := k8s.LoadWorkloadTemplate("", "jobset.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	data := jobSetTemplateData{WorkloadName: "zero-job", baseBlocks: blocks}
	rendered, err := data.RenderBlock("workload_labels", 2)
	if err != nil || !strings.Contains(rendered, "  gcluster.google.com/workload: zero-job") {
		t.Fatalf("RenderBlock(workload_labels) = (%q, %v)", rendered, err)
	}
	_, err = data.RenderBlock("does_not_exist", 2)
	if err == nil || !strings.Contains(err.Error(), "does_not_exist") || !strings.Contains(err.Error(), "workload_labels") {
		t.Errorf("expected unknown block error mentioning block name and available blocks, got: %v", err)
	}
	data.PriorityClassName = "high\n  hostPID: true"
	if _, err := data.RenderBlock("pod_auth_and_priority", 2); err == nil || !strings.Contains(err.Error(), "pod_auth_and_priority") {
		t.Errorf("expected YAML injection inside a block to fail naming the block, got: %v", err)
	}
}

func TestCustomTemplateLoading_PartialBaseBlockAndCommentsCRLF(t *testing.T) {
	baseOpts, profile := newCustomTemplateBaseOpts()
	customDir := t.TempDir()
	partialBaseCRLF := strings.Join([]string{
		"# Team-wide custom workload_labels override",
		"{{- define \"workload_labels\" -}}",
		"gcluster.google.com/workload: {{.WorkloadName}}",
		"example.com/team: platform",
		"{{- end -}}",
		"# Trailing comment outside define blocks",
		"",
	}, "\r\n")
	if err := os.WriteFile(filepath.Join(customDir, "base.tmpl"), []byte(partialBaseCRLF), 0644); err != nil {
		t.Fatal(err)
	}
	orc := newCustomTemplateTestOrc(t, customDir)

	pwJob := newCPUPathwaysJob("custom-tmpl-pw")

	for name, gen := range map[string]func() (string, error){
		"job":      func() (string, error) { return orc.GenerateJobManifest(baseOpts, profile) },
		"jobset":   func() (string, error) { return orc.GenerateGKEManifest(baseOpts, profile) },
		"pathways": func() (string, error) { return orc.GeneratePathwaysManifest(pwJob, "img:v1", profile, false, false) },
	} {
		manifest, err := gen()
		if err != nil {
			t.Fatalf("%s manifest generation failed with partial CRLF base.tmpl: %v", name, err)
		}
		if strings.Contains(manifest, "\r") || strings.Contains(manifest, "Team-wide custom") {
			t.Errorf("%s manifest leaked CRLF or top-level base.tmpl comment:\n%s", name, manifest)
		}
		if !strings.Contains(manifest, "example.com/team: platform") {
			t.Errorf("%s manifest missing custom workload_labels:\n%s", name, manifest)
		}
	}

	badPathOrc := newCustomTemplateTestOrc(t, filepath.Join(customDir, "does-not-exist"))
	if _, err := badPathOrc.GenerateGKEManifest(baseOpts, profile); err == nil || !strings.Contains(err.Error(), "--gke-custom-templates-path") {
		t.Errorf("expected manifest generation to fail when gkeCustomTemplatesPath does not exist, got: %v", err)
	}
}

// TestCustomTemplateLoading_LegacyJobSetTemplate renders the pre-refactor jobset.tmpl (the file users extracted
// before base.tmpl existed, kept verbatim under testdata/) through the current PrepareManifestOptions pipeline with
// a job that exercises every template input, and checks the result still validates.
func TestCustomTemplateLoading_LegacyJobSetTemplate(t *testing.T) {
	legacyJobSet, err := os.ReadFile(filepath.Join("testdata", "legacy_jobset.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(legacyJobSet, []byte(".RenderBlock")) {
		t.Fatal("testdata/legacy_jobset.tmpl must be the pre-base.tmpl template and not reference .RenderBlock")
	}
	customDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(customDir, "jobset.tmpl"), legacyJobSet, 0644); err != nil {
		t.Fatal(err)
	}
	orc := newGPUDerivationTestOrc(t)
	orc.gkeCustomTemplatesPath = customDir
	jobDef := gpuDerivationJobDef("legacy-gpu-job")

	profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&jobDef)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	opts, err := orc.PrepareManifestOptions(jobDef, "registry.local/trainer:v1", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("legacy custom jobset.tmpl failed to render: %v", err)
	}
	if err := ValidateJobSetManifest(manifest); err != nil {
		t.Fatalf("legacy manifest failed validation: %v\n%s", err, manifest)
	}
	assertLegacyJobSetRendersAllInputs(t, manifest, "legacy-gpu-job")
}

func assertLegacyJobSetRendersAllInputs(t *testing.T, manifest, workloadName string) {
	t.Helper()
	labels, templates := parseStrictJobSetTemplates(t, manifest)
	if labels["kueue.x-k8s.io/queue-name"] != "user-queue" || labels["managed-mldiagnostics-gke"] != "true" {
		t.Errorf("unexpected legacy JobSet labels: %+v", labels)
	}
	mainJob := templates["main-job"]
	if mainJob.Spec.PodFailurePolicy == nil || len(mainJob.Spec.PodFailurePolicy.Rules) != 1 {
		t.Fatalf("expected one podFailurePolicy rule, got %+v", mainJob.Spec.PodFailurePolicy)
	}
	if diff := cmp.Diff([]int32{137, 143}, mainJob.Spec.PodFailurePolicy.Rules[0].OnExitCodes.Values); diff != "" {
		t.Errorf("podFailurePolicy exit codes mismatch (-want +got):\n%s", diff)
	}
	podSpec := mainJob.Spec.Template.Spec
	if podSpec.PriorityClassName != "high" || podSpec.ServiceAccountName != "workload-ksa" || len(podSpec.ImagePullSecrets) != 2 {
		t.Errorf("legacy pod auth mismatch: priorityClassName=%q serviceAccountName=%q imagePullSecrets=%+v", podSpec.PriorityClassName, podSpec.ServiceAccountName, podSpec.ImagePullSecrets)
	}
	if podSpec.NodeSelector["team"] != "ml" || len(podSpec.Tolerations) == 0 || len(podSpec.SchedulingGates) == 0 {
		t.Errorf("legacy pod placement mismatch: nodeSelector=%+v tolerations=%+v schedulingGates=%+v", podSpec.NodeSelector, podSpec.Tolerations, podSpec.SchedulingGates)
	}
	coordHost := fmt.Sprintf("%s-main-job-0-0.%s", workloadName, workloadName)
	assertContainerEnvMap(t, podSpec.Containers[0], map[string]string{
		"MASTER_ADDR":        coordHost,
		"NCCL_DEBUG":         "INFO",
		"USER_VAR":           "hello",
		"WORKERS_0_HOSTNAME": coordHost,
	})
	assertContainerHasMount(t, podSpec.Containers[0], "/mnt/shared", true)
	assertContainerHasMount(t, podSpec.Containers[0], "/tmp/mtc", false)
}

func TestCustomTemplateValidation_WorkloadTelemetry(t *testing.T) {
	setupMockMachineConfig(t)
	tpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 112, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v6e-slice"}]}`}
	cases := []struct {
		name    string
		baseOvr string
		wantSub string
	}{
		{
			name: "missing ai-telemetry-collector sidecar",
			baseOvr: `{{- define "pod_template" -}}
metadata:
  labels:
    gcluster.google.com/workload: {{.WorkloadName}}
    tpu-workload-name: {{.WorkloadName}}
spec:
  restartPolicy: Never
  containers:
  - name: workload-container
    image: {{.FullImageName}}
{{- end -}}
`,
			wantSub: "ai-telemetry-collector",
		},
		{
			name: "missing tpu-workload-name pod label",
			baseOvr: `{{- define "pod_template" -}}
metadata:
  labels:
    gcluster.google.com/workload: {{.WorkloadName}}
spec:
  restartPolicy: Never
  initContainers:
{{(StructuralData (.RenderBlock "telemetry_sidecar" 2))}}
  containers:
  - name: workload-container
    image: {{.FullImageName}}
{{- end -}}
`,
			wantSub: "tpu-workload-name",
		},
		{
			name: "collector as a plain init container without restartPolicy",
			baseOvr: `{{- define "telemetry_sidecar" -}}
- name: ai-telemetry-collector
  image: {{.TelemetryCollectorImage}}
{{- end -}}
`,
			wantSub: "restartPolicy: Always",
		},
		{
			name: "stale workload_labels keeps the managed webhook label",
			baseOvr: `{{- define "workload_labels" -}}
gcluster.google.com/workload: {{.WorkloadName}}
managed-mldiagnostics-gke: "true"
{{- end -}}
`,
			wantSub: "managed-mldiagnostics-gke",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			customDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(customDir, "base.tmpl"), []byte(tc.baseOvr), 0644); err != nil {
				t.Fatal(err)
			}
			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				"gcloud compute machine-types describe tpu-v6e-slice --zone=us-central1-a --format=json": {tpuResp, tpuResp},
			})
			orc := newTestGKEOrchestrator(mockExec)
			orc.clusterZones = []string{"us-central1-a"}
			orc.gkeCustomTemplatesPath = customDir

			opts := ManifestOptions{
				WorkloadName:            "telemetry-job",
				FullImageName:           "registry.local/jax:v1",
				CommandToRun:            "python3 train.py",
				ComputeType:             "tpu-v6e-slice",
				MachineType:             "tpu-v6e-slice",
				ClusterLocation:         "us-central1-a",
				NumSlices:               1,
				NodesPerSlice:           1,
				MLDiagnosticsEnabled:    true,
				TelemetryCollectorImage: "registry.local/ai-telemetry-collector:v1",
				TelemetryCollectorPort:  8889,
			}
			profile := JobProfile{CapacityCount: 4}

			for kind, fn := range map[string]func() (string, error){
				"JobSet": func() (string, error) { return orc.GenerateGKEManifest(opts, profile) },
				"Job":    func() (string, error) { return orc.GenerateJobManifest(opts, profile) },
			} {
				_, err := fn()
				if err == nil || !strings.Contains(err.Error(), tc.wantSub) || !strings.Contains(err.Error(), "gcluster job gke-template-extract") {
					t.Errorf("%s expected error mentioning %q and gke-template-extract, got: %v", kind, tc.wantSub, err)
				}
			}
		})
	}
}

func TestCustomTemplateValidation_PathwaysTelemetry(t *testing.T) {
	setupMockMachineConfig(t)
	tpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 112, "memoryMb": 196608, "accelerators": [{"guestAcceleratorCount": 4, "guestAcceleratorType": "tpu-v6e-slice"}]}`}
	cases := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{
			name: "missing ai-telemetry-collector on worker",
			mutate: func(src string) string {
				return strings.ReplaceAll(src, `{{(StructuralData (.RenderBlock "telemetry_sidecar" 12))}}`, "")
			},
			want: "ai-telemetry-collector",
		},
		{
			name: "missing tpu-workload-name on worker",
			mutate: func(src string) string {
				return strings.ReplaceAll(src, "tpu-workload-name: {{.WorkloadName}}", "other-label: {{.WorkloadName}}")
			},
			want: "tpu-workload-name",
		},
		{
			name: "leaked tpu-workload-name onto pathways-head",
			mutate: func(src string) string {
				return strings.Replace(src, "gcluster.google.com/workload: {{.WorkloadName}}", "gcluster.google.com/workload: {{.WorkloadName}}\n              tpu-workload-name: {{.WorkloadName}}", 1)
			},
			want: "pathways-head",
		},
		{
			name: "leaked ai-telemetry-collector onto pathways-head",
			mutate: func(src string) string {
				return strings.Replace(src, "- name: pathways-rm", "- name: ai-telemetry-collector\n              image: {{.TelemetryCollectorImage}}\n            - name: pathways-rm", 1)
			},
			want: "pathways-head",
		},
		{
			name: "leaked ai-telemetry-collector into pathways-head regular containers",
			mutate: func(src string) string {
				return strings.Replace(src, "- name: workload-container", "- name: ai-telemetry-collector\n              image: {{.TelemetryCollectorImage}}\n            - name: workload-container", 1)
			},
			want: "pathways-head",
		},
		{
			name: "missing worker replicatedJob",
			mutate: func(src string) string {
				return strings.Replace(src, "  - name: worker", "  - name: renamed-worker", 1)
			},
			want: "no worker replicatedJob",
		},
		{
			name: "stale workload_labels keeps the managed webhook label on the JobSet",
			mutate: func(src string) string {
				return strings.Replace(src, `{{(StructuralData (.RenderBlock "workload_labels" 4))}}`, "    gcluster.google.com/workload: {{.WorkloadName}}\n    managed-mldiagnostics-gke: \"true\"", 1)
			},
			want: "managed-mldiagnostics-gke",
		},
		{
			name: "telemetry port already declared by pathways-worker",
			mutate: func(src string) string {
				return strings.Replace(src, "- containerPort: 8471", "- containerPort: 8471\n              - containerPort: 8889", 1)
			},
			want: `declared by container "pathways-worker"`,
		},
		{
			name: "ai-telemetry-collector placed in regular containers instead of initContainers",
			mutate: func(src string) string {
				withoutInit := strings.ReplaceAll(src, `{{(StructuralData (.RenderBlock "telemetry_sidecar" 12))}}`, "")
				return strings.Replace(withoutInit, "- name: pathways-worker", "- name: ai-telemetry-collector\n              image: {{.TelemetryCollectorImage}}\n            - name: pathways-worker", 1)
			},
			want: "initContainers",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			customDir := t.TempDir()
			if err := k8s.ExtractDefaultTemplates(customDir, false); err != nil {
				t.Fatalf("ExtractDefaultTemplates failed: %v", err)
			}
			pwPath := filepath.Join(customDir, "pathways_jobset.tmpl")
			raw, err := os.ReadFile(pwPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pwPath, []byte(tc.mutate(string(raw))), 0644); err != nil {
				t.Fatal(err)
			}

			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				"gcloud compute machine-types describe tpu-v6e-slice --zone=us-central1-a --format=json": {tpuResp},
			})
			orc := newTestGKEOrchestrator(mockExec)
			orc.clusterZones = []string{"us-central1-a"}
			orc.gkeCustomTemplatesPath = customDir

			pwJob := orchestrator.JobDefinition{
				WorkloadName:         "pw-telemetry",
				CommandToRun:         "python3 train.py",
				ComputeType:          "v6e-4",
				MachineType:          "tpu-v6e-slice",
				Topology:             "2x2",
				NumSlices:            1,
				NodesPerSlice:        1,
				ClusterLocation:      "us-central1-a",
				MLDiagnosticsEnabled: true,
				MLDiagnosticsImage:   "registry.local/ai-telemetry-collector:v1",
				IsPathwaysJob:        true,
				Pathways: orchestrator.PathwaysJobDefinition{
					GCSLocation:  "gs://bkt",
					HeadNodePool: "cpu-np",
				},
			}
			_, err = orc.GeneratePathwaysManifest(pwJob, "registry.local/jax:v1", JobProfile{CapacityCount: 4}, false, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "gcluster job gke-template-extract") {
				t.Errorf("expected error mentioning %q and gke-template-extract, got: %v", tc.want, err)
			}
		})
	}

	t.Run("built-in template port collision omits custom-template re-extract hint", func(t *testing.T) {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"gcloud compute machine-types describe tpu-v6e-slice --zone=us-central1-a --format=json": {tpuResp},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.clusterZones = []string{"us-central1-a"}
		_, err := orc.GeneratePathwaysManifest(orchestrator.JobDefinition{
			WorkloadName:               "pw-port-clash",
			CommandToRun:               "python3 train.py",
			ComputeType:                "v6e-4",
			MachineType:                "tpu-v6e-slice",
			Topology:                   "2x2",
			NumSlices:                  1,
			NodesPerSlice:              1,
			ClusterLocation:            "us-central1-a",
			MLDiagnosticsEnabled:       true,
			MLDiagnosticsImage:         "registry.local/ai-telemetry-collector:v1",
			MLDiagnosticsTelemetryPort: 8080,
			IsPathwaysJob:              true,
			Pathways:                   orchestrator.PathwaysJobDefinition{GCSLocation: "gs://bkt", HeadNodePool: "cpu-np"},
		}, "registry.local/jax:v1", JobProfile{CapacityCount: 4}, false, false)
		if err == nil || !strings.Contains(err.Error(), `declared by container "pathways-worker"`) || strings.Contains(err.Error(), "gke-template-extract") {
			t.Fatalf("got err = %v, want pathways-worker port collision without custom-template hint", err)
		}
	})
}

func TestCustomTemplateValidation_PodFailurePolicyAndTemplatePathErrors(t *testing.T) {
	customDir := t.TempDir()
	renamedContainerBase := `{{- define "pod_template" -}}
metadata:
  labels:
    gcluster.google.com/workload: {{.WorkloadName}}
spec:
  restartPolicy: Never
  containers:
  - name: custom-trainer
    image: {{.FullImageName}}
{{- end -}}
`
	if err := os.WriteFile(filepath.Join(customDir, "base.tmpl"), []byte(renamedContainerBase), 0644); err != nil {
		t.Fatal(err)
	}
	orc := newCustomTemplateTestOrc(t, customDir)
	baseOpts, profile := newCustomTemplateBaseOpts()
	unscopedJob := orchestrator.JobDefinition{
		WorkloadName:       baseOpts.WorkloadName,
		CommandToRun:       baseOpts.CommandToRun,
		ComputeType:        baseOpts.ComputeType,
		MachineType:        baseOpts.MachineType,
		ClusterLocation:    baseOpts.ClusterLocation,
		NumSlices:          1,
		NodesPerSlice:      1,
		RestartOnExitCodes: []int{42},
	}
	unscopedOpts, err := orc.PrepareManifestOptions(unscopedJob, baseOpts.FullImageName, profile, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	// --restart-on-exit-codes is always scoped to the workload container(s), so a custom template that renames
	// the container fails fast at preflight with an actionable hint rather than silently changing semantics.
	_, err = orc.GenerateGKEManifest(unscopedOpts, profile)
	if err == nil || !strings.Contains(err.Error(), `containerName "workload-container"`) || !strings.Contains(err.Error(), "keep the \"workload-container\" name prefix") {
		t.Errorf("expected PodFailurePolicy missing-container error with rename hint for renamed container, got: %v", err)
	}

	noPFPJob := unscopedJob
	noPFPJob.RestartOnExitCodes = nil
	noPFPOpts, err := orc.PrepareManifestOptions(noPFPJob, baseOpts.FullImageName, profile, false, false)
	if err != nil {
		t.Fatalf("PrepareManifestOptions failed: %v", err)
	}
	if _, err := orc.GenerateGKEManifest(noPFPOpts, profile); err != nil {
		t.Fatalf("renamed custom container template must succeed without --restart-on-exit-codes: %v", err)
	}

	// Rules without onExitCodes (e.g. onPodConditions) carry no container reference to validate.
	podConditionsManifest := `apiVersion: batch/v1
kind: Job
metadata:
  name: pod-conditions-pfp
spec:
  podFailurePolicy:
    rules:
    - action: Ignore
      onPodConditions:
      - type: DisruptionTarget
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: workload-container
        image: busybox
`
	if err := ValidateJobSetManifest(podConditionsManifest); err != nil {
		t.Errorf("expected ValidateJobSetManifest to accept onPodConditions rules, got: %v", err)
	}
}

// TestRenderBlock_RecursiveOverrideFailsFast pins that a custom base.tmpl block which calls .RenderBlock on itself
// returns an error instead of deadlocking on safetext's per-template execution lock, and that the same block can
// still be rendered more than once sequentially.
func TestRenderBlock_RecursiveOverrideFailsFast(t *testing.T) {
	baseOpts, profile := newCustomTemplateBaseOpts()
	customDir := t.TempDir()
	recursiveBase := "{{- define \"workload_labels\" -}}\n{{(StructuralData (.RenderBlock \"workload_labels\" 0))}}\n{{- end -}}\n"
	if err := os.WriteFile(filepath.Join(customDir, "base.tmpl"), []byte(recursiveBase), 0644); err != nil {
		t.Fatal(err)
	}
	orc := newCustomTemplateTestOrc(t, customDir)
	done := make(chan error, 1)
	go func() {
		_, err := orc.GenerateGKEManifest(baseOpts, profile)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), `block "workload_labels" renders itself`) {
			t.Fatalf("expected recursive .RenderBlock error, got: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("GenerateGKEManifest hung on a recursive .RenderBlock override instead of returning an error")
	}

	// Sequential reuse of one block (jobset.tmpl renders workload_labels once per JobSet; job.tmpl once per Job)
	// must keep working: render the embedded defaults twice with the same orchestrator.
	orc = newCustomTemplateTestOrc(t, "")
	for i := 0; i < 2; i++ {
		if _, err := orc.GenerateGKEManifest(baseOpts, profile); err != nil {
			t.Fatalf("render %d with embedded defaults failed: %v", i+1, err)
		}
	}
}

func assertRenderedNamespace(t *testing.T, kind, manifest, wantNS string) {
	t.Helper()
	var meta struct {
		Metadata struct {
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	if err := k8syaml.Unmarshal([]byte(manifest), &meta); err != nil {
		t.Fatalf("%s: failed to parse metadata: %v\n%s", kind, err, manifest)
	}
	if meta.Metadata.Namespace != wantNS {
		t.Errorf("%s: metadata.namespace = %q, want %q", kind, meta.Metadata.Namespace, wantNS)
	}
	if wantNS == "" && strings.Contains(manifest, "namespace:") {
		t.Errorf("%s: no namespace resolved but manifest renders one:\n%s", kind, manifest)
	}
}

// TestGenerateManifests_RenderResolvedNamespace pins that all workload templates declare metadata.namespace from
// the namespace resolved for the submission, so a saved manifest applied outside gcluster lands in the same
// namespace as the PV/PVC documents appended to it, and that nothing is rendered when no namespace was resolved.
func TestGenerateManifests_RenderResolvedNamespace(t *testing.T) {
	setupMockMachineConfig(t)
	newOrc := func(ns string) *GKEOrchestrator {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.projectID = "mock-project"
		orc.clusterZones = []string{"us-central1-a"}
		orc.namespace = ns
		orc.clusterDesc.NodePools = []gkeJobNodePool{{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}}}
		return orc
	}
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 2}
	jobSetOpts := ManifestOptions{WorkloadName: "ns-render", FullImageName: "img:v1", CommandToRun: "echo hi", ComputeType: "n2-standard-2", MachineType: "n2-standard-2", ClusterLocation: "us-central1-a", NumSlices: 1, NodesPerSlice: 1}
	pwJob := orchestrator.JobDefinition{
		WorkloadName: "ns-render-pw", CommandToRun: "echo hi", NumSlices: 1, NodesPerSlice: 1, ClusterLocation: "us-central1-a", ComputeType: "n2-standard-2", MachineType: "n2-standard-2", IsPathwaysJob: true,
		Pathways: orchestrator.PathwaysJobDefinition{ProxyServerImage: "proxy:latest", ServerImage: "server:latest", WorkerImage: "worker:latest", GCSLocation: "gs://bkt", HeadNodePool: "pathways-np"},
	}

	// The rendered namespace must always equal the one the orchestrator resolved: explicitly (--gke-namespace)
	// or implicitly during generation (pull-secret discovery resolves it through the pinned context), and
	// nothing is rendered while it is still unresolved.
	for _, ns := range []string{"team-a", ""} {
		t.Run("namespace="+ns, func(t *testing.T) {
			orc := newOrc(ns)
			jobSet, err := orc.GenerateGKEManifest(jobSetOpts, profile)
			if err != nil {
				t.Fatalf("GenerateGKEManifest failed: %v", err)
			}
			assertRenderedNamespace(t, "jobset", jobSet, ns)

			jobManifest := mustGenerateJobManifest(t, orc, jobSetOpts, profile)
			svcDoc, jobDoc, _ := parseServiceAndJobDocs(t, jobManifest)
			if svcDoc.Namespace != ns || jobDoc.Namespace != ns {
				t.Errorf("job: Service/Job metadata.namespace = (%q, %q), want %q", svcDoc.Namespace, jobDoc.Namespace, ns)
			}
			assertRenderedNamespace(t, "job-service", jobManifest, ns)

			pathways, err := orc.GeneratePathwaysManifest(pwJob, "img:v1", profile, false, false)
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}
			assertRenderedNamespace(t, "pathways", pathways, orc.namespace)
			if ns != "" && orc.namespace != ns {
				t.Errorf("explicit namespace %q was overwritten during generation with %q", ns, orc.namespace)
			}
		})
	}
}
