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
	"regexp"

	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	. "gopkg.in/check.v1"
)

func (s *MySuite) TestExpand(c *C) {
	dc := getDeploymentConfigForTest()
	fmt.Println("TEST_DEBUG: If tests die without report, check TestExpand")
	dc.expand()
}

func (s *MySuite) TestExpandBackends(c *C) {
	dc := getDeploymentConfigForTest()

	// Simple test: Does Nothing
	err := dc.expandBackends()
	c.Assert(err, IsNil)

	dc.Config.TerraformBackendDefaults = TerraformBackend{Type: "gcs"}
	err = dc.expandBackends()
	c.Assert(err, IsNil)
	grp := dc.Config.DeploymentGroups[0]
	c.Assert(grp.TerraformBackend.Type, Not(Equals), "")
	gotPrefix := grp.TerraformBackend.Configuration.Get("prefix")
	expPrefix := fmt.Sprintf("%s/%s/%s", dc.Config.BlueprintName,
		dc.Config.Vars["deployment_name"], grp.Name)
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
	expPrefix = fmt.Sprintf("%s/%s/%s", dc.Config.BlueprintName,
		dc.Config.Vars["deployment_name"], newGrp.Name)
	c.Assert(gotPrefix, Equals, cty.StringVal(expPrefix))
}

func (s *MySuite) TestGetModuleVarName(c *C) {
	groupID := "groupID"
	modID := "modID"
	varName := "varName"
	expected := fmt.Sprintf("$(%s.%s.%s)", groupID, modID, varName)
	got := getModuleVarName(groupID, modID, varName)
	c.Assert(got, Equals, expected)
}

// a simple function for comparing interfaces for use by TestAddListValue
func equalInterfaces(v1 interface{}, v2 interface{}) bool {
	return v1 == v2
}

func (s *MySuite) TestAddListValue(c *C) {
	mod := Module{
		ID:       "TestModule",
		Settings: make(map[string]interface{}),
	}

	settingName := "newSetting"
	nonListSettingName := "not-a-list"
	firstValue := "value1"
	secondValue := "value2"

	err := mod.addListValue(settingName, firstValue)
	c.Assert(err, IsNil)
	c.Assert(slices.EqualFunc(mod.Settings[settingName].([]interface{}),
		[]interface{}{firstValue}, equalInterfaces), Equals, true)
	err = mod.addListValue(settingName, secondValue)
	c.Assert(err, IsNil)
	c.Assert(slices.EqualFunc(mod.Settings[settingName].([]interface{}),
		[]interface{}{firstValue, secondValue}, equalInterfaces), Equals, true)
	mod.Settings[nonListSettingName] = "string-value"
	err = mod.addListValue(nonListSettingName, secondValue)
	c.Assert(err, NotNil)
}

