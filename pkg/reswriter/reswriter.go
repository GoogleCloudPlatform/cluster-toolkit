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
	"hpc-toolkit/pkg/blueprintio"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/sourcereader"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

const (
	hiddenGhpcDirName        = ".ghpc"
	prevResourceGroupDirName = "previous_resource_groups"
	tfStateFileName          = "terraform.tfstate"
)

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

func factory(kind string) ResWriter {
	writer, exists := kinds[kind]
	if !exists {
		log.Fatalf(
			"reswriter: Resource kind (%s) is not valid. "+
				"kind must be in (terraform, blueprint-controller).", kind)
	}
	return writer
}

// WriteBlueprint writes the blueprint using resources defined in config.
func WriteBlueprint(yamlConfig *config.YamlConfig, outputDir string) error {
	bpDir := filepath.Join(outputDir, yamlConfig.BlueprintName)
	if err := prepBpDir(bpDir, false /* overwrite */); err != nil {
		return err
	}

	copySource(bpDir, &yamlConfig.ResourceGroups)
	for _, writer := range kinds {
		if writer.getNumResources() > 0 {
			err := writer.writeResourceGroups(yamlConfig, outputDir)
			if err != nil {
				return fmt.Errorf("error writing resources to blueprint: %w", err)
			}
		}
	}

	if err := restoreTfState(bpDir); err != nil {
		return fmt.Errorf("Error trying to restore terraform state: %w", err)
	}

	return nil
}

func copySource(blueprintPath string, resourceGroups *[]config.ResourceGroup) {
	for iGrp, grp := range *resourceGroups {
		for iRes, resource := range grp.Resources {
			if sourcereader.IsGitHubPath(resource.Source) {
				continue
			}

			/* Copy source files */
			resourceName := filepath.Base(resource.Source)
			(*resourceGroups)[iGrp].Resources[iRes].ResourceName = resourceName
			basePath := filepath.Join(blueprintPath, grp.Name)
			var destPath string
			switch resource.Kind {
			case "terraform":
				destPath = filepath.Join(basePath, "modules", resourceName)
			case "packer":
				destPath = filepath.Join(basePath, resource.ID)
			}
			_, err := os.Stat(destPath)
			if err == nil {
				continue
			}

			reader := sourcereader.Factory(resource.Source)
			if err := reader.GetResource(resource.Source, destPath); err != nil {
				log.Fatalf("failed to get resource from %s to %s: %v", resource.Source, destPath, err)
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

// Prepares a blueprint directory to be written to.
func prepBpDir(bpDir string, overwrite bool) error {
	blueprintIO := blueprintio.GetBlueprintIOLocal()
	ghpcDir := filepath.Join(bpDir, hiddenGhpcDirName)

	// create blueprint directory
	if err := blueprintIO.CreateDirectory(bpDir); err != nil {
		if !overwrite {
			// TODO: Update error message to reference command line flag once feature is launched
			return fmt.Errorf("Blueprint direct failed to create a directory for blueprints: %w", err)
		}

		// Confirm we have a previously written blueprint dir before overwritting.
		if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
			return fmt.Errorf(
				"While trying to overwrite %s, the '.ghpc/' dir could not be found: %w",
				bpDir, err)
		}
	} else {
		blueprintIO.CreateDirectory(ghpcDir)
	}

	// clean up old dirs
	prevGroupDir := filepath.Join(ghpcDir, prevResourceGroupDirName)
	os.RemoveAll(prevGroupDir)
	if err := os.MkdirAll(prevGroupDir, 0755); err != nil {
		return fmt.Errorf("Failed to create the directory %s: %v", prevGroupDir, err)
	}

	// move resource groups
	files, err := ioutil.ReadDir(bpDir)
	if err != nil {
		return fmt.Errorf("Error trying to read directories in %s, %w", bpDir, err)
	}
	for _, f := range files {
		if !f.IsDir() || f.Name() == hiddenGhpcDirName {
			continue
		}
		src := filepath.Join(bpDir, f.Name())
		dest := filepath.Join(prevGroupDir, f.Name())
		if err := os.Rename(src, dest); err != nil {
			return fmt.Errorf("Error while moving old resource groups: %w", err)
		}
	}
	return nil
}

func restoreTfState(bpDir string) error {
	prevResourceGroupPath := filepath.Join(bpDir, hiddenGhpcDirName, prevResourceGroupDirName)
	files, err := ioutil.ReadDir(prevResourceGroupPath)
	if err != nil {
		return fmt.Errorf("Error trying to read previous resources in %s, %w", prevResourceGroupPath, err)
	}

	for _, f := range files {
		src := filepath.Join(prevResourceGroupPath, f.Name(), tfStateFileName)
		dest := filepath.Join(bpDir, f.Name(), tfStateFileName)

		if bytesRead, err := ioutil.ReadFile(src); err == nil {
			err = ioutil.WriteFile(dest, bytesRead, 0644)
			if err != nil {
				return fmt.Errorf("Failed to write previous state file %s, %w", dest, err)
			}
		}
	}
	return nil
}
