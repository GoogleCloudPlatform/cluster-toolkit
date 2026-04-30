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

package modulewriter

import (
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/sourcereader"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"
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

func TestIntegrationTerraformInit(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform not found in PATH")
	}

	// 1. Setup temp dir for the test (acting as repo root for embedded modules)
	repoDir, err := os.MkdirTemp("", "test-repo-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(repoDir)

	// Create modules/modA, modules/modB, and modules/modC
	modADir := filepath.Join(repoDir, "modules", "modA")
	modBDir := filepath.Join(repoDir, "modules", "modB")
	modCDir := filepath.Join(repoDir, "modules", "modC")

	for _, dir := range []string{modADir, modBDir, modCDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		// tfconfig needs at least one .tf file
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// modA depends on modB
	if err := os.WriteFile(filepath.Join(modADir, "main.tf"), []byte(`module "b" { source = "../modB" }`), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Mock ModuleFS
	oldFS := sourcereader.ModuleFS
	defer func() { sourcereader.ModuleFS = oldFS }()
	sourcereader.ModuleFS = mockFS{os.DirFS(repoDir)}

	// 3. Create Blueprint
	bp := config.Blueprint{
		Vars: config.Dict{}.With("deployment_name", cty.StringVal("test-dep")),
		Groups: []config.Group{{
			Name: "primary",
			Modules: []config.Module{{
				Source: "modules/modA",
				ID:     "modA_inst",
				Kind:   config.TerraformKind,
			}},
		}},
	}

	// 4. Setup temp dir for deployment
	depDir, err := os.MkdirTemp("", "test-dep-*")
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(depDir)
	defer os.RemoveAll(depDir)

	// 5. Write Deployment
	if err := WriteDeployment(bp, depDir); err != nil {
		t.Fatal(err)
	}

	// 6. Verify files were copied
	groupDir := filepath.Join(depDir, "primary")
	modAInDep := filepath.Join(depDir, config.SharedModulesDirName, "embedded/modules/modA")
	modBInDep := filepath.Join(depDir, config.SharedModulesDirName, "embedded/modules/modB")
	modCInDep := filepath.Join(depDir, config.SharedModulesDirName, "embedded/modules/modC")

	if _, err := os.Stat(modAInDep); os.IsNotExist(err) {
		t.Errorf("modA was not copied to %s", modAInDep)
	}
	if _, err := os.Stat(modBInDep); os.IsNotExist(err) {
		t.Errorf("modB was not copied to %s", modBInDep)
	}
	if _, err := os.Stat(modCInDep); err == nil {
		t.Errorf("modC was copied to %s but it was not referenced!", modCInDep)
	}

	// 7. Run terraform init
	cmd := exec.Command("terraform", "init")
	cmd.Dir = groupDir

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("terraform init failed: %v\nOutput:\n%s", err, string(output))
	}

	t.Logf("terraform init succeeded:\n%s", string(output))
}
