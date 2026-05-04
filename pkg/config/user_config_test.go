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

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// setupTestEnv creates a clean temporary directory and forces os.UserConfigDir()
// to use it via environment variables. It also resets the global Viper state.
func setupTestEnv(t *testing.T) string {
	tempDir := t.TempDir()

	// Override environment variables used by os.UserConfigDir() across different OSes
	t.Setenv("XDG_CONFIG_HOME", tempDir) // Linux
	t.Setenv("HOME", tempDir)            // macOS / Linux fallback
	t.Setenv("AppData", tempDir)         // Windows
	t.Setenv("LocalAppData", tempDir)    // Windows fallback

	viper.Reset()
	return tempDir
}

func TestInitUserConfig_NewUser(t *testing.T) {
	tempDir := setupTestEnv(t)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Verify Viper state
	userID := GetPersistentUserId()
	if userID == "" || len(userID) != 24 {
		t.Errorf("Expected valid 24-char user ID, got: %s", userID)
	}

	// Verify File creation
	configFile := filepath.Join(tempDir, "cluster-toolkit", "telemetry_config.json")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Errorf("Expected config file to be created at %s", configFile)
	}
}

func TestInitUserConfig_ExistingUser(t *testing.T) {
	tempDir := setupTestEnv(t)

	// Pre-populate an existing config file
	configFile := filepath.Join(tempDir, "cluster-toolkit", "telemetry_config.json")
	_ = os.MkdirAll(filepath.Dir(configFile), 0755)

	existingData := map[string]any{
		USER_ID_KEY:   "existing-test-id",
		TELEMETRY_KEY: true,
	}
	data, _ := json.Marshal(existingData)
	_ = os.WriteFile(configFile, data, 0644)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Verify Viper state loaded the existing data instead of generating new defaults
	if GetPersistentUserId() != "existing-test-id" {
		t.Errorf("Expected user ID 'existing-test-id', got: %s", GetPersistentUserId())
	}
	if !IsTelemetryEnabled() {
		t.Errorf("Expected telemetry to be true")
	}
}

func TestInitUserConfig_CorruptFile(t *testing.T) {
	tempDir := setupTestEnv(t)

	// Create a corrupt config file (invalid JSON)
	configFile := filepath.Join(tempDir, "cluster-toolkit", "telemetry_config.json")
	_ = os.MkdirAll(filepath.Dir(configFile), 0755)
	_ = os.WriteFile(configFile, []byte("{invalid_json_here]"), 0644)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Verify Viper safely recovered by generating a new ID
	userID := GetPersistentUserId()
	if userID == "" || len(userID) != 24 {
		t.Errorf("Expected valid 24-char user ID to be generated, got: %s", userID)
	}

	// Verify the corrupt file was successfully overwritten with valid JSON
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Errorf("Expected config file to be overwritten with valid JSON, got unmarshal error: %v", err)
	}
}

func TestSetTelemetry(t *testing.T) {
	tempDir := setupTestEnv(t)

	// Initialize first to set up the baseline
	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig setup failed: %v", err)
	}

	// Action: update telemetry
	err = SetTelemetry(true)
	if err != nil {
		t.Fatalf("SetTelemetry failed: %v", err)
	}

	// Verify Viper in-memory state
	if !IsTelemetryEnabled() {
		t.Errorf("Expected telemetry to be true in viper")
	}

	// Verify File on-disk state
	configFile := filepath.Join(tempDir, "cluster-toolkit", "telemetry_config.json")
	data, _ := os.ReadFile(configFile)

	var settings map[string]any
	_ = json.Unmarshal(data, &settings)

	if settings[TELEMETRY_KEY] != true {
		t.Errorf("Expected telemetry to be true in file, got: %v", settings[TELEMETRY_KEY])
	}
}

func TestGenerateUniqueID(t *testing.T) {
	id1 := generateUniqueID()
	id2 := generateUniqueID()

	// Length should be constrained to 24 characters
	if len(id1) != 24 {
		t.Errorf("Expected ID length 24, got %d", len(id1))
	}

	// Because we hash hostname and username, the ID should be deterministic across calls on the same machine during a single execution.
	if id1 != id2 {
		t.Errorf("Expected generateUniqueID to be deterministic for the same machine/user context")
	}
}
