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

package modulereader

import (
	"hpc-toolkit/pkg/sourcereader"
	"path"
	"path/filepath"
)

// ResolveDependencies takes a list of module paths (relative to toolkit root or absolute)
// and returns a deduplicated list of all transitive local dependencies.
// It only traces dependencies for embedded modules.
func ResolveDependencies(requestedModules []string) ([]string, error) {
	visited := make(map[string]bool)
	var result []string

	var dfs func(string) error
	dfs = func(mod string) error {
		if visited[mod] {
			return nil
		}
		visited[mod] = true
		result = append(result, mod)

		// Only trace dependencies for embedded modules
		if !sourcereader.IsEmbeddedPath(mod) {
			return nil
		}

		deps, err := GetLocalDependencies(mod)
		if err != nil {
			return err
		}

		for _, dep := range deps {
			if !visited[dep] {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for _, mod := range requestedModules {
		var normalizedMod string
		if sourcereader.IsEmbeddedPath(mod) {
			normalizedMod = path.Clean(mod)
		} else {
			normalizedMod = filepath.ToSlash(filepath.Clean(mod))
		}
		if err := dfs(normalizedMod); err != nil {
			return nil, err
		}
	}

	return result, nil
}
