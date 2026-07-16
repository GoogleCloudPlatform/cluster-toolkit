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

package gke

import (
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func defaultMockResponses(clusterName, location, project string) map[string][]shell.CommandResult {
	workloadsJSON := `{"items": [{"metadata":{"name":"jobset-test-workload","namespace":"custom-namespace","creationTimestamp":"2026-07-10T12:00:00Z","ownerReferences":[{"kind":"JobSet","name":"test-workload"}]},"spec":{"priorityClassName":"high-priority","podSets":[{"count":4}]},"status":{"admission":{"podSetAssignments":[{"count":4}]},"reclaimablePods":[{"count":0}],"conditions":[{"type":"Admitted","status":"True","message":"Admitted by ClusterQueue","lastTransitionTime":"2026-07-10T12:01:00Z"}]}}]}`
	workloadsResult := shell.CommandResult{ExitCode: 0, Stdout: workloadsJSON}

	return map[string][]shell.CommandResult{
		"gcloud container clusters get-credentials": {{ExitCode: 0}},
		"gcloud version":                                                  {{ExitCode: 0, Stdout: "Google Cloud SDK 400.0.0"}},
		"gcloud config list":                                              {{ExitCode: 0, Stdout: "project = test-project"}},
		"gcloud container clusters describe":                              {{ExitCode: 0, Stdout: "cluster-description"}},
		"gcloud container node-pools list":                                {{ExitCode: 0, Stdout: "node-pools"}},
		"kubectl get configmap":                                           {{ExitCode: 0, Stdout: "configmap-data"}},
		"kubectl get nodes -o wide":                                       {{ExitCode: 0, Stdout: "nodes-list"}},
		"kubectl get nodes -o json":                                       {{ExitCode: 0, Stdout: `{"items": [{"metadata":{"name":"node-1","labels":{"cloud.google.com/gke-nodepool":"pool-1"}},"status":{"conditions":[{"type":"Ready","status":"True"}]}}]}`}},
		"kubectl describe ClusterQueue":                                   {{ExitCode: 0, Stdout: "clusterqueue-details"}},
		"kubectl describe LocalQueue":                                     {{ExitCode: 0, Stdout: "localqueue-details"}},
		"kubectl describe ResourceFlavor":                                 {{ExitCode: 0, Stdout: "resourceflavor-details"}},
		"kubectl describe Deployment kueue-controller-manager":            {{ExitCode: 0, Stdout: "kueue-dep-details"}},
		"kubectl logs deployment/kueue-controller-manager":                {{ExitCode: 0, Stdout: "kueue-logs"}},
		"kubectl describe Deployment jobset-controller-manager":           {{ExitCode: 0, Stdout: "jobset-dep-details"}},
		"kubectl logs deployment/jobset-controller-manager":               {{ExitCode: 0, Stdout: "jobset-logs"}},
		"kubectl get crd topologies.kueue.x-k8s.io":                       {{ExitCode: 0, Stdout: "topologies-crd"}},
		"kubectl describe deployment slice-controller-controller-manager": {{ExitCode: 0, Stdout: "slice-dep-details"}},
		"kubectl logs deployment/slice-controller-controller-manager":     {{ExitCode: 0, Stdout: "slice-logs"}},
		"kubectl get workloads":                                           {workloadsResult, workloadsResult, workloadsResult, workloadsResult},
		"kubectl describe jobsets":                                        {{ExitCode: 0, Stdout: "jobset-config"}},
		"kubectl describe workloads":                                      {{ExitCode: 0, Stdout: "workload-config"}},
	}
}

func TestInspectCluster_Success(t *testing.T) {
	clusterName := "test-cluster-success"
	location := "us-central1-a"
	project := "test-project"

	t.Cleanup(func() {
		files, _ := filepath.Glob("gcluster-inspect-" + clusterName + "-*.log")
		for _, f := range files {
			_ = os.Remove(f)
		}
	})

	responses := defaultMockResponses(clusterName, location, project)
	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project
	orc.kubeClient = &MockKubeClient{
		Namespace: "custom-namespace",
		Workloads: []string{"jobset-test-workload-abcde"},
	}

	opts := orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		WorkloadName:    "test-workload",
		Show:            false,
	}

	err := orc.InspectCluster(opts)
	if err != nil {
		t.Fatalf("InspectCluster failed: %v", err)
	}

	// Verify log file was created
	files, err := filepath.Glob("gcluster-inspect-" + clusterName + "-*.log")
	if err != nil {
		t.Fatalf("failed to glob log files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 log file, found %d", len(files))
	}
	logFile := files[0]

	contentBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(contentBytes)

	// Verify expected output in log
	expectedSubstrings := []string{
		"Local Setup: gcloud version",
		"Google Cloud SDK 400.0.0",
		"GKE: Cluster Details",
		"cluster-description",
		"Node Pool Node Counts:",
		"pool-1: 1 total",
		"Healthy Node Counts Per Node Pool:",
		"pool-1: 1 healthy",
		"Kueue: ClusterQueue Details",
		"clusterqueue-details",
		"Kubectl: List Jobs with filter-by-status=EVERYTHING",
		"Kubectl: List Jobs with filter-by-status=QUEUED",
		"Kubectl: List Jobs with filter-by-status=RUNNING",
		"Kubectl: List Jobs with filter-by-status=EVERYTHING with filter-by-job=test-workload",
		"Jobset Name",
		"Created Time",
		"Priority",
		"TPU VMs Needed",
		"TPU VMs Running/Ran",
		"TPU VMs Done",
		"Status",
		"Status Message",
		"Status Time",
		"test-workload",
		"2026-07-10T12:00:00Z",
		"high-priority",
		"Admitted by ClusterQueue",
		"JobSet: Config for test-workload",
		"jobset-config",
		"Kueue: Workload config for test-workload",
		"workload-config",
		"Cloud Console Links",
		"https://console.cloud.google.com/kubernetes/clusters/details/us-central1-a/test-cluster-success/details?project=test-project",
		"https://console.cloud.google.com/kubernetes/workload/details/us-central1-a/test-cluster-success/custom-namespace/test-workload?project=test-project",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(content, sub) {
			t.Errorf("expected log file to contain %q, but it did not.", sub)
		}
	}
}

