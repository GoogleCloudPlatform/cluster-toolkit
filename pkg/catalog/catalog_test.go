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
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

var (
	cachedScanOnce       sync.Once
	cachedRoot           string
	cachedBlueprints     []BlueprintEntry
	cachedBlueprintsErr  error
	cachedModuleWiring   map[string]ModuleWiringEntry
	cachedModuleWiringEr error
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

func cachedScan(t *testing.T) (string, []BlueprintEntry) {
	t.Helper()
	root := testRepoRoot(t)
	cachedScanOnce.Do(func() {
		cachedRoot = root
		descs := parseReadmeDescriptions(filepath.Join(root, "examples", "README.md"))
		cachedBlueprints, cachedBlueprintsErr = ScanBlueprints(root, descs)
	})
	if cachedBlueprintsErr != nil {
		t.Fatalf("ScanBlueprints failed: %v", cachedBlueprintsErr)
	}
	return cachedRoot, cachedBlueprints
}

func TestScanBlueprintsAndPairing(t *testing.T) {
	root, blueprints := cachedScan(t)
	if len(blueprints) < 40 {
		t.Fatalf("expected at least 40 blueprints across examples/ and community/examples/, got %d", len(blueprints))
	}

	byPath := indexBlueprintsByPath(t, blueprints)
	verifyCoreBlueprints(t, byPath)
	verifyCommunityAndSpecialBlueprints(t, byPath)
	verifyAllDeploymentFilesPaired(t, root, blueprints)
}

func indexBlueprintsByPath(t *testing.T, blueprints []BlueprintEntry) map[string]BlueprintEntry {
	t.Helper()
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
	return byPath
}

func verifyCoreBlueprints(t *testing.T, byPath map[string]BlueprintEntry) {
	t.Helper()
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

	a3High, ok := byPath["examples/machine-learning/a3-highgpu-8g/a3high-slurm-blueprint.yaml"]
	if !ok || a3High.DeploymentPath != "examples/machine-learning/a3-highgpu-8g/a3high-slurm-deployment.yaml" {
		t.Errorf("expected a3high-slurm-blueprint paired with a3high-slurm-deployment.yaml, got %q", a3High.DeploymentPath)
	}
	if _, hasVar := a3High.Vars["a3_static_cluster_size"]; !hasVar {
		t.Errorf("expected a3_static_cluster_size from deployment file in a3high-slurm-blueprint vars")
	}
	verifyExpressionVarDefaults(t, byPath)
	verifyDeploymentPairings(t, byPath)
}

// verifyDeploymentPairings pins the expected deployment_path for every
// pairing shape present in the repository so the matcher cannot regress.
func verifyDeploymentPairings(t *testing.T, byPath map[string]BlueprintEntry) {
	t.Helper()
	for bp, want := range map[string]string{
		// exact: <stem>-deployment.yaml
		"examples/machine-learning/a3-ultragpu-8g/a3ultra-vm.yaml": "examples/machine-learning/a3-ultragpu-8g/a3ultra-vm-deployment.yaml",
		// prefix: -blueprint / -advanced / -infra / -experiment suffixes
		"examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml":            "examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-deployment.yaml",
		"examples/machine-learning/a3-megagpu-8g/a3mega-slurm-gcsfuse-lssd-blueprint.yaml": "examples/machine-learning/a3-megagpu-8g/a3mega-slurm-deployment.yaml",
		"examples/gke-tpu-7x/gke-tpu-7x-advanced.yaml":                                     "examples/gke-tpu-7x/gke-tpu-7x-deployment.yaml",
		"community/examples/alphaevolve/alpha-evolve-infra.yaml":                           "community/examples/alphaevolve/alpha-evolve-deployment.yaml",
		"community/examples/alphaevolve/alpha-evolve-experiment.yaml":                      "community/examples/alphaevolve/alpha-evolve-deployment.yaml",
		// two blueprints + two deployment files in one folder must not cross-pair
		"examples/machine-learning/a4x-highgpu-4g/a4x-vm.yaml":                  "examples/machine-learning/a4x-highgpu-4g/a4x-vm-deployment.yaml",
		"examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-blueprint.yaml": "examples/machine-learning/a4x-highgpu-4g/a4xhigh-slurm-deployment.yaml",
		// root-level blueprints have no companion
		"examples/hpc-slurm.yaml": "",
	} {
		entry, ok := byPath[bp]
		if !ok {
			t.Errorf("expected %s in catalog", bp)
			continue
		}
		if entry.DeploymentPath != want {
			t.Errorf("%s: deployment_path = %q, want %q", bp, entry.DeploymentPath, want)
		}
	}
}

func TestIsExampleSubfolder(t *testing.T) {
	root := "/repo"
	for dir, want := range map[string]bool{
		"/repo/examples/gke-a4":                true,
		"/repo/community/examples/alphaevolve": true,
		"/repo/examples":                       false,
		"/repo/community/examples":             false,
		"/repo":                                false,
		"/repo/tools/catalog-gen":              false,
		"/repo/examples-archive/foo":           false,
		"/repo/community/examples-old/foo":     false,
		"/elsewhere/examples/foo":              false,
	} {
		if got := isExampleSubfolder(root, dir); got != want {
			t.Errorf("isExampleSubfolder(%q, %q) = %v, want %v", root, dir, got, want)
		}
	}
}

func TestMatchDeploymentFile(t *testing.T) {
	cases := []struct {
		name         string
		base, bpName string
		candidates   []string
		inSubfolder  bool
		want         string
	}{
		{"exact stem", "gke-a4.yaml", "gke-a4", []string{"gke-a4-deployment.yaml"}, true, "gke-a4-deployment.yaml"},
		{"exact blueprint_name", "cluster.yaml", "my-cluster", []string{"my-cluster-deployment.yaml"}, true, "my-cluster-deployment.yaml"},
		{"exact beats prefix", "a4x-vm.yaml", "", []string{"a4x-deployment.yaml", "a4x-vm-deployment.yaml"}, true, "a4x-vm-deployment.yaml"},
		{"prefix -blueprint", "a3ultra-slurm-blueprint.yaml", "", []string{"a3ultra-slurm-deployment.yaml", "a3ultra-vm-deployment.yaml"}, true, "a3ultra-slurm-deployment.yaml"},
		{"prefix -advanced", "gke-tpu-7x-advanced.yaml", "", []string{"gke-tpu-7x-deployment.yaml"}, true, "gke-tpu-7x-deployment.yaml"},
		{"prefix arbitrary suffix", "foo-anything-else.yaml", "", []string{"foo-deployment.yaml"}, true, "foo-deployment.yaml"},
		{"longest prefix wins", "a3-slurm-gcsfuse-blueprint.yaml", "", []string{"a3-deployment.yaml", "a3-slurm-deployment.yaml"}, true, "a3-slurm-deployment.yaml"},
		{"reverse prefix", "foo.yaml", "", []string{"foo-bar-deployment.yaml"}, true, "foo-bar-deployment.yaml"},
		{"no partial-word prefix", "gke-a4x.yaml", "", []string{"gke-a4-deployment.yaml"}, false, ""},
		{"single file fallback in subfolder", "cluster.yaml", "", []string{"infra-deployment.yaml"}, true, "infra-deployment.yaml"},
		{"bare deployment.yaml fallback", "cluster.yaml", "", []string{"deployment.yaml"}, true, "deployment.yaml"},
		{"no fallback at examples root", "cluster.yaml", "", []string{"infra-deployment.yaml"}, false, ""},
		{"ambiguous without prefix stays unpaired", "cluster.yaml", "", []string{"a-deployment.yaml", "b-deployment.yaml"}, true, ""},
		{"empty candidates", "cluster.yaml", "cluster", nil, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchDeploymentFile(tc.base, tc.bpName, tc.candidates, tc.inSubfolder); got != tc.want {
				t.Errorf("matchDeploymentFile(%q, %q, %v, %v) = %q, want %q", tc.base, tc.bpName, tc.candidates, tc.inSubfolder, got, tc.want)
			}
		})
	}
}

