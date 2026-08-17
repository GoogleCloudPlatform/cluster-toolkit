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

package sourcereader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hpc-toolkit/pkg/dependencies"

	"github.com/hashicorp/go-getter"
)

var defaultDetectors = []getter.Detector{
	new(getter.GitHubDetector),
	new(getter.GitLabDetector),
	new(getter.GitDetector),
	new(getter.GCSDetector),
}

// GitMissingError indicates that git is required but missing from PATH.
type GitMissingError struct {
	Source string
}

func (e GitMissingError) Error() string {
	return fmt.Sprintf("'git' is required to download remote module %q but not found in PATH. Please install git via your OS package manager (e.g., `apt install git` or `brew install git`).", e.Source)
}

// GoGetterSourceReader reads modules from a git repository
type GoGetterSourceReader struct{}

func getterClient(source string, dst string) getter.Client {
	return getter.Client{
		Src: source,
		Dst: dst,
		Pwd: dst,

		//Mode: getter.ClientModeDir,
		Mode: getter.ClientModeAny,

		Detectors: defaultDetectors,
		Getters: map[string]getter.Getter{
			"git": &getter.GitGetter{Timeout: 5 * time.Minute},
			"gcs": &getter.GCSGetter{Timeout: 5 * time.Minute},
		},

		// Disable decompression (e.g. tar, zip) by supplying no decompressors
		Decompressors: map[string]getter.Decompressor{},
		Ctx:           context.Background(),
	}
}

// GetModule copies the git source to a provided destination (the deployment directory)
func (r GoGetterSourceReader) GetModule(source string, dst string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	detected, err := getter.Detect(source, pwd, defaultDetectors)
	if err != nil {
		return fmt.Errorf("failed to recognize module source %q: %w; "+
			"the Cluster Toolkit supports remote modules from Git repositories (e.g., GitHub, GitLab) and Google Cloud Storage (GCS); "+
			"if this is a local path, please ensure it starts with './' or '../'",
			source, err)
	}

	u, err := url.Parse(detected)
	if err != nil {
		return fmt.Errorf("invalid detected module URL %q: %w", detected, err)
	}

	isGit := u.Scheme == "git" || strings.HasPrefix(u.Scheme, "git+")
	isSupported := isGit || u.Scheme == "gcs"

	if !isSupported {
		return fmt.Errorf("unsupported module source protocol %q (detected from %q); "+
			"supported protocols are Git (e.g., git::https://..., git+ssh://...) and GCS (e.g., gcs::...)",
			u.Scheme, source)
	}

	if isGit && !dependencies.HasBinary("git") {
		return GitMissingError{Source: source}
	}

	tmp, err := os.MkdirTemp("", "get-module-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()

	writeDir := filepath.Join(tmp, "mod")
	client := getterClient(detected, writeDir)

	if err := client.Get(); err != nil {
		return fmt.Errorf("failed to get module at %q to %q: %w", detected, writeDir, err)
	}

	return copyFromPath(writeDir, dst)
}
