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
	"errors"
	"os"
	"os/exec"

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestPacker(c *C) {
	if _, err := exec.LookPath("packer"); err != nil {
		err := ConfigurePacker()
		c.Assert(err, NotNil)
		c.Skip("packer not found in PATH")
	}

	err := ConfigurePacker()
	c.Assert(err, IsNil)

	// test failure when terraform cannot be found in PATH
	pathEnv := os.Getenv("PATH")
	os.Setenv("PATH", "")
	err = ConfigurePacker()
	os.Setenv("PATH", pathEnv)
	c.Assert(err, NotNil)

	var tfe *TfError
	c.Assert(errors.As(err, &tfe), Equals, true)

	// executing with help argument (safe against RedHat binary named packer)
	err = ExecPackerCmd(".", true, "-h")
	c.Assert(err, IsNil)
	// executing with arguments will error
	err = ExecPackerCmd(".", false)
	c.Assert(err, NotNil)
}
