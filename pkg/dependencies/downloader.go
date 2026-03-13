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
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

var urlFormat = "https://releases.hashicorp.com/%s/%s/%s_%s_%s.zip"

func downloadAndExtract(binaryName, version, targetDir string) error {
	osArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	expectedChecksumKey := fmt.Sprintf("%s_%s", binaryName, osArch)

	expectedChecksum, ok := ExpectedChecksums[expectedChecksumKey]
	if !ok {
		return fmt.Errorf("unsupported OS/architecture: %s", osArch)
	}

	url := fmt.Sprintf(urlFormat, binaryName, version, binaryName, version, osArch)

	fmt.Printf("Downloading %s v%s...\n", binaryName, version)

	body, err := downloadReleaseBody(url, binaryName)
	if err != nil {
		return err
	}

	if err := verifyChecksum(body, expectedChecksum, binaryName); err != nil {
		return err
	}

	if err := extractBinary(body, binaryName, targetDir); err != nil {
		return err
	}

	return nil
}

func downloadReleaseBody(url string, binaryName string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", binaryName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download %s: HTTP %d", binaryName, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

func verifyChecksum(body []byte, expectedChecksum string, binaryName string) error {
	hasher := sha256.New()
	hasher.Write(body)
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch for %s. potential file corruption or Man-in-the-Middle (MITM) attack! expected: %s, got: %s", binaryName, expectedChecksum, actualChecksum)
	}

	return nil
}

func extractBinary(body []byte, binaryName string, targetDir string) error {
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("failed to read zip archive: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	tempDir, err := os.MkdirTemp(targetDir, "cluster-toolkit-deps-*")
	if err != nil {
		return fmt.Errorf("failed to create temporal directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var extractedTempPath string
	var extractedFileName string

	for _, file := range zipReader.File {
		if file.Name != binaryName && file.Name != binaryName+".exe" {
			continue // we only want the main executable
		}

		extractedTempPath = filepath.Join(tempDir, file.Name)
		extractedFileName = file.Name

		if err := extractFileFromZip(file, extractedTempPath); err != nil {
			return err
		}
	}

	if extractedTempPath == "" {
		return fmt.Errorf("executable not found in the zip archive")
	}

	targetPath := filepath.Join(targetDir, extractedFileName)

	if err := os.Rename(extractedTempPath, targetPath); err != nil {
		return fmt.Errorf("failed to move extracted file to target directory: %w", err)
	}

	return nil
}

func extractFileFromZip(file *zip.File, targetPath string) error {
	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in zip: %w", err)
	}
	defer rc.Close()

	out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return fmt.Errorf("failed to create extracted file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("failed to write extracted file: %w", err)
	}

	return nil
}
