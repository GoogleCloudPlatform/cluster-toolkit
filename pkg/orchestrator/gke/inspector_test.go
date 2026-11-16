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
	"context"
	"encoding/json"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
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
		"kubectl get pods -l":                                             {{ExitCode: 0, Stdout: "pods-status"}},
		"kubectl describe pods":                                           {{ExitCode: 0, Stdout: "pods-config-events"}},
		"kubectl config set-context":                                      {{ExitCode: 0}},
	}
}

func TestInspectCluster_Success(t *testing.T) {
	clusterName := "test-cluster-success"
	location := "us-central1-a"
	project := "test-project"

	tempDir := t.TempDir()
	logFilePath := filepath.Join(tempDir, "gcluster-inspect-"+clusterName+".log")

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
		OutputPath:      logFilePath,
	}

	err := orc.InspectCluster(opts)
	if err != nil {
		t.Fatalf("InspectCluster failed: %v", err)
	}

	// Verify log file was created
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		t.Fatalf("expected log file to exist at %s", logFilePath)
	}
	logFile := logFilePath

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
		// The logged command must be the pinned one that actually ran.
		"Command: kubectl [--context gke_test-project_us-central1-a_test-cluster-success get nodes -o wide]",
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
		"Pods: Status for test-workload",
		"pods-status",
		"Pods: Config and Events for test-workload",
		"pods-config-events",
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

func TestInspectCluster_DefaultKubeClientNilDynClient(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent"))
	clusterName := "test-cluster-nil-dynclient"
	location := "us-central1"
	project := "test-project"
	customPath := filepath.Join(t.TempDir(), "nil-dynclient-inspect.log")

	responses := defaultMockResponses(clusterName, location, project)
	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project
	// Simulate the state where getCurrentNamespace sets orc.kubeClient = &DefaultKubeClient{dynClient: nil}
	// and dynClient is nil.
	orc.kubeClient = &DefaultKubeClient{dynClient: nil}

	opts := orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		GKENamespace:    "default",
		WorkloadName:    "test-workload",
		OutputPath:      customPath,
		Show:            false,
	}

	err := orc.InspectCluster(opts)
	if err != nil {
		t.Fatalf("InspectCluster failed unexpectedly: %v", err)
	}
}

func TestDefaultKubeClient_NilDynClient(t *testing.T) {
	client := &DefaultKubeClient{dynClient: nil}

	if _, err := client.ListWorkloads("default", "test-workload"); err == nil {
		t.Errorf("expected ListWorkloads to return error when dynClient is nil, got nil")
	}
	if err := client.DeleteJobSet("default", "test-workload"); err == nil {
		t.Errorf("expected DeleteJobSet to return error when dynClient is nil, got nil")
	}
	if _, err := client.ListJobSets("default", ""); err == nil {
		t.Errorf("expected ListJobSets to return error when dynClient is nil, got nil")
	}
}

func TestInspectCluster_DefaultKubeClientWithDynamicClient(t *testing.T) {
	clusterName := "test-cluster-dynclient"
	location := "us-central1"
	project := "test-project"
	customPath := filepath.Join(t.TempDir(), "dynclient-inspect.log")

	responses := defaultMockResponses(clusterName, location, project)
	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project
	// Simulate production state where kubeClient is nil and dynClient is provided.
	orc.kubeClient = nil
	orc.SetDynamicClient(&mockDynamicClient{
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			obj := &unstructured.Unstructured{}
			obj.SetUID(types.UID("jobset-uid-123"))
			return obj, nil
		},
		listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
			wl := unstructured.Unstructured{}
			wl.SetName("jobset-test-workload-abcde")
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{wl}}, nil
		},
	})

	opts := orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		GKENamespace:    "custom-namespace",
		WorkloadName:    "test-workload",
		OutputPath:      customPath,
		Show:            false,
	}

	err := orc.InspectCluster(opts)
	if err != nil {
		t.Fatalf("InspectCluster failed unexpectedly: %v", err)
	}

	contentBytes, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("failed to read custom output file: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "jobset-test-workload-abcde") {
		t.Errorf("expected log file to describe discovered workload 'jobset-test-workload-abcde', got:\n%s", content)
	}
}

// spsJobSetJSON builds a JobSet as `kubectl get jobset -o json` returns it, with the given
// head containers, worker job name and worker containers.
func spsJobSetJSON(headContainers, workerJob, workerContainers, status string) string {
	return `{"spec":{"replicatedJobs":[` +
		`{"name":"pathways-head","template":{"spec":{"template":{"spec":{"containers":[` + headContainers + `]}}}}},` +
		`{"name":"` + workerJob + `","template":{"spec":{"template":{"spec":{"initContainers":[` + workerContainers + `]}}}}}` +
		`]},"status":` + status + `}`
}

const (
	spsRMContainer      = `{"name":"pathways-rm","image":"server:jax-0.11.1","args":["--server_port=29001","--node_type=resource_manager","--instance_count=2","--instance_type=tpuv6e:2x4"]}`
	spsWorkerContainer  = `{"name":"pathways-worker","image":"us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20260901-jax_0.11.1"}`
	spsSidecarContainer = `{"name":"colocated-python-sidecar","image":"us-docker.pkg.dev/cloud-tpu-v2-images/pathways-colocated-python/sidecar:20260831-maxtext-v0.2.4-jax0.11.1-pwutils"}`
	spsRunningStatus    = `{"replicatedJobsStatus":[{"name":"pathways-head","ready":1},{"name":"worker","ready":2}]}`
)

