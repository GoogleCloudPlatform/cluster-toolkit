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

package gke

import (
	"cmp"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"slices"
	"strconv"
	"strings"
)

var defaultTPUHardwareResolver = gceTPUHardwareResolver{}

// gceTPUFamilies centralizes GCE/GKE/TPU-slice family tokens and observed cluster-label shorthands
// (such as ironwood, v7x, tpu7x-podslice, v5lite-*, and tpu-v*-slice) locally to gceTPUHardwareResolver
// without expanding user-facing config.AcceleratorShorthandMap.
var gceTPUFamilies = []gceTPUFamilySpec{
	{
		canonical:      "tpu7x",
		normTokens:     []string{"tpu7x", "tpu-v7x", "tpu-7x", "ironwood"},
		shorthandBases: []string{"tpu7", "tpu7x", "tpu-7x", "tpu-v7x", "v7x", "ironwood"},
		tpu7xChipRules: true,
	},
	{
		canonical:      "v6e",
		normTokens:     []string{"ct6e", "v6e", "trillium"},
		shorthandBases: []string{"v6e", "tpu-v6e", "trillium"},
	},
	{
		canonical:      "v5p",
		normTokens:     []string{"ct5p", "v5p"},
		shorthandBases: []string{"v5p", "tpu-v5p"},
	},
	{
		canonical:      "v5e",
		normTokens:     []string{"ct5lp", "ct5l", "v5e", "v5lite", "v5-lite"},
		shorthandBases: []string{"v5e", "v5lite", "v5-lite", "v5litepod", "tpu-v5e", "tpu-v5-lite", "tpu-v5-lite-podslice", "tpu-v5-lite-device"},
	},
	{
		canonical:      "v4",
		normTokens:     []string{"ct4p", "tpu-v4"},
		shorthandBases: []string{"v4", "tpu-v4"},
	},
}

func (gceTPUHardwareResolver) IsTPUMachine(machineType string) bool {
	s := strings.ToLower(strings.TrimSpace(machineType))
	if s == "" {
		return false
	}
	return isKnownGCETPUMachine(s) || isGCEAcceleratorShorthand(s)
}

func (gceTPUHardwareResolver) NormalizeTPUType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	norm := strings.ReplaceAll(s, "_", "-")
	for _, fam := range gceTPUFamilies {
		if matchesNormalizedTPUFamily(norm, fam) {
			return fam.canonical
		}
	}
	return s
}

func (r gceTPUHardwareResolver) InferTPUMachine(tpuType, rawMachine string, chipsPerNode int) string {
	trimmedMachine := strings.TrimSpace(rawMachine)
	if trimmedMachine != "" && !isGCEAcceleratorShorthand(trimmedMachine) {
		return trimmedMachine
	}
	chips := chipsPerNode
	if chips <= 0 && trimmedMachine != "" {
		chips = r.ChipsPerNode(trimmedMachine)
	}
	normType := r.NormalizeTPUType(tpuType)
	if normType == "" && trimmedMachine != "" {
		normType = r.NormalizeTPUType(trimmedMachine)
	}
	return defaultGCETPUMachineType(normType, chips)
}

func (r gceTPUHardwareResolver) ChipsPerNode(machineType string) int {
	trimmed := strings.TrimSpace(machineType)
	if trimmed == "" || !r.IsTPUMachine(trimmed) {
		return 0
	}
	lower := strings.ToLower(trimmed)
	resolved := resolveGCETPUShorthand(lower)
	if isKnownGCETPUMachine(resolved) {
		if n, ok := parseStrictChipSuffix(resolved); ok {
			return n
		}
		return config.AcceleratorsPerVM(resolved)
	}
	return extraTPUShorthandChips(lower)
}

func resolveFallbackTPUChips(machineType string) int {
	if chips := defaultTPUHardwareResolver.ChipsPerNode(machineType); chips > 0 {
		return chips
	}
	return 4
}

func validateK8sPoolGPUs(pool k8s.PoolCapacity) (int, error) {
	if pool.GPUsPerNode <= 0 {
		return 0, fmt.Errorf("matched GPU pool %q reports 0 allocatable GPUs per node", cmp.Or(pool.MachineType, pool.GPUType))
	}
	return pool.GPUsPerNode, nil
}

func matchesNormalizedTPUFamily(norm string, fam gceTPUFamilySpec) bool {
	for _, tok := range fam.normTokens {
		if strings.Contains(norm, tok) {
			return true
		}
	}
	for _, base := range fam.shorthandBases {
		if norm == base || strings.HasPrefix(norm, base+"-") {
			return true
		}
	}
	return false
}

// parseGCETPUChipShorthand parses raw into a known GCE TPU family spec and optional numeric
// chip-count shorthand suffix (e.g. "v6e-16" -> (v6e, "16", true, true); "v6e" -> (v6e, "", false, true)).
func parseGCETPUChipShorthand(raw string) (fam gceTPUFamilySpec, chips string, isChipShorthand, isFam bool) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	fam, rest, isFam := matchGCEFamilyShorthand(lower)
	if !isFam {
		return gceTPUFamilySpec{}, "", false, false
	}
	if rest != "" && isAllDigits(rest) {
		return fam, rest, true, true
	}
	return fam, "", false, true
}

// resolveGCETPUShorthand delegates official shorthands to config.ResolveMachineType
// and normalizes cluster-label family aliases (e.g. v5lite-*, tpu-v6e-*, tpu-v5-lite-*)
// via gceTPUFamilies before lookup.
func resolveGCETPUShorthand(lower string) string {
	candidate := lower
	if fam, chips, isChip, _ := parseGCETPUChipShorthand(lower); isChip {
		candidate = fam.canonical + "-" + chips
	}
	if resolved := config.ResolveMachineType(candidate); resolved != candidate && config.IsTPU(resolved) {
		return strings.ToLower(resolved)
	}
	return lower
}

