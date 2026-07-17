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
	"fmt"
	"hpc-toolkit/pkg/intent/ast"
)

type modLoc struct {
	mod   ast.ModuleSpec
	isCP  bool
	index int
}

// ResolveDAG takes the current deployment groups and ensures that every module
// is placed at or after the maximum group index of its dependencies.
func ResolveDAG(groups []ast.DeploymentGroup) []ast.DeploymentGroup {
	// -------------------------------------------------------------------------
	// PHASE 1: Establish Base Deployment Groups
	// -------------------------------------------------------------------------
	// In legacy blueprints, deployment groups (e.g., cluster-env, cluster)
	// tightly control the order in which Terraform creates physical architectures.
	// We extract these physical base groups to maintain structural intent,
	// intentionally stripping away any abstract "auto" groups injected by Features.
	var baseGroupNames []string
	// baseGroupIndex maps a physical group name to its slot in baseGroupNames.
	// PHASE 4 rebuilds against baseGroupNames, so module start indices must be
	// expressed in *that* coordinate space, not in the index space of `groups`
	// (which still contains the abstract "auto" entries we are stripping here).
	baseGroupIndex := make(map[string]int)
	for _, g := range groups {
		if g.Group != "auto" {
			if _, seen := baseGroupIndex[g.Group]; !seen {
				baseGroupIndex[g.Group] = len(baseGroupNames)
				baseGroupNames = append(baseGroupNames, g.Group)
			}
		}
	}

	// -------------------------------------------------------------------------
	// PHASE 2: Build the Virtual DAG
	// -------------------------------------------------------------------------
	// We load every module into a location tracking map to prepare for sorting.
	modules := make(map[string]*modLoc)
	var orderedIDs []string // For deterministic rebuilding

	for _, g := range groups {
		// Default Rule: If a Feature was flagged as 'group: auto', we force its
		// baseline starting index to 0 (deploy immediately alongside VPC network).
		// The iterative DAG causal loop in Phase 3 will forcefully push it
		// downstream if its dependencies dictate it.
		startIndex := 0
		if g.Group != "auto" {
			startIndex = baseGroupIndex[g.Group]
		}

		for _, m := range g.Modules {
			modules[m.ID] = &modLoc{mod: m, isCP: m.IsComputePool, index: startIndex}
			orderedIDs = append(orderedIDs, m.ID)
		}
	}

	// -------------------------------------------------------------------------
	// PHASE 3: The Topological Sort Stabilization Loop
	// -------------------------------------------------------------------------
	stabilizeDAGLocations(modules, orderedIDs)

	// -------------------------------------------------------------------------
	// PHASE 4 & 5: Dynamic Stage Generation & Cleanup
	// -------------------------------------------------------------------------
	return rebuildGroupsFromDAG(baseGroupNames, baseGroupIndex, modules, orderedIDs)
}

func stabilizeDAGLocations(modules map[string]*modLoc, orderedIDs []string) {
	changed := true
	for changed {
		changed = false
		for _, id := range orderedIDs {
			loc := modules[id]
			targetIndex := loc.index
			for _, depID := range loc.mod.Use {
				if depLoc, ok := modules[depID]; ok && depLoc.index > targetIndex {
					targetIndex = depLoc.index
				}
			}
			if targetIndex > loc.index {
				loc.index = targetIndex
				changed = true
			}
		}
	}
}

func rebuildGroupsFromDAG(baseGroupNames []string, baseGroupIndex map[string]int, modules map[string]*modLoc, orderedIDs []string) []ast.DeploymentGroup {
	newGroups := make([]ast.DeploymentGroup, len(baseGroupNames))
	for i, name := range baseGroupNames {
		newGroups[i].Group = name
	}

	for _, wantCP := range []bool{false, true} {
		for _, id := range orderedIDs {
			loc := modules[id]
			if loc.isCP != wantCP {
				continue
			}
			for loc.index >= len(newGroups) {
				newGroups = append(newGroups, ast.DeploymentGroup{
					Group: fmt.Sprintf("stage-%d", len(newGroups)),
				})
			}
			newGroups[loc.index].Modules = append(newGroups[loc.index].Modules, loc.mod)
		}
	}

	var finalGroups []ast.DeploymentGroup
	for _, g := range newGroups {
		_, isBase := baseGroupIndex[g.Group]
		if len(g.Modules) > 0 || isBase {
			finalGroups = append(finalGroups, g)
		}
	}
	return finalGroups
}
