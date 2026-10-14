// Copyright 2026 Google LLC
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

package catalog

import (
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func testRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}
	sourcereader.ModuleFS = os.DirFS(root).(sourcereader.BaseFS)
	return root
}

func TestScanBlueprintsAndPairing(t *testing.T) {
	root := testRepoRoot(t)
	descs := parseReadmeDescriptions(filepath.Join(root, "examples", "README.md"))
	blueprints, err := ScanBlueprints(root, descs)
	if err != nil {
		t.Fatalf("ScanBlueprints failed: %v", err)
	}
	if len(blueprints) < 40 {
		t.Fatalf("expected at least 40 blueprints across examples/ and community/examples/, got %d", len(blueprints))
	}

	byPath := make(map[string]BlueprintEntry, len(blueprints))
	for _, bp := range blueprints {
		byPath[bp.Path] = bp
		if strings.HasSuffix(bp.Path, "-deployment.yaml") {
			t.Errorf("companion deployment file %s must not be listed as a standalone blueprint", bp.Path)
		}
		if strings.Contains(bp.Path, "nccl-installer") || strings.Contains(bp.Path, "job.yaml.tftpl") {
			t.Errorf("non-blueprint manifest %s must be skipped", bp.Path)
		}
	}

	slurm, ok := byPath["examples/hpc-slurm.yaml"]
	if !ok {
		t.Fatalf("expected examples/hpc-slurm.yaml in catalog")
	}
	if slurm.Scheduler != "slurm" {
		t.Errorf("expected hpc-slurm scheduler=slurm, got %q", slurm.Scheduler)
	}
	if !slices.Contains(slurm.MachineTypes, "c2-standard-60") {
		t.Errorf("expected c2-standard-60 in hpc-slurm machine_types, got %v", slurm.MachineTypes)
	}

	gkeA4, ok := byPath["examples/gke-a4/gke-a4.yaml"]
	if !ok {
		t.Fatalf("expected examples/gke-a4/gke-a4.yaml in catalog")
	}
	if gkeA4.DeploymentPath != "examples/gke-a4/gke-a4-deployment.yaml" {
		t.Errorf("expected gke-a4 deployment_path=examples/gke-a4/gke-a4-deployment.yaml, got %q", gkeA4.DeploymentPath)
	}
	if gkeA4.Scheduler != "gke" {
		t.Errorf("expected gke-a4 scheduler=gke, got %q", gkeA4.Scheduler)
	}

	slinky, ok := byPath["community/examples/hpc-slinky/hpc-slinky.yaml"]
	if !ok || slinky.Scheduler != "slurm-on-gke (slinky)" {
		t.Errorf("expected hpc-slinky scheduler=slurm-on-gke (slinky), got %+v", slinky)
	}
	batchBP, ok := byPath["community/examples/fsi-montecarlo-on-batch.yaml"]
	if !ok || batchBP.Scheduler != "batch" || !strings.Contains(batchBP.Description, "Value at Risk") {
		t.Errorf("expected fsi-montecarlo-on-batch scheduler=batch and Value at Risk description, got %+v", batchBP)
	}
	af3, ok := byPath["examples/science/af3-slurm/af3-slurm.yaml"]
	if !ok || !strings.Contains(af3.Description, "AlphaFold 3") {
		t.Errorf("expected af3-slurm description to preserve inline link text 'AlphaFold 3', got %q", af3.Description)
	}
	a3mImg, ok := byPath["examples/machine-learning/build-service-images/a3m/blueprint.yaml"]
	if !ok || strings.Contains(strings.ToLower(a3mImg.Description), "you may not use") {
		t.Errorf("expected license text to be stripped from description, got %q", a3mImg.Description)
	}
}