func parseStrictChipSuffix(machineType string) (int, bool) {
	lower := strings.ToLower(strings.TrimSpace(machineType))
	idx := strings.LastIndex(lower, "-")
	if idx < 0 || !strings.HasSuffix(lower, "t") {
		return 0, false
	}
	digits := lower[idx+1 : len(lower)-1]
	if digits == "" || !isAllDigits(digits) {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil && n > 0
}

func extraTPUShorthandChips(lower string) int {
	fam, rest, ok := matchGCEFamilyShorthand(lower)
	if !ok || !fam.tpu7xChipRules {
		return 0
	}
	if rest == "1" {
		return 1
	}
	return 4
}

func isKnownGCETPUMachine(lower string) bool {
	switch lower {
	case "tpu7x-standard", "ct6e-standard", "ct5p-hightpu", "ct5lp-hightpu", "ct5l-hightpu", "ct4p-hightpu":
		return true
	}
	if _, hasChipSuffix := parseStrictChipSuffix(lower); !hasChipSuffix {
		return false
	}
	for _, prefix := range []string{"tpu7x-", "ct6e-", "ct5p-", "ct5lp-", "ct5l-", "ct4p-"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isGCEAcceleratorShorthand(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || isKnownGCETPUMachine(s) {
		return false
	}
	if resolved := resolveGCETPUShorthand(s); resolved != s {
		return true
	}
	_, _, ok := matchGCEFamilyShorthand(s)
	return ok
}

func matchGCEFamilyShorthand(s string) (gceTPUFamilySpec, string, bool) {
	for _, fam := range gceTPUFamilies {
		for _, base := range fam.shorthandBases {
			if s == base {
				return fam, "", true
			}
			if rest, hasPrefix := strings.CutPrefix(s, base+"-"); hasPrefix && isValidShorthandSuffix(rest) {
				return fam, rest, true
			}
		}
	}
	return gceTPUFamilySpec{}, "", false
}

func isValidShorthandSuffix(rest string) bool {
	return (rest != "" && isAllDigits(rest)) || rest == "slice" || rest == "podslice"
}

func defaultGCETPUMachineType(tpuType string, chipsPerNode int) string {
	switch tpuType {
	case "tpu7x":
		if chipsPerNode == 1 {
			return "tpu7x-standard-1t"
		}
		return config.ResolveMachineType("tpu7x")
	case "v6e":
		return config.ResolveMachineType(fmt.Sprintf("v6e-%d", normalizeGCEChipCount(chipsPerNode)))
	case "v5p":
		return config.ResolveMachineType("v5p-8")
	case "v5e":
		return config.ResolveMachineType(fmt.Sprintf("v5e-%d", normalizeGCEChipCount(chipsPerNode)))
	case "v4":
		return config.ResolveMachineType("v4-8")
	default:
		return ""
	}
}

func normalizeGCEChipCount(chips int) int {
	switch chips {
	case 1, 4, 8:
		return chips
	default:
		return 4
	}
}

func allocatableMilliToWholeCPUs(milli int64) int {
	if milli <= 0 {
		return 0
	}
	return int((milli + 999) / 1000)
}

func (g *GKEOrchestrator) resolveGPUResourceName(opts ManifestOptions, profile JobProfile) string {
	if profile.GPUResourceName != "" {
		return profile.GPUResourceName
	}
	if g.mode == orchestrator.TypeK8s {
		if profile.K8sPool != nil && profile.K8sPool.GPUResourceName != "" {
			return profile.K8sPool.GPUResourceName
		}
		if mt := strings.TrimSpace(opts.MachineType); k8s.IsGPUResourceName(mt) {
			return mt
		}
		if ct := strings.TrimSpace(opts.ComputeType); k8s.IsGPUResourceName(ct) {
			return ct
		}
		if pool, ok := g.findK8sPoolForRequest(opts.ComputeType, opts.MachineType); ok && pool.GPUResourceName != "" {
			return pool.GPUResourceName
		}
	}
	return "nvidia.com/gpu"
}

func (g *GKEOrchestrator) calculateK8sMachineResourceLimits(opts ManifestOptions, profile JobProfile) (cpu, mem, gpu, tpu string, err error) {
	if profile.CapacityCount > 0 {
		if profile.IsTPU {
			return "", "", "", strconv.Itoa(profile.CapacityCount), nil
		}
		return "", "", strconv.Itoa(profile.CapacityCount), "", nil
	}
	if profile.K8sPool != nil {
		return limitsFromK8sPool(*profile.K8sPool)
	}
	if pool, ok := g.findK8sPoolForRequest(opts.ComputeType, opts.MachineType); ok {
		return limitsFromK8sPool(pool)
	}
	machineName := cmp.Or(strings.TrimSpace(opts.MachineType), strings.TrimSpace(opts.ComputeType))
	return "", "", "", "", fmt.Errorf("failed to determine accelerator capacity for machine type %s", machineName)
}

func limitsFromK8sPool(pool k8s.PoolCapacity) (cpu, mem, gpu, tpu string, err error) {
	if pool.IsTPU {
		chips := pool.TPUsPerNode
		if chips <= 0 {
			chips = resolveFallbackTPUChips(pool.MachineType)
		}
		return "", "", "", strconv.Itoa(chips), nil
	}
	if pool.IsGPU {
		gpus, err := validateK8sPoolGPUs(pool)
		if err != nil {
			return "", "", "", "", err
		}
		return "", "", strconv.Itoa(gpus), "", nil
	}
	return "", "", "", "", fmt.Errorf("pool %q has no accelerator capacity", pool.MachineType)
}

func selectBestK8sPool(candidates []k8s.PoolCapacity) (k8s.PoolCapacity, bool) {
	top := k8s.TopPreferredPools(candidates)
	if len(top) == 0 {
		return k8s.PoolCapacity{}, false
	}
	return top[0], true
}

func k8sPoolIdentityKey(pool k8s.PoolCapacity) string {
	mt := strings.ToLower(strings.TrimSpace(pool.MachineType))
	if pool.IsTPU {
		return "tpu:" + mt + ":" + strings.ToLower(strings.TrimSpace(pool.TPUType))
	}
	if pool.IsGPU {
		return "gpu:" + mt + ":" + strings.ToLower(strings.TrimSpace(pool.GPUType)) + ":" + strings.ToLower(strings.TrimSpace(pool.GPUResourceName))
	}
	return "cpu:" + mt
}

func k8sPoolDisplayLabel(pool k8s.PoolCapacity) string {
	if pool.IsTPU {
		base := cmp.Or(strings.TrimSpace(pool.MachineType), strings.TrimSpace(pool.TPUType), "tpu")
		if pool.TPUType != "" && !strings.EqualFold(pool.TPUType, base) {
			return fmt.Sprintf("%s (%s)", base, pool.TPUType)
		}
		return base
	}
	if pool.IsGPU {
		base := cmp.Or(strings.TrimSpace(pool.MachineType), strings.TrimSpace(pool.GPUType), strings.TrimSpace(pool.GPUResourceName), "gpu")
		var details []string
		if gt := strings.TrimSpace(pool.GPUType); gt != "" && !strings.EqualFold(gt, base) {
			details = append(details, gt)
		}
		if res := strings.TrimSpace(pool.GPUResourceName); res != "" && !strings.EqualFold(res, base) && !strings.EqualFold(res, pool.GPUType) && res != "nvidia.com/gpu" {
			details = append(details, res)
		}
		if len(details) > 0 {
			return fmt.Sprintf("%s (%s)", base, strings.Join(details, ", "))
		}
		return base
	}
	return cmp.Or(strings.TrimSpace(pool.MachineType), "cpu (unlabeled)")
}

func distinctK8sPools(pools []k8s.PoolCapacity) ([]k8s.PoolCapacity, []string) {
	seen := make(map[string]struct{}, len(pools))
	var distinct []k8s.PoolCapacity
	var labels []string
	for _, p := range pools {
		key := k8sPoolIdentityKey(p)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		distinct = append(distinct, p)
		labels = append(labels, k8sPoolDisplayLabel(p))
	}
	slices.Sort(labels)
	return distinct, labels
}

func (g *GKEOrchestrator) findK8sPoolForRequest(computeType, machineType string) (k8s.PoolCapacity, bool) {
	if g.inventory == nil {
		return k8s.PoolCapacity{}, false
	}
	ct := strings.TrimSpace(computeType)
	mt := strings.TrimSpace(machineType)
	if ct != "" {
		if pool, ok := g.findK8sGPUPoolByQuery(ct); ok {
			return pool, true
		}
	}
	if mt != "" {
		if pool, ok := g.findK8sPoolByMachineType(mt); ok {
			return pool, true
		}
		if pool, ok := g.findK8sGPUPoolByQuery(mt); ok {
			return pool, true
		}
		if defaultTPUHardwareResolver.IsTPUMachine(mt) {
			return g.findK8sTPUPoolForMachineType(mt)
		}
	}
	if ct != "" && ct != mt {
		if pool, ok := g.findK8sPoolByMachineType(ct); ok {
			return pool, true
		}
		if defaultTPUHardwareResolver.IsTPUMachine(ct) {
			return g.findK8sTPUPoolForMachineType(ct)
		}
	}
	return k8s.PoolCapacity{}, false
}

func (g *GKEOrchestrator) findK8sPoolByMachineType(machineType string) (k8s.PoolCapacity, bool) {
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if strings.EqualFold(pool.MachineType, machineType) {
			matches = append(matches, pool)
		}
	}
	return selectBestK8sPool(matches)
}

func (g *GKEOrchestrator) matchingK8sGPUPoolsByQuery(query string) []k8s.PoolCapacity {
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if !pool.IsGPU {
			continue
		}
		if strings.EqualFold(pool.GPUType, query) || strings.EqualFold(pool.GPUResourceName, query) || strings.EqualFold(name, query) {
			matches = append(matches, pool)
		}
	}
	return matches
}

func (g *GKEOrchestrator) findK8sGPUPoolByQuery(query string) (k8s.PoolCapacity, bool) {
	return selectBestK8sPool(g.matchingK8sGPUPoolsByQuery(query))
}

func (g *GKEOrchestrator) checkAmbiguousK8sPoolQuery(computeType, machineType string) error {
	if g.inventory == nil {
		return nil
	}
	q := strings.TrimSpace(computeType)
	if q != "" {
		gpuMatches := g.matchingK8sGPUPoolsByQuery(q)
		if len(gpuMatches) > 0 {
			distinct, labels := distinctK8sPools(k8s.TopPreferredPools(gpuMatches))
			if len(distinct) > 1 {
				return fmt.Errorf("--compute-type is required; %q matches multiple GPU pools, discovered: %s", q, strings.Join(labels, ", "))
			}
			return nil
		}
	}
	mt := strings.TrimSpace(machineType)
	if mt == "" {
		return nil
	}
	var mtMatches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if strings.EqualFold(pool.MachineType, mt) {
			mtMatches = append(mtMatches, pool)
		}
	}
	distinct, labels := distinctK8sPools(k8s.TopPreferredPools(mtMatches))
	if len(distinct) > 1 {
		return fmt.Errorf("--compute-type is required; %q matches multiple pools, discovered: %s", cmp.Or(q, mt), strings.Join(labels, ", "))
	}
	return nil
}

