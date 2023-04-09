/*
Copyright 2022 Google LLC

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

package config

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hpc-toolkit/pkg/modulereader"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

var (
	// Shared IO Values
	simpleYamlFilename string
	tmpTestDir         string

	// Expected/Input Values
	expectedYaml = []byte(`
blueprint_name: simple
vars:
  project_id: test-project
  labels:
    ghpc_blueprint: simple
    deployment_name: deployment_name
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: hpc-toolkit-tf-state
deployment_groups:
- group: group1
  modules:
  - source: ./modules/network/vpc
    kind: terraform
    id: "vpc"
    settings:
      network_name: $"${var.deployment_name}_net
`)
	testModules = []Module{
		{
			Source:           "./modules/network/vpc",
			Kind:             "terraform",
			ID:               "vpc",
			WrapSettingsWith: make(map[string][]string),
			Settings: map[string]interface{}{
				"network_name": "$\"${var.deployment_name}_net\"",
				"project_id":   "project_name",
			},
		},
	}
	defaultLabels = map[string]interface{}{
		"ghpc_blueprint":  "simple",
		"deployment_name": "deployment_name",
	}
	expectedSimpleBlueprint Blueprint = Blueprint{
		BlueprintName: "simple",
		Vars: map[string]interface{}{
			"project_id": "test-project",
			"labels":     defaultLabels},
		DeploymentGroups: []DeploymentGroup{{Name: "DeploymentGroup1", Modules: testModules}},
	}
	// For expand.go
	requiredVar = modulereader.VarInfo{
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

	// Create test directory with simple modules
	tmpTestDir, err = ioutil.TempDir("", "ghpc_config_tests_*")
	if err != nil {
		log.Fatalf("failed to create temp dir for config tests: %e", err)
	}
	moduleDir := filepath.Join(tmpTestDir, "module")
	err = os.Mkdir(moduleDir, 0755)
	if err != nil {
		log.Fatalf("failed to create test module dir: %v", err)
	}
	varFile, err := os.Create(filepath.Join(moduleDir, "variables.tf"))
	if err != nil {
		log.Fatalf("failed to create variables.tf in test module dir: %v", err)
	}
	testVariablesTF := `
	variable "test_variable" {
		description = "Test Variable"
		type        = string
	}`
	_, err = varFile.WriteString(testVariablesTF)
	if err != nil {
		log.Fatalf("failed to write variables.tf in test module dir: %v", err)
	}
}

// Delete the temp YAML file
func teardown() {
	err := os.Remove(simpleYamlFilename)
	if err != nil {
		log.Fatalf("config_test teardown: %v", err)
	}
	err = os.RemoveAll(tmpTestDir)
	if err != nil {
		log.Fatalf(
			"failed to tear down tmp directory (%s) for config unit tests: %v",
			tmpTestDir, err)
	}
}

// util function
func cleanErrorRegexp(errRegexp string) string {
	errRegexp = strings.ReplaceAll(errRegexp, "[", "\\[")
	errRegexp = strings.ReplaceAll(errRegexp, "]", "\\]")
	return errRegexp
}

func getDeploymentConfigForTest() DeploymentConfig {
	testModuleSource := "testSource"
	testModule := Module{
		Source:           testModuleSource,
		Kind:             "terraform",
		ID:               "testModule",
		Use:              []string{},
		WrapSettingsWith: make(map[string][]string),
		Settings:         make(map[string]interface{}),
	}
	testModuleSourceWithLabels := "./role/source"
	testModuleWithLabels := Module{
		Source:           testModuleSourceWithLabels,
		ID:               "testModuleWithLabels",
		Kind:             "terraform",
		Use:              []string{},
		WrapSettingsWith: make(map[string][]string),
		Settings: map[string]interface{}{
			"moduleLabel": "moduleLabelValue",
		},
	}
	testLabelVarInfo := modulereader.VarInfo{Name: "labels"}
	testModuleInfo := modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{testLabelVarInfo},
	}
	testBlueprint := Blueprint{
		BlueprintName: "simple",
		Validators:    nil,
		Vars: map[string]interface{}{
			"deployment_name": "deployment_name",
			"project_id":      "test-project",
		},
		DeploymentGroups: []DeploymentGroup{
			{
				Name:    "group1",
				Modules: []Module{testModule, testModuleWithLabels},
			},
		},
	}

	dc := DeploymentConfig{
		Config: testBlueprint,
		ModulesInfo: map[string]map[string]modulereader.ModuleInfo{
			"group1": {
				testModuleSource:           testModuleInfo,
				testModuleSourceWithLabels: testModuleInfo,
			},
		},
		moduleConnections: make(map[string][]ModConnection),
	}
	// the next two steps simulate relevant steps in ghpc expand
	dc.addMetadataToModules()
	dc.addDefaultValidators()
	return dc
}

func getBasicDeploymentConfigWithTestModule() DeploymentConfig {
	testModuleSource := filepath.Join(tmpTestDir, "module")
	testDeploymentGroup := DeploymentGroup{
		Name: "primary",
		Modules: []Module{
			{
				ID:       "TestModule",
				Kind:     "terraform",
				Source:   testModuleSource,
				Settings: map[string]interface{}{"test_variable": "test_value"},
			},
		},
	}
	return DeploymentConfig{
		Config: Blueprint{
			BlueprintName:    "simple",
			Vars:             map[string]interface{}{"deployment_name": "deployment_name"},
			DeploymentGroups: []DeploymentGroup{testDeploymentGroup},
		},
	}
}

// create a simple multigroup deployment with a use keyword that matches
// one module to another in an earlier group
func getMultiGroupDeploymentConfig() DeploymentConfig {
	testModuleSource0 := filepath.Join(tmpTestDir, "module0")
	testModuleSource1 := filepath.Join(tmpTestDir, "module1")
	testModuleSource2 := filepath.Join(tmpTestDir, "module2")

	matchingIntergroupName := "test_inter_0"
	matchingIntragroupName0 := "test_intra_0"
	matchingIntragroupName1 := "test_intra_1"
	matchingIntragroupName2 := "test_intra_2"

	altProjectIDSetting := "host_project_id"

	testModuleInfo0 := modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{
			{
				Name: "deployment_name",
				Type: "string",
			},
			{
				Name: altProjectIDSetting,
				Type: "string",
			},
		},
		Outputs: []modulereader.OutputInfo{
			{
				Name: matchingIntergroupName,
			},
			{
				Name: matchingIntragroupName0,
			},
			{
				Name: matchingIntragroupName1,
			},
			{
				Name: matchingIntragroupName2,
			},
		},
	}
	testModuleInfo1 := modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{
			{
				Name: matchingIntragroupName0,
			},
			{
				Name: matchingIntragroupName1,
			},
			{
				Name: matchingIntragroupName2,
			},
		},
		Outputs: []modulereader.OutputInfo{},
	}

	testModuleInfo2 := modulereader.ModuleInfo{
		Inputs: []modulereader.VarInfo{
			{
				Name: "deployment_name",
				Type: "string",
			},
			{
				Name: matchingIntergroupName,
			},
		},
		Outputs: []modulereader.OutputInfo{},
	}

	dg0Name := "primary"
	modID0 := "TestModule0"
	testDeploymentGroup0 := DeploymentGroup{
		Name: dg0Name,
		Modules: []Module{
			{
				ID:     modID0,
				Kind:   "terraform",
				Source: testModuleSource0,
				Settings: map[string]interface{}{
					altProjectIDSetting: "$(vars.project_id)",
				},
				Outputs: []modulereader.OutputInfo{
					{Name: matchingIntergroupName},
				},
			},
			{
				ID:     "TestModule1",
				Kind:   "terraform",
				Source: testModuleSource1,
				Settings: map[string]interface{}{
					matchingIntragroupName1: "explicit-intra-value",
					matchingIntragroupName2: fmt.Sprintf("$(%s.%s)", modID0, matchingIntragroupName2),
				},
				Use: []string{
					fmt.Sprintf("%s.%s", dg0Name, modID0),
				},
			},
		},
	}
	testDeploymentGroup1 := DeploymentGroup{
		Name: "secondary",
		Modules: []Module{
			{
				ID:       "TestModule2",
				Kind:     "terraform",
				Source:   testModuleSource2,
				Settings: map[string]interface{}{},
				Use: []string{
					fmt.Sprintf("%s.%s", testDeploymentGroup0.Name, testDeploymentGroup0.Modules[0].ID),
				},
			},
		},
	}

	dc := DeploymentConfig{
		Config: Blueprint{
			BlueprintName: "simple",
			Vars: map[string]interface{}{
				"deployment_name": "deployment_name",
				"project_id":      "test-project",
				"unused_key":      "unused_value",
			},
			DeploymentGroups: []DeploymentGroup{testDeploymentGroup0, testDeploymentGroup1},
		},
		ModulesInfo: map[string]map[string]modulereader.ModuleInfo{
			testDeploymentGroup0.Name: {
				testModuleSource0: testModuleInfo0,
				testModuleSource1: testModuleInfo1,
			},
			testDeploymentGroup1.Name: {
				testModuleSource2: testModuleInfo2,
			},
		},
		moduleConnections: make(map[string][]ModConnection),
	}

	dc.addSettingsToModules()
	dc.addMetadataToModules()
	dc.addDefaultValidators()
	reader := modulereader.Factory("terraform")
	reader.SetInfo(testModuleSource0, testModuleInfo0)
	reader.SetInfo(testModuleSource1, testModuleInfo1)
	reader.SetInfo(testModuleSource2, testModuleInfo2)

	return dc
}

func getDeploymentConfigWithTestModuleEmptyKind() DeploymentConfig {
	testModuleSource := filepath.Join(tmpTestDir, "module")
	testDeploymentGroup := DeploymentGroup{
		Name: "primary",
		Modules: []Module{
			{
				ID:       "TestModule1",
				Source:   testModuleSource,
				Settings: map[string]interface{}{"test_variable": "test_value"},
			},
			{
				ID:       "TestModule2",
				Kind:     "",
				Source:   testModuleSource,
				Settings: map[string]interface{}{"test_variable": "test_value"},
			},
		},
	}
	return DeploymentConfig{
		Config: Blueprint{
			BlueprintName:    "simple",
			Vars:             map[string]interface{}{"deployment_name": "deployment_name"},
			DeploymentGroups: []DeploymentGroup{testDeploymentGroup},
		},
	}
}

/* Tests */
// config.go
func (s *MySuite) TestExpandConfig(c *C) {
	dc := getBasicDeploymentConfigWithTestModule()
	dc.ExpandConfig()
}

