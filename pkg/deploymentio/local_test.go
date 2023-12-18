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

package deploymentio

import (
	"log"
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestCreateDirectoryLocal(c *C) {
	deploymentio := GetDeploymentioLocal()

	// Try to create the exist directory
	err := deploymentio.CreateDirectory(testDir)
	expErr := "the directory already exists: .*"
	c.Assert(err, ErrorMatches, expErr)

	directoryName := "dir_TestCreateDirectoryLocal"
	createdDir := filepath.Join(testDir, directoryName)
	err = deploymentio.CreateDirectory(createdDir)
	c.Assert(err, IsNil)

	_, err = os.Stat(createdDir)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestCopyFromPathLocal(c *C) {
	deploymentio := GetDeploymentioLocal()
	testSrcFilename := filepath.Join(testDir, "testSrc")
	str := []byte("TestCopyFromPathLocal")
	if err := os.WriteFile(testSrcFilename, str, 0755); err != nil {
		log.Fatalf("deploymentio_test: failed to create %s: %v", testSrcFilename, err)
	}

	testDstFilename := filepath.Join(testDir, "testDst")
	deploymentio.CopyFromPath(testSrcFilename, testDstFilename)

	src, err := os.ReadFile(testSrcFilename)
	if err != nil {
		log.Fatalf("deploymentio_test: failed to read %s: %v", testSrcFilename, err)
	}

	dst, err := os.ReadFile(testDstFilename)
	if err != nil {
		log.Fatalf("deploymentio_test: failed to read %s: %v", testDstFilename, err)
	}

	c.Assert(string(src), Equals, string(dst))
}

func (s *MySuite) TestMkdirWrapper(c *C) {
	// Success
	testMkdirWrapperDir := filepath.Join(testDir, "testMkdirWrapperDir")
	err := mkdirWrapper(testMkdirWrapperDir)
	c.Assert(err, IsNil)

	// Failure: Path is not a directory
	badMkdirWrapperDir := filepath.Join(testDir, "NotADir")
	_, err = os.Create(badMkdirWrapperDir)
	c.Assert(err, IsNil)
	err = mkdirWrapper(badMkdirWrapperDir)
	expErr := "failed to create the directory .*"
	c.Assert(err, ErrorMatches, expErr)
}

func (s *MySuite) TestCopyFromFS(c *C) {
	// Success
	deploymentio := GetDeploymentioLocal()
	testFS := getTestFS()
	testSrcGitignore := "pkg/modulewriter/deployment.gitignore.tmpl"
	testDstGitignore := filepath.Join(testDir, ".gitignore")
	err := deploymentio.CopyFromFS(testFS, testSrcGitignore, testDstGitignore)
	c.Assert(err, IsNil)
	data, err := os.ReadFile(testDstGitignore)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, testGitignoreTmpl)

	// Success: This truncates the file if it already exists in the destination
	testSrcNewGitignore := "pkg/modulewriter/deployment_new.gitignore.tmpl"
	err = deploymentio.CopyFromFS(testFS, testSrcNewGitignore, testDstGitignore)
	c.Assert(err, IsNil)
	newData, err := os.ReadFile(testDstGitignore)
	c.Assert(err, IsNil)
	c.Assert(string(newData), Equals, testGitignoreNewTmpl)

	// Failure: Invalid path
	err = deploymentio.CopyFromFS(testFS, "not/valid", testDstGitignore)
	c.Assert(err, ErrorMatches, "*file does not exist")
}
