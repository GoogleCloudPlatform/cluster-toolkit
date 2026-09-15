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
	"encoding/json"
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
				if ctx != tc.expectedCtx {
					t.Errorf("expected context %+v, got %+v", tc.expectedCtx, ctx)
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

func TestRunSystemEditor(t *testing.T) {
	tests := []struct {
		name      string
		editor    string
		expectErr string
	}{
		{
			name:   "editor with flags succeeds",
			editor: "true --wait",
		},
		{
			name:   "quoted executable path succeeds",
			editor: `"/bin/echo" -n`,
		},
		{
			name:   "empty quoted flag succeeds",
			editor: `true ""`,
		},
		{
			name:      "empty editor returns error",
			editor:    "",
			expectErr: "editor command is empty",
		},
		{
			name:      "unclosed quote returns error",
			editor:    `vim "unclosed`,
			expectErr: "unclosed quote in editor command",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runSystemEditor(tc.editor, "/dev/null")
			if tc.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func assertSavedConfig(t *testing.T, targetFile, expectedTarget string, expectedCtx *Context) {
	t.Helper()
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatal(err)
	}
	if expectedTarget != "" && string(data) != expectedTarget {
		t.Errorf("expected target file %q, got %q", expectedTarget, string(data))
	}
	if expectedCtx != nil {
		var ctx Context
		if err := json.Unmarshal(data, &ctx); err != nil {
			t.Fatalf("failed to unmarshal target file: %v", err)
		}
		if ctx != *expectedCtx {
			t.Errorf("expected context %+v, got %+v", *expectedCtx, ctx)
		}
	}
}

func TestValidateAndApplyConfig(t *testing.T) {
	tests := []struct {
		name           string
		tempContent    string
		initialTarget  string
		expectedTarget string
		expectedCtx    *Context
		expectErr      string
	}{
		{
			name:           "empty content sets empty json object",
			tempContent:    "",
			expectedTarget: "{}",
		},
		{
			name:           "valid json applied successfully",
			tempContent:    `{"project": "proj-1", "cluster": "clus-1", "location": "us-central1"}`,
			expectedTarget: `{"project": "proj-1", "cluster": "clus-1", "location": "us-central1"}`,
			expectedCtx:    &Context{ProjectID: "proj-1", ClusterName: "clus-1", Location: "us-central1"},
		},
		{
			name:          "invalid json returns error and preserves target",
			tempContent:   `{not-valid-json`,
			initialTarget: `{"project": "original"}`,
			expectErr:     "config file contains structural errors or invalid JSON",
		},
		{
			name:        "unknown fields in json returns error",
			tempContent: `{"project": "proj", "extra": "invalid"}`,
			expectErr:   "config file contains structural errors or invalid JSON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tempFile := filepath.Join(tempDir, "temp.json")
			targetFile := filepath.Join(tempDir, "target.json")

			if err := os.WriteFile(tempFile, []byte(tc.tempContent), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(targetFile, []byte(tc.initialTarget), 0644); err != nil {
				t.Fatal(err)
			}

			err := validateAndApplyConfig(tempFile, targetFile)

			if tc.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectErr, err)
				}
				assertSavedConfig(t, targetFile, tc.initialTarget, nil)
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertSavedConfig(t, targetFile, tc.expectedTarget, tc.expectedCtx)
		})
	}
}
