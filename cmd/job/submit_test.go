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
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
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

func TestSubmitCmd_TPUWithNumNodes_Fails(t *testing.T) {
	resetSubmitCmdFlags()

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

	output, err := executeCommand(JobCmd,
		"submit",
		"--name", "tpu-fail-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--compute-type", "v6e-8", // TPU type
		"--num-nodes", "2", // Explicitly set!
	)

	if err == nil {
		t.Fatalf("expected error when passing --num-nodes for TPU job, but got nil")
	}

	expectedErr := "--num-nodes cannot be used with TPU jobs"
	if !strings.Contains(output, expectedErr) && !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error message to contain %q, got output: %q, err: %v", expectedErr, output, err)
	}
}

func TestSubmitCmd_LongWorkloadName_Fails(t *testing.T) {
	resetSubmitCmdFlags()

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{
		State: PrereqState{
			LastCheckedTimestamp: time.Now(),
		},
	}

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "a-very-long-workload-name-that-exceeds-28-characters",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--compute-type", "n2-standard-4",
	)

	if err == nil {
		t.Fatalf("expected error when passing long workload name, but got nil")
	}

	expectedErr := "cannot exceed 28 characters"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error message to contain %q, got: %v", expectedErr, err)
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
	gkeNapProvisioning = ""
	gkeNapReservation = ""
	envVars = nil
	pathwaysProxyEnv = nil
	pathwaysServerEnv = nil
	pathwaysWorkerEnv = nil
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

func TestSubmitCmd_InvalidGKENAPProvisioning(t *testing.T) {
	resetSubmitCmdFlags()

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

	output, err := executeCommand(JobCmd,
		"submit",
		"--name", "consumption-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--compute-type", "n2-standard-4",
		"--gke-nap-provisioning", "invalid-model",
	)

	if err == nil {
		t.Fatalf("expected error when passing invalid provisioning model, but got nil")
	}

	expectedErr := "invalid value \"invalid-model\" for --gke-nap-provisioning"
	if !strings.Contains(output, expectedErr) && !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error message to contain %q, got output: %q, err: %v", expectedErr, output, err)
	}
}

func TestSubmitCmd_ReservationModelWithoutName_Fails(t *testing.T) {
	resetSubmitCmdFlags()

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

	output, err := executeCommand(JobCmd,
		"submit",
		"--name", "consumption-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--compute-type", "n2-standard-4",
		"--gke-nap-provisioning", "reservation",
	)

	if err == nil {
		t.Fatalf("expected error when passing reservation model without reservation name, but got nil")
	}

	expectedErr := "--gke-nap-reservation is required when --gke-nap-provisioning=reservation"
	if !strings.Contains(output, expectedErr) && !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error message to contain %q, got output: %q, err: %v", expectedErr, output, err)
	}
}

func TestSubmitCmd_NonReservationModelWithName_Fails(t *testing.T) {
	resetSubmitCmdFlags()

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

	output, err := executeCommand(JobCmd,
		"submit",
		"--name", "consumption-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--compute-type", "n2-standard-4",
		"--gke-nap-provisioning", "spot",
		"--gke-nap-reservation", "my-reservation",
	)

	if err == nil {
		t.Fatalf("expected error when passing reservation name with spot model, but got nil")
	}

	expectedErr := "--gke-nap-reservation should only be provided when --gke-nap-provisioning=reservation"
	if !strings.Contains(output, expectedErr) && !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error message to contain %q, got output: %q, err: %v", expectedErr, output, err)
	}
}

func TestSubmitCmd_DryRunMissingDir_Approved(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gcluster-submit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	missingDirPath := filepath.Join(tmpDir, "non-existent-sub-dir", "manifest.yaml")

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{State: PrereqState{LastCheckedTimestamp: time.Now()}}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	oldPrompt := shell.PromptYesNo
	defer func() { shell.PromptYesNo = oldPrompt }()
	shell.PromptYesNo = func(prompt string) bool { return true }

	resetSubmitCmdFlags()

	_, err = executeCommand(JobCmd,
		"submit",
		"--name", "dry-run-missing-dir-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--dry-run-out", missingDirPath,
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(missingDirPath)); err != nil {
		t.Errorf("expected directory %q to exist, got error: %v", filepath.Dir(missingDirPath), err)
	}

	if _, err := os.Stat(missingDirPath); err != nil {
		t.Errorf("expected file %q to exist, got error: %v", missingDirPath, err)
	}
}

func TestSubmitCmd_DryRunMissingDir_Rejected(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gcluster-submit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	missingDirPath := filepath.Join(tmpDir, "non-existent-sub-dir", "manifest.yaml")

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{State: PrereqState{LastCheckedTimestamp: time.Now()}}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	oldPrompt := shell.PromptYesNo
	defer func() { shell.PromptYesNo = oldPrompt }()
	shell.PromptYesNo = func(prompt string) bool { return false }

	resetSubmitCmdFlags()

	output, err := executeCommand(JobCmd,
		"submit",
		"--name", "dry-run-missing-dir-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--dry-run-out", missingDirPath,
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedErr := "directory \"" + filepath.Dir(missingDirPath) + "\" does not exist"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error containing %q, got: %v, output: %s", expectedErr, err, output)
	}

	if _, err := os.Stat(filepath.Dir(missingDirPath)); !os.IsNotExist(err) {
		t.Errorf("expected directory %q to not exist, but it does", filepath.Dir(missingDirPath))
	}
}

