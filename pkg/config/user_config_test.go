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
	"fmt"
	"os"
	"os/user"
	"testing"
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
