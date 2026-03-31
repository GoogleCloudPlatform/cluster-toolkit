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
	// Create a temporary file for the dry-run output
	tmpfile, err := os.CreateTemp("", "pathways-manifest-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	os.Setenv("GCLUSTER_SKIP_PREREQ_CHECKS", "true")

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() (*gke.GKEOrchestrator, error) {
		g, err := gke.NewGKEOrchestrator()
		if err != nil {
			return nil, err
		}
		g.SetExecutor(&mockExecutorForTest{})
		return g, nil
	}

	// Reset flags before each test
	resetSubmitCmdFlags()

	// Execute the command
	output, err := executeCommand(JobCmd,
		"submit",
		"--pathways",
		"--name", "pathways-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--cluster-region", "us-central1-a",
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

	// Read the output file
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
	// Create a temporary file for the dry-run output
	tmpfile, err := os.CreateTemp("", "regular-manifest-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	os.Setenv("GCLUSTER_SKIP_PREREQ_CHECKS", "true")

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() (*gke.GKEOrchestrator, error) {
		g, err := gke.NewGKEOrchestrator()
		if err != nil {
			return nil, err
		}
		g.SetExecutor(&mockExecutorForTest{})
		return g, nil
	}

	// Reset flags before each test
	resetSubmitCmdFlags()

	// Execute the command
	output, err := executeCommand(JobCmd,
		"submit",
		"--name", "regular-test",
		"--image", "busybox",
		"--command", "echo hello",
		"--cluster", "test-cluster",
		"--cluster-region", "us-central1-a",
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
	clusterLocation = ""
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
	scheduler = ""
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
