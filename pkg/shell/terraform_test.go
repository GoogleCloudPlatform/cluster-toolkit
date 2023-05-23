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
	"testing"

	. "gopkg.in/check.v1"
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *MySuite) TestFindTerraform(c *C) {
	if _, err := exec.LookPath("terraform"); err != nil {
		_, err := ConfigureTerraform(".")
		c.Assert(err, NotNil)
		c.Skip("terraform not found in PATH")
	}

	_, err := ConfigureTerraform(".")
	c.Assert(err, IsNil)

	// test failure when terraform cannot be found in PATH
	pathEnv := os.Getenv("PATH")
	os.Setenv("PATH", "")
	_, err = ConfigureTerraform(".")
	os.Setenv("PATH", pathEnv)
	c.Assert(err, NotNil)

	var tfe *TfError
	c.Assert(errors.As(err, &tfe), Equals, true)
}
