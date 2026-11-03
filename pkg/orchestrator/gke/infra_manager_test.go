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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestCleanAndProcessManifests(t *testing.T) {
	orc := &GKEOrchestrator{}

	inputYAML := `
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  description: this should be removed
spec:
  containers:
  - name: main
    image: nginx
    description: this should also be removed
`

	cleaned, err := orc.cleanAndProcessManifests([]byte(inputYAML), nil)
	if err != nil {
		t.Fatalf("cleanAndProcessManifests failed: %v", err)
	}

	output := string(cleaned)

	if strings.Contains(output, "description:") {
		t.Errorf("expected descriptions to be removed, but got: %s", output)
	}
	if !strings.Contains(output, "test-pod") {
		t.Errorf("expected test-pod to be preserved, but got: %s", output)
	}
}

func TestValidateClusterState_TargetNamespaceValidation(t *testing.T) {
	mockExec := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 0}
		},
	}

	var validated []string
	mockDyn := &mockDynamicClient{
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			validated = append(validated, name)
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name)
		},
	}

	// The kubeconfig namespace deliberately differs from --gke-namespace so the
	// test proves the seeded namespace (not the kubeconfig one) is validated.
	kube := &countingKubeClient{MockKubeClient: MockKubeClient{Namespace: "kube-ctx-ns"}}
	orc := &GKEOrchestrator{
		executor:         mockExec,
		dynClient:        mockDyn,
		kubeClient:       kube,
		kubeconfigLoader: staticLoader(clientcmdapi.NewConfig()),
	}

	job := &orchestrator.JobDefinition{
		ClusterName:     "test-cluster",
		ClusterLocation: "us-central1-a",
		ProjectID:       "test-project",
		GKENamespace:    "nonexistent-ns",
	}
	orc.namespace = job.GKENamespace // as done at the top of SubmitJob

	err := orc.ValidateClusterState(job)
	if err == nil {
		t.Fatal("expected ValidateClusterState to fail when namespace validation fails, got nil")
	}

	expectedErr := `target namespace "nonexistent-ns" does not exist on GKE cluster "test-cluster"`
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error to contain %q, got: %v", expectedErr, err)
	}
	if len(validated) != 1 || validated[0] != "nonexistent-ns" {
		t.Errorf("validated namespaces = %v, want [nonexistent-ns]", validated)
	}
	if kube.calls != 0 {
		t.Errorf("kubeconfig namespace lookup should be skipped when --gke-namespace is set, got %d calls", kube.calls)
	}
}

