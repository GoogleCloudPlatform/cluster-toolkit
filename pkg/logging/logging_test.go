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
