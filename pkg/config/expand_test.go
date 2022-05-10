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

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestExpand(c *C) {
	dc := getDeploymentConfigForTest()
	dc.expand()
}

func (s *MySuite) TestExpandBackends(c *C) {
	dc := getDeploymentConfigForTest()

	// Simple test: Does Nothing
	err := dc.expandBackends()
	c.Assert(err, IsNil)

	tfBackend := &TerraformBackend{
		Type:          "gcs",
		Configuration: make(map[string]interface{}),
	}
	dc.Config.TerraformBackendDefaults = *tfBackend
	err = dc.expandBackends()
	c.Assert(err, IsNil)
	grp := dc.Config.DeploymentGroups[0]
	c.Assert(grp.TerraformBackend.Type, Not(Equals), "")
	gotPrefix := grp.TerraformBackend.Configuration["prefix"]
	expPrefix := fmt.Sprintf("%s/%s", dc.Config.BlueprintName, grp.Name)
	c.Assert(gotPrefix, Equals, expPrefix)

	// Add a new resource group, ensure each group name is included
	newGroup := DeploymentGroup{
		Name: "group2",
	}
	dc.Config.DeploymentGroups = append(dc.Config.DeploymentGroups, newGroup)
	dc.Config.Vars["deployment_name"] = "testDeployment"
	err = dc.expandBackends()
	c.Assert(err, IsNil)
	newGrp := dc.Config.DeploymentGroups[1]
	c.Assert(newGrp.TerraformBackend.Type, Not(Equals), "")
	gotPrefix = newGrp.TerraformBackend.Configuration["prefix"]
	expPrefix = fmt.Sprintf("%s/%s/%s", dc.Config.BlueprintName,
		dc.Config.Vars["deployment_name"], newGrp.Name)
	c.Assert(gotPrefix, Equals, expPrefix)
}

func (s *MySuite) TestGetModuleVarName(c *C) {
	modID := "modID"
	varName := "varName"
	expected := fmt.Sprintf("$(%s.%s)", modID, varName)
	got := getModuleVarName(modID, varName)
	c.Assert(got, Equals, expected)
}

func (s *MySuite) TestUseModule(c *C) {
	// Setup
	modSource := "modSource"
	mod := Module{
		ID:       "PrimaryModule",
		Source:   modSource,
		Settings: make(map[string]interface{}),
	}
	useModSource := "useSource"
	useMod := Module{
		ID:     "UsedModule",
		Source: useModSource,
	}
	modInfo := modulereader.ModuleInfo{}
	useInfo := modulereader.ModuleInfo{}
	hasChanged := make(map[string]bool)

	// Pass: No Inputs, No Outputs
	modInputs := getModuleInputMap(modInfo.Inputs)
	useModule(&mod, useMod, modInputs, useInfo.Outputs, hasChanged)
	c.Assert(len(mod.Settings), Equals, 0)
	c.Assert(len(hasChanged), Equals, 0)

	// Pass: Has Output, no maching input
	varInfoNumber := modulereader.VarInfo{
		Name: "val1",
		Type: "number",
	}
	useInfo.Outputs = []modulereader.VarInfo{varInfoNumber}
	useModule(&mod, useMod, modInputs, useInfo.Outputs, hasChanged)
	c.Assert(len(mod.Settings), Equals, 0)
	c.Assert(len(hasChanged), Equals, 0)

	// Pass: Single Input/Output match - no lists
	modInfo.Inputs = []modulereader.VarInfo{varInfoNumber}
	modInputs = getModuleInputMap(modInfo.Inputs)
	useModule(&mod, useMod, modInputs, useInfo.Outputs, hasChanged)
	expectedSetting := getModuleVarName("UsedModule", "val1")
	c.Assert(mod.Settings["val1"], Equals, expectedSetting)
	c.Assert(len(hasChanged), Equals, 1)

	// Pass: Already set, has been changed by useModule
	useModule(&mod, useMod, modInputs, useInfo.Outputs, hasChanged)
	c.Assert(len(mod.Settings), Equals, 1)
	c.Assert(len(hasChanged), Equals, 1)

	// Pass: Already set, has not been changed by useModule
	hasChanged = make(map[string]bool)
	useModule(&mod, useMod, modInputs, useInfo.Outputs, hasChanged)
	c.Assert(len(mod.Settings), Equals, 1)
	c.Assert(len(hasChanged), Equals, 0)

	// Pass: Single Input/Output match, input is list, not already set
	varInfoList := modulereader.VarInfo{
		Name: "val1",
		Type: "list",
	}
	modInfo.Inputs = []modulereader.VarInfo{varInfoList}
	modInputs = getModuleInputMap(modInfo.Inputs)
	mod.Settings = make(map[string]interface{})
	useModule(&mod, useMod, modInputs, useInfo.Outputs, hasChanged)
	c.Assert(len(mod.Settings["val1"].([]interface{})), Equals, 1)
	c.Assert(mod.Settings["val1"], DeepEquals, []interface{}{expectedSetting})
	c.Assert(len(hasChanged), Equals, 1)

	// Pass: Setting exists, Input is List, Output is not a list
	useModule(&mod, useMod, modInputs, useInfo.Outputs, hasChanged)
	c.Assert(len(mod.Settings["val1"].([]interface{})), Equals, 2)
	c.Assert(
		mod.Settings["val1"],
		DeepEquals,
		[]interface{}{expectedSetting, expectedSetting})
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
	usedInfo.Outputs = []modulereader.VarInfo{sharedVar}
	err = dc.applyUseModules()
	c.Assert(err, IsNil)

	// Use ID doesn't exists (fail)
	modLen := len(dc.Config.DeploymentGroups[0].Modules)
	dc.Config.DeploymentGroups[0].Modules[modLen-1].ID = "wrongID"
	err = dc.applyUseModules()
	c.Assert(err, ErrorMatches, "could not find module .* used by .* in group .*")

}

