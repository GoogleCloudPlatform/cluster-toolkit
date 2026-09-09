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
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
)

type mockExecutor struct {
	executeCommandFunc func(name string, args ...string) shell.CommandResult
}

func (m *mockExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	if m.executeCommandFunc != nil {
		return m.executeCommandFunc(name, args...)
	}
	return shell.CommandResult{ExitCode: 0}
}

func (m *mockExecutor) ExecuteCommandStream(name string, args ...string) error {
	return nil
}

type mockHTTPClient struct {
	getFunc func(url string) (*http.Response, error)
}

func (m *mockHTTPClient) Get(url string) (*http.Response, error) {
	if m.getFunc != nil {
		return m.getFunc(url)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: mock-manifest\n"))),
	}, nil
}

func TestConfigureClusterEnvironment_AutoCreateQueues(t *testing.T) {
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(prompt string) bool { return true }

	responses := map[string][]shell.CommandResult{
		"kubectl get localqueue default -n default": {
			{ExitCode: 1, Stderr: "Error from server (NotFound): localqueues.kueue.x-k8s.io \"default\" not found"},
		},
		"kubectl get clusterqueue default": {
			{ExitCode: 1, Stderr: "Error from server (NotFound): clusterqueues.kueue.x-k8s.io \"default\" not found"},
		},
		"kubectl get resourceflavor": {
			{ExitCode: 0, Stdout: ""},
		},
		"kubectl apply -f": {
			{ExitCode: 0, Stdout: "resourceflavor.kueue.x-k8s.io/flavor-default created"},
			{ExitCode: 0, Stdout: "clusterqueue.kueue.x-k8s.io/default created"},
			{ExitCode: 0, Stdout: "localqueue.kueue.x-k8s.io/default created"},
		},
		"kubectl get localqueue default -n default -o jsonpath={.spec.clusterQueue}": {
			{ExitCode: 0, Stdout: "default"},
		},
		"kubectl get clusterqueue default -o json": {
			{ExitCode: 0, Stdout: "{\"apiVersion\":\"kueue.x-k8s.io/v1beta2\",\"kind\":\"ClusterQueue\",\"spec\":{\"resourceGroups\":[{\"coveredResources\":[\"cpu\"]}]}}"},
		},
		"kubectl patch clusterqueue default": {
			{ExitCode: 0, Stdout: "clusterqueue.kueue.x-k8s.io/default patched"},
		},
	}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.capacity = ClusterCapacity{
		Flavors: map[string]FlavorCapacity{
			"flavor-default": {CPUs: 30},
		},
	}

	job := &orchestrator.JobDefinition{
		KueueQueueName: "default",
	}

	err := orc.configureClusterEnvironment(job)
	if err != nil {
		t.Fatalf("configureClusterEnvironment failed: %v", err)
	}

	if mockExec.callCount["kubectl apply -f"] != 3 {
		t.Errorf("Expected 3 calls to kubectl apply -f, got %d", mockExec.callCount["kubectl apply -f"])
	}
}

