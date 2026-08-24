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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSetCmd_Strict(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cmd := configSetCmd

	err := cmd.RunE(cmd, []string{"project", ""})
	if err == nil || !strings.Contains(err.Error(), "configuration values cannot be empty") {
		t.Fatalf("Expected empty value error, got %v", err)
	}

	err = cmd.RunE(cmd, []string{"project"})
	if err == nil || !strings.Contains(err.Error(), "requires both a key and a value") {
		t.Fatalf("Expected error for 1 argument, got %v", err)
	}

	err = cmd.RunE(cmd, []string{"project", "my-super-project"})
	if err != nil {
		t.Fatalf("Unexpected error for valid set project: %v", err)
	}

	ctx, _ := loadContext()
	if ctx.ProjectID != "my-super-project" {
		t.Fatalf("Expected 'my-super-project', got '%v'", ctx.ProjectID)
	}

	err = cmd.RunE(cmd, []string{"invalidkey", "value"})
	if err == nil || !strings.Contains(err.Error(), "unknown configuration key") {
		t.Fatalf("Expected unknown key error, got %v", err)
	}
}

func TestConfigShowCmd_Strict(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	file := filepath.Join(tempHome, ".gcluster", "context.json")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(file, []byte("{\"project\": \"test-show-project\", \"cluster\": \"show-cluster\", \"location\": \"us-west\"}"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := configShowCmd
	b := bytes.NewBufferString("")
	cmd.SetOut(b)

	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("Unexpected error running show: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, "test-show-project") || !strings.Contains(out, "show-cluster") {
		t.Fatalf("Expected formatted output containing test-show-project, got: %s", out)
	}
}
