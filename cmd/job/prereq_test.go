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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/shell"
	"os"
	"os/exec"
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

func useFileStore(t *testing.T) {
	oldStore := store
	store = &FilePrereqStore{}
	t.Cleanup(func() {
		store = oldStore
	})
}

func TestLoadPrereqState_Success(t *testing.T) {
	useFileStore(t)
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
	useFileStore(t)
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
	useFileStore(t)
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
	useFileStore(t)
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
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
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
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	t.Run("Non-zero exit code", func(t *testing.T) {
		shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: 1, Stderr: "command error"}
		}
		err := ensureGCloudSDKInstalled()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Please install it from") {
			t.Errorf("expected installation prompt in error, got %v", err)
		}
	})

	t.Run("Binary not found (ExitCode -1)", func(t *testing.T) {
		shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
			return shell.CommandResult{ExitCode: -1, Err: &exec.Error{Name: "gcloud", Err: exec.ErrNotFound}}
		}
		err := ensureGCloudSDKInstalled()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Please install it from") {
			t.Errorf("expected installation prompt in error, got %v", err)
		}
	})
}

func TestEnsureGCloudAuthenticated_Success(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
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
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
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

func TestEnsureGCloudSDKInstalled_Timeout(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if timeout != shell.DefaultLocalCommandTimeout {
			t.Errorf("expected timeout %v, got %v", shell.DefaultLocalCommandTimeout, timeout)
		}
		return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
	}

	err := ensureGCloudSDKInstalled()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	wantSubstr := fmt.Sprintf("gcloud version check timed out after %v", shell.DefaultLocalCommandTimeout)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error to contain %q, got %v", wantSubstr, err)
	}
	if strings.Contains(err.Error(), "Please install it from") {
		t.Errorf("did not expect timeout error to suggest installing gcloud, got %v", err)
	}
}

func TestEnsureGCloudAuthenticated_Timeout(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if timeout != shell.DefaultLocalCommandTimeout {
			t.Errorf("expected timeout %v, got %v", shell.DefaultLocalCommandTimeout, timeout)
		}
		return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
	}

	err := ensureGCloudAuthenticated()
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	wantSubstr := fmt.Sprintf("gcloud authentication check timed out after %v", shell.DefaultLocalCommandTimeout)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error to contain %q, got %v", wantSubstr, err)
	}
}

func TestEnsureBasicPrerequisites_GCloudAuthTimeout(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if name == "gcloud" && len(args) > 1 && args[0] == "auth" && args[1] == "list" {
			return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
		}
		return shell.CommandResult{ExitCode: 0}
	}

	origStore := store
	defer func() { store = origStore }()
	store = &mockPrereqStore{}

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := ensureBasicPrerequisites(cmd, "test-project")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected error to wrap context.DeadlineExceeded, got %v", err)
	}
	wantSubstr := fmt.Sprintf("gcloud authentication check timed out after %v", shell.DefaultLocalCommandTimeout)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error to contain %q, got %v", wantSubstr, err)
	}
	if strings.Contains(buf.String(), "gcloud auth login") {
		t.Errorf("did not expect 'gcloud auth login' prompt on timeout, got output:\n%s", buf.String())
	}
}

func TestEnsureProjectExists_Timeout(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if timeout != shell.DefaultCloudAPITimeout {
			t.Errorf("expected timeout %v, got %v", shell.DefaultCloudAPITimeout, timeout)
		}
		return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
	}

	err := ensureProjectExists("test-project")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	wantSubstr := fmt.Sprintf("gcloud project validation timed out after %v", shell.DefaultCloudAPITimeout)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error to contain %q, got %v", wantSubstr, err)
	}
}

func TestCheckArtifactRegistryAPI_Timeout(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if timeout != shell.DefaultCloudAPITimeout {
			t.Errorf("expected timeout %v, got %v", shell.DefaultCloudAPITimeout, timeout)
		}
		return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
	}

	var state PrereqState
	var missing []missingPrereq

	checkArtifactRegistryAPI("test-project", &state, &missing)

	if len(missing) != 1 {
		t.Fatalf("expected 1 missing prereq, got %d", len(missing))
	}

	expectedName := fmt.Sprintf("Artifact Registry API (verification timed out after %v)", shell.DefaultCloudAPITimeout)
	if missing[0].name != expectedName {
		t.Errorf("missing[0].name = %q, want %q", missing[0].name, expectedName)
	}
	if state.ArtifactRegistryAPIEnabled {
		t.Error("expected ArtifactRegistryAPIEnabled to remain false on timeout")
	}
}

