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
	"hpc-toolkit/pkg/blueprintio"
	"hpc-toolkit/pkg/config"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	. "gopkg.in/check.v1"
)

var (
	testDir              string
	terraformResourceDir string
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

	// Create dummy resource in testDir
	terraformResourceDir = "tfResource"
	err = os.Mkdir(filepath.Join(testDir, terraformResourceDir), 0755)
	if err != nil {
		log.Fatal(err)
	}
}

func teardown() {
	os.RemoveAll(testDir)
}

// Test Data Producers
func getYamlConfigForTest() config.YamlConfig {
	testResourceSource := filepath.Join(testDir, terraformResourceDir)
	testResource := config.Resource{
		Source:   testResourceSource,
		Kind:     "terraform",
		ID:       "testResource",
		Settings: make(map[string]interface{}),
	}
	testResourceSourceWithLabels := filepath.Join(testDir, terraformResourceDir)
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

func createTestApplyFunctions(config config.YamlConfig) [][]map[string]string {
	applyFuncs := make([][]map[string]string, len(config.ResourceGroups))
	for iGrp, group := range config.ResourceGroups {
		applyFuncs[iGrp] = make([]map[string]string, len(group.Resources))
	}
	return applyFuncs
}

// Tests

// reswriter.go
func (s *MySuite) TestWriteBlueprint(c *C) {
	testYamlConfig := getYamlConfigForTest()
	blueprintName := "blueprints_TestWriteBlueprint"
	testYamlConfig.BlueprintName = blueprintName
	WriteBlueprint(&testYamlConfig, testDir)
}

func (s *MySuite) TestFlattenInterfaceMap(c *C) {
	wrapper := interfaceStruct{Elem: nil}
	inputMaps := []interface{}{
		// Just a string
		"str1",
		// map of strings
		map[string]interface{}{
			"str1": "val1",
			"str2": "val2",
		},
		// slice of strings
		[]interface{}{"str1", "str2"},
		// map of maps
		map[string]interface{}{
			"map1": map[string]interface{}{},
			"map2": map[string]interface{}{
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
		map[string]interface{}{
			"slice": []map[string]interface{}{
				map[string]interface{}{
					"str1": "val1",
					"str2": "val2",
				},
			},
		},
		// empty map
		map[string]interface{}{},
		// empty slice
		[]interface{}{},
	}
	// map of all 3
	inputMapAllThree := map[string]interface{}{
		"str": "val",
		"map": map[string]interface{}{
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
		err := flattenInterfaceMap(mapWrapper, &wrapper)
		c.Assert(err, IsNil)
		c.Assert(mapWrapper["key"], Equals, expectedOutputs[i])
	}

	// Test complicated case
	mapWrapper["key"] = inputMapAllThree
	err := flattenInterfaceMap(mapWrapper, &wrapper)
	c.Assert(err, IsNil)
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
	testMap := map[string]interface{}{
		"stringMap":   addQuotes,
		"variableMap": noQuotes,
		"deep": map[string]interface{}{
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
	interfaceMap := varMap["map"].(map[string]interface{})
	c.Assert(interfaceMap["\"stringMap\""],
		Equals,
		addQuotesExpected)
	c.Assert(interfaceMap["\"variableMap\""], Equals, noQuotesExpected)
	interfaceMap = interfaceMap["\"deep\""].(map[string]interface{})
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

// tfwriter.go
func (s *MySuite) TestGetTypeTokens(c *C) {
	// Success Integer
	tok := getTypeTokens(cty.NumberIntVal(-1))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	tok = getTypeTokens(cty.NumberIntVal(0))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	tok = getTypeTokens(cty.NumberIntVal(1))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	// Success Float
	tok = getTypeTokens(cty.NumberFloatVal(-99.9))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	tok = getTypeTokens(cty.NumberFloatVal(99.9))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	// Success String
	tok = getTypeTokens(cty.StringVal("Lorum"))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("string")))

	tok = getTypeTokens(cty.StringVal(""))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("string")))

	// Success Bool
	tok = getTypeTokens(cty.BoolVal(true))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("bool")))

	tok = getTypeTokens(cty.BoolVal(false))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("bool")))

	// Success tuple
	tok = getTypeTokens(cty.TupleVal([]cty.Value{}))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("list")))

	tok = getTypeTokens(cty.TupleVal([]cty.Value{cty.StringVal("Lorum")}))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("list")))

	// Success list
	tok = getTypeTokens(cty.ListVal([]cty.Value{cty.StringVal("Lorum")}))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("list")))

	// Success object
	tok = getTypeTokens(cty.ObjectVal(map[string]cty.Value{}))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("map")))

	val := cty.ObjectVal(map[string]cty.Value{"Lorum": cty.StringVal("Ipsum")})
	tok = getTypeTokens(val)
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("map")))

	// Success Map
	val = cty.MapVal(map[string]cty.Value{"Lorum": cty.StringVal("Ipsum")})
	tok = getTypeTokens(val)
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("map")))

	// Failure
	tok = getTypeTokens(cty.NullVal(cty.DynamicPseudoType))
	c.Assert(len(tok), Equals, 1)

}

