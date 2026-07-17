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
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"hpc-toolkit/pkg/intent/engine"
)

const maxDescLen = 90

// kindInfo is the user-facing vocabulary for each kind.
var kindInfo = map[engine.CatalogKind]struct {
	Title, Plural, YAMLKey, Blurb string
}{
	engine.KindBase:      {"Base", "bases", "config_base", "Orchestrator foundation"},
	engine.KindArchetype: {"Archetype", "archetypes", "compute_archetypes[].machine_type", "Machine / accelerator type"},
	engine.KindFeature:   {"Feature", "features", "features[].type", "Storage, queuing, monitoring"},
	engine.KindOverlay:   {"Overlay", "overlays", "overlays[].type", "Verification jobs, add-ons"},
	engine.KindExample:   {"Example", "examples", "(whole file)", "Ready-made cluster-configs"},
}

// exampleHint tells users how to start from a shipped example cluster-config.
const exampleHint = "\nStart from one: cp v2/cluster-configs/<name>.yaml my-cluster.yaml"

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func names(es []engine.CatalogEntry, max int) string {
	var ns []string
	for i, e := range es {
		if i == max {
			ns = append(ns, "…")
			break
		}
		ns = append(ns, e.Name)
	}
	return strings.Join(ns, ", ")
}

func renderOverview(w io.Writer, all map[engine.CatalogKind][]engine.CatalogEntry) {
	fmt.Fprintln(w, "A cluster-config is built from these building blocks:")
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  KIND\tYAML KEY\tCOUNT\tAVAILABLE")
	for _, k := range engine.AllCatalogKinds {
		ki := kindInfo[k]
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%s\n", ki.Title, ki.YAMLKey, len(all[k]), names(all[k], 4))
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Next steps:")
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  gcluster catalog <bases|archetypes|features|overlays|examples>\tList one kind")
	fmt.Fprintln(tw, "  gcluster catalog <archetypes|features|overlays|examples> --base <base>\tOnly what works with one base")
	fmt.Fprintln(tw, "  gcluster create <cluster-config.yaml>\tBuild a deployment from a cluster-config")
	_ = tw.Flush()
}

func renderList(w io.Writer, kind engine.CatalogKind, entries []engine.CatalogEntry) {
	ki := kindInfo[kind]
	if len(entries) == 0 {
		fmt.Fprintf(w, "No %s found in the catalog.\n", ki.Plural)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	switch kind {
	case engine.KindBase:
		fmt.Fprintln(tw, "NAME\tDESCRIPTION")
		for _, e := range entries {
			fmt.Fprintf(tw, "%s\t%s\n", e.Name, truncate(e.Description, maxDescLen))
		}
	case engine.KindExample:
		fmt.Fprintln(tw, "NAME\tBASE\tSUMMARY")
		for _, e := range entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, strings.Join(e.Bases, ","), truncate(e.Description, maxDescLen))
		}
	default:
		fmt.Fprintln(tw, "NAME\tBASES\tDESCRIPTION")
		for _, e := range entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, strings.Join(e.Bases, ","), truncate(e.Description, maxDescLen))
		}
	}
	_ = tw.Flush()
	if kind == engine.KindExample {
		fmt.Fprintln(w, exampleHint)
		return
	}
	fmt.Fprintf(w, "\nUse under `%s` in a cluster-config.\n", ki.YAMLKey)
}

// renderListForBase prints `gcluster catalog <kind> --base <base>`.
func renderListForBase(w io.Writer, kind engine.CatalogKind, base string, entries []engine.CatalogEntry) {
	ki := kindInfo[kind]
	if len(entries) == 0 {
		fmt.Fprintf(w, "No %s are available for '%s'.\n", ki.Plural, base)
		fmt.Fprintf(w, "See all %s: gcluster catalog %s\n", ki.Plural, ki.Plural)
		return
	}
	fmt.Fprintf(w, "Available %s for '%s':\n\n", ki.Plural, base)
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	switch kind {
	case engine.KindFeature, engine.KindArchetype:
		fmt.Fprintln(tw, "NAME\tPRIMARY MODULE\tDESCRIPTION")
		for _, e := range entries {
			mod := e.PrimaryModule
			if mod == "" {
				mod = "-"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, mod, truncate(e.Description, maxDescLen))
		}
	case engine.KindOverlay:
		fmt.Fprintln(tw, "NAME\tADDS FEATURE\tDESCRIPTION")
		for _, e := range entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, strings.Join(e.Features, ","), truncate(e.Description, maxDescLen))
		}
	case engine.KindExample:
		fmt.Fprintln(tw, "NAME\tSUMMARY")
		for _, e := range entries {
			fmt.Fprintf(tw, "%s\t%s\n", e.Name, truncate(e.Description, maxDescLen))
		}
	}
	_ = tw.Flush()
	if kind == engine.KindExample {
		fmt.Fprintln(w, exampleHint)
		return
	}
	fmt.Fprintf(w, "\nUse under `%s` in a cluster-config with `config_base: %s`.\n", ki.YAMLKey, base)
}