func TestParseSPSInstance(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    spsInstanceSummary
		wantErr string
	}{
		{
			name: "gcluster instance with sidecar",
			json: spsJobSetJSON(spsRMContainer, "worker", spsWorkerContainer+","+spsSidecarContainer, spsRunningStatus),
			want: spsInstanceSummary{Name: "sps", Namespace: "ns", Status: "Running", InstanceType: "tpuv6e:2x4", InstanceCount: 2,
				ReadySlices:  "2/2",
				ServerImage:  "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20260901-jax_0.11.1",
				SidecarImage: "us-docker.pkg.dev/cloud-tpu-v2-images/pathways-colocated-python/sidecar:20260831-maxtext-v0.2.4-jax0.11.1-pwutils",
				SidecarName:  "colocated-python-sidecar",
				SidecarJAX:   "0.11.1"},
		},
		{
			name: "deploy script instance without sidecar",
			json: spsJobSetJSON(spsRMContainer, "pathways-worker", spsWorkerContainer, `{"conditions":[{"type":"Suspended","status":"True"}]}`),
			want: spsInstanceSummary{Name: "sps", Namespace: "ns", Status: "Suspended (waiting for Kueue admission)", InstanceType: "tpuv6e:2x4", InstanceCount: 2,
				ServerImage: "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20260901-jax_0.11.1"},
		},
		{
			name: "space-separated rm args and external-sidecar",
			json: spsJobSetJSON(
				`{"name":"pathways-rm","image":"server:jax-0.11.1","args":["--node_type","resource_manager","--instance_count","2","--instance_type","tpuv6e:2x4"]}`,
				"pathways-worker",
				spsWorkerContainer+`,{"name":"external-sidecar","image":"repo/sidecar:py3.12-jax0.11.1@sha256:abc"}`,
				`{"replicatedJobsStatus":[{"name":"pathways-head","ready":1},{"name":"pathways-worker","ready":1}]}`,
			),
			want: spsInstanceSummary{Name: "sps", Namespace: "ns", Status: "Running", InstanceType: "tpuv6e:2x4", InstanceCount: 2,
				ReadySlices:   "1/2",
				ServerImage:   "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20260901-jax_0.11.1",
				SidecarImage:  "repo/sidecar:py3.12-jax0.11.1@sha256:abc",
				SidecarName:   "external-sidecar",
				SidecarPython: "3.12",
				SidecarJAX:    "0.11.1"},
		},
		{
			name: "finished instance",
			json: spsJobSetJSON(spsRMContainer, "worker", spsWorkerContainer, `{"terminalState":"Failed","replicatedJobsStatus":[{"name":"pathways-head","ready":1},{"name":"worker","ready":2}]}`),
			want: spsInstanceSummary{Name: "sps", Namespace: "ns", Status: "Failed", InstanceType: "tpuv6e:2x4", InstanceCount: 2,
				ServerImage: "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20260901-jax_0.11.1"},
		},
		{
			name:    "headless pathways job",
			json:    spsJobSetJSON(`{"name":"pathways-proxy"},`+spsRMContainer, "worker", spsWorkerContainer, `{}`),
			wantErr: "runs pathways-proxy, so it is a regular or headless Pathways job",
		},
		{
			name:    "regular pathways job",
			json:    spsJobSetJSON(spsRMContainer+`,{"name":"workload-container"}`, "worker", spsWorkerContainer, `{}`),
			wantErr: "runs workload-container",
		},
		{
			name:    "pathwaysutils user-workload job",
			json:    spsJobSetJSON(spsRMContainer+`,{"name":"user-workload"}`, "pathways-worker", spsWorkerContainer, `{}`),
			wantErr: "runs user-workload",
		},
		{
			name:    "no resource manager",
			json:    spsJobSetJSON(`{"name":"other"}`, "worker", spsWorkerContainer, `{}`),
			wantErr: "does not run pathways-rm",
		},
		{
			name:    "no worker job",
			json:    `{"spec":{"replicatedJobs":[{"name":"main"}]}}`,
			wantErr: "needs a pathways-head job and a worker job",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSPSInstance("sps", "ns", []byte(tc.json))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseSPSInstance() error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSPSInstance() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("parseSPSInstance() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLastReadySlices(t *testing.T) {
	logs := "I1030 rm.cc] 0/2 Pathways Slices Now Ready\nother line\nI1030 rm.cc] 2/2 Pathways Slices Now Ready\n"
	if got := lastReadySlices(logs); got != "2/2" {
		t.Errorf("lastReadySlices() = %q, want 2/2", got)
	}
	if got := lastReadySlices("no readiness yet"); got != "" {
		t.Errorf("lastReadySlices() = %q, want empty", got)
	}
}

func TestImageVersions(t *testing.T) {
	tests := []struct {
		image, python, jax string
	}{
		{image: "us-docker.pkg.dev/p/pathways-colocated-python/sidecar:20260831-maxtext-v0.2.4-jax0.11.1-pwutils", jax: "0.11.1"},
		{image: "repo/sidecar:py3.12-jax_0.8.0", python: "3.12", jax: "0.8.0"},
		{image: "repo/sidecar:py3.12-jax0.11.1@sha256:abc", python: "3.12", jax: "0.11.1"},
		{image: "repo/sidecar:python3.11-jax-0.7", python: "3.11", jax: "0.7"},
		{image: "repo/sidecar:latest"},
		{image: "repo/sidecar@sha256:abc"},
		{image: "localhost:5000/jax0.9/sidecar"},
	}
	for _, tc := range tests {
		python, jax := imageVersions(tc.image)
		if python != tc.python || jax != tc.jax {
			t.Errorf("imageVersions(%q) = (%q, %q), want (%q, %q)", tc.image, python, jax, tc.python, tc.jax)
		}
	}
}

func TestFormatSPSSummary(t *testing.T) {
	withSidecar := formatSPSSummary(spsInstanceSummary{Name: "sps", Namespace: "default", Status: "Running", InstanceType: "tpuv6e:2x4", InstanceCount: 1,
		ReadySlices: "1/1", ServerImage: "server:x", SidecarImage: "sidecar:20260831-jax0.11.1", SidecarJAX: "0.11.1"})
	for _, want := range []string{
		"Name:            sps (namespace: default)",
		"Status:          Running",
		"Endpoint:        sps-pathways-head-0-0.sps:29001",
		"Slices:          1 x tpuv6e:2x4 (1/1 ready)",
		"Server image:    server:x",
		"Sidecar image:   sidecar:20260831-jax0.11.1",
		"Sidecar Python:  unknown (not in image tag)",
		"Sidecar JAX:     0.11.1",
	} {
		if !strings.Contains(withSidecar, want) {
			t.Errorf("summary missing %q:\n%s", want, withSidecar)
		}
	}
	noSidecar := formatSPSSummary(spsInstanceSummary{Name: "sps", Namespace: "default", Status: "Starting", InstanceType: "tpuv6e:2x4", InstanceCount: 1})
	if !strings.Contains(noSidecar, "Sidecar image:  none") {
		t.Errorf("summary missing \"Sidecar image:  none\":\n%s", noSidecar)
	}
	if !strings.Contains(noSidecar, "(readiness not logged yet)") || strings.Contains(noSidecar, "Sidecar JAX") {
		t.Errorf("summary without sidecar = \n%s", noSidecar)
	}
}

func TestInspectCluster_SPS(t *testing.T) {
	responses := defaultMockResponses("c", "us-central1-a", "p")
	responses["kubectl get jobset sps-v6e -n default -o json --request-timeout=10s"] = []shell.CommandResult{{ExitCode: 0,
		Stdout: spsJobSetJSON(spsRMContainer, "worker", spsWorkerContainer+","+spsSidecarContainer, spsRunningStatus)}}
	responses["kubectl logs -n default -l jobset.sigs.k8s.io/jobset-name=sps-v6e,jobset.sigs.k8s.io/replicatedjob-name=pathways-head -c pathways-rm --tail=-1 --request-timeout=10s"] = []shell.CommandResult{{ExitCode: 0,
		Stdout: "2/2 Pathways Slices Now Ready\n"}}
	responses["kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=sps-v6e,jobset.sigs.k8s.io/replicatedjob-name!=pathways-head --field-selector=status.phase=Running -o jsonpath={.items[*].metadata.name} --request-timeout=10s"] = []shell.CommandResult{{ExitCode: 0,
		Stdout: "sps-v6e-worker-0-0-bad sps-v6e-worker-0-1-def"}}
	responses["kubectl exec -n default pod/sps-v6e-worker-0-0-bad -c colocated-python-sidecar -- python3 -c "+spsSidecarVersionScript] = []shell.CommandResult{{ExitCode: 0,
		Stdout: "malformed output"}}
	responses["kubectl exec -n default pod/sps-v6e-worker-0-1-def -c colocated-python-sidecar -- python3 -c "+spsSidecarVersionScript] = []shell.CommandResult{{ExitCode: 0,
		Stdout: "warning: sitecustomize diagnostic\n3.12,0.11.1,0.11.1\n"}}
	orc := newTestGKEOrchestrator(NewMockExecutor(responses))
	orc.projectID = "p"

	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report-sps.log")
	err := orc.InspectCluster(orchestrator.InspectOptions{ProjectID: "p", ClusterName: "c", ClusterLocation: "us-central1-a",
		WorkloadName: "sps-v6e", Filter: "pathways-sps", OutputPath: reportPath})
	if err != nil {
		t.Fatalf("InspectCluster() error = %v", err)
	}
	contentBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read SPS report file %s: %v", reportPath, err)
	}
	out := string(contentBytes)
	for _, want := range []string{
		"sps-v6e (namespace: default)",
		"Endpoint:        sps-v6e-pathways-head-0-0.sps-v6e:29001",
		"2 x tpuv6e:2x4 (2/2 ready)",
		"Sidecar Python:  3.12",
		"Sidecar JAX:     0.11.1 (jaxlib 0.11.1)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--filter=pathways-sps output missing %q:\n%s", want, out)
		}
	}
}

