/**
* Copyright 2022 Google LLC
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

// Package modulewriter writes modules to a deployment directory
package modulewriter

import (
	"embed"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/deploymentio"
	"hpc-toolkit/pkg/sourcereader"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	hiddenGhpcDirName          = ".ghpc"
	prevDeploymentGroupDirName = "previous_deployment_groups"
	gitignoreTemplate          = "deployment.gitignore.tmpl"
)

// ModuleWriter interface for writing modules to a deployment
type ModuleWriter interface {
	getNumModules() int
	addNumModules(int)
	writeDeploymentGroup(
		depGroup config.DeploymentGroup,
		globalVars map[string]interface{},
		deployDir string,
	) error
	restoreState(deploymentDir string) error
}

var kinds = map[string]ModuleWriter{
	"terraform": new(TFWriter),
	"packer":    new(PackerWriter),
}

//go:embed *.tmpl
var templatesFS embed.FS

func factory(kind string) ModuleWriter {
	writer, exists := kinds[kind]
	if !exists {
		log.Fatalf(
			"modulewriter: Module kind (%s) is not valid. "+
				"kind must be in (terraform, packer).", kind)
	}
	return writer
}

// WriteDeployment writes a deployment directory using modules defined the
// environment blueprint.
func WriteDeployment(blueprint *config.Blueprint, outputDir string, overwriteFlag bool) error {
	deploymentName, err := blueprint.DeploymentName()
	if err != nil {
		return err
	}
	deploymentDir := filepath.Join(outputDir, deploymentName)

	overwrite := isOverwriteAllowed(deploymentDir, blueprint, overwriteFlag)
	if err := prepDepDir(deploymentDir, overwrite); err != nil {
		return err
	}

	if err := copySource(deploymentDir, &blueprint.DeploymentGroups); err != nil {
		return err
	}

	if err := createGroupDirs(deploymentDir, &blueprint.DeploymentGroups); err != nil {
		return err
	}

	for _, grp := range blueprint.DeploymentGroups {
		writer, ok := kinds[grp.Kind]
		if !ok {
			return fmt.Errorf(
				"Invalid kind in deployment group %s, got '%s'", grp.Name, grp.Kind)
		}

		err := writer.writeDeploymentGroup(grp, blueprint.Vars, deploymentDir)
		if err != nil {
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

func createGroupDirs(deploymentPath string, deploymentGroups *[]config.DeploymentGroup) error {
	for _, grp := range *deploymentGroups {
		groupPath := filepath.Join(deploymentPath, grp.Name)
		// Create the deployment group directory if not already created.
		if _, err := os.Stat(groupPath); errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(groupPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory at %s for deployment group %s: err=%w",
					groupPath, grp.Name, err)
			}
		}
	}
	return nil
}

// Get module source within deployment group
// Rules are following:
//   - git source
//     => keep the same source
//   - packer
//     => use module.ID as source
//   - embedded (source starts with "modules" or "comunity/modules")
//     => ./modules/embedded/<source>
//   - within repos "modules" or "community/modules" folders.
//     => ./modules/dev/<source>
//   - outside of the repo folder.
//     => ./modules/local/<basename(source)>
func deploymentSource(mod config.Module) string {
	if sourcereader.IsGitPath(mod.Source) {
		return mod.Source
	}
	switch mod.Kind {
	case "packer":
		return mod.ID
	case "terraform": // see bellow
	default:
		log.Fatalf("unexpected module kind %s", mod.Kind)
	}
	const common = "./modules/"
	if sourcereader.IsEmbeddedPath(mod.Source) {
		return common + filepath.Join("embedded", mod.Source)
	}
	if !sourcereader.IsLocalPath(mod.Source) {
		log.Fatalf("unuexpected module source %s", mod.Source)
	}
	if strings.HasPrefix(mod.Source, "./modules/") ||
		strings.HasPrefix(mod.Source, "./community/modules/") {
		return common + filepath.Join("dev", mod.Source)
	}
	name := filepath.Base(mod.Source)
	return common + filepath.Join("local", name)

}

func copySource(deploymentPath string, deploymentGroups *[]config.DeploymentGroup) error {
	for iGrp := range *deploymentGroups {
		grp := &(*deploymentGroups)[iGrp]
		basePath := filepath.Join(deploymentPath, grp.Name)
		for iMod := range grp.Modules {
			mod := &grp.Modules[iMod]
			mod.DeploymentSource = deploymentSource(*mod)

			if sourcereader.IsGitPath(mod.Source) {
				continue // do not download
			}
			/* Copy source files */
			dst := filepath.Join(basePath, mod.DeploymentSource)
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			reader := sourcereader.Factory(mod.Source)
			if err := reader.GetModule(mod.Source, dst); err != nil {
				return fmt.Errorf("failed to get module from %s to %s: %v", mod.Source, dst, err)
			}
			/* Create module level files */
			writer := factory(mod.Kind)
			writer.addNumModules(1)
		}
	}
	return nil
}

