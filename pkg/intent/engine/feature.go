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
	"regexp"
	"sort"
	"strings"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/modulereader"

	"github.com/agext/levenshtein"
	"github.com/google/cel-go/cel"
)

// =========================================================================
// FEATURE ENGINE (The Blue/Green Decorators)
// =========================================================================

type prunedFeatureGroup struct {
	Group   string
	Modules []ast.ModuleSpec
}

type featurePrimaryInfo struct {
	primaryID           string
	primaryValidInputs  map[string]bool
	sidecarProducedKeys map[string]bool
	primaryDefaultMount string
}

// EvaluateFeature dynamically evaluates Feature macros.
// It conditionally prunes modules using CEL, hydrates {name} placeholders for namespace isolation,
// and injects user overrides into the primary "{name}" / "{name}_*" module and any secondary modules.
func EvaluateFeature(ref ast.FeatureReference, manifest ast.FeatureManifest, vars map[string]any, configBaseName string) ([]ast.DeploymentGroup, error) {
	env, evalCtx, err := buildFeatureCELEnv(ref, manifest, vars, configBaseName)
	if err != nil {
		return nil, err
	}
	if err := evaluateFeatureAssertions(env, evalCtx, ref, manifest, configBaseName); err != nil {
		return nil, err
	}
	groups, err := selectFeatureGroups(ref, manifest, configBaseName)
	if err != nil {
		return nil, err
	}

	isPerPoolWorkload := isPerPoolWorkloadConsumer(groups)
	prefixes := []string{""}
	if isPerPoolWorkload && len(ref.AttachTo) > 0 {
		prefixes = ref.AttachTo
	}

	var finalGroups []ast.DeploymentGroup
	for _, prefix := range prefixes {
		prefixGroups, err := evaluateFeatureInstance(prefix, prefixes, isPerPoolWorkload, ref, groups, env, evalCtx)
		if err != nil {
			return nil, err
		}
		finalGroups = append(finalGroups, prefixGroups...)
	}
	return finalGroups, nil
}

func findFeaturePrimarySource(ref ast.FeatureReference, manifest ast.FeatureManifest, configBaseName string) string {
	candidateGroups := append([]ast.DeploymentGroup{}, manifest.DeploymentGroups...)
	if baseBlock, ok := manifest.ConfigBase[configBaseName]; ok {
		candidateGroups = append(candidateGroups, baseBlock.DeploymentGroups...)
	}
	for _, g := range candidateGroups {
		for _, m := range g.Modules {
			if strings.ReplaceAll(m.ID, "{name}", ref.Name) == ref.Name {
				return m.Source
			}
		}
	}
	for _, g := range candidateGroups {
		for _, m := range g.Modules {
			resolvedID := strings.ReplaceAll(m.ID, "{name}", ref.Name)
			if strings.HasPrefix(resolvedID, ref.Name+"_") && m.Source != "" {
				return m.Source
			}
		}
	}
	return ""
}

func buildFeatureMergedSettings(ref ast.FeatureReference, manifest ast.FeatureManifest, configBaseName string) map[string]any {
	mergedSettings := make(map[string]any)
	if primarySource := findFeaturePrimarySource(ref, manifest, configBaseName); primarySource != "" {
		if info, err := modulereader.GetModuleInfo(primarySource, "terraform"); err == nil {
			for _, in := range info.Inputs {
				if in.Default != nil {
					mergedSettings[in.Name] = in.Default
				}
			}
		}
	}
	for k, v := range manifest.Vars {
		mergedSettings[k] = v
	}
	if baseBlock, ok := manifest.ConfigBase[configBaseName]; ok {
		for k, v := range baseBlock.Vars {
			mergedSettings[k] = v
		}
	}
	for k, v := range ref.Settings {
		mergedSettings[k] = v
	}
	return mergedSettings
}

