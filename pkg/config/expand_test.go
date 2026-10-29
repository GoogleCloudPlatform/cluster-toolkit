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

package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing/fstest"

	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

func (s *zeroSuite) TestExpandBackend(c *C) {
	type BE = TerraformBackend // alias for brevity
	noDefBe := Blueprint{BlueprintName: "tree"}

	{ // no def BE, no group BE
		g := Group{Name: "clown"}
		noDefBe.expandBackend(&g)
		c.Check(g.TerraformBackend, DeepEquals, BE{})
	}

	{ // no def BE, group BE
		g := Group{
			Name:             "clown",
			TerraformBackend: BE{Type: "gcs"}}
		noDefBe.expandBackend(&g)
		c.Check(g.TerraformBackend, DeepEquals, BE{Type: "gcs"})
	}

	defBe := noDefBe
	defBe.TerraformBackendDefaults = BE{
		Type: "gcs",
		Configuration: NewDict(map[string]cty.Value{
			"leave": cty.StringVal("fall")})}

	{ // def BE, no group BE
		g := Group{Name: "clown"}
		defBe.expandBackend(&g)

		c.Check(g.TerraformBackend, DeepEquals, BE{ // no change
			Type: "gcs",
			Configuration: NewDict(map[string]cty.Value{
				"prefix": MustParseExpression(`"tree/${var.deployment_name}/clown"`).AsValue(),
				"leave":  cty.StringVal("fall")})})
	}

	{ // def BE, group BE non-gcs
		g := Group{
			Name: "clown",
			TerraformBackend: BE{
				Type: "pure_gold",
				Configuration: NewDict(map[string]cty.Value{
					"branch": cty.False})}}
		defBe.expandBackend(&g)

		c.Check(g.TerraformBackend, DeepEquals, BE{ // no change
			Type: "pure_gold",
			Configuration: NewDict(map[string]cty.Value{
				"branch": cty.False})})
	}
}

func (s *zeroSuite) TestExpandProviders(c *C) {
	type PR = TerraformProvider // alias for brevity
	noDefPr := Blueprint{BlueprintName: "tree"}

	testProvider := map[string]PR{
		"test-provider": TerraformProvider{
			Source:  "test-src",
			Version: "test-vers",
			Configuration: Dict{}.
				With("project", cty.StringVal("test-prj")).
				With("region", cty.StringVal("reg1")).
				With("zone", cty.StringVal("zone1")).
				With("universe_domain", cty.StringVal("test-universe.com"))}}

	{ // no def PR, no group PR - match default values
		g := Group{Name: "clown"}
		noDefPr.expandProviders(&g)
		c.Check(g.TerraformProviders, DeepEquals, map[string]PR{
			"google": TerraformProvider{
				Source:  "hashicorp/google",
				Version: GoogleProviderVersion},
			"google-beta": TerraformProvider{
				Source:  "hashicorp/google-beta",
				Version: GoogleProviderVersion}})
	}

	{ // no def PR, group PR
		g := Group{
			Name:               "clown",
			TerraformProviders: testProvider}
		noDefPr.expandProviders(&g)
		c.Check(g.TerraformProviders, DeepEquals, testProvider)
	}

	defBe := noDefPr
	defBe.TerraformProviders = testProvider

	{ // def PR, no group PR
		g := Group{Name: "clown"}
		defBe.expandProviders(&g)

		c.Check(g.TerraformProviders, DeepEquals, testProvider)
	}

	group_provider := map[string]PR{
		"test-provider": TerraformProvider{
			Source:  "test-source",
			Version: "test-versions",
			Configuration: Dict{}.
				With("project", cty.StringVal("test-prj")).
				With("region", cty.StringVal("reg2")).
				With("zone", cty.StringVal("zone2s")).
				With("universe_domain", cty.StringVal("fake-universe.com"))}}

	{ // def PR, group PR set
		g := Group{
			Name:               "clown",
			TerraformProviders: group_provider}
		defBe.expandProviders(&g)

		c.Check(g.TerraformProviders, DeepEquals, group_provider)
	}

	empty_provider := map[string]PR{}

	{ // No def PR, group (nil PR != PR w/ len == 0) (nil PR results in default PR values, empty PR remains empty)
		g := Group{Name: "clown"}
		g2 := Group{Name: "bear",
			TerraformProviders: empty_provider}
		noDefPr.expandProviders(&g)
		noDefPr.expandProviders(&g2)
		c.Check(g.TerraformProviders, Not(DeepEquals), g2.TerraformProviders)
	}
}

func (s *zeroSuite) TestAddListValue(c *C) {
	mod := Module{ID: "TestModule"}

	setting := "newSetting"
	first := AsProductOfModuleUse(cty.StringVal("value1"), "mod1")
	second := AsProductOfModuleUse(cty.StringVal("value2"), "mod2")

	mod.addListValue(setting, first)
	c.Check(mod.Settings.Get(setting), DeepEquals,
		AsProductOfModuleUse(MustParseExpression(`flatten(["value1"])`).AsValue(), "mod1"))

	mod.addListValue(setting, second)
	c.Check(mod.Settings.Get(setting), DeepEquals,
		AsProductOfModuleUse(MustParseExpression(`flatten(["value2", flatten(["value1"])])`).AsValue(), "mod1", "mod2"))
}

func (s *zeroSuite) TestUseModule(c *C) {
	// Setup
	type VarInfo = modulereader.VarInfo // alias for brevity

	{ // No Inputs, No Outputs
		used := tMod("used").build()
		mod := tMod("lime").build()

		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Has Output, no matching input
		used := tMod("used").outputs("mud").build()
		mod := tMod("lime").build()

		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Single Input/Output match - no lists
		used := tMod("used").outputs("mud").build()
		mod := tMod("lime").inputs("mud").build()

		useModule(&mod, used)
		ref := AsProductOfModuleUse(ModuleRef("used", "mud").AsValue(), "used")
		c.Check(mod.Settings, DeepEquals, Dict{}.With("mud", ref))
	}

	{ // Single Input/Output match - but setting was in blueprint so no-op
		used := tMod("used").outputs("mud").build()
		mod := tMod("lime").inputs("mud").set("mud", "alkaline").build()

		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{}.With("mud", cty.StringVal("alkaline")))
	}

	{ // re-apply used modules, should be a no-op, no settings were in blueprint
		used := tMod("used").outputs("mud").build()
		cur := AsProductOfModuleUse(ModuleRef("used", "mud").AsValue(), "used")
		mod := tMod("lime").inputs("mud").set("mud", cur).build()

		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{}.With("mud", cur))
	}

	{ // Single Input/Output match, input is list, not already set
		used := tMod("used").outputs("mud").build()
		mod := tMod("lime").
			inputs(VarInfo{Name: "mud", Type: cty.List(cty.Number)}).build()

		useModule(&mod, used)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"mud": AsProductOfModuleUse(
				MustParseExpression(`flatten([module.used.mud])`).AsValue(),
				"used")})
	}

	{ // Setting exists, Input is List, was not set in blueprint
		used := tMod("used").outputs("mud").build()

		cur := AsProductOfModuleUse(
			MustParseExpression(`[module.other.mud]`).AsValue(), "other")

		mod := tMod("lime").
			inputs(VarInfo{Name: "mud", Type: cty.List(cty.Number)}).
			set("mud", cur).build()

		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{}.With("mud",
			AsProductOfModuleUse(
				MustParseExpression(`flatten([module.used.mud,[module.other.mud]])`).AsValue(),
				"other", "used")))
	}

	{ // Setting exists, Input is List, was set in blueprint
		used := tMod("used").outputs("mud").build()

		cur := MustParseExpression(`[module.other.mud]`).AsValue() // sic. Not a ProductOfModuleUse
		mod := tMod("lime").
			inputs(VarInfo{Name: "mud", Type: cty.List(cty.Number)}).
			set("mud", cur).build()

		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{}.With("mud", cur)) // no change
	}
}

