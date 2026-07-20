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
	"context"
	"encoding/json"
	"errors"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/shell"
	"net"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	isGkeModulePatterns        = []string{"gke-node-pool", "gke-cluster"}
	isSlurmModulePatterns      = []string{"schedmd-slurm-gcp-"}
	isVmInstanceModulePatterns = []string{"vm-instance"}
)

// NewCollector creates and initializes a new Telemetry Collector.
func NewCollector(cmd *cobra.Command, args []string, installationMode string) *Collector {
	return &Collector{
		eventCmd:         cmd,
		eventArgs:        args,
		eventStartTime:   time.Now(),
		blueprint:        getBlueprint(cmd, args),
		installationMode: installationMode,
		metadata:         make(map[string]string),
	}
}

// Main function for collecting Telemetry metrics.
func (c *Collector) CollectMetrics(errorCode int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	bpModulesList := getBpModulesList(c.blueprint)

	c.metadata[COMMAND_FLAGS] = getCmdFlags(c.eventCmd)
	c.metadata[BLUEPRINT] = getBlueprintName(c.blueprint)
	c.metadata[DEPLOYMENT_FILE] = getDeploymentFile(c.eventCmd)
	c.metadata[IS_GKE] = getIsGke(bpModulesList)
	c.metadata[IS_SLURM] = getIsSlurm(bpModulesList)
	c.metadata[IS_VM_INSTANCE] = getIsVmInstance(bpModulesList)
	c.metadata[MACHINE_TYPE] = getMachineType(c.blueprint)
	c.metadata[STORAGE_TYPE] = getStorageType(c.blueprint)
	c.metadata[REGION] = getRegion(c.blueprint)
	c.metadata[ZONE] = getZone(c.blueprint)
	c.metadata[MODULES] = getModules(bpModulesList)
	c.metadata[STATIC_NODE_COUNTS] = getStaticNodeCounts(c.blueprint)
	c.metadata[DYNAMIC_MIN_NODE_COUNTS] = getDynamicNodeCounts(c.blueprint, "min")
	c.metadata[DYNAMIC_MAX_NODE_COUNTS] = getDynamicNodeCounts(c.blueprint, "max")
	c.metadata[OS_NAME] = getOSName()
	c.metadata[OS_VERSION] = getOSVersion()
	c.metadata[TERRAFORM_VERSION] = getTerraformVersion()
	c.metadata[INSTALLATION_MODE] = c.installationMode
	c.metadata[IS_AI_ASSISTED] = strconv.FormatBool(c.blueprint.AIAssisted)
	c.metadata[IS_TEST_DATA] = getIsTestData()
	c.metadata[EXIT_CODE] = strconv.Itoa(errorCode)
	c.metadata[ERROR_TYPE] = getErrorType(err)
}

// Method to collect Concord metrics and build event.
func (c *Collector) BuildConcordEvent() ConcordEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	project_id := config.GetKeyFromBlueprint("project_id", c.blueprint)

	return ConcordEvent{
		ConsoleType:      CLUSTER_TOOLKIT,
		EventType:        "gclusterCLI",
		EventName:        getCommandName(c.eventCmd),
		EventMetadata:    getEventMetadataKVPairs(c.metadata),
		ProjectNumber:    getProjectNumber(project_id),
		ClientInstallId:  getClientInstallId(),
		BillingAccountId: getBillingAccountId(project_id),
		ReleaseVersion:   getReleaseVersion(),
		IsGoogler:        getIsGoogler(),
		LatencyMs:        getLatencyMs(c.eventStartTime),
	}
}

/** Private functions **/

func getClientInstallId() string {
	return config.GetPersistentUserId()
}

func getReleaseVersion() string {
	return config.GetToolkitVersion()
}

func getCommandName(cmd *cobra.Command) string {
	path := cmd.CommandPath() // Returns the full command path (e.g., "gcluster job cancel")

	if path == "" {
		return path
	} else {
		return strings.TrimPrefix(path, "gcluster ")
	}
}

