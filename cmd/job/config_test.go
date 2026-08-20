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
	"testing"
)

func TestConfigSetCmd_NoGlobalFlagsRequired(t *testing.T) {
	tempDir := t.TempDir()

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

func TestConfigSetCmd_BatchAssignment(t *testing.T) {
	tempDir := t.TempDir()

	t.Setenv("HOME", tempDir)

	output, err := executeCommand(JobCmd, "config", "set", "project=my-project", "cluster=my-cluster", "location=my-location")
	if err != nil {
		t.Fatalf("config set failed unexpectedly: %v, output: %s", err, output)
	}

	ctx := loadContext()
	if ctx.ProjectID != "my-project" {
		t.Errorf("expected project 'my-project', got '%s'", ctx.ProjectID)
	}
	if ctx.ClusterName != "my-cluster" {
		t.Errorf("expected cluster 'my-cluster', got '%s'", ctx.ClusterName)
	}
	if ctx.Location != "my-location" {
		t.Errorf("expected location 'my-location', got '%s'", ctx.Location)
	}
}

func TestConfigSetCmd_InvalidBatch(t *testing.T) {
	tempDir := t.TempDir()

	t.Setenv("HOME", tempDir)

	output, err := executeCommand(JobCmd, "config", "set", "project=my-project", "invalid-no-equals")
	if err == nil {
		t.Fatalf("expected config set to fail with invalid argument format, but it succeeded: %s", output)
	}
}

func TestConfigSetCmd_BackwardsCompatibleWithEquals(t *testing.T) {
	tempDir := t.TempDir()

	t.Setenv("HOME", tempDir)

	output, err := executeCommand(JobCmd, "config", "set", "project", "my-project=foo")
	if err != nil {
		t.Fatalf("config set failed unexpectedly: %v, output: %s", err, output)
	}

	ctx := loadContext()
	if ctx.ProjectID != "my-project=foo" {
		t.Errorf("expected project 'my-project=foo', got '%s'", ctx.ProjectID)
	}
}

func TestConfigSetCmd_LegacyBug(t *testing.T) {
	tempDir := t.TempDir()

	t.Setenv("HOME", tempDir)

	output, err := executeCommand(JobCmd, "config", "set", "project", "cluster=foo")
	if err == nil {
		t.Fatalf("expected config set to fail with invalid argument format for suspicious value, but it succeeded: %s", output)
	}
}