func TestInspectCluster_SPSNotAnInstance(t *testing.T) {
	responses := defaultMockResponses("c", "us-central1-a", "p")
	responses["kubectl get jobset train -n default -o json --request-timeout=10s"] = []shell.CommandResult{{ExitCode: 0,
		Stdout: spsJobSetJSON(`{"name":"pathways-proxy"},`+spsRMContainer, "worker", spsWorkerContainer, `{}`)}}
	orc := newTestGKEOrchestrator(NewMockExecutor(responses))
	err := orc.InspectCluster(orchestrator.InspectOptions{ProjectID: "p", ClusterName: "c", ClusterLocation: "us-central1-a", WorkloadName: "train", Filter: "pathways-sps"})
	if err == nil || !strings.Contains(err.Error(), `JobSet "train" is not an SPS instance`) {
		t.Fatalf("InspectCluster() error = %v, want not-an-SPS-instance error", err)
	}
}

func TestInspectCluster_ReportStartsWithSPSSummary(t *testing.T) {
	responses := defaultMockResponses("c", "us-central1-a", "p")
	responses["kubectl get jobset sps-v6e -n default -o json --request-timeout=10s"] = []shell.CommandResult{{ExitCode: 0,
		Stdout: spsJobSetJSON(spsRMContainer, "worker", spsWorkerContainer, spsRunningStatus)}}
	orc := newTestGKEOrchestrator(NewMockExecutor(responses))
	orc.projectID = "p"
	orc.kubeClient = &MockKubeClient{Namespace: "default", Workloads: []string{"jobset-sps-v6e-abcde"}}
	reportPath := filepath.Join(t.TempDir(), "report.log")
	if err := orc.InspectCluster(orchestrator.InspectOptions{ProjectID: "p", ClusterName: "c", ClusterLocation: "us-central1-a",
		WorkloadName: "sps-v6e", OutputPath: reportPath}); err != nil {
		t.Fatalf("InspectCluster() error = %v", err)
	}
	content, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	summaryAt := strings.Index(string(content), "SPS instance summary:")
	gcloudAt := strings.Index(string(content), "Local Setup: gcloud version")
	if summaryAt < 0 || gcloudAt < 0 || summaryAt > gcloudAt {
		t.Errorf("report should start with the SPS summary (summary at %d, gcloud section at %d):\n%s", summaryAt, gcloudAt, content)
	}
}

