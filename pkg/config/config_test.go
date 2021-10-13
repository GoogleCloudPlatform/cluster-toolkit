/*
Copyright 2021 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package config manages and updates the ghpc input config
package config

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"hpc-toolkit/pkg/resreader"

	. "gopkg.in/check.v1"
)

var (
	// Shared IO Values
	simpleYamlFilename string
	simpleYamlFile     os.File

	// Expected/Input Values
	expectedYaml = []byte(`
blueprint_name: simple
vars:
  labels:
    ghpc_blueprint: simple
    deployment_name: deployment_name
resource_groups:
- group: group1
  resources:
  - source: ./resources/network/vpc
    kind: terraform
    id: "vpc"
    settings:
      network_name: $"${var.deployment_name}_net
      project_id: project_name
`)
	testResources = []Resource{
		{
			Source: "./resources/network/vpc",
			Kind:   "terraform",
			ID:     "vpc",
			Settings: map[string]interface{}{
				"network_name": "$\"${var.deployment_name}_net\"",
				"project_id":   "project_name",
			},
		},
	}
	defaultLabels = map[interface{}]interface{}{
		"ghpc_blueprint":  "simple",
		"deployment_name": "deployment_name",
	}
	expectedSimpleYamlConfig YamlConfig = YamlConfig{
		BlueprintName: "simple",
		Vars: map[string]interface{}{
			"labels": defaultLabels,
		},
		ResourceGroups: []ResourceGroup{
			ResourceGroup{
				Name:      "ResourceGroup1",
				Resources: testResources,
			},
		},
	}
	// For expand.go
	requiredVar = resreader.VarInfo{
		Name:        "reqVar",
		Type:        "string",
		Description: "A test required variable",
		Default:     nil,
		Required:    true,
	}
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

// setup opens a temp file to store the yaml and saves it's name
func setup() {
	simpleYamlFile, err := ioutil.TempFile("", "*.yaml")
	if err != nil {
		log.Fatal(err)
	}
	_, err = simpleYamlFile.Write(expectedYaml)
	if err != nil {
		log.Fatal(err)
	}
	simpleYamlFilename = simpleYamlFile.Name()
	simpleYamlFile.Close()
}

// Delete the temp YAML file
func teardown() {
	err := os.Remove(simpleYamlFilename)
	if err != nil {
		log.Fatalf("config_test teardown: %v", err)
	}
}

/* Tests */
// config.go
func (s *MySuite) TestImportYamlConfig(c *C) {
	obtainedYamlConfig := importYamlConfig(simpleYamlFilename)
	c.Assert(obtainedYamlConfig.BlueprintName,
		Equals, expectedSimpleYamlConfig.BlueprintName)
	c.Assert(
		len(obtainedYamlConfig.Vars["labels"].(map[interface{}]interface{})),
		Equals,
		len(expectedSimpleYamlConfig.Vars["labels"].(map[interface{}]interface{})),
	)
	c.Assert(obtainedYamlConfig.ResourceGroups[0].Resources[0].ID,
		Equals, expectedSimpleYamlConfig.ResourceGroups[0].Resources[0].ID)
}

func (s *MySuite) TestExportYamlConfig(c *C) {
	bc := BlueprintConfig{}
	bc.Config = expectedSimpleYamlConfig
	obtainedYaml := bc.ExportYamlConfig("")
	c.Assert(obtainedYaml, Not(IsNil))
}

