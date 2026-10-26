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
	"bytes"
	"encoding/json"
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"
)

func setupExplainTestFS(c *C) (string, func()) {
	wd, err := os.Getwd()
	c.Assert(err, IsNil)

	_, thisFile, _, ok := runtime.Caller(0)
	c.Assert(ok, Equals, true)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))

	oldFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = os.DirFS(repoRoot).(sourcereader.BaseFS)
	c.Assert(os.Chdir(repoRoot), IsNil)

	cleanup := func() {
		sourcereader.ModuleFS = oldFS
		explainFlags.format = "text"
		explainFlags.deploymentFile = ""
		_ = os.Chdir(wd)
	}
	return repoRoot, cleanup
}

func runExplainForTest(args ...string) (string, error) {
	var buf bytes.Buffer
	explainCmd.SetOut(&buf)
	explainCmd.SetErr(&buf)
	defer func() {
		explainCmd.SetOut(nil)
		explainCmd.SetErr(nil)
	}()
	err := runExplainCmd(explainCmd, args)
	return buf.String(), err
}

func (s *MySuite) TestExplainZeroArgsListsBlueprintsAndModules(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	out, err := runExplainForTest()
	c.Assert(err, IsNil)
	c.Check(strings.Contains(out, "Available Blueprints"), Equals, true)
	c.Check(strings.Contains(out, "examples/hpc-slurm.yaml"), Equals, true)
	c.Check(strings.Contains(out, "examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml"), Equals, true)
	c.Check(strings.Contains(out, "Available Embedded Modules:"), Equals, true)
	c.Check(strings.Contains(out, "modules/file-system/managed-lustre"), Equals, true)
	c.Check(strings.Contains(out, "modules/network/vpc"), Equals, true)
	// Internal modules must never be exposed
	c.Check(strings.Contains(out, "community/modules/internal"), Equals, false)
	c.Check(strings.Contains(out, "modules/slurm_files"), Equals, false)
}

func (s *MySuite) TestExplainBlueprintAndDeploymentPairing(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	out, err := runExplainForTest("examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(out, "Blueprint: examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml"), Equals, true)
	c.Check(strings.Contains(out, "Companion Deployment File: examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-deployment.yaml"), Equals, true)
	c.Check(strings.Contains(out, "gcloud services enable"), Equals, true)
	c.Check(strings.Contains(out, "lustre.googleapis.com"), Equals, true)
	c.Check(strings.Contains(out, "a3u_cluster_size"), Equals, true)
	c.Check(strings.Contains(out, "Stage 1 [cluster-env]:"), Equals, true)
	c.Check(strings.Contains(out, "Stage 2 [cluster]:"), Equals, true)
	c.Check(strings.Contains(out, "./gcluster deploy -d examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-deployment.yaml examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml"), Equals, true)

	// Passing the companion *-deployment.yaml directly should resolve to its sibling blueprint
	outDepl, err := runExplainForTest("examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-deployment.yaml")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outDepl, "Blueprint: examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml"), Equals, true)
}

func (s *MySuite) TestExplainModuleAndPathNormalization(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	// 1. Canonical embedded module path
	out, err := runExplainForTest("modules/file-system/managed-lustre")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(out, "Module: modules/file-system/managed-lustre (terraform)"), Equals, true)
	c.Check(strings.Contains(out, "lustre.googleapis.com"), Equals, true)
	c.Check(strings.Contains(out, "use: [vpc, private_service_access]"), Equals, true)
	c.Check(strings.Contains(out, "size_gib: 36000"), Equals, true)
	c.Check(strings.Contains(out, "Inputs Filled Automatically"), Equals, true)

	// 2. Leading './' and trailing '/' from tab completion must not fail
	outNorm, err := runExplainForTest("./modules/file-system/managed-lustre/")
	c.Assert(err, IsNil)
	c.Check(outNorm, Equals, out)

	// 3. Short module name resolution
	outShort, err := runExplainForTest("managed-lustre")
	c.Assert(err, IsNil)
	c.Check(outShort, Equals, out)

	// 4. Packer module resolution
	outPacker, err := runExplainForTest("modules/packer/custom-image")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outPacker, "Module: modules/packer/custom-image (packer)"), Equals, true)
}

