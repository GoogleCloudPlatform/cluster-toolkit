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
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/reswriter"
	"log"

	"github.com/spf13/cobra"
)

const msgCLIVars = "Comma-separated list of name=value variables to override YAML configuration. Can be invoked multiple times."
const msgCLIBackendConfig = "Comma-separated list of name=value variables to set Terraform backend configuration. Can be invoked multiple times."

func init() {
	createCmd.Flags().StringVarP(&yamlFilename, "config", "c", "",
		"Configuration file for the new blueprints")
	cobra.CheckErr(createCmd.Flags().MarkDeprecated("config",
		"please see the command usage for more details."))

	createCmd.Flags().StringVarP(&bpDirectory, "out", "o", "",
		"Output directory for the new blueprints")
	createCmd.Flags().StringSliceVar(&cliVariables, "vars", nil, msgCLIVars)
	createCmd.Flags().StringSliceVar(&cliBEConfigVars, "backend-config", nil, msgCLIBackendConfig)
	createCmd.Flags().StringVarP(&validationLevel, "validation-level", "l", "WARNING",
		validationLevelDesc)
	createCmd.Flags().BoolVarP(&overwriteBlueprint, "overwrite-blueprint", "w", false,
		"if set, an existing blueprint dir can be overwritten by the created blueprint. \n"+
			"Note: Terraform state IS preserved. \n"+
			"Note: Terraform workspaces are NOT supported (behavior undefined). \n"+
			"Note: Packer is NOT supported.")
	rootCmd.AddCommand(createCmd)
}

var (
	yamlFilename        string
	bpDirectory         string
	cliVariables        []string
	cliBEConfigVars     []string
	overwriteBlueprint  bool
	validationLevel     string
	validationLevelDesc = "Set validation level to one of (\"ERROR\", \"WARNING\", \"IGNORE\")"
	createCmd           = &cobra.Command{
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
	if err := blueprintConfig.SetCLIVariables(cliVariables); err != nil {
		log.Fatalf("Failed to set the variables at CLI: %v", err)
	}
	if err := blueprintConfig.SetBackendConfig(cliBEConfigVars); err != nil {
		log.Fatalf("Failed to set the backend config at CLI: %v", err)
	}
	if err := blueprintConfig.SetValidationLevel(validationLevel); err != nil {
		log.Fatal(err)
	}
	blueprintConfig.ExpandConfig()
	if err := reswriter.WriteBlueprint(&blueprintConfig.Config, bpDirectory, overwriteBlueprint); err != nil {
		var target *reswriter.OverwriteDeniedError
		if errors.As(err, &target) {
			fmt.Printf("\n%s\n", err.Error())
		} else {
			log.Fatal(err)
		}
	}
}
