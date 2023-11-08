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

	"github.com/spf13/cobra"
)

func init() {
	expandCmd.Flags().StringVarP(&bpFilenameDeprecated, "config", "c", "", "")
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
		Use:               "expand BLUEPRINT_NAME",
		Short:             "Expand the Environment Blueprint.",
		Long:              "Updates the Environment Blueprint in the same way as create, but without writing the deployment.",
		Run:               runExpandCmd,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: filterYaml,
	}
)

func runExpandCmd(cmd *cobra.Command, args []string) {
	dc := expandOrDie(args[0])
	checkErr(dc.ExportBlueprint(outputFilename))
	fmt.Printf(boldGreen("Expanded Environment Definition created successfully, saved as %s.\n"), outputFilename)
}
