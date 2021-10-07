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

package reswriter

import (
	"fmt"
	"hpc-toolkit/pkg/config"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	. "gopkg.in/check.v1"
)

var (
	testDir string

	resources = []config.Resource{
		config.Resource{
			Source: "../../resources/network/vpc",
			Kind:   "terraform",
			ID:     "test-net",
			Settings: map[string]interface{}{
				"network_name": "deployment_name",
				"project_id":   "project_id",
				"region":       "region1",
			},
		},
	}

	resourceGroups = []config.ResourceGroup{
		config.ResourceGroup{
			Name:      "group1",
			Resources: resources,
		},
	}

	yamlConfig = config.YamlConfig{
		BlueprintName:  "UPDATE_IN_TEST",
		ResourceGroups: resourceGroups,
	}
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func setup() {
	t := time.Now()
	dirName := fmt.Sprintf("ghpc_reswriter_test_%s", t.Format(time.RFC3339))
	dir, err := ioutil.TempDir("", dirName)
	if err != nil {
		log.Fatalf("reswriter_test: %v", err)
	}
	testDir = dir
}

func teardown() {
	os.RemoveAll(testDir)
}

// Test Data Producer
func getYamlConfigForTest() config.YamlConfig {
	testResourceSource := "testSource"
	testResource := config.Resource{
		Source:   testResourceSource,
		Kind:     "terraform",
		ID:       "testResource",
		Settings: make(map[string]interface{}),
	}
	testResourceSourceWithLabels := "./role/source"
	testResourceWithLabels := config.Resource{
		Source: testResourceSourceWithLabels,
		ID:     "testResourceWithLabels",
		Kind:   "terraform",
		Settings: map[string]interface{}{
			"resourceLabel": "resourceLabelValue",
		},
	}
	testResourceGroups := []config.ResourceGroup{
		config.ResourceGroup{
			Resources: []config.Resource{testResource, testResourceWithLabels},
		},
	}
	testYamlConfig := config.YamlConfig{
		BlueprintName:  "simple",
		Vars:           map[string]interface{}{},
		ResourceGroups: testResourceGroups,
	}

	return testYamlConfig
}

// Tests
func (s *MySuite) TestFlattenInterfaceMap(c *C) {
	wrapper := interfaceStruct{Elem: nil}
	inputMaps := []interface{}{
		// Just a string
		"str1",
		// map of strings
		map[interface{}]interface{}{
			"str1": "val1",
			"str2": "val2",
		},
		// slice of strings
		[]interface{}{"str1", "str2"},
		// map of maps
		map[interface{}]interface{}{
			"map1": map[interface{}]interface{}{},
			"map2": map[interface{}]interface{}{
				"str1": "val1",
				"str2": "val2",
			},
		},
		// slice of slices
		[]interface{}{
			[]interface{}{},
			[]interface{}{"str1", "str2"},
		},
		// map of slice of map
		map[interface{}]interface{}{
			"slice": []map[interface{}]interface{}{
				map[interface{}]interface{}{
					"str1": "val1",
					"str2": "val2",
				},
			},
		},
		// empty map
		map[interface{}]interface{}{},
		// empty slice
		[]interface{}{},
	}
	// map of all 3
	inputMapAllThree := map[interface{}]interface{}{
		"str": "val",
		"map": map[interface{}]interface{}{
			"str1": "val1",
			"str2": "val2",
		},
		"slice": []interface{}{"str1", "str2"},
	}
	stringMapContents := "{str1: val1, str2: val2}"
	stringSliceContents := "[str1, str2]"
	expectedOutputs := []string{
		"str1",              // Just a string
		stringMapContents,   // map of strings
		stringSliceContents, // slice of strings
		fmt.Sprintf("{map1: {}, map2: %s}", stringMapContents), // map of maps
		fmt.Sprintf("[[], %s]", stringSliceContents),           // slice of slices
		fmt.Sprintf("{slice: [%s]}", stringMapContents),        // map of slice of map
		"{}",
		"[]",
	}

	// Test the test setup
	c.Assert(len(inputMaps), Equals, len(expectedOutputs))

	// Test common cases
	mapWrapper := make(map[string]interface{})
	for i := range inputMaps {
		mapWrapper["key"] = inputMaps[i]
		flattenInterfaceMap(mapWrapper, &wrapper)
		c.Assert(mapWrapper["key"], Equals, expectedOutputs[i])
	}

	// Test complicated case
	mapWrapper["key"] = inputMapAllThree
	flattenInterfaceMap(mapWrapper, &wrapper)
	c.Assert(
		strings.Contains(mapWrapper["key"].(string), "str: val"), Equals, true)
	mapString := fmt.Sprintf("map: %s", stringMapContents)
	c.Assert(
		strings.Contains(mapWrapper["key"].(string), mapString), Equals, true)
	sliceString := fmt.Sprintf("slice: %s", stringSliceContents)
	c.Assert(
		strings.Contains(mapWrapper["key"].(string), sliceString), Equals, true)
}

func testHandlePrimitivesCreateMap() map[string]interface{} {
	// String test variables
	addQuotes := "addQuotes"
	noQuotes := "((noQuotes))"

	// Composite test variables
	testMap := map[interface{}]interface{}{
		"stringMap":   addQuotes,
		"variableMap": noQuotes,
		"deep": map[interface{}]interface{}{
			"slice": []interface{}{addQuotes, noQuotes},
		},
	}
	testSlice := []interface{}{addQuotes, noQuotes}

	return map[string]interface{}{
		"string":   addQuotes,
		"variable": noQuotes,
		"map":      testMap,
		"slice":    testSlice,
	}
}

func testHandlePrimitivesHelper(c *C, varMap map[string]interface{}) {
	addQuotesExpected := fmt.Sprintf("\"%s\"", "addQuotes")
	noQuotesExpected := "noQuotes"

	// Test top level
	c.Assert(varMap["string"], Equals, addQuotesExpected)
	c.Assert(varMap["variable"], Equals, noQuotesExpected)

	// Test map
	interfaceMap := varMap["map"].(map[interface{}]interface{})
	c.Assert(interfaceMap["\"stringMap\""],
		Equals,
		addQuotesExpected)
	c.Assert(interfaceMap["\"variableMap\""], Equals, noQuotesExpected)
	interfaceMap = interfaceMap["\"deep\""].(map[interface{}]interface{})
	interfaceSlice := interfaceMap["\"slice\""].([]interface{})
	c.Assert(interfaceSlice[0], Equals, addQuotesExpected)
	c.Assert(interfaceSlice[1], Equals, noQuotesExpected)

	// Test slice
	interfaceSlice = varMap["slice"].([]interface{})
	c.Assert(interfaceSlice[0], Equals, addQuotesExpected)
	c.Assert(interfaceSlice[1], Equals, noQuotesExpected)
}

func (s *MySuite) TestUpdateStrings(c *C) {
	yamlConfig := getYamlConfigForTest()

	// Setup Vars
	yamlConfig.Vars = testHandlePrimitivesCreateMap()
	yamlConfig.ResourceGroups[0].Resources[0].Settings =
		testHandlePrimitivesCreateMap()

	updateStringsInConfig(&yamlConfig, "terraform")

	testHandlePrimitivesHelper(
		c, yamlConfig.ResourceGroups[0].Resources[0].Settings)

}

func (s *MySuite) TestCreateBlueprintDirectory(c *C) {
	blueprintName := "blueprints_TestCreateBlueprintDirectory"
	blueprintDir := path.Join(testDir, blueprintName)
	createBlueprintDirectory(blueprintDir)
	_, err := os.Stat(blueprintDir)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestCopySource(c *C) {
	blueprintName := "blueprints_TestCopySource"
	blueprintDir := path.Join(testDir, blueprintName)
	createBlueprintDirectory(blueprintDir)
	copySource(blueprintDir, &resourceGroups)
	_, err := os.Stat(
		path.Join(
			blueprintDir,
			resourceGroups[0].Name,
			"modules",
			resources[0].ResourceName))
	c.Assert(err, IsNil)
}

func (s *MySuite) TestWriteTopLayer_TFWriter(c *C) {
	// Shallow copy the struct so we can set the name
	blueprintName := "blueprints_TestWriteTopLayer_TFWriter"
	blueprintDir := path.Join(testDir, blueprintName)
	maintfPath := path.Join(blueprintDir, "group", "main.tf")

	createBlueprintDirectory(blueprintDir)
	// Normally handled by copySource, do it manually
	os.Mkdir(path.Join(blueprintDir, "group"), 0755)
	writeTopTerraformFile(blueprintDir, "group", "main.tf", resourceGroups[0])

	_, err := os.Stat(maintfPath)
	c.Assert(err, IsNil)

	dat, err := ioutil.ReadFile(maintfPath)
	text := string(dat)
	c.Assert(err, IsNil)
	c.Assert(len(strings.Split(text, "\n")) > 28, Equals, true)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}