func TestCheckK8sDependencies_Timeout(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()
	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		if timeout != shell.DefaultLocalCommandTimeout {
			t.Errorf("expected timeout %v, got %v", shell.DefaultLocalCommandTimeout, timeout)
		}
		return shell.CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
	}
	var state PrereqState
	var missing []missingPrereq
	checkK8sDependencies(&state, &missing)
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing prereqs, got %d", len(missing))
	}
	wantKubectl := fmt.Sprintf("kubectl (installed, but did not respond within %v)", shell.DefaultLocalCommandTimeout)
	if missing[0].name != wantKubectl {
		t.Errorf("missing[0].name = %q, want %q", missing[0].name, wantKubectl)
	}
	wantPlugin := fmt.Sprintf("gke-gcloud-auth-plugin (installed, but did not respond within %v)", shell.DefaultLocalCommandTimeout)
	if missing[1].name != wantPlugin {
		t.Errorf("missing[1].name = %q, want %q", missing[1].name, wantPlugin)
	}
	if state.KubectlInstalled || state.GKEGCloudAuthPluginInstalled {
		t.Error("expected K8s dependency state flags to remain false on timeout")
	}
}

func TestCheckK8sDependencies_Missing(t *testing.T) {
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		return shell.CommandResult{ExitCode: -1, Err: &exec.Error{Name: name, Err: exec.ErrNotFound}}
	}

	var state PrereqState
	var missing []missingPrereq
	checkK8sDependencies(&state, &missing)

	if len(missing) != 2 {
		t.Fatalf("expected 2 missing prereqs, got %d", len(missing))
	}
	for i, bin := range []string{"kubectl", "gke-gcloud-auth-plugin"} {
		if missing[i].name != bin {
			t.Errorf("missing[%d].name = %q, want %q", i, missing[i].name, bin)
		}
		if len(missing[i].commands) != 2 {
			t.Fatalf("expected 2 install commands for %s, got %d", bin, len(missing[i].commands))
		}
		wantCmd0 := fmt.Sprintf("gcloud components install %s --quiet   # gcloud from Google's installer", bin)
		wantCmd1 := fmt.Sprintf("# or install %s with your OS package manager (apt/dnf/brew) if gcloud came from there", bin)
		if missing[i].commands[0] != wantCmd0 {
			t.Errorf("missing[%d].commands[0] = %q, want %q", i, missing[i].commands[0], wantCmd0)
		}
		if missing[i].commands[1] != wantCmd1 {
			t.Errorf("missing[%d].commands[1] = %q, want %q", i, missing[i].commands[1], wantCmd1)
		}
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
	t.Setenv("USERPROFILE", tempDir) // os.UserHomeDir on Windows
	t.Setenv("DOCKER_CONFIG", "")

	dockerDir := filepath.Join(tempDir, ".docker")
	configPath := filepath.Join(dockerDir, "config.json")
	const registry = "us-central1-docker.pkg.dev"

	tests := []struct {
		name          string
		configContent string
		shouldExist   bool
		expected      bool
	}{
		{
			name:        "No config file",
			shouldExist: false,
			expected:    false,
		},
		{
			name:          "Corrupted config file",
			configContent: "invalid",
			shouldExist:   true,
			expected:      false,
		},
		{
			name:          "CredsStore set to gcloud",
			configContent: `{"credsStore": "gcloud"}`,
			shouldExist:   true,
			expected:      true,
		},
		{
			name:          "CredHelpers configured for gcr.io and the regional registry",
			configContent: `{"credHelpers": {"gcr.io": "gcloud", "us-central1-docker.pkg.dev": "gcloud"}}`,
			shouldExist:   true,
			expected:      true,
		},
		{
			name:          "CredHelpers configured for the regional registry only",
			configContent: `{"credHelpers": {"us-central1-docker.pkg.dev": "gcloud"}}`,
			shouldExist:   true,
			expected:      true,
		},
		{
			name:          "CredHelpers missing regional registry",
			configContent: `{"credHelpers": {"gcr.io": "gcloud"}}`,
			shouldExist:   true,
			expected:      false,
		},
		{
			name:          "CredHelpers wrong helper for the regional registry",
			configContent: `{"credHelpers": {"gcr.io": "gcloud", "us-central1-docker.pkg.dev": "desktop"}}`,
			shouldExist:   true,
			expected:      false,
		},
		{
			name:          "CredHelpers overrides CredsStore for the regional registry",
			configContent: `{"credsStore": "gcloud", "credHelpers": {"us-central1-docker.pkg.dev": "desktop"}}`,
			shouldExist:   true,
			expected:      false,
		},
		{
			name:          "CredHelpers set to gcr for the regional registry",
			configContent: `{"credHelpers": {"us-central1-docker.pkg.dev": "gcr"}}`,
			shouldExist:   true,
			expected:      true,
		},
		{
			name:          "CredsStore set to gcr",
			configContent: `{"credsStore": "gcr"}`,
			shouldExist:   true,
			expected:      true,
		},
		{
			name:          "Auths entry for the regional registry",
			configContent: `{"auths": {"us-central1-docker.pkg.dev": {"auth": "b2F1dGgyYWNjZXNzdG9rZW46dG9rZW4="}}}`,
			shouldExist:   true,
			expected:      true,
		},
		{
			name:          "Auths entry with https prefix for the regional registry",
			configContent: `{"auths": {"https://us-central1-docker.pkg.dev": {"auth": "b2F1dGgyYWNjZXNzdG9rZW46dG9rZW4="}}}`,
			shouldExist:   true,
			expected:      true,
		},
		{
			name:          "Empty auths entry for the regional registry",
			configContent: `{"auths": {"us-central1-docker.pkg.dev": {}}}`,
			shouldExist:   true,
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

			got := isDockerCredsConfigured(registry)
			if got != tt.expected {
				t.Errorf("isDockerCredsConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// stubPrereqCommands makes every local/gcloud command used by the prerequisite
// checks succeed, points HOME and DOCKER_CONFIG at empty directories (no cached
// state or Docker credentials on any OS), and returns the command lines executed.
func stubPrereqCommands(t *testing.T) *[]string {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	useFileStore(t)

	executed := new([]string)
	origExecuteCommand := shell.ExecuteCommandWithTimeout
	t.Cleanup(func() { shell.ExecuteCommandWithTimeout = origExecuteCommand })
	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		*executed = append(*executed, strings.Join(append([]string{name}, args...), " "))
		if name == "gcloud" && len(args) > 1 && args[0] == "auth" && args[1] == "list" {
			return shell.CommandResult{ExitCode: 0, Stdout: "user@example.com"}
		}
		if name == "gcloud" && len(args) > 1 && args[0] == "services" && args[1] == "list" {
			return shell.CommandResult{ExitCode: 0, Stdout: "ENABLED"}
		}
		return shell.CommandResult{ExitCode: 0}
	}

	origGetADCSetupCommand := getADCSetupCommandFunc
	t.Cleanup(func() { getADCSetupCommandFunc = origGetADCSetupCommand })
	getADCSetupCommandFunc = func() string { return "" }
	return executed
}

// writeDockerConfig writes a Docker config with a gcloud credential helper for the
// given registries into the DOCKER_CONFIG directory set by stubPrereqCommands.
func writeDockerConfig(t *testing.T, registries ...string) {
	t.Helper()
	helpers := map[string]string{}
	for _, r := range registries {
		helpers[r] = "gcloud"
	}
	data, err := json.Marshal(map[string]any{"credHelpers": helpers})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("DOCKER_CONFIG"), "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func ranArtifactRegistryCheck(executed []string) bool {
	for _, c := range executed {
		if strings.HasPrefix(c, "gcloud services list") {
			return true
		}
	}
	return false
}

func TestEnsureBuildPrerequisites_MissingDockerCreds(t *testing.T) {
	stubPrereqCommands(t)

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := ensureBuildPrerequisites(cmd, "test-project", "us-central1-a")
	if err == nil {
		t.Fatalf("expected an error without Docker credentials, got nil\noutput:\n%s", buf.String())
	}
	output := buf.String()
	if want := "gcloud auth configure-docker us-central1-docker.pkg.dev --quiet"; !strings.Contains(output, want) {
		t.Errorf("expected output to contain %q, got:\n%s", want, output)
	}
	if strings.Contains(output, "gcr.io") {
		t.Errorf("a Crane build only pushes to Artifact Registry; output must not ask for gcr.io credentials:\n%s", output)
	}
}

// With a fresh state the Artifact Registry API result is reused, but the Docker
// config is read for the current cluster's region on every build.
func TestEnsureBuildPrerequisites_FreshState(t *testing.T) {
	tests := []struct {
		name           string
		dockerRegistry string // registry with a gcloud helper; "" for no Docker config
		wantErr        bool
	}{
		{name: "credentials for the cluster's region skip all checks", dockerRegistry: "us-west4-docker.pkg.dev"},
		{name: "credentials for another region only are not enough", dockerRegistry: "us-central1-docker.pkg.dev", wantErr: true},
		{name: "no Docker config", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executed := stubPrereqCommands(t)
			if tt.dockerRegistry != "" {
				writeDockerConfig(t, tt.dockerRegistry)
			}

			store.Save(PrereqState{
				GCloudSDKInstalled:           true,
				GCloudProjectConfigured:      true,
				GCloudAuthenticated:          true,
				ADCConfigured:                true,
				KubectlInstalled:             true,
				GKEGCloudAuthPluginInstalled: true,
				ArtifactRegistryAPIEnabled:   true,
				LastCheckedProjectID:         "test-project",
				LastCheckedTimestamp:         time.Now(),
			})

			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			err := ensureBuildPrerequisites(cmd, "test-project", "us-west4-a")
			if (err != nil) != tt.wantErr {
				t.Fatalf("ensureBuildPrerequisites() error = %v, wantErr %v\noutput:\n%s", err, tt.wantErr, buf.String())
			}
			if got := strings.Contains(buf.String(), "gcloud auth configure-docker us-west4-docker.pkg.dev --quiet"); got != tt.wantErr {
				t.Errorf("remediation for us-west4 shown = %v, want %v\noutput:\n%s", got, tt.wantErr, buf.String())
			}
			if ran := len(*executed) > 0; ran != tt.wantErr {
				t.Errorf("executed %v; want checks to run = %v", *executed, tt.wantErr)
			}
		})
	}
}

type mockPrereqStore struct{}

func (m *mockPrereqStore) Load() PrereqState {
	return PrereqState{
		LastCheckedTimestamp: time.Now().Add(-48 * time.Hour), // Stale
	}
}

func (m *mockPrereqStore) Save(state PrereqState) {}

func TestEnsureBasicPrerequisites_InvalidProject(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		cmdStr := name + " " + strings.Join(args, " ")
		switch {
		case strings.HasPrefix(cmdStr, "gcloud auth list"):
			return shell.CommandResult{ExitCode: 0, Stdout: "user@example.com"}
		case cmdStr == "gcloud projects describe invalid-project":
			return shell.CommandResult{ExitCode: 1, Stderr: "Project not found"}
		default:
			return shell.CommandResult{ExitCode: 0}
		}
	}

	origStore := store
	defer func() { store = origStore }()
	store = &mockPrereqStore{}

	origGetADCSetupCommand := getADCSetupCommandFunc
	defer func() { getADCSetupCommandFunc = origGetADCSetupCommand }()
	getADCSetupCommandFunc = func() string { return "" }

	cmd := &cobra.Command{}
	projectID := "invalid-project"

	err := ensureBasicPrerequisites(cmd, projectID)
	if err == nil {
		t.Fatal("expected error because project is invalid, got nil")
	}

	expectedErrorMsg := "project \"invalid-project\" is invalid or inaccessible"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("expected error to contain %q, but got: %v", expectedErrorMsg, err)
	}
}

func TestEnsureBasicPrerequisites_SaveState(t *testing.T) {
	useFileStore(t)
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		return shell.CommandResult{ExitCode: 0, Stdout: "user@example.com"}
	}

	origGetADCSetupCommand := getADCSetupCommandFunc
	defer func() { getADCSetupCommandFunc = origGetADCSetupCommand }()
	getADCSetupCommandFunc = func() string { return "" }

	cmd := &cobra.Command{}
	projectID := "test-project"

	err := ensureBasicPrerequisites(cmd, projectID)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	state := store.Load()
	if !state.GCloudSDKInstalled {
		t.Error("expected GCloudSDKInstalled to be true")
	}
	if !state.GCloudProjectConfigured {
		t.Error("expected GCloudProjectConfigured to be true")
	}
	if !state.GCloudAuthenticated {
		t.Error("expected GCloudAuthenticated to be true")
	}
	if !state.ADCConfigured {
		t.Error("expected ADCConfigured to be true")
	}
	if !state.KubectlInstalled {
		t.Error("expected KubectlInstalled to be true")
	}
	if !state.GKEGCloudAuthPluginInstalled {
		t.Error("expected GKEGCloudAuthPluginInstalled to be true")
	}
	if state.LastCheckedProjectID != "test-project" {
		t.Errorf("expected LastCheckedProjectID to be 'test-project', got: %s", state.LastCheckedProjectID)
	}
	if time.Since(state.LastCheckedTimestamp) > 5*time.Second {
		t.Errorf("expected LastCheckedTimestamp to be recent, got: %v", state.LastCheckedTimestamp)
	}
}