func TestCreateDefaultQueues_CustomNamespace(t *testing.T) {
	responses := map[string][]shell.CommandResult{
		"kubectl get resourceflavor": {
			{ExitCode: 1, Stderr: "Error from server (NotFound)"},
		},
		"kubectl get clusterqueue default": {
			{ExitCode: 1, Stderr: "Error from server (NotFound)"},
		},
		"kubectl apply -f": {
			{ExitCode: 0, Stdout: "resourceflavor created"},
			{ExitCode: 0, Stdout: "clusterqueue created"},
			{ExitCode: 0, Stdout: "localqueue created"},
		},
	}

	mockExec := NewMockExecutor(responses)
	orc := newTestGKEOrchestrator(mockExec)
	orc.capacity = ClusterCapacity{
		Flavors: map[string]FlavorCapacity{
			"flavor-default": {CPUs: 10},
		},
	}

	err := orc.createDefaultQueues("team-queue", "team-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveKueueQueue(t *testing.T) {
	tests := []struct {
		name          string
		requestedName string
		kubectlOutput string
		wantName      string
		wantErr       bool
	}{
		{
			name:          "User requested name",
			requestedName: "custom-q",
			kubectlOutput: "",
			wantName:      "custom-q",
			wantErr:       false,
		},
		{
			name:          "No queues found, fallback to default",
			requestedName: "",
			kubectlOutput: "",
			wantName:      "default",
			wantErr:       false,
		},
		{
			name:          "Single queue found, auto-discover",
			requestedName: "",
			kubectlOutput: "queue-1",
			wantName:      "queue-1",
			wantErr:       false,
		},
		{
			name:          "Multiple queues found, prefer default",
			requestedName: "",
			kubectlOutput: "multislice-queue default",
			wantName:      "default",
			wantErr:       false,
		},
		{
			name:          "Multiple queues found, fallback to multislice-queue",
			requestedName: "",
			kubectlOutput: "multislice-queue custom-q",
			wantName:      "multislice-queue",
			wantErr:       false,
		},
		{
			name:          "Multiple queues found without default or multislice-queue, hard fail",
			requestedName: "",
			kubectlOutput: "queue-1 queue-2",
			wantName:      "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses := map[string][]shell.CommandResult{
				"kubectl get localqueue -n default -o jsonpath={.items[*].metadata.name}": {
					{ExitCode: 0, Stdout: tt.kubectlOutput},
				},
			}
			mockExec := NewMockExecutor(responses)
			orc := newTestGKEOrchestrator(mockExec)

			got, err := orc.resolveKueueQueue(tt.requestedName, "default")
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveKueueQueue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantName {
				t.Errorf("resolveKueueQueue() got = %v, want %v", got, tt.wantName)
			}
		})
	}
}

func TestCheckClusterQueueCoverage(t *testing.T) {
	tests := []struct {
		name         string
		jsonOutput   string
		commandExit  int
		wantCoverage bool
		wantEmpty    bool
		wantErr      bool
	}{
		{
			name:         "Covers CPU and Memory",
			jsonOutput:   `{"spec":{"resourceGroups":[{"coveredResources":["cpu","memory"]}]}}`,
			commandExit:  0,
			wantCoverage: true,
			wantEmpty:    false,
			wantErr:      false,
		},
		{
			name:         "Only covers CPU",
			jsonOutput:   `{"spec":{"resourceGroups":[{"coveredResources":["cpu"]}]}}`,
			commandExit:  0,
			wantCoverage: false,
			wantEmpty:    false,
			wantErr:      false,
		},
		{
			name:         "Empty spec resourceGroups",
			jsonOutput:   `{"spec":{"resourceGroups":[]}}`,
			commandExit:  0,
			wantCoverage: false,
			wantEmpty:    true,
			wantErr:      false,
		},
		{
			name:         "Missing spec",
			jsonOutput:   `{}`,
			commandExit:  0,
			wantCoverage: false,
			wantEmpty:    true,
			wantErr:      false,
		},
		{
			name:        "Invalid JSON",
			jsonOutput:  `{invalid-json}`,
			commandExit: 0,
			wantErr:     true,
		},
		{
			name:        "Command error",
			jsonOutput:  "",
			commandExit: 1,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses := map[string][]shell.CommandResult{
				"kubectl get clusterqueue default -o json": {
					{ExitCode: tt.commandExit, Stdout: tt.jsonOutput, Stderr: "error getting clusterqueue"},
				},
			}
			mockExec := NewMockExecutor(responses)
			orc := newTestGKEOrchestrator(mockExec)

			gotCoverage, gotEmpty, err := orc.checkClusterQueueCoverage("default")
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkClusterQueueCoverage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if gotCoverage != tt.wantCoverage || gotEmpty != tt.wantEmpty {
					t.Errorf("checkClusterQueueCoverage() gotCoverage = %v (want %v), gotEmpty = %v (want %v)", gotCoverage, tt.wantCoverage, gotEmpty, tt.wantEmpty)
				}
			}
		})
	}
}

