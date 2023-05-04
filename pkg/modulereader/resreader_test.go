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

package modulereader

import (
	"embed"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/afero"
	. "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"
)

const (
	pkrKindString = "packer"
	tfKindString  = "terraform"
	testMainTf    = `
module "test_module" {
	source = "testSource"
}
data "test_data" "test_data_name" {
	name = "test_data_name"
}
`
	testVariablesTf = `
variable "test_variable" {
	description = "This is just a test"
	type        = string
}
`
	testOutputsTf = `
output "test_output" {
	description = "This is just a test"
	value       = "test_value"
}
`
)

var (
	tmpModuleDir string
	terraformDir string
	packerDir    string
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

//go:embed modules
var testModuleFS embed.FS

func Test(t *testing.T) {
	TestingT(t)
}

// modulereader.go
func (s *MySuite) TestIsValidKind(c *C) {
	c.Assert(IsValidReaderKind(pkrKindString), Equals, true)
	c.Assert(IsValidReaderKind(tfKindString), Equals, true)
	c.Assert(IsValidReaderKind("Packer"), Equals, false)
	c.Assert(IsValidReaderKind("Terraform"), Equals, false)
	c.Assert(IsValidReaderKind("META"), Equals, false)
	c.Assert(IsValidReaderKind(""), Equals, false)
}

func (s *MySuite) TestGetOutputsAsMap(c *C) {
	// Simple: empty outputs
	modInfo := ModuleInfo{}
	outputMap := modInfo.GetOutputsAsMap()
	c.Assert(len(outputMap), Equals, 0)

	testDescription := "This is a test description"
	testName := "testName"
	outputInfo := OutputInfo{Name: testName, Description: testDescription}
	modInfo.Outputs = []OutputInfo{outputInfo}
	outputMap = modInfo.GetOutputsAsMap()
	c.Assert(len(outputMap), Equals, 1)
	c.Assert(outputMap[testName].Description, Equals, testDescription)
}

func (s *MySuite) TestFactory(c *C) {
	pkrReader := Factory(pkrKindString)
	c.Assert(reflect.TypeOf(pkrReader), Equals, reflect.TypeOf(PackerReader{}))
	tfReader := Factory(tfKindString)
	c.Assert(reflect.TypeOf(tfReader), Equals, reflect.TypeOf(TFReader{}))
}

func (s *MySuite) TestGetModuleInfo_Embedded(c *C) {
	ModuleFS = testModuleFS

	// Success
	moduleInfo, err := GetModuleInfo("modules/test_role/test_module", tfKindString)
	c.Assert(err, IsNil)
	c.Assert(moduleInfo.Inputs[0].Name, Equals, "test_variable")
	c.Assert(moduleInfo.Outputs[0].Name, Equals, "test_output")

	// Invalid: No embedded modules
	badEmbeddedMod := "modules/does/not/exist"
	moduleInfo, err = GetModuleInfo(badEmbeddedMod, tfKindString)
	expectedErr := "failed to get info using tfconfig for terraform module at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	moduleInfo, err = GetModuleInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetModuleInfo_Git(c *C) {

	// Invalid git repository - path does not exists
	badGitRepo := "github.com:not/exist.git"
	_, err := GetModuleInfo(badGitRepo, tfKindString)
	expectedErr := "failed to clone git module at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	_, err = GetModuleInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetModuleInfo_Local(c *C) {

	// Success
	moduleInfo, err := GetModuleInfo(terraformDir, tfKindString)
	c.Assert(err, IsNil)
	c.Assert(moduleInfo.Inputs[0].Name, Equals, "test_variable")
	c.Assert(moduleInfo.Outputs[0].Name, Equals, "test_output")

	// Invalid source path - path does not exists
	badLocalMod := "./not/a/real/path"
	moduleInfo, err = GetModuleInfo(badLocalMod, tfKindString)
	expectedErr := "failed to get info using tfconfig for terraform module at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	moduleInfo, err = GetModuleInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

// hcl_utils.go
func getTestFS() afero.IOFS {
	aferoFS := afero.NewMemMapFs()
	aferoFS.MkdirAll("modules/network/vpc", 0755)
	afero.WriteFile(
		aferoFS, "modules/network/vpc/main.tf", []byte(testMainTf), 0644)
	return afero.NewIOFS(aferoFS)
}

func (s *MySuite) TestGetHCLInfo(c *C) {
	// Invalid source path - path does not exists
	fakePath := "./not/a/real/path"
	_, err := getHCLInfo(fakePath)
	expectedErr := "Source to module does not exist: .*"
	c.Assert(err, ErrorMatches, expectedErr)
	// Invalid source path - points to a file
	pathToFile := filepath.Join(terraformDir, "main.tf")
	_, err = getHCLInfo(pathToFile)
	expectedErr = "Source of module must be a directory: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid source path - points to directory with no .tf files
	pathToEmptyDir := filepath.Join(packerDir, "emptyDir")
	err = os.Mkdir(pathToEmptyDir, 0755)
	if err != nil {
		log.Fatal("TestGetHCLInfo: Failed to create test directory.")
	}
	_, err = getHCLInfo(pathToEmptyDir)
	expectedErr = "Source is not a terraform or packer module: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

// tfreader.go
func (s *MySuite) TestGetInfo_TFReder(c *C) {
	reader := NewTFReader()
	info, err := reader.GetInfo(terraformDir)
	c.Assert(err, IsNil)
	c.Check(info, DeepEquals, ModuleInfo{
		Inputs:  []VarInfo{{Name: "test_variable", Type: "string", Description: "This is just a test", Required: true}},
		Outputs: []OutputInfo{{Name: "test_output", Description: "This is just a test"}},
	})

}

// packerreader.go
func (s *MySuite) TestGetInfo_PackerReader(c *C) {
	// Didn't already exist, succeeds
	reader := NewPackerReader()
	info, err := reader.GetInfo(packerDir)
	c.Assert(err, IsNil)
	c.Check(info, DeepEquals, ModuleInfo{
		Inputs: []VarInfo{{Name: "test_variable", Type: "string", Description: "This is just a test", Required: true}}})

	// Already exists, succeeds
	infoAgain, err := reader.GetInfo(packerDir)
	c.Assert(err, IsNil)
	c.Check(infoAgain, DeepEquals, info)
}

// metareader.go
func (s *MySuite) TestGetInfo_MetaReader(c *C) {
	// Not implemented, expect that error
	reader := MetaReader{}
	_, err := reader.GetInfo("")
	expErr := "Meta GetInfo not implemented: .*"
	c.Assert(err, ErrorMatches, expErr)
}

// module outputs can be specified as a simple string for the output name or as
// a YAML mapping of name/description/sensitive (str,str,bool)
func (s *MySuite) TestUnmarshalOutputInfo(c *C) {
	var oinfo OutputInfo
	var y string

	y = "foo"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "", Sensitive: false})

	y = "{ name: foo }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "", Sensitive: false})

	y = "{ name: foo, description: bar }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "bar", Sensitive: false})

	y = "{ name: foo, description: bar, sensitive: true }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "bar", Sensitive: true})

	// extra key should generate error
	y = "{ name: foo, description: bar, sensitive: true, extrakey: extraval }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)

	// missing required key name should generate error
	y = "{ description: bar, sensitive: true }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)

	// should not ummarshal a sequence
	y = "[ foo ]"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)

	// should not ummarshal an object with non-boolean sensitive type
	y = "{ name: foo, description: bar, sensitive: contingent }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)
}