func buildFeatureCELEnv(ref ast.FeatureReference, manifest ast.FeatureManifest, vars map[string]any, configBaseName string) (*cel.Env, map[string]any, error) {
	mergedSettings := buildFeatureMergedSettings(ref, manifest, configBaseName)
	evalCtx := map[string]any{
		"settings": mergedSettings,
		"vars":     vars,
	}
	for k, v := range vars {
		evalCtx[k] = v
	}
	for k, v := range mergedSettings {
		evalCtx[k] = v
	}
	ctxKeys := make([]string, 0, len(evalCtx))
	for k := range evalCtx {
		ctxKeys = append(ctxKeys, k)
	}
	sort.Strings(ctxKeys)
	opts := make([]cel.EnvOption, 0, len(ctxKeys))
	for _, k := range ctxKeys {
		opts = append(opts, cel.Variable(k, cel.DynType))
	}
	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CEL env: %w", err)
	}
	return env, evalCtx, nil
}

func evaluateFeatureAssertions(env *cel.Env, evalCtx map[string]any, ref ast.FeatureReference, manifest ast.FeatureManifest, configBaseName string) error {
	allAssertions := append([]ast.FeatureAssertion{}, manifest.Assertions...)
	if baseBlock, ok := manifest.ConfigBase[configBaseName]; ok {
		allAssertions = append(allAssertions, baseBlock.Assertions...)
	}
	for _, assertion := range allAssertions {
		astExpr, iss := env.Compile(assertion.Rule)
		if iss.Err() != nil {
			return fmt.Errorf("compile assertion failed: %w", iss.Err())
		}
		prg, err := env.Program(astExpr)
		if err != nil {
			return fmt.Errorf("failed to create program for assertion: %w", err)
		}
		out, _, err := prg.Eval(evalCtx)
		if err != nil {
			return fmt.Errorf("failed to evaluate assertion: %w", err)
		}
		if val, ok := out.Value().(bool); !ok || !val {
			return fmt.Errorf("feature '%s' validation failed: %s", ref.Name, assertion.Message)
		}
	}
	return nil
}

func selectFeatureGroups(ref ast.FeatureReference, manifest ast.FeatureManifest, configBaseName string) ([]ast.DeploymentGroup, error) {
	groups := copyDeploymentGroups(manifest.DeploymentGroups)
	if baseBlock, ok := manifest.ConfigBase[configBaseName]; ok {
		return mergeGroups(groups, copyDeploymentGroups(baseBlock.DeploymentGroups)), nil
	}
	if len(manifest.DeploymentGroups) == 0 && len(manifest.ConfigBase) > 0 {
		var supported []string
		for base := range manifest.ConfigBase {
			supported = append(supported, base)
		}
		sort.Strings(supported)
		return nil, fmt.Errorf("feature %q does not support config_base %q (supported config_bases: %v)", ref.Type, configBaseName, supported)
	}
	return groups, nil
}

func evaluateFeatureInstance(prefix string, prefixes []string, isPerPoolWorkload bool, ref ast.FeatureReference, groups []ast.DeploymentGroup, env *cel.Env, evalCtx map[string]any) ([]ast.DeploymentGroup, error) {
	instanceName := ref.Name
	if prefix != "" {
		instanceName = prefix + "_" + ref.Name
	}

	preGroups, allHydratedMods, conditionKeys, err := pruneAndHydrateFeatureGroups(instanceName, groups, env, evalCtx)
	if err != nil {
		return nil, err
	}

	flatSettings, submodSettings, err := splitAndValidateFeatureSettings(ref, instanceName, allHydratedMods, conditionKeys)
	if err != nil {
		return nil, err
	}

	pInfo := resolveFeaturePrimaryInfo(instanceName, allHydratedMods)

	var outGroups []ast.DeploymentGroup
	for _, pg := range preGroups {
		hydratedGroup := ast.DeploymentGroup{Group: pg.Group}
		for _, mod := range pg.Modules {
			mod = decorateFeatureModule(mod, instanceName, prefix, prefixes, isPerPoolWorkload, ref, pInfo, flatSettings, submodSettings, allHydratedMods)
			hydratedGroup.Modules = append(hydratedGroup.Modules, mod)
		}
		outGroups = append(outGroups, hydratedGroup)
	}
	return outGroups, nil
}

