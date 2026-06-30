// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/modulewriter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"cloud.google.com/go/billing/apiv1/billingpb"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"

	billing "cloud.google.com/go/billing/apiv1"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	resourcemanagerpb "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
)

type result struct {
	mi  modulereader.ModuleInfo
	err error
}

func getBlueprint(cmd *cobra.Command, args []string) config.Blueprint {
	if len(args) == 0 {
		return config.Blueprint{}
	}

	targetPath := resolveBlueprintPath(args[0])

	bp, _, err := config.NewBlueprint(targetPath)
	if err != nil {
		return config.Blueprint{} // Return empty if it fails to parse
	}

	mergeDeploymentFileVars(cmd, &bp)
	mergeCLIVars(cmd, &bp)

	return bp
}

func resolveBlueprintPath(targetPath string) string {
	// If the argument is a directory, it indicates a deployment folder (e.g., used in 'deploy' or 'destroy').
	// We read the expanded blueprint from the artifacts directory instead.
	if info, err := os.Stat(targetPath); err == nil && info.IsDir() {
		return filepath.Join(modulewriter.ArtifactsDir(targetPath), modulewriter.ExpandedBlueprintName)
	}
	return targetPath
}

func mergeDeploymentFileVars(cmd *cobra.Command, bp *config.Blueprint) {
	flag := cmd.Flag("deployment-file")
	if flag == nil || flag.Value.String() == "" {
		return
	}

	ds, _, err := config.NewDeploymentSettings(flag.Value.String())
	if err != nil {
		return
	}

	vars := bp.Vars.Items()
	maps.Copy(vars, ds.Vars.Items())
	bp.Vars = config.NewDict(vars)
}

func mergeCLIVars(cmd *cobra.Command, bp *config.Blueprint) {
	flag := cmd.Flag("vars")
	if flag == nil {
		return
	}

	varsSlice, err := cmd.Flags().GetStringSlice("vars")
	if err != nil {
		return
	}

	for _, cliVar := range varsSlice {
		arr := strings.SplitN(cliVar, "=", 2)
		if len(arr) != 2 {
			continue
		}

		key := arr[0]
		var v config.YamlValue
		// Use YAML unmarshal to support complex types (lists, maps) passed via CLI.
		if err := yaml.Unmarshal([]byte(arr[1]), &v); err == nil {
			bp.Vars = bp.Vars.With(key, v.Unwrap())
		}
	}
}

func getEventMetadataKVPairs(sourceMetadata map[string]string) []map[string]string {
	eventMetadata := make([]map[string]string, 0)
	for k, v := range sourceMetadata {
		eventMetadata = append(eventMetadata, map[string]string{
			"key":   k,
			"value": v,
		})
	}
	return eventMetadata
}

func getBpModulesList(bp config.Blueprint) []string {
	moduleInfos := config.GetAllBpModules(&bp)
	modules := make([]string, len(moduleInfos))
	for i, module := range moduleInfos {
		modules[i] = string(module.Source)
	}
	return modules
}

func ifModulesMatchPatterns(modulesList []string, patterns []string) string {
	for _, m := range modulesList {
		for _, p := range patterns {
			if strings.Contains(m, p) {
				return "true"
			}
		}
	}
	return "false"
}

func getMachineTypeFromModule(m config.Module, bp config.Blueprint) string {
	// 1. Try explicit settings first
	for _, key := range machineTypeSettings {
		if t := extractExplicitStringSetting(key, m, bp); t != "" {
			return t
		}
	}
	// 2. If no explicit setting, try defaults
	for _, key := range machineTypeSettings {
		if t := extractDefaultStringSetting(key, m); t != "" {
			return t
		}
	}

	return ""
}

// extractExplicitStringSetting attempts to get the given key string value if explicitly defined in the module's settings.
func extractExplicitStringSetting(key string, m config.Module, bp config.Blueprint) string {
	if !m.Settings.Has(key) {
		return ""
	}

	keyValue := m.Settings.Get(key)
	// Evaluate the value to resolve expressions like $(vars.key)
	evaluatedKey, err := bp.Eval(keyValue)
	if err != nil {
		return ""
	}

	// Some module outputs or references carry cty marks, so we unmark them safely before use.
	unmarkedKey, _ := evaluatedKey.Unmark()
	if !unmarkedKey.IsNull() && unmarkedKey.Type() == cty.String {
		return unmarkedKey.AsString()
	}

	return ""
}

