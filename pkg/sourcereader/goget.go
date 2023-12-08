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
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-getter"
)

// GoGetterSourceReader reads modules from a git repository
type GoGetterSourceReader struct{}

func getterClient(source string, dst string) getter.Client {
	return getter.Client{
		Src: source,
		Dst: dst,
		Pwd: dst,

		//Mode: getter.ClientModeDir,
		Mode: getter.ClientModeAny,

		Detectors: []getter.Detector{
			new(getter.GitHubDetector),
			new(getter.GitDetector),
			new(getter.GCSDetector),
		},
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
	tmp, err := os.MkdirTemp("", "get-module-*")
	defer os.RemoveAll(tmp)
	if err != nil {
		return err
	}

	writeDir := filepath.Join(tmp, "mod")
	client := getterClient(source, writeDir)

	if err := client.Get(); err != nil {
		return fmt.Errorf("failed to get module at %s to %s: %w", source, writeDir, err)
	}

	return copyFromPath(writeDir, dst)
}
