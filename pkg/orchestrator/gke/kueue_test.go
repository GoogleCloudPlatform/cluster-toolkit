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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/logging"
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

func (m *mockExecutor) ExecuteCommandWithTimeout(_ time.Duration, name string, args ...string) shell.CommandResult {
	return m.ExecuteCommand(name, args...)
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
		name           string
		requestedName  string
		kubectlOutput  string
		wantName       string
		wantDiscovered bool
		wantErr        bool
	}{
		{
			name:           "User requested name",
			requestedName:  "custom-q",
			kubectlOutput:  "",
			wantName:       "custom-q",
			wantDiscovered: false,
			wantErr:        false,
		},
		{
			name:           "User requested name with resource path",
			requestedName:  "namespaces/default/localQueues/custom-q",
			kubectlOutput:  "",
			wantName:       "custom-q",
			wantDiscovered: false,
			wantErr:        false,
		},
		{
			name:          "User requested invalid RFC 1123 name (uppercase)",
			requestedName: "CustomQueue",
			kubectlOutput: "",
			wantName:      "",
			wantErr:       true,
		},
		{
			name:          "User requested invalid RFC 1123 name (underscore)",
			requestedName: "custom_q",
			kubectlOutput: "",
			wantName:      "",
			wantErr:       true,
		},
		{
			name:          "User requested invalid RFC 1123 name (starts with hyphen)",
			requestedName: "-custom-q",
			kubectlOutput: "",
			wantName:      "",
			wantErr:       true,
		},
		{
			name:          "User requested invalid RFC 1123 name (raw slash path)",
			requestedName: "custom/invalid/queue",
			kubectlOutput: "",
			wantName:      "",
			wantErr:       true,
		},
		{
			name:          "User requested invalid RFC 1123 name (ends with hyphen)",
			requestedName: "custom-q-",
			kubectlOutput: "",
			wantName:      "",
			wantErr:       true,
		},
		{
			name:          "User requested invalid RFC 1123 name (exceeds 253 characters)",
			requestedName: strings.Repeat("a", 254),
			kubectlOutput: "",
			wantName:      "",
			wantErr:       true,
		},
		{
			name:           "No queues found, fallback to default",
			requestedName:  "",
			kubectlOutput:  "",
			wantName:       "default",
			wantDiscovered: false,
			wantErr:        false,
		},
		{
			name:           "Single queue found, auto-discover",
			requestedName:  "",
			kubectlOutput:  "queue-1",
			wantName:       "queue-1",
			wantDiscovered: true,
			wantErr:        false,
		},
		{
			name:           "Multiple queues found, prefer default",
			requestedName:  "",
			kubectlOutput:  "multislice-queue default",
			wantName:       "default",
			wantDiscovered: true,
			wantErr:        false,
		},
		{
			name:           "Multiple queues found, fallback to multislice-queue",
			requestedName:  "",
			kubectlOutput:  "multislice-queue custom-q",
			wantName:       "multislice-queue",
			wantDiscovered: true,
			wantErr:        false,
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

			got, discovered, err := orc.resolveKueueQueue(tt.requestedName, "default")
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveKueueQueue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantName {
				t.Errorf("resolveKueueQueue() got = %v, want %v", got, tt.wantName)
			}
			if discovered != tt.wantDiscovered {
				t.Errorf("resolveKueueQueue() discovered = %v, want %v", discovered, tt.wantDiscovered)
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

	expectedSubstrings := []string{
		"nominalQuota: \"999999\"",
		"nominalQuota: 999999T",
		"nominalQuota: 8",
		"nominalQuota: 480",
		"nominalQuota: 2000Gi",
		"nominalQuota: 0",
		"name: flavor-tpu",
		"name: pathways-flavor",
	}
	for _, want := range expectedSubstrings {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got %s", want, output)
		}
	}

	count := strings.Count(output, "coveredResources:")
	if count != 1 {
		t.Errorf("expected 1 coveredResources block for Pathways case (unified), got %d. Output: %s", count, output)
	}

	if strings.Index(output, "name: pathways-flavor") > strings.Index(output, "name: flavor-tpu") {
		t.Errorf("expected pathways-flavor before flavor-tpu in ClusterQueue flavors, got:\n%s", output)
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
					Stdout:   `{"spec":{"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.15.2"}]}}}}`,
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
					Stdout:   `{"spec":{"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.11.1"}]}}}}`,
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
		{pattern: "kubectl get deployment", res: shell.CommandResult{ExitCode: 0, Stdout: `{"spec":{"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.12.0"}]}}}}`}},
		{pattern: "kubectl get endpointslice", res: shell.CommandResult{ExitCode: 0, Stdout: `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true}}]}]}`}},
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

	err := resolveKueueOnlyForTest(orc, "test-cluster")
	if err != nil {
		t.Fatalf("resolveKueueOnlyForTest failed: %v", err)
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

	err := orc.EnsurePriorityClassesInstalled()
	if err != nil {
		t.Fatalf("EnsurePriorityClassesInstalled failed: %v", err)
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

	err := orc.EnsurePriorityClassesInstalled()
	if err != nil {
		t.Fatalf("EnsurePriorityClassesInstalled failed: %v", err)
	}

	if applyCalled {
		t.Errorf("expected priority classes to be skipped, but they were installed")
	}
}

func TestHandleKueueReinstallation_UserDeclines(t *testing.T) {
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })

	t.Run("installed_warns_about_deleting_workloads", func(t *testing.T) {
		var capturedPrompt string
		shell.PromptYesNo = func(prompt string) bool {
			capturedPrompt = prompt
			return false
		}
		orc := &GKEOrchestrator{}
		err := orc.handleKueueReinstallation("v0.15.2", "Current Kueue version v0.14.0 is below target v0.15.2.", true)
		if err == nil || !strings.Contains(err.Error(), "user declined to re-install Kueue") {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(capturedPrompt, "WARNING: This deletes all queued and suspended workloads") {
			t.Errorf("expected deletion warning when Kueue is installed, got prompt:\n%s", capturedPrompt)
		}
	})

	t.Run("not_installed_omits_deletion_warning", func(t *testing.T) {
		var capturedPrompt string
		shell.PromptYesNo = func(prompt string) bool {
			capturedPrompt = prompt
			return false
		}
		orc := &GKEOrchestrator{}
		err := orc.handleKueueReinstallation("v0.15.2", "Kueue is not installed.", false)
		if err == nil || !strings.Contains(err.Error(), "user declined to install Kueue") {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(capturedPrompt, "deletes all queued") || strings.Contains(capturedPrompt, "WARNING:") {
			t.Errorf("fresh install prompt must not warn about deleting queued workloads, got prompt:\n%s", capturedPrompt)
		}
	})
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
			if strings.Contains(fullCmd, "kubectl get deployment") {
				return shell.CommandResult{ExitCode: 0, Stdout: `{"spec":{"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.12.0"}]}}}}`}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{
		executor: mock,
	}

	err := resolveKueueOnlyForTest(orc, "test-cluster")
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
			if strings.Contains(fullCmd, "kubectl get deployment") {
				return shell.CommandResult{ExitCode: 0, Stdout: `{"spec":{"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.12.0"}]}}}}`}
			}
			if strings.Contains(fullCmd, "kubectl get endpointslice") {
				return shell.CommandResult{ExitCode: 0, Stdout: `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true}}]}]}`}
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

	err := resolveKueueOnlyForTest(orc, "test-cluster")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !deleteCalled {
		t.Errorf("expected DeleteAllKueueResources to be called, but it wasn't")
	}
}

func TestValidatePriorityClass_Empty(t *testing.T) {
	orc := &GKEOrchestrator{}
	err := orc.ensureAndValidatePriorityClass("")
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
	err := orc.ensureAndValidatePriorityClass("medium")
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
	err := orc.ensureAndValidatePriorityClass("medium")
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

	installed, _, err := orc.isKueueCRDInstalled()
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

	dep, installed, err := orc.getKueueDeployment()
	if err != nil {
		t.Fatalf("getKueueDeployment should succeed and return true on 403 Forbidden, got err: %v", err)
	}
	if dep != nil {
		t.Errorf("getKueueDeployment on 403 Forbidden dep = %v; want nil", dep)
	}
	if !installed {
		t.Errorf("getKueueDeployment on 403 Forbidden = false; want true")
	}
}

func TestCheckAndInstallKueue_VersionNormalization(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "auth can-i") {
				return shell.CommandResult{ExitCode: 0, Stdout: "yes"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment") {
				return shell.CommandResult{ExitCode: 0, Stdout: `{"spec":{"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.12.0"}]}}}}`}
			}
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 0, Stdout: "found"}
			}
			if strings.Contains(fullCmd, "kubectl get endpointslice") {
				return shell.CommandResult{ExitCode: 0, Stdout: `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true}}]}]}`}
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
	if got := normalizeKueueVersion("0.17.1"); got != "v0.17.1" {
		t.Fatalf("normalizeKueueVersion(\"0.17.1\") = %q, want v0.17.1", got)
	}
	err := orc.installKueueOnFreshCluster("0.17.1", "test-cluster")
	if err != nil {
		t.Fatalf("installKueueOnFreshCluster failed for version without 'v' prefix: %v", err)
	}
}

func TestValidatePriorityClass_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): priorityclasses.scheduling.k8s.io is forbidden"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	err := orc.ensureAndValidatePriorityClass("high")
	if err != nil {
		t.Fatalf("ensureAndValidatePriorityClass should succeed and skip on 403 Forbidden, got: %v", err)
	}
}

func TestEnsurePriorityClassesInstalled_Forbidden(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): priorityclasses.scheduling.k8s.io is forbidden"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}

	if err := orc.EnsurePriorityClassesInstalled(); err != nil {
		t.Fatalf("EnsurePriorityClassesInstalled should skip and succeed on 403 Forbidden, got error: %v", err)
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

	err := orc.checkKueuePermissions("re-install", "v0.15.2")
	if err == nil {
		t.Fatalf("checkKueuePermissions should fail on missing permission, got nil")
	}
	if !strings.Contains(err.Error(), "missing required RBAC permissions: ['create namespaces', 'create clusterroles.rbac.authorization.k8s.io']") {
		t.Errorf("checkKueuePermissions error = %v; want aggregated missing permissions list", err)
	}
}

func TestCheckKueueEndpoints_EndpointSlice(t *testing.T) {
	tests := []struct {
		name      string
		result    shell.CommandResult
		wantReady bool
		wantErr   bool
	}{
		{name: "ready endpoint", result: shell.CommandResult{ExitCode: 0, Stdout: `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true}}]}]}`}, wantReady: true},
		{name: "no ready endpoint", result: shell.CommandResult{ExitCode: 0, Stdout: `{"items":[]}`}},
		{name: "forbidden skips the check", result: shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): endpointslices is forbidden"}, wantReady: true},
		{name: "failed read is retried by the caller", result: shell.CommandResult{ExitCode: 1, Stderr: "Unable to connect to the server"}},
		{name: "malformed response is an error", result: shell.CommandResult{ExitCode: 0, Stdout: "not json"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc := &GKEOrchestrator{executor: &mockExecutor{executeCommandFunc: func(name string, args ...string) shell.CommandResult {
				if got, want := strings.Join(args, " "), "get endpointslice -l kubernetes.io/service-name=kueue-webhook-service -n kueue-system -o json --request-timeout=10s"; got != want {
					t.Errorf("kubectl args = %q, want %q", got, want)
				}
				return tc.result
			}}}

			ready, err := orc.checkKueueEndpoints()
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkKueueEndpoints() error = %v, wantErr %v", err, tc.wantErr)
			}
			if ready != tc.wantReady {
				t.Errorf("checkKueueEndpoints() = %v, want %v", ready, tc.wantReady)
			}
		})
	}
}

func TestParseKueueQueueName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantQueue string
		wantErr   bool
	}{
		{
			name:      "Valid simple name",
			input:     "default",
			wantQueue: "default",
		},
		{
			name:      "Valid with hyphens and dots",
			input:     "my-team.queue-1",
			wantQueue: "my-team.queue-1",
		},
		{
			name:      "Resource path parsed to bare name",
			input:     "namespaces/team-a/localQueues/custom-q",
			wantQueue: "custom-q",
		},
		{
			name:    "Uppercase rejected",
			input:   "DefaultQueue",
			wantErr: true,
		},
		{
			name:    "Underscore rejected",
			input:   "default_queue",
			wantErr: true,
		},
		{
			name:    "Starts with hyphen rejected",
			input:   "-default",
			wantErr: true,
		},
		{
			name:    "Ends with hyphen rejected",
			input:   "custom-q-",
			wantErr: true,
		},
		{
			name:    "Name exceeding 253 characters rejected",
			input:   strings.Repeat("a", 254),
			wantErr: true,
		},
		{
			name:      "Max length 253 characters valid RFC 1123 subdomain succeeds",
			input:     strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 60),
			wantQueue: strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 60),
			wantErr:   false,
		},
		{
			name:    "Empty name rejected",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseKueueQueueName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseKueueQueueName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.wantQueue {
				t.Errorf("parseKueueQueueName(%q) = %q, want %q", tt.input, got, tt.wantQueue)
			}
		})
	}
}

