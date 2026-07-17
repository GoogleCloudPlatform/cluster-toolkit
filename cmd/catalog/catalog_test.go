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

package catalog

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"hpc-toolkit/pkg/intent/engine"

	"github.com/spf13/cobra"
)

// useRealCatalog points the command at the repo's shipped v2/ catalog.
func useRealCatalog(t *testing.T) {
	t.Helper()
	old := newLoader
	newLoader = func() *engine.CatalogLoader { return engine.NewCatalogLoader(filepath.Join("..", "..")) }
	t.Cleanup(func() { newLoader = old })
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	CatalogCmd.SetOut(&buf)
	CatalogCmd.SetErr(&buf)
	CatalogCmd.SetArgs(args)
	err := CatalogCmd.Execute()
	return buf.String(), err
}

func assertContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}
}

func TestOverview(t *testing.T) {
	useRealCatalog(t)
	out, err := run(t)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Base", "Archetype", "Feature", "Overlay", "Example",
		"config_base", "compute_archetypes[].machine_type", "--base <base>")
}

func TestListCommands(t *testing.T) {
	useRealCatalog(t)
	cases := map[string][]string{
		"bases":      {"NAME", "DESCRIPTION", "gke", "slurm", "jbvm"},
		"archetypes": {"BASES", "a4-highgpu-8g", "cpu", "tpu-v6e"},
		"features":   {"BASES", "managed-lustre", "kueue-jobset", "monitoring-dashboard"},
		"overlays":   {"nccl-jobset-test", "run-nvidia-smi"},
		"examples":   {"SUMMARY", "a4high-slurm", "cpu-slurm", "cp v2/cluster-configs/<name>.yaml"},
	}
	for sub, wants := range cases {
		out, err := run(t, sub)
		if err != nil {
			t.Fatalf("%s: %v", sub, err)
		}
		assertContains(t, out, wants...)
	}
}

func TestShowRemoved(t *testing.T) {
	useRealCatalog(t)
	if _, err := run(t, "show", "managed-lustre"); err == nil {
		t.Error("`catalog show` was removed and should be rejected")
	}
}

func TestCatalogNotFound(t *testing.T) {
	old := newLoader
	dir := t.TempDir()
	newLoader = func() *engine.CatalogLoader { return engine.NewCatalogLoader(dir) }
	t.Cleanup(func() { newLoader = old })
	_, err := run(t, "features")
	if err == nil || !strings.Contains(err.Error(), "catalog not found") || !strings.Contains(err.Error(), "next to the gcluster binary") {
		t.Errorf("expected actionable catalog-not-found error, got %v", err)
	}
}

// resetBaseFlags clears --base between Execute calls (cobra keeps flag values).
func resetBaseFlags(t *testing.T) {
	t.Helper()
	for _, c := range CatalogCmd.Commands() {
		if f := c.Flags().Lookup("base"); f != nil {
			_ = f.Value.Set("")
			f.Changed = false
		}
	}
}

func TestListWithBase(t *testing.T) {
	useRealCatalog(t)
	t.Cleanup(func() { resetBaseFlags(t) })
	cases := []struct {
		args        []string
		wants, nots []string
	}{
		{[]string{"features", "--base", "gke"},
			[]string{"Available features for 'gke':", "PRIMARY MODULE", "managed-lustre", "modules/file-system/managed-lustre", "kueue", "modules/management/kubectl-apply", "config_base: gke"},
			[]string{"netapp-volume", "startup-script"}},
		{[]string{"features", "--base", "slurm"},
			[]string{"Available features for 'slurm':", "netapp-volume", "startup-script", "managed-lustre"},
			[]string{"kueue", "gke-hyperdisk"}},
		{[]string{"archetypes", "--base", "jbvm"},
			[]string{"Available archetypes for 'jbvm':", "a4-highgpu-8g", "modules/compute/vm-instance"},
			[]string{"tpu-v6e", "a3-megagpu-8g"}},
		{[]string{"archetypes", "--base", "gke"},
			[]string{"tpu-v6e", "a3-megagpu-8g", "modules/compute/gke-node-pool"}, nil},
		{[]string{"overlays", "--base", "gke"},
			[]string{"ADDS FEATURE", "nccl-jobset-test", "gke-job-template"}, nil},
		{[]string{"overlays", "--base", "slurm"},
			[]string{"No overlays are available for 'slurm'."}, nil},
		{[]string{"examples", "--base", "slurm"},
			[]string{"a4high-slurm", "cpu-slurm"}, []string{"a4high-gke", "tpu-v6e-gke"}},
	}
	for _, c := range cases {
		resetBaseFlags(t)
		out, err := run(t, c.args...)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		assertContains(t, out, c.wants...)
		for _, n := range c.nots {
			if strings.Contains(out, n) {
				t.Errorf("%v: output should not contain %q\n%s", c.args, n, out)
			}
		}
	}
}

