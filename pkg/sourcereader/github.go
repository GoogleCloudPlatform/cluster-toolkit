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
	"hpc-toolkit/pkg/resreader"
	"io/ioutil"
	"os"

	"github.com/hashicorp/go-getter"
)

var goGetterDetectors = []getter.Detector{
	new(getter.GitHubDetector),
	new(getter.GitDetector),
}

var goGetterGetters = map[string]getter.Getter{
	"git": new(getter.GitGetter),
}

var goGetterDecompressors = map[string]getter.Decompressor{}

// GitHubSourceReader reads resources from a GitHub repository
type GitHubSourceReader struct{}

func copyGitHubResources(srcPath string, destPath string) error {
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

// GetResourceInfo gets resreader.ResourceInfo for the given kind from the GitHub source
func (r GitHubSourceReader) GetResourceInfo(resPath string, kind string) (resreader.ResourceInfo, error) {
	if !IsGitHubPath(resPath) {
		return resreader.ResourceInfo{}, fmt.Errorf("Source is not valid: %s", resPath)
	}

	resDir, err := ioutil.TempDir("", "git-module-*")
	defer os.RemoveAll(resDir)
	if err != nil {
		return resreader.ResourceInfo{}, err
	}

	if err := copyGitHubResources(resPath, resDir); err != nil {
		return resreader.ResourceInfo{}, fmt.Errorf("failed to clone GitHub resource at %s to tmp dir %s: %v",
			resPath, resDir, err)
	}

	reader := resreader.Factory(kind)
	return reader.GetInfo(resDir)
}

// GetResource copies the GitHub source to a provided destination (the blueprint directory)
func (r GitHubSourceReader) GetResource(resPath string, copyPath string) error {
	if !IsGitHubPath(resPath) {
		return fmt.Errorf("Source is not valid: %s", resPath)
	}

	resDir, err := ioutil.TempDir("", "git-module-*")
	defer os.RemoveAll(resDir)
	if err != nil {
		return err
	}

	if err := copyGitHubResources(resPath, resDir); err != nil {
		return fmt.Errorf("failed to clone GitHub resource at %s to tmp dir %s: %v",
			resPath, resDir, err)
	}

	return copyFromPath(resDir, copyPath)
}
