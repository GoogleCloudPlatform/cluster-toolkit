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
      project_id: project_name
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
		BlueprintName:    "simple",
		Vars:             map[string]interface{}{"labels": defaultLabels},
		DeploymentGroups: []DeploymentGroup{{Name: "DeploymentGroup1", TerraformBackend: TerraformBackend{}, Modules: testModules}},
		TerraformBackendDefaults: TerraformBackend{
			Type:          "",
			Configuration: map[string]interface{}{},
		},
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
		Validators:    []validatorConfig{},
		Vars:          map[string]interface{}{"deployment_name": "deployment_name"},
		TerraformBackendDefaults: TerraformBackend{
			Type:          "",
			Configuration: map[string]interface{}{},
		},
		DeploymentGroups: []DeploymentGroup{
			{
				Name: "group1",
				TerraformBackend: TerraformBackend{
					Type:          "",
					Configuration: map[string]interface{}{},
				},
				Modules: []Module{testModule, testModuleWithLabels},
			},
		},
	}

	return DeploymentConfig{
		Config: testBlueprint,
		ModulesInfo: map[string]map[string]modulereader.ModuleInfo{
			"group1": {
				testModuleSource:           testModuleInfo,
				testModuleSourceWithLabels: testModuleInfo,
			},
		},
	}
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

