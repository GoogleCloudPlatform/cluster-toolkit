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
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeCatalogFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, "v2", rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mockCatalog(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeCatalogFile(t, root, "config-base/slurm.yaml", "name: slurm\ndescription: Slurm base\nvars:\n  project_id:\n")
	writeCatalogFile(t, root, "config-base/gke.yaml", "name: gke\ndescription: GKE base\n")
	writeCatalogFile(t, root, "compute-archetypes/generic-cpu.yaml",
		"name: generic-cpu\ndescription: Any CPU\nmatch_patterns: [\"*\"]\nconfig_base:\n  slurm: {}\n  gke: {}\n")
	writeCatalogFile(t, root, "compute-archetypes/tpu.yaml",
		"name: tpu-v6e\ndescription: TPU\nmatch_patterns: [ct6e-standard-4t]\nconfig_base:\n  gke: {}\n")
	writeCatalogFile(t, root, "features/lustre.yaml",
		"name: managed-lustre\ndescription: \"Lustre\\nsecond line\"\nconfig_base:\n  gke: {}\n")
	writeCatalogFile(t, root, "features/dash.yaml",
		"name: monitoring-dashboard\ndescription: Dash\ndeployment_groups:\n  - group: auto\n    modules:\n      - id: \"{name}\"\n        source: modules/monitoring/dashboard\n")
	writeCatalogFile(t, root, "overlays/smi.yaml",
		"name: run-nvidia-smi\ndescription: SMI\nconfig_base: gke\nfeatures:\n  - name: run-nvidia-smi\n    type: gke-job-template\n")
	writeCatalogFile(t, root, "cluster-configs/tpu-gke.yaml",
		"config_base: gke\nvars:\n  project_id:\ncompute_archetypes:\n  - name: p\n    machine_type: ct6e-standard-4t\nfeatures:\n  - name: q\n    type: kueue\n")
	return root
}

func entryNames(es []CatalogEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

func TestListEntries_Mock(t *testing.T) {
	l := NewCatalogLoader(mockCatalog(t))

	cases := []struct {
		kind  CatalogKind
		names []string
	}{
		{KindBase, []string{"gke", "slurm"}},
		{KindArchetype, []string{"generic-cpu", "tpu-v6e"}},
		{KindFeature, []string{"managed-lustre", "monitoring-dashboard"}},
		{KindOverlay, []string{"run-nvidia-smi"}},
		{KindExample, []string{"tpu-gke"}},
	}
	for _, c := range cases {
		es, err := l.ListEntries(c.kind)
		if err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		if got := entryNames(es); !reflect.DeepEqual(got, c.names) {
			t.Errorf("%s: got %v, want %v", c.kind, got, c.names)
		}
	}

	feats, _ := l.ListEntries(KindFeature)
	if feats[0].Description != "Lustre" {
		t.Errorf("multi-line description should be trimmed to first line, got %q", feats[0].Description)
	}
	if !reflect.DeepEqual(feats[1].Bases, []string{AnyBase}) {
		t.Errorf("root-level feature should report bases [any], got %v", feats[1].Bases)
	}
	if feats[0].File != filepath.Join("v2", "features", "lustre.yaml") {
		t.Errorf("unexpected relative file %q", feats[0].File)
	}

	archs, _ := l.ListEntries(KindArchetype)
	if !reflect.DeepEqual(archs[0].Bases, []string{"gke", "slurm"}) {
		t.Errorf("archetype bases should be sorted per-base keys, got %v", archs[0].Bases)
	}
}

func TestListExampleEntries_Summary(t *testing.T) {
	l := NewCatalogLoader(mockCatalog(t))
	es, err := l.ListEntries(KindExample)
	if err != nil {
		t.Fatal(err)
	}
	want := "ct6e-standard-4t · 1 feature · 0 overlays"
	if es[0].Description != want {
		t.Errorf("got %q, want %q", es[0].Description, want)
	}
}

func TestLookup_ExactOnly(t *testing.T) {
	l := NewCatalogLoader(mockCatalog(t))
	for _, name := range []string{"n2-standard-16", "ct6e-standard-4t", "managed-lustr"} {
		m, err := l.Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(m) != 0 {
			t.Errorf("Lookup(%q) must not resolve aliases or fuzzy names, got %v", name, m)
		}
	}
	m, _ := l.Lookup("tpu-gke.yaml")
	if len(m) != 1 || m[0].Kind != KindExample {
		t.Errorf("Lookup should accept a trailing .yaml for examples, got %v", m)
	}
}

func TestLookup_Ambiguous(t *testing.T) {
	root := mockCatalog(t)
	writeCatalogFile(t, root, "overlays/clash.yaml", "name: managed-lustre\nconfig_base: gke\nvars:\n  x: 1\n")
	m, err := NewCatalogLoader(root).Lookup("managed-lustre")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Errorf("expected 2 matches across kinds, got %v", m)
	}
}

