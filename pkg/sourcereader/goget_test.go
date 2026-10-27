// Copyright 2026 Google LLC
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

type goGetSuite struct {
	r GoGetterSourceReader
}

var _ = Suite(&goGetSuite{})

func (s *goGetSuite) TestGetModule_GoGet(c *C) {
	destDir := c.MkDir()

	// Ensure GIT_INDEX_FILE (set by git commit during pre-commit hooks) is not
	// modified by child git processes spawned by go-getter.
	fakeIndex := filepath.Join(destDir, "fake-git-index")
	sentinel := []byte("original-index-content")
	c.Assert(os.WriteFile(fakeIndex, sentinel, 0600), IsNil)
	origIndex, hadIndex := os.LookupEnv("GIT_INDEX_FILE")
	c.Assert(os.Setenv("GIT_INDEX_FILE", fakeIndex), IsNil)
	defer func() {
		if hadIndex {
			_ = os.Setenv("GIT_INDEX_FILE", origIndex)
		} else {
			_ = os.Unsetenv("GIT_INDEX_FILE")
		}
	}()

	// Success via HTTPS
	destDirForHTTPS := filepath.Join(destDir, "https")
	err := s.r.GetModule("github.com/terraform-google-modules/terraform-google-project-factory//helpers", destDirForHTTPS)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(filepath.Join(destDirForHTTPS, "terraform_validate"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "terraform_validate")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	gotIndex, err := os.ReadFile(fakeIndex)
	c.Assert(err, IsNil)
	c.Assert(string(gotIndex), Equals, string(sentinel))
	c.Assert(os.Getenv("GIT_INDEX_FILE"), Equals, fakeIndex)

	// Success via HTTPS (Root directory)
	destDirForHTTPSRootDir := filepath.Join(destDir, "https-rootdir")
	err = s.r.GetModule("github.com/terraform-google-modules/terraform-google-service-accounts.git?ref=v4.1.1", destDirForHTTPSRootDir)
	c.Assert(err, IsNil)
	fInfo, err = os.Stat(filepath.Join(destDirForHTTPSRootDir, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Invalid git repository - path does not exists
	badGitRepo := "github.com:not/exist.git"
	c.Assert(s.r.GetModule(badGitRepo, tfKindString), NotNil)

	// Invalid: Unsupported Module Source
	badSource := "wut::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	c.Assert(s.r.GetModule(badSource, tfKindString), NotNil)
}
