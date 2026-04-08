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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

const (
	stateFileName  = "job_prereq_state.json"
	stateDirName   = ".gcluster"
	stateFreshness = 24 * time.Hour // State is considered fresh for 24 hours
)

// PrereqState holds the current state of prerequisite checks.
type PrereqState struct {
	GCloudSDKInstalled           bool      `json:"gcloud_sdk_installed"`
	GCloudProjectConfigured      bool      `json:"gcloud_project_configured"`
	GCloudAuthenticated          bool      `json:"gcloud_authenticated"`
	ADCConfigured                bool      `json:"adc_configured"`
	KubectlInstalled             bool      `json:"kubectl_installed"`
	GKEGCloudAuthPluginInstalled bool      `json:"gke_gcloud_auth_plugin_installed"`
	DockerCredsConfigured        bool      `json:"docker_creds_configured"`
	ArtifactRegistryAPIEnabled   bool      `json:"artifact_registry_api_enabled"`
	LastCheckedProjectID         string    `json:"last_checked_project_id"`
	LastCheckedTimestamp         time.Time `json:"last_checked_timestamp"`
}

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

// savePrereqState saves the current prerequisite state to a file.
func savePrereqState(state PrereqState) {
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
		return
	}
	logging.Info("Prerequisite state saved to %s", filePath)
}

// loadPrereqState loads the prerequisite state from a file.
func loadPrereqState() PrereqState {
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
		logging.Error("Google Cloud SDK not found or not configured correctly. Please install it from https://cloud.google.com/sdk/docs/install and ensure it's in your PATH.")
		return fmt.Errorf("gcloud SDK not found: %s", result.Stderr)
	}
	return nil
}

// ensureGCloudProjectConfigured checks and configures the default gcloud project.
func ensureGCloudProjectConfigured(projectID *string) error {
	if *projectID != "" {
		logging.Info("Using provided project ID: %s", *projectID)
		currentProjectResult := shell.ExecuteCommand("gcloud", "config", "get-value", "project")
		if currentProjectResult.ExitCode != 0 || strings.TrimSpace(currentProjectResult.Stdout) != *projectID {
			logging.Info("Setting gcloud default project to provided project ID: %s", *projectID)
			setResult := shell.ExecuteCommand("gcloud", "config", "set", "project", *projectID)
			if setResult.ExitCode != 0 {
				return fmt.Errorf("failed to set gcloud project to %s: %s", *projectID, setResult.Stderr)
			}
		}
		return nil
	}

	logging.Info("Google Cloud project ID not provided via --project flag.")
	result := shell.ExecuteCommand("gcloud", "config", "get-value", "project")
	configuredProject := strings.TrimSpace(result.Stdout)
	if result.ExitCode == 0 && configuredProject != "" {
		logging.Info("Using project ID from gcloud configuration: %s", configuredProject)
		*projectID = configuredProject
		return nil
	}

	logging.Info("No default gcloud project configured.")

	fmt.Print("Please enter your Google Cloud Project ID: ")
	reader := bufio.NewReader(os.Stdin)
	inputProjectID, _ := reader.ReadString('\n')
	inputProjectID = strings.TrimSpace(inputProjectID)

	if inputProjectID == "" {
		return fmt.Errorf("Google Cloud Project ID is required")
	}

	logging.Info("Setting gcloud default project to: %s", inputProjectID)
	setResult := shell.ExecuteCommand("gcloud", "config", "set", "project", inputProjectID)
	if setResult.ExitCode != 0 {
		return fmt.Errorf("failed to set gcloud project to %s: %s", inputProjectID, setResult.Stderr)
	}
	*projectID = inputProjectID
	logging.Info("Google Cloud project set to: %s", *projectID)

	return nil
}

// ensureGCloudAuthenticated checks if gcloud is authenticated.
func ensureGCloudAuthenticated() error {
	result := shell.ExecuteCommand("gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
		logging.Error("gcloud is not authenticated. Please run 'gcloud auth login' manually in your terminal to complete the authentication process.")
		return fmt.Errorf("gcloud user authentication required")
	}
	return nil
}

// ensureApplicationDefaultCredentials checks and configures Application Default Credentials.
func ensureApplicationDefaultCredentials() error {
	creds, err := google.FindDefaultCredentials(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		logging.Error("Application Default Credentials (ADC) are not configured. Please run 'gcloud auth application-default login' manually in your terminal to complete the authentication process.")
		return fmt.Errorf("Application Default Credentials required: %w", err)
	}

	// Force token retrieval to verify validity
	_, err = creds.TokenSource.Token()
	if err != nil {
		logging.Error("Failed to retrieve valid token from ADC. Your credentials may have expired. Please run 'gcloud auth application-default login' manually in your terminal.")
		return fmt.Errorf("ADC token invalid: %w", err)
	}

	return nil
}

