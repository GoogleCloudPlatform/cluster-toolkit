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
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2/google"
)

type PrereqStore interface {
	Load() PrereqState
	Save(PrereqState)
}

type FilePrereqStore struct{}

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

// isStateStale checks if the loaded state is older than the defined freshness threshold
// or if the project ID has changed.
func isStateStale(state PrereqState, currentProjectID string) bool {
	if time.Since(state.LastCheckedTimestamp) > stateFreshness {
		return true
	}
	if state.LastCheckedProjectID != currentProjectID {
		return true
	}
	return false
}

// ensureGCloudSDKInstalled checks if gcloud SDK is installed and available in PATH.
func ensureGCloudSDKInstalled() error {
	result := shell.ExecuteCommand("gcloud", "version")
	if result.ExitCode != 0 {
		return fmt.Errorf("Google Cloud SDK (gcloud) is required to run prerequisite checks. Aborting job submission.\nPlease install it from https://cloud.google.com/sdk/docs/install and ensure it's in your PATH.\nAfter installation, please run 'gcloud auth login' to authenticate.\nError: %s", result.Stderr)
	}
	return nil
}

// ensureGCloudAuthenticated checks if gcloud is authenticated.
func ensureGCloudAuthenticated() error {
	result := shell.ExecuteCommand("gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
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

// isGCloudComponentManagerEnabled checks if component manager is enabled for gcloud.
func isGCloudComponentManagerEnabled() bool {
	result := shell.ExecuteCommand("gcloud", "components", "list", "--quiet")
	return !strings.Contains(result.Stderr, "component manager is disabled")
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

func checkK8sDependencies(state *PrereqState, missing *[]missingPrereq) {
	// Check kubectl
	if shell.ExecuteCommand("kubectl", "version", "--client", "--output=json").ExitCode != 0 {
		var cmds []string
		if isGCloudComponentManagerEnabled() {
			cmds = []string{"gcloud components install kubectl --quiet"}
		} else {
			cmds = []string{"# Please install kubectl manually for your operating system."}
		}
		*missing = append(*missing, missingPrereq{name: "kubectl", commands: cmds})
	} else {
		state.KubectlInstalled = true
	}

	// Check plugin
	if shell.ExecuteCommand("gke-gcloud-auth-plugin", "--version").ExitCode != 0 {
		var cmds []string
		if isGCloudComponentManagerEnabled() {
			cmds = []string{"gcloud components install gke-gcloud-auth-plugin --quiet"}
		} else {
			cmds = []string{"# Please install gke-gcloud-auth-plugin manually for your operating system."}
		}
		*missing = append(*missing, missingPrereq{name: "gke-gcloud-auth-plugin", commands: cmds})
	} else {
		state.GKEGCloudAuthPluginInstalled = true
	}
}

// isDockerCredsConfigured checks if Docker is configured to use gcloud credentials for the required registries.
func isDockerCredsConfigured(region string) bool {
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		configDir = filepath.Join(homeDir, ".docker")
	}
	configPath := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var config struct {
		CredHelpers map[string]string `json:"credHelpers"`
		CredsStore  string            `json:"credsStore"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		logging.Error("Failed to unmarshal Docker config from %s: %v", configPath, err)
		return false
	}

	if config.CredsStore == "gcloud" {
		return true
	}

	if config.CredHelpers["gcr.io"] != "gcloud" {
		return false
	}
	pkgDevReg := fmt.Sprintf("%s-docker.pkg.dev", region)
	return config.CredHelpers[pkgDevReg] == "gcloud"
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
	result := shell.ExecuteCommand("gcloud", "projects", "describe", projectID)
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

// ensureBasicPrerequisites checks for gcloud, auth, project existence, and kubectl.
func ensureBasicPrerequisites(cmd *cobra.Command, projectID string) error {
	if dryRunManifest != "" {
		return nil
	}

	state := store.Load()

	if !isStateStale(state, projectID) {
		logging.Info("Skipping basic checks; prerequisites are fresh (project: %s, checked: %v ago).", state.LastCheckedProjectID, time.Since(state.LastCheckedTimestamp).Round(time.Second))
		return nil
	}

	state = PrereqState{}

	var missing []missingPrereq

	// Hard dependency: gcloud must be installed
	if err := ensureGCloudSDKInstalled(); err != nil {
		return err
	}

	// Check GCloud Auth
	gcloudAuthOK := false
	if err := ensureGCloudAuthenticated(); err != nil {
		missing = append(missing, missingPrereq{name: "Google Cloud Authentication", commands: []string{"gcloud auth login"}})
	} else {
		gcloudAuthOK = true
	}

	// Check ADC
	adcCmd := getADCSetupCommandFunc()
	if adcCmd != "" {
		missing = append(missing, missingPrereq{name: "Application Default Credentials (ADC)", commands: []string{adcCmd}})
	}

	checkK8sDependencies(&state, &missing)

	// Run project validation if auth is OK, regardless of other missing checks
	var projectErr error
	if gcloudAuthOK && projectID != "" {
		projectErr = ensureProjectExists(projectID)
	}

	// Now decide what to return
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

	// All basic checks passed! Save state.
	state.GCloudSDKInstalled = true
	state.GCloudProjectConfigured = true
	state.GCloudAuthenticated = true
	state.ADCConfigured = (adcCmd == "")
	// state.KubectlInstalled and state.GKEGCloudAuthPluginInstalled are already set inside checkK8sDependencies

	state.LastCheckedTimestamp = time.Now()
	state.LastCheckedProjectID = projectID
	store.Save(state)

	return nil
}

// hasPassedBasicPrerequisites checks if all basic prerequisite checks are fresh and marked as passed.
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

func checkArtifactRegistryAPI(projectID string, state *PrereqState, missing *[]missingPrereq) {
	if projectID == "" {
		return
	}
	apiResult := shell.ExecuteCommand("gcloud", "services", "list", "--filter=NAME:artifactregistry.googleapis.com", "--format=value(STATE)", "--project", projectID)
	if strings.TrimSpace(apiResult.Stdout) != "ENABLED" {
		*missing = append(*missing, missingPrereq{
			name:     "Artifact Registry API",
			commands: []string{fmt.Sprintf("gcloud services enable artifactregistry.googleapis.com --project %s --quiet", projectID)},
		})
	} else {
		state.ArtifactRegistryAPIEnabled = true
	}
}

func checkDockerCredentials(location string, state *PrereqState, missing *[]missingPrereq) {
	region := shell.ExtractRegion(location)
	if !isDockerCredsConfigured(region) {
		cmds := []string{"gcloud auth configure-docker gcr.io --quiet"}
		if region != "" {
			cmds = append(cmds, fmt.Sprintf("gcloud auth configure-docker %s-docker.pkg.dev --quiet", region))
		}
		*missing = append(*missing, missingPrereq{
			name:     "Docker Credentials",
			commands: cmds,
		})
	} else {
		state.DockerCredsConfigured = true
	}
}

// EnsurePrerequisites checks all necessary gcloud and kubectl prerequisites.
func ensurePrerequisites(cmd *cobra.Command, projectID string, location string) error {
	if dryRunManifest != "" {
		return nil
	}

	state := store.Load()

	if !isStateStale(state, projectID) && state.DockerCredsConfigured && state.ArtifactRegistryAPIEnabled {
		logging.Info("Skipping checks; prerequisites are fresh (project: %s, checked: %v ago).", state.LastCheckedProjectID, time.Since(state.LastCheckedTimestamp).Round(time.Second))
		return nil
	}

	// Safety check: if basic checks haven't run or are not recorded as passed, run them now.
	if !hasPassedBasicPrerequisites(state, projectID) {
		if err := ensureBasicPrerequisites(cmd, projectID); err != nil {
			return err
		}
		state = store.Load() // Reload the state updated by ensureBasicPrerequisites
	}

	var missing []missingPrereq

	checkArtifactRegistryAPI(projectID, &state, &missing)
	checkDockerCredentials(location, &state, &missing)

	if len(missing) > 0 {
		printMissingPrereqs(cmd, missing)
		return fmt.Errorf("job could not be submitted because some prerequisites are missing")
	}

	state.LastCheckedTimestamp = time.Now()
	state.LastCheckedProjectID = projectID
	store.Save(state)

	logging.Info("Prerequisites checked successfully.")
	return nil
}
