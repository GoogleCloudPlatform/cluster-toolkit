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

package modulewriter

import (
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/deploymentio"
	"hpc-toolkit/pkg/sourcereader"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	. "gopkg.in/check.v1"
)

var (
	testDir            string
	terraformModuleDir string
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func setup() {
	t := time.Now()
	dirName := fmt.Sprintf("ghpc_modulewriter_test_%s", t.Format(time.RFC3339))
	dir, err := ioutil.TempDir("", dirName)
	if err != nil {
		log.Fatalf("modulewriter_test: %v", err)
	}
	testDir = dir

	// Create dummy module in testDir
	terraformModuleDir = "tfModule"
	err = os.Mkdir(filepath.Join(testDir, terraformModuleDir), 0755)
	if err != nil {
		log.Fatal(err)
	}
}

func teardown() {
	os.RemoveAll(testDir)
}

// Test Data Producers
func getBlueprintForTest() config.Blueprint {
	testModuleSource := filepath.Join(testDir, terraformModuleDir)
	testModule := config.Module{
		Source:   testModuleSource,
		Kind:     "terraform",
		ID:       "testModule",
		Settings: make(map[string]interface{}),
	}
	testModuleSourceWithLabels := filepath.Join(testDir, terraformModuleDir)
	testModuleWithLabels := config.Module{
		Source: testModuleSourceWithLabels,
		ID:     "testModuleWithLabels",
		Kind:   "terraform",
		Settings: map[string]interface{}{
			"moduleLabel": "moduleLabelValue",
		},
	}
	testDeploymentGroups := []config.DeploymentGroup{
		{
			Name:    "test_resource_group",
			Kind:    "terraform",
			Modules: []config.Module{testModule, testModuleWithLabels},
		},
	}
	testBlueprint := config.Blueprint{
		BlueprintName:    "simple",
		Vars:             map[string]interface{}{},
		DeploymentGroups: testDeploymentGroups,
	}

	return testBlueprint
}

// Tests

func isDeploymentDirPrepped(depDirectoryPath string) error {
	if _, err := os.Stat(depDirectoryPath); os.IsNotExist(err) {
		return fmt.Errorf("deloyment dir does not exist: %s: %w", depDirectoryPath, err)
	}

	ghpcDir := filepath.Join(depDirectoryPath, hiddenGhpcDirName)
	if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
		return fmt.Errorf(".ghpc working dir does not exist: %s: %w", ghpcDir, err)
	}

	prevModuleDir := filepath.Join(ghpcDir, prevDeploymentGroupDirName)
	if _, err := os.Stat(prevModuleDir); os.IsNotExist(err) {
		return fmt.Errorf("previous deployment group directory does not exist: %s: %w", prevModuleDir, err)
	}

	return nil
}

func (s *MySuite) TestPrepDepDir(c *C) {

	depDir := filepath.Join(testDir, "dep_prep_test_dir")

	// Prep a dir that does not yet exist
	err := prepDepDir(depDir, false /* overwrite */)
	c.Check(err, IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)

	// Prep of existing dir fails with overwrite set to false
	err = prepDepDir(depDir, false /* overwrite */)
	var e *OverwriteDeniedError
	c.Check(errors.As(err, &e), Equals, true)

	// Prep of existing dir succeeds when overwrite set true
	err = prepDepDir(depDir, true) /* overwrite */
	c.Check(err, IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)
}

func (s *MySuite) TestPrepDepDir_OverwriteRealDep(c *C) {
	// Test with a real deployment previously written
	testBlueprint := getBlueprintForTest()
	testBlueprint.Vars = map[string]interface{}{"deployment_name": "test_prep_dir"}
	realDepDir := filepath.Join(testDir, testBlueprint.Vars["deployment_name"].(string))

	// writes a full deployment w/ actual resource groups
	WriteDeployment(&testBlueprint, testDir, false /* overwrite */)

	// confirm existence of resource groups (beyond .ghpc dir)
	files, _ := ioutil.ReadDir(realDepDir)
	c.Check(len(files) > 1, Equals, true)

	err := prepDepDir(realDepDir, true /* overwrite */)
	c.Check(err, IsNil)
	c.Check(isDeploymentDirPrepped(realDepDir), IsNil)

	// Check prev resource groups were moved
	prevModuleDir := filepath.Join(testDir, testBlueprint.Vars["deployment_name"].(string), hiddenGhpcDirName, prevDeploymentGroupDirName)
	files1, _ := ioutil.ReadDir(prevModuleDir)
	c.Check(len(files1) > 0, Equals, true)

	files2, _ := ioutil.ReadDir(realDepDir)
	c.Check(len(files2), Equals, 2) // .ghpc and .gitignore
}

