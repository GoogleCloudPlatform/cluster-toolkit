/**
 * Copyright 2021 Google LLC
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

package modulereader

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

// PackerReader implements Modulereader for packer modules
type PackerReader struct {
	allModInfo map[string]ModuleInfo
}

// SetInfo sets the module info for a module key'd on the source
func (r PackerReader) SetInfo(source string, modInfo ModuleInfo) {
	r.allModInfo[source] = modInfo
}

func addTfExtension(filename string) {
	newFilename := fmt.Sprintf("%s.tf", filename)
	if err := os.Rename(filename, newFilename); err != nil {
		log.Fatalf(
			"failed to add .tf extension to %s needed to get info on packer module: %e",
			filename, err)
	}
}

func getHCLFiles(dir string) []string {
	allFiles, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatalf("Failed to read packer source directory %s", dir)
	}
	var hclFiles []string
	for _, f := range allFiles {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) == ".hcl" {
			hclFiles = append(hclFiles, filepath.Join(dir, f.Name()))
		}
	}
	return hclFiles
}

func copyHCLFilesToTmp(dir string) (string, []string, error) {
	tmpDir, err := ioutil.TempDir("", "pkwriter-*")
	if err != nil {
		return "", []string{}, fmt.Errorf(
			"failed to create temp directory for packer reader")
	}
	hclFiles := getHCLFiles(dir)
	var hclFilePaths []string

	for _, hclFilename := range hclFiles {

		// Open file for copying
		hclFile, err := os.Open(hclFilename)
		if err != nil {
			return "", hclFiles, fmt.Errorf(
				"failed to open packer HCL file %s: %v", hclFilename, err)
		}
		defer hclFile.Close()

		// Create a file to copy to
		destPath := filepath.Join(tmpDir, filepath.Base(hclFilename))
		destination, err := os.Create(destPath)
		if err != nil {
			return "", hclFiles, fmt.Errorf(
				"failed to create copy of packer HCL file %s: %v", hclFilename, err)
		}
		defer destination.Close()

		// Copy
		if _, err := io.Copy(destination, hclFile); err != nil {
			return "", hclFiles, fmt.Errorf(
				"failed to copy packer module at %s to temporary directory to inspect: %v",
				dir, err)
		}
		hclFilePaths = append(hclFilePaths, destPath)
	}
	return tmpDir, hclFilePaths, nil
}

// GetInfo reads the ModuleInfo for a packer module
func (r PackerReader) GetInfo(source string) (ModuleInfo, error) {
	if modInfo, ok := r.allModInfo[source]; ok {
		return modInfo, nil
	}
	tmpDir, packerFiles, err := copyHCLFilesToTmp(source)
	if err != nil {
		return ModuleInfo{}, err
	}
	defer os.RemoveAll(tmpDir)

	for _, packerFile := range packerFiles {
		addTfExtension(packerFile)
	}
	modInfo, err := getHCLInfo(tmpDir)
	if err != nil {
		return modInfo, fmt.Errorf("PackerReader: %v", err)
	}
	r.allModInfo[source] = modInfo
	return modInfo, nil
}