func TestGetClusterQueueName(t *testing.T) {
	tests := []struct {
		name     string
		stdout   string
		exitCode int
		wantCQ   string
		wantErr  bool
	}{
		{
			name:     "Explicit ClusterQueue name found",
			stdout:   "cq-custom",
			exitCode: 0,
			wantCQ:   "cq-custom",
			wantErr:  false,
		},
		{
			name:     "Empty ClusterQueue name falls back to LocalQueue name",
			stdout:   "",
			exitCode: 0,
			wantCQ:   "my-lq",
			wantErr:  false,
		},
		{
			name:     "Command error",
			stdout:   "",
			exitCode: 1,
			wantCQ:   "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses := map[string][]shell.CommandResult{
				"kubectl get localqueue my-lq -n default -o jsonpath={.spec.clusterQueue}": {
					{ExitCode: tt.exitCode, Stdout: tt.stdout, Stderr: "error"},
				},
			}
			mockExec := NewMockExecutor(responses)
			orc := newTestGKEOrchestrator(mockExec)

			got, err := orc.getClusterQueueName("my-lq", "default")
			if (err != nil) != tt.wantErr {
				t.Fatalf("getClusterQueueName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantCQ {
				t.Errorf("getClusterQueueName() got = %v, want %v", got, tt.wantCQ)
			}
		})
	}
}

