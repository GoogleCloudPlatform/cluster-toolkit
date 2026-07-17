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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"hpc-toolkit/pkg/intent/ast"
)

// =========================================================================
// CATALOG LOADER (The YAML Database)
// =========================================================================
// CatalogLoader lazy-loads catalog manifests (config-base, compute-archetypes, features)
// into memory to act as a blueprint database for the compiler.
type CatalogLoader struct {
	basePath string

	configBaseCache map[string]*ast.ConfigBaseManifest
	archetypeCache  map[string]*ast.ComputeArchetypeManifest
	featureCache    map[string]*ast.FeatureManifest
	overlayCache    map[string]*ast.OverlayManifest

	// Discovery-only state (see catalog.go). Not consulted by the compiler.
	files        map[CatalogKind]map[string]string // kind -> manifest name -> absolute file path
	exampleCache map[string]*ast.ClusterConfig     // v2/cluster-configs, keyed by file stem
}

// recordFile remembers which file a manifest was loaded from, for `gcluster catalog`.
func (l *CatalogLoader) recordFile(kind CatalogKind, name, path string) {
	if l.files == nil {
		l.files = make(map[CatalogKind]map[string]string)
	}
	if l.files[kind] == nil {
		l.files[kind] = make(map[string]string)
	}
	l.files[kind][name] = path
}

// NewCatalogLoader creates a new loader reading from the specified toolkit root path.
func NewCatalogLoader(basePath string) *CatalogLoader {
	return &CatalogLoader{
		basePath: basePath,
	}
}

// loadConfigBaseCache executes the actual directory scan. High-performance compilers
// cannot afford to open file handles for every AST evaluation. Here, the engine
// iterates through the file system ONCE, unmarshals every valid YAML into the memory
// structs (ast.ConfigBaseManifest), and caches it O(1) by its underlying 'Name'.
func (l *CatalogLoader) loadConfigBaseCache() error {
	if l.configBaseCache != nil {
		return nil
	}
	l.configBaseCache = make(map[string]*ast.ConfigBaseManifest)

	files, err := filepath.Glob(filepath.Join(l.basePath, "v2", "config-base", "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to scan v2/config-base directory: %w", err)
	}
	for _, f := range files {
		var manifest ast.ConfigBaseManifest
		if err := l.loadCatalogYAML(f, &manifest); err != nil {
			return fmt.Errorf("syntax error in config-base file %q: %w", filepath.Base(f), err)
		}
		if manifest.Name != "" {
			l.configBaseCache[manifest.Name] = &manifest
			l.recordFile(KindBase, manifest.Name, f)
		}
	}
	return nil
}

// loadArchetypeCache lazy-loads all compute archetypes into memory.
func (l *CatalogLoader) loadArchetypeCache() error {
	if l.archetypeCache != nil {
		return nil
	}
	l.archetypeCache = make(map[string]*ast.ComputeArchetypeManifest)

	files, err := filepath.Glob(filepath.Join(l.basePath, "v2", "compute-archetypes", "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to scan v2/compute-archetypes directory: %w", err)
	}
	for _, f := range files {
		var manifest ast.ComputeArchetypeManifest
		if err := l.loadCatalogYAML(f, &manifest); err != nil {
			return fmt.Errorf("syntax error in compute-archetype file %q: %w", filepath.Base(f), err)
		}
		if manifest.Name != "" {
			l.archetypeCache[manifest.Name] = &manifest
			l.recordFile(KindArchetype, manifest.Name, f)
		}
	}
	return nil
}

// loadFeatureCache lazy-loads all features into memory.
func (l *CatalogLoader) loadFeatureCache() error {
	if l.featureCache != nil {
		return nil
	}
	l.featureCache = make(map[string]*ast.FeatureManifest)

	files, err := filepath.Glob(filepath.Join(l.basePath, "v2", "features", "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to scan v2/features directory: %w", err)
	}
	for _, f := range files {
		var manifest ast.FeatureManifest
		if err := l.loadCatalogYAML(f, &manifest); err != nil {
			return fmt.Errorf("syntax error in features file %q: %w", filepath.Base(f), err)
		}
		if manifest.Name != "" {
			l.featureCache[manifest.Name] = &manifest
			l.recordFile(KindFeature, manifest.Name, f)
		}
	}
	return nil
}

