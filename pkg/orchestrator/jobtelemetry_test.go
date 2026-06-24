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

package orchestrator

import (
	"hpc-toolkit/pkg/shell"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestIsInternalUser(t *testing.T) {
	orig := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = orig }()

	tests := []struct {
		name           string
		mockStdout     string
		mockExitCode   int
		expectedResult bool
	}{
		{
			name:           "Internal User Google",
			mockStdout:     "testuser@google.com\n",
			mockExitCode:   0,
			expectedResult: true,
		},
		{
			name:           "External User",
			mockStdout:     "testuser@example.com\n",
			mockExitCode:   0,
			expectedResult: false,
		},
		{
			name:           "Command Error",
			mockStdout:     "",
			mockExitCode:   1,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset sync.Once and cached value for each test case
			isInternalOnce = sync.Once{}
			isInternalCached = false

			shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
				return shell.CommandResult{Stdout: tt.mockStdout, ExitCode: tt.mockExitCode}
			}

			got := isInternalUser()
			if got != tt.expectedResult {
				t.Errorf("isInternalUser() = %v, want %v", got, tt.expectedResult)
			}
		})
	}
}

func TestRecordLocalMetrics_Skip(t *testing.T) {
	os.Setenv("GCLUSTER_SKIP_TELEMETRY", "true")
	defer os.Unsetenv("GCLUSTER_SKIP_TELEMETRY")

	// If it skips, it shouldn't access file system or call gcloud
	RecordLocalMetrics("test-job", 1.0, true, nil)
	// Success if it doesn't panic or error out (since we aren't verify file output here yet, but cover the early exit)
}

func TestRecordLocalMetrics_ExternalUser(t *testing.T) {
	orig := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = orig }()

	// For external user, it should skip file writing
	isInternalOnce = sync.Once{}
	isInternalCached = false
	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		return shell.CommandResult{Stdout: "external@example.com\n", ExitCode: 0}
	}

	RecordLocalMetrics("test-job", 1.0, true, nil)
	// Success if it exits quietly
}

func TestRecordLocalMetrics_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "telemetry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	orig := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = orig }()

	isInternalOnce = sync.Once{}
	isInternalCached = false
	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		return shell.CommandResult{Stdout: "internal@google.com\n", ExitCode: 0}
	}

	RecordLocalMetrics("test-job", 1.0, true, map[string]string{"foo": "bar"})

	// Verify file was written
	metricsFile := filepath.Join(tempDir, ".gcluster", "job_telemetry_metrics.jsonl")
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", metricsFile)
	}
}

func TestRecordLocalMetrics_MkdirError(t *testing.T) {
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/dev/null") // Cannot create directories inside /dev/null
	defer os.Setenv("HOME", origHome)

	orig := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = orig }()

	isInternalOnce = sync.Once{}
	isInternalCached = false
	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		return shell.CommandResult{Stdout: "internal@google.com\n", ExitCode: 0}
	}

	RecordLocalMetrics("test-job", 1.0, true, nil)
	// Success if it exits quietly after logging error
}

func TestRecordLocalMetrics_FileOpenError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "telemetry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	metricsDir := filepath.Join(tempDir, ".gcluster")
	if err := os.MkdirAll(metricsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Make directory read-only to force open file error
	if err := os.Chmod(metricsDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(metricsDir, 0755) // Ensure cleanup works

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	orig := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = orig }()

	isInternalOnce = sync.Once{}
	isInternalCached = false
	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		return shell.CommandResult{Stdout: "internal@google.com\n", ExitCode: 0}
	}

	RecordLocalMetrics("test-job", 1.0, true, nil)
	// Success if it exits quietly after logging error
}

func TestRecordLocalMetrics_HomeDirError(t *testing.T) {
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "") // Empty HOME forces UserHomeDir error
	defer os.Setenv("HOME", origHome)

	orig := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = orig }()

	isInternalOnce = sync.Once{}
	isInternalCached = false
	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		return shell.CommandResult{Stdout: "internal@google.com\n", ExitCode: 0}
	}

	RecordLocalMetrics("test-job", 1.0, true, nil)
	// Success if it exits quietly after logging error
}

func TestRecordLocalMetrics_JsonMarshalError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "telemetry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	orig := shell.ExecuteCommand
	defer func() { shell.ExecuteCommand = orig }()

	isInternalOnce = sync.Once{}
	isInternalCached = false
	shell.ExecuteCommand = func(name string, args ...string) shell.CommandResult {
		return shell.CommandResult{Stdout: "internal@google.com\n", ExitCode: 0}
	}

	// math.NaN() cannot be marshaled to JSON, forcing failure
	RecordLocalMetrics("test-job", math.NaN(), true, nil)
	// Success if it exits quietly after logging error
}
