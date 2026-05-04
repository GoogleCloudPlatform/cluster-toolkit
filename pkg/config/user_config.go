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
)

// UserConfig holds the in-memory state of the user's telemetry preferences
type UserConfig struct {
	UserID           string `json:"user_id"`
	TelemetryEnabled bool   `json:"telemetry_enabled"`
}

// globalUserConfig is the package-level variable holding the state during execution
var globalUserConfig UserConfig

// InitUserConfig initializes the user's config, prioritizing a local JSON file over defaults.
func InitUserConfig() error {
	// Set the defaults
	globalUserConfig = UserConfig{
		UserID:           generateUniqueID(),
		TelemetryEnabled: true, // Default telemetry state
	}

	configFile := filepath.Join(getLocalDirPath(false), "telemetry_config.json")

	// Try to read from the local config file
	if data, err := os.ReadFile(configFile); err == nil {
		// If the file exists and is valid, overwrite the defaults
		if err := json.Unmarshal(data, &globalUserConfig); err == nil {
			return nil
		}
	}

	// If file doesn't exist or is invalid, save defaults to file
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

// SaveToFile saves the in-memory state back to a local JSON file
func SaveToFile() error {
	configFile := filepath.Join(getLocalDirPath(false), "telemetry_config.json")

	data, err := json.MarshalIndent(globalUserConfig, "", "  ")
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
