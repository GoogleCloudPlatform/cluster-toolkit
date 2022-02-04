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

package resutils

import (
	"context"
	"io/fs"
	"os"
	"path"
	"strings"

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

// IsLocalPath checks if a source path is a local FS path
func IsLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/")
}

// IsEmbeddedPath checks if a source path points to an embedded resources
func IsEmbeddedPath(source string) bool {
	return strings.HasPrefix(source, "resources/")
}

// IsGitHubPath checks if a source path points to GitHub
func IsGitHubPath(source string) bool {
	return strings.HasPrefix(source, "github.com") || strings.HasPrefix(source, "git@github.com")
}

// BaseFS is an extension of the io.fs interface with the functionality needed
// in CopyDirFromResources. Works with embed.FS and afero.FS
type BaseFS interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

// CopyDirFromResources copies an FS directory to a local path
func CopyDirFromResources(fs BaseFS, source string, dest string) error {
	dirEntries, err := fs.ReadDir(source)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		entryName := dirEntry.Name()
		entrySource := path.Join(source, entryName)
		entryDest := path.Join(dest, entryName)
		if dirEntry.IsDir() {
			if err := os.Mkdir(entryDest, 0755); err != nil {
				return err
			}
			if err = CopyDirFromResources(fs, entrySource, entryDest); err != nil {
				return err
			}
		} else {
			fileBytes, err := fs.ReadFile(entrySource)
			if err != nil {
				return err
			}
			copyFile, err := os.Create(entryDest)
			if err != nil {
				return err
			}
			if _, err = copyFile.Write(fileBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyGitHubResources copies resources from GitHub to a local path
func CopyGitHubResources(source string, dest string) error {
	client := getter.Client{
		Src: source,
		Dst: dest,
		Pwd: dest,

		Mode: getter.ClientModeDir,

		Detectors:     goGetterDetectors,
		Decompressors: goGetterDecompressors,
		Getters:       goGetterGetters,
		Ctx:           context.Background(),
	}
	err := client.Get()
	return err
}
