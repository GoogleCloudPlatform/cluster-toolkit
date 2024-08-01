/**
* Copyright 2022 Google LLC
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
	"path/filepath"

	"hpc-toolkit/pkg/config"

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

func (w PackerWriter) restoreState(deploymentDir string) error {
	// TODO: restore packer-manifest.json if it exists
	return nil
}

func (w PackerWriter) kind() config.ModuleKind {
	return config.PackerKind
}
