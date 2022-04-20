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
	numResources int
}

func (w *PackerWriter) getNumResources() int {
	return w.numResources
}

func (w *PackerWriter) addNumResources(value int) {
	w.numResources += value
}

func printPackerInstructions(grpPath string) {
	printInstructionsPreamble("Packer", grpPath)
	fmt.Printf("  cd %s\n", grpPath)
	fmt.Println("  packer init .")
	fmt.Println("  packer validate .")
	fmt.Println("  packer build .")
}

// writeResourceLevel writes any needed files to the resource layer
func (w PackerWriter) writeResourceLevel(yamlConfig *config.YamlConfig, bpDirectory string) error {
	for _, grp := range yamlConfig.ResourceGroups {
		groupPath := filepath.Join(bpDirectory, yamlConfig.BlueprintName, grp.Name)
		for _, res := range grp.Resources {
			if res.Kind != "packer" {
				continue
			}

			ctySettings, err := config.ConvertMapToCty(res.Settings)

			if err != nil {
				return fmt.Errorf(
					"error converting global vars to cty for writing: %v", err)
			}
			err = yamlConfig.ResolveGlobalVariables(ctySettings)
			if err != nil {
				return err
			}
			resPath := filepath.Join(groupPath, res.ID)
			err = writePackerAutovars(ctySettings, resPath)
			if err != nil {
				return err
			}
			printPackerInstructions(resPath)
		}
	}
	return nil
}

func writePackerAutovars(vars map[string]cty.Value, dst string) error {
	packerAutovarsPath := filepath.Join(dst, packerAutoVarFilename)
	err := writeHclAttributes(vars, packerAutovarsPath)
	return err
}

// writeResourceGroups writes any needed files to the top and resource levels
// of the blueprint
func (w PackerWriter) writeResourceGroups(yamlConfig *config.YamlConfig, bpDirectory string) error {
	return w.writeResourceLevel(yamlConfig, bpDirectory)
}

func (w PackerWriter) restoreState(bpDir string) error {
	// TODO: implement state restoration for Packer
	return nil
}
