// Copyright 2026 Google LLC
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

package cmd

import (
	"testing"

	"hpc-toolkit/pkg/config"
)

func TestTelemetryCmd_Logic(t *testing.T) {
	// Isolate the config directory for tests to prevent modifying the real user config file
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("APPDATA", tempDir) // For Windows compatibility
	t.Setenv("HOME", tempDir)    // For macOS/Linux compatibility

	// Initialize the user config to set defaults within the temporary directory
	err := config.InitUserConfig()
	if err != nil {
		t.Fatalf("Failed to initialize user config for tests: %v", err)
	}

	tests := []struct {
		name        string
		args        []string
		expectError bool
		expected    bool
	}{
		{
			name:        "Enable using 'on'",
			args:        []string{"on"},
			expectError: false,
			expected:    true,
		},
		{
			name:        "Enable using 'TRUE' (case-insensitive)",
			args:        []string{"TRUE"},
			expectError: false,
			expected:    true,
		},
		{
			name:        "Enable using 'enable'",
			args:        []string{"enable"},
			expectError: false,
			expected:    true,
		},
		{
			name:        "Disable using 'off'",
			args:        []string{"off"},
			expectError: false,
			expected:    false,
		},
		{
			name:        "Disable using 'False' (case-insensitive)",
			args:        []string{"False"},
			expectError: false,
			expected:    false,
		},
		{
			name:        "Disable using 'disable'",
			args:        []string{"disable"},
			expectError: false,
			expected:    false,
		},
		{
			name:        "Invalid argument",
			args:        []string{"invalid"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call RunE directly to bypass Cobra's global execution state in loops
			err := telemetryCmd.RunE(telemetryCmd, tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Verify the configuration was actually updated as expected
				if config.IsTelemetryEnabled() != tt.expected {
					t.Errorf("Expected telemetry enabled to be %v, got %v", tt.expected, config.IsTelemetryEnabled())
				}
			}
		})
	}
}

func TestTelemetryCmd_ArgsValidation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "Valid args",
			args:        []string{"on"},
			expectError: false,
		},
		{
			name:        "No arguments",
			args:        []string{},
			expectError: true,
		},
		{
			name:        "Too many arguments",
			args:        []string{"on", "off"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Directly test Cobra's ExactArgs(1) validation configured on the command
			err := telemetryCmd.Args(telemetryCmd, tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
