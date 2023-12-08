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
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// ModuleFS contains embedded modules (./modules) for use in building
// blueprints. The main package creates and injects the modules directory as
// hpc-toolkit/modules are not accessible at the package level.
var ModuleFS BaseFS

// BaseFS is an join interface with the functionality needed
// in copyDirFromModules. Works with embed.FS and afero.FS
type BaseFS interface {
	fs.ReadDirFS
	fs.ReadFileFS
}

// EmbeddedSourceReader reads modules from a local directory
type EmbeddedSourceReader struct{}

func copyFileOut(bfs BaseFS, src string, dst string) error {
	content, err := bfs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read embedded %#v: %v", src, err)
	}
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create %#v: %v", dst, err)
	}
	defer f.Close()
	if _, err = f.Write(content); err != nil {
		return fmt.Errorf("failed to write %#v: %v", dst, err)
	}
	return nil
}

// copyDirFromModules copies an FS directory to a local path
func copyDirFromModules(bfs BaseFS, source string, dest string) error {
	dirEntries, err := bfs.ReadDir(source)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		entryName := dirEntry.Name()
		// path package (not path/filepath) should be used for embedded source
		// as the path separator is a forward slash, even on Windows systems.
		// https://pkg.go.dev/embed#hdr-Directives
		entrySource := path.Join(source, entryName)
		entryDest := filepath.Join(dest, entryName)
		if dirEntry.IsDir() {
			if err := os.Mkdir(entryDest, 0755); err != nil {
				return err
			}
			if err = copyDirFromModules(bfs, entrySource, entryDest); err != nil {
				return err
			}
		} else {
			if err := copyFileOut(bfs, entrySource, entryDest); err != nil {
				return err
			}

		}
	}
	return nil
}

// copyFSToTempDir is a temporary workaround until tfconfig.ReadFromFilesystem
// works against embed.FS.
// Open Issue: https://github.com/hashicorp/terraform-config-inspect/issues/68
func copyFSToTempDir(bfs BaseFS, modulePath string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "tfconfig-module-*")
	if err != nil {
		return tmpDir, err
	}
	err = copyDirFromModules(bfs, modulePath, tmpDir)
	return tmpDir, err
}

// GetModule copies the embedded source to a provided destination (the deployment directory)
func (r EmbeddedSourceReader) GetModule(modPath string, copyPath string) error {
	if ModuleFS == nil {
		return fmt.Errorf("embedded file system is not initialized")
	}
	if !IsEmbeddedPath(modPath) {
		return fmt.Errorf("source is not valid: %s", modPath)
	}

	modDir, err := copyFSToTempDir(ModuleFS, modPath)
	defer os.RemoveAll(modDir)
	if err != nil {
		err = fmt.Errorf("failed to copy embedded module at %s to tmp dir %s: %v",
			modPath, modDir, err)
		return err
	}

	return copyFromPath(modDir, copyPath)
}

// CopyDir copies embedded directory to destination path
func (r EmbeddedSourceReader) CopyDir(src string, dst string) error {
	if ModuleFS == nil {
		return fmt.Errorf("embedded file system is not initialized")
	}
	return copyDirFromModules(ModuleFS, src, dst)
}