func (s *MySuite) TestUpdateVariableType(c *C) {
	// slice, success
	// empty
	testSlice := []interface{}{}
	ctx := varContext{}
	modToGrp := make(map[string]int)
	ret, err := updateVariableType(testSlice, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)
	// single string
	testSlice = append(testSlice, "string")
	ret, err = updateVariableType(testSlice, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)
	// add list
	testSlice = append(testSlice, []interface{}{})
	ret, err = updateVariableType(testSlice, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)
	// add map
	testSlice = append(testSlice, make(map[string]interface{}))
	ret, err = updateVariableType(testSlice, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testSlice, DeepEquals, ret)

	// map, success
	testMap := make(map[string]interface{})
	ret, err = updateVariableType(testMap, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)
	// add string
	testMap["string"] = "string"
	ret, err = updateVariableType(testMap, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)
	// add map
	testMap["map"] = make(map[string]interface{})
	ret, err = updateVariableType(testMap, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)
	// add slice
	testMap["slice"] = []interface{}{}
	ret, err = updateVariableType(testMap, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testMap, DeepEquals, ret)

	// string, success
	testString := "string"
	ret, err = updateVariableType(testString, ctx, modToGrp)
	c.Assert(err, IsNil)
	c.Assert(testString, DeepEquals, ret)
}

