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
	"hpc-toolkit/pkg/config"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
)

const packerAutoVarFilename = "variables.auto.pkrvars.hcl"

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

// prepareToWrite makes any resource kind specific changes to the config before
// writing to the blueprint directory
func (w PackerWriter) prepareToWrite(yamlConfig *config.YamlConfig) {
	updateStringsInConfig(yamlConfig, "packer")
	flattenToHCLStrings(yamlConfig, "packer")
}

func printPackerInstructions(grpPath string) {
	printInstructionsPreamble("Packer", grpPath)
	fmt.Printf("  cd %s\n", grpPath)
	fmt.Println("  packer build image.pkr.hcl")
}

// writeResourceLevel writes any needed files to the resource layer
func (w PackerWriter) writeResourceLevel(yamlConfig *config.YamlConfig, bpDirectory string) error {
	for _, grp := range yamlConfig.ResourceGroups {
		groupPath := filepath.Join(bpDirectory, yamlConfig.BlueprintName, grp.Name)
		for _, res := range grp.Resources {
			if res.Kind != "packer" {
				continue
			}
			ctyVars, err := convertMapToCty(res.Settings)
			if err != nil {
				return fmt.Errorf(
					"error converting global vars to cty for writing: %v", err)
			}
			resPath := filepath.Join(groupPath, res.ID)
			err = w.writePackerAutovars(ctyVars, resPath)
			if err != nil {
				return err
			}
			printPackerInstructions(resPath)
		}
	}
	return nil
}

func (w PackerWriter) writePackerAutovars(vars map[string]cty.Value, dst string) error {
	packerAutovarsPath := filepath.Join(dst, packerAutoVarFilename)
	err := writeHclAttributes(vars, packerAutovarsPath)
	return err
}

// writeResourceGroups writes any needed files to the top and resource levels
// of the blueprint
func (w PackerWriter) writeResourceGroups(yamlConfig *config.YamlConfig, bpDirectory string) error {
	w.prepareToWrite(yamlConfig)
	return w.writeResourceLevel(yamlConfig, bpDirectory)
}
