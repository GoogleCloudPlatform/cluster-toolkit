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

package cmd

import (
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestIsDir(c *C) {
	dir := c.MkDir()
	c.Assert(checkDir(nil, []string{dir}), IsNil)

	p := filepath.Join(dir, "does-not-exist")
	c.Assert(checkDir(nil, []string{p}), NotNil)

	f, err := os.CreateTemp(dir, "test-*")
	c.Assert(err, IsNil)
	c.Assert(checkDir(nil, []string{f.Name()}), NotNil)
}

func (s *MySuite) TestRunExport(c *C) {
	dir := c.MkDir()
	c.Assert(runExportCmd(nil, []string{dir}), NotNil)
}
