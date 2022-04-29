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
	"hpc-toolkit/pkg/blueprintio"
	"hpc-toolkit/pkg/resreader"
	"log"
	"strings"
)

const (
	local = iota
	embedded
	github
)

// SourceReader interface for reading modules from a source
type SourceReader interface {
	// GetModuleInfo would leverage resreader.GetInfo for the given kind.
	// GetModuleInfo would operate over the source without creating a local copy.
	// This would be very dependent on the kind of module.
	GetModuleInfo(modPath string, kind string) (resreader.ModuleInfo, error)

	// GetModule copies the source to a provided local destination (the deployment directory).
	GetModule(modPath string, copyPath string) error
}

var readers = map[int]SourceReader{
	local:    LocalSourceReader{},
	embedded: EmbeddedSourceReader{},
	github:   GitHubSourceReader{},
}

// IsLocalPath checks if a source path is a local FS path
func IsLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/")
}

// IsEmbeddedPath checks if a source path points to an embedded modules
func IsEmbeddedPath(source string) bool {
	return strings.HasPrefix(source, "modules/")
}

// IsGitHubPath checks if a source path points to GitHub
func IsGitHubPath(source string) bool {
	return strings.HasPrefix(source, "github.com") || strings.HasPrefix(source, "git@github.com")
}

// Factory returns a SourceReader of module path
func Factory(modPath string) SourceReader {
	switch {
	case IsLocalPath(modPath):
		return readers[local]
	case IsEmbeddedPath(modPath):
		return readers[embedded]
	case IsGitHubPath(modPath):
		return readers[github]
	default:
		log.Fatalf("Source (%s) not valid, should begin with /, ./, ../, modules/, git@ or github.com",
			modPath)
	}

	return nil
}

func copyFromPath(modPath string, copyPath string) error {
	// currently supporting only local blueprint directory
	blueprintio := blueprintio.GetBlueprintIOLocal()

	if err := blueprintio.CreateDirectory(copyPath); err != nil {
		return err
	}

	return blueprintio.CopyFromPath(modPath, copyPath)
}
