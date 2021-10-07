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
	. "gopkg.in/check.v1"
	"io/ioutil"
	"log"
	"os"
	"path"
	"reflect"
	"testing"
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

var tmpResourceDir string

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

//hcl_utils.go
func (s *MySuite) TestGetHCLInfo(c *C) {
	// Invalid source path - path does not exists
	fakePath := "./not/a/real/path"
	_, err := getHCLInfo(fakePath)
	expectedErr := "Source to resource does not exist: .*"
	c.Assert(err, ErrorMatches, expectedErr)
	// Invalid source path - points to a file
	pathToFile := path.Join(tmpResourceDir, "main.tf")
	_, err = getHCLInfo(pathToFile)
	expectedErr = "Source of resource must be a directory: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid source path - points to directory with no .tf files
	pathToEmptyDir := path.Join(tmpResourceDir, "emptyDir")
	err = os.Mkdir(pathToEmptyDir, 0755)
	if err != nil {
		log.Fatal("TestGetHCLInfo: Failed to create test directory.")
	}
	_, err = getHCLInfo(pathToEmptyDir)
	expectedErr = "Source is not a terraform or packer module: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

// tfreader.go
// util functions
func createTmpResource() {
	var err error
	tmpResourceDir, err = ioutil.TempDir("", "resreader_tests_*")
	if err != nil {
		log.Fatalf(
			"Failed to create temp dir for resource in resreader_test, %v", err)
	}
	mainFile, err := os.Create(path.Join(tmpResourceDir, "main.tf"))
	_, err = mainFile.WriteString(testMainTf)
	if err != nil {
		log.Fatalf("resreader_test: Failed to write main.tf test file. %v", err)
	}

	varFile, err := os.Create(path.Join(tmpResourceDir, "variables.tf"))
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"resreader_test: Failed to write variables.tf test file. %v", err)
	}

	outFile, err := os.Create(path.Join(tmpResourceDir, "outputs.tf"))
	_, err = outFile.WriteString(testOutputsTf)
	if err != nil {
		log.Fatalf("resreader_test: Failed to write outputs.tf test file. %v", err)
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

func (s *MySuite) TestTFGetInfo(c *C) {
	reader := TFReader{allResInfo: make(map[string]ResourceInfo)}
	resourceInfo := reader.GetInfo(tmpResourceDir)
	c.Assert(resourceInfo.Inputs[0].Name, Equals, "test_variable")
	c.Assert(resourceInfo.Outputs[0].Name, Equals, "test_output")
}

func TestMain(m *testing.M) {
	createTmpResource()
	code := m.Run()
	teardownTmpResource()
	os.Exit(code)
}
