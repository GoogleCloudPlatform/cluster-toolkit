/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cdapi "hpc-toolkit/pkg/clusterdirector"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"

	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
)

func tfGroup(name string) config.Group {
	return config.Group{
		Name:    config.GroupName(name),
		Modules: []config.Module{{Kind: config.TerraformKind}},
	}
}

func testBlueprint(zone string) config.Blueprint {
	enabled := true
	bp := config.Blueprint{
		Groups:                  []config.Group{tfGroup("network"), tfGroup("compute")},
		ExportToClusterDirector: &enabled,
	}
	vars := map[string]cty.Value{
		"project_id":      cty.StringVal("my-project"),
		"region":          cty.StringVal("us-central1"),
		"deployment_name": cty.StringVal("ctkdemo"),
	}
	if zone != "" {
		vars["zone"] = cty.StringVal(zone)
	}
	bp.Vars = config.NewDict(vars)
	return bp
}

func withGroupSelection(t *testing.T, only, skip []string) {
	t.Helper()
	prevOnly, prevSkip := flagOnlyGroups, flagSkipGroups
	flagOnlyGroups, flagSkipGroups = only, skip
	t.Cleanup(func() { flagOnlyGroups, flagSkipGroups = prevOnly, prevSkip })
}

func TestValidateClusterDirectorFlags(t *testing.T) {
	// Without --export-to-cluster-director, --only and --skip must continue to
	// work for existing users.
	unchangedCmd := addClusterDirectorFlags(&cobra.Command{Use: "deploy"})
	withGroupSelection(t, []string{"primary"}, nil)
	if err := validateClusterDirectorFlags(unchangedCmd); err != nil {
		t.Errorf("validateClusterDirectorFlags() with --only alone = %v, want nil", err)
	}
	withGroupSelection(t, nil, []string{"primary"})
	if err := validateClusterDirectorFlags(unchangedCmd); err != nil {
		t.Errorf("validateClusterDirectorFlags() with --skip alone = %v, want nil", err)
	}

	// With --export-to-cluster-director explicitly set, --only and --skip are rejected.
	changedCmd := addClusterDirectorFlags(&cobra.Command{Use: "deploy"})
	if err := changedCmd.Flags().Set("export-to-cluster-director", "false"); err != nil {
		t.Fatalf("Flags().Set(export-to-cluster-director, false) failed: %v", err)
	}
	withGroupSelection(t, nil, nil)
	if err := validateClusterDirectorFlags(changedCmd); err != nil {
		t.Errorf("validateClusterDirectorFlags() with no --only/--skip = %v, want nil", err)
	}
	withGroupSelection(t, []string{"primary"}, nil)
	if err := validateClusterDirectorFlags(changedCmd); err == nil {
		t.Error("validateClusterDirectorFlags() with --only succeeded, want error")
	}
	withGroupSelection(t, nil, []string{"primary"})
	if err := validateClusterDirectorFlags(changedCmd); err == nil {
		t.Error("validateClusterDirectorFlags() with --skip succeeded, want error")
	}
}

func TestDeploymentFileOverridesExportToClusterDirector(t *testing.T) {
	dir := t.TempDir()
	depFile := filepath.Join(dir, "deployment.yaml")
	if err := os.WriteFile(depFile, []byte(`export_to_cluster_director: true
vars:
  project_id: d-project
  region: us-west1
  zone: us-west1-a
  deployment_name: d-cluster
`), 0644); err != nil {
		t.Fatal(err)
	}

	ds, _, err := config.NewDeploymentSettings(depFile)
	if err != nil {
		t.Fatalf("NewDeploymentSettings() failed: %v", err)
	}
	bp := config.Blueprint{
		BlueprintName: "test-bp",
		Vars:          config.NewDict(map[string]cty.Value{"deployment_name": cty.StringVal("bp-default")}),
		Groups:        []config.Group{tfGroup("primary")},
	}
	if err := mergeDeploymentSettings(&bp, ds); err != nil {
		t.Fatalf("mergeDeploymentSettings() failed: %v", err)
	}
	if !bp.ExportToClusterDirectorEnabled() {
		t.Error("mergeDeploymentSettings() did not copy export_to_cluster_director: true from deployment file")
	}
}

func TestInheritExportToClusterDirectorOnOverwrite(t *testing.T) {
	outDir := t.TempDir()
	prevOutDir := createFlags.outputDir
	createFlags.outputDir = outDir
	t.Cleanup(func() { createFlags.outputDir = prevOutDir })

	deplDir := filepath.Join(outDir, "ctkdemo")
	artDir := modulewriter.ArtifactsDir(deplDir)
	if err := os.MkdirAll(artDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write an existing expanded_blueprint.yaml with export_to_cluster_director: true.
	prevBp := testBlueprint("us-central1-a")
	if err := prevBp.Export(filepath.Join(artDir, modulewriter.ExpandedBlueprintName)); err != nil {
		t.Fatal(err)
	}

	// 1. Re-creating (`deploy bp.yaml -w`) with export_to_cluster_director unset
	// inherits true from the previous deployment.
	unsetBp := testBlueprint("us-central1-a")
	unsetBp.ExportToClusterDirector = nil
	deployCmd := &cobra.Command{Use: "deploy"}
	deployCmd.SetContext(context.Background())
	inheritExportToClusterDirector(deployCmd, &unsetBp)
	if !unsetBp.ExportToClusterDirectorEnabled() {
		t.Error("inheritExportToClusterDirector() did not carry forward export_to_cluster_director: true on -w")
	}

	// 2. Explicit false in blueprint or deployment file is respected, not overwritten.
	explicitFalseBp := testBlueprint("us-central1-a")
	explicitFalseBp.SetExportToClusterDirector(false)
	inheritExportToClusterDirector(deployCmd, &explicitFalseBp)
	if explicitFalseBp.ExportToClusterDirector == nil || *explicitFalseBp.ExportToClusterDirector {
		t.Error("inheritExportToClusterDirector() overwrote an explicit export_to_cluster_director: false")
	}

	// 3. `gcluster expand` stays stateless and does not inherit from local folders.
	expandBp := testBlueprint("us-central1-a")
	expandBp.ExportToClusterDirector = nil
	inheritExportToClusterDirector(&cobra.Command{Use: "expand"}, &expandBp)
	if expandBp.ExportToClusterDirector != nil {
		t.Error("inheritExportToClusterDirector() mutated `gcluster expand` blueprint")
	}

	// 4. ApplyOverrideToArtifacts updates the persisted value in place.
	if err := cdapi.ApplyOverrideToArtifacts(artDir, false); err != nil {
		t.Fatalf("ApplyOverrideToArtifacts(false) failed: %v", err)
	}
	afterOptOut := testBlueprint("us-central1-a")
	afterOptOut.ExportToClusterDirector = nil
	inheritExportToClusterDirector(deployCmd, &afterOptOut)
	if afterOptOut.ExportToClusterDirector == nil || *afterOptOut.ExportToClusterDirector {
		t.Error("expected inherited setting after ApplyOverrideToArtifacts(false) to be false")
	}
}
