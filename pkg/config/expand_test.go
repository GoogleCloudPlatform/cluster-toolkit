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
		g := DeploymentGroup{Name: "clown"}
		noDefBe.expandBackend(&g)
		c.Check(g.TerraformBackend, DeepEquals, BE{})
	}

	{ // no def BE, group BE
		g := DeploymentGroup{
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
		g := DeploymentGroup{Name: "clown"}
		defBe.expandBackend(&g)

		c.Check(g.TerraformBackend, DeepEquals, BE{ // no change
			Type: "gcs",
			Configuration: NewDict(map[string]cty.Value{
				"prefix": MustParseExpression(`"tree/${var.deployment_name}/clown"`).AsValue(),
				"leave":  cty.StringVal("fall")})})
	}

	{ // def BE, group BE non-gcs
		g := DeploymentGroup{
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
	used := Module{
		ID:     "UsedModule",
		Source: "usedSource",
	}
	varInfoNumber := modulereader.VarInfo{Name: "val1", Type: cty.Number}
	ref := ModuleRef("UsedModule", "val1").AsValue()

	{ // Pass: No Inputs, No Outputs
		mod := Module{ID: "lime", Source: "modSource"}

		setTestModuleInfo(mod, modulereader.ModuleInfo{})
		setTestModuleInfo(used, modulereader.ModuleInfo{})

		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Pass: Has Output, no matching input
		mod := Module{ID: "lime", Source: "limeTree"}

		setTestModuleInfo(mod, modulereader.ModuleInfo{})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})
		useModule(&mod, used)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Pass: Single Input/Output match - no lists
		mod := Module{ID: "lime", Source: "limeTree"}
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{varInfoNumber},
		})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		useModule(&mod, used)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": AsProductOfModuleUse(ref, "UsedModule"),
		})
	}

	{ // Pass: Single Input/Output match - but setting was in blueprint so no-op
		mod := Module{
			ID:       "lime",
			Settings: Dict{}.With("val1", ref),
			Source:   "limeTree"}
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{varInfoNumber},
		})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		useModule(&mod, used)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{"val1": ref})
	}

	{ // Pass: re-apply used modules, should be a no-op
		// Assume no settings were in blueprint
		mod := Module{
			ID:       "lime",
			Settings: Dict{}.With("val1", AsProductOfModuleUse(ref, "UsedModule")),
			Source:   "limeTree"}
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{varInfoNumber},
		})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		useModule(&mod, used)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": AsProductOfModuleUse(ref, "UsedModule")})
	}

	{ // Pass: Single Input/Output match, input is list, not already set
		mod := Module{ID: "lime", Source: "limeTree"}
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{{Name: "val1", Type: cty.List(cty.Number)}},
		})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})
		useModule(&mod, used)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": AsProductOfModuleUse(
				MustParseExpression(`flatten([module.UsedModule.val1])`).AsValue(),
				"UsedModule")})
	}

	{ // Pass: Setting exists, Input is List, Output is not a list
		// Assume setting was not set in blueprint
		mod := Module{
			ID:       "lime",
			Settings: Dict{}.With("val1", AsProductOfModuleUse(cty.TupleVal([]cty.Value{ref}), "other")),
			Source:   "limeTree"}

		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{{Name: "val1", Type: cty.List(cty.Number)}},
		})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		useModule(&mod, used)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": AsProductOfModuleUse(
				MustParseExpression(`flatten([module.UsedModule.val1,[module.UsedModule.val1]])`).AsValue(),
				"other", "UsedModule")})
	}

	{ // Pass: Setting exists, Input is List, Output is not a list
		// Assume setting was set in blueprint
		mod := Module{
			ID:       "lime",
			Settings: Dict{}.With("val1", cty.TupleVal([]cty.Value{ref})),
			Source:   "limeTree"}
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{{Name: "val1", Type: cty.List(cty.Number)}},
		})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		useModule(&mod, used)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": cty.TupleVal([]cty.Value{ref})})
	}
}

func (s *zeroSuite) TestExpandModule(c *C) {
	u := Module{ID: "potato", Source: c.TestName() + "/potato"}
	setTestModuleInfo(u, modulereader.ModuleInfo{
		Outputs: []modulereader.OutputInfo{
			{Name: "az"},
			{Name: "rose"},
			{Name: "peony"}}})

	m := Module{
		ID:     "yarn",
		Source: c.TestName() + "/yarn",
		Use:    ModuleIDs{u.ID},
		Settings: NewDict(map[string]cty.Value{
			"az": cty.StringVal("alpha"),
		})}
	setTestModuleInfo(m, modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{
			{Name: "az"},     // set explicitly
			{Name: "buki"},   // set as global var
			{Name: "labels"}, // set as global var
			{Name: "rose", Type: cty.List(cty.String)}, // used from `u`
			{Name: "peony"},  // used from `u`, global var ignored
			{Name: "orchid"}, // not present anywhere
		},
	})

	bp := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"labels": cty.EmptyObjectVal,
			"az":     cty.StringVal("za"),   // will be ignored
			"buki":   cty.StringVal("ikub"), // will be used
			"vedi":   cty.StringVal("idav"), // not in module inputs
			"peon":   cty.StringVal("noep"), // will be ignored
		}),
		DeploymentGroups: []DeploymentGroup{
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
	mod := Module{
		ID:     "carrot",
		Source: c.TestName() + "/cabbage",
		Kind:   TerraformKind,
		Settings: NewDict(map[string]cty.Value{
			"silver": cty.StringVal("glagol")})}

	vars := NewDict(map[string]cty.Value{
		"polonium": cty.StringVal("az"),
		"pyrite":   cty.StringVal("buki"),
		"silver":   cty.StringVal("vedi"),
	})

	setTestModuleInfo(mod, modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{
			{Name: "gold"},   // doesn't exist in vars
			{Name: "pyrite"}, // exists in vars, not set in module
			{Name: "silver"}, // exists in vars, set in module
			{Name: "helium"}, // to be set to ModuleID
		},
		Metadata: modulereader.Metadata{
			Ghpc: modulereader.MetadataGhpc{
				InjectModuleId: "helium"}}})

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

	dg := []DeploymentGroup{
		{Name: "zero", Modules: []Module{a, b}},
		{Name: "half", Modules: []Module{pkr}},
		{Name: "one", Modules: []Module{y}},
	}

	bp := Blueprint{
		DeploymentGroups: dg,
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
	zebra := DeploymentGroup{
		Name: "zebra",
		Modules: []Module{
			{
				ID: "stripes",
				Outputs: []modulereader.OutputInfo{
					{Name: "length"}}}}}
	pony := DeploymentGroup{
		Name: "pony",
		Modules: []Module{
			{
				ID: "bucephalus",
				Settings: NewDict(map[string]cty.Value{
					"width": ModuleRef("stripes", "length").AsValue()})}}}
	bp := Blueprint{DeploymentGroups: []DeploymentGroup{zebra, pony}}

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