func (s *MySuite) TestCheckModuleAndGroupNames(c *C) {
	{ // Duplicate module name same group
		g := DeploymentGroup{Name: "ice", Modules: []Module{{ID: "pony"}, {ID: "pony"}}}
		err := checkModuleAndGroupNames([]DeploymentGroup{g})
		c.Check(err, ErrorMatches, "module IDs must be unique: pony used more than once")
	}
	{ // Duplicate module name different groups
		ice := DeploymentGroup{Name: "ice", Modules: []Module{{ID: "pony"}}}
		fire := DeploymentGroup{Name: "fire", Modules: []Module{{ID: "pony"}}}
		err := checkModuleAndGroupNames([]DeploymentGroup{ice, fire})
		c.Check(err, ErrorMatches, "module IDs must be unique: pony used more than once")
	}
	{ // Mixing module kinds
		g := DeploymentGroup{Name: "ice", Modules: []Module{
			{ID: "pony", Kind: "packer"},
			{ID: "zebra", Kind: "terraform"},
		}}
		err := checkModuleAndGroupNames([]DeploymentGroup{g})
		c.Check(err, ErrorMatches, "mixing modules of differing kinds in a deployment group is not supported: deployment group ice, got packer and terraform")
	}
}

func (s *MySuite) TestIsUnused(c *C) {
	// Use connection is not empty
	conn := ModConnection{
		kind:            useConnection,
		sharedVariables: []string{"var1"},
	}
	c.Assert(conn.isUnused(), Equals, false)

	// Use connection is empty
	conn = ModConnection{
		kind:            useConnection,
		sharedVariables: []string{},
	}
	c.Assert(conn.isUnused(), Equals, true)

	// Undefined connection kind
	conn = ModConnection{}
	c.Assert(conn.isUnused(), Equals, false)
}

