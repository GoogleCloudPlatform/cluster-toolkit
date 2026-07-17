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
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/intent/ast"

	"google.golang.org/api/googleapi"
)

var (
	validMachineTypePattern = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)
	machineTypeNotFound     sync.Map
	unverifiableProjectZone sync.Map
)

// Compiler coordinates the loading, hydration, merging, and validation of user intent.
type Compiler struct {
	loader *CatalogLoader
}

// NewCompiler creates a new Compiler instance.
func NewCompiler(toolkitPath string) *Compiler {
	return &Compiler{
		loader: NewCatalogLoader(toolkitPath),
	}
}

type compileVarState struct {
	mergedVars             map[string]any
	userDeclaredVars       map[string]bool
	promotedUnscopedVars   map[string]bool
	settingsOverriddenVars map[string]bool
	explicitDefaultPools   map[string]bool
	conflictingArchVars    map[string]bool
}

type poolVarMeta struct {
	vars        map[string]any
	setSettings map[string]bool
	validInputs map[string]bool
}

// Compile takes a ClusterConfig and transforms it into a list of fully hydrated, validated DeploymentGroups.
func (c *Compiler) Compile(userConfig ast.ClusterConfig) ([]ast.DeploymentGroup, error) {
	userDeclaredVars, err := c.validateAndPrepareConfig(&userConfig)
	if err != nil {
		return nil, err
	}

	baseManifest, finalGroups, allBaseMods, baseModIDs, state, err := c.initBaseAndVarState(userConfig, userDeclaredVars)
	if err != nil {
		return nil, err
	}

	finalGroups, err = c.processComputeArchetypes(&userConfig, finalGroups, state)
	if err != nil {
		return nil, err
	}

	if err := c.applyConfigBaseSettings(userConfig, baseManifest, allBaseMods, baseModIDs, finalGroups, state); err != nil {
		return nil, err
	}

	mergeUserClusterConfigVars(userConfig.Vars, state)
	normalizeMultiPoolAndFlexStart(finalGroups, c.loader.basePath, len(userConfig.ComputeArchetypes), state.explicitDefaultPools, state.mergedVars, state.userDeclaredVars)

	poolNames, err := extractAndValidatePoolNames(userConfig.ComputeArchetypes)
	if err != nil {
		return nil, err
	}

	finalGroups, err = c.processFeatures(&userConfig, finalGroups, state)
	if err != nil {
		return nil, err
	}

	finalGroups, err = mergeStartupScripts(finalGroups, poolNames)
	if err != nil {
		return nil, err
	}
	normalizePostFeatureGKE(finalGroups, c.loader.basePath, state.mergedVars, state.userDeclaredVars)
	if err := validatePoolConsumptionSettings(finalGroups, state.mergedVars, c.loader.basePath); err != nil {
		return nil, err
	}

	return c.finalizeGraphAndVars(&userConfig, baseManifest, finalGroups, state)
}

func (c *Compiler) validateAndPrepareConfig(userConfig *ast.ClusterConfig) (map[string]bool, error) {
	if err := validateMandatoryConfigFields(*userConfig); err != nil {
		return nil, err
	}
	if err := validateFeatureRefs(userConfig.Features); err != nil {
		return nil, err
	}
	if err := validateOverlayRefs(userConfig.Overlays); err != nil {
		return nil, err
	}

	userDeclaredVars := make(map[string]bool, len(userConfig.Vars))
	for k, v := range userConfig.Vars {
		if v != nil {
			userDeclaredVars[k] = true
		}
	}

	if err := c.applyOverlays(userConfig); err != nil {
		return nil, err
	}
	seenArchetypes, err := c.validateArchetypeRefs(userConfig.ComputeArchetypes)
	if err != nil {
		return nil, err
	}
	if err := c.validateFeatureAttachments(userConfig.ConfigBase.Type, userConfig.Features, seenArchetypes); err != nil {
		return nil, err
	}
	if err := c.validateSingleAcceleratorArchetype(userConfig.ComputeArchetypes); err != nil {
		return nil, err
	}
	return userDeclaredVars, nil
}

func validateMandatoryConfigFields(userConfig ast.ClusterConfig) error {
	if userConfig.ConfigBase.Type == "" {
		return fmt.Errorf("`config_base` is required: pick one of gke, jbvm, slurm")
	}
	if v, ok := userConfig.Vars["project_id"]; !ok || v == nil || v == "" {
		return fmt.Errorf(
			"`vars.project_id` is required and has no default; set it to the GCP project that will own this deployment")
	}
	if len(userConfig.ComputeArchetypes) == 0 {
		return fmt.Errorf(
			"`compute_archetypes` is required: declare at least one compute pool, " +
				"otherwise the cluster has no nodes to run work on")
	}
	return nil
}