// expand.go
func getBlueprintConfigForTest() BlueprintConfig {
	testResourceSource := "testSource"
	testResource := Resource{
		Source:   testResourceSource,
		Kind:     "terraform",
		ID:       "testResource",
		Settings: make(map[string]interface{}),
	}
	testResourceSourceWithLabels := "./role/source"
	testResourceWithLabels := Resource{
		Source: testResourceSourceWithLabels,
		ID:     "testResourceWithLabels",
		Kind:   "terraform",
		Settings: map[string]interface{}{
			"resourceLabel": "resourceLabelValue",
		},
	}
	testLabelVarInfo := resreader.VarInfo{Name: "labels"}
	testResourceInfo := resreader.ResourceInfo{
		Inputs: []resreader.VarInfo{testLabelVarInfo},
	}
	testYamlConfig := YamlConfig{
		BlueprintName: "simple",
		Vars:          map[string]interface{}{},
		ResourceGroups: []ResourceGroup{
			ResourceGroup{
				Name:      "group1",
				Resources: []Resource{testResource, testResourceWithLabels},
			},
		},
	}

	return BlueprintConfig{
		Config: testYamlConfig,
		ResourcesInfo: map[string]map[string]resreader.ResourceInfo{
			"group1": map[string]resreader.ResourceInfo{
				testResourceSource:           testResourceInfo,
				testResourceSourceWithLabels: testResourceInfo,
			},
		},
	}
}

func (s *MySuite) TestCombineLabels(c *C) {
	bc := getBlueprintConfigForTest()

	err := bc.combineLabels()
	c.Assert(err, IsNil)

	// Were global labels created?
	_, exists := bc.Config.Vars["labels"]
	c.Assert(exists, Equals, true)

	// Was the ghpc_blueprint label set correctly?
	globalLabels := bc.Config.Vars["labels"].(map[interface{}]interface{})
	ghpcBlueprint, exists := globalLabels[blueprintLabel]
	c.Assert(exists, Equals, true)
	c.Assert(ghpcBlueprint.(string), Equals, bc.Config.BlueprintName)

	// Was the ghpc_deployment label set correctly?
	ghpcDeployment, exists := globalLabels[deploymentLabel]
	c.Assert(exists, Equals, true)
	c.Assert(ghpcDeployment.(string), Equals, "undefined")

	// Was "labels" created for the resource with no settings?
	_, exists = bc.Config.ResourceGroups[0].Resources[0].Settings["labels"]
	c.Assert(exists, Equals, true)

	resourceLabels := bc.Config.ResourceGroups[0].Resources[0].
		Settings["labels"].(map[interface{}]interface{})

	// Was the role created correctly?
	ghpcRole, exists := resourceLabels[roleLabel]
	c.Assert(exists, Equals, true)
	c.Assert(ghpcRole, Equals, "other")

	// Test invalid labels
	bc.Config.Vars["labels"] = "notAMap"
	err = bc.combineLabels()
	expectedErrorStr := fmt.Sprintf("%s: found %T",
		errorMessages["globalLabelType"], bc.Config.Vars["labels"])
	c.Assert(err, ErrorMatches, expectedErrorStr)

}