func (s *MySuite) TestExplainSpecMatchingExactAndClosest(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	// 1. Exact blueprint match: slurm + a3u + lustre
	outExact, err := runExplainForTest("slurm", "a3u", "lustre")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outExact, "Exact Blueprint Match (3/3 specs matched: slurm, a3-ultragpu-8g, managed-lustre):"), Equals, true)
	c.Check(strings.Contains(outExact, "Blueprint: examples/machine-learning/a3-ultragpu-8g/a3ultra-slurm-blueprint.yaml"), Equals, true)

	// 2. Closest blueprint match with missing module snippet: gke + a3-megagpu + lustre
	outClosest, err := runExplainForTest("gke", "a3-megagpu", "lustre")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outClosest, "Closest Blueprint Match (2/3 specs matched: gke, a3-megagpu-8g):"), Equals, true)
	c.Check(strings.Contains(outClosest, "Blueprint: examples/gke-a3-megagpu/gke-a3-megagpu.yaml"), Equals, true)
	c.Check(strings.Contains(outClosest, "Missing Spec: lustre (modules/file-system/managed-lustre)"), Equals, true)
	c.Check(strings.Contains(outClosest, "To add missing spec (lustre -> modules/file-system/managed-lustre), insert into group [primary]:"), Equals, true)
	c.Check(strings.Contains(outClosest, "- id: private_service_access"), Equals, true)
	c.Check(strings.Contains(outClosest, "use: [network1]"), Equals, true)
	c.Check(strings.Contains(outClosest, "use: [network1, private_service_access]"), Equals, true)

	// 3. TPU 7x GKE match
	outTPU, err := runExplainForTest("gke", "tpu", "7x")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outTPU, "examples/gke-tpu-7x/gke-tpu-7x.yaml"), Equals, true)
}

func (s *MySuite) TestExplainJSONAndYAMLFormatsAndErrors(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	// JSON format
	explainFlags.format = "json"
	outJSON, err := runExplainForTest("modules/file-system/filestore")
	c.Assert(err, IsNil)
	var modRes ExplainModuleResult
	c.Assert(json.Unmarshal([]byte(outJSON), &modRes), IsNil)
	c.Check(modRes.Module, Equals, "modules/file-system/filestore")
	c.Check(modRes.Kind, Equals, "terraform")

	// YAML format
	explainFlags.format = "yaml"
	outYAML, err := runExplainForTest("examples/hpc-slurm.yaml")
	c.Assert(err, IsNil)
	var bpRes ExplainBlueprintResult
	c.Assert(yaml.Unmarshal([]byte(outYAML), &bpRes), IsNil)
	c.Check(bpRes.Path, Equals, "examples/hpc-slurm.yaml")

	// Invalid format error
	explainFlags.format = "xml"
	_, err = runExplainForTest()
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "invalid --format"), Equals, true)

	// Non-existent path error (must not trigger network download)
	explainFlags.format = "text"
	_, err = runExplainForTest("modules/file-system/nonexistent-module")
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "not found"), Equals, true)

	// Unmatched spec error
	_, err = runExplainForTest("nonexistent_spec_xyz_999")
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "no blueprint matched specs"), Equals, true)
}

func (s *MySuite) TestExplainSpecMatchingPrefersMachineTypeEvidence(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	// B1: 'gke a4x' must match gke-a4x.yaml (a4x-highgpu-4g), not gke-a4x-max-bm.yaml
	outA4X, err := runExplainForTest("gke", "a4x")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outA4X, "Blueprint: examples/gke-a4x/gke-a4x.yaml"), Equals, true)
	c.Check(strings.Contains(outA4X, "other blueprint(s) also matched"), Equals, false)

	// 'gke a4x-max' must match gke-a4x-max-bm.yaml (a4x-maxgpu-4g-metal)
	outA4XMax, err := runExplainForTest("gke", "a4x-max")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outA4XMax, "Blueprint: examples/gke-a4x-max-bm/gke-a4x-max-bm.yaml"), Equals, true)

	// TPU 7x Slurm & TPU v4 GKE exact match
	outSlurmTPU7x, err := runExplainForTest("slurm", "tpu", "7x")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outSlurmTPU7x, "Exact Blueprint Match (3/3 specs matched: slurm, tpu, tpu7x-standard-4t):"), Equals, true)
	c.Check(strings.Contains(outSlurmTPU7x, "Blueprint: examples/machine-learning/tpu7x-standard-4t/tpu7x-slurm-blueprint.yaml"), Equals, true)

	outTPUv4, err := runExplainForTest("gke", "tpu", "v4")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outTPUv4, "Blueprint: examples/gke-tpu-v4/gke-tpu-v4.yaml"), Equals, true)

	// Prefix boundary check: 'c2' must match c2-standard-60, not c2d-standard-112
	c.Check(matchesHardwareMachineType("c2-standard-60", []string{"c2"}), Equals, true)
	c.Check(matchesHardwareMachineType("c2d-standard-112", []string{"c2"}), Equals, false)
	c.Check(matchesHardwareMachineType("n2d-standard-32", []string{"n2"}), Equals, false)
}

