/**
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

package modulewriter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"

	"github.com/zclconf/go-cty/cty"
)

const packerAutoVarFilename = "defaults.auto.pkrvars.hcl"

// PackerWriter writes packer to the blueprint folder
type PackerWriter struct{}

func printPackerInstructions(w io.Writer, groupPath string, subPath string, printImportInputs bool) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Packer group was successfully created in directory %s\n", groupPath)
	fmt.Fprintln(w, "To deploy, run the following commands:")
	fmt.Fprintln(w)
	if printImportInputs {
		fmt.Fprintf(w, "gcluster import-inputs %s\n", groupPath)
	}
	fmt.Fprintf(w, "cd %s\n", filepath.Join(groupPath, subPath))
	fmt.Fprintln(w, "packer init .")
	fmt.Fprintln(w, "packer validate .")
	fmt.Fprintln(w, "packer build .")
	fmt.Fprintln(w, "cd -")
}

func writePackerAutovars(vars map[string]cty.Value, dst string) error {
	return WriteHclAttributes(vars, filepath.Join(dst, packerAutoVarFilename))
}

// writeGroup writes any needed files to the top and module levels
// of the blueprint
func (w PackerWriter) writeGroup(
	bp config.Blueprint,
	grpIdx int,
	groupPath string,
	instructionsFile io.Writer,
) error {
	mod := bp.Groups[grpIdx].Modules[0] // packer groups only have one module

	pure := map[string]cty.Value{}
	for setting, v := range mod.Settings.Items() {
		if len(config.FindIntergroupReferences(v, mod, bp)) == 0 {
			pure[setting] = v
		}
	}
	av, err := bp.EvalDict(config.NewDict(pure))
	if err != nil {
		return err
	}

	ds, err := DeploymentSource(mod)
	if err != nil {
		return err
	}
	modPath := filepath.Join(groupPath, ds)
	if err = writePackerAutovars(av.Items(), modPath); err != nil {
		return err
	}
	hasIgc := len(pure) < len(mod.Settings.Items())
	printPackerInstructions(instructionsFile, groupPath, ds, hasIgc)

	return nil
}

const packerManifestFileName = "packer-manifest.json"

func (w PackerWriter) restoreState(deploymentDir string) error {
	prevGroupPath := filepath.Join(HiddenGhpcDir(deploymentDir), prevGroupDirName)

	// If the previous deployment groups directory doesn't exist, there is nothing to restore.
	if _, err := os.Stat(prevGroupPath); os.IsNotExist(err) {
		return nil
	}

	err := filepath.WalkDir(prevGroupPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != packerManifestFileName {
			return nil
		}

		// Found a packer-manifest.json. Determine its relative path from prevGroupPath.
		relPath, err := filepath.Rel(prevGroupPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		dest := filepath.Join(deploymentDir, relPath)

		// Ensure the destination directory exists before writing (it should have been created by writeGroup).
		_, err = os.Stat(filepath.Dir(dest))
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("failed to stat destination directory %s: %w", filepath.Dir(dest), err)
		}

		bytesRead, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read previous packer manifest %s: %w", path, err)
		}
		err = os.WriteFile(dest, bytesRead, 0644)
		if err != nil {
			return fmt.Errorf("failed to write previous packer manifest %s: %w", dest, err)
		}
		logging.Info("Restored packer manifest to %s", dest)

		return nil
	})

	if err != nil {
		return fmt.Errorf("error trying to restore packer manifests: %w", err)
	}

	return nil
}

func (w PackerWriter) kind() config.ModuleKind {
	return config.PackerKind
}
