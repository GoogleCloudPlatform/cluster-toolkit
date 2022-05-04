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

func (s *MySuite) TestCopyGitHubModules(c *C) {
	// Setup
	destDir := filepath.Join(testDir, "TestCopyGitHubRepository")
	if err := os.Mkdir(destDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Success via HTTPS
	destDirForHTTPS := filepath.Join(destDir, "https")
	err := copyGitHubModules("github.com/terraform-google-modules/terraform-google-project-factory//helpers", destDirForHTTPS)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(filepath.Join(destDirForHTTPS, "terraform_validate"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "terraform_validate")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Success via HTTPS (Root directory)
	destDirForHTTPSRootDir := filepath.Join(destDir, "https-rootdir")
	err = copyGitHubModules("github.com/terraform-google-modules/terraform-google-service-accounts.git?ref=v4.1.1", destDirForHTTPSRootDir)
	c.Assert(err, IsNil)
	fInfo, err = os.Stat(filepath.Join(destDirForHTTPSRootDir, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
}

func (s *MySuite) TestGetModuleInfo_GitHub(c *C) {
	reader := GitHubSourceReader{}

	// Invalid GitHub repository - path does not exists
	badGitRepo := "github.com:not/exist.git"
	_, err := reader.GetModuleInfo(badGitRepo, tfKindString)
	expectedErr := "failed to clone GitHub module at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	_, err = reader.GetModuleInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetModule_GitHub(c *C) {
	reader := GitHubSourceReader{}

	// Invalid GitHub repository - path does not exists
	badGitRepo := "github.com:not/exist.git"
	err := reader.GetModule(badGitRepo, tfKindString)
	expectedErr := "failed to clone GitHub module at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Module Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	err = reader.GetModule(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}
