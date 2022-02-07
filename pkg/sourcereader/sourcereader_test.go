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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	. "gopkg.in/check.v1"
)

var (
	testDir      string
	terraformDir string
	packerDir    string
)

const (
	pkrKindString = "packer"
	tfKindString  = "terraform"
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func setup() {
	t := time.Now()
	dirName := fmt.Sprintf("ghpc_sourcereader_test_%s", t.Format(time.RFC3339))
	dir, err := ioutil.TempDir("", dirName)
	if err != nil {
		log.Fatalf("sourcereader_test: %v", err)
	}
	testDir = dir
}

func teardown() {
	os.RemoveAll(testDir)
}

func TestMain(m *testing.M) {
	setup()
	createTmpResource()
	code := m.Run()
	teardown()
	os.Exit(code)
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

func (s *MySuite) TestFactory(c *C) {
	// Local resources
	locSrcReader := Factory("./resources/anything/else")
	c.Assert(reflect.TypeOf(locSrcReader), Equals, reflect.TypeOf(LocalSourceReader{}))

	// Embedded resources
	embSrcReader := Factory("resources/anything/else")
	c.Assert(reflect.TypeOf(embSrcReader), Equals, reflect.TypeOf(EmbeddedSourceReader{}))

	// GitHub resources
	ghSrcString := Factory("github.com/resources")
	c.Assert(reflect.TypeOf(ghSrcString), Equals, reflect.TypeOf(GitHubSourceReader{}))
}

func (s *MySuite) TestCopyFromPath(c *C) {
	dstPath := path.Join(testDir, "TestCopyFromPath_Dst")

	err := copyFromPath(terraformDir, dstPath)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(path.Join(dstPath, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Invalid: Specify the same destination path again
	err = copyFromPath(terraformDir, dstPath)
	c.Assert(err, ErrorMatches, "The directory already exists: .*")
}
