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

package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextFilePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context-test-home")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Setenv("HOME", tempDir)

	path, err := contextFilePath()
	if err != nil {
		t.Fatalf("contextFilePath() error = %v", err)
	}

	expectedPrefix := filepath.Join(tempDir, stateDirName)
	if !strings.HasPrefix(path, expectedPrefix) {
		t.Errorf("expected path to start with %s, got %s", expectedPrefix, path)
	}
}

func TestSaveContext(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context-save-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Setenv("HOME", tempDir)

	ctx := Context{
		ProjectID:   "test-project",
		ClusterName: "test-cluster",
		Location:    "us-central1-a",
	}

	if err := saveContext(ctx); err != nil {
		t.Fatalf("saveContext() error = %v", err)
	}

	stateDir := filepath.Join(tempDir, stateDirName)
	filePath := filepath.Join(stateDir, contextFileName)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("context file was not created at %s", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read created context file: %v", err)
	}

	var savedCtx Context
	if err := json.Unmarshal(data, &savedCtx); err != nil {
		t.Fatalf("failed to unmarshal saved context: %v", err)
	}

	if savedCtx != ctx {
		t.Errorf("saved context = %+v, want %+v", savedCtx, ctx)
	}
}

func TestLoadContext_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context-load-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Setenv("HOME", tempDir)

	stateDir := filepath.Join(tempDir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(stateDir, contextFileName)

	ctx := Context{
		ProjectID:   "test-project",
		ClusterName: "test-cluster",
		Location:    "us-central1-a",
	}
	data, _ := json.Marshal(ctx)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded := loadContext()
	if loaded != ctx {
		t.Errorf("LoadContext() = %+v, want %+v", loaded, ctx)
	}
}

func TestLoadContext_NotExist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context-load-notexist-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Setenv("HOME", tempDir)

	loaded := loadContext()
	expected := Context{}
	if loaded != expected {
		t.Errorf("LoadContext() = %+v, want %+v", loaded, expected)
	}
}

func TestLoadContext_CorruptFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "context-load-corrupt-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Setenv("HOME", tempDir)

	stateDir := filepath.Join(tempDir, stateDirName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(stateDir, contextFileName)

	if err := os.WriteFile(filePath, []byte("invalid-json"), 0644); err != nil {
		t.Fatal(err)
	}

	loaded := loadContext()
	expected := Context{}
	if loaded != expected {
		t.Errorf("LoadContext() = %+v, want %+v", loaded, expected)
	}
}