func pruneAndHydrateFeatureGroups(instanceName string, groups []ast.DeploymentGroup, env *cel.Env, evalCtx map[string]any) ([]prunedFeatureGroup, []ast.ModuleSpec, map[string]bool, error) {
	var allHydratedMods []ast.ModuleSpec
	prunedByCondition := make(map[string]bool)
	conditionKeys := make(map[string]bool)
	var preGroups []prunedFeatureGroup

	for _, group := range groups {
		pg := prunedFeatureGroup{Group: group.Group}
		for _, mod := range group.Modules {
			keep, err := evaluateModuleCondition(mod.Condition, env, evalCtx, conditionKeys)
			if err != nil {
				return nil, nil, nil, err
			}
			hydratedMod := hydrateModule(mod, instanceName, "")
			if !keep {
				prunedByCondition[hydratedMod.ID] = true
				continue
			}
			pg.Modules = append(pg.Modules, hydratedMod)
			allHydratedMods = append(allHydratedMods, hydratedMod)
		}
		if len(pg.Modules) > 0 {
			preGroups = append(preGroups, pg)
		}
	}

	if len(prunedByCondition) > 0 {
		stripPrunedUses(preGroups, allHydratedMods, prunedByCondition)
	}
	return preGroups, allHydratedMods, conditionKeys, nil
}

func evaluateModuleCondition(cond string, env *cel.Env, evalCtx map[string]any, conditionKeys map[string]bool) (bool, error) {
	if cond == "" {
		return true, nil
	}
	for _, k := range conditionSettingKeys(cond) {
		conditionKeys[k] = true
	}
	astExpr, iss := env.Compile(cond)
	if iss.Err() != nil {
		return false, fmt.Errorf("failed to compile condition %q: %w", cond, iss.Err())
	}
	prg, err := env.Program(astExpr)
	if err != nil {
		return false, fmt.Errorf("failed to create program for %q: %w", cond, err)
	}
	out, _, err := prg.Eval(evalCtx)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate condition %q: %w", cond, err)
	}
	return out.Value() == true, nil
}

func stripPrunedUses(preGroups []prunedFeatureGroup, allHydratedMods []ast.ModuleSpec, pruned map[string]bool) {
	strip := func(uses []string) []string {
		var clean []string
		for _, u := range uses {
			if !pruned[u] {
				clean = append(clean, u)
			}
		}
		return clean
	}
	for gi := range preGroups {
		for mi := range preGroups[gi].Modules {
			preGroups[gi].Modules[mi].Use = strip(preGroups[gi].Modules[mi].Use)
		}
	}
	for mi := range allHydratedMods {
		allHydratedMods[mi].Use = strip(allHydratedMods[mi].Use)
	}
}

func splitAndValidateFeatureSettings(ref ast.FeatureReference, instanceName string, allHydratedMods []ast.ModuleSpec, conditionKeys map[string]bool) (map[string]any, map[string]map[string]any, error) {
	flatSettings, submodSettings := splitSubmoduleSettings(ref.Settings, instanceName, allHydratedMods, "")
	flatRef := ref
	flatRef.Settings = flatSettings
	if err := validateSettingKeys(flatRef, allHydratedMods, conditionKeys); err != nil {
		return nil, nil, err
	}
	for subID, subMap := range submodSettings {
		for _, m := range allHydratedMods {
			if m.ID == subID {
				subRef := ast.FeatureReference{Name: subID, Type: ref.Type, Settings: subMap}
				if err := validateSettingKeys(subRef, []ast.ModuleSpec{m}, conditionKeys); err != nil {
					return nil, nil, err
				}
				break
			}
		}
	}
	return flatSettings, submodSettings, nil
}

func resolveFeaturePrimaryInfo(instanceName string, allHydratedMods []ast.ModuleSpec) featurePrimaryInfo {
	var info featurePrimaryInfo
	for _, m := range allHydratedMods {
		if m.ID == instanceName {
			info.primaryID = m.ID
			break
		}
	}
	if info.primaryID == "" {
		for _, m := range allHydratedMods {
			if strings.HasPrefix(m.ID, instanceName+"_") && len(getValidInputs(m, "")) > 0 {
				info.primaryID = m.ID
				break
			}
		}
	}
	var primaryUse []string
	for _, m := range allHydratedMods {
		if m.ID == info.primaryID {
			info.primaryValidInputs = getValidInputs(m, "")
			primaryUse = m.Use
			info.primaryDefaultMount, _ = m.Settings["local_mount"].(string)
			break
		}
	}
	info.sidecarProducedKeys = collectSidecarProducedKeys(primaryUse, info.primaryValidInputs, allHydratedMods)
	return info
}

