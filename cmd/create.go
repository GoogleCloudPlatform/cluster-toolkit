/*
Copyright 2021 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package cmd defines command line utilities for ghpc
package cmd

import (
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/reswriter"

	"github.com/spf13/cobra"
)

func init() {
	createCmd.Flags().StringVarP(&yamlFilename, "config", "c", "",
		"Configuration file for the new blueprints")
	cobra.CheckErr(createCmd.Flags().MarkDeprecated("config",
		"please see the command usage for more details."))
	createCmd.Flags().StringVarP(&bpDirectory, "out", "o", "",
		"Output directory for the new blueprints")
	createCmd.Flags().StringSliceVar(&cliVariables, "vars", nil, "Variables to override the YAML config")
	rootCmd.AddCommand(createCmd)
}

var (
	yamlFilename string
	bpDirectory  string
	cliVariables []string
	createCmd    = &cobra.Command{
		Use:   "create FILENAME",
		Short: "Create a new blueprint.",
		Long:  "Create a new blueprint based on a provided YAML config.",
		Run:   runCreateCmd,
	}
)

func runCreateCmd(cmd *cobra.Command, args []string) {
	if yamlFilename == "" {
		if len(args) == 0 {
			fmt.Println(cmd.UsageString())
			return
		}

		yamlFilename = args[0]
	}

	blueprintConfig := config.NewBlueprintConfig(yamlFilename)
	blueprintConfig.SetCLIVariables(cliVariables)
	blueprintConfig.ExpandConfig()
	reswriter.WriteBlueprint(&blueprintConfig.Config, bpDirectory)
}