func TestRenderClusterQueue(t *testing.T) {
	orc := &GKEOrchestrator{
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-default": {
					CPUs:     100,
					MemoryGi: 400,
				},
				"flavor-gpu-test": {
					CPUs: 10,
					GPUs: 10,
				},
			},
		},
	}

	bytes, err := orc.renderClusterQueue("cluster-queue")
	if err != nil {
		t.Fatalf("renderClusterQueue failed: %v", err)
	}

	output := string(bytes)

	if !strings.Contains(output, "nominalQuota: 100") {
		t.Errorf("expected nominalQuota: 100 for CPU, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 400Gi") {
		t.Errorf("expected nominalQuota: 400Gi for Memory, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 10") {
		t.Errorf("expected nominalQuota: 10 for GPU, got %s", output)
	}

	count := strings.Count(output, "coveredResources:")
	if count != 1 {
		t.Errorf("expected 1 coveredResources block, got %d. Output: %s", count, output)
	}
}

func TestRenderClusterQueue_Pathways(t *testing.T) {
	orc := &GKEOrchestrator{
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-tpu": {
					CPUs:     100,
					MemoryGi: 400,
					TPUs:     8,
				},
				"pathways-flavor": {
					CPUs:     480,
					MemoryGi: 2000,
					TPUs:     0,
				},
			},
		},
	}

	bytes, err := orc.renderClusterQueue("cluster-queue")
	if err != nil {
		t.Fatalf("renderClusterQueue failed: %v", err)
	}

	output := string(bytes)

	if !strings.Contains(output, "nominalQuota: \"999999\"") {
		t.Errorf("expected nominalQuota: \"999999\" for TPU flavor CPU, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 999999T") {
		t.Errorf("expected nominalQuota: 999999T for TPU flavor Memory, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 8") {
		t.Errorf("expected nominalQuota: 8 for TPU flavor TPUs, got %s", output)
	}

	if !strings.Contains(output, "nominalQuota: 480") {
		t.Errorf("expected nominalQuota: 480 for Pathways flavor CPU, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 2000Gi") {
		t.Errorf("expected nominalQuota: 2000Gi for Pathways flavor Memory, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 0") {
		t.Errorf("expected nominalQuota: 0 for Pathways flavor TPUs, got %s", output)
	}

	count := strings.Count(output, "coveredResources:")
	if count != 1 {
		t.Errorf("expected 1 coveredResources block for Pathways case (unified), got %d. Output: %s", count, output)
	}

	if !strings.Contains(output, "name: flavor-tpu") {
		t.Errorf("expected flavor-tpu in output, got %s", output)
	}
	if !strings.Contains(output, "name: pathways-flavor") {
		t.Errorf("expected pathways-flavor in output, got %s", output)
	}
}

func TestRenderClusterQueue_Empty(t *testing.T) {
	orc := &GKEOrchestrator{
		capacity: ClusterCapacity{},
	}

	bytes, err := orc.renderClusterQueue("cluster-queue")
	if err != nil {
		t.Fatalf("renderClusterQueue failed: %v", err)
	}

	output := string(bytes)

	if strings.Contains(output, "nominalQuota") {
		t.Errorf("expected no nominalQuota rendered when capacity is empty, got %s", output)
	}
}

func TestWaitForKueueWebhook_Success(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			if name == "kubectl" && args[0] == "get" && args[1] == "deployment" {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   "registry.k8s.io/kueue/kueue:v0.15.2",
				}
			}
			if name == "kubectl" && args[0] == "get" && args[1] == "endpointslice" {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"items":[{"endpoints":[{"addresses":["10.4.1.3"],"conditions":{"ready":true}}]}]}`,
				}
			}
			if name == "kubectl" && args[0] == "rollout" {
				return shell.CommandResult{ExitCode: 0}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor: mock,
	}

	err := orc.waitForKueueWebhook()
	if err != nil {
		t.Fatalf("waitForKueueWebhook failed: %v", err)
	}
}

func TestWaitForKueueWebhook_Success_OlderVersion(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			if name == "kubectl" && args[0] == "get" && args[1] == "deployment" {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   "registry.k8s.io/kueue/kueue:v0.11.1",
				}
			}
			if name == "kubectl" && args[0] == "get" && args[1] == "endpoints" {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"subsets":[{"addresses":[{"ip":"10.4.1.3"}]}]}`,
				}
			}
			if name == "kubectl" && args[0] == "rollout" {
				return shell.CommandResult{ExitCode: 0}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor: mock,
	}

	err := orc.waitForKueueWebhook()
	if err != nil {
		t.Fatalf("waitForKueueWebhook failed: %v", err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		version string
		major   int
		minor   int
		patch   int
	}{
		{"v0.15.2", 0, 15, 2},
		{"v0.11.1", 0, 11, 1},
		{"v0.6.3", 0, 6, 3},
		{"1.2.3", 1, 2, 3},
		{"v1.2", 1, 2, 0},
		{"v0.16.6-some-random-suffix", 0, 16, 6},
		{"v0.16.6+build123", 0, 16, 6},
		{"v0.16.6-rc1+build123", 0, 16, 6},
		{"invalid", 0, 0, 0},
	}

	for _, tc := range tests {
		major, minor, patch := parseVersion(tc.version)
		if major != tc.major || minor != tc.minor || patch != tc.patch {
			t.Errorf("parseVersion(%s) = (%d, %d, %d); want (%d, %d, %d)", tc.version, major, minor, patch, tc.major, tc.minor, tc.patch)
		}
	}
}

func TestCheckAndInstallKueue_ReinstallNeeded_LowVersion(t *testing.T) {
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(prompt string) bool { return true }

	deleteCalled := false

	matchers := []struct {
		pattern string
		res     shell.CommandResult
		action  func()
	}{
		{pattern: "kubectl delete crd", action: func() { deleteCalled = true }, res: shell.CommandResult{ExitCode: 0}},
		{pattern: "auth can-i", res: shell.CommandResult{ExitCode: 0, Stdout: "yes"}},
		{pattern: "kubectl get crd", res: shell.CommandResult{ExitCode: 0, Stdout: "clusterqueues.kueue.x-k8s.io found"}},
		{pattern: "jsonpath", res: shell.CommandResult{ExitCode: 0, Stdout: "registry.k8s.io/kueue/kueue:v0.12.0"}},
		{pattern: "kubectl get deployment", res: shell.CommandResult{ExitCode: 0, Stdout: "kueue-controller-manager found"}},
		{pattern: "kubectl get endpoints", res: shell.CommandResult{ExitCode: 0, Stdout: `{"subsets": [{"addresses": [{"ip": "10.0.0.1"}]}]}`}},
		{pattern: "kubectl get endpointslice", res: shell.CommandResult{ExitCode: 0, Stdout: `{"subsets": [{"addresses": [{"ip": "10.0.0.1"}]}]}`}},
		{pattern: "apply", res: shell.CommandResult{ExitCode: 0}},
		{pattern: "priorityclass", res: shell.CommandResult{ExitCode: 0}},
		{pattern: "rollout", res: shell.CommandResult{ExitCode: 0}},
	}

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			for _, m := range matchers {
				if strings.Contains(fullCmd, m.pattern) {
					if m.action != nil {
						m.action()
					}
					return m.res
				}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor:   mock,
		httpClient: &mockHTTPClient{},
	}

	err := orc.CheckAndInstallKueue("", "test-cluster", "us-central1-a")
	if err != nil {
		t.Fatalf("CheckAndInstallKueue failed: %v", err)
	}

	if !deleteCalled {
		t.Errorf("expected DeleteAllKueueResources to be called, but it wasn't")
	}
}

func TestEnsurePriorityClassesInstalled_Missing(t *testing.T) {
	applyCalled := false
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "kubectl get priorityclass") {
				return shell.CommandResult{ExitCode: 0, Stdout: "system-cluster-critical system-node-critical"}
			}
			if strings.Contains(fullCmd, "kubectl apply") && strings.Contains(fullCmd, "kueue_priority_classes.yaml") {
				applyCalled = true
				return shell.CommandResult{ExitCode: 0}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor: mock,
	}

	err := orc.ensurePriorityClassesInstalled()
	if err != nil {
		t.Fatalf("ensurePriorityClassesInstalled failed: %v", err)
	}

	if !applyCalled {
		t.Errorf("expected priority classes to be installed, but they weren't")
	}
}

