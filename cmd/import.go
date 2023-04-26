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
		PreRun:            setArtifactsDir,
		RunE:              runImportCmd,
	}
)

func runImportCmd(cmd *cobra.Command, args []string) error {
	workingDir := path.Clean(args[0])
	deploymentGroup := path.Base(workingDir)
	deploymentRoot := path.Clean(path.Join(workingDir, ".."))

	if err := shell.CheckWritableDir(workingDir); err != nil {
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
	// TODO: support writing Packer inputs (complexity due to variable resolution)
	if groupKind != config.TerraformKind {
		return fmt.Errorf("import command is only supported (for now) on Terraform deployment groups")
	}

	if err = shell.ImportInputs(workingDir, metadataFile, artifactsDir); err != nil {
		return err
	}

	return nil
}