func TestResolveWorkloadStatus_LifecyclePriorityAndAuxiliaryMessage(t *testing.T) {
	tests := []struct {
		name           string
		conditions     []kueueWorkloadCondition
		wantStatus     string
		wantMsgSubstrs []string
		wantTime       string
	}{
		{
			name: "Finished takes priority over Admitted and QuotaReserved",
			conditions: []kueueWorkloadCondition{
				{Type: "QuotaReserved", Status: "True", Message: "Quota reserved", LastTransitionTime: "2026-07-10T12:00:00Z"},
				{Type: "Admitted", Status: "True", Message: "Admitted", LastTransitionTime: "2026-07-10T12:01:00Z"},
				{Type: "PodsReady", Status: "True", Message: "All pods ready", LastTransitionTime: "2026-07-10T12:02:00Z"},
				{Type: "Finished", Status: "True", Message: "JobSet finished successfully", LastTransitionTime: "2026-07-10T12:30:00Z"},
			},
			wantStatus:     "Finished",
			wantMsgSubstrs: []string{"JobSet finished successfully"},
			wantTime:       "2026-07-10T12:30:00Z",
		},
		{
			name: "Evicted takes priority over trailing Requeued condition",
			conditions: []kueueWorkloadCondition{
				{Type: "QuotaReserved", Status: "False", Message: "Prev quota", LastTransitionTime: "2026-07-10T12:00:00Z"},
				{Type: "Evicted", Status: "True", Reason: "Preempted", Message: "Preempted by higher-priority workload", LastTransitionTime: "2026-07-10T12:10:00Z"},
				{Type: "Admitted", Status: "False", Reason: "Evicted", Message: "Workload evicted", LastTransitionTime: "2026-07-10T12:10:00Z"},
				{Type: "Requeued", Status: "True", Reason: "Reactivated", Message: "Workload was requeued", LastTransitionTime: "2026-07-10T12:11:00Z"},
			},
			wantStatus:     "Evicted",
			wantMsgSubstrs: []string{"Preempted by higher-priority workload", "Requeued=True (Reactivated): Workload was requeued"},
			wantTime:       "2026-07-10T12:10:00Z",
		},
		{
			name: "Admitted preserved when PodsReady=False is last",
			conditions: []kueueWorkloadCondition{
				{Type: "QuotaReserved", Status: "True", Message: "Quota reserved", LastTransitionTime: "2026-07-10T12:00:00Z"},
				{Type: "Admitted", Status: "True", Message: "Admitted by ClusterQueue", LastTransitionTime: "2026-07-10T12:01:00Z"},
				{Type: "PodsReady", Status: "False", Reason: "WaitForPodsReady", Message: "Not all pods are ready or succeeded", LastTransitionTime: "2026-07-10T12:02:00Z"},
			},
			wantStatus:     "Admitted",
			wantMsgSubstrs: []string{"Admitted by ClusterQueue", "PodsReady=False (WaitForPodsReady): Not all pods are ready or succeeded"},
			wantTime:       "2026-07-10T12:01:00Z",
		},
		{
			name: "Admitted preserved when TPUSliceFailure and TPUSlicesProvisioned are last",
			conditions: []kueueWorkloadCondition{
				{Type: "QuotaReserved", Status: "True", Message: "Quota reserved", LastTransitionTime: "2026-07-10T12:00:00Z"},
				{Type: "Admitted", Status: "True", Message: "The workload is admitted", LastTransitionTime: "2026-07-10T12:01:00Z"},
				{Type: "TPUSlicesProvisioned", Status: "False", Reason: "ProvisioningInProgress", Message: "Waiting for TPU Slice creation", LastTransitionTime: "2026-07-10T12:02:00Z"},
				{Type: "TPUSliceFailure", Status: "True", Reason: "SliceCreationFailed", Message: "GCE stockout in zone", LastTransitionTime: "2026-07-10T12:03:00Z"},
			},
			wantStatus: "Admitted",
			wantMsgSubstrs: []string{
				"The workload is admitted",
				"TPUSlicesProvisioned=False (ProvisioningInProgress): Waiting for TPU Slice creation",
				"TPUSliceFailure=True (SliceCreationFailed): GCE stockout in zone",
			},
			wantTime: "2026-07-10T12:01:00Z",
		},
		{
			name: "QuotaReserved=True preserved when Admitted=False is last",
			conditions: []kueueWorkloadCondition{
				{Type: "QuotaReserved", Status: "True", Message: "Quota reserved in ClusterQueue", LastTransitionTime: "2026-07-10T12:00:00Z"},
				{Type: "Admitted", Status: "False", Reason: "UnsatisfiedChecks", Message: "Waiting for admission checks", LastTransitionTime: "2026-07-10T12:01:00Z"},
			},
			wantStatus:     "QuotaReserved",
			wantMsgSubstrs: []string{"Quota reserved in ClusterQueue", "Admitted=False (UnsatisfiedChecks): Waiting for admission checks"},
			wantTime:       "2026-07-10T12:00:00Z",
		},
		{
			name: "QuotaReserved=False and Admitted=False resolves to QuotaReserved",
			conditions: []kueueWorkloadCondition{
				{Type: "QuotaReserved", Status: "False", Reason: "Pending", Message: "insufficient unused quota for google.com/tpu", LastTransitionTime: "2026-07-10T12:00:00Z"},
				{Type: "Admitted", Status: "False", Reason: "Pending", Message: "workload not admitted", LastTransitionTime: "2026-07-10T12:00:00Z"},
			},
			wantStatus:     "QuotaReserved",
			wantMsgSubstrs: []string{"insufficient unused quota", "Admitted=False (Pending): workload not admitted"},
			wantTime:       "2026-07-10T12:00:00Z",
		},
		{
			name: "Only Admitted=False resolves status to QuotaReserved, not Admitted",
			conditions: []kueueWorkloadCondition{
				{Type: "Admitted", Status: "False", Reason: "Pending", Message: "waiting for quota", LastTransitionTime: "2026-07-10T12:00:00Z"},
			},
			wantStatus:     "QuotaReserved",
			wantMsgSubstrs: []string{"waiting for quota"},
			wantTime:       "2026-07-10T12:00:00Z",
		},
		{
			name: "Only auxiliary conditions without lifecycle conditions resolves status to QuotaReserved",
			conditions: []kueueWorkloadCondition{
				{Type: "PodsReady", Status: "False", Reason: "WaitForPodsReady", Message: "pods pending", LastTransitionTime: "2026-07-10T12:02:00Z"},
				{Type: "TPUSliceFailure", Status: "True", Reason: "SliceFailed", Message: "slice error", LastTransitionTime: "2026-07-10T12:03:00Z"},
			},
			wantStatus:     "QuotaReserved",
			wantMsgSubstrs: []string{"slice error", "PodsReady=False (WaitForPodsReady): pods pending"},
			wantTime:       "2026-07-10T12:03:00Z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wl := kueueWorkload{}
			wl.Status.Conditions = tc.conditions
			gotStatus, gotMsg, gotTime := resolveWorkloadStatus(wl)
			if gotStatus != tc.wantStatus {
				t.Errorf("resolveWorkloadStatus() status = %q, want %q", gotStatus, tc.wantStatus)
			}
			for _, sub := range tc.wantMsgSubstrs {
				if !strings.Contains(gotMsg, sub) {
					t.Errorf("resolveWorkloadStatus() message = %q, missing %q", gotMsg, sub)
				}
			}
			if gotTime != tc.wantTime {
				t.Errorf("resolveWorkloadStatus() time = %q, want %q", gotTime, tc.wantTime)
			}
		})
	}
}

