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

package dependencies

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDownloadAndExtract(t *testing.T) {
	// Create a mock zip file
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, _ := zw.Create("testbin")
	_, _ = fw.Write([]byte("mock executable content"))
	zw.Close()
	zipContent := buf.Bytes()

	// Calculate checksum
	hasher := sha256.New()
	hasher.Write(zipContent)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	binaryName := "testbin"
	version := "1.0.0"
	osArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	expectedChecksumKey := fmt.Sprintf("%s_%s", binaryName, osArch)

	// Inject checksum
	originalChecksums := ExpectedChecksums
	ExpectedChecksums = map[string]string{
		expectedChecksumKey: checksum,
	}
	defer func() { ExpectedChecksums = originalChecksums }()

	// Setup mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipContent)
	}))
	defer ts.Close()

	originalUrlFormat := urlFormat
	urlFormat = ts.URL + "/%s/%s/%s_%s_%s.zip"
	defer func() { urlFormat = originalUrlFormat }()

	targetDir := t.TempDir()

	err := downloadAndExtract(binaryName, version, targetDir)
	if err != nil {
		t.Fatalf("downloadAndExtract failed: %v", err)
	}

	// Verify extracted file
	extractedFile := filepath.Join(targetDir, "testbin")
	content, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(content) != "mock executable content" {
		t.Errorf("expected 'mock executable content', got '%s'", string(content))
	}
}

func TestDownloadAndExtract_NoExecutable(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, _ := zw.Create("otherfile")
	_, _ = fw.Write([]byte("not the executable"))
	zw.Close()
	zipContent := buf.Bytes()

	hasher := sha256.New()
	hasher.Write(zipContent)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	binaryName := "testbin"
	osArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)

	originalChecksums := ExpectedChecksums
	ExpectedChecksums = map[string]string{
		fmt.Sprintf("%s_%s", binaryName, osArch): checksum,
	}
	defer func() { ExpectedChecksums = originalChecksums }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipContent)
	}))
	defer ts.Close()

	originalUrlFormat := urlFormat
	urlFormat = ts.URL + "/%s/%s/%s_%s_%s.zip"
	defer func() { urlFormat = originalUrlFormat }()

	err := downloadAndExtract(binaryName, "1.0.0", t.TempDir())
	if err == nil {
		t.Fatalf("expected error due to missing executable")
	}
}

func TestVerifyChecksum_Failure(t *testing.T) {
	err := verifyChecksum([]byte("bad content"), "expectedchecksum", "testbin")
	if err == nil {
		t.Fatalf("expected checksum verification to fail")
	}
}

func TestDownloadRelease_Failure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := downloadRelease(ts.URL, "testbin")
	if err == nil {
		t.Fatalf("expected download Release to fail on 404")
	}
}

func TestDownloadAndExtract_UnsupportedOS(t *testing.T) {
	originalChecksums := ExpectedChecksums
	ExpectedChecksums = map[string]string{}
	defer func() { ExpectedChecksums = originalChecksums }()

	err := downloadAndExtract("testbin", "1.0.0", t.TempDir())
	if err == nil {
		t.Fatalf("expected unsupported OS/arch error")
	}
}
