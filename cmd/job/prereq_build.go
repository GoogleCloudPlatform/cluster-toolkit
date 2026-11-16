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
	"os"
	"path/filepath"
	"strings"
	"time"

	"hpc-toolkit/pkg/imagebuilder"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"

	"github.com/spf13/cobra"
)

func checkArtifactRegistryAPI(projectID string, state *PrereqState, missing *[]missingPrereq) {
	if projectID == "" {
		return
	}

	apiResult := shell.ExecuteCommandWithTimeout(shell.DefaultCloudAPITimeout, "gcloud", "services", "list", "--filter=NAME:artifactregistry.googleapis.com", "--format=value(STATE)", "--project", projectID)

	if errors.Is(apiResult.Err, context.DeadlineExceeded) {
		logging.Warn("Network timeout while checking Artifact Registry API state. Assuming it needs verification.")

		*missing = append(*missing, missingPrereq{
			name:     fmt.Sprintf("Artifact Registry API (verification timed out after %v)", shell.DefaultCloudAPITimeout),
			commands: []string{fmt.Sprintf("gcloud services enable artifactregistry.googleapis.com --project %s --quiet", projectID)},
		})
		return
	}

	if strings.TrimSpace(apiResult.Stdout) != "ENABLED" {
		if apiResult.ExitCode != 0 && isPermissionDeniedError(apiResult.Stderr, projectID) {
			logging.Warn("Could not verify Artifact Registry API state in project %q due to restricted IAM permissions; proceeding anyway.", projectID)
			state.ArtifactRegistryAPIEnabled = true
			return
		}
		*missing = append(*missing, missingPrereq{
			name:     "Artifact Registry API",
			commands: []string{fmt.Sprintf("gcloud services enable artifactregistry.googleapis.com --project %s --quiet", projectID)},
		})
	} else {
		state.ArtifactRegistryAPIEnabled = true
	}
}

// isDockerCredsConfigured reports whether Docker is configured to authenticate
// against the given Artifact Registry host via a GCP credential helper (gcloud or gcr)
// or an explicit docker login entry in auths.
func isDockerCredsConfigured(registry string) bool {
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

	var config dockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		logging.Error("Failed to unmarshal Docker config from %s: %v", configPath, err)
		return false
	}

	helper, hasHelper := config.CredHelpers[registry]
	if !hasHelper {
		helper = config.CredsStore
	}
	if helper == "gcloud" || helper == "gcr" {
		return true
	}

	for _, key := range []string{registry, "https://" + registry} {
		if config.Auths[key] != (dockerAuthEntry{}) {
			return true
		}
	}
	return false
}

// ensureBuildPrerequisites checks what a Crane build needs for the destination registry:
// Artifact Registry API + Docker gcloud helper for GAR, or a 2s /v2/ ping + Docker keychain check for non-GAR registries.
func ensureBuildPrerequisites(cmd *cobra.Command, projectID string, location string) error {
	if dryRunManifest != "" {
		return nil
	}

	dest, err := imagebuilder.ResolveDestinationRepo(projectID, location, baseImage, os.Getenv("GCLUSTER_IMAGE_REPO"))
	if err != nil {
		return err
	}

	if !dest.IsGAR {
		if err := (&imagebuilder.RegistryInspector{}).InspectTargetRegistry(dest.RegistryHost); err != nil {
			return imagebuilder.WrapDerivedRepoError(dest, err)
		}
		logging.Info("Prerequisites checked successfully.")
		return nil
	}

	return ensureGARBuildPrerequisites(cmd, dest.GARProject, dest.RegistryHost)
}

func effectiveARProjectID(state PrereqState) string {
	if state.LastCheckedARProjectID != "" {
		return state.LastCheckedARProjectID
	}
	return state.LastCheckedProjectID
}

func hasFreshGARBuildState(state PrereqState, arProject string) bool {
	if isTimestampExpired(state) || !state.ArtifactRegistryAPIEnabled {
		return false
	}
	return state.GCloudSDKInstalled &&
		state.GCloudAuthenticated &&
		state.ADCConfigured &&
		effectiveARProjectID(state) == arProject
}

func resetProjectScopedPrereqState(state PrereqState, effectiveProject string) PrereqState {
	state = expireStalePrereqState(state)
	if effectiveARProjectID(state) != effectiveProject {
		state.ArtifactRegistryAPIEnabled = false
		state.LastCheckedARProjectID = ""
	}
	return state
}

func ensureGARBuildPrerequisites(cmd *cobra.Command, effectiveProject, registry string) error {
	state := store.Load()

	// The Docker config is a local file and the registry depends on the target region,
	// so it is read on every build; only the Artifact Registry API lookup is cached.
	if hasFreshGARBuildState(state, effectiveProject) && isDockerCredsConfigured(registry) {
		logging.Info("Skipping checks; prerequisites are fresh (project: %s, checked: %v ago).", effectiveProject, time.Since(state.LastCheckedTimestamp).Round(time.Second))
		return nil
	}

	state = resetProjectScopedPrereqState(state, effectiveProject)

	var missing []missingPrereq
	checkedAuth, err := ensureGCloudAuthAndADC(&state, &missing)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		printMissingPrereqs(cmd, missing)
		return fmt.Errorf("some basic prerequisites are missing")
	}

	checkArtifactRegistryAPI(effectiveProject, &state, &missing)
	if !isDockerCredsConfigured(registry) {
		missing = append(missing, missingPrereq{
			name:     "Docker Credentials",
			commands: []string{fmt.Sprintf("gcloud auth configure-docker %s --quiet", registry)},
		})
	}

	if len(missing) > 0 {
		printMissingPrereqs(cmd, missing)
		return fmt.Errorf("job could not be submitted because some prerequisites are missing")
	}

	state.LastCheckedARProjectID = effectiveProject
	if checkedAuth || state.LastCheckedTimestamp.IsZero() {
		state.LastCheckedTimestamp = time.Now()
	}
	store.Save(state)

	logging.Info("Prerequisites checked successfully.")
	return nil
}