func TestEnsurePriorityClassesInstalled_Present(t *testing.T) {
	applyCalled := false
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "kubectl get priorityclass") {
				return shell.CommandResult{ExitCode: 0, Stdout: "system-cluster-critical system-node-critical low"}
			}
			if strings.Contains(fullCmd, "kubectl apply") && strings.Contains(fullCmd, "kueue_priority_classes.yaml") {
				applyCalled = true
				return shell.CommandResult{ExitCode: 0}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor: mock,
	}

	err := orc.ensurePriorityClassesInstalled()
	if err != nil {
		t.Fatalf("ensurePriorityClassesInstalled failed: %v", err)
	}

	if applyCalled {
		t.Errorf("expected priority classes to be skipped, but they were installed")
	}
}

func TestHandleKueueReinstallation_UserDeclines(t *testing.T) {
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(prompt string) bool { return false }

	orc := &GKEOrchestrator{}

	err := orc.handleKueueReinstallation("v0.15.2", "Test reason")
	if err == nil {
		t.Fatal("expected error when user declines, got nil")
	}

	if !strings.Contains(err.Error(), "user declined to re-install Kueue") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRenderClusterQueue_NAP(t *testing.T) {
	orc := &GKEOrchestrator{
		napEnabled: true,
		napLimits: map[string]int64{
			"cpu":            1000,
			"memory":         4000,
			"nvidia.com/gpu": 80,
		},
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-default": {
					CPUs:     4,
					MemoryGi: 16,
				},
				"nvidia-tesla-a100": {
					GPUs: 2,
				},
			},
		},
	}

	bytes, err := orc.renderClusterQueue("cluster-queue")
	if err != nil {
		t.Fatalf("renderClusterQueue failed: %v", err)
	}

	output := string(bytes)

	if !strings.Contains(output, "nominalQuota: 1000") {
		t.Errorf("expected nominalQuota: 1000 for CPU, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 4000G") {
		t.Errorf("expected nominalQuota: 4000G for Memory, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 80") {
		t.Errorf("expected nominalQuota: 80 for GPU, got %s", output)
	}
}

func TestCheckAndInstallKueue_PermissionDenied(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "auth can-i") {
				if strings.Contains(fullCmd, "clusterroles") {
					return shell.CommandResult{ExitCode: 0, Stdout: "no"}
				}
				return shell.CommandResult{ExitCode: 0, Stdout: "yes"}
			}
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 0, Stdout: "clusterqueues.kueue.x-k8s.io found"}
			}
			if strings.Contains(fullCmd, "jsonpath") {
				return shell.CommandResult{ExitCode: 0, Stdout: "registry.k8s.io/kueue/kueue:v0.12.0"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment") {
				return shell.CommandResult{ExitCode: 0, Stdout: "kueue-controller-manager found"}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor: mock,
	}

	err := orc.CheckAndInstallKueue("", "test-cluster", "us-central1-a")
	if err == nil {
		t.Fatal("expected error due to insufficient permissions, got nil")
	}

	expectedErr := "unable to re-install Kueue to version"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error containing %q, got: %v", expectedErr, err)
	}
}