func TestDescribe_AllKinds(t *testing.T) {
	l := NewCatalogLoader(mockCatalog(t))
	for _, name := range []string{"slurm", "tpu-v6e", "monitoring-dashboard", "run-nvidia-smi", "tpu-gke"} {
		m, err := l.Lookup(name)
		if err != nil || len(m) != 1 {
			t.Fatalf("Lookup(%q) = %v, %v", name, m, err)
		}
		d, err := l.Describe(m[0])
		if err != nil {
			t.Fatalf("Describe(%q): %v", name, err)
		}
		switch d.Kind {
		case KindExample:
			if !strings.Contains(string(d.Raw), "config_base: gke") {
				t.Errorf("example raw content missing")
			}
		case KindOverlay:
			if len(d.OverlayFeatures) != 1 {
				t.Errorf("overlay features missing")
			}
		case KindFeature:
			if len(d.Sections) != 1 || d.Sections[0].Base != "" || len(d.Sections[0].Modules) != 1 {
				t.Errorf("root-level feature section wrong: %+v", d.Sections)
			}
		}
	}
}

func TestCatalogNotFound(t *testing.T) {
	l := NewCatalogLoader(t.TempDir())
	if _, err := l.ListAll(); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("missing v2/ should return ErrCatalogNotFound, got %v", err)
	}
	if _, err := l.ListEntries(KindFeature); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("ListEntries: missing v2/ should return ErrCatalogNotFound, got %v", err)
	}
	empty := t.TempDir()
	if err := os.MkdirAll(filepath.Join(empty, "v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCatalogLoader(empty).ListAll(); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("empty v2/ should return ErrCatalogNotFound, got %v", err)
	}
}

// ---- Tests against the real shipped catalog -------------------------------------------

var realCatalogDirs = map[CatalogKind]string{
	KindBase:      "config-base",
	KindArchetype: "compute-archetypes",
	KindFeature:   "features",
	KindOverlay:   "overlays",
	KindExample:   "cluster-configs",
}

func realLoader(t *testing.T) (*CatalogLoader, string) {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	return NewCatalogLoader(root), root
}

// TestRealCatalog_Coverage: every shipped manifest is discoverable.
func TestRealCatalog_Coverage(t *testing.T) {
	l, root := realLoader(t)
	for kind, dir := range realCatalogDirs {
		files, _ := filepath.Glob(filepath.Join(root, "v2", dir, "*.yaml"))
		es, err := l.ListEntries(kind)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		listed := map[string]bool{}
		for _, e := range es {
			listed[e.File] = true
		}
		for _, f := range files {
			rel, _ := filepath.Rel(root, f)
			if !listed[rel] {
				t.Errorf("%s is not listed by `gcluster catalog %ss`", rel, kind)
			}
		}
	}
}