func (s *MySuite) TestListUnusedModules(c *C) {
	dc := getDeploymentConfigForTest()

	// No modules in "use"
	got := dc.listUnusedModules()
	c.Assert(got, HasLen, 0)

	modRef0 := modReference{
		toModuleID:   "usedModule",
		fromModuleID: "usingModule",
		toGroupID:    "group1",
		fromGroupID:  "group1",
		explicit:     true,
	}
	dc.addModuleConnection(modRef0, useConnection, []string{"var1"})
	got = dc.listUnusedModules()
	c.Assert(got["usingModule"], HasLen, 0)

	// test used module with no shared variables (i.e. "unused")
	modRef1 := modReference{
		toModuleID:   "firstUnusedModule",
		fromModuleID: "usingModule",
		toGroupID:    "group1",
		fromGroupID:  "group1",
		explicit:     true,
	}
	dc.addModuleConnection(modRef1, useConnection, []string{})
	got = dc.listUnusedModules()
	c.Assert(got["usingModule"], HasLen, 1)

	// test second used module with no shared variables (i.e. "unused")
	modRef2 := modReference{
		toModuleID:   "secondUnusedModule",
		fromModuleID: "usingModule",
		toGroupID:    "group1",
		fromGroupID:  "group1",
		explicit:     true,
	}
	dc.addModuleConnection(modRef2, useConnection, []string{})
	got = dc.listUnusedModules()
	c.Assert(got["usingModule"], HasLen, 2)
}

func (s *MySuite) TestListUnusedDeploymentVariables(c *C) {
	dc := getDeploymentConfigForTest()
	dc.applyGlobalVariables()
	dc.expandVariables()
	unusedVars := dc.listUnusedDeploymentVariables()
	c.Assert(unusedVars, DeepEquals, []string{"project_id"})
	dc = getMultiGroupDeploymentConfig()
	dc.applyGlobalVariables()
	dc.expandVariables()
	unusedVars = dc.listUnusedDeploymentVariables()
	c.Assert(unusedVars, DeepEquals, []string{"unused_key"})
}

func (s *MySuite) TestAddKindToModules(c *C) {
	/* Test addKindToModules() works when nothing to do */
	dc := getBasicDeploymentConfigWithTestModule()
	testMod, _ := dc.Config.DeploymentGroups[0].getModuleByID("TestModule1")
	expected := testMod.Kind
	dc.addKindToModules()
	testMod, _ = dc.Config.DeploymentGroups[0].getModuleByID("TestModule1")
	c.Assert(testMod.Kind, Equals, expected)

	/* Test addKindToModules() works when kind is absent*/
	dc = getDeploymentConfigWithTestModuleEmptyKind()
	expected = "terraform"
	dc.addKindToModules()
	testMod, _ = dc.Config.DeploymentGroups[0].getModuleByID("TestModule1")
	c.Assert(testMod.Kind, Equals, expected)

	/* Test addKindToModules() works when kind is empty*/
	dc = getDeploymentConfigWithTestModuleEmptyKind()
	expected = "terraform"
	dc.addKindToModules()
	testMod, _ = dc.Config.DeploymentGroups[0].getModuleByID("TestModule1")
	c.Assert(testMod.Kind, Equals, expected)

	/* Test addKindToModules() does nothing to packer types*/
	moduleID := "packerModule"
	expected = "packer"
	dc = getDeploymentConfigWithTestModuleEmptyKind()
	dc.Config.DeploymentGroups[0].Modules = append(dc.Config.DeploymentGroups[0].Modules, Module{ID: moduleID, Kind: expected})
	dc.addKindToModules()
	testMod, _ = dc.Config.DeploymentGroups[0].getModuleByID(moduleID)
	c.Assert(testMod.Kind, Equals, expected)

	/* Test addKindToModules() does nothing to invalid types*/
	moduleID = "funnyModule"
	expected = "funnyType"
	dc = getDeploymentConfigWithTestModuleEmptyKind()
	dc.Config.DeploymentGroups[0].Modules = append(dc.Config.DeploymentGroups[0].Modules, Module{ID: moduleID, Kind: expected})
	dc.addKindToModules()
	testMod, _ = dc.Config.DeploymentGroups[0].getModuleByID(moduleID)
	c.Assert(testMod.Kind, Equals, expected)
}