func TestFilterWorkloads_AuxiliaryConditionsAndPendingPods(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	makeWL := func(name string, admitted int, runningPods *int, conds ...kueueWorkloadCondition) kueueWorkload {
		wl := kueueWorkload{RunningPods: runningPods}
		wl.Metadata.Name = "jobset-" + name
		wl.Metadata.OwnerReferences = []kueueWorkloadOwnerRef{{Kind: "JobSet", Name: name}}
		wl.Spec.PodSets = []kueueWorkloadPodSet{{Name: "main-job", Count: admitted}}
		if admitted > 0 {
			wl.Status.Admission = &struct {
				PodSetAssignments []kueueWorkloadPodSet `json:"podSetAssignments"`
			}{
				PodSetAssignments: []kueueWorkloadPodSet{{Name: "main-job", Count: admitted}},
			}
		}
		wl.Status.Conditions = conds
		return wl
	}

	workloads := []kueueWorkload{
		// 1. Admitted in PreBind with 0 running pods and PodsReady=False as last condition -> QUEUED
		makeWL("prebind-pods-not-ready", 4, intPtr(0),
			kueueWorkloadCondition{Type: "QuotaReserved", Status: "True"},
			kueueWorkloadCondition{Type: "Admitted", Status: "True"},
			kueueWorkloadCondition{Type: "PodsReady", Status: "False", Reason: "WaitForPodsReady", Message: "Waiting for pods"},
		),
		// 2. Admitted with 0 running pods and TPUSliceFailure=True as last condition -> QUEUED
		makeWL("prebind-slice-failure", 4, intPtr(0),
			kueueWorkloadCondition{Type: "Admitted", Status: "True"},
			kueueWorkloadCondition{Type: "TPUSliceFailure", Status: "True", Message: "Slice creation failed"},
		),
		// 3. Evicted with Requeued=True and Preempted=True as trailing conditions -> QUEUED
		makeWL("evicted-requeued", 4, intPtr(0),
			kueueWorkloadCondition{Type: "Evicted", Status: "True", Message: "Preempted"},
			kueueWorkloadCondition{Type: "Preempted", Status: "True", Message: "Preempted in ClusterQueue"},
			kueueWorkloadCondition{Type: "Requeued", Status: "True", Message: "Requeued"},
		),
		// 4. QuotaReserved=True with Admitted=False as last condition (even with Admission set) -> QUEUED
		makeWL("quota-reserved-admitted-false", 4, nil,
			kueueWorkloadCondition{Type: "QuotaReserved", Status: "True"},
			kueueWorkloadCondition{Type: "Admitted", Status: "False", Message: "Waiting on check"},
		),
		// 5. Only Admitted=False -> QUEUED
		makeWL("only-admitted-false", 0, nil,
			kueueWorkloadCondition{Type: "Admitted", Status: "False", Message: "Pending quota"},
		),
		// 6. Admitted with running pods > 0 and PodsReady=True as last condition -> RUNNING
		makeWL("running-pods-ready", 4, intPtr(4),
			kueueWorkloadCondition{Type: "QuotaReserved", Status: "True"},
			kueueWorkloadCondition{Type: "Admitted", Status: "True"},
			kueueWorkloadCondition{Type: "PodsReady", Status: "True"},
		),
		// 7. Admitted with running pods > 0 and TPUSliceFailure=False / TPUSlicesProvisioned=True as last condition -> RUNNING
		makeWL("running-with-tpu-cond", 4, intPtr(2),
			kueueWorkloadCondition{Type: "Admitted", Status: "True"},
			kueueWorkloadCondition{Type: "TPUSlicesProvisioned", Status: "True"},
			kueueWorkloadCondition{Type: "TPUSliceFailure", Status: "False"},
		),
		// 8. Finished workload -> neither QUEUED nor RUNNING
		makeWL("finished-job", 4, intPtr(0),
			kueueWorkloadCondition{Type: "Admitted", Status: "True"},
			kueueWorkloadCondition{Type: "Finished", Status: "True"},
		),
	}

	queued := filterWorkloads(workloads, "QUEUED", "")
	var queuedNames []string
	for _, wl := range queued {
		queuedNames = append(queuedNames, workloadJobSetName(wl))
	}
	wantQueued := []string{
		"prebind-pods-not-ready",
		"prebind-slice-failure",
		"evicted-requeued",
		"quota-reserved-admitted-false",
		"only-admitted-false",
	}
	if strings.Join(queuedNames, ",") != strings.Join(wantQueued, ",") {
		t.Errorf("filterWorkloads(QUEUED) = %v, want %v", queuedNames, wantQueued)
	}

	running := filterWorkloads(workloads, "RUNNING", "")
	var runningNames []string
	for _, wl := range running {
		runningNames = append(runningNames, workloadJobSetName(wl))
	}
	wantRunning := []string{
		"running-pods-ready",
		"running-with-tpu-cond",
	}
	if strings.Join(runningNames, ",") != strings.Join(wantRunning, ",") {
		t.Errorf("filterWorkloads(RUNNING) = %v, want %v", runningNames, wantRunning)
	}

	everything := filterWorkloads(workloads, "EVERYTHING", "")
	if len(everything) != len(workloads) {
		t.Errorf("filterWorkloads(EVERYTHING) returned %d workloads, want %d", len(everything), len(workloads))
	}
}