func (g *GKEOrchestrator) findK8sTPUPoolForMachineType(machineType string) (k8s.PoolCapacity, bool) {
	wantType := defaultTPUHardwareResolver.NormalizeTPUType(machineType)
	wantChips, _ := parseStrictChipSuffix(machineType)
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if !pool.IsTPU || wantType == "" || !strings.EqualFold(pool.TPUType, wantType) {
			continue
		}
		if wantChips > 0 && pool.TPUsPerNode > 0 && pool.TPUsPerNode != wantChips {
			continue
		}
		matches = append(matches, pool)
	}
	return selectBestK8sPool(matches)
}

func stripGCEMachineVariantSuffixes(lower string) string {
	for {
		trimmed := lower
		for _, suffix := range []string{"-nolocalssd", "-nolssd", "-lssd", "-metal", "-ext"} {
			if cut, ok := strings.CutSuffix(trimmed, suffix); ok {
				trimmed = cut
			}
		}
		if trimmed == lower {
			return trimmed
		}
		lower = trimmed
	}
}

func hasKnownGPUFamilyPrefix(lower string) bool {
	for _, prefix := range []string{"a2-", "a3-", "a4-", "a4x-", "a5x-", "g2-", "g4-"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return strings.Contains(lower, "gpu")
}

func inferG2OrG4GPUCount(lower string) (int, bool) {
	if rest, ok := strings.CutPrefix(lower, "g2-standard-"); ok {
		return g2GPUCountFromVCPUs(rest)
	}
	if rest, ok := strings.CutPrefix(lower, "g2-custom-"); ok {
		parts := strings.SplitN(rest, "-", 2)
		return g2GPUCountFromVCPUs(parts[0])
	}
	if rest, ok := strings.CutPrefix(lower, "g4-standard-"); ok {
		vcpus, err := strconv.Atoi(rest)
		if err != nil || vcpus <= 0 {
			return 0, false
		}
		switch vcpus {
		case 384:
			return 8, true
		case 192:
			return 4, true
		case 96:
			return 2, true
		default:
			return 1, true
		}
	}
	return 0, false
}

func g2GPUCountFromVCPUs(vcpuStr string) (int, bool) {
	vcpus, err := strconv.Atoi(vcpuStr)
	if err != nil || vcpus <= 0 {
		return 0, false
	}
	switch vcpus {
	case 96:
		return 8, true
	case 48:
		return 4, true
	case 24:
		return 2, true
	default:
		return 1, true
	}
}

func inferFallbackGPUCount(machineType string) (int, bool) {
	trimmed := strings.TrimSpace(machineType)
	if k8s.IsGPUResourceName(trimmed) {
		return 1, true
	}
	lower := stripGCEMachineVariantSuffixes(strings.ToLower(trimmed))
	if count, ok := inferG2OrG4GPUCount(lower); ok {
		return count, true
	}
	if hasKnownGPUFamilyPrefix(lower) {
		if idx := strings.LastIndex(lower, "-"); idx != -1 && strings.HasSuffix(lower, "g") {
			if n, err := strconv.Atoi(lower[idx+1 : len(lower)-1]); err == nil && n > 0 {
				return n, true
			}
		}
	}
	if strings.Contains(lower, "nvidia") || strings.Contains(lower, "gpu") {
		return 1, true
	}
	return 0, false
}

func inferFallbackGCECPUCount(machineType string) int {
	lower := stripGCEMachineVariantSuffixes(strings.ToLower(strings.TrimSpace(machineType)))
	if rest, ok := strings.CutPrefix(lower, "custom-"); ok {
		return parsePositiveInt(strings.SplitN(rest, "-", 2)[0])
	}
	if !hasGCECPUPrefix(lower) {
		return 0
	}
	parts := strings.Split(lower, "-")
	if len(parts) < 3 {
		return 0
	}
	if parts[1] == "custom" {
		return parsePositiveInt(parts[2])
	}
	return parsePositiveInt(parts[len(parts)-1])
}

func hasGCECPUPrefix(lower string) bool {
	for _, p := range []string{
		"e2-", "n1-", "n2-", "n2d-", "n4-", "c2-", "c2d-", "c3-", "c3d-", "c4-", "c4a-", "c4d-",
		"h3-", "h4d-", "m1-", "m2-", "m3-", "m4-", "t2a-", "t2d-", "z3-",
	} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func parsePositiveInt(s string) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return 0
}

func (g *GKEOrchestrator) isK8sTPURequest(computeType, machineType string) bool {
	if pool, ok := g.findK8sPoolForRequest(computeType, machineType); ok {
		return pool.IsTPU
	}
	if g.mode != orchestrator.TypeK8s {
		return config.IsTPU(machineType) || config.IsTPU(computeType)
	}
	resolver := defaultTPUHardwareResolver
	return resolver.IsTPUMachine(machineType) || resolver.IsTPUMachine(computeType)
}

func (g *GKEOrchestrator) resolveK8sHardwareRequirements(job *orchestrator.JobDefinition) (JobProfile, bool, bool, error) {
	unlabeledCPU, err := g.resolveK8sComputeType(job)
	if err != nil {
		return JobProfile{}, false, false, err
	}
	if unlabeledCPU {
		return g.buildUnlabeledCPUProfile(job)
	}

	job.MachineType = g.resolveK8sMachineType(job.ComputeType, job.NodeConstraint)
	if err := g.checkAmbiguousK8sPoolQuery(job.ComputeType, job.MachineType); err != nil {
		return JobProfile{}, false, false, err
	}
	matchedPool, hasPool := g.findK8sPoolForRequest(job.ComputeType, job.MachineType)
	isTPU := (hasPool && matchedPool.IsTPU) || (!hasPool && (defaultTPUHardwareResolver.IsTPUMachine(job.MachineType) || defaultTPUHardwareResolver.IsTPUMachine(job.ComputeType)))
	if isTPU {
		if job.NumNodesSpecified {
			return JobProfile{}, false, false, fmt.Errorf("--num-nodes cannot be used with TPU jobs (it is calculated automatically from topology)")
		}
		if err := g.resolveK8sTPURequirements(job, matchedPool, hasPool); err != nil {
			return JobProfile{}, false, false, err
		}
	} else if job.Topology != "" {
		return JobProfile{}, false, false, fmt.Errorf("topology is not allowed for GPU and CPU jobs")
	}

	profile, err := g.buildK8sJobProfile(job, matchedPool, hasPool, isTPU)
	if err != nil {
		return JobProfile{}, false, false, err
	}
	return profile, false, false, nil
}

func (g *GKEOrchestrator) buildUnlabeledCPUProfile(job *orchestrator.JobDefinition) (JobProfile, bool, bool, error) {
	if job.Topology != "" {
		return JobProfile{}, false, false, fmt.Errorf("topology is not allowed for GPU and CPU jobs")
	}
	if job.NodesPerSlice <= 0 {
		job.NodesPerSlice = 1
	}
	var cp k8s.PoolCapacity
	if g.inventory != nil && g.inventory.CPUPoolCap != nil {
		cp = *g.inventory.CPUPoolCap
	} else if g.inventory != nil {
		if best, ok := selectBestK8sPool(g.candidateAutoDiscoverPools()); ok {
			cp = best
		}
	}
	return JobProfile{
		IsCPUMachine:        true,
		CapacityCount:       allocatableMilliToWholeCPUs(cp.AllocatableCPUMilli),
		AllocatableCPUMilli: cp.AllocatableCPUMilli,
		K8sPool:             &cp,
	}, false, false, nil
}

func (g *GKEOrchestrator) resolveK8sComputeType(job *orchestrator.JobDefinition) (unlabeledCPU bool, err error) {
	if strings.TrimSpace(job.ComputeType) != "" {
		return false, nil
	}
	if g.inventory != nil && g.inventory.ExplicitMode {
		return false, fmt.Errorf("--compute-type is required because listing cluster nodes was denied (403 Forbidden; explicit mode)")
	}
	ct, source, unlabeled, err := g.discoverK8sDefaultComputeType(job.NodeConstraint)
	if err != nil {
		return false, err
	}
	if unlabeled {
		logging.Info("Auto-discovered unlabeled Kubernetes CPU worker pool.")
		return true, nil
	}
	job.ComputeType = ct
	logging.Info("Auto-discovered compute type %q from %s.", ct, source)
	return false, nil
}

func (g *GKEOrchestrator) findSliceByName(name string) (k8s.SliceInfo, bool) {
	if g.inventory == nil {
		return k8s.SliceInfo{}, false
	}
	for _, s := range g.inventory.Slices {
		if s.Name == name {
			return s, true
		}
	}
	return k8s.SliceInfo{}, false
}

func (g *GKEOrchestrator) findTargetSlices(nodeConstraint map[string]string) ([]k8s.SliceInfo, error) {
	names, err := parseAffinityValues(k8s.TPUSliceNameLabel, strings.TrimSpace(nodeConstraint[k8s.TPUSliceNameLabel]))
	if err != nil || len(names) == 0 {
		return nil, err
	}
	var found []k8s.SliceInfo
	for _, name := range names {
		if s, ok := g.findSliceByName(name); ok {
			found = append(found, s)
		}
	}
	return found, nil
}

func (g *GKEOrchestrator) candidateAutoDiscoverPools() []k8s.PoolCapacity {
	var workers, nonCtrl, all []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		all = append(all, pool)
		if !pool.IsControlPlane {
			nonCtrl = append(nonCtrl, pool)
			if !pool.IsSystemPool {
				workers = append(workers, pool)
			}
		}
	}
	candidates := all
	if len(workers) > 0 {
		candidates = workers
	} else if len(nonCtrl) > 0 {
		candidates = nonCtrl
	}
	return filterUsableAutoDiscoverPools(candidates)
}

