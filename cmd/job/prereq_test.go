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
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

	err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, "test-project")
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
	if missing[0].name != "kubectl" || len(missing[0].commands) != 2 {
		t.Fatalf("missing[0] = %+v, want kubectl with 2 install commands", missing[0])
	}
	wantKubectlCmd0 := "gcloud components install kubectl --quiet   # gcloud from Google's installer"
	wantKubectlCmd1 := "# or install kubectl with your OS package manager (apt/dnf/brew on Linux/macOS, winget/choco on Windows) if gcloud came from there"
	if missing[0].commands[0] != wantKubectlCmd0 || missing[0].commands[1] != wantKubectlCmd1 {
		t.Errorf("kubectl commands = %q, want [%q, %q]", missing[0].commands, wantKubectlCmd0, wantKubectlCmd1)
	}

	if missing[1].name != "gke-gcloud-auth-plugin" || len(missing[1].commands) != 2 {
		t.Fatalf("missing[1] = %+v, want gke-gcloud-auth-plugin with 2 install commands", missing[1])
	}
	if !strings.Contains(missing[1].commands[0], "gcloud components install gke-gcloud-auth-plugin --quiet") {
		t.Errorf("plugin commands[0] = %q, want gcloud components install hint", missing[1].commands[0])
	}
	if !strings.Contains(missing[1].commands[1], "google-cloud-cli-gke-gcloud-auth-plugin") || strings.Contains(missing[1].commands[1], "winget") {
		t.Errorf("plugin commands[1] = %q, want apt/dnf google-cloud-cli-gke-gcloud-auth-plugin hint without winget/choco", missing[1].commands[1])
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

	err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, projectID)
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

	err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, projectID)
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

	err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, projectID)
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

	err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, projectID)
	// It should hard fail because it is a NOT_FOUND error, NOT a true IAM permission error.
	if err == nil {
		t.Fatal("expected error because project does not exist, but got nil")
	}

	if !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Errorf("expected error to contain NOT_FOUND, got: %v", err)
	}
}

func TestEnsureBasicPrerequisitesFor_K8sCacheHitAndGKEFullCheck(t *testing.T) {
	executed := stubPrereqCommands(t)
	adcCalls := 0
	getADCSetupCommandFunc = func() string {
		adcCalls++
		return ""
	}
	cmd := &cobra.Command{}

	if err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeK8s, "shared-proj"); err != nil {
		t.Fatalf("ensureBasicPrerequisitesFor(TypeK8s) err = %v, want nil", err)
	}
	if len(*executed) != 1 || !strings.HasPrefix((*executed)[0], "kubectl version") || adcCalls != 0 {
		t.Fatalf("TypeK8s executed = %v (adcCalls=%d), want only kubectl version and 0 ADC checks", *executed, adcCalls)
	}

	// Second TypeK8s run within 24h must hit the cache with zero subprocesses.
	*executed = nil
	if err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeK8s, "shared-proj"); err != nil {
		t.Fatalf("second ensureBasicPrerequisitesFor(TypeK8s) err = %v, want nil", err)
	}
	if len(*executed) != 0 || adcCalls != 0 {
		t.Fatalf("second TypeK8s executed = %v (adcCalls=%d), want 0 subprocesses", *executed, adcCalls)
	}

	// Subsequent TypeGKE run on the same project must still verify gcloud, auth, ADC, plugin, and project.
	*executed = nil
	if err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, "shared-proj"); err != nil {
		t.Fatalf("ensureBasicPrerequisitesFor(TypeGKE) err = %v, want nil", err)
	}
	wantGKECmds := []string{
		"gcloud version",
		"gcloud auth list --filter=status:ACTIVE --format=value(account)",
		"gke-gcloud-auth-plugin --version",
		"gcloud projects describe shared-proj",
	}
	if !slices.Equal(*executed, wantGKECmds) || adcCalls != 1 {
		t.Fatalf("TypeGKE executed = %v (adcCalls=%d), want %v and adcCalls=1", *executed, adcCalls, wantGKECmds)
	}
}

func TestEnsureBasicPrerequisitesFor_PreservesUnexpiredAndARProjectState(t *testing.T) {
	executed := stubPrereqCommands(t)
	adcCalls := 0
	getADCSetupCommandFunc = func() string {
		adcCalls++
		return ""
	}
	cmd := &cobra.Command{}
	tenHoursAgo := time.Now().Add(-10 * time.Hour).Round(time.Second)
	store.Save(PrereqState{
		GCloudSDKInstalled:           true,
		GCloudProjectConfigured:      true,
		GCloudAuthenticated:          true,
		ADCConfigured:                true,
		GKEGCloudAuthPluginInstalled: true,
		ArtifactRegistryAPIEnabled:   true,
		LastCheckedARProjectID:       "ar-proj-1",
		LastCheckedProjectID:         "gke-proj-a",
		LastCheckedTimestamp:         tenHoursAgo,
	})

	if err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeK8s, ""); err != nil {
		t.Fatalf("ensureBasicPrerequisitesFor(TypeK8s) err = %v, want nil", err)
	}
	if len(*executed) != 1 || !strings.HasPrefix((*executed)[0], "kubectl version") || adcCalls != 0 {
		t.Fatalf("TypeK8s executed = %v (adcCalls=%d), want only kubectl version", *executed, adcCalls)
	}
	saved := store.Load()
	if !saved.LastCheckedTimestamp.Equal(tenHoursAgo) {
		t.Fatalf("TypeK8s LastCheckedTimestamp = %v, want %v", saved.LastCheckedTimestamp, tenHoursAgo)
	}
	saved.LastCheckedTimestamp = time.Time{}
	wantAfterK8s := PrereqState{
		GCloudSDKInstalled:           true,
		GCloudProjectConfigured:      true,
		GCloudAuthenticated:          true,
		ADCConfigured:                true,
		KubectlInstalled:             true,
		GKEGCloudAuthPluginInstalled: true,
		ArtifactRegistryAPIEnabled:   true,
		LastCheckedARProjectID:       "ar-proj-1",
		LastCheckedProjectID:         "gke-proj-a",
	}
	if saved != wantAfterK8s {
		t.Fatalf("TypeK8s state = %+v, want %+v", saved, wantAfterK8s)
	}

	assertUnexpiredCrossProjectGKEPrereqs(t, cmd, executed, &adcCalls, tenHoursAgo)
}

