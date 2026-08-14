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

package logging

import (
	"testing"
)

func TestExitWithCode_FatalHook(t *testing.T) {
	// Mock os.Exit
	originalExit := Exit
	defer func() { Exit = originalExit }()

	var exitedCode int
	Exit = func(code int) {
		exitedCode = code
	}

	// Mock FatalHook
	var hookExitCode int
	var hookErr error
	FatalHook = func(code int, err error) {
		hookExitCode = code
		hookErr = err
	}
	defer func() { FatalHook = nil }()

	ExitWithCode(1, "test error %s", "message")

	if exitedCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitedCode)
	}
	if hookExitCode != 1 {
		t.Errorf("expected hook exit code 1, got %d", hookExitCode)
	}
	if hookErr == nil {
		t.Fatal("expected hook error, got nil")
	}
	if hookErr.Error() != "test error message" {
		t.Errorf("expected hook error message 'test error message', got '%s'", hookErr.Error())
	}
}

func TestFatal(t *testing.T) {
	// Mock os.Exit
	originalExit := Exit
	defer func() { Exit = originalExit }()

	var exitedCode int
	Exit = func(code int) {
		exitedCode = code
	}

	// Mock FatalHook
	var hookErr error
	FatalHook = func(code int, err error) {
		hookErr = err
	}
	defer func() { FatalHook = nil }()

	Fatal("fatal error %d", 404)

	if exitedCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitedCode)
	}
	if hookErr == nil || hookErr.Error() != "fatal error 404" {
		t.Errorf("expected hook error message 'fatal error 404', got %v", hookErr)
	}
}
