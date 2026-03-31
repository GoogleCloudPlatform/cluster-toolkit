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
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	stateFileName  = "prereq_state.json"
	stateDirName   = ".gcluster-job"
	stateFreshness = 24 * time.Hour // State is considered fresh for 24 hours
)

// PrereqState holds the current state of prerequisite checks.
type PrereqState struct {
	GCloudSDKInstalled         bool      `json:"gcloud_sdk_installed"`
	GCloudProjectConfigured    bool      `json:"gcloud_project_configured"`
	GCloudAuthenticated        bool      `json:"gcloud_authenticated"`
	ADCConfigured              bool      `json:"adc_configured"`
	KubectlInstalled           bool      `json:"kubectl_installed"`
	DockerCredsConfigured      bool      `json:"docker_creds_configured"`
	ArtifactRegistryAPIEnabled bool      `json:"artifact_registry_api_enabled"`
	LastCheckedProjectID       string    `json:"last_checked_project_id"`
	LastCheckedTimestamp       time.Time `json:"last_checked_timestamp"`
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
		if os.IsNotExist(err) {
			logging.Info("Prerequisite state file not found at %s. Starting with fresh state.", filePath)
		} else {
			logging.Error("Failed to read prerequisite state from %s: %v", filePath, err)
		}
		return PrereqState{}
	}

	var state PrereqState
	if err := json.Unmarshal(data, &state); err != nil {
		logging.Error("Failed to unmarshal prerequisite state from %s: %v. Starting with fresh state.", filePath, err)
		return PrereqState{}
	}
	logging.Info("Prerequisite state loaded from %s", filePath)
	return state
}

// isStateStale checks if the loaded state is older than the defined freshness threshold
// or if the project ID has changed.
func isStateStale(state PrereqState, currentProjectID string) bool {
	if time.Since(state.LastCheckedTimestamp) > stateFreshness {
		logging.Info("Prerequisite state is stale (older than %s). Re-running checks.", stateFreshness.String())
		return true
	}
	if state.LastCheckedProjectID != currentProjectID {
		logging.Info("Project ID changed from '%s' to '%s'. Re-running checks.", state.LastCheckedProjectID, currentProjectID)
		return true
	}
	logging.Info("Prerequisite state is fresh for project %s.", currentProjectID)
	return false
}

// ensureGCloudSDKInstalled checks if gcloud SDK is installed and available in PATH.
func ensureGCloudSDKInstalled() error {
	logging.Info("Checking for Google Cloud SDK installation...")
	result := shell.ExecuteCommand("gcloud", "version")
	if result.ExitCode != 0 {
		logging.Error("Google Cloud SDK not found or not configured correctly. Please install it from https://cloud.google.com/sdk/docs/install and ensure it's in your PATH.")
		return fmt.Errorf("gcloud SDK not found: %s", result.Stderr)
	}
	logging.Info("Google Cloud SDK is installed.")
	return nil
}

// ensureGCloudProjectConfigured checks and configures the default gcloud project.
func ensureGCloudProjectConfigured(projectID *string) error {
	logging.Info("Checking Google Cloud project configuration...")
	if *projectID == "" {
		logging.Info("Google Cloud project ID not provided via --project flag.")
		result := shell.ExecuteCommand("gcloud", "config", "get-value", "project")
		if result.ExitCode != 0 {
			logging.Info("No default gcloud project configured.")
		} else {
			configuredProject := strings.TrimSpace(result.Stdout)
			if configuredProject != "" {
				logging.Info("Using project ID from gcloud configuration: %s", configuredProject)
				*projectID = configuredProject
				return nil
			}
		}

		// Prompt user for project ID
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
	} else {
		logging.Info("Using provided project ID: %s", *projectID)
		// Ensure the provided projectID is actually set as current project, as many gcloud commands rely on this implicit value.
		currentProjectResult := shell.ExecuteCommand("gcloud", "config", "get-value", "project")
		if currentProjectResult.ExitCode != 0 || strings.TrimSpace(currentProjectResult.Stdout) != *projectID {
			logging.Info("Setting gcloud default project to provided project ID: %s", *projectID)
			setResult := shell.ExecuteCommand("gcloud", "config", "set", "project", *projectID)
			if setResult.ExitCode != 0 {
				return fmt.Errorf("failed to set gcloud project to %s: %s", *projectID, setResult.Stderr)
			}
		}
	}
	return nil
}