func collectSidecarProducedKeys(primaryUse []string, primaryValidInputs map[string]bool, allHydratedMods []ast.ModuleSpec) map[string]bool {
	sidecarProducedKeys := make(map[string]bool)
	for _, usedID := range primaryUse {
		for _, m := range allHydratedMods {
			if m.ID == usedID {
				sIn := getValidInputs(m, "")
				sOut := getValidOutputs(m, "")
				for k := range sIn {
					if sOut[k] && primaryValidInputs[k] {
						sidecarProducedKeys[k] = true
					}
				}
				break
			}
		}
	}
	return sidecarProducedKeys
}

func filterFeatureSettingsForMod(isPrimary bool, pInfo featurePrimaryInfo, settings map[string]any) map[string]any {
	if pInfo.primaryID == "" || len(pInfo.primaryValidInputs) == 0 || len(settings) == 0 {
		return settings
	}
	if isPrimary {
		if len(pInfo.sidecarProducedKeys) == 0 {
			return settings
		}
		primarySettings := make(map[string]any, len(settings))
		for k, v := range settings {
			if !pInfo.sidecarProducedKeys[k] {
				primarySettings[k] = v
			}
		}
		return primarySettings
	}
	sidecarSettings := make(map[string]any)
	for k, v := range settings {
		if !pInfo.primaryValidInputs[k] || pInfo.sidecarProducedKeys[k] {
			sidecarSettings[k] = v
		}
	}
	return sidecarSettings
}

func decorateFeatureModule(mod ast.ModuleSpec, instanceName, prefix string, prefixes []string, isPerPoolWorkload bool, ref ast.FeatureReference, pInfo featurePrimaryInfo, flatSettings map[string]any, submodSettings map[string]map[string]any, allHydratedMods []ast.ModuleSpec) ast.ModuleSpec {
	isPrimary := (pInfo.primaryID != "" && mod.ID == pInfo.primaryID)
	forcePrimaryFallback := isPrimary && mod.ID == instanceName
	modOverrides := filterFeatureSettingsForMod(isPrimary, pInfo, flatSettings)
	validSettings := partitionArchetypeSettings(mod, allHydratedMods, modOverrides, forcePrimaryFallback, "")
	mod.Settings = mergeSettingsDeep(mod.Settings, hydrateSettings(validSettings, instanceName, ""))
	rewriteDerivedSidecarMount(&mod, isPrimary, pInfo.primaryDefaultMount, flatSettings)
	if explicitSub, ok := submodSettings[mod.ID]; ok {
		mod.Settings = mergeSettingsDeep(mod.Settings, hydrateSettings(explicitSub, instanceName, ""))
	}
	mod.FeatureInstanceID = instanceName
	mod.Tier = 4
	if isPerPoolWorkload {
		mod.Tier = 5
		rewritePerPoolWorkloadName(&mod, instanceName, prefix, prefixes, ref.Name)
	}
	if prefix != "" {
		mod.AttachTo = []string{prefix}
	} else if len(ref.AttachTo) > 0 {
		mod.AttachTo = append([]string(nil), ref.AttachTo...)
	}
	return mod
}

func rewriteDerivedSidecarMount(mod *ast.ModuleSpec, isPrimary bool, primaryDefaultMount string, flatSettings map[string]any) {
	if isPrimary || primaryDefaultMount == "" {
		return
	}
	customMount, ok := flatSettings["local_mount"].(string)
	if !ok || customMount == "" || customMount == primaryDefaultMount {
		return
	}
	if curMount, ok := mod.Settings["local_mount"].(string); ok && strings.HasPrefix(curMount, primaryDefaultMount+"-") {
		baseMount := strings.TrimRight(customMount, "/")
		mod.Settings["local_mount"] = baseMount + strings.TrimPrefix(curMount, primaryDefaultMount)
	}
}

