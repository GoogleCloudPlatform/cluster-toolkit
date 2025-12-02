// Copyright 2025 Google LLC
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
	"bufio"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"hpc-toolkit/pkg/shell"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"context"

	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"

	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
)

var (
	robustDestroy bool
)

func init() {
	rootCmd.AddCommand(
		addGroupSelectionFlags(
			addAutoApproveFlag(
				addArtifactsDirFlag(destroyCmd))))
	destroyCmd.Flags().BoolVar(&robustDestroy, "robust", false, "Perform a robust destroy, including firewall rule cleanup.")
}

var (
	destroyCmd = &cobra.Command{
		Use:               "destroy DEPLOYMENT_DIRECTORY",
		Short:             "destroy all resources in a Toolkit deployment directory.",
		Long:              "destroy all resources in a Toolkit deployment directory.",
		Args:              cobra.MatchAll(cobra.ExactArgs(1), checkDir),
		ValidArgsFunction: matchDirs,
		Run:               runDestroyCmd,
		SilenceUsage:      true,
	}
)

var (
	destroyGroupsFunc = destroyGroups
)

func runDestroyCmd(cmd *cobra.Command, args []string) {
	deplRoot := args[0]
	artifactsDir := getArtifactsDir(deplRoot)

	if isDir, _ := shell.DirInfo(artifactsDir); !isDir {
		checkErr(fmt.Errorf("artifacts path %s is not a directory", artifactsDir), nil)
	}

	bp, ctx := artifactBlueprintOrDie(artifactsDir)
	checkErr(validateGroupSelectionFlags(bp), ctx)
	checkErr(shell.ValidateDeploymentDirectory(bp.Groups, deplRoot), ctx)

	destroyRunner(deplRoot, artifactsDir, bp, ctx)
}

func destroyRunner(deplRoot string, artifactsDir string, bp config.Blueprint, ctx *config.YamlCtx) {
	maxRetries := 1
	if robustDestroy {
		maxRetries = 3
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		logging.Info("Destroy attempt %d of %d", attempt, maxRetries)

		destroyFailed, packerManifests := destroyGroupsFunc(deplRoot, artifactsDir, bp, ctx)

		if !destroyFailed {
			logging.Info("Successfully destroyed all selected groups.")
			modulewriter.WritePackerDestroyInstructions(os.Stdout, packerManifests)
			return // Exit runDestroyCmd successfully
		}

		if attempt == maxRetries {
			logging.Fatal("Destruction of %q failed after %d attempts", deplRoot, maxRetries)
		}
		logging.Info("Retrying destroy...")
	}
}

func groupHasNetworkModule(group config.Group) bool {
	for _, module := range group.Modules {
		if strings.HasPrefix(module.Source, "modules/network/") || strings.HasPrefix(module.Source, "community/modules/network/") {
			return true
		}
	}
	return false
}

func destroyGroups(deplRoot string, artifactsDir string, bp config.Blueprint, ctx *config.YamlCtx) (bool, []string) {
	// destroy in reverse order of creation!
	packerManifests := []string{}
	destroyFailed := false
	for i := len(bp.Groups) - 1; i >= 0; i-- {
		group := bp.Groups[i]
		if !isGroupSelected(group.Name) {
			logging.Info("skipping group %q", group.Name)
			continue
		}

		if robustDestroy && groupHasNetworkModule(group) {
			projectID, deploymentName, err := getProjectAndDeploymentVars(bp.Vars)
			if err != nil {
				logging.Error("Skipping firewall cleanup: could not get required variables. %v", err)
				destroyFailed = true
				break
			} else if err := cleanupFirewallRules(projectID, deploymentName, group); err != nil {
				logging.Error("Failed to cleanup firewall rules for group %s: %v", group.Name, err)
				destroyFailed = true
				break
			}
		}

		groupDir := filepath.Join(deplRoot, string(group.Name))

		if err := shell.ImportInputs(groupDir, artifactsDir, bp); err != nil {
			logging.Error("failed to import inputs for group %q: %v", group.Name, err)
			// still proceed with destroying the group
		}

		var err error
		switch group.Kind() {
		case config.PackerKind:
			// Packer groups are enforced to have length 1
			// TODO: destroyPackerGroup(moduleDir)
			moduleDir := filepath.Join(groupDir, string(group.Modules[0].ID))
			packerManifests = append(packerManifests, filepath.Join(moduleDir, "packer-manifest.json"))
		case config.TerraformKind:
			err = destroyTerraformGroup(groupDir)
		default:
			err = fmt.Errorf("group %q is an unsupported kind %q", groupDir, group.Kind().String())
		}

		if err != nil {
			logging.Error("failed to destroy group %q:\n%s", group.Name, renderError(err, *ctx))
			destroyFailed = true
			if i == 0 || !destroyChoice(bp.Groups[i-1].Name) {
				break // Stop processing groups for this attempt
			}
		}
	}
	return destroyFailed, packerManifests
}

