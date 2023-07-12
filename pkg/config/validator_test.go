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
	"hpc-toolkit/pkg/modulereader"

	"github.com/zclconf/go-cty/cty"
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

func (s *MySuite) TestValidateSettings(c *C) {
	path := Root.Groups.At(7).Modules.At(2)
	testSettingName := "TestSetting"
	testSettingValue := cty.StringVal("TestValue")
	validSettingNames := []string{
		"a", "A", "_", "-", testSettingName, "abc_123-ABC",
	}
	invalidSettingNames := []string{
		"", "1", "Test.Setting", "Test$Setting", "1_TestSetting",
	}

	// Succeeds: No settings, no variables
	mod := Module{}
	info := modulereader.ModuleInfo{}
	err := validateSettings(path, mod, info)
	c.Check(err, IsNil)

	// Fails: One required variable, no settings
	mod.Settings = NewDict(map[string]cty.Value{testSettingName: testSettingValue})
	err = validateSettings(path, mod, info)
	c.Check(err, NotNil)

	// Fails: Invalid setting names
	for _, name := range invalidSettingNames {
		info.Inputs = []modulereader.VarInfo{
			{Name: name, Required: true},
		}
		mod.Settings = NewDict(map[string]cty.Value{name: testSettingValue})
		err = validateSettings(path, mod, info)
		c.Check(err, NotNil)
	}

	// Succeeds: Valid setting names
	for _, name := range validSettingNames {
		info.Inputs = []modulereader.VarInfo{
			{Name: name, Required: true},
		}
		mod.Settings = NewDict(map[string]cty.Value{name: testSettingValue})
		err = validateSettings(path, mod, info)
		c.Assert(err, IsNil)
	}

}

func (s *MySuite) TestValidateModule(c *C) {
	p := Root.Groups.At(2).Modules.At(1)

	{ // Catch no ID
		err := validateModule(p, Module{Source: "green"})
		c.Check(err, NotNil)
	}

	{ // Catch no Source
		err := validateModule(p, Module{ID: "bond"})
		c.Check(err, NotNil)
	}

	{ // Catch invalid kind
		err := validateModule(p, Module{
			ID:     "bond",
			Source: "green",
			Kind:   ModuleKind{kind: "mean"},
		})
		c.Check(err, NotNil)
	}

	{ // Successful validation
		mod := Module{
			ID:     "bond",
			Source: "green",
			Kind:   TerraformKind,
		}
		modulereader.SetModuleInfo(mod.Source, mod.Kind.String(), modulereader.ModuleInfo{})
		err := validateModule(p, mod)
		c.Check(err, IsNil)
	}
}

func (s *MySuite) TestValidateOutputs(c *C) {
	p := Root.Groups.At(2).Modules.At(1)

	{ // Simple case, no outputs in either
		mod := Module{}
		info := modulereader.ModuleInfo{}
		c.Check(validateOutputs(p, mod, info), IsNil)
	}

	{ // Output in varInfo, nothing in module
		mod := Module{}
		info := modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{
				{Name: "velvet"}}}
		c.Check(validateOutputs(p, mod, info), IsNil)
	}

	{ // Output matches between varInfo and module
		out := modulereader.OutputInfo{Name: "velvet"}
		mod := Module{
			Outputs: []modulereader.OutputInfo{out}}
		info := modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{out}}
		c.Check(validateOutputs(p, mod, info), IsNil)
	}

	{ // Addition output found in modules, not in varinfo
		out := modulereader.OutputInfo{Name: "velvet"}
		tuo := modulereader.OutputInfo{Name: "waldo"}
		mod := Module{
			Outputs: []modulereader.OutputInfo{out, tuo}}
		info := modulereader.ModuleInfo{
			Outputs: []modulereader.OutputInfo{out}}
		c.Check(validateOutputs(p, mod, info), NotNil)
	}
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
