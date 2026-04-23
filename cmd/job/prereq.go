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

func checkK8sDependencies(newState *PrereqState, missing *[]missingPrereq) {
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
		newState.KubectlInstalled = true
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
		newState.GKEGCloudAuthPluginInstalled = true
	}
}

// EnsurePrerequisites checks all necessary gcloud and kubectl prerequisites.
func ensurePrerequisites(cmd *cobra.Command, projectID *string) error {
	if os.Getenv("GCLUSTER_SKIP_PREREQ_CHECKS") == "true" || dryRunManifest != "" {
		return nil
	}

	logging.Info("Checking prerequisites...")

	state := loadPrereqState()
	newState := state

	if isStateStale(state, *projectID) {
		newState = PrereqState{}
	}

	var missing []missingPrereq

	// Hard dependency: gcloud must be installed
	if err := ensureGCloudSDKInstalled(); err != nil {
		return err
	}
	newState.GCloudSDKInstalled = true

	// Check GCloud Auth
	if err := ensureGCloudAuthenticated(); err != nil {
		missing = append(missing, missingPrereq{name: "Google Cloud Authentication", commands: []string{"gcloud auth login"}})
	} else {
		newState.GCloudAuthenticated = true
	}

	// Check ADC
	if adcCmd := getADCSetupCommand(); adcCmd != "" {
		missing = append(missing, missingPrereq{name: "Application Default Credentials (ADC)", commands: []string{adcCmd}})
	} else {
		newState.ADCConfigured = true
	}

	checkK8sDependencies(&newState, &missing)

	// Check Docker creds
	if !newState.DockerCredsConfigured {
		missing = append(missing, missingPrereq{
			name: "Docker Credentials",
			commands: []string{
				"gcloud auth configure-docker gcr.io --quiet",
				"gcloud auth configure-docker us-central1-docker.pkg.dev --quiet",
			},
		})
		newState.DockerCredsConfigured = true
	}

	// Check Artifact Registry API
	if *projectID != "" {
		apiResult := shell.ExecuteCommand("gcloud", "services", "list", "--filter=NAME:artifactregistry.googleapis.com", "--format=value(STATE)", "--project", *projectID)
		if strings.TrimSpace(apiResult.Stdout) != "ENABLED" {
			missing = append(missing, missingPrereq{
				name:     "Artifact Registry API",
				commands: []string{fmt.Sprintf("gcloud services enable artifactregistry.googleapis.com --project %s --quiet", *projectID)},
			})
		} else {
			newState.ArtifactRegistryAPIEnabled = true
		}
	}

	newState.LastCheckedTimestamp = time.Now()
	newState.LastCheckedProjectID = *projectID
	savePrereqState(newState)

	if len(missing) > 0 {
		printMissingPrereqs(cmd, missing)
		return fmt.Errorf("job could not be submitted because some prerequisites are missing.")
	}

	logging.Info("Prerequisites checked successfully.")
	return nil
}