func getCmdFlags(cmd *cobra.Command) string {
	numFlags := cmd.Flags().NFlag()
	if numFlags == 0 {
		return ""
	}
	flags := make([]string, 0, numFlags)
	cmd.Flags().Visit(func(f *pflag.Flag) {
		flags = append(flags, f.Name)
	})
	return strings.Join(flags, ",")
}

func getBlueprintName(bp config.Blueprint) string {
	bpName := bp.BlueprintName
	if bpName == "" {
		return bpName
	}

	standardBlueprints := config.GetStandardBlueprintNames()

	// If standardFiles is empty due to a fetch failure, the telemetry payload will correctly report "UNVERIFIED", rather than falsely implying no blueprint name was used.
	if len(standardBlueprints) == 0 {
		return "UNVERIFIED"
	}

	// Check if it matches a known blueprint name, otherwise mask as "Custom"
	if slices.Contains(standardBlueprints, bpName) {
		return bpName
	}

	return "Custom"
}

func getDeploymentFile(cmd *cobra.Command) string {
	flag := cmd.Flag("deployment-file")
	if flag == nil || flag.Value.String() == "" {
		return ""
	}

	path := flag.Value.String()

	// Force all backslashes to forward slashes first to ensure consistent cross-platform parsing
	path = strings.ReplaceAll(path, "\\", "/")
	// Clean the path, enforce forward slashes, and trim leading relative prefixes
	path = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./")

	standardFiles := config.GetPredefinedExampleFiles()

	// If standardFiles is empty due to a network fetch failure, the telemetry payload will correctly report "UNVERIFIED", rather than falsely implying no deployment file was used.
	if len(standardFiles) == 0 {
		return "UNVERIFIED"
	}

	// Check if it matches a known example, otherwise mask as "Custom"
	for _, sf := range standardFiles {
		if path == sf || strings.HasSuffix(path, "/"+sf) {
			return sf
		}
	}

	return "Custom"
}

func getIsGke(modulesList []string) string {
	return ifModulesMatchPatterns(modulesList, isGkeModulePatterns)
}

func getIsSlurm(modulesList []string) string {
	return ifModulesMatchPatterns(modulesList, isSlurmModulePatterns)
}

func getIsVmInstance(modulesList []string) string {
	return ifModulesMatchPatterns(modulesList, isVmInstanceModulePatterns)
}

func getProjectNumber(projectID string) string {
	if projectID == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	projectName, err := fetchProjectName(ctx, projectID)
	if err != nil || projectName == "" {
		return ""
	}

	return strings.TrimPrefix(projectName, "projects/")
}

func getMachineType(bp config.Blueprint) string {
	var machineTypes []string
	seen := make(map[string]bool) // To keep track of added machine types to avoid duplication

	for _, m := range config.GetAllBpModules(&bp) {
		var mType = getMachineTypeFromModule(m, bp)

		if mType != "" && !seen[mType] {
			machineTypes = append(machineTypes, mType)
			seen[mType] = true
		}
	}

	return strings.Join(machineTypes, ",")
}

func getStorageType(bp config.Blueprint) string {
	var storageTypes []string
	seen := make(map[string]bool)

	for _, m := range config.GetAllBpModules(&bp) {
		types := getStorageTypesFromModule(m, bp)
		for _, t := range types {
			if !seen[t] {
				storageTypes = append(storageTypes, t)
				seen[t] = true
			}
		}
	}

	slices.Sort(storageTypes)
	return strings.Join(storageTypes, ",")
}

func getRegion(bp config.Blueprint) string {
	return config.GetKeyFromBlueprint("region", bp)
}

func getZone(bp config.Blueprint) string {
	return config.GetKeyFromBlueprint("zone", bp)
}

// getModules returns a comma-separated string of sanitized module names.
// It checks each module in the provided list against the officially predefined standardModules as per the user's version.
// Standard modules are preserved, while any unrecognized module is replaced with "Custom" to protect user privacy and avoid exposing proprietary module paths.
func getModules(modulesList []string) string {
	if len(modulesList) == 0 {
		return ""
	}

	standardModules := config.GetPredefinedModules()

	// If standardModules is empty due to a network fetch failure, the telemetry payload will correctly report "UNVERIFIED", rather than falsely implying the blueprint had no modules.
	if len(standardModules) == 0 {
		return "UNVERIFIED"
	}

	sanitizedModules := make([]string, 0, len(modulesList))
	for _, m := range modulesList {
		if slices.Contains(standardModules, m) {
			sanitizedModules = append(sanitizedModules, m)
		} else {
			sanitizedModules = append(sanitizedModules, "Custom")
		}
	}

	return strings.Join(sanitizedModules, ",")
}

