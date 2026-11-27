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
	"runtime"
	"strings"
	"testing"
	"time"
)

// setupTestEnv creates a clean temporary directory and forces os.UserConfigDir()
// and os.UserCacheDir() to use it via environment variables. It also resets global state.
func setupTestEnv(t *testing.T) string {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")

	// Override environment variables used by os.UserConfigDir() and os.UserCacheDir() across OSes
	t.Setenv("XDG_CONFIG_HOME", tempDir) // Linux config
	t.Setenv("XDG_CACHE_HOME", cacheDir) // Linux cache
	t.Setenv("HOME", tempDir)            // macOS / Linux fallback
	t.Setenv("AppData", tempDir)         // Windows config
	t.Setenv("LocalAppData", cacheDir)   // Windows cache

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

func TestCachedProjectMetadata_BasicAndMultiProject(t *testing.T) {
	setupTestEnv(t)

	if _, ok := GetCachedProjectMetadata("my-proj", time.Hour); ok {
		t.Fatal("expected cache miss before storing metadata")
	}
	if _, ok := GetCachedProjectMetadata("   ", time.Hour); ok {
		t.Fatal("expected cache miss for blank project ID")
	}
	if err := SetCachedProjectMetadata("   ", ProjectMetadata{ProjectNumber: "1"}); err != nil {
		t.Fatalf("SetCachedProjectMetadata(blank) unexpected error: %v", err)
	}

	want := ProjectMetadata{
		ProjectNumber:    "123456789",
		BillingAccountID: "012345-6789AB-CDEF01",
		UpdatedAt:        time.Now().UTC(),
		ExpiresAt:        time.Now().UTC().Add(time.Hour),
	}
	if err := SetCachedProjectMetadata("my-proj", want); err != nil {
		t.Fatalf("SetCachedProjectMetadata failed: %v", err)
	}

	got, ok := GetCachedProjectMetadata("my-proj", time.Hour)
	if !ok || got.ProjectNumber != want.ProjectNumber || got.BillingAccountID != want.BillingAccountID {
		t.Fatalf("got (%+v, %v), want (%+v, true)", got, ok, want)
	}

	// Storing a second project preserves the first project's entry.
	second := ProjectMetadata{
		ProjectNumber: "222222",
		UpdatedAt:     time.Now().UTC(),
	}
	if err := SetCachedProjectMetadata("second-proj", second); err != nil {
		t.Fatalf("SetCachedProjectMetadata(second-proj) failed: %v", err)
	}
	if got1, ok1 := GetCachedProjectMetadata("my-proj", time.Hour); !ok1 || got1.ProjectNumber != want.ProjectNumber {
		t.Errorf("first entry lost after writing second project: got (%+v, %v)", got1, ok1)
	}
	if got2, ok2 := GetCachedProjectMetadata("second-proj", time.Hour); !ok2 || got2.ProjectNumber != second.ProjectNumber {
		t.Errorf("second entry mismatch: got (%+v, %v)", got2, ok2)
	}
}

func TestCachedProjectMetadata_IsolatedAtomicPermissions(t *testing.T) {
	tempDir := setupTestEnv(t)
	if err := InitUserConfig(); err != nil {
		t.Fatalf("InitUserConfig failed: %v", err)
	}

	// Simulate another process opting out via `gcluster telemetry off` while this
	// process still has TelemetryEnabled=true in memory.
	configFile := filepath.Join(tempDir, "cluster-toolkit", configFileName)
	optOutData, err := json.Marshal(UserConfig{
		UserID:           GetPersistentUserId(),
		TelemetryEnabled: false,
	})
	if err != nil {
		t.Fatalf("failed to marshal opt-out config: %v", err)
	}
	if err := os.WriteFile(configFile, optOutData, 0644); err != nil {
		t.Fatalf("failed to write opt-out config: %v", err)
	}

	if err := SetCachedProjectMetadata("my-proj", ProjectMetadata{
		ProjectNumber: "123456789",
		UpdatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SetCachedProjectMetadata failed: %v", err)
	}

	// telemetry_config.json must NOT be modified by project cache writes (no opt-out race).
	afterConfigBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read telemetry_config.json: %v", err)
	}
	if string(afterConfigBytes) != string(optOutData) {
		t.Fatalf("SetCachedProjectMetadata modified telemetry_config.json:\ngot:  %s\nwant: %s", afterConfigBytes, optOutData)
	}

	// Verify dedicated cache file location, 0600 permissions, and no leftover temp files.
	cacheFile := projectCacheFilePath()
	info, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatalf("expected project cache file at %s: %v", cacheFile, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("project cache file mode = %o, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(cacheFile))
	if err != nil {
		t.Fatalf("failed to list cache dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("unexpected leftover temp file in cache dir: %s", entry.Name())
		}
	}
}

func TestCachedProjectMetadata_ExpiryAndCorruptionRecovery(t *testing.T) {
	setupTestEnv(t)

	// Verify explicit ExpiresAt takes precedence over a longer ttl.
	shortExpired := ProjectMetadata{
		UpdatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(-time.Second),
	}
	if err := SetCachedProjectMetadata("short-expired", shortExpired); err != nil {
		t.Fatalf("SetCachedProjectMetadata(short-expired) failed: %v", err)
	}
	if _, ok := GetCachedProjectMetadata("short-expired", 24*time.Hour); ok {
		t.Error("expected cache miss when ExpiresAt has passed even if UpdatedAt is within ttl")
	}

	// Verify fallback TTL expiration when ExpiresAt is zero.
	expired := ProjectMetadata{
		ProjectNumber: "999",
		UpdatedAt:     time.Now().UTC().Add(-2 * time.Hour),
	}
	if err := SetCachedProjectMetadata("expired-proj", expired); err != nil {
		t.Fatalf("SetCachedProjectMetadata(expired) failed: %v", err)
	}
	if _, ok := GetCachedProjectMetadata("expired-proj", time.Hour); ok {
		t.Error("expected cache miss for expired entry")
	}

	// Verify corrupt cache file is treated as a miss and recovers cleanly on next write.
	cacheFile := projectCacheFilePath()
	if err := os.WriteFile(cacheFile, []byte("{truncated"), 0600); err != nil {
		t.Fatalf("failed to write corrupt cache file: %v", err)
	}
	if _, ok := GetCachedProjectMetadata("my-proj", time.Hour); ok {
		t.Error("expected cache miss on corrupt project cache file")
	}
	recovered := ProjectMetadata{ProjectNumber: "123456789", UpdatedAt: time.Now().UTC()}
	if err := SetCachedProjectMetadata("recovered-proj", recovered); err != nil {
		t.Fatalf("SetCachedProjectMetadata after corruption failed: %v", err)
	}
	if gotRec, ok := GetCachedProjectMetadata("recovered-proj", time.Hour); !ok || gotRec.ProjectNumber != recovered.ProjectNumber {
		t.Errorf("after recovery got (%+v, %v), want (%+v, true)", gotRec, ok, recovered)
	}
}
