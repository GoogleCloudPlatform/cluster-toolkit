// Copyright 2026 Google LLC
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

// Package cmd defines command line utilities for gcluster
package cmd

import (
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/validators"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

func addExpandFlags(c *cobra.Command, addOutFlag bool) *cobra.Command {
	if addOutFlag {
		c.Flags().StringVarP(&expandFlags.outputPath, "out", "o", "expanded.yaml",
			"Output file for the expanded HPC Environment Definition.")
	}

	c.Flags().StringVarP(&expandFlags.deploymentFile, "deployment-file", "d", "",
		"Deployment file to override blueprint variables and backend configuration")

	c.Flags().StringSliceVar(&expandFlags.cliVariables, "vars", nil,
		"Comma-separated list of name=value variables to override YAML configuration. Can be used multiple times.")
	c.Flags().StringSliceVar(&expandFlags.cliBEConfigVars, "backend-config", nil,
		"Comma-separated list of name=value variables to set Terraform backend configuration. Can be used multiple times.")
	c.Flags().StringVarP(&expandFlags.validationLevel, "validation-level", "l", "ERROR",
		"Set validation level to one of (\"ERROR\", \"WARNING\", \"IGNORE\")")
	c.Flags().StringSliceVar(&expandFlags.validatorsToSkip, "skip-validators", nil, "Validators to skip")
	c.Flags().BoolVar(&expandFlags.addCreatorLabel, "add-creator-label", false,
		"Add label ghpc_creator to the expanded blueprint. Defaults to true for @google.com accounts.")
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
		addCreatorLabel  bool
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
	bp, ctx := expandOrDie(cmd, args[0])
	checkErr(bp.Export(expandFlags.outputPath), ctx)
	logging.Info(boldGreen("Expanded Environment Definition created successfully, saved as %s."), expandFlags.outputPath)
}

func validateMaybeDie(bp config.Blueprint, ctx config.YamlCtx) {
	err := validators.Execute(bp)
	if err == nil {
		return
	}
	logging.Error("%s", renderError(err, ctx))

	const errorMsg = `One or more blueprint validators has failed. See messages above for suggested
actions. General troubleshooting guidance and instructions for configuring validators are shown below.

- https://goo.gle/hpc-toolkit-troubleshooting
- https://goo.gle/hpc-toolkit-validation

Validators can be silenced or treated as warnings or errors:

- https://goo.gle/hpc-toolkit-validation-levels
`
	logging.Error("%s", errorMsg)

	switch bp.ValidationLevel {
	case config.ValidationWarning:
		{
			logging.Error("%s\n", boldYellow("Validation failures were treated as a warning, continuing to create blueprint."))
		}
	case config.ValidationError:
		{
			logging.Fatal("%s", boldRed("Validation failed due to the issues listed above"))
		}
	}
}

func setCLIVariables(ds *config.DeploymentSettings, s []string) error {
	for _, cliVar := range s {
		arr := strings.SplitN(cliVar, "=", 2)

		if len(arr) != 2 {
			return fmt.Errorf("invalid format: '%s' should follow the 'name=value' format", cliVar)
		}
		// Convert the variable's string literal to its equivalent default type.
		key := arr[0]
		var v config.YamlValue
		if err := yaml.Unmarshal([]byte(arr[1]), &v); err != nil {
			return fmt.Errorf("invalid input: unable to convert '%s' value '%s' to known type", key, arr[1])
		}
		ds.Vars = ds.Vars.With(key, v.Unwrap())
	}
	return nil
}

func setBackendConfig(ds *config.DeploymentSettings, s []string) error {
	if len(s) == 0 {
		return nil // no op
	}
	be := config.TerraformBackend{Type: "gcs"}
	for _, config := range s {
		arr := strings.SplitN(config, "=", 2)

		if len(arr) != 2 {
			return fmt.Errorf("invalid format: '%s' should follow the 'name=value' format", config)
		}

		key, value := arr[0], arr[1]
		switch key {
		case "type":
			be.Type = value
		default:
			be.Configuration = be.Configuration.With(key, cty.StringVal(value))
		}
	}
	ds.TerraformBackendDefaults = be
	return nil
}

// SetValidationLevel allows command-line tools to set the validation level
func setValidationLevel(bp *config.Blueprint, s string) error {
	switch s {
	case "ERROR":
		bp.ValidationLevel = config.ValidationError
	case "WARNING":
		bp.ValidationLevel = config.ValidationWarning
	case "IGNORE":
		bp.ValidationLevel = config.ValidationIgnore
	default:
		return errors.New("invalid validation level (\"ERROR\", \"WARNING\", \"IGNORE\")")
	}
	return nil
}

func skipValidators(bp *config.Blueprint) {
	for _, v := range expandFlags.validatorsToSkip {
		bp.SkipValidator(v)
	}
}