func TestRenderWorkloadsTable_RunningPodsAndMultiPodSet(t *testing.T) {
	// 1. Admitted workload with 0 Running pods (all pods Pending) shows Running/Ran = 0.
	admittedZeroRunning := kueueWorkload{}
	admittedZeroRunning.Metadata.Name = "jobset-admitted-pending"
	admittedZeroRunning.Metadata.Namespace = "default"
	admittedZeroRunning.Metadata.CreationTimestamp = "2026-07-10T12:00:00Z"
	admittedZeroRunning.Metadata.OwnerReferences = []kueueWorkloadOwnerRef{{Kind: "JobSet", Name: "admitted-pending"}}
	admittedZeroRunning.Spec.PriorityClassName = "high"
	admittedZeroRunning.Spec.PodSets = []kueueWorkloadPodSet{{Name: "main-job", Count: 4}}
	admittedZeroRunning.Status.Admission = &struct {
		PodSetAssignments []kueueWorkloadPodSet `json:"podSetAssignments"`
	}{
		PodSetAssignments: []kueueWorkloadPodSet{{Name: "main-job", Count: 4}},
	}
	admittedZeroRunning.Status.Conditions = []kueueWorkloadCondition{
		{Type: "Admitted", Status: "True", Message: "Admitted by ClusterQueue", LastTransitionTime: "2026-07-10T12:01:00Z"},
		{Type: "PodsReady", Status: "False", Reason: "WaitForPodsReady", Message: "Waiting in PreBind", LastTransitionTime: "2026-07-10T12:01:05Z"},
	}

	// 2. Multi-PodSet workload summing Needed and Done across accelerator PodSets (excluding pathways-head).
	multiPodSet := kueueWorkload{}
	multiPodSet.Metadata.Name = "jobset-multi-podset"
	multiPodSet.Metadata.Namespace = "default"
	multiPodSet.Metadata.CreationTimestamp = "2026-07-10T12:05:00Z"
	multiPodSet.Metadata.OwnerReferences = []kueueWorkloadOwnerRef{{Kind: "JobSet", Name: "multi-podset"}}
	multiPodSet.Spec.PodSets = []kueueWorkloadPodSet{
		{Name: "pathways-head", Count: 1},
		{Name: "worker-0", Count: 4},
		{Name: "worker-1", Count: 8},
	}
	multiPodSet.Status.Admission = &struct {
		PodSetAssignments []kueueWorkloadPodSet `json:"podSetAssignments"`
	}{
		PodSetAssignments: []kueueWorkloadPodSet{
			{Name: "pathways-head", Count: 1},
			{Name: "worker-0", Count: 4},
			{Name: "worker-1", Count: 8},
		},
	}
	multiPodSet.Status.ReclaimablePods = []kueueWorkloadPodSet{
		{Name: "pathways-head", Count: 0},
		{Name: "worker-0", Count: 1},
		{Name: "worker-1", Count: 1},
	}
	multiPodSet.Status.Conditions = []kueueWorkloadCondition{
		{Type: "Admitted", Status: "True", Message: "Admitted", LastTransitionTime: "2026-07-10T12:06:00Z"},
	}

	items := []kueueWorkload{admittedZeroRunning, multiPodSet}
	pods := []inspectPod{
		// Pending pods for admitted-pending
		{
			Metadata: struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			}{
				Name:      "admitted-pending-main-job-0-0",
				Namespace: "default",
				Labels:    map[string]string{"jobset.sigs.k8s.io/jobset-name": "admitted-pending", "jobset.sigs.k8s.io/replicatedjob-name": "main-job"},
			},
			Status: struct {
				Phase string `json:"phase"`
			}{Phase: "Pending"},
		},
		// Running pathways-head pod (must be excluded from TPU VM count) + 3 Running/Succeeded worker pods + 1 Pending worker pod for multi-podset
		{
			Metadata: struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			}{
				Name:      "multi-podset-pathways-head-0-0",
				Namespace: "default",
				Labels:    map[string]string{"jobset.sigs.k8s.io/jobset-name": "multi-podset", "jobset.sigs.k8s.io/replicatedjob-name": "pathways-head"},
			},
			Status: struct {
				Phase string `json:"phase"`
			}{Phase: "Running"},
		},
		{
			Metadata: struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			}{
				Name:      "multi-podset-worker-0-0",
				Namespace: "default",
				Labels:    map[string]string{"jobset.sigs.k8s.io/jobset-name": "multi-podset", "jobset.sigs.k8s.io/replicatedjob-name": "worker-0"},
			},
			Status: struct {
				Phase string `json:"phase"`
			}{Phase: "Succeeded"},
		},
		{
			Metadata: struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			}{
				Name:      "multi-podset-worker-0-1",
				Namespace: "default",
				Labels:    map[string]string{"jobset.sigs.k8s.io/jobset-name": "multi-podset", "jobset.sigs.k8s.io/replicatedjob-name": "worker-0"},
			},
			Status: struct {
				Phase string `json:"phase"`
			}{Phase: "Running"},
		},
		{
			Metadata: struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			}{
				Name:      "multi-podset-worker-1-0",
				Namespace: "default",
				Labels:    map[string]string{"jobset.sigs.k8s.io/jobset-name": "multi-podset", "jobset.sigs.k8s.io/replicatedjob-name": "worker-1"},
			},
			Status: struct {
				Phase string `json:"phase"`
			}{Phase: "Running"},
		},
		{
			Metadata: struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			}{
				Name:      "multi-podset-worker-1-1",
				Namespace: "default",
				Labels:    map[string]string{"jobset.sigs.k8s.io/jobset-name": "multi-podset", "jobset.sigs.k8s.io/replicatedjob-name": "worker-1"},
			},
			Status: struct {
				Phase string `json:"phase"`
			}{Phase: "Pending"},
		},
	}

	applyPodCountsToWorkloads(items, pods)

	if gotNeeded := getWorkloadNeededCount(items[0]); gotNeeded != 4 {
		t.Errorf("admitted-pending Needed = %d, want 4", gotNeeded)
	}
	if gotRunning := getWorkloadRunningCount(items[0]); gotRunning != 0 {
		t.Errorf("admitted-pending Running/Ran = %d, want 0", gotRunning)
	}

	if gotNeeded := getWorkloadNeededCount(items[1]); gotNeeded != 12 {
		t.Errorf("multi-podset Needed = %d, want 12 (4 + 8 across worker PodSets)", gotNeeded)
	}
	if gotDone := getWorkloadDoneCount(items[1]); gotDone != 2 {
		t.Errorf("multi-podset Done = %d, want 2 (1 + 1 across worker PodSets)", gotDone)
	}
	if gotRunning := getWorkloadRunningCount(items[1]); gotRunning != 3 {
		t.Errorf("multi-podset Running/Ran = %d, want 3 (1 Succeeded + 2 Running worker pods, excluding pathways-head)", gotRunning)
	}

	var sb strings.Builder
	renderWorkloadsTable(&sb, items)
	out := sb.String()

	for _, wantLine := range []string{
		"admitted-pending  2026-07-10T12:00:00Z  high      4               0                    0             Admitted  Admitted by ClusterQueue [PodsReady=False (WaitForPodsReady): Waiting in PreBind]  2026-07-10T12:01:00Z",
		"multi-podset      2026-07-10T12:05:00Z  <none>    12              3                    2             Admitted  Admitted                                                                           2026-07-10T12:06:00Z",
	} {
		if !strings.Contains(out, wantLine) {
			t.Errorf("renderWorkloadsTable() output missing expected row:\nwant: %s\ngot:\n%s", wantLine, out)
		}
	}
}

