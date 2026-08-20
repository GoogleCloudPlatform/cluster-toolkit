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
	"context"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"hpc-toolkit/pkg/validators"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func addDeployFlags(c *cobra.Command) *cobra.Command {
	return addJsonOutputFlag(
		addGroupSelectionFlags(
			addAutoApproveFlag(
				addArtifactsDirFlag(
					addCreateFlags(c)))))
}

func init() {
	rootCmd.AddCommand(deployCmd)
}

var (
	deployCmd = addDeployFlags(&cobra.Command{
		Use:               "deploy (<DEPLOYMENT_DIRECTORY> | <BLUEPRINT_FILE>)",
		Short:             "deploy all resources in a Toolkit deployment directory.",
		Long:              "deploy all resources in a Toolkit deployment directory.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkExists),
		ValidArgsFunction: filterYaml,
		Run:               runDeployCmd,
		SilenceUsage:      true,
	})
)

func runDeployCmd(cmd *cobra.Command, args []string) {
	var deplRoot string

	if checkDir(cmd, args) != nil { // arg[0] is BLUEPRINT_FILE
		deplRoot = doCreate(cmd, args[0])
	} else { // arg[0] is DEPLOYMENT_DIRECTORY
		deplRoot = args[0]
		// check that no "create" flags were specified
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Changed && createCmd.LocalFlags().Lookup(f.Name) != nil {
				checkErr(fmt.Errorf("cannot specify flag %q with DEPLOYMENT_DIRECTORY provided", f.Name), nil)
			}
		})
	}
	skipSecurity, _ := cmd.Flags().GetBool("skip-gke-security-check")
	doDeploy(cmd, deplRoot, skipSecurity)
}

func doDeploy(cmd *cobra.Command, deplRoot string, skipSecurity bool) {
	artDir := getArtifactsDir(deplRoot)
	checkErr(shell.CheckWritableDir(artDir), nil)
	bp, ctx := artifactBlueprintOrDie(artDir)
	validators.PerformGkeVulnerabilitiesCheck(skipSecurity, &bp)
	groups := bp.Groups
	checkErr(validateGroupSelectionFlags(bp), ctx)

	var requiredTools []string
	if hasSelectedGroupOfKind(bp, config.TerraformKind) {
		requiredTools = append(requiredTools, "terraform")
	}
	if hasSelectedGroupOfKind(bp, config.PackerKind) {
		requiredTools = append(requiredTools, "packer")
	}

	if len(requiredTools) > 0 {
		checkDependencies(cmd, requiredTools...)
	}

	if os.Getenv("GHPC_SKIP_BUCKET_CREATION") != "true" {
		checkErr(createGcsBucketsIfMissing(cmd.Context(), bp), ctx)
	}

	checkErr(validateRuntimeDependencies(deplRoot, groups), ctx)
	checkErr(shell.ValidateDeploymentDirectory(groups, deplRoot), ctx)

	for ig, group := range groups {
		if !isGroupSelected(group.Name) {
			logging.Info("skipping group %q", group.Name)
			continue
		}

		groupDir := filepath.Join(deplRoot, string(group.Name))
		checkErr(shell.ImportInputs(groupDir, artDir, bp), ctx)

		switch group.Kind() {
		case config.PackerKind:
			// Packer groups are enforced to have length 1
			subPath, e := modulewriter.DeploymentSource(group.Modules[0])
			checkErr(e, ctx)
			moduleDir := filepath.Join(groupDir, subPath)
			checkErr(deployPackerGroup(moduleDir, getApplyBehavior()), ctx)
		case config.TerraformKind:
			checkErr(deployTerraformGroup(groupDir, artDir, getApplyBehavior(), getOutputFormat()), ctx)
		default:
			checkErr(
				config.BpError{
					Err:  fmt.Errorf("group %q is an unsupported kind %q", groupDir, group.Kind()),
					Path: config.Root.Groups.At(ig).Name}, ctx)
		}
	}
	logging.Info("\n###############################")
	printAdvancedInstructionsMessage(deplRoot)
}

