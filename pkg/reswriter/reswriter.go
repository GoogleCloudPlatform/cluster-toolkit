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
	"hpc-toolkit/pkg/blueprintio"
	"hpc-toolkit/pkg/config"
	"log"
	"os"
	"path"

	"hpc-toolkit/pkg/resutils"
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
	writeResourceGroups(*config.YamlConfig, string) error
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

func copySource(blueprintPath string, resourceGroups *[]config.ResourceGroup) {
	blueprintio := blueprintio.GetBlueprintIOLocal()
	for iGrp, grp := range *resourceGroups {
		for iRes, resource := range grp.Resources {

			/* Copy source files */
			// currently assuming local or embedded source
			resourceName := path.Base(resource.Source)
			(*resourceGroups)[iGrp].Resources[iRes].ResourceName = resourceName
			basePath := path.Join(blueprintPath, grp.Name)
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
				if err = blueprintio.CopyFromPath(src, destPath); err != nil {
					log.Fatal(err)
				}
			case resutils.IsEmbeddedPath(src):
				if err = blueprintio.CreateDirectory(destPath); err != nil {
					log.Fatalf("failed to create resource path %s: %v", destPath, err)
				}
				if err = copyEmbedded(ResourceFS, src, destPath); err != nil {
					log.Fatal(err)
				}
			case resutils.IsGitHubPath(src):
				if err = resutils.CopyGitHubResources(src, destPath); err != nil {
					log.Fatalf("failed to git clone from source %s to dest %s because %v", src, destPath, err)
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
func WriteBlueprint(yamlConfig *config.YamlConfig, bpDirectory string) {
	blueprintio := blueprintio.GetBlueprintIOLocal()
	bpDirectoryPath := path.Join(bpDirectory, yamlConfig.BlueprintName)
	if err := blueprintio.CreateDirectory(bpDirectoryPath); err != nil {
		log.Fatalf("failed to create a directory for blueprints: %v", err)
	}

	copySource(bpDirectoryPath, &yamlConfig.ResourceGroups)
	for _, writer := range kinds {
		if writer.getNumResources() > 0 {
			err := writer.writeResourceGroups(yamlConfig, bpDirectory)
			if err != nil {
				log.Fatalf("error writing resources to blueprint: %v", err)
			}
		}
	}
}
