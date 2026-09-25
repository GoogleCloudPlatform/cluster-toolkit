// Copyright 2026 Google LLC
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
	"path/filepath"
	"strings"
)

// SourceReader interface for reading modules from a source
type SourceReader interface {
	// GetModule copies the source to a provided local destination (the deployment directory).
	GetModule(modPath string, copyPath string) error
}

func isWindowsDrivePath(p string) bool {
	if len(p) >= 2 && p[1] == ':' {
		c := p[0]
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	return false
}

// ToSlash normalizes all directory separators (including Windows backslashes) to forward slashes.
// Unlike filepath.ToSlash, this replaces backslashes unconditionally on all platforms.
func ToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// IsLocalPath checks if a source path is a local FS path
func IsLocalPath(source string) bool {
	cleanSource := ToSlash(source)
	return strings.HasPrefix(cleanSource, "./") ||
		strings.HasPrefix(cleanSource, "../") ||
		strings.HasPrefix(cleanSource, "/") ||
		isWindowsDrivePath(source) ||
		filepath.IsAbs(source)
}

// IsEmbeddedPath checks if a source path points to an embedded modules
func IsEmbeddedPath(source string) bool {
	cleanSource := ToSlash(source)
	return strings.HasPrefix(cleanSource, "modules/") || strings.HasPrefix(cleanSource, "community/modules/")
}

// IsRemotePath checks if path neither Local nor Embedded
func IsRemotePath(source string) bool {
	return !IsLocalPath(source) && !IsEmbeddedPath(source)
}

// Factory returns a SourceReader of module path
func Factory(modPath string) SourceReader {
	switch {
	case IsLocalPath(modPath):
		return LocalSourceReader{}
	case IsEmbeddedPath(modPath):
		return EmbeddedSourceReader{}
	default:
		return GoGetterSourceReader{}
	}
}

func copyFromPath(modPath string, copyPath string) error {
	// currently supporting only local blueprint directory
	deploymentio := deploymentio.GetDeploymentioLocal()

	if err := deploymentio.CreateDirectory(copyPath); err != nil {
		return err
	}

	return deploymentio.CopyFromPath(modPath, copyPath)
}
