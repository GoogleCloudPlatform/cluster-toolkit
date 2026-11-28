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
	"runtime"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
		"kubectl get --raw /apis/kueue.x-k8s.io":                          {{ExitCode: 0, Stdout: "{}"}},
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

func TestInspectCluster_KueueLessK8s(t *testing.T) {
	k8sNodesJSON := `{"items": [
		{"metadata":{"name":"tpu-0","labels":{"node.kubernetes.io/instance-type":"ct5p-hightpu-4t"}},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"tpu-1","labels":{"node.kubernetes.io/instance-type":"ct5p-hightpu-4t"}},"status":{"conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"ctrl-0","labels":{"node.kubernetes.io/instance-type":"n2-standard-8"}},"status":{"conditions":[{"type":"Ready","status":"True"}]}}
	]}`
	responses := map[string][]shell.CommandResult{
		"kubectl get --raw /version":                            {{ExitCode: 0, Stdout: "{}"}},
		"kubectl get --raw /apis/kueue.x-k8s.io":                {{ExitCode: 1, Stderr: `Error from server (NotFound): the server could not find the requested resource`}},
		"kubectl get nodes -o wide":                             {{ExitCode: 0, Stdout: "tpu-0 Ready\ntpu-1 Ready\nctrl-0 Ready"}},
		"kubectl get nodes -o json":                             {{ExitCode: 0, Stdout: k8sNodesJSON}},
		"kubectl describe Deployment jobset-controller-manager": {{ExitCode: 0, Stdout: "jobset-dep"}},
		"kubectl logs deployment/jobset-controller-manager":     {{ExitCode: 0, Stdout: "jobset-logs"}},
		"kubectl get jobsets":                                   {{ExitCode: 0, Stdout: "my-k8s-job"}},
		"kubectl get jobset my-k8s-job":                         {{ExitCode: 1, Stderr: "not an sps instance"}},
		"kubectl get jobs -l " + workloadLabel:                  {{ExitCode: 0, Stdout: "No resources found"}},
		"kubectl describe jobsets my-k8s-job":                   {{ExitCode: 0, Stdout: "k8s-jobset-desc"}},
		"kubectl get pods -l":                                   {{ExitCode: 0, Stdout: "my-k8s-job-0"}},
		"kubectl describe pods":                                 {{ExitCode: 0, Stdout: "k8s-pod-events"}},
	}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) {
		return &clientcmdapi.Config{
			CurrentContext: "k8s-ctx",
			Contexts: map[string]*clientcmdapi.Context{
				"k8s-ctx": {Cluster: "k8s-cluster", Namespace: "ml-team"},
			},
		}, nil
	}
	orc.kubeClient = &MockKubeClient{Namespace: "ml-team"}

	logFilePath := filepath.Join(t.TempDir(), "k8s-inspect.log")
	err := orc.InspectCluster(orchestrator.InspectOptions{
		ClusterName:  "k8s-cluster",
		WorkloadName: "my-k8s-job",
		OutputPath:   logFilePath,
	})
	if err != nil {
		t.Fatalf("InspectCluster(K8s) failed: %v", err)
	}

	assertK8sInspectCallsClean(t, mockExec)
	mockExec.assertAllKubectlPinned(t, "k8s-ctx")

	contentBytes, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("failed to read inspect log: %v", err)
	}
	assertK8sInspectOutput(t, string(contentBytes))

	// TypeK8s mode must stay non-GKE even when an explicit ProjectID is supplied.
	mockExecWithProj := NewMockExecutor(responses)
	orc.SetExecutor(mockExecWithProj)
	orc.mode = orchestrator.TypeK8s
	projLogPath := filepath.Join(t.TempDir(), "k8s-with-project.log")
	if err := orc.InspectCluster(orchestrator.InspectOptions{
		ClusterName:  "k8s-cluster",
		ProjectID:    "explicit-project",
		WorkloadName: "my-k8s-job",
		OutputPath:   projLogPath,
	}); err != nil {
		t.Fatalf("InspectCluster(TypeK8s with ProjectID) failed: %v", err)
	}
	assertK8sInspectCallsClean(t, mockExecWithProj)
	projBytes, err := os.ReadFile(projLogPath)
	if err != nil {
		t.Fatalf("failed to read inspect log: %v", err)
	}
	assertK8sInspectOutput(t, string(projBytes))
}

