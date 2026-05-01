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
	"bytes"
	"context"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/shell"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zclconf/go-cty/cty"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	machineTypeModulePattern   = "modules.compute" // pattern for compute modules that set the machine.
	isGkeModulePatterns        = []string{"gke-node-pool", "gke-cluster"}
	isSlurmModulePatterns      = []string{"schedmd-slurm-gcp-"}
	isVmInstanceModulePatterns = []string{"vm-instance"}

	standardModules = config.FetchStandardModules(config.GetToolkitVersion())
)

// NewCollector creates and initializes a new Telemetry Collector.
func NewCollector(cmd *cobra.Command, args []string) *Collector {
	return &Collector{
		eventCmd:       cmd,
		eventArgs:      args,
		eventStartTime: time.Now(),
		blueprint:      getBlueprint(args),
		metadata:       make(map[string]string),
	}
}

// Main function for collecting Telemetry metrics.
func (c *Collector) CollectMetrics(errorCode int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	bpModulesList := getBpModulesList(c.blueprint)

	c.metadata[COMMAND_FLAGS] = getCmdFlags(c.eventCmd)
	c.metadata[IS_GKE] = getIsGke(bpModulesList)
	c.metadata[IS_SLURM] = getIsSlurm(bpModulesList)
	c.metadata[IS_VM_INSTANCE] = getIsVmInstance(bpModulesList)
	c.metadata[MACHINE_TYPE] = getMachineType(c.blueprint)
	c.metadata[REGION] = getRegion(c.blueprint)
	c.metadata[ZONE] = getZone(c.blueprint)
	c.metadata[MODULES] = getModules(bpModulesList)
	c.metadata[OS_NAME] = getOSName()
	c.metadata[OS_VERSION] = getOSVersion()
	c.metadata[TERRAFORM_VERSION] = getTerraformVersion()
	c.metadata[BILLING_ACCOUNT_ID] = getBillingAccountId(c.blueprint)
	c.metadata[IS_TEST_DATA] = getIsTestData()
	c.metadata[EXIT_CODE] = strconv.Itoa(errorCode)
}

// Method to collect Concord metrics and build event.
func (c *Collector) BuildConcordEvent() ConcordEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	return ConcordEvent{
		ConsoleType:      CLUSTER_TOOLKIT,
		EventType:        "gclusterCLI",
		EventName:        getCommandName(c.eventCmd),
		EventMetadata:    getEventMetadataKVPairs(c.metadata),
		ProjectNumber:    getProjectNumber(c.blueprint),
		ClientInstallId:  getClientInstallId(),
		BillingAccountId: c.metadata[BILLING_ACCOUNT_ID],
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

func getIsGke(modulesList []string) string {
	return ifModulesMatchPatterns(modulesList, isGkeModulePatterns)
}

func getIsSlurm(modulesList []string) string {
	return ifModulesMatchPatterns(modulesList, isSlurmModulePatterns)
}

func getIsVmInstance(modulesList []string) string {
	return ifModulesMatchPatterns(modulesList, isVmInstanceModulePatterns)
}

func getProjectNumber(bp config.Blueprint) string {
	projectID := getKeyFromBlueprint("project_id", bp)
	if projectID == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout10Sec)
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
	modules := getModulesWithPattern(machineTypeModulePattern, bp)

	evalAndAdd := func(key string, m config.Module) {
		if m.Settings.Has(key) {
			keyValue := m.Settings.Get(key)
			// Evaluate the value to resolve expressions like $(vars.key)
			evaluatedKey, err := bp.Eval(keyValue)
			if err != nil {
				return
			}
			// Some module outputs or references carry cty marks, so we unmark them safely before use.
			unmarkedKey, _ := evaluatedKey.Unmark()
			if !unmarkedKey.IsNull() && unmarkedKey.Type() == cty.String {
				mType := unmarkedKey.AsString()
				if !seen[mType] {
					machineTypes = append(machineTypes, mType)
					seen[mType] = true
				}
			}
		}
	}

	for _, m := range modules {
		evalAndAdd("machine_type", m)
		evalAndAdd("node_type", m) // For schedmd-slurm-gcp-v6-nodeset-tpu module. It uses node_type setting instead of machine_type.
	}
	return strings.Join(machineTypes, ",")
}

func getRegion(bp config.Blueprint) string {
	return getKeyFromBlueprint("region", bp)
}

func getZone(bp config.Blueprint) string {
	return getKeyFromBlueprint("zone", bp)
}

// getModules returns a comma-separated string of sanitized module names.
// It checks each module in the provided list against the officially predefined standardModules as per the user's version.
// Standard modules are preserved, while any unrecognized module is replaced with "Custom" to protect user privacy and avoid exposing proprietary module paths.
func getModules(modulesList []string) string {
	// If the blueprint has no modules, return empty string
	if len(modulesList) == 0 {
		return ""
	}

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

func getBillingAccountId(bp config.Blueprint) string {
	projectID := getKeyFromBlueprint("project_id", bp)
	if projectID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		billingAccount := getProjectBillingAccount(ctx, projectID)
		if billingAccount != "" {
			return strings.TrimPrefix(billingAccount, "billingAccounts/")
		}
	}
	return ""
}

// getIsGoogler determines if the credentials belong to a Google internal user or an internal CI service account.
func getIsGoogler() bool {
	// Check Application Default Credentials (ADC) for Service Accounts.
	// CI pipelines usually inject credentials via this environment variable.
	adcPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if adcPath != "" {
		isInternal, err := checkADCForInternalUser(adcPath)
		if err == nil && isInternal {
			return true
		}
	}

	// Fall back to checking the active gcloud authenticated account.
	ctx, cancel := context.WithTimeout(context.Background(), timeout2Sec)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gcloud", "config", "get-value", "core/account")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err == nil && stdout.Len() > 0 {
		email := strings.TrimSpace(stdout.String())
		if isInternalEmail(email) {
			return true
		}
	}
	return false
}

// This method intentionally returns "true", as all telemetry is in testing phase currently.
func getIsTestData() string {
	return "true" // do not modify
}

func getLatencyMs(eventStartTime time.Time) int64 {
	return time.Since(eventStartTime).Milliseconds()
}
