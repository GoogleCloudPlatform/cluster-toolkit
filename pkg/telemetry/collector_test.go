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
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func resetGlobalState() {
	eventStartTime = time.Time{}
	metadata = make(map[string]string)
}

func TestCollectPreMetrics(t *testing.T) {
	tests := []struct {
		name        string
		cmd         *cobra.Command
		args        []string
		wantCmdName string
		wantIsTest  string
	}{
		{
			name: "standard command",
			cmd: &cobra.Command{
				Use: "apply", // cobra.Command.Name() derives from the first word of Use
			},
			args:        []string{},
			wantCmdName: "apply",
			wantIsTest:  "true",
		},
		{
			name: "command with flags in use string",
			cmd: &cobra.Command{
				Use: "destroy [flags]",
			},
			args:        []string{"--force"},
			wantCmdName: "destroy",
			wantIsTest:  "true",
		},
		{
			name:        "empty command",
			cmd:         &cobra.Command{},
			args:        []string{},
			wantCmdName: "",
			wantIsTest:  "true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetGlobalState()

			// Capture bounding times to verify eventStartTime is correctly set to time.Now()
			before := time.Now()
			CollectPreMetrics(tc.cmd, tc.args)
			after := time.Now()

			if eventStartTime.Before(before) || eventStartTime.After(after) {
				t.Errorf("eventStartTime = %v, want between %v and %v", eventStartTime, before, after)
			}

			if got := metadata[COMMAND_NAME]; got != tc.wantCmdName {
				t.Errorf("metadata[%q] = %q, want %q", COMMAND_NAME, got, tc.wantCmdName)
			}

			if got := metadata[IS_TEST_DATA]; got != tc.wantIsTest {
				t.Errorf("metadata[%q] = %q, want %q", IS_TEST_DATA, got, tc.wantIsTest)
			}
		})
	}
}

func TestCollectPostMetrics(t *testing.T) {
	tests := []struct {
		name         string
		errorCode    int
		wantExitCode string
	}{
		{
			name:         "success execution",
			errorCode:    0,
			wantExitCode: "0",
		},
		{
			name:         "standard error",
			errorCode:    1,
			wantExitCode: "1",
		},
		{
			name:         "custom error code",
			errorCode:    127,
			wantExitCode: "127",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetGlobalState()

			// Set start time to a known duration in the past to simulate runtime
			eventStartTime = time.Now().Add(-50 * time.Millisecond)

			CollectPostMetrics(tc.errorCode)

			if got := metadata[EXIT_CODE]; got != tc.wantExitCode {
				t.Errorf("metadata[%q] = %q, want %q", EXIT_CODE, got, tc.wantExitCode)
			}

			runtimeMsStr, ok := metadata[RUNTIME_MS]
			if !ok {
				t.Fatalf("metadata[%q] missing, want populated", RUNTIME_MS)
			}

			runtimeMs, err := strconv.ParseInt(runtimeMsStr, 10, 64)
			if err != nil {
				t.Fatalf("failed to parse RUNTIME_MS %q: %v", runtimeMsStr, err)
			}

			// Validating that the calculated runtime is at least the 50ms we stubbed
			if runtimeMs < 50 {
				t.Errorf("RUNTIME_MS = %d, want >= 50", runtimeMs)
			}
		})
	}
}

func TestGetCommandName(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want string
	}{
		{
			name: "simple use",
			cmd:  &cobra.Command{Use: "deploy"},
			want: "deploy",
		},
		{
			name: "complex use",
			cmd:  &cobra.Command{Use: "create cluster"},
			want: "create",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := getCommandName(tc.cmd); got != tc.want {
				t.Errorf("getCommandName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetIsTestData(t *testing.T) {
	t.Run("returns true", func(t *testing.T) {
		if got := getIsTestData(); got != "true" {
			t.Errorf("getIsTestData() = %q, want %q", got, "true")
		}
	})
}

func TestGetRuntime(t *testing.T) {
	t.Run("calculates correct duration", func(t *testing.T) {
		resetGlobalState()

		// Mock start time to exactly 100ms ago
		eventStartTime = time.Now().Add(-100 * time.Millisecond)

		gotStr := getRuntime()

		got, err := strconv.ParseInt(gotStr, 10, 64)
		if err != nil {
			t.Fatalf("getRuntime() returned non-integer %q: %v", gotStr, err)
		}

		if got < 100 {
			t.Errorf("getRuntime() = %d, want >= 100", got)
		}
	})
}