func (s *MySuite) TestModuleConnections(c *C) {
	dc := getMultiGroupDeploymentConfig()
	modID0 := dc.Config.DeploymentGroups[0].Modules[0].ID
	modID1 := dc.Config.DeploymentGroups[0].Modules[1].ID
	modID2 := dc.Config.DeploymentGroups[1].Modules[0].ID

	err := dc.applyUseModules()
	c.Assert(err, IsNil)
	err = dc.applyGlobalVariables()
	c.Assert(err, IsNil)
	err = dc.expandVariables()
	// TODO: this will become nil once intergroup references are enabled
	c.Assert(err, NotNil)

	// check that ModuleConnections has map keys for each module ID
	c.Check(dc.moduleConnections, DeepEquals, map[string][]ModConnection{
		modID0: {
			{
				ref: varReference{
					name:         "deployment_name",
					toModuleID:   "vars",
					fromModuleID: "TestModule0",
					toGroupID:    globalGroupID,
					fromGroupID:  "primary",
					explicit:     false,
				},
				kind:            deploymentConnection,
				sharedVariables: []string{"deployment_name"},
			},
			{
				ref: varReference{
					name:         "project_id",
					toModuleID:   "vars",
					fromModuleID: "TestModule0",
					toGroupID:    globalGroupID,
					fromGroupID:  "primary",
					explicit:     false,
				},
				kind:            deploymentConnection,
				sharedVariables: []string{"project_id"},
			},
		},
		modID1: {
			{
				ref: modReference{
					toModuleID:   "TestModule0",
					fromModuleID: "TestModule1",
					toGroupID:    "primary",
					fromGroupID:  "primary",
					explicit:     true,
				},
				kind:            useConnection,
				sharedVariables: []string{"test_intra_0"},
			},
			{
				ref: varReference{
					name:         "test_intra_2",
					toModuleID:   "TestModule0",
					fromModuleID: "TestModule1",
					toGroupID:    "primary",
					fromGroupID:  "primary",
					explicit:     false,
				},
				kind:            explicitConnection,
				sharedVariables: []string{"test_intra_2"},
			},
		},
		modID2: {
			{
				ref: modReference{
					toModuleID:   "TestModule0",
					fromModuleID: "TestModule2",
					toGroupID:    "primary",
					fromGroupID:  "secondary",
					explicit:     true,
				},
				kind:            useConnection,
				sharedVariables: []string{"test_inter_0"},
			},
			{
				ref: varReference{
					name:         "deployment_name",
					toModuleID:   "vars",
					fromModuleID: "TestModule2",
					toGroupID:    globalGroupID,
					fromGroupID:  "secondary",
					explicit:     false,
				},
				kind:            deploymentConnection,
				sharedVariables: []string{"deployment_name"},
			},
		},
	})
}

func (s *MySuite) TestSetModulesInfo(c *C) {
	dc := getBasicDeploymentConfigWithTestModule()
	dc.setModulesInfo()
}

func (s *MySuite) TestCreateModuleInfo(c *C) {
	dc := getBasicDeploymentConfigWithTestModule()
	createModuleInfo(dc.Config.DeploymentGroups[0])
}

func (s *MySuite) TestGetResouceByID(c *C) {
	testID := "testID"

	// No Modules
	rg := DeploymentGroup{}
	got, err := rg.getModuleByID(testID)
	c.Assert(got, DeepEquals, Module{})
	c.Assert(err, NotNil)

	// No Match
	rg.Modules = []Module{{ID: "NoMatch"}}
	got, _ = rg.getModuleByID(testID)
	c.Assert(got, DeepEquals, Module{})
	c.Assert(err, NotNil)

	// Match
	expected := Module{ID: testID}
	rg.Modules = []Module{expected}
	got, err = rg.getModuleByID(testID)
	c.Assert(got, DeepEquals, expected)
	c.Assert(err, IsNil)

	dc := getBasicDeploymentConfigWithTestModule()
	groupID := dc.Config.DeploymentGroups[0].Name
	group, err := dc.getGroupByID(groupID)
	c.Assert(err, IsNil)
	c.Assert(group, DeepEquals, dc.Config.DeploymentGroups[0])

	badGroupID := "not-a-group"
	_, err = dc.getGroupByID(badGroupID)
	c.Assert(err, NotNil)
}

func (s *MySuite) TestHasKind(c *C) {
	// No Modules
	rg := DeploymentGroup{}
	c.Assert(rg.HasKind("terraform"), Equals, false)
	c.Assert(rg.HasKind("packer"), Equals, false)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// One terraform module
	rg.Modules = append(rg.Modules, Module{Kind: "terraform"})
	c.Assert(rg.HasKind("terraform"), Equals, true)
	c.Assert(rg.HasKind("packer"), Equals, false)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// Multiple terraform modules
	rg.Modules = append(rg.Modules, Module{Kind: "terraform"})
	rg.Modules = append(rg.Modules, Module{Kind: "terraform"})
	c.Assert(rg.HasKind("terraform"), Equals, true)
	c.Assert(rg.HasKind("packer"), Equals, false)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// One packer kind
	rg.Modules = []Module{{Kind: "packer"}}
	c.Assert(rg.HasKind("terraform"), Equals, false)
	c.Assert(rg.HasKind("packer"), Equals, true)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// One packer, one terraform
	rg.Modules = append(rg.Modules, Module{Kind: "terraform"})
	c.Assert(rg.HasKind("terraform"), Equals, true)
	c.Assert(rg.HasKind("packer"), Equals, true)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

}

