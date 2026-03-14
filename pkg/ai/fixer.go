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

package ai

import (
	"fmt"
	"hpc-toolkit/pkg/logging"
	"os"
	"os/exec"
)

type FixerOptions struct {
	MaxRetries int
	Verbose    bool
	Region     string
	Model      string
}

type Fixer struct {
	options FixerOptions
	client  *Client
}

func NewFixer(options FixerOptions) *Fixer {
	return &Fixer{
		options: options,
		client:  NewClient(options.Verbose, options.Region, options.Model),
	}
}

func (f *Fixer) Run(files []string) error {
	if _, err := exec.LookPath("pre-commit"); err != nil {
		return fmt.Errorf("pre-commit is not installed or not in PATH. Please install it first: https://pre-commit.com")
	}

	for i := 0; i < f.options.MaxRetries; i++ {
		logging.Info("[Attempt %d/%d] Running pre-commit hooks...", i+1, f.options.MaxRetries)
		failures, modified, err := f.runPreCommit(files)
		if err == nil {
			logging.Info("All pre-commit hooks passed!")
			return nil
		}

		if len(failures) == 0 {
			if modified {
				logging.Info("Some hooks modified files. Retrying...")
				continue
			}
			return fmt.Errorf("pre-commit failed with unknown errors: %w", err)
		}

		logging.Info("Found %d failures. Attempting to fix...", len(failures))
		fixedCount := 0
		for _, failure := range failures {
			logging.Info("Fixing %s...", failure.File)
			if err := f.fixFailure(failure); err != nil {
				logging.Error("Failed to fix %s: %v", failure.File, err)
			} else {
				fixedCount++
			}
		}

		if fixedCount == 0 {
			return fmt.Errorf("could not fix any failures, stopping loop")
		}
	}

	return fmt.Errorf("max retries reached, some hooks still failing")
}

func (f *Fixer) runPreCommit(files []string) ([]Failure, bool, error) {
	args := []string{"run"}
	if len(files) > 0 {
		args = append(args, "--files")
		args = append(args, files...)
	} else {
		args = append(args, "--all-files")
	}

	cmd := exec.Command("pre-commit", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	if f.options.Verbose {
		logging.Info("Pre-commit output:\n%s", string(output))
	}

	if err == nil {
		return nil, false, nil
	}

	failures, modified := ParseFailures(string(output))
	return failures, modified, err
}

func (f *Fixer) fixFailure(failure Failure) error {
	content, err := os.ReadFile(failure.File)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fixedContent, err := f.client.GenerateFix(string(content), failure)
	if err != nil {
		return fmt.Errorf("AI generation failed: %w", err)
	}

	if err := os.WriteFile(failure.File, []byte(fixedContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