func assertK8sInspectCallsClean(t *testing.T, mockExec *MockExecutor) {
	t.Helper()
	forbidden := []string{
		"gcloud ",
		"configmap",
		"ClusterQueue",
		"LocalQueue",
		"ResourceFlavor",
		"kueue-controller-manager",
		"topologies.kueue.x-k8s.io",
		"get workloads",
		"describe workloads",
	}
	for _, call := range mockExec.calls {
		for _, bad := range forbidden {
			if strings.Contains(call, bad) {
				t.Errorf("Kueue-less K8s inspect ran unexpected command %q (matched %q)", call, bad)
			}
		}
	}
}

func assertK8sInspectOutput(t *testing.T, content string) {
	t.Helper()
	for _, want := range []string{
		"ct5p-hightpu-4t: 2 total",
		"ct5p-hightpu-4t: 2 healthy",
		"n2-standard-8: 1 total",
		"JobSet: All JobSets",
		"JobSet: Config for my-k8s-job",
		"Pods: Status for my-k8s-job",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("K8s inspect log missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"unknown:",
		"GKE: Cluster Details",
		"Cloud Console Links",
		"Kueue: ClusterQueue Details",
		"Error (1):",
	} {
		if strings.Contains(content, unwanted) {
			t.Errorf("K8s inspect log unexpectedly contained %q", unwanted)
		}
	}
}

func TestCountNodes_GroupingPrecedence(t *testing.T) {
	list := &kubernetesNodeList{
		Items: []kubernetesNode{
			{Metadata: kubernetesNodeMetadata{Labels: map[string]string{
				"cloud.google.com/gke-nodepool":    "gke-pool",
				"node.kubernetes.io/instance-type": "ignored-it",
				"beta.kubernetes.io/instance-type": "ignored-beta",
			}}, Status: kubernetesNodeStatus{Conditions: []kubernetesNodeCondition{{Type: "Ready", Status: "True"}}}},
			{Metadata: kubernetesNodeMetadata{Labels: map[string]string{
				"node.kubernetes.io/instance-type": "n2-standard-8",
				"beta.kubernetes.io/instance-type": "ignored-beta",
			}}, Status: kubernetesNodeStatus{Conditions: []kubernetesNodeCondition{{Type: "Ready", Status: "False"}}}},
			{Metadata: kubernetesNodeMetadata{Labels: map[string]string{
				"beta.kubernetes.io/instance-type": "legacy-node",
			}}},
			{Metadata: kubernetesNodeMetadata{Labels: map[string]string{}}},
		},
	}

	total, healthy := countNodes(list)
	for _, tc := range []struct {
		key         string
		wantTotal   int
		wantHealthy int
	}{
		{"gke-pool", 1, 1},
		{"n2-standard-8", 1, 0},
		{"legacy-node", 1, 0},
		{"unknown", 1, 0},
	} {
		if total[tc.key] != tc.wantTotal || healthy[tc.key] != tc.wantHealthy {
			t.Errorf("group %q = (%d total, %d healthy), want (%d, %d)",
				tc.key, total[tc.key], healthy[tc.key], tc.wantTotal, tc.wantHealthy)
		}
	}
}

func chdirUnwritableTempDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot remove the current working directory")
	}
	// Enter a temp dir, then remove it: creating "./<file>" now fails with ENOENT even as root.
	// chmod 0555 does not work here because root bypasses it, and `make tests` runs as root in Cloud Build.
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatalf("remove cwd: %v", err)
	}
	return dir
}

