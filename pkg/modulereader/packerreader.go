/**
 * Copyright 2022 Google LLC
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
	"hpc-toolkit/pkg/sourcereader"
	"io/ioutil"
	"log"
	"os"
	"path"
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
			"failed to add .tf extension to %s needed to get info on packer module: %v",
			filename, err)
	}
}

func getHCLFiles(dir string) []string {
	allFiles, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatalf("Failed to read packer source directory at %s: %v", dir, err)
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

// GetInfo reads the ModuleInfo for a packer module
func (r PackerReader) GetInfo(source string) (ModuleInfo, error) {
	if modInfo, ok := r.allModInfo[source]; ok {
		return modInfo, nil
	}

	tmpDir, err := ioutil.TempDir("", "pkwriter-*")
	if err != nil {
		return ModuleInfo{}, fmt.Errorf(
			"failed to create temp directory for packer reader")
	}
	defer os.RemoveAll(tmpDir)

	modName := path.Base(source)
	modPath := path.Join(tmpDir, modName)

	sourceReader := sourcereader.Factory(source)
	if err = sourceReader.GetModule(source, modPath); err != nil {
		return ModuleInfo{}, err
	}
	packerFiles := getHCLFiles(modPath)

	for _, packerFile := range packerFiles {
		addTfExtension(packerFile)
	}
	modInfo, err := getHCLInfo(modPath)
	if err != nil {
		return modInfo, fmt.Errorf("PackerReader: %v", err)
	}
	r.allModInfo[source] = modInfo
	return modInfo, nil
}
