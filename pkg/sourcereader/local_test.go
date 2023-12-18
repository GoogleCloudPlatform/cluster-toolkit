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

type localSuite struct {
	r     LocalSourceReader
	tfDir string
}

var _ = Suite(&localSuite{})

func (s *localSuite) SetUpSuite(c *C) {
	s.r = LocalSourceReader{}
	d := c.MkDir()
	// copy embedded modules to a temp directory
	if err := copyDir(testEmbeddedFS, "modules", d); err != nil {
		c.Fatal(err)
	}
	s.tfDir = filepath.Join(d, "network/vpc")
}

func (s *localSuite) TestGetModule(c *C) {
	// Success
	dest := filepath.Join(c.MkDir(), c.TestName())
	err := s.r.GetModule(s.tfDir, dest)
	c.Assert(err, IsNil)

	// Invalid: Write to the same dest directory again
	err = s.r.GetModule(s.tfDir, dest)
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
	err = s.r.GetModule(badLocalMod, dest)
	expectedErr = "local module doesn't exist at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source by LocalSourceReader
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	err = s.r.GetModule(badSource, dest)
	expectedErr = "source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *localSuite) TestCopyFromPath_Absent(c *C) {
	src := filepath.Join(s.tfDir, "waldo")
	dst := filepath.Join(c.MkDir(), c.TestName())

	c.Assert(copyFromPath(src, dst), NotNil)
}

func (s *localSuite) TestCopyFromPath(c *C) {
	dst := filepath.Join(c.MkDir(), c.TestName())

	err := copyFromPath(s.tfDir, dst)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(filepath.Join(dst, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Invalid: Specify the same destination path again
	err = copyFromPath(s.tfDir, dst)
	c.Assert(err, ErrorMatches, "the directory already exists: .*")
}
