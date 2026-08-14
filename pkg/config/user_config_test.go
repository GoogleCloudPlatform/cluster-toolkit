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
)

// setupTestEnv creates a clean temporary directory and forces os.UserConfigDir()
// to use it via environment variables. It also resets the global state.
func setupTestEnv(t *testing.T) string {
	tempDir := t.TempDir()

	// Override environment variables used by os.UserConfigDir() across different OSes
	t.Setenv("XDG_CONFIG_HOME", tempDir) // Linux
	t.Setenv("HOME", tempDir)            // macOS / Linux fallback
	t.Setenv("AppData", tempDir)         // Windows
	t.Setenv("LocalAppData", tempDir)    // Windows fallback

	globalUserConfig = UserConfig{}
	return tempDir
}

func TestInitUserConfig_NewUser(t *testing.T) {
	tempDir := setupTestEnv(t)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Verify in-memory state
	userID := GetPersistentUserId()
	if userID == "" || len(userID) != 24 {
		t.Errorf("Expected valid 24-char user ID, got: %s", userID)
	}

	// Verify File creation
	configFile := filepath.Join(tempDir, "cluster-toolkit", configFileName)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Errorf("Expected config file to be created at %s", configFile)
	}
}

func TestInitUserConfig_ExistingUser(t *testing.T) {
	tempDir := setupTestEnv(t)

	// Pre-populate an existing config file
	configFile := filepath.Join(tempDir, "cluster-toolkit", configFileName)
	_ = os.MkdirAll(filepath.Dir(configFile), 0755)

	existingData := UserConfig{
		UserID:           "existing-test-id",
		TelemetryEnabled: true,
	}
	data, _ := json.Marshal(existingData)
	_ = os.WriteFile(configFile, data, 0644)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Verify the in-memory state loaded the existing data instead of generating new defaults
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
	configFile := filepath.Join(tempDir, "cluster-toolkit", configFileName)
	_ = os.MkdirAll(filepath.Dir(configFile), 0755)
	_ = os.WriteFile(configFile, []byte("{invalid_json_here]"), 0644)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Verify the system safely recovered by generating a new ID
	userID := GetPersistentUserId()
	if userID == "" || len(userID) != 24 {
		t.Errorf("Expected valid 24-char user ID to be generated, got: %s", userID)
	}

	// Verify the corrupt file was successfully overwritten with valid JSON
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	var settings UserConfig
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

	// Verify in-memory state
	if !IsTelemetryEnabled() {
		t.Errorf("Expected telemetry to be true in memory state")
	}

	// Verify File on-disk state
	configFile := filepath.Join(tempDir, "cluster-toolkit", configFileName)
	data, _ := os.ReadFile(configFile)

	var settings UserConfig
	_ = json.Unmarshal(data, &settings)

	if !settings.TelemetryEnabled {
		t.Errorf("Expected telemetry to be true in file, got: %v", settings.TelemetryEnabled)
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

// TestGetIsGoogler_Nil verifies the default state of the cache for a new user.
func TestGetIsGoogler_Nil(t *testing.T) {
	_ = setupTestEnv(t)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// For a fresh config without the is_googler key, the pointer should be nil.
	if GetIsGoogler() != nil {
		t.Errorf("Expected GetIsGoogler to return nil initially, got %v", *GetIsGoogler())
	}
}

// TestSetIsGoogler verifies setting the cache updates memory and persists to disk.
func TestSetIsGoogler(t *testing.T) {
	tempDir := setupTestEnv(t)

	// Initialize first to set up the baseline
	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig setup failed: %v", err)
	}

	// Action: update is_googler cache
	err = SetIsGoogler(true)
	if err != nil {
		t.Fatalf("SetIsGoogler failed: %v", err)
	}

	// Verify in-memory state
	cached := GetIsGoogler()
	if cached == nil || !*cached {
		t.Errorf("Expected IsGoogler to be true in memory state")
	}

	// Verify File on-disk state
	configFile := filepath.Join(tempDir, "cluster-toolkit", configFileName)
	data, _ := os.ReadFile(configFile)

	var settings UserConfig
	_ = json.Unmarshal(data, &settings)

	if settings.IsGoogler == nil || !*settings.IsGoogler {
		t.Errorf("Expected is_googler to be true in file, got: %v", settings.IsGoogler)
	}
}

// TestInitUserConfig_ExistingIsGoogler verifies loading an existing config file that contains the cached value.
func TestInitUserConfig_ExistingIsGoogler(t *testing.T) {
	tempDir := setupTestEnv(t)

	// Pre-populate an existing config file
	configFile := filepath.Join(tempDir, "cluster-toolkit", configFileName)
	_ = os.MkdirAll(filepath.Dir(configFile), 0755)

	isGoogler := false
	existingData := UserConfig{
		UserID:           "existing-test-id",
		TelemetryEnabled: true,
		IsGoogler:        &isGoogler,
	}
	data, _ := json.Marshal(existingData)
	_ = os.WriteFile(configFile, data, 0644)

	err := InitUserConfig()
	if err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Verify the in-memory state loaded the existing IsGoogler data
	cached := GetIsGoogler()
	if cached == nil {
		t.Fatalf("Expected IsGoogler to be loaded from file, got nil")
	}
	if *cached != false {
		t.Errorf("Expected IsGoogler to be false, got true")
	}
}
