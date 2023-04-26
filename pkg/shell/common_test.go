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
	"os"

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
	err := CheckWritableDir("")
	c.Assert(err, IsNil)

	dir, err := os.MkdirTemp("", "example")
	if err != nil {
		c.Fatal(err)
	}
	defer os.RemoveAll(dir)

	err = os.Chmod(dir, 0700)
	if err != nil {
		c.Error(err)
	}
	err = CheckWritableDir(dir)
	c.Assert(err, IsNil)

	// This test reliably fails in Cloud Build although it works in Linux
	// and in MacOS. TODO: investigate why
	// err = os.Chmod(dir, 0600)
	// if err != nil {
	// 	c.Error(err)
	// }
	// err = CheckWritableDir(dir)
	// c.Assert(err, NotNil)

	os.RemoveAll(dir)
	err = CheckWritableDir(dir)
	c.Assert(err, NotNil)
}