func getStringVar(vars config.Dict, key string) (string, error) {
	val := vars.Get(key)
	if val.IsNull() {
		return "", fmt.Errorf("%s not found or is null in blueprint vars", key)
	}
	if val.Type() != cty.String {
		return "", fmt.Errorf("%s is not a string, got type %s", key, val.Type().FriendlyName())
	}
	strVal := val.AsString()
	if strVal == "" {
		return "", fmt.Errorf("%s is empty in blueprint vars", key)
	}
	return strVal, nil
}

func getProjectAndDeploymentVars(vars config.Dict) (string, string, error) {
	projectID, err := getStringVar(vars, "project_id")
	if err != nil {
		return "", "", err
	}
	deploymentName, err := getStringVar(vars, "deployment_name")
	if err != nil {
		return "", "", err
	}
	return projectID, deploymentName, nil
}

func destroyTerraformGroup(groupDir string) error {
	tf, err := shell.ConfigureTerraform(groupDir)
	if err != nil {
		return err
	}

	// Always output text when destroying the cluster
	// The current implementation outputs JSON only for the "deploy" command
	return shell.Destroy(tf, getApplyBehavior(), shell.TextOutput)
}

func confirmAction(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(prompt)
		in, err := reader.ReadString('\n')
		if err != nil {
			logging.Error("failed to read user input: %v", err)
			return false // Default to no on error
		}
		switch strings.ToLower(strings.TrimSpace(in)) {
		case "y":
			return true
		case "n":
			return false
		default:
			fmt.Println("Please enter 'y' or 'n'.")
			continue
		}
	}
}

func cleanupFirewallRules(projectID string, deploymentName string, group config.Group) error {
	logging.Info("Cleaning up firewall rules for project %s, deployment %s, group %s", projectID, deploymentName, group.Name)

	ctx := context.Background()
	creds, err := google.FindDefaultCredentials(ctx, compute.ComputeScope)
	if err != nil {
		return fmt.Errorf("failed to find default credentials: %v", err)
	}

	computeService, err := compute.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return fmt.Errorf("failed to create compute service: %v", err)
	}

	potentialNetworkNames, err := getNetworkNamesFromBlueprint(deploymentName, group)
	if err != nil {
		return err
	}
	if len(potentialNetworkNames) == 0 {
		logging.Info("No network modules found or network names identified in group %s, skipping firewall cleanup.", group.Name)
		return nil
	}

	var filterParts []string
	for name := range potentialNetworkNames {
		filterParts = append(filterParts, fmt.Sprintf(`name = "%s"`, name))
	}
	filter := strings.Join(filterParts, " OR ")

	logging.Info("Using network filter: %s", filter)

	networks, err := computeService.Networks.List(projectID).Filter(filter).Do()
	if err != nil {
		return fmt.Errorf("failed to list networks: %v", err)
	}

	if len(networks.Items) == 0 {
		logging.Info("No matching networks found for project %s with filter %s.", projectID, filter)
		return nil
	}

	firewallsToDelete, err := listAssociatedFirewallRules(projectID, computeService, networks.Items)
	if err != nil {
		return err
	}

	if len(firewallsToDelete) == 0 {
		logging.Info("No firewall rules found to delete for the identified networks.")
		return nil
	}

	return confirmAndDeleteFirewallRules(projectID, deploymentName, &computeServiceWrapper{computeService}, firewallsToDelete)
}

type computeServiceWrapper struct {
	*compute.Service
}

func (w *computeServiceWrapper) FirewallsDelete(projectID string, firewall string) (*compute.Operation, error) {
	return w.Firewalls.Delete(projectID, firewall).Do()
}

// listAssociatedFirewallRules lists firewall rules associated with a given set of networks.
func listAssociatedFirewallRules(projectID string, computeService *compute.Service, networks []*compute.Network) ([]*compute.Firewall, error) {
	var firewallsToDelete []*compute.Firewall
	for _, network := range networks {
		fwList, err := computeService.Firewalls.List(projectID).Filter(fmt.Sprintf("network=\"%s\"", network.SelfLink)).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to list firewall rules for network %s: %v", network.Name, err)
		}
		firewallsToDelete = append(firewallsToDelete, fwList.Items...)
	}
	return firewallsToDelete, nil
}

