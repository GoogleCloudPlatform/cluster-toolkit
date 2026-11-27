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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2/google"
)

// Load reads the persisted prerequisite state from disk, returning a zero value on missing or corrupt files.
func (f *FilePrereqStore) Load() PrereqState {
	filePath, err := stateFilePath()
	if err != nil {
		logging.Error("Failed to get state file path for loading: %v", err)
		return PrereqState{}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Error("Failed to read prerequisite state from %s: %v", filePath, err)
		}
		return PrereqState{}
	}

	var state PrereqState
	if err := json.Unmarshal(data, &state); err != nil {
		logging.Error("Failed to unmarshal prerequisite state from %s: %v. Starting with fresh state.", filePath, err)
		return PrereqState{}
	}
	return state
}

// Save writes state as formatted JSON to the user's prerequisite state file.
func (f *FilePrereqStore) Save(state PrereqState) {
	filePath, err := stateFilePath()
	if err != nil {
		logging.Error("Failed to get state file path for saving: %v", err)
		return
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logging.Error("Failed to marshal prerequisite state: %v", err)
		return
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		logging.Error("Failed to write prerequisite state to %s: %v", filePath, err)
	}
}

var store PrereqStore = &FilePrereqStore{}

// stateFilePath returns the full path to the prerequisite state file.
func stateFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", fmt.Errorf("could not create state directory %s: %w", stateDir, err)
	}
	return filepath.Join(stateDir, stateFileName), nil
}

func isTimestampExpired(state PrereqState) bool {
	return state.LastCheckedTimestamp.IsZero() || time.Since(state.LastCheckedTimestamp) > stateFreshness
}

// isStateStale checks if the loaded state is older than the defined freshness threshold
// or if the project ID has changed.
func isStateStale(state PrereqState, currentProjectID string) bool {
	return isTimestampExpired(state) || state.LastCheckedProjectID != currentProjectID
}

func expireStalePrereqState(state PrereqState) PrereqState {
	if !isTimestampExpired(state) {
		return state
	}
	return PrereqState{}
}

func prepareBasicPrereqState(state PrereqState, projectID string) PrereqState {
	state = expireStalePrereqState(state)
	// Preserve legacy single-project AR cache ownership before switching cluster projects.
	if state.ArtifactRegistryAPIEnabled && state.LastCheckedARProjectID == "" && state.LastCheckedProjectID != "" {
		state.LastCheckedARProjectID = state.LastCheckedProjectID
	}
	if state.LastCheckedProjectID != projectID {
		state.GCloudProjectConfigured = false
		state.LastCheckedProjectID = ""
	}
	return state
}

const gcloudRequiredMsg = "Google Cloud SDK (gcloud) is required to run prerequisite checks. Aborting job submission.\nPlease install it from https://cloud.google.com/sdk/docs/install and ensure it's in your PATH.\nAfter installation, please run 'gcloud auth login' to authenticate."

// ensureGCloudSDKInstalled checks if gcloud SDK is installed and available in PATH.
func ensureGCloudSDKInstalled() error {

	result := shell.ExecuteCommandWithTimeout(shell.DefaultLocalCommandTimeout, "gcloud", "version")

	timeoutMsg := fmt.Sprintf("gcloud version check timed out after %v. Please verify your local gcloud installation is responsive", shell.DefaultLocalCommandTimeout)
	if err := shell.HandleExecError(result, "gcloud", timeoutMsg); err != nil {
		// only suggest installation if the binary failed to execute (missing from PATH / %PATH%)
		if errors.Is(result.Err, exec.ErrNotFound) {
			return fmt.Errorf("%s\nError: %w", gcloudRequiredMsg, err)
		}
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s\nError: %s", gcloudRequiredMsg, result.Stderr)
	}
	return nil
}

// ensureGCloudAuthenticated checks if gcloud is authenticated.
func ensureGCloudAuthenticated() error {

	result := shell.ExecuteCommandWithTimeout(shell.DefaultLocalCommandTimeout, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")

	if err := shell.HandleExecError(result, "gcloud", fmt.Sprintf("gcloud authentication check timed out after %v. Please verify your local gcloud installation is responsive", shell.DefaultLocalCommandTimeout)); err != nil {
		return err
	}

	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
		return fmt.Errorf("gcloud is not authenticated")
	}
	return nil
}

var getADCSetupCommandFunc = getADCSetupCommand

