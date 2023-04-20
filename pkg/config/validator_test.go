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
	"path/filepath"
	"sort"

	"hpc-toolkit/pkg/modulereader"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	. "gopkg.in/check.v1"
)

const (
	tooManyInputRegex            = "only [0-9]+ inputs \\[.*\\] should be provided to .*"
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
	dc.Config.Vars.Set("project_id", cty.NilVal)
	err = dc.validateVars()
	c.Assert(err, ErrorMatches, "deployment variable project_id was not set")

	// Fail: labels not a map
	dc.Config.Vars.Set("labels", cty.StringVal("a_string"))
	err = dc.validateVars()
	c.Assert(err, ErrorMatches, "vars.labels must be a map of strings")
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
		Config:      Blueprint{DeploymentGroups: []DeploymentGroup{testDeploymentGroup}},
		ModulesInfo: map[string]map[string]modulereader.ModuleInfo{},
		expanded:    false,
	}
	dc.validateModuleSettings()
}

func (s *MySuite) TestValidateSettings(c *C) {
	testSettingName := "TestSetting"
	testSettingValue := "TestValue"
	validSettingNames := []string{
		"a", "A", "_", "-", testSettingName, "abc_123-ABC",
	}
	invalidSettingNames := []string{
		"", "1", "Test.Setting", "Test$Setting", "1_TestSetting",
	}
	var e *InvalidSettingError

	// Succeeds: No settings, no variables
	mod := Module{}
	mod.Settings = make(map[string]interface{})
	info := modulereader.ModuleInfo{}
	err := validateSettings(mod, info)
	c.Assert(err, IsNil)

	// Fails: One required variable, no settings
	mod.Settings = map[string]interface{}{testSettingName: testSettingValue}
	err = validateSettings(mod, info)
	c.Check(errors.As(err, &e), Equals, true)

	// Fails: Invalid setting names
	for _, name := range invalidSettingNames {
		info.Inputs = []modulereader.VarInfo{
			{Name: name, Required: true},
		}
		mod.Settings = map[string]interface{}{name: testSettingValue}
		err = validateSettings(mod, info)
		c.Check(errors.As(err, &e), Equals, true)
	}

	// Succeeds: Valid setting names
	for _, name := range validSettingNames {
		info.Inputs = []modulereader.VarInfo{
			{Name: name, Required: true},
		}
		mod.Settings = map[string]interface{}{name: testSettingValue}
		err = validateSettings(mod, info)
		c.Assert(err, IsNil)
	}

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
	testModule.Kind = "invalidKind"
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
	testInfo := modulereader.ModuleInfo{Outputs: []modulereader.OutputInfo{}}
	err := validateOutputs(testMod, testInfo)
	c.Assert(err, IsNil)

	// Output in varInfo, nothing in module
	matchingName := "match"
	testVarInfo := modulereader.OutputInfo{Name: matchingName}
	testInfo.Outputs = append(testInfo.Outputs, testVarInfo)
	err = validateOutputs(testMod, testInfo)
	c.Assert(err, IsNil)

	// Output matches between varInfo and module
	testMod.Outputs = []modulereader.OutputInfo{
		{Name: matchingName},
	}
	err = validateOutputs(testMod, testInfo)
	c.Assert(err, IsNil)

	// Addition output found in modules, not in varinfo
	missingName := "missing"
	testMod.Outputs = append(testMod.Outputs, modulereader.OutputInfo{Name: missingName})
	err = validateOutputs(testMod, testInfo)
	c.Assert(err, Not(IsNil))
	expErr := fmt.Sprintf("%s.*", errorMessages["invalidOutput"])
	c.Assert(err, ErrorMatches, expErr)
}

func (s *MySuite) TestAddDefaultValidators(c *C) {
	dc := getDeploymentConfigForTest()
	dc.addDefaultValidators()
	c.Assert(dc.Config.Validators, HasLen, 4)

	dc.Config.Validators = nil
	dc.Config.Vars.Set("region", cty.StringVal("us-central1"))
	dc.addDefaultValidators()
	c.Assert(dc.Config.Validators, HasLen, 5)

	dc.Config.Validators = nil
	dc.Config.Vars.Set("zone", cty.StringVal("us-central1-c"))
	dc.addDefaultValidators()
	c.Assert(dc.Config.Validators, HasLen, 7)
}