// confirmAndDeleteFirewallRules confirms with the user and then deletes the specified firewall rules.
func confirmAndDeleteFirewallRules(projectID string, deploymentName string, computeService firewallDeleter, firewallsToDelete []*compute.Firewall) error {
	var firewallNames []string
	for _, fw := range firewallsToDelete {
		firewallNames = append(firewallNames, fw.Name)
	}
	logging.Info("Found firewall rules to delete: %v", firewallNames)

	if !flagAutoApprove {
		prompt := fmt.Sprintf("Do you want to delete these %d firewall rules associated with deployment %s? [y/n]: ", len(firewallNames), deploymentName)
		if !confirmAction(prompt) {
			logging.Info("Skipping firewall rule deletion.")
			return nil
		}
	}

	// Delete firewall rules
	var deletionErrors []string
	for _, fwName := range firewallNames {
		logging.Info("Deleting firewall rule %s...", fwName)
		_, err := computeService.FirewallsDelete(projectID, fwName)
		if err != nil {
			// Log non-critical errors and continue trying to delete other rules
			msg := fmt.Sprintf("Failed to delete firewall rule %s: %v", fwName, err)
			logging.Error("error deleting firewall rule: %s", msg)
			deletionErrors = append(deletionErrors, msg)
		}
	}

	if len(deletionErrors) > 0 {
		return fmt.Errorf("encountered errors while deleting firewall rules:\n%s", strings.Join(deletionErrors, "\n"))
	}

	logging.Info("Successfully submitted deletion requests for firewall rules.")
	return nil
}

type firewallDeleter interface {
	FirewallsDelete(projectID string, firewall string) (*compute.Operation, error)
}

// getNetworkNamesFromBlueprint determines potential network names by mimicking terraform logic
func getNetworkNamesFromBlueprint(deploymentName string, group config.Group) (map[string]bool, error) {
	potentialNetworkNames := map[string]bool{}
	for _, module := range group.Modules {
		source := module.Source
		settings := module.Settings

		if strings.Contains(source, "modules/network/vpc") {
			if val := settings.Get("network_name"); !val.IsNull() && val.Type() == cty.String && val.AsString() != "" {
				potentialNetworkNames[val.AsString()] = true
			} else {
				defaultName := strings.ReplaceAll(deploymentName, "_", "-") + "-net"
				potentialNetworkNames[defaultName] = true
			}
		} else if strings.Contains(source, "modules/network/multivpc") {
			names, err := getMultivpcNetworkNamesFromSettings(deploymentName, settings)
			if err != nil {
				return nil, err
			}
			for name := range names {
				potentialNetworkNames[name] = true
			}
		}
	}
	return potentialNetworkNames, nil
}

// getMultivpcNetworkNamesFromSettings extracts network names for multivpc modules.
func getMultivpcNetworkNamesFromSettings(deploymentName string, settings config.Dict) (map[string]bool, error) {
	multiVpcNames := map[string]bool{}
	prefixVal := settings.Get("network_name_prefix")
	countVal := settings.Get("network_count")

	prefix := deploymentName
	if !prefixVal.IsNull() && prefixVal.Type() == cty.String && prefixVal.AsString() != "" {
		prefix = prefixVal.AsString()
	}

	if !countVal.IsNull() && countVal.Type().IsPrimitiveType() {
		var count int64
		if countVal.Type() == cty.Number {
			bf := countVal.AsBigFloat()
			if !bf.IsInt() {
				return nil, fmt.Errorf("network_count must be an integer, but got %s", countVal.GoString())
			}
			var acc big.Accuracy
			count, acc = bf.Int64()
			if acc != big.Exact {
				return nil, fmt.Errorf("network_count %s is out of range for int64", countVal.GoString())
			}
		} else if countVal.Type() == cty.String {
			var err error
			count, err = strconv.ParseInt(countVal.AsString(), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse network_count string %q: %w", countVal.AsString(), err)
			}
		}

		for i := 0; i < int(count); i++ {
			multiVpcNames[fmt.Sprintf("%s-%d", prefix, i)] = true
		}
	}
	return multiVpcNames, nil
}

func destroyChoice(nextGroup config.GroupName) bool {
	switch getApplyBehavior() {
	case shell.AutomaticApply:
		return true
	case shell.PromptBeforeApply:
		// pass; proceed with prompt
	default:
		return false
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Do you want to delete the next group %q [y/n]?: ", nextGroup)

		in, err := reader.ReadString('\n')
		if err != nil {
			logging.Fatal("%v", err)
		}

		switch strings.ToLower(strings.TrimSpace(in)) {
		case "y":
			return true
		case "n":
			return false
		}
	}
}