// Util Functions
func createTmpModule() {
	var err error
	tmpModuleDir, err = ioutil.TempDir("", "modulereader_tests_*")
	if err != nil {
		log.Fatalf(
			"Failed to create temp dir for module in modulereader_test, %v", err)
	}

	// Create terraform module dir
	terraformDir = filepath.Join(tmpModuleDir, "terraformModule")
	err = os.Mkdir(terraformDir, 0755)
	if err != nil {
		log.Fatalf("error creating test terraform module dir: %e", err)
	}

	// main.tf file
	mainFile, err := os.Create(filepath.Join(terraformDir, "main.tf"))
	if err != nil {
		log.Fatalf("Failed to create main.tf: %v", err)
	}
	_, err = mainFile.WriteString(testMainTf)
	if err != nil {
		log.Fatalf("modulereader_test: Failed to write main.tf test file. %v", err)
	}

	// variables.tf file
	varFile, err := os.Create(filepath.Join(terraformDir, "variables.tf"))
	if err != nil {
		log.Fatalf("Failed to create variables.tf: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"modulereader_test: Failed to write variables.tf test file. %v", err)
	}

	// outputs.tf file
	outFile, err := os.Create(filepath.Join(terraformDir, "outputs.tf"))
	if err != nil {
		log.Fatalf("Failed to create outputs.tf: %v", err)
	}
	_, err = outFile.WriteString(testOutputsTf)
	if err != nil {
		log.Fatalf("modulereader_test: Failed to write outputs.tf test file. %v", err)
	}

	// Create packer module dir
	packerDir = filepath.Join(tmpModuleDir, "packerModule")
	err = os.Mkdir(packerDir, 0755)
	if err != nil {
		log.Fatalf("error creating test packer module dir: %e", err)
	}

	// main.pkr.hcl file
	mainFile, err = os.Create(filepath.Join(packerDir, "main.pkr.hcl"))
	if err != nil {
		log.Fatalf("Failed to create main.pkr.hcl: %v", err)
	}
	_, err = mainFile.WriteString(testMainTf)
	if err != nil {
		log.Fatalf("modulereader_test: Failed to write main.pkr.hcl test file. %v", err)
	}

	// variables.pkr.hcl file
	varFile, err = os.Create(filepath.Join(packerDir, "variables.pkr.hcl"))
	if err != nil {
		log.Fatalf("Failed to create variables.pkr.hcl: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"modulereader_test: Failed to write variables.pkr.hcl test file. %v", err)
	}
}

func teardownTmpModule() {
	err := os.RemoveAll(tmpModuleDir)
	if err != nil {
		log.Fatalf(
			"modulereader_test: Failed to delete contents of test directory %s, %v",
			tmpModuleDir, err)
	}
}

func TestMain(m *testing.M) {
	createTmpModule()
	code := m.Run()
	teardownTmpModule()
	os.Exit(code)
}
