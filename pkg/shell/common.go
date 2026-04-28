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
	"crypto/rand"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
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
}

// Command represents a shell command that can be executed.
type Command struct {
	cmd    *exec.Cmd
	stdin  bytes.Buffer
	stdout bytes.Buffer
	stderr bytes.Buffer
}

// NewCommand creates a new Command instance.
func NewCommand(name string, args ...string) *Command {
	cmd := exec.Command(name, args...)
	return &Command{cmd: cmd}
}

// SetInput sets the standard input for the command.
func (c *Command) SetInput(input string) {
	c.stdin.WriteString(input)
	c.cmd.Stdin = &c.stdin
}

// Execute runs the command and returns a CommandResult.
func (c *Command) Execute() CommandResult {
	c.cmd.Stdout = &c.stdout
	c.cmd.Stderr = &c.stderr

	err := c.cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return CommandResult{
				Stdout:   c.stdout.String(),
				Stderr:   c.stderr.String(),
				ExitCode: exitError.ExitCode(),
			}
		}
		// If it's not an ExitError, it's some other error during command execution
		return CommandResult{
			Stdout:   c.stdout.String(),
			Stderr:   c.stderr.String(),
			ExitCode: 1, // Generic error code
		}
	}
	return CommandResult{
		Stdout:   c.stdout.String(),
		Stderr:   c.stderr.String(),
		ExitCode: 0,
	}
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
	p, err := os.Lstat(path)
	if err != nil {
		return false, false
	}

	isDir = p.Mode().IsDir()
	isWritable = unix.Access(path, unix.W_OK|unix.R_OK|unix.X_OK) == nil

	return isDir, isWritable
}

// CheckWritableDir errors unless path is a directory we can write to
func CheckWritableDir(path string) error {
	if path == "" {
		return nil
	}
	if isDir, isWritable := DirInfo(path); !(isDir && isWritable) {
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
			logging.Fatal("user chose to stop execution of gcluster rather than make proposed changes to infrastructure")
		}
	}
}

// PromptYesNo prompts the user with a yes/no question.
// It returns true if the user answers 'y' or 'yes' or just presses Enter.
var PromptYesNo = func(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [Y/n]: ", prompt)
		response, err := reader.ReadString('\n')
		if err != nil {
			logging.Fatal("%v", err)
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response == "" || response == "y" || response == "yes" {
			return true
		}
		if response == "n" || response == "no" {
			return false
		}
		fmt.Println("Invalid input. Please enter 'Y' or 'n'.")
	}
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
