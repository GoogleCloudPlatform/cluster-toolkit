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
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"hpc-toolkit/pkg/modulereader"

	. "gopkg.in/check.v1"
)

const (
	missingRequiredInputRegex    = "at least one required input was not provided to .*"
	passedWrongValidatorRegex    = "passed wrong validator to .*"
	undefinedGlobalVariableRegex = ".* was not defined$"
)

func (s *MySuite) TestValidateModules(c *C) {
	dc := getDeploymentConfigForTest()
	dc.validateModules()
}

func (s *MySuite) TestValidateVars(c *C) {
	// Success
	dc := getDeploymentConfigForTest()
	err := dc.validateVars()
	c.Assert(err, IsNil)

	// Fail: Nil project_id
	dc.Config.Vars["project_id"] = nil
	err = dc.validateVars()
	c.Assert(err, ErrorMatches, "global variable project_id was not set")

	// Success: project_id not set
	delete(dc.Config.Vars, "project_id")
	var buf bytes.Buffer
	log.SetOutput(&buf)
	err = dc.validateVars()
	log.SetOutput(os.Stderr)
	c.Assert(err, IsNil)
	hasWarning := strings.Contains(buf.String(), "WARNING: No project_id")
	c.Assert(hasWarning, Equals, true)

	// Fail: labels not a map
	dc.Config.Vars["labels"] = "a_string"
	err = dc.validateVars()
	c.Assert(err, ErrorMatches, "vars.labels must be a map")
}

func (s *MySuite) TestValidateModuleSettings(c *C) {
	testSource := filepath.Join(tmpTestDir, "module")
	testSettings := map[string]interface{}{
		"test_variable": "test_value",
	}
	testDeploymentGroup := DeploymentGroup{
		Name:             "",
		TerraformBackend: TerraformBackend{},
		Modules:          []Module{{Kind: "terraform", Source: testSource, Settings: testSettings}},
	}
	dc := DeploymentConfig{
		Config:        Blueprint{DeploymentGroups: []DeploymentGroup{testDeploymentGroup}},
		ModulesInfo:   map[string]map[string]modulereader.ModuleInfo{},
		ModuleToGroup: map[string]int{},
		expanded:      false,
	}
	dc.validateModuleSettings()
}