func (s *MySuite) TestUseModule(c *C) {
	// Setup
	modSource := "modSource"
	mod := Module{
		ID:       "PrimaryModule",
		Source:   modSource,
		Settings: make(map[string]interface{}),
	}

	usedModGroup := "group0"
	usedModSource := "usedSource"
	usedMod := Module{
		ID:     "UsedModule",
		Source: usedModSource,
	}
	modInfo := modulereader.ModuleInfo{}
	usedInfo := modulereader.ModuleInfo{}

	// Pass: No Inputs, No Outputs
	usedVars, err := useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, []string{})
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 0)
	c.Assert(len(mod.Settings), Equals, 0)

	// Pass: Has Output, no maching input
	varInfoNumber := modulereader.VarInfo{
		Name: "val1",
		Type: "number",
	}
	outputInfoNumber := modulereader.OutputInfo{
		Name: "val1",
	}
	usedInfo.Outputs = []modulereader.OutputInfo{outputInfoNumber}
	usedVars, err = useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, []string{})
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 0)
	c.Assert(len(mod.Settings), Equals, 0)

	// Pass: Single Input/Output match - no lists
	modInfo.Inputs = []modulereader.VarInfo{varInfoNumber}
	usedVars, err = useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, []string{})
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 1)
	c.Assert(len(mod.Settings), Equals, 1)
	expectedSetting := getModuleVarName(usedModGroup, usedMod.ID, varInfoNumber.Name)
	c.Assert(mod.Settings["val1"], Equals, expectedSetting)

	// Pass: Single Input/Output match - but setting was in blueprint so no-op
	modInfo.Inputs = []modulereader.VarInfo{varInfoNumber}
	mod.Settings = make(map[string]interface{})
	mod.Settings["val1"] = expectedSetting
	usedVars, err = useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, maps.Keys(mod.Settings))
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 0)
	c.Assert(len(mod.Settings), Equals, 1)
	expectedSetting = getModuleVarName(usedModGroup, "UsedModule", "val1")
	c.Assert(mod.Settings["val1"], Equals, expectedSetting)

	// Pass: re-apply used modules, should be a no-op
	// Assume no settings were in blueprint
	usedVars, err = useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, []string{})
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 0)
	c.Assert(len(mod.Settings), Equals, 1)
	c.Assert(mod.Settings["val1"], Equals, expectedSetting)

	// Pass: Single Input/Output match, input is list, not already set
	varInfoList := modulereader.VarInfo{
		Name: "val1",
		Type: "list",
	}
	modInfo.Inputs = []modulereader.VarInfo{varInfoList}
	mod.Settings = make(map[string]interface{})
	usedVars, err = useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, []string{})
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 1)
	c.Assert(len(mod.Settings["val1"].([]interface{})), Equals, 1)
	c.Assert(mod.Settings["val1"], DeepEquals, []interface{}{expectedSetting})

	// Pass: Setting exists, Input is List, Output is not a list
	// Assume setting was not set in blueprint
	usedVars, err = useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, []string{})
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 1)
	c.Assert(len(mod.Settings["val1"].([]interface{})), Equals, 2)
	c.Assert(
		mod.Settings["val1"],
		DeepEquals,
		[]interface{}{expectedSetting, expectedSetting})

	// Pass: Setting exists, Input is List, Output is not a list
	// Assume setting was set in blueprint
	mod.Settings = make(map[string]interface{})
	mod.Settings["val1"] = []interface{}{expectedSetting}
	usedVars, err = useModule(&mod, usedMod, usedModGroup, modInfo.Inputs, usedInfo.Outputs, maps.Keys(mod.Settings))
	c.Assert(err, IsNil)
	c.Assert(len(usedVars), Equals, 0)
	c.Assert(len(mod.Settings["val1"].([]interface{})), Equals, 1)
	c.Assert(
		mod.Settings["val1"],
		DeepEquals,
		[]interface{}{expectedSetting})
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
	// Simple Case
	dc := getDeploymentConfigForTest()
	err := dc.applyUseModules()
	c.Assert(err, IsNil)

	// Has Use Modules
	dc.Config.DeploymentGroups[0].Modules = append(
		dc.Config.DeploymentGroups[0].Modules, usingModule)
	dc.Config.DeploymentGroups[0].Modules = append(
		dc.Config.DeploymentGroups[0].Modules, usedModule)

	grpName := dc.Config.DeploymentGroups[0].Name
	usingInfo := dc.ModulesInfo[grpName][usingModuleSource]
	usedInfo := dc.ModulesInfo[grpName][usedModuleSource]
	usingInfo.Inputs = []modulereader.VarInfo{sharedVar}
	usedInfo.Outputs = []modulereader.OutputInfo{sharedOutput}
	err = dc.applyUseModules()
	c.Assert(err, IsNil)

	// Use ID doesn't exists (fail)
	modLen := len(dc.Config.DeploymentGroups[0].Modules)
	dc.Config.DeploymentGroups[0].Modules[modLen-1].ID = "wrongID"
	err = dc.applyUseModules()
	c.Assert(err, ErrorMatches, fmt.Sprintf("%s: %s", errorMessages["invalidMod"], usedModuleID))

	// test multigroup deployment with config that has a known good match
	dc = getMultiGroupDeploymentConfig()
	c.Assert(len(dc.Config.DeploymentGroups[1].Modules[0].Settings), Equals, 0)
	err = dc.applyUseModules()
	c.Assert(err, IsNil)
	c.Assert(len(dc.Config.DeploymentGroups[1].Modules[0].Settings), Equals, 1)

	// Deliberately break the match and see that no settings are added
	dc = getMultiGroupDeploymentConfig()
	c.Assert(len(dc.Config.DeploymentGroups[1].Modules[0].Settings), Equals, 0)
	groupName0 := dc.Config.DeploymentGroups[0].Name
	moduleSource0 := dc.Config.DeploymentGroups[0].Modules[0].Source
	// this eliminates the matching output from the used module
	dc.ModulesInfo[groupName0][moduleSource0] = modulereader.ModuleInfo{}
	err = dc.applyUseModules()
	c.Assert(err, IsNil)
	c.Assert(len(dc.Config.DeploymentGroups[1].Modules[0].Settings), Equals, 0)

	// Use Packer module from group 0 (fail despite matching output/input)
	dc = getMultiGroupDeploymentConfig()
	dc.Config.DeploymentGroups[0].Modules[0].Kind = "packer"
	err = dc.applyUseModules()
	c.Assert(err, ErrorMatches, fmt.Sprintf("%s: %s", errorMessages["cannotUsePacker"], dc.Config.DeploymentGroups[0].Modules[0].ID))
}

