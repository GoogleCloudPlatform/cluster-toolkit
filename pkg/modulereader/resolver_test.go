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
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type mockFS struct {
	fs.FS
}

func (m mockFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(m.FS, name)
}

func (m mockFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(m.FS, name)
}

func TestGetLocalDependencies(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-module-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	depDir := filepath.Join(tmpDir, "dep-module")
	if err := os.Mkdir(depDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "main.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	tfContent := `
module "my_dep" {
  source = "./dep-module"
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(tfContent), 0644); err != nil {
		t.Fatal(err)
	}

	deps, err := GetLocalDependencies(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	expectedDep := filepath.Join(tmpDir, "dep-module")
	if deps[0] != expectedDep {
		t.Errorf("expected dependency %s, got %s", expectedDep, deps[0])
	}
}

func TestResolveDependencies(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-resolve-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create modules/modA and modules/modB
	modADir := filepath.Join(tmpDir, "modules", "modA")
	modBDir := filepath.Join(tmpDir, "modules", "modB")

	for _, dir := range []string{modADir, modBDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// modA depends on modB
	if err := os.WriteFile(filepath.Join(modADir, "main.tf"), []byte(`module "b" { source = "../modB" }`), 0644); err != nil {
		t.Fatal(err)
	}

	// Save old ModuleFS and restore it after test
	oldFS := sourcereader.ModuleFS
	defer func() { sourcereader.ModuleFS = oldFS }()

	// Set ModuleFS to mockFS wrapping tmpDir
	sourcereader.ModuleFS = mockFS{os.DirFS(tmpDir)}

	// Call ResolveDependencies with embedded path
	resolved, err := ResolveDependencies([]string{"modules/modA"})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"modules/modA", "modules/modB"}
	if len(resolved) != len(expected) {
		t.Fatalf("expected %d resolved modules, got %d", len(expected), len(resolved))
	}

	for i, v := range resolved {
		if v != expected[i] {
			t.Errorf("at index %d: expected %s, got %s", i, expected[i], v)
		}
	}
}

func TestResolveDependencies_Normalization(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-resolve-norm-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modADir := filepath.Join(tmpDir, "modules", "modA")
	if err := os.MkdirAll(modADir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modADir, "main.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	oldFS := sourcereader.ModuleFS
	defer func() { sourcereader.ModuleFS = oldFS }()
	sourcereader.ModuleFS = mockFS{os.DirFS(tmpDir)}

	resolved, err := ResolveDependencies([]string{"./modules/modA", "modules/modA"})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"modules/modA"}
	if len(resolved) != len(expected) {
		t.Fatalf("expected %d resolved modules, got %d", len(expected), len(resolved))
	}

	if resolved[0] != expected[0] {
		t.Errorf("expected %s, got %s", expected[0], resolved[0])
	}
}
