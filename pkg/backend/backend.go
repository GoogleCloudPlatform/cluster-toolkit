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

package backend

// Backend interface for writing blueprints to a storage
type Backend interface {
	CreateDirectory(bpDirectoryPath string) error
	CopyFromPath(src string, dst string) error
}

var backends = map[string]Backend{
	"local": new(Local),
}

// GetBackendLocal gets the instance writing blueprints to a local
func GetBackendLocal() Backend {
	return backends["local"]
}
