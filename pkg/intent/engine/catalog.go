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

// =========================================================================
// CATALOG DISCOVERY API (read-only; backs `gcluster catalog`)
// =========================================================================
// Everything here is derived from the same manifests the compiler consumes, so the
// discovery output cannot drift from what `gcluster create` actually accepts. Nothing in
// this file is used by the compiler itself.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hpc-toolkit/pkg/intent/ast"
)

// CatalogKind identifies a catalog tier. Values are the user-facing CLI words.
type CatalogKind string

// Catalog kinds, in the order a cluster-config is composed.
const (
	KindBase      CatalogKind = "base"      // v2/config-base        -> config_base
	KindArchetype CatalogKind = "archetype" // v2/compute-archetypes -> compute_archetypes[].machine_type
	KindFeature   CatalogKind = "feature"   // v2/features           -> features[].type
	KindOverlay   CatalogKind = "overlay"   // v2/overlays           -> overlays[].type
	KindExample   CatalogKind = "example"   // v2/cluster-configs    -> a complete cluster-config
)

// AllCatalogKinds lists every kind in display order.
var AllCatalogKinds = []CatalogKind{KindBase, KindArchetype, KindFeature, KindOverlay, KindExample}

// AnyBase is reported for manifests that are not tied to a specific config base.
const AnyBase = "any"

// ErrCatalogNotFound is returned when the v2/ catalog directory is missing or empty.
var ErrCatalogNotFound = errors.New("cluster-config catalog not found")