// loadOverlayCache lazy-loads all Tier 5 solution payloads into memory.
func (l *CatalogLoader) loadOverlayCache() error {
	if l.overlayCache != nil {
		return nil
	}
	l.overlayCache = make(map[string]*ast.OverlayManifest)

	files, err := filepath.Glob(filepath.Join(l.basePath, "v2", "overlays", "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to scan v2/overlays directory: %w", err)
	}
	for _, f := range files {
		var manifest ast.OverlayManifest
		if err := l.loadCatalogYAML(f, &manifest); err != nil {
			return fmt.Errorf("syntax error in overlays file %q: %w", filepath.Base(f), err)
		}
		if manifest.Name == "" {
			return fmt.Errorf("overlay file %q is missing required field 'name'", filepath.Base(f))
		}
		if len(manifest.ConfigBase.Settings) == 0 && len(manifest.Vars) == 0 && len(manifest.ComputeArchetypes) == 0 && len(manifest.Features) == 0 {
			return fmt.Errorf("overlay %q must define at least one of 'vars', 'compute_archetypes', or 'features'", manifest.Name)
		}
		l.overlayCache[manifest.Name] = &manifest
		l.recordFile(KindOverlay, manifest.Name, f)
	}
	return nil
}

// =========================================================================
// PUBLIC RETRIEVAL API
// =========================================================================

// LoadConfigBase retrieves a Tier 2 orchestrator skeleton (e.g., 'gke', 'slurm').
// This represents the entire deployment group foundation the rest of the AST is built upon.
func (l *CatalogLoader) LoadConfigBase(name string) (*ast.ConfigBaseManifest, error) {
	if err := l.loadConfigBaseCache(); err != nil {
		return nil, err
	}
	if manifest, ok := l.configBaseCache[name]; ok {
		return manifest, nil
	}
	var available []string
	for k := range l.configBaseCache {
		available = append(available, k)
	}
	sort.Strings(available)
	if len(available) > 0 {
		return nil, fmt.Errorf("config base %q not found in any manifest (available: %s)", name, strings.Join(available, ", "))
	}
	return nil, fmt.Errorf("config base %q not found in any manifest", name)
}

// LoadArchetype reads a Tier 3 hardware specification with family prefix / alias matching.
func (l *CatalogLoader) LoadArchetype(name string) (*ast.ComputeArchetypeManifest, error) {
	return l.LoadArchetypeForBase(name, "")
}

// LoadArchetypeForBase reads a Tier 3 hardware specification and scopes error suggestions to configBase when provided.
func (l *CatalogLoader) LoadArchetypeForBase(name, configBase string) (*ast.ComputeArchetypeManifest, error) {
	if err := l.loadArchetypeCache(); err != nil {
		return nil, err
	}
	// 1. Exact match in cache
	if manifest, ok := l.archetypeCache[name]; ok {
		return manifest, nil
	}

	// 2. Check explicit match_patterns across all loaded archetypes
	if manifest := l.matchExplicitArchetypePattern(name); manifest != nil {
		return manifest, nil
	}

	// 3. Catch-all fallback to the generic CPU archetype.
	//
	// This is deliberate: any valid Google Cloud CPU machine type should work without
	// us shipping an archetype for it. PHASE 2 of the compiler validates the machine
	// type against the live GCP API whenever resolution lands here, so a typo surfaces
	// as a 404 rather than a silent miswiring.
	//
	// It must NOT swallow accelerator families. Those need dedicated driver, network and
	// topology wiring; resolving them here would produce a CPU-only pool that compiles,
	// expands and validates cleanly while being completely wrong. The fallback archetype
	// declares the families it refuses via `excluded_prefixes`.
	for _, fallbackManifest := range l.archetypeCache {
		if !isFallbackArchetype(fallbackManifest) {
			continue
		}
		return l.resolveFallbackArchetype(name, configBase, fallbackManifest)
	}

	var available []string
	for k := range l.archetypeCache {
		available = append(available, k)
	}
	sort.Strings(available)
	if len(available) > 0 {
		return nil, fmt.Errorf("compute archetype %q not found in any manifest (available: %s)", name, strings.Join(available, ", "))
	}
	return nil, fmt.Errorf("compute archetype %q not found in any manifest", name)
}

