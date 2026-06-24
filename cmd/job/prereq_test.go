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
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestIsStateStale(t *testing.T) {
	now := time.Now()
	freshTime := now.Add(-1 * time.Hour)
	staleTime := now.Add(-48 * time.Hour)

	tests := []struct {
		name             string
		state            PrereqState
		currentProjectID string
		wantStale        bool
	}{
		{
			name: "Fresh state, same project",
			state: PrereqState{
				LastCheckedTimestamp: freshTime,
				LastCheckedProjectID: "test-project",
			},
			currentProjectID: "test-project",
			wantStale:        false,
		},
		{
			name: "Stale state (time), same project",
			state: PrereqState{
				LastCheckedTimestamp: staleTime,
				LastCheckedProjectID: "test-project",
			},
			currentProjectID: "test-project",
			wantStale:        true,
		},
		{
			name: "Fresh state, different project",
			state: PrereqState{
				LastCheckedTimestamp: freshTime,
				LastCheckedProjectID: "old-project",
			},
			currentProjectID: "new-project",
			wantStale:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStateStale(tt.state, tt.currentProjectID)
			if got != tt.wantStale {
				t.Errorf("isStateStale() = %v, want %v", got, tt.wantStale)
			}
		})
	}
}

func TestStateFilePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "prereq-test-home")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	path, err := stateFilePath()
	if err != nil {
		t.Fatalf("stateFilePath() error = %v", err)
	}

	expectedPrefix := filepath.Join(tempDir, stateDirName)
	if !strings.HasPrefix(path, expectedPrefix) {
		t.Errorf("expected path to start with %s, got %s", expectedPrefix, path)
	}
}

func TestLoadPrereqState_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "prereq-load-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	stateDir := filepath.Join(tempDir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(stateDir, stateFileName)

	state := PrereqState{
		GCloudSDKInstalled:   true,
		LastCheckedProjectID: "test-project",
		LastCheckedTimestamp: time.Now(),
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded := store.Load()
	if !loaded.GCloudSDKInstalled {
		t.Errorf("expected loaded state to be true for GCloudSDKInstalled, got false")
	}
	if loaded.LastCheckedProjectID != "test-project" {
		t.Errorf("expected loaded state project ID to be 'test-project', got %s", loaded.LastCheckedProjectID)
	}
}

func TestSavePrereqState_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "prereq-save-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	state := PrereqState{
		GCloudAuthenticated:  true,
		LastCheckedProjectID: "test-project",
		LastCheckedTimestamp: time.Now(),
	}

	store.Save(state)

	stateDir := filepath.Join(tempDir, stateDirName)
	statePath := filepath.Join(stateDir, stateFileName)

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatalf("state file was not created at %s", statePath)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read created state file: %v", err)
	}

	var savedState PrereqState
	if err := json.Unmarshal(data, &savedState); err != nil {
		t.Fatalf("failed to unmarshal saved state: %v", err)
	}

	if !savedState.GCloudAuthenticated {
		t.Errorf("expected saved state to be true for GCloudAuthenticated, got false")
	}
}

func TestLoadPrereqState_CorruptedFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "prereq-corrupt-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	stateDir := filepath.Join(tempDir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(stateDir, stateFileName)

	if err := os.WriteFile(statePath, []byte("invalid-json"), 0644); err != nil {
		t.Fatal(err)
	}

	loaded := store.Load()
	if loaded.GCloudSDKInstalled {
		t.Errorf("expected empty state for corrupted file, got GCloudSDKInstalled=true")
	}
}

func TestSavePrereqState_WriteError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "prereq-write-error-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	stateDir := filepath.Join(tempDir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Make directory read-only to force write error
	if err := os.Chmod(stateDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(stateDir, 0755) // Restore to allow cleanup

	state := PrereqState{
		GCloudAuthenticated: true,
	}

	// This should log an error but not panic
	store.Save(state)
}