func TestCheckAndInstallKueue_HealthySingleDeploymentRead(t *testing.T) {
	getDepCalls := 0
	rolloutCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 0, Stdout: "clusterqueues.kueue.x-k8s.io found"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment") {
				getDepCalls++
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"metadata":{"generation":1},"spec":{"replicas":1,"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.16.1"}]}}},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":1}}`,
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
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{executor: mock}
	if err := resolveKueueOnlyForTest(orc, "test-cluster"); err != nil {
		t.Fatalf("resolveKueueOnlyForTest failed: %v", err)
	}
	if getDepCalls != 1 {
		t.Errorf("kubectl get deployment calls = %d, want 1", getDepCalls)
	}
	if rolloutCalls != 0 {
		t.Errorf("kubectl rollout status calls = %d, want 0 when deployment is already rolled out", rolloutCalls)
	}
}

func TestGetKueueDeployment_InvalidJSON(t *testing.T) {
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 0, Stdout: "{not-valid-json"}
		},
	}
	orc := &GKEOrchestrator{executor: mock}
	dep, exists, err := orc.getKueueDeployment()
	if err == nil {
		t.Fatalf("expected error on invalid JSON, got nil (dep=%v, exists=%v)", dep, exists)
	}
	if !exists {
		t.Errorf("expected exists=true when kubectl get deployment succeeded with invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to unmarshal kueue deployment") {
		t.Errorf("expected unmarshal error message, got: %v", err)
	}
}

