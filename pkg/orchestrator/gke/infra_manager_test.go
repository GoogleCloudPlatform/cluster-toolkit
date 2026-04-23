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
	"hpc-toolkit/pkg/shell"
	"os"
	"strings"
	"testing"
)

func TestRenderClusterQueue(t *testing.T) {
	orc := &GKEOrchestrator{
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-default": {
					CPUs:     100,
					MemoryGi: 400,
				},
				"flavor-gpu-test": {
					CPUs: 10, // Share CPU
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

	// Verify quotas are rendered correctly
	if !strings.Contains(output, "nominalQuota: 100") {
		t.Errorf("expected nominalQuota: 100 for CPU, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 400Gi") {
		t.Errorf("expected nominalQuota: 400Gi for Memory, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 10") {
		t.Errorf("expected nominalQuota: 10 for GPU, got %s", output)
	}

	// Verify single resource group
	count := strings.Count(output, "coveredResources:")
	if count != 1 {
		t.Errorf("expected 1 coveredResources block, got %d. Output: %s", count, output)
	}
}

func TestRenderClusterQueue_Pathways(t *testing.T) {
	orc := &GKEOrchestrator{
		capacity: ClusterCapacity{
			Flavors: map[string]FlavorCapacity{
				"flavor-default": {
					CPUs:     100,
					MemoryGi: 400,
				},
				"cpu-user": { // Pathways flavor
					CPUs:     480,
					MemoryGi: 2000,
				},
			},
		},
	}

	bytes, err := orc.renderClusterQueue("cluster-queue")
	if err != nil {
		t.Fatalf("renderClusterQueue failed: %v", err)
	}

	output := string(bytes)

	// Verify quotas are rendered correctly
	if !strings.Contains(output, "nominalQuota: 100") {
		t.Errorf("expected nominalQuota: 100 for CPU, got %s", output)
	}
	if !strings.Contains(output, "nominalQuota: 480") {
		t.Errorf("expected nominalQuota: 480 for CPU, got %s", output)
	}

	// Verify TWO resource groups
	count := strings.Count(output, "coveredResources:")
	if count != 2 {
		t.Errorf("expected 2 coveredResources blocks for Pathways case, got %d. Output: %s", count, output)
	}
}

func TestRenderClusterQueue_Empty(t *testing.T) {
	orc := &GKEOrchestrator{
		capacity: ClusterCapacity{}, // Empty
	}

	bytes, err := orc.renderClusterQueue("cluster-queue")
	if err != nil {
		t.Fatalf("renderClusterQueue failed: %v", err)
	}

	output := string(bytes)

	// Verify no quotas are rendered
	if strings.Contains(output, "nominalQuota") {
		t.Errorf("expected no nominalQuota rendered when capacity is empty, got %s", output)
	}
}

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
			// Handle rollout status
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
			// Handle rollout status
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
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	_, _ = w.Write([]byte("yes\n"))
	w.Close()

	deleteCalled := false

	matchers := []struct {
		pattern string
		res     shell.CommandResult
		action  func()
	}{
		{pattern: "kubectl delete crd", action: func() { deleteCalled = true }, res: shell.CommandResult{ExitCode: 0}},
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
		executor: mock,
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
				return shell.CommandResult{ExitCode: 1, Stderr: "not found"}
			}
			if strings.Contains(fullCmd, "kubectl apply") && strings.Contains(fullCmd, "priority-classes.yaml") {
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
				return shell.CommandResult{ExitCode: 0}
			}
			if strings.Contains(fullCmd, "kubectl apply") && strings.Contains(fullCmd, "priority-classes.yaml") {
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
