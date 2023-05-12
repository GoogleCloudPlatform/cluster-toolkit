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
	"os"
	"path/filepath"

	"hpc-toolkit/pkg/config"

	"github.com/zclconf/go-cty/cty"
)

const packerAutoVarFilename = "defaults.auto.pkrvars.hcl"

// PackerWriter writes packer to the blueprint folder
type PackerWriter struct {
	numModules int
}

func (w *PackerWriter) getNumModules() int {
	return w.numModules
}

func (w *PackerWriter) addNumModules(value int) {
	w.numModules += value
}

func printPackerInstructions(f *os.File, modPath string, modID config.ModuleID, printImportInputs bool) {
	fmt.Fprintln(f)
	fmt.Fprintf(f, "Packer group '%s' was successfully created in directory %s\n", modID, modPath)
	fmt.Fprintln(f, "To deploy, run the following commands:")
	fmt.Fprintln(f)
	grpPath := filepath.Clean(filepath.Join(modPath, ".."))
	if printImportInputs {
		fmt.Fprintf(f, "ghpc import-inputs %s\n", grpPath)
	}
	fmt.Fprintf(f, "cd %s\n", modPath)
	fmt.Fprintln(f, "packer init .")
	fmt.Fprintln(f, "packer validate .")
	fmt.Fprintln(f, "packer build .")
	fmt.Fprintln(f, "cd -")
}

func writePackerAutovars(vars map[string]cty.Value, dst string) error {
	packerAutovarsPath := filepath.Join(dst, packerAutoVarFilename)
	err := WriteHclAttributes(vars, packerAutovarsPath)
	return err
}

// writeDeploymentGroup writes any needed files to the top and module levels
// of the blueprint
func (w PackerWriter) writeDeploymentGroup(
	dc config.DeploymentConfig,
	grpIdx int,
	deployDir string,
	instructionsFile *os.File,
) error {
	depGroup := dc.Config.DeploymentGroups[grpIdx]
	groupPath := filepath.Join(deployDir, string(depGroup.Name))
	igcInputs := map[string]bool{}

	for _, mod := range depGroup.Modules {
		pure := config.Dict{}
		for setting, v := range mod.Settings.Items() {
			igcRefs := config.FindIntergroupReferences(v, mod, dc.Config)
			if len(igcRefs) == 0 {
				pure.Set(setting, v)
			}
			for _, r := range igcRefs {
				n := config.AutomaticOutputName(r.Name, r.Module)
				igcInputs[n] = true
			}
		}

		av, err := pure.Eval(dc.Config)
		if err != nil {
			return err
		}

		modPath := filepath.Join(groupPath, mod.DeploymentSource)
		if err = writePackerAutovars(av.Items(), modPath); err != nil {
			return err
		}
		hasIgc := len(pure.Items()) < len(mod.Settings.Items())
		printPackerInstructions(instructionsFile, modPath, mod.ID, hasIgc)
	}

	return nil
}

func (w PackerWriter) restoreState(deploymentDir string) error {
	// TODO: implement state restoration for Packer
	return nil
}

func (w PackerWriter) kind() config.ModuleKind {
	return config.PackerKind
}
