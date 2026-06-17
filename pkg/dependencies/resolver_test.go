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

package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchPath(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	err := PatchPath()
	if err != nil {
		t.Fatalf("PatchPath() failed: %v", err)
	}

	newPath := os.Getenv("PATH")

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir() failed: %v", err)
	}

	expectedTfPath := filepath.Join(cacheDir, "cluster-toolkit", fmt.Sprintf("terraform-%s", TerraformVersion))
	expectedPackerPath := filepath.Join(cacheDir, "cluster-toolkit", fmt.Sprintf("packer-%s", PackerVersion))

	if !strings.Contains(newPath, expectedTfPath) {
		t.Errorf("Expected PATH to contain %s, got %s", expectedTfPath, newPath)
	}
	if !strings.Contains(newPath, expectedPackerPath) {
		t.Errorf("Expected PATH to contain %s, got %s", expectedPackerPath, newPath)
	}
	if !strings.HasSuffix(newPath, oldPath) {
		t.Errorf("Expected new PATH to end with old PATH")
	}
}

func TestEnsureBinary_MissingAndDecisionNo(t *testing.T) {
	binaryName := "fake-binary-that-does-not-exist"

	err := ensureBinary(binaryName, "1.0.0", DownloadDecisionNo)
	if err == nil {
		t.Fatalf("Expected error when binary is missing and decision is No")
	}
	expectedErrMsg := fmt.Sprintf("%s is missing. Download is explicitly disabled. Enable download by specifying --download-dependencies flag.", binaryName)
	if err.Error() != expectedErrMsg {
		t.Errorf("Expected error %q, got %q", expectedErrMsg, err.Error())
	}
}

func TestConfirmDownload_Ask_Yes(t *testing.T) {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	_, _ = w.Write([]byte("yes\n"))
	w.Close()

	err := confirmDownload("testbin", "1.0.0", DownloadDecisionAsk)
	if err != nil {
		t.Fatalf("expected no error for Ask(yes), got %v", err)
	}
}

func TestConfirmDownload_Ask_No(t *testing.T) {
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	_, _ = w.Write([]byte("no\n"))
	w.Close()

	err := confirmDownload("testbin", "1.0.0", DownloadDecisionAsk)
	if err == nil {
		t.Fatalf("expected error for Ask(no)")
	}
}

func TestConfirmDownload_Yes(t *testing.T) {
	err := confirmDownload("testbin", "1.0.0", DownloadDecisionYes)
	if err != nil {
		t.Fatalf("expected no error for DownloadDecisionYes, got %v", err)
	}
}

func TestEnsureBinary_Exists(t *testing.T) {
	tempDir := t.TempDir()
	binaryName := "fake-existing-binary"

	f, err := os.Create(filepath.Join(tempDir, binaryName))
	if err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}
	_ = f.Chmod(0755)
	f.Close()

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tempDir+string(os.PathListSeparator)+oldPath)

	err = ensureBinary(binaryName, "1.0.0", DownloadDecisionNo)
	if err != nil {
		t.Fatalf("expected no error when binary exists in PATH, got %v", err)
	}
}

func TestEnsureDependencies_Exists(t *testing.T) {
	tempDir := t.TempDir()

	tf, _ := os.Create(filepath.Join(tempDir, "terraform"))
	_, _ = tf.WriteString("#!/bin/sh\necho '{\"terraform_version\": \"1.12.2\"}'\n")
	_ = tf.Chmod(0755)
	tf.Close()

	packer, _ := os.Create(filepath.Join(tempDir, "packer"))
	_ = packer.Chmod(0755)
	packer.Close()

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tempDir+string(os.PathListSeparator)+oldPath)

	err := EnsureDependencies(DownloadDecisionNo)
	if err != nil {
		t.Fatalf("expected no error when dependencies exist, got %v", err)
	}
}

func TestEnsureDependencies_Missing(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", t.TempDir()) // Empty PATH basically

	err := EnsureDependencies(DownloadDecisionNo)
	if err == nil {
		t.Fatalf("expected error when dependencies are missing and decision is No")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2  string
		want    int
		wantErr bool
	}{
		{"1.12.2", "1.12.2", 0, false},
		{"1.12.3", "1.12.2", 1, false},
		{"1.13.0", "1.12.2", 1, false},
		{"1.12.1", "1.12.2", -1, false},
		{"1.11.0", "1.12.2", -1, false},
		{"v1.12.2", "1.12.2", 0, false},
		{"1.12.2-beta1", "1.12.2", -1, false},
		{"invalid", "1.12.2", 0, true},
	}

	for _, tt := range tests {
		got, err := compareVersions(tt.v1, tt.v2)
		if (err != nil) != tt.wantErr {
			t.Errorf("compareVersions(%q, %q) error = %v, wantErr %v", tt.v1, tt.v2, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestEnsureBinary_TerraformNewerVersion(t *testing.T) {
	tempDir := t.TempDir()
	tf, _ := os.Create(filepath.Join(tempDir, "terraform"))
	_, _ = tf.WriteString("#!/bin/sh\necho '{\"terraform_version\": \"1.13.0\"}'\n")
	_ = tf.Chmod(0755)
	tf.Close()

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tempDir+string(os.PathListSeparator)+oldPath)

	err := ensureBinary("terraform", "1.12.2", DownloadDecisionNo)
	if err != nil {
		t.Fatalf("expected no error for newer version, got %v", err)
	}
}

func TestEnsureBinary_TerraformOlderVersion(t *testing.T) {
	tempDir := t.TempDir()
	tf, _ := os.Create(filepath.Join(tempDir, "terraform"))
	_, _ = tf.WriteString("#!/bin/sh\necho '{\"terraform_version\": \"1.11.0\"}'\n")
	_ = tf.Chmod(0755)
	tf.Close()

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tempDir+string(os.PathListSeparator)+oldPath)

	err := ensureBinary("terraform", "1.12.2", DownloadDecisionNo)
	if err == nil {
		t.Fatalf("expected error for older version with DownloadDecisionNo")
	}
}
