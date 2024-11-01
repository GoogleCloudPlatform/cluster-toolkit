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

// Package cmd defines command line utilities for gcluster
package cmd

import (
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/validators"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

func addCreateFlags(c *cobra.Command) *cobra.Command {
	c.Flags().StringVarP(&createFlags.outputDir, "out", "o", "",
		"Sets the output directory where the HPC deployment directory will be created.")
	c.Flags().BoolVarP(&createFlags.overwriteDeployment, "overwrite-deployment", "w", false,
		"If specified, an existing deployment directory is overwritten by the new deployment. \n"+
			"Note: Terraform state IS preserved. \n"+
			"Note: Terraform workspaces are NOT supported (behavior undefined). \n"+
			"Note: Packer is NOT supported.")
	c.Flags().BoolVar(&createFlags.forceOverwrite, "force", false,
		"Forces overwrite of existing deployment directory. \n"+
			"If set, --overwrite-deployment is implied. \n"+
			"No validation is performed on the existing deployment directory.")
	return addExpandFlags(c, false /*addOutFlag to avoid clash with "create" `out` flag*/)
}

func init() {
	rootCmd.AddCommand(createCmd)
}

var (
	createFlags = struct {
		outputDir           string
		overwriteDeployment bool
		forceOverwrite      bool
	}{}

	createCmd = addCreateFlags(&cobra.Command{
		Use:               "create <BLUEPRINT_FILE>",
		Short:             "Create a new deployment.",
		Long:              "Create a new deployment based on a provided blueprint.",
		Run:               runCreateCmd,
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkExists),
		ValidArgsFunction: filterYaml,
	})
)

func runCreateCmd(cmd *cobra.Command, args []string) {
	deplDir := doCreate(args[0])
	logging.Info("To deploy your infrastructure please run:")
	logging.Info("")
	logging.Info(boldGreen("%s deploy %s"), execPath(), deplDir)
	logging.Info("")
	printAdvancedInstructionsMessage(deplDir)
}

func doCreate(path string) string {
	bp, ctx := expandOrDie(path)
	deplDir := filepath.Join(createFlags.outputDir, bp.DeploymentName())
	logging.Info("Creating deployment folder %q ...", deplDir)
	checkErr(checkOverwriteAllowed(deplDir, bp, createFlags.overwriteDeployment, createFlags.forceOverwrite), ctx)
	checkErr(modulewriter.WriteDeployment(bp, deplDir), ctx)
	return deplDir
}

func printAdvancedInstructionsMessage(deplDir string) {
	logging.Info("Find instructions for cleanly destroying infrastructure and advanced manual")
	logging.Info("deployment instructions at:")
	logging.Info("")
	logging.Info(modulewriter.InstructionsPath(deplDir))
}

// TODO: move to expand.go
func expandOrDie(path string) (config.Blueprint, *config.YamlCtx) {
	bp, ctx, err := config.NewBlueprint(path)
	checkErr(err, ctx)

	var ds config.DeploymentSettings
	var dCtx config.YamlCtx
	if expandFlags.deploymentFile != "" {
		ds, dCtx, err = config.NewDeploymentSettings(expandFlags.deploymentFile)
		checkErr(err, &dCtx)
	}
	if err := setCLIVariables(&ds, expandFlags.cliVariables); err != nil {
		logging.Fatal("Failed to set the variables at CLI: %v", err)
	}
	if err := setBackendConfig(&ds, expandFlags.cliBEConfigVars); err != nil {
		logging.Fatal("Failed to set the backend config at CLI: %v", err)
	}

	mergeDeploymentSettings(&bp, ds)

	checkErr(setValidationLevel(&bp, expandFlags.validationLevel), ctx)
	skipValidators(&bp)

	if bp.GhpcVersion != "" {
		logging.Info("ghpc_version setting is ignored.")
	}
	bp.GhpcVersion = GitCommitInfo

	// Expand the blueprint
	checkErr(bp.Expand(), ctx)
	validateMaybeDie(bp, *ctx)
	v5DeprecationWarning(bp)

	return bp, ctx
}

// TODO: Remove this warning when v5 deprecation is complete
func v5DeprecationWarning(bp config.Blueprint) {
	alreadyContainsV5 := false
	bp.WalkModulesSafe(func(mp config.ModulePath, m *config.Module) {
		if strings.Contains(m.Source, "schedmd-slurm-gcp-v5-controller") && !alreadyContainsV5 {
			logging.Info(boldYellow(
				"We have been supporting slurm-gcp v5 since July 2022 and are now deprecating it, as we've launched slurm-gcp v6 in June 2024. \n" +
					"Toolkit blueprints using Slurm-gcp v5 will be marked “deprecated” starting October 2024 and slurm-gcp v6 will be the default deployment. \n" +
					"However we won't begin removing slurm-gcp v5 blueprints until January 6, 2025. Beginning on January 6, 2025, the Cluster Toolkit team will cease their support for Slurm-gcp v5. \n" +
					"While this will not directly or immediately impact running clusters, we recommend replacing any v5 clusters with Slurm-gcp v6.",
			))
			alreadyContainsV5 = true // This is to avoid the logging message showing repeatedly for multiple v5 controllers
		}
	})
}

