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

type embeddedSuite struct {
	r EmbeddedSourceReader
}

var _ = Suite(&embeddedSuite{})

func (s *embeddedSuite) SetUpTest(c *C) {
	ModuleFS = testEmbeddedFS
	s.r = EmbeddedSourceReader{}
}

func (s *embeddedSuite) TestCopyDir_Embedded(c *C) {
	dst := c.MkDir()

	// Success
	err := s.r.CopyDir("modules/network/vpc", dst)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(filepath.Join(dst, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Success: copy files AND dirs
	err = s.r.CopyDir("modules/network", dst)
	c.Assert(err, IsNil)
	fInfo, err = os.Stat(filepath.Join(dst, "vpc/main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
	fInfo, err = os.Stat(filepath.Join(dst, "vpc"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "vpc")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, true)

	// Invalid path
	err = s.r.CopyDir("not/valid", dst)
	c.Assert(err, ErrorMatches, "*file does not exist")

	// Failure: File Already Exists
	err = s.r.CopyDir("modules/network", dst)
	c.Assert(err, ErrorMatches, "*file exists")
}

func (s *embeddedSuite) TestGetModule_Embedded(c *C) {
	// Success
	dest := filepath.Join(c.MkDir(), c.TestName())
	err := s.r.GetModule("modules/network", dest)
	c.Assert(err, IsNil)

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

	// Invalid: Write to the same dest directory again
	err = s.r.GetModule("modules/network", dest)
	c.Assert(err, ErrorMatches, "the directory already exists: .*")

	// Invalid: No embedded Module
	err = s.r.GetModule("modules/does/not/exist", dest)
	c.Assert(err, ErrorMatches, "failed to copy embedded module at .*")

	// Invalid: Unsupported Module Source by EmbeddedSourceReader
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	err = s.r.GetModule(badSource, dest)
	c.Assert(err, ErrorMatches, "source is not valid: .*")
}

func (s *embeddedSuite) TestGetModule_NilFs(c *C) {
	ModuleFS = nil
	c.Assert(s.r.GetModule("here", "there"), NotNil)
}

func (s *embeddedSuite) TestCopyDir_NilFs(c *C) {
	ModuleFS = nil
	c.Assert(s.r.CopyDir("here", "there"), NotNil)
}