// getADCSetupCommand checks if Application Default Credentials are valid and returns the setup command if not.
func getADCSetupCommand() string {
	creds, err := google.FindDefaultCredentials(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "gcloud auth application-default login"
	}

	// Force token retrieval to verify validity
	_, err = creds.TokenSource.Token()
	if err != nil {
		return "gcloud auth application-default login"
	}

	return ""
}

func ensureGCloudAuthAndADC(state *PrereqState, missing *[]missingPrereq) (bool, error) {
	if !state.GCloudSDKInstalled {
		if err := ensureGCloudSDKInstalled(); err != nil {
			return false, err
		}
		state.GCloudSDKInstalled = true
	}
	checkedAuth := false
	if !state.GCloudAuthenticated {
		checkedAuth = true
		if err := ensureGCloudAuthenticated(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return false, err
			}
			*missing = append(*missing, missingPrereq{name: "Google Cloud Authentication", commands: []string{"gcloud auth login"}})
		} else {
			state.GCloudAuthenticated = true
		}
	}
	if !state.ADCConfigured {
		checkedAuth = true
		if adcCmd := getADCSetupCommandFunc(); adcCmd != "" {
			*missing = append(*missing, missingPrereq{name: "Application Default Credentials (ADC)", commands: []string{adcCmd}})
		} else {
			state.ADCConfigured = true
		}
	}
	return checkedAuth, nil
}

func printMissingPrereqs(cmd *cobra.Command, missing []missingPrereq) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nSome required prerequisites are missing. Please install the dependencies or configure the credentials listed below to proceed:")
	for _, m := range missing {
		fmt.Fprintf(cmd.OutOrStdout(), "\n - %s\n", m.name)
		if len(m.commands) == 1 {
			fmt.Fprintf(cmd.OutOrStdout(), "   Command: %s\n", m.commands[0])
		} else if len(m.commands) > 1 {
			fmt.Fprintln(cmd.OutOrStdout(), "   Commands:")
			for _, c := range m.commands {
				fmt.Fprintf(cmd.OutOrStdout(), "     %s\n", c)
			}
		}
	}
	fmt.Fprintln(cmd.OutOrStdout())
}

func checkLocalCLI(name string, checkArgs []string, installHints []string, installedFlag *bool, missing *[]missingPrereq) {
	res := shell.ExecuteCommandWithTimeout(shell.DefaultLocalCommandTimeout, name, checkArgs...)
	if errors.Is(res.Err, context.DeadlineExceeded) {
		logging.Warn("Command timeout while checking %s. Please verify the binary is responsive.", name)
		*missing = append(*missing, missingPrereq{
			name:     fmt.Sprintf("%s (installed, but did not respond within %v)", name, shell.DefaultLocalCommandTimeout),
			commands: []string{fmt.Sprintf("# Check that '%s' completes on its own, then retry.", strings.Join(append([]string{name}, checkArgs...), " "))},
		})
		return
	}
	if res.ExitCode != 0 {
		if len(installHints) == 0 {
			installHints = []string{
				fmt.Sprintf("gcloud components install %s --quiet   # gcloud from Google's installer", name),
				fmt.Sprintf("# or install %s with your OS package manager (apt/dnf/brew on Linux/macOS, winget/choco on Windows) if gcloud came from there", name),
			}
		}
		*missing = append(*missing, missingPrereq{name: name, commands: installHints})
		return
	}
	*installedFlag = true
}

func checkK8sDependencies(state *PrereqState, missing *[]missingPrereq) {
	if !state.KubectlInstalled {
		checkLocalCLI("kubectl", []string{"version", "--client", "--output=json"}, nil, &state.KubectlInstalled, missing)
	}
	if !state.GKEGCloudAuthPluginInstalled {
		pluginHints := []string{
			"gcloud components install gke-gcloud-auth-plugin --quiet   # gcloud from Google's installer, Homebrew, or Windows SDK",
			"# or install google-cloud-cli-gke-gcloud-auth-plugin with apt/dnf on Linux if gcloud came from a system package",
		}
		checkLocalCLI("gke-gcloud-auth-plugin", []string{"--version"}, pluginHints, &state.GKEGCloudAuthPluginInstalled, missing)
	}
}

// isPermissionDeniedError checks if the error indicates a 403 / permission denied response.
func isPermissionDeniedError(stderr string, projectID string) bool {
	safeStderr := stderr
	if projectID != "" {
		safeStderr = strings.ReplaceAll(stderr, projectID, "")
	}
	lower := strings.ToLower(safeStderr)
	return strings.Contains(lower, "permission_denied") ||
		strings.Contains(lower, "permission") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "not authorized") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "403")
}