func getDeploymentConfigWithTestModuleEmtpyKind() DeploymentConfig {
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

func (s *MySuite) TestAddKindToModules(c *C) {
	/* Test addKindToModules() works when nothing to do */
	dc := getBasicDeploymentConfigWithTestModule()
	expected := dc.Config.DeploymentGroups[0].getModuleByID("TestModule1").Kind
	dc.addKindToModules()
	got := dc.Config.DeploymentGroups[0].getModuleByID("TestModule1").Kind
	c.Assert(got, Equals, expected)

	/* Test addKindToModules() works when kind is absent*/
	dc = getDeploymentConfigWithTestModuleEmtpyKind()
	expected = "terraform"
	dc.addKindToModules()
	got = dc.Config.DeploymentGroups[0].getModuleByID("TestModule1").Kind
	c.Assert(got, Equals, expected)

	/* Test addKindToModules() works when kind is empty*/
	dc = getDeploymentConfigWithTestModuleEmtpyKind()
	expected = "terraform"
	dc.addKindToModules()
	got = dc.Config.DeploymentGroups[0].getModuleByID("TestModule2").Kind
	c.Assert(got, Equals, expected)

	/* Test addKindToModules() does nothing to packer types*/
	moduleID := "packerModule"
	expected = "packer"
	dc = getDeploymentConfigWithTestModuleEmtpyKind()
	dc.Config.DeploymentGroups[0].Modules = append(dc.Config.DeploymentGroups[0].Modules, Module{ID: moduleID, Kind: expected})
	dc.addKindToModules()
	got = dc.Config.DeploymentGroups[0].getModuleByID(moduleID).Kind
	c.Assert(got, Equals, expected)

	/* Test addKindToModules() does nothing to invalid types*/
	moduleID = "funnyModule"
	expected = "funnyType"
	dc = getDeploymentConfigWithTestModuleEmtpyKind()
	dc.Config.DeploymentGroups[0].Modules = append(dc.Config.DeploymentGroups[0].Modules, Module{ID: moduleID, Kind: expected})
	dc.addKindToModules()
	got = dc.Config.DeploymentGroups[0].getModuleByID(moduleID).Kind
	c.Assert(got, Equals, expected)
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
	got := rg.getModuleByID(testID)
	c.Assert(got, DeepEquals, Module{})

	// No Match
	rg.Modules = []Module{{ID: "NoMatch"}}
	got = rg.getModuleByID(testID)
	c.Assert(got, DeepEquals, Module{})

	// Match
	expected := Module{ID: testID}
	rg.Modules = []Module{expected}
	got = rg.getModuleByID(testID)
	c.Assert(got, DeepEquals, expected)
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

func (s *MySuite) TestCheckModuleAndGroupNames(c *C) {
	dc := getDeploymentConfigForTest()
	checkModuleAndGroupNames(dc.Config.DeploymentGroups)
	testModID := dc.Config.DeploymentGroups[0].Modules[0].ID
	c.Assert(dc.ModuleToGroup[testModID], Equals, 0)
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
	cliVars := []string{
		fmt.Sprintf("project_id=%s", cliProjectID),
		fmt.Sprintf("deployment_name=%s", cliDeploymentName),
		fmt.Sprintf("region=%s", cliRegion),
		fmt.Sprintf("zone=%s", cliZone),
		fmt.Sprintf("kv=%s", cliKeyVal),
	}
	err := dc.SetCLIVariables(cliVars)

	c.Assert(err, IsNil)
	c.Assert(dc.Config.Vars["project_id"], Equals, cliProjectID)
	c.Assert(dc.Config.Vars["deployment_name"], Equals, cliDeploymentName)
	c.Assert(dc.Config.Vars["region"], Equals, cliRegion)
	c.Assert(dc.Config.Vars["zone"], Equals, cliZone)
	c.Assert(dc.Config.Vars["kv"], Equals, cliKeyVal)

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
	c.Assert(dc.Config.TerraformBackendDefaults.Type, Equals, "")
	c.Assert(dc.Config.TerraformBackendDefaults.Configuration["bucket"], IsNil)
	c.Assert(dc.Config.TerraformBackendDefaults.Configuration["impersonate_service_account"], IsNil)
	c.Assert(dc.Config.TerraformBackendDefaults.Configuration["prefix"], IsNil)

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
	c.Assert(dc.Config.TerraformBackendDefaults.Type, Equals, cliBEType)
	c.Assert(dc.Config.TerraformBackendDefaults.Configuration["bucket"], Equals, cliBEBucket)
	c.Assert(dc.Config.TerraformBackendDefaults.Configuration["impersonate_service_account"], Equals, cliBESA)
	c.Assert(dc.Config.TerraformBackendDefaults.Configuration["prefix"], Equals, cliBEPrefix)

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

func (s *MySuite) TestResolveGlobalVariables(c *C) {
	var err error
	var testkey1 = "testkey1"
	var testkey2 = "testkey2"
	var testkey3 = "testkey3"
	dc := getDeploymentConfigForTest()
	ctyMap := make(map[string]cty.Value)
	err = dc.Config.ResolveGlobalVariables(ctyMap)
	c.Assert(err, IsNil)

	// confirm plain string is unchanged and does not error
	testCtyString := cty.StringVal("testval")
	ctyMap[testkey1] = testCtyString
	err = dc.Config.ResolveGlobalVariables(ctyMap)
	c.Assert(err, IsNil)
	c.Assert(ctyMap[testkey1], Equals, testCtyString)

	// confirm literal, non-global, variable is unchanged and does not error
	testCtyString = cty.StringVal("((module.testval))")
	ctyMap[testkey1] = testCtyString
	err = dc.Config.ResolveGlobalVariables(ctyMap)
	c.Assert(err, IsNil)
	c.Assert(ctyMap[testkey1], Equals, testCtyString)

	// confirm failed resolution of a literal global
	testCtyString = cty.StringVal("((var.test_global_var))")
	ctyMap[testkey1] = testCtyString
	err = dc.Config.ResolveGlobalVariables(ctyMap)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*Unsupported attribute;.*")

	// confirm successful resolution of literal globals in presence of other strings
	testGlobalVarString := "test_global_string"
	testGlobalValString := "testval"
	testGlobalVarBool := "test_global_bool"
	testGlobalValBool := "testval"
	testPlainString := "plain-string"
	dc.Config.Vars[testGlobalVarString] = testGlobalValString
	dc.Config.Vars[testGlobalVarBool] = testGlobalValBool
	testCtyString = cty.StringVal(fmt.Sprintf("((var.%s))", testGlobalVarString))
	testCtyBool := cty.StringVal(fmt.Sprintf("((var.%s))", testGlobalVarBool))
	ctyMap[testkey1] = testCtyString
	ctyMap[testkey2] = testCtyBool
	ctyMap[testkey3] = cty.StringVal(testPlainString)
	err = dc.Config.ResolveGlobalVariables(ctyMap)
	c.Assert(err, IsNil)
	c.Assert(ctyMap[testkey1], Equals, cty.StringVal(testGlobalValString))
	c.Assert(ctyMap[testkey2], Equals, cty.StringVal(testGlobalValBool))
	c.Assert(ctyMap[testkey3], Equals, cty.StringVal(testPlainString))
}