func (s *MySuite) TestValidateSettings(c *C) {
	// Succeeds: No settings, no variables
	mod := Module{}
	info := modulereader.ModuleInfo{}
	err := validateSettings(mod, info)
	c.Assert(err, IsNil)

	// Failes One required variable, no settings
	mod.Settings = make(map[string]interface{})
	mod.Settings["TestSetting"] = "TestValue"
	err = validateSettings(mod, info)
	expErr := fmt.Sprintf("%s: .*", errorMessages["extraSetting"])
	c.Assert(err, ErrorMatches, expErr)

	// Succeeds: One required, setting exists
	info.Inputs = []modulereader.VarInfo{
		{Name: "TestSetting", Required: true},
	}
	err = validateSettings(mod, info)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestValidateModule(c *C) {
	// Catch no ID
	testModule := Module{
		ID:     "",
		Source: "testSource",
	}
	err := validateModule(testModule)
	expectedErrorStr := fmt.Sprintf(
		"%s\n%s", errorMessages["emptyID"], module2String(testModule))
	c.Assert(err, ErrorMatches, cleanErrorRegexp(expectedErrorStr))

	// Catch no Source
	testModule.ID = "testModule"
	testModule.Source = ""
	err = validateModule(testModule)
	expectedErrorStr = fmt.Sprintf(
		"%s\n%s", errorMessages["emptySource"], module2String(testModule))
	c.Assert(err, ErrorMatches, cleanErrorRegexp(expectedErrorStr))

	// Catch invalid kind
	testModule.Source = "testSource"
	testModule.Kind = ""
	err = validateModule(testModule)
	expectedErrorStr = fmt.Sprintf(
		"%s\n%s", errorMessages["wrongKind"], module2String(testModule))
	c.Assert(err, ErrorMatches, cleanErrorRegexp(expectedErrorStr))

	// Successful validation
	testModule.Kind = "terraform"
	err = validateModule(testModule)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestValidateOutputs(c *C) {
	// Simple case, no outputs in either
	testMod := Module{ID: "testMod"}
	testInfo := modulereader.ModuleInfo{Outputs: []modulereader.VarInfo{}}
	err := validateOutputs(testMod, testInfo)
	c.Assert(err, IsNil)

	// Output in varInfo, nothing in module
	matchingName := "match"
	testVarInfo := modulereader.VarInfo{Name: matchingName}
	testInfo.Outputs = append(testInfo.Outputs, testVarInfo)
	err = validateOutputs(testMod, testInfo)
	c.Assert(err, IsNil)

	// Output matches between varInfo and module
	testMod.Outputs = []string{matchingName}
	err = validateOutputs(testMod, testInfo)
	c.Assert(err, IsNil)

	// Addition output found in modules, not in varinfo
	missingName := "missing"
	testMod.Outputs = append(testMod.Outputs, missingName)
	err = validateOutputs(testMod, testInfo)
	c.Assert(err, Not(IsNil))
	expErr := fmt.Sprintf("%s.*", errorMessages["invalidOutput"])
	c.Assert(err, ErrorMatches, expErr)
}

func (s *MySuite) TestAddDefaultValidators(c *C) {
	dc := getDeploymentConfigForTest()
	dc.addDefaultValidators()
	c.Assert(dc.Config.Validators, HasLen, 0)

	dc.Config.Validators = nil
	dc.Config.Vars["project_id"] = "not-a-project"
	dc.addDefaultValidators()
	c.Assert(dc.Config.Validators, HasLen, 1)

	dc.Config.Validators = nil
	dc.Config.Vars["region"] = "us-central1"
	dc.addDefaultValidators()
	c.Assert(dc.Config.Validators, HasLen, 2)

	dc.Config.Validators = nil
	dc.Config.Vars["zone"] = "us-central1-c"
	dc.addDefaultValidators()
	c.Assert(dc.Config.Validators, HasLen, 4)
}

func (s *MySuite) TestTestInputList(c *C) {
	var err error
	var requiredInputs []string

	// SUCCESS: inputs is equal to required inputs without regard to ordering
	requiredInputs = []string{"in0", "in1"}
	inputs := map[string]interface{}{
		"in0": nil,
		"in1": nil,
	}
	err = testInputList("testfunc", inputs, requiredInputs)
	c.Assert(err, IsNil)
	requiredInputs = []string{"in1", "in0"}
	err = testInputList("testfunc", inputs, requiredInputs)
	c.Assert(err, IsNil)

	// FAIL: inputs are a proper subset of required inputs
	requiredInputs = []string{"in0", "in1", "in2"}
	err = testInputList("testfunc", inputs, requiredInputs)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// FAIL: inputs intersect with required inputs but are not a proper subset
	inputs = map[string]interface{}{
		"in0": nil,
		"in1": nil,
		"in3": nil,
	}
	err = testInputList("testfunc", inputs, requiredInputs)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// FAIL inputs are a proper superset of required inputs
	inputs = map[string]interface{}{
		"in0": nil,
		"in1": nil,
		"in2": nil,
		"in3": nil,
	}
	err = testInputList("testfunc", inputs, requiredInputs)
	c.Assert(err, ErrorMatches, "only [0-9]+ inputs \\[.*\\] should be provided to testfunc")
}

// return the actual value of a global variable specified by the literal
// variable inputReference in form ((var.project_id))
// if it is a literal global variable defined as a string, return value as string
// in all other cases, return empty string and error
func (s *MySuite) TestGetStringValue(c *C) {
	dc := getDeploymentConfigForTest()
	dc.Config.Vars["goodvar"] = "testval"
	dc.Config.Vars["badvar"] = 2

	// test non-string values return error
	_, err := dc.getStringValue(2)
	c.Assert(err, Not(IsNil))

	// test strings that are not literal variables return error and empty string
	strVal, err := dc.getStringValue("hello")
	c.Assert(err, Not(IsNil))
	c.Assert(strVal, Equals, "")

	// test literal variables that refer to strings return their value
	strVal, err = dc.getStringValue("(( var.goodvar ))")
	c.Assert(err, IsNil)
	c.Assert(strVal, Equals, dc.Config.Vars["goodvar"])

	// test literal variables that refer to non-strings return error
	_, err = dc.getStringValue("(( var.badvar ))")
	c.Assert(err, Not(IsNil))
}

func (s *MySuite) TestExecuteValidators(c *C) {
	dc := getDeploymentConfigForTest()
	dc.Config.Validators = []validatorConfig{
		{
			Validator: "unimplemented-validator",
			Inputs:    map[string]interface{}{},
		},
	}

	err := dc.executeValidators()
	c.Assert(err, ErrorMatches, validationErrorMsg)

	dc.Config.Validators = []validatorConfig{
		{
			Validator: testProjectExistsName.String(),
			Inputs:    map[string]interface{}{},
		},
	}

	err = dc.executeValidators()
	c.Assert(err, ErrorMatches, validationErrorMsg)
}

// this function tests that the "gateway" functions in this package for our
// validators fail under various conditions; it does not test the actual Cloud
// API calls in the validators package; we will defer success testing until the
// development of mock functions for Cloud API calls
func (s *MySuite) TestProjectExistsValidator(c *C) {
	var err error
	dc := getDeploymentConfigForTest()
	emptyValidator := validatorConfig{}

	// test validator fails for config without validator id
	err = dc.testProjectExists(emptyValidator)
	c.Assert(err, ErrorMatches, passedWrongValidatorRegex)

	// test validator fails for config without any inputs
	projectValidator := validatorConfig{
		Validator: testProjectExistsName.String(),
		Inputs:    map[string]interface{}{},
	}
	err = dc.testProjectExists(projectValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	projectValidator.Inputs["project_id"] = "((var.project_id))"
	err = dc.testProjectExists(projectValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)

	// TODO: implement a mock client to test success of test_project_exists
}

func (s *MySuite) TestRegionExistsValidator(c *C) {
	var err error
	dc := getDeploymentConfigForTest()
	emptyValidator := validatorConfig{}

	// test validator fails for config without validator id
	err = dc.testRegionExists(emptyValidator)
	c.Assert(err, ErrorMatches, passedWrongValidatorRegex)

	// test validator fails for config without any inputs
	regionValidator := validatorConfig{
		Validator: testRegionExistsName.String(),
		Inputs:    map[string]interface{}{},
	}
	err = dc.testRegionExists(regionValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	regionValidator.Inputs["project_id"] = "((var.project_id))"
	regionValidator.Inputs["region"] = "((var.region))"
	err = dc.testRegionExists(regionValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)
	dc.Config.Vars["project_id"] = "invalid-project"
	err = dc.testRegionExists(regionValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)

	// TODO: implement a mock client to test success of test_region_exists
}

func (s *MySuite) TestZoneExistsValidator(c *C) {
	var err error
	dc := getDeploymentConfigForTest()
	emptyValidator := validatorConfig{}

	// test validator fails for config without validator id
	err = dc.testZoneExists(emptyValidator)
	c.Assert(err, ErrorMatches, passedWrongValidatorRegex)

	// test validator fails for config without any inputs
	zoneValidator := validatorConfig{
		Validator: testZoneExistsName.String(),
		Inputs:    map[string]interface{}{},
	}
	err = dc.testZoneExists(zoneValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	zoneValidator.Inputs["project_id"] = "((var.project_id))"
	zoneValidator.Inputs["zone"] = "((var.zone))"
	err = dc.testZoneExists(zoneValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)
	dc.Config.Vars["project_id"] = "invalid-project"
	err = dc.testZoneExists(zoneValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)

	// TODO: implement a mock client to test success of test_zone_exists
}

func (s *MySuite) TestZoneInRegionValidator(c *C) {
	var err error
	dc := getDeploymentConfigForTest()
	emptyValidator := validatorConfig{}

	// test validator fails for config without validator id
	err = dc.testZoneInRegion(emptyValidator)
	c.Assert(err, ErrorMatches, passedWrongValidatorRegex)

	// test validator fails for config without any inputs
	zoneInRegionValidator := validatorConfig{
		Validator: testZoneInRegionName.String(),
		Inputs:    map[string]interface{}{},
	}
	err = dc.testZoneInRegion(zoneInRegionValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	zoneInRegionValidator.Inputs["project_id"] = "((var.project_id))"
	zoneInRegionValidator.Inputs["region"] = "((var.region))"
	zoneInRegionValidator.Inputs["zone"] = "((var.zone))"
	err = dc.testZoneInRegion(zoneInRegionValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)
	dc.Config.Vars["project_id"] = "invalid-project"
	err = dc.testZoneInRegion(zoneInRegionValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)
	dc.Config.Vars["zone"] = "invalid-zone"
	err = dc.testZoneInRegion(zoneInRegionValidator)
	c.Assert(err, ErrorMatches, undefinedGlobalVariableRegex)

	// TODO: implement a mock client to test success of test_zone_in_region
}