// TestRealCatalog_NoCrossKindNameCollisions protects `gcluster catalog show <name>`, which
// auto-detects the kind from the name.
func TestRealCatalog_NoCrossKindNameCollisions(t *testing.T) {
	l, _ := realLoader(t)
	all, err := l.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]CatalogKind{}
	for _, k := range AllCatalogKinds {
		for _, e := range all[k] {
			if prev, ok := seen[e.Name]; ok {
				t.Errorf("name %q is used by both %s and %s; `catalog show %s` would be ambiguous", e.Name, prev, k, e.Name)
			}
			seen[e.Name] = k
		}
	}
}

// TestRealCatalog_AllHaveDescriptions keeps list output useful.
func TestRealCatalog_AllHaveDescriptions(t *testing.T) {
	l, _ := realLoader(t)
	for _, k := range []CatalogKind{KindBase, KindArchetype, KindFeature, KindOverlay} {
		es, err := l.ListEntries(k)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			if strings.TrimSpace(e.Description) == "" {
				t.Errorf("%s %q (%s) has no description", k, e.Name, e.File)
			}
		}
	}
}

// TestRealCatalog_DescribeEverything: `show` works for every shipped item.
func TestRealCatalog_DescribeEverything(t *testing.T) {
	l, _ := realLoader(t)
	all, err := l.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range AllCatalogKinds {
		for _, e := range all[k] {
			if _, err := l.Describe(e); err != nil {
				t.Errorf("Describe(%s/%s): %v", k, e.Name, err)
			}
		}
	}
}

// TestFeatureWithRootModulesIsAnyBase mirrors feature.go: root-level deployment_groups are
// applied on every base, so per-base extras for one base must not narrow the list.
func TestFeatureWithRootModulesIsAnyBase(t *testing.T) {
	root := mockCatalog(t)
	writeCatalogFile(t, root, "features/fs.yaml",
		"name: filestore\ndescription: FS\ndeployment_groups:\n  - group: auto\n    modules:\n      - id: fs\n        source: modules/file-system/filestore\nconfig_base:\n  gke:\n    vars:\n      x: 1\n")
	es, err := NewCatalogLoader(root).ListEntries(KindFeature)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if e.Name == "filestore" && !reflect.DeepEqual(e.Bases, []string{AnyBase}) {
			t.Errorf("feature with root-level modules should be [any], got %v", e.Bases)
		}
	}
}

func supports(bases []string, base string) bool {
	for _, b := range bases {
		if b == base || b == AnyBase {
			return true
		}
	}
	return false
}

// TestRealCatalog_BasesAgreeWithShippedExamples: if a shipped example uses a feature or
// archetype on base X, the catalog must not claim that item is unavailable on X.
func TestRealCatalog_BasesAgreeWithShippedExamples(t *testing.T) {
	l, _ := realLoader(t)
	all, err := l.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	bases := map[CatalogKind]map[string][]string{}
	for _, k := range []CatalogKind{KindArchetype, KindFeature} {
		bases[k] = map[string][]string{}
		for _, e := range all[k] {
			bases[k][e.Name] = e.Bases
		}
	}
	if err := l.loadExampleCache(); err != nil {
		t.Fatal(err)
	}
	for name, cfg := range l.exampleCache {
		base := cfg.ConfigBase.Type
		for _, f := range cfg.Features {
			if b, ok := bases[KindFeature][f.Type]; ok && !supports(b, base) {
				t.Errorf("example %s uses feature %q on %s, but catalog reports bases %v", name, f.Type, base, b)
			}
		}
		for _, a := range cfg.ComputeArchetypes {
			if b, ok := bases[KindArchetype][a.MachineType]; ok && !supports(b, base) {
				t.Errorf("example %s uses archetype %q on %s, but catalog reports bases %v", name, a.MachineType, base, b)
			}
		}
	}
}

// ---- Base-scoped listing (--base) ------------------------------------------------------

