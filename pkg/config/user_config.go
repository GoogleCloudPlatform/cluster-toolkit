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
	"context"
	"crypto/sha256"

	"fmt"
	"os"
	"os/user"

	"hpc-toolkit/pkg/logging"

	"github.com/spf13/viper"
	_ "github.com/spf13/viper/remote"

	"cloud.google.com/go/firestore"
)

const (
	USER_ID_KEY    string = "user_id"
	TELEMETRY_KEY  string = "telemetry_enabled"
	projectID      string = "hpc-toolkit-gsc"
	collectionName string = "user_configs"
)

func InitUserConfig() error {
	ctx := context.Background()
	userID := generateUniqueID()

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to create firestore client: %v", err)
	}
	defer client.Close()

	// Set local Viper defaults
	viper.SetDefault(USER_ID_KEY, userID)

	// Try to fetch the document
	doc, err := client.Collection(collectionName).Doc(userID).Get(ctx)
	if err != nil {
		return SaveToFirestore()
	}

	// Merge Firestore data into Viper
	data := doc.Data()
	for k, v := range data {
		viper.Set(k, v)
	}

	return nil
}

// GetPersistentUserId returns the stored User ID from Viper config.
func GetPersistentUserId() string {
	_ = viper.ReadRemoteConfig()
	return viper.GetString(USER_ID_KEY)

}

// IsTelemetryEnabled returns the stored config setting for whether Telemetry data should be collected or not.
func IsTelemetryEnabled() bool {
	_ = viper.ReadRemoteConfig()
	return viper.GetBool(TELEMETRY_KEY)
}

// SetTelemetry sets the telemetry preference for the user.
func SetTelemetry(telemetry bool) {
	_ = viper.ReadRemoteConfig()
	viper.Set(TELEMETRY_KEY, telemetry)
	err := SaveToFirestore()
	if err != nil {
		logging.Error("Failed to save state to Firestore: %v", err)
	}
}

// Save Viper state back to Firestore
func SaveToFirestore() error {
	ctx := context.Background()
	userID := viper.GetString(USER_ID_KEY)

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return err
	}
	defer client.Close()

	settings := viper.AllSettings()

	_, err = client.Collection(collectionName).Doc(userID).Set(ctx, settings)
	if err != nil {
		return fmt.Errorf("failed to save to firestore: %v", err)
	}
	return nil
}

// generateUniqueID creates a stable hash based on the machine and user
func generateUniqueID() string {
	host, _ := os.Hostname()
	u, _ := user.Current()
	rawID := fmt.Sprintf("%s-%s", host, u.Username)

	// Hash it to create a clean, fixed-length unique ID (to avoid PII)
	hash := sha256.Sum256([]byte(rawID))
	return fmt.Sprintf("%x", hash)[:24]
}
