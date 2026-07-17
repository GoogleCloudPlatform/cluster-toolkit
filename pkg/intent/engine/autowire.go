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
	"sort"
	"strings"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/modulereader"

	"github.com/zclconf/go-cty/cty"
)

type moduleCache struct {
	inputs        []string
	collectionMap map[string]bool
	outputs       []string
	// inputDefaults maps an input's name to the default value declared in the
	// module's variables.tf. This ensures unset settings are compared by their
	// effective default value rather than treated as absent.
	inputDefaults map[string]any
	// compositeFields maps an input's name to the attribute names of its object type.
	// Since Terraform outputs are untyped, the shape of a composite value is derived
	// from consumer variable definitions.
	compositeFields map[string][]string
}

type exportedInterface struct {
	ID                string
	Source            string
	Tier              int
	FeatureInstanceID string
	AttachTo          []string
	IsComputePool     bool
	Outputs           []string
}

// Autowire scans all deployment groups and dynamically connects modules according to rules for
// echo suppression, collection aggregation, feature isolation, and fail-fast ambiguity detection.
func Autowire(groups []ast.DeploymentGroup, toolkitPath string) ([]ast.DeploymentGroup, error) {
	cache := make(map[string]moduleCache)

	if err := checkDuplicateLocalMounts(groups, cache); err != nil {
		return nil, err
	}

	consumedOutputsByUse := collectConsumedOutputsByUse(groups, cache)
	compositeIndex, collectionInputs, err := buildCompositeFieldIndex(groups, cache)
	if err != nil {
		return nil, err
	}

	exports, err := discoverExportedProducers(groups, consumedOutputsByUse, compositeIndex, collectionInputs, cache)
	if err != nil || len(exports) == 0 {
		return groups, err
	}

	sortExportsByTier(exports)
	poolNames := getPoolNames(groups)
	accelPools := collectAcceleratorPools(groups)
	scalarClaims := map[string]map[string][]wiringClaim{}

	for i := range groups {
		for j := range groups[i].Modules {
			if err := wireConsumerModule(&groups[i].Modules[j], exports, groups, poolNames, accelPools, cache, scalarClaims); err != nil {
				return nil, err
			}
		}
	}

	if reports := findAmbiguousWiring(scalarClaims); len(reports) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(reports, "\n\n"))
	}

	return groups, nil
}

func collectConsumedOutputsByUse(groups []ast.DeploymentGroup, cache map[string]moduleCache) map[string]map[string]bool {
	consumed := make(map[string]map[string]bool)
	for _, group := range groups {
		for _, mod := range group.Modules {
			recordModuleConsumedUse(mod, groups, cache, consumed)
		}
	}
	return consumed
}

func recordModuleConsumedUse(mod ast.ModuleSpec, groups []ast.DeploymentGroup, cache map[string]moduleCache, consumed map[string]map[string]bool) {
	if len(mod.Use) == 0 {
		return
	}
	cInfo, err := getCachedInfo(mod.Source, cache)
	if err != nil {
		return
	}
	cIn := make(map[string]bool, len(cInfo.inputs))
	for _, in := range cInfo.inputs {
		cIn[in] = true
	}
	for _, u := range mod.Use {
		for out := range getProvidedOutputs([]string{u}, groups, cache) {
			if cIn[out] {
				if consumed[u] == nil {
					consumed[u] = make(map[string]bool)
				}
				consumed[u][out] = true
			}
		}
	}
}

func discoverExportedProducers(groups []ast.DeploymentGroup, consumedOutputsByUse, compositeIndex map[string]map[string]bool, collectionInputs map[string]bool, cache map[string]moduleCache) ([]exportedInterface, error) {
	var exports []exportedInterface
	for _, group := range groups {
		for _, mod := range group.Modules {
			if !mod.Export {
				continue
			}
			exp, err := buildExportedInterface(mod, groups, consumedOutputsByUse, compositeIndex, collectionInputs, cache)
			if err != nil {
				return nil, err
			}
			exports = append(exports, exp)
		}
	}
	return exports, nil
}