func (s *zeroSuite) TestExpandModule(c *C) {
	type VarInfo = modulereader.VarInfo // alias for brevity

	u := tMod("potato").outputs("az", "rose", "peony").build()

	m := tMod("yarn").
		inputs(
			VarInfo{Name: "az"},                               // set explicitly
			VarInfo{Name: "buki"},                             // set as global var
			VarInfo{Name: "labels"},                           // set as global var
			VarInfo{Name: "rose", Type: cty.List(cty.String)}, // used from `u`
			VarInfo{Name: "peony"},                            // used from `u`, global var ignored
			VarInfo{Name: "orchid"},                           // not present anywhere
		).
		uses("potato").
		set("az", "alpha").
		build()

	bp := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"labels": cty.EmptyObjectVal,
			"az":     cty.StringVal("za"),   // will be ignored
			"buki":   cty.StringVal("ikub"), // will be used
			"vedi":   cty.StringVal("idav"), // not in module inputs
			"peon":   cty.StringVal("noep"), // will be ignored
		}),
		Groups: []Group{
			{Modules: []Module{u, m}}},
	}

	mp := Root.Groups.At(0).Modules.At(1)
	c.Assert(bp.expandModule(mp, &m), IsNil)
	c.Check(m.Settings.Items(), DeepEquals, map[string]cty.Value{
		"az":    cty.StringVal("alpha"),
		"peony": AsProductOfModuleUse(ModuleRef(u.ID, "peony").AsValue(), u.ID),
		"rose": AsProductOfModuleUse(MustParseExpression(
			`flatten([module.potato.rose])`).AsValue(), u.ID),

		"labels": GlobalRef("labels").AsValue(),
		"buki":   GlobalRef("buki").AsValue(),
	})
}

func (s *zeroSuite) TestApplyGlobalVarsInModule(c *C) {
	builder := tMod("carrot").
		inputs(
			"gold",   // doesn't exist in vars
			"pyrite", // exists in vars, not set in module
			"silver", // exists in vars, set in module
			"helium", // to be set to ModuleID
		).
		set("silver", "glagol")
	builder.i.Metadata.Ghpc.InjectModuleId = "helium"
	mod := builder.build()

	vars := NewDict(map[string]cty.Value{
		"polonium": cty.StringVal("az"),
		"pyrite":   cty.StringVal("buki"),
		"silver":   cty.StringVal("vedi"),
	})

	Blueprint{Vars: vars}.applyGlobalVarsInModule(&mod)

	c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
		"silver": cty.StringVal("glagol"),
		"helium": cty.StringVal("carrot"),
		"pyrite": GlobalRef("pyrite").AsValue()})
}

func (s *zeroSuite) TestValidateModuleReference(c *C) {
	a := Module{ID: "moduleA"}
	b := Module{ID: "moduleB"}
	y := Module{ID: "moduleY"}
	pkr := Module{ID: "modulePkr", Kind: PackerKind}

	dg := []Group{
		{Name: "zero", Modules: []Module{a, b}},
		{Name: "half", Modules: []Module{pkr}},
		{Name: "one", Modules: []Module{y}},
	}

	bp := Blueprint{
		Groups: dg,
	}

	// An intragroup reference from group 0 to module B in 0 (good)
	c.Check(validateModuleReference(bp, a, b.ID), IsNil)

	// An intergroup reference from group 1 to module A in 0 (good)
	c.Check(validateModuleReference(bp, y, a.ID), IsNil)

	{ // An intergroup reference from group 0 to module 1 in 1 (bad due to group ordering)
		err := validateModuleReference(bp, a, y.ID)
		c.Check(err, ErrorMatches, fmt.Sprintf("%s: .*", errMsgIntergroupOrder))
	}

	// A target module that doesn't exist (bad)
	c.Check(validateModuleReference(bp, y, "bad-module"), NotNil)

	// Reference packer module (bad)
	c.Check(validateModuleReference(bp, y, pkr.ID), NotNil)

}

func (s *zeroSuite) TestSubstituteModuleSources(c *C) {
	a := Module{ID: "moduleA", Source: "modules/network/pre-existing-vpc"}
	b := Module{ID: "moduleB", Source: "modules/file-system/managed-lustre"}
	y := Module{ID: "moduleY", Source: "./modules/network/pre-existing-vpc"}

	dg := []Group{
		{Name: "zero", Modules: []Module{a, b}},
		{Name: "one", Modules: []Module{y}},
	}

	// toolkit_modules_url and toolkit_modules_version not provided
	bp := Blueprint{
		Groups: dg,
	}
	bp.substituteModuleSources()
	// Check that sources remain unchanged
	c.Assert(bp.Groups[0].Modules[0].Source, Equals, "modules/network/pre-existing-vpc")
	c.Assert(bp.Groups[0].Modules[1].Source, Equals, "modules/file-system/managed-lustre")
	c.Assert(bp.Groups[1].Modules[0].Source, Equals, "./modules/network/pre-existing-vpc")

	// toolkit_modules_url and toolkit_modules_version provided
	bp = Blueprint{
		Groups: dg, ToolkitModulesURL: "github.com/GoogleCloudPlatform/cluster-toolkit", ToolkitModulesVersion: "v1.15.0",
	}
	bp.substituteModuleSources()
	// Check that embedded sources (a and b) are transformed correctly
	expectedSourceA := "github.com/GoogleCloudPlatform/cluster-toolkit//modules/network/pre-existing-vpc?ref=v1.15.0&depth=1"
	expectedSourceB := "github.com/GoogleCloudPlatform/cluster-toolkit//modules/file-system/managed-lustre?ref=v1.15.0&depth=1"
	c.Assert(bp.Groups[0].Modules[0].Source, Equals, expectedSourceA)
	c.Assert(bp.Groups[0].Modules[1].Source, Equals, expectedSourceB)

	// Check that the non-embedded source (y) remains unchanged
	c.Assert(bp.Groups[1].Modules[0].Source, Equals, "./modules/network/pre-existing-vpc")
}

