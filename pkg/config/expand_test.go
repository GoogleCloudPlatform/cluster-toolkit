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

func (s *MySuite) TestExpand(c *C) {
	dc := getDeploymentConfigForTest()
	fmt.Println("TEST_DEBUG: If tests die without report, check TestExpand")
	dc.expand()
}

func (s *MySuite) TestExpandBackends(c *C) {
	dc := getDeploymentConfigForTest()
	deplName := dc.Config.Vars.Get("deployment_name").AsString()

	// Simple test: Does Nothing
	err := dc.expandBackends()
	c.Assert(err, IsNil)

	dc.Config.TerraformBackendDefaults = TerraformBackend{Type: "gcs"}
	err = dc.expandBackends()
	c.Assert(err, IsNil)
	grp := dc.Config.DeploymentGroups[0]
	c.Assert(grp.TerraformBackend.Type, Not(Equals), "")
	gotPrefix := grp.TerraformBackend.Configuration.Get("prefix")
	expPrefix := fmt.Sprintf("%s/%s/%s", dc.Config.BlueprintName, deplName, grp.Name)
	c.Assert(gotPrefix, Equals, cty.StringVal(expPrefix))

	// Add a new resource group, ensure each group name is included
	newGroup := DeploymentGroup{
		Name: "group2",
	}
	dc.Config.DeploymentGroups = append(dc.Config.DeploymentGroups, newGroup)
	err = dc.expandBackends()
	c.Assert(err, IsNil)
	newGrp := dc.Config.DeploymentGroups[1]
	c.Assert(newGrp.TerraformBackend.Type, Not(Equals), "")
	gotPrefix = newGrp.TerraformBackend.Configuration.Get("prefix")
	expPrefix = fmt.Sprintf("%s/%s/%s", dc.Config.BlueprintName, deplName, newGrp.Name)
	c.Assert(gotPrefix, Equals, cty.StringVal(expPrefix))
}

func (s *MySuite) TestAddListValue(c *C) {
	mod := Module{ID: "TestModule"}

	setting := "newSetting"
	nonListSetting := "not-a-list"
	first := cty.StringVal("value1")
	second := cty.StringVal("value2")

	c.Assert(mod.addListValue(setting, first), IsNil)
	c.Check(mod.Settings.Get(setting), DeepEquals, cty.TupleVal([]cty.Value{first}))

	c.Assert(mod.addListValue(setting, second), IsNil)
	c.Check(mod.Settings.Get(setting), DeepEquals, cty.TupleVal([]cty.Value{first, second}))

	mod.Settings.Set(nonListSetting, cty.StringVal("string-value"))
	c.Assert(mod.addListValue(nonListSetting, second), NotNil)
}