func (s *MySuite) TestCombineLabels(c *C) {
	dc := getDeploymentConfigForTest()

	err := dc.combineLabels()
	c.Assert(err, IsNil)

	// Were global labels created?
	_, exists := dc.Config.Vars["labels"]
	c.Assert(exists, Equals, true)

	// Was the ghpc_blueprint label set correctly?
	globalLabels := dc.Config.Vars["labels"].(map[string]interface{})
	ghpcBlueprint, exists := globalLabels[blueprintLabel]
	c.Assert(exists, Equals, true)
	c.Assert(ghpcBlueprint, Equals, dc.Config.BlueprintName)

	// Was the ghpc_deployment label set correctly?
	ghpcDeployment, exists := globalLabels[deploymentLabel]
	c.Assert(exists, Equals, true)
	c.Assert(ghpcDeployment, Equals, "undefined")

	// Was "labels" created for the module with no settings?
	_, exists = dc.Config.DeploymentGroups[0].Modules[0].Settings["labels"]
	c.Assert(exists, Equals, true)

	moduleLabels := dc.Config.DeploymentGroups[0].Modules[0].
		Settings["labels"].(map[interface{}]interface{})

	// Was the role created correctly?
	ghpcRole, exists := moduleLabels[roleLabel]
	c.Assert(exists, Equals, true)
	c.Assert(ghpcRole, Equals, "other")

	// Test invalid labels
	dc.Config.Vars["labels"] = "notAMap"
	err = dc.combineLabels()
	expectedErrorStr := fmt.Sprintf("%s: found %T",
		errorMessages["globalLabelType"], dc.Config.Vars["labels"])
	c.Assert(err, ErrorMatches, expectedErrorStr)

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

func (s *MySuite) TestExpandSimpleVariable(c *C) {
	// Setup
	testModID := "existingModule"
	testModule := Module{
		ID:     testModID,
		Kind:   "terraform",
		Source: "./module/testpath",
	}
	testBlueprint := Blueprint{
		BlueprintName: "",
		Vars:          make(map[string]interface{}),
		DeploymentGroups: []DeploymentGroup{{
			Name:             "",
			TerraformBackend: TerraformBackend{},
			Modules:          []Module{testModule},
		}},
		TerraformBackendDefaults: TerraformBackend{},
	}
	testVarContext := varContext{
		blueprint:  testBlueprint,
		modIndex:   0,
		groupIndex: 0,
	}
	testModToGrp := make(map[string]int)

	// Invalid variable -> no .
	testVarContext.varString = "$(varsStringWithNoDot)"
	_, err := expandSimpleVariable(testVarContext, testModToGrp)
	expectedErr := fmt.Sprintf("%s.*", errorMessages["invalidVar"])
	c.Assert(err, ErrorMatches, expectedErr)

	// Global variable: Invalid -> not found
	testVarContext.varString = "$(vars.doesntExists)"
	_, err = expandSimpleVariable(testVarContext, testModToGrp)
	expectedErr = fmt.Sprintf("%s: .*", errorMessages["varNotFound"])
	c.Assert(err, ErrorMatches, expectedErr)

	// Global variable: Success
	testVarContext.blueprint.Vars["globalExists"] = "existsValue"
	testVarContext.varString = "$(vars.globalExists)"
	got, err := expandSimpleVariable(testVarContext, testModToGrp)
	c.Assert(err, IsNil)
	expected := "((var.globalExists))"
	c.Assert(got, Equals, expected)

	// Module variable: Invalid -> Module not found
	testVarContext.varString = "$(notAMod.someVar)"
	_, err = expandSimpleVariable(testVarContext, testModToGrp)
	expectedErr = fmt.Sprintf("%s: .*", errorMessages["varNotFound"])
	c.Assert(err, ErrorMatches, expectedErr)

	// Module variable: Invalid -> Output not found
	reader := modulereader.Factory("terraform")
	reader.SetInfo(testModule.Source, modulereader.ModuleInfo{})
	testModToGrp[testModID] = 0
	fakeOutput := "doesntExist"
	testVarContext.varString = fmt.Sprintf("$(%s.%s)", testModule.ID, fakeOutput)
	_, err = expandSimpleVariable(testVarContext, testModToGrp)
	expectedErr = fmt.Sprintf("%s: module %s did not have output %s",
		errorMessages["noOutput"], testModID, fakeOutput)
	c.Assert(err, ErrorMatches, expectedErr)

	// Module variable: Success
	existingOutput := "outputExists"
	testVarInfoOutput := modulereader.VarInfo{Name: existingOutput}
	testModInfo := modulereader.ModuleInfo{
		Outputs: []modulereader.VarInfo{testVarInfoOutput},
	}
	reader.SetInfo(testModule.Source, testModInfo)
	testVarContext.varString = fmt.Sprintf(
		"$(%s.%s)", testModule.ID, existingOutput)
	got, err = expandSimpleVariable(testVarContext, testModToGrp)
	c.Assert(err, IsNil)
	expected = fmt.Sprintf("((module.%s.%s))", testModule.ID, existingOutput)
	c.Assert(got, Equals, expected)
}
