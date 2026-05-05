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
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// telemetryMockTransport implements http.RoundTripper to intercept HTTP calls
// triggered by the Flush() function inside Execute()
type telemetryMockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *telemetryMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

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

func TestExecute(t *testing.T) {
	// Save the original transport and restore it after tests run
	// to prevent Flush() from triggering real external network requests
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	http.DefaultTransport = &telemetryMockTransport{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			// Mock a successful 200 OK response from Clearcut
			return &http.Response{
				StatusCode: http.StatusOK,
			}, nil
		},
	}

	// Setup dummy cobra command and collector
	cmd := &cobra.Command{Use: "test-execute"}
	c := NewCollector(cmd, nil, SOURCE)

	// Trigger Execute. Since Execute doesn't return anything, we verify
	// it functions without panicking or failing.
	c.Execute(0)
}