// extractDefaultStringSetting attempts to get the given key string value from the module's defaults, with a timeout.
func extractDefaultStringSetting(key string, m config.Module) string {
	if m.Source == "" {
		return ""
	}

	kindStr := m.Kind.String()
	// Default to terraform if Kind is omitted (as happens in tests or unexpanded blueprints)
	if kindStr == "" {
		kindStr = config.TerraformKind.String()
	}

	// Only fetch module info if the kind is valid, avoiding a fatal error in GetModuleInfo
	if kindStr != config.TerraformKind.String() && kindStr != config.PackerKind.String() {
		return ""
	}

	resCh := make(chan result, 1)

	// Use a strict timeout. GetModuleInfo can trigger network requests (e.g. git clone).
	go func() {
		mi, err := modulereader.GetModuleInfo(m.Source, kindStr)
		resCh <- result{mi: mi, err: err}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			return ""
		}
		for _, input := range res.mi.Inputs {
			if input.Name == key && input.Default != nil {
				// Verify the default is a string (protects against complex types)
				if mType, ok := input.Default.(string); ok {
					return mType
				}
			}
		}
	case <-time.After(500 * time.Millisecond):
		// Timeout reached: gracefully return empty string to prevent blocking
	}

	return ""
}

func getStaticNodeCountFromModule(m config.Module, bp config.Blueprint) int {
	// 1. Try explicit settings first
	for _, key := range staticNodeCountSettings {
		if t := extractExplicitIntSetting(key, m, bp); t != 0 {
			return t
		}
	}
	// 2. If no explicit setting, try defaults
	for _, key := range staticNodeCountSettings {
		if t := extractDefaultIntSetting(key, m); t != 0 {
			return t
		}
	}
	return 0
}

// extractExplicitIntSetting attempts to get the given key int value if explicitly defined in the module's settings.
func extractExplicitIntSetting(key string, m config.Module, bp config.Blueprint) int {
	if !m.Settings.Has(key) {
		return 0
	}

	keyValue := m.Settings.Get(key)
	// Evaluate the value to resolve expressions like $(vars.key)
	evaluatedKey, err := bp.Eval(keyValue)
	if err != nil {
		return 0
	}

	// Some module outputs or references carry cty marks, so we unmark them safely before use.
	unmarkedKey, _ := evaluatedKey.Unmark()
	if !unmarkedKey.IsNull() && unmarkedKey.Type() == cty.Number {
		out, _ := unmarkedKey.AsBigFloat().Int64()
		return int(out)
	}

	return 0
}

// extractDefaultIntSetting attempts to get the given key int value from the module's defaults, with a timeout.
func extractDefaultIntSetting(key string, m config.Module) int {
	if m.Source == "" {
		return 0
	}

	kindStr := m.Kind.String()
	// Default to terraform if Kind is omitted (as happens in tests or unexpanded blueprints)
	if kindStr == "" {
		kindStr = config.TerraformKind.String()
	}

	// Only fetch module info if the kind is valid, avoiding a fatal error in GetModuleInfo
	if kindStr != config.TerraformKind.String() && kindStr != config.PackerKind.String() {
		return 0
	}

	resCh := make(chan result, 1)

	// Use a strict timeout. GetModuleInfo can trigger network requests (e.g. git clone).
	go func() {
		mi, err := modulereader.GetModuleInfo(m.Source, kindStr)
		resCh <- result{mi: mi, err: err}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			return 0
		}
		for _, input := range res.mi.Inputs {
			if input.Name == key && input.Default != nil {
				// Verify the default is an int (protects against complex types)
				if val, ok := input.Default.(int); ok {
					return val
				}
			}
		}
	case <-time.After(500 * time.Millisecond):
		// Timeout reached: gracefully return empty string to prevent blocking
	}

	return 0
}

// getProjectBillingAccount fetches the billing account associated with a given GCP project in the format "billingAccounts/{billing_account_id}". If billing is disabled for the project, this will return an empty string.
var getProjectBillingAccount = func(ctx context.Context, projectID string) (string, error) {
	client, err := billing.NewCloudBillingClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	req := &billingpb.GetProjectBillingInfoRequest{
		Name: fmt.Sprintf("projects/%s", projectID),
	}

	var info *billingpb.ProjectBillingInfo
	var apiErr error

	// Retry up to 3 times for transient failures (e.g., rate limits or network flakes)
	for attempt := 1; attempt <= 3; attempt++ {
		info, apiErr = client.GetProjectBillingInfo(ctx, req)
		if apiErr == nil {
			return info.GetBillingAccountName(), nil
		}
		// Check for context expiration and avoid sleep on the last iteration to reduce unnecessary latency on failure
		if attempt == 3 || ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) // simple backoff
	}
	return "", apiErr
}

// fetchProjectName retrieves the project name (which contains the project number) for a given project ID.
var fetchProjectName = func(ctx context.Context, projectID string) (string, error) {
	client, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	req := &resourcemanagerpb.GetProjectRequest{Name: fmt.Sprintf("projects/%s", projectID)}

	var project *resourcemanagerpb.Project
	var apiErr error

	// Retry up to 3 times for transient failures (e.g., rate limits or network flakes)
	for attempt := 1; attempt <= 3; attempt++ {
		project, apiErr = client.GetProject(ctx, req)
		if apiErr == nil {
			return project.Name, nil
		}
		// Check for context expiration and avoid sleep on the last iteration to reduce unnecessary latency on failure
		if attempt == 3 || ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) // simple backoff
	}

	return "", apiErr
}