func TestSubmitCmd_DryRunIsDir_Existing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gcluster-submit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{State: PrereqState{LastCheckedTimestamp: time.Now()}}

	resetSubmitCmdFlags()

	_, err = executeCommand(JobCmd,
		"submit",
		"--name", "dry-run-dir-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--dry-run-out", tmpDir,
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedErr := "must be a file path, not a directory path"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error containing %q, got: %v", expectedErr, err)
	}
}

func TestSubmitCmd_DryRunIsDir_TrailingSlash(t *testing.T) {
	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{State: PrereqState{LastCheckedTimestamp: time.Now()}}

	resetSubmitCmdFlags()

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "dry-run-dir-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--dry-run-out", "/some/random/dir/path/",
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedErr := "must be a file path, not a directory path"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error containing %q, got: %v", expectedErr, err)
	}
}

func TestSubmitCmd_ValidEnvVars(t *testing.T) {
	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{
		State: PrereqState{
			LastCheckedTimestamp: time.Now(),
			LastCheckedProjectID: "test-project",
			GCloudSDKInstalled:   true,
			GCloudAuthenticated:  true,
			ADCConfigured:        true,
			KubectlInstalled:     true,
		},
	}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	resetSubmitCmdFlags()

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "env-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--env", "MY_VAR=value",
		"--env", "ANOTHER_VAR=foo=bar", // handles multiple '='
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubmitCmd_InvalidEnvFormat_Fails(t *testing.T) {
	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{
		State: PrereqState{
			LastCheckedTimestamp: time.Now(),
			LastCheckedProjectID: "test-project",
			GCloudSDKInstalled:   true,
			GCloudAuthenticated:  true,
			ADCConfigured:        true,
			KubectlInstalled:     true,
		},
	}

	resetSubmitCmdFlags()

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "env-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--env", "INVALID_ENV",
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedErr := "invalid environment variable format"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error containing %q, got: %v", expectedErr, err)
	}
}

func TestSubmitCmd_PathwaysEnv_Success(t *testing.T) {
	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{
		State: PrereqState{
			LastCheckedTimestamp: time.Now(),
			LastCheckedProjectID: "test-project",
			GCloudSDKInstalled:   true,
			GCloudAuthenticated:  true,
			ADCConfigured:        true,
			KubectlInstalled:     true,
		},
	}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	resetSubmitCmdFlags()

	_, err := executeCommand(JobCmd,
		"submit",
		"--pathways",
		"--pathways-gcs-location", "gs://foo",
		"--name", "pathways-env-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--pathways-proxy-env", "PROXY_VAR=value",
		"--pathways-server-env", "SERVER_VAR=foo=bar",
		"--pathways-worker-env", "WORKER_VAR=baz",
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubmitCmd_PathwaysEnv_InvalidFormat_Fails(t *testing.T) {
	oldStore := store
	defer func() { store = oldStore }()
	store = &MockPrereqStore{
		State: PrereqState{
			LastCheckedTimestamp: time.Now(),
			LastCheckedProjectID: "test-project",
			GCloudSDKInstalled:   true,
			GCloudAuthenticated:  true,
			ADCConfigured:        true,
			KubectlInstalled:     true,
		},
	}

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{}
	}

	resetSubmitCmdFlags()

	_, err := executeCommand(JobCmd,
		"submit",
		"--pathways",
		"--pathways-gcs-location", "gs://foo",
		"--name", "pathways-env-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--compute-type", "n2-standard-4",
		"--cluster", "test-cluster",
		"--location", "test-location",
		"--project", "test-project",
		"--pathways-server-env", "INVALID_ENV",
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedErr := "invalid environment variable format"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error containing %q, got: %v", expectedErr, err)
	}
}

func TestSubmitCmd_InvalidEnvKey_Fails(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		expectedErr string
	}{
		{
			name:        "Starts with digit",
			env:         "1VAR=value",
			expectedErr: "invalid environment variable name",
		},
		{
			name:        "Empty key",
			env:         "=value",
			expectedErr: "invalid environment variable key",
		},
		{
			name:        "Special characters in key",
			env:         "MY-VAR=value",
			expectedErr: "invalid environment variable name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStore := store
			defer func() { store = oldStore }()
			store = &MockPrereqStore{
				State: PrereqState{
					LastCheckedTimestamp: time.Now(),
					LastCheckedProjectID: "test-project",
					GCloudSDKInstalled:   true,
					GCloudAuthenticated:  true,
					ADCConfigured:        true,
					KubectlInstalled:     true,
				},
			}

			resetSubmitCmdFlags()

			_, err := executeCommand(JobCmd,
				"submit",
				"--name", "env-test",
				"--image", "busybox",
				"--command", "echo hello",
				"--compute-type", "n2-standard-4",
				"--cluster", "test-cluster",
				"--location", "test-location",
				"--project", "test-project",
				"--env", tt.env,
			)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("expected error containing %q, got: %v", tt.expectedErr, err)
			}
		})
	}
}