func TestListWithUnknownBase(t *testing.T) {
	useRealCatalog(t)
	t.Cleanup(func() { resetBaseFlags(t) })
	resetBaseFlags(t)
	_, err := run(t, "features", "--base", "gek")
	if err == nil || !strings.Contains(err.Error(), `unknown base "gek" (available: gke, jbvm, slurm)`) {
		t.Errorf("got %v", err)
	}
}

func TestBasesHasNoBaseFlag(t *testing.T) {
	useRealCatalog(t)
	if _, err := run(t, "bases", "--base", "gke"); err == nil {
		t.Error("`bases --base` should be rejected")
	}
}

func TestSingularAliases(t *testing.T) {
	useRealCatalog(t)
	t.Cleanup(func() { resetBaseFlags(t) })
	pairs := map[string]string{
		"base": "bases", "archetype": "archetypes", "feature": "features",
		"overlay": "overlays", "example": "examples",
		"cluster-configs": "examples", "cluster-config": "examples",
	}
	for singular, plural := range pairs {
		resetBaseFlags(t)
		want, err := run(t, plural)
		if err != nil {
			t.Fatalf("%s: %v", plural, err)
		}
		got, err := run(t, singular)
		if err != nil {
			t.Fatalf("%s: %v", singular, err)
		}
		if got != want {
			t.Errorf("`%s` output differs from `%s`", singular, plural)
		}
	}
	// Aliases accept the same flags as the plural command.
	resetBaseFlags(t)
	out, err := run(t, "feature", "--base", "gke")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Available features for 'gke':")
	// The plural name stays the one shown in `gcluster catalog --help`.
	help, _ := run(t, "--help")
	for singular := range pairs {
		if strings.Contains(help, "\n  "+singular+" ") {
			t.Errorf("catalog --help should list only plural names, found %q", singular)
		}
	}
	// A subcommand's own help tells the user the singular form works.
	fh, _ := run(t, "features", "--help")
	assertContains(t, fh, "Aliases:", "features, feature")
}

// withRoot attaches CatalogCmd to a throwaway root, like cmd/root.go does in production.
func withRoot(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "gcluster"}
	root.AddCommand(CatalogCmd, &cobra.Command{Use: "create", Run: func(*cobra.Command, []string) {}})
	t.Cleanup(func() { root.RemoveCommand(CatalogCmd) })
	return root
}

func runRoot(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestHiddenV2Alias(t *testing.T) {
	useRealCatalog(t)
	t.Cleanup(func() { resetBaseFlags(t) })
	root := withRoot(t)

	for _, args := range [][]string{{}, {"features"}, {"feature", "--base", "gke"}} {
		resetBaseFlags(t)
		want, err := runRoot(t, root, append([]string{"catalog"}, args...)...)
		if err != nil {
			t.Fatalf("catalog %v: %v", args, err)
		}
		resetBaseFlags(t)
		got, err := runRoot(t, root, append([]string{"v2"}, args...)...)
		if err != nil {
			t.Fatalf("v2 %v: %v", args, err)
		}
		if got != want {
			t.Errorf("`v2 %v` output differs from `catalog %v`", args, args)
		}
	}

	// Hidden: not in `gcluster --help`, `gcluster catalog --help`, or `gcluster v2 --help`.
	for _, args := range [][]string{{"--help"}, {"catalog", "--help"}, {"v2", "--help"}} {
		out, err := runRoot(t, root, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out, "v2") {
			t.Errorf("%v must not reveal the v2 alias:\n%s", args, out)
		}
	}
	// Rendering help must restore the alias it temporarily removed.
	if len(CatalogCmd.Aliases) != 1 || CatalogCmd.Aliases[0] != hiddenAlias {
		t.Errorf("aliases not restored: %v", CatalogCmd.Aliases)
	}
}
