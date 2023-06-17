// Copyright 2022 Google LLC
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

package sourcereader

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-getter"
)

var goGetterDetectors = []getter.Detector{
	new(getter.GitHubDetector),
	new(getter.GitDetector),
}

var goGetterGetters = map[string]getter.Getter{
	"git": &getter.GitGetter{Timeout: 5 * time.Minute},
}

var goGetterDecompressors = map[string]getter.Decompressor{}

// GitSourceReader reads modules from a git repository
type GitSourceReader struct{}

func copyGitModules(srcPath string, destPath string) error {
	client := getter.Client{
		Src: srcPath,
		Dst: destPath,
		Pwd: destPath,

		Mode: getter.ClientModeDir,

		Detectors:     goGetterDetectors,
		Decompressors: goGetterDecompressors,
		Getters:       goGetterGetters,
		Ctx:           context.Background(),
	}
	err := client.Get()
	return err
}

// GetModule copies the git source to a provided destination (the deployment directory)
func (r GitSourceReader) GetModule(modPath string, copyPath string) error {
	if !IsGitPath(modPath) {
		return fmt.Errorf("source is not valid: %s", modPath)
	}

	modDir, err := ioutil.TempDir("", "git-module-*")
	defer os.RemoveAll(modDir)
	writeDir := filepath.Join(modDir, "mod")
	if err != nil {
		return err
	}

	if err := copyGitModules(modPath, writeDir); err != nil {
		return fmt.Errorf("failed to clone git module at %s to tmp dir %s: %v",
			modPath, writeDir, err)
	}

	return copyFromPath(writeDir, copyPath)
}