func TestInspectCluster_CommandFailure(t *testing.T) {
	clusterName := "test-cluster-failure"
	location := "us-central1-a"
	project := "test-project"

	t.Cleanup(func() {
		files, _ := filepath.Glob("gcluster-inspect-" + clusterName + "-*.log")
		for _, f := range files {
			_ = os.Remove(f)
		}
	})

	responses := defaultMockResponses(clusterName, location, project)
	// Make describe cluster fail
	responses["gcloud container clusters describe"] = []shell.CommandResult{{
		ExitCode: 1,
		Stderr:   "cluster not found",
	}}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project

	opts := orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		Show:            false,
	}

	err := orc.InspectCluster(opts)
	if err != nil {
		t.Fatalf("InspectCluster failed: %v", err)
	}

	files, err := filepath.Glob("gcluster-inspect-" + clusterName + "-*.log")
	if err != nil || len(files) != 1 {
		t.Fatalf("expected 1 log file")
	}
	logFile := files[0]

	contentBytes, _ := os.ReadFile(logFile)
	content := string(contentBytes)

	if !strings.Contains(content, "Error (1):") || !strings.Contains(content, "cluster not found") {
		t.Errorf("expected log file to contain the error message, but got:\n%s", content)
	}

	// Node counts should still be there because it continued
	if !strings.Contains(content, "Node Pool Node Counts:") {
		t.Errorf("expected node pool counts to be logged even after previous error")
	}
}

func TestInspectCluster_CustomOutputPath(t *testing.T) {
	clusterName := "test-cluster-custom-path"
	location := "us-central1-a"
	project := "test-project"
	customPath := filepath.Join(t.TempDir(), "my-custom-inspect.log")

	responses := defaultMockResponses(clusterName, location, project)
	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project

	opts := orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		OutputPath:      customPath,
		Show:            false,
	}

	err := orc.InspectCluster(opts)
	if err != nil {
		t.Fatalf("InspectCluster failed: %v", err)
	}

	// Verify custom file was created
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Fatalf("expected custom output file to exist at %s", customPath)
	}

	contentBytes, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("failed to read custom output file: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "Local Setup: gcloud version") {
		t.Errorf("expected custom log file to contain diagnostic logs, but it did not.")
	}
}
