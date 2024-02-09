package cmd

import (
	"fmt"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"os"

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

func checkExists(cmd *cobra.Command, args []string) error {
	path := args[0]
	if path == "" {
		return nil
	}
	if _, err := os.Lstat(path); err != nil {
		return fmt.Errorf("%q does not exist", path)
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