func (s *MySuite) TestApplyGlobalVariables(c *C) {
	bc := getBlueprintConfigForTest()
	testResource := bc.Config.ResourceGroups[0].Resources[0]

	// Test no inputs, none required
	err := bc.applyGlobalVariables()
	c.Assert(err, IsNil)

	// Test no inputs, one required
	bc.ResourcesInfo["group1"][testResource.Source] = resreader.ResourceInfo{
		Inputs: []resreader.VarInfo{requiredVar},
	}
	err = bc.applyGlobalVariables()
	expectedErrorStr := fmt.Sprintf("%s: Resource.ID: %s Setting: %s",
		errorMessages["missingSetting"], testResource.ID, requiredVar.Name)
	c.Assert(err, ErrorMatches, expectedErrorStr)

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
	testResID := "existingResource"
	testResource := Resource{
		ID:     testResID,
		Kind:   "terraform",
		Source: "./resource/testpath",
	}
	testYamlConfig := YamlConfig{
		Vars: make(map[string]interface{}),
		ResourceGroups: []ResourceGroup{
			ResourceGroup{
				Resources: []Resource{
					testResource,
				},
			},
		},
	}
	testVarContext := varContext{
		yamlConfig: testYamlConfig,
		resIndex:   0,
		groupIndex: 0,
	}
	testResToGrp := make(map[string]int)

	// Invalid variable -> no .
	testVarContext.varString = "$(varsStringWithNoDot)"
	_, err := expandSimpleVariable(testVarContext, testResToGrp)
	expectedErr := fmt.Sprintf("%s.*", errorMessages["invalidVar"])
	c.Assert(err, ErrorMatches, expectedErr)

	// Global variable: Invalid -> not found
	testVarContext.varString = "$(vars.doesntExists)"
	_, err = expandSimpleVariable(testVarContext, testResToGrp)
	expectedErr = fmt.Sprintf("%s: .*", errorMessages["varNotFound"])
	c.Assert(err, ErrorMatches, expectedErr)

	// Global variable: Success
	testVarContext.yamlConfig.Vars["globalExists"] = "existsValue"
	testVarContext.varString = "$(vars.globalExists)"
	got, err := expandSimpleVariable(testVarContext, testResToGrp)
	c.Assert(err, IsNil)
	expected := "((var.globalExists))"
	c.Assert(got, Equals, expected)

	// Resource variable: Invalid -> Resource not found
	testVarContext.varString = "$(notARes.someVar)"
	_, err = expandSimpleVariable(testVarContext, testResToGrp)
	expectedErr = fmt.Sprintf("%s: .*", errorMessages["varNotFound"])
	c.Assert(err, ErrorMatches, expectedErr)

	// Resource variable: Invalid -> Output not found
	reader := resreader.Factory("terraform")
	reader.SetInfo(testResource.Source, resreader.ResourceInfo{})
	testResToGrp[testResID] = 0
	fakeOutput := "doesntExist"
	testVarContext.varString = fmt.Sprintf("$(%s.%s)", testResource.ID, fakeOutput)
	_, err = expandSimpleVariable(testVarContext, testResToGrp)
	expectedErr = fmt.Sprintf("%s: resource %s did not have output %s",
		errorMessages["noOutput"], testResID, fakeOutput)
	c.Assert(err, ErrorMatches, expectedErr)

	// Resource variable: Success
	existingOutput := "outputExists"
	testVarInfoOutput := resreader.VarInfo{Name: existingOutput}
	testResInfo := resreader.ResourceInfo{
		Outputs: []resreader.VarInfo{testVarInfoOutput},
	}
	reader.SetInfo(testResource.Source, testResInfo)
	testVarContext.varString = fmt.Sprintf(
		"$(%s.%s)", testResource.ID, existingOutput)
	got, err = expandSimpleVariable(testVarContext, testResToGrp)
	c.Assert(err, IsNil)
	expected = fmt.Sprintf("((module.%s.%s))", testResource.ID, existingOutput)
	c.Assert(got, Equals, expected)
}

// validator.go
func (s *MySuite) TestValidateResource(c *C) {
	// Catch no ID
	testResource := Resource{
		ID:     "",
		Source: "testSource",
	}
	err := validateResource(testResource)
	expectedErrorStr := fmt.Sprintf(
		"%s\n%s", errorMessages["emptyID"], resource2String(testResource))
	c.Assert(err, ErrorMatches, expectedErrorStr)

	// Catch no Source
	testResource.ID = "testResource"
	testResource.Source = ""
	err = validateResource(testResource)
	expectedErrorStr = fmt.Sprintf(
		"%s\n%s", errorMessages["emptySource"], resource2String(testResource))
	c.Assert(err, ErrorMatches, expectedErrorStr)

	// Catch invalid kind
	testResource.Source = "testSource"
	testResource.Kind = ""
	err = validateResource(testResource)
	expectedErrorStr = fmt.Sprintf(
		"%s\n%s", errorMessages["wrongKind"], resource2String(testResource))
	c.Assert(err, ErrorMatches, expectedErrorStr)

	// Successful validation
	testResource.Kind = "terraform"
	err = validateResource(testResource)
	c.Assert(err, IsNil)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}
