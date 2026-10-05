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
	"strings"
)

// SourceReader interface for reading modules from a source
type SourceReader interface {
	// GetModule copies the source to a provided local destination (the deployment directory).
	GetModule(modPath string, copyPath string) error
}

// isWindowsDrivePath checks if a path begins with a Windows drive letter (e.g., C:\, C:/, C:relative).
// It accepts cleanSource which has already been normalized via ToSlash(source) in IsLocalPath.
//
// Architectural design tradeoffs:
//   - Single-character URL schemes with authorities (s://...) and forced protocol prefixes (s::...) are excluded.
//   - Unnormalized multi-slash local paths (e.g. C://foo, C:\\foo) are intentionally classified as remote
//     to avoid collision with RFC 3986 hierarchical URIs.
//   - Single-character opaque URIs without slashes (e.g. s:manifest.json) are classified as drive-relative
//     local paths due to structural ambiguity with C:foo.
func isWindowsDrivePath(cleanSource string) bool {
	if len(cleanSource) < 2 || cleanSource[1] != ':' {
		return false
	}
	c := cleanSource[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return false
	}
	if len(cleanSource) > 2 {
		// Reject go-getter forced protocol syntax (e.g., s::https://...)
		if cleanSource[2] == ':' {
			return false
		}
		// Reject URI schemes (e.g., s://...)
		if strings.HasPrefix(cleanSource[2:], "//") {
			return false
		}
	}
	return true
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
		isWindowsDrivePath(cleanSource)
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
