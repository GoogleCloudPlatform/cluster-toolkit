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

package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"hpc-toolkit/pkg/intent/engine"
)

func TestCatalogLoader(t *testing.T) {
	tempDir := t.TempDir()

	// Scaffold mock directory structure
	os.MkdirAll(filepath.Join(tempDir, "v2", "config-base"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "v2", "compute-archetypes"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "v2", "features"), 0755)

	// Write mock YAMLs
	os.WriteFile(filepath.Join(tempDir, "v2", "config-base", "slurm.yaml"), []byte(`name: slurm`), 0644)
	os.WriteFile(filepath.Join(tempDir, "v2", "compute-archetypes", "a3ultra.yaml"), []byte(`name: a3-ultragpu-8g`), 0644)
	os.WriteFile(filepath.Join(tempDir, "v2", "compute-archetypes", "generic-cpu.yaml"), []byte("name: generic-cpu\nmatch_patterns: [\"*\"]\n"), 0644)
	os.WriteFile(filepath.Join(tempDir, "v2", "features", "lustre.yaml"), []byte(`name: managed-lustre`), 0644)

	loader := engine.NewCatalogLoader(tempDir)

	// Test Config Base
	base, err := loader.LoadConfigBase("slurm")
	if err != nil {
		t.Fatalf("Failed to load config base: %v", err)
	}
	if base.Name != "slurm" {
		t.Errorf("Expected Name 'slurm', got %q", base.Name)
	}

	// Test Archetype
	arch, err := loader.LoadArchetype("a3-ultragpu-8g")
	if err != nil {
		t.Fatalf("Failed to load archetype: %v", err)
	}
	if arch.Name != "a3-ultragpu-8g" {
		t.Errorf("Expected Name 'a3-ultragpu-8g', got %q", arch.Name)
	}

	// Test Feature
	feat, err := loader.LoadFeature("managed-lustre")
	if err != nil {
		t.Fatalf("Failed to load feature: %v", err)
	}
	if feat.Name != "managed-lustre" {
		t.Errorf("Expected Name 'managed-lustre', got %q", feat.Name)
	}

	// Test Generic CPU Fallback
	archFallback, err := loader.LoadArchetype("n2-standard-60")
	if err != nil {
		t.Fatalf("Failed to load generic CPU fallback archetype: %v", err)
	}
	if archFallback.Name != "generic-cpu" {
		t.Errorf("Expected Name 'generic-cpu' for fallback, got %q", archFallback.Name)
	}

	// Test Missing File
	_, err = loader.LoadConfigBase("missing")
	if err == nil {
		t.Errorf("Expected error loading missing config base, got nil")
	}
}
