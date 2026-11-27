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
		"kubectl api-resources --api-group=kueue.x-k8s.io -o name":        {{ExitCode: 0, Stdout: "clusterqueues.kueue.x-k8s.io\nlocalqueues.kueue.x-k8s.io\nresourceflavors.kueue.x-k8s.io\ntopologies.kueue.x-k8s.io\nworkloads.kueue.x-k8s.io\n"}},
		"kubectl api-resources --api-group=accelerator.gke.io -o name":    {{ExitCode: 0, Stdout: ""}},
		"kubectl api-resources --api-group=scheduling.k8s.io -o name":     {{ExitCode: 0, Stdout: "priorityclasses.scheduling.k8s.io\n"}},
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

func TestInspectCluster_StandaloneWASWithoutKueue(t *testing.T) {
	clusterName := "test-cluster-was-standalone"
	location := "us-central1-a"
	project := "test-project"
	reportPath := filepath.Join(t.TempDir(), "inspect-was-standalone.log")

	responses := defaultMockResponses(clusterName, location, project)
	// True Standalone WAS: no Kueue APIs installed; built-in scheduling.k8s.io PodGroup/CompositePodGroup
	// and accelerator.gke.io Slice APIs are exposed via api-resources.
	responses["kubectl api-resources --api-group=kueue.x-k8s.io -o name"] = []shell.CommandResult{{ExitCode: 0, Stdout: ""}}
	responses["kubectl api-resources --api-group=accelerator.gke.io -o name"] = []shell.CommandResult{{ExitCode: 0, Stdout: "slices.accelerator.gke.io\n"}}
	responses["kubectl api-resources --api-group=scheduling.k8s.io -o name"] = []shell.CommandResult{{ExitCode: 0, Stdout: "compositepodgroups.scheduling.k8s.io\npodgroups.scheduling.k8s.io\npriorityclasses.scheduling.k8s.io\n"}}

	// Simulate KCP-hosted slice-controller (no slice-controller-system deployment in user cluster).
	delete(responses, "kubectl describe deployment slice-controller-controller-manager")
	delete(responses, "kubectl logs deployment/slice-controller-controller-manager")
	responses["kubectl get configmap"] = []shell.CommandResult{
		{ExitCode: 0, Stdout: "metadata-configmap"},
		{ExitCode: 0, Stdout: "resources-configmap"},
	}

	// Include both a TPU node (with dynamic partition/slice labels and taints) and a non-TPU node.
	responses["kubectl get nodes -o json"] = []shell.CommandResult{{
		ExitCode: 0,
		Stdout: `{"items":[` +
			`{"metadata":{"name":"cpu-n-0","labels":{"cloud.google.com/gke-nodepool":"default-pool"}},"status":{"conditions":[{"type":"Ready","status":"True"}]}},` +
			`{"metadata":{"name":"tpu-n-0","labels":{"cloud.google.com/gke-nodepool":"tpu-pool","cloud.google.com/gke-tpu-accelerator":"tpu7x","cloud.google.com/gke-tpu-topology":"4x4x4","cloud.google.com/gce-topology-block":"block-1","cloud.google.com/gke-tpu-partition-4x4x4-id":"p4x4x4-1","cloud.google.com/gke-tpu-partition-4x4x4-state":"ACTIVE","cloud.google.com/gke-tpu-slice":"slice-test-workload-0","cloud.google.com/gke-tpu-slice-2x2x1-id":"s2x2x1-9","cloud.google.com/gke-tpu-slice-2x2x1-state":"READY"}},"spec":{"taints":[{"key":"google.com/tpu","value":"present","effect":"NoSchedule"}]},"status":{"conditions":[{"type":"Ready","status":"True"}]}}` +
			`]}`,
	}}

	slicesYAML := `apiVersion: v1
kind: List
items:
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: slice-test-workload-0
    deletionTimestamp: "2026-11-16T10:00:00Z"
    finalizers:
    - accelerator.gke.io/slice-finalizer
    managedFields:
    - manager: slice-controller
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: compositepodgroup/custom-namespace/test-workload
      kubectl.kubernetes.io/last-applied-configuration: '{"secret":"should-be-stripped"}'
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
      tpuplugin.cloud.google.com/slice-owner-reference: compositepodgroup/custom-namespace/other-workload
`
	cpgYAML := `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: test-workload
    namespace: custom-namespace
    managedFields:
    - manager: tpu-workload-controller
    annotations:
      tpuplugin.cloud.google.com/slice-name: slice-test-workload-0
      kubectl.kubernetes.io/last-applied-configuration: '{"cpg":"strip-me"}'
    ownerReferences:
    - kind: JobSet
      name: test-workload
  spec:
    workloadRef:
      workloadName: test-workload
      templateName: default
    schedulingPolicy:
      gang:
        minGroupCount: 1
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: test-workload-worker-0
    namespace: custom-namespace
    ownerReferences:
    - kind: Job
      name: test-workload-worker-0
  spec:
    parentCompositePodGroupName: test-workload
    workloadRef:
      workloadName: test-workload-worker-0
      templateName: default
    schedulingPolicy:
      gang:
        minGroupCount: 1
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: other-workload
    namespace: custom-namespace
    ownerReferences:
    - kind: JobSet
      name: other-workload
`
	pgYAML := `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: test-workload-worker-0-cube-0
    namespace: custom-namespace
    ownerReferences:
    - kind: Job
      name: test-workload-worker-0
  spec:
    parentCompositePodGroupName: test-workload-worker-0
    workloadRef:
      workloadName: test-workload-worker-0
      templateName: default
    schedulingPolicy:
      gang:
        minCount: 4
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: other-workload-worker-0-cube-0
    namespace: custom-namespace
    ownerReferences:
    - kind: Job
      name: other-workload-worker-0
  spec:
    parentCompositePodGroupName: other-workload
`
	workloadPodsJSON := `{"items":[{"metadata":{"name":"test-workload-worker-0-0","namespace":"custom-namespace","ownerReferences":[{"kind":"Job","name":"test-workload-worker-0"}]},"spec":{"schedulingGroup":{"podGroupName":"test-workload-worker-0-cube-0"}}}]}`

	responses["kubectl get slices.accelerator.gke.io -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: slicesYAML}}
	responses["kubectl get compositepodgroups.scheduling.k8s.io -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: cpgYAML}}
	responses["kubectl get podgroups.scheduling.k8s.io -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: pgYAML}}
	responses["kubectl get pods -n custom-namespace -l jobset.sigs.k8s.io/jobset-name=test-workload -o json"] = []shell.CommandResult{{ExitCode: 0, Stdout: workloadPodsJSON}}

	// Scoped describe commands must name only owned resources with group-qualified resource names.
	responses["kubectl describe compositepodgroups.scheduling.k8s.io test-workload test-workload-worker-0 -n custom-namespace"] = []shell.CommandResult{{ExitCode: 0, Stdout: "cpg-describe-details"}}
	responses["kubectl describe podgroups.scheduling.k8s.io test-workload-worker-0-cube-0 -n custom-namespace"] = []shell.CommandResult{{ExitCode: 0, Stdout: "pg-describe-details"}}
	responses["kubectl describe slices.accelerator.gke.io slice-test-workload-0"] = []shell.CommandResult{{ExitCode: 0, Stdout: "slice-describe-events"}}

	// Current tpu-scheduler deployment lives in tpu-scheduler-system.
	responses["kubectl get pods -A -l component=tpu-scheduler"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-scheduler-system\n"}}
	responses["kubectl describe deployment tpu-scheduler -n tpu-scheduler-system"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-scheduler-dep-details"}}
	responses["kubectl logs deployment/tpu-scheduler -n tpu-scheduler-system -c kube-scheduler --tail=500"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-scheduler-current-logs"}}
	responses["kubectl logs deployment/tpu-scheduler -n tpu-scheduler-system -c kube-scheduler --previous --tail=500"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-scheduler-previous-logs"}}

	responses["kubectl describe deployment tpu-workload-controller-controller-manager -n tpu-workload-controller-system"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-wc-dep-details"}}
	responses["kubectl logs deployment/tpu-workload-controller-controller-manager -n tpu-workload-controller-system -c manager --tail=500"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-wc-current-logs"}}
	responses["kubectl logs deployment/tpu-workload-controller-controller-manager -n tpu-workload-controller-system -c manager --previous --tail=500"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-wc-previous-logs"}}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project
	orc.kubeClient = &MockKubeClient{
		Namespace: "custom-namespace",
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
		"tpu-n-0",
		"google.com/tpu=present:NoSchedule",
		"block-1",
		"4x4x4:p4x4x4-1(ACTIVE)",
		"slice-test-workload-0,2x2x1:s2x2x1-9(READY)",
		"Slice Controller: Slices (YAML)",
		"WAS: CompositePodGroups (YAML)",
		"WAS: PodGroups (YAML)",
		"WAS: TPU Scheduler Deployment Details",
		"tpu-scheduler-dep-details",
		"WAS: TPU Scheduler Logs (tail 500)",
		"tpu-scheduler-current-logs",
		"WAS: TPU Scheduler Previous Logs (tail 500)",
		"tpu-scheduler-previous-logs",
		"WAS: TPU Workload Controller Deployment Details",
		"tpu-wc-dep-details",
		"WAS: TPU Workload Controller Logs (tail 500)",
		"tpu-wc-current-logs",
		"WAS: TPU Workload Controller Previous Logs (tail 500)",
		"tpu-wc-previous-logs",
		"WAS: CompositePodGroups for test-workload",
		"cpg-describe-details",
		"WAS: PodGroups for test-workload",
		"pg-describe-details",
		"WAS: Slices for test-workload",
		"slice-describe-events",
		"WAS: Owned Resources for test-workload",
	}
	for _, want := range expectedSubstrings {
		if !strings.Contains(content, want) {
			t.Errorf("expected report to contain %q, but it did not:\n%s", want, content)
		}
	}

	// Verify KCP-hosted slice-controller emits no error noise, non-TPU nodes aren't in TPU table,
	// and managedFields / last-applied-configuration are stripped from all YAML dumps.
	for _, unwanted := range []string{
		"Slice Controller: Deployment Details",
		"Error (1):",
		"managedFields",
		"last-applied-configuration",
		"should-be-stripped",
		"strip-me",
		"Kueue: ClusterQueue Details",
		"Kueue: Workload config",
	} {
		if strings.Contains(content, unwanted) {
			t.Errorf("expected Standalone WAS report to exclude %q, got:\n%s", unwanted, content)
		}
	}

	// Verify zero Kueue calls, zero CRD probing for built-in scheduling.k8s.io, and single get nodes -o json call.
	getNodesJSONCount := 0
	for _, call := range mockExec.calls {
		if call == "kubectl get nodes -o json" {
			getNodesJSONCount++
		}
		for _, forbiddenPrefix := range []string{
			"kubectl get crd",
			"kubectl describe ClusterQueue",
			"kubectl describe LocalQueue",
			"kubectl describe ResourceFlavor",
			"kubectl describe Deployment kueue-controller-manager",
			"kubectl logs deployment/kueue-controller-manager",
			"kubectl get workloads",
			"kubectl describe workloads",
			"kubectl get nodes -l cloud.google.com/gke-tpu-accelerator",
		} {
			if strings.HasPrefix(call, forbiddenPrefix) {
				t.Errorf("unexpected kubectl call %q on Standalone WAS cluster", call)
			}
		}
	}
	if getNodesJSONCount != 1 {
		t.Errorf("expected kubectl get nodes -o json to be called once and reused, got %d", getNodesJSONCount)
	}

	ownedIdx := strings.Index(content, "Description: WAS: Owned Resources for test-workload")
	if ownedIdx < 0 {
		t.Fatalf("missing owned resources section in:\n%s", content)
	}
	ownedSection := content[ownedIdx:]
	if nextSpacer := strings.Index(ownedSection, spacer); nextSpacer > 0 {
		ownedSection = ownedSection[:nextSpacer]
	}
	for _, wantOwned := range []string{
		"test-workload",
		"test-workload-worker-0",
		"test-workload-worker-0-cube-0",
		"slice-test-workload-0",
		"minCount: 4",
		"minGroupCount: 1",
		"deletionTimestamp: \"2026-11-16T10:00:00Z\"",
		"accelerator.gke.io/slice-finalizer",
	} {
		if !strings.Contains(ownedSection, wantOwned) {
			t.Errorf("expected owned section to contain %q, got:\n%s", wantOwned, ownedSection)
		}
	}
	for _, unwanted := range []string{"other-workload", "other-workload-worker-0-cube-0", "slice-other-workload-0", "spec: {}"} {
		if strings.Contains(ownedSection, unwanted) {
			t.Errorf("expected owned section to exclude %q, got:\n%s", unwanted, ownedSection)
		}
	}
}

func TestInspectCluster_KueuePlusWAS(t *testing.T) {
	clusterName := "test-cluster-kueue-was"
	location := "us-central1-a"
	project := "test-project"
	reportPath := filepath.Join(t.TempDir(), "inspect-kueue-was.log")

	responses := defaultMockResponses(clusterName, location, project)
	responses["kubectl api-resources --api-group=accelerator.gke.io -o name"] = []shell.CommandResult{{ExitCode: 0, Stdout: "slices.accelerator.gke.io\n"}}
	responses["kubectl api-resources --api-group=scheduling.k8s.io -o name"] = []shell.CommandResult{{ExitCode: 0, Stdout: "compositepodgroups.scheduling.k8s.io\npodgroups.scheduling.k8s.io\n"}}
	responses["kubectl get slices.accelerator.gke.io -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}
	responses["kubectl get compositepodgroups.scheduling.k8s.io -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}
	responses["kubectl get podgroups.scheduling.k8s.io -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}
	responses["kubectl logs deployment/slice-controller-controller-manager"] = []shell.CommandResult{
		{ExitCode: 0, Stdout: "slice-current-logs"},
		{ExitCode: 0, Stdout: "slice-previous-logs"},
	}
	// Legacy kube-system tpu-scheduler deployment discovered via component=tpu-scheduler label.
	responses["kubectl get pods -A -l component=tpu-scheduler"] = []shell.CommandResult{{ExitCode: 0, Stdout: "kube-system\n"}}
	responses["kubectl describe deployment tpu-scheduler -n kube-system"] = []shell.CommandResult{{ExitCode: 0, Stdout: "kube-sys-tpu-sched-dep"}}
	responses["kubectl logs deployment/tpu-scheduler -n kube-system -c kube-scheduler --tail=500"] = []shell.CommandResult{{ExitCode: 0, Stdout: "kube-sys-tpu-sched-logs"}}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = project
	orc.kubeClient = &MockKubeClient{
		Namespace: "custom-namespace",
		Workloads: []string{"jobset-test-workload-abcde"},
	}

	if err := orc.InspectCluster(orchestrator.InspectOptions{
		ProjectID:       project,
		ClusterName:     clusterName,
		ClusterLocation: location,
		WorkloadName:    "test-workload",
		OutputPath:      reportPath,
	}); err != nil {
		t.Fatalf("InspectCluster() error = %v", err)
	}

	contentBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	content := string(contentBytes)
	for _, want := range []string{
		"Kueue: ClusterQueue Details",
		"Kueue: Workload config for test-workload",
		"Slice Controller: Deployment Details",
		"slice-dep-details",
		"Slice Controller: Logs (tail 500)",
		"slice-current-logs",
		"Slice Controller: Previous Logs (tail 500)",
		"slice-previous-logs",
		"kube-sys-tpu-sched-dep",
		"kube-sys-tpu-sched-logs",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected Kueue+WAS report to contain %q, got:\n%s", want, content)
		}
	}
}

func TestInspectCluster_NoWASOrSliceCRDs(t *testing.T) {
	clusterName := "test-cluster-no-was"
	location := "us-central1-a"
	project := "test-project"
	reportPath := filepath.Join(t.TempDir(), "inspect-no-was.log")

	responses := defaultMockResponses(clusterName, location, project)
	// Kueue without topologies, no accelerator.gke.io Slices, and only PriorityClasses in scheduling.k8s.io.
	responses["kubectl api-resources --api-group=kueue.x-k8s.io -o name"] = []shell.CommandResult{{ExitCode: 0, Stdout: "clusterqueues.kueue.x-k8s.io\nlocalqueues.kueue.x-k8s.io\nresourceflavors.kueue.x-k8s.io\nworkloads.kueue.x-k8s.io\n"}}

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
		"Slice Controller: Logs (tail 500)",
		"Slice Controller: Previous Logs (tail 500)",
		"WAS: CompositePodGroups",
		"WAS: PodGroups",
		"WAS: TPU Scheduler",
		"WAS: TPU Workload Controller",
		"WAS: Owned Resources",
	}
	for _, unexpected := range unexpectedSections {
		if strings.Contains(content, unexpected) {
			t.Errorf("expected report without WAS/Slice APIs to skip %q, but found it in:\n%s", unexpected, content)
		}
	}

	for _, call := range mockExec.calls {
		for _, forbiddenPrefix := range []string{
			"kubectl get slices.accelerator.gke.io",
			"kubectl get compositepodgroups",
			"kubectl get podgroups",
			"kubectl describe deployment slice-controller-controller-manager",
			"kubectl describe deployment tpu-scheduler",
			"kubectl describe deployment tpu-workload-controller",
			"kubectl get pods -A -l component=tpu-scheduler",
			"kubectl describe compositepodgroups",
			"kubectl describe podgroups",
			"kubectl describe slices.accelerator.gke.io",
		} {
			if strings.HasPrefix(call, forbiddenPrefix) {
				t.Errorf("unexpected kubectl call %q when WAS/Slice APIs are absent", call)
			}
		}
	}
}

func TestInspectCluster_DiscoveryFailOpenAndPodLabelFallback(t *testing.T) {
	clusterName := "test-cluster-fail-open"
	location := "us-central1-a"
	project := "test-project"
	reportPath := filepath.Join(t.TempDir(), "inspect-fail-open.log")

	responses := defaultMockResponses(clusterName, location, project)
	// Simulate restricted discovery RBAC where kubectl api-resources fails for all groups.
	responses["kubectl api-resources --api-group=scheduling.k8s.io -o name"] = []shell.CommandResult{{ExitCode: 1, Stderr: "forbidden"}}
	responses["kubectl api-resources --api-group=accelerator.gke.io -o name"] = []shell.CommandResult{{ExitCode: 1, Stderr: "forbidden"}}
	responses["kubectl api-resources --api-group=kueue.x-k8s.io -o name"] = []shell.CommandResult{{ExitCode: 1, Stderr: "forbidden"}}

	// Fail-open runAndLogOptional must still collect group-qualified built-in scheduling.k8s.io APIs and Slices.
	responses["kubectl get slices.accelerator.gke.io -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}
	responses["kubectl get compositepodgroups.scheduling.k8s.io -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}
	responses["kubectl get podgroups.scheduling.k8s.io -A -o yaml"] = []shell.CommandResult{{ExitCode: 0, Stdout: "apiVersion: v1\nkind: List\nitems: []\n"}}

	// Pod discovery and deployment lookups fail; fallback probes tpu-scheduler-system then kube-system pod-label logs.
	responses["kubectl logs -l component=tpu-scheduler -n kube-system -c kube-scheduler --tail=500"] = []shell.CommandResult{{ExitCode: 0, Stdout: "tpu-scheduler-pod-current-logs"}}
	responses["kubectl logs -l component=tpu-scheduler -n kube-system -c kube-scheduler --previous --tail=500"] = []shell.CommandResult{{ExitCode: 1, Stderr: "previous terminated container not found"}}

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

	for _, want := range []string{
		"Slice Controller: Slices (YAML)",
		"WAS: CompositePodGroups (YAML)",
		"WAS: PodGroups (YAML)",
		"tpu-scheduler-pod-current-logs",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected fail-open report to contain %q, got:\n%s", want, content)
		}
	}
	if strings.Contains(content, "previous terminated container not found") {
		t.Errorf("expected failed --previous logs to be skipped gracefully without error output, got:\n%s", content)
	}
}

func TestFilterOwnedWASResources_HierarchyAndIsolation(t *testing.T) {
	wasState := inspectWASState{
		workloadPodsJSON: `{"items":[` +
			`{"metadata":{"name":"train-job-lws-leader-0","namespace":"ns-a","annotations":{"tpuplugin.cloud.google.com/pod-group-names":"ann-pg-0"},"ownerReferences":[{"kind":"Job","name":"train-job-worker-0"}]},"spec":{"schedulingGroup":{"podGroupName":"lws-leaf-pg-0"}}}` +
			`]}`,
		compositePodGroupsYAML: `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: train-job
    namespace: ns-a
    annotations:
      tpuplugin.cloud.google.com/slice-name: explicit-slice-0
    ownerReferences:
    - kind: JobSet
      name: train-job
  spec:
    schedulingPolicy:
      gang:
        minGroupCount: 2
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: train-job-worker-0
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: train-job-worker-0
  spec:
    parentCompositePodGroupName: train-job
    workloadRef:
      workloadName: train-job-worker-0
      templateName: default
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: lws-parent-cpg
    namespace: ns-a
    annotations:
      tpuplugin.cloud.google.com/slice-name: lws-parent-slice
  spec:
    workloadRef:
      workloadName: train-job-lws-leader-0
      templateName: default
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: init-owner-cpg
    namespace: ns-a
    annotations:
      tpuplugin.cloud.google.com/init-owner-ref: jobset.x-k8s.io/v1alpha2/JobSet/train-job
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: train-job-v2
    namespace: ns-a
    ownerReferences:
    - kind: JobSet
      name: train-job-v2
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: train-job-v2-worker-0
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: train-job-v2-worker-0
  spec:
    parentCompositePodGroupName: train-job-v2
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: shared-name
    namespace: ns-a
    ownerReferences:
    - kind: JobSet
      name: other-job
`,
		podGroupsYAML: `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: train-job-worker-0-cube-0
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: train-job-worker-0
  spec:
    parentCompositePodGroupName: train-job-worker-0
    workloadRef:
      workloadName: train-job-worker-0
      templateName: default
    schedulingPolicy:
      gang:
        minCount: 4
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: lws-leaf-pg-0
    namespace: ns-a
    ownerReferences:
    - kind: Pod
      name: train-job-lws-leader-0
  spec:
    parentCompositePodGroupName: lws-parent-cpg
    workloadRef:
      workloadName: train-job-lws-leader-0
      templateName: default
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: lws-leaf-pg-1
    namespace: ns-a
  spec:
    parentCompositePodGroupName: lws-parent-cpg
    workloadRef:
      workloadName: train-job-lws-leader-1
      templateName: default
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: ann-pg-0
    namespace: ns-a
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: workloadref-pg
    namespace: ns-a
  spec:
    workloadRef:
      workloadName: train-job
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: shared-name
    namespace: ns-a
  spec:
    parentCompositePodGroupName: train-job
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: train-job-v2-worker-0-cube-0
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: train-job-v2-worker-0
  spec:
    parentCompositePodGroupName: train-job-v2-worker-0
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: wrong-ns-pg
    namespace: ns-b
  spec:
    parentCompositePodGroupName: train-job
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
    name: lws-parent-slice
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: cpg-ref-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: compositepodgroup/ns-a/train-job
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: pg-shared-name-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: podgroup/ns-a/shared-name
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: cpg-shared-name-other-job-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: compositepodgroup/ns-a/shared-name
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: sibling-v2-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: compositepodgroup/ns-a/train-job-v2
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: train-job-delayed-delete-abcd1
    annotations:
      tpuplugin.cloud.google.com/delayed-slice-deletion-time: "2026-11-16T10:05:00Z"
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: two-part-ref-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: ns-a/lws-leaf-pg-1
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: wrong-ns-ref-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: podgroup/ns-b/train-job-worker-0-cube-0
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: labeled-slice
    labels:
      tpuplugin.cloud.google.com/owner-namespace: ns-a
      tpuplugin.cloud.google.com/owner-name: lws-leaf-pg-0
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
      name: lws-leaf-pg-0
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: train-job-unowned-prefix-only
`,
	}

	cpgs, pgs, slices := filterOwnedWASResources("train-job", "ns-a", "jobset-train-job-123", wasState)

	var gotCPGs, gotPGs, gotSlices []string
	for _, c := range cpgs {
		gotCPGs = append(gotCPGs, c.Metadata.Name)
	}
	for _, p := range pgs {
		gotPGs = append(gotPGs, p.Metadata.Name)
	}
	for _, s := range slices {
		gotSlices = append(gotSlices, s.Metadata.Name)
	}

	wantCPGs := []string{"train-job", "train-job-worker-0", "lws-parent-cpg", "init-owner-cpg"}
	if strings.Join(gotCPGs, ",") != strings.Join(wantCPGs, ",") {
		t.Errorf("cpgs = %v, want %v", gotCPGs, wantCPGs)
	}
	wantPGs := []string{"train-job-worker-0-cube-0", "lws-leaf-pg-0", "lws-leaf-pg-1", "ann-pg-0", "workloadref-pg", "shared-name"}
	if strings.Join(gotPGs, ",") != strings.Join(wantPGs, ",") {
		t.Errorf("pgs = %v, want %v", gotPGs, wantPGs)
	}
	wantSlices := []string{"explicit-slice-0", "lws-parent-slice", "cpg-ref-slice", "pg-shared-name-slice", "two-part-ref-slice", "labeled-slice", "ownerref-slice"}
	if strings.Join(gotSlices, ",") != strings.Join(wantSlices, ",") {
		t.Errorf("slices = %v, want %v", gotSlices, wantSlices)
	}

	singleParsed := parseWASResourceItems("apiVersion: scheduling.k8s.io/v1alpha3\nkind: CompositePodGroup\nmetadata:\n  name: single-cpg\n")
	if len(singleParsed) != 1 || singleParsed[0].Metadata.Name != "single-cpg" || singleParsed[0].raw == nil {
		t.Errorf("expected single object YAML to parse with raw map preserved, got %+v", singleParsed)
	}
	if emptyCPGs, emptyPGs, emptySlices := filterOwnedWASResources("", "ns-a", "", wasState); emptyCPGs != nil || emptyPGs != nil || emptySlices != nil {
		t.Errorf("expected nil results for empty workloadName")
	}
	if items := parseWASResourceItems(":::invalid yaml"); items != nil {
		t.Errorf("expected nil for invalid YAML, got %+v", items)
	}

	// Pre-pod downward walk: no pods exist yet, and child CPGs/leaf PGs set spec.workloadRef.workloadName
	// to the immediate child Job name rather than the root JobSet name.
	prePodState := inspectWASState{
		compositePodGroupsYAML: `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: prepod-job
    namespace: ns-a
    ownerReferences:
    - kind: JobSet
      name: prepod-job
  spec:
    workloadRef:
      workloadName: prepod-job
      templateName: default
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: prepod-job-worker-0
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: prepod-job-worker-0
  spec:
    parentCompositePodGroupName: prepod-job
    workloadRef:
      workloadName: prepod-job-worker-0
      templateName: default
`,
		podGroupsYAML: `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: prepod-job-worker-0-cube-0
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: prepod-job-worker-0
  spec:
    parentCompositePodGroupName: prepod-job-worker-0
    workloadRef:
      workloadName: prepod-job-worker-0
      templateName: default
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: prepod-job-worker-0-cube-1
    namespace: ns-a
    ownerReferences:
    - kind: Job
      name: prepod-job-worker-0
  spec:
    parentCompositePodGroupName: prepod-job-worker-0
    workloadRef:
      workloadName: prepod-job-worker-0
      templateName: default
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: prepod-job
    namespace: ns-a
    annotations:
      tpuplugin.cloud.google.com/init-owner-ref: batch/v1/Job/other-unrelated-job
`,
		slicesYAML: `apiVersion: v1
kind: List
items:
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: prepod-cube-1-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: podgroup/ns-a/prepod-job-worker-0-cube-1
`,
	}
	ppCPGs, ppPGs, ppSlices := filterOwnedWASResources("prepod-job", "ns-a", "", prePodState)
	if len(ppCPGs) != 2 || ppCPGs[0].Metadata.Name != "prepod-job" || ppCPGs[1].Metadata.Name != "prepod-job-worker-0" {
		t.Errorf("pre-pod cpgs = %+v, want [prepod-job prepod-job-worker-0]", ppCPGs)
	}
	if len(ppPGs) != 2 || ppPGs[0].Metadata.Name != "prepod-job-worker-0-cube-0" || ppPGs[1].Metadata.Name != "prepod-job-worker-0-cube-1" {
		t.Errorf("pre-pod pgs = %+v, want [prepod-job-worker-0-cube-0 prepod-job-worker-0-cube-1]", ppPGs)
	}
	if len(ppSlices) != 1 || ppSlices[0].Metadata.Name != "prepod-cube-1-slice" {
		t.Errorf("pre-pod slices = %+v, want [prepod-cube-1-slice]", ppSlices)
	}

	// Leaked Slice whose owning CompositePodGroup was deleted while an unrelated workload's CPG
	// remains live in the same namespace, plus a 2-part ref pointing to an existing foreign CPG.
	leakedState := inspectWASState{
		compositePodGroupsYAML: `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: other-workload
    namespace: ns-a
    ownerReferences:
    - kind: JobSet
      name: other-workload
- apiVersion: scheduling.k8s.io/v1alpha3
  kind: CompositePodGroup
  metadata:
    name: colliding-job
    namespace: ns-a
    ownerReferences:
    - kind: JobSet
      name: unrelated-owner
`,
		slicesYAML: `apiVersion: v1
kind: List
items:
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: leaked-job-slice-0
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: compositepodgroup/ns-a/leaked-job
- apiVersion: accelerator.gke.io/v1beta1
  kind: Slice
  metadata:
    name: foreign-two-part-slice
    annotations:
      tpuplugin.cloud.google.com/slice-owner-reference: ns-a/colliding-job
`,
	}
	_, _, leakedSlices := filterOwnedWASResources("leaked-job", "ns-a", "", leakedState)
	if len(leakedSlices) != 1 || leakedSlices[0].Metadata.Name != "leaked-job-slice-0" {
		t.Errorf("leaked slices = %+v, want [leaked-job-slice-0]", leakedSlices)
	}
	_, _, collidingSlices := filterOwnedWASResources("colliding-job", "ns-a", "", leakedState)
	if len(collidingSlices) != 0 {
		t.Errorf("colliding slices = %+v, want none when existing CPG belongs to unrelated-owner", collidingSlices)
	}
}

func TestInspectRedactionAndYAMLSanitization(t *testing.T) {
	// Construct synthetic token patterns dynamically so secret scanners do not flag test fixtures.
	oauthTok := "ya" + "29.a0AfB_secret_oauth_token"
	iamTok := "iam" + "-token1234567890"
	jwtTok := "ey" + "JhbGciOiJIUzI1NiJ9." + "ey" + "JzdWIiOiIxMjM0NTYifQ.sigvalue123"
	awsKey := "AK" + "IAIOSFODNN7EXAMPLE"
	pemBegin := "-----BEGIN RSA " + "PRIVATE KEY-----"
	pemEnd := "-----END RSA " + "PRIVATE KEY-----"
	annOAuthTok := "ya" + "29.secret_in_annotation"
	ctrlLogTok := "ya" + "29.controller_log_leak"
	cmTok := "ya" + "29.cm_token_leak"

	describeInput := "Name:         train-pod-0\n" +
		"Containers:\n" +
		"  trainer:\n" +
		"    Args:\n" +
		"      --v=5\n" +
		"      --api-key=super-secret-api-key\n" +
		"      --client-secret\n" +
		"      multiline-secret-val\n" +
		"      --token my-inline-token\n" +
		"    Environment:\n" +
		"      HF_TOKEN:        hf_plaintext_secret_999\n" +
		"      WANDB_API_KEY:   <set to the key 'api-key' in secret 'wandb-secret'>  Optional: false\n" +
		"      POD_IP:           (v1:status.podIP)\n" +
		"      DB_PASSWORD:     db-pass-12345\n" +
		"      EMPTY_VAR:\n" +
		"    Mounts:\n" +
		"      /var/run/secrets from kube-api-access (ro)\n" +
		"Events:\n" +
		"  Normal  Pulling  Authorization: Bearer " + jwtTok + "\n" +
		"  Warning Auth     token=" + oauthTok + " " + iamTok + " " + awsKey + " https://alice:hunter2@proxy.example.com/v1\n" +
		pemBegin + "\n" +
		"MIIEowIBAAKCAQEAsecretkeybytes\n" +
		pemEnd + "\n"
	redacted := redactSensitiveText(describeInput)

	for _, secret := range []string{
		"super-secret-api-key",
		"multiline-secret-val",
		"my-inline-token",
		"hf_plaintext_secret_999",
		"db-pass-12345",
		jwtTok,
		oauthTok,
		iamTok,
		awsKey,
		"hunter2",
		"MIIEowIBAAKCAQEAsecretkeybytes",
	} {
		if strings.Contains(redacted, secret) {
			t.Errorf("redacted text leaked secret %q:\n%s", secret, redacted)
		}
	}

	for _, preserved := range []string{
		"Name:         train-pod-0",
		"--v=5",
		"HF_TOKEN: [REDACTED]",
		"WANDB_API_KEY:   <set to the key 'api-key' in secret 'wandb-secret'>  Optional: false",
		"POD_IP:           (v1:status.podIP)",
		"DB_PASSWORD: [REDACTED]",
		"EMPTY_VAR:",
		"https://alice:[REDACTED]@proxy.example.com/v1",
		"[REDACTED_PRIVATE_KEY]",
	} {
		if !strings.Contains(redacted, preserved) {
			t.Errorf("redacted text missing preserved content %q:\n%s", preserved, redacted)
		}
	}

	rawYAML := `apiVersion: v1
kind: List
items:
- apiVersion: scheduling.k8s.io/v1beta1
  kind: PodGroup
  metadata:
    name: pg-1
    namespace: ns-a
    uid: 1111-2222
    creationTimestamp: "2026-11-16T09:00:00Z"
    managedFields:
    - manager: tpu-workload-controller
    annotations:
      tpuplugin.cloud.google.com/slice-name: slice-1
      kubectl.kubernetes.io/last-applied-configuration: '{"secret":"drop-me"}'
      custom-note: "Bearer ` + annOAuthTok + `"
  spec:
    parentCompositePodGroupName: cpg-1
    template:
      spec:
        containers:
        - name: app
          env:
          - name: API_TOKEN
            value: raw-yaml-env-secret
          - name: NUMERIC_PIN
            value: 987654321
          - name: FROM_SECRET
            valueFrom:
              secretKeyRef:
                name: my-sec
                key: token
          envFrom:
          - secretRef:
              name: all-secrets
`
	sanitizedYAML := sanitizeK8sYAML(rawYAML)
	for _, stripped := range []string{
		"managedFields",
		"last-applied-configuration",
		"drop-me",
		annOAuthTok,
		"raw-yaml-env-secret",
		"987654321",
	} {
		if strings.Contains(sanitizedYAML, stripped) {
			t.Errorf("sanitized YAML leaked %q:\n%s", stripped, sanitizedYAML)
		}
	}
	for _, kept := range []string{
		"apiVersion: scheduling.k8s.io/v1beta1",
		"kind: PodGroup",
		"name: pg-1",
		"uid: 1111-2222",
		"tpuplugin.cloud.google.com/slice-name: slice-1",
		"parentCompositePodGroupName: cpg-1",
		"name: API_TOKEN",
		"name: NUMERIC_PIN",
		"value: '[REDACTED]'",
		"name: my-sec",
	} {
		if !strings.Contains(sanitizedYAML, kept) {
			t.Errorf("sanitized YAML dropped expected field %q:\n%s", kept, sanitizedYAML)
		}
	}

	// End-to-end check: secrets in pod describe, ConfigMap YAML (including sensitive key names), and controller logs never reach the report file.
	reportPath := filepath.Join(t.TempDir(), "inspect-redacted.log")
	responses := defaultMockResponses("sec-cluster", "us-central1-a", "test-project")
	responses["kubectl describe pods"] = []shell.CommandResult{{ExitCode: 0, Stdout: describeInput}}
	responses["kubectl logs deployment/jobset-controller-manager"] = []shell.CommandResult{{ExitCode: 0, Stdout: "I1125 auth header Authorization: Bearer " + ctrlLogTok}}
	responses["kubectl get configmap"] = []shell.CommandResult{
		{ExitCode: 0, Stdout: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: sec-cluster-metadata\n  managedFields:\n  - manager: kubectl\n  annotations:\n    kubectl.kubernetes.io/last-applied-configuration: '{\"cm\":\"strip\"}'\ndata:\n  cluster_name: sec-cluster\n  api_key: cm-plaintext-api-key\n  client_secret: cm-plaintext-client-secret\n  note: \"token " + cmTok + "\"\nbinaryData:\n  auth_token: Y20tYmluYXJ5LXNlY3JldA==\n"},
		{ExitCode: 0, Stdout: "configmap-data"},
	}

	orc := newTestGKEOrchestrator(NewMockExecutor(responses))
	orc.projectID = "test-project"
	orc.kubeClient = &MockKubeClient{Namespace: "default", Workloads: []string{"jobset-train-1"}}
	if err := orc.InspectCluster(orchestrator.InspectOptions{
		ProjectID:       "test-project",
		ClusterName:     "sec-cluster",
		ClusterLocation: "us-central1-a",
		WorkloadName:    "train",
		OutputPath:      reportPath,
	}); err != nil {
		t.Fatalf("InspectCluster() error = %v", err)
	}
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}
	report := string(reportBytes)
	for _, secret := range []string{
		"hf_plaintext_secret_999",
		"db-pass-12345",
		ctrlLogTok,
		cmTok,
		"cm-plaintext-api-key",
		"cm-plaintext-client-secret",
		"Y20tYmluYXJ5LXNlY3JldA==",
		"managedFields",
		"last-applied-configuration",
	} {
		if strings.Contains(report, secret) {
			t.Errorf("generated inspect report leaked %q", secret)
		}
	}
	if !strings.Contains(report, "cluster_name: sec-cluster") {
		t.Errorf("expected non-sensitive ConfigMap key cluster_name to be preserved")
	}
}
