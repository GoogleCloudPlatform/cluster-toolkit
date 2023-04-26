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
		err := useModule(&mod, usedMod, nil /*modInputs*/, nil /*usedModOutputs*/, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Pass: Has Output, no matching input
		mod := Module{ID: "lime", Source: "limeTree"}
		usedOutputs := []modulereader.OutputInfo{{Name: "val1"}}
		err := useModule(&mod, usedMod, nil /*modInputs*/, usedOutputs, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings, DeepEquals, Dict{})
	}

	{ // Pass: Single Input/Output match - no lists
		mod := Module{ID: "lime", Source: "limeTree"}
		modInputs := []modulereader.VarInfo{varInfoNumber}
		usedOutputs := []modulereader.OutputInfo{{Name: "val1"}}

		err := useModule(&mod, usedMod, modInputs, usedOutputs, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": ref.Mark(useMark),
		})
	}

	{ // Pass: Single Input/Output match - but setting was in blueprint so no-op
		mod := Module{ID: "lime", Source: "limeTree"}
		mod.Settings.Set("val1", ref)
		modInputs := []modulereader.VarInfo{varInfoNumber}
		usedOutputs := []modulereader.OutputInfo{{Name: "val1"}}

		err := useModule(&mod, usedMod, modInputs, usedOutputs, []string{"val1"})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{"val1": ref})
	}

	{ // Pass: re-apply used modules, should be a no-op
		// Assume no settings were in blueprint
		mod := Module{ID: "lime", Source: "limeTree"}
		mod.Settings.Set("val1", ref.Mark(useMark))
		modInputs := []modulereader.VarInfo{varInfoNumber}
		usedOutputs := []modulereader.OutputInfo{{Name: "val1"}}

		err := useModule(&mod, usedMod, modInputs, usedOutputs, []string{})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{"val1": ref.Mark(useMark)})
	}

	{ // Pass: Single Input/Output match, input is list, not already set
		mod := Module{ID: "lime", Source: "limeTree"}
		modInputs := []modulereader.VarInfo{{Name: "val1", Type: "list"}}
		usedOutputs := []modulereader.OutputInfo{{Name: "val1"}}

		err := useModule(&mod, usedMod, modInputs, usedOutputs, []string{})
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
		modInputs := []modulereader.VarInfo{{Name: "val1", Type: "list"}}
		usedOutputs := []modulereader.OutputInfo{{Name: "val1"}}

		err := useModule(&mod, usedMod, modInputs, usedOutputs, []string{})
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
		modInputs := []modulereader.VarInfo{{Name: "val1", Type: "list"}}
		usedOutputs := []modulereader.OutputInfo{{Name: "val1"}}

		err := useModule(&mod, usedMod, modInputs, usedOutputs, []string{"val1"})
		c.Check(err, IsNil)
		c.Check(mod.Settings.Items(), DeepEquals, map[string]cty.Value{
			"val1": cty.TupleVal([]cty.Value{ref})})
	}
}

