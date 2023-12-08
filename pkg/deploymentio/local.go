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

package deploymentio

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/otiai10/copy"
)

// Local writes blueprints to a local directory
type Local struct{}

func mkdirWrapper(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return fmt.Errorf("failed to create the directory %s: %v", directory, err)
	}

	return nil
}

// CreateDirectory creates the directory
func (b *Local) CreateDirectory(directory string) error {
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		return fmt.Errorf("the directory already exists: %s", directory)
	}

	// Create directory
	return mkdirWrapper(directory)
}

// CopyFromPath copies the source file to the destination file
func (b *Local) CopyFromPath(src string, dst string) error {
	absPath, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	return copy.Copy(absPath, dst)
}

// CopyFromFS copies the embedded source file to the destination file
func (b *Local) CopyFromFS(fs BaseFS, src string, dst string) error {
	data, err := fs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file %s: err=%w", src, err)
	}

	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("failed to write data in destination file %s: err=%w", dst, err)
	}

	return nil
}
