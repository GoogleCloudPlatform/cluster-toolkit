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
	"hpc-toolkit/pkg/config"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zclconf/go-cty/cty"
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
	// Define all expected metric keys from types.go
	expectedKeys := []string{
		COMMAND_FLAGS,
		REGION,
		ZONE,
		IS_TEST_DATA,
		EXIT_CODE,
	}

	tests := []struct {
		name           string
		errorCode      int
		setupCmd       func(cmd *cobra.Command) // Hook to configure the command
		setupCollector func(c *Collector)       // Hook to mock internal collector state
		expectedValues map[string]string
	}{
		{
			name:      "Success exit code",
			errorCode: 0,
			setupCmd: func(cmd *cobra.Command) {
				// Define dummy flags for the mock command
				cmd.Flags().Bool("force", false, "Force execution")
				cmd.Flags().String("project", "", "GCP Project")

				// Simulate the user providing these flags at runtime
				_ = cmd.Flags().Set("force", "true")
				_ = cmd.Flags().Set("project", "test-project")
			},
			setupCollector: func(c *Collector) {
				// Mock the blueprint variables to include region and zone
				c.blueprint = config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"region": cty.StringVal("us-central1"),
						"zone":   cty.StringVal("us-central1-a"),
					}),
				}
			},
			expectedValues: map[string]string{
				COMMAND_FLAGS: "force,project",
				IS_TEST_DATA:  "true",
				EXIT_CODE:     "0",
				REGION:        "us-central1",
				ZONE:          "us-central1-a",
			},
		},
		{
			name:      "Failure exit code with missing region and zone",
			errorCode: 1, // Simulating a failure
			setupCmd: func(cmd *cobra.Command) {
				// No flags set for this test case
			},
			setupCollector: func(c *Collector) {
				// Blueprint with empty vars to simulate missing region and zone
				c.blueprint = config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{}),
				}
			},
			expectedValues: map[string]string{
				IS_TEST_DATA:  "true",
				EXIT_CODE:     "1", // Verify failure code is captured
				COMMAND_FLAGS: "",  // Verify empty flags
				REGION:        "",  // Verify missing region
				ZONE:          "",  // Verify missing zone
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "mock"}
			// Execute the setup function to apply flags to the command
			if tt.setupCmd != nil {
				tt.setupCmd(cmd)
			}

			// Initialize the collector
			c := NewCollector(cmd, []string{})

			// Execute the setup function to apply the blueprint state to the collector
			if tt.setupCollector != nil {
				tt.setupCollector(c)
			}

			// Run the method being tested
			c.CollectMetrics(tt.errorCode)

			// Assert that all expected keys are populated in the metadata
			for _, key := range expectedKeys {
				if _, exists := c.metadata[key]; !exists {
					t.Errorf("Expected key %q missing from metadata", key)
				}
			}

			// Assert that the specifically expected values match
			for key, expectedVal := range tt.expectedValues {
				if actualVal, exists := c.metadata[key]; !exists || actualVal != expectedVal {
					t.Errorf("For key %q, expected value %q, got %q", key, expectedVal, actualVal)
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

func TestGetCmdFlags(t *testing.T) {
	tests := []struct {
		name     string
		setupCmd func(cmd *cobra.Command)
		expected string
	}{
		{
			name: "No flags set",
			setupCmd: func(cmd *cobra.Command) {
				// Define a flag but do not set it
				cmd.Flags().Bool("force", false, "Force execution")
			},
			expected: "",
		},
		{
			name: "Single flag set",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().Bool("force", false, "Force execution")

				// Simulate user passing --force
				_ = cmd.Flags().Set("force", "true")
			},
			expected: "force",
		},
		{
			name: "Multiple flags set",
			setupCmd: func(cmd *cobra.Command) {
				cmd.Flags().Bool("force", false, "Force execution")
				cmd.Flags().String("project", "", "GCP Project")
				cmd.Flags().Int("retries", 3, "Number of retries")

				// Simulate user passing --force and --project
				_ = cmd.Flags().Set("force", "true")
				_ = cmd.Flags().Set("project", "test-project")
				// Leave "retries" unset to ensure it isn't collected
			},
			// pflag typically stores flags in alphabetical order, but adjust if your function sorts them differently
			expected: "force,project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "mock"}

			if tt.setupCmd != nil {
				tt.setupCmd(cmd)
			}

			actual := getCmdFlags(cmd)

			if actual != tt.expected {
				t.Errorf("getCmdFlags() = %q, want %q", actual, tt.expected)
			}
		})
	}
}

// TestGetKeyFromBlueprint verifies that the keys are correctly extracted from the blueprint.
func TestGetKeyFromBlueprint(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		setupBp  func() config.Blueprint
		expected string
	}{
		{
			name: "Valid region",
			key:  "region",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"region": cty.StringVal("us-central1"),
					}),
				}
			},
			expected: "us-central1",
		},
		{
			name: "Valid zone",
			key:  "zone",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{
						"zone": cty.StringVal("us-central1-a"),
					}),
				}
			},
			expected: "us-central1-a",
		},
		{
			name: "Missing key",
			key:  "zone",
			setupBp: func() config.Blueprint {
				return config.Blueprint{
					Vars: config.NewDict(map[string]cty.Value{}),
				}
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := tt.setupBp()
			actual := getKeyFromBlueprint(tt.key, bp)

			if actual != tt.expected {
				t.Errorf("getKeyFromBlueprint(%q) = %q, want %q", tt.key, actual, tt.expected)
			}
		})
	}
}