func (s *zeroSuite) TestIntersection(c *C) {
	is := intersection([]string{"A", "B", "C"}, []string{"A", "B", "C"})
	c.Assert(is, DeepEquals, []string{"A", "B", "C"})

	is = intersection([]string{"A", "B", "C"}, []string{"C", "B", "A"})
	c.Assert(is, DeepEquals, []string{"A", "B", "C"})

	is = intersection([]string{"C", "B", "A"}, []string{"A", "B", "C", "C"})
	c.Assert(is, DeepEquals, []string{"A", "B", "C"})

	is = intersection([]string{"A", "B", "C"}, []string{"D", "C", "B", "A"})
	c.Assert(is, DeepEquals, []string{"A", "B", "C"})

	is = intersection([]string{"A", "C"}, []string{"D", "C", "B", "A"})
	c.Assert(is, DeepEquals, []string{"A", "C"})

	is = intersection([]string{"A", "C"}, []string{})
	c.Assert(is, DeepEquals, []string{})

	is = intersection([]string{"A", "C"}, nil)
	c.Assert(is, DeepEquals, []string{})
}

func (s *zeroSuite) TestOutputNamesByGroup(c *C) {
	zebra := Group{
		Name: "zebra",
		Modules: []Module{
			{
				ID: "stripes",
				Outputs: []modulereader.OutputInfo{
					{Name: "length"}}}}}
	pony := Group{
		Name: "pony",
		Modules: []Module{

			tMod("bucephalus").set("width", ModuleRef("stripes", "length")).build(),
		}}
	bp := Blueprint{Groups: []Group{zebra, pony}}

	{
		got, err := OutputNamesByGroup(zebra, bp)
		c.Check(err, IsNil)
		c.Check(got, DeepEquals, map[GroupName][]string{})
	}

	{
		got, err := OutputNamesByGroup(pony, bp)
		c.Check(err, IsNil)
		c.Check(got, DeepEquals, map[GroupName][]string{
			"zebra": {"length_stripes"},
		})
	}
}

func (s *zeroSuite) TestExpandGlobalLabels(c *C) {
	{ // AddCreatorLabel false
		bp := Blueprint{
			BlueprintName:   "tree",
			Vars:            NewDict(map[string]cty.Value{}),
			AddCreatorLabel: false,
		}
		bp.expandGlobalLabels()
		labelsVal := bp.Vars.Get("labels")

		expectedLabels := cty.ObjectVal(map[string]cty.Value{
			"ghpc_blueprint":  cty.StringVal("tree"),
			"ghpc_deployment": GlobalRef("deployment_name").AsValue(),
		})
		c.Check(labelsVal, DeepEquals, expectedLabels)
	}

	{ // AddCreatorLabel true
		bp := Blueprint{
			BlueprintName:   "tree",
			Vars:            NewDict(map[string]cty.Value{}),
			AddCreatorLabel: true,
			CreatorUsername: "testuser",
		}
		bp.expandGlobalLabels()
		labelsVal := bp.Vars.Get("labels")

		expectedLabels := cty.ObjectVal(map[string]cty.Value{
			"ghpc_blueprint":  cty.StringVal("tree"),
			"ghpc_deployment": GlobalRef("deployment_name").AsValue(),
			"ghpc_creator":    cty.StringVal("testuser"),
		})
		c.Check(labelsVal, DeepEquals, expectedLabels)
	}

	{ // AddCreatorLabel true with sanitization
		bp := Blueprint{
			BlueprintName:   "tree",
			Vars:            NewDict(map[string]cty.Value{}),
			AddCreatorLabel: true,
			CreatorUsername: "user@example.com",
		}
		bp.expandGlobalLabels()
		labelsVal := bp.Vars.Get("labels")

		expectedLabels := cty.ObjectVal(map[string]cty.Value{
			"ghpc_blueprint":  cty.StringVal("tree"),
			"ghpc_deployment": GlobalRef("deployment_name").AsValue(),
			"ghpc_creator":    cty.StringVal("user_example_com"),
		})
		c.Check(labelsVal, DeepEquals, expectedLabels)
	}

	{ // AddCreatorLabel true with uppercase and special characters
		bp := Blueprint{
			BlueprintName:   "tree",
			Vars:            NewDict(map[string]cty.Value{}),
			AddCreatorLabel: true,
			CreatorUsername: "User+Name@Example.COM",
		}
		bp.expandGlobalLabels()
		labelsVal := bp.Vars.Get("labels")

		expectedLabels := cty.ObjectVal(map[string]cty.Value{
			"ghpc_blueprint":  cty.StringVal("tree"),
			"ghpc_deployment": GlobalRef("deployment_name").AsValue(),
			"ghpc_creator":    cty.StringVal("user_name_example_com"),
		})
		c.Check(labelsVal, DeepEquals, expectedLabels)
	}

	{ // AddCreatorLabel true with long username
		longUsername := "a" + strings.Repeat("b", 70)
		bp := Blueprint{
			BlueprintName:   "tree",
			Vars:            NewDict(map[string]cty.Value{}),
			AddCreatorLabel: true,
			CreatorUsername: longUsername,
		}
		bp.expandGlobalLabels()
		labelsVal := bp.Vars.Get("labels")

		expectedLabels := cty.ObjectVal(map[string]cty.Value{
			"ghpc_blueprint":  cty.StringVal("tree"),
			"ghpc_deployment": GlobalRef("deployment_name").AsValue(),
			"ghpc_creator":    cty.StringVal("a" + strings.Repeat("b", 62)),
		})
		c.Check(labelsVal, DeepEquals, expectedLabels)
	}

	{ // AddCreatorLabel true with spaces and symbols
		bp := Blueprint{
			BlueprintName:   "tree",
			Vars:            NewDict(map[string]cty.Value{}),
			AddCreatorLabel: true,
			CreatorUsername: "user name!#$",
		}
		bp.expandGlobalLabels()
		labelsVal := bp.Vars.Get("labels")

		expectedLabels := cty.ObjectVal(map[string]cty.Value{
			"ghpc_blueprint":  cty.StringVal("tree"),
			"ghpc_deployment": GlobalRef("deployment_name").AsValue(),
			"ghpc_creator":    cty.StringVal("user_name___"),
		})
		c.Check(labelsVal, DeepEquals, expectedLabels)
	}

	{ // AddCreatorLabel true with leading number
		bp := Blueprint{
			BlueprintName:   "tree",
			Vars:            NewDict(map[string]cty.Value{}),
			AddCreatorLabel: true,
			CreatorUsername: "123user",
		}
		bp.expandGlobalLabels()
		labelsVal := bp.Vars.Get("labels")

		expectedLabels := cty.ObjectVal(map[string]cty.Value{
			"ghpc_blueprint":  cty.StringVal("tree"),
			"ghpc_deployment": GlobalRef("deployment_name").AsValue(),
			"ghpc_creator":    cty.StringVal("123user"),
		})
		c.Check(labelsVal, DeepEquals, expectedLabels)
	}
}

