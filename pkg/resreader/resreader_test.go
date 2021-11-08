// Copyright 2021 Google LLC
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

package resreader

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/spf13/afero"
	. "gopkg.in/check.v1"
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
	tmpResourceDir string
	terraformDir   string
	packerDir      string
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

// resreader.go
func (s *MySuite) TestIsValidKind(c *C) {
	c.Assert(IsValidKind(pkrKindString), Equals, true)
	c.Assert(IsValidKind(tfKindString), Equals, true)
	c.Assert(IsValidKind("Packer"), Equals, false)
	c.Assert(IsValidKind("Terraform"), Equals, false)
	c.Assert(IsValidKind("META"), Equals, false)
	c.Assert(IsValidKind(""), Equals, false)
}

func (s *MySuite) TestFactory(c *C) {
	pkrReader := Factory(pkrKindString)
	c.Assert(reflect.TypeOf(pkrReader), Equals, reflect.TypeOf(PackerReader{}))
	tfReader := Factory(tfKindString)
	c.Assert(reflect.TypeOf(tfReader), Equals, reflect.TypeOf(TFReader{}))
}

// hcl_utils.go
func getTestFS() afero.IOFS {
	aferoFS := afero.NewMemMapFs()
	aferoFS.MkdirAll("resources/network/vpc", 0755)
	afero.WriteFile(
		aferoFS, "resources/network/vpc/main.tf", []byte(testMainTf), 0644)
	return afero.NewIOFS(aferoFS)
}