func TestEnsureGCloudSDKInstalled_Success(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = origExecuteCommand }()

	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		if name == "gcloud" && len(args) > 0 && args[0] == "version" {
			return shell.CommandResult{ExitCode: 0, Stdout: "Google Cloud SDK 123.0.0"}
		}
		return shell.CommandResult{ExitCode: 1}
	}

	err := ensureGCloudSDKInstalled()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestEnsureGCloudSDKInstalled_Failure(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = origExecuteCommand }()

	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		return shell.CommandResult{ExitCode: 1, Stderr: "command not found"}
	}

	err := ensureGCloudSDKInstalled()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestEnsureGCloudAuthenticated_Success(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = origExecuteCommand }()

	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		if name == "gcloud" && len(args) > 1 && args[0] == "auth" {
			return shell.CommandResult{ExitCode: 0, Stdout: "user@example.com"}
		}
		return shell.CommandResult{ExitCode: 0}
	}

	err := ensureGCloudAuthenticated()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestEnsureGCloudAuthenticated_Failure(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = origExecuteCommand }()

	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		if name == "gcloud" && len(args) > 1 && args[0] == "auth" {
			return shell.CommandResult{ExitCode: 0, Stdout: ""}
		}
		return shell.CommandResult{ExitCode: 0}
	}

	err := ensureGCloudAuthenticated()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestEnsureApplicationDefaultCredentials_Failure(t *testing.T) {
	origCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	defer os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origCreds)

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/non/existent/file.json")

	tempDir, err := os.MkdirTemp("", "adc-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	got := getADCSetupCommand()
	want := "gcloud auth application-default login"
	if got != want {
		t.Errorf("getADCSetupCommand() = %q, want %q", got, want)
	}
}

func TestIsDockerCredsConfigured(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	dockerDir := filepath.Join(tempDir, ".docker")
	configPath := filepath.Join(dockerDir, "config.json")

	tests := []struct {
		name          string
		configContent string
		shouldExist   bool
		region        string
		expected      bool
	}{
		{
			name:        "No config file",
			shouldExist: false,
			region:      "us-central1",
			expected:    false,
		},
		{
			name:          "Corrupted config file",
			configContent: "invalid",
			shouldExist:   true,
			region:        "us-central1",
			expected:      false,
		},
		{
			name:          "CredsStore set to gcloud",
			configContent: `{"credsStore": "gcloud"}`,
			shouldExist:   true,
			region:        "us-central1",
			expected:      true,
		},
		{
			name:          "CredHelpers configured correctly",
			configContent: `{"credHelpers": {"gcr.io": "gcloud", "us-central1-docker.pkg.dev": "gcloud"}}`,
			shouldExist:   true,
			region:        "us-central1",
			expected:      true,
		},
		{
			name:          "CredHelpers missing gcr.io",
			configContent: `{"credHelpers": {"us-central1-docker.pkg.dev": "gcloud"}}`,
			shouldExist:   true,
			region:        "us-central1",
			expected:      false,
		},
		{
			name:          "CredHelpers missing regional registry",
			configContent: `{"credHelpers": {"gcr.io": "gcloud"}}`,
			shouldExist:   true,
			region:        "us-central1",
			expected:      false,
		},
		{
			name:          "CredHelpers wrong helper",
			configContent: `{"credHelpers": {"gcr.io": "desktop", "us-central1-docker.pkg.dev": "gcloud"}}`,
			shouldExist:   true,
			region:        "us-central1",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.shouldExist {
				os.Remove(configPath)
			} else {
				if err := os.MkdirAll(dockerDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, []byte(tt.configContent), 0644); err != nil {
					t.Fatal(err)
				}
			}

			got := isDockerCredsConfigured(tt.region)
			if got != tt.expected {
				t.Errorf("isDockerCredsConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEnsurePrerequisites_DockerCreds(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	origExecuteCommand := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = origExecuteCommand }()

	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		if name == "gcloud" && len(args) > 1 && args[0] == "auth" && args[1] == "list" {
			return shell.CommandResult{ExitCode: 0, Stdout: "user@example.com"}
		}
		if name == "gcloud" && len(args) > 1 && args[0] == "services" && args[1] == "list" {
			return shell.CommandResult{ExitCode: 0, Stdout: "artifactregistry.googleapis.com"}
		}
		return shell.CommandResult{ExitCode: 0}
	}

	origStore := store
	defer func() { store = origStore }()
	store = &mockPrereqStore{}

	cmd := &cobra.Command{}
	projectID := "test-project"
	location := "us-central1-a"

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := ensurePrerequisites(cmd, &projectID, location)
	if err == nil {
		t.Error("expected error because prerequisites are missing, got nil")
	}

	output := buf.String()
	expectedCmd := "gcloud auth configure-docker us-central1-docker.pkg.dev --quiet"
	if !strings.Contains(output, expectedCmd) {
		t.Errorf("expected output to contain %q, but got:\n%s", expectedCmd, output)
	}
}

type mockPrereqStore struct{}

func (m *mockPrereqStore) Load() PrereqState {
	return PrereqState{
		LastCheckedTimestamp: time.Now().Add(-48 * time.Hour), // Stale
	}
}

func (m *mockPrereqStore) Save(state PrereqState) {}