func filterUsableAutoDiscoverPools(pools []k8s.PoolCapacity) []k8s.PoolCapacity {
	var usable []k8s.PoolCapacity
	for _, p := range pools {
		if (!p.IsGPU || p.GPUsPerNode > 0) && (!p.IsTPU || p.TPUsPerNode > 0) {
			usable = append(usable, p)
		}
	}
	if len(usable) > 0 {
		return usable
	}
	return pools
}

func k8sPoolComputeTarget(pool k8s.PoolCapacity) (computeType, source string, unlabeledCPU bool) {
	if pool.IsTPU {
		return cmp.Or(strings.TrimSpace(pool.TPUType), strings.TrimSpace(pool.MachineType)), "Kubernetes TPU inventory", false
	}
	if pool.IsGPU {
		return cmp.Or(strings.TrimSpace(pool.MachineType), strings.TrimSpace(pool.GPUType), strings.TrimSpace(pool.GPUResourceName)), "Kubernetes GPU inventory", false
	}
	if mt := strings.TrimSpace(pool.MachineType); mt != "" {
		return mt, "Kubernetes CPU inventory", false
	}
	return "", "Kubernetes CPU inventory", true
}

func (g *GKEOrchestrator) discoverK8sDefaultComputeType(nodeConstraint map[string]string) (computeType, source string, unlabeledCPU bool, err error) {
	if slices, err := g.findTargetSlices(nodeConstraint); err == nil && len(slices) > 0 {
		for _, s := range slices {
			if ct := cmp.Or(strings.TrimSpace(s.MachineType), strings.TrimSpace(s.TPUType)); ct != "" {
				return ct, fmt.Sprintf("TPU slice %q", s.Name), false, nil
			}
		}
	}
	if g.inventory == nil {
		return "", "", false, fmt.Errorf("--compute-type is required: no schedulable worker nodes were discovered in Kubernetes inventory")
	}
	distinct, labels := distinctK8sPools(g.candidateAutoDiscoverPools())
	if len(distinct) > 1 {
		return "", "", false, fmt.Errorf("--compute-type is required; discovered: %s", strings.Join(labels, ", "))
	}
	if len(distinct) == 1 {
		ct, src, unlabeled := k8sPoolComputeTarget(distinct[0])
		if unlabeled || ct != "" {
			return ct, src, unlabeled, nil
		}
	}
	return g.discoverK8sDefaultFromSummary()
}