func buildExportedInterface(mod ast.ModuleSpec, groups []ast.DeploymentGroup, consumedOutputsByUse, compositeIndex map[string]map[string]bool, collectionInputs map[string]bool, cache map[string]moduleCache) (exportedInterface, error) {
	info, err := getCachedInfo(mod.Source, cache)
	if err != nil {
		return exportedInterface{}, err
	}
	upstreamOutputs := getProvidedOutputs(mod.Use, groups, cache)
	inputSet := make(map[string]bool, len(info.inputs))
	for _, in := range info.inputs {
		inputSet[in] = true
	}
	settingSet := make(map[string]bool, len(mod.Settings))
	for k := range mod.Settings {
		settingSet[k] = true
	}

	var filteredOutputs []string
	for _, out := range info.outputs {
		if inputSet[out] && (upstreamOutputs[out] || settingSet[out]) {
			continue
		}
		if isProjectionOfSibling(out, info.outputs, compositeIndex, collectionInputs) {
			continue
		}
		if consumedOutputsByUse[mod.ID][out] {
			continue
		}
		filteredOutputs = append(filteredOutputs, out)
	}
	return exportedInterface{
		ID:                mod.ID,
		Source:            mod.Source,
		Tier:              mod.Tier,
		FeatureInstanceID: mod.FeatureInstanceID,
		AttachTo:          mod.AttachTo,
		IsComputePool:     mod.IsComputePool,
		Outputs:           filteredOutputs,
	}, nil
}

func sortExportsByTier(exports []exportedInterface) {
	sort.SliceStable(exports, func(i, j int) bool {
		ti, tj := exports[i].Tier, exports[j].Tier
		if ti == 0 {
			ti = 3
		}
		if tj == 0 {
			tj = 3
		}
		return ti < tj
	})
}

func collectAcceleratorPools(groups []ast.DeploymentGroup) map[string]bool {
	accelPools := make(map[string]bool)
	for _, g := range groups {
		for _, m := range g.Modules {
			if isAcceleratorComputePool(m) {
				accelPools[m.ID] = true
			}
		}
	}
	return accelPools
}

func wireConsumerModule(mod *ast.ModuleSpec, exports []exportedInterface, groups []ast.DeploymentGroup, poolNames []string, accelPools map[string]bool, cache map[string]moduleCache, scalarClaims map[string]map[string][]wiringClaim) error {
	info, err := getCachedInfo(mod.Source, cache)
	if err != nil {
		return err
	}
	pruneShadowedUseEdges(mod, info, groups, cache)
	unsatisfied, openScalars := collectUnsatisfiedInputs(mod, info, groups, cache)

	for _, exp := range exports {
		eligible, crossScope, crossFeatureOverlay := evaluateProducerEligibility(mod, info, exp, groups, poolNames, accelPools, cache)
		if !eligible {
			continue
		}
		recordScalarWiringClaims(mod.ID, exp, crossScope, crossFeatureOverlay, openScalars, scalarClaims)
		unsatisfied = applyProducerOutputs(mod, info, exp, crossScope, crossFeatureOverlay, unsatisfied)
	}
	return nil
}

func pruneShadowedUseEdges(mod *ast.ModuleSpec, info moduleCache, groups []ast.DeploymentGroup, cache map[string]moduleCache) {
	if len(mod.Use) == 0 || len(mod.Settings) == 0 {
		return
	}
	inputSet := make(map[string]bool, len(info.inputs))
	for _, in := range info.inputs {
		inputSet[in] = true
	}
	var prunedUse []string
	for _, u := range mod.Use {
		if shouldKeepUseEdge(u, mod.Settings, inputSet, groups, cache) {
			prunedUse = append(prunedUse, u)
		}
	}
	mod.Use = prunedUse
}

func shouldKeepUseEdge(u string, settings map[string]any, inputSet map[string]bool, groups []ast.DeploymentGroup, cache map[string]moduleCache) bool {
	uOuts := getProvidedOutputs([]string{u}, groups, cache)
	matchedAny := false
	for out := range uOuts {
		if inputSet[out] {
			matchedAny = true
			if _, explicitlySet := settings[out]; !explicitlySet {
				return true
			}
		}
	}
	return !matchedAny
}

