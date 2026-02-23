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

package deploymentio

import (
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"
)

func (s *zeroSuite) TestCreateDirectoryLocal(c *C) {
	dio := GetDeploymentioLocal()
	dir := c.MkDir()

	{ // Try to create the exist directory
		err := dio.CreateDirectory(dir)
		c.Assert(err, ErrorMatches, "the directory already exists: .*")
	}

	{ // Ok
		sub := filepath.Join(dir, "dog/cat")
		c.Assert(dio.CreateDirectory(sub), IsNil)
		stat, err := os.Stat(sub)
		c.Assert(err, IsNil)
		c.Check(stat.IsDir(), Equals, true)
	}
}

func (s *zeroSuite) TestCopyFromPathLocal(c *C) {
	dio := GetDeploymentioLocal()
	dir := c.MkDir()

	src := filepath.Join(dir, "zebra")
	if err := os.WriteFile(src, []byte("jupiter"), 0755); err != nil {
		c.Fatal(err)
	}

	dst := filepath.Join(dir, "pony")
	c.Assert(dio.CopyFromPath(src, dst), IsNil)

	got, err := os.ReadFile(dst)
	if err != nil {
		c.Fatal(err)
	}

	c.Assert("jupiter", Equals, string(got))
}

func (s *zeroSuite) TestMkdirWrapper(c *C) {
	dir := c.MkDir()
	{ // Success
		dst := filepath.Join(dir, "piranha/barracuda")
		c.Assert(mkdirWrapper(dst), IsNil)
	}

	{ // Failure: Path is not a directory
		dst := filepath.Join(dir, "watermelon")
		_, err := os.Create(dst)
		c.Assert(err, IsNil)

		c.Assert(mkdirWrapper(dst), ErrorMatches, "failed to create the directory .*")
	}
}

func (s *zeroSuite) TestCopyFromFS(c *C) {
	dio := GetDeploymentioLocal()
	testFS := getTestFS()
	dir := c.MkDir()

	// Success
	src := "pkg/modulewriter/deployment.gitignore.tmpl"
	dst := filepath.Join(dir, ".gitignore")
	err := dio.CopyFromFS(testFS, src, dst)
	c.Assert(err, IsNil)
	data, err := os.ReadFile(dst)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, testGitignoreTmpl)

	// Success: This truncates the file if it already exists in the destination
	newSrc := "pkg/modulewriter/deployment_new.gitignore.tmpl"
	err = dio.CopyFromFS(testFS, newSrc, dst)
	c.Assert(err, IsNil)
	newData, err := os.ReadFile(dst)
	c.Assert(err, IsNil)
	c.Assert(string(newData), Equals, testGitignoreNewTmpl)

	// Failure: Invalid path
	err = dio.CopyFromFS(testFS, "not/valid", dst)
	c.Assert(err, ErrorMatches, "*file does not exist")
}