func (s *MySuite) TestDeploymentName(c *C) {
	dc := getDeploymentConfigForTest()
	var e *InputValueError

	// Is deployment_name a valid string?
	deploymentName, err := dc.Config.DeploymentName()
	c.Assert(deploymentName, Equals, "deployment_name")
	c.Assert(err, IsNil)

	// Is deployment_name an empty string?
	dc.Config.Vars["deployment_name"] = ""
	deploymentName, err = dc.Config.DeploymentName()
	c.Assert(deploymentName, Equals, "")
	c.Check(errors.As(err, &e), Equals, true)

	// Is deployment_name not a string?
	dc.Config.Vars["deployment_name"] = 100
	deploymentName, err = dc.Config.DeploymentName()
	c.Assert(deploymentName, Equals, "")
	c.Check(errors.As(err, &e), Equals, true)

	// Is deployment_names longer than 63 characters?
	dc.Config.Vars["deployment_name"] = "deployment_name-deployment_name-deployment_name-deployment_name-0123"
	deploymentName, err = dc.Config.DeploymentName()
	c.Assert(deploymentName, Equals, "")
	c.Check(errors.As(err, &e), Equals, true)

	// Does deployment_name contain special characters other than dashes or underscores?
	dc.Config.Vars["deployment_name"] = "deployment.name"
	deploymentName, err = dc.Config.DeploymentName()
	c.Assert(deploymentName, Equals, "")
	c.Check(errors.As(err, &e), Equals, true)

	// Does deployment_name contain capital letters?
	dc.Config.Vars["deployment_name"] = "Deployment_name"
	deploymentName, err = dc.Config.DeploymentName()
	c.Assert(deploymentName, Equals, "")
	c.Check(errors.As(err, &e), Equals, true)

	// Is deployment_name not set?
	delete(dc.Config.Vars, "deployment_name")
	deploymentName, err = dc.Config.DeploymentName()
	c.Assert(deploymentName, Equals, "")
	c.Check(errors.As(err, &e), Equals, true)
}

func (s *MySuite) TestCheckBlueprintName(c *C) {
	dc := getDeploymentConfigForTest()
	var e *InputValueError

	// Is blueprint_name a valid string?
	err := dc.Config.checkBlueprintName()
	c.Assert(err, IsNil)

	// Is blueprint_name a valid string with an underscore and dash?
	dc.Config.BlueprintName = "blue-print_name"
	err = dc.Config.checkBlueprintName()
	c.Check(err, IsNil)

	// Is blueprint_name an empty string?
	dc.Config.BlueprintName = ""
	err = dc.Config.checkBlueprintName()
	c.Check(errors.As(err, &e), Equals, true)

	// Is blueprint_name longer than 63 characters?
	dc.Config.BlueprintName = "blueprint-name-blueprint-name-blueprint-name-blueprint-name-0123"
	err = dc.Config.checkBlueprintName()
	c.Check(errors.As(err, &e), Equals, true)

	// Does blueprint_name contain special characters other than dashes or underscores?
	dc.Config.BlueprintName = "blueprint.name"
	err = dc.Config.checkBlueprintName()
	c.Check(errors.As(err, &e), Equals, true)

	// Does blueprint_name contain capital letters?
	dc.Config.BlueprintName = "Blueprint_name"
	err = dc.Config.checkBlueprintName()
	c.Check(errors.As(err, &e), Equals, true)
}

func (s *MySuite) TestNewBlueprint(c *C) {
	dc := getDeploymentConfigForTest()
	outFile := filepath.Join(tmpTestDir, "out_TestNewBlueprint.yaml")
	dc.ExportBlueprint(outFile)
	newDC, err := NewDeploymentConfig(outFile)
	c.Assert(err, IsNil)
	c.Assert(dc.Config, DeepEquals, newDC.Config)
}

func (s *MySuite) TestImportBlueprint(c *C) {
	obtainedBlueprint, err := importBlueprint(simpleYamlFilename)
	c.Assert(err, IsNil)
	c.Assert(obtainedBlueprint.BlueprintName,
		Equals, expectedSimpleBlueprint.BlueprintName)
	c.Assert(
		len(obtainedBlueprint.Vars["labels"].(map[string]interface{})),
		Equals,
		len(expectedSimpleBlueprint.Vars["labels"].(map[string]interface{})),
	)
	c.Assert(obtainedBlueprint.DeploymentGroups[0].Modules[0].ID,
		Equals, expectedSimpleBlueprint.DeploymentGroups[0].Modules[0].ID)
}

func (s *MySuite) TestImportBlueprint_ExtraField_ThrowsError(c *C) {
	yaml := []byte(`
blueprint_name: hpc-cluster-high-io
# line below is not in our schema
dragon: "Lews Therin Telamon"`)
	file, _ := ioutil.TempFile("", "*.yaml")
	file.Write(yaml)
	filename := file.Name()
	file.Close()

	// should fail on strict unmarshal as field does not match schema
	_, err := importBlueprint(filename)
	c.Check(err, NotNil)
}

func (s *MySuite) TestExportBlueprint(c *C) {
	// Return bytes
	dc := DeploymentConfig{}
	dc.Config = expectedSimpleBlueprint
	obtainedYaml, err := dc.ExportBlueprint("")
	c.Assert(err, IsNil)
	c.Assert(obtainedYaml, Not(IsNil))

	// Write file
	outFilename := "out_TestExportBlueprint.yaml"
	outFile := filepath.Join(tmpTestDir, outFilename)
	dc.ExportBlueprint(outFile)
	fileInfo, err := os.Stat(outFile)
	c.Assert(err, IsNil)
	c.Assert(fileInfo.Name(), Equals, outFilename)
	c.Assert(fileInfo.Size() > 0, Equals, true)
	c.Assert(fileInfo.IsDir(), Equals, false)
}

