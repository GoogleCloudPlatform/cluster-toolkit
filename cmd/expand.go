// Copyright 2021 Google LLC
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

	"github.com/spf13/cobra"
)

func init() {
	expandCmd.Flags().StringVarP(&yamlFilename, "config", "c", "",
		"Configuration file for the new blueprints")
	cobra.CheckErr(expandCmd.Flags().MarkDeprecated("config",
		"please see the command usage for more details."))
	expandCmd.Flags().StringVarP(&outputFilename, "out", "o", "expanded.yaml",
		"Output file for the expanded yaml.")
	rootCmd.AddCommand(expandCmd)
}

var (
	outputFilename string
	expandCmd      = &cobra.Command{
		Use:   "expand",
		Short: "Expand the YAML Config.",
		Long:  "Updates the YAML Config in the same way as create, but without writing the blueprint.",
		Run:   runExpandCmd,
	}
)

func runExpandCmd(cmd *cobra.Command, args []string) {
	if yamlFilename == "" {
		if len(args) == 0 {
			fmt.Println(cmd.UsageString())
			return
		}

		yamlFilename = args[0]
	}

	blueprintConfig := config.NewBlueprintConfig(yamlFilename)
	blueprintConfig.ExpandConfig()
	blueprintConfig.ExportYamlConfig(outputFilename)
	fmt.Printf(
		"Expanded config created successfully, saved as %s.\n", outputFilename)
}