func (s *MySuite) TestUseModule(c *C) {
	// Setup
	usedMod := Module{
		ID:     "UsedModule",
		Source: "usedSource",
	}
	varInfoNumber := modulereader.VarInfo{
		Name: "val1",
		Type: "number",
	}
	ref := ModuleRef("UsedModule", "val1").AsExpression().AsValue()
	useMark := ProductOfModuleUse{"UsedModule"}

	{ // Pass: No Inputs, No Outputs
		mod := Module{ID: "lime", Source: "modSource"}

		setTestModuleInfo(mod, modulereader.ModuleInfo{})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{})

		err := useModule(&mod, usedMod, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Pass: Has Output, no matching input
		mod := Module{ID: "lime", Source: "limeTree"}

		setTestModuleInfo(mod, modulereader.ModuleInfo{})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})
		err := useModule(&mod, usedMod, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Pass: Single Input/Output match - no lists
		mod := Module{ID: "lime", Source: "limeTree"}
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{varInfoNumber},
		})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		err := useModule(&mod, usedMod, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": ref.Mark(useMark),
		})
	}

	{ // Pass: Single Input/Output match - but setting was in blueprint so no-op
		mod := Module{ID: "lime", Source: "limeTree"}
		mod.Settings.Set("val1", ref)
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{varInfoNumber},
		})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		err := useModule(&mod, usedMod, []string{"val1"})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{"val1": ref})
	}

	{ // Pass: re-apply used modules, should be a no-op
		// Assume no settings were in blueprint
		mod := Module{ID: "lime", Source: "limeTree"}
		mod.Settings.Set("val1", ref.Mark(useMark))
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{varInfoNumber},
		})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		err := useModule(&mod, usedMod, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{"val1": ref.Mark(useMark)})
	}

	{ // Pass: Single Input/Output match, input is list, not already set
		mod := Module{ID: "lime", Source: "limeTree"}
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{{Name: "val1", Type: "list"}},
		})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})
		err := useModule(&mod, usedMod, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": cty.TupleVal([]cty.Value{
				ref.Mark(useMark),
			})})
	}

	{ // Pass: Setting exists, Input is List, Output is not a list
		// Assume setting was not set in blueprint
		mod := Module{ID: "lime", Source: "limeTree"}
		mod.Settings.Set("val1", cty.TupleVal([]cty.Value{ref}))
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{{Name: "val1", Type: "list"}},
		})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		err := useModule(&mod, usedMod, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": cty.TupleVal([]cty.Value{
				ref,
				ref.Mark(useMark),
			})})
	}

	{ // Pass: Setting exists, Input is List, Output is not a list
		// Assume setting was set in blueprint
		mod := Module{ID: "lime", Source: "limeTree"}
		mod.Settings.Set("val1", cty.TupleVal([]cty.Value{ref}))
		setTestModuleInfo(mod, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{{Name: "val1", Type: "list"}},
		})
		setTestModuleInfo(usedMod, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{{Name: "val1"}},
		})

		err := useModule(&mod, usedMod, []string{"val1"})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": cty.TupleVal([]cty.Value{ref})})
	}
}

func (s *MySuite) TestApplyUseModules(c *C) {

	{ // Simple Case
		dc := getDeploymentConfigForTest()
		c.Assert(dc.applyUseModules(), IsNil)
	}
	{ // Has Use Modules
		dc := getDeploymentConfigForTest()
		g := &dc.Config.DeploymentGroups[0]

		using := Module{
			ID:     "usingModule",
			Source: "path/using",
			Use:    []ModuleID{"usedModule"},
		}
		used := Module{ID: "usedModule", Source: "path/used"}

		g.Modules = append(g.Modules, using, used)

		setTestModuleInfo(using, modulereader.ModuleInfo{
			Inputs: []modulereader.VarInfo{{
				Name: "potato",
				Type: "number",
			}}})
		setTestModuleInfo(used, modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{
				{Name: "potato"}}})

		c.Assert(dc.applyUseModules(), IsNil)

		// Use ID doesn't exists (fail)
		g.Modules[len(g.Modules)-1].ID = "wrongID"
		c.Assert(dc.applyUseModules(), ErrorMatches, fmt.Sprintf("%s: %s", errorMessages["invalidMod"], used.ID))
	}

	{ // test multigroup deployment with config that has a known good match
		dc := getMultiGroupDeploymentConfig()
		m := &dc.Config.DeploymentGroups[1].Modules[0]
		c.Assert(m.Settings, DeepEquals, Dict{})
		c.Assert(dc.applyUseModules(), IsNil)
		ref := ModuleRef("TestModule0", "test_inter_0").AsExpression().AsValue()
		c.Assert(m.Settings.Items(), DeepEquals, map[string]cty.Value{
			"test_inter_0": ref.Mark(ProductOfModuleUse{"TestModule0"}),
		})
	}

	{ // Deliberately break the match and see that no settings are added
		dc := getMultiGroupDeploymentConfig()
		mod := &dc.Config.DeploymentGroups[1].Modules[0]
		c.Assert(mod.Settings, DeepEquals, Dict{})

		// this eliminates the matching output from the used module
		setTestModuleInfo(*mod, modulereader.ModuleInfo{})

		c.Assert(dc.applyUseModules(), IsNil)
		c.Assert(mod.Settings, DeepEquals, Dict{})
	}
}

