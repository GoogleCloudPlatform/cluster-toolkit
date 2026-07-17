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

package engine

import (
	"strings"
	"testing"

	"hpc-toolkit/pkg/intent/ast"
)

func TestSerialize(t *testing.T) {
	groups := []ast.DeploymentGroup{
		{
			Group: "primary",
			Modules: []ast.ModuleSpec{
				{
					ID:        "fs",
					Source:    "modules/fs",
					Condition: "true", // Should be stripped
					Export:    true,   // Should be stripped
					Settings: map[string]any{
						"size": 100,
					},
				},
				{
					ID:            "pool",
					Source:        "modules/pool",
					Use:           []string{"fs"},
					IsComputePool: true,
				},
			},
		},
	}

	vars := map[string]any{
		"project_id": "test-project",
	}

	out, err := Serialize("test-cluster", vars, groups)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if !strings.HasPrefix(out, "---") {
		t.Errorf("expected YAML to start with ---")
	}

	if !strings.Contains(out, "blueprint_name: test-cluster") {
		t.Errorf("expected blueprint_name in output")
	}

	if !strings.Contains(out, "project_id: test-project") {
		t.Errorf("expected vars in output")
	}

	if strings.Contains(out, "condition") || strings.Contains(out, "export") {
		t.Errorf("expected intent metadata (condition, export) to be stripped, got:\n%s", out)
	}

	if !strings.Contains(out, "modules:\n") {
		t.Errorf("expected ComputePool to be merged under 'modules'")
	}
}