func (g *GKEOrchestrator) discoverK8sDefaultFromSummary() (computeType, source string, unlabeledCPU bool, err error) {
	tpuCT := ""
	if g.inventory.TPU != nil {
		tpuCT = cmp.Or(strings.TrimSpace(g.inventory.TPU.TPUType), strings.TrimSpace(g.inventory.TPU.MachineType))
	}
	hasCPU := g.inventory.CPUPoolCap != nil
	if tpuCT != "" && hasCPU {
		cpuLabel := cmp.Or(strings.TrimSpace(g.inventory.CPUPoolCap.MachineType), "cpu (unlabeled)")
		discovered := []string{tpuCT, cpuLabel}
		slices.Sort(discovered)
		return "", "", false, fmt.Errorf("--compute-type is required; discovered: %s", strings.Join(discovered, ", "))
	}
	if tpuCT != "" {
		return tpuCT, "Kubernetes TPU inventory", false, nil
	}
	if hasCPU {
		if mt := strings.TrimSpace(g.inventory.CPUPoolCap.MachineType); mt != "" {
			return mt, "Kubernetes CPU inventory", false, nil
		}
		return "", "Kubernetes CPU inventory", true, nil
	}
	return "", "", false, fmt.Errorf("--compute-type is required: no schedulable worker nodes were discovered in Kubernetes inventory")
}

func (g *GKEOrchestrator) preferK8sPoolMachineType(computeType, candidate string) string {
	if pool, found := g.findK8sPoolForRequest(computeType, candidate); found && pool.MachineType != "" {
		return pool.MachineType
	}
	return candidate
}

func (g *GKEOrchestrator) resolveK8sTPUShorthandFromPools(computeType, defaultCandidate string) string {
	if pool, found := g.findK8sPoolForRequest("", defaultCandidate); found && pool.MachineType != "" {
		return pool.MachineType
	}
	if g.inventory == nil {
		return defaultCandidate
	}
	wantType := defaultTPUHardwareResolver.NormalizeTPUType(defaultCandidate)
	normalizedShorthand := normalizeTPUShorthandForTopology(computeType)
	var matches []k8s.PoolCapacity
	for _, name := range g.inventory.PoolNames {
		pool := g.inventory.Pools[name]
		if !pool.IsTPU || pool.MachineType == "" || (wantType != "" && !strings.EqualFold(pool.TPUType, wantType)) {
			continue
		}
		if isCandidateCompatibleForComputeType(normalizedShorthand, pool.MachineType) {
			matches = append(matches, pool)
		}
	}
	if best, ok := selectBestK8sPool(matches); ok {
		return best.MachineType
	}
	if candidate := g.resolveTPUShorthandFromInventoryTPU(wantType, normalizedShorthand); candidate != "" {
		return candidate
	}
	return defaultCandidate
}

func (g *GKEOrchestrator) resolveTPUShorthandFromInventoryTPU(wantType, normalizedShorthand string) string {
	if g.inventory == nil || len(g.inventory.PoolNames) > 0 || g.inventory.TPU == nil {
		return ""
	}
	invTPU := g.inventory.TPU
	if wantType != "" && !strings.EqualFold(invTPU.TPUType, wantType) {
		return ""
	}
	candidate := cmp.Or(strings.TrimSpace(invTPU.MachineType), defaultTPUHardwareResolver.InferTPUMachine(invTPU.TPUType, "", invTPU.ChipsPerNode))
	if candidate != "" && isCandidateCompatibleForComputeType(normalizedShorthand, candidate) {
		return candidate
	}
	return ""
}

func sliceCandidateMachineType(s k8s.SliceInfo, wantType string) string {
	if s.TPUType != "" && wantType != "" && !strings.EqualFold(wantType, s.TPUType) {
		return ""
	}
	if s.MachineType != "" {
		return s.MachineType
	}
	if s.ChipsPerNode > 0 {
		return defaultTPUHardwareResolver.InferTPUMachine(cmp.Or(s.TPUType, wantType), "", s.ChipsPerNode)
	}
	return ""
}

func (g *GKEOrchestrator) resolveMachineFromNamedSlices(lower, wantType string, nodeConstraint map[string]string, isChipShorthand bool) string {
	slices, err := g.findTargetSlices(nodeConstraint)
	if err != nil || len(slices) == 0 {
		return ""
	}
	normalized := normalizeTPUShorthandForTopology(lower)
	for _, s := range slices {
		cand := sliceCandidateMachineType(s, wantType)
		if cand == "" {
			continue
		}
		if !isChipShorthand || isCandidateCompatibleForComputeType(normalized, cand) {
			return cand
		}
	}
	return ""
}

func (g *GKEOrchestrator) resolveK8sMachineType(computeType string, nodeConstraint map[string]string) string {
	trimmed := strings.TrimSpace(computeType)
	lower := strings.ToLower(trimmed)
	if k8s.IsGPUResourceName(trimmed) {
		return g.preferK8sPoolMachineType(trimmed, trimmed)
	}
	if mapped, ok := config.AcceleratorShorthandMap[lower]; ok {
		if defaultTPUHardwareResolver.IsTPUMachine(mapped) {
			if fromSlice := g.resolveShorthandFromNamedSlices(lower, nodeConstraint); fromSlice != "" {
				return fromSlice
			}
			return g.resolveK8sTPUShorthandFromPools(lower, mapped)
		}
		return g.preferK8sPoolMachineType(trimmed, mapped)
	}
	if resolved := g.resolveK8sFamilyShorthand(trimmed, lower, nodeConstraint); resolved != "" {
		return resolved
	}
	return g.preferK8sPoolMachineType(trimmed, trimmed)
}

