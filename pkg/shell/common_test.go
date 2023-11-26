/*
Copyright 2023 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package shell

import (
	"hpc-toolkit/pkg/config"
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestIntersectMapKeys(c *C) {
	// test map whose keys completely overlap with slice
	a := []string{"key0", "key1", "key2"}
	m := make(map[string]bool)
	for _, v := range a {
		m[v] = true
	}
	intersection := intersectMapKeys(a, m)
	c.Assert(intersection, DeepEquals, m)

	// test that additional key in map does not affect intersection
	mCopy := make(map[string]bool)
	for k, v := range m {
		mCopy[k] = v
	}
	mCopy["foo"] = true
	intersection = intersectMapKeys(a, mCopy)
	c.Assert(intersection, DeepEquals, m)

	// test that removal of key from slice results in expected reduced overlap
	mCopy = make(map[string]bool)
	for k, v := range m {
		mCopy[k] = v
	}
	delete(mCopy, a[0])
	intersection = intersectMapKeys(a[1:], m)
	c.Assert(intersection, DeepEquals, mCopy)
}

func (s *MySuite) TestCheckWritableDir(c *C) {
	c.Assert(CheckWritableDir(""), IsNil)

	dir := c.MkDir()
	if err := os.Chmod(dir, 0700); err != nil {
		c.Fatal(err)
	}
	c.Assert(CheckWritableDir(dir), IsNil)

	// This test reliably fails in Cloud Build although it works in Linux
	// and in MacOS. TODO: investigate why
	// err = os.Chmod(dir, 0600)
	// if err != nil {
	//      c.Error(err)
	// }
	// err = CheckWritableDir(dir)
	// c.Assert(err, NotNil)

	os.RemoveAll(dir)
	c.Assert(CheckWritableDir(dir), NotNil)
}

func (s *MySuite) TestMergeMapsWithoutLoss(c *C) {
	t := map[string]int{"foo": 0}
	f := map[string]int{"bar": 1}

	c.Check(mergeMapsWithoutLoss(t, f), IsNil)
	c.Check(f, DeepEquals, map[string]int{"bar": 1})
	c.Check(t, DeepEquals, map[string]int{"foo": 0, "bar": 1})

	c.Check(mergeMapsWithoutLoss(t, f), ErrorMatches, "duplicate key bar")
}

func (s *MySuite) TestValidateDeploymentDirectory(c *C) {
	dir := c.MkDir()
	groups := []config.DeploymentGroup{
		{
			Name: "zero",
		},
		{
			Name: "one",
		},
		{
			Name: "two",
		},
	}

	for _, g := range groups {
		err := os.Mkdir(filepath.Join(dir, string(g.Name)), 0755)
		if err != nil {
			c.Fatal(err)
		}
	}

	// do not fail if exactly matching directories
	c.Assert(ValidateDeploymentDirectory(groups, dir), IsNil)

	// do not fail for extra directories
	badGroupDir := filepath.Join(dir, "not-a-group-name")
	os.Mkdir(badGroupDir, 0755)
	c.Assert(ValidateDeploymentDirectory(groups, dir), IsNil)
	os.Remove(badGroupDir)

	// do fail if missing directories
	os.Remove(filepath.Join(dir, string(groups[0].Name)))
	c.Assert(ValidateDeploymentDirectory(groups, dir), NotNil)
}
