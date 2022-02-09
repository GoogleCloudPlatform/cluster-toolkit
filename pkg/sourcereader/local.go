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
	"os"
)

// LocalSourceReader reads resources from a local directory
type LocalSourceReader struct{}

// GetResourceInfo gets resreader.ResourceInfo for the given kind from the local source
func (r LocalSourceReader) GetResourceInfo(resPath string, kind string) (resreader.ResourceInfo, error) {
	if !IsLocalPath(resPath) {
		return resreader.ResourceInfo{}, fmt.Errorf("Source is not valid: %s", resPath)
	}

	reader := resreader.Factory(kind)
	return reader.GetInfo(resPath)
}

// GetResource copies the local source to a provided destination (the blueprint directory)
func (r LocalSourceReader) GetResource(resPath string, copyPath string) error {
	if !IsLocalPath(resPath) {
		return fmt.Errorf("Source is not valid: %s", resPath)
	}

	if _, err := os.Stat(resPath); os.IsNotExist(err) {
		return fmt.Errorf("Local resource doesn't exist at %s", resPath)
	}

	return copyFromPath(resPath, copyPath)
}