func (g *GKEOrchestrator) resolveShorthandFromNamedSlices(lower string, nodeConstraint map[string]string) string {
	fam, _, isChip, isFam := parseGCETPUChipShorthand(lower)
	if !isFam {
		return ""
	}
	return g.resolveMachineFromNamedSlices(lower, fam.canonical, nodeConstraint, isChip)
}

func (g *GKEOrchestrator) resolveK8sFamilyShorthand(computeType, lower string, nodeConstraint map[string]string) string {
	fam, _, isChip, isFam := parseGCETPUChipShorthand(lower)
	if !isFam {
		return ""
	}
	if fromSlice := g.resolveMachineFromNamedSlices(lower, fam.canonical, nodeConstraint, isChip); fromSlice != "" {
		return fromSlice
	}
	chipsHint := 0
	if !isChip && g.inventory != nil && g.inventory.TPU != nil {
		if g.inventory.TPU.TPUType == "" || strings.EqualFold(g.inventory.TPU.TPUType, fam.canonical) {
			chipsHint = g.inventory.TPU.ChipsPerNode
		}
	}
	if inferred := defaultTPUHardwareResolver.InferTPUMachine("", computeType, chipsHint); defaultTPUHardwareResolver.IsTPUMachine(inferred) {
		return g.resolveK8sTPUShorthandFromPools(lower, inferred)
	}
	return ""
}

func (g *GKEOrchestrator) resolveK8sTPURequirements(job *orchestrator.JobDefinition, matchedPool k8s.PoolCapacity, hasPool bool) error {
	if done, err := g.resolveAndValidateTPUSlices(job); err != nil || done {
		return err
	}
	if job.Topology == "" {
		topo, err := g.resolveK8sTPUTopology(job)
		if err != nil {
			return err
		}
		job.Topology = topo
	}
	if err := validateK8sTPUTopology(job.MachineType, job.Topology); err != nil {
		return err
	}
	chipsPerNode := g.resolveK8sTPUChipsPerNode(job.MachineType, matchedPool, hasPool, g.resolveTargetSliceChips(job.NodeConstraint))
	return calculateK8sTPUNodesPerSlice(job, chipsPerNode)
}

func (g *GKEOrchestrator) resolveTargetSliceChips(nodeConstraint map[string]string) int {
	slices, err := g.findTargetSlices(nodeConstraint)
	if err != nil {
		return 0
	}
	for _, s := range slices {
		if s.ChipsPerNode > 0 {
			return s.ChipsPerNode
		}
	}
	return 0
}

func normalizeConfigTPUMachine(machineType string) string {
	mt := strings.ToLower(strings.TrimSpace(machineType))
	if strings.HasPrefix(mt, "ct5l-") {
		return "ct5lp-" + strings.TrimPrefix(mt, "ct5l-")
	}
	return mt
}

func validateK8sTPUTopology(machineType, topology string) error {
	if !config.TopologyRegex.MatchString(topology) {
		return fmt.Errorf("provided topology has invalid format %q", topology)
	}
	mt := normalizeConfigTPUMachine(machineType)
	if topology == "1x1x1" && strings.HasPrefix(mt, "tpu7x") && defaultTPUHardwareResolver.ChipsPerNode(mt) == 1 {
		return nil
	}
	if defaultTPUHardwareResolver.IsTPUMachine(mt) {
		if err := config.ValidateHardwareRequest(mt, topology); err != nil {
			// Newer GCE TPU families not yet in config's static 2D/3D shape tables still pass TopologyRegex.
			if !strings.Contains(err.Error(), "topology family is unknown") {
				return err
			}
		}
	}
	return nil
}

func (g *GKEOrchestrator) resolveK8sTPUChipsPerNode(machineType string, matchedPool k8s.PoolCapacity, hasPool bool, sliceChips int) int {
	if hasPool && matchedPool.TPUsPerNode > 0 {
		return matchedPool.TPUsPerNode
	}
	if sliceChips > 0 {
		return sliceChips
	}
	if chips, hasExplicit := parseStrictChipSuffix(machineType); hasExplicit {
		return chips
	}
	if g.inventory != nil && g.inventory.TPU != nil && g.inventory.TPU.ChipsPerNode > 0 {
		wantType := defaultTPUHardwareResolver.NormalizeTPUType(machineType)
		if g.inventory.TPU.TPUType == "" || wantType == "" || strings.EqualFold(g.inventory.TPU.TPUType, wantType) {
			return g.inventory.TPU.ChipsPerNode
		}
	}
	return resolveFallbackTPUChips(machineType)
}

func calculateK8sTPUNodesPerSlice(job *orchestrator.JobDefinition, chipsPerNode int) error {
	if job.Topology == "" {
		return fmt.Errorf("could not resolve TPU topology for the provided machine type: %q", job.MachineType)
	}
	nodes, err := config.CalculateAcceleratorNodes(job.MachineType, job.Topology, chipsPerNode)
	if err != nil {
		return fmt.Errorf("failed to calculate nodes from topology: %w", err)
	}
	if nodes <= 0 {
		return fmt.Errorf("invalid nodes_per_slice (%d) for topology %s", nodes, job.Topology)
	}
	job.NodesPerSlice = nodes
	logging.Info("Dynamically determined nodes_per_slice for %s: %d", job.Topology, job.NodesPerSlice)
	return nil
}

func (g *GKEOrchestrator) validateSliceHardwareMatch(job *orchestrator.JobDefinition, sliceName string, s k8s.SliceInfo) error {
	if err := g.validateSliceTypeAndChipsMatch(job, sliceName, s); err != nil {
		return err
	}
	if job.Topology != "" && s.Topology != "" && !strings.EqualFold(job.Topology, s.Topology) {
		return fmt.Errorf("requested topology %q does not match slice %q topology %q", job.Topology, sliceName, s.Topology)
	}
	if job.Topology != "" && s.Topology == "" && s.NodesPerSlice > 0 {
		chips := cmp.Or(s.ChipsPerNode, defaultTPUHardwareResolver.ChipsPerNode(job.MachineType))
		if chips > 0 {
			if nodes, err := config.CalculateAcceleratorNodes(job.MachineType, job.Topology, chips); err == nil && nodes != s.NodesPerSlice {
				return fmt.Errorf("requested topology %q (%d nodes) does not match slice %q (%d nodes)", job.Topology, nodes, sliceName, s.NodesPerSlice)
			}
		}
	}
	return nil
}

