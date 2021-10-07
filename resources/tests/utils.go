// Copyright 2021 Google LLC
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

package restests

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/gruntwork-io/terratest/modules/test-structure"
)

const rootDir = ".."

var timeStamp = time.Now().Format(strings.ToLower(time.RFC3339))

func getDepth(dir string) int {
	dir = filepath.ToSlash(dir)
	return len(strings.Split(dir, "/"))
}

func cleanupTestDirectory(dir string, sourceDir string) {
	depth := getDepth(sourceDir) + 1 // for rootDir
	for i := 0; i < depth; i++ {
		dir, _ = filepath.Split(dir)
		// remove trailing /
		dir = filepath.Clean(dir)
	}
	_, err := os.Stat(dir)
	if err != nil {
		return
	}
	err = os.RemoveAll(dir)
	if err != nil {
		log.Fatalf("Failed to cleanup tmp test directory %s, %v", dir, err)
	}
}

func testInitAndValidate(
	t *testing.T,
	rootDir string,
	terraformDirRelativeToRoot string) {

	t.Parallel()
	tmpTestFolder := test_structure.CopyTerraformFolderToTemp(
		t, rootDir, terraformDirRelativeToRoot)
	defer cleanupTestDirectory(tmpTestFolder, terraformDirRelativeToRoot)
	terraformOptions := terraform.WithDefaultRetryableErrors(t,
		&terraform.Options{TerraformDir: tmpTestFolder})
	terraform.InitAndValidate(t, terraformOptions)
}
