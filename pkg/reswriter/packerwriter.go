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

func printPackerInstructions(grpPath string) {
	printInstructionsPreamble("Packer", grpPath)
	fmt.Printf("  cd %s\n", grpPath)
	fmt.Println("  packer init .")
	fmt.Println("  packer validate .")
	fmt.Println("  packer build .")
}

// writeModuleLevel writes any needed files to the module layer
func (w PackerWriter) writeModuleLevel(blueprint *config.Blueprint, outputDir string) error {
	for _, grp := range blueprint.DeploymentGroups {
		deploymentName, err := blueprint.DeploymentName()
		if err != nil {
			return err
		}
		groupPath := filepath.Join(outputDir, deploymentName, grp.Name)
		for _, mod := range grp.Modules {
			if mod.Kind != "packer" {
				continue
			}

			ctySettings, err := config.ConvertMapToCty(mod.Settings)

			if err != nil {
				return fmt.Errorf(
					"error converting global vars to cty for writing: %v", err)
			}
			err = blueprint.ResolveGlobalVariables(ctySettings)
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
	}
	return nil
}

func writePackerAutovars(vars map[string]cty.Value, dst string) error {
	packerAutovarsPath := filepath.Join(dst, packerAutoVarFilename)
	err := writeHclAttributes(vars, packerAutovarsPath)
	return err
}

// writeDeploymentGroups writes any needed files to the top and module levels
// of the blueprint
func (w PackerWriter) writeDeploymentGroups(blueprint *config.Blueprint, outputDir string) error {
	return w.writeModuleLevel(blueprint, outputDir)
}

func (w PackerWriter) restoreState(deploymentDir string) error {
	// TODO: implement state restoration for Packer
	return nil
}