func TestCheckAndInstallKueue_PermissionGranted(t *testing.T) {
	deleteCalled := false
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "auth can-i") {
				return shell.CommandResult{ExitCode: 0, Stdout: "yes"}
			}
			if strings.Contains(fullCmd, "kubectl delete crd") {
				deleteCalled = true
				return shell.CommandResult{ExitCode: 0}
			}
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 0, Stdout: "clusterqueues.kueue.x-k8s.io found"}
			}
			if strings.Contains(fullCmd, "jsonpath") {
				return shell.CommandResult{ExitCode: 0, Stdout: "registry.k8s.io/kueue/kueue:v0.12.0"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment") {
				return shell.CommandResult{ExitCode: 0, Stdout: "kueue-controller-manager found"}
			}
			if strings.Contains(fullCmd, "kubectl get endpoints") || strings.Contains(fullCmd, "kubectl get endpointslice") {
				return shell.CommandResult{ExitCode: 0, Stdout: `{"subsets": [{"addresses": [{"ip": "10.0.0.1"}]}]}`}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(prompt string) bool { return true }

	orc := &GKEOrchestrator{
		executor:   mock,
		httpClient: &mockHTTPClient{},
	}

	err := orc.CheckAndInstallKueue("", "test-cluster", "us-central1-a")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !deleteCalled {
		t.Errorf("expected DeleteAllKueueResources to be called, but it wasn't")
	}
}

func TestValidatePriorityClass_Empty(t *testing.T) {
	orc := &GKEOrchestrator{}
	err := orc.validatePriorityClass("")
	if err != nil {
		t.Fatalf("expected no error for empty priority, got %v", err)
	}
}

func TestValidatePriorityClass_Exists(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			if strings.Contains(name+" "+strings.Join(args, " "), "kubectl get priorityclass") {
				return shell.CommandResult{ExitCode: 0, Stdout: "system-cluster-critical low medium high"}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}
	orc := &GKEOrchestrator{executor: mock}
	err := orc.validatePriorityClass("medium")
	if err != nil {
		t.Fatalf("expected no error for existing priority, got %v", err)
	}
}

func TestValidatePriorityClass_NotExist(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			if strings.Contains(name+" "+strings.Join(args, " "), "kubectl get priorityclass") {
				return shell.CommandResult{ExitCode: 0, Stdout: "system-cluster-critical low high"}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}
	orc := &GKEOrchestrator{executor: mock}
	err := orc.validatePriorityClass("medium")
	if err == nil {
		t.Fatal("expected error for non-existing priority, got nil")
	}
	expected := `priority class "medium" does not exist in the cluster`
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to contain %q, got %v", expected, err)
	}
}