func TestEnsureBasicPrerequisites_RestrictedPermissions_Proceeds(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		cmdStr := name + " " + strings.Join(args, " ")
		switch {
		case strings.HasPrefix(cmdStr, "gcloud auth list"):
			return shell.CommandResult{ExitCode: 0, Stdout: "user@example.com"}
		case cmdStr == "gcloud projects describe restricted-project":
			return shell.CommandResult{ExitCode: 1, Stderr: "ERROR: (gcloud.projects.describe) PERMISSION_DENIED: The caller does not have permission"}
		default:
			return shell.CommandResult{ExitCode: 0}
		}
	}

	origStore := store
	defer func() { store = origStore }()
	store = &mockPrereqStore{}

	origGetADCSetupCommand := getADCSetupCommandFunc
	defer func() { getADCSetupCommandFunc = origGetADCSetupCommand }()
	getADCSetupCommandFunc = func() string { return "" }

	cmd := &cobra.Command{}
	projectID := "restricted-project"

	err := ensureBasicPrerequisites(cmd, projectID)
	if err != nil {
		t.Fatalf("expected nil error when user lacks resourcemanager permission, got: %v", err)
	}
}

func TestEnsureBasicPrerequisites_ProjectNameContainsPermission_Fails(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	origExecuteCommand := shell.ExecuteCommandWithTimeout
	defer func() { shell.ExecuteCommandWithTimeout = origExecuteCommand }()

	shell.ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) shell.CommandResult {
		cmdStr := name + " " + strings.Join(args, " ")
		switch {
		case strings.HasPrefix(cmdStr, "gcloud auth list"):
			return shell.CommandResult{ExitCode: 0, Stdout: "user@example.com"}
		case cmdStr == "gcloud projects describe my-permission-test":
			return shell.CommandResult{ExitCode: 1, Stderr: "ERROR: (gcloud.projects.describe) NOT_FOUND: Project 'my-permission-test' was not found"}
		default:
			return shell.CommandResult{ExitCode: 0}
		}
	}

	origStore := store
	defer func() { store = origStore }()
	store = &mockPrereqStore{}

	origGetADCSetupCommand := getADCSetupCommandFunc
	defer func() { getADCSetupCommandFunc = origGetADCSetupCommand }()
	getADCSetupCommandFunc = func() string { return "" }

	cmd := &cobra.Command{}
	projectID := "my-permission-test"

	err := ensureBasicPrerequisites(cmd, projectID)
	// It should hard fail because it is a NOT_FOUND error, NOT a true IAM permission error.
	if err == nil {
		t.Fatal("expected error because project does not exist, but got nil")
	}

	if !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Errorf("expected error to contain NOT_FOUND, got: %v", err)
	}
}
