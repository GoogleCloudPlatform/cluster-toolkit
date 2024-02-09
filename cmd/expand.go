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
	"hpc-toolkit/pkg/logging"

	"github.com/spf13/cobra"
)

func addExpandFlags(c *cobra.Command, addOutFlag bool) *cobra.Command {
	if addOutFlag {
		c.Flags().StringVarP(&expandFlags.outputPath, "out", "o", "expanded.yaml",
			"Output file for the expanded HPC Environment Definition.")
	}

	c.Flags().StringVarP(&expandFlags.deploymentFile, "deployment-file", "d", "",
		"Toolkit Deployment File.")
	c.Flags().MarkHidden("deployment-file")

	c.Flags().StringSliceVar(&expandFlags.cliVariables, "vars", nil,
		"Comma-separated list of name=value variables to override YAML configuration. Can be used multiple times.")
	c.Flags().StringSliceVar(&expandFlags.cliBEConfigVars, "backend-config", nil,
		"Comma-separated list of name=value variables to set Terraform backend configuration. Can be used multiple times.")
	c.Flags().StringVarP(&expandFlags.validationLevel, "validation-level", "l", "WARNING",
		"Set validation level to one of (\"ERROR\", \"WARNING\", \"IGNORE\")")
	c.Flags().StringSliceVar(&expandFlags.validatorsToSkip, "skip-validators", nil, "Validators to skip")
	return c
}

func init() {
	rootCmd.AddCommand(expandCmd)
}

var (
	expandFlags = struct {
		outputPath       string
		deploymentFile   string
		cliVariables     []string
		cliBEConfigVars  []string
		validationLevel  string
		validatorsToSkip []string
	}{}

	expandCmd = addExpandFlags(&cobra.Command{
		Use:               "expand BLUEPRINT_NAME",
		Short:             "Expand the Environment Blueprint.",
		Long:              "Updates the Environment Blueprint in the same way as create, but without writing the deployment.",
		Run:               runExpandCmd,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: filterYaml,
	}, true /*addOutFlag*/)
)

func runExpandCmd(cmd *cobra.Command, args []string) {
	bp := expandOrDie(args[0])
	checkErr(bp.Export(expandFlags.outputPath))
	logging.Info(boldGreen("Expanded Environment Definition created successfully, saved as %s."), expandFlags.outputPath)
}
