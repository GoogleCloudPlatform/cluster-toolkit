/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package shell

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultLocalCommandTimeout = 15 * time.Second // For local checks: gcloud version, config reads, kubectl --client
	DefaultCloudAPITimeout     = 30 * time.Second // For remote GCP APIs: clusters describe, projects describe
)

// ProposedChanges provides summary and full description of proposed changes
// to cloud infrastructure
type ProposedChanges struct {
	Summary string
	Full    string
}

// CommandResult holds the output and exit code of an executed command.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// Command represents a shell command that can be executed.
type Command struct {
	cmd    *exec.Cmd
	stdin  bytes.Buffer
	stdout bytes.Buffer
	stderr bytes.Buffer
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCommand creates a new Command instance.
func NewCommand(name string, args ...string) *Command {
	cmd := exec.Command(name, args...)
	return &Command{cmd: cmd}
}

// NewCommandWithTimeout creates a new Command instance with a timeout.
func NewCommandWithTimeout(timeout time.Duration, name string, args ...string) *Command {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := exec.CommandContext(ctx, name, args...)
	// Forcefully close I/O pipes if orphaned child processes keep them open after timeout.
	// This unblocks cmd.Run() reliably on both Windows (TerminateProcess) and Unix (SIGKILL).
	cmd.WaitDelay = 100 * time.Millisecond
	return &Command{cmd: cmd, ctx: ctx, cancel: cancel}
}

// SetInput sets the standard input for the command.
func (c *Command) SetInput(input string) {
	c.stdin.WriteString(input)
	c.cmd.Stdin = &c.stdin
}

// Execute runs the command and returns a CommandResult.
func (c *Command) Execute() CommandResult {
	if c.cancel != nil {
		defer c.cancel()
	}
	c.cmd.Stdout = &c.stdout
	c.cmd.Stderr = &c.stderr

	err := c.cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		// The process exited successfully; only a child kept stdout/stderr open.
		err = nil
	} else if err != nil && c.ctx != nil && c.ctx.Err() == context.DeadlineExceeded {
		err = context.DeadlineExceeded
	}
	result := CommandResult{
		Stdout: c.stdout.String(),
		Stderr: c.stderr.String(),
		Err:    err,
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			result.ExitCode = 124
		} else if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			result.ExitCode = -1
		}
	} else {
		result.ExitCode = 0
	}
	return result
}

// ExecuteCommand executes a shell command and returns its output and exit code.
// It takes the command name as the first argument, followed by its arguments.
var ExecuteCommand = func(name string, args ...string) CommandResult {
	cmd := NewCommand(name, args...)
	return cmd.Execute()
}

// RandomString generates a random string of a given length.
func RandomString(length int) (string, error) {
	b := make([]byte, (length+1)/2)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random string: %w", err)
	}
	return fmt.Sprintf("%x", b)[:length], nil
}

// ValidateDeploymentDirectory ensures that the deployment directory structure
// appears valid given a mapping of group names to module kinds
// TODO: verify kind fully by auto-detecting type from group directory
func ValidateDeploymentDirectory(groups []config.Group, deploymentRoot string) error {
	for _, group := range groups {
		groupPath := filepath.Join(deploymentRoot, string(group.Name))
		if isDir, _ := DirInfo(groupPath); !isDir {
			return fmt.Errorf("improper deployment: %s is not a directory for group %s", groupPath, group.Name)
		}
	}
	return nil
}

func intersectMapKeys[K comparable, T any](s []K, m map[K]T) map[K]T {
	intersection := make(map[K]T)
	for _, e := range s {
		if val, ok := m[e]; ok {
			intersection[e] = val
		}
	}
	return intersection
}

func mergeMapsWithoutLoss[K comparable, V any](to map[K]V, from map[K]V) error {
	for k, v := range from {
		if _, ok := to[k]; ok {
			return fmt.Errorf("duplicate key %v", k)
		}
		to[k] = v
	}
	return nil
}

// DirInfo reports if path is a directory and new files can be written in it
func DirInfo(path string) (isDir bool, isWritable bool) {
	p, err := os.Stat(path)
	if err != nil {
		return false, false
	}

	isDir = p.Mode().IsDir()
	isWritable = isDir && isDirWritable(path)

	return isDir, isWritable
}

