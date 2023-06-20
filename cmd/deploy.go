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
	"log"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	artifactsFlag := "artifacts"

	deployCmd.Flags().StringVarP(&artifactsDir, artifactsFlag, "a", "", "Artifacts output directory (automatically configured if unset)")
	deployCmd.MarkFlagDirname(artifactsFlag)

	autoApproveFlag := "auto-approve"
	deployCmd.Flags().BoolVarP(&autoApprove, autoApproveFlag, "", false, "Automatically approve proposed changes")

	rootCmd.AddCommand(deployCmd)
}

var (
	deploymentRoot string
	autoApprove    bool
	applyBehavior  shell.ApplyBehavior
	deployCmd      = &cobra.Command{
		Use:               "deploy DEPLOYMENT_DIRECTORY",
		Short:             "deploy all resources in a Toolkit deployment directory.",
		Long:              "deploy all resources in a Toolkit deployment directory.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		PreRunE:           parseDeployArgs,
		Run:               runDeployCmd,
		SilenceUsage:      true,
	}
)

func parseDeployArgs(cmd *cobra.Command, args []string) error {
	applyBehavior = getApplyBehavior(autoApprove)

	deploymentRoot = args[0]
	artifactsDir = getArtifactsDir(deploymentRoot)
	if err := shell.CheckWritableDir(artifactsDir); err != nil {
		return err
	}

	return nil
}

func getApplyBehavior(autoApprove bool) shell.ApplyBehavior {
	if autoApprove {
		return shell.AutomaticApply
	}
	return shell.PromptBeforeApply
}

func runDeployCmd(cmd *cobra.Command, args []string) {
	expandedBlueprintFile := filepath.Join(artifactsDir, expandedBlueprintFilename)
	dc, err := config.NewDeploymentConfig(expandedBlueprintFile)
	cobra.CheckErr(err)
	cobra.CheckErr(shell.ValidateDeploymentDirectory(dc.Config.DeploymentGroups, deploymentRoot))

	for _, group := range dc.Config.DeploymentGroups {
		groupDir := filepath.Join(deploymentRoot, string(group.Name))
		cobra.CheckErr(shell.ImportInputs(groupDir, artifactsDir, expandedBlueprintFile))

		var err error
		switch group.Kind {
		case config.PackerKind:
			// Packer groups are enforced to have length 1
			subPath, e := modulewriter.DeploymentSource(group.Modules[0])
			cobra.CheckErr(e)
			moduleDir := filepath.Join(groupDir, subPath)
			err = deployPackerGroup(moduleDir)
		case config.TerraformKind:
			err = deployTerraformGroup(groupDir)
		default:
			err = fmt.Errorf("group %s is an unsupported kind %s", groupDir, group.Kind.String())
		}
		cobra.CheckErr(err)
	}
	fmt.Println("\n###############################")
	printAdvancedInstructionsMessage(deploymentRoot)
}

func deployPackerGroup(moduleDir string) error {
	if err := shell.ConfigurePacker(); err != nil {
		return err
	}
	c := shell.ProposedChanges{
		Summary: fmt.Sprintf("Proposed change: use packer to build image in %s", moduleDir),
		Full:    fmt.Sprintf("Proposed change: use packer to build image in %s", moduleDir),
	}
	buildImage := applyBehavior == shell.AutomaticApply || shell.ApplyChangesChoice(c)
	if buildImage {
		log.Printf("initializing packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, false, "init", "."); err != nil {
			return err
		}
		log.Printf("validating packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, false, "validate", "."); err != nil {
			return err
		}
		log.Printf("building image using packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, true, "build", "."); err != nil {
			return err
		}
	}
	return nil
}

func deployTerraformGroup(groupDir string) error {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}

	if err = shell.ExportOutputs(tf, artifactsDir, applyBehavior); err != nil {
		return err
	}
	return nil
}