func (g *GKEOrchestrator) validateSliceTypeAndChipsMatch(job *orchestrator.JobDefinition, sliceName string, s k8s.SliceInfo) error {
	resolver := defaultTPUHardwareResolver
	sameMachine := s.MachineType != "" && strings.EqualFold(strings.TrimSpace(job.MachineType), strings.TrimSpace(s.MachineType))
	if !sameMachine && s.TPUType != "" {
		jobTPUType := ""
		if pool, ok := g.findK8sPoolForRequest(job.ComputeType, job.MachineType); ok && pool.IsTPU && pool.TPUType != "" {
			jobTPUType = pool.TPUType
		} else {
			jobTPUType = resolver.NormalizeTPUType(cmp.Or(strings.TrimSpace(job.MachineType), strings.TrimSpace(job.ComputeType)))
		}
		if jobTPUType != "" && !strings.EqualFold(jobTPUType, s.TPUType) {
			return fmt.Errorf("requested TPU type %q does not match slice %q type %q", jobTPUType, sliceName, s.TPUType)
		}
	}
	if wantChips := resolver.ChipsPerNode(job.MachineType); wantChips > 0 && s.ChipsPerNode > 0 && wantChips != s.ChipsPerNode {
		return fmt.Errorf("requested TPU host shape %q (%d chips/node) does not match slice %q (%d chips/node)", job.MachineType, wantChips, sliceName, s.ChipsPerNode)
	}
	return nil
}

func deriveSliceTopologyFromNodes(job *orchestrator.JobDefinition, s k8s.SliceInfo) string {
	if s.NodesPerSlice <= 0 {
		return ""
	}
	resolver := defaultTPUHardwareResolver
	chips := cmp.Or(s.ChipsPerNode, resolver.ChipsPerNode(job.MachineType))
	if chips <= 0 {
		return ""
	}
	familyTarget := normalizeConfigTPUMachine(cmp.Or(strings.TrimSpace(s.TPUType), resolver.NormalizeTPUType(cmp.Or(job.MachineType, job.ComputeType))))
	if resolver.IsTPUMachine(job.MachineType) {
		familyTarget = normalizeConfigTPUMachine(job.MachineType)
	}
	topo, ok := config.DefaultTopologyForChips(familyTarget, s.NodesPerSlice*chips)
	if !ok {
		return ""
	}
	return topo
}

// resolveAndValidateTPUSlices validates target TPU slice constraints and populates topology/nodes from
// matching slices. On SchemeTPUSlice, it enforces explicit READY slice targeting (for both JobSet and
// standalone batch/v1 Indexed Job) before topology resolution so missing or unready slices report the
// slice requirement instead of "--topology is required".
func (g *GKEOrchestrator) resolveAndValidateTPUSlices(job *orchestrator.JobDefinition) (bool, error) {
	isSliceScheme := g.labelScheme() == k8s.SchemeTPUSlice
	hasSlices := g.inventory != nil && len(g.inventory.Slices) > 0
	if !isSliceScheme && !hasSlices {
		return false, nil
	}
	rawSlice := strings.TrimSpace(job.NodeConstraint[k8s.TPUSliceNameLabel])
	names, err := parseAffinityValues(k8s.TPUSliceNameLabel, rawSlice)
	if err != nil {
		return false, err
	}
	if err := g.validateTPUSliceTargetCount(job, rawSlice, names, isSliceScheme); err != nil {
		return false, err
	}
	if len(names) == 0 {
		return false, nil
	}
	resolved, err := g.resolveNamedTPUSlices(names, isSliceScheme)
	if err != nil || len(resolved) == 0 {
		return false, err
	}
	return g.applyResolvedTargetSlices(job, resolved, isSliceScheme && !g.jobSetEnabled())
}

func (g *GKEOrchestrator) validateTPUSliceTargetCount(job *orchestrator.JobDefinition, rawSlice string, names []string, isSliceScheme bool) error {
	tier3 := !g.jobSetEnabled()
	if len(names) == 0 {
		if !isSliceScheme {
			return nil
		}
		if tier3 {
			return fmt.Errorf("standalone batch/v1 Indexed Job without JobSet cannot dynamically provision a new TPU slice; target an existing READY slice via --node-constraint %s=<ready-slice-name>", k8s.TPUSliceNameLabel)
		}
		return fmt.Errorf("TPU workloads on tpu-slice clusters currently require targeting an existing READY slice via --node-constraint %s=<ready-slice-name>", k8s.TPUSliceNameLabel)
	}
	if tier3 && len(names) > 1 {
		return fmt.Errorf("standalone batch/v1 Indexed Job without JobSet cannot target multiple TPU slices (%q); specify a single existing READY slice via --node-constraint %s=<ready-slice-name>", rawSlice, k8s.TPUSliceNameLabel)
	}
	if isSliceScheme && !tier3 {
		if need := max(1, job.NumSlices); len(names) < need {
			return fmt.Errorf("--num-slices=%d requires at least %d distinct READY slices in --node-constraint %s (got %d: %s)",
				need, need, k8s.TPUSliceNameLabel, len(names), strings.Join(names, ", "))
		}
	}
	return nil
}

func (g *GKEOrchestrator) resolveNamedTPUSlices(names []string, isSliceScheme bool) ([]k8s.SliceInfo, error) {
	tier3 := !g.jobSetEnabled()
	var resolved []k8s.SliceInfo
	for _, name := range names {
		s, found := g.findSliceByName(name)
		if !found {
			if isSliceScheme && g.inventory != nil && !(g.inventory.ExplicitMode && g.inventory.SliceInventoryUnavailable) {
				return nil, missingTPUSliceError(name, tier3, g.inventory.SliceInventoryUnavailable)
			}
			if isSliceScheme && tier3 {
				logging.Info("JobSet is unavailable; targeting explicitly named TPU slice %q via standalone batch/v1 Indexed Job.", name)
			}
			continue
		}
		if isSliceScheme && !s.Ready {
			return nil, notReadyTPUSliceError(name, s.State, tier3)
		}
		resolved = append(resolved, s)
	}
	return resolved, nil
}

func missingTPUSliceError(sliceName string, tier3, sliceAPIDenied bool) error {
	note := ""
	if sliceAPIDenied {
		note = " (Slice objects could not be listed; only node-derived slices were checked)"
	}
	if tier3 {
		return fmt.Errorf("standalone batch/v1 Indexed Job without JobSet cannot dynamically provision TPU slice %q (not found in cluster inventory)%s; target an existing READY slice via --node-constraint %s=<ready-slice-name>",
			sliceName, note, k8s.TPUSliceNameLabel)
	}
	return fmt.Errorf("TPU slice %q does not exist%s; target an existing READY slice via --node-constraint %s=<ready-slice-name>", sliceName, note, k8s.TPUSliceNameLabel)
}