func TestNonAcceleratorPodSets_ExplicitContainerResources(t *testing.T) {
	var wl kueueWorkload
	rawJSON := `{
		"spec": {
			"podSets": [
				{
					"name": "coordinator",
					"count": 1,
					"template": {
						"spec": {
							"containers": [
								{"name": "coord", "resources": {"requests": {"cpu": "4", "memory": "16Gi"}}}
							]
						}
					}
				},
				{
					"name": "tpu-slice-0",
					"count": 4,
					"template": {
						"spec": {
							"containers": [
								{"name": "worker", "resources": {"requests": {"google.com/tpu": "4"}}}
							]
						}
					}
				},
				{
					"name": "tpu-slice-1",
					"count": 4,
					"template": {
						"spec": {
							"containers": [
								{"name": "worker", "resources": {"limits": {"google.com/tpu": "4"}}}
							]
						}
					}
				}
			]
		},
		"status": {
			"reclaimablePods": [
				{"name": "coordinator", "count": 1},
				{"name": "tpu-slice-0", "count": 4},
				{"name": "tpu-slice-1", "count": 4}
			]
		}
	}`
	if err := json.Unmarshal([]byte(rawJSON), &wl); err != nil {
		t.Fatalf("failed to unmarshal workload: %v", err)
	}
	if got := getWorkloadNeededCount(wl); got != 8 {
		t.Errorf("getWorkloadNeededCount() = %d, want 8", got)
	}
	if got := getWorkloadDoneCount(wl); got != 8 {
		t.Errorf("getWorkloadDoneCount() = %d, want 8", got)
	}
}

func TestLogWorkloadList_PopulatesPodCountsFromKubectl(t *testing.T) {
	workloadsJSON := `{"items": [
		{
			"metadata": {"name": "jobset-prebind-job", "namespace": "default", "creationTimestamp": "2026-07-10T12:00:00Z", "ownerReferences": [{"kind": "JobSet", "name": "prebind-job"}]},
			"spec": {"priorityClassName": "high", "podSets": [{"name": "main-job", "count": 4}]},
			"status": {
				"admission": {"podSetAssignments": [{"name": "main-job", "count": 4}]},
				"conditions": [
					{"type": "QuotaReserved", "status": "True", "lastTransitionTime": "2026-07-10T12:00:10Z"},
					{"type": "Admitted", "status": "True", "message": "Admitted", "lastTransitionTime": "2026-07-10T12:00:10Z"},
					{"type": "PodsReady", "status": "False", "reason": "WaitForPodsReady", "message": "Waiting for Slices", "lastTransitionTime": "2026-07-10T12:00:11Z"}
				]
			}
		},
		{
			"metadata": {"name": "jobset-active-job", "namespace": "default", "creationTimestamp": "2026-07-10T12:00:00Z", "ownerReferences": [{"kind": "JobSet", "name": "active-job"}]},
			"spec": {"priorityClassName": "high", "podSets": [{"name": "main-job", "count": 2}]},
			"status": {
				"admission": {"podSetAssignments": [{"name": "main-job", "count": 2}]},
				"conditions": [
					{"type": "Admitted", "status": "True", "message": "Admitted", "lastTransitionTime": "2026-07-10T12:00:10Z"}
				]
			}
		}
	]}`
	podsJSON := `{"items": [
		{"metadata": {"name": "prebind-job-0", "namespace": "default", "labels": {"gcluster.google.com/workload": "prebind-job"}}, "status": {"phase": "Pending"}},
		{"metadata": {"name": "active-job-0", "namespace": "default", "labels": {"gcluster.google.com/workload": "active-job"}}, "status": {"phase": "Running"}},
		{"metadata": {"name": "active-job-1", "namespace": "default", "labels": {"gcluster.google.com/workload": "active-job"}}, "status": {"phase": "Running"}}
	]}`

	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get workloads -n default -o json": {
			{ExitCode: 0, Stdout: workloadsJSON},
			{ExitCode: 0, Stdout: workloadsJSON},
		},
		"kubectl get pods -n default -o json": {
			{ExitCode: 0, Stdout: podsJSON},
			{ExitCode: 0, Stdout: podsJSON},
		},
	})
	orc := newTestGKEOrchestrator(mockExec)

	var queuedBuf strings.Builder
	orc.logWorkloadList(&queuedBuf, "QUEUED", "", "default")
	queuedOut := queuedBuf.String()
	if !strings.Contains(queuedOut, "prebind-job") || strings.Contains(queuedOut, "active-job") {
		t.Errorf("QUEUED table should contain prebind-job and not active-job, got:\n%s", queuedOut)
	}

	var runningBuf strings.Builder
	orc.logWorkloadList(&runningBuf, "RUNNING", "", "default")
	runningOut := runningBuf.String()
	if !strings.Contains(runningOut, "active-job") || strings.Contains(runningOut, "prebind-job") {
		t.Errorf("RUNNING table should contain active-job and not prebind-job, got:\n%s", runningOut)
	}

	// Also verify all-namespaces (-A) path and malformed pod JSON fallback.
	mockExecAllNS := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get workloads -A -o json": {{ExitCode: 0, Stdout: workloadsJSON}},
		"kubectl get pods -A -o json":      {{ExitCode: 0, Stdout: "not-json"}},
	})
	orcAllNS := newTestGKEOrchestrator(mockExecAllNS)
	var allBuf strings.Builder
	orcAllNS.logWorkloadList(&allBuf, "EVERYTHING", "active-job", "")
	if !strings.Contains(allBuf.String(), "active-job") || strings.Contains(allBuf.String(), "prebind-job") {
		t.Errorf("filtered EVERYTHING table should contain only active-job, got:\n%s", allBuf.String())
	}
}