func validateRuntimeDependencies(deplDir string, groups []config.Group) error {
	for ig, group := range groups {
		if !isGroupSelected(group.Name) {
			continue
		}
		var err error
		switch group.Kind() {
		case config.PackerKind:
			err = shell.ConfigurePacker()
		case config.TerraformKind:
			groupDir := filepath.Join(deplDir, string(group.Name))
			_, err = shell.ConfigureTerraform(groupDir)
		default:
			err = config.BpError{
				Path: config.Root.Groups.At(ig).Name,
				Err:  fmt.Errorf("group %s is an unsupported kind %q", group.Name, group.Kind().String())}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func deployPackerGroup(moduleDir string, applyBehavior shell.ApplyBehavior) error {
	if err := shell.ConfigurePacker(); err != nil {
		return err
	}
	c := shell.ProposedChanges{
		Summary: fmt.Sprintf("Proposed change: use packer to build image in %s", moduleDir),
		Full:    fmt.Sprintf("Proposed change: use packer to build image in %s", moduleDir),
	}
	buildImage := applyBehavior == shell.AutomaticApply || shell.ApplyChangesChoice(c)
	if buildImage {
		logging.Info("initializing packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, false, "init", "."); err != nil {
			return err
		}
		logging.Info("validating packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, false, "validate", "."); err != nil {
			return err
		}
		logging.Info("building image using packer module at %s", moduleDir)
		if err := shell.ExecPackerCmd(moduleDir, true, "build", "."); err != nil {
			return err
		}
	}
	return nil
}

func deployTerraformGroup(groupDir string, artifactsDir string, applyBehavior shell.ApplyBehavior, outputFormat shell.OutputFormat) error {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}
	return shell.ExportOutputs(tf, artifactsDir, applyBehavior, outputFormat)
}

func createGcsBucketsIfMissing(ctx context.Context, bp config.Blueprint) error {
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
				return fmt.Errorf("failed to initialize GCS client: %w", err)
			}
		}

		bucketHandle := client.Bucket(bucketName)
		ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = bucketHandle.Attrs(ctxTimeout)
		cancel()

		if errors.Is(err, storage.ErrBucketNotExist) {
			if !flagAutoApprove {
				prompt := fmt.Sprintf("The required Terraform state bucket '%s' is missing. We can create it now, but for data safety, 'gcluster destroy' will not delete it later. You must delete it manually when done.\nAlternatively, you can create it manually via `gcloud storage buckets create gs://%s` and then re-run this command.\nCreate it automatically? [y/N]: ", bucketName, bucketName)
				if !confirmActionFunc(prompt) {
					return fmt.Errorf("user aborted")
				}
			} else {
				logging.Info("The required Terraform state bucket '%s' is missing. Auto-approving creation. Note: for data safety, 'gcluster destroy' will not delete it later. You must delete it manually when done.", bucketName)
			}

			projectID := config.GetKeyFromBlueprint("project_id", bp)
			if projectID == "" {
				return fmt.Errorf("cannot create bucket: project_id is missing or invalid in blueprint vars")
			}

			region := config.GetKeyFromBlueprint("region", bp)
			var bucketAttrs *storage.BucketAttrs
			if region != "" {
				bucketAttrs = &storage.BucketAttrs{Location: region}
			}

			ctxCreate, cancelCreate := context.WithTimeout(ctx, 30*time.Second)
			err = bucketHandle.Create(ctxCreate, projectID, bucketAttrs)
			cancelCreate()

			if err != nil {
				return fmt.Errorf("failed to create GCS bucket %q: %w", bucketName, err)
			}
		} else if err != nil {
			logging.Warn("Unable to verify if GCS bucket %q exists: %v", bucketName, err)
		}
	}
	return nil
}
