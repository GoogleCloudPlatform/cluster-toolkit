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

// The following implementation is done for sending one LogEvent per LogRequest as per the telemetry logic.

package telemetry

import (
	"encoding/json"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"time"

	"github.com/spf13/cobra"
)

func Initialize(cmd *cobra.Command, args []string) {
	CollectPreMetrics(cmd, args)
}

func Finalize(exitCode int) {
	CollectPostMetrics(exitCode)
	payload := ConstructPayload()
	Flush(payload)
}

func ConstructPayload() LogRequest {
	sourceExtensionJSON, err := json.Marshal(map[string]any{
		"event_type":      "GCluster CLI",
		"console_type":    CLUSTER_TOOLKIT,
		"release_version": config.GetToolkitVersion(),
		"event_metadata":  getEventMetadataKVPairs(),
	})
	if err != nil {
		logging.Error("Error collecting telemetry event metadata: %v", err)
		return LogRequest{}
	}

	logEvent := LogEvent{
		EventTimeMs:         time.Now().UnixMilli(),
		SourceExtensionJson: string(sourceExtensionJSON),
	}

	logRequest := LogRequest{
		RequestTimeMs: time.Now().UnixMilli(),
		ClientInfo:    ClientInfo{ClientType: CLUSTER_TOOLKIT},
		LogSourceName: CONCORD,
		LogEvent:      []LogEvent{logEvent},
	}
	return logRequest
}

func getEventMetadataKVPairs() []map[string]string {
	eventMetadata := make([]map[string]string, 0)
	for k, v := range metadata {
		eventMetadata = append(eventMetadata, map[string]string{
			"key":   k,
			"value": v,
		})
	}
	return eventMetadata
}
