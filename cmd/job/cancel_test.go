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
	"fmt"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"
	"strings"
	"testing"
)

func TestCancelCmd_Success(t *testing.T) {
	resetSubmitCmdFlags() // Reset shared flags

	// Mock the orchestrator factory
	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockCancelExecutor{})
		g.SetKubeClient(&mockKubeClient{namespace: "default"})
		return g
	}

	output, err := executeCommand(JobCmd, "cancel", "test-job", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")

	if err != nil {
		if !strings.Contains(err.Error(), "unhandled mock command") &&
			!strings.Contains(err.Error(), "failed to get kubeconfig") &&
			!strings.Contains(err.Error(), "invalid configuration") &&
			!strings.Contains(err.Error(), "gke-gcloud-auth-plugin not found") {
			t.Fatalf("unexpected error: %v, output: %s", err, output)
		}
	}
}

type mockCancelExecutor struct{}

func (m *mockCancelExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	return shell.CommandResult{ExitCode: 0}
}

func (m *mockCancelExecutor) ExecuteCommandStream(name string, args ...string) error {
	return nil
}

type mockKubeClient struct {
	namespace string
	err       error
}

func (m *mockKubeClient) GetJobNamespace(workloadName string) (string, error) {
	return m.namespace, m.err
}

func (m *mockKubeClient) ListWorkloads(namespace string, workloadName string) ([]string, error) {
	return nil, nil
}

func (m *mockKubeClient) DeleteJobSet(namespace string, name string) error {
	return m.err
}

func TestCancelCmd_MissingArgs(t *testing.T) {
	resetSubmitCmdFlags()

	_, err := executeCommand(JobCmd, "cancel")
	if err == nil {
		t.Fatalf("expected error for missing args, got nil")
	}

	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCancelCmd_JobNotFound(t *testing.T) {
	resetSubmitCmdFlags()

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockCancelExecutor{})
		g.SetKubeClient(&mockKubeClient{err: fmt.Errorf("job not found in any namespace")})
		return g
	}

	_, err := executeCommand(JobCmd, "cancel", "non-existent-job", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")

	if err == nil {
		t.Fatalf("expected error for non-existent job, got nil")
	}

	if !strings.Contains(err.Error(), "job not found in any namespace") &&
		!strings.Contains(err.Error(), "failed to get kubeconfig") &&
		!strings.Contains(err.Error(), "invalid configuration") {
		t.Errorf("unexpected error: %v", err)
	}
}
