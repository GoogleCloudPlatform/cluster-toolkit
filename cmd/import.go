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
	rootCmd.AddCommand(importCmd)
}

var (
	importCmd = addArtifactsDirFlag(&cobra.Command{
		Use:               "import-inputs DEPLOYMENT_GROUP_DIRECTORY",
		Short:             "Import input values from previous deployment groups.",
		Long:              "Import input values from previous deployment groups upon which this group depends.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		Run:               runImportCmd,
		SilenceUsage:      true,
	})
)

func runImportCmd(cmd *cobra.Command, args []string) {
	deplRoot, groupDir := parseExportImportArgs(args)
	artifactsDir := getArtifactsDir(deplRoot)

	checkErr(shell.CheckWritableDir(groupDir))

	expandedBlueprintFile := filepath.Join(artifactsDir, modulewriter.ExpandedBlueprintName)
	bp, _, err := config.NewBlueprint(expandedBlueprintFile)
	checkErr(err)

	checkErr(shell.ValidateDeploymentDirectory(bp.Groups, deplRoot))
	checkErr(shell.ImportInputs(groupDir, artifactsDir, bp))
}
