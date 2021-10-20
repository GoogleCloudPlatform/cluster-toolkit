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
	"embed"
	"hpc-toolkit/pkg/config"
	"log"
	"os"
	"path"
	"strings"

	"github.com/otiai10/copy"
)

const (
	beginPassthroughExp string = `^\(\(.*$`
	fullPassthroughExp  string = `^\(\((.*)\)\)$`
)

// ResWriter interface for writing resources to a blueprint
type ResWriter interface {
	getNumResources() int
	addNumResources(int)
	prepareToWrite(*config.YamlConfig)
	writeResourceLevel(*config.YamlConfig) error
	writeTopLevels(*config.YamlConfig)
}

var kinds = map[string]ResWriter{
	"terraform": new(TFWriter),
	"packer":    new(PackerWriter),
}

//go:embed *.tmpl
var templatesFS embed.FS

func factory(kind string) ResWriter {
	writer, exists := kinds[kind]
	if !exists {
		log.Fatalf(
			"reswriter: Resource kind (%s) is not valid. "+
				"kind must be in (terraform, blueprint-controller).", kind)
	}
	return writer
}

func mkdirWrapper(path string) {
	err := os.Mkdir(path, 0755)
	if err != nil {
		log.Fatalf("createBlueprintDirectory: %v", err)
	}
}

func createBlueprintDirectory(blueprintName string) {
	if _, err := os.Stat(blueprintName); !os.IsNotExist(err) {
		log.Fatalf(
			"reswriter: Blueprint directory already exists: %s", blueprintName)
	}
	// Create blueprint directory
	mkdirWrapper(blueprintName)
}

func getAbsSourcePath(sourcePath string) string {
	if strings.HasPrefix(sourcePath, "/") { // Absolute Path Already
		return sourcePath
	}
	// Otherwise base it off of the CWD
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("reswriter: %v", err)
	}
	return path.Join(cwd, sourcePath)
}

func getTemplate(filename string) string {
	// Create path to template from the embedded template FS
	tmplText, err := templatesFS.ReadFile(filename)
	if err != nil {
		log.Fatalf("reswriter: %v", err)
	}
	return string(tmplText)
}

func copySource(blueprintName string, resourceGroups *[]config.ResourceGroup) {
	for iGrp, grp := range *resourceGroups {
		for iRes, resource := range grp.Resources {

			/* Copy source files */
			// currently assuming local source only
			resourceName := path.Base(resource.Source)
			(*resourceGroups)[iGrp].Resources[iRes].ResourceName = resourceName
			basePath := path.Join(blueprintName, grp.Name)
			var destPath string
			switch resource.Kind {
			case "terraform":
				destPath = path.Join(basePath, "modules", resourceName)
			case "packer":
				destPath = path.Join(basePath, resource.ID)
			}
			_, err := os.Stat(destPath)
			if err == nil {
				continue
			}
			sourcePath := getAbsSourcePath(resource.Source)
			err = copy.Copy(sourcePath, destPath)
			if err != nil {
				log.Fatalf("reswriter: Failed to copy resource %s: %v", resource.ID, err)
			}

			/* Create resource level files */
			writer := factory(resource.Kind)
			writer.addNumResources(1)
		}
	}
}

// WriteBlueprint writes the blueprint using resources defined in config.
func WriteBlueprint(yamlConfig *config.YamlConfig) {
	createBlueprintDirectory(yamlConfig.BlueprintName)
	copySource(yamlConfig.BlueprintName, &yamlConfig.ResourceGroups)
	err := updateStringsInMap(yamlConfig.Vars)
	if err != nil {
		log.Fatalf("updateStringsInConfig: %v", err)
	}
	wrapper := interfaceStruct{Elem: nil}
	err = flattenInterfaceMap(yamlConfig.Vars, &wrapper)
	if err != nil {
		log.Fatalf("Error flattening data structures in vars: %v", err)
	}
	for _, writer := range kinds {
		writer.prepareToWrite(yamlConfig)
		if writer.getNumResources() > 0 {
			writer.writeTopLevels(yamlConfig)
			err = writer.writeResourceLevel(yamlConfig)
			if err != nil {
				log.Fatalf("error writing resources to blueprint: %e", err)
			}
		}
	}
}