func (l *CatalogLoader) matchExplicitArchetypePattern(name string) *ast.ComputeArchetypeManifest {
	for _, manifest := range l.archetypeCache {
		for _, pattern := range manifest.MatchPatterns {
			if pattern == "*" {
				continue
			}
			if pattern == name {
				return manifest
			}
			if strings.HasSuffix(pattern, "*") && strings.HasPrefix(name, strings.TrimSuffix(pattern, "*")) {
				return manifest
			}
		}
	}
	return nil
}

func (l *CatalogLoader) resolveFallbackArchetype(name, configBase string, fallbackManifest *ast.ComputeArchetypeManifest) (*ast.ComputeArchetypeManifest, error) {
	for _, prefix := range fallbackManifest.ExcludedPrefixes {
		if strings.HasPrefix(name, prefix) {
			available := l.ListArchetypesForBase(configBase)
			if configBase != "" && len(available) > 0 {
				return nil, fmt.Errorf(
					"machine type %q belongs to an accelerator family with no compute archetype for config_base %q. "+
						"Available accelerator archetypes for %s: %s",
					name, configBase, configBase, strings.Join(available, ", "))
			}
			return nil, fmt.Errorf(
				"machine type %q belongs to an accelerator family with no compute archetype. "+
					"Available accelerator archetypes: %s",
				name, strings.Join(l.ListArchetypes(), ", "))
		}
	}
	return fallbackManifest, nil
}

// LoadFeature reads a Tier 4 feature component.
func (l *CatalogLoader) LoadFeature(name string) (*ast.FeatureManifest, error) {
	if err := l.loadFeatureCache(); err != nil {
		return nil, err
	}
	if manifest, ok := l.featureCache[name]; ok {
		return manifest, nil
	}
	var available []string
	for k := range l.featureCache {
		available = append(available, k)
	}
	sort.Strings(available)
	if len(available) > 0 {
		return nil, fmt.Errorf("feature %q not found in any manifest (available: %s)", name, strings.Join(available, ", "))
	}
	return nil, fmt.Errorf("feature %q not found in any manifest", name)
}

// LoadOverlay reads a Tier 5 solution payload component.
func (l *CatalogLoader) LoadOverlay(name string) (*ast.OverlayManifest, error) {
	if err := l.loadOverlayCache(); err != nil {
		return nil, err
	}
	if manifest, ok := l.overlayCache[name]; ok {
		return manifest, nil
	}
	var available []string
	for k := range l.overlayCache {
		available = append(available, k)
	}
	sort.Strings(available)
	if len(available) > 0 {
		return nil, fmt.Errorf("overlay %q not found in any manifest (available: %s)", name, strings.Join(available, ", "))
	}
	return nil, fmt.Errorf("overlay %q not found in any manifest", name)
}

// LoadUserFeature retrieves a Tier 4 Feature that a user named directly in a cluster-config
// `features:` list.
func (l *CatalogLoader) LoadUserFeature(name string) (*ast.FeatureManifest, error) {
	manifest, err := l.LoadFeature(name)
	if err != nil {
		return nil, err
	}
	if len(manifest.DeploymentGroups) == 0 && len(manifest.ConfigBase) == 0 {
		return nil, fmt.Errorf(
			"feature %q cannot be listed under `features:` -- it has no modules of its own. "+
				"List an overlay under `overlays:`%s instead",
			name, l.overlaysNaming(name))
	}
	return manifest, nil
}

