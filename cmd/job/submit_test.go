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
	dryRunManifest = ""
	clusterName = ""
	location = ""
	projectID = ""
	workloadName = ""
	kueueQueueName = ""
	numSlicesOrNodes = 1
	vmsPerSlice = 1
	maxRestarts = 1
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
	pathways = orchestrator.PathwaysJobDefinition{}
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
		{
			name:      "Invalid Format (Missing destination for URI)",
			vStrs:     []string{"gs://my-bucket"},
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
			got, err := parseDurationToSeconds(tt.dStr)
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
