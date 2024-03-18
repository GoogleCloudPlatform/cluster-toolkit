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
	"errors"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(exportCmd)
}

var (
	exportCmd = addArtifactsDirFlag(&cobra.Command{
		Use:               "export-outputs DEPLOYMENT_GROUP_DIRECTORY",
		Short:             "Export outputs from deployment group.",
		Long:              "Export output values from deployment group to other deployment groups that depend upon them.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		Run:               runExportCmd,
		SilenceUsage:      true,
	})
)

func parseExportImportArgs(args []string) (string, string) {
	gd, err := filepath.Abs(args[0])
	checkErr(err)
	return filepath.Join(gd, ".."), gd
}

func runExportCmd(cmd *cobra.Command, args []string) {
	deplRoot, groupDir := parseExportImportArgs(args)

	artifactsDir := getArtifactsDir(deplRoot)
	groupName := config.GroupName(filepath.Base(groupDir))

	checkErr(shell.CheckWritableDir(artifactsDir))

	expandedBlueprintFile := filepath.Join(artifactsDir, modulewriter.ExpandedBlueprintName)
	bp, _, err := config.NewBlueprint(expandedBlueprintFile)
	checkErr(err)

	checkErr(shell.ValidateDeploymentDirectory(bp.Groups, deplRoot))

	group, err := bp.Group(groupName)
	checkErr(err)

	if group.Kind() == config.PackerKind {
		checkErr(errors.New("export command is unsupported on Packer modules because they do not have outputs"))
	}
	if group.Kind() != config.TerraformKind {
		checkErr(errors.New("export command is supported for Terraform modules only"))
	}

	tf, err := shell.ConfigureTerraform(groupDir)
	checkErr(err)

	checkErr(shell.ExportOutputs(tf, artifactsDir, shell.NeverApply))
}