func TestWorkloadStatsAndPodMatchingEdgeCases(t *testing.T) {
	// 1. Workload without OwnerReferences falls back to metadata.name, andPodsReady=True without pod list uses admitted count.
	wl := kueueWorkload{}
	wl.Metadata.Name = "standalone-wl"
	wl.Spec.PodSets = []kueueWorkloadPodSet{
		{Name: "cpu-head", Count: 1, Flavors: map[string]string{"cpu": "default-flavor"}},
		{Name: "tpu-worker", Count: 4, Flavors: map[string]string{"google.com/tpu": "tpu-v6e"}},
	}
	wl.Status.Admission = &struct {
		PodSetAssignments []kueueWorkloadPodSet `json:"podSetAssignments"`
	}{
		PodSetAssignments: []kueueWorkloadPodSet{
			{Name: "cpu-head", Count: 1, ResourceUsage: map[string]string{"cpu": "2"}},
			{Name: "tpu-worker", Count: 4, ResourceUsage: map[string]string{"google.com/tpu": "16"}},
		},
	}
	wl.Status.Conditions = []kueueWorkloadCondition{
		{Type: "Admitted", Status: "True", Reason: "Admitted"},
		{Type: "PodsReady", Status: "True", Reason: "PodsReady"},
	}

	running, status := getWorkloadStats(wl)
	if status != "Admitted" || running != 4 {
		t.Errorf("getWorkloadStats(PodsReady=True without pod list) = (%d, %q), want (4, \"Admitted\")", running, status)
	}

	// 2. Finished workload falls back to ReclaimablePods or Admitted count when pods already garbage-collected.
	zero := 0
	finishedWL := wl
	finishedWL.RunningPods = &zero
	finishedWL.Status.ReclaimablePods = []kueueWorkloadPodSet{
		{Name: "cpu-head", Count: 1},
		{Name: "tpu-worker", Count: 4},
	}
	finishedWL.Status.Conditions = []kueueWorkloadCondition{
		{Type: "Admitted", Status: "True"},
		{Type: "Finished", Status: "True", Reason: "Succeeded"},
	}
	if got := getWorkloadRunningCount(finishedWL); got != 4 {
		t.Errorf("getWorkloadRunningCount(Finished with reclaimed pods) = %d, want 4", got)
	}

	finishedNoReclaim := wl
	finishedNoReclaim.RunningPods = &zero
	finishedNoReclaim.Status.Conditions = finishedWL.Status.Conditions
	if got := getWorkloadRunningCount(finishedNoReclaim); got != 4 {
		t.Errorf("getWorkloadRunningCount(Finished with RunningPods=&0 and no reclaimablePods) = %d, want 4", got)
	}

	finishedNilPods := wl
	finishedNilPods.Status.Conditions = finishedWL.Status.Conditions
	if got := getWorkloadRunningCount(finishedNilPods); got != 4 {
		t.Errorf("getWorkloadRunningCount(Finished with RunningPods=nil and no reclaimablePods) = %d, want 4", got)
	}

	one := 1
	admittedWithReclaimed := finishedWL
	admittedWithReclaimed.RunningPods = &one
	admittedWithReclaimed.Status.Conditions = []kueueWorkloadCondition{{Type: "Admitted", Status: "True"}}
	if got := getWorkloadRunningCount(admittedWithReclaimed); got != 4 {
		t.Errorf("getWorkloadRunningCount(Admitted with RunningPods=1 < Done=4) = %d, want 4", got)
	}

	// 3. NodeSelector accelerator detection and podMatchesWorkload namespace check.
	gpuPodSet := kueueWorkloadPodSet{
		Name:  "gpu-worker",
		Count: 2,
	}
	gpuPodSet.Template.Spec.NodeSelector = map[string]string{"cloud.google.com/gke-accelerator": "nvidia-h100-80gb"}
	if !podSetRequestsAccelerator(gpuPodSet, nil) {
		t.Errorf("expected gpuPodSet with cloud.google.com/gke-accelerator nodeSelector to be detected as accelerator")
	}
	tpuPodSet := kueueWorkloadPodSet{
		Name:  "tpu-worker",
		Count: 2,
	}
	tpuPodSet.Template.Spec.NodeSelector = map[string]string{"cloud.google.com/gke-tpu-accelerator": "tpu-v6e-slice"}
	if !podSetRequestsAccelerator(tpuPodSet, nil) {
		t.Errorf("expected tpuPodSet with cloud.google.com/gke-tpu-accelerator nodeSelector to be detected as accelerator")
	}

	podOtherNS := inspectPod{}
	podOtherNS.Metadata.Namespace = "other-ns"
	podOtherNS.Metadata.Labels = map[string]string{"gcluster.google.com/workload": "standalone-wl"}
	if podMatchesWorkload(podOtherNS, "standalone-wl", "default") {
		t.Errorf("expected podMatchesWorkload to reject pod from different namespace")
	}
	if podMatchesWorkload(podOtherNS, "", "other-ns") {
		t.Errorf("expected podMatchesWorkload to reject empty jobsetName")
	}
}
