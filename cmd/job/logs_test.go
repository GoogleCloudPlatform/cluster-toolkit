// Copyright 2026 Google LLC
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
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"
	"strings"
	"testing"
)

type mockLogsExecutor struct{}

func (m *mockLogsExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	if name == "kubectl" && len(args) > 0 && args[0] == "logs" {
		return shell.CommandResult{ExitCode: 0, Stdout: "mock logs output"}
	}
	return shell.CommandResult{ExitCode: 0}
}

func (m *mockLogsExecutor) ExecuteCommandStream(name string, args ...string) error {
	return nil
}

func TestLogsCmd_Success(t *testing.T) {
	resetSubmitCmdFlags() // Reset shared flags

	// Mock the orchestrator factory
	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockLogsExecutor{})
		g.SetKubeClient(&mockKubeClient{namespace: "default"})
		return g
	}

	output, err := executeCommand(JobCmd, "logs", "test-job", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")

	if err != nil {
		if !strings.Contains(err.Error(), "unhandled mock command") &&
			!strings.Contains(err.Error(), "failed to get kubeconfig") &&
			!strings.Contains(err.Error(), "invalid configuration") &&
			!strings.Contains(err.Error(), "gke-gcloud-auth-plugin not found") {
			t.Fatalf("unexpected error: %v, output: %s", err, output)
		}
	}

	if !strings.Contains(output, "mock logs output") {
		t.Errorf("expected output to contain 'mock logs output', got %q", output)
	}
}
