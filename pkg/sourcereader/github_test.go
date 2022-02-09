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

func (s *MySuite) TestCopyGitHubResources(c *C) {
	// Setup
	destDir := path.Join(testDir, "TestCopyGitHubRepository")
	if err := os.Mkdir(destDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Success via HTTPS
	destDirForHTTPS := path.Join(destDir, "https")
	err := copyGitHubResources("github.com/terraform-google-modules/terraform-google-project-factory//helpers", destDirForHTTPS)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(path.Join(destDirForHTTPS, "terraform_validate"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "terraform_validate")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
}

func (s *MySuite) TestGetResourceInfo_GitHub(c *C) {
	reader := GitHubSourceReader{}

	// Invalid GitHub repository - path does not exists
	badGitRepo := "github.com:not/exist.git"
	_, err := reader.GetResourceInfo(badGitRepo, tfKindString)
	expectedErr := "failed to clone GitHub resource at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/resources"
	_, err = reader.GetResourceInfo(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetResource_GitHub(c *C) {
	reader := GitHubSourceReader{}

	// Invalid GitHub repository - path does not exists
	badGitRepo := "github.com:not/exist.git"
	err := reader.GetResource(badGitRepo, tfKindString)
	expectedErr := "failed to clone GitHub resource at .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid: Unsupported Resource Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/resources"
	err = reader.GetResource(badSource, tfKindString)
	expectedErr = "Source is not valid: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}