func collectUnsatisfiedInputs(mod *ast.ModuleSpec, info moduleCache, groups []ast.DeploymentGroup, cache map[string]moduleCache) ([]string, map[string]bool) {
	provided := getProvidedOutputs(mod.Use, groups, cache)
	var unsatisfied []string
	openScalars := make(map[string]bool)
	for _, in := range info.inputs {
		if _, ok := mod.Settings[in]; ok {
			continue
		}
		if !provided[in] || info.collectionMap[in] {
			unsatisfied = append(unsatisfied, in)
			if !info.collectionMap[in] {
				openScalars[in] = true
			}
		}
	}
	return unsatisfied, openScalars
}

func evaluateProducerEligibility(mod *ast.ModuleSpec, info moduleCache, exp exportedInterface, groups []ast.DeploymentGroup, poolNames []string, accelPools map[string]bool, cache map[string]moduleCache) (bool, bool, bool) {
	ok, crossFeatureOverlay := canWireFeatureScope(mod, exp, accelPools)
	if !ok {
		return false, false, false
	}
	if !checkAttachEligibility(mod, info, exp, crossFeatureOverlay, groups, poolNames, cache) {
		return false, false, false
	}
	ok, crossScope := canWirePoolScope(mod, exp, groups, poolNames)
	return ok, crossScope, crossFeatureOverlay
}

func canWireFeatureScope(mod *ast.ModuleSpec, exp exportedInterface, accelPools map[string]bool) (bool, bool) {
	if exp.ID == mod.ID || exp.Source == mod.Source || isSkippedNonAccelPool(mod, exp, accelPools) {
		return false, false
	}
	crossFeatureOverlay := mod.Tier == 5 && exp.Tier == 4 && mod.FeatureInstanceID != exp.FeatureInstanceID
	if !crossFeatureOverlay && mod.FeatureInstanceID != "" && exp.FeatureInstanceID != "" && mod.FeatureInstanceID != exp.FeatureInstanceID {
		return false, false
	}
	return true, crossFeatureOverlay
}

func isSkippedNonAccelPool(mod *ast.ModuleSpec, exp exportedInterface, accelPools map[string]bool) bool {
	return mod.Tier == 5 && len(mod.AttachTo) == 0 && len(accelPools) > 0 && exp.IsComputePool && !accelPools[exp.ID]
}

func canWirePoolScope(mod *ast.ModuleSpec, exp exportedInterface, groups []ast.DeploymentGroup, poolNames []string) (bool, bool) {
	producerPrefix := getPoolPrefix(exp.ID, poolNames)
	consumerPrefix := getPoolPrefix(mod.ID, poolNames)
	if producerPrefix != "" && consumerPrefix != "" && producerPrefix != consumerPrefix {
		return false, false
	}
	if dependsOn(exp.ID, mod.ID, groups) {
		return false, false
	}
	return true, producerPrefix != "" && consumerPrefix == ""
}

func checkAttachEligibility(mod *ast.ModuleSpec, info moduleCache, exp exportedInterface, crossFeatureOverlay bool, groups []ast.DeploymentGroup, poolNames []string, cache map[string]moduleCache) bool {
	if len(mod.AttachTo) > 0 && exp.Tier >= 3 && !crossFeatureOverlay {
		if !matchesAnyAttachTarget(exp.ID, exp.FeatureInstanceID, mod.AttachTo, poolNames) {
			return false
		}
	}
	if len(exp.AttachTo) > 0 && exp.Tier >= 4 && (!crossFeatureOverlay || len(mod.AttachTo) > 0) {
		bridgeRoleTargetOutputs(mod, info, exp, groups, poolNames, cache)
		if !matchesAnyAttachTarget(mod.ID, mod.FeatureInstanceID, exp.AttachTo, poolNames) {
			return false
		}
	}
	return true
}

