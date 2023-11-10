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
	"log"
	"os"
	"path/filepath"
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
	dir, err := os.MkdirTemp("", dirName)
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
	createTmpModule()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func (s *MySuite) TestIsEmbeddedPath(c *C) {
	// True: Is an embedded path
	ret := IsEmbeddedPath("modules/anything/else")
	c.Assert(ret, Equals, true)

	// False: Local path
	ret = IsEmbeddedPath("./modules/else")
	c.Assert(ret, Equals, false)

	ret = IsEmbeddedPath("./modules")
	c.Assert(ret, Equals, false)

	ret = IsEmbeddedPath("../modules/")
	c.Assert(ret, Equals, false)

	// False, other
	ret = IsEmbeddedPath("github.com/modules")
	c.Assert(ret, Equals, false)
}

func (s *MySuite) TestIsLocalPath(c *C) {
	// False: Embedded Path
	ret := IsLocalPath("modules/anything/else")
	c.Assert(ret, Equals, false)

	// True: Local path
	ret = IsLocalPath("./anything/else")
	c.Assert(ret, Equals, true)

	ret = IsLocalPath("./modules")
	c.Assert(ret, Equals, true)

	ret = IsLocalPath("../modules/")
	c.Assert(ret, Equals, true)

	// False, other
	ret = IsLocalPath("github.com/modules")
	c.Assert(ret, Equals, false)
}

func (s *MySuite) TestIsRemotePath(c *C) {
	// False: Is an embedded path
	ret := IsRemotePath("modules/anything/else")
	c.Check(ret, Equals, false)

	// False: Local path
	ret = IsRemotePath("./anything/else")
	c.Check(ret, Equals, false)

	ret = IsRemotePath("./modules")
	c.Check(ret, Equals, false)

	ret = IsRemotePath("../modules/")
	c.Check(ret, Equals, false)

	// True, other
	ret = IsRemotePath("github.com/modules")
	c.Check(ret, Equals, true)

	// True, genetic git repository
	ret = IsRemotePath("git::https://gitlab.com/modules")
	c.Check(ret, Equals, true)

	// True, invalid path though nor local nor embedded
	ret = IsRemotePath("wut:://modules")
	c.Check(ret, Equals, true)
}

func (s *MySuite) TestFactory(c *C) {
	// Local modules
	locSrcReader := Factory("./modules/anything/else")
	c.Check(reflect.TypeOf(locSrcReader), Equals, reflect.TypeOf(LocalSourceReader{}))

	// Embedded modules
	embSrcReader := Factory("modules/anything/else")
	c.Check(reflect.TypeOf(embSrcReader), Equals, reflect.TypeOf(EmbeddedSourceReader{}))

	// GitHub modules
	ghSrcString := Factory("github.com/modules")
	c.Check(reflect.TypeOf(ghSrcString), Equals, reflect.TypeOf(GoGetterSourceReader{}))

	// Git modules
	gitSrcString := Factory("git::https://gitlab.com/modules")
	c.Check(reflect.TypeOf(gitSrcString), Equals, reflect.TypeOf(GoGetterSourceReader{}))
}

func (s *MySuite) TestCopyFromPath(c *C) {
	dstPath := filepath.Join(testDir, "TestCopyFromPath_Dst")

	err := copyFromPath(terraformDir, dstPath)
	c.Assert(err, IsNil)
	fInfo, err := os.Stat(filepath.Join(dstPath, "main.tf"))
	c.Assert(err, IsNil)
	c.Assert(fInfo.Name(), Equals, "main.tf")
	c.Assert(fInfo.Size() > 0, Equals, true)
	c.Assert(fInfo.IsDir(), Equals, false)

	// Invalid: Specify the same destination path again
	err = copyFromPath(terraformDir, dstPath)
	c.Assert(err, ErrorMatches, "the directory already exists: .*")
}
