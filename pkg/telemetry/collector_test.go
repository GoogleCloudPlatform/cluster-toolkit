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
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestNewCollector(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	// Passing nil for args prevents getBlueprint from attempting to read a file
	c := NewCollector(cmd, nil)

	if c == nil {
		t.Fatal("Expected NewCollector to return a valid Collector, got nil")
	}
	if c.eventCmd != cmd {
		t.Errorf("Expected eventCmd to be %v, got %v", cmd, c.eventCmd)
	}
	if c.metadata == nil {
		t.Error("Expected metadata map to be initialized")
	}
}

// TestCollectMetrics_Extensible uses a table-driven approach.
// Future metrics can be seamlessly verified by adding keys to `expectedKeys`
// and values to `expectedValues`.
func TestCollectMetrics_Extensible(t *testing.T) {
	tests := []struct {
		name           string
		errorCode      int
		expectedKeys   []string
		expectedValues map[string]string
	}{
		{
			name:         "Success exit code",
			errorCode:    0,
			expectedKeys: []string{IS_TEST_DATA, EXIT_CODE},
			expectedValues: map[string]string{
				IS_TEST_DATA: "true",
				EXIT_CODE:    "0",
			},
		},
		{
			name:         "Failure exit code",
			errorCode:    1,
			expectedKeys: []string{IS_TEST_DATA, EXIT_CODE},
			expectedValues: map[string]string{
				IS_TEST_DATA: "true",
				EXIT_CODE:    "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCollector(nil, nil)
			c.CollectMetrics(tt.errorCode)

			for _, key := range tt.expectedKeys {
				val, exists := c.metadata[key]
				if !exists {
					t.Errorf("CollectMetrics() missing expected metric key: %s", key)
				}
				if expectedVal, ok := tt.expectedValues[key]; ok && val != expectedVal {
					t.Errorf("CollectMetrics() metric %s = %v, want %v", key, val, expectedVal)
				}
			}
		})
	}
}

func TestBuildConcordEvent(t *testing.T) {
	rootCmd := &cobra.Command{Use: "gcluster"}
	childCmd := &cobra.Command{Use: "deploy"}
	rootCmd.AddCommand(childCmd)

	c := NewCollector(childCmd, nil)
	c.CollectMetrics(0)

	event := c.BuildConcordEvent()

	if event.ConsoleType != CLUSTER_TOOLKIT {
		t.Errorf("BuildConcordEvent() ConsoleType = %v, want %v", event.ConsoleType, CLUSTER_TOOLKIT)
	}
	if event.EventType != "gclusterCLI" {
		t.Errorf("BuildConcordEvent() EventType = %v, want gclusterCLI", event.EventType)
	}
	if event.EventName != "deploy" {
		t.Errorf("BuildConcordEvent() EventName = %v, want deploy", event.EventName)
	}
	if event.LatencyMs < 0 {
		t.Errorf("BuildConcordEvent() LatencyMs = %v, want >= 0", event.LatencyMs)
	}
	if event.ReleaseVersion == "" {
		t.Error("BuildConcordEvent() ReleaseVersion is empty")
	}

	// Verify metadata KV pairs mapping
	foundExitCode := false
	for _, meta := range event.EventMetadata {
		if meta["key"] == EXIT_CODE && meta["value"] == "0" {
			foundExitCode = true
			break
		}
	}

	if !foundExitCode {
		t.Errorf("BuildConcordEvent() EventMetadata did not properly translate metadata key-value pairs")
	}
}

func TestGetCommandName(t *testing.T) {
	tests := []struct {
		name     string
		cmdSetup func() *cobra.Command
		want     string
	}{
		{
			name: "Empty path",
			cmdSetup: func() *cobra.Command {
				return &cobra.Command{}
			},
			want: "",
		},
		{
			name: "Root command",
			cmdSetup: func() *cobra.Command {
				return &cobra.Command{Use: "gcluster"}
			},
			want: "gcluster",
		},
		{
			name: "Subcommand",
			cmdSetup: func() *cobra.Command {
				root := &cobra.Command{Use: "gcluster"}
				sub := &cobra.Command{Use: "job"}
				root.AddCommand(sub)
				return sub
			},
			want: "job",
		},
		{
			name: "Nested subcommand",
			cmdSetup: func() *cobra.Command {
				root := &cobra.Command{Use: "gcluster"}
				sub := &cobra.Command{Use: "job"}
				subsub := &cobra.Command{Use: "cancel"}
				root.AddCommand(sub)
				sub.AddCommand(subsub)
				return subsub
			},
			want: "job cancel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmdSetup()
			if got := getCommandName(cmd); got != tt.want {
				t.Errorf("getCommandName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetReleaseVersion(t *testing.T) {
	if got := getReleaseVersion(); got == "" {
		t.Errorf("getReleaseVersion() returned an empty string, expected toolkit version")
	}
}

func TestGetIsTestData(t *testing.T) {
	if got := getIsTestData(); got != "true" {
		t.Errorf("getIsTestData() = %v, want true", got)
	}
}

func TestGetLatencyMs(t *testing.T) {
	// Set the start time to 50ms in the past to test positive latency
	eventStartTime := time.Now().Add(-50 * time.Millisecond)
	latency := getLatencyMs(eventStartTime)

	if latency < 50 {
		t.Errorf("getLatencyMs() expected >= 50ms, got %d ms", latency)
	}
}

func TestGetClientInstallId(t *testing.T) {
	// Ensure Viper is reset after all tests to prevent config leakage
	defer viper.Reset()

	tests := []struct {
		name       string
		mockConfig func()
		want       string
	}{
		{
			name: "returns valid client install id when set in config",
			mockConfig: func() {
				// Mocks the case where the CLI has already bootstrapped the config
				// and saved a hashed persistent user_id.
				viper.Set("user_id", "a1b2c3d4e5f6")
			},
			want: "a1b2c3d4e5f6",
		},
		{
			name: "returns empty string when client install id is missing",
			mockConfig: func() {
				// Mocks the case where the config is missing or uninitialized.
				viper.Set("user_id", "")
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset Viper state before each sub-test to ensure isolation
			viper.Reset()

			// Apply the mocked configuration for this specific test case
			tt.mockConfig()

			// Act
			got := getClientInstallId()

			// Assert
			if got != tt.want {
				t.Errorf("getClientInstallId() = %q, want %q", got, tt.want)
			}
		})
	}
}
