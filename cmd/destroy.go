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
	rootCmd.AddCommand(
		addAutoApproveFlag(
			addArtifactsDirFlag(destroyCmd)))
}

var (
	destroyCmd = &cobra.Command{
		Use:               "destroy DEPLOYMENT_DIRECTORY",
		Short:             "destroy all resources in a Toolkit deployment directory.",
		Long:              "destroy all resources in a Toolkit deployment directory.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		Run:               runDestroyCmd,
		SilenceUsage:      true,
	}
)

func runDestroyCmd(cmd *cobra.Command, args []string) {
	deplRoot := args[0]
	artifactsDir := getArtifactsDir(deplRoot)

	if isDir, _ := shell.DirInfo(artifactsDir); !isDir {
		checkErr(fmt.Errorf("artifacts path %s is not a directory", artifactsDir))
	}

	expandedBlueprintFile := filepath.Join(artifactsDir, modulewriter.ExpandedBlueprintName)
	bp, _, err := config.NewBlueprint(expandedBlueprintFile)
	checkErr(err)

	checkErr(shell.ValidateDeploymentDirectory(bp.Groups, deplRoot))

	// destroy in reverse order of creation!
	packerManifests := []string{}
	for i := len(bp.Groups) - 1; i >= 0; i-- {
		group := bp.Groups[i]
		groupDir := filepath.Join(deplRoot, string(group.Name))

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
		checkErr(err)
	}

	modulewriter.WritePackerDestroyInstructions(os.Stdout, packerManifests)
}

func destroyTerraformGroup(groupDir string) error {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}

	return shell.Destroy(tf, getApplyBehavior())
}
