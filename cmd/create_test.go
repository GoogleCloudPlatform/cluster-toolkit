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

package cmd

import (
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"os"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

func (s *MySuite) TestMergeDeploymentSettings(c *C) {
	ds1 := config.DeploymentSettings{
		Vars: config.Dict{}.
			With("project_id", cty.StringVal("ds_test_project_id")).
			With("deployment_name", cty.StringVal("ds_deployment_name"))}

	bp1 := config.Blueprint{
		Vars: config.Dict{}.
			With("project_id", cty.StringVal("bp_test_project_id")).
			With("example_var", cty.StringVal("bp_example_value"))}

	// test priority-based merging of deployment variables
	mergeDeploymentSettings(&bp1, ds1)
	c.Check(bp1.Vars.Items(), DeepEquals, map[string]cty.Value{
		"project_id":      cty.StringVal("ds_test_project_id"),
		"deployment_name": cty.StringVal("ds_deployment_name"),
		"example_var":     cty.StringVal("bp_example_value"),
	})

	// check merging zero-value backends
	ds2 := config.DeploymentSettings{
		TerraformBackendDefaults: config.TerraformBackend{},
	}
	bp2 := config.Blueprint{
		TerraformBackendDefaults: config.TerraformBackend{},
	}
	mergeDeploymentSettings(&bp2, ds2)
	c.Check(bp2.TerraformBackendDefaults, DeepEquals, config.TerraformBackend{})

	// check keeping blueprint defined backend with no backend in deployment file
	bp3 := config.Blueprint{
		TerraformBackendDefaults: config.TerraformBackend{
			Type: "gsc",
			Configuration: config.NewDict(map[string]cty.Value{
				"bucket": cty.StringVal("bp_bucket"),
			}),
		},
	}
	mergeDeploymentSettings(&bp3, ds2)
	c.Check(bp3.TerraformBackendDefaults, DeepEquals, config.TerraformBackend{
		Type: "gsc",
		Configuration: config.NewDict(map[string]cty.Value{
			"bucket": cty.StringVal("bp_bucket"),
		}),
	})

	// check overriding blueprint defined backend with deployment file
	ds3 := config.DeploymentSettings{
		TerraformBackendDefaults: config.TerraformBackend{
			Type: "gsc",
			Configuration: config.NewDict(map[string]cty.Value{
				"bucket": cty.StringVal("ds_bucket"),
			}),
		},
	}
	mergeDeploymentSettings(&bp3, ds3)
	c.Check(bp3.TerraformBackendDefaults, DeepEquals, config.TerraformBackend{
		Type: "gsc",
		Configuration: config.NewDict(map[string]cty.Value{
			"bucket": cty.StringVal("ds_bucket"),
		}),
	})
}

func (s *MySuite) TestIsOverwriteAllowed_Absent(c *C) {
	testDir := c.MkDir()
	depDir := filepath.Join(testDir, "casper")

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/), IsNil)
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_NotGHPC(c *C) {
	depDir := c.MkDir() // empty deployment folder considered malformed

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* not a valid GHPC deployment folder.*")
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* not a valid GHPC deployment folder.*")

	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_NoExpanded(c *C) {
	depDir := c.MkDir() // empty deployment folder considered malformed
	if err := os.MkdirAll(modulewriter.HiddenGhpcDir(depDir), 0755); err != nil {
		c.Fatal(err)
	}

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* changing GHPC version.*")
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/),
		ErrorMatches, ".* changing GHPC version.*")

	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_Malformed(c *C) {
	depDir := c.MkDir() // empty deployment folder considered malformed
	if err := os.MkdirAll(modulewriter.ArtifactsDir(depDir), 0755); err != nil {
		c.Fatal(err)
	}
	expPath := filepath.Join(modulewriter.ArtifactsDir(depDir), "expanded_blueprint.yaml")
	if err := os.WriteFile(expPath, []byte("humus"), 0644); err != nil {
		c.Fatal(err)
	}

	bp := config.Blueprint{}
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, false /*forceOverwrite*/), NotNil)
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, false /*forceOverwrite*/), NotNil)
	// force
	c.Check(checkOverwriteAllowed(depDir, bp, false /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
	c.Check(checkOverwriteAllowed(depDir, bp, true /*overwriteFlag*/, true /*forceOverwrite*/), IsNil)
}

func (s *MySuite) TestIsOverwriteAllowed_Present(c *C) {
	p := c.MkDir()
	artDir := modulewriter.ArtifactsDir(p)
	if err := os.MkdirAll(artDir, 0755); err != nil {
		c.Fatal(err)
	}

	prev := config.Blueprint{
		GhpcVersion: "TaleOfBygoneYears",
		Groups: []config.Group{
			{Name: "isildur"}}}
	if err := prev.Export(filepath.Join(artDir, "expanded_blueprint.yaml")); err != nil {
		c.Fatal(err)
	}
	noW, yesW, noForce, yesForce := false, true, false, true

	{ // Superset
		bp := config.Blueprint{
			GhpcVersion: "TaleOfBygoneYears",
			Groups: []config.Group{
				{Name: "isildur"},
				{Name: "elendil"}}}
		c.Check(checkOverwriteAllowed(p, bp, noW, noForce), ErrorMatches, ".* already exists.*")
		c.Check(checkOverwriteAllowed(p, bp, yesW, noForce), IsNil)
	}

	{ // Version mismatch
		bp := config.Blueprint{
			GhpcVersion: "TheAlloyOfLaw",
			Groups: []config.Group{
				{Name: "isildur"}}}
		c.Check(checkOverwriteAllowed(p, bp, noW, noForce), ErrorMatches, ".*ghpc_version has changed.*")
		c.Check(checkOverwriteAllowed(p, bp, yesW, noForce), ErrorMatches, ".*ghpc_version has changed.*")
		c.Check(checkOverwriteAllowed(p, bp, noW, yesForce), IsNil)
	}

	{ // Subset
		bp := config.Blueprint{
			GhpcVersion: "TaleOfBygoneYears",
			Groups: []config.Group{
				{Name: "aragorn"}}}
		c.Check(checkOverwriteAllowed(p, bp, noW, noForce), ErrorMatches, `.* already exists.*`)
		c.Check(checkOverwriteAllowed(p, bp, yesW, noForce), ErrorMatches, `.*remove a deployment group "isildur".*`)
		c.Check(checkOverwriteAllowed(p, bp, noW, yesForce), IsNil)
	}
}
