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

// Package reswriter writes modules to a blueprint directory
package reswriter

import (
	"embed"
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
	hiddenGhpcDirName          = ".ghpc"
	prevDeploymentGroupDirName = "previous_resource_groups"
	gitignoreTemplate          = "blueprint.gitignore.tmpl"
)

// ModWriter interface for writing modules to a blueprint
type ModWriter interface {
	getNumModules() int
	addNumModules(int)
	writeDeploymentGroup(
		depGroup config.DeploymentGroup,
		globalVars map[string]interface{},
		deployDir string,
	) error
	restoreState(deploymentDir string) error
}

var kinds = map[string]ModWriter{
	"terraform": new(TFWriter),
	"packer":    new(PackerWriter),
}

//go:embed *.tmpl
var templatesFS embed.FS

func factory(kind string) ModWriter {
	writer, exists := kinds[kind]
	if !exists {
		log.Fatalf(
			"reswriter: Module kind (%s) is not valid. "+
				"kind must be in (terraform, blueprint-controller).", kind)
	}
	return writer
}

// WriteBlueprint writes a deployment directory using modules defined the environment blueprint.
func WriteBlueprint(blueprint *config.Blueprint, outputDir string, overwriteFlag bool) error {
	deploymentName, err := blueprint.DeploymentName()
	if err != nil {
		return err
	}
	deploymentDir := filepath.Join(outputDir, deploymentName)

	overwrite := isOverwriteAllowed(deploymentDir, blueprint, overwriteFlag)
	if err := prepBpDir(deploymentDir, overwrite); err != nil {
		return err
	}

	copySource(deploymentDir, &blueprint.DeploymentGroups)

	for _, grp := range blueprint.DeploymentGroups {

		deploymentName, err := blueprint.DeploymentName()
		if err != nil {
			return err
		}

		deploymentPath := filepath.Join(outputDir, deploymentName)
		writer, ok := kinds[grp.Kind]
		if !ok {
			return fmt.Errorf(
				"Invalid kind in deployment group %s, got '%s'", grp.Name, grp.Kind)
		}

		if err := writer.writeDeploymentGroup(
			grp, blueprint.Vars, deploymentPath,
		); err != nil {
			return fmt.Errorf("error writing deployment group %s: %w", grp.Name, err)
		}
	}

	for _, writer := range kinds {
		if writer.getNumModules() > 0 {
			if err := writer.restoreState(deploymentDir); err != nil {
				return fmt.Errorf("error trying to restore terraform state: %w", err)
			}
		}
	}
	return nil
}

func copySource(blueprintPath string, deploymentGroups *[]config.DeploymentGroup) {
	for iGrp, grp := range *deploymentGroups {
		for iMod, module := range grp.Modules {
			if sourcereader.IsGitHubPath(module.Source) {
				continue
			}

			/* Copy source files */
			moduleName := filepath.Base(module.Source)
			(*deploymentGroups)[iGrp].Modules[iMod].ModuleName = moduleName
			basePath := filepath.Join(blueprintPath, grp.Name)
			var destPath string
			switch module.Kind {
			case "terraform":
				destPath = filepath.Join(basePath, "modules", moduleName)
			case "packer":
				destPath = filepath.Join(basePath, module.ID)
			}
			_, err := os.Stat(destPath)
			if err == nil {
				continue
			}

			reader := sourcereader.Factory(module.Source)
			if err := reader.GetModule(module.Source, destPath); err != nil {
				log.Fatalf("failed to get module from %s to %s: %v", module.Source, destPath, err)
			}

			/* Create module level files */
			writer := factory(module.Kind)
			writer.addNumModules(1)
		}
	}
}

func printInstructionsPreamble(kind string, path string) {
	fmt.Printf("%s group was successfully created in directory %s\n", kind, path)
	fmt.Println("To deploy, run the following commands:")
}

// Determines if overwrite is allowed
func isOverwriteAllowed(bpDir string, overwritingConfig *config.Blueprint, overwriteFlag bool) bool {
	if !overwriteFlag {
		return false
	}

	files, err := ioutil.ReadDir(bpDir)
	if err != nil {
		return false
	}

	// build list of previous and current resource groups
	var prevGroups []string
	for _, f := range files {
		if f.IsDir() && f.Name() != hiddenGhpcDirName {
			prevGroups = append(prevGroups, f.Name())
		}
	}

	var curGroups []string
	for _, group := range overwritingConfig.DeploymentGroups {
		curGroups = append(curGroups, group.Name)
	}

	return isSubset(prevGroups, curGroups)
}

func isSubset(sub, super []string) bool {
	// build set (map keys) from slice
	superM := make(map[string]bool)
	for _, item := range super {
		superM[item] = true
	}

	for _, item := range sub {
		if _, found := superM[item]; !found {
			return false
		}
	}
	return true
}

// OverwriteDeniedError signifies when a blueprint overwrite was denied.
type OverwriteDeniedError struct {
	cause error
}

func (err *OverwriteDeniedError) Error() string {
	return fmt.Sprintf("Failed to overwrite existing blueprint.\n\n"+
		"Use the -w command line argument to enable overwrite.\n"+
		"If overwrite is already enabled then this may be because "+
		"you are attempting to remove a deployment group, which is not supported.\n"+
		"original error: %v",
		err.cause)
}

// Prepares a blueprint directory to be written to.
func prepBpDir(bpDir string, overwrite bool) error {
	blueprintIO := blueprintio.GetBlueprintIOLocal()
	ghpcDir := filepath.Join(bpDir, hiddenGhpcDirName)
	gitignoreFile := filepath.Join(bpDir, ".gitignore")

	// create blueprint directory
	if err := blueprintIO.CreateDirectory(bpDir); err != nil {
		if !overwrite {
			return &OverwriteDeniedError{err}
		}

		// Confirm we have a previously written blueprint dir before overwritting.
		if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
			return fmt.Errorf(
				"While trying to update the blueprint directory at %s, the '.ghpc/' dir could not be found", bpDir)
		}
	} else {
		if err := blueprintIO.CreateDirectory(ghpcDir); err != nil {
			return fmt.Errorf("Failed to create directory at %s: err=%w", ghpcDir, err)
		}

		if err := blueprintIO.CopyFromFS(templatesFS, gitignoreTemplate, gitignoreFile); err != nil {
			return fmt.Errorf("Failed to copy template.gitignore file to %s: err=%w", gitignoreFile, err)
		}
	}

	// clean up old dirs
	prevGroupDir := filepath.Join(ghpcDir, prevDeploymentGroupDirName)
	os.RemoveAll(prevGroupDir)
	if err := os.MkdirAll(prevGroupDir, 0755); err != nil {
		return fmt.Errorf("Failed to create directory to save previous deployment groups at %s: %w", prevGroupDir, err)
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
			return fmt.Errorf("Error while moving previous deployment groups: failed on %s: %w", f.Name(), err)
		}
	}
	return nil
}
