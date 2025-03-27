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

// Package cmd defines command line utilities for gcluster
package cmd

import (
	"bufio"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(
		addGroupSelectionFlags(
			addAutoApproveFlag(
				addArtifactsDirFlag(destroyCmd))))
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
		checkErr(fmt.Errorf("artifacts path %s is not a directory", artifactsDir), nil)
	}

	bp, ctx := artifactBlueprintOrDie(artifactsDir)
	checkErr(validateGroupSelectionFlags(bp), ctx)
	checkErr(shell.ValidateDeploymentDirectory(bp.Groups, deplRoot), ctx)

	// destroy in reverse order of creation!
	packerManifests := []string{}
	for i := len(bp.Groups) - 1; i >= 0; i-- {
		group := bp.Groups[i]
		if !isGroupSelected(group.Name) {
			logging.Info("skipping group %q", group.Name)
			continue
		}
		groupDir := filepath.Join(deplRoot, string(group.Name))

		if err := shell.ImportInputs(groupDir, artifactsDir, bp); err != nil {
			logging.Error("failed to import inputs for group %q: %v", group.Name, err)
			// still proceed with destroying the group
		}

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
			err = fmt.Errorf("group %q is an unsupported kind %q", groupDir, group.Kind().String())
		}

		if err != nil {
			logging.Error("failed to destroy group %q:\n%s", group.Name, renderError(err, *ctx))
			if i == 0 || !destroyChoice(bp.Groups[i-1].Name) {
				logging.Fatal("destruction of %q failed", deplRoot)
			}
		}

	}

	modulewriter.WritePackerDestroyInstructions(os.Stdout, packerManifests)
}

func destroyTerraformGroup(groupDir string) error {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}

	// Always output text when destroying the cluster
	// The current implementation outputs JSON only for the "deploy" command
	return shell.Destroy(tf, getApplyBehavior(), shell.TextOutput)
}

func destroyChoice(nextGroup config.GroupName) bool {
	switch getApplyBehavior() {
	case shell.AutomaticApply:
		return true
	case shell.PromptBeforeApply:
		// pass; proceed with prompt
	default:
		return false
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Do you want to delete the next group %q [y/n]?: ", nextGroup)

		in, err := reader.ReadString('\n')
		if err != nil {
			logging.Fatal("%v", err)
		}

		switch strings.ToLower(strings.TrimSpace(in)) {
		case "y":
			return true
		case "n":
			return false
		}
	}
}