func (s *MySuite) TestExplainRejectsMissingDeploymentFileAndBadBlueprint(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	// B2: Explicitly passed -d / --deployment-file that does not exist must fail fast
	explainFlags.deploymentFile = "nonexistent-deployment-file.yaml"
	_, err := runExplainForTest("examples/hpc-slurm.yaml")
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "failed to load deployment file"), Equals, true)

	// N3: -d / --deployment-file outside blueprint mode must be rejected
	_, err = runExplainForTest("slurm", "a3u")
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "--deployment-file is only valid"), Equals, true)
	_, err = runExplainForTest("modules/file-system/filestore")
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "--deployment-file is only valid"), Equals, true)
	explainFlags.deploymentFile = ""

	// N1: Existing .yaml file with invalid blueprint syntax or no deployment_groups must report parse error
	tmpDir := c.MkDir()
	badBP := filepath.Join(tmpDir, "broken-blueprint.yaml")
	c.Assert(os.WriteFile(badBP, []byte("blueprint_name: broken\nvars:\n  project_id: test\n"), 0644), IsNil)
	_, err = runExplainForTest(badBP)
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "not found"), Equals, false)
}

func (s *MySuite) TestExplainHardcodedModuleSourcesExist(c *C) {
	_, cleanup := setupExplainTestFS(c)
	defer cleanup()

	for _, def := range knownSpecAliasTable {
		if def.ModuleSource != "" {
			c.Check(detectEmbeddedModuleKind(sourcereader.ModuleFS, def.ModuleSource), Not(Equals), "",
				Commentf("knownSpecAliasTable[%s] -> %s", def.Canonical, def.ModuleSource))
		}
	}
	for _, src := range preferredProviderOrder {
		c.Check(detectEmbeddedModuleKind(sourcereader.ModuleFS, src), Not(Equals), "",
			Commentf("preferredProviderOrder -> %s", src))
	}

	// Verify synonym deduplication (N5) and reference blueprints (B2)
	specs := parseSpecTokens([]string{"slurm", "a3u", "a3ultra", "h200"})
	c.Check(len(specs), Equals, 2)

	outGkeLustre, err := runExplainForTest("gke", "a3-megagpu", "lustre")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outGkeLustre, "examples/gke-managed-lustre.yaml"), Equals, true)

	// Directory with 1 blueprint resolves automatically; directory with >1 blueprint lists choices
	outDir, err := runExplainForTest("examples/gke-a3-ultragpu/")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outDir, "examples/gke-a3-ultragpu/gke-a3-ultragpu.yaml"), Equals, true)

	_, err = runExplainForTest("examples/machine-learning/a3-ultragpu-8g/")
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "contains multiple blueprints"), Equals, true)
	c.Check(strings.Contains(err.Error(), "a3ultra-slurm-blueprint.yaml"), Equals, true)

	// Verify 'a4h' hardware token matches gke-a4.yaml and multi-stage targetGroup picks [setup] over [enable_apis]
	outGkeA4, err := runExplainForTest("gke", "a4h")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outGkeA4, "Exact Blueprint Match"), Equals, true)
	c.Check(strings.Contains(outGkeA4, "examples/gke-a4/gke-a4.yaml"), Equals, true)

	outHclsPstore, err := runExplainForTest("slurm", "c2", "parallelstore")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outHclsPstore, "insert into group [setup]:"), Equals, true)
	c.Check(strings.Contains(outHclsPstore, "use: [network, private_service_access]"), Equals, true)
}

