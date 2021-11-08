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
	"io/fs"
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

// ResourceFS contains embedded resources (./resources) for use in building
// blueprints. The main package creates and injects the resources directory as
// hpc-toolkit/resources are not accessible at the package level.
var ResourceFS embed.FS

// ResWriter interface for writing resources to a blueprint
type ResWriter interface {
	getNumResources() int
	addNumResources(int)
	writeResourceGroups(*config.YamlConfig) error
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

type baseFS interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

func copyDirFromResources(fs baseFS, source string, dest string) error {
	dirEntries, err := fs.ReadDir(source)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		entryName := dirEntry.Name()
		entrySource := path.Join(source, entryName)
		entryDest := path.Join(dest, entryName)
		if dirEntry.IsDir() {
			if err := os.Mkdir(entryDest, 0755); err != nil {
				return err
			}
			if err = copyDirFromResources(fs, entrySource, entryDest); err != nil {
				return err
			}
		} else {
			fileBytes, err := fs.ReadFile(entrySource)
			if err != nil {
				return err
			}
			copyFile, err := os.Create(entryDest)
			if err != nil {
				return nil
			}
			if _, err = copyFile.Write(fileBytes); err != nil {
				return nil
			}
		}
	}
	return nil
}

func copyEmbedded(fs baseFS, source string, dest string) error {
	return copyDirFromResources(fs, source, dest)
}

func copyFromPath(source string, dest string) error {
	absPath := getAbsSourcePath(source)
	err := copy.Copy(absPath, dest)
	if err != nil {
		return err
	}
	return nil
}

func isLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/")
}

func isEmbeddedPath(source string) bool {
	return strings.HasPrefix(source, "resources/")
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
			case isLocalPath(src):
				if err = copyFromPath(src, destPath); err != nil {
					log.Fatal(err)
				}
			case isEmbeddedPath(src):
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

// WriteBlueprint writes the blueprint using resources defined in config.
func WriteBlueprint(yamlConfig *config.YamlConfig) {
	createBlueprintDirectory(yamlConfig.BlueprintName)
	copySource(yamlConfig.BlueprintName, &yamlConfig.ResourceGroups)
	for _, writer := range kinds {
		if writer.getNumResources() > 0 {
			err := writer.writeResourceGroups(yamlConfig)
			if err != nil {
				log.Fatalf("error writing resources to blueprint: %e", err)
			}
		}
	}
}
