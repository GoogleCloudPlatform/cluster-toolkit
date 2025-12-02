// Copyright 2022 Google LLC
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
	"fmt"
	"hpc-toolkit/pkg/modulereader"

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
				Version: ">= 6.9.0, <= 7.11.0"},
			"google-beta": TerraformProvider{
				Source:  "hashicorp/google-beta",
				Version: ">= 6.9.0, <= 7.11.0"}})
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
	b := Module{ID: "moduleB", Source: "community/modules/file-system/DDN-EXAScaler"}
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
	c.Assert(bp.Groups[0].Modules[1].Source, Equals, "community/modules/file-system/DDN-EXAScaler")
	c.Assert(bp.Groups[1].Modules[0].Source, Equals, "./modules/network/pre-existing-vpc")

	// toolkit_modules_url and toolkit_modules_version provided
	bp = Blueprint{
		Groups: dg, ToolkitModulesURL: "github.com/GoogleCloudPlatform/cluster-toolkit", ToolkitModulesVersion: "v1.15.0",
	}
	bp.substituteModuleSources()
	// Check that embedded sources (a and b) are transformed correctly
	expectedSourceA := "github.com/GoogleCloudPlatform/cluster-toolkit//modules/network/pre-existing-vpc?ref=v1.15.0&depth=1"
	expectedSourceB := "github.com/GoogleCloudPlatform/cluster-toolkit//community/modules/file-system/DDN-EXAScaler?ref=v1.15.0&depth=1"
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
