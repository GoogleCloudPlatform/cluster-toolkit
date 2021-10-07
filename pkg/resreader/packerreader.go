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

package resreader

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
)

// PackerReader implements ResReader for packer resources
type PackerReader struct {
	allResInfo map[string]ResourceInfo
}

// SetInfo sets the resource info for a resource key'd on the source
func (r PackerReader) SetInfo(source string, resInfo ResourceInfo) {
	r.allResInfo[source] = resInfo
}

func addTfExtension(filename string) {
	newFilename := fmt.Sprintf("%s.tf", filename)
	os.Rename(filename, newFilename)
}

func getHCLFiles(dir string) []string {
	all_files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatalf("Failed to read packer source directory %s", dir)
	}
	var hcl_files []string
	for _, f := range all_files {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) == ".hcl" {
			hcl_files = append(hcl_files, path.Join(dir, f.Name()))
		}
	}
	return hcl_files
}

func copyHCLFilesToTmp(dir string) (string, []string) {
	tmpDir, err := ioutil.TempDir("", "pkwriter-*")
	if err != nil {
		log.Fatalf("Failed to create temp directory for packer writer.")
	}
	hclFiles := getHCLFiles(dir)
	var hclFilePaths []string

	for _, hclFilename := range hclFiles {

		// Open file for copying
		hclFile, err := os.Open(hclFilename)
		if err != nil {
			log.Fatalf("Failed to open packer HCL file %s: %v", hclFilename, err)
		}
		defer hclFile.Close()

		// Create a file to copy to
		destPath := path.Join(tmpDir, path.Base(hclFilename))
		destination, err := os.Create(destPath)
		if err != nil {
			log.Fatalf(
				"Failed to create copy of packer HCL file %s: %v", hclFilename, err)
		}
		defer destination.Close()

		// Copy
		io.Copy(destination, hclFile)
		hclFilePaths = append(hclFilePaths, destPath)
	}
	return tmpDir, hclFilePaths
}

// GetInfo reads the ResourceInfo for a packer module
func (r PackerReader) GetInfo(source string) ResourceInfo {
	if resInfo, ok := r.allResInfo[source]; ok {
		return resInfo
	}
	tmpDir, packerFiles := copyHCLFilesToTmp(source)
	defer os.RemoveAll(tmpDir)
	for _, packerFile := range packerFiles {
		addTfExtension(packerFile)
	}
	resInfo, err := getHCLInfo(tmpDir)
	if err != nil {
		log.Fatalf("PackerReader: %v", err)
	}
	r.allResInfo[source] = resInfo
	return resInfo
}