// CheckWritableDir errors unless path is a directory we can write to
func CheckWritableDir(path string) error {
	if path == "" {
		return nil
	}
	if isDir, isWritable := DirInfo(path); !isDir || !isWritable {
		return fmt.Errorf("%s must be a writable directory", path)
	}
	return nil
}

// ApplyChangesChoice prompts the user to decide whether they want to approve
// changes to cloud configuration, to stop execution of gcluster entirely, or to
// skip making the proposed changes and continue execution (in deploy command)
// only if the user responds with "y" or "yes" (case-insensitive)
func ApplyChangesChoice(c ProposedChanges) bool {
	logging.Info("Summary of proposed changes: %s", strings.TrimSpace(c.Summary))
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(`(D)isplay full proposed changes,
(A)pply proposed changes,
(S)top and exit,
(C)ontinue without applying
Please select an option [d,a,s,c]: `)

		in, err := reader.ReadString('\n')
		if err != nil {
			logging.Fatal("%v", err)
		}

		switch strings.ToLower(strings.TrimSpace(in)) {
		case "a":
			return true
		case "c":
			return false
		case "d":
			fmt.Println(c.Full)
		case "s":
			logging.ExitWithCode(0, "user chose to stop execution of gcluster rather than make proposed changes to infrastructure")
		}
	}
}

// PromptYesNo prompts the user with a yes/no question and reads one line from standard input.
// It returns true for 'y', 'yes' or an empty line (Enter) and false for 'n' or 'no'. It also
// returns false, without exiting, if standard input ends or fails before a valid answer.
var PromptYesNo = func(prompt string) bool {
	return promptYesNo(os.Stdin, prompt)
}

func promptYesNo(in io.Reader, prompt string) bool {
	for {
		fmt.Printf("%s [Y/n]: ", prompt)
		line, err := readLine(in)
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Println()
			logging.Warn("Could not read standard input (%v); answering 'no'.", err)
			return false
		}
		// Windows PowerShell can prepend a UTF-8 byte-order mark, which TrimSpace keeps.
		switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))) {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		case "":
			if err == nil {
				return true
			}
		}
		if err != nil {
			fmt.Println()
			logging.Warn("No answer on standard input (it is closed or ended); answering 'no'.")
			return false
		}
		fmt.Println("Invalid input. Please enter 'Y' or 'n'.")
	}
}

// readLine reads from r up to and including the next '\n' and returns the line without it.
// It reads one byte at a time so input after the line stays unread for later prompts. The
// error is nil only if a '\n' was read.
func readLine(r io.Reader) (string, error) {
	var line []byte
	b := make([]byte, 1)
	for i := 0; i < 100; {
		n, err := r.Read(b)
		if n == 1 {
			i = 0 // reset retry counter on successful read
			if b[0] == '\n' {
				return string(line), nil
			}
			line = append(line, b[0])
		}
		if err != nil {
			return string(line), err
		}
		if n == 0 {
			i++
		}
	}
	return string(line), io.ErrNoProgress
}

// ExtractRegion extracts the region from a location (region or zone).
func ExtractRegion(location string) string {
	parts := strings.Split(location, "-")
	if len(parts) == 3 {
		// likely a zone, return region (first two parts)
		return parts[0] + "-" + parts[1]
	}
	return location
}

// ExecuteCommandWithTimeout executes a shell command but forcibly kills the
// process if it does not complete within the provided timeout duration.
var ExecuteCommandWithTimeout = func(timeout time.Duration, name string, args ...string) CommandResult {
	return NewCommandWithTimeout(timeout, name, args...).Execute()
}

// HandleExecError checks if a CommandResult failed due to a timeout or failed to start.
func HandleExecError(result CommandResult, cmdName string, timeoutMsg string) error {
	if result.Err != nil {
		if errors.Is(result.Err, context.DeadlineExceeded) {
			return fmt.Errorf("%s: %w", timeoutMsg, result.Err)
		}
		if result.ExitCode == -1 {
			return fmt.Errorf("failed to execute %s: %w", cmdName, result.Err)
		}
	}
	return nil
}
