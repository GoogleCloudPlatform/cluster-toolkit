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
	"hpc-toolkit/pkg/shell"
	"log"
	"os"
	"path"

	"github.com/spf13/cobra"
)

func init() {
	metadataFlag := "blueprint-metadata"
	artifactsFlag := "artifacts"
	exportCmd.Flags().StringVarP(&artifactsDir, artifactsFlag, "a", "", "Alternative artifacts output directory (automatically configured if unset)")
	exportCmd.Flags().StringVarP(&metadataFile, metadataFlag, "b", "", "Blueprint metadata YAML file (automatically configured if unset)")
	exportCmd.MarkFlagDirname(artifactsFlag)
	exportCmd.MarkFlagFilename(metadataFlag, "yaml", "yml")
	rootCmd.AddCommand(exportCmd)
}

const defaultMetadataFile string = "../.ghpc/deployment_metadata.yaml"

var (
	artifactsDir string
	metadataFile string
	exportCmd    = &cobra.Command{
		Use:               "export-outputs DEPLOYMENT_DIRECTORY",
		Short:             "Export outputs from deployment group.",
		Long:              "Export output values from deployment group to other deployment groups that depend upon them.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), isDir),
		ValidArgsFunction: matchDirs,
		Run:               runExportCmd,
	}
)

func isDir(cmd *cobra.Command, args []string) error {
	path := args[0]
	p, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s must be a directory but does not exist", path)
	}

	if !p.Mode().IsDir() {
		return fmt.Errorf("%s must be a directory but is a file or link", path)
	}

	return nil
}

func matchDirs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs | cobra.ShellCompDirectiveNoFileComp
}

func runExportCmd(cmd *cobra.Command, args []string) {
	workingDir := path.Clean(args[0])

	// if user has not set metadata file, find it in hidden .ghpc directory
	// use this approach rather than set default with Cobra because a relative
	// path to working dir may cause user confusion
	if metadataFile == "" {
		metadataFile = path.Clean(path.Join(workingDir, defaultMetadataFile))
	}

	_, err := shell.ConfigureTerraform(workingDir)
	if err != nil {
		log.Fatal(err)
	}
}
