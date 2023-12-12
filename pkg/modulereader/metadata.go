// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modulereader

import (
	"errors"
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path/filepath"

	"github.com/GoogleCloudPlatform/cloud-foundation-toolkit/cli/bpmetadata"
	"gopkg.in/yaml.v3"
)

// GetMetadata reads and parses `metadata.yaml` from module root.
// Expects source to be either a local or embedded path.
func GetMetadata(source string) (*bpmetadata.BlueprintMetadata, error) {
	var err error
	var data []byte
	// TODO: use bpmetadata.UnmarshalMetadata, it performs some additional checks
	filePath := filepath.Join(source, "metadata.yaml")

	switch {
	case sourcereader.IsEmbeddedPath(source):
		data, err = sourcereader.ModuleFS.ReadFile(filePath)
	case sourcereader.IsLocalPath(source):
		var absPath string
		if absPath, err = filepath.Abs(filePath); err == nil {
			data, err = os.ReadFile(absPath)
		}
	default:
		err = errors.New("source must be local or embedded")
	}
	if err != nil {
		return nil, err
	}

	var mtd bpmetadata.BlueprintMetadata
	err = yaml.Unmarshal(data, &mtd)
	return &mtd, err
}

// GetMetadataSafe attempts to GetMetadata if it fails returns
// hardcoded legacy metadata.
func GetMetadataSafe(source string) *bpmetadata.BlueprintMetadata {
	if mtd, err := GetMetadata(source); err == nil {
		return mtd
	}
	return legacyMetadata(source)
}
