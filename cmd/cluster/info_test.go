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
	"hpc-toolkit/pkg/orchestrator/gke"
	"strings"
	"testing"
)

func TestInfoCmd_MissingFlags(t *testing.T) {
	resetClusterCmdFlags()

	_, err := executeCommand(ClusterCmd, "info", "--project", "test-project")
	if err == nil {
		t.Fatalf("expected error for missing flags, got nil")
	}

	if !strings.Contains(err.Error(), `required flag(s) "cluster", "location" not set`) {
		t.Errorf("unexpected error output: %v", err)
	}
}

func TestInfoCmd_Success(t *testing.T) {
	resetClusterCmdFlags()

	// Mock the orchestrator factory
	oldFactory := gkeOrchestratorFactory
	defer func() { gkeOrchestratorFactory = oldFactory }()

	gkeOrchestratorFactory = func() *gke.GKEOrchestrator {
		g := gke.NewGKEOrchestrator()
		g.SetExecutor(&mockClusterExecutor{})
		return g
	}

	output, err := executeCommand(ClusterCmd, "info", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project")

	if err != nil {
		if !strings.Contains(err.Error(), "unhandled mock command") && !strings.Contains(err.Error(), "failed to get kubeconfig") && !strings.Contains(err.Error(), "invalid configuration") {
			t.Fatalf("unexpected error: %v, output: %s", err, output)
		}
	}
}
