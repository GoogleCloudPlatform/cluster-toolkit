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

	. "gopkg.in/check.v1"
)

// Util Functions
func createTmpResource() {
	var err error

	// Create terraform resource dir
	terraformDir = path.Join(testDir, "terraformResource")
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
		log.Fatalf("sourcereader_local_test: Failed to write main.tf test file. %v", err)
	}

	// variables.tf file
	varFile, err := os.Create(path.Join(terraformDir, "variables.tf"))
	if err != nil {
		log.Fatalf("Failed to create variables.tf: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"sourcereader_local_test: Failed to write variables.tf test file. %v", err)
	}

	// outputs.tf file
	outFile, err := os.Create(path.Join(terraformDir, "outputs.tf"))
	if err != nil {
		log.Fatalf("Failed to create outputs.tf: %v", err)
	}
	_, err = outFile.WriteString(testOutputsTf)
	if err != nil {
		log.Fatalf("sourcereader_local_test: Failed to write outputs.tf test file. %v", err)
	}

	// Create packer resource dir
	packerDir = path.Join(testDir, "packerResource")
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
		log.Fatalf("sourcereader_local_test: Failed to write main.pkr.hcl test file. %v", err)
	}

	// variables.pkr.hcl file
	varFile, err = os.Create(path.Join(packerDir, "variables.pkr.hcl"))
	if err != nil {
		log.Fatalf("Failed to create variables.pkr.hcl: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"sourcereader_local_test: Failed to write variables.pkr.hcl test file. %v", err)
	}
}

func (s *MySuite) TestValidateResource_Local(c *C) {
	reader := LocalSourceReader{}

	// Invalid source path - points to directory with no .tf files
	pathToEmptyDir := path.Join(testDir, "emptyDir")
	err := os.Mkdir(pathToEmptyDir, 0755)
	if err != nil {
		log.Fatal("TestValidateResource_Local: Failed to create test directory.")
	}
	err = reader.ValidateResource(pathToEmptyDir, tfKindString)
	expectedErr := "failed to get info using tfconfig for terraform resource at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/resources"
	err = reader.ValidateResource(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetResourceInfo_Local(c *C) {
	reader := LocalSourceReader{}

	// Success
	resourceInfo, err := reader.GetResourceInfo(terraformDir, tfKindString)
	c.Assert(err, IsNil)
	c.Assert(resourceInfo.Inputs[0].Name, Equals, "test_variable")
	c.Assert(resourceInfo.Outputs[0].Name, Equals, "test_output")

	// Invalid source path - path does not exists
	badLocalRes := "./not/a/real/path"
	resourceInfo, err = reader.GetResourceInfo(badLocalRes, tfKindString)
	expectedErr := "failed to get info using tfconfig for terraform resource at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/resources"
	resourceInfo, err = reader.GetResourceInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetResource_Local(c *C) {
	reader := LocalSourceReader{}

	// Success
	dest := path.Join(testDir, "TestGetResource_Local")
	err := reader.GetResource(terraformDir, dest)
	c.Assert(err, IsNil)

	// Invalid: Write to the same dest directory again
	err = reader.GetResource(terraformDir, dest)
	expectedErr := "The directory already exists: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Success
	fInfo, err := os.Stat(path.Join(dest, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Invalid: No local resource
	badLocalRes := "./resources/does/not/exist"
	err = reader.GetResource(badLocalRes, dest)
	expectedErr = "Local resource doesn't exist at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source by LocalSourceReader
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/resources"
	err = reader.GetResource(badSource, dest)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}