func TestRenderResourceFlavor_TopologyFiltering(t *testing.T) {
	orc := &GKEOrchestrator{}

	inputLabels := map[string]string{
		"cloud.google.com/gke-tpu-accelerator":      "tpu7x",
		"cloud.google.com/gke-nodepool":             "tpu-pool",
		"cloud.google.com/gke-tpu-topology":         "4x4x4",
		"cloud.google.com/gke-tpu-slice-1x1-id":     "some-id",
		"cloud.google.com/gke-tpu-partition-2x2-id": "some-partition-id",
	}

	bytes, err := orc.renderResourceFlavor("flavor-tpu7x", inputLabels)
	if err != nil {
		t.Fatalf("renderResourceFlavor failed: %v", err)
	}

	output := string(bytes)

	if !strings.Contains(output, "cloud.google.com/gke-tpu-accelerator: tpu7x") {
		t.Errorf("expected cloud.google.com/gke-tpu-accelerator to be present, got:\n%s", output)
	}
	if !strings.Contains(output, "cloud.google.com/gke-nodepool: tpu-pool") {
		t.Errorf("expected cloud.google.com/gke-nodepool to be present, got:\n%s", output)
	}

	if strings.Contains(output, "cloud.google.com/gke-tpu-topology") {
		t.Errorf("cloud.google.com/gke-tpu-topology should be filtered out, got:\n%s", output)
	}
	if strings.Contains(output, "cloud.google.com/gke-tpu-slice-") {
		t.Errorf("cloud.google.com/gke-tpu-slice-* labels should be filtered out, got:\n%s", output)
	}
	if strings.Contains(output, "cloud.google.com/gke-tpu-partition-") {
		t.Errorf("cloud.google.com/gke-tpu-partition-* labels should be filtered out, got:\n%s", output)
	}
}

func TestEnsureResourceFlavors_ExistingSkipped(t *testing.T) {
	applyCount := 0
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "kubectl get resourceflavor") {
				return shell.CommandResult{ExitCode: 0, Stdout: "flavor-default flavor-tpu7x"}
			}
			if strings.Contains(fullCmd, "kubectl apply") {
				applyCount++
				return shell.CommandResult{ExitCode: 0}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor: mock,
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-default": {CPUs: 10},
				"flavor-tpu7x":   {TPUs: 8},
			},
		},
	}

	err := orc.EnsureResourceFlavors()
	if err != nil {
		t.Fatalf("EnsureResourceFlavors failed: %v", err)
	}

	if applyCount != 0 {
		t.Errorf("expected 0 apply calls for pre-existing flavors, got %d", applyCount)
	}
}

func TestCheckClusterQueueExists_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): clusterqueues.kueue.x-k8s.io \"default\" is forbidden"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	exists, err := orc.checkClusterQueueExists("default")
	if err == nil {
		t.Fatalf("checkClusterQueueExists should return error on Forbidden, got err = nil")
	}
	if exists {
		t.Errorf("checkClusterQueueExists on Forbidden = true; want false")
	}
}

func TestCreateDefaultQueues_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "kubectl get clusterqueue") {
				return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): clusterqueues.kueue.x-k8s.io \"default\" is forbidden"}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	err := orc.createDefaultQueues("default", "team-a")
	if err == nil {
		t.Fatalf("createDefaultQueues should fail fast on Forbidden, got err = nil")
	}
	if !strings.Contains(err.Error(), "restricted (403 Forbidden)") {
		t.Errorf("createDefaultQueues error = %v; want 403 Forbidden message", err)
	}
}

func TestIsKueueInstalled_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{
				ExitCode: 1,
				Stderr:   "Error from server (Forbidden): customresourcedefinitions.apiextensions.k8s.io \"clusterqueues.kueue.x-k8s.io\" is forbidden",
			}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	installed, err := orc.isKueueInstalled()
	if err != nil {
		t.Fatalf("isKueueInstalled should succeed and return true on 403 Forbidden, got err: %v", err)
	}
	if !installed {
		t.Errorf("isKueueInstalled on 403 Forbidden = false; want true")
	}
}

func TestIsKueueDeploymentInstalled_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{
				ExitCode: 1,
				Stderr:   "Error from server (Forbidden): deployments.apps \"kueue-controller-manager\" is forbidden",
			}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	installed, err := orc.isKueueDeploymentInstalled()
	if err != nil {
		t.Fatalf("isKueueDeploymentInstalled should succeed and return true on 403 Forbidden, got err: %v", err)
	}
	if !installed {
		t.Errorf("isKueueDeploymentInstalled on 403 Forbidden = false; want true")
	}
}

