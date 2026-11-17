// Copyright 2026 "Google LLC"
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

package telemetry

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/shell"

	"github.com/spf13/cobra"
)

func TestBuildPayload(t *testing.T) {
	// Create a dummy ConcordEvent
	event := ConcordEvent{
		ConsoleType: CLUSTER_TOOLKIT,
		EventType:   "gclusterCLI",
		EventName:   "deploy",
		LatencyMs:   100,
	}

	// Capture time bounds to verify the assigned timestamps
	before := time.Now().UnixMilli()
	payload := BuildPayload(event)
	after := time.Now().UnixMilli()

	// Verify top-level payload constants
	if payload.LogSourceName != CONCORD {
		t.Errorf("Expected LogSourceName %s, got %s", CONCORD, payload.LogSourceName)
	}
	if payload.ClientInfo.ClientType != CLUSTER_TOOLKIT {
		t.Errorf("Expected ClientType %s, got %s", CLUSTER_TOOLKIT, payload.ClientInfo.ClientType)
	}

	// Verify request time is within bounds
	if payload.RequestTimeMs < before || payload.RequestTimeMs > after {
		t.Errorf("RequestTimeMs %d is out of bounds [%d, %d]", payload.RequestTimeMs, before, after)
	}

	// Ensure there is exactly 1 log event as designed
	if len(payload.LogEvent) != 1 {
		t.Fatalf("Expected 1 LogEvent, got %d", len(payload.LogEvent))
	}

	logEvent := payload.LogEvent[0]
	if logEvent.EventTimeMs < before || logEvent.EventTimeMs > after {
		t.Errorf("EventTimeMs %d is out of bounds [%d, %d]", logEvent.EventTimeMs, before, after)
	}

	// Unmarshal the SourceExtensionJson back to a ConcordEvent to ensure integrity
	var unmarshaledEvent ConcordEvent
	err := json.Unmarshal([]byte(logEvent.SourceExtensionJson), &unmarshaledEvent)
	if err != nil {
		t.Fatalf("Failed to unmarshal SourceExtensionJson: %v", err)
	}

	if unmarshaledEvent.EventName != event.EventName {
		t.Errorf("Expected EventName %s, got %s", event.EventName, unmarshaledEvent.EventName)
	}
	if unmarshaledEvent.ConsoleType != event.ConsoleType {
		t.Errorf("Expected ConsoleType %s, got %s", event.ConsoleType, unmarshaledEvent.ConsoleType)
	}
}

func isolateTelemetryTestEnv(t *testing.T) (httpCalls, tfCalls *atomic.Int32) {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_CACHE_HOME", tempDir)
	t.Setenv("HOME", tempDir)
	t.Setenv("AppData", tempDir)
	t.Setenv("LocalAppData", tempDir)
	t.Setenv("CLOUDSDK_CONFIG", filepath.Join(tempDir, "gcloud"))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv(telemetryEndpointEnvVar, "")
	t.Setenv(shell.AirGappedEnvVar, "")
	shell.SetDefaultAirGapped(false)
	if err := config.InitUserConfig(); err != nil {
		t.Fatalf("failed to initialize isolated user config: %v", err)
	}
	origTransport := http.DefaultTransport
	origTfVersion := tfVersionFunc
	t.Cleanup(func() {
		http.DefaultTransport = origTransport
		tfVersionFunc = origTfVersion
		shell.SetDefaultAirGapped(false)
	})

	var httpCount, tfCount atomic.Int32
	tfVersionFunc = func() (string, error) {
		tfCount.Add(1)
		return "1.5.0", nil
	}
	http.DefaultTransport = &mockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			httpCount.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		},
	}
	return &httpCount, &tfCount
}

func TestExecute(t *testing.T) {
	calls, tfCalls := isolateTelemetryTestEnv(t)

	cmd := &cobra.Command{Use: "test-execute"}
	c := NewCollector(cmd, nil, SOURCE)
	c.Execute(0, nil)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 Clearcut HTTP call when not air-gapped, got %d", got)
	}
	if got := tfCalls.Load(); got != 1 {
		t.Fatalf("expected CollectMetrics to invoke tfVersionFunc once when not air-gapped, got %d", got)
	}
}

func TestExecute_AirGappedSkipsClearcut(t *testing.T) {
	calls, tfCalls := isolateTelemetryTestEnv(t)
	cmd := &cobra.Command{Use: "test-airgapped"}

	var nilCollector *Collector
	nilCollector.Execute(0, nil)
	if calls.Load() != 0 || tfCalls.Load() != 0 {
		t.Fatalf("expected nil collector Execute to be a no-op, got http=%d tf=%d", calls.Load(), tfCalls.Load())
	}

	t.Setenv(shell.AirGappedEnvVar, "true")
	shell.SetDefaultAirGapped(false)
	NewCollector(cmd, nil, SOURCE).Execute(0, nil)
	if calls.Load() != 0 || tfCalls.Load() != 0 || config.GetIsGoogler() != nil {
		t.Fatalf("expected 0 HTTP, 0 CollectMetrics, and nil IsGoogler when GCLUSTER_AIR_GAPPED=true, got http=%d tf=%d isGoogler=%v", calls.Load(), tfCalls.Load(), config.GetIsGoogler())
	}

	t.Setenv(shell.AirGappedEnvVar, "")
	shell.SetDefaultAirGapped(true)
	NewCollector(cmd, nil, SOURCE).Execute(0, nil)
	if calls.Load() != 0 || tfCalls.Load() != 0 || config.GetIsGoogler() != nil {
		t.Fatalf("expected 0 HTTP, 0 CollectMetrics, and nil IsGoogler when SetDefaultAirGapped(true) is active, got http=%d tf=%d isGoogler=%v", calls.Load(), tfCalls.Load(), config.GetIsGoogler())
	}

	t.Setenv(shell.AirGappedEnvVar, "false")
	NewCollector(cmd, nil, SOURCE).Execute(0, nil)
	if calls.Load() != 1 || tfCalls.Load() != 1 || config.GetIsGoogler() == nil {
		t.Fatalf("expected 1 HTTP, 1 CollectMetrics, and populated IsGoogler when GCLUSTER_AIR_GAPPED=false overrides default, got http=%d tf=%d isGoogler=%v", calls.Load(), tfCalls.Load(), config.GetIsGoogler())
	}
}