func (s *MySuite) TestIsSubset(c *C) {
	baseConfig := []string{"group1", "group2", "group3"}
	subsetConfig := []string{"group1", "group2"}
	swapConfig := []string{"group1", "group4", "group3"}
	c.Check(isSubset(subsetConfig, baseConfig), Equals, true)
	c.Check(isSubset(baseConfig, subsetConfig), Equals, false)
	c.Check(isSubset(baseConfig, swapConfig), Equals, false)
}

func (s *MySuite) TestIsOverwriteAllowed(c *C) {
	depDir := filepath.Join(testDir, "overwrite_test")
	ghpcDir := filepath.Join(depDir, hiddenGhpcDirName)
	module1 := filepath.Join(depDir, "group1")
	module2 := filepath.Join(depDir, "group2")
	os.MkdirAll(ghpcDir, 0755)
	os.MkdirAll(module1, 0755)
	os.MkdirAll(module2, 0755)

	supersetConfig := config.Blueprint{
		DeploymentGroups: []config.DeploymentGroup{
			{Name: "group1"},
			{Name: "group2"},
			{Name: "group3"},
		},
	}
	swapConfig := config.Blueprint{
		DeploymentGroups: []config.DeploymentGroup{
			{Name: "group1"},
			{Name: "group4"},
		},
	}

	// overwrite allowed when new resource group is added
	c.Check(isOverwriteAllowed(depDir, &supersetConfig, true /* overwriteFlag */), Equals, true)
	// overwrite fails when resource group is deleted
	c.Check(isOverwriteAllowed(depDir, &swapConfig, true /* overwriteFlag */), Equals, false)
	// overwrite fails when overwrite is false
	c.Check(isOverwriteAllowed(depDir, &supersetConfig, false /* overwriteFlag */), Equals, false)
}

// modulewriter.go
func (s *MySuite) TestWriteDeployment(c *C) {
	aferoFS := afero.NewMemMapFs()
	aferoFS.MkdirAll("modules/red/pink", 0755)
	afero.WriteFile(aferoFS, "modules/red/pink/main.tf", []byte("pink"), 0644)
	aferoFS.MkdirAll("community/modules/green/lime", 0755)
	afero.WriteFile(aferoFS, "community/modules/green/lime/main.tf", []byte("lime"), 0644)
	sourcereader.ModuleFS = afero.NewIOFS(aferoFS)

	testBlueprint := getBlueprintForTest()
	testBlueprint.Vars = map[string]interface{}{"deployment_name": "test_write_deployment"}
	err := WriteDeployment(&testBlueprint, testDir, false /* overwriteFlag */)
	c.Check(err, IsNil)
	// Overwriting the deployment fails
	err = WriteDeployment(&testBlueprint, testDir, false /* overwriteFlag */)
	c.Check(err, NotNil)
	// Overwriting the deployment succeeds with flag
	err = WriteDeployment(&testBlueprint, testDir, true /* overwriteFlag */)
	c.Check(err, IsNil)
}

func (s *MySuite) TestCreateGroupDirs(c *C) {
	// Setup
	testDeployDir := filepath.Join(testDir, "test_createGroupDirs")
	if err := os.Mkdir(testDeployDir, 0755); err != nil {
		log.Fatal("Failed to create test deployment directory for createGroupDirs")
	}
	groupNames := []string{"group0", "group1", "group2"}

	// No deployment groups
	testDepGroups := []config.DeploymentGroup{}
	err := createGroupDirs(testDeployDir, &testDepGroups)
	c.Check(err, IsNil)

	// Single deployment group
	testDepGroups = []config.DeploymentGroup{{Name: groupNames[0]}}
	err = createGroupDirs(testDeployDir, &testDepGroups)
	c.Check(err, IsNil)
	grp0Path := filepath.Join(testDeployDir, groupNames[0])
	_, err = os.Stat(grp0Path)
	c.Check(errors.Is(err, os.ErrNotExist), Equals, false)
	c.Check(err, IsNil)
	err = os.Remove(grp0Path)
	c.Check(err, IsNil)

	// Multiple deployment groups
	testDepGroups = []config.DeploymentGroup{
		{Name: groupNames[0]},
		{Name: groupNames[1]},
		{Name: groupNames[2]},
	}
	err = createGroupDirs(testDeployDir, &testDepGroups)
	c.Check(err, IsNil)
	// Check for group 0
	_, err = os.Stat(grp0Path)
	c.Check(errors.Is(err, os.ErrNotExist), Equals, false)
	c.Check(err, IsNil)
	err = os.Remove(grp0Path)
	c.Check(err, IsNil)
	// Check for group 1
	grp1Path := filepath.Join(testDeployDir, groupNames[1])
	_, err = os.Stat(grp1Path)
	c.Check(errors.Is(err, os.ErrNotExist), Equals, false)
	c.Check(err, IsNil)
	err = os.Remove(grp1Path)
	c.Check(err, IsNil)
	// Check for group 2
	grp2Path := filepath.Join(testDeployDir, groupNames[2])
	_, err = os.Stat(grp2Path)
	c.Check(errors.Is(err, os.ErrNotExist), Equals, false)
	c.Check(err, IsNil)
	err = os.Remove(grp2Path)
	c.Check(err, IsNil)

	// deployment group(s) already exists
	err = createGroupDirs(testDeployDir, &testDepGroups)
	c.Check(err, IsNil)
}