func evaluateIsGoogler() bool {
	// Check Application Default Credentials (ADC) for Service Accounts. CI pipelines usually inject credentials via this environment variable.
	adcPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if adcPath != "" {
		isInternal, err := checkADCForInternalUser(adcPath)
		if err == nil && isInternal {
			return true
		}
	}

	// Fall back to reading the gcloud active config file directly.
	return checkGcloudConfigForInternalUser()
}

// getGcloudConfigDir resolves the gcloud configuration directory based on environment and OS.
func getGcloudConfigDir() (string, error) {
	// Respect the CLOUDSDK_CONFIG environment variable if set
	if envDir := os.Getenv("CLOUDSDK_CONFIG"); envDir != "" {
		return envDir, nil
	}

	// Fall back to OS-specific default paths
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud"), nil
	}

	return filepath.Join(homeDir, ".config", "gcloud"), nil
}

func checkGcloudConfigForInternalUser() bool {
	configDir, err := getGcloudConfigDir()
	if err != nil {
		return false
	}

	// Find the active configuration name
	activeConfigPath := filepath.Join(configDir, "active_config")
	activeConfigBytes, err := os.ReadFile(activeConfigPath)
	if err != nil {
		return false
	}

	activeConfig := strings.TrimSpace(string(activeConfigBytes))
	if activeConfig == "" {
		return false
	}

	// Read the active configuration file
	configFile := filepath.Join(configDir, "configurations", "config_"+activeConfig)
	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		return false
	}

	// Parse the INI-style file to extract the account under [core]
	email := extractAccountFromConfig(configBytes)
	return isInternalEmail(email)
}

// extractAccountFromConfig parses the INI-style gcloud config bytes to extract the account email.
func extractAccountFromConfig(configBytes []byte) string {
	lines := strings.Split(string(configBytes), "\n")
	inCoreSection := false
	for _, line := range lines {
		// Strip inline comments before doing any processing
		if idx := strings.IndexAny(line, "#;"); idx != -1 {
			line = line[:idx]
		}

		// Trim surrounding whitespaces
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inCoreSection = strings.EqualFold(line, "[core]")
			continue
		}

		if inCoreSection {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == "account" {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// checkADCForInternalUser parses the ADC JSON file to extract the client email.
func checkADCForInternalUser(credentialsPath string) (bool, error) {
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return false, err // Fail open (treat as external) if file can't be read
	}

	var key ServiceAccountKey
	if err := json.Unmarshal(data, &key); err != nil {
		return false, err
	}

	return isInternalEmail(key.ClientEmail), nil
}

// isInternalEmail contains the logic to identify Google emails and internal SA domains.
func isInternalEmail(email string) bool {
	if email == "" {
		return false
	}

	// Direct Google employees workstation accounts
	if strings.HasSuffix(email, "@google.com") || strings.HasSuffix(email, ".google.com") {
		return true
	}

	// Allowlist specific internal Cluster Toolkit project IDs that tests use.
	internalProjectNames := []string{
		"hpc-toolkit-dev",
		"hpc-toolkit-demo",
		"hpc-toolkit-gsc",
	}

	for _, projectName := range internalProjectNames {
		pattern := ".*" + projectName + ".*gserviceaccount.com"
		matched, err := regexp.MatchString(pattern, email)

		if err == nil && matched {
			return true
		}
	}

	// Allowlist specific internal Cluster Toolkit project numbers that tests use.
	internalProjectNumbers := []string{
		"508417052821",
		"858831239249",
		"266450182917",
	}
	for _, projectNum := range internalProjectNumbers {
		pattern := ".*" + projectNum + ".*@cloudbuild.gserviceaccount.com"
		matched, err := regexp.MatchString(pattern, email)

		if err == nil && matched {
			return true
		}
	}

	return false
}

// getLinuxVersion parses /etc/os-release to find the pretty name or version ID.
func getLinuxVersion() string {
	// Standard way to identify Linux distribution version
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux (unknown version)"
	}
	defer f.Close()

	var prettyName, versionID string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			prettyName = parseOsReleaseField(line)
		} else if strings.HasPrefix(line, "VERSION_ID=") {
			versionID = parseOsReleaseField(line)
		}
	}

	if prettyName != "" {
		return prettyName
	}
	if versionID != "" {
		return versionID
	}
	return "Linux (unknown version)"
}

// getMacVersion uses sw_vers to get the macOS product version.
func getMacVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return "Darwin (unknown version)"
	}
	return "Darwin " + strings.TrimSpace(string(out))
}

// getWindowsVersion uses the ver command to get the Windows version.
func getWindowsVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cmd", "/c", "ver")
	out, err := cmd.Output()
	if err != nil {
		return "Windows (unknown version)"
	}
	return strings.TrimSpace(string(out))
}

// parseOsReleaseField helper to clean up quotes from /etc/os-release values
func parseOsReleaseField(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.Trim(parts[1], "'\"")
}