func (s *MySuite) TestSetCLIVariables(c *C) {
	// Success
	dc := getBasicDeploymentConfigWithTestModule()
	c.Assert(dc.Config.Vars["project_id"], IsNil)
	c.Assert(dc.Config.Vars["deployment_name"], Equals, "deployment_name")
	c.Assert(dc.Config.Vars["region"], IsNil)
	c.Assert(dc.Config.Vars["zone"], IsNil)

	cliProjectID := "cli_test_project_id"
	cliDeploymentName := "cli_deployment_name"
	cliRegion := "cli_region"
	cliZone := "cli_zone"
	cliKeyVal := "key=val"
	cliKeyBool := "true"
	cliKeyInt := "15"
	cliKeyFloat := "15.43"
	cliKeyArray := "[1, 2, 3]"
	cliKeyMap := "{bar: baz, qux: 1}"
	cliKeyArrayOfMaps := "[foo, {bar: baz, qux: 1}]"
	cliKeyMapOfArrays := "{foo: [1, 2, 3], bar: [a, b, c]}"
	cliVars := []string{
		fmt.Sprintf("project_id=%s", cliProjectID),
		fmt.Sprintf("deployment_name=%s", cliDeploymentName),
		fmt.Sprintf("region=%s", cliRegion),
		fmt.Sprintf("zone=%s", cliZone),
		fmt.Sprintf("kv=%s", cliKeyVal),
		fmt.Sprintf("keyBool=%s", cliKeyBool),
		fmt.Sprintf("keyInt=%s", cliKeyInt),
		fmt.Sprintf("keyFloat=%s", cliKeyFloat),
		fmt.Sprintf("keyMap=%s", cliKeyMap),
		fmt.Sprintf("keyArray=%s", cliKeyArray),
		fmt.Sprintf("keyArrayOfMaps=%s", cliKeyArrayOfMaps),
		fmt.Sprintf("keyMapOfArrays=%s", cliKeyMapOfArrays),
	}
	err := dc.SetCLIVariables(cliVars)

	c.Assert(err, IsNil)
	c.Assert(dc.Config.Vars["project_id"], Equals, cliProjectID)
	c.Assert(dc.Config.Vars["deployment_name"], Equals, cliDeploymentName)
	c.Assert(dc.Config.Vars["region"], Equals, cliRegion)
	c.Assert(dc.Config.Vars["zone"], Equals, cliZone)
	c.Assert(dc.Config.Vars["kv"], Equals, cliKeyVal)

	// Bool in string is converted to bool
	boolValue, _ := strconv.ParseBool(cliKeyBool)
	c.Assert(dc.Config.Vars["keyBool"], Equals, boolValue)

	// Int in string is converted to int
	intValue, _ := strconv.Atoi(cliKeyInt)
	c.Assert(dc.Config.Vars["keyInt"], Equals, intValue)

	// Float in string is converted to float
	floatValue, _ := strconv.ParseFloat(cliKeyFloat, 64)
	c.Assert(dc.Config.Vars["keyFloat"], Equals, floatValue)

	// Map in string is converted to map
	mapValue := make(map[string]interface{})
	mapValue["bar"] = "baz"
	mapValue["qux"] = 1
	c.Assert(dc.Config.Vars["keyMap"], DeepEquals, mapValue)

	// Array in string is converted to array
	arrayValue := []interface{}{1, 2, 3}
	c.Assert(dc.Config.Vars["keyArray"], DeepEquals, arrayValue)

	// Array of maps in string is converted to array
	arrayOfMapsValue := []interface{}{"foo", mapValue}
	c.Assert(dc.Config.Vars["keyArrayOfMaps"], DeepEquals, arrayOfMapsValue)

	// Map of arrays in string is converted to array
	mapOfArraysValue := make(map[string]interface{})
	mapOfArraysValue["foo"] = []interface{}{1, 2, 3}
	mapOfArraysValue["bar"] = []interface{}{"a", "b", "c"}
	c.Assert(dc.Config.Vars["keyMapOfArrays"], DeepEquals, mapOfArraysValue)

	// Failure: Variable without '='
	dc = getBasicDeploymentConfigWithTestModule()
	c.Assert(dc.Config.Vars["project_id"], IsNil)

	invalidNonEQVars := []string{
		fmt.Sprintf("project_id%s", cliProjectID),
	}
	err = dc.SetCLIVariables(invalidNonEQVars)

	expErr := "invalid format: .*"
	c.Assert(err, ErrorMatches, expErr)
	c.Assert(dc.Config.Vars["project_id"], IsNil)
}

