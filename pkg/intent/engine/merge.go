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

package engine

import (
	"fmt"
	"regexp"
	"strings"

	"hpc-toolkit/pkg/intent/ast"
)

// reservedPoolNames are names reserved for role targeting ("controller", "login", "compute").
var reservedPoolNames = map[string]string{
	"controller": "the Slurm controller",
	"login":      "the Slurm login node",
	"compute":    "all compute pools",
}

var validPoolNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// validatePoolNames rejects compute pool names reserved for role targeting.
func validatePoolNames(pools []string) error {
	for _, p := range pools {
		if meaning, bad := reservedPoolNames[strings.ToLower(p)]; bad {
			return fmt.Errorf(
				"compute archetype name %q is reserved: it will mean %s when attach_to gains role "+
					"targeting. Rename the pool, for example %q",
				p, meaning, p+"-nodes")
		}
		if !validPoolNamePattern.MatchString(p) {
			return fmt.Errorf(
				"invalid compute archetype name %q: must start with a letter and contain only letters, digits, underscores, or hyphens",
				p)
		}
	}
	return nil
}

// moduleBelongsToPool reports whether a module ID was generated for a given pool.
// Archetypes name their per-pool modules "{pool}_<something>", so a prefix test is
// exact rather than heuristic.
func moduleBelongsToPool(id, pool string) bool {
	return id == pool || strings.HasPrefix(id, pool+"_")
}

// matchesAttachTarget reports whether a module matches an attach_to target (pool name or role name).
func matchesAttachTarget(modID, featureInstanceID, target string, poolNames []string) bool {
	targetLower := strings.ToLower(target)
	if modID == target || strings.HasPrefix(modID, target+"_") || modID == targetLower || strings.HasPrefix(modID, targetLower+"_") || featureInstanceID == target {
		return true
	}
	if strings.EqualFold(target, "compute") {
		return getPoolPrefix(modID, poolNames) != ""
	}
	if !containsString(poolNames, target) && getPoolPrefix(modID, poolNames) == "" && strings.HasSuffix(modID, "_"+targetLower) {
		return true
	}
	return false
}

func isRunnerModule(m ast.ModuleSpec) bool {
	if m.Settings == nil {
		return false
	}
	_, hasRunners := m.Settings["runners"]
	return hasRunners
}

// mergeStartupScripts automatically merges Tier 4/5 runner-aggregating startup script modules
// into their target compute pool or role startup scripts without requiring `merge_into:` or `provides:` in YAML.
func mergeStartupScripts(groups []ast.DeploymentGroup, poolNames []string) ([]ast.DeploymentGroup, error) {
	var contributions []ast.ModuleSpec
	for gi := range groups {
		var kept []ast.ModuleSpec
		for _, m := range groups[gi].Modules {
			if m.Tier >= 4 && m.Export && isRunnerModule(m) {
				contributions = append(contributions, m)
				continue
			}
			kept = append(kept, m)
		}
		groups[gi].Modules = kept
	}

	if len(contributions) == 0 {
		return groups, nil
	}

	for _, c := range contributions {
		targetPools, targetRoles := classifyStartupTargets(c.AttachTo, poolNames)
		for _, role := range targetRoles {
			groups = mergeIntoSingletonStartup(groups, role+"_startup", []string{role}, c.Source, c.Settings)
		}
		seenPool := make(map[string]bool, len(targetPools))
		for _, pool := range targetPools {
			if seenPool[pool] {
				continue
			}
			seenPool[pool] = true
			var err error
			groups, err = mergeIntoPoolStartup(groups, pool, c.Source, c.Settings)
			if err != nil {
				return nil, err
			}
		}
	}

	return groups, nil
}

func classifyStartupTargets(attachTo, poolNames []string) ([]string, []string) {
	targets := attachTo
	if len(targets) == 0 {
		targets = poolNames
	}
	var targetPools []string
	var targetRoles []string
	seenRoles := make(map[string]bool)

	for _, t := range targets {
		switch {
		case strings.EqualFold(t, "compute"):
			targetPools = append(targetPools, poolNames...)
		case containsString(poolNames, t):
			targetPools = append(targetPools, t)
		default:
			role := strings.ToLower(t)
			if !seenRoles[role] {
				seenRoles[role] = true
				targetRoles = append(targetRoles, role)
			}
		}
	}
	return targetPools, targetRoles
}

func mergeIntoSingletonStartup(groups []ast.DeploymentGroup, id string, attachTo []string, source string, settings map[string]any) []ast.DeploymentGroup {
	for gi := range groups {
		for mi := range groups[gi].Modules {
			if groups[gi].Modules[mi].ID == id {
				groups[gi].Modules[mi].Settings = mergeSettingsDeep(groups[gi].Modules[mi].Settings, settings)
				return groups
			}
		}
	}
	targetGi := len(groups) - 1
	for gi := range groups {
		if groups[gi].Group == "cluster" {
			targetGi = gi
			break
		}
	}
	if targetGi < 0 {
		groups = append(groups, ast.DeploymentGroup{Group: "cluster"})
		targetGi = 0
	}
	groups[targetGi].Modules = append(groups[targetGi].Modules, ast.ModuleSpec{
		ID:       id,
		Source:   source,
		Export:   true,
		Tier:     4,
		AttachTo: attachTo,
		Settings: mergeSettingsDeep(nil, settings),
	})
	return groups
}

func mergeIntoPoolStartup(groups []ast.DeploymentGroup, pool string, source string, settings map[string]any) ([]ast.DeploymentGroup, error) {
	for gi := range groups {
		for mi := range groups[gi].Modules {
			m := groups[gi].Modules[mi]
			if m.Source == source && moduleBelongsToPool(m.ID, pool) {
				groups[gi].Modules[mi].Settings = mergeSettingsDeep(groups[gi].Modules[mi].Settings, settings)
				return groups, nil
			}
		}
	}

	// Pool has no startup-script module yet (e.g. cpu); create `{pool}_startup` in the pool's group.
	poolGi := -1
	for gi := range groups {
		for _, m := range groups[gi].Modules {
			if moduleBelongsToPool(m.ID, pool) {
				poolGi = gi
				break
			}
		}
		if poolGi >= 0 {
			break
		}
	}
	if poolGi < 0 {
		return groups, fmt.Errorf("cannot attach startup-script to pool %q: no module belongs to pool %q", pool, pool)
	}
	groups[poolGi].Modules = append(groups[poolGi].Modules, ast.ModuleSpec{
		ID:       pool + "_startup",
		Source:   source,
		Export:   true,
		Tier:     4,
		Settings: mergeSettingsDeep(nil, settings),
	})
	return groups, nil
}

// toAnySlice converts the slice representations yaml.v3 and Go literals may produce into []any.
func toAnySlice(v any) []any {
	switch s := v.(type) {
	case nil:
		return nil
	case []any:
		return s
	case []string:
		out := make([]any, len(s))
		for i, x := range s {
			out[i] = x
		}
		return out
	case []map[string]any:
		out := make([]any, len(s))
		for i, x := range s {
			out[i] = x
		}
		return out
	default:
		return nil
	}
}
