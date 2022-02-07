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
	"fmt"
	"hpc-toolkit/pkg/resreader"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
)

// ResourceFS contains embedded resources (./resources) for use in building
// blueprints. The main package creates and injects the resources directory as
// hpc-toolkit/resources are not accessible at the package level.
var ResourceFS BaseFS

// BaseFS is an extension of the io.fs interface with the functionality needed
// in CopyDirFromResources. Works with embed.FS and afero.FS
type BaseFS interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

// EmbeddedSourceReader reads resources from a local directory
type EmbeddedSourceReader struct{}

// copyDirFromResources copies an FS directory to a local path
func copyDirFromResources(fs BaseFS, source string, dest string) error {
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
			if err = copyDirFromResources(fs, entrySource, entryDest); err != nil {
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

// copyFSToTempDir is a temporary workaround until tfconfig.ReadFromFilesystem
// works against embed.FS.
// Open Issue: https://github.com/hashicorp/terraform-config-inspect/issues/68
func copyFSToTempDir(fs BaseFS, modulePath string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "tfconfig-module-*")
	if err != nil {
		return tmpDir, err
	}
	err = copyDirFromResources(fs, modulePath, tmpDir)
	return tmpDir, err
}

// ValidateResource runs a basic validation that the embedded resource exists and contains the expected directories and files
func (r EmbeddedSourceReader) ValidateResource(resPath string, kind string) error {
	if !IsEmbeddedPath(resPath) {
		return fmt.Errorf("Source is not valid: %s", resPath)
	}

	resDir, err := copyFSToTempDir(ResourceFS, resPath)
	defer os.RemoveAll(resDir)
	if err != nil {
		return fmt.Errorf("failed to copy embedded resource at %s to tmp dir %s: %v",
			resPath, resDir, err)
	}

	reader := resreader.Factory(kind)
	_, err = reader.GetInfo(resDir)
	return err
}

// GetResourceInfo gets resreader.ResourceInfo for the given kind from the embedded source
func (r EmbeddedSourceReader) GetResourceInfo(resPath string, kind string) (resreader.ResourceInfo, error) {
	if !IsEmbeddedPath(resPath) {
		return resreader.ResourceInfo{}, fmt.Errorf("Source is not valid: %s", resPath)
	}

	resDir, err := copyFSToTempDir(ResourceFS, resPath)
	defer os.RemoveAll(resDir)
	if err != nil {
		err = fmt.Errorf("failed to copy embedded resource at %s to tmp dir %s: %v",
			resPath, resDir, err)
		return resreader.ResourceInfo{}, err
	}

	reader := resreader.Factory(kind)
	return reader.GetInfo(resDir)
}

// GetResource copies the embedded source to a provided destination (the blueprint directory)
func (r EmbeddedSourceReader) GetResource(resPath string, copyPath string) error {
	if !IsEmbeddedPath(resPath) {
		return fmt.Errorf("Source is not valid: %s", resPath)
	}

	resDir, err := copyFSToTempDir(ResourceFS, resPath)
	defer os.RemoveAll(resDir)
	if err != nil {
		err = fmt.Errorf("failed to copy embedded resource at %s to tmp dir %s: %v",
			resPath, resDir, err)
		return err
	}

	return copyFromPath(resDir, copyPath)
}