// ensureKubectlInstalled checks and installs kubectl component.
func ensureKubectlInstalled(useAptFallback *bool) error {
	result := shell.ExecuteCommand("kubectl", "version", "--client", "--output=json")
	if result.ExitCode == 0 {
		return nil
	}

	if !*useAptFallback {
		logging.Info("kubectl not found. Attempting to install via gcloud components.")
		if !shell.PromptYesNo("Do you want to install 'kubectl' via 'gcloud components install kubectl'?") {
			return fmt.Errorf("kubectl installation required but declined by user")
		}

		installResult := shell.ExecuteCommand("gcloud", "components", "install", "kubectl", "--quiet")
		if installResult.ExitCode == 0 {
			logging.Info("kubectl installed successfully via gcloud components.")
			return nil
		}

		logging.Error("gcloud components install kubectl failed: %s", installResult.Stderr)

		if strings.Contains(installResult.Stderr, "sudo apt-get install kubectl") ||
			strings.Contains(installResult.Stderr, "component manager is disabled") {

			if !shell.PromptYesNo("gcloud components install kubectl failed. Do you want to try 'sudo apt-get install kubectl' as a fallback?") {
				return fmt.Errorf("kubectl installation required but declined apt-get fallback by user")
			}
			*useAptFallback = true
		} else {
			return fmt.Errorf("failed to install kubectl via gcloud components: %s", installResult.Stderr)
		}
	}

	if *useAptFallback {
		logging.Info("Attempting to install kubectl via apt-get.")
		aptInstallResult := shell.ExecuteCommand("sudo", "apt-get", "install", "kubectl", "--quiet")
		if aptInstallResult.ExitCode != 0 {
			return fmt.Errorf("failed to install kubectl via apt-get: %s", aptInstallResult.Stderr)
		}
		logging.Info("kubectl installed successfully via apt-get.")
		return nil
	}

	return fmt.Errorf("failed to install kubectl")
}

func ensureGKEGCloudAuthPluginInstalled(useAptFallback *bool) error {
	result := shell.ExecuteCommand("gke-gcloud-auth-plugin", "--version")
	if result.ExitCode == 0 {
		return nil
	}

	logging.Info("gke-gcloud-auth-plugin not found. Attempting to install.")

	if !*useAptFallback {
		if !shell.PromptYesNo("Do you want to install 'gke-gcloud-auth-plugin' via 'gcloud components install gke-gcloud-auth-plugin'?") {
			return fmt.Errorf("gke-gcloud-auth-plugin installation required but declined by user")
		}

		installResult := shell.ExecuteCommand("gcloud", "components", "install", "gke-gcloud-auth-plugin", "--quiet")
		if installResult.ExitCode == 0 {
			logging.Info("gke-gcloud-auth-plugin installed successfully via gcloud components.")
			return nil
		}

		logging.Error("gcloud components install gke-gcloud-auth-plugin failed: %s", installResult.Stderr)

		if strings.Contains(installResult.Stderr, "sudo apt-get install") ||
			strings.Contains(installResult.Stderr, "component manager is disabled") {

			if !shell.PromptYesNo("gcloud components install gke-gcloud-auth-plugin failed. Do you want to try 'sudo apt-get install google-cloud-sdk-gke-gcloud-auth-plugin' as a fallback?") {
				return fmt.Errorf("gke-gcloud-auth-plugin installation required but declined apt-get fallback by user")
			}
			*useAptFallback = true
		} else {
			return fmt.Errorf("failed to install gke-gcloud-auth-plugin via gcloud components: %s", installResult.Stderr)
		}
	}

	if *useAptFallback {
		logging.Info("Attempting to install gke-gcloud-auth-plugin via apt-get.")
		aptInstallResult := shell.ExecuteCommand("sudo", "apt-get", "install", "google-cloud-sdk-gke-gcloud-auth-plugin", "--quiet")
		if aptInstallResult.ExitCode != 0 {
			return fmt.Errorf("failed to install gke-gcloud-auth-plugin via apt-get: %s", aptInstallResult.Stderr)
		}
		logging.Info("gke-gcloud-auth-plugin installed successfully via apt-get.")
		return nil
	}

	return fmt.Errorf("failed to install gke-gcloud-auth-plugin")
}

// configureDockerCredentialHelper configures Docker to authenticate to Google Container Registry and Artifact Registry.
func configureDockerCredentialHelper() error {
	logging.Info("Configuring Docker credential helper for Google Container Registry and Artifact Registry...")
	gcrResult := shell.ExecuteCommand("gcloud", "auth", "configure-docker", "gcr.io", "--quiet")
	if gcrResult.ExitCode != 0 {
		return fmt.Errorf("failed to configure Docker for gcr.io: %s", gcrResult.Stderr)
	}
	usCentral1Result := shell.ExecuteCommand("gcloud", "auth", "configure-docker", "us-central1-docker.pkg.dev", "--quiet")
	if usCentral1Result.ExitCode != 0 {
		return fmt.Errorf("failed to configure Docker for us-central1-docker.pkg.dev: %s", usCentral1Result.Stderr)
	}
	return nil
}

