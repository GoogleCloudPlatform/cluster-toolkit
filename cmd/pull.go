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

// Package cmd defines command line utilities for gcluster

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/spf13/cobra"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
)

var (
	pullFlags = struct {
		force bool
	}{}

	pullCmd = &cobra.Command{
		Use:   "pull <GCS_URI>",
		Short: "Pull a deployment configuration from a GCS bucket.",
		Long:  "Downloads the expanded blueprint from a GCS bucket and recreates the deployment directory locally.",
		Args:  cobra.ExactArgs(1),
		Run:   runPullCmd,
	}
)

func init() {
	pullCmd.Flags().BoolVar(&pullFlags.force, "force", false, "Overwrite existing directory if it exists.")
	rootCmd.AddCommand(pullCmd)
}

func runPullCmd(cmd *cobra.Command, args []string) {
	gcsURI := args[0]
	if !strings.HasPrefix(gcsURI, "gs://") {
		logging.Fatal("Invalid GCS URI: %s. Must start with gs://", gcsURI)
	}

	bucket, key, err := parseGCSURI(gcsURI)
	if err != nil {
		logging.Fatal("Failed to parse GCS URI: %v", err)
	}

	// We assume the URI points to the deployment root in GCS.
	// The expanded blueprint should be at <URI>/artifacts/expanded_blueprint.yaml
	blueprintKey := filepath.Join(key, modulewriter.ArtifactsDirName, modulewriter.ExpandedBlueprintName)

	logging.Info("Downloading blueprint from gs://%s/%s...", bucket, blueprintKey)

	// Create a temporary file to store the downloaded blueprint
	tmpFile, err := os.CreateTemp("", "expanded_blueprint_*.yaml")
	if err != nil {
		logging.Fatal("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if err := downloadFile(bucket, blueprintKey, tmpFile); err != nil {
		logging.Fatal("Failed to download blueprint: %v", err)
	}
	tmpFile.Close() // Close so we can read it in doCreate

	logging.Info("Blueprint downloaded successfully. Recreating deployment...")

	// We need to set create flags to mimic what the user likely wants.
	// Users can pass --force to pull to overwrite local dir.
	createFlags.forceOverwrite = pullFlags.force
	createFlags.overwriteDeployment = pullFlags.force

	// Skip validators that are known to fail on expanded blueprints
	expandFlags.validatorsToSkip = append(expandFlags.validatorsToSkip, "test_module_not_used", "test_deployment_variable_not_used")

	// Call start of doCreate with the downloaded blueprint
	deplDir := DoCreate(tmpFile.Name())

	logging.Info("\nSuccessfully pulled deployment to %s", deplDir)
	logging.Info("To deploy your infrastructure (and connect to the existing cluster) please run:")
	logging.Info("")
	logging.Info(boldGreen("%s deploy %s"), execPath(), deplDir)
	logging.Info("")
	printAdvancedInstructionsMessage(deplDir)
}

func parseGCSURI(uri string) (bucket, key string, err error) {
	parts := strings.SplitN(strings.TrimPrefix(uri, "gs://"), "/", 2)
	if len(parts) < 1 {
		return "", "", fmt.Errorf("invalid URI format")
	}
	bucket = parts[0]
	if len(parts) > 1 {
		key = parts[1]
	}
	return bucket, key, nil
}

func downloadFile(bucket, key string, dest *os.File) error {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer client.Close()

	rc, err := client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to open GCS object: %w", err)
	}
	defer rc.Close()

	if _, err := io.Copy(dest, rc); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}
	return nil
}
