/*
Copyright 2026 Google LLC

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
	"os/exec"
	"path/filepath"
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
}

func (s *MySuite) TestInit(c *C) {
	if _, err := exec.LookPath("terraform"); err != nil {
		c.Skip("terraform not found in PATH")
	}

	tmpDir := c.MkDir()

	err := Init(tmpDir)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestInit_NoTerraform(c *C) {
	pathEnv := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", pathEnv)

	tmpDir := c.MkDir()
	err := Init(tmpDir)
	c.Assert(err, NotNil)
}

func (s *MySuite) TestInit_WithDummyTerraform(c *C) {
	tmpBinDir := c.MkDir()
	dummyTf := filepath.Join(tmpBinDir, "terraform")

	content := []byte(`#!/bin/sh
if [ "$1" = "version" ]; then
	echo '{"terraform_version": "1.5.7"}'
	exit 0
fi
exit 0
`)
	err := os.WriteFile(dummyTf, content, 0755)
	c.Assert(err, IsNil)

	pathEnv := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpBinDir+string(os.PathListSeparator)+pathEnv); err != nil {
		c.Fatal(err)
	}
	defer os.Setenv("PATH", pathEnv)

	tmpDir := c.MkDir()
	err = Init(tmpDir)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestTfVersion(c *C) {
	tmpBinDir := c.MkDir()
	dummyTf := filepath.Join(tmpBinDir, "terraform")
	content := []byte(`#!/bin/sh
if [ "$1" = "version" ]; then
	echo '{"terraform_version": "1.5.7"}'
	exit 0
fi
exit 0
`)
	err := os.WriteFile(dummyTf, content, 0755)
	c.Assert(err, IsNil)

	pathEnv := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpBinDir+string(os.PathListSeparator)+pathEnv); err != nil {
		c.Fatal(err)
	}
	defer os.Setenv("PATH", pathEnv)

	v, err := TfVersion()
	c.Assert(err, IsNil)
	c.Assert(v, Equals, "1.5.7")
}

func (s *MySuite) TestDestroy(c *C) {
	tmpBinDir := c.MkDir()
	dummyTf := filepath.Join(tmpBinDir, "terraform")
	content := []byte(`#!/bin/sh
# Handle version
if [ "$1" = "version" ]; then
	echo '{"terraform_version": "1.5.7"}'
	exit 0
fi

# Handle init
if [ "$1" = "init" ]; then
	exit 0
fi

# Handle plan
for arg in "$@"; do
    if [[ "$arg" == "plan" ]]; then
        IS_PLAN=1
    fi
    if [[ "$arg" == "-json" ]]; then
        IS_JSON=1
    fi
done

if [[ "$IS_PLAN" == "1" ]]; then
    if [[ "$IS_JSON" == "1" ]]; then
        echo '{"resource_changes": [], "output_changes": {}}'
    fi
    exit 0
fi

# Handle apply
for arg in "$@"; do
    if [[ "$arg" == "apply" ]]; then
        exit 0
    fi
done

exit 0
`)
	err := os.WriteFile(dummyTf, content, 0755)
	c.Assert(err, IsNil)

	pathEnv := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpBinDir+string(os.PathListSeparator)+pathEnv); err != nil {
		c.Fatal(err)
	}
	defer os.Setenv("PATH", pathEnv)

	tmpDir := c.MkDir()
	err = Init(tmpDir) // Ensure init works
	c.Assert(err, IsNil)

	tf, err := ConfigureTerraform(tmpDir)
	c.Assert(err, IsNil)

	err = Destroy(tf, AutomaticApply, TextOutput)
	c.Assert(err, IsNil)
}
