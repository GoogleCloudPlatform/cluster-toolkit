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

// Package reswriter writes resources to a blueprint directory
package reswriter

import (
	"fmt"
	"log"
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
func (w PackerWriter) writeResourceLevel(yamlConfig *config.YamlConfig) {
	for _, grp := range yamlConfig.ResourceGroups {
		groupPath := path.Join(yamlConfig.BlueprintName, grp.Name)
		for _, res := range grp.Resources {
			if res.Kind != "packer" {
				continue
			}
			resPath := path.Join(groupPath, res.ID)
			writePackerAutoVariables(packerAutoVarFilename, res, resPath)
		}
	}

}

func writePackerAutoVariables(tmplFilename string, resource config.Resource, destPath string) {
	tmplText := getTemplate(fmt.Sprintf("%s.tmpl", tmplFilename))

	funcMap := template.FuncMap{
		"getType": getType,
	}
	tmpl, err := template.New(tmplFilename).Funcs(funcMap).Parse(tmplText)

	if err != nil {
		log.Fatalf("PackerWriter: %v", err)
	}
	if tmpl == nil {
		log.Fatalf("PackerWriter: Failed to parse the %s template.", tmplFilename)
	}

	outputPath := path.Join(destPath, tmplFilename)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf(
			"Couldn't create top-layer %s, does it already exist? %v",
			err, tmplFilename)
	}
	tmpl.Execute(outputFile, resource)
}

// writeTopLevel writes any needed files to the top layer of the blueprint
func (w PackerWriter) writeTopLevels(yamlConfig *config.YamlConfig) {
}
