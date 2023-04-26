// Copyright 2022 Google LLC
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
	"log"

	"github.com/spf13/cobra"
)

func init() {
	expandCmd.Flags().StringVarP(&bpFilename, "config", "c", "",
		"HPC Environment Blueprint file to be expanded.")
	cobra.CheckErr(expandCmd.Flags().MarkDeprecated("config",
		"please see the command usage for more details."))

	expandCmd.Flags().StringVarP(&outputFilename, "out", "o", "expanded.yaml",
		"Output file for the expanded HPC Environment Definition.")
	expandCmd.Flags().StringSliceVar(&cliVariables, "vars", nil, msgCLIVars)
	expandCmd.Flags().StringSliceVar(&cliBEConfigVars, "backend-config", nil, msgCLIBackendConfig)
	expandCmd.Flags().StringVarP(&validationLevel, "validation-level", "l", "WARNING", validationLevelDesc)
	expandCmd.Flags().StringSliceVar(&validatorsToSkip, "skip-validators", nil, skipValidatorsDesc)
	rootCmd.AddCommand(expandCmd)
}

var (
	outputFilename string
	expandCmd      = &cobra.Command{
		Use:   "expand BLUEPRINT_NAME",
		Short: "Expand the Environment Blueprint.",
		Long:  "Updates the Environment Blueprint in the same way as create, but without writing the deployment.",
		Run:   runExpandCmd,
		Args:  cobra.ExactArgs(1),
	}
)

func runExpandCmd(cmd *cobra.Command, args []string) {
	if bpFilename == "" {
		if len(args) == 0 {
			fmt.Println(cmd.UsageString())
			return
		}

		bpFilename = args[0]
	}

	deploymentConfig, err := config.NewDeploymentConfig(bpFilename)
	if err != nil {
		log.Fatal(err)
	}
	if err := deploymentConfig.SetCLIVariables(cliVariables); err != nil {
		log.Fatalf("Failed to set the variables at CLI: %v", err)
	}
	if err := deploymentConfig.SetBackendConfig(cliBEConfigVars); err != nil {
		log.Fatalf("Failed to set the backend config at CLI: %v", err)
	}
	if err := deploymentConfig.SetValidationLevel(validationLevel); err != nil {
		log.Fatal(err)
	}
	if err := skipValidators(&deploymentConfig); err != nil {
		log.Fatal(err)
	}
	if err := deploymentConfig.ExpandConfig(); err != nil {
		log.Fatal(err)
	}
	deploymentConfig.Config.GhpcVersion = GitCommitInfo
	deploymentConfig.ExportBlueprint(outputFilename)
	fmt.Printf(
		"Expanded Environment Definition created successfully, saved as %s.\n", outputFilename)
}
