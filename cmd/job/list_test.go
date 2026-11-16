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
	"strings"
	"testing"
)

func TestListWorkloadsCmd_Success(t *testing.T) {
	setupSubmitTestEnv(t) // Reset shared flags

	// Mock the orchestrator factory
	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockCancelExecutor{}) // Use the mock from cancel_test.go if available
		g.SetKubeClient(&mockKubeClient{namespace: "default"})
		return g
	}

	output, err := executeCommand(JobCmd, "list", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")

	if err != nil {
		if !strings.Contains(err.Error(), "unhandled mock command") &&
			!strings.Contains(err.Error(), "failed to get kubeconfig") &&
			!strings.Contains(err.Error(), "invalid configuration") &&
			!strings.Contains(err.Error(), "gke-gcloud-auth-plugin not found") {
			t.Fatalf("unexpected error: %v, output: %s", err, output)
		}
	}
}

func TestListWorkloadsCmd_InvalidStatus(t *testing.T) {
	setupSubmitTestEnv(t)

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockCancelExecutor{}) // Use the mock from cancel_test.go if available
		g.SetKubeClient(&mockKubeClient{namespace: "default"})
		return g
	}

	_, err := executeCommand(JobCmd, "list", "--status", "InvalidStatus", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")
	if err == nil {
		t.Fatalf("expected error for invalid status, got nil")
	}

	expectedErr := "invalid value for --status"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error to contain %q, got %v", expectedErr, err)
	}
}

type mockListJobOrchestrator struct {
	mockJobOrchestrator
	jobs     []orchestrator.JobStatus
	lastOpts orchestrator.ListOptions
}

func (m *mockListJobOrchestrator) ListJobs(opts orchestrator.ListOptions) ([]orchestrator.JobStatus, error) {
	m.lastOpts = opts
	var out []orchestrator.JobStatus
	for _, j := range m.jobs {
		if opts.Status != "" && !strings.EqualFold(j.Status, opts.Status) {
			continue
		}
		if opts.NameContains != "" && !strings.Contains(j.Name, opts.NameContains) {
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func TestListWorkloadsCmd_StatusPending(t *testing.T) {
	setupSubmitTestEnv(t)

	oldFactory := gkeOrchestratorFactory
	defer func() {
		gkeOrchestratorFactory = oldFactory
		filterStatus = ""
		filterName = ""
	}()

	mockOrc := &mockListJobOrchestrator{
		jobs: []orchestrator.JobStatus{
			{Name: "prebind-job", Status: "Pending", CreationTime: "2026-07-10T12:00:00Z"},
			{Name: "active-job", Status: "Running", CreationTime: "2026-07-10T12:05:00Z"},
			{Name: "queued-job", Status: "Suspended", CreationTime: "2026-07-10T12:10:00Z"},
		},
	}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return mockOrc
	}

	output, err := executeCommand(JobCmd, "list", "--status", "Pending", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockOrc.lastOpts.Status != "Pending" {
		t.Errorf("ListOptions.Status = %q, want %q", mockOrc.lastOpts.Status, "Pending")
	}
	if !strings.Contains(output, "prebind-job") || !strings.Contains(output, "Pending") {
		t.Errorf("expected output to contain prebind-job with Pending status, got:\n%s", output)
	}
	if strings.Contains(output, "active-job") || strings.Contains(output, "queued-job") {
		t.Errorf("expected output to filter out non-Pending jobs, got:\n%s", output)
	}
}