// overlaysNaming returns a paste-ready hint listing the shipped overlays that supply content
// for featureType, e.g. " (container-runtime, gpu-healthcheck)".
//
// It returns the empty string when none exist or the cache cannot be read, so the caller's
// sentence still reads correctly without it. An unreadable overlay cache must not turn a
// clear validation error into an unrelated I/O error.
func (l *CatalogLoader) overlaysNaming(featureType string) string {
	if err := l.loadOverlayCache(); err != nil {
		return ""
	}
	var names []string
	for name, m := range l.overlayCache {
		for _, f := range m.Features {
			ft := f.Type
			if ft == "" {
				ft = f.Name
			}
			if ft == featureType {
				names = append(names, name)
				break
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return " (" + strings.Join(names, ", ") + ")"
}

// ghpcStagePattern matches a ghpc_stage("<path>") call inside catalog YAML text.
var ghpcStagePattern = regexp.MustCompile(`ghpc_stage\(\s*"([^"]*)"\s*\)`)

// loadCatalogYAML reads a Tier 2-5 catalog manifest.
//
// Catalog manifests are read from a fixed location under the toolkit root, but
// `ghpc_stage()` resolves its argument relative to the *user's* cluster-config file
// (pkg/config/staging.go). Those two directories are unrelated, so a catalog-authored
// relative path only worked while the user's config sat in examples/cluster-configs/.
// Copying a config anywhere else produced:
//
//	Error: file for staging /tmp/gke-a3-ultragpu/nccl-installer.yaml.tftpl does not exists
//
// and only at `create` time -- `expand` passes, so CI that stops at expand never sees it.
//
// Catalog paths are therefore written toolkit-root-relative and absolutised here.
// `pkg/config/staging.go` passes absolute paths through untouched, so no change is
// needed there. Paths written by users in their own cluster-config are NOT touched:
// those correctly remain relative to the file that wrote them.
func (l *CatalogLoader) loadCatalogYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(l.absolutiseStagePaths(data)))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// absolutiseStagePaths rewrites relative ghpc_stage() arguments to absolute paths
// anchored at the toolkit root. Already-absolute paths are left alone.
func (l *CatalogLoader) absolutiseStagePaths(data []byte) []byte {
	if l.basePath == "" {
		return data
	}
	root, err := filepath.Abs(l.basePath)
	if err != nil {
		return data
	}
	return ghpcStagePattern.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := ghpcStagePattern.FindSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		p := string(sub[1])
		if p == "" || filepath.IsAbs(p) {
			return match
		}
		return []byte(fmt.Sprintf("ghpc_stage(%q)", filepath.Join(root, p)))
	})
}

// isFallbackArchetype reports whether an archetype manifest is the catch-all CPU fallback.
func isFallbackArchetype(arch *ast.ComputeArchetypeManifest) bool {
	if arch == nil {
		return false
	}
	if len(arch.ExcludedPrefixes) > 0 {
		return true
	}
	for _, a := range arch.MatchPatterns {
		if a == "*" {
			return true
		}
	}
	return false
}

// HasExplicitArchetype checks if a specific machine type maps to an explicit hardware archetype (not generic-cpu fallback).
func (l *CatalogLoader) HasExplicitArchetype(name string) bool {
	arch, err := l.LoadArchetype(name)
	if err != nil {
		return false
	}
	return !isFallbackArchetype(arch)
}

// ListArchetypes returns a sorted list of explicit accelerators for error boundaries.
func (l *CatalogLoader) ListArchetypes() []string {
	_ = l.loadArchetypeCache()
	var available []string
	for k, arch := range l.archetypeCache {
		if !isFallbackArchetype(arch) {
			available = append(available, k)
		}
	}
	sort.Strings(available)
	return available
}

// ListArchetypesForBase returns a sorted list of explicit accelerator archetypes
// that support the given config_base (or all explicit archetypes if configBase is empty).
func (l *CatalogLoader) ListArchetypesForBase(configBase string) []string {
	if configBase == "" {
		return l.ListArchetypes()
	}
	_ = l.loadArchetypeCache()
	var available []string
	for k, arch := range l.archetypeCache {
		if isFallbackArchetype(arch) {
			continue
		}
		if len(arch.ConfigBase) == 0 {
			available = append(available, k)
		} else if _, ok := arch.ConfigBase[configBase]; ok {
			available = append(available, k)
		}
	}
	sort.Strings(available)
	return available
}
