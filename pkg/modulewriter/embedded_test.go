/*
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package modulewriter writes modules to a deployment directory

package modulewriter

import (
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestCopyEmbeddedModules(t *testing.T) {
	mockFS := fstest.MapFS{
		"modules/network/vpc/main.tf": {Data: []byte("resource \"foo\" \"bar\" {}")},
		"community/modules/dummy":     {Data: []byte("dummy")},
	}

	oldFS := sourcereader.ModuleFS
	sourcereader.ModuleFS = mockFS
	defer func() { sourcereader.ModuleFS = oldFS }()

	t.Run("EmbeddedModule", func(t *testing.T) {

		bp := config.Blueprint{
			Groups: []config.Group{{
				Name: "group1",
				Modules: []config.Module{{
					ID:     "mod1",
					Kind:   config.TerraformKind,
					Source: "modules/network/vpc", // This is a real embedded module
				}},
			}},
		}

		tmpDir := filepath.Join(t.TempDir(), "deployment")

		err := WriteDeployment(bp, tmpDir)
		if err != nil {
			t.Fatalf("WriteDeployment failed: %v", err)
		}

		embeddedPath := filepath.Join(tmpDir, "group1", "modules", "embedded", "modules", "network", "vpc")
		if _, err := os.Stat(embeddedPath); err != nil {
			t.Errorf("Expected embedded module copy at %s, got error: %v", embeddedPath, err)
		}
	})
}
