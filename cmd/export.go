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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	artifactsFlag := "artifacts"
	exportCmd.Flags().StringVarP(&artifactsDir, artifactsFlag, "a", "", "Artifacts output directory (automatically configured if unset)")
	exportCmd.MarkFlagDirname(artifactsFlag)
	rootCmd.AddCommand(exportCmd)
}

var (
	artifactsDir string
	exportCmd    = &cobra.Command{
		Use:               "export-outputs DEPLOYMENT_GROUP_DIRECTORY",
		Short:             "Export outputs from deployment group.",
		Long:              "Export output values from deployment group to other deployment groups that depend upon them.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		PreRun:            parseExportImportArgs,
		RunE:              runExportCmd,
		SilenceUsage:      true,
	}
)

func checkDir(cmd *cobra.Command, args []string) error {
	path := args[0]
	if path == "" {
		return nil
	}
	if isDir, _ := shell.DirInfo(path); !(isDir) {
		return fmt.Errorf("%s must be a directory", path)
	}

	return nil
}

func matchDirs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs | cobra.ShellCompDirectiveNoFileComp
}

func parseExportImportArgs(cmd *cobra.Command, args []string) {
	deploymentRoot = filepath.Join(filepath.Clean(args[0]), "..")
	artifactsDir = getArtifactsDir(deploymentRoot)
}

func getArtifactsDir(deploymentRoot string) string {
	if artifactsDir == "" {
		return modulewriter.ArtifactsDir(deploymentRoot)
	}
	return artifactsDir
}

func runExportCmd(cmd *cobra.Command, args []string) error {
	groupDir := filepath.Clean(args[0])
	deploymentGroup := config.GroupName(filepath.Base(args[0]))

	if err := shell.CheckWritableDir(artifactsDir); err != nil {
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

	group, err := dc.Config.Group(deploymentGroup)
	if err != nil {
		return err
	}
	if group.Kind() == config.PackerKind {
		return fmt.Errorf("export command is unsupported on Packer modules because they do not have outputs")
	}
	if group.Kind() != config.TerraformKind {
		return fmt.Errorf("export command is supported for Terraform modules only")
	}

	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}
	if err = shell.ExportOutputs(tf, artifactsDir, shell.NeverApply); err != nil {
		return err
	}
	return nil
}