const (
	jobSetCRDGetCmd       = "kubectl get crd jobsets.jobset.x-k8s.io --ignore-not-found -o name --request-timeout=10s"
	jobSetServiceGetCmd   = "kubectl get service jobset-webhook-service -n jobset-system --ignore-not-found -o name --request-timeout=10s"
	jobSetEndpointsCmd    = "kubectl get endpointslice -l kubernetes.io/service-name=jobset-webhook-service -n jobset-system -o json --request-timeout=10s"
	jobSetSelectorCmd     = "kubectl get service jobset-webhook-service -n jobset-system -o jsonpath={.spec.selector} --request-timeout=10s"
	jobSetPodsCmd         = "kubectl get pods -n jobset-system -o json --request-timeout=10s"
	jobSetRolloutCmd      = "kubectl rollout status deployment/jobset-controller-manager -n jobset-system --timeout=600s"
	jobSetApplyCmd        = "kubectl apply -f"
	authCanICreateCmd     = "kubectl auth can-i create"
	readyEndpointsJSON    = `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true}}]}]}`
	notReadyEndpointsJSON = `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":false}}]}]}`
	jobSetTestKubeContext = "gke_test-project_us-central1_test-cluster"
	jobSetPinnedKubectl   = "kubectl --context " + jobSetTestKubeContext
	testJobSetManifest    = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobset-controller-manager
  namespace: jobset-system
spec:
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - name: manager
        image: jobset:test
`
)

var (
	objectPresent     = shell.CommandResult{ExitCode: 0, Stdout: "object/present\n"}
	objectAbsent      = shell.CommandResult{ExitCode: 0}
	readForbidden     = shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): access denied"}
	readUnreachable   = shell.CommandResult{ExitCode: 1, Stderr: "Unable to connect to the server"}
	readyEndpoints    = shell.CommandResult{ExitCode: 0, Stdout: readyEndpointsJSON}
	notReadyEndpoints = shell.CommandResult{ExitCode: 0, Stdout: notReadyEndpointsJSON}
	commandOK         = shell.CommandResult{ExitCode: 0}
	canIYes           = shell.CommandResult{ExitCode: 0, Stdout: "yes\n"}
	legacySelector    = shell.CommandResult{ExitCode: 0, Stdout: `{"control-plane":"controller-manager"}`}
	noPods            = shell.CommandResult{ExitCode: 0, Stdout: `{"items":[]}`}
)

func repeatResult(r shell.CommandResult, n int) []shell.CommandResult {
	results := make([]shell.CommandResult, n)
	for i := range results {
		results[i] = r
	}
	return results
}

func commandsWithPrefix(m *MockExecutor, prefix string) []string {
	var cmds []string
	for _, c := range m.calls {
		if strings.HasPrefix(c, prefix) {
			cmds = append(cmds, c)
		}
	}
	return cmds
}

func callsWithPrefix(m *MockExecutor, prefix string) int {
	return len(commandsWithPrefix(m, prefix))
}

// generatedJobSetManifest is where a fresh install in these tests writes the JobSet manifest.
func generatedJobSetManifest() string {
	return filepath.Join(os.Getenv("HOME"), ".gcluster", "generated", "jobset-"+jobSetTestKubeContext+".yaml")
}

// newJobSetTestOrchestrator wires responses into an orchestrator with no poll delay, an isolated
// home directory, and an HTTP client and prompt that fail the test if JobSet manifests are
// downloaded or the user is prompted.
func newJobSetTestOrchestrator(t *testing.T, responses map[string][]shell.CommandResult) (*GKEOrchestrator, *MockExecutor) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	origInterval := jobSetWebhookPollInterval
	jobSetWebhookPollInterval = 0
	t.Cleanup(func() { jobSetWebhookPollInterval = origInterval })
	origPrompt := shell.PromptYesNo
	shell.PromptYesNo = func(prompt string) bool {
		t.Fatalf("unexpected prompt: %s", prompt)
		return false
	}
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.kubeContext = jobSetTestKubeContext
	orc.httpClient = &mockHTTPClient{getFunc: func(url string) (*http.Response, error) {
		t.Fatalf("unexpected download of %s", url)
		return nil, nil
	}}
	return orc, mockExec
}

func serveJobSetManifest(orc *GKEOrchestrator, status int, body string) {
	orc.httpClient = &mockHTTPClient{getFunc: func(url string) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	}}
}

// assertNoJobSetMutation fails unless every command was a kubectl read (or starts with one of
// alsoAllowed) and nothing was streamed.
func assertNoJobSetMutation(t *testing.T, m *MockExecutor, alsoAllowed ...string) {
	t.Helper()
	allowed := append([]string{"kubectl get "}, alsoAllowed...)
	for _, c := range m.calls {
		if !slices.ContainsFunc(allowed, func(p string) bool { return strings.HasPrefix(c, p) }) {
			t.Errorf("unexpected command %q; only commands starting with %q are allowed", c, allowed)
		}
	}
	if len(m.streamCalls) != 0 {
		t.Errorf("unexpected streamed commands: %v", m.streamCalls)
	}
}

// assertEveryKubectlCallPinned fails unless every kubectl call, streamed or not, carried --context ctx.
func assertEveryKubectlCallPinned(t *testing.T, m *MockExecutor, ctx string) {
	t.Helper()
	kubectlCalls := 0
	for _, c := range slices.Concat(m.calls, m.streamCalls) {
		if strings.HasPrefix(c, "kubectl ") {
			kubectlCalls++
		}
	}
	if len(m.contexts) != kubectlCalls {
		t.Errorf("%d of %d kubectl calls carried --context; want all of them", len(m.contexts), kubectlCalls)
	}
	m.assertAllKubectlPinned(t, ctx)
}

func assertContainsAll(t *testing.T, got string, want, notWant []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q; got:\n%s", w, got)
		}
	}
	for _, w := range notWant {
		if strings.Contains(got, w) {
			t.Errorf("unexpectedly contains %q; got:\n%s", w, got)
		}
	}
}

func TestJobSetWebhookWaitBudget(t *testing.T) {
	if jobSetWebhookWaitTimeout != 5*time.Minute || jobSetWebhookPollInterval != 3*time.Second || jobSetWebhookPollAttempts != 100 {
		t.Errorf("wait budget = %s, poll interval = %s, attempts = %d; want 5m0s, 3s, 100 (the documented 5-minute wait)",
			jobSetWebhookWaitTimeout, jobSetWebhookPollInterval, jobSetWebhookPollAttempts)
	}
}

func TestCheckAndInstallJobSetCRD_ExistingInstallWaitsWithoutModifying(t *testing.T) {
	tests := []struct {
		name              string
		crd, service      shell.CommandResult
		endpoints         []shell.CommandResult
		wantServiceCalls  int
		wantEndpointCalls int
	}{
		{
			name: "webhook ready on first read skips Service read",
			crd:  objectPresent, service: objectPresent,
			endpoints:         []shell.CommandResult{readyEndpoints},
			wantServiceCalls:  0,
			wantEndpointCalls: 1,
		},
		{
			name: "controller pod replacement: no ready endpoint until the third read",
			crd:  objectPresent, service: objectPresent,
			endpoints:         []shell.CommandResult{notReadyEndpoints, {ExitCode: 0, Stdout: `{"items":[]}`}, readyEndpoints},
			wantServiceCalls:  1,
			wantEndpointCalls: 3,
		},
		{
			name: "transient endpoint read failure, then ready",
			crd:  objectPresent, service: objectPresent,
			endpoints:         []shell.CommandResult{readUnreachable, readyEndpoints},
			wantServiceCalls:  1,
			wantEndpointCalls: 2,
		},
		{
			name: "forbidden reads count as installed and healthy",
			crd:  readForbidden, service: readForbidden,
			endpoints:         []shell.CommandResult{readForbidden},
			wantServiceCalls:  1,
			wantEndpointCalls: 1,
		},
		{
			name: "present CRD with forbidden endpoint read verifies Service once",
			crd:  objectPresent, service: objectPresent,
			endpoints:         []shell.CommandResult{readForbidden},
			wantServiceCalls:  1,
			wantEndpointCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc, mockExec := newJobSetTestOrchestrator(t, map[string][]shell.CommandResult{
				jobSetCRDGetCmd:     {tc.crd},
				jobSetServiceGetCmd: {tc.service},
				jobSetEndpointsCmd:  tc.endpoints,
			})

			if err := orc.checkAndInstallJobSetCRD(); err != nil {
				t.Fatalf("checkAndInstallJobSetCRD() error = %v, want nil", err)
			}
			if got := callsWithPrefix(mockExec, jobSetServiceGetCmd); got != tc.wantServiceCalls {
				t.Errorf("Service reads = %d, want %d", got, tc.wantServiceCalls)
			}
			if got := callsWithPrefix(mockExec, jobSetEndpointsCmd); got != tc.wantEndpointCalls {
				t.Errorf("EndpointSlice reads = %d, want %d", got, tc.wantEndpointCalls)
			}
			assertNoJobSetMutation(t, mockExec)
			assertEveryKubectlCallPinned(t, mockExec, jobSetTestKubeContext)
		})
	}
}

func TestCheckAndInstallJobSetCRD_ExistingInstallNeverReady(t *testing.T) {
	podsJSON := `{"items":[{"metadata":{"name":"jobset-controller-manager-abc","labels":{"control-plane":"controller-manager"}},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}]}`
	tests := []struct {
		name          string
		endpoints     []shell.CommandResult
		want, notWant []string
		wantDiagnosis bool
	}{
		{
			name:      "readable but no ready endpoint",
			endpoints: repeatResult(notReadyEndpoints, jobSetWebhookPollAttempts),
			want: []string{
				"JobSet is installed but its webhook is not ready, and gcluster did not modify the existing JobSet installation: ",
				"the JobSet webhook Service jobset-system/jobset-webhook-service has no ready endpoint after ",
				"failurePolicy: Fail",
				"0 of 1 pods in jobset-system match",
				"No pod matches the Service selector",
				jobSetPinnedKubectl + " -n jobset-system get deployments,services,pods -o wide --show-labels",
			},
			notWant:       []string{"the last read failed"},
			wantDiagnosis: true,
		},
		{
			name:          "readable at first, then the last read fails",
			endpoints:     append([]shell.CommandResult{notReadyEndpoints}, repeatResult(readUnreachable, jobSetWebhookPollAttempts-1)...),
			want:          []string{"has no ready endpoint after ", "the last read failed: ", "Unable to connect to the server", "failurePolicy: Fail"},
			wantDiagnosis: true,
		},
		{
			name:      "endpoints never readable",
			endpoints: repeatResult(readUnreachable, jobSetWebhookPollAttempts),
			want: []string{
				"gcluster did not modify the existing JobSet installation",
				"could not read the endpoints of Service jobset-system/jobset-webhook-service for ",
				"Unable to connect to the server",
				"Check your connection to the cluster and your credentials",
			},
			notWant: []string{"failurePolicy", "Diagnostics"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc, mockExec := newJobSetTestOrchestrator(t, map[string][]shell.CommandResult{
				jobSetCRDGetCmd:     {objectPresent},
				jobSetServiceGetCmd: {objectPresent},
				jobSetEndpointsCmd:  tc.endpoints,
				jobSetSelectorCmd:   {{ExitCode: 0, Stdout: `{"app.kubernetes.io/instance":"jobset","app.kubernetes.io/managed-by":"kustomize","app.kubernetes.io/name":"jobset","control-plane":"controller-manager"}`}},
				jobSetPodsCmd:       {{ExitCode: 0, Stdout: podsJSON}},
			})

			err := orc.checkAndInstallJobSetCRD()
			if err == nil {
				t.Fatal("checkAndInstallJobSetCRD() error = nil, want timeout error")
			}
			assertContainsAll(t, err.Error(), tc.want, tc.notWant)
			if got := callsWithPrefix(mockExec, jobSetEndpointsCmd); got != jobSetWebhookPollAttempts {
				t.Errorf("EndpointSlice reads = %d, want %d", got, jobSetWebhookPollAttempts)
			}
			wantDiagnosisReads := 0
			if tc.wantDiagnosis {
				wantDiagnosisReads = 1
			}
			if got := callsWithPrefix(mockExec, jobSetSelectorCmd); got != wantDiagnosisReads {
				t.Errorf("Service selector reads = %d, want %d", got, wantDiagnosisReads)
			}
			assertNoJobSetMutation(t, mockExec)
		})
	}
}

func TestWaitForJobSetWebhookEndpoints_StopsAtDeadline(t *testing.T) {
	orc, mockExec := newJobSetTestOrchestrator(t, map[string][]shell.CommandResult{
		jobSetEndpointsCmd: repeatResult(notReadyEndpoints, jobSetWebhookPollAttempts),
		jobSetSelectorCmd:  {legacySelector},
		jobSetPodsCmd:      {noPods},
	})
	origTimeout := jobSetWebhookWaitTimeout
	t.Cleanup(func() { jobSetWebhookWaitTimeout = origTimeout })
	// The first pause already ends past the deadline, so no second read may start.
	jobSetWebhookWaitTimeout = time.Millisecond
	jobSetWebhookPollInterval = 50 * time.Millisecond

	err := orc.waitForJobSetWebhookEndpoints()
	if err == nil || !strings.Contains(err.Error(), "has no ready endpoint after ") {
		t.Fatalf("waitForJobSetWebhookEndpoints() error = %v, want a no-ready-endpoint timeout", err)
	}
	if got := callsWithPrefix(mockExec, jobSetEndpointsCmd); got != 1 {
		t.Errorf("EndpointSlice reads = %d, want 1", got)
	}
}

func TestCheckAndInstallJobSetCRD_MalformedEndpointsIsHardError(t *testing.T) {
	orc, mockExec := newJobSetTestOrchestrator(t, map[string][]shell.CommandResult{
		jobSetCRDGetCmd:     {objectPresent},
		jobSetServiceGetCmd: {objectPresent},
		jobSetEndpointsCmd:  {{ExitCode: 0, Stdout: "not json"}},
	})

	err := orc.checkAndInstallJobSetCRD()
	if err == nil || !strings.Contains(err.Error(), "failed to parse EndpointSlices") {
		t.Fatalf("checkAndInstallJobSetCRD() error = %v, want parse error", err)
	}
	if got := callsWithPrefix(mockExec, jobSetEndpointsCmd); got != 1 {
		t.Errorf("EndpointSlice reads = %d, want 1", got)
	}
	assertNoJobSetMutation(t, mockExec)
}

func TestCheckAndInstallJobSetCRD_PartialInstallNeverModifies(t *testing.T) {
	crdOnly := []string{
		"JobSet is only partly installed: CRD jobsets.jobset.x-k8s.io exists (or could not be read; see the warning above), but Service jobset-system/jobset-webhook-service was not found. gcluster made no changes.",
		"reject every JobSet create in the cluster",
		"ask your cluster administrator to install it",
		"wait a minute and re-run this command",
		"re-run gcluster deploy",
		jobSetPinnedKubectl + " apply --server-side --force-conflicts -f https://github.com/kubernetes-sigs/jobset/releases/download/<version>/manifests.yaml",
		jobSetPinnedKubectl + " -n jobset-system get deployments -o wide",
		"this gcluster installs " + defaultJobSetVersion,
		jobSetPinnedKubectl + " get jobsets -A",
		jobSetPinnedKubectl + " delete crd jobsets.jobset.x-k8s.io",
		jobSetPinnedKubectl + " delete namespace jobset-system",
		"Never delete the CRD while JobSets exist",
		"requires JobSet in namespace jobset-system",
	}
	serviceOnly := []string{
		"JobSet is only partly installed: Service jobset-system/jobset-webhook-service exists (or could not be read; see the warning above), but CRD jobsets.jobset.x-k8s.io was not found. gcluster made no changes.",
		"No JobSet can exist without its CRD",
		jobSetPinnedKubectl + " delete namespace jobset-system",
		"ask your cluster administrator",
	}
	// A recovery file on disk may come from another cluster, and the CRD must never be restored from a different release.
	never := []string{".gcluster", "restore the CRD"}
	tests := []struct {
		name         string
		crd, service shell.CommandResult
		endpoints    []shell.CommandResult
		wantCalls    int
		want         []string
	}{
		{name: "CRD without webhook Service", crd: objectPresent, service: objectAbsent, endpoints: []shell.CommandResult{noPods}, wantCalls: 3, want: crdOnly},
		{name: "unreadable CRD and missing Service", crd: readForbidden, service: objectAbsent, wantCalls: 2, want: crdOnly},
		{name: "webhook Service without CRD", crd: objectAbsent, service: objectPresent, wantCalls: 2, want: serviceOnly},
		{name: "missing CRD and unreadable Service", crd: objectAbsent, service: readForbidden, wantCalls: 2, want: serviceOnly},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc, mockExec := newJobSetTestOrchestrator(t, map[string][]shell.CommandResult{
				jobSetCRDGetCmd:     {tc.crd},
				jobSetServiceGetCmd: {tc.service},
				jobSetEndpointsCmd:  tc.endpoints,
			})

			err := orc.checkAndInstallJobSetCRD()
			if err == nil {
				t.Fatal("checkAndInstallJobSetCRD() error = nil, want partial installation error")
			}
			assertContainsAll(t, err.Error(), tc.want, never)
			if got := len(mockExec.calls); got != tc.wantCalls {
				t.Errorf("kubectl calls = %d, want %d; calls: %v", got, tc.wantCalls, mockExec.calls)
			}
			assertNoJobSetMutation(t, mockExec)
			assertEveryKubectlCallPinned(t, mockExec, jobSetTestKubeContext)
		})
	}
}

func TestCheckAndInstallJobSetCRD_ReadErrorsNeverModify(t *testing.T) {
	tests := []struct {
		name      string
		responses map[string][]shell.CommandResult
		wantErr   string
	}{
		{
			name:      "CRD read fails",
			responses: map[string][]shell.CommandResult{jobSetCRDGetCmd: {readUnreachable}},
			wantErr:   "failed to check for the JobSet CRD",
		},
		{
			name:      "Service read fails when CRD is absent",
			responses: map[string][]shell.CommandResult{jobSetCRDGetCmd: {objectAbsent}, jobSetServiceGetCmd: {readUnreachable}},
			wantErr:   "failed to check for the JobSet webhook Service",
		},
		{
			name:      "Service read fails when CRD exists and endpoints are not ready",
			responses: map[string][]shell.CommandResult{jobSetCRDGetCmd: {objectPresent}, jobSetEndpointsCmd: {noPods}, jobSetServiceGetCmd: {readUnreachable}},
			wantErr:   "failed to check for the JobSet webhook Service",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc, mockExec := newJobSetTestOrchestrator(t, tc.responses)

			err := orc.checkAndInstallJobSetCRD()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "Unable to connect to the server") {
				t.Fatalf("checkAndInstallJobSetCRD() error = %v, want %q with the kubectl error", err, tc.wantErr)
			}
			assertNoJobSetMutation(t, mockExec)
		})
	}
}

func freshJobSetResponses() map[string][]shell.CommandResult {
	return map[string][]shell.CommandResult{
		jobSetCRDGetCmd:     {objectAbsent},
		jobSetServiceGetCmd: {objectAbsent},
		authCanICreateCmd:   repeatResult(canIYes, len(jobSetInstallPermissionChecks)),
		jobSetApplyCmd:      {commandOK},
		jobSetRolloutCmd:    {commandOK},
		jobSetEndpointsCmd:  {readyEndpoints},
	}
}

func TestCheckAndInstallJobSetCRD_FreshClusterInstalls(t *testing.T) {
	orc, mockExec := newJobSetTestOrchestrator(t, freshJobSetResponses())
	serveJobSetManifest(orc, http.StatusOK, testJobSetManifest)

	if err := orc.checkAndInstallJobSetCRD(); err != nil {
		t.Fatalf("checkAndInstallJobSetCRD() error = %v, want nil", err)
	}
	// Every checked resource is cluster-scoped, so a namespaced RoleBinding must not count.
	wantChecks := []string{
		"kubectl auth can-i create namespaces --all-namespaces",
		"kubectl auth can-i create customresourcedefinitions.apiextensions.k8s.io --all-namespaces",
		"kubectl auth can-i create clusterroles.rbac.authorization.k8s.io --all-namespaces",
		"kubectl auth can-i create clusterrolebindings.rbac.authorization.k8s.io --all-namespaces",
		"kubectl auth can-i create mutatingwebhookconfigurations.admissionregistration.k8s.io --all-namespaces",
		"kubectl auth can-i create validatingwebhookconfigurations.admissionregistration.k8s.io --all-namespaces",
	}
	if got := commandsWithPrefix(mockExec, authCanICreateCmd); !slices.Equal(got, wantChecks) {
		t.Errorf("permission checks = %q, want %q", got, wantChecks)
	}
	manifest := generatedJobSetManifest()
	if got, want := commandsWithPrefix(mockExec, jobSetApplyCmd), []string{"kubectl apply -f " + manifest}; !slices.Equal(got, want) {
		t.Errorf("kubectl apply calls = %q, want %q", got, want)
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Errorf("generated JobSet manifest not written: %v", err)
	}
	assertEveryKubectlCallPinned(t, mockExec, jobSetTestKubeContext)
}

func TestCheckAndInstallJobSetCRD_FreshClusterWithoutPermissions(t *testing.T) {
	responses := freshJobSetResponses()
	canINo := shell.CommandResult{ExitCode: 1, Stdout: "no\n"}
	responses[authCanICreateCmd] = []shell.CommandResult{canIYes, canINo, canIYes, canINo, canIYes, canIYes}
	orc, mockExec := newJobSetTestOrchestrator(t, responses)

	err := orc.checkAndInstallJobSetCRD()
	if err == nil {
		t.Fatal("checkAndInstallJobSetCRD() error = nil, want missing permissions error")
	}
	assertContainsAll(t, err.Error(), []string{
		"JobSet is not installed, and your account cannot install it",
		"'create customresourcedefinitions.apiextensions.k8s.io'",
		"'create clusterrolebindings.rbac.authorization.k8s.io'",
		"gcluster made no changes",
		"jobset.install: true",
	}, []string{"apply -f"})
	assertNoJobSetMutation(t, mockExec, authCanICreateCmd)
}

func TestCheckAndInstallJobSetCRD_FreshInstallFailureHints(t *testing.T) {
	const waitHint = "Later runs of gcluster will wait for the JobSet controller and will not apply the manifests again"
	tests := []struct {
		name          string
		setup         func(t *testing.T, orc *GKEOrchestrator, responses map[string][]shell.CommandResult)
		wantErr       string
		wantApplyHint bool
		wantWaitHint  bool
	}{
		{
			name: "manifest download fails",
			setup: func(t *testing.T, orc *GKEOrchestrator, _ map[string][]shell.CommandResult) {
				serveJobSetManifest(orc, http.StatusInternalServerError, "")
			},
			wantErr: "received status code 500",
		},
		{
			name: "manifest is not valid YAML",
			setup: func(t *testing.T, orc *GKEOrchestrator, _ map[string][]shell.CommandResult) {
				serveJobSetManifest(orc, http.StatusOK, "kind: [unclosed")
			},
			wantErr: "failed to decode YAML document",
		},
		{
			name: "home directory is unknown",
			setup: func(t *testing.T, orc *GKEOrchestrator, _ map[string][]shell.CommandResult) {
				t.Setenv("HOME", "")
				t.Setenv("USERPROFILE", "")
			},
			wantErr: "failed to get user home directory",
		},
		{
			name: "generated manifests directory cannot be created",
			setup: func(t *testing.T, orc *GKEOrchestrator, _ map[string][]shell.CommandResult) {
				homeFile := filepath.Join(t.TempDir(), "home-is-a-file")
				if err := os.WriteFile(homeFile, nil, 0644); err != nil {
					t.Fatal(err)
				}
				t.Setenv("HOME", homeFile)
				t.Setenv("USERPROFILE", homeFile)
			},
			wantErr: "failed to create directory for generated manifests",
		},
		{
			name: "generated manifest cannot be written",
			setup: func(t *testing.T, orc *GKEOrchestrator, _ map[string][]shell.CommandResult) {
				if err := os.MkdirAll(generatedJobSetManifest(), 0755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "failed to write manifests",
		},
		{
			name: "kubectl apply fails",
			setup: func(t *testing.T, orc *GKEOrchestrator, responses map[string][]shell.CommandResult) {
				responses[jobSetApplyCmd] = []shell.CommandResult{{ExitCode: 1, Stderr: "admission webhook denied the request"}}
			},
			wantErr:       "admission webhook denied the request",
			wantApplyHint: true,
		},
		{
			name: "controller rollout fails",
			setup: func(t *testing.T, orc *GKEOrchestrator, responses map[string][]shell.CommandResult) {
				responses[jobSetRolloutCmd] = []shell.CommandResult{{ExitCode: 1, Stderr: "deadline exceeded"}}
				responses[jobSetSelectorCmd] = []shell.CommandResult{legacySelector}
				responses[jobSetPodsCmd] = []shell.CommandResult{noPods}
			},
			wantErr:      "the JobSet controller did not become ready: deadline exceeded\nDiagnostics: Service jobset-system/jobset-webhook-service selects control-plane=controller-manager",
			wantWaitHint: true,
		},
		{
			name: "webhook never becomes ready",
			setup: func(t *testing.T, orc *GKEOrchestrator, responses map[string][]shell.CommandResult) {
				responses[jobSetEndpointsCmd] = repeatResult(notReadyEndpoints, jobSetWebhookPollAttempts)
				responses[jobSetSelectorCmd] = []shell.CommandResult{legacySelector}
				responses[jobSetPodsCmd] = []shell.CommandResult{noPods}
			},
			wantErr:      "has no ready endpoint after ",
			wantWaitHint: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			responses := freshJobSetResponses()
			orc, mockExec := newJobSetTestOrchestrator(t, responses)
			serveJobSetManifest(orc, http.StatusOK, testJobSetManifest)
			tc.setup(t, orc, responses)

			err := orc.checkAndInstallJobSetCRD()
			if err == nil {
				t.Fatal("checkAndInstallJobSetCRD() error = nil, want installation error")
			}
			assertContainsAll(t, err.Error(), []string{"failed to install JobSet: ", tc.wantErr}, nil)
			applyHint := "JobSet may now be partly installed, and later runs of gcluster will not modify it. To finish the installation, run:\n  " +
				jobSetPinnedKubectl + " apply -f " + generatedJobSetManifest()
			if got := strings.Contains(err.Error(), applyHint); got != tc.wantApplyHint {
				t.Errorf("apply hint present = %v, want %v; err:\n%v", got, tc.wantApplyHint, err)
			}
			if got := strings.Contains(err.Error(), waitHint); got != tc.wantWaitHint {
				t.Errorf("wait hint present = %v, want %v; err:\n%v", got, tc.wantWaitHint, err)
			}
			wantApplies := 0
			if tc.wantApplyHint || tc.wantWaitHint {
				wantApplies = 1
			}
			if got := callsWithPrefix(mockExec, jobSetApplyCmd); got != wantApplies {
				t.Errorf("kubectl apply calls = %d, want %d", got, wantApplies)
			}
		})
	}
}

func TestJobSetManifestFile(t *testing.T) {
	tests := []struct{ kubeContext, want string }{
		{kubeContext: "", want: "jobset.yaml"},
		{kubeContext: "gke_my-project_us-central1_my-cluster", want: "jobset-gke_my-project_us-central1_my-cluster.yaml"},
		{kubeContext: "arn:aws:eks:us-east-1:123:cluster/a b", want: "jobset-arn_aws_eks_us-east-1_123_cluster_a_b.yaml"},
		{kubeContext: `..\..\x`, want: "jobset-.._.._x.yaml"},
	}
	for _, tc := range tests {
		if got := jobSetManifestFile(tc.kubeContext); got != tc.want {
			t.Errorf("jobSetManifestFile(%q) = %q, want %q", tc.kubeContext, got, tc.want)
		}
	}
}

func TestWriteGeneratedManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := writeGeneratedManifest([]byte("kind: ConfigMap\n"), "jobset-ctx.yaml")
	if err != nil {
		t.Fatalf("writeGeneratedManifest() error = %v", err)
	}
	if want := filepath.Join(home, ".gcluster", "generated", "jobset-ctx.yaml"); path != want {
		t.Errorf("writeGeneratedManifest() path = %q, want %q", path, want)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "kind: ConfigMap\n" {
		t.Errorf("written manifest = %q, %v; want %q", got, err, "kind: ConfigMap\n")
	}
}

func TestResourceExists(t *testing.T) {
	tests := []struct {
		name            string
		kind, rname, ns string
		result          shell.CommandResult
		wantArgs        string
		wantExists      bool
		wantForbidden   bool
		wantErrContains string
	}{
		{
			name: "cluster-scoped object found", kind: "crd", rname: "jobsets.jobset.x-k8s.io",
			result:     shell.CommandResult{ExitCode: 0, Stdout: "customresourcedefinition.apiextensions.k8s.io/jobsets.jobset.x-k8s.io\n"},
			wantArgs:   "get crd jobsets.jobset.x-k8s.io --ignore-not-found -o name --request-timeout=10s",
			wantExists: true,
		},
		{
			name: "namespaced object absent", kind: "service", rname: "jobset-webhook-service", ns: "jobset-system",
			result:   shell.CommandResult{ExitCode: 0},
			wantArgs: "get service jobset-webhook-service -n jobset-system --ignore-not-found -o name --request-timeout=10s",
		},
		{
			name: "forbidden counts as existing", kind: "service", rname: "jobset-webhook-service", ns: "jobset-system",
			result:        readForbidden,
			wantArgs:      "get service jobset-webhook-service -n jobset-system --ignore-not-found -o name --request-timeout=10s",
			wantExists:    true,
			wantForbidden: true,
		},
		{
			name: "other failure is an error", kind: "crd", rname: "jobsets.jobset.x-k8s.io",
			result:          readUnreachable,
			wantArgs:        "get crd jobsets.jobset.x-k8s.io --ignore-not-found -o name --request-timeout=10s",
			wantErrContains: "Unable to connect to the server",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotArgs string
			orc := &GKEOrchestrator{executor: &mockExecutor{executeCommandFunc: func(name string, args ...string) shell.CommandResult {
				gotArgs = strings.Join(args, " ")
				return tc.result
			}}}

			exists, forbidden, err := orc.checkResourceExists(tc.kind, tc.rname, tc.ns)
			if gotArgs != tc.wantArgs {
				t.Errorf("kubectl args = %q, want %q", gotArgs, tc.wantArgs)
			}
			if tc.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("checkResourceExists() error = %v, want error containing %q", err, tc.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkResourceExists() error = %v, want nil", err)
			}
			if exists != tc.wantExists || forbidden != tc.wantForbidden {
				t.Errorf("checkResourceExists() = (%v, %v), want (%v, %v)", exists, forbidden, tc.wantExists, tc.wantForbidden)
			}
		})
	}
}

func TestServiceEndpointsReady(t *testing.T) {
	tests := []struct {
		name           string
		result         shell.CommandResult
		wantReady      bool
		wantForbidden  bool
		wantUnreadable bool
		wantOtherErr   bool
	}{
		{name: "ready endpoint", result: readyEndpoints, wantReady: true},
		{name: "endpoint without a ready condition counts as ready", result: shell.CommandResult{ExitCode: 0, Stdout: `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{}}]}]}`}, wantReady: true},
		{name: "ready endpoint without addresses", result: shell.CommandResult{ExitCode: 0, Stdout: `{"items":[{"endpoints":[{"conditions":{"ready":true}}]}]}`}},
		{name: "no ready endpoint", result: notReadyEndpoints},
		{name: "forbidden counts as ready", result: readForbidden, wantReady: true, wantForbidden: true},
		{name: "failed read is retryable", result: readUnreachable, wantUnreadable: true},
		{name: "malformed response is a hard error", result: shell.CommandResult{ExitCode: 0, Stdout: "not json"}, wantOtherErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotArgs string
			orc := &GKEOrchestrator{executor: &mockExecutor{executeCommandFunc: func(name string, args ...string) shell.CommandResult {
				gotArgs = strings.Join(args, " ")
				return tc.result
			}}}

			ready, forbidden, err := orc.checkServiceEndpointsReady("kueue-system", "kueue-webhook-service")
			if want := "get endpointslice -l kubernetes.io/service-name=kueue-webhook-service -n kueue-system -o json --request-timeout=10s"; gotArgs != want {
				t.Errorf("kubectl args = %q, want %q", gotArgs, want)
			}
			if ready != tc.wantReady || forbidden != tc.wantForbidden {
				t.Errorf("(ready, forbidden) = (%v, %v), want (%v, %v)", ready, forbidden, tc.wantReady, tc.wantForbidden)
			}
			if got := errors.Is(err, errEndpointsUnreadable); got != tc.wantUnreadable {
				t.Errorf("errors.Is(err, errEndpointsUnreadable) = %v, want %v (err: %v)", got, tc.wantUnreadable, err)
			}
			if got := err != nil && !errors.Is(err, errEndpointsUnreadable); got != tc.wantOtherErr {
				t.Errorf("hard error = %v, want %v (err: %v)", got, tc.wantOtherErr, err)
			}
		})
	}
}

func TestDescribeJobSetPods(t *testing.T) {
	selector := map[string]string{"control-plane": "controller-manager"}
	pod := func(name string, podLabels map[string]string, phase corev1.PodPhase, ready bool) corev1.Pod {
		readyStatus := corev1.ConditionFalse
		if ready {
			readyStatus = corev1.ConditionTrue
		}
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: podLabels},
			Status:     corev1.PodStatus{Phase: phase, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: readyStatus}}},
		}
	}
	crashLooping := pod("ctrl-1", selector, corev1.PodRunning, false)
	crashLooping.Status.ContainerStatuses = []corev1.ContainerStatus{
		{Name: "kube-rbac-proxy", Image: "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1", Ready: true},
		{
			Name:  "manager",
			Image: "registry.k8s.io/jobset/jobset:v0.10.1",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off 5m0s restarting failed container"}},
		},
	}
	pullFailing := pod("ctrl-2", selector, corev1.PodPending, false)
	pullFailing.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  "kube-rbac-proxy",
		Image: "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
	}}
	initFailing := pod("ctrl-init", selector, corev1.PodPending, false)
	initFailing.Status.InitContainerStatuses = []corev1.ContainerStatus{{
		Name:  "cert-init",
		Image: "registry.k8s.io/jobset/cert-init:v0.10.1",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "Back-off pulling image"}},
	}}
	unschedulable := pod("ctrl-3", selector, corev1.PodPending, false)
	unschedulable.Status.Conditions = append(unschedulable.Status.Conditions, corev1.PodCondition{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "0/3 nodes are available: 3 node(s) had untolerated taint {dedicated: gpu}.",
	})
	tests := []struct {
		name          string
		pods          []corev1.Pod
		want, notWant []string
	}{
		{
			name:    "matching ready pod",
			pods:    []corev1.Pod{pod("ctrl-1", map[string]string{"control-plane": "controller-manager", "extra": "x"}, corev1.PodRunning, true)},
			want:    []string{"1 of 1 pods in jobset-system match", "ctrl-1: phase=Running, ready=true, matches selector=true"},
			notWant: []string{"No pod matches"},
		},
		{
			name: "crash-looping container is named first",
			pods: []corev1.Pod{crashLooping},
			want: []string{"ctrl-1: CrashLoopBackOff (container manager, image registry.k8s.io/jobset/jobset:v0.10.1: back-off 5m0s restarting failed container); phase=Running, ready=false"},
		},
		{
			name: "image pull failure without a message",
			pods: []corev1.Pod{pullFailing},
			want: []string{"ctrl-2: ImagePullBackOff (container kube-rbac-proxy, image gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1); phase=Pending"},
		},
		{
			name: "waiting init container is named",
			pods: []corev1.Pod{initFailing},
			want: []string{"ctrl-init: ImagePullBackOff (init container cert-init, image registry.k8s.io/jobset/cert-init:v0.10.1: Back-off pulling image); phase=Pending"},
		},
		{
			name: "unschedulable pod says why",
			pods: []corev1.Pod{unschedulable},
			want: []string{"ctrl-3: Unschedulable (0/3 nodes are available: 3 node(s) had untolerated taint {dedicated: gpu}.); phase=Pending"},
		},
		{
			name: "selector matches no pod",
			pods: []corev1.Pod{pod("ctrl-1", map[string]string{"app.kubernetes.io/name": "jobset"}, corev1.PodRunning, true)},
			want: []string{"0 of 1 pods", "labels=app.kubernetes.io/name=jobset", "No pod matches the Service selector", "the controller Deployment's selector"},
		},
		{
			name: "no pods",
			want: []string{"0 of 0 pods", "No pods are running in namespace jobset-system"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertContainsAll(t, describeJobSetPods(selector, tc.pods), tc.want, tc.notWant)
		})
	}
}

func TestDiagnoseJobSetWebhook(t *testing.T) {
	nextSteps := "To investigate, run:\n  " + jobSetPinnedKubectl + " -n jobset-system get deployments,services,pods -o wide --show-labels\n  " +
		jobSetPinnedKubectl + " -n jobset-system describe pods"
	tests := []struct {
		name          string
		responses     map[string][]shell.CommandResult
		want, notWant []string
	}{
		{
			name:      "Service without selector",
			responses: map[string][]shell.CommandResult{jobSetSelectorCmd: {{ExitCode: 0}}},
			want:      []string{"Service jobset-system/jobset-webhook-service has no selector"},
		},
		{
			name:      "selector unreadable",
			responses: map[string][]shell.CommandResult{jobSetSelectorCmd: {readForbidden}},
			want:      []string{"(diagnostics unavailable: "},
			notWant:   []string{"To investigate"},
		},
		{
			name:      "pods unreadable",
			responses: map[string][]shell.CommandResult{jobSetSelectorCmd: {legacySelector}, jobSetPodsCmd: {readForbidden}},
			want:      []string{"(diagnostics unavailable: "},
			notWant:   []string{"To investigate"},
		},
		{
			name:      "pods listed, with the commands to investigate",
			responses: map[string][]shell.CommandResult{jobSetSelectorCmd: {legacySelector}, jobSetPodsCmd: {noPods}},
			want:      []string{"No pods are running in namespace jobset-system", nextSteps},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc, mockExec := newJobSetTestOrchestrator(t, tc.responses)
			assertContainsAll(t, orc.diagnoseJobSetWebhook(), tc.want, tc.notWant)
			assertNoJobSetMutation(t, mockExec)
			assertEveryKubectlCallPinned(t, mockExec, jobSetTestKubeContext)
		})
	}
}

// recordingKubeClient wraps MockKubeClient with per-object GetResource errors and records every object read.
type recordingKubeClient struct {
	MockKubeClient
	getErrs map[string]error // keyed like reads
	reads   []string         // "<resource>.<group>/<version> <namespace>/<name>"
}

func (c *recordingKubeClient) GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	key := fmt.Sprintf("%s/%s %s/%s", gvr.GroupResource(), gvr.Version, namespace, name)
	c.reads = append(c.reads, key)
	if err := c.getErrs[key]; err != nil {
		return nil, err
	}
	return c.MockKubeClient.GetResource(gvr, namespace, name)
}

const (
	liveDeploymentRead     = "deployments.apps/v1 jobset-system/jobset-controller-manager"
	liveHelmDeploymentRead = "deployments.apps/v1 jobset-system/jobset-controller"
	liveWebhookServiceRead = "services/v1 jobset-system/jobset-webhook-service"
	liveMetricsServiceRead = "services/v1 jobset-system/jobset-controller-manager-metrics-service"
)

func liveJobSetUnstructured(kind, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"kind":     kind,
		"metadata": map[string]interface{}{"name": name, "namespace": "jobset-system"},
	}}
}

func TestCheckNoLiveJobSetObjects(t *testing.T) {
	t.Run("all absent", func(t *testing.T) {
		kube := &recordingKubeClient{}
		orc := &GKEOrchestrator{kubeClient: kube, kubeContext: jobSetTestKubeContext}
		if err := orc.checkNoLiveJobSetObjects(); err != nil {
			t.Fatalf("checkNoLiveJobSetObjects() error = %v, want nil", err)
		}
		want := []string{liveDeploymentRead, liveHelmDeploymentRead, liveWebhookServiceRead, liveMetricsServiceRead}
		if !slices.Equal(kube.reads, want) {
			t.Errorf("reads = %v, want %v", kube.reads, want)
		}
	})

	for _, existing := range []struct {
		resource, kind, name string
	}{
		{"deployments", "Deployment", "jobset-controller-manager"},
		{"deployments", "Deployment", "jobset-controller"},
		{"services", "Service", "jobset-webhook-service"},
		{"services", "Service", "jobset-controller-manager-metrics-service"},
	} {
		t.Run("existing "+existing.kind+" "+existing.name, func(t *testing.T) {
			kube := &recordingKubeClient{MockKubeClient: MockKubeClient{Objects: map[string][]unstructured.Unstructured{
				existing.resource: {liveJobSetUnstructured(existing.kind, existing.name)},
			}}}
			orc := &GKEOrchestrator{kubeClient: kube, kubeContext: jobSetTestKubeContext}
			err := orc.checkNoLiveJobSetObjects()
			if err == nil {
				t.Fatal("checkNoLiveJobSetObjects() error = nil, want existing-object error")
			}
			assertContainsAll(t, err.Error(), []string{
				existing.kind + " jobset-system/" + existing.name + " already exists",
				"gcluster made no changes",
				jobSetPinnedKubectl + " delete namespace jobset-system",
			}, nil)
		})
	}

	readErrs := map[string]error{
		"forbidden": apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, "jobset", errors.New("RBAC: access denied")),
		"timeout":   apierrors.NewServerTimeout(schema.GroupResource{Resource: "services"}, "get", 1),
		"transport": errors.New("dial tcp 10.0.0.1:443: connect: connection refused"),
	}
	for _, read := range []string{liveDeploymentRead, liveHelmDeploymentRead, liveWebhookServiceRead, liveMetricsServiceRead} {
		for errName, readErr := range readErrs {
			t.Run(read+" "+errName, func(t *testing.T) {
				kube := &recordingKubeClient{getErrs: map[string]error{read: readErr}}
				orc := &GKEOrchestrator{kubeClient: kube, kubeContext: jobSetTestKubeContext}
				err := orc.checkNoLiveJobSetObjects()
				if !errors.Is(err, readErr) {
					t.Fatalf("checkNoLiveJobSetObjects() error = %v, want it to wrap %v", err, readErr)
				}
				name := read[strings.LastIndex(read, "/")+1:]
				assertContainsAll(t, err.Error(), []string{name, "gcluster made no changes"}, nil)
			})
		}
	}

	t.Run("client init failure fails closed", func(t *testing.T) {
		orc := &GKEOrchestrator{kubeconfigLoader: staticLoader(clientcmdapi.NewConfig())}
		if err := orc.checkNoLiveJobSetObjects(); err == nil {
			t.Fatal("checkNoLiveJobSetObjects() error = nil, want the Kubernetes client error")
		}
	})
}

func TestCheckAndInstallJobSetCRD_AbortsWhenNamespaceHasJobSetObjects(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "deployments"}, "jobset-controller-manager", errors.New("RBAC: access denied"))
	tests := []struct {
		name    string
		kube    KubeClient
		wantErr string
	}{
		{
			name: "leftover Kustomize Deployment",
			kube: &MockKubeClient{Objects: map[string][]unstructured.Unstructured{
				"deployments": {liveJobSetUnstructured("Deployment", "jobset-controller-manager")},
			}},
			wantErr: "Deployment jobset-system/jobset-controller-manager already exists",
		},
		{
			name: "leftover Helm Deployment",
			kube: &MockKubeClient{Objects: map[string][]unstructured.Unstructured{
				"deployments": {liveJobSetUnstructured("Deployment", "jobset-controller")},
			}},
			wantErr: "Deployment jobset-system/jobset-controller already exists",
		},
		{
			name: "webhook Service created concurrently",
			kube: &MockKubeClient{Objects: map[string][]unstructured.Unstructured{
				"services": {liveJobSetUnstructured("Service", "jobset-webhook-service")},
			}},
			wantErr: "Service jobset-system/jobset-webhook-service already exists",
		},
		{
			name: "leftover metrics Service",
			kube: &MockKubeClient{Objects: map[string][]unstructured.Unstructured{
				"services": {liveJobSetUnstructured("Service", "jobset-controller-manager-metrics-service")},
			}},
			wantErr: "Service jobset-system/jobset-controller-manager-metrics-service already exists",
		},
		{
			name:    "Deployment read forbidden",
			kube:    &recordingKubeClient{getErrs: map[string]error{liveDeploymentRead: forbidden}},
			wantErr: "failed to check whether JobSet Deployment jobset-system/jobset-controller-manager already exists",
		},
		{
			name:    "metrics Service read fails",
			kube:    &recordingKubeClient{getErrs: map[string]error{liveMetricsServiceRead: errors.New("connection reset by peer")}},
			wantErr: "failed to check whether JobSet Service jobset-system/jobset-controller-manager-metrics-service already exists",
		},
		{
			name:    "Kubernetes client cannot be created",
			kube:    nil,
			wantErr: "kubeconfig",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc, mockExec := newJobSetTestOrchestrator(t, freshJobSetResponses())
			orc.kubeClient = tc.kube

			err := orc.checkAndInstallJobSetCRD()
			if err == nil {
				t.Fatal("checkAndInstallJobSetCRD() error = nil, want an error")
			}
			assertContainsAll(t, err.Error(), []string{"failed to install JobSet: ", tc.wantErr}, []string{"apply -f"})
			assertNoJobSetMutation(t, mockExec, authCanICreateCmd)
			assertEveryKubectlCallPinned(t, mockExec, jobSetTestKubeContext)
			if _, statErr := os.Stat(generatedJobSetManifest()); !os.IsNotExist(statErr) {
				t.Errorf("os.Stat(%s) error = %v, want the manifest not to be written", generatedJobSetManifest(), statErr)
			}
		})
	}
}

func TestInitialize_LocationFallback(t *testing.T) {
	setupMockMachineConfig(t)

	mockDescribeOutput := `{
		"locations": ["us-central1-a", "us-central1-b"],
		"nodePools": [],
		"autoscaling": {}
	}`

	mockResponses := map[string][]shell.CommandResult{
		"gcloud container clusters describe my-cluster --location us-central1-a --project my-project --format=json": {
			{
				ExitCode: 1,
				Stderr:   "Resource my-cluster was not found in us-central1-a",
			},
		},
		"gcloud container clusters describe my-cluster --location us-central1 --project my-project --format=json": {
			{
				ExitCode: 0,
				Stdout:   mockDescribeOutput,
			},
		},
	}

	orc := newTestGKEOrchestrator(NewMockExecutor(mockResponses))
	job := &orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterName:     "my-cluster",
		ClusterLocation: "us-central1-a",
	}

	loc, err := orc.Initialize(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	job.ClusterLocation = loc

	err = orc.populateClusterMetadata(job)
	if err != nil {
		t.Fatalf("populateClusterMetadata failed: %v", err)
	}

	if job.ClusterLocation != "us-central1" {
		t.Errorf("Expected job.ClusterLocation to fall back to 'us-central1', got %q", job.ClusterLocation)
	}
}

func TestInitialize_TimeoutAndExecErrors(t *testing.T) {
	primaryCmd := "gcloud container clusters describe my-cluster --location us-central1-a --project my-project --format=json"
	fallbackCmd := "gcloud container clusters describe my-cluster --location us-central1 --project my-project --format=json"

	tests := []struct {
		name          string
		mockResponses map[string][]shell.CommandResult
		wantErrSubstr string
	}{
		{
			name: "Primary describe query times out",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: 124,
						Err:      context.DeadlineExceeded,
					},
				},
			},
			wantErrSubstr: fmt.Sprintf("timed out after %v while trying to reach GKE cluster 'my-cluster' in 'us-central1-a'", shell.DefaultCloudAPITimeout),
		},
		{
			name: "Primary describe query fails to execute",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: -1,
						Err:      fmt.Errorf("executable file not found in $PATH"),
					},
				},
			},
			wantErrSubstr: "failed to execute gcloud: executable file not found in $PATH",
		},
		{
			name: "Fallback regional describe query times out",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: 1,
						Stderr:   "Resource my-cluster was not found in us-central1-a",
					},
				},
				fallbackCmd: {
					{
						ExitCode: 124,
						Err:      context.DeadlineExceeded,
					},
				},
			},
			wantErrSubstr: fmt.Sprintf("timed out after %v while trying to reach GKE cluster 'my-cluster' in fallback region 'us-central1'", shell.DefaultCloudAPITimeout),
		},
		{
			name: "Fallback regional describe query fails to execute",
			mockResponses: map[string][]shell.CommandResult{
				primaryCmd: {
					{
						ExitCode: 1,
						Stderr:   "Resource my-cluster was not found in us-central1-a",
					},
				},
				fallbackCmd: {
					{
						ExitCode: -1,
						Err:      fmt.Errorf("fork/exec gcloud: resource temporarily unavailable"),
					},
				},
			},
			wantErrSubstr: "failed to execute gcloud: fork/exec gcloud: resource temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := newTestGKEOrchestrator(NewMockExecutor(tt.mockResponses))
			_, err := orc.Initialize("my-cluster", "us-central1-a", "my-project")
			if err == nil {
				t.Fatalf("expected Initialize to fail, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Errorf("Initialize() error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
			}
		})
	}
}

// countingKubeClient wraps MockKubeClient to count GetCurrentNamespace calls.
type countingKubeClient struct {
	MockKubeClient
	calls int
}

func (c *countingKubeClient) GetCurrentNamespace(clusterName, location, projectID string) (string, error) {
	c.calls++
	return c.MockKubeClient.GetCurrentNamespace(clusterName, location, projectID)
}

func TestGetCurrentNamespace(t *testing.T) {
	tests := []struct {
		name          string
		cached        string
		kubeClient    *countingKubeClient
		want          string
		wantErr       bool
		wantKubeCalls int
		wantCached    string
	}{
		{
			name:          "cached namespace (from --gke-namespace) short-circuits kubeconfig lookup",
			cached:        "explicit-ns",
			kubeClient:    &countingKubeClient{MockKubeClient: MockKubeClient{Namespace: "kube-ns"}},
			want:          "explicit-ns",
			wantKubeCalls: 0,
			wantCached:    "explicit-ns",
		},
		{
			name:          "falls back to kubeconfig namespace and caches it",
			kubeClient:    &countingKubeClient{MockKubeClient: MockKubeClient{Namespace: "current-ns"}},
			want:          "current-ns",
			wantKubeCalls: 1,
			wantCached:    "current-ns",
		},
		{
			name:          "kubeconfig lookup failure is propagated and not cached",
			kubeClient:    &countingKubeClient{MockKubeClient: MockKubeClient{Err: fmt.Errorf("kubeclient error")}},
			wantErr:       true,
			wantKubeCalls: 1,
			wantCached:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GKEOrchestrator{kubeClient: tt.kubeClient, namespace: tt.cached}
			got, err := g.getCurrentNamespace("cluster", "us-central1", "project")
			if (err != nil) != tt.wantErr {
				t.Fatalf("getCurrentNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("getCurrentNamespace() = %q, want %q", got, tt.want)
			}
			if tt.kubeClient.calls != tt.wantKubeCalls {
				t.Errorf("GetCurrentNamespace calls = %d, want %d", tt.kubeClient.calls, tt.wantKubeCalls)
			}
			if g.namespace != tt.wantCached {
				t.Errorf("cached g.namespace = %q, want %q", g.namespace, tt.wantCached)
			}

			// A second call must be served from cache without another lookup.
			if !tt.wantErr {
				if _, err := g.getCurrentNamespace("cluster", "us-central1", "project"); err != nil {
					t.Fatalf("second getCurrentNamespace() error = %v", err)
				}
				if tt.kubeClient.calls != tt.wantKubeCalls {
					t.Errorf("second call triggered kubeconfig lookup: calls = %d, want %d", tt.kubeClient.calls, tt.wantKubeCalls)
				}
			}
		})
	}
}

func TestValidateNamespaceExists(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		kubeClient    KubeClient
		dynClient     dynamic.Interface
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:          "kubeconfig namespace lookup failure is propagated",
			namespace:     "",
			kubeClient:    &MockKubeClient{Err: fmt.Errorf("failed to initialize Kubernetes client")},
			dynClient:     nil,
			wantErr:       true,
			wantErrSubstr: "failed to initialize Kubernetes client",
		},
		{
			name:       "namespace exists",
			namespace:  "exists",
			kubeClient: &MockKubeClient{Namespace: "exists"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Namespace"}}, nil
				},
			},
			wantErr: false,
		},
		{
			name:       "explicit --gke-namespace override wins over kubeconfig namespace",
			namespace:  "custom-job-ns",
			kubeClient: &MockKubeClient{Namespace: "default-kube-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					if name != "custom-job-ns" {
						return nil, fmt.Errorf("expected Get for custom-job-ns, got %s", name)
					}
					return &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Namespace"}}, nil
				},
			},
			wantErr: false,
		},
		{
			name:       "falls back to kubeconfig namespace when --gke-namespace unset",
			namespace:  "",
			kubeClient: &MockKubeClient{Namespace: "kube-ctx-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					if name != "kube-ctx-ns" {
						return nil, fmt.Errorf("expected Get for kube-ctx-ns, got %s", name)
					}
					return &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Namespace"}}, nil
				},
			},
			wantErr: false,
		},
		{
			name:       "namespace does not exist",
			namespace:  "nonexistent",
			kubeClient: &MockKubeClient{Namespace: "nonexistent"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name)
				},
			},
			wantErr:       true,
			wantErrSubstr: `target namespace "nonexistent" does not exist on GKE cluster "test-cluster"`,
		},
		{
			name:          "empty namespace",
			namespace:     "",
			kubeClient:    &MockKubeClient{ExplicitEmpty: true},
			dynClient:     &mockDynamicClient{},
			wantErr:       true,
			wantErrSubstr: `target namespace cannot be empty for GKE cluster "test-cluster". Please pass --gke-namespace, or set a default namespace in your kubeconfig context`,
		},
		{
			name:       "403 forbidden (RBAC restricted user proceeds with warning)",
			namespace:  "restricted-ns",
			kubeClient: &MockKubeClient{Namespace: "restricted-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, name, fmt.Errorf("user cannot get resource"))
				},
			},
			wantErr: false,
		},
		{
			name:       "403 forbidden string error (RBAC restricted user proceeds with warning)",
			namespace:  "restricted-string-ns",
			kubeClient: &MockKubeClient{Namespace: "restricted-string-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, fmt.Errorf("Error from server (Forbidden): namespaces %q is forbidden", name)
				},
			},
			wantErr: false,
		},
		{
			name:       "other API error is wrapped",
			namespace:  "some-ns",
			kubeClient: &MockKubeClient{Namespace: "some-ns"},
			dynClient: &mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					return nil, fmt.Errorf("connection refused")
				},
			},
			wantErr:       true,
			wantErrSubstr: `failed to verify existence of namespace "some-ns" on cluster "test-cluster": connection refused`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mirrors SubmitJob, which seeds g.namespace from job.GKENamespace.
			orch := &GKEOrchestrator{
				kubeClient: tt.kubeClient,
				dynClient:  tt.dynClient,
				namespace:  tt.namespace,
			}

			err := orch.validateTargetNamespaceExists("test-cluster", "us-central1-a", "test-project")

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, but got nil")
				} else if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})
	}
}

func TestValidateMTCConfig(t *testing.T) {
	var ksaGetCalled bool
	tests := []struct {
		name          string
		job           *orchestrator.JobDefinition
		clusterDesc   gkeCluster
		dynClient     dynamic.Interface
		kubeClient    KubeClient
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "MTC Disabled - Pass",
			job:  &orchestrator.JobDefinition{GKEMTCEnabled: false},
		},
		{
			name:          "MTC Enabled without HighScaleCheckpointing Addon - Fail",
			job:           &orchestrator.JobDefinition{GKEMTCEnabled: true, GKEMTCRamdiskDirectory: "/tmp/ramdisk"},
			wantErr:       true,
			wantErrSubstr: "HighScaleCheckpointing addon",
		},
		{
			name: "MTC Enabled with DryRunManifest - Pass without k8s client",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				DryRunManifest:         "manifest.yaml",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
		},
		{
			name: "MTC Enabled with CheckpointConfiguration CR present - Pass",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{
						Items: []unstructured.Unstructured{
							{Object: map[string]interface{}{"kind": "CheckpointConfiguration"}},
						},
					}, nil
				},
			},
		},
		{
			name: "MTC Enabled with CheckpointConfiguration CR missing - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
				},
			},
			wantErr:       true,
			wantErrSubstr: "requires a CheckpointConfiguration resource",
		},
		{
			name: "MTC Enabled with 403 Forbidden - Pass with warning",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "restricted-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "checkpointconfigurations"}, "test", fmt.Errorf("forbidden"))
				},
			},
		},
		{
			name: "MTC Enabled with CheckpointConfiguration CRD Unregistered - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "checkpointconfigurations"}, "")
				},
			},
			wantErr:       true,
			wantErrSubstr: "is not registered on the cluster",
		},
		{
			name: "MTC Enabled with generic k8s client error - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				GKENamespace:           "custom-ns",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, fmt.Errorf("internal server error")
				},
			},
			wantErr:       true,
			wantErrSubstr: "failed to verify CheckpointConfiguration resource",
		},
		{
			name: "Dry-run without HighScaleCheckpointing addon - Fail",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
				DryRunManifest:         "manifest.yaml",
			},
			wantErr:       true,
			wantErrSubstr: "enable_multi_tier_checkpointing: true",
		},
		{
			name: "CheckpointConfiguration present and legacy KSA annotation - Pass and checks KSA",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/tmp/ramdisk",
			},
			clusterDesc: gkeCluster{
				AddonsConfig: &gkeAddonsConfig{
					HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
				},
			},
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{
						Items: []unstructured.Unstructured{
							{Object: map[string]interface{}{"kind": "CheckpointConfiguration"}},
						},
					}, nil
				},
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					ksaGetCalled = true
					if name != mtcDriverKSA {
						t.Errorf("unexpected Get(%q), want %q", name, mtcDriverKSA)
					}
					ksa := &unstructured.Unstructured{Object: map[string]interface{}{}}
					ksa.SetAnnotations(map[string]string{"iam.gke.io/gcp-service-account": "node-sa@test-project.iam.gserviceaccount.com"})
					return ksa, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				clusterDesc:      tt.clusterDesc,
				dynClient:        tt.dynClient,
				kubeClient:       tt.kubeClient,
				kubeconfigLoader: staticLoader(clientcmdapi.NewConfig()),
			}
			err := orc.validateMTCConfig(tt.job)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, but got nil")
				} else if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})
	}
	if !ksaGetCalled {
		t.Errorf("expected Get(%q) to be called", mtcDriverKSA)
	}
}

// recordingDynamicClient is a mockDynamicClient that records the resource and
// namespace of the last request.
type recordingDynamicClient struct {
	mockDynamicClient
	resource  schema.GroupVersionResource
	namespace string
}

func (r *recordingDynamicClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	r.resource = gvr
	return &recordingResource{mockNamespaceableResource{getFunc: r.getFunc, listFunc: r.listFunc}, r}
}

type recordingResource struct {
	mockNamespaceableResource
	client *recordingDynamicClient
}

func (r *recordingResource) Namespace(ns string) dynamic.ResourceInterface {
	r.client.namespace = ns
	return &r.mockNamespaceableResource
}

func TestLegacyMTCDriverGSA(t *testing.T) {
	const ksaNamespace, ksaName = "gke-managed-checkpointing", "gke-checkpointing-multitier-node"
	ksaGVR := schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}
	ksaWith := func(annotations map[string]string) *unstructured.Unstructured {
		ksa := &unstructured.Unstructured{Object: map[string]interface{}{}}
		ksa.SetAnnotations(annotations)
		return ksa
	}

	tests := []struct {
		name     string
		noClient bool
		ksa      *unstructured.Unstructured
		getErr   error
		want     string
	}{
		{
			name: "Annotated KSA returns the GSA",
			ksa:  ksaWith(map[string]string{"iam.gke.io/gcp-service-account": "node-sa@test-project.iam.gserviceaccount.com"}),
			want: "node-sa@test-project.iam.gserviceaccount.com",
		},
		{
			name: "KSA without the annotation returns empty",
			ksa:  ksaWith(map[string]string{"unrelated": "value"}),
		},
		{
			name:   "Missing KSA returns empty",
			getErr: apierrors.NewNotFound(schema.GroupResource{Resource: "serviceaccounts"}, ksaName),
		},
		{
			name:   "Forbidden returns empty",
			getErr: apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, ksaName, fmt.Errorf("forbidden")),
		},
		{
			name:     "No usable kubeconfig returns empty",
			noClient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{kubeconfigLoader: staticLoader(clientcmdapi.NewConfig())}
			var gotName string
			client := &recordingDynamicClient{mockDynamicClient: mockDynamicClient{
				getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
					gotName = name
					return tt.ksa, tt.getErr
				},
			}}
			if !tt.noClient {
				orc.dynClient = client
			}

			if got := orc.legacyMTCDriverGSA(); got != tt.want {
				t.Errorf("legacyMTCDriverGSA() = %q, want %q", got, tt.want)
			}
			if tt.noClient {
				return
			}
			if client.resource != ksaGVR || client.namespace != ksaNamespace || gotName != ksaName {
				t.Errorf("Get() targeted %v %s/%s, want %v %s/%s", client.resource, client.namespace, gotName, ksaGVR, ksaNamespace, ksaName)
			}
		})
	}
}

func TestIsForbiddenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Nil error returns false",
			err:  nil,
			want: false,
		},
		{
			name: "Typed 403 Forbidden returns true",
			err:  apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, "test-svc", fmt.Errorf("forbidden")),
			want: true,
		},
		{
			name: "Generic error with forbidden substring returns true",
			err:  fmt.Errorf("Error from server (Forbidden): request forbidden"),
			want: true,
		},
		{
			name: "Other error returns false",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isForbiddenError(tt.err); got != tt.want {
				t.Errorf("isForbiddenError(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestVerifyCheckpointConfigurationCR(t *testing.T) {
	const docRemediation = "Please follow the documentation."

	tests := []struct {
		name          string
		dynClient     dynamic.Interface
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "Success - CR exists",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{
						Items: []unstructured.Unstructured{
							{
								Object: map[string]interface{}{
									"metadata": map[string]interface{}{"name": "default"},
								},
							},
						},
					}, nil
				},
			},
			wantErr: false,
		},
		{
			name: "CRD Not Found - returns error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "checkpointconfigurations"}, "")
				},
			},
			wantErr:       true,
			wantErrSubstr: "the CheckpointConfiguration CustomResourceDefinition (CRD) is not registered on the cluster",
		},
		{
			name: "403 Forbidden typed error - proceeds without error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "checkpointconfigurations"}, "", fmt.Errorf("user cannot list resource"))
				},
			},
			wantErr: false,
		},
		{
			name: "403 Forbidden string error - proceeds without error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return nil, fmt.Errorf("Error from server (Forbidden): checkpointconfigurations.checkpointing.gke.io is forbidden")
				},
			},
			wantErr: false,
		},
		{
			name: "0 CR items - returns error",
			dynClient: &mockDynamicClient{
				listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
					return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}, nil
				},
			},
			wantErr:       true,
			wantErrSubstr: "Multi-Tier Checkpointing (MTC) requires a CheckpointConfiguration resource to be deployed on the cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				dynClient: tt.dynClient,
			}
			err := orc.verifyCheckpointConfigurationCR(&orchestrator.JobDefinition{}, docRemediation)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, but got nil")
				} else if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("expected error to contain %q, but got: %v", tt.wantErrSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, but got: %v", err)
			}
		})
	}
}

func TestValidateClusterState_DedupesReadinessReads(t *testing.T) {
	versionProbeCalls := 0
	getDepCalls := 0
	rolloutCalls := 0
	priorityClassListCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "/version") {
				versionProbeCalls++
				return shell.CommandResult{ExitCode: 0}
			}
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 0, Stdout: "found"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment kueue-controller-manager") {
				getDepCalls++
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"metadata":{"generation":1},"spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"manager","image":"registry.k8s.io/kueue/kueue:v0.16.1"}]}}},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":1}}`,
				}
			}
			if strings.Contains(fullCmd, "kubectl rollout status") {
				rolloutCalls++
				return shell.CommandResult{ExitCode: 0}
			}
			if strings.Contains(fullCmd, "kubectl get endpointslice") {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"items":[{"endpoints":[{"addresses":["10.4.1.3"],"conditions":{"ready":true}}]}]}`,
				}
			}
			if strings.HasPrefix(fullCmd, "kubectl get priorityclass") {
				priorityClassListCalls++
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   "system-node-critical high medium",
				}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := newTestGKEOrchestrator(mock)
	orc.namespace = "default"
	orc.dynClient = &mockDynamicClient{
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		},
	}
	job := &orchestrator.JobDefinition{
		ClusterName:       "test-cluster",
		ClusterLocation:   "us-central1-a",
		ProjectID:         "test-project",
		PriorityClassName: "high",
	}
	if err := orc.ValidateClusterState(job); err != nil {
		t.Fatalf("ValidateClusterState failed: %v", err)
	}
	if versionProbeCalls != 0 {
		t.Errorf("ValidateClusterState /version probe calls = %d, want 0 (already probed in configureKubectl)", versionProbeCalls)
	}
	if getDepCalls != 1 {
		t.Errorf("ValidateClusterState kueue deployment get calls = %d, want 1", getDepCalls)
	}
	if rolloutCalls != 0 {
		t.Errorf("ValidateClusterState kueue rollout status calls = %d, want 0 when healthy", rolloutCalls)
	}
	if priorityClassListCalls != 1 {
		t.Errorf("ValidateClusterState priorityclass list calls = %d, want 1", priorityClassListCalls)
	}
}

func TestValidateCheckpointConfigurationCompatibility(t *testing.T) {
	oldInterval := mtcDaemonSetPollInterval
	oldTimeout := mtcDaemonSetPollTimeout
	mtcDaemonSetPollInterval = time.Millisecond
	mtcDaemonSetPollTimeout = 200 * time.Millisecond
	defer func() {
		mtcDaemonSetPollInterval = oldInterval
		mtcDaemonSetPollTimeout = oldTimeout
	}()

	tpuToleration := map[string]interface{}{
		"key":      "google.com/tpu",
		"operator": "Exists",
		"effect":   "NoSchedule",
	}
	gpuToleration := map[string]interface{}{
		"key":      "nvidia.com/gpu",
		"operator": "Equal",
		"value":    "present",
		"effect":   "NoSchedule",
	}
	gpuPool := gkeJobNodePool{
		Name: "gpu-np",
		Config: gkeNodePoolConfig{
			MachineType:  "a3-highgpu-8g",
			Accelerators: []gkeAccelerator{{AcceleratorType: "nvidia-h100-80gb", AcceleratorCount: "8"}},
		},
	}
	a3Caps := MachineTypeCap{Accelerators: []struct {
		Count int    `json:"guestAcceleratorCount"`
		Type  string `json:"guestAcceleratorType"`
	}{{Count: 8, Type: "nvidia-h100-80gb"}}}
	readyDaemonSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "multitier-driver-uid-123"},
			"status": map[string]interface{}{
				"desiredNumberScheduled": int64(4),
				"numberReady":            int64(4),
			},
		},
	}

	var transientCallCount int
	var transientAPICallCount int
	var obsGenCallCount int
	var unauthorizedCallCount int
	var noStatusCallCount int

	tests := []struct {
		name          string
		job           *orchestrator.JobDefinition
		clusterDesc   gkeCluster
		machineCaps   map[string]MachineTypeCap
		cr            *unstructured.Unstructured
		getDS         func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error)
		checkCalls    func(t *testing.T)
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "nodeSelector instance-type mismatch fails",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default"},
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"node.kubernetes.io/instance-type": "ct5lp-hightpu-4t",
						},
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: `targets node.kubernetes.io/instance-type="ct5lp-hightpu-4t" in spec.nodeSelector, which does not match the job's machine type "ct6e-standard-4t"`,
		},
		{
			name: "nodeSelector gke-nodepool mismatch fails",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{
						Name:   "tpu-np-0",
						Config: gkeNodePoolConfig{MachineType: "ct6e-standard-4t"},
					},
				},
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default"},
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"cloud.google.com/gke-nodepool": "other-pool",
						},
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: `targets cloud.google.com/gke-nodepool="other-pool" in spec.nodeSelector, which does not match the job's node pool "tpu-np-0"`,
		},
		{
			name: "missing TPU toleration fails",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default"},
					"spec":     map[string]interface{}{},
				},
			},
			wantErr:       true,
			wantErrSubstr: `is missing a toleration for taint "google.com/tpu=present:NoSchedule"`,
		},
		{
			name: "missing GPU toleration on a GPU node pool fails",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "a3-highgpu-8g",
			},
			clusterDesc: gkeCluster{NodePools: []gkeJobNodePool{gpuPool}},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default"},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: `is missing a toleration for taint "nvidia.com/gpu=present:NoSchedule"`,
		},
		{
			name: "missing GPU toleration for a machine type with accelerators and no node pool fails",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "a3-highgpu-8g",
			},
			machineCaps: map[string]MachineTypeCap{"a3-highgpu-8g:": a3Caps},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default"},
					"spec":     map[string]interface{}{},
				},
			},
			wantErr:       true,
			wantErrSubstr: `is missing a toleration for taint "nvidia.com/gpu=present:NoSchedule"`,
		},
		{
			name: "missing custom node pool taint toleration fails",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{
						Name: "tpu-np-0",
						Config: gkeNodePoolConfig{
							MachineType: "ct6e-standard-4t",
							Taints: []gkeTaint{
								{Key: "dedicated", Value: "ml-team", Effect: "NoSchedule"},
							},
						},
					},
				},
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default"},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: `is missing a toleration for taint "dedicated=ml-team:NoSchedule"`,
		},
		{
			name: "multitier-driver DaemonSet not found fails after polling timeout",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name)
			},
			wantErr:       true,
			wantErrSubstr: `the MTC CSI driver DaemonSet "multitier-driver-uid-123" was not found`,
		},
		{
			name: "multitier-driver DaemonSet desired=0 on static pool fails after polling timeout",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{
						Name:             "tpu-np-0",
						InitialNodeCount: 4,
						Config:           gkeNodePoolConfig{MachineType: "ct6e-standard-4t"},
					},
				},
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"name": name},
						"status": map[string]interface{}{
							"desiredNumberScheduled": int64(0),
							"numberReady":            int64(0),
						},
					},
				}, nil
			},
			wantErr:       true,
			wantErrSubstr: "has 0 desired pods (desiredNumberScheduled=0)",
		},
		{
			name: "multitier-driver DaemonSet desired>0 and ready=0 fails after polling timeout",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"name": name},
						"status": map[string]interface{}{
							"desiredNumberScheduled": int64(4),
							"numberReady":            int64(0),
						},
					},
				}, nil
			},
			wantErr:       true,
			wantErrSubstr: "has 0 of 4 pods ready",
		},
		{
			name: "multitier-driver DaemonSet with 3 of 4 pods ready fails after polling timeout",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default", "uid": "uid-123"},
					"spec":     map[string]interface{}{"tolerations": []interface{}{tpuToleration}},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"name": name},
						"status":   map[string]interface{}{"desiredNumberScheduled": int64(4), "numberReady": int64(3)},
					},
				}, nil
			},
			wantErr:       true,
			wantErrSubstr: "has 3 of 4 pods ready",
		},
		{
			name: "multitier-driver DaemonSet desired=0 on autoscaling pool scaled to zero passes",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{{
					Name:             "tpu-np-0",
					InitialNodeCount: 1,
					Autoscaling:      gkeAutoscaling{Enabled: true, MinNodeCount: 0, MaxNodeCount: 4},
					Config:           gkeNodePoolConfig{MachineType: "ct6e-standard-4t"},
				}},
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default", "uid": "uid-123"},
					"spec":     map[string]interface{}{"tolerations": []interface{}{tpuToleration}},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"name": name},
						"status":   map[string]interface{}{"desiredNumberScheduled": int64(0), "numberReady": int64(0)},
					},
				}, nil
			},
			wantErr: false,
		},
		{
			name: "multitier-driver DaemonSet whose status is never populated fails after polling timeout",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default", "uid": "uid-123"},
					"spec":     map[string]interface{}{"tolerations": []interface{}{tpuToleration}},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				noStatusCallCount++
				return &unstructured.Unstructured{
					Object: map[string]interface{}{"metadata": map[string]interface{}{"name": name, "generation": int64(1)}},
				}, nil
			},
			checkCalls: func(t *testing.T) {
				if noStatusCallCount < 2 {
					t.Errorf("expected polling while the DaemonSet status is missing, got %d calls", noStatusCallCount)
				}
			},
			wantErr:       true,
			wantErrSubstr: "is still reconciling generation 1 (observedGeneration=0)",
		},
		{
			name: "multitier-driver DaemonSet becomes ready during polling passes",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				transientCallCount++
				if transientCallCount == 1 {
					return &unstructured.Unstructured{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{"name": name},
							"status": map[string]interface{}{
								"desiredNumberScheduled": int64(4),
								"numberReady":            int64(0),
							},
						},
					}, nil
				}
				return readyDaemonSet, nil
			},
			wantErr: false,
		},
		{
			name: "non-NotFound error on DaemonSet Get is retried and returned after the polling timeout",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				unauthorizedCallCount++
				return nil, apierrors.NewUnauthorized("token expired")
			},
			checkCalls: func(t *testing.T) {
				if unauthorizedCallCount < 2 {
					t.Errorf("expected the error to be retried, got %d calls", unauthorizedCallCount)
				}
			},
			wantErr:       true,
			wantErrSubstr: "failed to verify MTC driver DaemonSet gke-managed-checkpointing/multitier-driver-uid-123: token expired",
		},
		{
			name: "retryable 503 ServiceUnavailable on DaemonSet Get polls and recovers",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				transientAPICallCount++
				if transientAPICallCount == 1 {
					return nil, apierrors.NewServiceUnavailable("apiserver temporarily unavailable")
				}
				return readyDaemonSet, nil
			},
			wantErr: false,
		},
		{
			name: "observedGeneration < generation polls until generation is observed",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				obsGenCallCount++
				obsGen := int64(1)
				if obsGenCallCount > 1 {
					obsGen = 2
				}
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":       name,
							"generation": int64(2),
						},
						"status": map[string]interface{}{
							"observedGeneration":     obsGen,
							"desiredNumberScheduled": int64(4),
							"numberReady":            int64(4),
						},
					},
				}, nil
			},
			wantErr: false,
		},
		{
			name: "matching GPU toleration and ready DaemonSet passes",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "a3-highgpu-8g",
			},
			clusterDesc: gkeCluster{NodePools: []gkeJobNodePool{gpuPool}},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"node.kubernetes.io/instance-type": "a3-highgpu-8g",
						},
						"tolerations": []interface{}{gpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return readyDaemonSet, nil
			},
			wantErr: false,
		},
		{
			name: "CheckpointConfiguration pinned to the second node pool with the job's machine type passes",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{Name: "tpu-np-0", Config: gkeNodePoolConfig{MachineType: "ct6e-standard-4t"}},
					{Name: "tpu-np-1", Config: gkeNodePoolConfig{MachineType: "ct6e-standard-4t"}},
				},
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default", "uid": "uid-123"},
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{"cloud.google.com/gke-nodepool": "tpu-np-1"},
						"tolerations":  []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return readyDaemonSet, nil
			},
			wantErr: false,
		},
		{
			name: "multi-pool: surfaces missing toleration error from pool matching nodeSelector instead of nodeSelector error from second pool",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{Name: "tpu-np-0", Config: gkeNodePoolConfig{MachineType: "ct6e-standard-4t"}},
					{Name: "tpu-np-1", Config: gkeNodePoolConfig{MachineType: "ct6e-standard-4t"}},
				},
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default", "uid": "uid-123"},
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{"cloud.google.com/gke-nodepool": "tpu-np-0"},
					},
				},
			},
			wantErr:       true,
			wantErrSubstr: `missing a toleration for taint "google.com/tpu=present:NoSchedule"`,
		},
		{
			name: "multi-pool: desired=0 fails when covered pool is fixed-size even if uncovered pool scales from zero",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			clusterDesc: gkeCluster{
				NodePools: []gkeJobNodePool{
					{
						Name:        "tpu-np-0",
						Config:      gkeNodePoolConfig{MachineType: "ct6e-standard-4t"},
						Autoscaling: gkeAutoscaling{Enabled: true, MinNodeCount: 0},
					},
					{
						Name:        "tpu-np-1",
						Config:      gkeNodePoolConfig{MachineType: "ct6e-standard-4t"},
						Autoscaling: gkeAutoscaling{Enabled: false},
					},
				},
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "default", "uid": "uid-123"},
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{"cloud.google.com/gke-nodepool": "tpu-np-1"},
						"tolerations":  []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"name": name},
						"status": map[string]interface{}{
							"desiredNumberScheduled": int64(0),
							"numberReady":            int64(0),
						},
					},
				}, nil
			},
			wantErr:       true,
			wantErrSubstr: "has 0 desired pods (desiredNumberScheduled=0)",
		},
		{
			name: "403 Forbidden on DaemonSet Get proceeds with warning",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, name, fmt.Errorf("forbidden"))
			},
			wantErr: false,
		},
		{
			name: "updatedNumberScheduled < desiredNumberScheduled fails after timeout",
			job: &orchestrator.JobDefinition{
				GKEMTCEnabled:          true,
				GKEMTCRamdiskDirectory: "/local/ramdisk",
				MachineType:            "ct6e-standard-4t",
			},
			cr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "default",
						"uid":  "uid-123",
					},
					"spec": map[string]interface{}{
						"tolerations": []interface{}{tpuToleration},
					},
				},
			},
			getDS: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
				return &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{"name": name},
						"status": map[string]interface{}{
							"desiredNumberScheduled": int64(4),
							"numberReady":            int64(4),
							"updatedNumberScheduled": int64(2),
						},
					},
				}, nil
			},
			wantErr:       true,
			wantErrSubstr: "has only 2 of 4 pods updated to the latest version; wait for the rollout to finish",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterDesc := tt.clusterDesc
			clusterDesc.AddonsConfig = &gkeAddonsConfig{
				HighScaleCheckpointingConfig: &gkeHighScaleCheckpointingConfig{Enabled: true},
			}
			orc := &GKEOrchestrator{
				clusterDesc:      clusterDesc,
				machineCapCache:  tt.machineCaps,
				dynClient:        newMTCTestDynamicClient(tt.cr, tt.getDS),
				kubeconfigLoader: staticLoader(clientcmdapi.NewConfig()),
			}

			err := orc.validateMTCConfig(tt.job)
			if tt.checkCalls != nil {
				tt.checkCalls(t)
			}
			assertWantErrSubstr(t, err, tt.wantErr, tt.wantErrSubstr)
		})
	}
}

func newMTCTestDynamicClient(
	cr *unstructured.Unstructured,
	getDS func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error),
) dynamic.Interface {
	return &mockDynamicClient{
		listFunc: func(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
			return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*cr}}, nil
		},
		getFunc: func(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
			if strings.HasPrefix(name, "multitier-driver") && getDS != nil {
				return getDS(ctx, name, options, subresources...)
			}
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "serviceaccounts"}, name)
		},
	}
}

func assertWantErrSubstr(t *testing.T, err error, wantErr bool, wantErrSubstr string) {
	t.Helper()
	if wantErr {
		if err == nil {
			t.Fatalf("expected error containing %q, got nil", wantErrSubstr)
		}
		if !strings.Contains(err.Error(), wantErrSubstr) {
			t.Errorf("expected error containing %q, got: %v", wantErrSubstr, err)
		}
	} else if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}
