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

func (s *zeroSuite) TestValidateVars(c *C) {
	base := map[string]cty.Value{
		"deployment_name": cty.StringVal("serengeti"),
	}

	{ // Success
		vars := Dict{base}
		c.Check(validateVars(vars), IsNil)
	}

	{ // Fail: Nil value
		vars := Dict{base}
		vars.Set("fork", cty.NilVal)
		c.Check(validateVars(vars), NotNil)
	}

	{ // Fail: labels not a map
		vars := Dict{base}
		vars.Set("labels", cty.StringVal("a_string"))
		c.Check(validateVars(vars), NotNil)
	}
}

func (s *zeroSuite) TestValidateSettings(c *C) {
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

func (s *zeroSuite) TestValidateModule(c *C) {
	p := Root.Groups.At(2).Modules.At(1)
	dummyBp := Blueprint{}

	{ // Catch no ID
		err := validateModule(p, Module{Source: "green"}, dummyBp)
		c.Check(err, NotNil)
	}

	{ // Catch invalid ID
		err := validateModule(p, Module{
			ID:     "vars",
			Source: "green",
			Kind:   TerraformKind,
		}, dummyBp)
		c.Check(err, NotNil)
	}

	{ // Catch no Source
		err := validateModule(p, Module{ID: "bond"}, dummyBp)
		c.Check(err, NotNil)
	}

	{ // Catch invalid kind
		err := validateModule(p, Module{
			ID:     "bond",
			Source: "green",
			Kind:   ModuleKind{kind: "mean"},
		}, dummyBp)
		c.Check(err, NotNil)
	}

	{ // Successful validation
		mod := Module{
			ID:     "bond",
			Source: "green",
			Kind:   TerraformKind,
		}
		modulereader.SetModuleInfo(mod.Source, mod.Kind.String(), modulereader.ModuleInfo{})
		err := validateModule(p, mod, dummyBp)
		c.Check(err, IsNil)
	}
}

func (s *zeroSuite) TestValidateOutputs(c *C) {
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