func verifyExpressionVarDefaults(t *testing.T, byPath map[string]BlueprintEntry) {
	t.Helper()
	gkeA4 := byPath["examples/gke-a4/gke-a4.yaml"]
	if got := gkeA4.Vars["kueue_configuration_path"]; got != `$(ghpc_stage("./kueue-configuration.yaml.tftpl"))` {
		t.Errorf("expected gke-a4 kueue_configuration_path expression preserved, got %v", got)
	}
	gkeA4x := byPath["examples/gke-a4x/gke-a4x.yaml"]
	if got := gkeA4x.Vars["reservation_project_id"]; got != "$(vars.project_id)" {
		t.Errorf("expected gke-a4x reservation_project_id=$(vars.project_id), got %v", got)
	}
	tpuV6e := byPath["examples/gke-tpu-v6e/gke-tpu-v6e.yaml"]
	if got, exists := tpuV6e.Vars["authorized_cidr"]; !exists || got != nil {
		t.Errorf("expected gke-tpu-v6e authorized_cidr=nil for $(null), got %v (exists=%v)", got, exists)
	}
}

func verifyAllDeploymentFilesPaired(t *testing.T, root string, blueprints []BlueprintEntry) {
	t.Helper()
	paired := make(map[string]bool, len(blueprints))
	for _, bp := range blueprints {
		if bp.DeploymentPath != "" {
			paired[bp.DeploymentPath] = true
		}
	}
	roots := []string{"examples", filepath.Join("community", "examples")}
	tracked := gitTrackedFiles(root, roots...)
	for _, relRoot := range roots {
		_ = filepath.WalkDir(filepath.Join(root, relRoot), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), "-deployment.yaml") || !isTracked(root, path, tracked) {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			relSlash := filepath.ToSlash(rel)
			if !paired[relSlash] {
				t.Errorf("deployment file %s is not paired with any blueprint in catalog", relSlash)
			}
			return nil
		})
	}
}