// ensureArtifactRegistryAPIEnabled checks and enables the Artifact Registry API.
func ensureArtifactRegistryAPIEnabled(projectID string) error {

	result := shell.ExecuteCommand("gcloud", "services", "list", "--filter=NAME:artifactregistry.googleapis.com", "--format=value(STATE)", "--project", projectID)
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to check Artifact Registry API status: %s", result.Stderr)
	}

	state := strings.TrimSpace(result.Stdout)
	if state != "ENABLED" {
		logging.Info("Artifact Registry API is not enabled for project %s. Attempting to enable it.", projectID)
		if !shell.PromptYesNo(fmt.Sprintf("Do you want to enable 'artifactregistry.googleapis.com' for project %s?", projectID)) {
			return fmt.Errorf("Artifact Registry API enabling required but declined by user")
		}
		enableResult := shell.ExecuteCommand("gcloud", "services", "enable", "artifactregistry.googleapis.com", "--project", projectID, "--quiet")
		if enableResult.ExitCode != 0 {
			return fmt.Errorf("failed to enable Artifact Registry API for project %s: %s", projectID, enableResult.Stderr)
		}
		logging.Info("Artifact Registry API enabled successfully.")
	} else {
		logging.Info("Artifact Registry API is already enabled.")
	}
	return nil
}

func checkGCloudProject(newState *PrereqState, projectID *string) error {
	if !newState.GCloudProjectConfigured {
		if err := ensureGCloudProjectConfigured(projectID); err != nil {
			return err
		}
		newState.GCloudProjectConfigured = true
	} else {
		if *projectID == "" && newState.LastCheckedProjectID != "" {
			*projectID = newState.LastCheckedProjectID
		}
		if err := ensureGCloudProjectConfigured(projectID); err != nil {
			return err
		}
	}
	return nil
}

func checkAndConfigure(flag *bool, action func() error) error {
	if !*flag {
		if err := action(); err != nil {
			return err
		}
		*flag = true
	}
	return nil
}

func checkArtifactRegistryAPIEnabled(newState *PrereqState, projectID *string) error {
	if !newState.ArtifactRegistryAPIEnabled {

		if err := ensureArtifactRegistryAPIEnabled(*projectID); err != nil {
			return err
		}
		newState.ArtifactRegistryAPIEnabled = true
	}
	return nil
}

func resolveProjectID(projectID *string) string {
	if *projectID != "" {
		return *projectID
	}
	projectResult := shell.ExecuteCommand("gcloud", "config", "get-value", "project")
	if projectResult.ExitCode == 0 {
		return strings.TrimSpace(projectResult.Stdout)
	}
	return ""
}

func logPrereqStatus(state PrereqState) {
	var found []string
	if state.GCloudSDKInstalled {
		found = append(found, "gcloud")
	}
	if state.KubectlInstalled {
		found = append(found, "kubectl")
	}
	if state.ADCConfigured {
		found = append(found, "ADC")
	}

	if len(found) > 0 {
		logging.Info("%s are in place.", strings.Join(found, ", "))
	}
}

// EnsurePrerequisites checks all necessary gcloud and kubectl prerequisites.
func ensurePrerequisites(projectID *string) error {
	if os.Getenv("GCLUSTER_SKIP_PREREQ_CHECKS") == "true" {
		logging.Info("Skipping prerequisite checks due to GCLUSTER_SKIP_PREREQ_CHECKS environment variable.")
		return nil
	}

	logging.Info("Running prerequisite checks for 'gcluster job submit'...")

	state := loadPrereqState()
	newState := state

	actualProjectID := resolveProjectID(projectID)

	if isStateStale(state, actualProjectID) {
		newState = PrereqState{}
	}

	var useAptFallback bool

	if err := checkAndConfigure(&newState.GCloudSDKInstalled, ensureGCloudSDKInstalled); err != nil {
		return err
	}
	if err := checkGCloudProject(&newState, projectID); err != nil {
		return err
	}
	if err := checkAndConfigure(&newState.GCloudAuthenticated, ensureGCloudAuthenticated); err != nil {
		return err
	}
	if err := checkAndConfigure(&newState.ADCConfigured, ensureApplicationDefaultCredentials); err != nil {
		return err
	}
	if err := checkAndConfigure(&newState.KubectlInstalled, func() error {
		return ensureKubectlInstalled(&useAptFallback)
	}); err != nil {
		return err
	}
	if err := checkAndConfigure(&newState.GKEGCloudAuthPluginInstalled, func() error {
		return ensureGKEGCloudAuthPluginInstalled(&useAptFallback)
	}); err != nil {
		return err
	}
	if err := checkAndConfigure(&newState.DockerCredsConfigured, configureDockerCredentialHelper); err != nil {
		return err
	}
	if err := checkArtifactRegistryAPIEnabled(&newState, projectID); err != nil {
		return err
	}

	newState.LastCheckedTimestamp = time.Now()
	newState.LastCheckedProjectID = *projectID
	savePrereqState(newState)
	logPrereqStatus(newState)

	return nil
}
