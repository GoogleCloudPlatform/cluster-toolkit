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
	"time"

	"hpc-toolkit/pkg/config"
	"sync"

	"github.com/spf13/cobra"
)

const (
	ClearcutProdURL   = "https://play.googleapis.com/log"
	configDirName     = "cluster-toolkit"
	HttpServerTimeout = 10 * time.Second
	CLUSTER_TOOLKIT   = "CLUSTER_TOOLKIT"
	CONCORD           = "CONCORD"
)

// Collector encapsulates the telemetry state (avoids global variables).
type Collector struct {
	eventCmd       *cobra.Command
	eventArgs      []string
	eventStartTime time.Time
	blueprint      config.Blueprint
	metadata       map[string]string

	mu sync.Mutex // Protects state against concurrent access
}

type LogRequest struct {
	RequestTimeMs int64      `json:"request_time_ms"`
	ClientInfo    ClientInfo `json:"client_info"`
	LogSourceName string     `json:"log_source_name"`
	LogEvent      []LogEvent `json:"log_event"`
}

type LogEvent struct {
	EventTimeMs         int64  `json:"event_time_ms"`
	SourceExtensionJson string `json:"source_extension_json"` // ConcordEvent format.
}

type ConcordEvent struct {
	ConsoleType      string              `json:"console_type"`
	EventType        string              `json:"event_type"`
	EventName        string              `json:"event_name"`
	EventMetadata    []map[string]string `json:"event_metadata"`
	LatencyMs        int64               `json:"latency_ms"`
	ProjectNumber    string              `json:"project_number"`
	ClientInstallId  string              `json:"client_install_id"`
	BillingAccountId string              `json:"billing_account_id"`
	IsGoogler        bool                `json:"is_googler"`
	ReleaseVersion   string              `json:"release_version"`
}

type ClientInfo struct {
	ClientType string `json:"client_type"`
}

const (
	COMMAND_FLAGS = "CLUSTER_TOOLKIT_COMMAND_FLAGS"
	REGION        = "CLUSTER_TOOLKIT_REGION"
	ZONE          = "CLUSTER_TOOLKIT_ZONE"
	IS_TEST_DATA  = "CLUSTER_TOOLKIT_IS_TEST_DATA"
	EXIT_CODE     = "CLUSTER_TOOLKIT_EXIT_CODE"
)
