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

package deploymentio

import (
	"io/fs"
)

// BaseFS is an extension of the io.fs interface with the functionality needed
// in CopyFromFS. Works with embed.FS and afero.FS
type BaseFS interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

// Deploymentio interface for writing blueprints to a storage
type Deploymentio interface {
	CreateDirectory(DepDirectoryPath string) error
	CopyFromPath(src string, dst string) error
	CopyFromFS(fs BaseFS, src string, dst string) error
}

var deploymentios = map[string]Deploymentio{
	"local": new(Local),
}

// GetDeploymentioLocal gets the instance writing blueprints to a local
func GetDeploymentioLocal() Deploymentio {
	return deploymentios["local"]
}
