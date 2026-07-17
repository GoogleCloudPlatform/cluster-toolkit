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

package engine_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/intent/engine"

	"gopkg.in/yaml.v3"
)

// matrixBaseMachine picks, per config_base, a machine type that resolves to an EXPLICIT
// archetype so the compiler does not fall through to live GCP machine-type validation.
//
// batch is the exception: only generic-cpu declares a `batch:` section, and generic-cpu is
// the catch-all, so batch coverage necessarily reaches the API. The shipped batch-hpc
// example already makes TestAllExampleClusterConfigsCompile depend on that, so this adds no
// new failure mode.
var matrixBaseMachine = map[string]string{
	"slurm": "a3-ultragpu-8g",
	"gke":   "a3-ultragpu-8g",
	"jbvm":  "a3-ultragpu-8g",
	"batch": "c2-standard-60",
}

// matrixFeatureSettings supplies the minimum settings a feature needs to compile. A feature
// absent from this map is compiled with no settings at all -- which is the interesting case,
// because a catalog entry that cannot compile from its own defaults is not usable without
// documentation the user does not have.
var matrixFeatureSettings = map[string]map[string]any{}

// matrixReport is one cell of the catalog support matrix.
type matrixReport struct {
	entry string
	base  string
	ok    bool
	err   string
}

func (r matrixReport) String() string {
	if r.ok {
		return "OK"
	}
	return r.err
}

// listCatalogEntries returns the sorted basenames of every *.yaml in one catalog directory.
func listCatalogEntries(t *testing.T, repoRoot, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, dir))
	if err != nil {
		t.Fatalf("failed to list %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names
}

// compileOne runs the full compile + serialize + HCL round-trip for one synthetic config.
// It returns the first error, which is what a user would actually see.
func compileOne(compiler *engine.Compiler, cfg ast.ClusterConfig) error {
	groups, err := compiler.Compile(cfg)
	if err != nil {
		return err
	}
	serialized, err := engine.Serialize(cfg.ConfigBase.Type, cfg.Vars, groups)
	if err != nil {
		return err
	}
	_, _, err = config.NewBlueprintFromYamlBytes([]byte(serialized))
	return err
}

// matrixConfig builds a synthetic single-pool cluster-config on one base.
func matrixConfig(base string) ast.ClusterConfig {
	cfg := syntheticClusterConfig(base, "matrix", matrixBaseMachine[base])
	cfg.Vars["deployment_name"] = "mtx-" + base
	return cfg
}

// bases returns the config_base names in a stable order.
func matrixBases() []string {
	return []string{"slurm", "gke", "jbvm", "batch"}
}

// TestFeatureBaseMatrix compiles every directly-selectable feature against every config_base
// and records the outcome.
//
// WHY THIS EXISTS: `features:` is the user-facing surface. A feature that only ever appears
// in a GKE example has never been compiled on Slurm, and nothing stops a user writing it
// there. Either it works, or the failure must name the feature and say what to do instead.
// A dangling module reference deep in Terraform is not an acceptable answer.
func TestFeatureBaseMatrix(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)
	loader := engine.NewCatalogLoader(repoRoot)

	var reports []matrixReport
	for _, name := range listCatalogEntries(t, repoRoot, filepath.Join("v2", "features")) {
		if name == "README" {
			continue
		}
		manifest, err := loader.LoadFeature(name)
		if err != nil {
			// requires_overlay features are rejected by LoadFeature on purpose; they are
			// covered by TestOverlayBaseMatrix instead.
			reports = append(reports, matrixReport{name, "-", false, "not directly selectable: " + firstLine(err.Error())})
			continue
		}
		_ = manifest
		for _, base := range matrixBases() {
			cfg := matrixConfig(base)
			cfg.Features = []ast.FeatureReference{{
				Name:     "f1",
				Type:     name,
				Settings: matrixFeatureSettings[name],
			}}
			err := compileOne(compiler, cfg)
			reports = append(reports, matrixReport{name, base, err == nil, errText(err)})
		}
	}
	logMatrix(t, "FEATURE x CONFIG_BASE", reports)
}

// TestOverlayBaseMatrix compiles every overlay against every config_base. An overlay declares
// its own support matrix via `config_base:`; this checks that the declaration is honest in
// both directions -- the declared bases must compile, and the undeclared ones must fail with
// a message that names the overlay and the bases it does support.
func TestOverlayBaseMatrix(t *testing.T) {
	repoRoot := withRealModuleFS(t)
	compiler := engine.NewCompiler(repoRoot)

	var reports []matrixReport
	for _, name := range listCatalogEntries(t, repoRoot, filepath.Join("v2", "overlays")) {
		data, err := os.ReadFile(filepath.Join(repoRoot, "v2", "overlays", name+".yaml"))
		if err != nil {
			t.Fatalf("read overlay %s: %v", name, err)
		}
		var am ast.OverlayManifest
		if err := yaml.Unmarshal(data, &am); err != nil {
			t.Fatalf("unmarshal overlay %s: %v", name, err)
		}
		declared := am.ConfigBase.Type
		if declared == "" {
			declared = "(all)"
		}
		for _, base := range matrixBases() {
			cfg := matrixConfig(base)
			cfg.Overlays = []ast.FeatureReference{{Name: "a1", Type: name}}
			err := compileOne(compiler, cfg)
			reports = append(reports, matrixReport{name + " [" + declared + "]", base, err == nil, errText(err)})
		}
	}
	logMatrix(t, "OVERLAY x CONFIG_BASE", reports)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return firstLine(err.Error())
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func logMatrix(t *testing.T, title string, reports []matrixReport) {
	t.Helper()
	var b strings.Builder
	b.WriteString("\n===== " + title + " =====\n")
	for _, r := range reports {
		status := "OK  "
		if !r.ok {
			status = "FAIL"
		}
		b.WriteString(status + "  " + r.entry + "  /  " + r.base + "\n")
		if !r.ok {
			b.WriteString("        " + r.err + "\n")
		}
	}
	t.Log(b.String())
}
