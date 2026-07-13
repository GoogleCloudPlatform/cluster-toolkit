/*
Copyright 2026 Google LLC

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
	"bytes"
	"context"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
	deplDir := doCreate(cmd, args[0])
	logging.Info("To deploy your infrastructure please run:")
	logging.Info("")
	logging.Info(boldGreen("%s deploy %s"), execPath(), deplDir)
	logging.Info("")
	printAdvancedInstructionsMessage(deplDir)
}

func doCreate(cmd *cobra.Command, path string) string {
	bp, ctx := expandOrDie(cmd, path)
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
	logging.Info("%s", modulewriter.InstructionsPath(deplDir))
}

func detectUsername(ctx context.Context) string {
	// Try env var first
	if account := os.Getenv("CLOUDSDK_CORE_ACCOUNT"); account != "" {
		return account
	}

	// Try gcloud next
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		email := strings.TrimSpace(out.String())
		if email != "" {
			return email
		}
	}

	// Fallback to shell
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}

	// Last resort
	u, err := user.Current()
	if err == nil {
		return u.Username
	}

	return "unknown"
}

func expandOrDie(cmd *cobra.Command, path string) (config.Blueprint, *config.YamlCtx) {
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

	if cmd.Flags().Changed("add-creator-label") {
		bp.AddCreatorLabel = expandFlags.addCreatorLabel
		if bp.AddCreatorLabel {
			bp.CreatorUsername = detectUsername(cmd.Context())
		}
	} else {
		username := detectUsername(cmd.Context())
		if strings.HasSuffix(username, "@google.com") {
			bp.AddCreatorLabel = true
			bp.CreatorUsername = username
		}
	}

	// Expand the blueprint
	checkErr(bp.Expand(), ctx)
	validateMaybeDie(bp, *ctx)

	return bp, ctx
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
