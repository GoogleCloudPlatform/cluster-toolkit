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
	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"
	"os"
	"strings"
	"testing"

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

	os.Setenv("GCLUSTER_SKIP_PREREQ_CHECKS", "true")

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() *gke.GKEOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockExecutorForTest{})
		return g
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
		"--accelerator", "n2-standard-4",
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

	os.Setenv("GCLUSTER_SKIP_PREREQ_CHECKS", "true")

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() *gke.GKEOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockExecutorForTest{})
		return g
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
		"--accelerator", "n2-standard-4",
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
	acceleratorType = ""
	outputManifest = ""
	clusterName = ""
	location = ""
	projectID = ""
	workloadName = ""
	kueueQueueName = ""
	numSlicesOrNodes = 1
	vmsPerSlice = 1
	maxRestarts = 1
	ttlSecondsAfterFinished = 3600
	placementPolicy = ""
	nodeSelector = nil
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
	pathways = orchestrator.PathwaysJobDefinition{}
}

type mockExecutorForTest struct{}

func (m *mockExecutorForTest) ExecuteCommand(name string, args ...string) shell.CommandResult {
	if name == "gcloud" && len(args) > 3 && args[0] == "compute" && args[1] == "machine-types" && args[2] == "describe" {
		return shell.CommandResult{
			ExitCode: 0,
			Stdout:   `{"guestCpus": 4}`,
		}
	}
	return shell.CommandResult{ExitCode: 1, Stderr: "unhandled mock command"}
}

func (m *mockExecutorForTest) ExecuteCommandStream(name string, args ...string) error {
	return nil
}

func TestParseVolumeFlag(t *testing.T) {
	tests := []struct {
		name      string
		vStrs     []string
		wantCount int
		wantErr   bool
		checkFunc func([]orchestrator.VolumeDefinition) bool
	}{
		{
			name:      "Valid PVC (default)",
			vStrs:     []string{"my-pvc:/mnt/data"},
			wantCount: 1,
			wantErr:   false,
			checkFunc: func(v []orchestrator.VolumeDefinition) bool {
				return v[0].Type == "pvc" && v[0].MountPath == "/mnt/data"
			},
		},
		{
			name:      "Valid GCS Fuse",
			vStrs:     []string{"gs://my-bucket:/mnt/gcs"},
			wantCount: 1,
			wantErr:   false,
			checkFunc: func(v []orchestrator.VolumeDefinition) bool {
				return v[0].Type == "gcsfuse" && v[0].MountPath == "/mnt/gcs"
			},
		},
		{
			name:      "Valid Host Path",
			vStrs:     []string{"/home/user/data:/mnt/host"},
			wantCount: 1,
			wantErr:   false,
			checkFunc: func(v []orchestrator.VolumeDefinition) bool {
				return v[0].Type == "hostPath" && v[0].MountPath == "/mnt/host"
			},
		},
		{
			name:      "Invalid Format (No separator)",
			vStrs:     []string{"invalid-format"},
			wantCount: 0,
			wantErr:   true,
			checkFunc: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVolumeFlag(tt.vStrs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseVolumeFlag() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(got) != tt.wantCount {
					t.Errorf("parseVolumeFlag() got %v volumes, want %v", len(got), tt.wantCount)
				}
				if tt.checkFunc != nil && !tt.checkFunc(got) {
					t.Errorf("parseVolumeFlag() did not match assertions: %+v", got)
				}
			}
		})
	}
}
