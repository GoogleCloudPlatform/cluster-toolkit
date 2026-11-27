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
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/telemetry"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
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

func verifyGcsBuckets(ctx context.Context, bp config.Blueprint) error {
	buckets, err := modulewriter.GetUniqueGcsBuckets(bp)
	if err != nil {
		return err
	}

	var client *storage.Client
	defer func() {
		if client != nil {
			_ = client.Close()
		}
	}()

	for _, bucketName := range buckets {
		if client == nil {
			client, err = storage.NewClient(ctx)
			if err != nil {
				logging.Warn("Failed to initialize GCS client to verify bucket '%s': %v. Ensure you have the necessary permissions and network access.", bucketName, err)
				continue
			}
		}

		bucketHandle := client.Bucket(bucketName)
		ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = bucketHandle.Attrs(ctxTimeout)
		cancel()

		if errors.Is(err, storage.ErrBucketNotExist) {
			logging.Warn("GCS backend bucket '%s' does not exist. It will be created during the deploy phase.", bucketName)
		} else if err != nil {
			logging.Warn("Failed to connect to GCS to verify bucket '%s': %v. Ensure you have the necessary permissions and network access.", bucketName, err)
		}
	}
	return nil
}

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
	if os.Getenv("GHPC_SKIP_BUCKET_CREATION") != "true" {
		checkErr(verifyGcsBuckets(cmd.Context(), bp), ctx)
	}
	checkErr(modulewriter.WriteDeployment(bp, deplDir), ctx)
	return deplDir
}

func printAdvancedInstructionsMessage(deplDir string) {
	logging.Info("Find instructions for cleanly destroying infrastructure and advanced manual")
	logging.Info("deployment instructions at:")
	logging.Info("")
	logging.Info("%s", modulewriter.InstructionsPath(deplDir))
}

// gcloudAccountTimeout bounds the `gcloud auth list` call used to detect the
// active account. gcloud is a Python program and a cold start can take several
// seconds, so the limit is generous to avoid silently falling back to $USER.
// It is a variable so tests can shorten it.
var gcloudAccountTimeout = 10 * time.Second

// creatorLabelKey is the label that records which account created a deployment.
const creatorLabelKey = "ghpc_creator"