func (s *MySuite) TestSimpleTokenFromString(c *C) {
	inputString := "Lorem"
	tok := simpleTokenFromString("Lorem")
	c.Assert(tok.Type, Equals, hclsyntax.TokenIdent)
	c.Assert(len(tok.Bytes), Equals, len(inputString))
	c.Assert(string(tok.Bytes), Equals, inputString)
}

func (s *MySuite) TestCreateBaseFile(c *C) {
	// Success
	baseFilename := "main.tf_TestCreateBaseFile"
	goodPath := filepath.Join(testDir, baseFilename)
	err := createBaseFile(goodPath)
	c.Assert(err, IsNil)
	fi, err := os.Stat(goodPath)
	c.Assert(err, IsNil)
	c.Assert(fi.Name(), Equals, baseFilename)
	c.Assert(fi.Size() > 0, Equals, true)
	c.Assert(fi.IsDir(), Equals, false)
	b, err := ioutil.ReadFile(goodPath)
	c.Assert(strings.Contains(string(b), "Licensed under the Apache License"),
		Equals, true)

	// Error: not a correct path
	fakePath := filepath.Join("not/a/real/dir", "main.tf_TestCreateBaseFile")
	err = createBaseFile(fakePath)
	c.Assert(err, ErrorMatches, ".* no such file or directory")
}

func (s *MySuite) TestAppendHCLToFile(c *C) {
	// Setup
	testFilename := "main.tf_TestAppendHCLToFile"
	testPath := filepath.Join(testDir, testFilename)
	_, err := os.Create(testPath)
	c.Assert(err, IsNil)
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()
	hclBody.SetAttributeValue("dummyAttributeName", cty.NumberIntVal(0))

	// Success
	err = appendHCLToFile(testPath, hclFile.Bytes())
	c.Assert(err, IsNil)
}

func stringExistsInFile(str string, filename string) (bool, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(b), str), nil
}

// hcl_utils.go
func (s *MySuite) TestFlattenToHCLStrings(c *C) {
	testConfig := getYamlConfigForTest()
	flattenToHCLStrings(&testConfig, "terraform")
}

func (s *MySuite) TestGetType(c *C) {

	// string
	testString := "test string"
	ret := getType(testString)
	c.Assert(ret, Equals, "string")

	// map
	testMap := "{testMap: testVal}"
	ret = getType(testMap)
	c.Assert(ret, Equals, "map")

	// list
	testList := "[testList0,testList]"
	ret = getType(testList)
	c.Assert(ret, Equals, "list")

	// non-string input
	testNull := 42 // random int
	ret = getType(testNull)
	c.Assert(ret, Equals, "null")

	// nil input
	ret = getType(nil)
	c.Assert(ret, Equals, "null")
}