// TODO: move to expand.go
func validateMaybeDie(bp config.Blueprint, ctx config.YamlCtx) {
	err := validators.Execute(bp)
	if err == nil {
		return
	}
	logging.Error(renderError(err, ctx))

	logging.Error("One or more blueprint validators has failed. See messages above for suggested")
	logging.Error("actions. General troubleshooting guidance and instructions for configuring")
	logging.Error("validators are shown below.")
	logging.Error("")
	logging.Error("- https://goo.gle/hpc-toolkit-troubleshooting")
	logging.Error("- https://goo.gle/hpc-toolkit-validation")
	logging.Error("")
	logging.Error("Validators can be silenced or treated as warnings or errors:")
	logging.Error("")
	logging.Error("- https://goo.gle/hpc-toolkit-validation-levels")
	logging.Error("")

	switch bp.ValidationLevel {
	case config.ValidationWarning:
		{
			logging.Error(boldYellow("Validation failures were treated as a warning, continuing to create blueprint."))
			logging.Error("")
		}
	case config.ValidationError:
		{
			logging.Fatal(boldRed("validation failed due to the issues listed above"))
		}
	}

}

// TODO: move to expand.go
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

// TODO: move to expand.go
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

func mergeDeploymentSettings(bp *config.Blueprint, ds config.DeploymentSettings) error {
	for k, v := range ds.Vars.Items() {
		bp.Vars = bp.Vars.With(k, v)
	}
	if ds.TerraformBackendDefaults.Type != "" {
		bp.TerraformBackendDefaults = ds.TerraformBackendDefaults
	}
	return nil
}

// SetValidationLevel allows command-line tools to set the validation level
// TODO: move to expand.go
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

// TODO: move to expand.go
func skipValidators(bp *config.Blueprint) {
	for _, v := range expandFlags.validatorsToSkip {
		bp.SkipValidator(v)
	}
}

func forceErr(err error) error {
	return config.HintError{
		Err:  err,
		Hint: "Use `--force` to overwrite the deployment anyway. Proceed at your own risk."}
}

// Determines if overwrite is allowed
func checkOverwriteAllowed(depDir string, bp config.Blueprint, overwriteFlag bool, forceFlag bool) error {
	if _, err := os.Stat(depDir); os.IsNotExist(err) || forceFlag {
		return nil // all good, no previous deployment
	}

	if _, err := os.Stat(modulewriter.HiddenGhpcDir(depDir)); os.IsNotExist(err) {
		// hidden ghpc dir does not exist
		return forceErr(fmt.Errorf("folder %q already exists, and it is not a valid GHPC deployment folder", depDir))
	}

	// try to get previous deployment
	expPath := filepath.Join(modulewriter.ArtifactsDir(depDir), modulewriter.ExpandedBlueprintName)
	if _, err := os.Stat(expPath); os.IsNotExist(err) {
		return forceErr(fmt.Errorf("expanded blueprint file %q is missing, this could be a result of changing GHPC version between consecutive deployments", expPath))
	}
	prev, _, err := config.NewBlueprint(expPath)
	if err != nil {
		return forceErr(err)
	}

	if prev.GhpcVersion != bp.GhpcVersion {
		return forceErr(fmt.Errorf(
			"ghpc_version has changed from %q to %q, using different versions of GHPC to update a live deployment is not officially supported",
			prev.GhpcVersion, bp.GhpcVersion))
	}

	if !overwriteFlag {
		return config.HintError{
			Err:  fmt.Errorf("deployment folder %q already exists", depDir),
			Hint: "use -w to overwrite"}
	}

	newGroups := map[config.GroupName]bool{}
	for _, g := range bp.Groups {
		newGroups[g.Name] = true
	}

	for _, g := range prev.Groups {
		if !newGroups[g.Name] {
			return forceErr(fmt.Errorf("you are attempting to remove a deployment group %q, which is not supported", g.Name))
		}
	}

	return nil
}

// Reads an expanded blueprint from the artifacts directory
// IMPORTANT: returned blueprint is "materialized", see config.Blueprint.Materialize
func artifactBlueprintOrDie(artDir string) (config.Blueprint, *config.YamlCtx) {
	path := filepath.Join(artDir, modulewriter.ExpandedBlueprintName)
	bp, ctx, err := config.NewBlueprint(path)
	checkErr(err, ctx)
	checkErr(bp.Materialize(), ctx)
	return bp, ctx
}
