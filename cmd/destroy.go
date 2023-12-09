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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	artifactsFlag := "artifacts"
	destroyCmd.Flags().StringVarP(&artifactsDir, artifactsFlag, "a", "", "Artifacts output directory (automatically configured if unset)")
	destroyCmd.MarkFlagDirname(artifactsFlag)

	destroyCmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Automatically approve proposed changes")

	rootCmd.AddCommand(destroyCmd)
}

var (
	destroyCmd = &cobra.Command{
		Use:               "destroy DEPLOYMENT_DIRECTORY",
		Short:             "destroy all resources in a Toolkit deployment directory.",
		Long:              "destroy all resources in a Toolkit deployment directory.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		PreRunE:           parseDestroyArgs,
		RunE:              runDestroyCmd,
		SilenceUsage:      true,
	}
)

func parseDestroyArgs(cmd *cobra.Command, args []string) error {
	applyBehavior = getApplyBehavior(autoApprove)

	deploymentRoot = args[0]
	artifactsDir = getArtifactsDir(deploymentRoot)

	if isDir, _ := shell.DirInfo(artifactsDir); !isDir {
		return fmt.Errorf("artifacts path %s is not a directory", artifactsDir)
	}

	return nil
}

func runDestroyCmd(cmd *cobra.Command, args []string) error {
	expandedBlueprintFile := filepath.Join(artifactsDir, modulewriter.ExpandedBlueprintName)
	dc, _, err := config.NewDeploymentConfig(expandedBlueprintFile)
	if err != nil {
		return err
	}

	if err := shell.ValidateDeploymentDirectory(dc.Config.DeploymentGroups, deploymentRoot); err != nil {
		return err
	}

	// destroy in reverse order of creation!
	packerManifests := []string{}
	for i := len(dc.Config.DeploymentGroups) - 1; i >= 0; i-- {
		group := dc.Config.DeploymentGroups[i]
		groupDir := filepath.Join(deploymentRoot, string(group.Name))

		var err error
		switch group.Kind() {
		case config.PackerKind:
			// Packer groups are enforced to have length 1
			// TODO: destroyPackerGroup(moduleDir)
			moduleDir := filepath.Join(groupDir, string(group.Modules[0].ID))
			packerManifests = append(packerManifests, filepath.Join(moduleDir, "packer-manifest.json"))
		case config.TerraformKind:
			err = destroyTerraformGroup(groupDir)
		default:
			err = fmt.Errorf("group %s is an unsupported kind %s", groupDir, group.Kind().String())
		}
		if err != nil {
			return err
		}
	}

	modulewriter.WritePackerDestroyInstructions(os.Stdout, packerManifests)
	return nil
}

func destroyTerraformGroup(groupDir string) error {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}

	return shell.Destroy(tf, applyBehavior)
}