func rewritePerPoolWorkloadName(mod *ast.ModuleSpec, instanceName, prefix string, prefixes []string, refName string) {
	jobName, ok := mod.Settings["name"].(string)
	if !ok || (jobName != instanceName && jobName != resourceSafeName(instanceName)) {
		return
	}
	if len(prefixes) == 1 {
		mod.Settings["name"] = resourceSafeName(refName)
	} else {
		mod.Settings["name"] = resourceSafeName(prefix + "-" + refName)
	}
}

func isPerPoolWorkloadConsumer(groups []ast.DeploymentGroup) bool {
	for _, g := range groups {
		for _, m := range g.Modules {
			inputs := getValidInputs(m, "")
			if inputs["node_pool_names"] && inputs["persistent_volume_claims"] {
				return true
			}
		}
	}
	return false
}

// celStringLiteral matches single- or double-quoted CEL string literals, which are
// stripped before identifier extraction so that a value such as 'PRIVATE' is not
// mistaken for a settings name.
var celStringLiteral = regexp.MustCompile(`'[^']*'|"[^"]*"`)

// celIdentifier matches a bare identifier, optionally qualified by `settings.`.
var celIdentifier = regexp.MustCompile(`(?:settings\.)?([A-Za-z_][A-Za-z0-9_]*)`)

// celReserved are tokens that look like identifiers but never name a setting.
var celReserved = map[string]bool{
	"true": true, "false": true, "null": true,
	"has": true, "in": true, "settings": true, "vars": true,
}

// conditionSettingKeys returns the settings names a CEL condition reads.
// Over-approximates to ensure gating flags are not rejected as unknown.
func conditionSettingKeys(expr string) []string {
	cleaned := celStringLiteral.ReplaceAllString(expr, " ")
	var keys []string
	for _, m := range celIdentifier.FindAllStringSubmatch(cleaned, -1) {
		if name := m[1]; !celReserved[name] {
			keys = append(keys, name)
		}
	}
	return keys
}

// maxSettingHintDist is the Levenshtein radius within which an unknown settings key is
// reported as a probable typo of a known one. Mirrors pkg/config's maxHintDist.
const maxSettingHintDist = 3

// validateSettingKeys fails fast on unknown settings keys to prevent silent drops.
// Fails OPEN unless all sourced modules yield a schema. conditionKeys are treated
// as known inputs to allow gating modules off.
func validateSettingKeys(ref ast.FeatureReference, mods []ast.ModuleSpec, conditionKeys map[string]bool) error {
	if len(ref.Settings) == 0 {
		return nil
	}

	known, ok := collectKnownSettingKeys(mods, conditionKeys)
	if !ok || len(known) == 0 {
		return nil
	}

	var unknown []string
	for k := range ref.Settings {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	accepted := sortedKeys(known)
	reports := formatUnknownKeyReports(unknown, accepted)
	return fmt.Errorf("%q sets unknown key(s) %s", ref.Type, strings.Join(reports, ", "))
}

func collectKnownSettingKeys(mods []ast.ModuleSpec, conditionKeys map[string]bool) (map[string]bool, bool) {
	known := make(map[string]bool)
	for k := range conditionKeys {
		known[k] = true
	}
	for _, m := range mods {
		if m.Source == "" {
			continue
		}
		inputs := getValidInputs(m, "")
		if len(inputs) == 0 {
			return nil, false
		}
		for k := range inputs {
			known[k] = true
		}
	}
	return known, true
}

func formatUnknownKeyReports(unknown, accepted []string) []string {
	reports := make([]string, 0, len(unknown))
	for _, k := range unknown {
		best, minDist := "", maxSettingHintDist+1
		for _, cand := range accepted {
			if d := levenshtein.Distance(k, cand, nil); d < minDist {
				best, minDist = cand, d
			}
		}
		if minDist <= maxSettingHintDist {
			reports = append(reports, fmt.Sprintf("%q (did you mean %q?)", k, best))
		} else {
			reports = append(reports, fmt.Sprintf("%q", k))
		}
	}
	return reports
}
