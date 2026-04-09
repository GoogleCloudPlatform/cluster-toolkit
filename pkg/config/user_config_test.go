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
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/spf13/viper"
)

// TestGenerateUniqueID verifies that the unique ID generation is stable
// and matches the exact hashing logic (24 hex characters).
func TestGenerateUniqueID(t *testing.T) {
	id1 := generateUniqueID()
	id2 := generateUniqueID()

	if id1 == "" {
		t.Fatalf("generateUniqueID() returned an empty string")
	}
	if len(id1) != 24 {
		t.Errorf("expected ID length of 24, got %d", len(id1))
	}
	if id1 != id2 {
		t.Errorf("generateUniqueID() is not deterministic: %s != %s", id1, id2)
	}

	// Validate against the exact expected hashing logic
	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname() failed: %v", err)
	}
	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() failed: %v", err)
	}

	rawID := fmt.Sprintf("%s-%s", host, u.Username)
	hash := sha256.Sum256([]byte(rawID))
	expected := fmt.Sprintf("%x", hash)[:24]

	if id1 != expected {
		t.Errorf("expected ID %s, got %s", expected, id1)
	}
}

// TestSaveToFirestore verifies that viper settings are successfully written
// to Firestore. It requires the Firestore emulator.
func TestSaveToFirestore(t *testing.T) {
	// Setup a clean memory state
	viper.Reset()
	testUserID := "test-user-save"
	viper.Set(USER_ID_KEY, testUserID)
	viper.Set("theme", "light")
	viper.Set("telemetry_enabled", true)

	// Execute SaveToFirestore
	err := SaveToFirestore()
	if err != nil {
		t.Fatalf("SaveToFirestore() failed: %v", err)
	}

	// Verify data was actually written to the emulator database
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to create firestore client for verification: %v", err)
	}
	defer client.Close()

	doc, err := client.Collection(collectionName).Doc(testUserID).Get(ctx)
	if err != nil {
		t.Fatalf("failed to retrieve saved document: %v", err)
	}

	data := doc.Data()
	if data["theme"] != "light" {
		t.Errorf("expected theme 'light', got '%v'", data["theme"])
	}
	if data["telemetry_enabled"] != true {
		t.Errorf("expected telemetry_enabled to be true, got '%v'", data["telemetry_enabled"])
	}
}
