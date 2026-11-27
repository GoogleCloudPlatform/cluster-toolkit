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

package cmd

import (
	"context"
	"errors"
	cdapi "hpc-toolkit/pkg/clusterdirector"
	"hpc-toolkit/pkg/config"
	"path/filepath"

	"github.com/spf13/cobra"
)

var flagExportToClusterDirector bool

func validateClusterDirectorFlags(cmd *cobra.Command) error {
	if _, ok := clusterDirectorFlagOverride(cmd); ok && (flagOnlyGroups != nil || flagSkipGroups != nil) {
		return errors.New("cannot combine --export-to-cluster-director with --only or --skip")
	}
	return nil
}

// addClusterDirectorFlags registers the --export-to-cluster-director flag on
// `gcluster deploy` to override the blueprint's `export_to_cluster_director`
// setting (true to export/update, false to skip or delete from Cluster Director).
func addClusterDirectorFlags(c *cobra.Command) *cobra.Command {
	c.Flags().BoolVar(&flagExportToClusterDirector, "export-to-cluster-director", false,
		"Export the deployment to Cluster Director, overriding the blueprint's export_to_cluster_director setting (true to export or update, false to skip, or to delete a previous export)")
	return c
}

func clusterDirectorFlagOverride(cmd *cobra.Command) (bool, bool) {
	if cmd == nil {
		return false, false
	}
	if f := cmd.Flags().Lookup("export-to-cluster-director"); f != nil && f.Changed {
		return flagExportToClusterDirector, true
	}
	return false, false
}

func clusterDirectorOverridePtr(cmd *cobra.Command) *bool {
	if v, ok := clusterDirectorFlagOverride(cmd); ok {
		return &v
	}
	return nil
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

// inheritExportToClusterDirector carries forward the export_to_cluster_director
// decision recorded in an existing deployment folder when re-creating (`-w`) a
// deployment whose blueprint, deployment file, and CLI flags leave the setting
// unset.
func inheritExportToClusterDirector(cmd *cobra.Command, bp *config.Blueprint) {
	if bp.ExportToClusterDirector != nil || (cmd != nil && cmd.Name() == "expand") {
		return
	}
	name, ok := deploymentNameOf(*bp)
	if !ok {
		return
	}
	if prev, hasPrev := cdapi.PreviousExportSetting(filepath.Join(createFlags.outputDir, name)); hasPrev {
		bp.SetExportToClusterDirector(prev)
	}
}

func exportClusterDirector(cmd *cobra.Command, deplRoot string, artifactsDir string, bp config.Blueprint) error {
	return cdapi.Export(commandContext(cmd), cdapi.ExportOptions{
		DeplRoot:     deplRoot,
		ArtifactsDir: artifactsDir,
		Blueprint:    bp,
		Override:     clusterDirectorOverridePtr(cmd),
	})
}

func deleteClusterDirector(cmd *cobra.Command, deplRoot string, artifactsDir string, bp config.Blueprint, skippedGroups map[config.GroupName]bool) error {
	return cdapi.Delete(commandContext(cmd), cdapi.DeleteOptions{
		DeplRoot:        deplRoot,
		ArtifactsDir:    artifactsDir,
		Blueprint:       bp,
		Override:        clusterDirectorOverridePtr(cmd),
		IsGroupSelected: isGroupSelected,
		SkippedGroups:   skippedGroups,
	})
}