func (s *zeroSuite) TestCheckInputValueMatchesType_Panic(c *C) {
	m := tMod("yarn").
		inputs(modulereader.VarInfo{Name: "bad_input", Type: cty.String}).
		set("bad_input", cty.NilVal).
		build()

	bp := Blueprint{
		Groups: []Group{
			{Modules: []Module{m}}},
	}

	mp := Root.Groups.At(0).Modules.At(0)
	err := bp.expandModule(mp, &m)
	c.Assert(err, NotNil)
	c.Check(err, ErrorMatches, ".*panic during type conversion for \"bad_input\".*")
}

func (s *zeroSuite) TestDeduplicateDranetTemplates(c *C) {
	// Case 1: Standard deduplication for same device class (netdev.google.com)
	{
		bp := &Blueprint{}
		m1 := Module{
			ID:     "pool-a",
			Source: "community/modules/compute/gke-node-pool",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(true),
				"machine_type":  cty.StringVal("ct6e-standard-4t"), // auto-detects to netdev.google.com
			}),
		}
		m2 := Module{
			ID:     "pool-b",
			Source: "community/modules/compute/gke-node-pool",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(true),
				"machine_type":  cty.StringVal("ct6e-standard-4t"), // auto-detects to netdev.google.com
			}),
		}
		bp.Groups = []Group{{Modules: []Module{m1, m2}}}

		bp.deduplicateDranetTemplates()

		// pool-a is first, should install
		c.Assert(bp.Groups[0].Modules[0].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[0].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(true))

		// pool-b is second of same class, should NOT install
		c.Assert(bp.Groups[0].Modules[1].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[1].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(false))
	}

	// Case 2: Different device classes (mrdma.google.com vs netdev.google.com) - both should install
	{
		bp := &Blueprint{}
		m1 := Module{
			ID:     "pool-a",
			Source: "community/modules/compute/gke-node-pool",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(true),
				"machine_type":  cty.StringVal("a3-ultragpu-8g"), // auto-detects to mrdma.google.com
			}),
		}
		m2 := Module{
			ID:     "pool-b",
			Source: "community/modules/compute/gke-node-pool",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(true),
				"machine_type":  cty.StringVal("ct6e-standard-4t"), // auto-detects to netdev.google.com
			}),
		}
		bp.Groups = []Group{{Modules: []Module{m1, m2}}}

		bp.deduplicateDranetTemplates()

		// Both are first of their class, both should install
		c.Assert(bp.Groups[0].Modules[0].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[0].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(true))

		c.Assert(bp.Groups[0].Modules[1].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[1].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(true))
	}

	// Case 3: Explicit device class name overrides auto-detect
	{
		bp := &Blueprint{}
		m1 := Module{
			ID:     "pool-a",
			Source: "community/modules/compute/gke-node-pool",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet":            cty.BoolVal(true),
				"dranet_device_class_name": cty.StringVal("custom.google.com"),
			}),
		}
		m2 := Module{
			ID:     "pool-b",
			Source: "community/modules/compute/gke-node-pool",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet":            cty.BoolVal(true),
				"dranet_device_class_name": cty.StringVal("custom.google.com"),
			}),
		}
		bp.Groups = []Group{{Modules: []Module{m1, m2}}}

		bp.deduplicateDranetTemplates()

		c.Assert(bp.Groups[0].Modules[0].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[0].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(true))

		c.Assert(bp.Groups[0].Modules[1].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[1].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(false))
	}

	// Case 4: Non-GKE node pool module or enable_dranet=false should be ignored
	{
		bp := &Blueprint{}
		m1 := Module{
			ID:     "not-gke-pool",
			Source: "community/modules/compute/gke-cluster",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(true),
			}),
		}
		m2 := Module{
			ID:     "pool-a",
			Source: "community/modules/compute/gke-node-pool",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(false),
			}),
		}
		bp.Groups = []Group{{Modules: []Module{m1, m2}}}

		bp.deduplicateDranetTemplates()

		c.Check(bp.Groups[0].Modules[0].Settings.Has("install_dranet_template"), Equals, false)
		c.Check(bp.Groups[0].Modules[1].Settings.Has("install_dranet_template"), Equals, false)
	}

	// Case 5: Remote module sources with query parameters should be correctly matched
	{
		bp := &Blueprint{}
		m1 := Module{
			ID:     "pool-a",
			Source: "github.com/GoogleCloudPlatform/cluster-toolkit//modules/compute/gke-node-pool?ref=v1.15.0&depth=1",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(true),
				"machine_type":  cty.StringVal("ct6e-standard-4t"),
			}),
		}
		m2 := Module{
			ID:     "pool-b",
			Source: "github.com/GoogleCloudPlatform/cluster-toolkit//modules/compute/gke-node-pool?ref=v1.15.0&depth=1",
			Settings: NewDict(map[string]cty.Value{
				"enable_dranet": cty.BoolVal(true),
				"machine_type":  cty.StringVal("ct6e-standard-4t"),
			}),
		}
		bp.Groups = []Group{{Modules: []Module{m1, m2}}}

		bp.deduplicateDranetTemplates()

		// Both should be correctly identified as GKE node pools, and deduplicated
		c.Assert(bp.Groups[0].Modules[0].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[0].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(true))

		c.Assert(bp.Groups[0].Modules[1].Settings.Has("install_dranet_template"), Equals, true)
		c.Check(bp.Groups[0].Modules[1].Settings.Get("install_dranet_template"), DeepEquals, cty.BoolVal(false))
	}

}