func assertUnexpiredCrossProjectGKEPrereqs(t *testing.T, cmd *cobra.Command, executed *[]string, adcCalls *int, tenHoursAgo time.Time) {
	t.Helper()
	*executed = nil
	if err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, "gke-proj-b"); err != nil {
		t.Fatalf("ensureBasicPrerequisitesFor(TypeGKE, gke-proj-b) err = %v, want nil", err)
	}
	if !slices.Equal(*executed, []string{"gcloud projects describe gke-proj-b"}) || *adcCalls != 0 {
		t.Fatalf("cross-project GKE executed = %v (adcCalls=%d), want only [gcloud projects describe gke-proj-b] and 0 ADC calls", *executed, *adcCalls)
	}
	saved := store.Load()
	if !saved.ArtifactRegistryAPIEnabled || saved.LastCheckedARProjectID != "ar-proj-1" || saved.LastCheckedProjectID != "gke-proj-b" || !saved.LastCheckedTimestamp.Equal(tenHoursAgo) {
		t.Fatalf("cross-project GKE state = %+v, want LastCheckedARProjectID=\"ar-proj-1\" and timestamp=%v preserved", saved, tenHoursAgo)
	}
}

func TestEnsureBasicPrerequisitesFor_ExpiredGKEFlagsNotResurrectedByK8sRefresh(t *testing.T) {
	executed := stubPrereqCommands(t)
	adcCalls := 0
	getADCSetupCommandFunc = func() string {
		adcCalls++
		return ""
	}
	cmd := &cobra.Command{}
	store.Save(PrereqState{
		GCloudSDKInstalled:           true,
		GCloudProjectConfigured:      true,
		GCloudAuthenticated:          true,
		ADCConfigured:                true,
		KubectlInstalled:             true,
		GKEGCloudAuthPluginInstalled: true,
		ArtifactRegistryAPIEnabled:   true,
		LastCheckedARProjectID:       "ar-proj-old",
		LastCheckedProjectID:         "shared-proj",
		LastCheckedTimestamp:         time.Now().Add(-48 * time.Hour),
	})

	if err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeK8s, ""); err != nil {
		t.Fatalf("ensureBasicPrerequisitesFor(TypeK8s) after expiry err = %v, want nil", err)
	}
	savedAfterK8s := store.Load()
	cleared := savedAfterK8s
	cleared.LastCheckedTimestamp = time.Time{}
	if cleared != (PrereqState{KubectlInstalled: true}) || isTimestampExpired(savedAfterK8s) {
		t.Fatalf("saved state after TypeK8s refresh = %+v, want only KubectlInstalled=true with fresh timestamp", savedAfterK8s)
	}

	*executed = nil
	if err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeGKE, "shared-proj"); err != nil {
		t.Fatalf("ensureBasicPrerequisitesFor(TypeGKE) after K8s refresh err = %v, want nil", err)
	}
	wantGKECmds := []string{
		"gcloud version",
		"gcloud auth list --filter=status:ACTIVE --format=value(account)",
		"gke-gcloud-auth-plugin --version",
		"gcloud projects describe shared-proj",
	}
	if !slices.Equal(*executed, wantGKECmds) || adcCalls != 1 {
		t.Fatalf("TypeGKE executed = %v (adcCalls=%d), want %v and adcCalls=1", *executed, adcCalls, wantGKECmds)
	}
}

func TestEnsureK8sOnlyBasicPrerequisites_MissingKubectl(t *testing.T) {
	stubPrereqCommands(t)
	origExec := shell.ExecuteCommandWithTimeout
	t.Cleanup(func() { shell.ExecuteCommandWithTimeout = origExec })
	shell.ExecuteCommandWithTimeout = func(_ time.Duration, name string, args ...string) shell.CommandResult {
		return shell.CommandResult{ExitCode: 127, Stderr: "kubectl: command not found"}
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	err := ensureBasicPrerequisitesFor(cmd, orchestrator.TypeK8s, "")
	if err == nil || !strings.Contains(err.Error(), "prerequisites are missing") {
		t.Fatalf("ensureBasicPrerequisitesFor(cmd, %q, \"\") err = %v, want missing prerequisites error", orchestrator.TypeK8s, err)
	}
	if !strings.Contains(out.String(), "kubectl") || strings.Contains(out.String(), "gcloud") {
		t.Errorf("TypeK8s missing kubectl remediation = %q, want kubectl hint without gcloud", out.String())
	}
}
