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

package intent

import (
	"os"
	"path/filepath"
)

// ToolkitPath returns the toolkit root that contains the v2/ catalog: the directory of
// the running gcluster binary. `gcluster create` and `gcluster catalog` both use it so
// that discovery always shows exactly the catalog the compiler will read.
func ToolkitPath() string {
	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}
	execDir := filepath.Dir(execPath)
	if info, err := os.Stat(filepath.Join(execDir, "v2")); err == nil && info.IsDir() {
		return execDir
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; {
			if info, err := os.Stat(filepath.Join(dir, "v2")); err == nil && info.IsDir() {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return execDir
}