func (s *MySuite) TestUpdateVariableType(c *C) {
	// slice, success
	// empty
	testSlice := []interface{}{}
	ctx := varContext{}
	ret, err := updateVariableType(testSlice, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)
	// single string
	testSlice = append(testSlice, "string")
	ret, err = updateVariableType(testSlice, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)
	// add list
	testSlice = append(testSlice, []interface{}{})
	ret, err = updateVariableType(testSlice, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)
	// add map
	testSlice = append(testSlice, make(map[string]interface{}))
	ret, err = updateVariableType(testSlice, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)

	// map, success
	testMap := make(map[string]interface{})
	ret, err = updateVariableType(testMap, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)
	// add string
	testMap["string"] = "string"
	ret, err = updateVariableType(testMap, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)
	// add map
	testMap["map"] = make(map[string]interface{})
	ret, err = updateVariableType(testMap, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)
	// add slice
	testMap["slice"] = []interface{}{}
	ret, err = updateVariableType(testMap, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)

	// string, success
	testString := "string"
	ret, err = updateVariableType(testString, ctx, false)
	c.Assert(err, IsNil)
	c.Assert(testString, DeepEquals, ret)
}

func (s *MySuite) TestCombineLabels(c *C) {
	infoWithLabels := modulereader.ModuleInfo{Inputs: []modulereader.VarInfo{{Name: "labels"}}}

	dc := DeploymentConfig{
		Config: Blueprint{
			BlueprintName: "simple",
			Vars: map[string]interface{}{
				"deployment_name": "golden"},
			DeploymentGroups: []DeploymentGroup{
				{
					Name: "lime",
					Modules: []Module{
						{Source: "blue/salmon", Kind: "terraform", ID: "coral", Settings: map[string]interface{}{
							"labels": map[string]interface{}{
								"magenta":   "orchid",
								"ghpc_role": "maroon",
							},
						}},
						{Source: "brown/oak", Kind: "terraform", ID: "khaki", Settings: map[string]interface{}{
							// has no labels set
						}},
						{Source: "ivory/black", Kind: "terraform", ID: "silver", Settings: map[string]interface{}{
							// has no labels set, also module has no labels input
						}},
					},
				},
				{
					Name: "pink",
					Modules: []Module{
						{Source: "red/velvet", Kind: "packer", ID: "orange", Settings: map[string]interface{}{
							"labels": map[string]interface{}{
								"olive":           "teal",
								"ghpc_deployment": "navy",
							},
						}},
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
	c.Check(dc.Config.Vars["labels"], DeepEquals, map[string]interface{}{
		"ghpc_blueprint":  "simple",
		"ghpc_deployment": "golden",
	})

	lime := dc.Config.DeploymentGroups[0]
	// Labels are set and override role
	coral := lime.Modules[0]
	c.Check(coral.WrapSettingsWith["labels"], DeepEquals, []string{"merge(", ")"})
	c.Check(coral.Settings["labels"], DeepEquals, []interface{}{
		"((var.labels))",
		map[string]interface{}{"magenta": "orchid", "ghpc_role": "maroon"},
	})
	// Labels are not set, infer role from module.source
	khaki := lime.Modules[1]
	c.Check(khaki.WrapSettingsWith["labels"], DeepEquals, []string{"merge(", ")"})
	c.Check(khaki.Settings["labels"], DeepEquals, []interface{}{
		"((var.labels))",
		map[string]interface{}{"ghpc_role": "brown"},
	})
	// No labels input
	silver := lime.Modules[2]
	c.Check(silver.WrapSettingsWith["labels"], IsNil)
	c.Check(silver.Settings["labels"], IsNil)

	// Packer, include global include explicitly
	// Keep overriden ghpc_deployment=navy
	orange := dc.Config.DeploymentGroups[1].Modules[0]
	c.Check(orange.WrapSettingsWith["labels"], IsNil)
	c.Check(orange.Settings["labels"], DeepEquals, map[string]interface{}{
		"ghpc_blueprint":  "simple",
		"ghpc_deployment": "navy",
		"ghpc_role":       "red",
		"olive":           "teal",
	})

	// Test invalid labels
	dc.Config.Vars["labels"] = "notAMap"
	expectedErrorStr := fmt.Sprintf("%s: found %T",
		errorMessages["globalLabelType"], dc.Config.Vars["labels"])
	c.Check(dc.combineLabels(), ErrorMatches, expectedErrorStr)
}

func (s *MySuite) TestApplyGlobalVariables(c *C) {
	dc := getDeploymentConfigForTest()
	testModule := dc.Config.DeploymentGroups[0].Modules[0]

	// Test no inputs, none required
	err := dc.applyGlobalVariables()
	c.Assert(err, IsNil)

	// Test no inputs, one required, doesn't exist in globals
	dc.ModulesInfo["group1"][testModule.Source] = modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{requiredVar},
	}
	err = dc.applyGlobalVariables()
	expectedErrorStr := fmt.Sprintf("%s: Module ID: %s Setting: %s",
		errorMessages["missingSetting"], testModule.ID, requiredVar.Name)
	c.Assert(err, ErrorMatches, expectedErrorStr)

	// Test no input, one required, exists in globals
	dc.Config.Vars[requiredVar.Name] = "val"
	err = dc.applyGlobalVariables()
	c.Assert(err, IsNil)
	c.Assert(
		dc.Config.DeploymentGroups[0].Modules[0].Settings[requiredVar.Name],
		Equals, fmt.Sprintf("((var.%s))", requiredVar.Name))

	// Test one input, one required
	dc.Config.DeploymentGroups[0].Modules[0].Settings[requiredVar.Name] = "val"
	err = dc.applyGlobalVariables()
	c.Assert(err, IsNil)

	// Test one input, none required, exists in globals
	dc.ModulesInfo["group1"][testModule.Source].Inputs[0].Required = false
	err = dc.applyGlobalVariables()
	c.Assert(err, IsNil)
}

func (s *MySuite) TestIsGlobalVariable(c *C) {
	// True: Correct global variable
	got := isDeploymentVariable("$(vars.name)")
	c.Assert(got, Equals, true)
	// False: Missing $
	got = isDeploymentVariable("(vars.name)")
	c.Assert(got, Equals, false)
	// False: Missing (
	got = isDeploymentVariable("$vars.name)")
	c.Assert(got, Equals, false)
	// False: Missing )
	got = isDeploymentVariable("$(vars.name")
	c.Assert(got, Equals, false)
	// False: Contains Prefix
	got = isDeploymentVariable("prefix-$(vars.name)")
	c.Assert(got, Equals, false)
	// False: Contains Suffix
	got = isDeploymentVariable("$(vars.name)-suffix")
	c.Assert(got, Equals, false)
	// False: Contains prefix and suffix
	got = isDeploymentVariable("prefix-$(vars.name)-suffix")
	c.Assert(got, Equals, false)
	// False: empty string
	got = isDeploymentVariable("")
	c.Assert(got, Equals, false)
	// False: is a variable, but not global
	got = isDeploymentVariable("$(moduleid.name)")
	c.Assert(got, Equals, false)
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

	dg := DeploymentGroup{
		Name: "zero",
	}

	fromMod := Module{
		ID: "from_module_id",
	}

	ref, err = identifyModuleByReference("to_module_id", dg, fromMod)
	c.Assert(err, IsNil)
	c.Assert(ref.toGroupID, Equals, dg.Name)
	c.Assert(ref.fromGroupID, Equals, dg.Name)
	c.Assert(ref.fromModuleID, Equals, fromMod.ID)
	c.Assert(ref.toModuleID, Equals, "to_module_id")
	c.Assert(ref.explicit, Equals, false)

	ref, err = identifyModuleByReference("explicit_group_id.module_id", dg, fromMod)
	c.Assert(err, IsNil)
	c.Assert(ref.toGroupID, Equals, "explicit_group_id")
	c.Assert(ref.fromGroupID, Equals, dg.Name)
	c.Assert(ref.fromModuleID, Equals, fromMod.ID)
	c.Assert(ref.toModuleID, Equals, "module_id")
	c.Assert(ref.explicit, Equals, true)

	ref, err = identifyModuleByReference(fmt.Sprintf("%s.module_id", dg.Name), dg, fromMod)
	c.Assert(err, IsNil)
	c.Assert(ref.toGroupID, Equals, dg.Name)
	c.Assert(ref.fromGroupID, Equals, dg.Name)
	c.Assert(ref.fromModuleID, Equals, fromMod.ID)
	c.Assert(ref.toModuleID, Equals, "module_id")
	c.Assert(ref.explicit, Equals, true)

	ref, err = identifyModuleByReference("explicit_group_id.module_id.output_name", dg, fromMod)
	c.Assert(err, ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["invalidMod"]))

	ref, err = identifyModuleByReference("module_id.", dg, fromMod)
	c.Assert(err, ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["invalidMod"]))

	ref, err = identifyModuleByReference(".module_id", dg, fromMod)
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
		explicit:     false,
	}
	c.Assert(ref0ToB0.validate(bp), IsNil)

	// An explicit intergroup reference from group 1 to module A in 0 (good)
	xRef1ToA0 := modReference{
		toModuleID:   dg[0].Modules[0].ID,
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  dg[1].Name,
		explicit:     true,
	}
	c.Assert(xRef1ToA0.validate(bp), IsNil)

	// An implicit intergroup reference from group 1 to module A in 0 (bad due to implicit)
	iRef1ToA0 := modReference{
		toModuleID:   dg[0].Modules[0].ID,
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  dg[1].Name,
		explicit:     false,
	}
	c.Assert(iRef1ToA0.validate(bp), ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["intergroupImplicit"]))

	// An explicit intergroup reference from group 0 to module 1 in 1 (bad due to group ordering)
	xRefA0To1 := modReference{
		toModuleID:   dg[1].Modules[0].ID,
		fromModuleID: "",
		toGroupID:    dg[1].Name,
		fromGroupID:  dg[0].Name,
		explicit:     true,
	}
	c.Assert(xRefA0To1.validate(bp), ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["intergroupOrder"]))

	// An explicit intergroup reference from group 0 to B0 with a bad Group ID
	badRef0ToB0 := modReference{
		toModuleID:   dg[0].Modules[1].ID,
		fromModuleID: "",
		toGroupID:    dg[1].Name,
		fromGroupID:  dg[0].Name,
		explicit:     true,
	}
	c.Assert(badRef0ToB0.validate(bp), ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["referenceWrongGroup"]))

	// A target module that doesn't exist (bad)
	badTargetMod := modReference{
		toModuleID:   "bad-module",
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  dg[0].Name,
		explicit:     true,
	}
	c.Assert(badTargetMod.validate(bp), ErrorMatches, "module bad-module was not found")

	// A source group ID that doesn't exist (bad)
	badSourceGroup := modReference{
		toModuleID:   dg[0].Modules[0].ID,
		fromModuleID: "",
		toGroupID:    dg[0].Name,
		fromGroupID:  "bad-group",
		explicit:     true,
	}
	c.Assert(badSourceGroup.validate(bp), ErrorMatches, fmt.Sprintf("%s: .*", errorMessages["groupNotFound"]))
}

