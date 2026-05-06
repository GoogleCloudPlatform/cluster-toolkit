// Copyright 2024 Google LLC
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

package job

import (
	"bytes"
	"hpc-toolkit/pkg/orchestrator"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err := root.Execute()

	return buf.String(), err
}

func TestSubmitCmd_PathwaysDryRun(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "pathways-manifest-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{
		State: PrereqState{
			LastCheckedTimestamp:         time.Now(),
			LastCheckedProjectID:         "test-project",
			GCloudSDKInstalled:           true,
			GCloudAuthenticated:          true,
			ADCConfigured:                true,
			KubectlInstalled:             true,
			GKEGCloudAuthPluginInstalled: true,
			DockerCredsConfigured:        true,
		},
	}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	// Reset flags before each test
	resetSubmitCmdFlags()

	output, err := executeCommand(JobCmd,
		"submit",
		"--pathways",
		"--name", "pathways-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--dry-run-out", tmpfile.Name(),
		"--pathways-proxy-server-image", "proxy:latest",
		"--pathways-server-image", "server:latest",
		"--pathways-worker-image", "worker:latest",
		"--pathways-gcs-location", "gs://my-bucket",
		"--compute-type", "n2-standard-4",
	)

	if err != nil {
		if !strings.Contains(output, "gcloud not found") {
			t.Fatalf("command failed with error: %v, output: %s", err, output)
		}
	}

	manifest, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to read dry-run output file: %v", err)
	}

	// Verify the manifest content
	manifestStr := string(manifest)
	if !strings.Contains(manifestStr, "name: pathways-test") {
		t.Errorf("manifest does not contain correct workload name")
	}

	if !strings.Contains(manifestStr, "image: proxy:latest") {
		t.Errorf("manifest does not contain correct proxy image")
	}

	if !strings.Contains(manifestStr, "--gcs_location=gs://my-bucket") {
		t.Errorf("manifest does not contain correct GCS location")
	}
}

func TestSubmitCmd_RegularDryRun(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "regular-manifest-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{
		State: PrereqState{
			LastCheckedTimestamp:         time.Now(),
			LastCheckedProjectID:         "test-project",
			GCloudSDKInstalled:           true,
			GCloudAuthenticated:          true,
			ADCConfigured:                true,
			KubectlInstalled:             true,
			GKEGCloudAuthPluginInstalled: true,
			DockerCredsConfigured:        true,
		},
	}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	// Reset flags before each test
	resetSubmitCmdFlags()

	output, err := executeCommand(JobCmd,
		"submit",
		"--name", "regular-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--dry-run-out", tmpfile.Name(),
		"--compute-type", "n2-standard-4",
	)

	if err != nil {
		if !strings.Contains(output, "gcloud not found") {
			t.Fatalf("command failed with error: %v, output: %s", err, output)
		}
	}

	// Read the output file
	manifest, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to read dry-run output file: %v", err)
	}

	// Verify the manifest content
	manifestStr := string(manifest)
	if !strings.Contains(manifestStr, "name: regular-test") {
		t.Errorf("manifest does not contain correct workload name")
	}

	if !strings.Contains(manifestStr, "image: busybox") {
		t.Errorf("manifest does not contain correct image")
	}
}

func resetSubmitCmdFlags() {
	imageName = ""
	baseImage = ""
	buildContext = ""
	commandToRun = ""
	computeType = ""
	dryRunManifest = ""
	clusterName = ""
	location = ""
	projectID = ""
	workloadName = ""
	kueueQueueName = ""
	numNodes = 1
	numSlices = 1
	restarts = 1
	ttlAfterFinished = "1h"
	gracePeriodStr = "30s"
	placementPolicy = ""
	nodeConstraint = nil
	cpuAffinityStr = ""
	restartOnExitCodes = nil
	imagePullSecrets = ""
	serviceAccountName = ""
	topology = ""
	gkeScheduler = ""
	platform = "linux/amd64"
	awaitJobCompletion = false
	priorityClassName = "medium"
	isPathwaysJob = false
	pathways = orchestrator.PathwaysJobDefinition{MaxSliceRestarts: 1}
}

