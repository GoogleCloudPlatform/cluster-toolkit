// Copyright 2023 Google LLC
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

// Package cmd defines command line utilities for ghpc
package cmd

import (
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	artifactsFlag := "artifacts"
	importCmd.Flags().StringVarP(&artifactsDir, artifactsFlag, "a", "", "Artifacts directory (automatically configured if unset)")
	importCmd.MarkFlagDirname(artifactsFlag)
	rootCmd.AddCommand(importCmd)
}

var (
	importCmd = &cobra.Command{
		Use:               "import-inputs DEPLOYMENT_GROUP_DIRECTORY",
		Short:             "Import input values from previous deployment groups.",
		Long:              "Import input values from previous deployment groups upon which this group depends.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		PreRun:            parseExportImportArgs,
		RunE:              runImportCmd,
		SilenceUsage:      true,
	}
)

func runImportCmd(cmd *cobra.Command, args []string) error {
	groupDir := filepath.Clean(args[0])

	if err := shell.CheckWritableDir(groupDir); err != nil {
		return err
	}

	expandedBlueprintFile := filepath.Join(artifactsDir, modulewriter.ExpandedBlueprintName)
	dc, _, err := config.NewDeploymentConfig(expandedBlueprintFile)
	if err != nil {
		return err
	}

	if err := shell.ValidateDeploymentDirectory(dc.Config.DeploymentGroups, deploymentRoot); err != nil {
		return err
	}

	if err := shell.ImportInputs(groupDir, artifactsDir, expandedBlueprintFile); err != nil {
		return err
	}

	return nil
}
