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
	"os"
	"path"
	"text/template"

	"hpc-toolkit/pkg/config"
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
	flattenToHCLStrings(yamlConfig, "pakcer")
}

// writeResourceLevel writes any needed files to the resource layer
func (w PackerWriter) writeResourceLevel(yamlConfig *config.YamlConfig) error {
	for _, grp := range yamlConfig.ResourceGroups {
		groupPath := path.Join(yamlConfig.BlueprintName, grp.Name)
		for _, res := range grp.Resources {
			if res.Kind != "packer" {
				continue
			}
			resPath := path.Join(groupPath, res.ID)
			return writePackerAutoVariables(packerAutoVarFilename, res, resPath)
		}
	}
	return nil
}

func writePackerAutoVariables(
	tmplFilename string, resource config.Resource, destPath string) error {
	tmplText := getTemplate(fmt.Sprintf("%s.tmpl", tmplFilename))

	funcMap := template.FuncMap{
		"getType": getType,
	}
	tmpl, err := template.New(tmplFilename).Funcs(funcMap).Parse(tmplText)

	if err != nil {
		return fmt.Errorf(
			"failed to create template %s when writing packer resource at %s: %v",
			tmplFilename, resource.Source, err)
	}
	if tmpl == nil {
		return fmt.Errorf(
			"failed to parse the %s template", tmplFilename)
	}

	outputPath := path.Join(destPath, tmplFilename)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf(
			"failed to create packer file %s: %v", tmplFilename, err)
	}
	if err := tmpl.Execute(outputFile, resource); err != nil {
		return fmt.Errorf(
			"failed to write template for %s file when writing packer resource %s: %e",
			tmplFilename, resource.ID, err)
	}
	return nil
}

// writeTopLevel writes any needed files to the top layer of the blueprint
func (w PackerWriter) writeTopLevels(yamlConfig *config.YamlConfig) {
}