func verifyCommunityAndSpecialBlueprints(t *testing.T, byPath map[string]BlueprintEntry) {
	t.Helper()
	for path, wantSched := range map[string]string{
		"community/examples/hpc-slinky/hpc-slinky.yaml":       "slurm-on-gke (slinky)",
		"community/examples/slurm-gke/slurm-gke.yaml":         "slurm-on-gke (slinky)",
		"community/examples/flux-framework/flux-cluster.yaml": "flux",
	} {
		if bp, ok := byPath[path]; !ok || bp.Scheduler != wantSched {
			t.Errorf("expected %s scheduler=%s, got %+v", path, wantSched, bp)
		}
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
	verifyMultiBlueprintDescriptions(t, byPath)
}

func verifyMultiBlueprintDescriptions(t *testing.T, byPath map[string]BlueprintEntry) {
	t.Helper()
	a3uVM := byPath["examples/machine-learning/a3-ultragpu-8g/a3ultra-vm.yaml"]
	if !strings.Contains(a3uVM.Description, "standalone VM blueprint (a3ultra-vm.yaml)") {
		t.Errorf("expected standalone VM description for a3ultra-vm.yaml, got %q", a3uVM.Description)
	}
	a4hVM := byPath["examples/machine-learning/a4-highgpu-8g/a4high-vm.yaml"]
	if !strings.Contains(a4hVM.Description, "standalone VM blueprint (a4high-vm.yaml)") {
		t.Errorf("expected standalone VM description for a4high-vm.yaml, got %q", a4hVM.Description)
	}
	a4xVM := byPath["examples/machine-learning/a4x-highgpu-4g/a4x-vm.yaml"]
	if !strings.Contains(a4xVM.Description, "standalone group") {
		t.Errorf("expected standalone group description for a4x-vm.yaml, got %q", a4xVM.Description)
	}
	a3mGcs := byPath["examples/machine-learning/a3-megagpu-8g/a3mega-slurm-gcsfuse-lssd-blueprint.yaml"]
	if !strings.Contains(a3mGcs.Description, "GCSFuse with Local SSD Cache") {
		t.Errorf("expected GCSFuse with Local SSD Cache description for a3mega-slurm-gcsfuse-lssd-blueprint.yaml, got %q", a3mGcs.Description)
	}
	for bpPath := range byPath {
		if strings.HasPrefix(bpPath, "examples/gke-consumption-options/") {
			desc := byPath[bpPath].Description
			if !strings.Contains(desc, "DWS") && !strings.Contains(desc, "Dynamic Workload Scheduler") {
				t.Errorf("expected DWS description for %s, got %q", bpPath, desc)
			}
		}
	}
}

func TestSkipAIAssistedAndYAMLQuoting(t *testing.T) {
	root := testRepoRoot(t)
	tmpDir := t.TempDir()
	aiBP := filepath.Join(tmpDir, "custom.yaml")
	content := "blueprint_name: custom\nai_assisted: true\ndeployment_groups:\n- group: primary\n  modules:\n  - id: net\n    source: modules/network/vpc\n"
	if err := os.WriteFile(aiBP, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp blueprint: %v", err)
	}
	if _, ok := parseBlueprintEntry(root, aiBP, nil, nil); ok {
		t.Errorf("expected blueprint with ai_assisted: true to be skipped by parseBlueprintEntry")
	}
	for _, s := range []string{"line1\nline2", "col1\tcol2", "cr\rreturn"} {
		if !needsQuotedYAMLString(s) {
			t.Errorf("expected needsQuotedYAMLString(%q) to be true", s)
		}
	}
	untrackedDeploy := filepath.Join(root, "examples", "hpc-slurm-deployment.yaml")
	if err := os.WriteFile(untrackedDeploy, []byte("vars:\n  project_id: leaked-project\n"), 0644); err != nil {
		t.Fatalf("failed to write untracked deployment file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(untrackedDeploy) })

	blueprints, err := ScanBlueprints(root, nil)
	if err != nil {
		t.Fatalf("ScanBlueprints failed with untracked file present: %v", err)
	}
	verifyAllDeploymentFilesPaired(t, root, blueprints)
	byPath := indexBlueprintsByPath(t, blueprints)
	if slurm := byPath["examples/hpc-slurm.yaml"]; slurm.DeploymentPath != "" || slurm.Vars["project_id"] != nil {
		t.Errorf("untracked deployment file leaked into hpc-slurm catalog entry: deployment_path=%q project_id=%v", slurm.DeploymentPath, slurm.Vars["project_id"])
	}
}

var cachedWiringOnce sync.Once

func TestScanModuleWiringAndEchoSuppression(t *testing.T) {
	root, blueprints := cachedScan(t)
	cachedWiringOnce.Do(func() {
		cachedModuleWiring, cachedModuleWiringEr = ScanModuleWiring(root, blueprints)
	})
	if cachedModuleWiringEr != nil {
		t.Fatalf("ScanModuleWiring failed: %v", cachedModuleWiringEr)
	}

	verifyLustreWiring(t, cachedModuleWiring)
	verifyEchoAndInjectedIDSuppression(t, cachedModuleWiring)
}

func verifyLustreWiring(t *testing.T, wiring map[string]ModuleWiringEntry) {
	t.Helper()
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
}

func verifyEchoAndInjectedIDSuppression(t *testing.T, wiring map[string]ModuleWiringEntry) {
	t.Helper()
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
	// 4. Root providers with multiple outputs (vpc, pubsub/topic, bigquery-dataset) must retain their outputs.
	bqTable := wiring["community/modules/database/bigquery-table"]
	if !slices.Contains(bqTable.RecommendedUse, "community/modules/database/bigquery-dataset") {
		t.Errorf("expected bigquery-dataset in bigquery-table recommended_use, got %v", bqTable.RecommendedUse)
	}
	packer := wiring["modules/packer/custom-image"]
	if !slices.Contains(packer.RecommendedUse, "modules/network/vpc") {
		t.Errorf("expected modules/network/vpc in custom-image recommended_use, got %v", packer.RecommendedUse)
	}
}

func TestMarshalCatalogDeterminismAndUpdateReadme(t *testing.T) {
	root, blueprints := cachedScan(t)
	cachedWiringOnce.Do(func() {
		cachedModuleWiring, cachedModuleWiringEr = ScanModuleWiring(root, blueprints)
	})
	if cachedModuleWiringEr != nil {
		t.Fatalf("ScanModuleWiring failed: %v", cachedModuleWiringEr)
	}
	cat := Catalog{Blueprints: blueprints, ModuleWiring: cachedModuleWiring}
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
