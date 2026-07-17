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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"
)

// =========================================================================
// ARCHETYPE HYDRATION ENGINE
// =========================================================================

// partitionArchetypeSettings inspects all modules in a DeploymentGroup (both ComputePool and Modules)
// and partitions user overrides:
//   - A setting is applied to any module that declares it as a valid input in variables.tf.
//   - If a setting is NOT declared by ANY module in the group (e.g., a typo or dummy test variable),
//     it is assigned to the primary ComputePool module so that ValidateModuleSettings can catch the error.
func partitionArchetypeSettings(mod ast.ModuleSpec, allMods []ast.ModuleSpec, overrides map[string]any, isPrimaryCompute bool, toolkitPath string) map[string]any {
	if len(overrides) == 0 {
		return nil
	}
	validForThisMod := getValidInputs(mod, toolkitPath)
	if validForThisMod == nil {
		if isPrimaryCompute {
			return overrides
		}
		return nil
	}

	// Build union of valid inputs across ALL other peer modules in the group
	validForAnyPeer := make(map[string]bool)
	for _, peer := range allMods {
		if peer.ID == mod.ID {
			continue
		}
		for k := range getValidInputs(peer, toolkitPath) {
			validForAnyPeer[k] = true
		}
	}

	result := make(map[string]any)
	for k, v := range overrides {
		if validForThisMod[k] {
			// This module explicitly supports key k
			result[k] = v
		} else if !validForAnyPeer[k] && isPrimaryCompute {
			// Key k is NOT supported by ANY module in the archetype (typo or test dummy setting).
			// Assign it to the primary compute module so ValidateModuleSettings catches the error!
			result[k] = v
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func resolveModuleSourcePath(source, toolkitPath string) string {
	if source == "" || toolkitPath == "" || filepath.IsAbs(source) {
		return source
	}
	// Embedded sources (prefixed with modules/ or community/modules/) must be
	// passed to modulereader verbatim so it reads them from ModuleFS. Prefixing
	// them with toolkitPath yields a local path to an embedded module, which
	// modulereader.GetModuleInfo rejects.
	if sourcereader.IsEmbeddedPath(source) && sourcereader.ModuleFS != nil {
		return source
	}
	candidate := filepath.Join(toolkitPath, source)
	if _, err := os.Stat(candidate); err == nil || sourcereader.ModuleFS == nil {
		return candidate
	}
	return source
}

func getValidInputs(mod ast.ModuleSpec, toolkitPath string) map[string]bool {
	if mod.Source == "" {
		return nil
	}
	fullPath := resolveModuleSourcePath(mod.Source, toolkitPath)
	modInfo, err := modulereader.GetModuleInfo(fullPath, "terraform")
	if err != nil {
		return nil
	}
	res := make(map[string]bool)
	for _, input := range modInfo.Inputs {
		res[input.Name] = true
	}
	return res
}

func getValidOutputs(mod ast.ModuleSpec, toolkitPath string) map[string]bool {
	if mod.Source == "" {
		return nil
	}
	fullPath := resolveModuleSourcePath(mod.Source, toolkitPath)
	modInfo, err := modulereader.GetModuleInfo(fullPath, "terraform")
	if err != nil {
		return nil
	}
	res := make(map[string]bool, len(modInfo.Outputs))
	for _, out := range modInfo.Outputs {
		res[out.Name] = true
	}
	return res
}

// HydrateArchetype takes a user's archetype request (e.g., 'a3u-primary') and the loaded
// static archetype manifest (e.g., 'a3-ultragpu-8g' YAML blueprint) and returns a structural AST.
//
// Its main architectural purpose is acting as a "Multiply and Inject" macro:
// 1. It copies the master blueprint.
// 2. It replaces `{name}` placeholders with the user's specific pool name (Namespace isolation).
// 3. It intelligently routes user overrides to the appropriate module using schema inspection.
func HydrateArchetype(ref ast.ArchetypeReference, manifest ast.ComputeArchetypeManifest, configBaseName string, toolkitPath string) ([]ast.DeploymentGroup, error) {
	baseBlock, ok := manifest.ConfigBase[configBaseName]
	if !ok {
		supported := make([]string, 0, len(manifest.ConfigBase))
		for b := range manifest.ConfigBase {
			supported = append(supported, b)
		}
		sort.Strings(supported)
		return nil, fmt.Errorf(
			"archetype %q does not support config_base %q (supported config_bases: %v)",
			manifest.Name, configBaseName, supported)
	}

	effectiveMachineType := resolveEffectiveMachineType(ref.MachineType, manifest)
	allGlobalMods, primaryComputeID, primaryValidInputs := collectPoolModulesAndPrimary(baseBlock.DeploymentGroups, ref.Name, effectiveMachineType, toolkitPath)
	flatSettings, submodSettings := splitSubmoduleSettings(ref.Settings, ref.Name, allGlobalMods, toolkitPath)

	var hydratedGroups []ast.DeploymentGroup
	for _, group := range baseBlock.DeploymentGroups {
		hydratedGroup := ast.DeploymentGroup{Group: group.Group}
		for _, mod := range group.Modules {
			hasPoolTemplate := strings.Contains(mod.ID, "{name}")
			mod = hydrateModule(mod, ref.Name, effectiveMachineType)
			if hasPoolTemplate {
				mod.PoolName = ref.Name
			}
			if mod.IsComputePool || moduleBelongsToPool(mod.ID, ref.Name) {
				isPrimaryCompute := (mod.ID == primaryComputeID)
				modOverrides := flatSettings
				if !isPrimaryCompute {
					modOverrides = filterSidecarSettings(flatSettings, primaryValidInputs)
				}
				validSettings := partitionArchetypeSettings(mod, allGlobalMods, modOverrides, isPrimaryCompute, toolkitPath)
				mod.Settings = mergeSettingsDeep(mod.Settings, hydrateSettings(validSettings, ref.Name, effectiveMachineType))
				if explicitSub, ok := submodSettings[mod.ID]; ok {
					mod.Settings = mergeSettingsDeep(mod.Settings, hydrateSettings(explicitSub, ref.Name, effectiveMachineType))
				}
			}
			hydratedGroup.Modules = append(hydratedGroup.Modules, mod)
		}
		hydratedGroups = append(hydratedGroups, hydratedGroup)
	}

	return hydratedGroups, nil
}

func resolveEffectiveMachineType(machineType string, manifest ast.ComputeArchetypeManifest) string {
	if machineType == manifest.Name {
		for _, pattern := range manifest.MatchPatterns {
			if !strings.Contains(pattern, "*") {
				return pattern
			}
		}
	}
	return machineType
}

func collectPoolModulesAndPrimary(groups []ast.DeploymentGroup, poolName, effectiveMachineType, toolkitPath string) ([]ast.ModuleSpec, string, map[string]bool) {
	var allGlobalMods []ast.ModuleSpec
	primaryComputeID := ""
	var primaryValidInputs map[string]bool
	for _, group := range groups {
		for _, mod := range group.Modules {
			hm := hydrateModule(mod, poolName, effectiveMachineType)
			if mod.IsComputePool || moduleBelongsToPool(hm.ID, poolName) {
				allGlobalMods = append(allGlobalMods, hm)
			}
			if mod.IsComputePool && primaryComputeID == "" {
				primaryComputeID = hm.ID
				primaryValidInputs = getValidInputs(hm, toolkitPath)
			}
		}
	}
	if primaryComputeID == "" && len(allGlobalMods) > 0 {
		primaryComputeID = allGlobalMods[0].ID
		primaryValidInputs = getValidInputs(allGlobalMods[0], toolkitPath)
	}
	return allGlobalMods, primaryComputeID, primaryValidInputs
}

func filterSidecarSettings(settings map[string]any, primaryValidInputs map[string]bool) map[string]any {
	if len(primaryValidInputs) == 0 || len(settings) == 0 {
		return settings
	}
	sidecar := make(map[string]any)
	for k, v := range settings {
		if !primaryValidInputs[k] {
			sidecar[k] = v
		}
	}
	return sidecar
}

// splitSubmoduleSettings separates flat module settings from nested sub-module maps inside `settings:`
// (e.g. `settings: { slurm_login: { machine_type: n2-standard-8 } }` or
// `settings: { checkpoints: { local_mount: /my-ckpt } }`).
func splitSubmoduleSettings(settings map[string]any, instanceName string, allMods []ast.ModuleSpec, toolkitPath string) (map[string]any, map[string]map[string]any) {
	if len(settings) == 0 {
		return nil, nil
	}
	allValidInputs := collectAllValidInputs(allMods, toolkitPath)
	flat := make(map[string]any, len(settings))
	submod := make(map[string]map[string]any)
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := settings[k]
		if subMap, ok := v.(map[string]any); ok {
			if modID, ok := matchSubmoduleTarget(k, subMap, instanceName, allMods, allValidInputs, toolkitPath); ok {
				submod[modID] = mergeSettingsDeep(submod[modID], subMap)
				continue
			}
		}
		flat[k] = v
	}
	return flat, submod
}

func collectAllValidInputs(allMods []ast.ModuleSpec, toolkitPath string) map[string]bool {
	allValidInputs := make(map[string]bool)
	for _, m := range allMods {
		for in := range getValidInputs(m, toolkitPath) {
			allValidInputs[in] = true
		}
	}
	return allValidInputs
}

func matchSubmoduleTarget(k string, subMap map[string]any, instanceName string, allMods []ast.ModuleSpec, allValidInputs map[string]bool, toolkitPath string) (string, bool) {
	resolvedKey := strings.ReplaceAll(k, "{name}", instanceName)
	var matchedMod *ast.ModuleSpec
	for i := range allMods {
		m := &allMods[i]
		if m.ID == k || m.ID == resolvedKey || (instanceName != "" && m.ID == instanceName+"_"+k) {
			matchedMod = m
			break
		}
	}
	if matchedMod == nil {
		return "", false
	}
	if !allValidInputs[k] {
		return matchedMod.ID, true
	}
	if len(subMap) == 0 {
		return "", false
	}
	modInputs := getValidInputs(*matchedMod, toolkitPath)
	if len(modInputs) == 0 {
		return "", false
	}
	for subKey := range subMap {
		if !modInputs[subKey] {
			return "", false
		}
	}
	return matchedMod.ID, true
}

// hydrateModule dynamically translates abstract template definitions into concrete implementation IDs.
// For example, if the blueprint dictates a module `id: {name}_partition`, and the user requested
// a pool named `a3u-primary`, this function rewrites the ID and all network/use bindings
// to exactly `a3u-primary_partition`.
func hydrateModule(mod ast.ModuleSpec, poolName, machineType string) ast.ModuleSpec {
	mod.ID = strings.ReplaceAll(mod.ID, "{name}", poolName)
	mod.Tier = 3

	var hydratedUse []string
	for _, u := range mod.Use {
		hydratedUse = append(hydratedUse, strings.ReplaceAll(u, "{name}", poolName))
	}
	mod.Use = hydratedUse

	mod.Settings = hydrateSettings(mod.Settings, poolName, machineType)

	// =========================================================================
	// EXPLICIT EXPORT ONLY (Architectural Fix)
	// =========================================================================
	// Previously we used an "Implicit Export Heuristic" here to automatically export
	// all modules starting with the poolName + "_", attempting to avoid Echo Output Collisions.
	// However, this brutally forced agnostic modules (like JBVM instances) to broadcast
	// generically named outputs (`name`), trapping Autowire in cyclical dependencies.
	//
	// We now strictly require blueprint authors to define `export: true` explicitly
	// in their archetype YAML ONLY for modules that truly need to broadcast to controllers
	// (like nodesets and partitions in Slurm).

	return mod
}

// hydrateSettings acts as a recursive dictionary crawler. Because settings in YAML can be deeply
// nested objects or arrays, this function walks the entire tree and executes string-templating
// against every possible terminal value.
func hydrateSettings(settings map[string]any, poolName, machineType string) map[string]any {
	if settings == nil {
		return nil
	}
	hydrated := make(map[string]any)
	for k, v := range settings {
		hydrated[k] = hydrateAny(v, poolName, machineType)
	}
	return hydrated
}

func hydrateAny(val any, poolName, machineType string) any {
	switch v := val.(type) {
	case string:
		// `{name}` plays two roles in a setting string:
		//   - a MODULE REFERENCE: `$({name}_bucket.x)` or `((module.{name}_instance.name))`.
		//     This must be the exact module ID, which keeps any `_` (a feature attached
		//     to pool "gpu" is instance "gpu_data", and pool scoping keys off "gpu_").
		//   - part of a CLOUD RESOURCE NAME: `$(vars.deployment_name)-{name}-lustre`,
		//     `{name}-bucket-pv`. GCE and Kubernetes reject `_` here, so it becomes `-`.
		s := strings.ReplaceAll(v, "$({name}", "$("+poolName)
		s = strings.ReplaceAll(s, "module.{name}", "module."+poolName)
		s = strings.ReplaceAll(s, "{name}", resourceSafeName(poolName))
		return strings.ReplaceAll(s, "{machine_type}", machineType)
	case map[string]any:
		return hydrateSettings(v, poolName, machineType)
	case []any:
		var arr []any
		for _, item := range v {
			arr = append(arr, hydrateAny(item, poolName, machineType))
		}
		return arr
	default:
		return v
	}
}

// resourceSafeName converts a module-ID-style name into one valid inside GCE and
// Kubernetes resource names, which allow `-` but not `_`.
func resourceSafeName(name string) string {
	return strings.ReplaceAll(name, "_", "-")
}

// substituteVarRefs rewrites `$(vars.<name>)` references inside the modules OWNED by
// poolName (IDs hydrated from `{name}`, i.e. "<pool>" or "<pool>_*") with the
// pool-scoped override value for <name>.
func substituteVarRefs(groups []ast.DeploymentGroup, poolName string, overrides map[string]any) {
	owned := func(id string) bool { return id == poolName || strings.HasPrefix(id, poolName+"_") }
	for gi := range groups {
		for mi := range groups[gi].Modules {
			if owned(groups[gi].Modules[mi].ID) {
				groups[gi].Modules[mi].Settings = substituteVarRefsInMap(groups[gi].Modules[mi].Settings, overrides)
			}
		}
	}
}

func substituteVarRefsInMap(settings map[string]any, overrides map[string]any) map[string]any {
	if settings == nil {
		return nil
	}
	out := make(map[string]any, len(settings))
	for k, v := range settings {
		out[k] = substituteVarRefsInAny(v, overrides)
	}
	return out
}

func substituteVarRefsInAny(val any, overrides map[string]any) any {
	switch v := val.(type) {
	case string:
		keys := make([]string, 0, len(overrides))
		for k := range overrides {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, name := range keys {
			override := overrides[name]
			ref := "$(vars." + name + ")"
			if v == ref {
				return override
			}
			if strings.Contains(v, ref) {
				v = strings.ReplaceAll(v, ref, fmt.Sprintf("%v", override))
			}
		}
		return v
	case map[string]any:
		return substituteVarRefsInMap(v, overrides)
	case []any:
		arr := make([]any, len(v))
		for i, item := range v {
			arr[i] = substituteVarRefsInAny(item, overrides)
		}
		return arr
	default:
		return v
	}
}

// mergeSettingsDeep recursively merges overrides into the base map.
func mergeSettingsDeep(base, overrides map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	if overrides == nil {
		return base
	}

	res := make(map[string]any)
	for k, v := range base {
		res[k] = v
	}

	for k, v := range overrides {
		if vMap, ok := v.(map[string]any); ok {
			if bMap, ok := res[k].(map[string]any); ok {
				res[k] = mergeSettingsDeep(bMap, vMap)
				continue
			}
		}
		if existing, hasExisting := res[k]; hasExisting && isScriptEntrySlice(existing, v) {
			res[k] = appendUniqueRunners(toAnySlice(existing), toAnySlice(v))
			continue
		}
		res[k] = v
	}
	return res
}

func isScriptEntrySlice(a, b any) bool {
	sa, sb := toAnySlice(a), toAnySlice(b)
	if sa == nil || sb == nil {
		return false
	}
	for _, s := range [][]any{sa, sb} {
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				for _, key := range []string{"destination", "content", "filename", "source"} {
					if _, has := m[key]; has {
						return true
					}
				}
			}
		}
	}
	return false
}

// appendUniqueRunners appends slice items from overrides to base without duplicating
// identical runner definitions, and disambiguates duplicate destination names when
// content differs across pools (satisfying startup-script's unique destination constraint).
func appendUniqueRunners(base, incoming []any) []any {
	out := append([]any{}, base...)
	seenRepr := make(map[string]bool, len(out))
	seenDest := make(map[string]int, len(out))
	for _, item := range out {
		seenRepr[fmt.Sprintf("%#v", item)] = true
		if m, ok := item.(map[string]any); ok {
			if dest, ok := m["destination"].(string); ok && dest != "" {
				seenDest[dest]++
			}
		}
	}
	for _, item := range incoming {
		repr := fmt.Sprintf("%#v", item)
		if seenRepr[repr] {
			continue
		}
		if m, ok := item.(map[string]any); ok {
			item, repr = disambiguateRunnerDestination(m, item, repr, seenDest)
		}
		seenRepr[repr] = true
		out = append(out, item)
	}
	return out
}

func disambiguateRunnerDestination(m map[string]any, item any, repr string, seenDest map[string]int) (any, string) {
	dest, ok := m["destination"].(string)
	if !ok || dest == "" {
		return item, repr
	}
	if seenDest[dest] == 0 {
		seenDest[dest]++
		return item, repr
	}
	cloned := make(map[string]any, len(m))
	for mk, mv := range m {
		cloned[mk] = mv
	}
	ext := filepath.Ext(dest)
	stem := strings.TrimSuffix(dest, ext)
	for idx := 2; ; idx++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, idx, ext)
		if seenDest[candidate] == 0 {
			seenDest[candidate] = 1
			cloned["destination"] = candidate
			break
		}
	}
	return cloned, fmt.Sprintf("%#v", cloned)
}