func (s *MySuite) TestCombineLabels(c *C) {
	infoWithLabels := modulereader.ModuleInfo{Inputs: []modulereader.VarInfo{{Name: "labels"}}}

	coral := Module{
		Source: "blue/salmon",
		Kind:   TerraformKind,
		ID:     "coral",
		Settings: NewDict(map[string]cty.Value{
			"labels": cty.ObjectVal(map[string]cty.Value{
				"magenta":   cty.StringVal("orchid"),
				"ghpc_role": cty.StringVal("maroon"),
			}),
		}),
	}
	setTestModuleInfo(coral, infoWithLabels)

	// has no labels set
	khaki := Module{Source: "brown/oak", Kind: TerraformKind, ID: "khaki"}
	setTestModuleInfo(khaki, infoWithLabels)

	// has no labels set, also module has no labels input
	silver := Module{Source: "ivory/black", Kind: TerraformKind, ID: "silver"}
	setTestModuleInfo(silver, modulereader.ModuleInfo{Inputs: []modulereader.VarInfo{}})

	orange := Module{Source: "red/velvet", Kind: PackerKind, ID: "orange", Settings: NewDict(map[string]cty.Value{
		"labels": cty.ObjectVal(map[string]cty.Value{
			"olive":           cty.StringVal("teal"),
			"ghpc_deployment": cty.StringVal("navy"),
		}),
	})}
	setTestModuleInfo(orange, infoWithLabels)

	dc := DeploymentConfig{
		Config: Blueprint{
			BlueprintName: "simple",
			Vars: NewDict(map[string]cty.Value{
				"deployment_name": cty.StringVal("golden"),
			}),
			DeploymentGroups: []DeploymentGroup{
				{Name: "lime", Modules: []Module{coral, khaki, silver}},
				{Name: "pink", Modules: []Module{orange}},
			},
		},
	}
	c.Check(dc.combineLabels(), IsNil)

	// Were global labels created?
	c.Check(dc.Config.Vars.Get("labels"), DeepEquals, cty.ObjectVal(map[string]cty.Value{
		"ghpc_blueprint":  cty.StringVal("simple"),
		"ghpc_deployment": cty.StringVal("golden"),
	}))

	labelsRef := GlobalRef("labels").AsExpression().AsValue()

	lime := dc.Config.DeploymentGroups[0]
	// Labels are set and override role
	coral = lime.Modules[0]
	c.Check(coral.WrapSettingsWith["labels"], DeepEquals, []string{"merge(", ")"})
	c.Check(coral.Settings.Get("labels"), DeepEquals, cty.TupleVal([]cty.Value{
		labelsRef,
		cty.ObjectVal(map[string]cty.Value{
			"magenta":   cty.StringVal("orchid"),
			"ghpc_role": cty.StringVal("maroon"),
		}),
	}))
	// Labels are not set, infer role from module.source
	khaki = lime.Modules[1]
	c.Check(khaki.WrapSettingsWith["labels"], DeepEquals, []string{"merge(", ")"})
	c.Check(khaki.Settings.Get("labels"), DeepEquals, cty.TupleVal([]cty.Value{
		labelsRef,
		cty.ObjectVal(map[string]cty.Value{
			"ghpc_role": cty.StringVal("brown")}),
	}))
	// No labels input
	silver = lime.Modules[2]
	c.Check(silver.WrapSettingsWith["labels"], IsNil)
	c.Check(silver.Settings.Get("labels"), DeepEquals, cty.NilVal)

	// Packer, include global include explicitly
	// Keep overridden ghpc_deployment=navy
	orange = dc.Config.DeploymentGroups[1].Modules[0]
	c.Check(orange.WrapSettingsWith["labels"], IsNil)
	c.Check(orange.Settings.Get("labels"), DeepEquals, cty.ObjectVal(map[string]cty.Value{
		"ghpc_blueprint":  cty.StringVal("simple"),
		"ghpc_deployment": cty.StringVal("navy"),
		"ghpc_role":       cty.StringVal("red"),
		"olive":           cty.StringVal("teal"),
	}))
}

