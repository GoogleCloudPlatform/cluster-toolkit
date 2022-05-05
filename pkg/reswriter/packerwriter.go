/**
* Copyright 2021 Google LLC
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

package reswriter

import (
	"fmt"
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

func printPackerInstructions(modPath string) {
	printInstructionsPreamble("Packer", modPath)
	fmt.Printf("  cd %s\n", modPath)
	fmt.Println("  packer init .")
	fmt.Println("  packer validate .")
	fmt.Println("  packer build .")
}

func writePackerAutovars(vars map[string]cty.Value, dst string) error {
	packerAutovarsPath := filepath.Join(dst, packerAutoVarFilename)
	err := writeHclAttributes(vars, packerAutovarsPath)
	return err
}

// writeDeploymentGroup writes any needed files to the top and module levels
// of the blueprint
func (w PackerWriter) writeDeploymentGroup(
	depGroup config.DeploymentGroup,
	globalVars map[string]interface{},
	deployDir string,
) error {
	ctyVars, err := config.ConvertMapToCty(globalVars)
	if err != nil {
		return fmt.Errorf(
			"error converting global vars to cty for writing: %w", err)
	}
	groupPath := filepath.Join(deployDir, depGroup.Name)
	for _, mod := range depGroup.Modules {

		ctySettings, err := config.ConvertMapToCty(mod.Settings)

		if err != nil {
			return fmt.Errorf(
				"error converting packer module settings to cty for writing: %w", err)
		}
		err = config.ResolveGlobalVariables(ctySettings, ctyVars)
		if err != nil {
			return err
		}
		modPath := filepath.Join(groupPath, mod.ID)
		err = writePackerAutovars(ctySettings, modPath)
		if err != nil {
			return err
		}
		printPackerInstructions(modPath)
	}
	return nil
}

func (w PackerWriter) restoreState(deploymentDir string) error {
	// TODO: implement state restoration for Packer
	return nil
}