func (s *zeroSuite) TestExpandWorkloadPolicy(c *C) {
	// Case 1: Injects conditional expression into TPU workload_policy when enable_dynamic_slicing_for_tpus is in Vars
	{
		bp := Blueprint{
			Vars: NewDict(map[string]cty.Value{
				"enable_dynamic_slicing_for_tpus": cty.BoolVal(false),
			}),
		}
		wpMod := Module{
			ID:     "workload_policy",
			Source: "modules/compute/resource-policy",
			Settings: NewDict(map[string]cty.Value{
				"workload_policy": cty.ObjectVal(map[string]cty.Value{
					"accelerator_topology": cty.StringVal("2x2x2"),
					"type":                 cty.StringVal("HIGH_THROUGHPUT"),
				}),
			}),
		}
		bp.Groups = []Group{{Modules: []Module{wpMod}}}

		expandWorkloadPolicy(bp, &bp.Groups[0].Modules[0])

		wpVal := bp.Groups[0].Modules[0].Settings.Get("workload_policy")
		wpMap := wpVal.AsValueMap()
		modeVal, ok := wpMap["accelerator_topology_mode"]
		c.Assert(ok, Equals, true)
		exp, isExp := IsExpressionValue(modeVal)
		c.Assert(isExp, Equals, true)
		c.Check(string(exp.Tokenize().Bytes()), Equals, `(var.enable_dynamic_slicing_for_tpus)?"PROVISION_ONLY":null`)
		c.Check(strings.Contains(string(TokensForValue(wpVal).Bytes()), `accelerator_topology_mode = (var.enable_dynamic_slicing_for_tpus) ? "PROVISION_ONLY" : null`), Equals, true)

		// Evaluates to null when enable_dynamic_slicing_for_tpus is false
		evalVal, err := bp.Eval(wpVal)
		c.Assert(err, IsNil)
		c.Check(evalVal.AsValueMap()["accelerator_topology_mode"].IsNull(), Equals, true)

		// Evaluates to PROVISION_ONLY when enable_dynamic_slicing_for_tpus is true
		bp.Vars = bp.Vars.With("enable_dynamic_slicing_for_tpus", cty.BoolVal(true))
		evalValTrue, err := bp.Eval(wpVal)
		c.Assert(err, IsNil)
		c.Check(evalValTrue.AsValueMap()["accelerator_topology_mode"], DeepEquals, cty.StringVal("PROVISION_ONLY"))
	}

	// Case 2: Explicit accelerator_topology_mode set by user is preserved
	{
		bp := Blueprint{
			Vars: NewDict(map[string]cty.Value{
				"enable_dynamic_slicing_for_tpus": cty.BoolVal(true),
			}),
		}
		wpMod := Module{
			ID:     "workload_policy",
			Source: "modules/compute/resource-policy",
			Settings: NewDict(map[string]cty.Value{
				"workload_policy": cty.ObjectVal(map[string]cty.Value{
					"accelerator_topology":      cty.StringVal("2x2x2"),
					"type":                      cty.StringVal("HIGH_THROUGHPUT"),
					"accelerator_topology_mode": cty.StringVal("AUTO_CONNECT"),
				}),
			}),
		}
		bp.Groups = []Group{{Modules: []Module{wpMod}}}

		expandWorkloadPolicy(bp, &bp.Groups[0].Modules[0])

		wpMap := bp.Groups[0].Modules[0].Settings.Get("workload_policy").AsValueMap()
		c.Check(wpMap["accelerator_topology_mode"], DeepEquals, cty.StringVal("AUTO_CONNECT"))
	}

	// Case 3: Workload policy without accelerator_topology or with 2D GPU topology (e.g., 1x72) is untouched
	{
		bp := Blueprint{
			Vars: NewDict(map[string]cty.Value{
				"enable_dynamic_slicing_for_tpus": cty.BoolVal(true),
			}),
		}
		noTopoMod := Module{
			ID:     "gpu_distance_policy",
			Source: "modules/compute/resource-policy",
			Settings: NewDict(map[string]cty.Value{
				"workload_policy": cty.ObjectVal(map[string]cty.Value{
					"max_topology_distance": cty.StringVal("SUBBLOCK"),
					"type":                  cty.StringVal("HIGH_THROUGHPUT"),
				}),
			}),
		}
		gpuTopoMod := Module{
			ID:     "a4x_workload_policy",
			Source: "modules/compute/resource-policy",
			Settings: NewDict(map[string]cty.Value{
				"workload_policy": cty.ObjectVal(map[string]cty.Value{
					"accelerator_topology": cty.StringVal("1x72"),
					"type":                 cty.StringVal("HIGH_THROUGHPUT"),
				}),
			}),
		}
		bp.Groups = []Group{{Modules: []Module{noTopoMod, gpuTopoMod}}}

		expandWorkloadPolicy(bp, &bp.Groups[0].Modules[0])
		expandWorkloadPolicy(bp, &bp.Groups[0].Modules[1])

		_, existsNoTopo := bp.Groups[0].Modules[0].Settings.Get("workload_policy").AsValueMap()["accelerator_topology_mode"]
		c.Check(existsNoTopo, Equals, false)
		_, existsGpuTopo := bp.Groups[0].Modules[1].Settings.Get("workload_policy").AsValueMap()["accelerator_topology_mode"]
		c.Check(existsGpuTopo, Equals, false)
	}

	// Case 4: Blueprint without enable_dynamic_slicing_for_tpus leaves workload_policy untouched
	{
		bp := Blueprint{}
		wpMod := Module{
			ID:     "workload_policy",
			Source: "modules/compute/resource-policy",
			Settings: NewDict(map[string]cty.Value{
				"workload_policy": cty.ObjectVal(map[string]cty.Value{
					"accelerator_topology": cty.StringVal("2x2x2"),
					"type":                 cty.StringVal("HIGH_THROUGHPUT"),
				}),
			}),
		}
		bp.Groups = []Group{{Modules: []Module{wpMod}}}

		expandWorkloadPolicy(bp, &bp.Groups[0].Modules[0])

		wpMap := bp.Groups[0].Modules[0].Settings.Get("workload_policy").AsValueMap()
		_, exists := wpMap["accelerator_topology_mode"]
		c.Check(exists, Equals, false)
	}
}

func extractHintError(c *C, err error) HintError {
	c.Assert(err, NotNil)
	var hErr HintError
	c.Assert(errors.As(err, &hErr), Equals, true)
	return hErr
}

func reqInput(name, desc string) modulereader.VarInfo {
	return modulereader.VarInfo{Name: name, Type: cty.String, Description: desc, Required: true}
}