func (s *MySuite) TestMergeBlueprintRequirements(c *C) {
	map1 := make(map[string][]string)
	map2 := make(map[string][]string)

	// each expected value should individually be sorted and have no duplicate
	// elements, although different values may share elements
	expectedValues1 := []string{"bar", "bat"}
	expectedValues2 := []string{"value2", "value3"}

	reversedValues1 := slices.Clone(expectedValues1)
	sort.Sort(sort.Reverse(sort.StringSlice(reversedValues1)))

	// TEST: merge with identical keys and duplicate elements in values
	map1["key1"] = slices.Clone(reversedValues1)
	map2["key1"] = []string{expectedValues1[0], expectedValues1[0]}
	map3 := mergeBlueprintRequirements(map1, map2)

	// expected value (duplicates removed and sorted)
	expectedMap := map[string][]string{
		"key1": expectedValues1,
	}
	c.Assert(maps.EqualFunc(map3, expectedMap, slices.Equal[string]), Equals, true)

	// unexpected value (duplicates removed and reverse sorted)
	unexpectedMap := map[string][]string{
		"key1": reversedValues1,
	}
	c.Assert(maps.EqualFunc(map3, unexpectedMap, slices.Equal[string]), Equals, false)

	// TEST: merge with additional key in 1st map
	map1["key2"] = []string{expectedValues2[1], expectedValues2[0]}
	map3 = mergeBlueprintRequirements(map1, map2)

	// test the expected value (duplicates removed and sorted)
	expectedMap = map[string][]string{
		"key1": slices.Clone(expectedValues1),
		"key2": slices.Clone(expectedValues2),
	}
	c.Assert(maps.EqualFunc(map3, expectedMap, slices.Equal[string]), Equals, true)

	// TEST: merge with additional key in 2nd map (expected value unchanged!)
	delete(map1, "key2")
	map2["key2"] = slices.Clone(expectedValues2)
	map3 = mergeBlueprintRequirements(map1, map2)
	c.Assert(maps.EqualFunc(map3, expectedMap, slices.Equal[string]), Equals, true)
}

func (s *MySuite) TestExecuteValidators(c *C) {
	dc := getDeploymentConfigForTest()
	dc.Config.Validators = []validatorConfig{
		{Validator: "unimplemented-validator"}}

	err := dc.executeValidators()
	c.Assert(err, ErrorMatches, validationErrorMsg)

	dc.Config.Validators = []validatorConfig{
		{Validator: testProjectExistsName.String()}}

	err = dc.executeValidators()
	c.Assert(err, ErrorMatches, validationErrorMsg)
}

func (s *MySuite) TestApisEnabledValidator(c *C) {
	var err error
	dc := getDeploymentConfigForTest()
	emptyValidator := validatorConfig{}

	// test validator fails for config without validator id
	err = dc.testApisEnabled(emptyValidator)
	c.Assert(err, ErrorMatches, passedWrongValidatorRegex)

	apisEnabledValidator := validatorConfig{
		Validator: testApisEnabledName.String()}

	// this test succeeds because the list of required APIs for the test
	// Deployment Config is empty; no actual API calls get made in this case.
	// When full automation of required API detection is implemented, we may
	// need to modify this test
	err = dc.testApisEnabled(apisEnabledValidator)
	c.Assert(err, IsNil)

	// this validator reads blueprint directly so 1 inputs should fail
	apisEnabledValidator.Inputs.Set("foo", cty.StringVal("bar"))
	err = dc.testApisEnabled(apisEnabledValidator)
	c.Assert(err, ErrorMatches, tooManyInputRegex)
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
	projectValidator := validatorConfig{Validator: testProjectExistsName.String()}
	err = dc.testProjectExists(projectValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	projectValidator.Inputs.Set("project_id", MustParseExpression("var.undefined").AsValue())
	c.Assert(dc.testProjectExists(projectValidator), NotNil)

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
	regionValidator := validatorConfig{Validator: testRegionExistsName.String()}
	err = dc.testRegionExists(regionValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	regionValidator.Inputs.
		Set("project_id", MustParseExpression("var.project_id").AsValue()).
		Set("region", MustParseExpression("var.region").AsValue())
	c.Assert(dc.testRegionExists(regionValidator), NotNil)

	dc.Config.Vars.Set("project_id", cty.StringVal("invalid-project"))
	c.Assert(dc.testRegionExists(regionValidator), NotNil)

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
	zoneValidator := validatorConfig{Validator: testZoneExistsName.String()}
	err = dc.testZoneExists(zoneValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	zoneValidator.Inputs.
		Set("project_id", MustParseExpression("var.project_id").AsValue()).
		Set("zone", MustParseExpression("var.zone").AsValue())
	c.Assert(dc.testZoneExists(zoneValidator), NotNil)

	dc.Config.Vars.Set("project_id", cty.StringVal("invalid-project"))
	c.Assert(dc.testZoneExists(zoneValidator), NotNil)

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
	zoneInRegionValidator := validatorConfig{Validator: testZoneInRegionName.String()}
	err = dc.testZoneInRegion(zoneInRegionValidator)
	c.Assert(err, ErrorMatches, missingRequiredInputRegex)

	// test validators fail when input global variables are undefined
	zoneInRegionValidator.Inputs.
		Set("project_id", MustParseExpression("var.project_id").AsValue()).
		Set("region", MustParseExpression("var.region").AsValue()).
		Set("zone", MustParseExpression("var.zone").AsValue())
	c.Assert(dc.testZoneInRegion(zoneInRegionValidator), NotNil)

	dc.Config.Vars.Set("project_id", cty.StringVal("invalid-project"))
	c.Assert(dc.testZoneInRegion(zoneInRegionValidator), NotNil)

	dc.Config.Vars.Set("zone", cty.StringVal("invalid-zone"))
	c.Assert(dc.testZoneInRegion(zoneInRegionValidator), NotNil)

	// TODO: implement a mock client to test success of test_zone_in_region
}