func TestOpenInspectOutput_UnwritableCWDFallback(t *testing.T) {
	unwritableDir := chdirUnwritableTempDir(t)

	t.Run("falls back to ~/.gcluster/logs when CWD is not writable", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)

		file, _, path, err := openInspectOutput(orchestrator.InspectOptions{ClusterName: "k8s-c"}, ".log")
		if err != nil {
			t.Fatalf("expected fallback to ~/.gcluster/logs, got err: %v", err)
		}
		info, statErr := file.Stat()
		_ = file.Close()
		if statErr != nil {
			t.Fatalf("stat fallback file: %v", statErr)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Errorf("~/.gcluster/logs fallback file mode = %v, want 0600", got)
		}
		wantPrefix := filepath.Join(homeDir, ".gcluster", "logs")
		if !strings.HasPrefix(path, wantPrefix) {
			t.Errorf("fallback path = %q, want prefix %q", path, wantPrefix)
		}
	})

	t.Run("falls back to os.TempDir with 0600 mode and sanitizes slashes in cluster name", func(t *testing.T) {
		// Point HOME at a regular file so MkdirAll(~/.gcluster/logs) fails cleanly.
		blockingFile := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(blockingFile, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", blockingFile)

		file, _, path, err := openInspectOutput(orchestrator.InspectOptions{ClusterName: "arn:aws:eks:us-east-1:123:cluster/prod"}, ".log")
		if err != nil {
			t.Fatalf("expected fallback to TempDir, got err: %v", err)
		}
		info, statErr := file.Stat()
		_ = file.Close()
		t.Cleanup(func() { _ = os.Remove(path) })
		if statErr != nil || info.Mode().Perm() != 0600 {
			t.Errorf("TempDir fallback file mode = %v (err=%v), want 0600", info.Mode().Perm(), statErr)
		}
		base := filepath.Base(path)
		if !strings.HasPrefix(path, os.TempDir()) || strings.Contains(base, "/") || !strings.Contains(base, "cluster-prod") {
			t.Errorf("TempDir fallback path = %q, want sanitized cluster segment under %q", path, os.TempDir())
		}
	})

	t.Run("explicit OutputPath is never silently redirected", func(t *testing.T) {
		explicitPath := filepath.Join(unwritableDir, "explicit.log")
		if _, _, _, err := openInspectOutput(orchestrator.InspectOptions{ClusterName: "k8s-c", OutputPath: explicitPath}, ".log"); err == nil {
			t.Fatalf("expected error for unwritable explicit OutputPath %q", explicitPath)
		}
	})
}

func TestConfigureK8sContextAndKueueFailOpen(t *testing.T) {
	t.Run("resolves current-context when cluster name is empty", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) {
			return &clientcmdapi.Config{
				CurrentContext: "cur-ctx",
				Contexts:       map[string]*clientcmdapi.Context{"cur-ctx": {Cluster: "c1"}},
			}, nil
		}
		if err := orc.configureK8sKubectl(""); err != nil || orc.kubeContext != "cur-ctx" {
			t.Fatalf("configureK8sKubectl(\"\") = (%q, %v), want (cur-ctx, nil)", orc.kubeContext, err)
		}
	})

	t.Run("rejects missing cluster and clears stale kubeContext while allowing matching pinned context", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) { return &clientcmdapi.Config{}, nil }
		orc.kubeContext = "pinned-ctx"
		if err := orc.configureK8sKubectl("missing"); err == nil || !strings.Contains(err.Error(), "missing") || orc.kubeContext != "" {
			t.Fatalf("expected missing-cluster error and cleared kubeContext, got (%q, %v)", orc.kubeContext, err)
		}

		orc.kubeContext = "pinned-ctx"
		if err := orc.configureK8sKubectl("pinned-ctx"); err != nil || orc.kubeContext != "pinned-ctx" {
			t.Fatalf("expected matching pinned context reuse, got (%q, %v)", orc.kubeContext, err)
		}
		if err := orc.configureK8sKubectl(""); err != nil || orc.kubeContext != "pinned-ctx" {
			t.Fatalf("expected empty cluster to reuse pinned context, got (%q, %v)", orc.kubeContext, err)
		}
	})

	t.Run("propagates kubeconfig load errors and fails open on Kueue probe errors", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get --raw /apis/kueue.x-k8s.io --request-timeout=10s": {{ExitCode: 1, Stderr: "Unable to connect to the server: timeout"}},
		}))
		orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) { return nil, os.ErrPermission }
		if err := orc.configureK8sKubectl("k8s-c"); err == nil || !strings.Contains(err.Error(), "failed to load kubeconfig") {
			t.Fatalf("expected kubeconfig load error to propagate, got %v", err)
		}
		if !orc.hasKueueInstalled() {
			t.Errorf("expected hasKueueInstalled() to fail open (true) on transient error")
		}
	})
}

