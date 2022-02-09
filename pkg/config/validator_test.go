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
	"path"
	"strings"

	"hpc-toolkit/pkg/resreader"

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestValidateResources(c *C) {
	bc := getBlueprintConfigForTest()
	bc.validateResources()
}

func (s *MySuite) TestValidateVars(c *C) {
	// Success
	bc := getBlueprintConfigForTest()
	err := bc.validateVars()
	c.Assert(err, IsNil)

	// Fail: Nil project_id
	bc.Config.Vars["project_id"] = nil
	err = bc.validateVars()
	c.Assert(err, ErrorMatches, "global variable project_id was not set")

	// Success: project_id not set
	delete(bc.Config.Vars, "project_id")
	var buf bytes.Buffer
	log.SetOutput(&buf)
	err = bc.validateVars()
	log.SetOutput(os.Stderr)
	c.Assert(err, IsNil)
	hasWarning := strings.Contains(buf.String(), "WARNING: No project_id")
	c.Assert(hasWarning, Equals, true)

	// Fail: labels not a map
	bc.Config.Vars["labels"] = "a_string"
	err = bc.validateVars()
	c.Assert(err, ErrorMatches, "vars.labels must be a map")
}

func (s *MySuite) TestValidateResouceSettings(c *C) {
	testSource := path.Join(tmpTestDir, "resource")
	testSettings := map[string]interface{}{
		"test_variable": "test_value",
	}
	testResourceGroup := ResourceGroup{
		Name:             "",
		TerraformBackend: TerraformBackend{},
		Resources:        []Resource{{Kind: "terraform", Source: testSource, Settings: testSettings}},
	}
	bc := BlueprintConfig{
		Config:          YamlConfig{ResourceGroups: []ResourceGroup{testResourceGroup}},
		ResourcesInfo:   map[string]map[string]resreader.ResourceInfo{},
		ResourceToGroup: map[string]int{},
		expanded:        false,
	}
	bc.validateResourceSettings()
}

func (s *MySuite) TestValidateSettings(c *C) {
	// Succeeds: No settings, no variables
	res := Resource{}
	info := resreader.ResourceInfo{}
	err := validateSettings(res, info)
	c.Assert(err, IsNil)

	// Failes One required variable, no settings
	res.Settings = make(map[string]interface{})
	res.Settings["TestSetting"] = "TestValue"
	err = validateSettings(res, info)
	expErr := fmt.Sprintf("%s: .*", errorMessages["extraSetting"])
	c.Assert(err, ErrorMatches, expErr)

	// Succeeds: One required, setting exists
	info.Inputs = []resreader.VarInfo{
		{Name: "TestSetting", Required: true},
	}
	err = validateSettings(res, info)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestValidateResource(c *C) {
	// Catch no ID
	testResource := Resource{
		ID:     "",
		Source: "testSource",
	}
	err := validateResource(testResource)
	expectedErrorStr := fmt.Sprintf(
		"%s\n%s", errorMessages["emptyID"], resource2String(testResource))
	c.Assert(err, ErrorMatches, cleanErrorRegexp(expectedErrorStr))

	// Catch no Source
	testResource.ID = "testResource"
	testResource.Source = ""
	err = validateResource(testResource)
	expectedErrorStr = fmt.Sprintf(
		"%s\n%s", errorMessages["emptySource"], resource2String(testResource))
	c.Assert(err, ErrorMatches, cleanErrorRegexp(expectedErrorStr))

	// Catch invalid kind
	testResource.Source = "testSource"
	testResource.Kind = ""
	err = validateResource(testResource)
	expectedErrorStr = fmt.Sprintf(
		"%s\n%s", errorMessages["wrongKind"], resource2String(testResource))
	c.Assert(err, ErrorMatches, cleanErrorRegexp(expectedErrorStr))

	// Successful validation
	testResource.Kind = "terraform"
	err = validateResource(testResource)
	c.Assert(err, IsNil)
}