func (s *MySuite) TestIdentifySimpleVariable(c *C) {
	var ref varReference
	var err error

	dg := DeploymentGroup{
		Name: "from_group_id",
	}

	fromMod := Module{
		ID: "from_module_id",
	}

	ref, err = identifySimpleVariable("$(group_id.module_id.output_name)", dg, fromMod)
	c.Assert(err, IsNil)
	c.Assert(ref.toGroupID, Equals, "group_id")
	c.Assert(ref.fromGroupID, Equals, dg.Name)
	c.Assert(ref.toModuleID, Equals, "module_id")
	c.Assert(ref.fromModuleID, Equals, fromMod.ID)
	c.Assert(ref.name, Equals, "output_name")
	c.Assert(ref.explicit, Equals, true)

	ref, err = identifySimpleVariable("$(module_id.output_name)", dg, fromMod)
	c.Assert(err, IsNil)
	c.Assert(ref.toGroupID, Equals, dg.Name)
	c.Assert(ref.fromGroupID, Equals, dg.Name)
	c.Assert(ref.toModuleID, Equals, "module_id")
	c.Assert(ref.fromModuleID, Equals, fromMod.ID)
	c.Assert(ref.name, Equals, "output_name")
	c.Assert(ref.explicit, Equals, false)

	ref, err = identifySimpleVariable("$(vars.variable_name)", dg, fromMod)
	c.Assert(err, IsNil)
	c.Assert(ref.toGroupID, Equals, globalGroupID)
	c.Assert(ref.fromGroupID, Equals, dg.Name)
	c.Assert(ref.toModuleID, Equals, "vars")
	c.Assert(ref.fromModuleID, Equals, fromMod.ID)
	c.Assert(ref.name, Equals, "variable_name")
	c.Assert(ref.explicit, Equals, false)

	ref, err = identifySimpleVariable("$(foo)", dg, fromMod)
	c.Assert(err, NotNil)
	ref, err = identifySimpleVariable("$(foo.bar.baz.qux)", dg, fromMod)
	c.Assert(err, NotNil)
	ref, err = identifySimpleVariable("$(foo..bar)", dg, fromMod)
	c.Assert(err, NotNil)
	ref, err = identifySimpleVariable("$(foo.bar.)", dg, fromMod)
	c.Assert(err, NotNil)
	ref, err = identifySimpleVariable("$(foo..)", dg, fromMod)
	c.Assert(err, NotNil)
	ref, err = identifySimpleVariable("$(.foo)", dg, fromMod)
	c.Assert(err, NotNil)
	ref, err = identifySimpleVariable("$(..foo)", dg, fromMod)
	c.Assert(err, NotNil)
}