func matchesAnyAttachTarget(id, featureInstanceID string, targets, poolNames []string) bool {
	for _, target := range targets {
		if matchesAttachTarget(id, featureInstanceID, target, poolNames) {
			return true
		}
	}
	return false
}

func bridgeRoleTargetOutputs(mod *ast.ModuleSpec, info moduleCache, exp exportedInterface, groups []ast.DeploymentGroup, poolNames []string, cache map[string]moduleCache) {
	for _, target := range exp.AttachTo {
		for _, out := range exp.Outputs {
			if targetAcceptsOutputDirectly(groups, target, out, poolNames, cache) {
				continue
			}
			bridgedInput := strings.ToLower(target) + "_" + out
			if !containsString(info.inputs, bridgedInput) {
				continue
			}
			injectBridgedOutput(mod, info, bridgedInput, fmt.Sprintf("$(%s.%s)", exp.ID, out))
		}
	}
}

func injectBridgedOutput(mod *ast.ModuleSpec, info moduleCache, bridgedInput, refExpr string) {
	if mod.Settings == nil {
		mod.Settings = make(map[string]any)
	}
	if info.collectionMap[bridgedInput] {
		existing := toAnySlice(mod.Settings[bridgedInput])
		for _, item := range existing {
			if s, ok := item.(string); ok && s == refExpr {
				return
			}
		}
		mod.Settings[bridgedInput] = append(existing, refExpr)
	} else if _, exists := mod.Settings[bridgedInput]; !exists {
		mod.Settings[bridgedInput] = refExpr
	}
}

func recordScalarWiringClaims(modID string, exp exportedInterface, crossScope, crossFeatureOverlay bool, openScalars map[string]bool, scalarClaims map[string]map[string][]wiringClaim) {
	if crossScope || crossFeatureOverlay {
		return
	}
	tier := exp.Tier
	if tier == 0 {
		tier = 3
	}
	for _, out := range exp.Outputs {
		if !openScalars[out] {
			continue
		}
		if scalarClaims[modID] == nil {
			scalarClaims[modID] = map[string][]wiringClaim{}
		}
		scalarClaims[modID][out] = append(scalarClaims[modID][out], wiringClaim{exp.ID, tier})
	}
}

func applyProducerOutputs(mod *ast.ModuleSpec, info moduleCache, exp exportedInterface, crossScope, crossFeatureOverlay bool, unsatisfied []string) []string {
	matchedVars := getIntersection(exp.Outputs, unsatisfied)
	if crossScope || crossFeatureOverlay {
		var collectionsOnly []string
		for _, v := range matchedVars {
			if info.collectionMap[v] {
				collectionsOnly = append(collectionsOnly, v)
			}
		}
		matchedVars = collectionsOnly
	}
	if len(matchedVars) == 0 {
		return unsatisfied
	}
	if !containsString(mod.Use, exp.ID) {
		mod.Use = append(mod.Use, exp.ID)
	}
	var remainingUnsatisfied []string
	for _, u := range unsatisfied {
		if containsString(matchedVars, u) && !info.collectionMap[u] {
			continue
		}
		remainingUnsatisfied = append(remainingUnsatisfied, u)
	}
	return remainingUnsatisfied
}

// checkDuplicateLocalMounts enforces Law 7 for storage mount points.
//
// A module's mount path has two possible origins, and BOTH must be considered:
//
// Modules that declare no `local_mount` input at all are skipped, so this is a
// no-op for compute, networking and scheduler modules.
func checkDuplicateLocalMounts(groups []ast.DeploymentGroup, cache map[string]moduleCache) error {
	// mountPath -> modules that claimed it.
	type claim struct {
		modID      string
		fromModule bool // true if inherited from variables.tf rather than declared
		attachTo   []string
	}
	seenMounts := make(map[string][]claim)

	for _, g := range groups {
		for _, m := range g.Modules {
			mountPath, inherited, ok := resolveLocalMount(m, cache)
			if !ok {
				continue
			}
			for _, prev := range seenMounts[mountPath] {
				if prev.modID != m.ID && attachScopesOverlap(prev.attachTo, m.AttachTo) {
					return fmt.Errorf(
						"ambiguous storage configuration: duplicate local_mount path %q claimed by modules %q (%s) and %q (%s); "+
							"set a distinct `local_mount` on one of them",
						mountPath,
						prev.modID, mountOrigin(prev.fromModule),
						m.ID, mountOrigin(inherited),
					)
				}
			}
			seenMounts[mountPath] = append(seenMounts[mountPath], claim{
				modID:      m.ID,
				fromModule: inherited,
				attachTo:   m.AttachTo,
			})
		}
	}
	return nil
}

func attachScopesOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, ta := range a {
		for _, tb := range b {
			if strings.EqualFold(ta, tb) {
				return true
			}
		}
	}
	return false
}

// mountOrigin renders where a mount path came from, so the error message tells
// the user whether they need to look in their config or in the module default.
func mountOrigin(inherited bool) string {
	if inherited {
		return "module default"
	}
	return "explicitly set"
}

func normalizeMountPath(p string) string {
	trimmed := strings.TrimSpace(p)
	if len(trimmed) > 1 {
		trimmed = strings.TrimRight(trimmed, "/")
	}
	return trimmed
}

// resolveLocalMount returns a module's effective mount path. The second return
// value reports whether the value was inherited from variables.tf rather than
// declared in settings; the third reports whether the module has a mount at all.
func resolveLocalMount(m ast.ModuleSpec, cache map[string]moduleCache) (string, bool, bool) {
	for _, key := range []string{"local_mount", "pv_mount_path"} {
		if raw, ok := m.Settings[key]; ok {
			// An explicit empty string is treated as "unset" so that blanking the
			// value in a manifest falls through to the module default, which is
			// what Terraform itself would do.
			if s, ok := raw.(string); ok && s != "" {
				return normalizeMountPath(s), false, true
			}
		}
	}

	info, err := getCachedInfo(m.Source, cache)
	if err != nil {
		// Unreadable module: stay silent rather than fail the compile on a
		// check that is only meant to catch a specific ambiguity.
		return "", false, false
	}
	for _, key := range []string{"local_mount", "pv_mount_path"} {
		if def, ok := info.inputDefaults[key]; ok {
			if s, ok := def.(string); ok && s != "" {
				return normalizeMountPath(s), true, true
			}
		}
	}
	return "", false, false
}

func getPoolNames(groups []ast.DeploymentGroup) []string {
	poolSet := make(map[string]bool)
	for _, g := range groups {
		for _, cp := range g.Modules {
			if cp.PoolName != "" {
				poolSet[cp.PoolName] = true
			} else if cp.IsComputePool {
				if idx := strings.LastIndex(cp.ID, "_"); idx != -1 {
					poolName := cp.ID[:idx]
					poolSet[poolName] = true
				}
			}
		}
	}
	var pools []string
	for p := range poolSet {
		pools = append(pools, p)
	}
	sort.Slice(pools, func(i, j int) bool {
		return len(pools[i]) > len(pools[j])
	})
	return pools
}

func getPoolPrefix(id string, poolNames []string) string {
	for _, p := range poolNames {
		if strings.HasPrefix(id, p+"_") {
			return p
		}
	}
	return ""
}

// targetAcceptsOutputDirectly reports whether any module matching `target`
// directly declares `outputName` as a Terraform input variable.
func targetAcceptsOutputDirectly(groups []ast.DeploymentGroup, target, outputName string, poolNames []string, cache map[string]moduleCache) bool {
	for _, g := range groups {
		for _, m := range g.Modules {
			if !matchesAttachTarget(m.ID, m.FeatureInstanceID, target, poolNames) {
				continue
			}
			info, err := getCachedInfo(m.Source, cache)
			if err != nil {
				continue
			}
			if containsString(info.inputs, outputName) {
				return true
			}
		}
	}
	return false
}

func getProvidedOutputs(useIDs []string, groups []ast.DeploymentGroup, cache map[string]moduleCache) map[string]bool {
	provided := make(map[string]bool)
	for _, id := range useIDs {
		source := findSourceByID(id, groups)
		if source != "" {
			info, _ := getCachedInfo(source, cache)
			for _, out := range info.outputs {
				provided[out] = true
			}
		}
	}
	return provided
}