// ensureProjectExists checks if the project exists and is accessible.
func ensureProjectExists(projectID string) error {

	result := shell.ExecuteCommandWithTimeout(shell.DefaultCloudAPITimeout, "gcloud", "projects", "describe", projectID)
	if err := shell.HandleExecError(result, "gcloud", fmt.Sprintf("gcloud project validation timed out after %v. Please check your network connection", shell.DefaultCloudAPITimeout)); err != nil {
		return err
	}

	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		if isPermissionDeniedError(stderr, projectID) {
			logging.Warn("Could not verify project %q existence due to restricted IAM permissions; proceeding anyway.", projectID)
			return nil
		}
		return fmt.Errorf("failed to validate project: %s", stderr)
	}
	return nil
}

func reportBasicPrereqFailure(cmd *cobra.Command, projectID string, projectErr error, missing []missingPrereq) error {
	if projectErr != nil {
		if len(missing) > 0 {
			printMissingPrereqs(cmd, missing)
		}
		return fmt.Errorf("project %q is invalid or inaccessible: %w", projectID, projectErr)
	}
	if len(missing) > 0 {
		printMissingPrereqs(cmd, missing)
		return fmt.Errorf("some basic prerequisites are missing")
	}
	return nil
}

// ensureBasicPrerequisitesFor runs only the basic prerequisites declared by orchType.
func ensureBasicPrerequisitesFor(cmd *cobra.Command, orchType orchestrator.Type, projectID string) error {
	if dryRunManifest != "" {
		return nil
	}
	kinds := orchestrator.BasicPrerequisites(orchType)
	switch {
	case slices.Contains(kinds, orchestrator.PrereqGKE):
		return ensureGKEBasicPrerequisites(cmd, projectID)
	case slices.Contains(kinds, orchestrator.PrereqKubectl):
		return ensureK8sOnlyBasicPrerequisites(cmd)
	default:
		return nil
	}
}

func ensureGKEBasicPrerequisites(cmd *cobra.Command, projectID string) error {
	state := store.Load()
	if hasPassedBasicPrerequisites(state, projectID) {
		logging.Info("Skipping basic checks; prerequisites are fresh (project: %s, checked: %v ago).", state.LastCheckedProjectID, time.Since(state.LastCheckedTimestamp).Round(time.Second))
		return nil
	}

	state = prepareBasicPrereqState(state, projectID)

	var missing []missingPrereq
	checkedAuth, err := ensureGCloudAuthAndADC(&state, &missing)
	if err != nil {
		return err
	}

	checkK8sDependencies(&state, &missing)

	var projectErr error
	if state.GCloudAuthenticated && !state.GCloudProjectConfigured && projectID != "" {
		projectErr = ensureProjectExists(projectID)
	}

	if failErr := reportBasicPrereqFailure(cmd, projectID, projectErr, missing); failErr != nil {
		return failErr
	}

	state.GCloudProjectConfigured = true
	state.LastCheckedProjectID = projectID
	if checkedAuth || state.LastCheckedTimestamp.IsZero() {
		state.LastCheckedTimestamp = time.Now()
	}
	store.Save(state)
	return nil
}

func ensureK8sOnlyBasicPrerequisites(cmd *cobra.Command) error {
	state := store.Load()
	if !isTimestampExpired(state) && state.KubectlInstalled {
		logging.Info("Skipping basic checks; cached kubectl prerequisite is fresh.")
		return nil
	}
	state = expireStalePrereqState(state)

	var missing []missingPrereq
	kubectlHints := []string{"# Install kubectl with your OS package manager or offline bundle and ensure it is in your PATH"}
	checkLocalCLI("kubectl", []string{"version", "--client", "--output=json"}, kubectlHints, &state.KubectlInstalled, &missing)
	if len(missing) > 0 {
		printMissingPrereqs(cmd, missing)
		return fmt.Errorf("some basic prerequisites are missing")
	}

	if state.LastCheckedTimestamp.IsZero() {
		state.LastCheckedTimestamp = time.Now()
	}
	store.Save(state)
	return nil
}

// hasPassedBasicPrerequisites checks if all GKE basic prerequisite checks are fresh and marked as passed.
func hasPassedBasicPrerequisites(state PrereqState, projectID string) bool {
	if isStateStale(state, projectID) {
		return false
	}
	return state.GCloudSDKInstalled &&
		state.GCloudProjectConfigured &&
		state.GCloudAuthenticated &&
		state.ADCConfigured &&
		state.KubectlInstalled &&
		state.GKEGCloudAuthPluginInstalled
}
