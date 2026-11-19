// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
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
	"strings"
	"testing"
	"time"
)

func withSubblockPrompt(t *testing.T, terminal bool, input string, autoApprove bool) {
	t.Helper()
	origIn, origTerm, origAuto := subblockStdin, subblockIsTerminal, flagAutoApprove
	subblockStdin = strings.NewReader(input)
	subblockIsTerminal = func() bool { return terminal }
	flagAutoApprove = autoApprove
	t.Cleanup(func() { subblockStdin, subblockIsTerminal, flagAutoApprove = origIn, origTerm, origAuto })
}

func TestConfirmSubblocks(t *testing.T) {
	for _, tc := range []struct {
		name        string
		terminal    bool
		input       string
		autoApprove bool
		want        bool
		wantErr     bool
	}{
		{"auto-approve", false, "", true, true, false},
		{"yes", true, "y\n", false, true, false},
		{"YES", true, "YES\n", false, true, false},
		{"no", true, "n\n", false, false, false},
		{"empty means no", true, "\n", false, false, false},
		{"no terminal", false, "y\n", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSubblockPrompt(t, tc.terminal, tc.input, tc.autoApprove)
			got, err := confirmSubblocks("Save?")
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("got %v, %v; want %v, error=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestSaveSubblockMap(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "deployment.yaml")
	text := "# my deployment\nvars:\n  project_id: p  # keep\n  reservation_subblocks: {}\n  zone: z\n"
	if err := os.WriteFile(file, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	orig := subblockNow
	subblockNow = func() time.Time { return time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { subblockNow = orig })

	m := map[string]string{"01-0002": "my-res/reservationBlocks/b/reservationSubBlocks/b-subblock-0002"}
	if err := saveSubblockMap(file, "reservation_subblocks", m); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	want := "# my deployment\nvars:\n  project_id: p  # keep\n  reservation_subblocks:\n    \"01-0002\": my-res/reservationBlocks/b/reservationSubBlocks/b-subblock-0002\n  zone: z\n"
	if string(got) != want {
		t.Errorf("file:\n%s\nwant:\n%s", got, want)
	}
	backup, err := os.ReadFile(file + ".bak-20261024-100000")
	if err != nil || string(backup) != text {
		t.Errorf("backup = %q, %v", backup, err)
	}
}

func TestSaveSubblockMapNeedsVars(t *testing.T) {
	file := filepath.Join(t.TempDir(), "deployment.yaml")
	if err := os.WriteFile(file, []byte("terraform_backend_defaults: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := saveSubblockMap(file, "reservation_subblocks", map[string]string{"a": "b"}); err == nil {
		t.Error("a file without vars: must be an error")
	}
}
