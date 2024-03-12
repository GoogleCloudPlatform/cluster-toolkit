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
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func addDeployFlags(c *cobra.Command) *cobra.Command {
	return addAutoApproveFlag(
		addArtifactsDirFlag(
			addCreateFlags(c)))
}

func init() {
	rootCmd.AddCommand(deployCmd)
}

var (
	deployCmd = addDeployFlags(&cobra.Command{
		Use:               "deploy (<DEPLOYMENT_DIRECTORY> | <BLUEPRINT_FILE>)",
		Short:             "deploy all resources in a Toolkit deployment directory.",
		Long:              "deploy all resources in a Toolkit deployment directory.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkExists),
		ValidArgsFunction: matchDirs,
		Run:               runDeployCmd,
		SilenceUsage:      true,
	})
)

func runDeployCmd(cmd *cobra.Command, args []string) {
	var deplRoot string

	if checkDir(cmd, args) != nil { // arg[0] is BLUEPRINT_FILE
		deplRoot = doCreate(args[0])
	} else { // arg[0] is DEPLOYMENT_DIRECTORY
		deplRoot = args[0]
		// check that no "create" flags were specified
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Changed && createCmd.Flag(f.Name) != nil {
				checkErr(fmt.Errorf("cannot specify flag %q with DEPLOYMENT_DIRECTORY provided", f.Name))
			}
		})
	}
	doDeploy(deplRoot)
}

func doDeploy(deplRoot string) {
	artDir := getArtifactsDir(deplRoot)
	checkErr(shell.CheckWritableDir(artDir))

	expandedBlueprintFile := filepath.Join(artDir, modulewriter.ExpandedBlueprintName)
	bp, _, err := config.NewBlueprint(expandedBlueprintFile)
	checkErr(err)
	groups := bp.Groups
	checkErr(validateRuntimeDependencies(deplRoot, groups))
	checkErr(shell.ValidateDeploymentDirectory(groups, deplRoot))

	for _, group := range groups {
		groupDir := filepath.Join(deplRoot, string(group.Name))
		checkErr(shell.ImportInputs(groupDir, artDir, bp))

		switch group.Kind() {
		case config.PackerKind:
			// Packer groups are enforced to have length 1
			subPath, e := modulewriter.DeploymentSource(group.Modules[0])
			checkErr(e)
			moduleDir := filepath.Join(groupDir, subPath)
			checkErr(deployPackerGroup(moduleDir, getApplyBehavior()))
		case config.TerraformKind:
			checkErr(deployTerraformGroup(groupDir, artDir, getApplyBehavior()))
		default:
			checkErr(fmt.Errorf("group %s is an unsupported kind %s", groupDir, group.Kind().String()))
		}
	}
	logging.Info("\n###############################")
	printAdvancedInstructionsMessage(deplRoot)
}

func validateRuntimeDependencies(deplDir string, groups []config.Group) error {
	for _, group := range groups {
		var err error
		switch group.Kind() {
		case config.PackerKind:
			err = shell.ConfigurePacker()
		case config.TerraformKind:
			groupDir := filepath.Join(deplDir, string(group.Name))
			_, err = shell.ConfigureTerraform(groupDir)
		default:
			err = fmt.Errorf("group %s is an unsupported kind %q", group.Name, group.Kind().String())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func deployPackerGroup(moduleDir string, applyBehavior shell.ApplyBehavior) error {
	if err := shell.ConfigurePacker(); err != nil {
		return err
	}
	c := shell.ProposedChanges{
		Summary: fmt.Sprintf("Proposed change: use packer to build image in %s", moduleDir),
		Full:    fmt.Sprintf("Proposed change: use packer to build image in %s", moduleDir),
	}
	buildImage := applyBehavior == shell.AutomaticApply || shell.ApplyChangesChoice(c)
	if buildImage {
		logging.Info("initializing packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, false, "init", "."); err != nil {
			return err
		}
		logging.Info("validating packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, false, "validate", "."); err != nil {
			return err
		}
		logging.Info("building image using packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, true, "build", "."); err != nil {
			return err
		}
	}
	return nil
}

func deployTerraformGroup(groupDir string, artifactsDir string, applyBehavior shell.ApplyBehavior) error {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}
	return shell.ExportOutputs(tf, artifactsDir, applyBehavior)
}
