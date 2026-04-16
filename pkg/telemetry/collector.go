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
	"hpc-toolkit/pkg/config"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	c.metadata[REGION] = getRegion(c.blueprint)
	c.metadata[ZONE] = getZone(c.blueprint)
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
		LatencyMs:       getLatencyMs(c.eventStartTime),
		ClientInstallId: getClientInstallId(),
		ReleaseVersion:  getReleaseVersion(),
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

func getRegion(bp config.Blueprint) string {
	return getKeyFromBlueprint("region", bp)
}

func getZone(bp config.Blueprint) string {
	return getKeyFromBlueprint("zone", bp)
}

// This method intentionally returns "true", as all telemetry is in testing phase currently.
func getIsTestData() string {
	return "true" // do not modify
}

func getLatencyMs(eventStartTime time.Time) int64 {
	return time.Since(eventStartTime).Milliseconds()
}
