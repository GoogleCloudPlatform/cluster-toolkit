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
	"log"
	"os"
	"path"
	"strings"
	"text/template"

	"hpc-toolkit/pkg/config"
)

// TFWriter writes terraform to the blueprint folder
type TFWriter struct {
	numResources int
}

// interfaceStruct is a struct wrapper for converting interface data structures
// to yaml flow style: one line wrapped in {} for maps and [] for lists.
type interfaceStruct struct {
	Elem interface{} `yaml:",flow"`
}

// GetNumResources getter for resource count
func (w *TFWriter) getNumResources() int {
	return w.numResources
}

// AddNumResources add value to resource count
func (w *TFWriter) addNumResources(value int) {
	w.numResources += value
}

// prepareToWrite makes any resource kind specific changes to the config before
// writing to the blueprint directory
func (w TFWriter) prepareToWrite(yamlConfig *config.YamlConfig) {
	updateStringsInConfig(yamlConfig, "terraform")
	flattenToHCLStrings(yamlConfig, "terraform")
}

// writeResourceLevel writes any needed files to the resource layer
func (w TFWriter) writeResourceLevel(yamlConfig *config.YamlConfig) {
}

func getType(obj interface{}) string {
	// This does not handle variables with arbitrary types
	str, ok := obj.(string)
	if !ok { // We received a nil value.
		return "null"
	}
	if strings.HasPrefix(str, "{") {
		return "map"
	}
	if strings.HasPrefix(str, "[") {
		return "list"
	}
	return "string"
}

func writeTopTerraformFile(
	blueprintName string,
	resourceGroup string,
	tmplFilename string,
	data interface{}) {
	tmplText := getTemplate(fmt.Sprintf("%s.tmpl", tmplFilename))

	funcMap := template.FuncMap{
		"getType": getType,
	}
	tmpl, err := template.New(tmplFilename).Funcs(funcMap).Parse(tmplText)

	if err != nil {
		log.Fatalf("TFWriter: %v", err)
	}
	if tmpl == nil {
		log.Fatalf("TFWriter: Failed to parse the %s template.", tmplFilename)
	}

	outputPath := path.Join(blueprintName, resourceGroup, tmplFilename)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf(
			"Couldn't create top-layer %s, does it already exist? %v",
			err, tmplFilename)
	}
	if err := tmpl.Execute(outputFile, data); err != nil {
		log.Fatalf("error writing %s template: %v", tmplFilename, err)
	}

}

// writeTopLevel writes any needed files to the top layer of the blueprint
func (w TFWriter) writeTopLevels(yamlConfig *config.YamlConfig) {
	bpName := yamlConfig.BlueprintName
	for _, resGroup := range yamlConfig.ResourceGroups {
		if !resGroup.HasKind("terraform") {
			continue
		}
		writeTopTerraformFile(bpName, resGroup.Name, "main.tf", resGroup)
		writeTopTerraformFile(bpName, resGroup.Name, "outputs.tf", nil)
		writeTopTerraformFile(
			bpName, resGroup.Name, "providers.tf", yamlConfig.Vars)
		writeTopTerraformFile(
			bpName, resGroup.Name, "variables.tf", yamlConfig.Vars)
		writeTopTerraformFile(bpName, resGroup.Name, "versions.tf", nil)
		writeTopTerraformFile(
			bpName, resGroup.Name, "terraform.tfvars", yamlConfig.Vars)
	}
}
