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
		PreRun:            parseArgs,
		RunE:              runDeployCmd,
		SilenceUsage:      true,
	}
)

func parseArgs(cmd *cobra.Command, args []string) {
	if autoApprove {
		applyBehavior = shell.AutomaticApply
	} else {
		applyBehavior = shell.PromptBeforeApply
	}

	deploymentRoot = args[0]
}

func runDeployCmd(cmd *cobra.Command, args []string) error {
	if artifactsDir == "" {
		artifactsDir = filepath.Clean(filepath.Join(deploymentRoot, defaultArtifactsDir))
	}

	if err := shell.CheckWritableDir(artifactsDir); err != nil {
		return err
	}

	expandedBlueprintFile := filepath.Join(artifactsDir, expandedBlueprintFilename)
	dc, err := config.NewDeploymentConfig(expandedBlueprintFile)
	if err != nil {
		return err
	}

	if err := shell.ValidateDeploymentDirectory(dc.Config.DeploymentGroups, deploymentRoot); err != nil {
		return err
	}

	for _, group := range dc.Config.DeploymentGroups {
		groupDir := filepath.Join(deploymentRoot, string(group.Name))
		if err = shell.ImportInputs(groupDir, artifactsDir, expandedBlueprintFile); err != nil {
			return err
		}

		var err error
		switch group.Kind {
		case config.PackerKind:
			// Packer groups are enforced to have length 1
			moduleDir := filepath.Join(groupDir, string(group.Modules[0].ID))
			err = deployPackerGroup(moduleDir)
		case config.TerraformKind:
			err = deployTerraformGroup(groupDir)
		default:
			err = fmt.Errorf("group %s is an unsupported kind %s", groupDir, group.Kind.String())
		}
		if err != nil {
			return err
		}

	}
	return nil
}

func deployPackerGroup(moduleDir string) error {
	if err := shell.ConfigurePacker(); err != nil {
		return err
	}
	proposedChange := fmt.Sprintf("Proposed change: use packer to build image in %s", moduleDir)
	buildImage := applyBehavior == shell.AutomaticApply || shell.ApplyChangesChoice(proposedChange)
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