func findSourceByID(id string, groups []ast.DeploymentGroup) string {
	for _, g := range groups {
		for _, m := range g.Modules {
			if m.ID == id {
				return m.Source
			}
		}
	}
	return ""
}

func getCachedInfo(source string, cache map[string]moduleCache) (moduleCache, error) {
	if source == "" {
		return moduleCache{}, nil
	}
	if info, ok := cache[source]; ok {
		return info, nil
	}

	info, err := modulereader.GetModuleInfo(source, "terraform")
	if err != nil {
		return moduleCache{}, fmt.Errorf("failed to read module info for %q: %w", source, err)
	}

	mc := moduleCache{
		collectionMap:   make(map[string]bool),
		compositeFields: make(map[string][]string),
		inputDefaults:   make(map[string]any),
	}
	for _, in := range info.Inputs {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(in.Description)), "DEPRECATED") {
			continue
		}
		mc.inputs = append(mc.inputs, in.Name)
		if in.Default != nil {
			mc.inputDefaults[in.Name] = in.Default
		}
		// Law 3: Identify collection inputs (list, set, tuple)
		if in.Type.IsListType() || in.Type.IsSetType() || in.Type.IsTupleType() {
			mc.collectionMap[in.Name] = true
		}
		// Law 4a source data: if this input is a bundle, remember what is inside it.
		if fields := objectFieldNames(in.Type); len(fields) > 0 {
			mc.compositeFields[in.Name] = fields
		}
	}
	for _, out := range info.Outputs {
		mc.outputs = append(mc.outputs, out.Name)
	}
	cache[source] = mc
	return mc, nil
}

// objectFieldNames returns the attribute names of an object type, first unwrapping any
// list/set/map wrapper. Anything that is not ultimately an object yields nil.
//
// `list(object({server_ip = string, mount_runner = map(string)}))` therefore yields
// ["mount_runner", "server_ip"]. Tuples are excluded on purpose: their elements are
// positional and unnamed, so they cannot tell us a field name.
func objectFieldNames(ty cty.Type) []string {
	for ty.IsListType() || ty.IsSetType() || ty.IsMapType() {
		ty = ty.ElementType()
	}
	if !ty.IsObjectType() {
		return nil
	}
	attrs := ty.AttributeTypes()
	names := make([]string, 0, len(attrs))
	for n := range attrs {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic; the index is a set, but tests read this
	return names
}

// buildCompositeFieldIndex maps a composite value's NAME to the set of field names it
// carries, e.g. "network_storage" -> {server_ip, remote_mount, mount_runner, ...}.
//
// It is built from INPUT declarations, not outputs, because Terraform `output` blocks are
// untyped -- there is no `type =` attribute on an output. The only machine-readable
// statement of a composite's shape is the consumer's variable block, e.g. vm-instance's
//
//	variable "network_storage" { type = list(object({ ... mount_runner = map(string) })) }
//
// Every module in the graph contributes, so a composite is known as long as ANY module
// present declares it. In practice that is guaranteed: exporting a composite nothing
// consumes would be a no-op, and 16 modules declare `network_storage` alone.
func buildCompositeFieldIndex(groups []ast.DeploymentGroup, cache map[string]moduleCache) (map[string]map[string]bool, map[string]bool, error) {
	index := make(map[string]map[string]bool)
	collections := make(map[string]bool)
	for _, g := range groups {
		for _, m := range g.Modules {
			if m.Source == "" {
				continue
			}
			info, err := getCachedInfo(m.Source, cache)
			if err != nil {
				return nil, nil, err
			}
			for col := range info.collectionMap {
				collections[col] = true
			}
			for composite, fields := range info.compositeFields {
				if index[composite] == nil {
					index[composite] = make(map[string]bool)
				}
				for _, f := range fields {
					index[composite][f] = true
				}
			}
		}
	}
	return index, collections, nil
}

// isProjectionOfSibling reports whether `out` is a loose scalar output on a module
// that already publishes a composite object bundle (`sib` in `index`).
func isProjectionOfSibling(out string, siblingOutputs []string, index map[string]map[string]bool, collectionInputs map[string]bool) bool {
	if len(index[out]) > 0 {
		return false
	}
	for _, sib := range siblingOutputs {
		if sib == out {
			continue
		}
		if len(index[sib]) > 0 && (!collectionInputs[out] || index[sib][out]) {
			return true
		}
	}
	return false
}

func getIntersection(a, b []string) []string {
	m := make(map[string]bool)
	for _, x := range a {
		m[x] = true
	}
	var res []string
	for _, x := range b {
		if m[x] {
			res = append(res, x)
		}
	}
	return res
}

func hasIntersection(a, b []string) bool {
	return len(getIntersection(a, b)) > 0
}

func containsString(arr []string, s string) bool {
	for _, x := range arr {
		if x == s {
			return true
		}
	}
	return false
}

func dependsOn(targetID, searchID string, groups []ast.DeploymentGroup) bool {
	visited := make(map[string]bool)
	queue := []string{targetID}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr == searchID {
			return true
		}
		if visited[curr] {
			continue
		}
		visited[curr] = true
		for _, g := range groups {
			for _, m := range g.Modules {
				if m.ID == curr {
					queue = append(queue, m.Use...)
				}
			}
		}
	}
	return false
}

