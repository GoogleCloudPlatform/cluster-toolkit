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

package gcloud

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"

	. "gopkg.in/check.v1"
)

type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if os.Getenv("WANT_ERROR") == "1" {
		os.Exit(1)
	}
	defer os.Exit(0)

	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Too few arguments\n")
		os.Exit(1)
	}

	// Simulated gcloud output
	if len(args) > 3 && args[1] == "compute" && args[2] == "machine-types" && args[3] == "describe" {
		fmt.Print(`{"guestCpus": 8, "accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-tesla-t4"}]}`)
	} else {
		fmt.Print(`{"dummy": "data"}`)
	}
}

func (s *MySuite) TestRunGcloudJsonCommand_Success(c *C) {
	old := execCommand
	defer func() { execCommand = old }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}

	// Clear cache
	gcloudCache = sync.Map{}

	out, err := RunGcloudJsonCommand("compute", "machine-types", "describe", "n1-standard-8", "--zone", "us-central1-a")
	c.Assert(err, IsNil)
	c.Assert(string(out), Equals, `{"guestCpus": 8, "accelerators": [{"guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-tesla-t4"}]}`)
}

func (s *MySuite) TestRunGcloudJsonCommand_Caching(c *C) {
	old := execCommand
	defer func() { execCommand = old }()

	callCount := 0
	execCommand = func(name string, args ...string) *exec.Cmd {
		callCount++
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}

	gcloudCache = sync.Map{}

	_, err := RunGcloudJsonCommand("test-cache")
	c.Assert(err, IsNil)
	c.Assert(callCount, Equals, 1)

	_, err = RunGcloudJsonCommand("test-cache")
	c.Assert(err, IsNil)
	c.Assert(callCount, Equals, 1)
}

func (s *MySuite) TestRunGcloudJsonCommand_FormatAppend(c *C) {
	old := execCommand
	defer func() { execCommand = old }()

	var capturedArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		capturedArgs = args
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}

	gcloudCache = sync.Map{}

	_, err := RunGcloudJsonCommand("test-format")
	c.Assert(err, IsNil)
	c.Assert(capturedArgs[len(capturedArgs)-1], Equals, "--format=json")

	// Call with format already
	gcloudCache = sync.Map{} // Clear cache to force execution
	_, err = RunGcloudJsonCommand("test-format", "--format=yaml")
	c.Assert(err, IsNil)
	c.Assert(capturedArgs[len(capturedArgs)-1], Equals, "--format=yaml")
}

func (s *MySuite) TestRunGcloudJsonCommand_Error(c *C) {
	old := execCommand
	defer func() { execCommand = old }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "WANT_ERROR=1")
		return cmd
	}

	gcloudCache = sync.Map{}

	_, err := RunGcloudJsonCommand("fail-test")
	c.Assert(err, NotNil)
}