func (s *MySuite) TestApplyGlobalVariables(c *C) {
	dc := getDeploymentConfigForTest()
	mod := &dc.Config.DeploymentGroups[0].Modules[0]

	// Test no inputs, none required
	c.Check(dc.applyGlobalVariables(), IsNil)

	// Test no inputs, one required, doesn't exist in globals
	setTestModuleInfo(*mod, modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{{
			Name:     "gold",
			Type:     "string",
			Required: true,
		}},
	})

	err := dc.applyGlobalVariables()
	expectedErrorStr := fmt.Sprintf("%s: Module ID: %s Setting: gold",
		errorMessages["missingSetting"], mod.ID)
	c.Check(err, ErrorMatches, expectedErrorStr)

	// Test no input, one required, exists in globals
	dc.Config.Vars.Set("gold", cty.StringVal("val"))
	c.Check(dc.applyGlobalVariables(), IsNil)
	c.Assert(
		mod.Settings.Get("gold"),
		DeepEquals,
		GlobalRef("gold").AsExpression().AsValue())

	// Test one input, one required
	mod.Settings.Set(requiredVar.Name, cty.StringVal("val"))
	err = dc.applyGlobalVariables()
	c.Assert(err, IsNil)

	// Test one input, none required, exists in globals
	setTestModuleInfo(*mod, modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{{
			Name:     "gold",
			Type:     "string",
			Required: false,
		}},
	})
	err = dc.applyGlobalVariables()
	c.Assert(err, IsNil)
}

func (s *MySuite) TestIsSimpleVariable(c *C) {
	// True: Correct simple variable
	got := isSimpleVariable("$(some_text)")
	c.Assert(got, Equals, true)
	// False: Missing $
	got = isSimpleVariable("(some_text)")
	c.Assert(got, Equals, false)
	// False: Missing (
	got = isSimpleVariable("$some_text)")
	c.Assert(got, Equals, false)
	// False: Missing )
	got = isSimpleVariable("$(some_text")
	c.Assert(got, Equals, false)
	// False: Contains Prefix
	got = isSimpleVariable("prefix-$(some_text)")
	c.Assert(got, Equals, false)
	// False: Contains Suffix
	got = isSimpleVariable("$(some_text)-suffix")
	c.Assert(got, Equals, false)
	// False: Contains prefix and suffix
	got = isSimpleVariable("prefix-$(some_text)-suffix")
	c.Assert(got, Equals, false)
	// False: empty string
	got = isSimpleVariable("")
	c.Assert(got, Equals, false)
}

func (s *MySuite) TestHasVariable(c *C) {
	// True: simple variable
	got := hasVariable("$(some_text)")
	c.Assert(got, Equals, true)
	// True: has prefix
	got = hasVariable("prefix-$(some_text)")
	c.Assert(got, Equals, true)
	// True: has suffix
	got = hasVariable("$(some_text)-suffix")
	c.Assert(got, Equals, true)
	// True: Two variables
	got = hasVariable("$(some_text)$(some_more)")
	c.Assert(got, Equals, true)
	// True: two variable with other text
	got = hasVariable("prefix-$(some_text)-$(some_more)-suffix")
	c.Assert(got, Equals, true)
	// False: missing $
	got = hasVariable("(some_text)")
	c.Assert(got, Equals, false)
	// False: missing (
	got = hasVariable("$some_text)")
	c.Assert(got, Equals, false)
	// False: missing )
	got = hasVariable("$(some_text")
	c.Assert(got, Equals, false)
}

func (s *MySuite) TestValidateModuleReference(c *C) {
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
		c.Check(err, ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["intergroupOrder"]))
	}

	// A target module that doesn't exist (bad)
	c.Check(validateModuleReference(bp, y, "bad-module"), NotNil)

	// Reference packer module (bad)
	c.Check(validateModuleReference(bp, y, pkr.ID), NotNil)

}

func (s *MySuite) TestIntersection(c *C) {
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

func (s *MySuite) TestOutputNamesByGroup(c *C) {
	dc := getMultiGroupDeploymentConfig()
	dc.applyGlobalVariables()
	dc.applyUseModules()

	group0 := dc.Config.DeploymentGroups[0]
	mod0 := group0.Modules[0]
	group1 := dc.Config.DeploymentGroups[1]

	outputNamesGroup0, err := OutputNamesByGroup(group0, dc)
	c.Assert(err, IsNil)
	c.Assert(outputNamesGroup0, DeepEquals, map[GroupName][]string{})

	outputNamesGroup1, err := OutputNamesByGroup(group1, dc)
	c.Assert(err, IsNil)
	c.Assert(outputNamesGroup1, DeepEquals, map[GroupName][]string{
		group0.Name: {AutomaticOutputName("test_inter_0", mod0.ID)},
	})
}
