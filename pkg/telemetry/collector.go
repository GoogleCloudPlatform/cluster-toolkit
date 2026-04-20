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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/shell"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zclconf/go-cty/cty"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	machineTypeModulePattern = ".*modules.compute.*"
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

	c.metadata[COMMAND_FLAGS] = getCmdFlags(c.eventCmd)
	c.metadata[MACHINE_TYPE] = getMachineType(c.blueprint)
	c.metadata[REGION] = getRegion(c.blueprint)
	c.metadata[ZONE] = getZone(c.blueprint)
	c.metadata[OS_NAME] = getOSName()
	c.metadata[OS_VERSION] = getOSVersion()
	c.metadata[TERRAFORM_VERSION] = getTerraformVersion()
	c.metadata[IS_TEST_DATA] = getIsTestData()
	c.metadata[EXIT_CODE] = strconv.Itoa(errorCode)
}

// Method to collect Concord metrics and build event.
func (c *Collector) BuildConcordEvent() ConcordEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	return ConcordEvent{
		ConsoleType:     CLUSTER_TOOLKIT,
		EventType:       "gclusterCLI",
		EventName:       getCommandName(c.eventCmd),
		EventMetadata:   getEventMetadataKVPairs(c.metadata),
		ProjectNumber:   getProjectNumber(c.blueprint),
		ClientInstallId: getClientInstallId(),
		ReleaseVersion:  getReleaseVersion(),
		LatencyMs:       getLatencyMs(c.eventStartTime),
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

// func getProjectNumber(bp config.Blueprint) string {
// 	ctx, cancel := context.WithTimeout(context.Background(), timeout10Sec)
// 	defer cancel()

// 	projectID := getKeyFromBlueprint("project_id", bp)
// 	client, err := resourcemanager.NewProjectsClient(ctx)
// 	if err != nil || projectID == "" {
// 		return ""
// 	}
// 	defer client.Close()

// 	req := &resourcemanagerpb.GetProjectRequest{
// 		Name: fmt.Sprintf("projects/%s", projectID),
// 	}
// 	project, err := client.GetProject(ctx, req)

// 	if err != nil || project == nil || project.Name == "" {
// 		return ""
// 	} else {
// 		return strings.TrimPrefix(project.Name, "projects/")
// 	}
// }

// --- Interface to abstract the Projects Client ---
type ProjectsClientInterface interface {
	GetProject(ctx context.Context, req *GetProjectRequest) (*Project, error)
	Close() error
}

type GetProjectRequest struct {
	Name string
}

type Project struct {
	Name string
}

var NewProjectsClient = func(ctx context.Context) (ProjectsClientInterface, error) {
	return nil, fmt.Errorf("NewProjectsClient not implemented")
}

func getProjectNumber(bp config.Blueprint) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout10Sec)
	defer cancel()

	projectID := getKeyFromBlueprint("project_id", bp)
	client, err := NewProjectsClient(ctx)
	if err != nil || projectID == "" {
		return ""
	}
	defer client.Close()

	req := &GetProjectRequest{
		Name: fmt.Sprintf("projects/%s", projectID),
	}

	project, err := client.GetProject(ctx, req)
	if err != nil || project == nil || project.Name == "" {
		return ""
	}
	return strings.TrimPrefix(project.Name, "projects/")
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

// This method intentionally returns "true", as all telemetry is in testing phase currently.
func getIsTestData() string {
	return "true" // do not modify
}

func getLatencyMs(eventStartTime time.Time) int64 {
	return time.Since(eventStartTime).Milliseconds()
}
