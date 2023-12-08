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
	"path/filepath"

	. "gopkg.in/check.v1"
)

// Util Functions
func createTmpModule() {
	var err error

	// Create terraform module dir
	terraformDir = filepath.Join(testDir, "terraformModule")
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
		log.Fatalf("sourcereader_local_test: Failed to write main.tf test file. %v", err)
	}

	// variables.tf file
	varFile, err := os.Create(filepath.Join(terraformDir, "variables.tf"))
	if err != nil {
		log.Fatalf("Failed to create variables.tf: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"sourcereader_local_test: Failed to write variables.tf test file. %v", err)
	}

	// outputs.tf file
	outFile, err := os.Create(filepath.Join(terraformDir, "outputs.tf"))
	if err != nil {
		log.Fatalf("Failed to create outputs.tf: %v", err)
	}
	_, err = outFile.WriteString(testOutputsTf)
	if err != nil {
		log.Fatalf("sourcereader_local_test: Failed to write outputs.tf test file. %v", err)
	}

	// Create packer module dir
	packerDir = filepath.Join(testDir, "packerModule")
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
		log.Fatalf("sourcereader_local_test: Failed to write main.pkr.hcl test file. %v", err)
	}

	// variables.pkr.hcl file
	varFile, err = os.Create(filepath.Join(packerDir, "variables.pkr.hcl"))
	if err != nil {
		log.Fatalf("Failed to create variables.pkr.hcl: %v", err)
	}
	_, err = varFile.WriteString(testVariablesTf)
	if err != nil {
		log.Fatalf(
			"sourcereader_local_test: Failed to write variables.pkr.hcl test file. %v", err)
	}
}

func (s *MySuite) TestGetModule_Local(c *C) {
	reader := LocalSourceReader{}

	// Success
	dest := filepath.Join(testDir, "TestGetModule_Local")
	err := reader.GetModule(terraformDir, dest)
	c.Assert(err, IsNil)

	// Invalid: Write to the same dest directory again
	err = reader.GetModule(terraformDir, dest)
	expectedErr := "the directory already exists: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Success
	fInfo, err := os.Stat(filepath.Join(dest, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Invalid: No local module
	badLocalMod := "./modules/does/not/exist"
	err = reader.GetModule(badLocalMod, dest)
	expectedErr = "local module doesn't exist at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source by LocalSourceReader
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	err = reader.GetModule(badSource, dest)
	expectedErr = "source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}
