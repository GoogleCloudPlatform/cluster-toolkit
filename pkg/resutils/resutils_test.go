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

package resutils

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"testing"
	"time"

	"github.com/spf13/afero"
	. "gopkg.in/check.v1"
)

var testDir string

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func setup() {
	t := time.Now()
	dirName := fmt.Sprintf("ghpc_reswriter_test_%s", t.Format(time.RFC3339))
	dir, err := ioutil.TempDir("", dirName)
	if err != nil {
		log.Fatalf("reswriter_test: %v", err)
	}
	testDir = dir
}

func teardown() {
	os.RemoveAll(testDir)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

// Tests
func getTestFS() afero.IOFS {
	aferoFS := afero.NewMemMapFs()
	aferoFS.MkdirAll("resources/network/vpc", 0755)
	afero.WriteFile(
		aferoFS, "resources/network/vpc/main.tf", []byte("test string"), 0644)
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
	err := CopyDirFromResources(testResFS, "resources/network/vpc", copyDir)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(path.Join(copyDir, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Success: copy files AND dirs
	err = CopyDirFromResources(testResFS, "resources/network/", copyDir)
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
	err = CopyDirFromResources(testResFS, "not/valid", copyDir)
	c.Assert(err, ErrorMatches, "*file does not exist")

	// Failure: File Already Exists
	err = CopyDirFromResources(testResFS, "resources/network/", copyDir)
	c.Assert(err, ErrorMatches, "*file exists")
}

func (s *MySuite) TestCopyGitHubResources(c *C) {
	// Setup
	destDir := path.Join(testDir, "TestCloneGitHubRepository")
	if err := os.Mkdir(destDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Success via SSH
	destDirForSSH := path.Join(destDir, "ssh")
	err := CopyGitHubResources("git@github.com:google/google.github.io.git//_layouts", destDirForSSH)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(path.Join(destDirForSSH, "redirect.html"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "redirect.html")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Success via HTTPS
	destDirForHTTPS := path.Join(destDir, "https")
	err = CopyGitHubResources("github.com/google/google.github.io//_layouts", destDirForHTTPS)
	c.Assert(err, IsNil)
	fInfo, err = os.Stat(path.Join(destDirForSSH, "redirect.html"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "redirect.html")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)
}

func (s *MySuite) TestIsEmbeddedPath(c *C) {
	// True: Is an embedded path
	ret := IsEmbeddedPath("resources/anything/else")
	c.Assert(ret, Equals, true)

	// False: Local path
	ret = IsEmbeddedPath("./anything/else")
	c.Assert(ret, Equals, false)

	ret = IsEmbeddedPath("./resources")
	c.Assert(ret, Equals, false)

	ret = IsEmbeddedPath("../resources/")
	c.Assert(ret, Equals, false)

	// False, other
	ret = IsEmbeddedPath("github.com/resources")
	c.Assert(ret, Equals, false)
}

func (s *MySuite) TestIsLocalPath(c *C) {
	// False: Embedded Path
	ret := IsLocalPath("resources/anything/else")
	c.Assert(ret, Equals, false)

	// True: Local path
	ret = IsLocalPath("./anything/else")
	c.Assert(ret, Equals, true)

	ret = IsLocalPath("./resources")
	c.Assert(ret, Equals, true)

	ret = IsLocalPath("../resources/")
	c.Assert(ret, Equals, true)

	// False, other
	ret = IsLocalPath("github.com/resources")
	c.Assert(ret, Equals, false)
}

func (s *MySuite) TestIsGitHubRepository(c *C) {
	// False: Is an embedded path
	ret := IsGitHubPath("resources/anything/else")
	c.Assert(ret, Equals, false)

	// False: Local path
	ret = IsGitHubPath("./anything/else")
	c.Assert(ret, Equals, false)

	ret = IsGitHubPath("./resources")
	c.Assert(ret, Equals, false)

	ret = IsGitHubPath("../resources/")
	c.Assert(ret, Equals, false)

	// True, other
	ret = IsGitHubPath("github.com/resources")
	c.Assert(ret, Equals, true)
}
