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

package job

import (
	"os"
	"testing"
)

func TestConfigSetCmd_NoGlobalFlagsRequired(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-test-home")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Setenv("HOME", tempDir)

	// We do NOT set cluster, location, or project flags here.
	// The command should still succeed because we overrode PersistentPreRunE for ConfigCmd.
	output, err := executeCommand(JobCmd, "config", "set", "cluster", "my-new-cluster")

	if err != nil {
		t.Fatalf("config set failed unexpectedly: %v, output: %s", err, output)
	}

	// Verify that the context was saved correctly
	ctx := loadContext()
	if ctx.ClusterName != "my-new-cluster" {
		t.Errorf("expected cluster name to be 'my-new-cluster', got '%s'", ctx.ClusterName)
	}
}
