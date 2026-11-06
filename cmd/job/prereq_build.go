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
	"errors"
	"fmt"
	"os"
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

func ensureGARBuildAuthPrerequisites(cmd *cobra.Command, state *PrereqState) error {
	if err := ensureGCloudSDKInstalled(); err != nil {
		return err
	}
	var missing []missingPrereq
	if err := ensureGCloudAuthenticated(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		missing = append(missing, missingPrereq{name: "Google Cloud Authentication", commands: []string{"gcloud auth login"}})
	}
	if adcCmd := getADCSetupCommandFunc(); adcCmd != "" {
		missing = append(missing, missingPrereq{name: "Application Default Credentials (ADC)", commands: []string{adcCmd}})
	}
	if len(missing) > 0 {
		printMissingPrereqs(cmd, missing)
		return fmt.Errorf("some basic prerequisites are missing")
	}
	state.GCloudSDKInstalled = true
	state.GCloudAuthenticated = true
	state.ADCConfigured = true
	return nil
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
			return err
		}
		logging.Info("Prerequisites checked successfully.")
		return nil
	}

	effectiveProject := dest.GARProject
	if effectiveProject == "" {
		effectiveProject = projectID
	}
	return ensureGARBuildPrerequisites(cmd, effectiveProject, dest.RegistryHost)
}

func resetProjectScopedPrereqState(state PrereqState, effectiveProject string) PrereqState {
	if !isStateStale(state, effectiveProject) {
		return state
	}
	preserved := PrereqState{
		GCloudSDKInstalled:           state.GCloudSDKInstalled,
		KubectlInstalled:             state.KubectlInstalled,
		GKEGCloudAuthPluginInstalled: state.GKEGCloudAuthPluginInstalled,
	}
	if time.Since(state.LastCheckedTimestamp) <= stateFreshness {
		preserved.GCloudAuthenticated = state.GCloudAuthenticated
		preserved.ADCConfigured = state.ADCConfigured
		preserved.LastCheckedTimestamp = state.LastCheckedTimestamp
	}
	return preserved
}

func ensureGARBuildPrerequisites(cmd *cobra.Command, effectiveProject, registry string) error {
	state := store.Load()

	// The Docker config is a local file and the registry depends on the target region,
	// so it is read on every build; only the Artifact Registry API lookup is cached.
	if !isStateStale(state, effectiveProject) && state.ArtifactRegistryAPIEnabled && isDockerCredsConfigured(registry) {
		logging.Info("Skipping checks; prerequisites are fresh (project: %s, checked: %v ago).", state.LastCheckedProjectID, time.Since(state.LastCheckedTimestamp).Round(time.Second))
		return nil
	}

	state = resetProjectScopedPrereqState(state, effectiveProject)
	if !state.GCloudSDKInstalled || !state.GCloudAuthenticated || !state.ADCConfigured {
		if err := ensureGARBuildAuthPrerequisites(cmd, &state); err != nil {
			return err
		}
	}

	var missing []missingPrereq
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

	state.LastCheckedTimestamp = time.Now()
	state.LastCheckedProjectID = effectiveProject
	store.Save(state)

	logging.Info("Prerequisites checked successfully.")
	return nil
}
