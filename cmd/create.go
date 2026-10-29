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
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/modulewriter"
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
	setSubblockIO(cmd, &bp, path)

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

	for _, w := range getModifiedIgnoredSettingsWarnings(prev, bp) {
		logging.Warn("%s", w)
	}

	return nil
}

// getModifiedIgnoredSettingsWarnings compares an existing deployment's expanded blueprint
// against the incoming blueprint and returns user-facing warnings for any modified settings
// that correspond to attributes protected by Terraform's lifecycle.ignore_changes.
func getModifiedIgnoredSettingsWarnings(prev, curr config.Blueprint) []string {
	var warnings []string

	prevModules := make(map[config.ModuleID]config.Module)
	for _, g := range prev.Groups {
		for _, m := range g.Modules {
			prevModules[m.ID] = m
		}
	}

	metadataCache := make(map[string]modulereader.Metadata)
	seenWarnings := make(map[string]bool)

	for _, g := range curr.Groups {
		for _, currMod := range g.Modules {
			prevMod, exists := prevModules[currMod.ID]
			if !exists {
				continue // New module being added; ignore_changes only affects existing resources.
			}

			meta, ok := metadataCache[currMod.Source]
			if !ok {
				meta = modulereader.GetMetadataSafe(currMod.Source)
				metadataCache[currMod.Source] = meta
			}

			for _, rule := range meta.Ghpc.Day2IgnoredSettings {
				prevVal := getSettingVal(prevMod.Settings, rule.SettingKey, rule.SettingAliases, rule.NestedKey)
				currVal := getSettingVal(currMod.Settings, rule.SettingKey, rule.SettingAliases, rule.NestedKey)

				if settingChanged(prevVal, currVal) {
					dedupKey := fmt.Sprintf("%s:%s", currMod.ID, rule.Attribute)
					if seenWarnings[dedupKey] {
						continue
					}
					seenWarnings[dedupKey] = true
					warnings = append(warnings, formatWarning(currMod, rule))
				}
			}
		}
	}

	return warnings
}

func isNilOrNull(v cty.Value) bool {
	return v == cty.NilVal || v.IsNull()
}

func getSettingVal(settings config.Dict, key string, aliases []string, nestedKey string) cty.Value {
	val := settings.Get(key)
	if isNilOrNull(val) {
		for _, alias := range aliases {
			if v := settings.Get(alias); !isNilOrNull(v) {
				val = v
				break
			}
		}
	}

	if nestedKey != "" {
		if !isNilOrNull(val) && val.IsKnown() && (val.Type().IsObjectType() || val.Type().IsMapType()) {
			m := val.AsValueMap()
			if v, ok := m[nestedKey]; ok {
				return v
			}
		}
		return cty.NilVal
	}
	return val
}

func formatWarning(mod config.Module, rule modulereader.Day2IgnoredSetting) string {
	var settingPhrase string
	if rule.DisplayName != "" {
		if strings.Contains(rule.DisplayName, "modified") {
			settingPhrase = rule.DisplayName
		} else {
			settingPhrase = fmt.Sprintf("%s was modified", rule.DisplayName)
		}
	} else {
		settingPhrase = fmt.Sprintf("setting '%s' was modified", rule.SettingKey)
	}

	msg := fmt.Sprintf("In module %q (%s): %s from the previous deployment.\n",
		mod.ID, filepath.Base(mod.Source), settingPhrase)
	msg += fmt.Sprintf("Note: %s\n", rule.Reason)
	msg += "This change will NOT be applied to existing resources via Terraform."

	if rule.Remediation != "" {
		resName := string(mod.ID)
		if n := mod.Settings.Get("name"); !isNilOrNull(n) && n.IsKnown() && n.Type() == cty.String && n.AsString() != "" {
			resName = n.AsString()
		}
		remediation := strings.ReplaceAll(rule.Remediation, "{module_id}", string(mod.ID))
		remediation = strings.ReplaceAll(remediation, "{name}", resName)
		msg += fmt.Sprintf("\nTo update safely: %s", remediation)
	}
	return msg
}

func settingChanged(prev, curr cty.Value) bool {
	if isNilOrNull(prev) && isNilOrNull(curr) {
		return false
	}
	if isNilOrNull(prev) || isNilOrNull(curr) {
		return true
	}
	return !prev.RawEquals(curr)
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