func notReadyTPUSliceError(sliceName, state string, tier3 bool) error {
	s := cmp.Or(state, "unknown")
	if tier3 {
		return fmt.Errorf("standalone batch/v1 Indexed Job without JobSet requires a READY TPU slice, but slice %q is not READY (state=%q); wait for it to become READY or target a READY slice via --node-constraint %s=<ready-slice-name>",
			sliceName, s, k8s.TPUSliceNameLabel)
	}
	return fmt.Errorf("TPU slice %q is not READY (state=%q); wait for it to become READY or target a READY slice via --node-constraint %s=<ready-slice-name>",
		sliceName, s, k8s.TPUSliceNameLabel)
}

func (g *GKEOrchestrator) applyResolvedTargetSlices(job *orchestrator.JobDefinition, slices []k8s.SliceInfo, logTier3Reuse bool) (bool, error) {
	first := slices[0]
	if err := g.populateSliceTopology(job, first.Name, first); err != nil {
		return false, err
	}
	for _, s := range slices[1:] {
		if err := g.validateSliceHardwareMatch(job, s.Name, s); err != nil {
			return false, err
		}
		if first.NodesPerSlice > 0 && s.NodesPerSlice > 0 && s.NodesPerSlice != first.NodesPerSlice {
			return false, fmt.Errorf("target TPU slice %q (%d nodes) does not match slice %q (%d nodes)", s.Name, s.NodesPerSlice, first.Name, first.NodesPerSlice)
		}
	}
	if logTier3Reuse {
		logging.Info("JobSet is unavailable; reusing existing READY TPU slice %q (type=%s, topology=%s) via standalone batch/v1 Indexed Job.", first.Name, first.TPUType, first.Topology)
	}
	if first.NodesPerSlice > 0 && job.Topology != "" {
		job.NodesPerSlice = first.NodesPerSlice
		return true, nil
	}
	return false, nil
}

func (g *GKEOrchestrator) populateSliceTopology(job *orchestrator.JobDefinition, sliceName string, s k8s.SliceInfo) error {
	if job.Topology == "" {
		if shorthandTopo, err := resolveShorthandTopologyIfPresent(job.ComputeType, job.MachineType); err != nil {
			return err
		} else if shorthandTopo != "" {
			job.Topology = shorthandTopo
		}
	}
	if err := g.validateSliceHardwareMatch(job, sliceName, s); err != nil {
		return err
	}
	if job.Topology == "" {
		if s.Topology != "" {
			job.Topology = s.Topology
			logging.Info("Auto-discovered topology %q from TPU slice %q.", job.Topology, sliceName)
		} else if derived := deriveSliceTopologyFromNodes(job, s); derived != "" {
			job.Topology = derived
			logging.Info("Derived topology %q from TPU slice %q (%d nodes).", job.Topology, sliceName, s.NodesPerSlice)
		}
	}
	if job.Topology != "" {
		return validateK8sTPUTopology(job.MachineType, job.Topology)
	}
	return nil
}

func resolveShorthandTopologyIfPresent(computeType, machineType string) (string, error) {
	fam, chips, isChip, _ := parseGCETPUChipShorthand(computeType)
	if !isChip {
		return "", nil
	}
	return config.ResolveTopologyForChips(fam.canonical+"-"+chips, normalizeConfigTPUMachine(machineType))
}

func normalizeTPUShorthandForTopology(computeType string) string {
	if fam, chips, isChip, _ := parseGCETPUChipShorthand(computeType); isChip {
		return fam.canonical + "-" + chips
	}
	return strings.ToLower(strings.TrimSpace(computeType))
}

func (g *GKEOrchestrator) resolveK8sTPUTopology(job *orchestrator.JobDefinition) (string, error) {
	topo, err := resolveShorthandTopologyIfPresent(job.ComputeType, job.MachineType)
	if err != nil {
		return "", err
	}
	if topo != "" {
		logging.Info("Auto-resolved topology %s for shorthand %s", topo, job.ComputeType)
		return topo, nil
	}
	if g.inventory != nil && g.inventory.ExplicitMode {
		return "", fmt.Errorf("--topology is required for TPU workloads because listing cluster nodes was denied (403 Forbidden; explicit mode)")
	}
	if g.labelScheme() != k8s.SchemeTPUSlice && g.inventory != nil && g.inventory.TPU != nil && g.inventory.TPU.Topology != "" {
		logging.Info("Auto-discovered topology %q from Kubernetes inventory.", g.inventory.TPU.Topology)
		return g.inventory.TPU.Topology, nil
	}
	return "", fmt.Errorf("--topology is required for TPU machine type %s", job.MachineType)
}

func (g *GKEOrchestrator) buildK8sJobProfile(job *orchestrator.JobDefinition, matchedPool k8s.PoolCapacity, hasPool, isTPU bool) (JobProfile, error) {
	var poolPtr *k8s.PoolCapacity
	if hasPool {
		poolCopy := matchedPool
		poolPtr = &poolCopy
	}
	if isTPU {
		chips := g.resolveK8sTPUChipsPerNode(job.MachineType, matchedPool, hasPool, g.resolveTargetSliceChips(job.NodeConstraint))
		return JobProfile{
			IsCPUMachine:  false,
			IsTPU:         true,
			CapacityCount: chips,
			K8sPool:       poolPtr,
		}, nil
	}
	if job.NodesPerSlice <= 0 {
		job.NodesPerSlice = 1
	}
	if hasPool {
		if matchedPool.IsGPU {
			gpus, err := validateK8sPoolGPUs(matchedPool)
			if err != nil {
				return JobProfile{}, err
			}
			return JobProfile{
				IsCPUMachine:    false,
				CapacityCount:   gpus,
				GPUResourceName: matchedPool.GPUResourceName,
				K8sPool:         poolPtr,
			}, nil
		}
		return JobProfile{
			IsCPUMachine:        true,
			CapacityCount:       allocatableMilliToWholeCPUs(matchedPool.AllocatableCPUMilli),
			AllocatableCPUMilli: matchedPool.AllocatableCPUMilli,
			K8sPool:             poolPtr,
		}, nil
	}
	gpuKey := ""
	if k8s.IsGPUResourceName(job.MachineType) {
		gpuKey = strings.TrimSpace(job.MachineType)
	} else if k8s.IsGPUResourceName(job.ComputeType) {
		gpuKey = strings.TrimSpace(job.ComputeType)
	}
	if count, isGPU := inferFallbackGPUCount(job.MachineType); isGPU {
		return JobProfile{
			IsCPUMachine:    false,
			CapacityCount:   count,
			GPUResourceName: gpuKey,
		}, nil
	}
	vCPUs := inferFallbackGCECPUCount(job.MachineType)
	if vCPUs == 0 && strings.TrimSpace(job.MachineType) != "" {
		logging.Warn("Unable to infer vCPU count for machine type %q without cluster node inventory; pod will omit CPU requests/limits (BestEffort QoS), which ResourceQuota or Kueue may reject.", job.MachineType)
	}
	return JobProfile{
		IsCPUMachine:  true,
		CapacityCount: vCPUs,
	}, nil
}