func (s *MySuite) TestWriteMain(c *C) {
	// Setup
	testMainDir := filepath.Join(testDir, "TestWriteMain")
	mainFilePath := filepath.Join(testMainDir, "main.tf")
	if err := os.Mkdir(testMainDir, 0755); err != nil {
		log.Fatal("Failed to create test dir for creating main.tf file")
	}

	// Simple success
	testResources := []config.Resource{}
	testBackend := config.TerraformBackend{}
	err := writeMain(testResources, testBackend, testMainDir)
	c.Assert(err, IsNil)

	// Test with resource
	testResource := config.Resource{
		ID: "test_resource",
		Settings: map[string]interface{}{
			"testSetting": "testValue",
		},
	}
	testResources = append(testResources, testResource)
	err = writeMain(testResources, testBackend, testMainDir)
	c.Assert(err, IsNil)
	exists, err := stringExistsInFile("testSetting", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Test with labels setting
	testResource.Settings["labels"] = map[string]interface{}{
		"ghpc_role":    "testResource",
		"custom_label": "",
	}
	err = writeMain(testResources, testBackend, testMainDir)
	c.Assert(err, IsNil)
	exists, err = stringExistsInFile("custom_label", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
	exists, err = stringExistsInFile("var.labels", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Test with Backend
	testBackend.Type = "gcs"
	testBackend.Configuration = map[string]interface{}{
		"bucket": "a_bucket",
	}
	err = writeMain(testResources, testBackend, testMainDir)
	c.Assert(err, IsNil)
	exists, err = stringExistsInFile("a_bucket", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Test with WrapSettingsWith
	testResourceWithWrap := config.Resource{
		ID: "test_resource_with_wrap",
		WrapSettingsWith: map[string][]string{
			"wrappedSetting": []string{"list(flatten(", "))"},
		},
		Settings: map[string]interface{}{
			"wrappedSetting": []interface{}{"val1", "val2"},
		},
	}
	testResources = append(testResources, testResourceWithWrap)
	err = writeMain(testResources, testBackend, testMainDir)
	c.Assert(err, IsNil)
	exists, err = stringExistsInFile("list(flatten(", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

func (s *MySuite) TestWriteOutputs(c *C) {
	// Setup
	testOutputsDir := filepath.Join(testDir, "TestWriteOutputs")
	outputsFilePath := filepath.Join(testOutputsDir, "outputs.tf")
	if err := os.Mkdir(testOutputsDir, 0755); err != nil {
		log.Fatal("Failed to create test directory for creating outputs.tf file")
	}

	// Simple success, no resources
	testResources := []config.Resource{}
	err := writeOutputs(testResources, testOutputsDir)
	c.Assert(err, IsNil)

	// Failure: Bad path
	err = writeOutputs(testResources, "not/a/real/path")
	c.Assert(err, ErrorMatches, "error creating outputs.tf file: .*")

	// Success: Outputs added
	outputList := []string{"output1", "output2"}
	resourceWithOutputs := config.Resource{Outputs: outputList, ID: "testRes"}
	testResources = []config.Resource{resourceWithOutputs}
	err = writeOutputs(testResources, testOutputsDir)
	c.Assert(err, IsNil)
	exists, err := stringExistsInFile(outputList[0], outputsFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
	exists, err = stringExistsInFile(outputList[1], outputsFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

func (s *MySuite) TestWriteVariables(c *C) {
	// Setup
	testVarDir := filepath.Join(testDir, "TestWriteVariables")
	varsFilePath := filepath.Join(testVarDir, "variables.tf")
	if err := os.Mkdir(testVarDir, 0755); err != nil {
		log.Fatal("Failed to create test directory for creating variables.tf file")
	}

	// Simple success, empty vars
	testVars := make(map[string]cty.Value)
	err := writeVariables(testVars, testVarDir)
	c.Assert(err, IsNil)

	// Failure: Bad path
	err = writeVariables(testVars, "not/a/real/path")
	c.Assert(err, ErrorMatches, "error creating variables.tf file: .*")

	// Success, common vars
	testVars["deployment_name"] = cty.StringVal("test_deployment")
	testVars["project_id"] = cty.StringVal("test_project")
	err = writeVariables(testVars, testVarDir)
	c.Assert(err, IsNil)
	exists, err := stringExistsInFile("\"deployment_name\"", varsFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Success, "dynamic type"
	testVars = make(map[string]cty.Value)
	testVars["project_id"] = cty.NullVal(cty.DynamicPseudoType)
	err = writeVariables(testVars, testVarDir)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestWriteProviders(c *C) {
	// Setup
	testProvDir := filepath.Join(testDir, "TestWriteProviders")
	provFilePath := filepath.Join(testProvDir, "providers.tf")
	if err := os.Mkdir(testProvDir, 0755); err != nil {
		log.Fatal("Failed to create test directory for creating providers.tf file")
	}

	// Simple success, empty vars
	testVars := make(map[string]cty.Value)
	err := writeProviders(testVars, testProvDir)
	c.Assert(err, IsNil)
	exists, err := stringExistsInFile("google-beta", provFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
	exists, err = stringExistsInFile("project", provFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	// Failure: Bad Path
	err = writeProviders(testVars, "not/a/real/path")
	c.Assert(err, ErrorMatches, "error creating providers.tf file: .*")

	// Success: All vars
	testVars["project_id"] = cty.StringVal("test_project")
	testVars["zone"] = cty.StringVal("test_zone")
	testVars["region"] = cty.StringVal("test_region")
	err = writeProviders(testVars, testProvDir)
	c.Assert(err, IsNil)
	exists, err = stringExistsInFile("var.region", provFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

// packerwriter.go
func (s *MySuite) TestNumResources_PackerWriter(c *C) {
	testWriter := PackerWriter{}
	c.Assert(testWriter.getNumResources(), Equals, 0)
	testWriter.addNumResources(-1)
	c.Assert(testWriter.getNumResources(), Equals, -1)
	testWriter.addNumResources(2)
	c.Assert(testWriter.getNumResources(), Equals, 1)
	testWriter.addNumResources(0)
	c.Assert(testWriter.getNumResources(), Equals, 1)
}

func (s *MySuite) TestWriteResourceLevel_PackerWriter(c *C) {
	blueprintio := blueprintio.GetBlueprintIOLocal()
	testWriter := PackerWriter{}
	// Empty Config
	testWriter.writeResourceLevel(&config.YamlConfig{}, testDir)

	// No Packer resources
	testYamlConfig := getYamlConfigForTest()
	testWriter.writeResourceLevel(&testYamlConfig, testDir)

	blueprintName := "blueprints_TestWriteResourceLevel_PackerWriter"
	testYamlConfig.BlueprintName = blueprintName
	blueprintDir := filepath.Join(testDir, blueprintName)
	if err := blueprintio.CreateDirectory(blueprintDir); err != nil {
		log.Fatal(err)
	}
	groupDir := filepath.Join(blueprintDir, "packerGroup")
	if err := blueprintio.CreateDirectory(groupDir); err != nil {
		log.Fatal(err)
	}
	resourceDir := filepath.Join(groupDir, "testPackerResource")
	if err := blueprintio.CreateDirectory(resourceDir); err != nil {
		log.Fatal(err)
	}

	testPackerResource := config.Resource{
		Kind: "packer",
		ID:   "testPackerResource",
	}
	testYamlConfig.ResourceGroups = append(testYamlConfig.ResourceGroups,
		config.ResourceGroup{
			Name:      "packerGroup",
			Resources: []config.Resource{testPackerResource},
		})
	testWriter.writeResourceLevel(&testYamlConfig, testDir)
	_, err := os.Stat(filepath.Join(resourceDir, packerAutoVarFilename))
	c.Assert(err, IsNil)
}

func (s *MySuite) TestWritePackerAutoVariables(c *C) {
	// The happy path is tested outside of this funcation already

	// Bad tmplFilename
	badDestPath := "not/a/real/path"
	err := writePackerAutoVariables(
		packerAutoVarFilename, config.Resource{}, badDestPath)
	expErr := "failed to create packer file .*"
	c.Assert(err, ErrorMatches, expErr)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}