func (s *MySuite) TestWriteDeployment_BadDeploymentName(c *C) {
	testBlueprint := getBlueprintForTest()
	var e *config.InputValueError

	testBlueprint.Vars = map[string]interface{}{"deployment_name": 100}
	err := WriteDeployment(&testBlueprint, testDir, false /* overwriteFlag */)
	c.Check(errors.As(err, &e), Equals, true)

	testBlueprint.Vars = map[string]interface{}{"deployment_name": false}
	err = WriteDeployment(&testBlueprint, testDir, false /* overwriteFlag */)
	c.Check(errors.As(err, &e), Equals, true)

	testBlueprint.Vars = map[string]interface{}{"deployment_name": ""}
	err = WriteDeployment(&testBlueprint, testDir, false /* overwriteFlag */)
	c.Check(errors.As(err, &e), Equals, true)

	testBlueprint.Vars = map[string]interface{}{}
	err = WriteDeployment(&testBlueprint, testDir, false /* overwriteFlag */)
	c.Check(errors.As(err, &e), Equals, true)
}

// tfwriter.go
func (s *MySuite) TestRestoreTfState(c *C) {
	// set up dir structure
	//
	// └── test_restore_state
	//    ├── .ghpc
	//       └── previous_resource_groups
	//          └── fake_resource_group
	//             └── terraform.tfstate
	//    └── fake_resource_group
	depDir := filepath.Join(testDir, "test_restore_state")
	deploymentGroupName := "fake_resource_group"

	prevDeploymentGroup := filepath.Join(
		depDir, hiddenGhpcDirName, prevDeploymentGroupDirName, deploymentGroupName)
	curDeploymentGroup := filepath.Join(depDir, deploymentGroupName)
	prevStateFile := filepath.Join(prevDeploymentGroup, tfStateFileName)
	prevBuStateFile := filepath.Join(prevDeploymentGroup, tfStateBackupFileName)
	os.MkdirAll(prevDeploymentGroup, 0755)
	os.MkdirAll(curDeploymentGroup, 0755)
	emptyFile, _ := os.Create(prevStateFile)
	emptyFile.Close()
	emptyFile, _ = os.Create(prevBuStateFile)
	emptyFile.Close()

	testWriter := TFWriter{}
	testWriter.restoreState(depDir)

	// check state file was moved to current resource group dir
	curStateFile := filepath.Join(curDeploymentGroup, tfStateFileName)
	curBuStateFile := filepath.Join(curDeploymentGroup, tfStateBackupFileName)
	_, err := os.Stat(curStateFile)
	c.Check(err, IsNil)
	_, err = os.Stat(curBuStateFile)
	c.Check(err, IsNil)
}

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
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))

	val := cty.ObjectVal(map[string]cty.Value{"Lorum": cty.StringVal("Ipsum")})
	tok = getTypeTokens(val)
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))

	// Success Map
	val = cty.MapVal(map[string]cty.Value{"Lorum": cty.StringVal("Ipsum")})
	tok = getTypeTokens(val)
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))

	// Success any
	tok = getTypeTokens(cty.NullVal(cty.DynamicPseudoType))
	c.Assert(len(tok), Equals, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))

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
	b, _ := ioutil.ReadFile(goodPath)
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

