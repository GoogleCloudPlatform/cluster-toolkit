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

package config

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"testing"

	"hpc-toolkit/pkg/resreader"

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
			Source:           "./resources/network/vpc",
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

	// Create test directory with simple resources
	tmpTestDir, err = ioutil.TempDir("", "ghpc_config_tests_*")
	if err != nil {
		log.Fatalf("failed to create temp dir for config tests: %e", err)
	}
	resourceDir := path.Join(tmpTestDir, "resource")
	err = os.Mkdir(resourceDir, 0755)
	if err != nil {
		log.Fatalf("failed to create test resource dir: %v", err)
	}
	varFile, err := os.Create(path.Join(resourceDir, "variables.tf"))
	if err != nil {
		log.Fatalf("failed to create variables.tf in test resource dir: %v", err)
	}
	testVariablesTF := `
	variable "test_variable" {
		description = "Test Variable"
		type        = string
	}`
	_, err = varFile.WriteString(testVariablesTF)
	if err != nil {
		log.Fatalf("failed to write variables.tf in test resource dir: %v", err)
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

func getBlueprintConfigForTest() BlueprintConfig {
	testResourceSource := "testSource"
	testResource := Resource{
		Source:           testResourceSource,
		Kind:             "terraform",
		ID:               "testResource",
		Use:              []string{},
		WrapSettingsWith: make(map[string][]string),
		Settings:         make(map[string]interface{}),
	}
	testResourceSourceWithLabels := "./role/source"
	testResourceWithLabels := Resource{
		Source:           testResourceSourceWithLabels,
		ID:               "testResourceWithLabels",
		Kind:             "terraform",
		Use:              []string{},
		WrapSettingsWith: make(map[string][]string),
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
		TerraformBackendDefaults: TerraformBackend{
			Type:          "",
			Configuration: map[string]interface{}{},
		},
		ResourceGroups: []ResourceGroup{
			ResourceGroup{
				Name: "group1",
				TerraformBackend: TerraformBackend{
					Type:          "",
					Configuration: map[string]interface{}{},
				},
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

func getBasicBlueprintConfigWithTestResource() BlueprintConfig {
	testResourceSource := path.Join(tmpTestDir, "resource")
	testResourceGroup := ResourceGroup{
		Name: "primary",
		Resources: []Resource{
			Resource{
				ID:       "TestResource",
				Kind:     "terraform",
				Source:   testResourceSource,
				Settings: map[string]interface{}{"test_variable": "test_value"},
			},
		},
	}
	return BlueprintConfig{
		Config: YamlConfig{
			Vars:           make(map[string]interface{}),
			ResourceGroups: []ResourceGroup{testResourceGroup},
		},
	}
}

/* Tests */
// config.go
func (s *MySuite) TestExpandConfig(c *C) {
	bc := getBasicBlueprintConfigWithTestResource()
	bc.ExpandConfig()
}

func (s *MySuite) TestSetResourcesInfo(c *C) {
	bc := getBasicBlueprintConfigWithTestResource()
	bc.setResourcesInfo()
}

func (s *MySuite) TestCreateResourceInfo(c *C) {
	bc := getBasicBlueprintConfigWithTestResource()
	createResourceInfo(bc.Config.ResourceGroups[0])
}

func (s *MySuite) TestGetResouceByID(c *C) {
	testID := "testID"

	// No Resources
	rg := ResourceGroup{}
	got := rg.getResourceByID(testID)
	c.Assert(got, DeepEquals, Resource{})

	// No Match
	rg.Resources = []Resource{Resource{ID: "NoMatch"}}
	got = rg.getResourceByID(testID)
	c.Assert(got, DeepEquals, Resource{})

	// Match
	expected := Resource{ID: testID}
	rg.Resources = []Resource{expected}
	got = rg.getResourceByID(testID)
	c.Assert(got, DeepEquals, expected)
}

func (s *MySuite) TestHasKind(c *C) {
	// No resources
	rg := ResourceGroup{}
	c.Assert(rg.HasKind("terraform"), Equals, false)
	c.Assert(rg.HasKind("packer"), Equals, false)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// One terraform resources
	rg.Resources = append(rg.Resources, Resource{Kind: "terraform"})
	c.Assert(rg.HasKind("terraform"), Equals, true)
	c.Assert(rg.HasKind("packer"), Equals, false)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// Multiple terraform resources
	rg.Resources = append(rg.Resources, Resource{Kind: "terraform"})
	rg.Resources = append(rg.Resources, Resource{Kind: "terraform"})
	c.Assert(rg.HasKind("terraform"), Equals, true)
	c.Assert(rg.HasKind("packer"), Equals, false)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// One packer kind
	rg.Resources = []Resource{Resource{Kind: "packer"}}
	c.Assert(rg.HasKind("terraform"), Equals, false)
	c.Assert(rg.HasKind("packer"), Equals, true)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

	// One packer, one terraform
	rg.Resources = append(rg.Resources, Resource{Kind: "terraform"})
	c.Assert(rg.HasKind("terraform"), Equals, true)
	c.Assert(rg.HasKind("packer"), Equals, true)
	c.Assert(rg.HasKind("notAKind"), Equals, false)

}

func (s *MySuite) TestCheckResourceAndGroupNames(c *C) {
	bc := getBlueprintConfigForTest()
	checkResourceAndGroupNames(bc.Config.ResourceGroups)
	testResID := bc.Config.ResourceGroups[0].Resources[0].ID
	c.Assert(bc.ResourceToGroup[testResID], Equals, 0)
}

func (s *MySuite) TestNewBlueprint(c *C) {
	bc := getBlueprintConfigForTest()
	outFile := path.Join(tmpTestDir, "out_TestNewBlueprint.yaml")
	bc.ExportYamlConfig(outFile)
	newBC := NewBlueprintConfig(outFile)
	c.Assert(bc.Config, DeepEquals, newBC.Config)
}

func (s *MySuite) TestImportYamlConfig(c *C) {
	obtainedYamlConfig := importYamlConfig(simpleYamlFilename)
	c.Assert(obtainedYamlConfig.BlueprintName,
		Equals, expectedSimpleYamlConfig.BlueprintName)
	c.Assert(
		len(obtainedYamlConfig.Vars["labels"].(map[interface{}]interface{})),
		Equals,
		len(expectedSimpleYamlConfig.Vars["labels"].(map[string]interface{})),
	)
	c.Assert(obtainedYamlConfig.ResourceGroups[0].Resources[0].ID,
		Equals, expectedSimpleYamlConfig.ResourceGroups[0].Resources[0].ID)
}

func (s *MySuite) TestExportYamlConfig(c *C) {
	// Return bytes
	bc := BlueprintConfig{}
	bc.Config = expectedSimpleYamlConfig
	obtainedYaml := bc.ExportYamlConfig("")
	c.Assert(obtainedYaml, Not(IsNil))

	// Write file
	outFilename := "out_TestExportYamlConfig.yaml"
	outFile := path.Join(tmpTestDir, outFilename)
	bc.ExportYamlConfig(outFile)
	fileInfo, err := os.Stat(outFile)
	c.Assert(err, IsNil)
	c.Assert(fileInfo.Name(), Equals, outFilename)
	c.Assert(fileInfo.Size() > 0, Equals, true)
	c.Assert(fileInfo.IsDir(), Equals, false)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}
