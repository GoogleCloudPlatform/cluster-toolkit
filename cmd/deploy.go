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

	// anticipate mutually exclusive flags from-directory, from-group, from-blueprint
	deploymentFlag := "from-directory"
	deployCmd.Flags().StringVarP(&deploymentRoot, deploymentFlag, "d", "", "Deployment root directory")
	deployCmd.MarkFlagDirname(deploymentFlag)
	deployCmd.MarkFlagRequired(deploymentFlag)
	rootCmd.AddCommand(deployCmd)
}

var (
	deploymentRoot string
	autoApprove    bool
	applyBehavior  shell.ApplyBehavior
	deployCmd      = &cobra.Command{
		Use:          "deploy -d DEPLOYMENT_DIRECTORY",
		Short:        "deploy all resources in a Toolkit deployment directory.",
		Long:         "deploy all resources in a Toolkit deployment directory.",
		Args:         cobra.ExactArgs(0),
		PreRun:       setApplyBehavior,
		RunE:         runDeployCmd,
		SilenceUsage: true,
	}
)

func setApplyBehavior(cmd *cobra.Command, args []string) {
	if autoApprove {
		applyBehavior = shell.AutomaticApply
	} else {
		applyBehavior = shell.PromptBeforeApply
	}
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
	if err := shell.TestPacker(); err != nil {
		return err
	}
	buildImage := applyBehavior == shell.AutomaticApply || shell.AskForConfirmation("Build Packer image?")
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
