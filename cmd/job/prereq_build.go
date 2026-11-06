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
		*missing = append(*missing, missingPrereq{
			name:     "Artifact Registry API",
			commands: []string{fmt.Sprintf("gcloud services enable artifactregistry.googleapis.com --project %s --quiet", projectID)},
		})
	} else {
		state.ArtifactRegistryAPIEnabled = true
	}
}

// checkDockerCredentials verifies Docker uses gcloud credentials for the Artifact Registry host a Crane build pushes to.
func checkDockerCredentials(registry string, missing *[]missingPrereq) {
	if !isDockerCredsConfigured(registry) {
		*missing = append(*missing, missingPrereq{
			name:     "Docker Credentials",
			commands: []string{fmt.Sprintf("gcloud auth configure-docker %s --quiet", registry)},
		})
	}
}

func resolveBuildTargetRegistry(projectID, location string) (imagebuilder.DestinationRepo, error) {
	dest, err := imagebuilder.ResolveDestinationRepo(projectID, location, baseImage, os.Getenv("GCLUSTER_IMAGE_REPO"))
	// Direct callers of ensureBuildPrerequisites (such as unit tests) may pass project+location before GCLUSTER_IMAGE_REPO is set.
	if errors.Is(err, imagebuilder.ErrMissingImageRepo) && projectID != "" && location != "" {
		host := shell.ExtractRegion(location) + "-docker.pkg.dev"
		return imagebuilder.DestinationRepo{RegistryHost: host, GARProject: projectID, IsGAR: true}, nil
	}
	return dest, err
}

func hasPassedGARAuthPrerequisites(state PrereqState, projectID string) bool {
	if isStateStale(state, projectID) {
		return false
	}
	return state.GCloudSDKInstalled && state.GCloudAuthenticated && state.ADCConfigured
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

	dest, err := resolveBuildTargetRegistry(projectID, location)
	if err != nil {
		return err
	}

	if !dest.IsGAR {
		if err := imagebuilder.InspectTargetRegistry(dest.RegistryHost); err != nil {
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

func ensureGARBuildPrerequisites(cmd *cobra.Command, effectiveProject, registry string) error {
	state := store.Load()

	// The Docker config is a local file and the registry depends on the target region,
	// so it is read on every build; only the Artifact Registry API lookup is cached.
	if !isStateStale(state, effectiveProject) && state.ArtifactRegistryAPIEnabled && isDockerCredsConfigured(registry) {
		logging.Info("Skipping checks; prerequisites are fresh (project: %s, checked: %v ago).", state.LastCheckedProjectID, time.Since(state.LastCheckedTimestamp).Round(time.Second))
		return nil
	}

	if !hasPassedGARAuthPrerequisites(state, effectiveProject) {
		if isStateStale(state, effectiveProject) {
			state = PrereqState{KubectlInstalled: state.KubectlInstalled}
		}
		if err := ensureGARBuildAuthPrerequisites(cmd, &state); err != nil {
			return err
		}
	}

	var missing []missingPrereq
	checkArtifactRegistryAPI(effectiveProject, &state, &missing)
	checkDockerCredentials(registry, &missing)

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
