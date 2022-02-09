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

package sourcereader

import (
	"log"
	"os"
	"path"

	"github.com/spf13/afero"
	. "gopkg.in/check.v1"
)

const (
	testMainTf = `
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

func getTestFS() afero.IOFS {
	aferoFS := afero.NewMemMapFs()
	aferoFS.MkdirAll("resources/network/vpc", 0755)
	afero.WriteFile(
		aferoFS, "resources/network/vpc/main.tf", []byte(testMainTf), 0644)
	afero.WriteFile(
		aferoFS, "resources/network/vpc/variables.tf", []byte(testVariablesTf), 0644)
	afero.WriteFile(
		aferoFS, "resources/network/vpc/output.tf", []byte(testOutputsTf), 0644)
	return afero.NewIOFS(aferoFS)
}

func (s *MySuite) TestCopyDirFromResources(c *C) {
	// Setup
	testResFS := getTestFS()
	copyDir := path.Join(testDir, "TestCopyDirFromResources")
	if err := os.Mkdir(copyDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Success
	err := copyDirFromResources(testResFS, "resources/network/vpc", copyDir)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(path.Join(copyDir, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Success: copy files AND dirs
	err = copyDirFromResources(testResFS, "resources/network/", copyDir)
	c.Assert(err, IsNil)
	fInfo, err = os.Stat(path.Join(copyDir, "vpc/main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
	fInfo, err = os.Stat(path.Join(copyDir, "vpc"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "vpc")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, true)

	// Invalid path
	err = copyDirFromResources(testResFS, "not/valid", copyDir)
	c.Assert(err, ErrorMatches, "*file does not exist")

	// Failure: File Already Exists
	err = copyDirFromResources(testResFS, "resources/network/", copyDir)
	c.Assert(err, ErrorMatches, "*file exists")
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

func (s *MySuite) TestGetResourceInfo_Embedded(c *C) {
	ResourceFS = getTestFS()
	reader := EmbeddedSourceReader{}

	// Success
	resourceInfo, err := reader.GetResourceInfo("resources/network/vpc", tfKindString)
	c.Assert(err, IsNil)
	c.Assert(resourceInfo.Inputs[0].Name, Equals, "test_variable")
	c.Assert(resourceInfo.Outputs[0].Name, Equals, "test_output")

	// Invalid: No embedded resource
	badEmbeddedRes := "resources/does/not/exist"
	resourceInfo, err = reader.GetResourceInfo(badEmbeddedRes, tfKindString)
	expectedErr := "failed to copy embedded resource at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/resources"
	resourceInfo, err = reader.GetResourceInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetResource_Embedded(c *C) {
	ResourceFS = getTestFS()
	reader := EmbeddedSourceReader{}

	// Success
	dest := path.Join(testDir, "TestGetResource_Embedded")
	err := reader.GetResource("resources/network", dest)
	c.Assert(err, IsNil)

	// Invalid: Write to the same dest directory again
	err = reader.GetResource("resources/network", dest)
	expectedErr := "The directory already exists: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Success
	fInfo, err := os.Stat(path.Join(dest, "vpc/main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
	fInfo, err = os.Stat(path.Join(dest, "vpc"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "vpc")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, true)

	// Invalid: No embedded resource
	badEmbeddedRes := "resources/does/not/exist"
	err = reader.GetResource(badEmbeddedRes, dest)
	expectedErr = "failed to copy embedded resource at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source by EmbeddedSourceReader
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/resources"
	err = reader.GetResource(badSource, dest)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}
