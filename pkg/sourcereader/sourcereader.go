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
	"hpc-toolkit/pkg/deploymentio"
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
	// GetModule copies the source to a provided local destination (the deployment directory).
	GetModule(modPath string, copyPath string) error
}

var readers = map[int]SourceReader{
	local:    LocalSourceReader{},
	embedded: EmbeddedSourceReader{},
	github:   GitSourceReader{},
}

// IsLocalPath checks if a source path is a local FS path
func IsLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/")
}

// IsEmbeddedPath checks if a source path points to an embedded modules
func IsEmbeddedPath(source string) bool {
	return strings.HasPrefix(source, "modules/") || strings.HasPrefix(source, "community/modules/")
}

// IsGitPath checks if a source path points to GitHub or has the git:: prefix
func IsGitPath(source string) bool {
	return strings.HasPrefix(source, "github.com") ||
		strings.HasPrefix(source, "git@github.com") ||
		strings.HasPrefix(source, "git::")
}

// Factory returns a SourceReader of module path
func Factory(modPath string) SourceReader {
	validPrefixes := []string{
		"/", "./", "../",
		"modules/", "community/modules/",
		"git@", "github.com",
	}
	switch {
	case IsLocalPath(modPath):
		return readers[local]
	case IsEmbeddedPath(modPath):
		return readers[embedded]
	case IsGitPath(modPath):
		return readers[github]
	default:
		log.Fatalf(
			"Source (%s) not valid, must begin with one of: %s",
			modPath, strings.Join(validPrefixes, ", "))
	}

	return nil
}

func copyFromPath(modPath string, copyPath string) error {
	// currently supporting only local blueprint directory
	deploymentio := deploymentio.GetDeploymentioLocal()

	if err := deploymentio.CreateDirectory(copyPath); err != nil {
		return err
	}

	return deploymentio.CopyFromPath(modPath, copyPath)
}