func (s *MySuite) TestApplyUseModules(c *C) {
	// Setup
	usingModuleID := "usingModule"
	usingModuleSource := "path/using"
	usedModuleID := "usedModule"
	usedModuleSource := "path/used"
	sharedVarName := "sharedVar"
	usingModule := Module{
		ID:     usingModuleID,
		Source: usingModuleSource,
		Use:    []string{usedModuleID},
	}
	usedModule := Module{
		ID:     usedModuleID,
		Source: usedModuleSource,
	}
	sharedVar := modulereader.VarInfo{
		Name: sharedVarName,
		Type: "number",
	}
	sharedOutput := modulereader.OutputInfo{
		Name: sharedVarName,
	}
	{ // Simple Case
		dc := getDeploymentConfigForTest()
		err := dc.applyUseModules()
		c.Assert(err, IsNil)

		g := &dc.Config.DeploymentGroups[0]
		// Has Use Modules
		g.Modules = append(g.Modules, usingModule, usedModule)

		usingInfo := dc.ModulesInfo[g.Name][usingModuleSource]
		usedInfo := dc.ModulesInfo[g.Name][usedModuleSource]
		usingInfo.Inputs = []modulereader.VarInfo{sharedVar}
		usedInfo.Outputs = []modulereader.OutputInfo{sharedOutput}
		err = dc.applyUseModules()
		c.Assert(err, IsNil)

		// Use ID doesn't exists (fail)
		g.Modules[len(g.Modules)-1].ID = "wrongID"
		err = dc.applyUseModules()
		c.Assert(err, ErrorMatches, fmt.Sprintf("%s: %s", errorMessages["invalidMod"], usedModuleID))
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

		c.Assert(dc.Config.DeploymentGroups[1].Modules[0].Settings, DeepEquals, Dict{})
		groupName0 := dc.Config.DeploymentGroups[0].Name
		moduleSource0 := dc.Config.DeploymentGroups[0].Modules[0].Source
		// this eliminates the matching output from the used module
		dc.ModulesInfo[groupName0][moduleSource0] = modulereader.ModuleInfo{}
		c.Assert(dc.applyUseModules(), IsNil)
		c.Assert(dc.Config.DeploymentGroups[1].Modules[0].Settings, DeepEquals, Dict{})
	}

	{ // Use Packer module from group 0 (fail despite matching output/input)
		dc := getMultiGroupDeploymentConfig()
		dc.Config.DeploymentGroups[0].Modules[0].Kind = PackerKind
		err := dc.applyUseModules()
		c.Assert(err, ErrorMatches,
			fmt.Sprintf("%s: %s", errorMessages["cannotUsePacker"], dc.Config.DeploymentGroups[0].Modules[0].ID))
	}
}

