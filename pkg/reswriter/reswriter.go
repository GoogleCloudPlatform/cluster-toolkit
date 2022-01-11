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
	"fmt"
	"hpc-toolkit/pkg/config"
	"log"
	"os"
	"path"
	"strings"

	"hpc-toolkit/pkg/resutils"

	"github.com/otiai10/copy"
)

const (
	beginLiteralExp string = `^\(\(.*$`
	fullLiteralExp  string = `^\(\((.*)\)\)$`
)

// ResourceFS contains embedded resources (./resources) for use in building
// blueprints. The main package creates and injects the resources directory as
// hpc-toolkit/resources are not accessible at the package level.
var ResourceFS embed.FS

// ResWriter interface for writing resources to a blueprint
type ResWriter interface {
	getNumResources() int
	addNumResources(int)
	writeResourceGroups(*config.YamlConfig, [][]map[string]string) error
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

func copyEmbedded(fs resutils.BaseFS, source string, dest string) error {
	return resutils.CopyDirFromResources(fs, source, dest)
}

func copyFromPath(source string, dest string) error {
	absPath := getAbsSourcePath(source)
	err := copy.Copy(absPath, dest)
	if err != nil {
		return err
	}
	return nil
}

func copySource(blueprintName string, resourceGroups *[]config.ResourceGroup) {
	for iGrp, grp := range *resourceGroups {
		for iRes, resource := range grp.Resources {

			/* Copy source files */
			// currently assuming local or embedded source
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

			// Check source type and copy
			switch src := resource.Source; {
			case resutils.IsLocalPath(src):
				if err = copyFromPath(src, destPath); err != nil {
					log.Fatal(err)
				}
			case resutils.IsEmbeddedPath(src):
				if err = os.MkdirAll(destPath, 0755); err != nil {
					log.Fatalf("failed to create resource path %s: %v", destPath, err)
				}
				if err = copyEmbedded(ResourceFS, src, destPath); err != nil {
					log.Fatal(err)
				}
			default:
				log.Fatalf("resource %s source (%s) not valid, should begin with /, ./, ../ or resources/",
					resource.ID, resource.Source)
			}

			/* Create resource level files */
			writer := factory(resource.Kind)
			writer.addNumResources(1)
		}
	}
}

func printInstructionsPreamble(kind string, path string) {
	fmt.Printf("%s group was successfully created in directory %s\n", kind, path)
	fmt.Println("To deploy, run the following commands:")
}

// WriteBlueprint writes the blueprint using resources defined in config.
func WriteBlueprint(
	yamlConfig *config.YamlConfig, applyFunctions [][]map[string]string,
) {
	createBlueprintDirectory(yamlConfig.BlueprintName)
	copySource(yamlConfig.BlueprintName, &yamlConfig.ResourceGroups)
	for _, writer := range kinds {
		if writer.getNumResources() > 0 {
			err := writer.writeResourceGroups(yamlConfig, applyFunctions)
			if err != nil {
				log.Fatalf("error writing resources to blueprint: %e", err)
			}
		}
	}
}
