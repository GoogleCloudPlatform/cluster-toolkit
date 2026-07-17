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

package intent_test

import (
	"os"
	"path/filepath"
	"testing"

	"hpc-toolkit/pkg/intent"
)

func TestIsV2Config(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		wantV2 bool
	}{
		{
			name:   "Valid v2 Config Base",
			yaml:   "config_base: slurm\nvars:\n  deployment_name: foo",
			wantV2: true,
		},
		{
			name:   "Valid v2 Compute Archetypes",
			yaml:   "compute_archetypes:\n  - name: pool\n    machine_type: a3-ultragpu-8g",
			wantV2: true,
		},
		{
			name:   "Valid v2 Features",
			yaml:   "features:\n  - name: fs\n    type: filestore",
			wantV2: true,
		},
		{
			name:   "Valid v2 Overlays",
			yaml:   "overlays:\n  - name: smi\n    type: run-nvidia-smi",
			wantV2: true,
		},
		{
			name:   "Legacy Blueprint File",
			yaml:   "blueprint_name: hpc-slurm\nvars:\n  project_id: test\ndeployment_groups: []",
			wantV2: false,
		},
		{
			name:   "Invalid YAML",
			yaml:   ":::invalid-yaml:::",
			wantV2: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intent.IsV2Config([]byte(tt.yaml))
			if got != tt.wantV2 {
				t.Errorf("IsV2Config() = %v, want %v", got, tt.wantV2)
			}
		})
	}
}

func TestIsV2ConfigFileAndToolkitPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("config_base: gke\n"), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	isV2, err := intent.IsV2ConfigFile(cfgFile)
	if err != nil || !isV2 {
		t.Fatalf("IsV2ConfigFile() = (%v, %v), want (true, nil)", isV2, err)
	}
	if _, err := intent.IsV2ConfigFile(filepath.Join(tmpDir, "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}

	root := intent.ToolkitPath()
	if root == "" {
		t.Fatal("ToolkitPath() returned empty string")
	}

	origDir, err := os.Getwd()
	if err == nil {
		defer os.Chdir(origDir)
		_ = os.Chdir(tmpDir)
		if got := intent.ToolkitPath(); got == "" {
			t.Fatal("ToolkitPath() from temp dir returned empty string")
		}
	}
}