func (s *MySuite) TestExpandSimpleVariable(c *C) {
	// Setup
	testModule0 := Module{
		ID:     "module0",
		Kind:   "terraform",
		Source: "./module/testpath",
	}
	testModule1 := Module{
		ID:     "module1",
		Kind:   "terraform",
		Source: "./module/testpath",
	}
	testBlueprint := Blueprint{
		BlueprintName: "test-blueprint",
		Vars:          make(map[string]interface{}),
		DeploymentGroups: []DeploymentGroup{
			{
				Name:             "zero",
				TerraformBackend: TerraformBackend{},
				Modules:          []Module{testModule0},
			},
			{
				Name:             "one",
				TerraformBackend: TerraformBackend{},
				Modules:          []Module{testModule1},
			},
		},
		TerraformBackendDefaults: TerraformBackend{},
	}

	testVarContext0 := varContext{
		dc: &DeploymentConfig{
			Config: testBlueprint,
		},
		modIndex:   0,
		groupIndex: 0,
	}

	testVarContext1 := varContext{
		dc: &DeploymentConfig{
			Config: testBlueprint,
		},
		modIndex:   0,
		groupIndex: 1,
	}

	// Invalid variable -> no .
	testVarContext1.varString = "$(varsStringWithNoDot)"
	_, err := expandSimpleVariable(testVarContext1, false)
	c.Assert(err, NotNil)

	// Global variable: Invalid -> not found
	testVarContext1.varString = "$(vars.doesntExists)"
	_, err = expandSimpleVariable(testVarContext1, false)
	expectedErr := fmt.Sprintf("%s: .*", errorMessages["varNotFound"])
	c.Assert(err, ErrorMatches, expectedErr)

	// Global variable: Success
	testVarContext1.dc.Config.Vars["globalExists"] = "existsValue"
	testVarContext1.varString = "$(vars.globalExists)"
	got, err := expandSimpleVariable(testVarContext1, false)
	c.Assert(err, IsNil)
	c.Assert(got, Equals, "((var.globalExists))")

	// Module variable: Invalid -> Module not found
	testVarContext1.varString = "$(bad_mod.someVar)"
	_, err = expandSimpleVariable(testVarContext1, false)
	c.Assert(err, ErrorMatches, "module bad_mod was not found")

	// Module variable: Invalid -> Output not found
	reader := modulereader.Factory("terraform")
	reader.SetInfo(testModule1.Source, modulereader.ModuleInfo{})
	fakeOutput := "doesntExist"
	testVarContext1.varString = fmt.Sprintf("$(%s.%s)", testModule1.ID, fakeOutput)
	_, err = expandSimpleVariable(testVarContext1, false)
	expectedErr = fmt.Sprintf("%s: module %s did not have output %s",
		errorMessages["noOutput"], testModule1.ID, fakeOutput)
	c.Assert(err, ErrorMatches, expectedErr)

	// Module variable: Success
	existingOutput := "outputExists"
	testVarInfoOutput := modulereader.OutputInfo{Name: existingOutput}
	testModInfo := modulereader.ModuleInfo{
		Outputs: []modulereader.OutputInfo{testVarInfoOutput},
	}
	reader.SetInfo(testModule1.Source, testModInfo)
	testVarContext1.varString = fmt.Sprintf(
		"$(%s.%s)", testModule1.ID, existingOutput)
	got, err = expandSimpleVariable(testVarContext1, false)
	c.Assert(err, IsNil)
	expectedErr = fmt.Sprintf("((module.%s.%s))", testModule1.ID, existingOutput)
	c.Assert(got, Equals, expectedErr)

	// Module variable: Success when using correct explicit intragroup
	existingOutput = "outputExists"
	testVarInfoOutput = modulereader.OutputInfo{Name: existingOutput}
	testModInfo = modulereader.ModuleInfo{
		Outputs: []modulereader.OutputInfo{testVarInfoOutput},
	}
	reader.SetInfo(testModule1.Source, testModInfo)
	testVarContext1.varString = fmt.Sprintf(
		"$(%s.%s.%s)", testBlueprint.DeploymentGroups[1].Name, testModule1.ID, existingOutput)
	got, err = expandSimpleVariable(testVarContext1, false)
	c.Assert(err, IsNil)
	c.Assert(got, Equals, fmt.Sprintf("((module.%s.%s))", testModule1.ID, existingOutput))

	// Module variable: Failure when using incorrect explicit intragroup
	// Correct group is at index 1, specify group at index 0
	existingOutput = "outputExists"
	testVarInfoOutput = modulereader.OutputInfo{Name: existingOutput}
	testModInfo = modulereader.ModuleInfo{
		Outputs: []modulereader.OutputInfo{testVarInfoOutput},
	}
	reader.SetInfo(testModule1.Source, testModInfo)
	testVarContext1.varString = fmt.Sprintf(
		"$(%s.%s.%s)", testBlueprint.DeploymentGroups[0].Name, testModule1.ID, existingOutput)
	_, err = expandSimpleVariable(testVarContext1, false)
	c.Assert(err, NotNil)

	expectedErr = fmt.Sprintf("%s: %s.%s should be %s.%s",
		errorMessages["referenceWrongGroup"],
		testBlueprint.DeploymentGroups[0].Name, testModule1.ID,
		testBlueprint.DeploymentGroups[1].Name, testModule1.ID)
	c.Assert(err, ErrorMatches, expectedErr)

	// Intergroup variable: failure because other group was implicit in reference
	testVarInfoOutput = modulereader.OutputInfo{Name: existingOutput}
	testModInfo = modulereader.ModuleInfo{
		Outputs: []modulereader.OutputInfo{testVarInfoOutput},
	}
	reader.SetInfo(testModule0.Source, testModInfo)
	testVarContext1.varString = fmt.Sprintf(
		"$(%s.%s)", testModule0.ID, existingOutput)
	_, err = expandSimpleVariable(testVarContext1, false)
	expectedErr = fmt.Sprintf("%s: %s .*",
		errorMessages["intergroupImplicit"], testModule0.ID)
	c.Assert(err, ErrorMatches, expectedErr)

	// Intergroup variable: failure because explicit group and module does not exist
	testVarContext1.varString = fmt.Sprintf("$(%s.%s.%s)",
		testBlueprint.DeploymentGroups[0].Name, "bad_module", "bad_output")
	_, err = expandSimpleVariable(testVarContext1, false)
	c.Assert(err, ErrorMatches, "module bad_module was not found")

	// Intergroup variable: failure because explicit group and output does not exist
	fakeOutput = "bad_output"
	testVarContext1.varString = fmt.Sprintf("$(%s.%s.%s)",
		testBlueprint.DeploymentGroups[0].Name, testModule0.ID, fakeOutput)
	_, err = expandSimpleVariable(testVarContext1, false)
	expectedErr = fmt.Sprintf("%s: module %s did not have output %s",
		errorMessages["noOutput"], testModule0.ID, fakeOutput)
	c.Assert(err, ErrorMatches, expectedErr)

	// Intergroup variable: failure due to later group
	testVarInfoOutput = modulereader.OutputInfo{Name: existingOutput}
	testModInfo = modulereader.ModuleInfo{
		Outputs: []modulereader.OutputInfo{testVarInfoOutput},
	}
	reader.SetInfo(testModule1.Source, testModInfo)
	testVarContext0.varString = fmt.Sprintf(
		"$(%s.%s.%s)", testBlueprint.DeploymentGroups[1].Name, testModule1.ID, existingOutput)
	_, err = expandSimpleVariable(testVarContext0, false)
	expectedErr = fmt.Sprintf("%s: %s .*",
		errorMessages["intergroupOrder"], testModule1.ID)
	c.Assert(err, ErrorMatches, expectedErr)

	// Intergroup variable: proper explicit reference to earlier group
	// TODO: failure is temporary when support is added this should be a success!
	testVarInfoOutput = modulereader.OutputInfo{Name: existingOutput}
	testModInfo = modulereader.ModuleInfo{
		Outputs: []modulereader.OutputInfo{testVarInfoOutput},
	}
	reader.SetInfo(testModule0.Source, testModInfo)
	testVarContext1.varString = fmt.Sprintf(
		"$(%s.%s.%s)", testBlueprint.DeploymentGroups[0].Name, testModule0.ID, existingOutput)
	_, err = expandSimpleVariable(testVarContext1, false)
	c.Assert(err, ErrorMatches, fmt.Sprintf("%s: %s .*", errorMessages["varInAnotherGroup"], regexp.QuoteMeta(testVarContext1.varString)))
}