// wiringClaim is one producer that was eligible to satisfy a consumer's scalar input.
type wiringClaim struct {
	producerID string
	tier       int
}

// findAmbiguousWiring reports scalar inputs that two or more producers could have
// satisfied equally well.
//
// Autowire picks the first eligible producer in tier order. When the winning tier holds
// exactly one candidate that is a real decision: higher tiers deliberately outrank lower
// ones. When it holds two or more, the winner is decided by slice order -- deterministic,
// but arbitrary, and the user has no way to know a choice was made on their behalf.
//
// Observed in hpc-slurm.yaml: `hpc-cpu_nodeset.startup_script` had both `gromacs_setup`
// and `hpl-bench_setup` available. gromacs won; the ramble runner was dropped silently.
//
// Inputs backed by a single producer, or by a global, never appear here -- a user should
// never be asked to restate something with only one possible answer.
func findAmbiguousWiring(claims map[string]map[string][]wiringClaim) []string {
	var reports []string

	consumers := make([]string, 0, len(claims))
	for c := range claims {
		consumers = append(consumers, c)
	}
	sort.Strings(consumers)

	for _, consumer := range consumers {
		byInput := claims[consumer]
		inputs := make([]string, 0, len(byInput))
		for in := range byInput {
			inputs = append(inputs, in)
		}
		sort.Strings(inputs)

		for _, input := range inputs {
			candidates := byInput[input]
			if len(candidates) < 2 {
				continue
			}

			// Only candidates at the winning (lowest) tier are genuinely competing.
			best := candidates[0].tier
			for _, c := range candidates[1:] {
				if c.tier < best {
					best = c.tier
				}
			}
			var tied []string
			for _, c := range candidates {
				if c.tier == best {
					tied = append(tied, c.producerID)
				}
			}
			if len(tied) < 2 {
				continue
			}
			sort.Strings(tied)

			reports = append(reports, fmt.Sprintf(
				"ambiguous wiring for input %q on module %q: %d modules export it (%s). "+
					"Autowire cannot choose. Disambiguate by naming the one you want:\n"+
					"\n    use: [%s]\n",
				input, consumer, len(tied), strings.Join(tied, ", "), tied[0]))
		}
	}
	return reports
}

func isAcceleratorComputePool(mod ast.ModuleSpec) bool {
	if !mod.IsComputePool {
		return false
	}
	if ga, ok := mod.Settings["guest_accelerator"]; ok && ga != nil {
		return true
	}
	if pp, ok := mod.Settings["placement_policy"].(map[string]any); ok {
		if topo, hasTopo := pp["tpu_topology"]; hasTopo && topo != nil && topo != "" {
			return true
		}
	}
	return false
}