func getStaticNodeCounts(bp config.Blueprint) string {
	countsByMachineType := make(map[string]int)

	for _, m := range config.GetAllBpModules(&bp) {
		moduleCounts := getModuleNodeCounts(m, bp)
		for mType, count := range moduleCounts {
			if count > 0 {
				countsByMachineType[mType] += count
			}
		}
	}

	counts, err := json.Marshal(countsByMachineType)
	if err != nil || len(countsByMachineType) == 0 {
		return ""
	}

	// Trim the curly braces and remove the double quotes for a cleaner metric.
	// Expected return format: "g4-standard-48:3,a3-ultragpu-8g:2"
	return strings.ReplaceAll(strings.Trim(string(counts), "{}"), `"`, "")

}

func getDynamicNodeCounts(bp config.Blueprint, kind string) string {
	counts := make(map[string]int)
	targetKeys := dynamicMinNodeCountSettings
	if kind == "max" {
		targetKeys = dynamicMaxNodeCountSettings
	}

	for _, m := range config.GetAllBpModules(&bp) {
		moduleCounts := getModuleDynamicNodeCounts(m, bp, targetKeys)
		for mt, cnt := range moduleCounts {
			if cnt > 0 {
				counts[mt] += cnt
			}
		}
	}
	countsJSON, err := json.Marshal(counts)
	if err != nil || len(counts) == 0 {
		return ""
	}
	return strings.ReplaceAll(strings.Trim(string(countsJSON), "{}"), "\"", "")
}

func getOSName() string {
	return runtime.GOOS
}

// getOSVersion returns the OS version of the current system.
func getOSVersion() string {
	switch runtime.GOOS {
	case "linux":
		return getLinuxVersion()
	case "darwin":
		return getMacVersion()
	case "windows":
		return getWindowsVersion()
	default:
		return ""
	}
}

var tfVersionFunc = shell.TfVersion

// getTerraformVersion returns the version of the Terraform in use.
func getTerraformVersion() string {
	version, err := tfVersionFunc()
	if err != nil {
		return ""
	}
	return version
}

func getBillingAccountId(projectID string) string {
	if projectID == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	billingAccount, err := getProjectBillingAccount(ctx, projectID)
	if err != nil || billingAccount == "" {
		return ""
	}
	return strings.TrimPrefix(billingAccount, "billingAccounts/")
}

// getIsGoogler determines if the credentials belong to a Google internal user or an internal CI service account.
func getIsGoogler() bool {
	// Check if the value is already cached in the local config file
	if cached := config.GetIsGoogler(); cached != nil {
		return *cached
	}

	// Evaluate if the user is internal
	isInternal := evaluateIsGoogler()

	// Cache the evaluated result in the local config for future commands
	_ = config.SetIsGoogler(isInternal)

	return isInternal
}

// getErrorType maps an error to a broad, PII-safe category.
func getErrorType(err error) string {
	if err == nil {
		return ""
	}

	// Standard Go error checks
	for _, m := range exactErrMatchers {
		if errors.Is(err, m.target) {
			return m.category
		}
	}

	// Check for networking/timeout errors specifically
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrTypeTimeout
		}
		return ErrTypeNetwork
	}

	// Fallback string matching on safe keywords
	errMsg := strings.ToLower(err.Error())
	for _, m := range substringErrMatchers {
		if strings.Contains(errMsg, m.substring) {
			return m.category
		}
	}

	return ErrTypeUnknown
}

// This method intentionally returns "true", as all telemetry is in testing phase currently.
func getIsTestData() string {
	return "true" // do not modify
}

func getLatencyMs(eventStartTime time.Time) int64 {
	return time.Since(eventStartTime).Milliseconds()
}