func TestScanModuleWiringAndEchoSuppression(t *testing.T) {
	root := testRepoRoot(t)
	descs := parseReadmeDescriptions(filepath.Join(root, "examples", "README.md"))
	blueprints, err := ScanBlueprints(root, descs)
	if err != nil {
		t.Fatalf("ScanBlueprints failed: %v", err)
	}
	wiring, err := ScanModuleWiring(root, blueprints)
	if err != nil {
		t.Fatalf("ScanModuleWiring failed: %v", err)
	}

	lustre, ok := wiring["modules/file-system/managed-lustre"]
	if !ok {
		t.Fatalf("expected modules/file-system/managed-lustre in module_wiring")
	}
	for _, globalVar := range []string{"project_id", "deployment_name", "region", "zone", "labels"} {
		if slices.Contains(lustre.RequiredInputs, globalVar) {
			t.Errorf("auto-injected global %q must not appear in required_inputs", globalVar)
		}
	}
	if !slices.Contains(lustre.RecommendedUse, "modules/network/vpc") || !slices.Contains(lustre.RecommendedUse, "modules/network/private-service-access") {
		t.Errorf("expected vpc and private-service-access in managed-lustre recommended_use, got %v", lustre.RecommendedUse)
	}
	if slices.Contains(lustre.RecommendedUse, "modules/compute/vm-instance") {
		t.Errorf("generic 'name' output on vm-instance must not pollute managed-lustre recommended_use: %v", lustre.RecommendedUse)
	}
	if !slices.Contains(lustre.ConsumedBy, "modules/file-system/gke-persistent-volume") || !slices.Contains(lustre.ConsumedBy, "community/modules/scheduler/schedmd-slurm-gcp-v6-controller") {
		t.Errorf("expected gke-persistent-volume and schedmd-slurm-gcp-v6-controller in managed-lustre consumed_by, got %v", lustre.ConsumedBy)
	}

	// Verify echo variable suppression:
	// 1. gke-node-pool echoes cluster_id and machine_type; it must not recommend itself or act as cluster_id provider for gke-persistent-volume.
	pvc := wiring["modules/file-system/gke-persistent-volume"]
	if slices.Contains(pvc.RecommendedUse, "modules/compute/gke-node-pool") {
		t.Errorf("echo output cluster_id on gke-node-pool must be suppressed from gke-persistent-volume recommended_use: %v", pvc.RecommendedUse)
	}
	// 2. spanner echoes instance_name; it must not recommend itself or chrome-remote-desktop.
	spanner := wiring["modules/database/spanner"]
	if slices.Contains(spanner.RecommendedUse, "modules/database/spanner") || slices.Contains(spanner.RecommendedUse, "community/modules/remote-desktop/chrome-remote-desktop") {
		t.Errorf("spanner must not recommend itself or chrome-remote-desktop via instance_name: %v", spanner.RecommendedUse)
	}
	// 3. Auto-injected module IDs (inject_module_id) must be excluded from required_inputs.
	np := wiring["modules/compute/gke-node-pool"]
	if slices.Contains(np.RequiredInputs, "internal_ghpc_module_id") {
		t.Errorf("internal_ghpc_module_id must be excluded from gke-node-pool required_inputs: %v", np.RequiredInputs)
	}
	gcloudMod := wiring["community/modules/gcloud/gcloud"]
	if slices.Contains(gcloudMod.RequiredInputs, "module_instance_id") {
		t.Errorf("module_instance_id must be excluded from gcloud required_inputs: %v", gcloudMod.RequiredInputs)
	}
	// 4. bigquery-table must recommend bigquery-dataset.
	bqTable := wiring["community/modules/database/bigquery-table"]
	if !slices.Contains(bqTable.RecommendedUse, "community/modules/database/bigquery-dataset") {
		t.Errorf("expected bigquery-dataset in bigquery-table recommended_use, got %v", bqTable.RecommendedUse)
	}
}

func TestMarshalCatalogDeterminismAndUpdateReadme(t *testing.T) {
	root := testRepoRoot(t)
	cat, err := BuildCatalog(root)
	if err != nil {
		t.Fatalf("BuildCatalog failed: %v", err)
	}
	b1, err := MarshalCatalog(cat)
	if err != nil {
		t.Fatalf("MarshalCatalog 1 failed: %v", err)
	}
	b2, err := MarshalCatalog(cat)
	if err != nil {
		t.Fatalf("MarshalCatalog 2 failed: %v", err)
	}
	if string(b1) != string(b2) {
		t.Errorf("MarshalCatalog is not deterministic across runs")
	}

	tmpDir := t.TempDir()
	readmeTmp := filepath.Join(tmpDir, "README.md")
	initial := "# Heading\n\nIntro text.\n\n" + BeginCatalogMarker + "\nold content\n" + EndCatalogMarker + "\n\n### [hpc-slurm.yaml]\nAfter text.\n"
	if err := os.WriteFile(readmeTmp, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write temp README: %v", err)
	}
	if err := UpdateExamplesReadme(readmeTmp, cat.Blueprints[:2]); err != nil {
		t.Fatalf("UpdateExamplesReadme failed: %v", err)
	}
	updatedBytes, err := os.ReadFile(readmeTmp)
	if err != nil {
		t.Fatalf("failed to read updated README: %v", err)
	}
	updated := string(updatedBytes)
	if !strings.Contains(updated, "Intro text.") || !strings.Contains(updated, "### [hpc-slurm.yaml]\nAfter text.") {
		t.Errorf("surrounding prose in README was modified: %s", updated)
	}
	if !strings.Contains(updated, "| Blueprint | Scheduler | Machine Types | Description |") {
		t.Errorf("expected markdown table in updated README, got: %s", updated)
	}
}
