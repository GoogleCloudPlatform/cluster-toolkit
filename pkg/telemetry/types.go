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

import "time"

var (
	ClearcutProdURL   = "https://play.googleapis.com/log"
	HttpServerTimeout = 10 * time.Second
)

type ClientInfo struct {
	ClientType string `json:"client_type"`
}

type LogEvent struct {
	EventTimeMs         int64  `json:"event_time_ms"`
	SourceExtensionJson string `json:"source_extension_json"` // Contains event metadata as key-value pairs.
}

type LogRequest struct {
	RequestTimeMs int64      `json:"request_time_ms"`
	ClientInfo    ClientInfo `json:"client_info"`
	LogSourceName string     `json:"log_source_name"`
	LogEvent      []LogEvent `json:"log_event"`
}

const (
	CLUSTER_TOOLKIT string = "CLUSTER_TOOLKIT"
	CONCORD         string = "CONCORD"
)

// CTK Metrics being collected
const (
	COMMAND_NAME = "CLUSTER_TOOLKIT_COMMAND_NAME"
	IS_TEST_DATA = "CLUSTER_TOOLKIT_IS_TEST_DATA"
	RUNTIME_MS   = "CLUSTER_TOOLKIT_RUNTIME_MS"
	EXIT_CODE    = "CLUSTER_TOOLKIT_EXIT_CODE"
)
