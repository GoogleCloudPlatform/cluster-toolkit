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
	"hpc-toolkit/pkg/shell"
	"path"

	"github.com/spf13/cobra"
)

func init() {
	artifactsFlag := "artifacts"
	exportCmd.Flags().StringVarP(&artifactsDir, artifactsFlag, "a", "", "Artifacts output directory (automatically configured if unset)")
	exportCmd.MarkFlagDirname(artifactsFlag)
	rootCmd.AddCommand(exportCmd)
}

const defaultArtifactsDir string = ".ghpc"

var (
	artifactsDir string
	exportCmd    = &cobra.Command{
		Use:               "export-outputs DEPLOYMENT_DIRECTORY",
		Short:             "Export outputs from deployment group.",
		Long:              "Export output values from deployment group to other deployment groups that depend upon them.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		PreRun:            setArtifactsDir,
		RunE:              runExportCmd,
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
	workingDir := path.Clean(args[0])
	deploymentRoot := path.Join(workingDir, "..")

	if artifactsDir == "" {
		artifactsDir = path.Clean(path.Join(deploymentRoot, defaultArtifactsDir))
	}
}

func runExportCmd(cmd *cobra.Command, args []string) error {
	workingDir := path.Clean(args[0])
	deploymentGroup := path.Base(workingDir)
	deploymentRoot := path.Clean(path.Join(workingDir, ".."))

	if err := shell.CheckWritableDir(artifactsDir); err != nil {
		return err
	}

	// only Terraform groups support outputs; fail on any other kind
	metadataFile := path.Join(artifactsDir, "deployment_metadata.yaml")
	groupKinds, err := shell.GetDeploymentKinds(metadataFile, deploymentRoot)
	if err != nil {
		return err
	}
	groupKind, ok := groupKinds[deploymentGroup]
	if !ok {
		return fmt.Errorf("deployment group %s not found at %s", deploymentGroup, workingDir)
	}
	if groupKind == config.PackerKind {
		return fmt.Errorf("export command is unsupported on Packer modules because they do not have outputs")
	}
	if groupKind != config.TerraformKind {
		return fmt.Errorf("export command is not supported on deployment group: %s", deploymentGroup)
	}

	tf, err := shell.ConfigureTerraform(workingDir)
	if err != nil {
		return err
	}
	if err = shell.ExportOutputs(tf, metadataFile, artifactsDir); err != nil {
		return err
	}
	return nil
}
