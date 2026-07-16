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
	"testing"
)

type mockJobOrchestrator struct {
	inspectOpts   orchestrator.InspectOptions
	inspectErr    error
	inspectCalled bool
}

func (m *mockJobOrchestrator) SubmitJob(job orchestrator.JobDefinition) error { return nil }
func (m *mockJobOrchestrator) ListJobs(opts orchestrator.ListOptions) ([]orchestrator.JobStatus, error) {
	return nil, nil
}
func (m *mockJobOrchestrator) CancelJob(name string, opts orchestrator.CancelOptions) error {
	return nil
}
func (m *mockJobOrchestrator) GetJobLogs(name string, opts orchestrator.LogsOptions) (string, error) {
	return "", nil
}
func (m *mockJobOrchestrator) InspectCluster(opts orchestrator.InspectOptions) error {
	m.inspectCalled = true
	m.inspectOpts = opts
	return m.inspectErr
}

func TestInspectCmd_Success(t *testing.T) {
	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	mockOrc := &mockJobOrchestrator{}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return mockOrc
	}

	// We need to set them so that PersistentPreRunE passes.
	clusterName = "test-cluster"
	location = "us-central1-a"
	projectID = "test-project"
	inspectWorkloadName = ""
	inspectOutputPath = ""
	inspectShow = false
	defer func() {
		clusterName = ""
		location = ""
		projectID = ""
		inspectWorkloadName = ""
		inspectOutputPath = ""
		inspectShow = false
	}()

	_, err := executeCommand(JobCmd, "inspect", "--name", "test-workload", "--show", "--output", "/tmp/custom.log")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockOrc.inspectCalled {
		t.Errorf("expected InspectCluster to be called")
	}
	if mockOrc.inspectOpts.ClusterName != "test-cluster" {
		t.Errorf("expected ClusterName to be 'test-cluster', got %q", mockOrc.inspectOpts.ClusterName)
	}
	if mockOrc.inspectOpts.ClusterLocation != "us-central1-a" {
		t.Errorf("expected ClusterLocation to be 'us-central1-a', got %q", mockOrc.inspectOpts.ClusterLocation)
	}
	if mockOrc.inspectOpts.ProjectID != "test-project" {
		t.Errorf("expected ProjectID to be 'test-project', got %q", mockOrc.inspectOpts.ProjectID)
	}
	if mockOrc.inspectOpts.WorkloadName != "test-workload" {
		t.Errorf("expected WorkloadName to be 'test-workload', got %q", mockOrc.inspectOpts.WorkloadName)
	}
	if mockOrc.inspectOpts.OutputPath != "/tmp/custom.log" {
		t.Errorf("expected OutputPath to be '/tmp/custom.log', got %q", mockOrc.inspectOpts.OutputPath)
	}
	if !mockOrc.inspectOpts.Show {
		t.Errorf("expected Show to be true")
	}
}