func (s *MySuite) TestCopyDirFromResources(c *C) {
	// Setup
	testResFS := getTestFS()
	testDir := path.Join(tmpResourceDir, "TestCopyDirFromResources")
	if err := os.Mkdir(testDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Success
	err := copyDirFromResources(testResFS, "resources/network/vpc", testDir)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(path.Join(testDir, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Invalid path
	err = copyDirFromResources(testResFS, "not/valid", testDir)
	c.Assert(err, ErrorMatches, "*file does not exist")

}

func (s *MySuite) TestCopyFSToTempDir(c *C) {
	// Setup
	testResFS := getTestFS()

	// Success
	testDir, err := copyFSToTempDir(testResFS, "resources/")
	defer os.RemoveAll(testDir)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(path.Join(testDir, "network/vpc/main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
	fInfo, err = os.Stat(path.Join(testDir, "network/vpc"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "vpc")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, true)
}

func (s *MySuite) TestGetHCLInfo(c *C) {
	// Invalid source path - path does not exists
	fakePath := "./not/a/real/path"
	_, err := getHCLInfo(fakePath)
	expectedErr := "Source to resource does not exist: .*"
	c.Assert(err, ErrorMatches, expectedErr)
	// Invalid source path - points to a file
	pathToFile := path.Join(terraformDir, "main.tf")
	_, err = getHCLInfo(pathToFile)
	expectedErr = "Source of resource must be a directory: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid source path - points to directory with no .tf files
	pathToEmptyDir := path.Join(packerDir, "emptyDir")
	err = os.Mkdir(pathToEmptyDir, 0755)
	if err != nil {
		log.Fatal("TestGetHCLInfo: Failed to create test directory.")
	}
	_, err = getHCLInfo(pathToEmptyDir)
	expectedErr = "Source is not a terraform or packer module: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: No embedded resource
	badEmbeddedRes := "resources/does/not/exist"
	_, err = getHCLInfo(badEmbeddedRes)
	expectedErr = "failed to copy embedded resource at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source
	badSource := "github.com/GoogleCloudPlatform/hpc-toolkit/resources"
	_, err = getHCLInfo(badSource)
	expectedErr = "invalid source .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

// tfreader.go
func (s *MySuite) TestGetInfo_TFWriter(c *C) {
	reader := TFReader{allResInfo: make(map[string]ResourceInfo)}
	resourceInfo, err := reader.GetInfo(terraformDir)
	c.Assert(err, IsNil)
	c.Assert(resourceInfo.Inputs[0].Name, Equals, "test_variable")
	c.Assert(resourceInfo.Outputs[0].Name, Equals, "test_output")
}

// packerreader.go
func (s *MySuite) TestGetInfo_PackerReader(c *C) {
	// Didn't already exist, succeeds
	reader := PackerReader{allResInfo: make(map[string]ResourceInfo)}
	resourceInfo, err := reader.GetInfo(packerDir)
	c.Assert(err, IsNil)
	c.Assert(resourceInfo.Inputs[0].Name, Equals, "test_variable")

	// Already exists, succeeds
	existingResourceInfo, err := reader.GetInfo(packerDir)
	c.Assert(err, IsNil)
	c.Assert(
		existingResourceInfo.Inputs[0].Name, Equals, resourceInfo.Inputs[0].Name)
}

// metareader.go
func (s *MySuite) TestGetInfo_MetaReader(c *C) {
	// Not implemented, expect that error
	reader := MetaReader{}
	_, err := reader.GetInfo("")
	expErr := "Meta GetInfo not implemented: .*"
	c.Assert(err, ErrorMatches, expErr)
}

// Util Functions
func createTmpResource() {
	var err error
	tmpResourceDir, err = ioutil.TempDir("", "resreader_tests_*")
	if err != nil {
		log.Fatalf(
			"Failed to create temp dir for resource in resreader_test, %v", err)
	}

	// Create terraform resource dir
	terraformDir = path.Join(tmpResourceDir, "terraformResource")
	err = os.Mkdir(terraformDir, 0755)
	if err != nil {
		log.Fatalf("error creating test terraform resource dir: %e", err)
	}

	// main.tf file
	mainFile, err := os.Create(path.Join(terraformDir, "main.tf"))
	if err != nil {
		log.Fatalf("Failed to create main.tf: %v", err)
	}
	_, err = mainFile.WriteString(testMainTf)
	if err != nil {
		log.Fatalf("resreader_test: Failed to write main.tf test file. %v", err)
	}

	// variables.tf file
	varFile, err := os.Create(path.Join(terraformDir, "variables.tf"))
	if err != nil {
		log.Fatalf("Failed to create variables.tf: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"resreader_test: Failed to write variables.tf test file. %v", err)
	}

	// outputs.tf file
	outFile, err := os.Create(path.Join(terraformDir, "outputs.tf"))
	if err != nil {
		log.Fatalf("Failed to create outputs.tf: %v", err)
	}
	_, err = outFile.WriteString(testOutputsTf)
	if err != nil {
		log.Fatalf("resreader_test: Failed to write outputs.tf test file. %v", err)
	}

	// Create packer resource dir
	packerDir = path.Join(tmpResourceDir, "packerResource")
	err = os.Mkdir(packerDir, 0755)
	if err != nil {
		log.Fatalf("error creating test packer resource dir: %e", err)
	}

	// main.pkr.hcl file
	mainFile, err = os.Create(path.Join(packerDir, "main.pkr.hcl"))
	if err != nil {
		log.Fatalf("Failed to create main.pkr.hcl: %v", err)
	}
	_, err = mainFile.WriteString(testMainTf)
	if err != nil {
		log.Fatalf("resreader_test: Failed to write main.pkr.hcl test file. %v", err)
	}

	// variables.pkr.hcl file
	varFile, err = os.Create(path.Join(packerDir, "variables.pkr.hcl"))
	if err != nil {
		log.Fatalf("Failed to create variables.pkr.hcl: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"resreader_test: Failed to write variables.pkr.hcl test file. %v", err)
	}
}

func teardownTmpResource() {
	err := os.RemoveAll(tmpResourceDir)
	if err != nil {
		log.Fatalf(
			"resreader_test: Failed to delete contents of test directory %s, %v",
			tmpResourceDir, err)
	}
}

func TestMain(m *testing.M) {
	createTmpResource()
	code := m.Run()
	teardownTmpResource()
	os.Exit(code)
}
