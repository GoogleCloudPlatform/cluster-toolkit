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
	"context"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulewriter"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
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

	// check ai_assisted in deployment file propagates to global labels on Expand
	ds4 := config.DeploymentSettings{
		AIAssisted: true,
		Vars: config.Dict{}.
			With("project_id", cty.StringVal("ds_test_project_id")).
			With("deployment_name", cty.StringVal("ds_deployment_name")),
	}
	bp4 := config.Blueprint{
		BlueprintName: "tree",
		Vars:          config.NewDict(map[string]cty.Value{}),
	}
	c.Assert(mergeDeploymentSettings(&bp4, ds4), IsNil)
	c.Check(bp4.AIAssisted, Equals, true)
	c.Assert(bp4.Expand(), IsNil)
	c.Check(bp4.Vars.Get("labels"), DeepEquals, cty.ObjectVal(map[string]cty.Value{
		"ghpc_blueprint":   cty.StringVal("tree"),
		"ghpc_deployment":  config.GlobalRef("deployment_name").AsValue(),
		"ghpc_ai_assisted": cty.StringVal("true"),
	}))
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

// writePrevDeployment writes an expanded blueprint for deployment "name" under
// outDir, with the given global labels.
func writePrevDeployment(c *C, outDir, name string, labels map[string]cty.Value) {
	artDir := modulewriter.ArtifactsDir(filepath.Join(outDir, name))
	c.Assert(os.MkdirAll(artDir, 0755), IsNil)
	vars := config.Dict{}.With("deployment_name", cty.StringVal(name))
	if labels != nil {
		vars = vars.With("labels", cty.ObjectVal(labels))
	}
	prev := config.Blueprint{BlueprintName: "bp", Vars: vars}
	c.Assert(prev.Export(filepath.Join(artDir, modulewriter.ExpandedBlueprintName)), IsNil)
}

func newCreatorLabelTestCmd() *cobra.Command {
	return addExpandFlags(&cobra.Command{Use: "create"}, false)
}

func (s *MySuite) TestPreviousCreatorLabel(c *C) {
	out := c.MkDir()
	bp := func(name string) config.Blueprint {
		return config.Blueprint{Vars: config.Dict{}.With("deployment_name", cty.StringVal(name))}
	}

	// No previous deployment.
	_, ok := previousCreatorLabel(out, bp("absent"))
	c.Check(ok, Equals, false)

	// Previous deployment with ghpc_creator.
	writePrevDeployment(c, out, "with", map[string]cty.Value{
		"ghpc_blueprint": cty.StringVal("bp"),
		"ghpc_creator":   cty.StringVal("jdoe_google_com"),
	})
	creator, ok := previousCreatorLabel(out, bp("with"))
	c.Check(ok, Equals, true)
	c.Check(creator, Equals, "jdoe_google_com")

	// Previous deployment without ghpc_creator.
	writePrevDeployment(c, out, "without", map[string]cty.Value{
		"ghpc_blueprint": cty.StringVal("bp"),
	})
	creator, ok = previousCreatorLabel(out, bp("without"))
	c.Check(ok, Equals, true)
	c.Check(creator, Equals, "")

	// Previous deployment without any labels is treated as unknown.
	writePrevDeployment(c, out, "nolabels", nil)
	_, ok = previousCreatorLabel(out, bp("nolabels"))
	c.Check(ok, Equals, false)

	// Missing deployment_name.
	_, ok = previousCreatorLabel(out, config.Blueprint{})
	c.Check(ok, Equals, false)

	// Unreadable previous expanded blueprint is treated as unknown.
	badArt := modulewriter.ArtifactsDir(filepath.Join(out, "malformed"))
	c.Assert(os.MkdirAll(badArt, 0755), IsNil)
	c.Assert(os.WriteFile(filepath.Join(badArt, modulewriter.ExpandedBlueprintName), []byte("humus"), 0644), IsNil)
	_, ok = previousCreatorLabel(out, bp("malformed"))
	c.Check(ok, Equals, false)

	// Labels that are not a map are treated as unknown.
	writePrevDeploymentVars(c, out, "scalarlabels", cty.StringVal("not-a-map"))
	_, ok = previousCreatorLabel(out, bp("scalarlabels"))
	c.Check(ok, Equals, false)

	// A non-string ghpc_creator is treated as unknown.
	writePrevDeployment(c, out, "numcreator", map[string]cty.Value{"ghpc_creator": cty.NumberIntVal(7)})
	_, ok = previousCreatorLabel(out, bp("numcreator"))
	c.Check(ok, Equals, false)
}

// writePrevDeploymentVars writes an expanded blueprint for deployment "name"
// under outDir with an arbitrary vars.labels value.
func writePrevDeploymentVars(c *C, outDir, name string, labels cty.Value) {
	artDir := modulewriter.ArtifactsDir(filepath.Join(outDir, name))
	c.Assert(os.MkdirAll(artDir, 0755), IsNil)
	prev := config.Blueprint{BlueprintName: "bp", Vars: config.Dict{}.
		With("deployment_name", cty.StringVal(name)).
		With("labels", labels)}
	c.Assert(prev.Export(filepath.Join(artDir, modulewriter.ExpandedBlueprintName)), IsNil)
}

