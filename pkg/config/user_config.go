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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	configFileName       = "telemetry_config.json"
	projectCacheFileName = "telemetry_project_cache.json"
)

// ProjectMetadata caches resolved GCP project identifiers for telemetry so
// repeated CLI invocations avoid redundant Resource Manager and Billing RPCs.
type ProjectMetadata struct {
	ProjectNumber    string    `json:"project_number,omitempty"`
	BillingAccountID string    `json:"billing_account_id,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
}

// UserConfig holds the in-memory state of the user information and telemetry preferences.
type UserConfig struct {
	UserID           string `json:"user_id"`
	TelemetryEnabled bool   `json:"telemetry_enabled"`
	IsGoogler        *bool  `json:"is_googler,omitempty"`
}

// globalUserConfig is the package-level variable holding the state during execution
var (
	globalUserConfig UserConfig
	mu               sync.RWMutex
	projectCacheMu   sync.Mutex
)

// InitUserConfig initializes the user's config, prioritizing a local JSON file over defaults.
func InitUserConfig() error {
	// Prepare defaults locally
	defaultConfig := UserConfig{
		UserID:           GenerateUniqueIDFunc(),
		TelemetryEnabled: true, // Default telemetry state
	}

	configFile := filepath.Join(getLocalDirPath(false), configFileName)

	// Try to read from the local config file
	if data, err := os.ReadFile(configFile); err == nil {
		// Unmarshal into the local struct to avoid partial state corruption
		tempConfig := defaultConfig
		if err := json.Unmarshal(data, &tempConfig); err == nil {
			needsRepair := false
			if tempConfig.UserID == "" {
				tempConfig.UserID = GenerateUniqueIDFunc()
				needsRepair = true
			}

			// Atomically apply the loaded config
			mu.Lock()
			globalUserConfig = tempConfig
			mu.Unlock()

			// If we repaired an empty ID, write the fix back to the JSON file
			if needsRepair {
				return SaveToFile()
			}
			return nil
		}
	}

	// If the file is missing or invalid, atomically apply defaults
	mu.Lock()
	globalUserConfig = defaultConfig
	mu.Unlock()

	return SaveToFile()
}

// GetPersistentUserId returns the stored User ID from the in-memory config.
func GetPersistentUserId() string {
	return globalUserConfig.UserID
}

// IsTelemetryEnabled returns the stored config setting for whether Telemetry data should be collected or not.
func IsTelemetryEnabled() bool {
	return globalUserConfig.TelemetryEnabled
}

// SetTelemetry sets the telemetry preference for the user and saves to disk.
func SetTelemetry(telemetry bool) error {
	globalUserConfig.TelemetryEnabled = telemetry
	err := SaveToFile()
	if err != nil {
		return fmt.Errorf("failed to save state to file: %v", err)
	}
	return nil
}

// GetIsGoogler returns the cached IsGoogler value if it exists, otherwise nil. This refers to whether the user is internal to Google or not.
func GetIsGoogler() *bool {
	mu.RLock()
	defer mu.RUnlock()

	return globalUserConfig.IsGoogler
}

// SetIsGoogler sets the IsGoogler status and persists it to disk.
func SetIsGoogler(isGoogler bool) error {
	mu.Lock()
	globalUserConfig.IsGoogler = &isGoogler
	mu.Unlock()

	err := SaveToFile()
	if err != nil {
		return fmt.Errorf("failed to save state to file: %v", err)
	}
	return nil
}

func projectCacheFilePath() string {
	return filepath.Join(getLocalDirPath(true), projectCacheFileName)
}

func readProjectCacheLocked(cacheFile string) map[string]ProjectMetadata {
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return make(map[string]ProjectMetadata)
	}
	var cache map[string]ProjectMetadata
	if err := json.Unmarshal(data, &cache); err != nil || cache == nil {
		return make(map[string]ProjectMetadata)
	}
	return cache
}

// GetCachedProjectMetadata returns cached project metadata for projectID if it
// exists and has not expired. If the stored entry specifies ExpiresAt, that
// deadline takes precedence; otherwise ttl is evaluated against UpdatedAt.
func GetCachedProjectMetadata(projectID string, ttl time.Duration) (ProjectMetadata, bool) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectMetadata{}, false
	}

	projectCacheMu.Lock()
	cache := readProjectCacheLocked(projectCacheFilePath())
	projectCacheMu.Unlock()

	entry, ok := cache[projectID]
	if !ok || entry.UpdatedAt.IsZero() {
		return ProjectMetadata{}, false
	}
	if !entry.ExpiresAt.IsZero() {
		if !time.Now().Before(entry.ExpiresAt) {
			return ProjectMetadata{}, false
		}
		return entry, true
	}
	if ttl > 0 && time.Since(entry.UpdatedAt) >= ttl {
		return ProjectMetadata{}, false
	}
	return entry, true
}

// SetCachedProjectMetadata stores resolved project metadata for projectID in a
// dedicated cache file under the cache directory using atomic write-and-rename
// with 0600 permissions, leaving telemetry_config.json untouched.
func SetCachedProjectMetadata(projectID string, meta ProjectMetadata) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = time.Now().UTC()
	}

	cacheFile := projectCacheFilePath()
	dir := filepath.Dir(cacheFile)

	projectCacheMu.Lock()
	defer projectCacheMu.Unlock()

	cache := readProjectCacheLocked(cacheFile)
	cache[projectID] = meta

	data, err := json.MarshalIndent(cache, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal project cache: %v", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %v", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".telemetry_project_cache-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp project cache file: %v", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to set permissions on temp project cache file: %v", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp project cache file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp project cache file: %v", err)
	}
	if err := os.Rename(tmpPath, cacheFile); err != nil {
		return fmt.Errorf("failed to replace project cache file: %v", err)
	}
	cleanup = false
	return nil
}

// SaveToFile saves the in-memory state back to a local JSON file
func SaveToFile() error {
	configFile := filepath.Join(getLocalDirPath(false), configFileName)

	mu.RLock()
	data, err := json.MarshalIndent(globalUserConfig, "", " ")
	mu.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to marshal user config: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to save to file: %v", err)
	}
	return nil
}

// GenerateUniqueIDFunc is exposed as a variable so it can be mocked in tests.
var GenerateUniqueIDFunc = generateUniqueID

// generateUniqueID creates a stable hash based on the machine and user
func generateUniqueID() string {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown-host"
	}
	username := "unknown-user"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	rawID := fmt.Sprintf("%s-%s", host, username)

	// Hash it to create a clean, fixed-length unique ID (to avoid PII)
	hash := sha256.Sum256([]byte(rawID))
	return fmt.Sprintf("%x", hash)[:24]
}