func (s *MySuite) TestExplainVarsMirrorDeploymentMergeSemantics(c *C) {
	repoRoot, cleanup := setupExplainTestFS(c)
	defer cleanup()

	dir := c.MkDir()
	bp := filepath.Join(dir, "t-blueprint.yaml")
	c.Assert(os.WriteFile(bp, []byte(`blueprint_name: t
vars:
  project_id: p
  deployment_name: from-blueprint
  zone: us-central1-a
  optional_thing: $(null)
deployment_groups:
- group: primary
  modules:
  - id: network
    source: modules/network/vpc
`), 0644), IsNil)
	c.Assert(os.WriteFile(filepath.Join(dir, "t-deployment.yaml"), []byte(`vars:
  deployment_name:
  zone: us-east1-b
`), 0644), IsNil)

	explainFlags.format = "json"
	out, err := runExplainForTest(bp)
	c.Assert(err, IsNil)
	var res ExplainBlueprintResult
	c.Assert(json.Unmarshal([]byte(out), &res), IsNil)
	got := map[string]ExplainVariable{}
	for _, v := range res.Variables {
		got[v.Name] = v
	}
	// cmd/create.go:mergeDeploymentSettings: a deployment-file null overrides the
	// blueprint default, so `gcluster deploy -d` rejects it as unset.
	c.Check(got["deployment_name"].Required, Equals, true)
	c.Check(got["zone"].Default, Equals, `"us-east1-b"`)
	// $(null) is the intentional "optional, unset by default" marker; validateVars accepts it.
	c.Check(got["optional_thing"].Required, Equals, false)

	// B2: Packer module snippet includes 'kind: packer' and instructs adding a new group
	explainFlags.format = "text"
	outPackerSpec, err := runExplainForTest("a4x", "custom-image", "lustre")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outPackerSpec, "kind: packer"), Equals, true)
	c.Check(strings.Contains(outPackerSpec, "NEW deployment group"), Equals, true)

	// B3: Multi-VPC blueprint prefers cluster VPC ('network') over Packer image-builder VPC ('builder-net')
	outG4Lustre, err := runExplainForTest("slurm", "g4", "lustre")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outG4Lustre, "use: [network]"), Equals, true)
	c.Check(strings.Contains(outG4Lustre, "use: [network, private_service_access]"), Equals, true)
	c.Check(strings.Contains(outG4Lustre, "use: [builder-net"), Equals, false)

	// B4: Running from inside community/examples still resolves the top-level repo root
	c.Assert(os.Chdir(filepath.Join(repoRoot, "community", "examples")), IsNil)
	c.Check(findExplainRepoRoot(), Equals, repoRoot)
	c.Assert(os.Chdir(repoRoot), IsNil)

	// Generic outputs (capacity_gb, gcp_service_list) must not trigger false upstream use:
	outGkeStorage, err := runExplainForTest("modules/file-system/gke-storage")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outGkeStorage, "netapp_storage_pool"), Equals, false)
	c.Check(strings.Contains(outGkeStorage, "capacity_gb:"), Equals, true)

	outSvcEnable, err := runExplainForTest("community/modules/project/service-enablement")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outSvcEnable, "htcondor_install"), Equals, false)
	c.Check(strings.Contains(outSvcEnable, "gcp_service_list"), Equals, true)

	// Mode 1 advertised 'gcs-bucket' tag resolves to cloud-storage-bucket in spec mode
	outGkeGcsBucket, err := runExplainForTest("gke", "gcs-bucket")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outGkeGcsBucket, "cloud-storage-bucket"), Equals, true)

	// Multi-input provider selection prefers already-selected provider (spack_setup)
	outSpackExec, err := runExplainForTest("community/modules/scripts/spack-execute")
	c.Assert(err, IsNil)
	c.Check(strings.Contains(outSpackExec, "use: [spack_setup]"), Equals, true)

	// Invalid auto-discovered companion deployment file fails fast
	c.Assert(os.WriteFile(filepath.Join(dir, "t-deployment.yaml"), []byte("vars: [\ninvalid yaml"), 0644), IsNil)
	_, err = runExplainForTest(bp)
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "failed to load deployment file"), Equals, true)
}
