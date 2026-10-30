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
	"context"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing/iotest"
	"time"

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

	func() {
		cleanup := makeDirReadOnly(c, dir)
		defer cleanup()
		if runtime.GOOS == "windows" || os.Geteuid() != 0 {
			c.Assert(CheckWritableDir(dir), NotNil)
		}
	}()

	os.RemoveAll(dir)
	c.Assert(CheckWritableDir(dir), NotNil)
}

func makeDirReadOnly(c *C, dir string) func() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("icacls", dir, "/deny", "*S-1-1-0:(W)")
		if err := cmd.Run(); err != nil {
			c.Fatal(err)
		}
		return func() {
			_ = exec.Command("icacls", dir, "/remove:d", "*S-1-1-0").Run()
		}
	}

	if os.Geteuid() != 0 {
		if err := os.Chmod(dir, 0555); err != nil {
			c.Fatal(err)
		}
		return func() {
			_ = os.Chmod(dir, 0700)
		}
	}

	return func() {}
}

func (s *MySuite) TestDirInfo(c *C) {
	isDir, isWritable := DirInfo("")
	c.Assert(isDir, Equals, false)
	c.Assert(isWritable, Equals, false)

	dir := c.MkDir()
	isDir, isWritable = DirInfo(dir)
	c.Assert(isDir, Equals, true)
	c.Assert(isWritable, Equals, true)

	// read-only directory
	func() {
		readOnlyDir := c.MkDir()
		cleanupRO := makeDirReadOnly(c, readOnlyDir)
		defer cleanupRO()
		if runtime.GOOS == "windows" || os.Geteuid() != 0 {
			isDir, isWritable = DirInfo(readOnlyDir)
			c.Assert(isDir, Equals, true)
			c.Assert(isWritable, Equals, false)
		}
	}()

	// regular file should report isDir=false, isWritable=false
	filePath := filepath.Join(dir, "testfile")
	err := os.WriteFile(filePath, []byte("content"), 0644)
	c.Assert(err, IsNil)
	isDir, isWritable = DirInfo(filePath)
	c.Assert(isDir, Equals, false)
	c.Assert(isWritable, Equals, false)

	// non-existent path
	isDir, isWritable = DirInfo(filepath.Join(dir, "nonexistent"))
	c.Assert(isDir, Equals, false)
	c.Assert(isWritable, Equals, false)
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
	groups := []config.Group{
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

func (s *MySuite) TestRandomString(c *C) {
	res, err := RandomString(10)
	c.Assert(err, IsNil)
	c.Assert(len(res), Equals, 10)

	res2, err := RandomString(10)
	c.Assert(err, IsNil)
	c.Assert(res, Not(Equals), res2)
}

func (s *MySuite) TestAskForConfirmation_Yes(c *C) {
	pipeRead, pipeWrite, err := os.Pipe()
	c.Assert(err, IsNil)
	defer pipeRead.Close()
	defer pipeWrite.Close()

	origStdin := os.Stdin
	os.Stdin = pipeRead
	defer func() { os.Stdin = origStdin }()

	if _, err := pipeWrite.Write([]byte("y\n")); err != nil {
		c.Fatal(err)
	}

	got := PromptYesNo("Test prompt")
	c.Assert(got, Equals, true)
}

func (s *MySuite) TestAskForConfirmation_No(c *C) {
	pipeRead, pipeWrite, err := os.Pipe()
	c.Assert(err, IsNil)
	defer pipeRead.Close()
	defer pipeWrite.Close()

	origStdin := os.Stdin
	os.Stdin = pipeRead
	defer func() { os.Stdin = origStdin }()

	if _, err := pipeWrite.Write([]byte("n\n")); err != nil {
		c.Fatal(err)
	}

	got := PromptYesNo("Test prompt")
	c.Assert(got, Equals, false)
}

// failOnExit makes logging.Exit panic so a regression to logging.Fatal fails the test
// instead of exiting the test binary (or looping, if Exit returned). Call the returned
// func to restore logging.Exit.
func failOnExit() (restore func()) {
	origExit := logging.Exit
	logging.Exit = func(code int) { panic(fmt.Sprintf("logging.Exit(%d) called", code)) }
	return func() { logging.Exit = origExit }
}

type zeroReader struct{}

func (zeroReader) Read([]byte) (int, error) { return 0, nil }

// transientZeroReader returns zerosBeforeByte (0, nil) reads before each byte in data, so
// readLine must both tolerate transient empty reads and reset its retry counter after a byte.
type transientZeroReader struct {
	data            string
	zerosBeforeByte int
	zerosRemaining  int
	hangAtEnd       bool
}

func (r *transientZeroReader) Read(p []byte) (int, error) {
	if r.data == "" {
		if r.hangAtEnd {
			return 0, nil
		}
		return 0, io.EOF
	}
	if r.zerosRemaining > 0 {
		r.zerosRemaining--
		return 0, nil
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	r.zerosRemaining = r.zerosBeforeByte
	return 1, nil
}

func (s *MySuite) TestReadLine(c *C) {
	errBoom := errors.New("boom")
	tests := []struct {
		name     string
		r        io.Reader
		wantLine string
		wantErr  error
	}{
		{"newline", strings.NewReader("y\n"), "y", nil},
		{"crlf keeps carriage return", strings.NewReader("y\r\n"), "y\r", nil},
		{"empty line", strings.NewReader("\n"), "", nil},
		{"eof without newline", strings.NewReader("y"), "y", io.EOF},
		{"eof without data", strings.NewReader(""), "", io.EOF},
		{"one byte reader", iotest.OneByteReader(strings.NewReader("yes\n")), "yes", nil},
		{"data with eof", iotest.DataErrReader(strings.NewReader("y")), "y", io.EOF},
		{"newline with eof", iotest.DataErrReader(strings.NewReader("y\n")), "y", nil},
		{"read error", iotest.ErrReader(errBoom), "", errBoom},
		{"data then read error", io.MultiReader(strings.NewReader("y"), iotest.ErrReader(errBoom)), "y", errBoom},
		{"transient zero reads reset after progress", &transientZeroReader{data: "y\n", zerosBeforeByte: 99, zerosRemaining: 99}, "y", nil},
		{"data then no progress", &transientZeroReader{data: "y", zerosBeforeByte: 5, zerosRemaining: 5, hangAtEnd: true}, "y", io.ErrNoProgress},
		{"no progress", zeroReader{}, "", io.ErrNoProgress},
	}
	for _, tc := range tests {
		line, err := readLine(tc.r)
		c.Check(line, Equals, tc.wantLine, Commentf(tc.name))
		c.Check(err, Equals, tc.wantErr, Commentf(tc.name))
	}
}

func (s *MySuite) TestReadLine_LeavesRemainingInputUnread(c *C) {
	r := strings.NewReader("n\ny\n")
	first, err := readLine(r)
	c.Assert(err, IsNil)
	c.Check(first, Equals, "n")
	second, err := readLine(r)
	c.Assert(err, IsNil)
	c.Check(second, Equals, "y")
}

func (s *MySuite) TestPromptYesNo_Answers(c *C) {
	defer failOnExit()()
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"No\ny\n", false},
		{"\n", true},
		{"y\r\n", true},
		{"\r\n", true},
		{"\uFEFFn\r\ny\r\n", false},
		{"\uFEFFy\n", true},
		{" n \r\n", false},
		{"y", true},
		{"no", false},
		{"", false},
		{"maybe", false},
		{"maybe\n", false},
		{"maybe\ny\n", true},
		{"maybe\n\n", true},
	}
	for _, tc := range tests {
		got := promptYesNo(strings.NewReader(tc.input), "Test prompt")
		c.Check(got, Equals, tc.want, Commentf("input %q", tc.input))
	}
}

func (s *MySuite) TestPromptYesNo_ReadErrorsDecline(c *C) {
	defer failOnExit()()
	errBoom := errors.New("boom")
	c.Check(promptYesNo(iotest.ErrReader(errBoom), "Test prompt"), Equals, false)
	// A partial answer is honored only when input ends with EOF, not on other read errors.
	c.Check(promptYesNo(io.MultiReader(strings.NewReader("y"), iotest.ErrReader(errBoom)), "Test prompt"), Equals, false)
	c.Check(promptYesNo(zeroReader{}, "Test prompt"), Equals, false)
}

func (s *MySuite) TestPromptYesNo_SuccessivePromptsReadOneLineEach(c *C) {
	defer failOnExit()()
	for _, input := range []string{"n\ny\n", "n\r\ny\r\n"} {
		r := strings.NewReader(input)
		c.Check(promptYesNo(r, "First"), Equals, false, Commentf("input %q", input))
		c.Check(promptYesNo(r, "Second"), Equals, true, Commentf("input %q", input))
	}
}

func (s *MySuite) TestAskForConfirmation_ClosedStdinDeclines(c *C) {
	defer failOnExit()()
	pipeRead, pipeWrite, err := os.Pipe()
	c.Assert(err, IsNil)
	defer func() { _ = pipeRead.Close() }()
	c.Assert(pipeWrite.Close(), IsNil)

	origStdin := os.Stdin
	os.Stdin = pipeRead
	defer func() { os.Stdin = origStdin }()

	c.Assert(PromptYesNo("Test prompt"), Equals, false)
}

func (s *MySuite) TestAskForConfirmation_UnreadableStdinDeclines(c *C) {
	defer failOnExit()()
	pipeRead, pipeWrite, err := os.Pipe()
	c.Assert(err, IsNil)
	defer func() { _ = pipeWrite.Close() }()
	c.Assert(pipeRead.Close(), IsNil)

	origStdin := os.Stdin
	os.Stdin = pipeRead
	defer func() { os.Stdin = origStdin }()

	c.Assert(PromptYesNo("Test prompt"), Equals, false)
}

func (s *MySuite) TestAskForConfirmation_PipedAnswersSuccessivePrompts(c *C) {
	defer failOnExit()()
	pipeRead, pipeWrite, err := os.Pipe()
	c.Assert(err, IsNil)
	defer func() { _ = pipeRead.Close() }()
	_, err = pipeWrite.Write([]byte("n\ny\n"))
	c.Assert(err, IsNil)
	c.Assert(pipeWrite.Close(), IsNil)

	origStdin := os.Stdin
	os.Stdin = pipeRead
	defer func() { os.Stdin = origStdin }()

	c.Check(PromptYesNo("First"), Equals, false)
	c.Check(PromptYesNo("Second"), Equals, true)
}

func (s *MySuite) TestExecuteCommandWithTimeout_SuccessfulExecution(c *C) {
	timeout := 2 * time.Second

	var cmdName string
	var cmdArgs []string
	if runtime.GOOS == "windows" {
		cmdName = "cmd"
		cmdArgs = []string{"/c", "echo hello world"}
	} else {
		cmdName = "echo"
		cmdArgs = []string{"hello world"}
	}

	result := ExecuteCommandWithTimeout(timeout, cmdName, cmdArgs...)

	c.Assert(result.Err, IsNil)
	c.Assert(result.ExitCode, Equals, 0)
	c.Assert(strings.TrimSpace(result.Stdout), Equals, "hello world")
}

func (s *MySuite) TestExecuteCommandWithTimeout_KillsHangingProcess(c *C) {
	timeout := 100 * time.Millisecond

	var cmdName string
	var cmdArgs []string
	if runtime.GOOS == "windows" {
		cmdName = "ping"
		cmdArgs = []string{"127.0.0.1", "-n", "6"}
	} else {
		cmdName = "sleep"
		cmdArgs = []string{"5"}
	}

	result := ExecuteCommandWithTimeout(timeout, cmdName, cmdArgs...)

	c.Assert(result.Err, NotNil)
	c.Assert(errors.Is(result.Err, context.DeadlineExceeded), Equals, true)
	c.Assert(result.ExitCode, Equals, 124)
}

func (s *MySuite) TestExecuteCommandWithTimeout_NonZeroExitCode(c *C) {
	timeout := 2 * time.Second

	var cmdName string
	var cmdArgs []string
	if runtime.GOOS == "windows" {
		cmdName = "cmd"
		cmdArgs = []string{"/c", "exit 42"}
	} else {
		cmdName = "sh"
		cmdArgs = []string{"-c", "exit 42"}
	}

	result := ExecuteCommandWithTimeout(timeout, cmdName, cmdArgs...)
	c.Assert(result.Err, NotNil)
	c.Assert(result.ExitCode, Equals, 42)
}

func (s *MySuite) TestExecuteCommandWithTimeout_BinaryNotFound(c *C) {
	timeout := 2 * time.Second
	result := ExecuteCommandWithTimeout(timeout, "nonexistent-binary-xyz-12345")
	c.Assert(result.Err, NotNil)
	c.Assert(result.ExitCode, Equals, -1)
}

func (s *MySuite) TestHandleExecError(c *C) {
	timeoutMsg := "command timed out after 30s"

	okRes := CommandResult{ExitCode: 0, Stdout: "ok", Err: nil}
	c.Assert(HandleExecError(okRes, "gcloud", timeoutMsg), IsNil)

	timeoutRes := CommandResult{ExitCode: 124, Err: context.DeadlineExceeded}
	err := HandleExecError(timeoutRes, "gcloud", timeoutMsg)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, timeoutMsg+": context deadline exceeded")
	c.Assert(errors.Is(err, context.DeadlineExceeded), Equals, true)

	startErr := errors.New("executable file not found in $PATH")
	notFoundRes := CommandResult{ExitCode: -1, Err: startErr}
	err = HandleExecError(notFoundRes, "gcloud", timeoutMsg)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "failed to execute gcloud: executable file not found in \\$PATH")
	c.Assert(errors.Is(err, startErr), Equals, true)

	exitErrRes := CommandResult{ExitCode: 42, Stderr: "some cli error", Err: errors.New("exit status 42")}
	c.Assert(HandleExecError(exitErrRes, "gcloud", timeoutMsg), IsNil)
}
