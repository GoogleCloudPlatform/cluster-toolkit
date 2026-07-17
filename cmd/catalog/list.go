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
	"hpc-toolkit/pkg/intent/engine"

	"github.com/spf13/cobra"
)

// newListCmd builds one of the `gcluster catalog <kind-plural>` list commands. The singular
// form (e.g. `feature`) is accepted as an alias, matching the singular nouns used elsewhere in
// gcluster (`gcluster cluster`, `gcluster job`); extra aliases can follow (e.g. examples also
// answers to `cluster-configs`, the name used in docs and the v2/cluster-configs directory).
// Every kind except bases also accepts `--base <gke|slurm|jbvm>` to show only what works on
// that base.
func newListCmd(use string, aliases []string, kind engine.CatalogKind, short string) *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   short,
		Args:    cobra.NoArgs,
		Example: "  gcluster catalog " + use,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			loader := newLoader()
			if base != "" {
				entries, err := loader.ListEntriesForBase(kind, base)
				if err != nil {
					return friendlyErr(err)
				}
				renderListForBase(cmd.OutOrStdout(), kind, base, entries)
				return nil
			}
			entries, err := loader.ListEntries(kind)
			if err != nil {
				return friendlyErr(err)
			}
			renderList(cmd.OutOrStdout(), kind, entries)
			return nil
		},
	}
	if kind != engine.KindBase {
		cmd.Flags().StringVar(&base, "base", "", "Only show items that work with this config base (e.g. gke, slurm, jbvm)")
		cmd.Example += "\n  gcluster catalog " + use + " --base gke"
		_ = cmd.RegisterFlagCompletionFunc("base", completeBases)
	}
	return cmd
}

// completeBases suggests base names for --base from the catalog.
func completeBases(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	entries, err := newLoader().ListEntries(engine.KindBase)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name+"\t"+e.Description)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
