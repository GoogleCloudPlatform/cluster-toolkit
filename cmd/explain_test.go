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
