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
