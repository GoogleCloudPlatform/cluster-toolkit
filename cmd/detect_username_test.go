// Copyright 2026 Google LLC
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
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeGcloud puts a `gcloud` shell script with the given body first on PATH.
func fakeGcloud(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake gcloud script requires a POSIX shell")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "gcloud")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestDetectUsername(t *testing.T) {
	ctx := t.Context()

	t.Run("CLOUDSDK_CORE_ACCOUNT wins", func(t *testing.T) {
		fakeGcloud(t, "echo gcloud@google.com")
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "env@google.com")
		if got := detectUsername(ctx); got != "env@google.com" {
			t.Errorf("got %q, want env@google.com", got)
		}
	})

	t.Run("active gcloud account", func(t *testing.T) {
		fakeGcloud(t, "echo '  alice@google.com  '")
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		if got := detectUsername(ctx); got != "alice@google.com" {
			t.Errorf("got %q, want alice@google.com", got)
		}
	})

	t.Run("empty gcloud output falls back to USER", func(t *testing.T) {
		fakeGcloud(t, "exit 0")
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("USER", "localuser")
		if got := detectUsername(ctx); got != "localuser" {
			t.Errorf("got %q, want localuser", got)
		}
	})

	t.Run("failing gcloud falls back to USERNAME", func(t *testing.T) {
		fakeGcloud(t, "exit 1")
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("USER", "")
		t.Setenv("USERNAME", "winuser")
		if got := detectUsername(ctx); got != "winuser" {
			t.Errorf("got %q, want winuser", got)
		}
	})

	t.Run("no gcloud and no env falls back to the OS user", func(t *testing.T) {
		fakeGcloud(t, "exit 1")
		t.Setenv("CLOUDSDK_CORE_ACCOUNT", "")
		t.Setenv("USER", "")
		t.Setenv("USERNAME", "")
		if got := detectUsername(ctx); got == "" {
			t.Error("got empty username")
		}
	})
}
