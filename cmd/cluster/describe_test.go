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

package cluster

import (
	"bytes"
	"context"
	"fmt"
	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err := root.Execute()

	return buf.String(), err
}

func TestDescribeCmd_MissingFlags(t *testing.T) {
	resetClusterCmdFlags()

	_, err := executeCommand(ClusterCmd, "describe", "--project", "test-project")
	if err == nil {
		t.Fatalf("expected error for missing flags, got nil")
	}

	if !strings.Contains(err.Error(), `required flag(s) "cluster", "location" not set`) {
		t.Errorf("unexpected error output: %v", err)
	}
}

func TestDescribeCmd_Success(t *testing.T) {
	resetClusterCmdFlags()

	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() *gke.GKEOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockClusterExecutor{})
		return g
	}

	output, err := executeCommand(ClusterCmd, "describe", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "status: RUNNING") {
		t.Errorf("expected output to contain status: RUNNING, got %s", output)
	}
}

func TestClusterCmd_AmbientProjectResolution(t *testing.T) {
	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() *gke.GKEOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockClusterExecutor{})
		return g
	}

	origExecWithTimeout := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecWithTimeout }()

	t.Run("Resolves ambient project from gcloud config", func(t *testing.T) {
		resetClusterCmdFlags()

		shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
			if name == "gcloud" && strings.Join(args, " ") == "config get-value project" {
				return shell.CommandResult{ExitCode: 0, Stdout: "ambient-test-project\n"}
			}
			return shell.CommandResult{ExitCode: 1}
		}

		output, err := executeCommand(ClusterCmd, "describe", "--cluster", "test-cluster", "--location", "us-central1-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if projectID != "ambient-test-project" {
			t.Errorf("expected projectID to be 'ambient-test-project', got %q", projectID)
		}
		if !strings.Contains(output, "status: RUNNING") {
			t.Errorf("expected output to contain status: RUNNING, got %s", output)
		}
	})

	t.Run("Returns error when gcloud config get-value project times out", func(t *testing.T) {
		resetClusterCmdFlags()

		shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
		}

		_, err := executeCommand(ClusterCmd, "describe", "--cluster", "test-cluster", "--location", "us-central1-a")
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		wantSubstr := "gcloud config get-value project timed out after"
		if !strings.Contains(err.Error(), wantSubstr) {
			t.Errorf("expected error containing %q, got: %v", wantSubstr, err)
		}
	})

	t.Run("Returns error when gcloud binary fails to start", func(t *testing.T) {
		resetClusterCmdFlags()
		shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: -1, Err: fmt.Errorf("executable file not found in PATH")}
		}
		_, err := executeCommand(ClusterCmd, "describe", "--cluster", "test-cluster", "--location", "us-central1-a")
		if err == nil {
			t.Fatal("expected start error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to execute gcloud") {
			t.Errorf("expected error containing 'failed to execute gcloud', got: %v", err)
		}
	})
}

func resetClusterCmdFlags() {
	clusterName = ""
	location = ""
	projectID = ""
}

type mockClusterExecutor struct{}

func (m *mockClusterExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	if name == "gcloud" {
		if len(args) > 2 && args[0] == "container" && args[1] == "clusters" {
			if args[2] == "describe" {
				if strings.Contains(strings.Join(args, " "), "--format=yaml") {
					return shell.CommandResult{
						ExitCode: 0,
						Stdout:   "status: RUNNING\nname: test-cluster\n",
					}
				}
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"status": "RUNNING", "name": "test-cluster"}`,
				}
			}
			if args[2] == "list" {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `[]`,
				}
			}
		}
	}
	if name == "kubectl" {
		if len(args) > 1 && args[0] == "get" {
			if args[1] == "pvc" {
				return shell.CommandResult{
					ExitCode: 0,
					Stdout:   `{"items": []}`,
				}
			}
		}
	}
	return shell.CommandResult{ExitCode: 0, Stdout: "{}"} // Default to empty object JSON
}

func (m *mockClusterExecutor) ExecuteCommandWithTimeout(_ time.Duration, name string, args ...string) shell.CommandResult {
	return m.ExecuteCommand(name, args...)
}

func (m *mockClusterExecutor) ExecuteCommandStream(name string, args ...string) error {
	return nil
}