func (s *zeroSuite) TestValidateModuleInputs_BlueprintUseHints(c *C) {
	origFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = nil
	defer func() { sourcereader.ModuleFS = origFS }()

	// Single provider in earlier group preserves existing entries in Use
	net := tMod("net").outputs("network_id", "network_self_link").build()
	psa := tMod("psa").outputs("private_vpc_connection_peering").build()
	lustre := tMod("lustre").uses("net").inputs(reqInput("private_vpc_connection_peering", "VPC peering connection.")).build()
	bpSingle := Blueprint{Groups: []Group{{Name: "network", Modules: []Module{net, psa}}, {Name: "storage", Modules: []Module{lustre}}}}
	hSingle := extractHintError(c, validateModuleInputs(Root.Groups.At(1).Modules.At(0), lustre, bpSingle))
	c.Check(hSingle.Err.Error(), Equals, `a required setting "private_vpc_connection_peering" is missing from a module "lustre"`)
	c.Check(hSingle.Hint, Equals, `"private_vpc_connection_peering" description: VPC peering connection. Consider adding module "psa" to the "use" list of module "lustre" (e.g. use: [net, psa]).`)

	// Multiple providers across earlier and same unnamed group (including forward intra-group), multiline description, later group excluded
	gke1, gke2, gkeLate := tMod("gke1").outputs("cluster_id").build(), tMod("gke2").outputs("cluster_id").build(), tMod("gke_late").outputs("cluster_id").build()
	pool := tMod("pool").inputs(reqInput("cluster_id", "Cluster identifier.\n")).build()
	bpMulti := Blueprint{Groups: []Group{{Modules: []Module{gke1}}, {Modules: []Module{pool, gke2}}, {Modules: []Module{gkeLate}}}}
	hMulti := extractHintError(c, validateModuleInputs(Root.Groups.At(1).Modules.At(0), pool, bpMulti))
	c.Check(strings.HasPrefix(hMulti.Hint, "\"cluster_id\" description: Cluster identifier.\n"), Equals, true)
	c.Check(hMulti.Hint, Equals, "\"cluster_id\" description: Cluster identifier.\nConsider adding one of modules [\"gke1\", \"gke2\"] to the \"use\" list of module \"pool\".")

	// Multiple missing inputs on one module (generic "name" input gets no use: hint)
	multiMissing := tMod("multi_missing").inputs(
		reqInput("network_id", "VPC network ID."),
		reqInput("private_vpc_connection_peering", "Peering."),
		reqInput("name", "Instance name."),
	).build()
	msg := validateModuleInputs(Root.Groups.At(0).Modules.At(2), multiMissing, Blueprint{Groups: []Group{{Name: "primary", Modules: []Module{net, psa, multiMissing}}}}).Error()
	c.Check(strings.Contains(msg, `"network_id" description: VPC network ID. Consider adding module "net" to the "use" list of module "multi_missing" (e.g. use: [net]).`), Equals, true)
	c.Check(strings.Contains(msg, `"private_vpc_connection_peering" description: Peering. Consider adding module "psa" to the "use" list of module "multi_missing" (e.g. use: [psa]).`), Equals, true)
	c.Check(strings.Contains(msg, `"name" description: Instance name. Consider`), Equals, false)

	// Exclusions: Packer candidate, already-used, self, later group, and malformed modules
	pkrCand, already, late := tMod("pkr_cand").packer().outputs("special_out").build(), tMod("already").outputs("special_out").build(), tMod("late").outputs("special_out").build()
	consumer := tMod("consumer").uses("already").outputs("special_out").inputs(reqInput("special_out", "Special")).build()
	bpExcluded := Blueprint{Groups: []Group{
		{Name: "g0", Modules: []Module{pkrCand, already, {ID: "empty_src"}, {ID: "empty_src_with_out", Outputs: []modulereader.OutputInfo{{Name: "special_out"}}}, {ID: "bad_src", Source: "./does/not/exist"}, {ID: "bad_kind", Source: "modules/network/vpc", Kind: ModuleKind{kind: "bogus"}}}},
		{Name: "g1", Modules: []Module{consumer}},
		{Name: "g2", Modules: []Module{late}},
	}}
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(1).Modules.At(0), consumer, bpExcluded)).Hint, Equals, `"special_out" description: Special`)

	// Standalone consumer outside bp.Groups inspects all groups without panicking
	standalone := tMod("standalone").inputs(reqInput("special_out", "Special")).build()
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), standalone, Blueprint{Groups: []Group{{Modules: []Module{late}}}})).Hint,
		Equals, `"special_out" description: Special Consider adding module "late" to the "use" list of module "standalone" (e.g. use: [late]).`)

	// Packer consumer only considers strictly earlier groups (never same-group Terraform candidates)
	inlineNet := tMod("inline_net").outputs("subnetwork_name").build()
	sameGrpNet := tMod("same_grp_net").outputs("subnetwork_name").build()
	pkrImg := tMod("pkr_img").packer().inputs(reqInput("subnetwork_name", "Subnet")).build()
	bpPkr := Blueprint{Groups: []Group{{Name: "net", Modules: []Module{inlineNet}}, {Name: "img", Modules: []Module{sameGrpNet, pkrImg}}}}
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(1).Modules.At(1), pkrImg, bpPkr)).Hint,
		Equals, `"subnetwork_name" description: Subnet Consider adding module "inline_net" to the "use" list of module "pkr_img" (e.g. use: [inline_net]).`)
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(1), pkrImg, Blueprint{Groups: []Group{{Name: "img0", Modules: []Module{sameGrpNet, pkrImg}}}})).Hint,
		Equals, `"subnetwork_name" description: Subnet`)
}

