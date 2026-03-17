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
	if !strings.HasPrefix(newPath, oldPath) {
		t.Errorf("Expected new PATH to start with old PATH")
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
