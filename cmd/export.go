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

var defaultArtifactsDir = filepath.Join(modulewriter.HiddenGhpcDirName, modulewriter.ArtifactsDirName)

const expandedBlueprintFilename string = "expanded_blueprint.yaml"

var (
	artifactsDir string
	exportCmd    = &cobra.Command{
		Use:               "export-outputs DEPLOYMENT_GROUP_DIRECTORY",
		Short:             "Export outputs from deployment group.",
		Long:              "Export output values from deployment group to other deployment groups that depend upon them.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		PreRun:            setArtifactsDir,
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

func setArtifactsDir(cmd *cobra.Command, args []string) {
	workingDir := filepath.Clean(args[0])
	deploymentRoot := filepath.Join(workingDir, "..")

	if artifactsDir == "" {
		artifactsDir = filepath.Clean(filepath.Join(deploymentRoot, defaultArtifactsDir))
	}
}

func verifyDeploymentAgainstBlueprint(expandedBlueprintFile string, group string, deploymentRoot string) (config.ModuleKind, error) {
	groupKinds, err := shell.GetDeploymentKinds(expandedBlueprintFile)
	if err != nil {
		return config.UnknownKind, err
	}

	kind, ok := groupKinds[group]
	if !ok {
		return config.UnknownKind, fmt.Errorf("deployment group %s not found in expanded blueprint", group)
	}

	if err := shell.ValidateDeploymentDirectory(groupKinds, deploymentRoot); err != nil {
		return config.UnknownKind, err
	}
	return kind, nil
}

func runExportCmd(cmd *cobra.Command, args []string) error {
	workingDir := filepath.Clean(args[0])
	deploymentGroup := filepath.Base(workingDir)
	deploymentRoot := filepath.Clean(filepath.Join(workingDir, ".."))

	if err := shell.CheckWritableDir(artifactsDir); err != nil {
		return err
	}

	expandedBlueprintFile := filepath.Join(artifactsDir, expandedBlueprintFilename)
	kind, err := verifyDeploymentAgainstBlueprint(expandedBlueprintFile, deploymentGroup, deploymentRoot)
	if err != nil {
		return err
	}
	if kind == config.PackerKind {
		return fmt.Errorf("export command is unsupported on Packer modules because they do not have outputs")
	}

	tf, err := shell.ConfigureTerraform(workingDir)
	if err != nil {
		return err
	}
	if err = shell.ExportOutputs(tf, artifactsDir); err != nil {
		return err
	}
	return nil
}