func (s *MySuite) TestCombineLabels(c *C) {
	infoWithLabels := modulereader.ModuleInfo{Inputs: []modulereader.VarInfo{{Name: "labels"}}}

	dc := DeploymentConfig{
		Config: Blueprint{
			BlueprintName: "simple",
			Vars:          NewDict(map[string]cty.Value{"deployment_name": cty.StringVal("golden")}),
			DeploymentGroups: []DeploymentGroup{
				{
					Name: "lime",
					Modules: []Module{
						{Source: "blue/salmon", Kind: TerraformKind, ID: "coral", Settings: NewDict(map[string]cty.Value{
							"labels": cty.ObjectVal(map[string]cty.Value{
								"magenta":   cty.StringVal("orchid"),
								"ghpc_role": cty.StringVal("maroon"),
							}),
						})},
						{Source: "brown/oak", Kind: TerraformKind, ID: "khaki"},    // has no labels set
						{Source: "ivory/black", Kind: TerraformKind, ID: "silver"}, // has no labels set, also module has no labels input
					},
				},
				{
					Name: "pink",
					Modules: []Module{
						{Source: "red/velvet", Kind: PackerKind, ID: "orange", Settings: NewDict(map[string]cty.Value{
							"labels": cty.ObjectVal(map[string]cty.Value{
								"olive":           cty.StringVal("teal"),
								"ghpc_deployment": cty.StringVal("navy"),
							}),
						})},
					},
				},
			},
		},
		ModulesInfo: map[string]map[string]modulereader.ModuleInfo{
			"lime": {
				"blue/salmon": infoWithLabels,
				"brown/oak":   infoWithLabels,
				"ivory/black": modulereader.ModuleInfo{Inputs: []modulereader.VarInfo{}},
			},
			"pink": {
				"red/velvet": infoWithLabels,
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
	coral := lime.Modules[0]
	c.Check(coral.WrapSettingsWith["labels"], DeepEquals, []string{"merge(", ")"})
	c.Check(coral.Settings.Get("labels"), DeepEquals, cty.TupleVal([]cty.Value{
		labelsRef,
		cty.ObjectVal(map[string]cty.Value{
			"magenta":   cty.StringVal("orchid"),
			"ghpc_role": cty.StringVal("maroon"),
		}),
	}))
	// Labels are not set, infer role from module.source
	khaki := lime.Modules[1]
	c.Check(khaki.WrapSettingsWith["labels"], DeepEquals, []string{"merge(", ")"})
	c.Check(khaki.Settings.Get("labels"), DeepEquals, cty.TupleVal([]cty.Value{
		labelsRef,
		cty.ObjectVal(map[string]cty.Value{
			"ghpc_role": cty.StringVal("brown")}),
	}))
	// No labels input
	silver := lime.Modules[2]
	c.Check(silver.WrapSettingsWith["labels"], IsNil)
	c.Check(silver.Settings.Get("labels"), DeepEquals, cty.NilVal)

	// Packer, include global include explicitly
	// Keep overridden ghpc_deployment=navy
	orange := dc.Config.DeploymentGroups[1].Modules[0]
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
	dc.ModulesInfo["group1"][mod.Source] = modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{{
			Name:     "gold",
			Type:     "string",
			Required: true,
		}},
	}
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
	dc.ModulesInfo["group1"][mod.Source].Inputs[0].Required = false
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

func (s *MySuite) TestIdentifyModuleByReference(c *C) {
	var ref modReference
	var err error

	dc := getDeploymentConfigForTest()
	dg := dc.Config.DeploymentGroups[0]
	fromModID := dc.Config.DeploymentGroups[0].Modules[0].ID
	toModID := dc.Config.DeploymentGroups[0].Modules[1].ID

	ref, err = identifyModuleByReference(toModID, dc.Config, fromModID)
	c.Assert(err, IsNil)
	c.Assert(ref.toGroupID, Equals, dg.Name)
	c.Assert(ref.fromGroupID, Equals, dg.Name)
	c.Assert(ref.fromModuleID, Equals, fromModID)
	c.Assert(ref.toModuleID, Equals, toModID)

	ref, err = identifyModuleByReference("bad_module_id", dc.Config, fromModID)
	c.Assert(err, ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["invalidMod"]))

	ref, err = identifyModuleByReference(toModID, dc.Config, "bad_module_id")
	c.Assert(err, ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["invalidMod"]))
}

func (s *MySuite) TestValidateModuleReference(c *C) {
	dg := []DeploymentGroup{
		{
			Name: "zero",
			Modules: []Module{
				{
					ID: "moduleA",
				},
				{
					ID: "moduleB",
				},
			},
		},
		{
			Name: "one",
			Modules: []Module{
				{
					ID: "module1",
				},
			},
		},
	}

	bp := Blueprint{
		DeploymentGroups: dg,
	}

	// An intragroup reference from group 0 to module B in 0 (good)
	ref0ToB0 := modReference{
		toModuleID:   dg[0].Modules[1].ID,
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  dg[0].Name,
	}
	c.Assert(ref0ToB0.validate(bp), IsNil)

	// An explicit intergroup reference from group 1 to module A in 0 (good)
	xRef1ToA0 := modReference{
		toModuleID:   dg[0].Modules[0].ID,
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  dg[1].Name,
	}
	c.Assert(xRef1ToA0.validate(bp), IsNil)

	// An explicit intergroup reference from group 0 to module 1 in 1 (bad due to group ordering)
	xRefA0To1 := modReference{
		toModuleID:   dg[1].Modules[0].ID,
		fromModuleID: "",
		toGroupID:    dg[1].Name,
		fromGroupID:  dg[0].Name,
	}
	c.Assert(xRefA0To1.validate(bp), ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["intergroupOrder"]))

	// An explicit intergroup reference from group 0 to B0 with a bad Group ID
	badRef0ToB0 := modReference{
		toModuleID:   dg[0].Modules[1].ID,
		fromModuleID: "",
		toGroupID:    dg[1].Name,
		fromGroupID:  dg[0].Name,
	}
	c.Assert(badRef0ToB0.validate(bp), ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["referenceWrongGroup"]))

	// A target module that doesn't exist (bad)
	badTargetMod := modReference{
		toModuleID:   "bad-module",
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  dg[0].Name,
	}
	c.Assert(badTargetMod.validate(bp), ErrorMatches, "module bad-module was not found")

	// A source group ID that doesn't exist (bad)
	badSourceGroup := modReference{
		toModuleID:   dg[0].Modules[0].ID,
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  "bad-group",
	}
	c.Assert(badSourceGroup.validate(bp), ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["groupNotFound"]))
}
