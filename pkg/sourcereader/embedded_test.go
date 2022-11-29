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
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestGetModuleInfo_Embedded(c *C) {
	ModuleFS = moduleFS
	reader := EmbeddedSourceReader{}

	// Success
	moduleInfo, err := reader.GetModuleInfo("modules/test_data/network/vpc", tfKindString)
	c.Assert(err, IsNil)
	c.Assert(moduleInfo.Inputs[0].Name, Equals, "test_variable")
	c.Assert(moduleInfo.Outputs[0].Name, Equals, "test_output")

	// Invalid: No embedded modules
	badEmbeddedMod := "modules/does/not/exist"
	moduleInfo, err = reader.GetModuleInfo(badEmbeddedMod, tfKindString)
	expectedErr := "Failed to read module directory: Module directory modules/does/not/exist does not exist or cannot be read."
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	moduleInfo, err = reader.GetModuleInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetModule_Embedded(c *C) {
	ModuleFS = moduleFS
	reader := EmbeddedSourceReader{}

	// Success
	dest := filepath.Join(testDir, "TestGetModule_Embedded")
	err := reader.GetModule("modules/test_data/network", dest)
	c.Assert(err, IsNil)

	// Invalid: Write to the same dest directory again
	err = reader.GetModule("modules/test_data/network", dest)
	expectedErr := "The directory already exists: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Success
	fInfo, err := os.Stat(filepath.Join(dest, "vpc/main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
	fInfo, err = os.Stat(filepath.Join(dest, "vpc"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "vpc")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, true)

	// Invalid: No embedded Module
	badEmbeddedMod := "modules/does/not/exist"
	err = reader.GetModule(badEmbeddedMod, dest)
	expectedErr = "Failed to read module directory: Module directory modules/does/not/exist does not exist or cannot be read."
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source by EmbeddedSourceReader
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	err = reader.GetModule(badSource, dest)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}
