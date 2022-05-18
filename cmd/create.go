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
	"hpc-toolkit/pkg/modulewriter"
	"log"

	"github.com/spf13/cobra"
)

const msgCLIVars = "Comma-separated list of name=value variables to override YAML configuration. Can be used multiple times."
const msgCLIBackendConfig = "Comma-separated list of name=value variables to set Terraform backend configuration. Can be used multiple times."

func init() {
	createCmd.Flags().StringVarP(&bpFilename, "config", "c", "",
		"HPC Environment Blueprint file to be used to create an HPC deployment dir.")
	cobra.CheckErr(createCmd.Flags().MarkDeprecated("config",
		"please see the command usage for more details."))

	createCmd.Flags().StringVarP(&outputDir, "out", "o", "",
		"Sets the output directory where the HPC deployment directory will be created.")
	createCmd.Flags().StringSliceVar(&cliVariables, "vars", nil, msgCLIVars)
	createCmd.Flags().StringSliceVar(&cliBEConfigVars, "backend-config", nil, msgCLIBackendConfig)
	createCmd.Flags().StringVarP(&validationLevel, "validation-level", "l", "WARNING",
		validationLevelDesc)
	createCmd.Flags().BoolVarP(&overwriteDeployment, "overwrite-deployment", "w", false,
		"If specified, an existing deployment directory is overwritten by the new deployment. \n"+
			"Note: Terraform state IS preserved. \n"+
			"Note: Terraform workspaces are NOT supported (behavior undefined). \n"+
			"Note: Packer is NOT supported.")
	rootCmd.AddCommand(createCmd)
}

var (
	bpFilename   string
	outputDir    string
	cliVariables []string

	cliBEConfigVars     []string
	overwriteDeployment bool
	validationLevel     string
	validationLevelDesc = "Set validation level to one of (\"ERROR\", \"WARNING\", \"IGNORE\")"
	createCmd           = &cobra.Command{
		Use:   "create BLUEPRINT_NAME",
		Short: "Create a new deployment.",
		Long:  "Create a new deployment based on a provided blueprint.",
		Run:   runCreateCmd,
	}
)

func runCreateCmd(cmd *cobra.Command, args []string) {
	if bpFilename == "" {
		if len(args) == 0 {
			fmt.Println(cmd.UsageString())
			return
		}

		bpFilename = args[0]
	}

	deploymentConfig := config.NewDeploymentConfig(bpFilename)
	if err := deploymentConfig.SetCLIVariables(cliVariables); err != nil {
		log.Fatalf("Failed to set the variables at CLI: %v", err)
	}
	if err := deploymentConfig.SetBackendConfig(cliBEConfigVars); err != nil {
		log.Fatalf("Failed to set the backend config at CLI: %v", err)
	}
	if err := deploymentConfig.SetValidationLevel(validationLevel); err != nil {
		log.Fatal(err)
	}
	deploymentConfig.ExpandConfig()
	if err := modulewriter.WriteDeployment(&deploymentConfig.Config, outputDir, overwriteDeployment); err != nil {
		var target *modulewriter.OverwriteDeniedError
		if errors.As(err, &target) {
			fmt.Printf("\n%s\n", err.Error())
		} else {
			log.Fatal(err)
		}
	}
}
