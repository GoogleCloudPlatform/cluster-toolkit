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
	"errors"
	"hpc-toolkit/pkg/config"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withStdin replaces os.Stdin with a file containing input for the duration
// of the test.
func withStdin(t *testing.T, input string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = old
		_ = f.Close()
	})
}

func TestConfirmAction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"yes", "y\n", true},
		{"upper-case yes with spaces", "  Y  \n", true},
		{"no", "n\n", false},
		{"invalid then yes", "maybe\ny\n", true},
		{"invalid then no", "\nN\n", false},
		{"EOF defaults to no", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withStdin(t, tc.input)
			if got := confirmAction("continue? "); got != tc.want {
				t.Errorf("confirmAction(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestDestroyChoice(t *testing.T) {
	oldAutoApprove := flagAutoApprove
	t.Cleanup(func() { flagAutoApprove = oldAutoApprove })

	t.Run("auto-approve", func(t *testing.T) {
		flagAutoApprove = true
		if !destroyChoice("primary") {
			t.Error("destroyChoice() = false with --auto-approve, want true")
		}
	})

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"prompt yes", "y\n", true},
		{"prompt no", "n\n", false},
		{"prompt invalid then yes", "later\nY\n", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flagAutoApprove = false
			withStdin(t, tc.input)
			if got := destroyChoice("primary"); got != tc.want {
				t.Errorf("destroyChoice() with input %q = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestHasSelectedGroupOfKind(t *testing.T) {
	oldOnly, oldSkip := flagOnlyGroups, flagSkipGroups
	t.Cleanup(func() { flagOnlyGroups, flagSkipGroups = oldOnly, oldSkip })

	bp := config.Blueprint{Groups: []config.Group{
		{Name: "tf", Modules: []config.Module{{Kind: config.TerraformKind}}},
		{Name: "img", Modules: []config.Module{{Kind: config.PackerKind}}},
	}}

	flagOnlyGroups, flagSkipGroups = nil, nil
	if !hasSelectedGroupOfKind(bp, config.TerraformKind) || !hasSelectedGroupOfKind(bp, config.PackerKind) {
		t.Error("expected both kinds to be selected when no group filter is set")
	}

	flagOnlyGroups = []string{"tf"}
	if !hasSelectedGroupOfKind(bp, config.TerraformKind) {
		t.Error("expected terraform kind to be selected with --only tf")
	}
	if hasSelectedGroupOfKind(bp, config.PackerKind) {
		t.Error("expected packer kind not to be selected with --only tf")
	}
}

func TestValidateRuntimeDependencies(t *testing.T) {
	oldOnly, oldSkip := flagOnlyGroups, flagSkipGroups
	t.Cleanup(func() { flagOnlyGroups, flagSkipGroups = oldOnly, oldSkip })

	groups := []config.Group{{Name: "empty"}} // no modules -> unknown kind

	flagOnlyGroups, flagSkipGroups = nil, []string{"empty"}
	if err := validateRuntimeDependencies(t.TempDir(), groups); err != nil {
		t.Errorf("expected skipped group to be ignored, got %v", err)
	}

	flagSkipGroups = nil
	if err := validateRuntimeDependencies(t.TempDir(), groups); err == nil {
		t.Error("expected an error for a group of unknown kind")
	}
}

func TestFirstBlueprintInDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.yaml"), 0755); err != nil {
		t.Fatal(err)
	}
	write("notes.txt", "not yaml")
	write("a-invalid.yaml", "::: not a blueprint")
	write("b-no-groups.yaml", "blueprint_name: empty\nvars: {}\n")
	skipped := write("c-skipped.yaml", "blueprint_name: skipped\ndeployment_groups:\n- group: primary\n  modules: []\n")
	want := write("d-valid.yaml", "blueprint_name: valid\ndeployment_groups:\n- group: primary\n  modules: []\n")

	if got := firstBlueprintInDir(dir, filepath.Base(skipped)); got != want {
		t.Errorf("firstBlueprintInDir() = %q, want %q", got, want)
	}
	if got := firstBlueprintInDir(dir, ""); got != skipped {
		t.Errorf("firstBlueprintInDir() without skip = %q, want %q", got, skipped)
	}
	if got := firstBlueprintInDir(filepath.Join(dir, "missing"), ""); got != "" {
		t.Errorf("firstBlueprintInDir() on a missing dir = %q, want empty", got)
	}
}

func TestRenderMultiError(t *testing.T) {
	errs := (&config.Errors{}).Add(errors.New("first problem")).Add(errors.New("second problem"))
	got := renderError(*errs, config.YamlCtx{})
	for _, want := range []string{"first problem", "second problem"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderError() = %q, want it to contain %q", got, want)
		}
	}
}

func TestExecPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX executable files")
	}
	oldArgs0 := os.Args[0]
	t.Cleanup(func() { os.Args[0] = oldArgs0 })

	newExe := func(t *testing.T, dir string) string {
		t.Helper()
		p := filepath.Join(dir, "gcluster")
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("plain name", func(t *testing.T) {
		os.Args[0] = "gcluster"
		if got := execPath(); got != "gcluster" {
			t.Errorf("execPath() = %q, want gcluster", got)
		}
	})

	t.Run("not on PATH returns the given path", func(t *testing.T) {
		exe := newExe(t, t.TempDir())
		t.Setenv("PATH", t.TempDir())
		os.Args[0] = exe
		if got := execPath(); got != exe && !strings.HasSuffix(exe, strings.TrimPrefix(got, ".")) {
			t.Errorf("execPath() = %q, want a path to %q", got, exe)
		}
	})

	t.Run("same binary on PATH returns gcluster", func(t *testing.T) {
		dir := t.TempDir()
		exe := newExe(t, dir)
		t.Setenv("PATH", dir)
		os.Args[0] = exe
		if got := execPath(); got != "gcluster" {
			t.Errorf("execPath() = %q, want gcluster", got)
		}
	})

	t.Run("different binary on PATH returns the given path", func(t *testing.T) {
		onPath := t.TempDir()
		newExe(t, onPath)
		exe := newExe(t, t.TempDir())
		t.Setenv("PATH", onPath)
		os.Args[0] = exe
		if got := execPath(); got == "gcluster" {
			t.Errorf("execPath() = %q, want a path to %q", got, exe)
		}
	})

	t.Run("missing binary with gcluster on PATH returns gcluster", func(t *testing.T) {
		onPath := t.TempDir()
		newExe(t, onPath)
		t.Setenv("PATH", onPath)
		os.Args[0] = filepath.Join(t.TempDir(), "does-not-exist")
		if got := execPath(); got != "gcluster" {
			t.Errorf("execPath() = %q, want gcluster", got)
		}
	})
}
