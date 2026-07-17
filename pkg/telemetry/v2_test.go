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

package telemetry

import (
	"os"
	"path/filepath"
	"testing"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"

	"github.com/spf13/cobra"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsV2Invocation(t *testing.T) {
	dir := t.TempDir()

	v2Config := filepath.Join(dir, "cluster-config.yaml")
	writeFile(t, v2Config, "config_base: slurm\ncompute_archetypes:\n  - name: gpu\n    machine_type: a4-highgpu-8g\n")

	legacyBlueprint := filepath.Join(dir, "blueprint.yaml")
	writeFile(t, legacyBlueprint, "blueprint_name: hpc-slurm\nvars:\n  project_id: p\n")

	v2Deployment := filepath.Join(dir, "v2-deployment")
	writeFile(t, filepath.Join(modulewriter.ArtifactsDir(v2Deployment), modulewriter.ExpandedBlueprintName),
		"blueprint_name: slurm\nv2_config_base: slurm\n")

	legacyDeployment := filepath.Join(dir, "legacy-deployment")
	writeFile(t, filepath.Join(modulewriter.ArtifactsDir(legacyDeployment), modulewriter.ExpandedBlueprintName),
		"blueprint_name: hpc-slurm\n")

	emptyDeployment := filepath.Join(dir, "empty-deployment")
	if err := os.MkdirAll(emptyDeployment, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", nil, false},
		{"v2 cluster-config file", []string{v2Config}, true},
		{"legacy blueprint file", []string{legacyBlueprint}, false},
		{"v2 deployment folder", []string{v2Deployment}, true},
		{"legacy deployment folder", []string{legacyDeployment}, false},
		{"deployment folder without expanded blueprint", []string{emptyDeployment}, false},
		{"missing file", []string{filepath.Join(dir, "nope.yaml")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsV2Invocation(tt.args); got != tt.want {
				t.Errorf("IsV2Invocation(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestIsV2Invocation_ExportRoundTrip writes the expanded blueprint with Blueprint.Export,
// the same writer `create` uses, so a rename of the marker's yaml tag cannot silently
// break deploy/destroy attribution.
func TestIsV2Invocation_ExportRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		bp   config.Blueprint
		want bool
	}{
		{"v2 blueprint", config.Blueprint{BlueprintName: "slurm", V2ConfigBase: "slurm"}, true},
		{"legacy blueprint", config.Blueprint{BlueprintName: "hpc-slurm"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			depl := filepath.Join(t.TempDir(), "depl")
			art := modulewriter.ArtifactsDir(depl)
			if err := os.MkdirAll(art, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := tc.bp.Export(filepath.Join(art, modulewriter.ExpandedBlueprintName)); err != nil {
				t.Fatal(err)
			}
			if got := IsV2Invocation([]string{depl}); got != tc.want {
				t.Errorf("IsV2Invocation(deployment) = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollectMetrics_IsV2(t *testing.T) {
	for _, isV2 := range []bool{true, false} {
		c := NewCollector(&cobra.Command{Use: "create"}, nil, SOURCE, isV2)
		c.CollectMetrics(0, nil)

		want := "false"
		wantBlueprint := ""
		if isV2 {
			want = "true"
			wantBlueprint = v2BlueprintName
		}
		if got := c.metadata[IS_V2]; got != want {
			t.Errorf("isV2=%v: metadata[IS_V2] = %q, want %q", isV2, got, want)
		}
		if got := c.metadata[BLUEPRINT]; got != wantBlueprint {
			t.Errorf("isV2=%v: metadata[BLUEPRINT] = %q, want %q", isV2, got, wantBlueprint)
		}
	}
}

func TestSetBlueprint(t *testing.T) {
	c := NewCollector(&cobra.Command{Use: "create"}, nil, SOURCE, true)
	c.SetBlueprint(config.Blueprint{BlueprintName: "slurm", AIAssisted: true})
	c.CollectMetrics(0, nil)

	// The expanded blueprint feeds the standard metrics...
	if got := c.metadata[IS_AI_ASSISTED]; got != "true" {
		t.Errorf("metadata[IS_AI_ASSISTED] = %q, want %q", got, "true")
	}
	// ...but BLUEPRINT stays "v2" rather than being masked as "Custom".
	if got := c.metadata[BLUEPRINT]; got != v2BlueprintName {
		t.Errorf("metadata[BLUEPRINT] = %q, want %q", got, v2BlueprintName)
	}
}