func TestCheckAndInstallKueue_VersionNormalization(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "auth can-i") {
				return shell.CommandResult{ExitCode: 0, Stdout: "yes"}
			}
			if strings.Contains(fullCmd, "jsonpath") {
				return shell.CommandResult{ExitCode: 0, Stdout: "registry.k8s.io/kueue/kueue:v0.12.0"}
			}
			if strings.Contains(fullCmd, "kubectl get crd") || strings.Contains(fullCmd, "kubectl get deployment") {
				return shell.CommandResult{ExitCode: 0, Stdout: "found"}
			}
			if strings.Contains(fullCmd, "kubectl get endpoints") || strings.Contains(fullCmd, "kubectl get endpointslice") {
				return shell.CommandResult{ExitCode: 0, Stdout: `{"subsets": [{"addresses": [{"ip": "10.0.0.1"}]}]}`}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(prompt string) bool { return true }

	orc := &GKEOrchestrator{
		executor:   mock,
		httpClient: &mockHTTPClient{},
	}

	// Pass version without 'v' prefix: "0.17.1"
	err := orc.CheckAndInstallKueue("0.17.1", "test-cluster", "us-central1-a")
	if err != nil {
		t.Fatalf("CheckAndInstallKueue failed for version without 'v' prefix: %v", err)
	}
}

func TestValidatePriorityClass_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): priorityclasses.scheduling.k8s.io is forbidden"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	err := orc.validatePriorityClass("high")
	if err != nil {
		t.Fatalf("validatePriorityClass should succeed and skip on 403 Forbidden, got: %v", err)
	}
}

func TestHasUserPriorityClasses_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): priorityclasses.scheduling.k8s.io is forbidden"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	hasUser, err := orc.hasUserPriorityClasses()
	if err != nil {
		t.Fatalf("hasUserPriorityClasses should return true and skip on 403 Forbidden, got error: %v", err)
	}
	if !hasUser {
		t.Errorf("expected hasUserPriorityClasses to return true on 403 Forbidden to skip installation")
	}
}

func TestEnsureResourceFlavors_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): resourceflavors.kueue.x-k8s.io is forbidden"}
		},
	}
	orc := &GKEOrchestrator{
		executor: mock,
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-default": {},
			},
		},
	}

	err := orc.EnsureResourceFlavors()
	if err != nil {
		t.Fatalf("EnsureResourceFlavors should succeed and skip on 403 Forbidden, got: %v", err)
	}
}

func TestCheckClusterQueueCoverage_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): clusterqueues.kueue.x-k8s.io is forbidden"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	covered, isNew, err := orc.checkClusterQueueCoverage("default")
	if err != nil {
		t.Fatalf("checkClusterQueueCoverage should succeed and skip on 403 Forbidden, got: %v", err)
	}
	if !covered || isNew {
		t.Errorf("expected covered=true, isNew=false on 403 Forbidden, got covered=%v, isNew=%v", covered, isNew)
	}
}

func TestEnsureClusterQueueCoverage_Covered(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "get localqueue") {
				return shell.CommandResult{ExitCode: 0, Stdout: "default"}
			}
			if strings.Contains(fullCmd, "get clusterqueue") {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"spec": {"resourceGroups": [{"coveredResources": ["cpu", "memory"]}]}}`,
				}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	err := orc.ensureClusterQueueCoverage("default", "default")
	if err != nil {
		t.Fatalf("ensureClusterQueueCoverage failed on covered queue: %v", err)
	}
}

func TestCheckKueueInstallPermissions_MissingPermission(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "auth can-i create namespaces") || strings.Contains(fullCmd, "auth can-i create clusterroles") {
				return shell.CommandResult{ExitCode: 1, Stdout: "no"}
			}
			return shell.CommandResult{ExitCode: 0, Stdout: "yes"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	err := orc.checkKueueInstallPermissions("v0.15.2")
	if err == nil {
		t.Fatalf("checkKueueInstallPermissions should fail on missing permission, got nil")
	}
	if !strings.Contains(err.Error(), "missing required RBAC permissions: ['create namespaces', 'create clusterroles.rbac.authorization.k8s.io']") {
		t.Errorf("checkKueueInstallPermissions error = %v; want aggregated missing permissions list", err)
	}
}
