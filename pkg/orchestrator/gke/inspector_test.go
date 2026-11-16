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

func TestInspectCluster_WASStandaloneWithoutTopologiesCRD(t *testing.T) {
	clusterName := "test-cluster-was"
	location := "us-central1-a"
	project := "test-project"
	reportPath := filepath.Join(t.TempDir(), "inspect-was.log")

	responses := defaultMockResponses(clusterName, location, project)
	// Simulate a standalone WAS cluster where Kueue topologies CRD is absent,
	// while Slices, PodGroups, and CompositePodGroups CRDs exist.
	delete(responses, "kubectl get crd topologies.kueue.x-k8s.io")
	responses["kubectl get crd slices.accelerator.gke.io"] = []shell.CommandResult{{ExitCode: 0, Stdout: "slices.accelerator.gke.io"}}
	responses["kubectl get crd compositepodgroups.scheduling.x-k8s.io"] = []shell.CommandResult{{ExitCode: 0, Stdout: "compositepodgroups.scheduling.x-k8s.io"}}
	responses["kubectl get crd podgroups.scheduling.x-k8s.io"] = []shell.CommandResult{{ExitCode: 0, Stdout: "podgroups.scheduling.x-k8s.io"}}

	responses["kubectl get nodes -l cloud.google.com/gke-tpu-accelerator"] = []shell.CommandResult{{
		ExitCode: 0,
		Stdout:   "NAME      READY   TAINTS           ACCELERATOR   TOPOLOGY   PARTITION_ID   PARTITION_STATE   SLICE                   SLICE_READINESS\ntpu-n-0   True    google.com/tpu   tpu7x         4x4x4      p4x4x4-1       ACTIVE            slice-test-workload-0   True",
	}}

	slicesYAML := `apiVersion: v1
kind: List
items:
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: slice-test-workload-0
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: CompositePodGroup/custom-namespace/test-workload
  spec:
    type: tpu7x
    topology: 4x4x4
    partitionIds:
    - p4x4x4-1
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: slice-other-workload-0
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: CompositePodGroup/custom-namespace/other-workload
`
	cpgYAML := `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.x-k8s.io/v1alpha1
  kind: CompositePodGroup
  metadata:
    name: test-workload
    namespace: custom-namespace
    annotations:
      tpuplugin.cloud.google.com/slice-name: slice-test-workload-0
    ownerReferences:
    - kind: JobSet
      name: test-workload
  spec:
    podGroupNames:
    - test-workload-worker-0
- apiVersion: scheduling.x-k8s.io/v1alpha1
  kind: CompositePodGroup
  metadata:
    name: other-workload
    namespace: custom-namespace
`
	pgYAML := `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.x-k8s.io/v1alpha1
  kind: PodGroup
  metadata:
    name: test-workload-worker-0
    namespace: custom-namespace
    ownerReferences:
    - kind: Job
      name: test-workload-worker-0
- apiVersion: scheduling.x-k8s.io/v1alpha1
  kind: PodGroup
  metadata:
    name: other-workload-worker-0
    namespace: custom-namespace
`

	responses["kubectl get slices.accelerator.gke.io -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: slicesYAML}}
	responses["kubectl get compositepodgroups -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: cpgYAML}}
	responses["kubectl get podgroups -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: pgYAML}}
	responses["kubectl describe compositepodgroups -n custom-namespace"] = []shell.CommandResult{{ExitCode: 0, Stdout: "cpg-describe-details"}}
	responses["kubectl describe podgroups -n custom-namespace"] = []shell.CommandResult{{ExitCode: 0, Stdout: "pg-describe-details"}}

	responses["kubectl logs deployment/slice-controller-controller-manager"] = []shell.CommandResult{
		{ExitCode: 0, Stdout: "slice-current-logs"},
		{ExitCode: 0, Stdout: "slice-previous-logs"},
	}

	responses["kubectl describe deployment tpu-scheduler -n kube-system"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-scheduler-dep-details"}}
	responses["kubectl logs deployment/tpu-scheduler"] = []shell.CommandResult{
		{ExitCode: 0, Stdout: "tpu-scheduler-current-logs"},
		{ExitCode: 0, Stdout: "tpu-scheduler-previous-logs"},
	}

	responses["kubectl describe deployment tpu-workload-controller-controller-manager -n tpu-workload-controller-system"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-wc-dep-details"}}
	responses["kubectl logs deployment/tpu-workload-controller-controller-manager"] = []shell.CommandResult{
		{ExitCode: 0, Stdout: "tpu-wc-current-logs"},
		{ExitCode: 0, Stdout: "tpu-wc-previous-logs"},
	}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project
	orc.kubeClient = &MockKubeClient{
		Namespace: "custom-namespace",
		Workloads: []string{"jobset-test-workload-abcde"},
	}

	err := orc.InspectCluster(orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		WorkloadName:    "test-workload",
		OutputPath:      reportPath,
	})
	if err != nil {
		t.Fatalf("InspectCluster() error = %v", err)
	}

	contentBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	content := string(contentBytes)

	expectedSubstrings := []string{
		"Kubectl: TPU Node Summary",
		"cloud.google.com/gke-tpu-accelerator",
		"tpu-n-0",
		"p4x4x4-1",
		"Slice Controller: Slices (YAML)",
		"slice-test-workload-0",
		"Slice Controller: Deployment Details",
		"slice-dep-details",
		"Slice Controller: Logs (tail 100)",
		"slice-current-logs",
		"Slice Controller: Previous Logs (tail 100)",
		"slice-previous-logs",
		"WAS: CompositePodGroups (YAML)",
		"WAS: PodGroups (YAML)",
		"WAS: TPU Scheduler Deployment Details",
		"tpu-scheduler-dep-details",
		"WAS: TPU Scheduler Logs (tail 100)",
		"tpu-scheduler-current-logs",
		"WAS: TPU Scheduler Previous Logs (tail 100)",
		"tpu-scheduler-previous-logs",
		"WAS: TPU Workload Controller Deployment Details",
		"tpu-wc-dep-details",
		"WAS: TPU Workload Controller Logs (tail 100)",
		"tpu-wc-current-logs",
		"WAS: TPU Workload Controller Previous Logs (tail 100)",
		"tpu-wc-previous-logs",
		"WAS: CompositePodGroups for test-workload",
		"cpg-describe-details",
		"WAS: PodGroups for test-workload",
		"pg-describe-details",
		"WAS: Owned Resources for test-workload",
	}
	for _, want := range expectedSubstrings {
		if !strings.Contains(content, want) {
			t.Errorf("expected report to contain %q, but it did not:\n%s", want, content)
		}
	}

	ownedIdx := strings.Index(content, "Description: WAS: Owned Resources for test-workload")
	if ownedIdx < 0 {
		t.Fatalf("missing owned resources section in:\n%s", content)
	}
	ownedSection := content[ownedIdx:]
	if nextSpacer := strings.Index(ownedSection, spacer); nextSpacer > 0 {
		ownedSection = ownedSection[:nextSpacer]
	}
	for _, wantOwned := range []string{"test-workload", "test-workload-worker-0", "slice-test-workload-0"} {
		if !strings.Contains(ownedSection, wantOwned) {
			t.Errorf("expected owned section to contain %q, got:\n%s", wantOwned, ownedSection)
		}
	}
	for _, unwanted := range []string{"other-workload", "other-workload-worker-0", "slice-other-workload-0"} {
		if strings.Contains(ownedSection, unwanted) {
			t.Errorf("expected owned section to exclude %q, got:\n%s", unwanted, ownedSection)
		}
	}
}

func TestInspectCluster_NoWASOrSliceCRDs(t *testing.T) {
	clusterName := "test-cluster-no-was"
	location := "us-central1-a"
	project := "test-project"
	reportPath := filepath.Join(t.TempDir(), "inspect-no-was.log")

	responses := defaultMockResponses(clusterName, location, project)
	// Remove topologies CRD so neither Kueue topologies nor WAS/Slice CRDs exist.
	delete(responses, "kubectl get crd topologies.kueue.x-k8s.io")

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project
	orc.kubeClient = &MockKubeClient{
		Namespace: "custom-namespace",
		Workloads: []string{"jobset-test-workload-abcde"},
	}

	err := orc.InspectCluster(orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		WorkloadName:    "test-workload",
		OutputPath:      reportPath,
	})
	if err != nil {
		t.Fatalf("InspectCluster() error = %v", err)
	}

	contentBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	content := string(contentBytes)

	unexpectedSections := []string{
		"Kubectl: TPU Node Summary",
		"Slice Controller: Slices (YAML)",
		"Slice Controller: Deployment Details",
		"Slice Controller: Logs (tail 100)",
		"Slice Controller: Previous Logs (tail 100)",
		"WAS: CompositePodGroups",
		"WAS: PodGroups",
		"WAS: TPU Scheduler",
		"WAS: TPU Workload Controller",
		"WAS: Owned Resources",
	}
	for _, unexpected := range unexpectedSections {
		if strings.Contains(content, unexpected) {
			t.Errorf("expected report without WAS/Slice CRDs to skip %q, but found it in:\n%s", unexpected, content)
		}
	}

	for _, call := range mockExec.calls {
		for _, forbiddenPrefix := range []string{
			"kubectl get slices.accelerator.gke.io",
			"kubectl get compositepodgroups",
			"kubectl get podgroups",
			"kubectl describe deployment slice-controller-controller-manager",
			"kubectl describe compositepodgroups",
			"kubectl describe podgroups",
		} {
			if strings.HasPrefix(call, forbiddenPrefix) {
				t.Errorf("unexpected kubectl call %q when WAS/Slice CRDs are absent", call)
			}
		}
	}
}

func TestInspectCluster_TPUSchedulerPodLabelFallback(t *testing.T) {
	clusterName := "test-cluster-pod-label"
	location := "us-central1-a"
	project := "test-project"
	reportPath := filepath.Join(t.TempDir(), "inspect-pod-label.log")

	responses := defaultMockResponses(clusterName, location, project)
	// Test k8s.io CRD group fallback and pod-label fallback for tpu-scheduler when deployment is absent.
	responses["kubectl get crd compositepodgroups.scheduling.k8s.io"] = []shell.CommandResult{{ExitCode: 0, Stdout: "cpg-k8s-crd"}}
	responses["kubectl get crd podgroups.scheduling.k8s.io"] = []shell.CommandResult{{ExitCode: 0, Stdout: "pg-k8s-crd"}}
	responses["kubectl get compositepodgroups -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}
	responses["kubectl get podgroups -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}
	responses["kubectl logs -l component=tpu-scheduler"] = []shell.CommandResult{
		{ExitCode: 0, Stdout: "tpu-scheduler-pod-current-logs"},
		{ExitCode: 1, Stderr: "previous terminated container not found"},
	}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project

	err := orc.InspectCluster(orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		OutputPath:      reportPath,
	})
	if err != nil {
		t.Fatalf("InspectCluster() error = %v", err)
	}

	contentBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "tpu-scheduler-pod-current-logs") {
		t.Errorf("expected fallback pod-label logs for tpu-scheduler, got:\n%s", content)
	}
	if strings.Contains(content, "previous terminated container not found") {
		t.Errorf("expected failed --previous logs to be skipped gracefully without error output, got:\n%s", content)
	}
}

func TestFilterOwnedWASResources_EdgeCases(t *testing.T) {
	wasState := inspectWASState{
		compositePodGroupsYAML: `apiVersion: scheduling.k8s.io/v1alpha3
kind: CompositePodGroup
metadata:
  name: custom-cpg
  namespace: ns-a
  labels:
    jobset.sigs.k8s.io/jobset-name: train-job
  annotations:
    tpuplugin.cloud.google.com/pod-group-names: leaf-pg-0, leaf-pg-1
    tpuplugin.cloud.google.com/slice-name: explicit-slice-0
`,
		podGroupsYAML: `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: leaf-pg-0
    namespace: ns-a
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: ownerref-pg
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: jobset-train-job-123-worker
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: wrong-ns-pg
    namespace: ns-b
    labels:
      jobset.sigs.k8s.io/jobset-name: train-job
`,
		slicesYAML: `apiVersion: v1
kind: List
items:
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: explicit-slice-0
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: two-part-ref-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: ns-a/leaf-pg-1
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: one-part-ref-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: train-job
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: wrong-ns-ref-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: PodGroup/ns-b/train-job
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: train-job-reused-by-other
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: PodGroup/ns-a/other-job
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: labeled-slice
    labels:
      tpuplugin.cloud.google.com/owner-namespace: ns-a
      tpuplugin.cloud.google.com/owner-name: leaf-pg-0
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: kueue-labeled-wrong-ns
    labels:
      kueue.gke.io/tpu-slice-used-by-namespace: ns-b
      kueue.gke.io/tpu-slice-used-by-workload: jobset-train-job-123
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: ownerref-slice
    ownerReferences:
    - kind: PodGroup
      name: leaf-pg-0
`,
	}

	cpgs, pgs, slices := filterOwnedWASResources("train-job", "ns-a", "jobset-train-job-123", wasState)
	if len(cpgs) != 1 || cpgs[0].Metadata.Name != "custom-cpg" {
		t.Errorf("cpgs = %+v, want [custom-cpg]", cpgs)
	}
	if len(pgs) != 2 || pgs[0].Metadata.Name != "leaf-pg-0" || pgs[1].Metadata.Name != "ownerref-pg" {
		t.Errorf("pgs = %+v, want [leaf-pg-0 ownerref-pg]", pgs)
	}
	var gotSlices []string
	for _, s := range slices {
		gotSlices = append(gotSlices, s.Metadata.Name)
	}
	wantSlices := []string{"explicit-slice-0", "two-part-ref-slice", "one-part-ref-slice", "labeled-slice", "ownerref-slice"}
	if strings.Join(gotSlices, ",") != strings.Join(wantSlices, ",") {
		t.Errorf("slices = %v, want %v", gotSlices, wantSlices)
	}

	if emptyCPGs, emptyPGs, emptySlices := filterOwnedWASResources("", "ns-a", "", wasState); emptyCPGs != nil || emptyPGs != nil || emptySlices != nil {
		t.Errorf("expected nil results for empty workloadName")
	}
	if items := parseWASResourceItems(":::invalid yaml"); items != nil {
		t.Errorf("expected nil for invalid YAML, got %+v", items)
	}
}