// ensureGCloudAuthenticated checks if gcloud is authenticated.
func ensureGCloudAuthenticated() error {
	logging.Info("Checking gcloud user authentication...")
	result := shell.ExecuteCommand("gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
		logging.Error("gcloud is not authenticated. Please run 'gcloud auth login' manually in your terminal to complete the authentication process.")
		return fmt.Errorf("gcloud user authentication required")
	} else {
		logging.Info("gcloud is authenticated as: %s", strings.TrimSpace(result.Stdout))
	}
	return nil
}

// ensureApplicationDefaultCredentials checks and configures Application Default Credentials.
func ensureApplicationDefaultCredentials() error {
	logging.Info("Checking Application Default Credentials (ADC)...")
	result := shell.ExecuteCommand("gcloud", "auth", "application-default", "print-access-token")
	if result.ExitCode != 0 {
		logging.Error("Application Default Credentials (ADC) are not configured. Please run 'gcloud auth application-default login' manually in your terminal to complete the authentication process.")
		return fmt.Errorf("Application Default Credentials required")
	} else {
		logging.Info("Application Default Credentials are configured.")
	}
	return nil
}

// ensureKubectlInstalled checks and installs kubectl component.
func ensureKubectlInstalled() error {
	logging.Info("Checking kubectl installation...")
	result := shell.ExecuteCommand("kubectl", "version", "--client", "--output=json")
	if result.ExitCode != 0 {
		logging.Info("kubectl not found. Attempting to install via gcloud components.")
		if !askForConfirmation("Do you want to install 'kubectl' via 'gcloud components install kubectl'?") {
			return fmt.Errorf("kubectl installation required but declined by user")
		}

		installResult := shell.ExecuteCommand("gcloud", "components", "install", "kubectl", "--quiet")
		if installResult.ExitCode != 0 {
			// Check if the error suggests apt-get
			if strings.Contains(installResult.Stderr, "sudo apt-get install kubectl") ||
				strings.Contains(installResult.Stderr, "component manager is disabled") {
				logging.Error("gcloud components install kubectl failed: %s", installResult.Stderr)
				if askForConfirmation("gcloud components install kubectl failed. Do you want to try 'sudo apt-get install kubectl' as a fallback?") {
					aptInstallResult := shell.ExecuteCommand("sudo", "apt-get", "install", "kubectl", "--quiet")
					if aptInstallResult.ExitCode != 0 {
						return fmt.Errorf("failed to install kubectl via apt-get: %s", aptInstallResult.Stderr)
					}
					logging.Info("kubectl installed successfully via apt-get.")
					return nil
				}
				return fmt.Errorf("kubectl installation required but declined apt-get fallback by user")
			}
			return fmt.Errorf("failed to install kubectl via gcloud components: %s", installResult.Stderr)
		}
		logging.Info("kubectl installed successfully via gcloud components.")
	} else {
		logging.Info("kubectl is installed.")
	}
	return nil
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
	logging.Info("Docker credential helper configured for Google registries.")
	return nil
}

// ensureArtifactRegistryAPIEnabled checks and enables the Artifact Registry API.
func ensureArtifactRegistryAPIEnabled(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("cannot check/enable Artifact Registry API: project ID is not set")
	}
	logging.Info("Checking Artifact Registry API status for project %s...", projectID)
	result := shell.ExecuteCommand("gcloud", "services", "list", "--filter=NAME:artifactregistry.googleapis.com", "--format=value(STATE)", "--project", projectID)
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to check Artifact Registry API status: %s", result.Stderr)
	}

	state := strings.TrimSpace(result.Stdout)
	if state != "ENABLED" {
		logging.Info("Artifact Registry API is not enabled for project %s. Attempting to enable it.", projectID)
		if !askForConfirmation(fmt.Sprintf("Do you want to enable 'artifactregistry.googleapis.com' for project %s?", projectID)) {
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

// askForConfirmation prompts the user for a yes/no confirmation.
func askForConfirmation(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s (y/n): ", prompt)
		response, _ := reader.ReadString('\n')
		response = strings.ToLower(strings.TrimSpace(response))
		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		} else {
			fmt.Println("Invalid input. Please enter 'y' or 'n'.")
		}
	}
}