func TestIsKueueDeploymentRolledOut(t *testing.T) {
	parseDep := func(t *testing.T, raw string) *kueueDeployment {
		t.Helper()
		var dep kueueDeployment
		if err := json.Unmarshal([]byte(raw), &dep); err != nil {
			t.Fatalf("failed to unmarshal test deployment JSON: %v", err)
		}
		return &dep
	}

	tests := []struct {
		name string
		json string
		want bool
	}{
		{
			name: "nil deployment",
			json: "",
			want: false,
		},
		{
			name: "healthy single replica deployment",
			json: `{"metadata":{"generation":2},"spec":{"replicas":1},"status":{"observedGeneration":2,"replicas":1,"updatedReplicas":1,"availableReplicas":1}}`,
			want: true,
		},
		{
			name: "paused deployment",
			json: `{"metadata":{"generation":1},"spec":{"replicas":1,"paused":true},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":1}}`,
			want: false,
		},
		{
			name: "progress deadline exceeded condition",
			json: `{"metadata":{"generation":1},"spec":{"replicas":1},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":1,"conditions":[{"type":"Progressing","status":"False","reason":"ProgressDeadlineExceeded"}]}}`,
			want: false,
		},
		{
			name: "available condition false",
			json: `{"metadata":{"generation":1},"spec":{"replicas":1},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":1,"conditions":[{"type":"Available","status":"False"}]}}`,
			want: false,
		},
		{
			name: "zero desired replicas",
			json: `{"metadata":{"generation":1},"spec":{"replicas":0},"status":{"observedGeneration":1}}`,
			want: false,
		},
		{
			name: "generation ahead of observedGeneration",
			json: `{"metadata":{"generation":3},"spec":{"replicas":1},"status":{"observedGeneration":2,"replicas":1,"updatedReplicas":1,"availableReplicas":1}}`,
			want: false,
		},
		{
			name: "old pods still terminating",
			json: `{"metadata":{"generation":2},"spec":{"replicas":1},"status":{"observedGeneration":2,"replicas":2,"updatedReplicas":1,"availableReplicas":1}}`,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var dep *kueueDeployment
			if tc.json != "" {
				dep = parseDep(t, tc.json)
			}
			if got := isKueueDeploymentRolledOut(dep); got != tc.want {
				t.Errorf("isKueueDeploymentRolledOut() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckAndInstallKueue_UnrolledDeploymentRunsRolloutStatus(t *testing.T) {
	getDepCalls := 0
	rolloutCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 0, Stdout: "clusterqueues.kueue.x-k8s.io found"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment") {
				getDepCalls++
				// AvailableReplicas=0 means rollout is still in progress.
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"metadata":{"generation":1},"spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"sidecar","image":"busybox:latest"},{"name":"manager","image":"registry.k8s.io/kueue/kueue:v0.16.1"}]}}},"status":{"observedGeneration":1,"replicas":1,"updatedReplicas":1,"availableReplicas":0}}`,
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
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{executor: mock}
	if err := resolveKueueOnlyForTest(orc, "test-cluster"); err != nil {
		t.Fatalf("resolveKueueOnlyForTest failed: %v", err)
	}
	if getDepCalls != 1 {
		t.Errorf("kubectl get deployment calls = %d, want 1 (reused for version check)", getDepCalls)
	}
	if rolloutCalls != 1 {
		t.Errorf("kubectl rollout status calls = %d, want 1 when deployment is not yet rolled out", rolloutCalls)
	}
}

func TestEnsureAndValidatePriorityClass_SingleListAndRelistOnInstall(t *testing.T) {
	t.Run("Single list when user priority classes already exist", func(t *testing.T) {
		listCalls := 0
		applyCalls := 0
		mock := &mockExecutor{
			executeCommandFunc: func(name string, args ...string) shell.CommandResult {
				fullCmd := name + " " + strings.Join(args, " ")
				if strings.HasPrefix(fullCmd, "kubectl get priorityclass") {
					listCalls++
					return shell.CommandResult{
						ExitCode: 0,
						Stdout:   "system-node-critical high medium",
					}
				}
				if strings.Contains(fullCmd, "kubectl apply") {
					applyCalls++
					return shell.CommandResult{ExitCode: 0}
				}
				return shell.CommandResult{ExitCode: 0}
			},
		}

		orc := &GKEOrchestrator{executor: mock}
		if err := orc.ensureAndValidatePriorityClass("high"); err != nil {
			t.Fatalf("ensureAndValidatePriorityClass failed: %v", err)
		}
		if listCalls != 1 {
			t.Errorf("kubectl get priorityclass calls = %d, want 1", listCalls)
		}
		if applyCalls != 0 {
			t.Errorf("kubectl apply calls = %d, want 0", applyCalls)
		}
	})

	t.Run("Re-lists after installing default priority classes", func(t *testing.T) {
		listCalls := 0
		applyCalls := 0
		mock := &mockExecutor{
			executeCommandFunc: func(name string, args ...string) shell.CommandResult {
				fullCmd := name + " " + strings.Join(args, " ")
				if strings.HasPrefix(fullCmd, "kubectl get priorityclass") {
					listCalls++
					if listCalls == 1 {
						// Only system-* classes exist initially.
						return shell.CommandResult{
							ExitCode: 0,
							Stdout:   "system-cluster-critical system-node-critical",
						}
					}
					return shell.CommandResult{
						ExitCode: 0,
						Stdout:   "system-cluster-critical high medium low",
					}
				}
				if strings.Contains(fullCmd, "kubectl apply") {
					applyCalls++
					return shell.CommandResult{ExitCode: 0}
				}
				return shell.CommandResult{ExitCode: 0}
			},
		}

		orc := &GKEOrchestrator{executor: mock}
		if err := orc.ensureAndValidatePriorityClass("high"); err != nil {
			t.Fatalf("ensureAndValidatePriorityClass failed: %v", err)
		}
		if applyCalls != 1 {
			t.Errorf("kubectl apply calls = %d, want 1", applyCalls)
		}
		if listCalls != 2 {
			t.Errorf("kubectl get priorityclass calls = %d, want 2 (initial list + re-list after install)", listCalls)
		}
	})
}

func TestConfigureClusterEnvironment_AutoDiscoveredQueueSkipsGet(t *testing.T) {
	listCalls := 0
	getSingleCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if fullCmd == "kubectl get localqueue -n default -o jsonpath={.items[*].metadata.name}" {
				listCalls++
				return shell.CommandResult{ExitCode: 0, Stdout: "default"}
			}
			if strings.HasPrefix(fullCmd, "kubectl get localqueue ") {
				getSingleCalls++
				return shell.CommandResult{ExitCode: 0}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := newTestGKEOrchestrator(mock)
	job := &orchestrator.JobDefinition{
		KueueQueueName: "",
	}
	if err := orc.configureClusterEnvironment(job); err != nil {
		t.Fatalf("configureClusterEnvironment failed: %v", err)
	}
	if job.KueueQueueName != "default" {
		t.Errorf("job.KueueQueueName = %q, want %q", job.KueueQueueName, "default")
	}
	if listCalls != 1 {
		t.Errorf("localqueue list calls = %d, want 1", listCalls)
	}
	if getSingleCalls != 0 {
		t.Errorf("localqueue get calls = %d, want 0 when auto-discovered from list", getSingleCalls)
	}
}

func TestConfigureClusterEnvironment_ExplicitQueueDoesSingleGet(t *testing.T) {
	listCalls := 0
	getSingleCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if fullCmd == "kubectl get localqueue -n default -o jsonpath={.items[*].metadata.name}" {
				listCalls++
				return shell.CommandResult{ExitCode: 0, Stdout: "default"}
			}
			if fullCmd == "kubectl get localqueue custom-queue -n default" {
				getSingleCalls++
				return shell.CommandResult{ExitCode: 0}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := newTestGKEOrchestrator(mock)
	job := &orchestrator.JobDefinition{
		KueueQueueName: "custom-queue",
	}
	if err := orc.configureClusterEnvironment(job); err != nil {
		t.Fatalf("configureClusterEnvironment failed: %v", err)
	}
	if job.KueueQueueName != "custom-queue" {
		t.Errorf("job.KueueQueueName = %q, want %q", job.KueueQueueName, "custom-queue")
	}
	if listCalls != 0 {
		t.Errorf("localqueue list calls = %d, want 0 when explicit --queue is provided", listCalls)
	}
	if getSingleCalls != 1 {
		t.Errorf("localqueue get calls = %d, want 1 when explicit --queue is provided", getSingleCalls)
	}
}

func TestConfigureClusterEnvironment_NoQueuesFoundSkipsRedundantGetAndList(t *testing.T) {
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(prompt string) bool { return false }

	listCalls := 0
	getSingleCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if fullCmd == "kubectl get localqueue -n default -o jsonpath={.items[*].metadata.name}" {
				listCalls++
				return shell.CommandResult{ExitCode: 0, Stdout: ""}
			}
			if strings.HasPrefix(fullCmd, "kubectl get localqueue ") {
				getSingleCalls++
				return shell.CommandResult{ExitCode: 1, Stderr: "NotFound"}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := newTestGKEOrchestrator(mock)
	job := &orchestrator.JobDefinition{
		KueueQueueName: "",
	}
	err := orc.configureClusterEnvironment(job)
	if err == nil {
		t.Fatal("expected error when user declines to create default queues, got nil")
	}
	if listCalls != 1 {
		t.Errorf("localqueue list calls = %d, want 1 (should not re-list when initial list returned 0 queues)", listCalls)
	}
	if getSingleCalls != 0 {
		t.Errorf("localqueue get calls = %d, want 0 (initial list already proved 'default' does not exist)", getSingleCalls)
	}
}

func TestEnsureAndValidatePriorityClass_ForbiddenSingleList(t *testing.T) {
	listCalls := 0
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.HasPrefix(fullCmd, "kubectl get priorityclass") {
				listCalls++
				return shell.CommandResult{
					ExitCode: 1,
					Stderr:   "Error from server (Forbidden): priorityclasses.scheduling.k8s.io is forbidden",
				}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{executor: mock}
	if err := orc.ensureAndValidatePriorityClass("high"); err != nil {
		t.Fatalf("ensureAndValidatePriorityClass should skip and succeed on 403 Forbidden, got: %v", err)
	}
	if listCalls != 1 {
		t.Errorf("kubectl get priorityclass calls on 403 Forbidden = %d, want 1", listCalls)
	}
}

func TestWaitForKueueWebhook_RolloutBeforeDeploymentGet(t *testing.T) {
	var callOrder []string
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			switch {
			case strings.Contains(fullCmd, "kubectl rollout status"):
				callOrder = append(callOrder, "rollout")
				return shell.CommandResult{ExitCode: 0}
			case strings.Contains(fullCmd, "kubectl get deployment"):
				callOrder = append(callOrder, "get-deployment")
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"spec":{"template":{"spec":{"containers":[{"name":"manager","image":"registry.k8s.io/kueue/kueue:v0.16.1"}]}}}}`,
				}
			case strings.Contains(fullCmd, "kubectl get endpointslice"):
				callOrder = append(callOrder, "get-endpointslice")
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"items":[{"endpoints":[{"addresses":["10.4.1.3"],"conditions":{"ready":true}}]}]}`,
				}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{executor: mock}
	if err := orc.waitForKueueWebhook(); err != nil {
		t.Fatalf("waitForKueueWebhook failed: %v", err)
	}
	if len(callOrder) < 2 || callOrder[0] != "rollout" || callOrder[1] != "get-endpointslice" {
		t.Errorf("waitForKueueWebhook call order = %v, want [rollout get-endpointslice ...]", callOrder)
	}
}

func TestGetKueueVersion_SidecarInKueueRepoBeforeManager(t *testing.T) {
	raw := `{"spec":{"template":{"spec":{"containers":[` +
		`{"name":"kube-rbac-proxy","image":"us-docker.pkg.dev/my-proj/kueue-mirror/kube-rbac-proxy:v0.18.0"},` +
		`{"name":"manager","image":"us-docker.pkg.dev/my-proj/kueue-mirror/kueue:v0.16.1"}` +
		`]}}}}`
	var dep kueueDeployment
	if err := json.Unmarshal([]byte(raw), &dep); err != nil {
		t.Fatalf("failed to unmarshal deployment JSON: %v", err)
	}

	orc := &GKEOrchestrator{}
	ver, err := orc.getKueueVersion(&dep)
	if err != nil {
		t.Fatalf("getKueueVersion failed: %v", err)
	}
	if ver != "v0.16.1" {
		t.Errorf("getKueueVersion() = %q, want %q (manager container version)", ver, "v0.16.1")
	}
}

func TestGetKueueVersion_ImageRefFormats(t *testing.T) {
	t.Parallel()
	orc := &GKEOrchestrator{}
	for _, tc := range []struct {
		name    string
		image   string
		want    string
		wantErr string
	}{
		{name: "registry with port, tag, and digest", image: "localhost:5000/kueue/kueue:v0.16.1@sha256:abcdef", want: "v0.16.1"},
		{name: "registry with port and no tag", image: "localhost:5000/kueue/kueue", wantErr: "no tag found in Kueue image"},
		{name: "trailing colon empty tag", image: "registry.k8s.io/kueue/kueue:", wantErr: "no tag found in Kueue image"},
		{name: "digest only", image: "registry.k8s.io/kueue/kueue@sha256:abcdef", wantErr: "no tag found in Kueue image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dep := kueueDeployment{}
			dep.Spec.Template.Spec.Containers = []struct {
				Name  string `json:"name"`
				Image string `json:"image"`
			}{{Name: "manager", Image: tc.image}}
			got, err := orc.getKueueVersion(&dep)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("getKueueVersion(%q) err = %v, want substring %q", tc.image, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("getKueueVersion(%q) unexpected err: %v", tc.image, err)
			}
			if got != tc.want {
				t.Errorf("getKueueVersion(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}

func TestCheckAndInstallKueue_SharedClusterForbidden(t *testing.T) {
	getDepCalls := 0
	rolloutCalls := 0

	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			fullCmd := name + " " + strings.Join(args, " ")
			if strings.Contains(fullCmd, "kubectl get crd") {
				return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): forbidden"}
			}
			if strings.Contains(fullCmd, "kubectl get deployment") {
				getDepCalls++
				return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): forbidden"}
			}
			if strings.Contains(fullCmd, "kubectl rollout status") {
				rolloutCalls++
				return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): forbidden"}
			}
			if strings.Contains(fullCmd, "kubectl get endpointslice") {
				return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): forbidden"}
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}

	orc := &GKEOrchestrator{executor: mock}
	if err := resolveKueueOnlyForTest(orc, "test-cluster"); err != nil {
		t.Fatalf("resolveKueueOnlyForTest should succeed on 403 Forbidden in shared cluster, got: %v", err)
	}
	if getDepCalls != 1 {
		t.Errorf("kubectl get deployment calls = %d, want 1", getDepCalls)
	}
	if rolloutCalls != 1 {
		t.Errorf("kubectl rollout status calls = %d, want 1", rolloutCalls)
	}
}

func TestAirGapAndSafeReinstall_DownloadBeforeDelete(t *testing.T) {
	t.Run("downloadManifests_fast_fails_when_air_gapped", func(t *testing.T) {
		t.Setenv(shell.AirGappedEnvVar, "true")
		httpCalls := 0
		orc := &GKEOrchestrator{
			httpClient: &mockHTTPClient{
				getFunc: func(string) (*http.Response, error) {
					httpCalls++
					return nil, errors.New("unexpected network call")
				},
			},
		}
		_, err := orc.downloadManifests("https://github.com/kubernetes-sigs/kueue/releases/download/v0.15.2/manifests.yaml")
		if err == nil || !strings.Contains(err.Error(), shell.AirGappedEnvVar) {
			t.Fatalf("downloadManifests(air-gapped) error = %v, want %s fast-fail", err, shell.AirGappedEnvVar)
		}
		if httpCalls != 0 {
			t.Fatalf("downloadManifests(air-gapped) HTTP calls = %d, want 0", httpCalls)
		}
	})

	t.Run("reinstall_aborts_before_delete_on_download_validation_or_write_failure", func(t *testing.T) {
		assertReinstallAbortsBeforeDelete(t)
	})

	t.Run("air_gap_fast_fails_before_rbac_or_prompt", func(t *testing.T) {
		assertAirGapFastFailBeforeRBAC(t)
	})

	t.Run("reinstall_downloads_and_writes_before_delete_and_logs_target_version", func(t *testing.T) {
		assertHappyPathKueueReinstallOrder(t)
	})
}

func assertReinstallAbortsBeforeDelete(t *testing.T) {
	t.Helper()
	t.Setenv(shell.AirGappedEnvVar, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(string) bool { return true }

	deleteCalled := false
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			if strings.Contains(name+" "+strings.Join(args, " "), "delete") {
				deleteCalled = true
			}
			return shell.CommandResult{ExitCode: 0}
		},
	}
	orc := &GKEOrchestrator{
		kubeContext: "k8s-ctx",
		executor:    mock,
		httpClient: &mockHTTPClient{
			getFunc: func(string) (*http.Response, error) {
				return nil, errors.New("github unreachable")
			},
		},
	}
	if err := orc.handleKueueReinstallation("v0.15.2", "incomplete", true); err == nil || !strings.Contains(err.Error(), "github unreachable") {
		t.Fatalf("handleKueueReinstallation(download error) = %v, want github unreachable", err)
	}
	if deleteCalled {
		t.Fatal("DeleteAllKueueResources ran before manifest download succeeded")
	}

	assertInvalidManifestBodiesAbortBeforeDelete(t, orc, &deleteCalled)
	assertUnwritableHomeAbortsBeforeDelete(t, orc, &deleteCalled)
}

func assertInvalidManifestBodiesAbortBeforeDelete(t *testing.T, orc *GKEOrchestrator, deleteCalled *bool) {
	t.Helper()
	for _, tc := range []struct {
		name       string
		body       string
		wantSubstr string
	}{
		{
			name:       "html_scalar_body",
			body:       "<html>proxy error</html>",
			wantSubstr: "expected Kubernetes manifest object with apiVersion and kind",
		},
		{
			name:       "json_map_missing_apiversion_and_kind",
			body:       `{"message":"API rate limit exceeded"}`,
			wantSubstr: "expected Kubernetes manifest object with apiVersion and kind",
		},
		{
			name:       "yaml_map_missing_kind",
			body:       "apiVersion: v1\nmetadata:\n  name: incomplete\n",
			wantSubstr: "expected Kubernetes manifest object with apiVersion and kind",
		},
		{
			name:       "empty_document_stream",
			body:       "---\n# comments only\n---\n",
			wantSubstr: "manifest stream is empty",
		},
	} {
		orc.httpClient = &mockHTTPClient{
			getFunc: func(string) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			},
		}
		err := orc.handleKueueReinstallation("v0.15.2", "incomplete", true)
		if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
			t.Fatalf("handleKueueReinstallation(%s) = %v, want substring %q", tc.name, err, tc.wantSubstr)
		}
		if *deleteCalled {
			t.Fatalf("DeleteAllKueueResources ran despite invalid manifest (%s)", tc.name)
		}
	}
}

func assertUnwritableHomeAbortsBeforeDelete(t *testing.T, orc *GKEOrchestrator, deleteCalled *bool) {
	t.Helper()
	blockedHome := t.TempDir()
	// Placing a regular file at ~/.gcluster fails MkdirAll(~/.gcluster/generated) deterministically even when running as root.
	if err := os.WriteFile(filepath.Join(blockedHome, ".gcluster"), []byte("blocker"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", blockedHome)
	t.Setenv("USERPROFILE", blockedHome)
	orc.httpClient = &mockHTTPClient{
		getFunc: func(string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: kueue-system\n")),
			}, nil
		},
	}
	err := orc.handleKueueReinstallation("v0.15.2", "incomplete", true)
	if err == nil || !strings.Contains(err.Error(), "failed to create directory for generated manifests") {
		t.Fatalf("handleKueueReinstallation(unwritable HOME) = %v, want directory creation error", err)
	}
	if *deleteCalled {
		t.Fatal("DeleteAllKueueResources ran before local kueue.yaml manifest write succeeded")
	}
}

func assertAirGapFastFailBeforeRBAC(t *testing.T) {
	t.Helper()
	t.Setenv(shell.AirGappedEnvVar, "true")
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	promptCalled := false
	shell.PromptYesNo = func(string) bool {
		promptCalled = true
		return true
	}

	canICalled := false
	airGapOrc := &GKEOrchestrator{
		kubeContext: "gke-ctx",
		kubeClient:  &MockKubeClient{},
		httpClient: &mockHTTPClient{
			getFunc: func(string) (*http.Response, error) {
				t.Errorf("unexpected outbound HTTP call while %s=true", shell.AirGappedEnvVar)
				return nil, errors.New("forbidden outbound HTTP")
			},
		},
		executor: &mockExecutor{
			executeCommandFunc: func(name string, args ...string) shell.CommandResult {
				full := name + " " + strings.Join(args, " ")
				if strings.Contains(full, "auth can-i") {
					canICalled = true
				}
				if strings.Contains(full, "get deployment") {
					return shell.CommandResult{ExitCode: 1, Stderr: "Error from server (NotFound): deployments.apps \"kueue-controller-manager\" not found"}
				}
				if strings.Contains(full, "get") {
					return shell.CommandResult{ExitCode: 0, Stdout: ""}
				}
				return shell.CommandResult{ExitCode: 0, Stdout: "yes"}
			},
		},
	}
	if err := airGapOrc.installJobSetOnFreshCluster(); err == nil || !strings.Contains(err.Error(), shell.AirGappedEnvVar) {
		t.Fatalf("installJobSetOnFreshCluster(air-gapped) = %v, want %s error", err, shell.AirGappedEnvVar)
	}
	if err := airGapOrc.installKueueOnFreshCluster("v0.15.2", "clus"); err == nil || !strings.Contains(err.Error(), shell.AirGappedEnvVar) {
		t.Fatalf("installKueueOnFreshCluster(air-gapped) = %v, want %s error", err, shell.AirGappedEnvVar)
	}
	if canICalled || promptCalled {
		t.Fatalf("air-gapped installation ran can-i=%v prompt=%v, want both false", canICalled, promptCalled)
	}
}

func assertHappyPathKueueReinstallOrder(t *testing.T) {
	t.Helper()
	t.Setenv(shell.AirGappedEnvVar, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	origPrompt := shell.PromptYesNo
	t.Cleanup(func() { shell.PromptYesNo = origPrompt })
	shell.PromptYesNo = func(string) bool { return true }

	manifestPath := filepath.Join(home, ".gcluster", "generated", kueueManifestFile("gke-ctx"))
	var events []string
	applyMustFail := false
	mock := &mockExecutor{
		executeCommandFunc: func(name string, args ...string) shell.CommandResult {
			full := name + " " + strings.Join(args, " ")
			if name == "kubectl" && !strings.HasPrefix(strings.Join(args, " "), "--context gke-ctx ") {
				t.Errorf("unpinned kubectl call in handleKueueReinstallation: %q", full)
			}
			if strings.Contains(full, "delete crd") {
				if _, statErr := os.Stat(manifestPath); statErr != nil {
					t.Errorf("kueue.yaml not written to disk before DeleteAllKueueResources: %v", statErr)
				}
			}
			if applyMustFail && strings.Contains(full, "apply -f") {
				return shell.CommandResult{ExitCode: 1, Stderr: "apiserver webhook timeout"}
			}
			recordKueueReinstallEvent(full, &events)
			return mockKueueReinstallReadResult(full)
		},
	}
	orc := &GKEOrchestrator{
		kubeContext: "gke-ctx",
		executor:    mock,
		httpClient: &mockHTTPClient{
			getFunc: func(string) (*http.Response, error) {
				events = append(events, "download")
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: kueue-system\n")),
				}, nil
			},
		},
	}
	snapLogs, restoreLogs := logging.CaptureForTesting(t)
	defer restoreLogs()
	if err := orc.handleKueueReinstallation("v0.15.2", "incomplete", true); err != nil {
		t.Fatalf("handleKueueReinstallation(happy path) error = %v", err)
	}
	wantOrder := []string{"download", "delete", "apply"}
	if len(events) < 3 || !slices.Equal(events[:3], wantOrder) {
		t.Fatalf("reinstall event order = %v, want prefix %v", events, wantOrder)
	}
	infoOut, _ := snapLogs()
	if !strings.Contains(infoOut, "Installing Kueue version v0.15.2...") {
		t.Errorf("info logs missing target version message; got:\n%s", infoOut)
	}

	applyMustFail = true
	applyErr := orc.handleKueueReinstallation("v0.15.2", "incomplete", true)
	wantRecovery := "kubectl --context gke-ctx apply -f"
	if applyErr == nil || !strings.Contains(applyErr.Error(), wantRecovery) {
		t.Fatalf("handleKueueReinstallation(apply failure) = %v, want recovery hint %q", applyErr, wantRecovery)
	}

	applyMustFail = false
	assertFreshKueueInstallSkipsDelete(t, orc, &events)
}

func assertFreshKueueInstallSkipsDelete(t *testing.T, orc *GKEOrchestrator, events *[]string) {
	t.Helper()
	*events = nil
	var freshPrompt string
	shell.PromptYesNo = func(p string) bool {
		freshPrompt = p
		return true
	}
	if err := orc.handleKueueReinstallation("v0.15.2", "Kueue is not installed.", false); err != nil {
		t.Fatalf("handleKueueReinstallation(kueueInstalled=false) error = %v", err)
	}
	if strings.Contains(freshPrompt, "deletes all queued") {
		t.Errorf("kueueInstalled=false prompt unexpectedly warned about deleting workloads:\n%s", freshPrompt)
	}
	if slices.Contains(*events, "delete") {
		t.Errorf("kueueInstalled=false must not delete Kueue CRDs, events=%v", *events)
	}
	if !slices.Equal(*events, []string{"download", "apply"}) {
		t.Errorf("kueueInstalled=false events = %v, want [download apply]", *events)
	}
}

func recordKueueReinstallEvent(full string, events *[]string) {
	if strings.Contains(full, "delete crd") {
		*events = append(*events, "delete")
	}
	if strings.Contains(full, "apply -f") && strings.HasSuffix(full, kueueManifestFile("gke-ctx")) {
		*events = append(*events, "apply")
	}
}

func mockKueueReinstallReadResult(full string) shell.CommandResult {
	if strings.Contains(full, "get deployment") {
		return shell.CommandResult{ExitCode: 0, Stdout: `{"spec":{"template":{"spec":{"containers":[{"image":"registry.k8s.io/kueue/kueue:v0.15.2"}]}}}}`}
	}
	if strings.Contains(full, "get endpointslice") {
		return shell.CommandResult{ExitCode: 0, Stdout: `{"items":[{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true}}]}]}`}
	}
	return shell.CommandResult{ExitCode: 0}
}

func TestKueueProbes_DoNotMisclassifyExecPluginNotFoundOrSpawnErrors(t *testing.T) {
	t.Run("missing_exec_plugin_returns_error_not_missing_kueue", func(t *testing.T) {
		mock := &mockExecutor{
			executeCommandFunc: func(string, ...string) shell.CommandResult {
				return shell.CommandResult{
					ExitCode: 1,
					Stderr:   `exec: "gke-gcloud-auth-plugin": executable file not found in $PATH`,
				}
			},
		}
		orc := &GKEOrchestrator{executor: mock}
		if _, _, err := orc.isKueueCRDInstalled(); err == nil {
			t.Errorf("isKueueCRDInstalled(missing exec plugin) = nil, want error")
		}
		if _, _, _, err := orc.getKueueDeploymentWithStatus(); err == nil {
			t.Errorf("getKueueDeploymentWithStatus(missing exec plugin) = nil, want error")
		}
	})

	t.Run("exit_code_minus_one_preserves_res_err_even_with_misleading_stderr", func(t *testing.T) {
		spawnErr := errors.New("kubectl binary missing")
		mock := &mockExecutor{
			executeCommandFunc: func(string, ...string) shell.CommandResult {
				return shell.CommandResult{ExitCode: -1, Stderr: "Error from server (NotFound): misleading", Err: spawnErr}
			},
		}
		orc := &GKEOrchestrator{executor: mock}
		if _, _, err := orc.isKueueCRDInstalled(); !errors.Is(err, spawnErr) {
			t.Errorf("isKueueCRDInstalled(ExitCode -1) = %v, want wrapped %v", err, spawnErr)
		}
		if _, _, _, err := orc.getKueueDeploymentWithStatus(); !errors.Is(err, spawnErr) {
			t.Errorf("getKueueDeploymentWithStatus(ExitCode -1) = %v, want wrapped %v", err, spawnErr)
		}
	})

	t.Run("kubectlCommandError_empty_output_and_nil_err", func(t *testing.T) {
		err := kubectlCommandError("probe failed", shell.CommandResult{ExitCode: 2})
		if err == nil || !strings.Contains(err.Error(), "exit status 2") {
			t.Errorf("kubectlCommandError() = %v, want exit status 2", err)
		}
	})
}
