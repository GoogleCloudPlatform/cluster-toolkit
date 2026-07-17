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
	"hpc-toolkit/pkg/intent/ast"
	"testing"
)

func TestResolveDAG(t *testing.T) {
	groups := []ast.DeploymentGroup{
		{
			Group: "cluster-env",
			Modules: []ast.ModuleSpec{
				{ID: "network", Use: nil},
			},
		},
		{
			Group: "cluster",
			Modules: []ast.ModuleSpec{
				// Originally depends on network, and gets auto-wired to homefs
				{ID: "slurm_controller", Use: []string{"network", "homefs"}},
			},
		},
		{
			// Simulated "auto" group injected by mergeGroups
			Group: "auto",
			Modules: []ast.ModuleSpec{
				{ID: "homefs", Use: []string{"network"}},
			},
		},
	}

	resolved := ResolveDAG(groups)

	// homefs depends on network (index 0), so it should evaluate to index 0 (cluster-env).
	// slurm_controller depends on network (index 0) and homefs (now index 0),
	// so slurm_controller evaluates to max(0, 0) = 0?
	// Wait, slurm_controller started at index 1 ("cluster").
	// The DAG algorithm does targetIndex = max(loc.index, depIndices).
	// So slurm_controller's loc.index is 1, max(1, 0, 0) is 1.
	// It should remain in "cluster".

	if len(resolved) != 2 {
		t.Fatalf("Expected 2 groups after resolving 'auto', got %d", len(resolved))
	}

	// Verify homefs was pushed to index 0 (cluster-env)
	foundHomeFS := false
	for _, m := range resolved[0].Modules {
		if m.ID == "homefs" {
			foundHomeFS = true
		}
	}
	if !foundHomeFS {
		t.Errorf("Expected homefs to be in group 'cluster-env' (index 0)")
	}

	// Verify slurm_controller remained in index 1 (cluster)
	foundController := false
	for _, m := range resolved[1].Modules {
		if m.ID == "slurm_controller" {
			foundController = true
		}
	}
	if !foundController {
		t.Errorf("Expected slurm_controller to be in group 'cluster' (index 1)")
	}
}

func TestResolveDAG_DownstreamPush(t *testing.T) {
	groups := []ast.DeploymentGroup{
		{
			Group: "cluster-env",
			Modules: []ast.ModuleSpec{
				{ID: "network", Use: nil},
			},
		},
		{
			Group: "cluster",
			Modules: []ast.ModuleSpec{
				{ID: "slurm_controller", Use: []string{"network", "special_feature"}},
			},
		},
		{
			Group: "stage-2",
			Modules: []ast.ModuleSpec{
				{ID: "special_feature", Use: nil}, // Placed explicitly in a later stage
			},
		},
	}

	resolved := ResolveDAG(groups)

	// slurm_controller is at index 1 ("cluster").
	// It depends on special_feature at index 2 ("stage-2").
	// It should be PUSHED DOWN to index 2!

	if len(resolved[1].Modules) != 0 {
		t.Errorf("Expected slurm_controller to be pushed out of 'cluster'")
	}

	foundController := false
	for _, m := range resolved[2].Modules {
		if m.ID == "slurm_controller" {
			foundController = true
		}
	}
	if !foundController {
		t.Errorf("Expected slurm_controller to be in group 'stage-2' (index 2)")
	}
}