func TestInspectWorkload_Tier3BatchJobFallback(t *testing.T) {
	jobJSON := `{"metadata":{"name":"t3-job","labels":{"gcluster.google.com/workload":"t3-job"}},"spec":{"parallelism":1},"status":{"active":1}}`
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl describe jobsets t3-job -n ml": {{ExitCode: 1, Stderr: `error: the server doesn't have a resource type "jobsets"`}},
		"kubectl get job t3-job -n ml -o json":  {{ExitCode: 0, Stdout: jobJSON}},
		"kubectl describe job t3-job -n ml":     {{ExitCode: 0, Stdout: "Name: t3-job"}},
		"kubectl describe service t3-job -n ml": {{ExitCode: 0, Stdout: "ClusterIP: None"}},
		"kubectl get pods -l gcluster.google.com/workload=t3-job,batch.kubernetes.io/job-name=t3-job -n ml -o wide": {{ExitCode: 0, Stdout: "t3-job-0 Running"}},
		"kubectl describe pods -l gcluster.google.com/workload=t3-job,batch.kubernetes.io/job-name=t3-job -n ml":    {{ExitCode: 0, Stdout: "Events: <none>"}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	var buf strings.Builder
	w := &inspectWriter{writer: &buf, orc: orc}
	orc.inspectWorkload(w, "t3-job", "ml", false, true)
	out := buf.String()
	for _, want := range []string{"Job: Config for t3-job", "Service: Config for t3-job", "gcluster.google.com/workload=t3-job,batch.kubernetes.io/job-name=t3-job"} {
		if !strings.Contains(out, want) {
			t.Errorf("inspectWorkload(Tier-3) missing %q in output:\n%s", want, out)
		}
	}
}

func TestInspector_HasJobSetGateAndUnqueuedWorkloadsList(t *testing.T) {
	t.Run("disable_jobset_skips_describe_jobsets_and_uses_job_prefix", func(t *testing.T) {
		jobJSON := `{"metadata":{"name":"t3-job","labels":{"gcluster.google.com/workload":"t3-job"}},"spec":{"parallelism":1},"status":{"active":1}}`
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get job t3-job -n ml -o json":  {{ExitCode: 0, Stdout: jobJSON}},
			"kubectl describe job t3-job -n ml":     {{ExitCode: 0, Stdout: "Name: t3-job"}},
			"kubectl describe service t3-job -n ml": {{ExitCode: 0, Stdout: "ClusterIP: None"}},
			"kubectl get pods -l gcluster.google.com/workload=t3-job,batch.kubernetes.io/job-name=t3-job -n ml -o wide": {{ExitCode: 0, Stdout: "t3-job-0 Running"}},
			"kubectl describe pods -l gcluster.google.com/workload=t3-job,batch.kubernetes.io/job-name=t3-job -n ml":    {{ExitCode: 0, Stdout: "Events: <none>"}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.kubeClient = nil
		orc.connectivity = &connectivityChecker{
			getenv: func(k string) string {
				if k == disableJobSetEnvVar {
					return "1"
				}
				return ""
			},
		}
		if orc.hasJobSetInstalled() {
			t.Fatalf("hasJobSetInstalled() = true when %s=1, want false", disableJobSetEnvVar)
		}
		orc.connectivity = &connectivityChecker{
			getenv: func(k string) string {
				if k == disableJobSetEnvVar || k == disableKueueEnvVar {
					return "1"
				}
				return ""
			},
		}
		if orc.hasKueueInstalled() {
			t.Fatalf("hasKueueInstalled() = true when %s=1, want false", disableKueueEnvVar)
		}
		var buf strings.Builder
		w := &inspectWriter{writer: &buf, orc: orc}
		orc.inspectWorkload(w, "t3-job", "ml", true, false)
		sawJobWorkload := false
		for _, call := range mockExec.calls {
			if strings.Contains(call, "describe jobsets") {
				t.Errorf("must not call describe jobsets when hasJobSet=false: %v", mockExec.calls)
			}
			if strings.Contains(call, "describe workloads job-t3-job -n ml") {
				sawJobWorkload = true
			}
		}
		if !sawJobWorkload {
			t.Errorf("expected describe workloads job-t3-job when hasKueue=true & hasJobSet=false; calls=%v", mockExec.calls)
		}
		if !strings.Contains(buf.String(), "Job: Config for t3-job") {
			t.Errorf("expected batch Job inspection output, got:\n%s", buf.String())
		}
	})

	t.Run("logClusterWorkloads_lists_standalone_jobs_even_when_hasKueue", func(t *testing.T) {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get workloads ": {{ExitCode: 0, Stdout: `{"items":[]}`}, {ExitCode: 0, Stdout: `{"items":[]}`}, {ExitCode: 0, Stdout: `{"items":[]}`}},
			"kubectl get jobs -l gcluster.google.com/workload,!jobset.sigs.k8s.io/jobset-name -n ml -o wide": {{ExitCode: 0, Stdout: "NAME\nt3-only\n"}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		var buf strings.Builder
		w := &inspectWriter{writer: &buf, orc: orc}
		orc.logClusterWorkloads(w, true, false, "ml")
		out := buf.String()
		if !strings.Contains(out, "t3-only") {
			t.Errorf("logClusterWorkloads(hasKueue=true, hasJobSet=false) must still list standalone batch Jobs; got:\n%s", out)
		}
	})
}

func TestInspector_MissingBatchJobWhenHasJobSetFalseKeepsJobSelectors(t *testing.T) {
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get job gone-job -n ml -o json": {{ExitCode: 1, Stderr: `Error from server (NotFound): jobs.batch "gone-job" not found`}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.kubeClient = nil
	var buf strings.Builder
	w := &inspectWriter{writer: &buf, orc: orc}
	orc.inspectWorkload(w, "gone-job", "ml", true, false)
	joined := strings.Join(mockExec.calls, "\n")
	if strings.Contains(joined, "jobset-gone-job") || strings.Contains(joined, "jobset.sigs.k8s.io/jobset-name") || strings.Contains(joined, "kind=JobSet") {
		t.Errorf("must not fall back to JobSet selectors/events when hasJobSet=false:\n%s", joined)
	}
	if !strings.Contains(joined, "describe workloads job-gone-job -n ml") || !strings.Contains(joined, "batch.kubernetes.io/job-name=gone-job") {
		t.Errorf("expected job- prefix and batch.kubernetes.io/job-name selector:\n%s", joined)
	}
}

func TestInspectCluster_JobSetNotFoundSkipsSPSJobSetProbe(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "inspect.log")
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl version --client":                {{ExitCode: 0, Stdout: "v1.32"}},
		"kubectl config current-context":          {{ExitCode: 0, Stdout: testContext}},
		"kubectl get --raw=/readyz":               {{ExitCode: 0, Stdout: "ok"}},
		"kubectl get --raw /apis/jobset.x-k8s.io": {{ExitCode: 1, Stderr: "Error from server (NotFound): not found"}},
		"kubectl get --raw /apis/kueue.x-k8s.io":  {{ExitCode: 1, Stderr: "Error from server (NotFound): not found"}},
		"kubectl get nodes -o wide":               {{ExitCode: 0, Stdout: "node-1"}},
		"kubectl describe nodes":                  {{ExitCode: 0, Stdout: "node-desc"}},
		"kubectl get jobs":                        {{ExitCode: 0, Stdout: "jobs-list"}},
		"kubectl get job my-job -n default":       {{ExitCode: 0, Stdout: `{"metadata":{"name":"my-job","labels":{"gcluster.google.com/workload":"my-job"}},"status":{"active":1}}`}},
		"kubectl describe job my-job -n default":  {{ExitCode: 0, Stdout: "job-desc"}},
		"kubectl describe service my-job":         {{ExitCode: 0, Stdout: "svc-desc"}},
		"kubectl get pods":                        {{ExitCode: 0, Stdout: "pods-list"}},
		"kubectl describe pods":                   {{ExitCode: 0, Stdout: "pods-desc"}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.mode = orchestrator.TypeK8s
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://10.0.0.1:6443", nil, nil, testContext))
	if err := orc.InspectCluster(orchestrator.InspectOptions{ClusterName: testContext, WorkloadName: "my-job", GKENamespace: "default", OutputPath: outFile}); err != nil {
		t.Fatalf("InspectCluster(JobSet NotFound) err = %v", err)
	}
	for _, call := range mockExec.calls {
		if strings.Contains(call, "get jobset ") || strings.Contains(call, "get jobsets") || strings.Contains(call, "describe jobsets") {
			t.Errorf("InspectCluster must not query jobsets when /apis/jobset.x-k8s.io is NotFound: %q", call)
		}
	}
}

func TestDescribeWorkloadResource_ForbiddenJobSetFallsBackToTier3Job(t *testing.T) {
	jobJSON := `{"metadata":{"name":"t3-rb","labels":{"gcluster.google.com/workload":"t3-rb"}},"spec":{"parallelism":1},"status":{"active":1}}`
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl describe jobsets t3-rb -n ml": {{ExitCode: 1, Stderr: `Error from server (Forbidden): jobsets.jobset.x-k8s.io "t3-rb" is forbidden`}},
		"kubectl get job t3-rb -n ml -o json":  {{ExitCode: 0, Stdout: jobJSON}},
		"kubectl describe job t3-rb -n ml":     {{ExitCode: 0, Stdout: "Name: t3-rb"}},
		"kubectl describe service t3-rb -n ml": {{ExitCode: 0, Stdout: "ClusterIP: None"}},
		"kubectl describe workloads job-t3-rb": {{ExitCode: 0, Stdout: "Workload: job-t3-rb"}},
		"kubectl get pods -l gcluster.google.com/workload=t3-rb,batch.kubernetes.io/job-name=t3-rb -n ml -o wide": {{ExitCode: 0, Stdout: "t3-rb-0 Running"}},
		"kubectl describe pods -l gcluster.google.com/workload=t3-rb,batch.kubernetes.io/job-name=t3-rb -n ml":    {{ExitCode: 0, Stdout: "Events: <none>"}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.kubeClient = nil
	var buf strings.Builder
	w := &inspectWriter{writer: &buf, orc: orc}
	orc.inspectWorkload(w, "t3-rb", "ml", true, true)
	out := buf.String()
	joined := strings.Join(mockExec.calls, "\n")
	if !strings.Contains(out, "is forbidden") {
		t.Errorf("expected 403 JobSet error logged verbatim, got:\n%s", out)
	}
	for _, want := range []string{"describe job t3-rb -n ml", "describe service t3-rb -n ml", "describe workloads job-t3-rb", "batch.kubernetes.io/job-name=t3-rb"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected fallback inspect call %q, calls:\n%s", want, joined)
		}
	}
}
