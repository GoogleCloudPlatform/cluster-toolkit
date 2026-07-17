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

// Package catalog implements `gcluster catalog`, a read-only browser for the building
// blocks of a cluster-config: config bases, compute archetypes, features, overlays and
// ready-made examples.
package catalog

import (
	"errors"
	"fmt"

	"hpc-toolkit/pkg/intent"
	"hpc-toolkit/pkg/intent/engine"

	"github.com/spf13/cobra"
)

// newLoader is a seam for tests. Production resolves the catalog exactly like
// `gcluster create` does, so discovery always matches what the compiler reads.
var newLoader = func() *engine.CatalogLoader {
	return engine.NewCatalogLoader(intent.ToolkitPath())
}

// CatalogCmd is the `gcluster catalog` command.
//
// It deliberately defines no PersistentPreRun(E): unlike `gcluster cluster`, it must work
// offline, without gcloud or a Google Cloud project.
var CatalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Browse building blocks for cluster-configs (bases, archetypes, features, overlays, examples).",
	Long: `Browse the building blocks you can combine in a cluster-config file:

  Base        config_base                        Orchestrator foundation (e.g. gke, slurm)
  Archetype   compute_archetypes[].machine_type  Machine / accelerator type for a compute pool
  Feature     features[].type                    Storage, job queuing, monitoring, ...
  Overlay     overlays[].type                    Verification jobs and other add-ons
  Example     (whole file)                       Ready-made cluster-configs to start from

Run with no subcommand for a one-screen overview.`,
	Example: `  gcluster catalog
  gcluster catalog archetypes
  gcluster catalog features
  gcluster catalog features --base slurm`,
	Args: cobra.NoArgs,
	RunE: runOverview,
	// "v2" is a hidden alias for people who know the engine by its internal name. It is not
	// listed in `gcluster --help` (which shows only command names) and hideAliasUsage keeps it
	// out of `gcluster catalog --help`, so beginners only ever see "catalog".
	Aliases: []string{hiddenAlias},
}

const hiddenAlias = "v2"

func init() {
	CatalogCmd.AddCommand(
		newListCmd("bases", []string{"base"}, engine.KindBase, "List config bases (values for `config_base`)."),
		newListCmd("archetypes", []string{"archetype"}, engine.KindArchetype, "List compute archetypes (values for `compute_archetypes[].machine_type`)."),
		newListCmd("features", []string{"feature"}, engine.KindFeature, "List features (values for `features[].type`)."),
		newListCmd("overlays", []string{"overlay"}, engine.KindOverlay, "List overlays (values for `overlays[].type`)."),
		newListCmd("examples", []string{"example", "cluster-configs", "cluster-config"}, engine.KindExample, "List ready-made example cluster-configs."),
	)
	CatalogCmd.SetUsageFunc(hideAliasUsage)
}

// hideAliasUsage renders the normal cobra usage, except that the catalog command's own
// "Aliases:" section (which would reveal the hidden "v2" alias) is omitted. Subcommands
// inherit this func and still show their aliases (e.g. "features, feature").
func hideAliasUsage(c *cobra.Command) error {
	if c == CatalogCmd {
		saved := c.Aliases
		c.Aliases = nil
		defer func() { c.Aliases = saved }()
	}
	// The root command has no custom usage func, so this is cobra's default renderer.
	root := c.Root()
	if root == CatalogCmd { // only in tests, where catalog is executed without a parent
		root = &cobra.Command{}
	}
	return root.UsageFunc()(c)
}

// friendlyErr adds an actionable hint to catalog-not-found errors.
func friendlyErr(err error) error {
	if errors.Is(err, engine.ErrCatalogNotFound) {
		return fmt.Errorf("%w\ngcluster reads the catalog from the v2/ directory next to the gcluster binary.\n"+
			"Run the gcluster built in your Cluster Toolkit checkout (e.g. ./gcluster from the repo root)", err)
	}
	return err
}

func runOverview(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	all, err := newLoader().ListAll()
	if err != nil {
		return friendlyErr(err)
	}
	renderOverview(cmd.OutOrStdout(), all)
	return nil
}
