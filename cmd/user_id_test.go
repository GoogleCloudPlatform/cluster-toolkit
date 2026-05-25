// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUserIdCmd(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantErr        bool
		expectedOutput string
	}{
		{
			name:           "success_with_no_arguments",
			args:           []string{},
			wantErr:        false,
			expectedOutput: "User ID:",
		},
		{
			name:    "failure_with_unexpected_arguments",
			args:    []string{"extra-arg"},
			wantErr: true,
		},
		{
			name:    "failure_with_multiple_unexpected_arguments",
			args:    []string{"arg1", "arg2"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// CRITICAL FIX: Create a fresh command shell for each test case.
			// This prevents Cobra from caching arguments between test runs.
			testCmd := &cobra.Command{
				Use:  userIdCmd.Use,
				Args: userIdCmd.Args,
				RunE: userIdCmd.RunE,
			}

			outBuf := new(bytes.Buffer)
			testCmd.SetOut(outBuf)
			testCmd.SetErr(outBuf)
			testCmd.SetArgs(tt.args)

			err := testCmd.Execute()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				output := outBuf.String()
				if !strings.Contains(output, tt.expectedOutput) {
					t.Errorf("Expected output to contain %q, but got %q", tt.expectedOutput, output)
				}
			}
		})
	}
}
