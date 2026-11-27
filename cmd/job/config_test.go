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
	"slices"
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
			t.Setenv("USERPROFILE", tempHome)

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
	t.Setenv("USERPROFILE", tempHome)

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

func TestParseEditorArgs(t *testing.T) {
	tests := []struct {
		name      string
		editor    string
		expected  []string
		expectErr string
	}{
		{
			name:     "editor with flags",
			editor:   "code --wait",
			expected: []string{"code", "--wait"},
		},
		{
			name:     "quoted executable path with spaces",
			editor:   `"/Applications/Sublime Text.app/subl" -w`,
			expected: []string{"/Applications/Sublime Text.app/subl", "-w"},
		},
		{
			name:     "empty quoted flag is preserved",
			editor:   `emacsclient -c -a ""`,
			expected: []string{"emacsclient", "-c", "-a", ""},
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
			got, err := parseEditorArgs(tc.editor)
			if tc.expectErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tc.expected) {
				t.Errorf("expected %q, got %q", tc.expected, got)
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

func TestConfigUnsetCmd(t *testing.T) {
	initial := Context{ProjectID: "proj-1", ClusterName: "clus-1", Location: "us-central1"}
	tests := []struct {
		name      string
		corrupt   bool
		args      []string
		wantCtx   Context
		wantErr   string
		wantPrint string
	}{
		{
			name:      "unset_project",
			args:      []string{"project"},
			wantCtx:   Context{ClusterName: "clus-1", Location: "us-central1"},
			wantPrint: "Cleared default for 'project'",
		},
		{
			name:      "unset_cluster",
			args:      []string{"cluster"},
			wantCtx:   Context{ProjectID: "proj-1", Location: "us-central1"},
			wantPrint: "Cleared default for 'cluster'",
		},
		{
			name:      "unset_location",
			args:      []string{"location"},
			wantCtx:   Context{ProjectID: "proj-1", ClusterName: "clus-1"},
			wantPrint: "Cleared default for 'location'",
		},
		{
			name:      "trims_and_lowercases_key",
			args:      []string{" PROJECT "},
			wantCtx:   Context{ClusterName: "clus-1", Location: "us-central1"},
			wantPrint: "Cleared default for 'project'",
		},
		{
			name:      "unset_all",
			args:      []string{"all"},
			wantCtx:   Context{},
			wantPrint: "Cleared all saved defaults",
		},
		{
			name:      "unset_all_recovers_corrupt_context_file",
			corrupt:   true,
			args:      []string{"all"},
			wantCtx:   Context{},
			wantPrint: "Cleared all saved defaults",
		},
		{
			name:    "unset_single_key_on_corrupt_context_fails_cleanly",
			corrupt: true,
			args:    []string{"project"},
			wantErr: "gcluster job config unset all",
		},
		{
			name:    "missing_key_arg_fails",
			args:    nil,
			wantCtx: initial,
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "unknown_key_fails",
			args:    []string{"bogus"},
			wantCtx: initial,
			wantErr: "unknown configuration key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			seedConfigUnsetState(t, initial, tc.corrupt)
			out, err := executeCommand(JobCmd, append([]string{"config", "unset"}, tc.args...)...)
			assertConfigUnsetOutcome(t, tc.args, out, err, tc.wantErr, tc.wantPrint, tc.wantCtx, tc.corrupt)
		})
	}
}

func seedConfigUnsetState(t *testing.T, initial Context, corrupt bool) {
	t.Helper()
	if corrupt {
		p := filepath.Join(os.Getenv("HOME"), ".gcluster", "context.json")
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte("{corrupt-json"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		return
	}
	if err := saveContext(initial); err != nil {
		t.Fatalf("saveContext: %v", err)
	}
}

func assertConfigUnsetOutcome(t *testing.T, args []string, out string, err error, wantErr, wantPrint string, wantCtx Context, corrupt bool) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("executeCommand(config unset %v) err = %v, want %q", args, err, wantErr)
		}
		if corrupt {
			return
		}
	} else if err != nil {
		t.Fatalf("executeCommand(config unset %v) err = %v, want nil", args, err)
	}
	got, loadErr := loadContext()
	if loadErr != nil {
		t.Fatalf("loadContext: %v", loadErr)
	}
	if got != wantCtx {
		t.Errorf("loadContext() = %+v, want %+v", got, wantCtx)
	}
	if wantPrint != "" && !strings.Contains(out, wantPrint) {
		t.Errorf("output = %q, want substring %q", out, wantPrint)
	}
}

func TestConfigCmd_ReportsTelemetryProjectID(t *testing.T) {
	origHook := OnTelemetryProjectResolved
	t.Cleanup(func() { OnTelemetryProjectResolved = origHook })

	var reported string
	OnTelemetryProjectResolved = func(p string) {
		reported = p
	}

	t.Run("config set project reports project", func(t *testing.T) {
		setupSubmitTestEnv(t)
		reported = ""
		if _, err := executeCommand(JobCmd, "config", "set", "project", "cfg-proj"); err != nil {
			t.Fatalf("config set project err = %v", err)
		}
		if reported != "cfg-proj" {
			t.Errorf("reported = %q, want %q", reported, "cfg-proj")
		}
	})

	t.Run("config show reports saved project", func(t *testing.T) {
		setupSubmitTestEnv(t)
		if err := saveContext(Context{ProjectID: "shown-proj"}); err != nil {
			t.Fatalf("saveContext: %v", err)
		}
		reported = ""
		if _, err := executeCommand(JobCmd, "config", "show"); err != nil {
			t.Fatalf("config show err = %v", err)
		}
		if reported != "shown-proj" {
			t.Errorf("reported = %q, want %q", reported, "shown-proj")
		}
	})

	t.Run("validateAndApplyConfig reports project from edited JSON", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "tmp.json")
		dst := filepath.Join(dir, "context.json")
		if err := os.WriteFile(src, []byte(`{"project":"editor-proj","cluster":"c1","location":"us-central1"}`), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		reported = ""
		if err := validateAndApplyConfig(src, dst); err != nil {
			t.Fatalf("validateAndApplyConfig err = %v", err)
		}
		if reported != "editor-proj" {
			t.Errorf("reported = %q, want %q", reported, "editor-proj")
		}
	})

	t.Run("config unset all reports empty project", func(t *testing.T) {
		setupSubmitTestEnv(t)
		if err := saveContext(Context{ProjectID: "old-proj"}); err != nil {
			t.Fatalf("saveContext: %v", err)
		}
		reported = "sentinel"
		if _, err := executeCommand(JobCmd, "config", "unset", "all"); err != nil {
			t.Fatalf("config unset all err = %v", err)
		}
		if reported != "" {
			t.Errorf("reported = %q, want empty", reported)
		}
	})
}