func (s *zeroSuite) TestValidateModuleInputs_OutputFiltering(c *C) {
	origFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = nil
	defer func() { sourcereader.ModuleFS = origFS }()

	c.Check(IsAutoInjectedGlobal("project_id"), Equals, true)
	c.Check(IsAutoInjectedGlobal("cluster_id"), Equals, false)
	c.Check(IsGenericOutputName("capacity_gb"), Equals, true)
	c.Check(IsGenericOutputName("gcp_service_list"), Equals, true)
	c.Check(IsGenericOutputName("self_link"), Equals, true)
	c.Check(IsGenericOutputName("cluster_id"), Equals, false)
	c.Check(IsIntegrationInputName("network_storage"), Equals, true)
	c.Check(IsIntegrationInputName("cluster_id"), Equals, false)

	// Auto-injected globals and generic output names never receive a use: hint
	excludedNames := []string{
		"project_id", "deployment_name", "region", "zone", "labels",
		"capacity_gb", "gcp_service_list", "id", "instance_name", "name", "self_link",
	}
	noisy := tMod("noisy").outputs(excludedNames...).build()
	for _, name := range excludedNames {
		consumer := tMod("c_" + ModuleID(name)).inputs(reqInput(name, "desc")).build()
		bp := Blueprint{Groups: []Group{{Name: "g0", Modules: []Module{noisy, consumer}}}}
		c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(1), consumer, bp)).Hint, Equals, fmt.Sprintf("%q description: desc", name))
		c.Check(IsTrueProviderOutput(noisy.InfoOrDie(), name), Equals, false)
	}

	// Non-root module echoing an input is suppressed
	echoNodePool := tMod("node_pool").inputs(reqInput("cluster_id", "desc")).outputs("cluster_id").build()
	c.Check(IsTrueProviderOutput(echoNodePool.InfoOrDie(), "cluster_id"), Equals, false)

	// Root provider (0 required non-global/non-injected inputs) with optional override input IS a true provider
	rootVpcBuilder := tMod("vpc").
		inputs(
			reqInput("project_id", "desc"),
			reqInput("injected_id", "desc"),
			modulereader.VarInfo{Name: "subnetwork_name", Type: cty.String},
			modulereader.VarInfo{Name: "network_storage", Type: cty.String},
		).
		outputs("subnetwork_name", "network_storage", "startup_script")
	rootVpcBuilder.i.Metadata.Ghpc.InjectModuleId = "injected_id"
	rootVpc := rootVpcBuilder.build()

	c.Check(IsTrueProviderOutput(rootVpc.InfoOrDie(), "subnetwork_name"), Equals, true)
	c.Check(IsTrueProviderOutput(rootVpc.InfoOrDie(), "network_storage"), Equals, false)
	c.Check(IsTrueProviderOutput(rootVpc.InfoOrDie(), "startup_script"), Equals, true)
	c.Check(IsTrueProviderOutput(rootVpc.InfoOrDie(), "nonexistent_output"), Equals, false)

	// All integration inputs: root echo is suppressed, true producer without matching input is included
	for _, name := range []string{
		"network_storage", "partitions", "nodeset", "nodeset_dyn", "nodeset_tpu",
		"login_nodes", "runners", "startup_script", "compute_startup_script", "controller_startup_script",
	} {
		echoRoot := tMod("echo_" + ModuleID(name)).inputs(modulereader.VarInfo{Name: name, Type: cty.String}).outputs(name).build()
		c.Check(IsTrueProviderOutput(echoRoot.InfoOrDie(), name), Equals, false)

		trueProv := tMod("prov_" + ModuleID(name)).inputs(reqInput("other_req", "desc")).outputs(name).build()
		c.Check(IsTrueProviderOutput(trueProv.InfoOrDie(), name), Equals, true)
	}
}

func (s *zeroSuite) TestValidateModuleInputs_EmbeddedUseHints(c *C) {
	origFS := sourcereader.ModuleFS
	defer func() { sourcereader.ModuleFS = origFS }()

	// Nil ModuleFS produces plain description only
	sourcereader.ModuleFS = nil
	solo := tMod("solo").inputs(reqInput("unique_tok", "desc")).build()
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), solo, Blueprint{})).Hint, Equals, `"unique_tok" description: desc`)

	// Synthetic MapFS testing single/multi matches, Terraform vs Packer placement text, and skipping internal, deprecated, packer, and non-TF entries
	sourcereader.ModuleFS = fstest.MapFS{
		"modules/README.md":                           &fstest.MapFile{Data: []byte("# root readme")},
		"modules/testcat/notes.txt":                   &fstest.MapFile{Data: []byte("non-dir entry")},
		"modules/testcat/prov-a/subdir/nested.txt":    &fstest.MapFile{Data: []byte("nested dir")},
		"modules/testcat/prov-a/outputs.tf":           &fstest.MapFile{Data: []byte("output \"shared_tok\" { value = \"a\" }\noutput \"unique_tok\" { value = \"u\" }")},
		"community/modules/testcat/prov-b/outputs.tf": &fstest.MapFile{Data: []byte(`output "shared_tok" { value = "b" }`)},
		"community/modules/internal/hid/outputs.tf":   &fstest.MapFile{Data: []byte(`output "unique_tok" { value = "h" }`)},
		"modules/testcat/deprecated/outputs.tf":       &fstest.MapFile{Data: []byte(`output "unique_tok" { value = "d" }`)},
		"modules/testcat/deprecated/metadata.yaml":    &fstest.MapFile{Data: []byte("ghpc:\n  deprecation_date: \"2026-12-31\"\n")},
		"modules/testcat/packer-mod/main.pkr.hcl":     &fstest.MapFile{Data: []byte(`variable "x" { type = string }`)},
		"modules/testcat/packer-mod/outputs.tf":       &fstest.MapFile{Data: []byte(`output "unique_tok" { value = "p" }`)},
		"modules/testcat/empty-dir/README.md":         &fstest.MapFile{Data: []byte("# not terraform")},
	}

	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), solo, Blueprint{})).Hint,
		Equals, `"unique_tok" description: desc Module source "modules/testcat/prov-a" provides this output; add it to an earlier or same deployment group and reference its id in the "use" list of module "solo".`)

	pkrSingle := tMod("pkr_single").packer().inputs(reqInput("unique_tok", "desc")).build()
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), pkrSingle, Blueprint{})).Hint,
		Equals, `"unique_tok" description: desc Module source "modules/testcat/prov-a" provides this output; add it to an earlier deployment group and reference its id in the "use" list of module "pkr_single".`)

	tfMulti := tMod("tf_multi").inputs(reqInput("shared_tok", "desc")).build()
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), tfMulti, Blueprint{})).Hint,
		Equals, `"shared_tok" description: desc Module sources ["community/modules/testcat/prov-b", "modules/testcat/prov-a"] provide this output; add one to an earlier or same deployment group and reference its id in the "use" list of module "tf_multi".`)

	pkrMulti := tMod("pkr_multi").packer().inputs(reqInput("shared_tok", "desc")).build()
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), pkrMulti, Blueprint{})).Hint,
		Equals, `"shared_tok" description: desc Module sources ["community/modules/testcat/prov-b", "modules/testcat/prov-a"] provide this output; add one to an earlier deployment group and reference its id in the "use" list of module "pkr_multi".`)

	// Fallback to embedded catalog when the only in-blueprint provider is in a later group
	lateProv := tMod("late_prov").outputs("unique_tok").build()
	bpLateOnly := Blueprint{Groups: []Group{{Name: "early", Modules: []Module{solo}}, {Name: "later", Modules: []Module{lateProv}}}}
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), solo, bpLateOnly)).Hint,
		Equals, `"unique_tok" description: desc Module source "modules/testcat/prov-a" provides this output; add it to an earlier or same deployment group and reference its id in the "use" list of module "solo".`)

	// Self-source exclusion (exact, ./ prefix, backslash, and transformSource-rewritten) vs third-party colliding subpath
	bpRewritten := Blueprint{ToolkitModulesURL: "https://github.com/my-fork/cluster-toolkit", ToolkitModulesVersion: "v1.0.0"}
	for _, selfSrc := range []string{"modules/testcat/prov-a", "./modules/testcat/prov-a", `modules\testcat\prov-a`, bpRewritten.transformSource("modules/testcat/prov-a")} {
		c.Check(findEmbeddedProviders("unique_tok", Module{ID: "self_m", Source: selfSrc}, bpRewritten), HasLen, 0)
	}
	c.Check(findEmbeddedProviders("unique_tok", Module{ID: "third_party", Source: "github.com/my-org/custom//modules/testcat/prov-a?ref=main"}, bpRewritten),
		DeepEquals, []string{"modules/testcat/prov-a"})

	// Real embedded modules
	sourcereader.ModuleFS = os.DirFS("../..").(sourcereader.BaseFS)
	lustre := tMod("lustre").inputs(reqInput("private_vpc_connection_peering", "desc")).build()
	c.Check(extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), lustre, Blueprint{})).Hint,
		Equals, `"private_vpc_connection_peering" description: desc Module source "modules/network/private-service-access" provides this output; add it to an earlier or same deployment group and reference its id in the "use" list of module "lustre".`)

	pool := tMod("pool").inputs(reqInput("cluster_id", "desc")).build()
	hCluster := extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), pool, Blueprint{})).Hint
	c.Check(strings.Contains(hCluster, `"modules/scheduler/gke-cluster"`), Equals, true)
	c.Check(strings.Contains(hCluster, `"modules/scheduler/pre-existing-gke-cluster"`), Equals, true)
	c.Check(strings.Contains(hCluster, `add one to an earlier or same deployment group and reference its id in the "use" list of module "pool".`), Equals, true)

	img := tMod("img").packer().inputs(reqInput("subnetwork_name", "desc")).build()
	hSubnet := extractHintError(c, validateModuleInputs(Root.Groups.At(0).Modules.At(0), img, Blueprint{})).Hint
	c.Check(strings.Contains(hSubnet, `"modules/network/vpc"`), Equals, true)
	c.Check(strings.Contains(hSubnet, "add one to an earlier deployment group"), Equals, true)
	c.Check(strings.Contains(hSubnet, "or same"), Equals, false)
}

