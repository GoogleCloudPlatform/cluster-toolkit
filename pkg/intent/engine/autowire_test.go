// Copyright 2026 "Google LLC"
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

package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"
)

type mockFS struct {
	fs.FS
}

func (m mockFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(m.FS, name)
}

func (m mockFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(m.FS, name)
}

// withRepoModuleFS points the module reader at the genuine repository sources for
// the duration of a test. It is the in-package twin of compiler_test.go's
// withRealModuleFS, which is not reachable from here because that file is in the
// external `engine_test` package.
//
// The schema cache is cleared on both entry and exit: TestAutowire installs stub
// modules at real repository paths, and modulereader's cache is global, so a stub
// schema would otherwise leak into (or out of) this test.
func withRepoModuleFS(t *testing.T) {
	t.Helper()
	absRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}
	oldFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = mockFS{os.DirFS(absRoot)}
	modulereader.ClearModuleInfoCache()
	t.Cleanup(func() {
		sourcereader.ModuleFS = oldFS
		modulereader.ClearModuleInfoCache()
	})
}

func TestAutowire(t *testing.T) {
	// Create temporary mock modules
	tempDir := t.TempDir()
	oldFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = mockFS{os.DirFS(tempDir)}
	defer func() { sourcereader.ModuleFS = oldFS }()

	// 1. Exported FS Module (outputs: network_storage)
	fsDir := filepath.Join(tempDir, "modules/file-system/managed-lustre")
	if err := os.MkdirAll(fsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fsDir, "outputs.tf"), []byte(`
output "network_storage" {
  value = "mock"
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Consumer Module (inputs: network_storage)
	consumerDir := filepath.Join(tempDir, "modules/scheduler/controller")
	if err := os.MkdirAll(consumerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(consumerDir, "variables.tf"), []byte(`
variable "network_storage" {
  type = list(any)
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Unrelated Module (no inputs matching)
	unrelatedDir := filepath.Join(tempDir, "modules/network/vpc")
	if err := os.MkdirAll(unrelatedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unrelatedDir, "variables.tf"), []byte(`
variable "network_name" {
  type = string
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	groups := []ast.DeploymentGroup{
		{
			Group: "primary",
			Modules: []ast.ModuleSpec{
				{
					ID:     "fs1",
					Source: "modules/file-system/managed-lustre",
					Export: true,
				},
				{
					ID:     "fs2",
					Source: "modules/file-system/managed-lustre",
					Export: true,
				},
				{
					ID:     "controller",
					Source: "modules/scheduler/controller",
					Use:    []string{"existing_dependency"},
				},
				{
					ID:     "vpc",
					Source: "modules/network/vpc",
				},
			},
		},
	}

	groups, err := Autowire(groups, tempDir)
	if err != nil {
		t.Fatalf("Autowire failed: %v", err)
	}

	// Validate Controller was wired to both fs1 and fs2
	controllerMod := groups[0].Modules[2]
	if len(controllerMod.Use) != 3 {
		t.Fatalf("expected controller to have 3 dependencies (1 existing + 2 wired), got %d: %v", len(controllerMod.Use), controllerMod.Use)
	}
	if controllerMod.Use[0] != "existing_dependency" || controllerMod.Use[1] != "fs1" || controllerMod.Use[2] != "fs2" {
		t.Errorf("controller use array incorrect: %v", controllerMod.Use)
	}

	// Validate VPC was NOT wired to fs1 or fs2
	vpcMod := groups[0].Modules[3]
	if len(vpcMod.Use) != 0 {
		t.Errorf("expected vpc to have 0 dependencies, got %v", vpcMod.Use)
	}
}

func TestRealModuleAutowire(t *testing.T) {
	oldFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = mockFS{os.DirFS("../../../")}
	defer func() { sourcereader.ModuleFS = oldFS }()

	groups := []ast.DeploymentGroup{
		{
			Group: "cluster",
			Modules: []ast.ModuleSpec{
				{
					ID:     "slurm_controller",
					Source: "community/modules/scheduler/schedmd-slurm-gcp-v6-controller",
					Use:    []string{"primary_vpc"},
				},
				{
					ID:            "a3u-primary_partition",
					Source:        "community/modules/compute/schedmd-slurm-gcp-v6-partition",
					IsComputePool: true,
				},
			},
		},
	}

	res, err := Autowire(groups, ".")
	if err != nil {
		t.Fatalf("Autowire failed: %v", err)
	}

	t.Logf("Controller use after Autowire: %v", res[0].Modules[0].Use)
}

func TestAutowire_DuplicateLocalMountError(t *testing.T) {
	groups := []ast.DeploymentGroup{
		{
			Group: "primary",
			Modules: []ast.ModuleSpec{
				{
					ID:       "lustre_1",
					Source:   "modules/file-system/managed-lustre",
					Settings: map[string]any{"local_mount": "/mnt/shared"},
				},
				{
					ID:       "filestore_1",
					Source:   "modules/file-system/filestore",
					Settings: map[string]any{"local_mount": "/mnt/shared"},
				},
			},
		},
	}

	_, err := Autowire(groups, ".")
	if err == nil {
		t.Fatalf("expected error for duplicate local_mount path '/mnt/shared', got nil")
	}
}

// Neither module sets local_mount, so the collision exists only in the
// variables.tf defaults: managed-lustre and filestore both default to
// "/shared". The pre-default-resolution check saw two empty settings maps and
// passed, letting the second mount clobber the first at runtime.
func TestAutowire_DuplicateLocalMountFromModuleDefault(t *testing.T) {
	withRepoModuleFS(t) // resolving defaults requires the genuine variables.tf
	groups := []ast.DeploymentGroup{
		{
			Group: "primary",
			Modules: []ast.ModuleSpec{
				{ID: "lustre_1", Source: "modules/file-system/managed-lustre"},
				{ID: "filestore_1", Source: "modules/file-system/filestore"},
			},
		},
	}

	_, err := Autowire(groups, ".")
	if err == nil {
		t.Fatalf("expected error for local_mount '/shared' inherited by both modules, got nil")
	}
	if !strings.Contains(err.Error(), "module default") {
		t.Errorf("error should identify the mount as a module default so the user knows where to look; got: %v", err)
	}
}

// An explicit override on one side resolves the default collision above, and a
// module with no local_mount input at all must not be dragged into the check.
func TestAutowire_DistinctLocalMountsAllowed(t *testing.T) {
	withRepoModuleFS(t)
	groups := []ast.DeploymentGroup{
		{
			Group: "primary",
			Modules: []ast.ModuleSpec{
				{
					ID:       "lustre_1",
					Source:   "modules/file-system/managed-lustre",
					Settings: map[string]any{"local_mount": "/lustre"},
				},
				{ID: "filestore_1", Source: "modules/file-system/filestore"},
				{ID: "net", Source: "modules/network/vpc"},
			},
		},
	}

	if _, err := Autowire(groups, "."); err != nil {
		t.Fatalf("distinct mount paths should compile, got: %v", err)
	}
}

func TestCatalogLoader_AllArchetypesAndOverlays(t *testing.T) {
	loader := NewCatalogLoader("../../../")

	// 1. Check all 4 Config Bases
	// NOTE: no "packer" here. config-base/packer.yaml was deleted: it declared only a VPC
	// and no compute archetype ever exposed a `packer:` block, so `config_base: packer`
	// could never compile. Re-add it alongside a real image-builder archetype, not before.
	expectedBases := []string{"gke", "slurm", "jbvm"}
	for _, b := range expectedBases {
		if _, err := loader.LoadConfigBase(b); err != nil {
			t.Errorf("failed to load config_base %q: %v", b, err)
		}
	}

	// 2. Check all 10 Compute Archetypes + Prefix matching
	expectedArchs := map[string]string{
		"cpu":              "cpu",
		"c2-standard-60":   "cpu",
		"ct6e-standard-4t": "tpu-v6e",
		"tpu-v6e":          "tpu-v6e",
		"a3-megagpu-8g":    "a3-megagpu-8g",
		"a3-ultragpu-8g":   "a3-ultragpu-8g",
		"a4-highgpu-8g":    "a4-highgpu-8g",
		"a4x-highgpu-4g":   "a4x-highgpu-4g",
	}
	for query, expectedType := range expectedArchs {
		m, err := loader.LoadArchetype(query)
		if err != nil {
			t.Errorf("LoadArchetype(%q) failed: %v", query, err)
			continue
		}
		if m.Name != expectedType {
			t.Errorf("LoadArchetype(%q) resolved to %q, expected %q", query, m.Name, expectedType)
		}
	}

	// 2b. GPU families with no archetype MUST fail to resolve.
	// These previously matched `g2-*` / `a2-*` aliases on g4-standard and silently
	// deployed an RTX PRO 6000 Blackwell configuration onto A100 / L4 hardware:
	// wrong accelerator type, wrong driver installer, wrong network assumptions,
	// with compile, expand and `terraform validate` all passing.
	// A hard resolution failure is the correct behaviour until real archetypes exist.
	unsupportedMachineTypes := []string{
		"a2-highgpu-1g",  // A100 40GB
		"a2-megagpu-16g", // A100 40GB
		"a2-ultragpu-8g", // A100 80GB
		"g2-standard-4",  // L4
		"g2-custom-16-55296",
	}
	for _, query := range unsupportedMachineTypes {
		if m, err := loader.LoadArchetype(query); err == nil {
			t.Errorf("LoadArchetype(%q) unexpectedly resolved to %q; unsupported GPU families must error, not mis-resolve", query, m.Name)
		}
	}

	testCatalogLoaderFeaturesAndOverlays(t, loader)
}

func testCatalogLoaderFeaturesAndOverlays(t *testing.T, loader *CatalogLoader) {
	t.Helper()
	// 3. Features (Tier 4) load from features/
	expectedFeatures := []string{
		// inference-gateway, slinky and chrome-remote-desktop were promoted from overlays/
		// to features/: each owns real wiring (a cluster toggle, the community slinky
		// module, and a dedicated remote-desktop VM respectively), which an overlay is
		// forbidden to carry.
		// pre-existing-network-storage is the escape hatch for storage the user already
		// owns; see escapeHatchFeatures in catalog_coverage_test.go.
		"pre-existing-network-storage",
	}
	for _, name := range expectedFeatures {
		if _, err := loader.LoadFeature(name); err != nil {
			t.Errorf("LoadFeature(%q) failed: %v", name, err)
		}
	}

	// 4. Overlays (Tier 5) live in a SEPARATE namespace and must not resolve as features.
	if _, err := loader.LoadFeature("run-nvidia-smi"); err == nil {
		t.Error("LoadFeature(\"run-nvidia-smi\") unexpectedly succeeded: overlays must not be loadable as features")
	}

	// Every overlay here is carried by a shipped cluster-config that reproduces a real
	// blueprint. Twelve others (spack-gromacs, ramble-hpl, slurm-ior-bench, gpu-healthcheck,
	// alphafold-v3, cae-toolchain, dcgm-healthcheck, ml-diagnostics-job,
	// openfoam-solver-job, slurm-nccl-test, chrome-remote-desktop, container-runtime) were deleted or promoted: no reference blueprint contained
	// them, and the spack/ramble pair could not even be placed correctly, since those
	// references build on the login node or a dedicated builder VM.
	expectedOverlays := []string{
		"run-nvidia-smi", "nccl-jobset-test", "fio-bench-job",
	}
	for _, name := range expectedOverlays {
		if _, err := loader.LoadOverlay(name); err != nil {
			t.Errorf("LoadOverlay(%q) failed: %v", name, err)
		}
	}

	// 5. Silicon enablement must NOT be an overlay: it is owned by the compute archetype,
	//    which already provisions a kubectl-apply manager. Re-adding it as an overlay would
	//    emit a second manager module racing the archetype's against the same cluster.
	for _, name := range []string{"nvidia-dra-driver", "nri-device-injector", "asapd-lite-installer",
		// Deleted in Phase 1; must not silently reappear.
		"spack-gromacs", "ramble-hpl", "slurm-ior-bench", "gpu-healthcheck", "container-runtime",
		// slurm-nccl-test staged an sbatch invoking /opt/nccl-tests/build/all_reduce_perf,
		// which nothing in the catalog builds, onto compute nodes -- where no user ever
		// types `sbatch`. The references that do ship an NCCL benchmark stage it from
		// controller_startup into NFS-exported /opt/apps, a placement Phase 1 overlays
		// cannot express.
		"slurm-nccl-test",
		// chrome-remote-desktop was promoted to a FEATURE. As an overlay it apt-installed
		// XFCE and no Chrome Remote Desktop, onto compute nodes nobody logs into. The
		// reference stands up a dedicated VM via
		// community/modules/remote-desktop/chrome-remote-desktop, which needs a
		// `source:` -- and anything needing a `source:` is a feature.
		"chrome-remote-desktop"} {
		if _, err := loader.LoadOverlay(name); err == nil {
			t.Errorf("LoadOverlay(%q) unexpectedly succeeded: it is either silicon enablement "+
				"(owned by compute-archetypes) or an entry deleted for having no reference blueprint", name)
		}
	}
}

// TestEveryOverlayNamesAnOverlayOnlyFeature guarantees every shipped overlay is a valid thin
// payload: it must name a feature via `feature`, that feature must exist, must be marked
// `requires_overlay: true`, and the overlay must carry no module wiring of its own.
func TestEveryOverlayNamesAnOverlayOnlyFeature(t *testing.T) {
	loader := NewCatalogLoader("../../../")

	entries, err := os.ReadDir(filepath.Join("..", "..", "..", "v2", "overlays"))
	if err != nil {
		t.Fatalf("failed to read v2/overlays dir: %v", err)
	}

	seen := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		seen++
		name := strings.TrimSuffix(e.Name(), ".yaml")
		overlay, err := loader.LoadOverlay(name)
		if err != nil {
			t.Errorf("overlay %q failed to load: %v", name, err)
			continue
		}
		if len(overlay.Features) == 0 {
			t.Errorf("overlay %q declares no `features`", name)
			continue
		}
		for _, ovFeat := range overlay.Features {
			verifyOverlayFeatureRef(t, loader, name, ovFeat)
		}
	}
	if seen == 0 {
		t.Fatal("no overlay manifests discovered")
	}
}

func verifyOverlayFeatureRef(t *testing.T, loader *CatalogLoader, name string, ovFeat ast.FeatureReference) {
	t.Helper()
	featType := ovFeat.Type
	if featType == "" {
		featType = ovFeat.Name
	}
	feature, err := loader.LoadFeature(featType)
	if err != nil {
		t.Errorf("overlay %q names non-existent feature %q", name, featType)
		return
	}
	// A payload-bearing overlay sits on a parameterized feature template (e.g. gke-job-template)
	// that enforces required payload keys via CEL assertions.
	hasAssertions := len(feature.Assertions) > 0
	for _, cb := range feature.ConfigBase {
		if len(cb.Assertions) > 0 {
			hasAssertions = true
		}
	}
	if !hasAssertions {
		t.Errorf("overlay %q names feature %q which has no CEL assertions guarding its required template settings", name, featType)
	}
	if len(ovFeat.Settings) == 0 {
		t.Errorf("overlay %q feature %q carries no settings - it specialises nothing", name, featType)
	}
}
