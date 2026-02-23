// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"os"
	"slices"

	"github.com/spf13/cobra"
)

var flagArtifactsDir string

func addArtifactsDirFlag(c *cobra.Command) *cobra.Command {
	c.Flags().StringVarP(&flagArtifactsDir, "artifacts", "a", "", "Artifacts directory (automatically configured if unset)")
	c.MarkFlagDirname("artifacts")
	return c
}

func getArtifactsDir(deploymentRoot string) string {
	if flagArtifactsDir == "" {
		return modulewriter.ArtifactsDir(deploymentRoot)
	}
	return flagArtifactsDir
}

var flagAutoApprove bool

func getApplyBehavior() shell.ApplyBehavior {
	if flagAutoApprove {
		return shell.AutomaticApply
	}
	return shell.PromptBeforeApply
}

func addAutoApproveFlag(c *cobra.Command) *cobra.Command {
	c.Flags().BoolVar(&flagAutoApprove, "auto-approve", false, "Automatically approve proposed changes")
	return c
}

func checkExists(cmd *cobra.Command, args []string) error {
	path := args[0]
	if _, err := os.Lstat(path); err != nil {
		return fmt.Errorf("%q does not exist", path)
	}
	return nil
}

func checkDir(cmd *cobra.Command, args []string) error {
	path := args[0]
	if isDir, _ := shell.DirInfo(path); !(isDir) {
		return fmt.Errorf("%q must be a directory", path)
	}
	return nil
}

func matchDirs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs | cobra.ShellCompDirectiveNoFileComp
}

func filterYaml(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
}

var flagJsonOutput bool

func getOutputFormat() shell.OutputFormat {
	if flagJsonOutput {
		return shell.JsonOutput
	}
	return shell.TextOutput
}

func addJsonOutputFlag(c *cobra.Command) *cobra.Command {
	c.Flags().BoolVar(&flagJsonOutput, "json-output", false, "Write Terraform output to JSON file (instead of default: English text to console)")
	return c
}

var flagSkipGroups []string
var flagOnlyGroups []string

func addGroupSelectionFlags(c *cobra.Command) *cobra.Command {
	c.Flags().StringSliceVar(&flagSkipGroups, "skip", nil, "Skip groups with the given names")
	c.Flags().StringSliceVar(&flagOnlyGroups, "only", nil, "Only apply to groups with the given names")
	return c
}

func validateGroupSelectionFlags(bp config.Blueprint) error {
	if flagOnlyGroups != nil && flagSkipGroups != nil {
		return errors.New("cannot specify both --only and --skip")
	}

	dict := []string{}
	for _, group := range bp.Groups {
		dict = append(dict, string(group.Name))
	}

	for _, g := range append(flagOnlyGroups, flagSkipGroups...) {
		if !slices.Contains(dict, g) {
			return config.HintSpelling(g, dict, fmt.Errorf("group %q not found", g))
		}
	}

	return nil
}

func isGroupSelected(g config.GroupName) bool {
	if flagOnlyGroups != nil {
		return slices.Contains(flagOnlyGroups, string(g))
	}
	if flagSkipGroups != nil {
		return !slices.Contains(flagSkipGroups, string(g))
	}
	return true
}