func (s *MySuite) TestWriteMain(c *C) {
	// Setup
	testMainDir := filepath.Join(testDir, "TestWriteMain")
	mainFilePath := filepath.Join(testMainDir, "main.tf")
	if err := os.Mkdir(testMainDir, 0755); err != nil {
		log.Fatal("Failed to create test dir for creating main.tf file")
	}

	// Simple success
	testModules := []config.Module{}
	testBackend := config.TerraformBackend{}
	err := writeMain(testModules, testBackend, testMainDir)
	c.Assert(err, IsNil)

	// Test with modules
	testModule := config.Module{
		ID: "test_module",
		Settings: map[string]interface{}{
			"testSetting": "testValue",
		},
	}
	testModules = append(testModules, testModule)
	err = writeMain(testModules, testBackend, testMainDir)
	c.Assert(err, IsNil)
	exists, err := stringExistsInFile("testSetting", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Test with Backend
	testBackend.Type = "gcs"
	testBackend.Configuration.Set("bucket", cty.StringVal("a_bucket"))

	err = writeMain(testModules, testBackend, testMainDir)
	c.Assert(err, IsNil)
	exists, err = stringExistsInFile("a_bucket", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Test with WrapSettingsWith
	testModuleWithWrap := config.Module{
		ID: "test_module_with_wrap",
		WrapSettingsWith: map[string][]string{
			"wrappedSetting": {"list(flatten(", "))"},
		},
		Settings: map[string]interface{}{
			"wrappedSetting": []interface{}{"val1", "val2"},
		},
	}
	testModules = append(testModules, testModuleWithWrap)
	err = writeMain(testModules, testBackend, testMainDir)
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

	// Simple success, no modules
	testModules := []config.Module{}
	err := writeOutputs(testModules, testOutputsDir)
	c.Assert(err, IsNil)

	// Failure: Bad path
	err = writeOutputs(testModules, "not/a/real/path")
	c.Assert(err, ErrorMatches, "error creating outputs.tf file: .*")

	// Success: Outputs added
	outputList := []string{"output1", "output2"}
	moduleWithOutputs := config.Module{Outputs: outputList, ID: "testMod"}
	testModules = []config.Module{moduleWithOutputs}
	err = writeOutputs(testModules, testOutputsDir)
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

func (s *MySuite) TestHandleLiteralVariables(c *C) {
	// Setup
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// Set literal var value
	hclBody.SetAttributeValue("dummyAttributeName1", cty.StringVal("((var.literal))"))
	hclBody.AppendNewline()
	hclBytes := handleLiteralVariables(hclFile.Bytes())
	hclString := string(hclBytes)

	// Sucess
	exists := strings.Contains(hclString, "dummyAttributeName1 = var.literal")
	c.Assert(exists, Equals, true)
}

// packerwriter.go
func (s *MySuite) TestNumModules_PackerWriter(c *C) {
	testWriter := PackerWriter{}
	c.Assert(testWriter.getNumModules(), Equals, 0)
	testWriter.addNumModules(-1)
	c.Assert(testWriter.getNumModules(), Equals, -1)
	testWriter.addNumModules(2)
	c.Assert(testWriter.getNumModules(), Equals, 1)
	testWriter.addNumModules(0)
	c.Assert(testWriter.getNumModules(), Equals, 1)
}

func (s *MySuite) TestWriteDeploymentGroup_PackerWriter(c *C) {
	deploymentio := deploymentio.GetDeploymentioLocal()
	testWriter := PackerWriter{}

	// No Packer modules
	deploymentName := "deployment_TestWriteModuleLevel_PackerWriter"
	testVars := map[string]interface{}{"deployment_name": deploymentName}
	deploymentDir := filepath.Join(testDir, deploymentName)
	if err := deploymentio.CreateDirectory(deploymentDir); err != nil {
		log.Fatal(err)
	}
	groupDir := filepath.Join(deploymentDir, "packerGroup")
	if err := deploymentio.CreateDirectory(groupDir); err != nil {
		log.Fatal(err)
	}
	moduleDir := filepath.Join(groupDir, "testPackerModule")
	if err := deploymentio.CreateDirectory(moduleDir); err != nil {
		log.Fatal(err)
	}

	testPackerModule := config.Module{
		Kind:             "packer",
		ID:               "testPackerModule",
		DeploymentSource: "testPackerModule",
	}
	testDeploymentGroup := config.DeploymentGroup{
		Name:    "packerGroup",
		Modules: []config.Module{testPackerModule},
	}
	testWriter.writeDeploymentGroup(testDeploymentGroup, testVars, deploymentDir)
	_, err := os.Stat(filepath.Join(moduleDir, packerAutoVarFilename))
	c.Assert(err, IsNil)
}

func (s *MySuite) TestWritePackerAutoVars(c *C) {
	testBlueprint := getBlueprintForTest()
	testBlueprint.Vars["testkey"] = "testval"
	ctyVars, _ := config.ConvertMapToCty(testBlueprint.Vars)

	// fail writing to a bad path
	badDestPath := "not/a/real/path"
	err := writePackerAutovars(ctyVars, badDestPath)
	expErr := fmt.Sprintf("error creating variables file %s:.*", packerAutoVarFilename)
	c.Assert(err, ErrorMatches, expErr)

	testPackerTemplateDir := filepath.Join(testDir, "TestWritePackerTemplate")
	if err := os.Mkdir(testPackerTemplateDir, 0755); err != nil {
		log.Fatalf("Failed to create test dir for creating %s file", packerAutoVarFilename)
	}
	err = writePackerAutovars(ctyVars, testPackerTemplateDir)
	c.Assert(err, IsNil)

}

// hcl_utils.go
func (s *MySuite) TestescapeLiteralVariables(c *C) {
	// Setup
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// Set escaped var value
	hclBody.SetAttributeValue("dummyAttributeName1", cty.StringVal("\\((not.var))"))
	hclBody.SetAttributeValue("dummyAttributeName2", cty.StringVal("abc\\((not.var))abc"))
	hclBody.SetAttributeValue("dummyAttributeName3", cty.StringVal("abc \\((not.var)) abc"))
	hclBody.SetAttributeValue("dummyAttributeName4", cty.StringVal("abc \\((not.var1)) abc \\((not.var2)) abc"))
	hclBody.SetAttributeValue("dummyAttributeName5", cty.StringVal("abc \\\\((escape.backslash))"))
	hclBody.AppendNewline()
	hclBytes := escapeLiteralVariables(hclFile.Bytes())
	hclString := string(hclBytes)

	// Sucess
	exists := strings.Contains(hclString, "dummyAttributeName1 = \"((not.var))\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName2 = \"abc((not.var))abc\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName3 = \"abc ((not.var)) abc\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName4 = \"abc ((not.var1)) abc ((not.var2)) abc\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName5 = \"abc \\\\((escape.backslash))\"")
	c.Assert(exists, Equals, true)
}

func (s *MySuite) TestescapeBlueprintVariables(c *C) {
	// Setup
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// Set escaped var value
	hclBody.SetAttributeValue("dummyAttributeName1", cty.StringVal("\\$(not.var)"))
	hclBody.SetAttributeValue("dummyAttributeName2", cty.StringVal("abc\\$(not.var)abc"))
	hclBody.SetAttributeValue("dummyAttributeName3", cty.StringVal("abc \\$(not.var) abc"))
	hclBody.SetAttributeValue("dummyAttributeName4", cty.StringVal("abc \\$(not.var1) abc \\$(not.var2) abc"))
	hclBody.SetAttributeValue("dummyAttributeName5", cty.StringVal("abc \\\\$(escape.backslash)"))
	hclBody.AppendNewline()
	hclBytes := escapeBlueprintVariables(hclFile.Bytes())
	hclString := string(hclBytes)

	// Sucess
	exists := strings.Contains(hclString, "dummyAttributeName1 = \"$(not.var)\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName2 = \"abc$(not.var)abc\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName3 = \"abc $(not.var) abc\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName4 = \"abc $(not.var1) abc $(not.var2) abc\"")
	c.Assert(exists, Equals, true)
	exists = strings.Contains(hclString, "dummyAttributeName5 = \"abc \\\\$(escape.backslash)\"")
	c.Assert(exists, Equals, true)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func (s *MySuite) TestDeploymentSource(c *C) {
	{ // git
		m := config.Module{Kind: "terraform", Source: "github.com/x/y.git"}
		s, err := deploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "github.com/x/y.git")
	}
	{ // packer
		m := config.Module{Kind: "packer", Source: "modules/packer/custom-image", ID: "custom-image"}
		s, err := deploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "custom-image")
	}
	{ // embedded core
		m := config.Module{Kind: "terraform", Source: "modules/x/y"}
		s, err := deploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "./modules/embedded/modules/x/y")
	}
	{ // embedded community
		m := config.Module{Kind: "terraform", Source: "community/modules/x/y"}
		s, err := deploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "./modules/embedded/community/modules/x/y")
	}
	{ // local rel in repo
		m := config.Module{Kind: "terraform", Source: "./modules/x/y"}
		s, err := deploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Matches, `^\./modules/y-\w\w\w\w$`)
	}
	{ // local rel
		m := config.Module{Kind: "terraform", Source: "./../../../../x/y"}
		s, err := deploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Matches, `^\./modules/y-\w\w\w\w$`)
	}
	{ // local abs
		m := config.Module{Kind: "terraform", Source: "/tmp/x/y"}
		s, err := deploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Matches, `^\./modules/y-\w\w\w\w$`)
	}
}
