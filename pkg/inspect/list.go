// Copyright 2026 Google LLC
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

package inspect

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// SourceAndKind is source and kind
type SourceAndKind struct {
	Source string
	Kind   string
}

// ListModules in directory
func ListModules(root string, dir string) ([]SourceAndKind, error) {
	ret := []SourceAndKind{}
	err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".terraform" {
			return filepath.SkipDir
		}
		src, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(d.Name()) == ".tf" {
			ret = append(ret, SourceAndKind{src, "terraform"})
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".pkr.hcl") {
			ret = append(ret, SourceAndKind{src, "packer"})
			return filepath.SkipDir
		}
		return nil
	})
	return ret, err
}

// LocalModules returns source and kind for all local modules
func LocalModules() ([]SourceAndKind, error) {
	ret := []SourceAndKind{}

	for _, sub := range []string{"modules", "community/modules"} {
		mods, err := ListModules("../../", sub)
		if err != nil {
			return []SourceAndKind{}, err
		}
		ret = append(ret, mods...)
	}
	return ret, nil
}