func (s *zeroSuite) TestPrivateServiceAccessHasToBeUsed(c *C) {
	origFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = os.DirFS("../..").(sourcereader.BaseFS)
	defer func() { sourcereader.ModuleFS = origFS }()

	info, err := modulereader.GetModuleInfo("modules/network/private-service-access", TerraformKind.String())
	c.Assert(err, IsNil)
	c.Check(info.Metadata.Ghpc.HasToBeUsed, Equals, true)

	baseVars := NewDict(map[string]cty.Value{
		"deployment_name": cty.StringVal("psa-test"),
		"project_id":      cty.StringVal("test-project"),
		"region":          cty.StringVal("us-central1"),
		"zone":            cty.StringVal("us-central1-a"),
	})
	lustreSettings := NewDict(map[string]cty.Value{
		"name":         cty.StringVal("my-lustre"),
		"size_gib":     cty.NumberIntVal(18000),
		"local_mount":  cty.StringVal("/lustre"),
		"remote_mount": cty.StringVal("lustrefs"),
	})
	mkBP := func(mods ...Module) *Blueprint {
		return &Blueprint{BlueprintName: "psa-test", Vars: baseVars, Groups: []Group{{Name: "primary", Modules: mods}}}
	}
	net := Module{ID: "net", Source: "modules/network/vpc"}
	psa := Module{ID: "psa", Source: "modules/network/private-service-access", Use: ModuleIDs{"net"}}

	// Unwired private-service-access fails validateModulesAreUsed
	err = mkBP(net, psa).Expand()
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), `module "psa" was not used`), Equals, true)

	// Consumer listed before psa without psa in Use suggests use: [net, psa]
	err = mkBP(Module{ID: "lustre", Source: "modules/file-system/managed-lustre", Use: ModuleIDs{"net"}, Settings: lustreSettings}, net, psa).Expand()
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), `Consider adding module "psa" to the "use" list of module "lustre" (e.g. use: [net, psa]).`), Equals, true)

	// Applying forward intra-group use: [net, psa] expands cleanly and satisfies HasToBeUsed
	c.Check(mkBP(Module{ID: "lustre", Source: "modules/file-system/managed-lustre", Use: ModuleIDs{"net", "psa"}, Settings: lustreSettings}, net, psa).Expand(), IsNil)
}

func (s *zeroSuite) TestModuleInfo(c *C) {
	origFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = os.DirFS("../..").(sourcereader.BaseFS)
	defer func() { sourcereader.ModuleFS = origFS }()

	// Empty Source returns EmptyModuleSource and InfoOrDie panics even when blueprint Outputs are populated
	for _, m := range []Module{
		{ID: "empty"},
		{ID: "empty_with_outputs", Outputs: []modulereader.OutputInfo{{Name: "out1"}}},
	} {
		_, err := m.Info()
		c.Check(errors.Is(err, EmptyModuleSource), Equals, true)
		mod := m
		c.Check(func() { mod.InfoOrDie() }, PanicMatches, ".*source.*")
	}

	// Invalid ModuleKind returns InvalidModuleKind and InfoOrDie panics
	badKind := Module{ID: "bad", Source: "modules/network/vpc", Kind: ModuleKind{kind: "bogus"}}
	_, err := badKind.Info()
	c.Check(errors.Is(err, InvalidModuleKind), Equals, true)
	c.Check(func() { badKind.InfoOrDie() }, PanicMatches, ".*kind.*")

	// UnknownKind defaults to TerraformKind
	unk := tMod("unk_kind_mod").outputs("custom_out").build()
	unk.Kind = UnknownKind
	miUnk, err := unk.Info()
	c.Assert(err, IsNil)
	c.Check(miUnk.Outputs, HasLen, 1)

	// Third-party remote source with colliding //modules/... subpath preserves remote error without hijacking embedded FS
	colliding := Module{
		ID:     "custom_vpc",
		Source: "invalid::github.com/my-org/custom-infra//modules/network/vpc?ref=bad-ref",
		Kind:   TerraformKind,
	}
	_, err = colliding.Info()
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "unsupported module source protocol"), Equals, true)
	c.Check(strings.Contains(err.Error(), "embedded module"), Equals, false)
	c.Check(func() { colliding.InfoOrDie() }, PanicMatches, ".*unsupported module source protocol.*")
}
