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

func printPackerInstructions(modPath string, moduleName string) {
	printInstructionsPreamble("Packer", modPath, moduleName)
	fmt.Printf("  cd %s\n", modPath)
	fmt.Println("  packer init .")
	fmt.Println("  packer validate .")
	fmt.Println("  packer build .")
	fmt.Printf("  cd -\n\n")
}

func writePackerAutovars(vars map[string]cty.Value, dst string) error {
	packerAutovarsPath := filepath.Join(dst, packerAutoVarFilename)
	err := writeHclAttributes(vars, packerAutovarsPath)
	return err
}

// writeDeploymentGroup writes any needed files to the top and module levels
// of the blueprint
func (w PackerWriter) writeDeploymentGroup(
	dc config.DeploymentConfig,
	grpIdx int,
	deployDir string,
) (groupMetadata, error) {
	ctyGlobals, err := config.ConvertMapToCty(dc.Config.Vars)
	if err != nil {
		return groupMetadata{}, fmt.Errorf(
			"error converting deployment vars to cty for writing: %w", err)
	}

	depGroup := dc.Config.DeploymentGroups[grpIdx]
	groupPath := filepath.Join(deployDir, depGroup.Name)
	allInputs := make(map[string]bool)
	for _, mod := range depGroup.Modules {

		ctySettings, err := config.ConvertMapToCty(mod.Settings)
		for k := range ctySettings {
			allInputs[k] = true
		}

		if err != nil {
			return groupMetadata{}, fmt.Errorf(
				"error converting packer module settings to cty for writing: %w", err)
		}
		err = config.ResolveVariables(ctySettings, ctyGlobals)
		if err != nil {
			return groupMetadata{}, err
		}
		modPath := filepath.Join(groupPath, mod.DeploymentSource)
		err = writePackerAutovars(ctySettings, modPath)
		if err != nil {
			return groupMetadata{}, err
		}
		printPackerInstructions(modPath, mod.ID)
	}

	return groupMetadata{
		Name:    depGroup.Name,
		Inputs:  orderKeys(allInputs),
		Outputs: []string{},
	}, nil
}

func (w PackerWriter) restoreState(deploymentDir string) error {
	// TODO: implement state restoration for Packer
	return nil
}
