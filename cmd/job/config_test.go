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

func TestConfigSetCmd(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedCtx Context
		expectErr   string
	}{
		{
			name:      "empty value fails",
			args:      []string{"project", ""},
			expectErr: "configuration values cannot be empty",
		},
		{
			name:      "single argument fails",
			args:      []string{"project"},
			expectErr: "requires both a key and a value",
		},
		{
			name:      "invalid key fails",
			args:      []string{"invalidkey", "value"},
			expectErr: "unknown configuration key",
		},
		{
			name: "valid project sets successfully",
			args: []string{"project", "my-super-project"},
			expectedCtx: Context{
				ProjectID: "my-super-project",
			},
		},
		{
			name: "valid cluster sets successfully",
			args: []string{"cluster", "my-super-cluster"},
			expectedCtx: Context{
				ClusterName: "my-super-cluster",
			},
		},
		{
			name: "valid location sets successfully",
			args: []string{"location", "us-west1-a"},
			expectedCtx: Context{
				Location: "us-west1-a",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempHome := t.TempDir()
			t.Setenv("HOME", tempHome)

			cmd := configSetCmd
			err := cmd.RunE(cmd, tc.args)

			if tc.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Fatalf("expected error containing %q, got %v", tc.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				ctx, err := loadContext()
				if err != nil {
					t.Fatalf("failed to load context: %v", err)
				}
				if tc.expectedCtx.ProjectID != "" && ctx.ProjectID != tc.expectedCtx.ProjectID {
					t.Errorf("expected ProjectID %q, got %q", tc.expectedCtx.ProjectID, ctx.ProjectID)
				}
				if tc.expectedCtx.ClusterName != "" && ctx.ClusterName != tc.expectedCtx.ClusterName {
					t.Errorf("expected ClusterName %q, got %q", tc.expectedCtx.ClusterName, ctx.ClusterName)
				}
				if tc.expectedCtx.Location != "" && ctx.Location != tc.expectedCtx.Location {
					t.Errorf("expected Location %q, got %q", tc.expectedCtx.Location, ctx.Location)
				}
			}
		})
	}
}

func TestConfigShowCmd(t *testing.T) {
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
		t.Fatalf("unexpected error running show: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, "test-show-project") || !strings.Contains(out, "show-cluster") {
		t.Fatalf("expected formatted output containing test-show-project, got: %s", out)
	}
}
