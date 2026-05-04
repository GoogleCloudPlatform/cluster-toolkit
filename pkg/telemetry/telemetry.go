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

// The following implementation is done for sending one LogEvent per LogRequest as required by the telemetry logic.

package telemetry

import (
	"encoding/json"
	"hpc-toolkit/pkg/logging"
	"time"
)

func (c *Collector) Execute(exitCode int, installationMode string) {
	c.CollectMetrics(exitCode, installationMode)
	concordEvent := c.BuildConcordEvent()
	payload := BuildPayload(concordEvent)
	Flush(payload)
}

func BuildPayload(concordEvent ConcordEvent) LogRequest {
	sourceExtensionJSON, err := json.Marshal(concordEvent)
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