// CatalogEntry is the display-oriented summary of one catalog item.
type CatalogEntry struct {
	Kind        CatalogKind `json:"kind" yaml:"kind"`
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description" yaml:"description"`
	Bases       []string    `json:"bases" yaml:"bases"`
	Aliases     []string    `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	File        string      `json:"file" yaml:"file"` // relative to the toolkit root

	// Populated only by ListEntriesForBase (the answer depends on the chosen base).
	PrimaryModule string   `json:"primary_module,omitempty" yaml:"primary_module,omitempty"` // archetypes, features
	Features      []string `json:"features,omitempty" yaml:"features,omitempty"`             // overlays: feature types it adds
}

// CatalogSection is the per-base portion of a manifest. Base is "" for root-level content
// that applies regardless of the chosen config base.
type CatalogSection struct {
	Base    string
	Vars    map[string]any
	Modules []ast.ModuleSpec
}

// CatalogDetail is everything `gcluster catalog show` renders for one item.
type CatalogDetail struct {
	CatalogEntry
	Sections          []CatalogSection
	OverlayFeatures   []ast.FeatureReference   // overlays only
	OverlayArchetypes []ast.ArchetypeReference // overlays only
	Example           *ast.ClusterConfig       // examples only
	Raw               []byte                   // examples only: the file, verbatim
	AbsFile           string                   // absolute path of File, for copy/paste hints
}

// CatalogRoot returns the absolute path of the v2/ directory this loader reads.
func (l *CatalogLoader) CatalogRoot() string {
	root, err := filepath.Abs(filepath.Join(l.basePath, "v2"))
	if err != nil {
		return filepath.Join(l.basePath, "v2")
	}
	return root
}

// ensureCatalog fails with ErrCatalogNotFound when v2/ does not exist. filepath.Glob on a
// missing directory silently returns no matches, which would otherwise render as an empty
// (and misleading) catalog.
func (l *CatalogLoader) ensureCatalog() error {
	info, err := os.Stat(l.CatalogRoot())
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w at %s", ErrCatalogNotFound, l.CatalogRoot())
	}
	if m, _ := filepath.Glob(filepath.Join(l.CatalogRoot(), "*", "*.yaml")); len(m) == 0 {
		return fmt.Errorf("%w at %s (directory contains no manifests)", ErrCatalogNotFound, l.CatalogRoot())
	}
	return nil
}

// loadExampleCache lazy-loads the shipped cluster-configs (Tier 1 examples).
func (l *CatalogLoader) loadExampleCache() error {
	if l.exampleCache != nil {
		return nil
	}
	l.exampleCache = make(map[string]*ast.ClusterConfig)
	files, err := filepath.Glob(filepath.Join(l.basePath, "v2", "cluster-configs", "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to scan v2/cluster-configs directory: %w", err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		cfg, err := ast.UnmarshalClusterConfig(data)
		if err != nil {
			return fmt.Errorf("syntax error in cluster-config file %q: %w", filepath.Base(f), err)
		}
		name := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		l.exampleCache[name] = &cfg
		l.recordFile(KindExample, name, f)
	}
	return nil
}

func (l *CatalogLoader) relFile(kind CatalogKind, name string) string {
	p := l.files[kind][name]
	if p == "" {
		return ""
	}
	absBase, err := filepath.Abs(l.basePath)
	if err != nil {
		return p
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if rel, err := filepath.Rel(absBase, absP); err == nil {
		return rel
	}
	return p
}

// firstLine trims a (possibly multi-line) description to its first non-empty line.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// basesOfBlocks returns the sorted per-base keys, or [AnyBase] when the manifest only has
// root-level content (e.g. monitoring-dashboard).
func basesOfBlocks(perBase map[string]ast.BlueprintBlock) []string {
	if len(perBase) == 0 {
		return []string{AnyBase}
	}
	bases := make([]string, 0, len(perBase))
	for b := range perBase {
		bases = append(bases, b)
	}
	sort.Strings(bases)
	return bases
}

func baseOrAny(b string) []string {
	if strings.TrimSpace(b) == "" {
		return []string{AnyBase}
	}
	return []string{b}
}

// featureBases mirrors the compiler's rule in feature.go (PHASE 3): root-level
// deployment_groups are applied on every base, and per-base blocks only add to them. So a
// feature with root-level modules works on any base (e.g. managed-lustre, filestore), even
// if it has extra per-base content for only some bases.
func featureBases(m *ast.FeatureManifest) []string {
	if len(m.DeploymentGroups) > 0 {
		return []string{AnyBase}
	}
	return basesOfBlocks(m.ConfigBase)
}

func flattenModules(groups []ast.DeploymentGroup) []ast.ModuleSpec {
	var mods []ast.ModuleSpec
	for _, g := range groups {
		mods = append(mods, g.Modules...)
	}
	return mods
}

func sectionsOf(root ast.BlueprintBlock, perBase map[string]ast.BlueprintBlock) []CatalogSection {
	var out []CatalogSection
	if len(root.Vars) > 0 || len(root.DeploymentGroups) > 0 {
		out = append(out, CatalogSection{Vars: root.Vars, Modules: flattenModules(root.DeploymentGroups)})
	}
	for _, b := range basesOfBlocks(perBase) {
		if b == AnyBase {
			continue
		}
		blk := perBase[b]
		out = append(out, CatalogSection{Base: b, Vars: blk.Vars, Modules: flattenModules(blk.DeploymentGroups)})
	}
	return out
}

// exampleSummary builds a one-line description for a cluster-config, which (unlike catalog
// manifests) carries no description field.
func exampleSummary(cfg *ast.ClusterConfig) string {
	var parts []string
	var machines []string
	seen := map[string]bool{}
	for _, a := range cfg.ComputeArchetypes {
		if a.MachineType != "" && !seen[a.MachineType] {
			seen[a.MachineType] = true
			machines = append(machines, a.MachineType)
		}
	}
	if len(machines) > 0 {
		parts = append(parts, strings.Join(machines, ", "))
	}
	parts = append(parts, plural(len(cfg.Features), "feature"), plural(len(cfg.Overlays), "overlay"))
	return strings.Join(parts, " · ")
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func sortEntries(es []CatalogEntry) []CatalogEntry {
	sort.Slice(es, func(i, j int) bool { return es[i].Name < es[j].Name })
	return es
}

// ListEntries returns the sorted catalog entries of one kind.
func (l *CatalogLoader) ListEntries(kind CatalogKind) ([]CatalogEntry, error) {
	if err := l.ensureCatalog(); err != nil {
		return nil, err
	}
	out, err := l.listRawEntries(kind)
	if err != nil {
		return nil, err
	}
	return sortEntries(out), nil
}

func (l *CatalogLoader) listRawEntries(kind CatalogKind) ([]CatalogEntry, error) {
	switch kind {
	case KindBase:
		return l.listBaseEntries()
	case KindArchetype:
		return l.listArchetypeEntries()
	case KindFeature:
		return l.listFeatureEntries()
	case KindOverlay:
		return l.listOverlayEntries()
	case KindExample:
		return l.listExampleEntries()
	default:
		return nil, fmt.Errorf("unknown catalog kind %q", kind)
	}
}

func (l *CatalogLoader) listBaseEntries() ([]CatalogEntry, error) {
	if err := l.loadConfigBaseCache(); err != nil {
		return nil, err
	}
	var out []CatalogEntry
	for name, m := range l.configBaseCache {
		out = append(out, CatalogEntry{Kind: KindBase, Name: name, Description: firstLine(m.Description),
			Bases: []string{name}, File: l.relFile(KindBase, name)})
	}
	return out, nil
}

func (l *CatalogLoader) listArchetypeEntries() ([]CatalogEntry, error) {
	if err := l.loadArchetypeCache(); err != nil {
		return nil, err
	}
	var out []CatalogEntry
	for name, m := range l.archetypeCache {
		out = append(out, CatalogEntry{Kind: KindArchetype, Name: name, Description: firstLine(m.Description),
			Bases: basesOfBlocks(m.ConfigBase), Aliases: m.MatchPatterns, File: l.relFile(KindArchetype, name)})
	}
	return out, nil
}

func (l *CatalogLoader) listFeatureEntries() ([]CatalogEntry, error) {
	if err := l.loadFeatureCache(); err != nil {
		return nil, err
	}
	var out []CatalogEntry
	for name, m := range l.featureCache {
		out = append(out, CatalogEntry{Kind: KindFeature, Name: name, Description: firstLine(m.Description),
			Bases: featureBases(m), File: l.relFile(KindFeature, name)})
	}
	return out, nil
}

func (l *CatalogLoader) listOverlayEntries() ([]CatalogEntry, error) {
	if err := l.loadOverlayCache(); err != nil {
		return nil, err
	}
	var out []CatalogEntry
	for name, m := range l.overlayCache {
		out = append(out, CatalogEntry{Kind: KindOverlay, Name: name, Description: firstLine(m.Description),
			Bases: baseOrAny(m.ConfigBase.Type), File: l.relFile(KindOverlay, name)})
	}
	return out, nil
}

func (l *CatalogLoader) listExampleEntries() ([]CatalogEntry, error) {
	if err := l.loadExampleCache(); err != nil {
		return nil, err
	}
	var out []CatalogEntry
	for name, cfg := range l.exampleCache {
		out = append(out, CatalogEntry{Kind: KindExample, Name: name, Description: exampleSummary(cfg),
			Bases: baseOrAny(cfg.ConfigBase.Type), File: l.relFile(KindExample, name)})
	}
	return out, nil
}

// ListAll returns the entries of every kind.
func (l *CatalogLoader) ListAll() (map[CatalogKind][]CatalogEntry, error) {
	all := make(map[CatalogKind][]CatalogEntry, len(AllCatalogKinds))
	for _, k := range AllCatalogKinds {
		es, err := l.ListEntries(k)
		if err != nil {
			return nil, err
		}
		all[k] = es
	}
	total := 0
	for _, es := range all {
		total += len(es)
	}
	if total == 0 {
		return nil, fmt.Errorf("%w at %s (directory contains no manifests)", ErrCatalogNotFound, l.CatalogRoot())
	}
	return all, nil
}

// Lookup returns every entry whose name matches exactly, across all kinds. A trailing
// ".yaml" is ignored so example file names can be pasted as-is.
//
// Archetype aliases are deliberately NOT consulted: generic-cpu declares the alias "*",
// which would turn every typo into a silent match.
func (l *CatalogLoader) Lookup(name string) ([]CatalogEntry, error) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".yaml")
	all, err := l.ListAll()
	if err != nil {
		return nil, err
	}
	var matches []CatalogEntry
	for _, k := range AllCatalogKinds {
		for _, e := range all[k] {
			if e.Name == name {
				matches = append(matches, e)
			}
		}
	}
	return matches, nil
}

// Describe returns the full detail for an entry previously returned by ListEntries/Lookup.
func (l *CatalogLoader) Describe(e CatalogEntry) (*CatalogDetail, error) {
	d := &CatalogDetail{CatalogEntry: e}
	if err := l.populateCatalogDetail(d, e); err != nil {
		return nil, err
	}
	l.relativizeSections(d.Sections)
	d.Sections = nonEmptySections(d.Sections)
	if p := l.files[e.Kind][e.Name]; p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			d.AbsFile = abs
		}
	}
	return d, nil
}

func (l *CatalogLoader) populateCatalogDetail(d *CatalogDetail, e CatalogEntry) error {
	switch e.Kind {
	case KindBase:
		m, err := l.LoadConfigBase(e.Name)
		if err != nil {
			return err
		}
		d.Sections = []CatalogSection{{Base: e.Name, Vars: m.Vars, Modules: flattenModules(m.DeploymentGroups)}}
	case KindArchetype:
		return l.populateArchetypeDetail(d, e.Name)
	case KindFeature:
		m, err := l.LoadFeature(e.Name)
		if err != nil {
			return err
		}
		d.Sections = sectionsOf(m.BlueprintBlock, m.ConfigBase)
	case KindOverlay:
		return l.populateOverlayDetail(d, e.Name)
	case KindExample:
		return l.populateExampleDetail(d, e.Name)
	default:
		return fmt.Errorf("unknown catalog kind %q", e.Kind)
	}
	return nil
}

func (l *CatalogLoader) populateArchetypeDetail(d *CatalogDetail, name string) error {
	if err := l.loadArchetypeCache(); err != nil {
		return err
	}
	m, ok := l.archetypeCache[name] // exact; never alias-resolved
	if !ok {
		return fmt.Errorf("compute archetype %q not found", name)
	}
	d.Sections = sectionsOf(m.BlueprintBlock, m.ConfigBase)
	return nil
}

func (l *CatalogLoader) populateOverlayDetail(d *CatalogDetail, name string) error {
	m, err := l.LoadOverlay(name)
	if err != nil {
		return err
	}
	if len(m.Vars) > 0 {
		d.Sections = []CatalogSection{{Base: m.ConfigBase.Type, Vars: m.Vars}}
	}
	d.OverlayFeatures = m.Features
	d.OverlayArchetypes = m.ComputeArchetypes
	return nil
}

func (l *CatalogLoader) populateExampleDetail(d *CatalogDetail, name string) error {
	if err := l.loadExampleCache(); err != nil {
		return err
	}
	cfg, ok := l.exampleCache[name]
	if !ok {
		return fmt.Errorf("example %q not found", name)
	}
	raw, err := os.ReadFile(l.files[KindExample][name])
	if err != nil {
		return err
	}
	d.Example = cfg
	d.Raw = raw
	return nil
}

// nonEmptySections drops sections with nothing to show (e.g. a base block that only
// carries assertions).
func nonEmptySections(in []CatalogSection) []CatalogSection {
	var out []CatalogSection
	for _, s := range in {
		if len(s.Vars) > 0 || len(s.Modules) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// relativizeSections undoes absolutiseStagePaths for display: the loader rewrites
// ghpc_stage("<rel>") to an absolute path for the compiler, but users should see the
// toolkit-relative path that is actually written in the manifest.
func (l *CatalogLoader) relativizeSections(sections []CatalogSection) {
	root, err := filepath.Abs(l.basePath)
	if err != nil || root == "" {
		return
	}
	prefix := root + string(filepath.Separator)
	for i := range sections {
		if len(sections[i].Vars) == 0 {
			continue
		}
		vars := make(map[string]any, len(sections[i].Vars))
		for k, v := range sections[i].Vars {
			if s, ok := v.(string); ok {
				v = strings.ReplaceAll(s, prefix, "")
			}
			vars[k] = v
		}
		sections[i].Vars = vars
	}
}

// =========================================================================
// BASE-SCOPED LISTING (`gcluster catalog <kind> --base <base>`)
// =========================================================================

// ValidateBase returns an error naming the available bases if base is not one of them.
func (l *CatalogLoader) ValidateBase(base string) error {
	bases, err := l.ListEntries(KindBase)
	if err != nil {
		return err
	}
	var names []string
	for _, b := range bases {
		if b.Name == base {
			return nil
		}
		names = append(names, b.Name)
	}
	return fmt.Errorf("unknown base %q (available: %s)", base, strings.Join(names, ", "))
}

func supportsBase(bases []string, base string) bool {
	for _, b := range bases {
		if b == base || b == AnyBase {
			return true
		}
	}
	return false
}

// primaryFeatureModule returns the module that IS the feature on the given base: the one
// whose ID is exactly "{name}", else the first "{name}*" module. Shared helpers such as
// `cluster` or `private_service_access` are skipped. Root-level modules are searched
// first because the compiler always applies them (feature.go PHASE 3).
func primaryFeatureModule(m *ast.FeatureManifest, base string) string {
	mods := flattenModules(m.DeploymentGroups)
	if blk, ok := m.ConfigBase[base]; ok {
		mods = append(mods, flattenModules(blk.DeploymentGroups)...)
	}
	for _, mod := range mods {
		if mod.ID == "{name}" && mod.Source != "" {
			return mod.Source
		}
	}
	for _, mod := range mods {
		if strings.HasPrefix(mod.ID, "{name}") && mod.Source != "" {
			return mod.Source
		}
	}
	return ""
}

// primaryArchetypeModule returns the compute-pool module an archetype emits on the given
// base (the "{name}*" module that takes a machine_type), e.g. gke-node-pool,
// schedmd-slurm-gcp-v6-nodeset or vm-instance.
func primaryArchetypeModule(m *ast.ComputeArchetypeManifest, base string) string {
	blk, ok := m.ConfigBase[base]
	if !ok {
		return ""
	}
	mods := flattenModules(blk.DeploymentGroups)
	for _, mod := range mods {
		if strings.Contains(mod.ID, "{name}") && mod.Source != "" {
			if _, ok := mod.Settings["machine_type"]; ok {
				return mod.Source
			}
		}
	}
	for _, mod := range mods {
		if strings.Contains(mod.ID, "{name}") && mod.Source != "" {
			return mod.Source
		}
	}
	return ""
}

// ListEntriesForBase returns the entries of one kind that can be used with the given
// config base, with base-specific fields (PrimaryModule, Features) populated.
// Entries that work on any base are included.
func (l *CatalogLoader) ListEntriesForBase(kind CatalogKind, base string) ([]CatalogEntry, error) {
	if err := l.ValidateBase(base); err != nil {
		return nil, err
	}
	all, err := l.ListEntries(kind)
	if err != nil {
		return nil, err
	}
	var out []CatalogEntry
	for _, e := range all {
		if !supportsBase(e.Bases, base) {
			continue
		}
		switch kind {
		case KindFeature:
			e.PrimaryModule = primaryFeatureModule(l.featureCache[e.Name], base)
		case KindArchetype:
			e.PrimaryModule = primaryArchetypeModule(l.archetypeCache[e.Name], base)
		case KindOverlay:
			seen := map[string]bool{}
			for _, f := range l.overlayCache[e.Name].Features {
				t := f.Type
				if t == "" {
					t = f.Name
				}
				if !seen[t] {
					seen[t] = true
					e.Features = append(e.Features, t)
				}
			}
		}
		out = append(out, e)
	}
	return out, nil
}