func TestParseVolumeFlag_PVC(t *testing.T) {
	got, err := parseVolumeFlag([]string{"my-pvc:/mnt/data"})
	if err != nil {
		t.Fatalf("parseVolumeFlag() unexpected error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(got))
	}
	if got[0].Type != "pvc" || got[0].MountPath != "/mnt/data" || got[0].ReadOnly != true {
		t.Errorf("unexpected volume definition: %+v", got[0])
	}
}

func TestParseVolumeFlag_GCS(t *testing.T) {
	tests := []struct {
		name     string
		vStr     string
		readOnly bool
	}{
		{"Valid GCS Fuse", "gs://my-bucket:/mnt/gcs", true},
		{"Valid GCS Fuse with ro", "gs://my-bucket:/mnt/gcs:ro", true},
		{"Valid GCS Fuse with rw", "gs://my-bucket:/mnt/gcs:rw", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVolumeFlag([]string{tt.vStr})
			if err != nil {
				t.Fatalf("parseVolumeFlag() unexpected error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("expected 1 volume, got %d", len(got))
			}
			if got[0].Type != "gcsfuse" || got[0].MountPath != "/mnt/gcs" || got[0].ReadOnly != tt.readOnly {
				t.Errorf("unexpected volume definition: %+v", got[0])
			}
		})
	}
}

func TestParseVolumeFlag_HostPath(t *testing.T) {
	got, err := parseVolumeFlag([]string{"/home/user/data:/mnt/host"})
	if err != nil {
		t.Fatalf("parseVolumeFlag() unexpected error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(got))
	}
	if got[0].Type != "hostPath" || got[0].MountPath != "/mnt/host" || got[0].ReadOnly != true {
		t.Errorf("unexpected volume definition: %+v", got[0])
	}
}

func TestParseVolumeFlag_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		vStrs []string
	}{
		{
			name:  "Invalid mode",
			vStrs: []string{"gs://my-bucket:/mnt/gcs:invalid"},
		},
		{
			name:  "Duplicate source",
			vStrs: []string{"gs://my-bucket:/mnt/gcs1", "gs://my-bucket:/mnt/gcs2"},
		},
		{
			name:  "Duplicate destination",
			vStrs: []string{"gs://my-bucket1:/mnt/gcs", "gs://my-bucket2:/mnt/gcs"},
		},
		{
			name:  "Invalid Format (No separator)",
			vStrs: []string{"invalid-format"},
		},
		{
			name:  "Invalid Format (Missing destination for URI)",
			vStrs: []string{"gs://my-bucket"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseVolumeFlag(tt.vStrs)
			if err == nil {
				t.Fatalf("parseVolumeFlag() expected error, got nil")
			}
		})
	}
}

type mockOrchestrator struct {
	orchestrator.JobOrchestrator
}

func (m *mockOrchestrator) SubmitJob(job orchestrator.JobDefinition) error {
	if job.DryRunManifest != "" {
		var content string
		if job.IsPathwaysJob {
			content = "name: " + job.WorkloadName + "\nimage: proxy:latest\n--gcs_location=gs://my-bucket"
		} else {
			content = "name: " + job.WorkloadName + "\nimage: busybox"
		}
		return os.WriteFile(job.DryRunManifest, []byte(content), 0644)
	}
	return nil
}

type MockPrereqStore struct {
	State PrereqState
}

func (m *MockPrereqStore) Load() PrereqState {
	return m.State
}

func (m *MockPrereqStore) Save(state PrereqState) {
	m.State = state
}

func TestParseDurationToSeconds(t *testing.T) {
	tests := []struct {
		name    string
		dStr    string
		want    int
		wantErr bool
	}{
		{"Seconds", "30s", 30, false},
		{"Minutes", "5m", 300, false},
		{"Hours", "1h", 3600, false},
		{"Raw Integer", "3600", 3600, false},
		{"Invalid Format", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDurationToSeconds(tt.dStr, "--test-flag")
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDurationToSeconds() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseDurationToSeconds() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubmitCmd_MissingRepoEnvVar(t *testing.T) {
	resetSubmitCmdFlags()

	origRepo := os.Getenv("GCLUSTER_IMAGE_REPO")
	os.Setenv("GCLUSTER_IMAGE_REPO", "")
	defer os.Setenv("GCLUSTER_IMAGE_REPO", origRepo)

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{State: PrereqState{LastCheckedTimestamp: time.Now()}}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "fail-test",
		"--base-image", "python:3.9-slim",
		"--build-context", "job_details",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
	)

	if err == nil {
		t.Fatal("expected error for missing GCLUSTER_IMAGE_REPO, got nil")
	}

	if !strings.Contains(err.Error(), "GCLUSTER_IMAGE_REPO environment variable is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSubmitCmd_MissingUserEnvVar(t *testing.T) {
	resetSubmitCmdFlags()

	origUser := os.Getenv("USER")
	origUsername := os.Getenv("USERNAME")
	os.Setenv("USER", "")
	os.Setenv("USERNAME", "")
	defer os.Setenv("USER", origUser)
	defer os.Setenv("USERNAME", origUsername)

	origRepo := os.Getenv("GCLUSTER_IMAGE_REPO")
	os.Setenv("GCLUSTER_IMAGE_REPO", "gcluster")
	defer os.Setenv("GCLUSTER_IMAGE_REPO", origRepo)

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{State: PrereqState{LastCheckedTimestamp: time.Now()}}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "fail-test",
		"--base-image", "python:3.9-slim",
		"--build-context", "job_details",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
	)

	if err == nil {
		t.Fatal("expected error for missing user identity, got nil")
	}

	if !strings.Contains(err.Error(), "failed to determine user identity") {
		t.Errorf("unexpected error: %v", err)
	}
}