func checkGCloudSDK(newState *PrereqState) error {
	if !newState.GCloudSDKInstalled {
		if err := ensureGCloudSDKInstalled(); err != nil {
			return err
		}
		newState.GCloudSDKInstalled = true
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

func checkGCloudAuthenticated(newState *PrereqState) error {
	if !newState.GCloudAuthenticated {
		if err := ensureGCloudAuthenticated(); err != nil {
			return err
		}
		newState.GCloudAuthenticated = true
	}
	return nil
}

func checkADCConfigured(newState *PrereqState) error {
	if !newState.ADCConfigured {
		if err := ensureApplicationDefaultCredentials(); err != nil {
			return err
		}
		newState.ADCConfigured = true
	}
	return nil
}

func checkKubectlInstalled(newState *PrereqState) error {
	if !newState.KubectlInstalled {
		if err := ensureKubectlInstalled(); err != nil {
			return err
		}
		newState.KubectlInstalled = true
	}
	return nil
}

func checkDockerCredsConfigured(newState *PrereqState) error {
	if !newState.DockerCredsConfigured {
		if err := configureDockerCredentialHelper(); err != nil {
			return err
		}
		newState.DockerCredsConfigured = true
	}
	return nil
}

func checkArtifactRegistryAPIEnabled(newState *PrereqState, projectID *string) error {
	if !newState.ArtifactRegistryAPIEnabled {
		if *projectID == "" {
			return fmt.Errorf("project ID is not set after configuration, cannot enable Artifact Registry API")
		}
		if err := ensureArtifactRegistryAPIEnabled(*projectID); err != nil {
			return err
		}
		newState.ArtifactRegistryAPIEnabled = true
	}
	return nil
}

// EnsurePrerequisites checks all necessary gcloud and kubectl prerequisites.
func EnsurePrerequisites(projectID *string) error {
	if os.Getenv("GCLUSTER_SKIP_PREREQ_CHECKS") == "true" {
		logging.Info("Skipping prerequisite checks due to GCLUSTER_SKIP_PREREQ_CHECKS environment variable.")
		return nil
	}

	state := loadPrereqState()
	newState := state

	var actualProjectID string
	if *projectID != "" {
		actualProjectID = *projectID
	} else {
		projectResult := shell.ExecuteCommand("gcloud", "config", "get-value", "project")
		if projectResult.ExitCode == 0 {
			actualProjectID = strings.TrimSpace(projectResult.Stdout)
		}
	}

	if isStateStale(state, actualProjectID) {
		logging.Info("State is stale or project changed, re-running all prerequisite checks.")
		newState = PrereqState{}
	} else {
		logging.Info("Prerequisite state is fresh, skipping already completed checks.")
	}

	checks := []func(*PrereqState, *string) error{
		func(ns *PrereqState, pID *string) error { return checkGCloudSDK(ns) },
		func(ns *PrereqState, pID *string) error { return checkGCloudProject(ns, pID) },
		func(ns *PrereqState, pID *string) error { return checkGCloudAuthenticated(ns) },
		func(ns *PrereqState, pID *string) error { return checkADCConfigured(ns) },
		func(ns *PrereqState, pID *string) error { return checkKubectlInstalled(ns) },
		func(ns *PrereqState, pID *string) error { return checkDockerCredsConfigured(ns) },
		func(ns *PrereqState, pID *string) error { return checkArtifactRegistryAPIEnabled(ns, pID) },
	}

	for _, check := range checks {
		if err := check(&newState, projectID); err != nil {
			return err
		}
	}

	newState.LastCheckedTimestamp = time.Now()
	newState.LastCheckedProjectID = *projectID
	savePrereqState(newState)

	return nil
}
