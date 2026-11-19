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
	"strings"
	"sync"
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

func TestCaptureForTesting_RaceSafe(t *testing.T) {
	snapshot, restore := CaptureForTesting(t)
	defer restore()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			Info("info-%d", id)
			Warn("warn-%d", id)
			Error("err-%d", id)
			_, _ = snapshot()
		}(i)
	}
	wg.Wait()

	infoOut, errOut := snapshot()
	if !strings.Contains(infoOut, "info-0") {
		t.Errorf("CaptureForTesting() infoOut = %q, want substring %q", infoOut, "info-0")
	}
	if !strings.Contains(errOut, "WARNING: warn-0") || !strings.Contains(errOut, "err-0") {
		t.Errorf("CaptureForTesting() errOut = %q, want warn-0 and err-0", errOut)
	}
	restore()
}

type fakeCleanupTB struct {
	cleanups []func()
}

func (f *fakeCleanupTB) Cleanup(fn func()) {
	f.cleanups = append(f.cleanups, fn)
}

func TestCaptureForTesting_CleanupAutoRestores(t *testing.T) {
	tb := &fakeCleanupTB{}
	snap1, _ := CaptureForTesting(tb)
	Info("first-capture")
	if info1, _ := snap1(); !strings.Contains(info1, "first-capture") {
		t.Fatalf("first capture = %q, want first-capture", info1)
	}
	if len(tb.cleanups) != 1 {
		t.Fatalf("registered cleanups = %d, want 1", len(tb.cleanups))
	}
	// Simulate test teardown without an explicit restore() call; subsequent CaptureForTesting must not deadlock.
	tb.cleanups[0]()

	snap2, restore2 := CaptureForTesting(t)
	defer restore2()
	Info("second-capture")
	if info2, _ := snap2(); !strings.Contains(info2, "second-capture") {
		t.Fatalf("second capture = %q, want second-capture", info2)
	}
}