func (s *MySuite) TestSetBackendConfig(c *C) {
	// Success
	dc := getDeploymentConfigForTest()
	be := dc.Config.TerraformBackendDefaults
	c.Check(be, DeepEquals, TerraformBackend{})

	cliBEType := "gcs"
	cliBEBucket := "a_bucket"
	cliBESA := "a_bucket_reader@project.iam.gserviceaccount.com"
	cliBEPrefix := "test/prefix"
	cliBEConfigVars := []string{
		fmt.Sprintf("type=%s", cliBEType),
		fmt.Sprintf("bucket=%s", cliBEBucket),
		fmt.Sprintf("impersonate_service_account=%s", cliBESA),
		fmt.Sprintf("prefix=%s", cliBEPrefix),
	}
	err := dc.SetBackendConfig(cliBEConfigVars)

	c.Assert(err, IsNil)
	be = dc.Config.TerraformBackendDefaults
	c.Check(be.Type, Equals, cliBEType)
	c.Check(be.Configuration.Items(), DeepEquals, map[string]cty.Value{
		"bucket":                      cty.StringVal(cliBEBucket),
		"impersonate_service_account": cty.StringVal(cliBESA),
		"prefix":                      cty.StringVal(cliBEPrefix),
	})

	// Failure: Variable without '='
	dc = getDeploymentConfigForTest()
	c.Assert(dc.Config.TerraformBackendDefaults.Type, Equals, "")

	invalidNonEQVars := []string{
		fmt.Sprintf("type%s", cliBEType),
		fmt.Sprintf("bucket%s", cliBEBucket),
	}
	err = dc.SetBackendConfig(invalidNonEQVars)

	expErr := "invalid format: .*"
	c.Assert(err, ErrorMatches, expErr)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func (s *MySuite) TestValidationLevels(c *C) {
	var err error
	var ok bool
	dc := getDeploymentConfigForTest()
	validLevels := []string{"ERROR", "WARNING", "IGNORE"}
	for idx, level := range validLevels {
		err = dc.SetValidationLevel(level)
		c.Assert(err, IsNil)
		ok = isValidValidationLevel(idx)
		c.Assert(ok, Equals, true)
	}

	err = dc.SetValidationLevel("INVALID")
	c.Assert(err, NotNil)

	// check that our test for iota enum is working
	ok = isValidValidationLevel(-1)
	c.Assert(ok, Equals, false)
	invalidLevel := len(validLevels) + 1
	ok = isValidValidationLevel(invalidLevel)
	c.Assert(ok, Equals, false)
}

func (s *MySuite) TestIsLiteralVariable(c *C) {
	var matched bool
	matched = IsLiteralVariable("((var.project_id))")
	c.Assert(matched, Equals, true)
	matched = IsLiteralVariable("(( var.project_id ))")
	c.Assert(matched, Equals, true)
	matched = IsLiteralVariable("(var.project_id)")
	c.Assert(matched, Equals, false)
	matched = IsLiteralVariable("var.project_id")
	c.Assert(matched, Equals, false)
}

func (s *MySuite) TestIdentifyLiteralVariable(c *C) {
	var ctx, name string
	var ok bool
	ctx, name, ok = IdentifyLiteralVariable("((var.project_id))")
	c.Assert(ctx, Equals, "var")
	c.Assert(name, Equals, "project_id")
	c.Assert(ok, Equals, true)

	ctx, name, ok = IdentifyLiteralVariable("((module.structure.nested_value))")
	c.Assert(ctx, Equals, "module")
	c.Assert(name, Equals, "structure.nested_value")
	c.Assert(ok, Equals, true)

	// TODO: properly variables with periods in them!
	// One purpose of literal variables is to refer to values in nested
	// structures of a module output; should probably accept that case
	// but not global variables with periods in them
	ctx, name, ok = IdentifyLiteralVariable("var.project_id")
	c.Assert(ctx, Equals, "")
	c.Assert(name, Equals, "")
	c.Assert(ok, Equals, false)
}

func (s *MySuite) TestConvertToCty(c *C) {
	var testval interface{}
	var testcty cty.Value
	var err error

	testval = "test"
	testcty, err = ConvertToCty(testval)
	c.Assert(testcty.Type(), Equals, cty.String)
	c.Assert(err, IsNil)

	testval = complex(1, -1)
	testcty, err = ConvertToCty(testval)
	c.Assert(testcty.Type(), Equals, cty.NilType)
	c.Assert(err, NotNil)
}

func (s *MySuite) TestConvertMapToCty(c *C) {
	var testmap map[string]interface{}
	var testcty map[string]cty.Value
	var err error
	var testkey = "testkey"
	var testval = "testval"
	testmap = map[string]interface{}{
		testkey: testval,
	}

	testcty, err = ConvertMapToCty(testmap)
	c.Assert(err, IsNil)
	ctyval, found := testcty[testkey]
	c.Assert(found, Equals, true)
	c.Assert(ctyval.Type(), Equals, cty.String)

	testmap = map[string]interface{}{
		"testkey": complex(1, -1),
	}
	testcty, err = ConvertMapToCty(testmap)
	c.Assert(err, NotNil)
	_, found = testcty[testkey]
	c.Assert(found, Equals, false)
}

func (s *MySuite) TestCheckMovedModules(c *C) {

	dc := DeploymentConfig{
		Config: Blueprint{
			DeploymentGroups: []DeploymentGroup{
				{Modules: []Module{
					{Source: "some/module/that/has/not/moved"}}}}}}

	// base case should not err
	err := dc.checkMovedModules()
	c.Assert(err, IsNil)

	// embedded moved
	dc.Config.DeploymentGroups[0].Modules[0].Source = "community/modules/scheduler/cloud-batch-job"
	err = dc.checkMovedModules()
	c.Assert(err, NotNil)

	// local moved
	dc.Config.DeploymentGroups[0].Modules[0].Source = "./community/modules/scheduler/cloud-batch-job"
	err = dc.checkMovedModules()
	c.Assert(err, NotNil)
}

func (s *MySuite) TestValidatorConfigCheck(c *C) {
	const vn = testProjectExistsName // some valid name

	{ // FAIL: names mismatch
		v := validatorConfig{"who_is_this", map[string]interface{}{}, false}
		err := v.check(vn, []string{})
		c.Check(err, ErrorMatches, "passed wrong validator to test_project_exists implementation")
	}

	{ // OK: names match
		v := validatorConfig{vn.String(), map[string]interface{}{}, false}
		c.Check(v.check(vn, []string{}), IsNil)
	}

	{ // OK: Inputs is equal to required inputs without regard to ordering
		v := validatorConfig{
			vn.String(),
			map[string]interface{}{"in0": nil, "in1": nil},
			false,
		}
		c.Check(v.check(vn, []string{"in0", "in1"}), IsNil)
		c.Check(v.check(vn, []string{"in1", "in0"}), IsNil)
	}

	{ // FAIL: inputs are a proper subset of required inputs
		v := validatorConfig{
			vn.String(),
			map[string]interface{}{"in0": nil, "in1": nil},
			false,
		}
		err := v.check(vn, []string{"in0", "in1", "in2"})
		c.Check(err, ErrorMatches, missingRequiredInputRegex)
	}

	{ // FAIL: inputs intersect with required inputs but are not a proper subset
		v := validatorConfig{
			vn.String(),
			map[string]interface{}{"in0": nil, "in1": nil, "in3": nil},
			false,
		}
		err := v.check(vn, []string{"in0", "in1", "in2"})
		c.Check(err, ErrorMatches, missingRequiredInputRegex)
	}

	{ // FAIL inputs are a proper superset of required inputs
		v := validatorConfig{
			vn.String(),
			map[string]interface{}{"in0": nil, "in1": nil, "in2": nil, "in3": nil},
			false,
		}
		err := v.check(vn, []string{"in0", "in1", "in2"})
		c.Check(err, ErrorMatches, "only 3 inputs \\[in0 in1 in2\\] should be provided to test_project_exists")
	}
}

func (s *MySuite) TestCheckBackends(c *C) {
	// Helper to create blueprint with backend blocks only (first one is defaults)
	// and run checkBackends.
	check := func(d TerraformBackend, gb ...TerraformBackend) error {
		gs := []DeploymentGroup{}
		for _, b := range gb {
			gs = append(gs, DeploymentGroup{TerraformBackend: b})
		}
		bp := Blueprint{
			TerraformBackendDefaults: d,
			DeploymentGroups:         gs,
		}
		return checkBackends(bp)
	}
	dummy := TerraformBackend{}

	{ // OK. Absent
		c.Check(checkBackends(Blueprint{}), IsNil)
	}

	{ // OK. Dummies
		c.Check(check(dummy, dummy, dummy), IsNil)
	}

	{ // OK. No variables used
		b := TerraformBackend{Type: "gcs"}
		b.Configuration.
			Set("bucket", cty.StringVal("trenta")).
			Set("impersonate_service_account", cty.StringVal("who"))
		c.Check(check(b), IsNil)
	}

	{ // FAIL. Variable in defaults type
		b := TerraformBackend{Type: "$(vartype)"}
		c.Check(check(b), ErrorMatches, ".*type.*vartype.*")
	}

	{ // FAIL. Variable in group backend type
		b := TerraformBackend{Type: "$(vartype)"}
		c.Check(check(dummy, b), ErrorMatches, ".*type.*vartype.*")
	}

	{ // FAIL. Deployment variable in defaults type
		b := TerraformBackend{Type: "$(vars.type)"}
		c.Check(check(b), ErrorMatches, ".*type.*vars\\.type.*")
	}

	{ // OK. Not a variable
		b := TerraformBackend{Type: "\\$(vartype)"}
		c.Check(check(b), IsNil)
	}

	{ // FAIL. Mid-string variable in defaults type
		b := TerraformBackend{Type: "hugs_$(vartype)_hugs"}
		c.Check(check(b), ErrorMatches, ".*type.*vartype.*")
	}

	{ // FAIL. Variable in defaults configuration
		b := TerraformBackend{Type: "gcs"}
		b.Configuration.Set("bucket", cty.StringVal("$(trenta)"))
		c.Check(check(b), ErrorMatches, ".*bucket.*trenta.*")
	}

	{ // OK. handles nested configuration
		b := TerraformBackend{Type: "gcs"}
		b.Configuration.
			Set("bucket", cty.StringVal("trenta")).
			Set("complex", cty.MapVal(map[string]cty.Value{
				"alpha": cty.StringVal("a"),
				"beta":  cty.StringVal("b"),
			}))
		c.Check(check(b), IsNil)
	}
}

func (s *MySuite) TestSkipValidator(c *C) {
	{
		dc := DeploymentConfig{Config: Blueprint{Validators: nil}}
		c.Check(dc.SkipValidator("zebra"), IsNil)
		c.Check(dc.Config.Validators, DeepEquals, []validatorConfig{
			{Validator: "zebra", Skip: true}})
	}
	{
		dc := DeploymentConfig{Config: Blueprint{Validators: []validatorConfig{
			{Validator: "pony"}}}}
		c.Check(dc.SkipValidator("zebra"), IsNil)
		c.Check(dc.Config.Validators, DeepEquals, []validatorConfig{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}
	{
		dc := DeploymentConfig{Config: Blueprint{Validators: []validatorConfig{
			{Validator: "pony"},
			{Validator: "zebra"}}}}
		c.Check(dc.SkipValidator("zebra"), IsNil)
		c.Check(dc.Config.Validators, DeepEquals, []validatorConfig{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}
	{
		dc := DeploymentConfig{Config: Blueprint{Validators: []validatorConfig{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}}}}
		c.Check(dc.SkipValidator("zebra"), IsNil)
		c.Check(dc.Config.Validators, DeepEquals, []validatorConfig{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}
	{
		dc := DeploymentConfig{Config: Blueprint{Validators: []validatorConfig{
			{Validator: "zebra"},
			{Validator: "pony"},
			{Validator: "zebra"}}}}
		c.Check(dc.SkipValidator("zebra"), IsNil)
		c.Check(dc.Config.Validators, DeepEquals, []validatorConfig{
			{Validator: "zebra", Skip: true},
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}

}
