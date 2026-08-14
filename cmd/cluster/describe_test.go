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
	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"
	"strings"
	"testing"

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

func (m *mockClusterExecutor) ExecuteCommandStream(name string, args ...string) error {
	return nil
}
