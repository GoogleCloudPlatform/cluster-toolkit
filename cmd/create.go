/*
Copyright 2022 Google LLC

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
	"hpc-toolkit/pkg/validators"
	"log"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

const msgCLIVars = "Comma-separated list of name=value variables to override YAML configuration. Can be used multiple times."
const msgCLIBackendConfig = "Comma-separated list of name=value variables to set Terraform backend configuration. Can be used multiple times."

func init() {
	createCmd.Flags().StringVarP(&bpFilenameDeprecated, "config", "c", "", "")
	cobra.CheckErr(createCmd.Flags().MarkDeprecated("config",
		"please see the command usage for more details."))

	createCmd.Flags().StringVarP(&outputDir, "out", "o", "",
		"Sets the output directory where the HPC deployment directory will be created.")
	createCmd.Flags().StringSliceVar(&cliVariables, "vars", nil, msgCLIVars)
	createCmd.Flags().StringSliceVar(&cliBEConfigVars, "backend-config", nil, msgCLIBackendConfig)
	createCmd.Flags().StringVarP(&validationLevel, "validation-level", "l", "WARNING", validationLevelDesc)
	createCmd.Flags().StringSliceVar(&validatorsToSkip, "skip-validators", nil, skipValidatorsDesc)
	createCmd.Flags().BoolVarP(&overwriteDeployment, "overwrite-deployment", "w", false,
		"If specified, an existing deployment directory is overwritten by the new deployment. \n"+
			"Note: Terraform state IS preserved. \n"+
			"Note: Terraform workspaces are NOT supported (behavior undefined). \n"+
			"Note: Packer is NOT supported.")
	rootCmd.AddCommand(createCmd)
}

var (
	bpFilenameDeprecated string
	outputDir            string
	cliVariables         []string

	cliBEConfigVars     []string
	overwriteDeployment bool
	validationLevel     string
	validationLevelDesc = "Set validation level to one of (\"ERROR\", \"WARNING\", \"IGNORE\")"
	validatorsToSkip    []string
	skipValidatorsDesc  = "Validators to skip"

	createCmd = &cobra.Command{
		Use:               "create BLUEPRINT_NAME",
		Short:             "Create a new deployment.",
		Long:              "Create a new deployment based on a provided blueprint.",
		Run:               runCreateCmd,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: filterYaml,
	}
)

func runCreateCmd(cmd *cobra.Command, args []string) {
	dc := expandOrDie(args[0])
	deplName, err := dc.Config.DeploymentName()
	checkErr(err)
	deplDir := filepath.Join(outputDir, deplName)
	checkErr(modulewriter.WriteDeployment(dc, deplDir, overwriteDeployment))

	fmt.Println("To deploy your infrastructure please run:")
	fmt.Println()
	fmt.Printf(boldGreen("%s deploy %s\n"), execPath(), deplDir)
	fmt.Println()
	printAdvancedInstructionsMessage(deplDir)
}

func printAdvancedInstructionsMessage(deplDir string) {
	fmt.Println("Find instructions for cleanly destroying infrastructure and advanced manual")
	fmt.Println("deployment instructions at:")
	fmt.Println()
	fmt.Printf("%s\n", modulewriter.InstructionsPath(deplDir))
}

func expandOrDie(path string) config.DeploymentConfig {
	dc, ctx, err := config.NewDeploymentConfig(path)
	if err != nil {
		log.Fatal(renderError(err, ctx))
	}
	// Set properties from CLI
	if err := setCLIVariables(&dc.Config, cliVariables); err != nil {
		log.Fatalf("Failed to set the variables at CLI: %v", err)
	}
	if err := setBackendConfig(&dc.Config, cliBEConfigVars); err != nil {
		log.Fatalf("Failed to set the backend config at CLI: %v", err)
	}
	if err := setValidationLevel(&dc.Config, validationLevel); err != nil {
		log.Fatal(err)
	}
	if err := skipValidators(&dc); err != nil {
		log.Fatal(err)
	}
	if dc.Config.GhpcVersion != "" {
		fmt.Printf("ghpc_version setting is ignored.")
	}
	dc.Config.GhpcVersion = GitCommitInfo

	// Expand the blueprint
	if err := dc.ExpandConfig(); err != nil {
		log.Fatal(renderError(err, ctx))
	}

	validateMaybeDie(dc.Config, ctx)
	return dc
}

func validateMaybeDie(bp config.Blueprint, ctx config.YamlCtx) {
	err := validators.Execute(bp)
	if err == nil {
		return
	}
	log.Println(renderError(err, ctx))

	log.Println("One or more blueprint validators has failed. See messages above for suggested")
	log.Println("actions. General troubleshooting guidance and instructions for configuring")
	log.Println("validators are shown below.")
	log.Println("")
	log.Println("- https://goo.gle/hpc-toolkit-troubleshooting")
	log.Println("- https://goo.gle/hpc-toolkit-validation")
	log.Println("")
	log.Println("Validators can be silenced or treated as warnings or errors:")
	log.Println("")
	log.Println("- https://goo.gle/hpc-toolkit-validation-levels")
	log.Println("")

	switch bp.ValidationLevel {
	case config.ValidationWarning:
		{
			log.Println(boldYellow("Validation failures were treated as a warning, continuing to create blueprint."))
			log.Println("")
		}
	case config.ValidationError:
		{
			log.Fatal(boldRed("validation failed due to the issues listed above"))
		}
	}

}

func findPos(path config.Path, ctx config.YamlCtx) (config.Pos, bool) {
	pos, ok := ctx.Pos(path)
	for !ok && path.Parent() != nil {
		path = path.Parent()
		pos, ok = ctx.Pos(path)
	}
	return pos, ok
}

func renderError(err error, ctx config.YamlCtx) string {
	switch te := err.(type) {
	case config.Errors:
		var sb strings.Builder
		for _, e := range te.Errors {
			sb.WriteString(renderError(e, ctx))
			sb.WriteString("\n")
		}
		return sb.String()
	case validators.ValidatorError:
		title := boldRed(fmt.Sprintf("validator %q failed:", te.Validator))
		return fmt.Sprintf("%s\n%v\n", title, renderError(te.Err, ctx))
	case config.BpError:
		if pos, ok := findPos(te.Path, ctx); ok {
			return renderRichError(te.Err, pos, ctx)
		}
		return renderError(te.Err, ctx)
	default:
		return err.Error()
	}
}

func renderRichError(err error, pos config.Pos, ctx config.YamlCtx) string {
	line := pos.Line - 1
	if line < 0 {
		line = 0
	}
	if line >= len(ctx.Lines) {
		line = len(ctx.Lines) - 1
	}

	pref := fmt.Sprintf("%d: ", pos.Line)
	arrow := " "
	if pos.Column > 0 {
		spaces := strings.Repeat(" ", len(pref)+pos.Column-1)
		arrow = spaces + "^"
	}

	return fmt.Sprintf(`%s: %s
%s%s
%s`, boldRed("Error"), err, pref, ctx.Lines[line], arrow)
}

func setCLIVariables(bp *config.Blueprint, s []string) error {
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
		bp.Vars.Set(key, v.Unwrap())
	}
	return nil
}

func setBackendConfig(bp *config.Blueprint, s []string) error {
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
			be.Configuration.Set(key, cty.StringVal(value))
		}
	}
	bp.TerraformBackendDefaults = be
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

func skipValidators(dc *config.DeploymentConfig) error {
	if validatorsToSkip == nil {
		return nil
	}
	for _, v := range validatorsToSkip {
		if err := dc.SkipValidator(v); err != nil {
			return err
		}
	}
	return nil
}

func filterYaml(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
}