func printInstructionsPreamble(kind string, path string, name string) {
	fmt.Printf("%s group '%s' was successfully created in directory %s\n", kind, name, path)
	fmt.Println("To deploy, run the following commands:")
}

// Determines if overwrite is allowed
func isOverwriteAllowed(depDir string, overwritingConfig *config.Blueprint, overwriteFlag bool) bool {
	if !overwriteFlag {
		return false
	}

	files, err := ioutil.ReadDir(depDir)
	if err != nil {
		return false
	}

	// build list of previous and current deployment groups
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

// OverwriteDeniedError signifies when a deployment overwrite was denied.
type OverwriteDeniedError struct {
	cause error
}

func (err *OverwriteDeniedError) Error() string {
	return fmt.Sprintf("Failed to overwrite existing deployment.\n\n"+
		"Use the -w command line argument to enable overwrite.\n"+
		"If overwrite is already enabled then this may be because "+
		"you are attempting to remove a deployment group, which is not supported.\n"+
		"original error: %v",
		err.cause)
}

// Prepares a deployment directory to be written to.
func prepDepDir(depDir string, overwrite bool) error {
	deploymentio := deploymentio.GetDeploymentioLocal()
	ghpcDir := filepath.Join(depDir, hiddenGhpcDirName)
	gitignoreFile := filepath.Join(depDir, ".gitignore")

	// create deployment directory
	if err := deploymentio.CreateDirectory(depDir); err != nil {
		if !overwrite {
			return &OverwriteDeniedError{err}
		}

		// Confirm we have a previously written deployment dir before overwritting.
		if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
			return fmt.Errorf(
				"While trying to update the deployment directory at %s, the '.ghpc/' dir could not be found", depDir)
		}
	} else {
		if err := deploymentio.CreateDirectory(ghpcDir); err != nil {
			return fmt.Errorf("Failed to create directory at %s: err=%w", ghpcDir, err)
		}

		if err := deploymentio.CopyFromFS(templatesFS, gitignoreTemplate, gitignoreFile); err != nil {
			return fmt.Errorf("Failed to copy template.gitignore file to %s: err=%w", gitignoreFile, err)
		}
	}

	// clean up old dirs
	prevGroupDir := filepath.Join(ghpcDir, prevDeploymentGroupDirName)
	os.RemoveAll(prevGroupDir)
	if err := os.MkdirAll(prevGroupDir, 0755); err != nil {
		return fmt.Errorf("Failed to create directory to save previous deployment groups at %s: %w", prevGroupDir, err)
	}

	// move deployment groups
	files, err := ioutil.ReadDir(depDir)
	if err != nil {
		return fmt.Errorf("Error trying to read directories in %s, %w", depDir, err)
	}
	for _, f := range files {
		if !f.IsDir() || f.Name() == hiddenGhpcDirName {
			continue
		}
		src := filepath.Join(depDir, f.Name())
		dest := filepath.Join(prevGroupDir, f.Name())
		if err := os.Rename(src, dest); err != nil {
			return fmt.Errorf("Error while moving previous deployment groups: failed on %s: %w", f.Name(), err)
		}
	}
	return nil
}