// writeExpandedPrevDeployment expands a blueprint that sets its own vars.labels
// (so the expanded blueprint stores labels as a merge(...) expression) and
// writes it as the previous deployment "name" under outDir.
func writeExpandedPrevDeployment(c *C, outDir, name, creator string) {
	prev := config.Blueprint{
		BlueprintName:   "bp",
		AddCreatorLabel: creator != "",
		CreatorUsername: creator,
		Vars: config.Dict{}.
			With("deployment_name", cty.StringVal(name)).
			With("labels", cty.ObjectVal(map[string]cty.Value{"team": cty.StringVal("hpc")})),
	}
	c.Assert(prev.Expand(), IsNil)

	artDir := modulewriter.ArtifactsDir(filepath.Join(outDir, name))
	c.Assert(os.MkdirAll(artDir, 0755), IsNil)
	expPath := filepath.Join(artDir, modulewriter.ExpandedBlueprintName)
	c.Assert(prev.Export(expPath), IsNil)

	raw, err := os.ReadFile(expPath)
	c.Assert(err, IsNil)
	c.Assert(string(raw), Matches, `(?s).*merge\(.*`) // labels are stored as an expression
}

func (s *MySuite) TestPreviousCreatorLabel_UserLabels(c *C) {
	out := c.MkDir()
	bp := func(name string) config.Blueprint {
		return config.Blueprint{Vars: config.Dict{}.With("deployment_name", cty.StringVal(name))}
	}

	// Previous deployment with user labels and ghpc_creator.
	writeExpandedPrevDeployment(c, out, "with", "alice@google.com")
	creator, ok := previousCreatorLabel(out, bp("with"))
	c.Check(ok, Equals, true)
	c.Check(creator, Equals, "alice_google_com")

	// Previous deployment with user labels and no ghpc_creator.
	writeExpandedPrevDeployment(c, out, "without", "")
	creator, ok = previousCreatorLabel(out, bp("without"))
	c.Check(ok, Equals, true)
	c.Check(creator, Equals, "")
}

func (s *MySuite) TestSetCreatorLabel(c *C) {
	out := c.MkDir()
	oldOut := createFlags.outputDir
	createFlags.outputDir = out
	defer func() { createFlags.outputDir = oldOut }()
	oldAddCreatorLabel := expandFlags.addCreatorLabel
	defer func() { expandFlags.addCreatorLabel = oldAddCreatorLabel }()

	oldAccount, hadAccount := os.LookupEnv("CLOUDSDK_CORE_ACCOUNT")
	c.Assert(os.Setenv("CLOUDSDK_CORE_ACCOUNT", "new@google.com"), IsNil)
	defer func() {
		if hadAccount {
			os.Setenv("CLOUDSDK_CORE_ACCOUNT", oldAccount)
		} else {
			os.Unsetenv("CLOUDSDK_CORE_ACCOUNT")
		}
	}()

	writePrevDeployment(c, out, "with", map[string]cty.Value{"ghpc_creator": cty.StringVal("old_google_com")})
	writePrevDeployment(c, out, "without", map[string]cty.Value{"ghpc_blueprint": cty.StringVal("bp")})

	bp := func(name string) config.Blueprint {
		return config.Blueprint{Vars: config.Dict{}.With("deployment_name", cty.StringVal(name))}
	}

	{ // New deployment: detect the active account.
		b := bp("fresh")
		setCreatorLabel(newCreatorLabelTestCmd(), &b)
		c.Check(b.AddCreatorLabel, Equals, true)
		c.Check(b.CreatorUsername, Equals, "new@google.com")
	}
	{ // Existing deployment with ghpc_creator: keep the recorded creator.
		b := bp("with")
		setCreatorLabel(newCreatorLabelTestCmd(), &b)
		c.Check(b.AddCreatorLabel, Equals, true)
		c.Check(b.CreatorUsername, Equals, "old_google_com")
	}
	{ // Existing deployment without ghpc_creator: keep it absent.
		b := bp("without")
		setCreatorLabel(newCreatorLabelTestCmd(), &b)
		c.Check(b.AddCreatorLabel, Equals, false)
	}
	{ // An explicit flag behaves as before: false drops the label.
		b := bp("with")
		cmd := newCreatorLabelTestCmd()
		c.Assert(cmd.Flags().Set("add-creator-label", "false"), IsNil)
		setCreatorLabel(cmd, &b)
		c.Check(b.AddCreatorLabel, Equals, false)
	}
	{ // An explicit flag behaves as before: true detects the active account.
		b := bp("without")
		cmd := newCreatorLabelTestCmd()
		c.Assert(cmd.Flags().Set("add-creator-label", "true"), IsNil)
		setCreatorLabel(cmd, &b)
		c.Check(b.AddCreatorLabel, Equals, true)
		c.Check(b.CreatorUsername, Equals, "new@google.com")
	}
	{ // New deployment with a non-@google.com account: no label.
		c.Assert(os.Setenv("CLOUDSDK_CORE_ACCOUNT", "someone@example.com"), IsNil)
		b := bp("fresh")
		setCreatorLabel(newCreatorLabelTestCmd(), &b)
		c.Check(b.AddCreatorLabel, Equals, false)
	}
}

func (s *MySuite) TestVerifyGcsBucketsEmptyBucket(c *C) {
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id": cty.StringVal("my-project"),
		}),
		Groups: []config.Group{
			{
				Name: "group1",
				TerraformBackend: config.TerraformBackend{
					Type: "gcs",
					Configuration: config.NewDict(map[string]cty.Value{
						"bucket": cty.NullVal(cty.String),
					}),
				},
			},
		},
	}

	err := verifyGcsBuckets(context.Background(), bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name for group \"group1\" cannot be empty or unknown")

	bp.Groups[0].TerraformBackend.Configuration = config.NewDict(map[string]cty.Value{
		"bucket": cty.StringVal(""),
	})
	err = verifyGcsBuckets(context.Background(), bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name for group \"group1\" cannot be empty")
}