func detectUsername(ctx context.Context) string {
	// Try env var first
	if account := os.Getenv("CLOUDSDK_CORE_ACCOUNT"); account != "" {
		return account
	}

	// Try gcloud next
	account, reason, expected := activeGcloudAccount(ctx)
	if account != "" {
		return account
	}
	const consequence = "A new deployment only gets the ghpc_creator label for @google.com accounts, and later renders keep that choice. " +
		"Set CLOUDSDK_CORE_ACCOUNT or pass --add-creator-label to override."
	if expected {
		// gcloud is not installed or not logged in, as is common for external
		// users and CI: nothing went wrong, so keep this quiet.
		logging.Info("No active gcloud account (%s); using the local user name. %s", reason, consequence)
	} else {
		logging.Warn("Could not detect the active gcloud account (%s); falling back to the local user name. %s", reason, consequence)
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

// activeGcloudAccount returns the active gcloud account, or "" and the reason
// it could not be determined. expected is true when that is a normal setup
// (gcloud not installed, or no account logged in) rather than a failure such
// as a timeout or an error from gcloud.
func activeGcloudAccount(ctx context.Context) (account string, reason string, expected bool) {
	ctx, cancel := context.WithTimeout(ctx, gcloudAccountTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
	var out bytes.Buffer
	cmd.Stdout = &out
	// gcloud is a wrapper script; if it is killed on timeout, its child may
	// keep stdout open. Stop waiting for it shortly after the timeout.
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", fmt.Sprintf("timed out after %s", gcloudAccountTimeout), false
	case errors.Is(err, exec.ErrNotFound):
		return "", "gcloud is not installed", true
	case err != nil:
		return "", err.Error(), false
	}
	if email := strings.TrimSpace(out.String()); email != "" {
		return email, "", false
	}
	return "", "gcloud reported no active account", true
}

// setCreatorLabel decides whether the expanded blueprint gets a ghpc_creator
// label and with which value.
//
// --add-creator-label behaves as before when it is passed. Without the flag,
// when the deployment folder already exists, the decision recorded in its
// expanded blueprint is reused, so re-rendering an existing deployment does
// not add, remove or change ghpc_creator. That label is stamped on every
// resource, and on some resources (e.g. instance templates) a label change
// forces replacement. Only new deployments detect the active account.
//
// The deployment folder is <outputDir>/<deployment_name>. `gcluster expand`
// has no output directory flag, so for expand the lookup is relative to the
// current directory, matching what `gcluster create` does without -o.
func setCreatorLabel(cmd *cobra.Command, bp *config.Blueprint) {
	if cmd.Flags().Changed("add-creator-label") {
		bp.AddCreatorLabel = expandFlags.addCreatorLabel
		if bp.AddCreatorLabel {
			bp.CreatorUsername = detectUsername(cmd.Context())
		}
		return
	}

	if creator, ok := previousCreatorLabel(createFlags.outputDir, *bp); ok {
		if creator != "" {
			bp.AddCreatorLabel = true
			bp.CreatorUsername = creator
		}
		return
	}

	username := detectUsername(cmd.Context())
	if strings.HasSuffix(username, "@google.com") {
		bp.AddCreatorLabel = true
		bp.CreatorUsername = username
	}
}

// previousCreatorLabel reads the ghpc_creator label from the expanded
// blueprint of an existing deployment folder for bp. It returns ok=false when
// there is no readable previous deployment, and ("", true) when the previous
// deployment was rendered without a ghpc_creator label.
func previousCreatorLabel(outputDir string, bp config.Blueprint) (string, bool) {
	name, ok := deploymentNameOf(bp)
	if !ok {
		return "", false
	}
	labels, ok := previousGlobalLabels(filepath.Join(outputDir, name))
	if !ok {
		return "", false
	}
	return creatorFromLabels(labels)
}

// deploymentNameOf returns the deployment_name of bp, if it is a known,
// non-empty string.
func deploymentNameOf(bp config.Blueprint) (string, bool) {
	if !bp.Vars.Has("deployment_name") {
		return "", false
	}
	name, err := bp.Eval(config.GlobalRef("deployment_name").AsValue())
	if err != nil || name.IsNull() || !name.IsKnown() || name.Type() != cty.String || name.AsString() == "" {
		return "", false
	}
	return name.AsString(), true
}

// previousGlobalLabels returns the evaluated vars.labels of the expanded
// blueprint stored in an existing deployment folder.
func previousGlobalLabels(deplDir string) (cty.Value, bool) {
	expPath := filepath.Join(modulewriter.ArtifactsDir(deplDir), modulewriter.ExpandedBlueprintName)
	if _, err := os.Stat(expPath); err != nil {
		return cty.NilVal, false
	}
	prev, _, err := config.NewBlueprint(expPath)
	if err != nil {
		logging.Warn("Could not read ghpc_creator from %s (%v); detecting the active account instead.", expPath, err)
		return cty.NilVal, false
	}
	if !prev.Vars.Has("labels") {
		return cty.NilVal, false
	}

	// When the blueprint or deployment file sets its own vars.labels, the
	// expanded blueprint stores labels as a merge(...) expression rather than
	// a plain map, so evaluate it instead of reading the raw value.
	labels, err := prev.Eval(config.GlobalRef("labels").AsValue())
	if err != nil {
		logging.Warn("Could not read ghpc_creator from %s (%v); detecting the active account instead.", expPath, err)
		return cty.NilVal, false
	}
	labels, _ = labels.Unmark()
	if labels.IsNull() || !labels.IsKnown() || !labels.CanIterateElements() {
		return cty.NilVal, false
	}
	return labels, true
}

// creatorFromLabels returns the ghpc_creator value from labels. It returns
// ("", true) when the label is absent and ok=false when it is not a string.
func creatorFromLabels(labels cty.Value) (string, bool) {
	for it := labels.ElementIterator(); it.Next(); {
		k, v := it.Element()
		if k.Type() != cty.String || k.AsString() != creatorLabelKey {
			continue
		}
		if v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
			return "", false
		}
		return v.AsString(), true
	}
	return "", true
}

func expandOrDie(cmd *cobra.Command, path string) (config.Blueprint, *config.YamlCtx) {
	path = resolveBlueprintPath(path)
	bp, ctx, err := config.NewBlueprint(path)
	checkErr(err, ctx)

	var ds config.DeploymentSettings
	var dCtx config.YamlCtx
	if expandFlags.deploymentFile != "" {
		ds, dCtx, err = readDeploymentFile(expandFlags.deploymentFile)
		checkErr(err, &dCtx)
	}
	if err := setCLIVariables(&ds, expandFlags.cliVariables); err != nil {
		logging.Fatal("Failed to set the variables at CLI: %v", err)
	}
	if err := setBackendConfig(&ds, expandFlags.cliBEConfigVars); err != nil {
		logging.Fatal("Failed to set the backend config at CLI: %v", err)
	}

	checkErr(mergeDeploymentSettings(&bp, ds), ctx)
	setSubblockIO(cmd, &bp, path)

	checkErr(setValidationLevel(&bp, expandFlags.validationLevel), ctx)
	skipValidators(&bp)

	if bp.GhpcVersion != "" {
		logging.Info("ghpc_version setting is ignored.")
	}
	bp.GhpcVersion = GitCommitInfo

	setCreatorLabel(cmd, &bp)

	// Expand the blueprint
	checkErr(bp.Expand(), ctx)
	validateMaybeDie(bp, *ctx)
	applyAgentEnvAttribution(cmd, &bp)

	return bp, ctx
}

// applyAgentEnvAttribution persists ai_assisted: true into deployment artifacts
// and telemetry for create/deploy when invoked inside a coding agent environment,
// after blueprint expansion so shell environment variables do not mutate vars.labels.
func applyAgentEnvAttribution(cmd *cobra.Command, bp *config.Blueprint) {
	if cmd != nil && cmd.Name() != "expand" && telemetry.IsAgentEnv() {
		bp.AIAssisted = true
	}
}

func mergeDeploymentSettings(bp *config.Blueprint, ds config.DeploymentSettings) error {
	if ds.AIAssisted {
		bp.AIAssisted = true
	}
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