func validateFeatureRefs(features []ast.FeatureReference) error {
	seen := make(map[string]bool, len(features))
	for _, f := range features {
		if f.Name == "" {
			return fmt.Errorf("feature name cannot be empty")
		}
		if !validPoolNamePattern.MatchString(f.Name) {
			return fmt.Errorf("invalid feature name %q: must start with a letter and contain only letters, digits, underscores, or hyphens", f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("duplicate feature name %q found", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}

func validateOverlayRefs(overlays []ast.FeatureReference) error {
	seen := make(map[string]bool, len(overlays))
	for _, o := range overlays {
		if o.Name == "" {
			return fmt.Errorf("overlay name cannot be empty")
		}
		if !validPoolNamePattern.MatchString(o.Name) {
			return fmt.Errorf("invalid overlay name %q: must start with a letter and contain only letters, digits, underscores, or hyphens", o.Name)
		}
		if len(o.Settings) > 0 {
			return fmt.Errorf("overlay %q does not accept inline settings in cluster config; configure settings inside the overlay manifest", o.Name)
		}
		if seen[o.Name] {
			return fmt.Errorf("duplicate overlay name %q found", o.Name)
		}
		seen[o.Name] = true
	}
	return nil
}

func (c *Compiler) validateArchetypeRefs(archetypes []ast.ArchetypeReference) (map[string]bool, error) {
	seen := make(map[string]bool, len(archetypes))
	for _, a := range archetypes {
		if a.Name == "" {
			return nil, fmt.Errorf("compute archetype name cannot be empty")
		}
		mt := strings.TrimSpace(a.MachineType)
		if mt == "" {
			return nil, fmt.Errorf("compute archetype %q is missing required field `machine_type`", a.Name)
		}
		if c.isInvalidFallbackMachineTypeName(mt) {
			return nil, fmt.Errorf("compute archetype %q: invalid machine_type %q: specify a concrete Google Cloud CPU machine type (e.g. \"n2-standard-8\", \"c2-standard-60\", \"h3-standard-88\")", a.Name, mt)
		}
		if seen[a.Name] {
			return nil, fmt.Errorf("duplicate compute archetype name %q found", a.Name)
		}
		seen[a.Name] = true
	}
	return seen, nil
}

func (c *Compiler) isInvalidFallbackMachineTypeName(mt string) bool {
	arch, err := c.loader.LoadArchetype(mt)
	return err == nil && isFallbackArchetype(arch) && (mt == arch.Name || strings.HasSuffix(mt, "-"+arch.Name))
}

func (c *Compiler) validateFeatureAttachments(configBaseType string, features []ast.FeatureReference, seenArchetypes map[string]bool) error {
	for _, f := range features {
		if seenArchetypes[f.Name] {
			return fmt.Errorf("feature %q conflicts with compute archetype of the same name %q", f.Name, f.Name)
		}
		for _, attachedPool := range f.AttachTo {
			if attachedPool == "" {
				return fmt.Errorf("feature %q has an empty attach_to pool string segment", f.Name)
			}
			if !seenArchetypes[attachedPool] && !c.isRoleTarget(configBaseType, attachedPool) {
				return fmt.Errorf("feature %q is attached to undefined pool %q", f.Name, attachedPool)
			}
		}
	}
	return nil
}

func (c *Compiler) initBaseAndVarState(userConfig ast.ClusterConfig, userDeclaredVars map[string]bool) (*ast.ConfigBaseManifest, []ast.DeploymentGroup, []ast.ModuleSpec, map[string]bool, *compileVarState, error) {
	baseManifest, err := c.loader.LoadConfigBase(userConfig.ConfigBase.Type)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to load config base: %w", err)
	}

	finalGroups := copyDeploymentGroups(baseManifest.DeploymentGroups)
	var allBaseMods []ast.ModuleSpec
	baseModIDs := make(map[string]bool)
	for i := range finalGroups {
		for j := range finalGroups[i].Modules {
			finalGroups[i].Modules[j].Tier = 2
			m := finalGroups[i].Modules[j]
			allBaseMods = append(allBaseMods, m)
			baseModIDs[m.ID] = true
		}
	}

	mergedVars := make(map[string]any)
	for k, v := range baseManifest.Vars {
		mergedVars[k] = v
	}

	state := &compileVarState{
		mergedVars:             mergedVars,
		userDeclaredVars:       userDeclaredVars,
		promotedUnscopedVars:   make(map[string]bool),
		settingsOverriddenVars: make(map[string]bool),
		explicitDefaultPools:   make(map[string]bool),
		conflictingArchVars:    c.detectConflictingArchetypeVars(userConfig),
	}
	return baseManifest, finalGroups, allBaseMods, baseModIDs, state, nil
}

func (c *Compiler) detectConflictingArchetypeVars(userConfig ast.ClusterConfig) map[string]bool {
	firstArchVarVal := make(map[string]any)
	conflictingArchVars := make(map[string]bool)
	var poolMetas []poolVarMeta

	for _, archRef := range userConfig.ComputeArchetypes {
		archManifest, err := c.loader.LoadArchetypeForBase(archRef.MachineType, userConfig.ConfigBase.Type)
		if err != nil {
			continue
		}
		poolMetas = append(poolMetas, c.buildPoolVarMeta(archRef, archManifest, userConfig.ConfigBase.Type, firstArchVarVal, conflictingArchVars))
	}

	markAsymmetricPoolVarConflicts(poolMetas, conflictingArchVars)
	return conflictingArchVars
}

func (c *Compiler) buildPoolVarMeta(archRef ast.ArchetypeReference, archManifest *ast.ComputeArchetypeManifest, configBaseType string, firstArchVarVal map[string]any, conflictingArchVars map[string]bool) poolVarMeta {
	meta := poolVarMeta{
		vars:        make(map[string]any),
		setSettings: make(map[string]bool),
		validInputs: make(map[string]bool),
	}
	checkDefault := func(k string, v any) {
		meta.vars[k] = v
		if prev, ok := firstArchVarVal[k]; ok {
			if !reflect.DeepEqual(prev, v) {
				conflictingArchVars[k] = true
			}
		} else {
			firstArchVarVal[k] = v
		}
	}
	for k, v := range archManifest.Vars {
		checkDefault(k, v)
	}
	if baseBlock, ok := archManifest.ConfigBase[configBaseType]; ok {
		for k, v := range baseBlock.Vars {
			checkDefault(k, v)
		}
		collectPoolTemplateInputs(baseBlock.DeploymentGroups, c.loader.basePath, &meta)
	}
	for k := range archRef.Settings {
		meta.setSettings[k] = true
	}
	return meta
}

func collectPoolTemplateInputs(groups []ast.DeploymentGroup, basePath string, meta *poolVarMeta) {
	for _, g := range groups {
		for _, m := range g.Modules {
			if !strings.Contains(m.ID, "{name}") {
				continue
			}
			for k := range m.Settings {
				meta.setSettings[k] = true
			}
			for k := range getValidInputs(m, basePath) {
				meta.validInputs[k] = true
			}
		}
	}
}

func markAsymmetricPoolVarConflicts(poolMetas []poolVarMeta, conflictingArchVars map[string]bool) {
	if len(poolMetas) <= 1 {
		return
	}
	for i, mA := range poolMetas {
		for k := range mA.vars {
			for j, mB := range poolMetas {
				if i != j && mB.vars[k] == nil && mB.validInputs[k] && !mB.setSettings[k] {
					if _, hasVar := mB.vars[k]; !hasVar {
						conflictingArchVars[k] = true
					}
				}
			}
		}
	}
}

func (c *Compiler) processComputeArchetypes(userConfig *ast.ClusterConfig, finalGroups []ast.DeploymentGroup, state *compileVarState) ([]ast.DeploymentGroup, error) {
	for _, archRef := range userConfig.ComputeArchetypes {
		hydratedArchGroups, err := c.processSingleArchetype(archRef, *userConfig, state)
		if err != nil {
			return nil, err
		}
		finalGroups = mergeGroups(finalGroups, hydratedArchGroups)
	}
	return finalGroups, nil
}

func (c *Compiler) processSingleArchetype(archRef ast.ArchetypeReference, userConfig ast.ClusterConfig, state *compileVarState) ([]ast.DeploymentGroup, error) {
	archManifest, err := c.loader.LoadArchetypeForBase(archRef.MachineType, userConfig.ConfigBase.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to load archetype %q: %w", archRef.MachineType, err)
	}
	if !c.loader.HasExplicitArchetype(archRef.MachineType) {
		if err := c.validateFallbackMachineType(archRef.MachineType, userConfig); err != nil {
			return nil, err
		}
	}
	recordExplicitDefaultPool(archRef, state.explicitDefaultPools)

	poolLiteralOverrides, poolExprOverrides, filteredSettings := extractPoolVarOverrides(archRef.Settings, archManifest, userConfig.ConfigBase.Type)
	archRef.Settings = filteredSettings
	poolConflictingDefaults := mergeArchetypeDefaultVars(archManifest, userConfig, poolLiteralOverrides, poolExprOverrides, state)

	hydratedArchGroups, err := HydrateArchetype(archRef, *archManifest, userConfig.ConfigBase.Type, c.loader.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate archetype %q: %w", archRef.Name, err)
	}
	if len(poolExprOverrides) > 0 {
		substituteVarRefs(hydratedArchGroups, archRef.Name, poolExprOverrides)
	}

	scopedArchVars := promoteScopedArchetypeVars(archRef.Name, hydratedArchGroups, userConfig.Features, poolConflictingDefaults, poolLiteralOverrides, state)
	if len(scopedArchVars) > 0 {
		hydratedArchGroups = rewriteScopedVarRefs(hydratedArchGroups, archRef.Name, scopedArchVars)
	}
	return hydratedArchGroups, nil
}

func recordExplicitDefaultPool(archRef ast.ArchetypeReference, explicitDefaultPools map[string]bool) {
	if archRef.Settings == nil {
		return
	}
	if v, ok := archRef.Settings["is_default"].(bool); ok && v {
		explicitDefaultPools[archRef.Name] = true
	}
	for _, rawSub := range archRef.Settings {
		if subMap, ok := rawSub.(map[string]any); ok {
			if v, ok := subMap["is_default"].(bool); ok && v {
				explicitDefaultPools[archRef.Name] = true
			}
		}
	}
}

func extractPoolVarOverrides(settings map[string]any, archManifest *ast.ComputeArchetypeManifest, configBaseType string) (map[string]any, map[string]any, map[string]any) {
	poolLiteralOverrides := make(map[string]any)
	poolExprOverrides := make(map[string]any)
	if len(settings) == 0 {
		return poolLiteralOverrides, poolExprOverrides, settings
	}
	archVars := collectManifestVarKeys(archManifest.Vars, archManifest.ConfigBase[configBaseType])
	filteredSettings := make(map[string]any, len(settings))
	for k, v := range settings {
		if !archVars[k] {
			filteredSettings[k] = v
			continue
		}
		if s, isStr := v.(string); isStr && (strings.Contains(s, "$(") || strings.Contains(s, "((")) {
			poolExprOverrides[k] = v
		} else {
			poolLiteralOverrides[k] = v
		}
	}
	return poolLiteralOverrides, poolExprOverrides, filteredSettings
}

func collectManifestVarKeys(rootVars map[string]any, baseBlock ast.BlueprintBlock) map[string]bool {
	keys := make(map[string]bool)
	for k := range rootVars {
		keys[k] = true
	}
	for k := range baseBlock.Vars {
		keys[k] = true
	}
	return keys
}

func mergeArchetypeDefaultVars(archManifest *ast.ComputeArchetypeManifest, userConfig ast.ClusterConfig, poolLiteralOverrides, poolExprOverrides map[string]any, state *compileVarState) map[string]any {
	poolConflictingDefaults := make(map[string]any)
	mergeOne := func(k string, v any) {
		if state.conflictingArchVars[k] {
			_, hasLit := poolLiteralOverrides[k]
			_, hasExpr := poolExprOverrides[k]
			userVal, inUser := userConfig.Vars[k]
			if !hasLit && !hasExpr && (!inUser || userVal == nil) {
				poolConflictingDefaults[k] = v
			}
			return
		}
		if !state.promotedUnscopedVars[k] {
			state.mergedVars[k] = v
		}
	}
	for k, v := range archManifest.Vars {
		mergeOne(k, v)
	}
	if baseBlock, ok := archManifest.ConfigBase[userConfig.ConfigBase.Type]; ok {
		for k, v := range baseBlock.Vars {
			mergeOne(k, v)
		}
	}
	return poolConflictingDefaults
}

func promoteScopedArchetypeVars(poolName string, hydratedArchGroups []ast.DeploymentGroup, features []ast.FeatureReference, poolConflictingDefaults, poolLiteralOverrides map[string]any, state *compileVarState) map[string]string {
	scopedArchVars := make(map[string]string)
	for _, k := range sortedAnyMapKeys(poolConflictingDefaults) {
		poolRefs, _ := classifyVarReferences(hydratedArchGroups, features, poolName, k)
		if poolRefs {
			scopedKey := sanitizeVarPrefix(poolName) + "_" + k
			state.mergedVars[scopedKey] = poolConflictingDefaults[k]
			scopedArchVars[k] = scopedKey
		}
	}
	for _, k := range sortedAnyMapKeys(poolLiteralOverrides) {
		v := poolLiteralOverrides[k]
		poolRefs, nonPoolRefs := classifyVarReferences(hydratedArchGroups, features, poolName, k)
		if poolRefs {
			scopedKey := sanitizeVarPrefix(poolName) + "_" + k
			state.mergedVars[scopedKey] = v
			state.userDeclaredVars[scopedKey] = true
			delete(state.userDeclaredVars, k)
			scopedArchVars[k] = scopedKey
		}
		if (!poolRefs || nonPoolRefs) && !state.promotedUnscopedVars[k] {
			state.mergedVars[k] = v
			if !poolRefs {
				state.userDeclaredVars[k] = true
			}
			state.promotedUnscopedVars[k] = true
			state.settingsOverriddenVars[k] = true
		}
	}
	return scopedArchVars
}

func sortedAnyMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (c *Compiler) applyConfigBaseSettings(userConfig ast.ClusterConfig, baseManifest *ast.ConfigBaseManifest, allBaseMods []ast.ModuleSpec, baseModIDs map[string]bool, finalGroups []ast.DeploymentGroup, state *compileVarState) error {
	if len(userConfig.ConfigBase.Settings) == 0 {
		return nil
	}
	remainingBaseSettings := make(map[string]any, len(userConfig.ConfigBase.Settings))
	for k, v := range userConfig.ConfigBase.Settings {
		if baseManifest.Vars != nil {
			if _, isBaseVar := baseManifest.Vars[k]; isBaseVar {
				state.mergedVars[k] = v
				state.userDeclaredVars[k] = true
				state.settingsOverriddenVars[k] = true
				continue
			}
		}
		remainingBaseSettings[k] = v
	}
	if len(remainingBaseSettings) == 0 {
		return nil
	}
	flatBaseSettings, submodBaseSettings, err := splitAndValidateBaseSettings(userConfig.ConfigBase.Type, remainingBaseSettings, allBaseMods, c.loader.basePath)
	if err != nil {
		return err
	}
	c.routeSettingsToBaseModules(allBaseMods, baseModIDs, flatBaseSettings, submodBaseSettings, finalGroups)
	return nil
}

func splitAndValidateBaseSettings(baseType string, remainingBaseSettings map[string]any, allBaseMods []ast.ModuleSpec, basePath string) (map[string]any, map[string]map[string]any, error) {
	flatBaseSettings, submodBaseSettings := splitSubmoduleSettings(remainingBaseSettings, baseType, allBaseMods, basePath)
	baseRef := ast.FeatureReference{Name: baseType, Type: baseType, Settings: flatBaseSettings}
	if err := validateSettingKeys(baseRef, allBaseMods, nil); err != nil {
		return nil, nil, err
	}
	for subID, subMap := range submodBaseSettings {
		for _, m := range allBaseMods {
			if m.ID == subID {
				subRef := ast.FeatureReference{Name: subID, Type: baseType, Settings: subMap}
				if err := validateSettingKeys(subRef, []ast.ModuleSpec{m}, nil); err != nil {
					return nil, nil, err
				}
				break
			}
		}
	}
	return flatBaseSettings, submodBaseSettings, nil
}

func (c *Compiler) routeSettingsToBaseModules(allBaseMods []ast.ModuleSpec, baseModIDs map[string]bool, flatBaseSettings map[string]any, submodBaseSettings map[string]map[string]any, finalGroups []ast.DeploymentGroup) {
	primaryBaseID := ""
	var primaryBaseInputs map[string]bool
	if len(allBaseMods) > 0 {
		lastMod := allBaseMods[len(allBaseMods)-1]
		primaryBaseID = lastMod.ID
		primaryBaseInputs = getValidInputs(lastMod, c.loader.basePath)
	}
	for gi := range finalGroups {
		for mi := range finalGroups[gi].Modules {
			mod := finalGroups[gi].Modules[mi]
			if !baseModIDs[mod.ID] {
				continue
			}
			isPrimary := mod.ID == primaryBaseID
			modOverrides := flatBaseSettings
			if !isPrimary {
				modOverrides = filterSidecarSettings(flatBaseSettings, primaryBaseInputs)
			}
			validSettings := partitionArchetypeSettings(mod, allBaseMods, modOverrides, isPrimary, c.loader.basePath)
			finalGroups[gi].Modules[mi].Settings = mergeSettingsDeep(mod.Settings, validSettings)
			if explicitSub, ok := submodBaseSettings[mod.ID]; ok {
				finalGroups[gi].Modules[mi].Settings = mergeSettingsDeep(finalGroups[gi].Modules[mi].Settings, explicitSub)
			}
		}
	}
}

func mergeUserClusterConfigVars(userVars map[string]any, state *compileVarState) {
	for k, v := range userVars {
		if state.settingsOverriddenVars[k] {
			continue
		}
		if v == nil && state.mergedVars[k] != nil {
			continue
		}
		state.mergedVars[k] = v
	}
}

func extractAndValidatePoolNames(archetypes []ast.ArchetypeReference) ([]string, error) {
	poolNames := make([]string, 0, len(archetypes))
	for _, a := range archetypes {
		poolNames = append(poolNames, a.Name)
	}
	if err := validatePoolNames(poolNames); err != nil {
		return nil, err
	}
	return poolNames, nil
}

func (c *Compiler) processFeatures(userConfig *ast.ClusterConfig, finalGroups []ast.DeploymentGroup, state *compileVarState) ([]ast.DeploymentGroup, error) {
	for _, featureRef := range userConfig.Features {
		groups, err := c.processSingleFeature(featureRef, *userConfig, state)
		if err != nil {
			return nil, err
		}
		finalGroups = mergeGroups(finalGroups, groups)
	}
	return finalGroups, nil
}

func (c *Compiler) processSingleFeature(featureRef ast.FeatureReference, userConfig ast.ClusterConfig, state *compileVarState) ([]ast.DeploymentGroup, error) {
	if featureRef.Type == "" {
		featureRef.Type = featureRef.Name
	}
	manifest, err := c.loader.LoadUserFeature(featureRef.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to load feature type %q: %w", featureRef.Type, err)
	}
	mergeFeatureDefaultVars(manifest, userConfig, state)
	scopedFeatureVars := promoteScopedFeatureVars(&featureRef, manifest, userConfig.ConfigBase.Type, state)

	groups, err := EvaluateFeature(featureRef, *manifest, state.mergedVars, userConfig.ConfigBase.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate feature/overlay %q: %w", featureRef.Name, err)
	}
	if len(scopedFeatureVars) > 0 {
		groups = rewriteScopedVarRefs(groups, featureRef.Name, scopedFeatureVars)
	}
	return groups, nil
}

func mergeFeatureDefaultVars(manifest *ast.FeatureManifest, userConfig ast.ClusterConfig, state *compileVarState) {
	mergeMap := func(vars map[string]any) {
		for k, v := range vars {
			if existing, exists := userConfig.Vars[k]; (!exists || existing == nil) && !state.promotedUnscopedVars[k] {
				state.mergedVars[k] = v
			}
		}
	}
	mergeMap(manifest.Vars)
	if baseBlock, ok := manifest.ConfigBase[userConfig.ConfigBase.Type]; ok {
		mergeMap(baseBlock.Vars)
	}
}

func promoteScopedFeatureVars(featureRef *ast.FeatureReference, manifest *ast.FeatureManifest, configBaseType string, state *compileVarState) map[string]string {
	scopedFeatureVars := make(map[string]string)
	if len(featureRef.Settings) == 0 {
		return scopedFeatureVars
	}
	featVars := collectManifestVarKeys(manifest.Vars, manifest.ConfigBase[configBaseType])
	if len(featVars) == 0 {
		return scopedFeatureVars
	}
	filteredSettings := make(map[string]any, len(featureRef.Settings))
	for _, k := range sortedAnyMapKeys(featureRef.Settings) {
		v := featureRef.Settings[k]
		if !featVars[k] {
			filteredSettings[k] = v
			continue
		}
		scopedKey := sanitizeVarPrefix(featureRef.Name) + "_" + k
		state.mergedVars[scopedKey] = v
		state.userDeclaredVars[scopedKey] = true
		delete(state.userDeclaredVars, k)
		scopedFeatureVars[k] = scopedKey
		if !state.promotedUnscopedVars[k] {
			state.mergedVars[k] = v
			state.promotedUnscopedVars[k] = true
		}
	}
	featureRef.Settings = filteredSettings
	return scopedFeatureVars
}

func (c *Compiler) finalizeGraphAndVars(userConfig *ast.ClusterConfig, baseManifest *ast.ConfigBaseManifest, finalGroups []ast.DeploymentGroup, state *compileVarState) ([]ast.DeploymentGroup, error) {
	if len(baseManifest.Aliases) > 0 {
		resolveAliases(finalGroups, baseManifest.Aliases)
	}
	pruneDeadUses(finalGroups)

	var err error
	finalGroups, err = Autowire(finalGroups, c.loader.basePath)
	if err != nil {
		return nil, fmt.Errorf("autowire failed: %w", err)
	}
	finalGroups = ResolveDAG(finalGroups)

	for _, group := range finalGroups {
		for _, mod := range group.Modules {
			if err := ValidateModuleSettings(mod, c.loader.basePath); err != nil {
				return nil, err
			}
		}
	}

	state.mergedVars = pruneUnusedVars(state.mergedVars, state.userDeclaredVars, finalGroups)
	if userConfig.Vars != nil {
		for k := range userConfig.Vars {
			if _, keep := state.mergedVars[k]; !keep {
				delete(userConfig.Vars, k)
			}
		}
		for k, v := range state.mergedVars {
			userConfig.Vars[k] = v
		}
	}
	return finalGroups, nil
}

// validateSingleAcceleratorArchetype rejects clusters that combine more than one
// accelerator archetype (GPU+GPU or GPU+TPU). One accelerator archetype may back any
// number of pools, and generic-cpu pools may be added freely.
//
// Phase 1 cannot compose two different accelerator archetypes safely: each one ships its own
// cluster-level modules (controller, login, networks, GKE cluster toggles) and var
// defaults under the same IDs and names, and combining them silently keeps whichever
// archetype is listed last.
func (c *Compiler) validateSingleAcceleratorArchetype(refs []ast.ArchetypeReference) error {
	pools := map[string][]string{} // accelerator archetype -> pool names
	var order []string
	for _, ref := range refs {
		m, err := c.loader.LoadArchetype(ref.MachineType)
		if err != nil || isFallbackArchetype(m) {
			continue // unknown types are reported with full context in Phase 2
		}
		if _, seen := pools[m.Name]; !seen {
			order = append(order, m.Name)
		}
		pools[m.Name] = append(pools[m.Name], ref.Name)
	}
	if len(order) <= 1 {
		return nil
	}
	parts := make([]string, 0, len(order))
	for _, mt := range order {
		parts = append(parts, fmt.Sprintf("%s (pools: %s)", mt, strings.Join(pools[mt], ", ")))
	}
	return fmt.Errorf(
		"combining different accelerator archetypes in one cluster is not currently supported: %s. "+
			"Use a single accelerator archetype per cluster (it may back several pools, and CPU "+
			"pools can be added alongside it), or deploy each accelerator type as its own cluster",
		strings.Join(parts, "; "))
}

// =========================================================================
// AST MEMORY SAFETY UTILS
// =========================================================================

// copyDeploymentGroups performs a deep copy of deployment groups to avoid mutating the global cache.
// Because the CatalogLoader caches the 'baseManifest' in memory (so we don't hit the disk
// parsing YAML repeatedly), if we injected Feature modules directly into the baseManifest variable,
// we would permanently corrupt the global cache for any other cluster being compiled.
func copyDeploymentGroups(groups []ast.DeploymentGroup) []ast.DeploymentGroup {
	copied := make([]ast.DeploymentGroup, 0, len(groups))
	for _, g := range groups {
		newGroup := ast.DeploymentGroup{
			Group: g.Group,
		}
		for _, m := range g.Modules {
			newGroup.Modules = append(newGroup.Modules, copyModule(m))
		}
		copied = append(copied, newGroup)
	}
	return copied
}

func copyModule(m ast.ModuleSpec) ast.ModuleSpec {
	newMod := ast.ModuleSpec{
		ID:                m.ID,
		Source:            m.Source,
		Kind:              m.Kind,
		Condition:         m.Condition,
		Export:            m.Export,
		FeatureInstanceID: m.FeatureInstanceID,
		Tier:              m.Tier,
		PoolName:          m.PoolName,
		Origin:            m.Origin,
		IsComputePool:     m.IsComputePool,
	}
	if len(m.AttachTo) > 0 {
		newMod.AttachTo = make([]string, len(m.AttachTo))
		copy(newMod.AttachTo, m.AttachTo)
	}
	if len(m.Use) > 0 {
		newMod.Use = make([]string, len(m.Use))
		copy(newMod.Use, m.Use)
	}
	if len(m.Outputs) > 0 {
		newMod.Outputs = make([]any, len(m.Outputs))
		copy(newMod.Outputs, m.Outputs)
	}
	if len(m.Settings) > 0 {
		newMod.Settings = make(map[string]any)
		for k, v := range m.Settings {
			newMod.Settings[k] = v // Shallow copy of values is sufficient here
		}
	}
	return newMod
}

// sanitizeVarPrefix converts a pool or feature instance name (e.g. "train-pool")
// into a valid Terraform/Blueprint variable prefix ("train_pool").
func sanitizeVarPrefix(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "-", "_")
}

// classifyVarReferences reports whether modules belonging to `instanceName` (poolRefs)
// and/or shared singleton modules (nonPoolRefs) reference `vars.<varName>`.
func classifyVarReferences(groups []ast.DeploymentGroup, features []ast.FeatureReference, instanceName, varName string) (poolRefs, nonPoolRefs bool) {
	pattern := regexp.MustCompile(`\bvars\.` + regexp.QuoteMeta(varName) + `\b`)
	for _, g := range groups {
		for _, mod := range g.Modules {
			if !moduleReferencesPattern(mod, pattern) {
				continue
			}
			if moduleBelongsToPool(mod.ID, instanceName) || mod.FeatureInstanceID == instanceName {
				poolRefs = true
			} else if _, isDirectSingletonInput := mod.Settings[varName]; !isDirectSingletonInput {
				nonPoolRefs = true
			}
		}
	}
	for _, f := range features {
		if valueMatchesPattern(f.Settings, pattern) {
			nonPoolRefs = true
			break
		}
	}
	return poolRefs, nonPoolRefs
}

func moduleReferencesPattern(mod ast.ModuleSpec, pattern *regexp.Regexp) bool {
	return valueMatchesPattern(mod.Settings, pattern) || (mod.Condition != "" && pattern.MatchString(mod.Condition))
}

func valueMatchesPattern(v any, pattern *regexp.Regexp) bool {
	switch t := v.(type) {
	case string:
		return pattern.MatchString(t)
	case map[string]any:
		for _, mv := range t {
			if valueMatchesPattern(mv, pattern) {
				return true
			}
		}
	case []any:
		for _, item := range t {
			if valueMatchesPattern(item, pattern) {
				return true
			}
		}
	}
	return false
}

// rewriteScopedVarRefs replaces references to `vars.<old>` with `vars.<new>` inside
// modules belonging to `instanceName` so per-instance variable promotions remain isolated.
func rewriteScopedVarRefs(groups []ast.DeploymentGroup, instanceName string, replacements map[string]string) []ast.DeploymentGroup {
	if len(replacements) == 0 {
		return groups
	}
	sortedOldKeys := make([]string, 0, len(replacements))
	patterns := make(map[string]*regexp.Regexp, len(replacements))
	for oldKey := range replacements {
		sortedOldKeys = append(sortedOldKeys, oldKey)
		patterns[oldKey] = regexp.MustCompile(`\bvars\.` + regexp.QuoteMeta(oldKey) + `\b`)
	}
	sort.Strings(sortedOldKeys)

	for gi := range groups {
		for mi := range groups[gi].Modules {
			mod := &groups[gi].Modules[mi]
			if (moduleBelongsToPool(mod.ID, instanceName) || mod.FeatureInstanceID == instanceName) && mod.Settings != nil {
				mod.Settings = rewriteValueVarRefs(mod.Settings, sortedOldKeys, patterns, replacements).(map[string]any)
			}
		}
	}
	return groups
}

func rewriteValueVarRefs(v any, sortedOldKeys []string, patterns map[string]*regexp.Regexp, replacements map[string]string) any {
	switch t := v.(type) {
	case string:
		out := t
		for _, oldKey := range sortedOldKeys {
			out = patterns[oldKey].ReplaceAllString(out, "vars."+replacements[oldKey])
		}
		return out
	case map[string]any:
		m := make(map[string]any, len(t))
		for mk, mv := range t {
			m[mk] = rewriteValueVarRefs(mv, sortedOldKeys, patterns, replacements)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, item := range t {
			s[i] = rewriteValueVarRefs(item, sortedOldKeys, patterns, replacements)
		}
		return s
	default:
		return v
	}
}

// =========================================================================
// AST SET UNION (THE DEEP MERGE STRATEGY)
// =========================================================================

// mergeGroups executes a Mathematical Set Union. When Features or Archetypes are evaluated,
// they return their own mini-ASTs. This function injects them into the Master AST.
//
// Algorithm:
//  1. If the Feature module introduces an entirely new Module ID (e.g. 'lustre_firewall'), it is appended.
//  2. If the Feature module references an existing Module ID (e.g. 'private_service_access'),
//     it does NOT duplicate it (which would crash Terraform). Instead, it recursively performs
//     a deep-merge of their 'settings' so they share the underlying cloud resource.
//  3. An ID collision merges BOTH 'settings' and 'use'. 'use' is unioned, not replaced:
//     an archetype or feature that adds a dependency to a config-base module must keep
//     the base's wiring as well as its own.
func mergeModuleIntoExisting(existing *ast.ModuleSpec, mod ast.ModuleSpec) {
	if mod.Source != "" && mod.Source != existing.Source {
		existing.Source = mod.Source
		existing.Settings = mod.Settings
	} else if existing.Tier == 3 && mod.Tier == 3 && !mod.IsComputePool {
		existing.Settings = mergeSettingsDeep(mod.Settings, existing.Settings)
	} else {
		existing.Settings = mergeSettingsDeep(existing.Settings, mod.Settings)
	}
	existing.Use = unionUse(existing.Use, mod.Use)
	if len(mod.Outputs) > 0 && len(existing.Outputs) == 0 {
		existing.Outputs = append([]any(nil), mod.Outputs...)
	}
	if mod.IsComputePool {
		existing.IsComputePool = true
	}
	if mod.PoolName != "" {
		existing.PoolName = mod.PoolName
	}
}

func findModuleInGroups(groups []ast.DeploymentGroup, id string) (int, int) {
	for gi := range groups {
		if mi := findModuleIndex(groups[gi].Modules, id); mi != -1 {
			return gi, mi
		}
	}
	return -1, -1
}

func mergeGroups(base []ast.DeploymentGroup, overrides []ast.DeploymentGroup) []ast.DeploymentGroup {
	// Create a fast lookup map for base groups
	groupMap := make(map[string]int)
	for i := range base {
		groupMap[base[i].Group] = i
	}

	for _, override := range overrides {
		if idx, ok := groupMap[override.Group]; ok {
			for _, mod := range override.Modules {
				if mIdx := findModuleIndex(base[idx].Modules, mod.ID); mIdx != -1 {
					mergeModuleIntoExisting(&base[idx].Modules[mIdx], mod)
				} else if gIdx, otherMIdx := findModuleInGroups(base, mod.ID); gIdx != -1 {
					mergeModuleIntoExisting(&base[gIdx].Modules[otherMIdx], mod)
				} else {
					base[idx].Modules = append(base[idx].Modules, mod)
				}
			}
		} else {
			var remaining []ast.ModuleSpec
			for _, mod := range override.Modules {
				if gIdx, otherMIdx := findModuleInGroups(base, mod.ID); gIdx != -1 {
					mergeModuleIntoExisting(&base[gIdx].Modules[otherMIdx], mod)
				} else {
					remaining = append(remaining, mod)
				}
			}
			if len(remaining) > 0 {
				newGroup := override
				newGroup.Modules = remaining
				base = append(base, newGroup)
				groupMap[override.Group] = len(base) - 1
			}
		}
	}

	return base
}

// unionUse appends entries from override that base does not already have, preserving
// base's relative order and appending new entries in the order given.
func unionUse(base, override []string) []string {
	if len(override) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(override))
	for _, u := range base {
		seen[u] = true
	}
	merged := base
	for _, u := range override {
		if !seen[u] {
			merged = append(merged, u)
			seen[u] = true
		}
	}
	return merged
}

func findModuleIndex(list []ast.ModuleSpec, id string) int {
	for i, m := range list {
		if m.ID == id {
			return i
		}
	}
	return -1
}

// =========================================================================
// DEPENDENCY RESOLUTION & GRAPH SANITATION
// =========================================================================

// resolveAliases acts as a Dynamic Pointer Rebinding mechanism for dependencies.
// If a generic base module requires a 'network' (Interface), and the user's YAML configuration
// defines an alias: 'network: vpc_high_bandwith' (Implementation), this function sweeps every single
// 'use' array in the AST and natively swaps the string.
func resolveAliases(groups []ast.DeploymentGroup, aliases map[string]string) {
	for i := range groups {
		for j := range groups[i].Modules {
			groups[i].Modules[j].Use = applyAliasToUse(groups[i].Modules[j].Use, aliases)
		}
	}
}

// pruneDeadUses is a critical graph sanitation step.
// When Features are evaluated through CEL (e.g., condition evaluates to false),
// their physical modules are dropped from the AST. However, other modules might still have
// them listed in their hardcoded 'use' arrays.
// If Terraform's DAG engine sees a resource requesting a module that doesn't exist, it fatally crashes.
// This function sweeps the final AST, catalogs every surviving Module ID, and surgically strips
// any phantom dependencies out of everyone's 'use' arrays to guarantee a pristine DAG topology.
func pruneDeadUses(groups []ast.DeploymentGroup) {
	survivors := make(map[string]bool)
	for _, g := range groups {
		for _, m := range g.Modules {
			survivors[m.ID] = true
		}
	}

	for i := range groups {
		for j := range groups[i].Modules {
			var validUses []string
			for _, u := range groups[i].Modules[j].Use {
				if survivors[u] {
					validUses = append(validUses, u)
				}
			}
			groups[i].Modules[j].Use = validUses
		}
	}
}

func applyAliasToUse(use []string, aliases map[string]string) []string {
	if len(use) == 0 {
		return use
	}
	var newUse []string
	for _, u := range use {
		if mapped, ok := aliases[u]; ok {
			newUse = append(newUse, mapped)
		} else {
			newUse = append(newUse, u)
		}
	}
	return newUse
}

// =========================================================================
// DEPLOYMENT VARIABLE PRUNING
// =========================================================================

// alwaysUsedVars mirrors the allowlist in pkg/config.ListUnusedVariables. These are
// consumed by the Google provider or by gcluster itself, never by an explicit reference,
// so an unreferenced-token scan would wrongly consider them dead.
var alwaysUsedVars = map[string]bool{
	"labels":          true,
	"deployment_name": true,
	"project_id":      true,
	"region":          true,
	"zone":            true,
}

// varRefPattern matches a reference to deployment variable `name` in either the v2
// authoring form `$(vars.name)` or the expanded form `((var.name))`. The trailing
// assertion stops `vars.a3_reservation_name` from matching a scan for `a3_reservation`.
func varRefPattern(name string) *regexp.Regexp {
	return regexp.MustCompile(`\bvars?\.` + regexp.QuoteMeta(name) + `([^A-Za-z0-9_]|$)`)
}

// pruneUnusedVars drops manifest-offered deployment variables that no surviving module
// references.
func pruneUnusedVars(vars map[string]any, userDeclared map[string]bool, groups []ast.DeploymentGroup) map[string]any {
	surviving := make(map[string]any, len(vars))
	for k, v := range vars {
		surviving[k] = v
	}

	// Everything a module could possibly reference, rendered once per round.
	moduleText := renderModuleReferences(groups)

	for {
		haystack := moduleText
		for k, v := range surviving {
			haystack += "\n" + k + "=" + fmt.Sprintf("%v", v)
		}

		var dropped []string
		for name := range surviving {
			if userDeclared[name] || alwaysUsedVars[name] {
				continue
			}
			// Exclude this var's own value from the haystack, or a self-referential
			// default would keep itself alive.
			self := "\n" + name + "=" + fmt.Sprintf("%v", surviving[name])
			if varRefPattern(name).MatchString(strings.Replace(haystack, self, "", 1)) {
				continue
			}
			dropped = append(dropped, name)
		}
		if len(dropped) == 0 {
			return surviving
		}
		for _, name := range dropped {
			delete(surviving, name)
		}
	}
}

// renderModuleReferences flattens every place a module can name a deployment variable --
// settings values (at any nesting depth) and `use` entries -- into one searchable string.
func renderModuleReferences(groups []ast.DeploymentGroup) string {
	var b strings.Builder
	write := func(m ast.ModuleSpec) {
		b.WriteString(m.ID)
		b.WriteString("\n")
		for _, u := range m.Use {
			b.WriteString(u)
			b.WriteString("\n")
		}
		// %#v renders nested maps and slices in full, which is what matters: a var
		// reference is frequently buried inside a runners list or a settings object.
		b.WriteString(fmt.Sprintf("%#v\n", m.Settings))
	}
	for _, g := range groups {
		for _, m := range g.Modules {
			write(m)
		}
	}
	return b.String()
}

// applyCompositeOverlays applies composite OverlayManifests (`config_base.settings`,
// `compute_archetypes`, `features`, `vars`) onto `userConfig`, while preserving
// single-feature overlays (`feature: ...`) in `userConfig.Overlays` for PHASE 3.
func (c *Compiler) isRoleTarget(configBaseType, target string) bool {
	if strings.EqualFold(target, "compute") {
		return true
	}
	targetLower := strings.ToLower(target)
	if baseManifest, err := c.loader.LoadConfigBase(configBaseType); err == nil && baseManifest != nil {
		if _, ok := baseManifest.Aliases[targetLower]; ok {
			return true
		}
		for _, g := range baseManifest.DeploymentGroups {
			for _, m := range g.Modules {
				if strings.HasSuffix(m.ID, "_"+targetLower) {
					return true
				}
			}
		}
	}
	return false
}

func (c *Compiler) applyOverlays(userConfig *ast.ClusterConfig) error {
	if len(userConfig.Overlays) == 0 {
		return nil
	}
	seenOverlays := make(map[string]bool, len(userConfig.Overlays))
	for _, ovRef := range userConfig.Overlays {
		if ovRef.Name != "" {
			if seenOverlays[ovRef.Name] {
				return fmt.Errorf("duplicate overlay name %q found", ovRef.Name)
			}
			seenOverlays[ovRef.Name] = true
		}
		if err := c.applySingleOverlay(userConfig, ovRef); err != nil {
			return err
		}
	}
	userConfig.Overlays = nil
	return nil
}

func (c *Compiler) applySingleOverlay(userConfig *ast.ClusterConfig, ovRef ast.FeatureReference) error {
	ovType := ovRef.Type
	if ovType == "" {
		ovType = ovRef.Name
	}
	ovManifest, err := c.loader.LoadOverlay(ovType)
	if err != nil {
		return fmt.Errorf("failed to resolve overlay %q: %w", ovRef.Name, err)
	}
	if ovManifest.ConfigBase.Type != "" && ovManifest.ConfigBase.Type != userConfig.ConfigBase.Type {
		return fmt.Errorf("failed to resolve overlay %q: overlay %q does not support config_base %q (supported: %s)",
			ovRef.Name, ovManifest.Name, userConfig.ConfigBase.Type, ovManifest.ConfigBase.Type)
	}
	if err := mergeOverlayVarsAndArchetypes(userConfig, ovRef.Name, ovManifest); err != nil {
		return err
	}
	if err := c.validateOverlayAttachTo(userConfig, ovRef); err != nil {
		return err
	}
	return c.mergeOverlayFeatures(userConfig, ovRef, ovManifest)
}

func mergeOverlayVarsAndArchetypes(userConfig *ast.ClusterConfig, ovName string, ovManifest *ast.OverlayManifest) error {
	if len(ovManifest.Vars) > 0 {
		if userConfig.Vars == nil {
			userConfig.Vars = make(map[string]any, len(ovManifest.Vars))
		}
		for k, v := range ovManifest.Vars {
			if existing, exists := userConfig.Vars[k]; exists && existing != nil {
				return fmt.Errorf("overlay %q cannot override existing var %q in cluster config; overlays are strictly additive", ovName, k)
			}
			userConfig.Vars[k] = v
		}
	}
	if len(ovManifest.ConfigBase.Settings) > 0 {
		return fmt.Errorf("overlay %q cannot modify config_base settings; overlays are strictly additive", ovName)
	}
	for _, patchArch := range ovManifest.ComputeArchetypes {
		if patchArch.Name == "*" || (patchArch.Name == "" && patchArch.MachineType == "") {
			return fmt.Errorf("overlay %q cannot modify existing compute archetypes; overlays are strictly additive", ovName)
		}
		for i := range userConfig.ComputeArchetypes {
			if userConfig.ComputeArchetypes[i].Name == patchArch.Name {
				return fmt.Errorf("overlay %q conflicts with existing compute archetype %q in cluster config", ovName, patchArch.Name)
			}
		}
		userConfig.ComputeArchetypes = append(userConfig.ComputeArchetypes, patchArch)
	}
	return nil
}

func (c *Compiler) validateOverlayAttachTo(userConfig *ast.ClusterConfig, ovRef ast.FeatureReference) error {
	seenPools := make(map[string]bool, len(userConfig.ComputeArchetypes))
	for _, a := range userConfig.ComputeArchetypes {
		seenPools[a.Name] = true
	}
	for _, attachedPool := range ovRef.AttachTo {
		if attachedPool == "" {
			return fmt.Errorf("overlay %q has an empty attach_to pool string segment", ovRef.Name)
		}
		if !seenPools[attachedPool] && !c.isRoleTarget(userConfig.ConfigBase.Type, attachedPool) {
			return fmt.Errorf("overlay %q is attached to undefined pool %q", ovRef.Name, attachedPool)
		}
	}
	return nil
}

func (c *Compiler) mergeOverlayFeatures(userConfig *ast.ClusterConfig, ovRef ast.FeatureReference, ovManifest *ast.OverlayManifest) error {
	for _, patchFeat := range ovManifest.Features {
		if patchFeat.Type == "" {
			patchFeat.Type = patchFeat.Name
		}
		featManifest, err := c.loader.LoadFeature(patchFeat.Type)
		if err != nil {
			return fmt.Errorf("failed to resolve overlay %q: overlay %q names unknown feature %q: %w", ovRef.Name, ovManifest.Name, patchFeat.Type, err)
		}
		if err := validateOverlayFeatureBaseSupport(ovRef.Name, ovManifest, featManifest, patchFeat.Type, userConfig.ConfigBase.Type); err != nil {
			return err
		}
		patchFeat.Name = resolveOverlayFeatureInstanceName(patchFeat.Name, ovManifest, ovRef.Name)
		for i := range userConfig.Features {
			if userConfig.Features[i].Name == patchFeat.Name {
				return fmt.Errorf("overlay %q conflicts with existing feature %q in cluster config", ovRef.Name, patchFeat.Name)
			}
		}
		if len(ovRef.AttachTo) > 0 {
			patchFeat.AttachTo = append([]string(nil), ovRef.AttachTo...)
		}
		patchFeat.Settings = hydrateSettings(patchFeat.Settings, patchFeat.Name, "")
		userConfig.Features = append(userConfig.Features, patchFeat)
	}
	return nil
}

func validateOverlayFeatureBaseSupport(ovName string, ovManifest *ast.OverlayManifest, featManifest *ast.FeatureManifest, featType, configBaseType string) error {
	if ovManifest.ConfigBase.Type != "" || len(featManifest.ConfigBase) == 0 || len(featManifest.DeploymentGroups) > 0 {
		return nil
	}
	if _, ok := featManifest.ConfigBase[configBaseType]; ok {
		return nil
	}
	var supported []string
	for base := range featManifest.ConfigBase {
		supported = append(supported, base)
	}
	sort.Strings(supported)
	return fmt.Errorf(
		"failed to resolve overlay %q: overlay %q (feature %q) does not support config_base %q (supported: %s)",
		ovName, ovManifest.Name, featType, configBaseType, strings.Join(supported, ", "))
}

func resolveOverlayFeatureInstanceName(featName string, ovManifest *ast.OverlayManifest, ovRefName string) string {
	if len(ovManifest.Features) == 1 && (featName == ovManifest.Name || featName == "{name}" || featName == "") && ovRefName != "" {
		return ovRefName
	}
	if strings.Contains(featName, "{name}") {
		return strings.ReplaceAll(featName, "{name}", ovRefName)
	}
	return featName
}

func isExplicitDefaultPoolMod(modID string, explicitDefaultPools map[string]bool) bool {
	for pool := range explicitDefaultPools {
		if moduleBelongsToPool(modID, pool) {
			return true
		}
	}
	return false
}

func normalizeMultiPoolAndFlexStart(
	groups []ast.DeploymentGroup,
	basePath string,
	poolCount int,
	explicitDefaultPools map[string]bool,
	mergedVars map[string]any,
	userDeclaredVars map[string]bool,
) {
	if poolCount > 1 {
		normalizeDefaultPoolFlags(groups, explicitDefaultPools)
	}
	normalizeDWSAndPlacementHostMaintenance(groups, basePath, mergedVars)
	if poolCount > 1 {
		disambiguateDuplicatePoolNamePrefixes(groups)
	}
	normalizePostFeatureGKE(groups, basePath, mergedVars, userDeclaredVars)
}

func normalizeDefaultPoolFlags(groups []ast.DeploymentGroup, explicitDefaultPools map[string]bool) {
	hasExplicitDefault := len(explicitDefaultPools) > 0
	defaultAssigned := false
	for gi := range groups {
		for mi := range groups[gi].Modules {
			mod := &groups[gi].Modules[mi]
			isDef, ok := mod.Settings["is_default"].(bool)
			if !ok || !isDef {
				continue
			}
			keepDefault := !defaultAssigned && (!hasExplicitDefault || isExplicitDefaultPoolMod(mod.ID, explicitDefaultPools))
			if keepDefault {
				defaultAssigned = true
			} else {
				mod.Settings["is_default"] = false
			}
		}
	}
}

func normalizeDWSAndPlacementHostMaintenance(groups []ast.DeploymentGroup, basePath string, mergedVars map[string]any) {
	for gi := range groups {
		for mi := range groups[gi].Modules {
			mod := &groups[gi].Modules[mi]
			if !mod.IsComputePool || mod.Settings == nil {
				continue
			}
			dwsEnabled := false
			if dwsMap, ok := mod.Settings["dws_flex"].(map[string]any); ok {
				dwsEnabled = isFlexStartEnabled(dwsMap["enabled"], mergedVars)
			}
			if dwsEnabled || isFlexStartEnabled(mod.Settings["enable_placement"], mergedVars) {
				if inputs := getValidInputs(*mod, basePath); len(inputs) == 0 || inputs["on_host_maintenance"] {
					mod.Settings["on_host_maintenance"] = "TERMINATE"
				}
			}
		}
	}
}

func disambiguateDuplicatePoolNamePrefixes(groups []ast.DeploymentGroup) {
	seenPrefixes := make(map[string]int)
	for gi := range groups {
		for mi := range groups[gi].Modules {
			mod := &groups[gi].Modules[mi]
			if !mod.IsComputePool || mod.Settings == nil {
				continue
			}
			if prefix, ok := mod.Settings["name_prefix"].(string); ok && prefix != "" {
				seenPrefixes[prefix]++
				if seenPrefixes[prefix] > 1 && mod.PoolName != "" {
					mod.Settings["name_prefix"] = fmt.Sprintf("%s-%s", prefix, mod.PoolName)
				}
			}
		}
	}
}

func normalizePostFeatureGKE(
	groups []ast.DeploymentGroup,
	basePath string,
	mergedVars map[string]any,
	userDeclaredVars map[string]bool,
) {
	for gi := range groups {
		for mi := range groups[gi].Modules {
			mod := &groups[gi].Modules[mi]
			if mod.IsComputePool && mod.Settings != nil {
				normalizeGKEPoolProvisioning(mod, basePath, mergedVars, userDeclaredVars)
			}
		}
	}
	backfillKueueAcceleratorType(groups, mergedVars)
}

func normalizeGKEPoolProvisioning(mod *ast.ModuleSpec, basePath string, mergedVars map[string]any, userDeclaredVars map[string]bool) {
	flexStart := isFlexStartEnabled(mod.Settings["enable_flex_start"], mergedVars)
	queuedProv := isFlexStartEnabled(mod.Settings["enable_queued_provisioning"], mergedVars)
	autoscaleOrInitial := hasAutoscalingOrInitialNodeCount(mod.Settings)
	if flexStart {
		if inputs := getValidInputs(*mod, basePath); len(inputs) == 0 || inputs["auto_repair"] {
			mod.Settings["auto_repair"] = false
		}
	}
	rawSNC, hasSNC := mod.Settings["static_node_count"]
	if !hasSNC {
		return
	}
	isVarExpr := false
	if snc, ok := rawSNC.(string); ok {
		isVarExpr = strings.Contains(snc, "$(") || strings.Contains(snc, "((")
	}
	if rawSNC == nil || flexStart || queuedProv || (autoscaleOrInitial && isVarExpr) {
		unmarkDroppedStaticNodeCountVars(rawSNC, userDeclaredVars)
		delete(mod.Settings, "static_node_count")
	}
}

func hasAutoscalingOrInitialNodeCount(settings map[string]any) bool {
	for _, k := range []string{"autoscaling_total_min_nodes", "autoscaling_total_max_nodes", "autoscaling_min_node_count", "autoscaling_max_node_count", "initial_node_count"} {
		if settings[k] != nil {
			return true
		}
	}
	return false
}

func unmarkDroppedStaticNodeCountVars(rawSNC any, userDeclaredVars map[string]bool) {
	if snc, ok := rawSNC.(string); ok {
		for varName := range userDeclaredVars {
			if varRefPattern(varName).MatchString(snc) {
				delete(userDeclaredVars, varName)
			}
		}
		return
	}
	if rawSNC == nil {
		for varName := range userDeclaredVars {
			if varName == "cluster_size" || strings.HasSuffix(varName, "_cluster_size") {
				delete(userDeclaredVars, varName)
			}
		}
	}
}

func backfillKueueAcceleratorType(groups []ast.DeploymentGroup, mergedVars map[string]any) {
	defAccel, ok := mergedVars["kueue_accelerator_type"].(string)
	if !ok || strings.TrimSpace(defAccel) == "" {
		return
	}
	for gi := range groups {
		for mi := range groups[gi].Modules {
			applyKueueDefaultAccel(&groups[gi].Modules[mi], defAccel, mergedVars)
		}
	}
}

func applyKueueDefaultAccel(mod *ast.ModuleSpec, defAccel string, mergedVars map[string]any) {
	if mod.Settings == nil {
		return
	}
	kueueMap, ok := mod.Settings["kueue"].(map[string]any)
	if !ok || !isFlexStartEnabled(kueueMap["install"], mergedVars) {
		return
	}
	if cfgPath, hasPath := kueueMap["config_path"].(string); hasPath && strings.TrimSpace(cfgPath) != "" {
		return
	}
	tmplVars, ok := kueueMap["config_template_vars"].(map[string]any)
	if !ok || tmplVars == nil {
		tmplVars = make(map[string]any)
		kueueMap["config_template_vars"] = tmplVars
	}
	if accel, hasAccel := tmplVars["accelerator_type"].(string); !hasAccel || strings.TrimSpace(accel) == "" {
		tmplVars["accelerator_type"] = defAccel
	}
}

func isFlexStartEnabled(raw any, vars map[string]any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		trimmed := strings.TrimSpace(v)
		if strings.EqualFold(trimmed, "true") {
			return true
		}
		for _, prefix := range []string{"$(vars.", "((var."} {
			if strings.HasPrefix(trimmed, prefix) {
				varName := strings.TrimPrefix(trimmed, prefix)
				varName = strings.TrimSuffix(strings.TrimSuffix(varName, "))"), ")")
				if val, ok := vars[varName]; ok {
					if b, isBool := val.(bool); isBool {
						return b
					}
					if s, isStr := val.(string); isStr {
						return strings.EqualFold(strings.TrimSpace(s), "true")
					}
				}
			}
		}
	}
	return false
}

func resolveStringSetting(raw any, vars map[string]any) string {
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	trimmed := strings.TrimSpace(s)
	for _, prefix := range []string{"$(vars.", "((var."} {
		if strings.HasPrefix(trimmed, prefix) {
			varName := strings.TrimPrefix(trimmed, prefix)
			varName = strings.TrimSuffix(strings.TrimSuffix(varName, "))"), ")")
			if val, exists := vars[varName]; exists && val != nil {
				if strVal, isStr := val.(string); isStr {
					return strings.TrimSpace(strVal)
				}
				return strings.TrimSpace(fmt.Sprintf("%v", val))
			}
			return ""
		}
	}
	return trimmed
}

func validatePoolConsumptionSettings(groups []ast.DeploymentGroup, vars map[string]any, toolkitPath string) error {
	for _, g := range groups {
		for _, mod := range g.Modules {
			if !mod.IsComputePool || mod.Settings == nil {
				continue
			}
			if err := validateSinglePoolConsumption(mod, vars, toolkitPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSinglePoolConsumption(mod ast.ModuleSpec, vars map[string]any, toolkitPath string) error {
	poolLabel := mod.PoolName
	if poolLabel == "" {
		poolLabel = mod.ID
	}
	inputs := getValidInputs(mod, toolkitPath)
	if inputs["spot"] && inputs["reservation_affinity"] {
		return validateGKEPoolConsumption(mod, poolLabel, vars)
	}
	if inputs["enable_spot_vm"] && inputs["reservation_name"] {
		return validateSlurmPoolConsumption(mod, poolLabel, vars)
	}
	return nil
}

func validateGKEPoolConsumption(mod ast.ModuleSpec, poolLabel string, vars map[string]any) error {
	spot := isFlexStartEnabled(mod.Settings["spot"], vars)
	flexStart := isFlexStartEnabled(mod.Settings["enable_flex_start"], vars) ||
		isFlexStartEnabled(mod.Settings["enable_queued_provisioning"], vars)
	hasSpecificReservation := false
	if resAff, ok := mod.Settings["reservation_affinity"].(map[string]any); ok {
		consumeType := resolveStringSetting(resAff["consume_reservation_type"], vars)
		hasSpecificReservation = strings.EqualFold(consumeType, "SPECIFIC_RESERVATION")
	}
	if spot && hasSpecificReservation {
		return fmt.Errorf("pool %q cannot combine spot VMs (spot: true) with a specific reservation (reservation_affinity: SPECIFIC_RESERVATION)", poolLabel)
	}
	if spot && flexStart {
		return fmt.Errorf("pool %q cannot combine spot VMs (spot: true) with DWS Flex-Start (enable_flex_start: true)", poolLabel)
	}
	if flexStart && hasSpecificReservation {
		return fmt.Errorf("pool %q cannot combine DWS Flex-Start (enable_flex_start: true) with a specific reservation (reservation_affinity: SPECIFIC_RESERVATION)", poolLabel)
	}
	return nil
}

func validateSlurmPoolConsumption(mod ast.ModuleSpec, poolLabel string, vars map[string]any) error {
	spot := isFlexStartEnabled(mod.Settings["enable_spot_vm"], vars)
	dwsFlex := false
	if dwsMap, ok := mod.Settings["dws_flex"].(map[string]any); ok {
		dwsFlex = isFlexStartEnabled(dwsMap["enabled"], vars)
	}
	resName := resolveStringSetting(mod.Settings["reservation_name"], vars)
	hasReservation := resName != ""
	if spot && hasReservation {
		return fmt.Errorf("pool %q cannot combine spot VMs (enable_spot_vm: true) with a reservation (reservation_name: %q)", poolLabel, resName)
	}
	if spot && dwsFlex {
		return fmt.Errorf("pool %q cannot combine spot VMs (enable_spot_vm: true) with DWS Flex-Start (dws_flex.enabled: true)", poolLabel)
	}
	if dwsFlex && hasReservation {
		return fmt.Errorf("pool %q cannot combine DWS Flex-Start (dws_flex.enabled: true) with a reservation (reservation_name: %q)", poolLabel, resName)
	}
	return nil
}

func (c *Compiler) validateFallbackMachineType(machineType string, userConfig ast.ClusterConfig) error {
	zone, _ := userConfig.Vars["zone"].(string)
	if !validMachineTypePattern.MatchString(machineType) {
		available := c.loader.ListArchetypesForBase(userConfig.ConfigBase.Type)
		return fmt.Errorf("machine type %q does not exist in zone %q. Please check if its a valid machine type or You may specify any valid Google Cloud CPU machine type, or use an available GPU hardware archetype: %s", machineType, zone, strings.Join(available, ", "))
	}
	project, _ := userConfig.Vars["project_id"].(string)
	if project == "" || project == "null" || zone == "" || zone == "null" {
		return nil
	}
	if lookupMachineTypeNotFound(project, zone, machineType) {
		available := c.loader.ListArchetypesForBase(userConfig.ConfigBase.Type)
		return fmt.Errorf("machine type %q does not exist in zone %q. Please check if its a valid machine type or You may specify any valid Google Cloud CPU machine type, or use an available GPU hardware archetype: %s", machineType, zone, strings.Join(available, ", "))
	}
	return nil
}

func lookupMachineTypeNotFound(project, zone, machineType string) bool {
	pzKey := project + "/" + zone
	if _, ok := unverifiableProjectZone.Load(pzKey); ok {
		return false
	}
	mtKey := pzKey + "/" + machineType
	if cached, ok := machineTypeNotFound.Load(mtKey); ok {
		return cached.(bool)
	}
	_, err := config.GetMachineType(project, zone, machineType)
	if err == nil {
		machineTypeNotFound.Store(mtKey, false)
		return false
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == 404 && strings.Contains(apiErr.Message, "machineTypes") {
		machineTypeNotFound.Store(mtKey, true)
		return true
	}
	unverifiableProjectZone.Store(pzKey, true)
	return false
}
