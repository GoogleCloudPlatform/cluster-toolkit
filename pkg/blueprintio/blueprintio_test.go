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

package blueprintio

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"testing"
	"time"

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
	dirName := fmt.Sprintf("ghpc_blueprintio_test_%s", t.Format(time.RFC3339))
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

func (s *MySuite) TestGetBlueprintIOLocal(c *C) {
	blueprintio := GetBlueprintIOLocal()
	c.Assert(blueprintio, Equals, blueprintios["local"])
}

func (s *MySuite) TestCreateDirectoryLocal(c *C) {
	blueprintio := GetBlueprintIOLocal()

	// Try to create the exist directory
	err := blueprintio.CreateDirectory(testDir)
	expErr := "The directory already exists: .*"
	c.Assert(err, ErrorMatches, expErr)

	directoryName := "dir_TestCreateDirectoryLocal"
	createdDir := path.Join(testDir, directoryName)
	err = blueprintio.CreateDirectory(createdDir)
	c.Assert(err, IsNil)

	_, err = os.Stat(createdDir)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestGetAbsSourcePath(c *C) {
	// Already abs path
	gotPath := getAbsSourcePath(testDir)
	c.Assert(gotPath, Equals, testDir)

	// Relative path
	relPath := "relative/path"
	cwd, err := os.Getwd()
	c.Assert(err, IsNil)
	gotPath = getAbsSourcePath(relPath)
	c.Assert(gotPath, Equals, path.Join(cwd, relPath))
}

func (s *MySuite) TestCopyFromPathLocal(c *C) {
	blueprintio := GetBlueprintIOLocal()
	testSrcFilename := path.Join(testDir, "testSrc")
	str := []byte("TestCopyFromPathLocal")
	if err := os.WriteFile(testSrcFilename, str, 0755); err != nil {
		log.Fatalf("blueprintio_test: failed to create %s: %v", testSrcFilename, err)
	}

	testDstFilename := path.Join(testDir, "testDst")
	blueprintio.CopyFromPath(testSrcFilename, testDstFilename)

	src, err := ioutil.ReadFile(testSrcFilename)
	if err != nil {
		log.Fatalf("blueprintio_test: failed to read %s: %v", testSrcFilename, err)
	}

	dst, err := ioutil.ReadFile(testDstFilename)
	if err != nil {
		log.Fatalf("blueprintio_test: failed to read %s: %v", testDstFilename, err)
	}

	c.Assert(string(src), Equals, string(dst))
}

func (s *MySuite) TestMkdirWrapper(c *C) {
	testMkdirWrapperDir := path.Join(testDir, "testMkdirWrapperDir")
	if err := os.Mkdir(testMkdirWrapperDir, 0444); err != nil { // Make it ReadOnly
		log.Fatalf("Failed to create the directory %s: %v", testMkdirWrapperDir, err)
	}
	testNewDir := path.Join(testMkdirWrapperDir, "testNewDir")

	// Failure: permission denied
	err := mkdirWrapper(testNewDir)
	expErr := "Failed to create the directory .*"
	c.Assert(err, ErrorMatches, expErr)

	// Success: create a new directory under the writable directory
	if err := os.Chmod(testMkdirWrapperDir, 0755); err != nil { // Make it Writable
		log.Fatalf("Failed to change the permission %s: %v", testMkdirWrapperDir, err)
	}
	err = mkdirWrapper(testNewDir)
	c.Assert(err, IsNil)
}