func TestListEntriesForBase_Mock(t *testing.T) {
	l := NewCatalogLoader(mockCatalog(t))
	es, err := l.ListEntriesForBase(KindFeature, "gke")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryNames(es); !reflect.DeepEqual(got, []string{"managed-lustre", "monitoring-dashboard"}) {
		t.Errorf("gke features = %v", got)
	}
	es, _ = l.ListEntriesForBase(KindFeature, "slurm")
	if got := entryNames(es); !reflect.DeepEqual(got, []string{"monitoring-dashboard"}) {
		t.Errorf("slurm features should only include any-base items, got %v", got)
	}
	if es[0].PrimaryModule != "modules/monitoring/dashboard" {
		t.Errorf("primary module = %q", es[0].PrimaryModule)
	}
	es, _ = l.ListEntriesForBase(KindOverlay, "slurm")
	if len(es) != 0 {
		t.Errorf("no overlays should match slurm, got %v", es)
	}
	es, _ = l.ListEntriesForBase(KindOverlay, "gke")
	if len(es) != 1 || !reflect.DeepEqual(es[0].Features, []string{"gke-job-template"}) {
		t.Errorf("overlay features = %+v", es)
	}
	if _, err := l.ListEntriesForBase(KindFeature, "gkee"); err == nil || !strings.Contains(err.Error(), "available: gke, slurm") {
		t.Errorf("unknown base should list available bases, got %v", err)
	}
}

func TestRealCatalog_ListEntriesForBase(t *testing.T) {
	l, _ := realLoader(t)
	primary := func(kind CatalogKind, base, name string) (string, bool) {
		es, err := l.ListEntriesForBase(kind, base)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			if e.Name == name {
				return e.PrimaryModule, true
			}
		}
		return "", false
	}
	cases := []struct {
		kind             CatalogKind
		base, name, want string
	}{
		{KindFeature, "gke", "managed-lustre", "modules/file-system/managed-lustre"},
		{KindFeature, "slurm", "managed-lustre", "modules/file-system/managed-lustre"},
		{KindFeature, "gke", "filestore", "modules/file-system/filestore"},
		{KindFeature, "gke", "kueue-jobset", "modules/management/kubectl-apply"},
		{KindFeature, "gke", "cloud-storage", "modules/file-system/cloud-storage-bucket"},
		{KindFeature, "slurm", "netapp-volume", "modules/file-system/netapp-volume"},
		{KindArchetype, "gke", "a4-highgpu-8g", "modules/compute/gke-node-pool"},
		{KindArchetype, "slurm", "a4-highgpu-8g", "community/modules/compute/schedmd-slurm-gcp-v6-nodeset"},
		{KindArchetype, "jbvm", "a4-highgpu-8g", "modules/compute/vm-instance"},
	}
	for _, c := range cases {
		got, ok := primary(c.kind, c.base, c.name)
		if !ok {
			t.Errorf("%s %q missing from --base %s", c.kind, c.name, c.base)
			continue
		}
		if got != c.want {
			t.Errorf("%s %q --base %s: primary module %q, want %q", c.kind, c.name, c.base, got, c.want)
		}
	}
	if _, ok := primary(KindFeature, "slurm", "kueue-jobset"); ok {
		t.Error("kueue-jobset is gke-only and must not be listed for slurm")
	}
	if _, ok := primary(KindArchetype, "jbvm", "tpu-v6e"); ok {
		t.Error("tpu-v6e is gke-only and must not be listed for jbvm")
	}
	// Every listed feature/archetype must resolve a primary module on that base.
	for _, base := range []string{"gke", "slurm", "jbvm"} {
		for _, k := range []CatalogKind{KindFeature, KindArchetype} {
			es, _ := l.ListEntriesForBase(k, base)
			for _, e := range es {
				if e.PrimaryModule == "" {
					t.Errorf("%s %q on %s has no primary module", k, e.Name, base)
				}
			}
		}
	}
}
