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
	"crypto/md5"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/deploymentio"
	"hpc-toolkit/pkg/sourcereader"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/hashicorp/go-getter"
)

// strings that get re-used throughout this package and others
const (
	HiddenGhpcDirName          = ".ghpc"
	ArtifactsDirName           = "artifacts"
	prevDeploymentGroupDirName = "previous_deployment_groups"
	gitignoreTemplate          = "deployment.gitignore.tmpl"
	artifactsWarningFilename   = "DO_NOT_MODIFY_THIS_DIRECTORY"
	expandedBlueprintName      = "expanded_blueprint.yaml"
)

// ModuleWriter interface for writing modules to a deployment
type ModuleWriter interface {
	writeDeploymentGroup(
		dc config.DeploymentConfig,
		grpIdx int,
		deployDir string,
		instructionsFile io.Writer,
	) error
	restoreState(deploymentDir string) error
	kind() config.ModuleKind
}

var kinds = map[string]ModuleWriter{
	config.TerraformKind.String(): new(TFWriter),
	config.PackerKind.String():    new(PackerWriter),
}

//go:embed *.tmpl
var templatesFS embed.FS

// WriteDeployment writes a deployment directory using modules defined the
// environment blueprint.
func WriteDeployment(dc config.DeploymentConfig, deploymentDir string, overwriteFlag bool) error {
	overwrite := isOverwriteAllowed(deploymentDir, &dc.Config, overwriteFlag)
	if err := prepDepDir(deploymentDir, overwrite); err != nil {
		return err
	}

	if err := copySource(deploymentDir, &dc.Config.DeploymentGroups); err != nil {
		return err
	}

	if err := createGroupDirs(deploymentDir, &dc.Config.DeploymentGroups); err != nil {
		return err
	}

	f, err := os.Create(InstructionsPath(deploymentDir))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "Advanced Deployment Instructions")
	fmt.Fprintln(f, "================================")

	for grpIdx, grp := range dc.Config.DeploymentGroups {
		writer, ok := kinds[grp.Kind().String()]
		if !ok {
			return fmt.Errorf(
				"invalid kind in deployment group %s, got '%s'", grp.Name, grp.Kind())
		}

		err := writer.writeDeploymentGroup(dc, grpIdx, deploymentDir, f)
		if err != nil {
			return fmt.Errorf("error writing deployment group %s: %w", grp.Name, err)
		}
	}

	writeDestroyInstructions(f, dc, deploymentDir)

	if err := writeExpandedBlueprint(deploymentDir, dc); err != nil {
		return err
	}

	for _, writer := range kinds {
		if err := writer.restoreState(deploymentDir); err != nil {
			return fmt.Errorf("error trying to restore terraform state: %w", err)
		}
	}
	return nil
}

// InstructionsPath returns the path to the instructions file for a deployment
func InstructionsPath(deploymentDir string) string {
	return filepath.Join(deploymentDir, "instructions.txt")
}

func createGroupDirs(deploymentPath string, deploymentGroups *[]config.DeploymentGroup) error {
	for _, grp := range *deploymentGroups {
		groupPath := filepath.Join(deploymentPath, string(grp.Name))
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

// DeploymentSource returns module source within deployment group
// Rules are following:
//   - remote source
//     = terraform => <mod.Source>
//     = packer    => <mod.ID>/<package_subdir>
//   - packer
//     => <mod.ID>
//   - embedded (source starts with "modules" or "comunity/modules")
//     => ./modules/embedded/<mod.Source>
//   - other
//     => ./modules/<basename(mod.Source)>-<hash(abs(mod.Source))>
func DeploymentSource(mod config.Module) (string, error) {
	switch mod.Kind {
	case config.TerraformKind:
		return tfDeploymentSource(mod)
	case config.PackerKind:
		return packerDeploymentSource(mod), nil
	default:
		return "", fmt.Errorf("unexpected module kind %#v", mod.Kind)
	}
}

func tfDeploymentSource(mod config.Module) (string, error) {
	switch {
	case sourcereader.IsEmbeddedPath(mod.Source):
		return "./modules/" + filepath.Join("embedded", mod.Source), nil
	case sourcereader.IsLocalPath(mod.Source):
		abs, err := filepath.Abs(mod.Source)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path for %#v: %v", mod.Source, err)
		}
		base := filepath.Base(mod.Source)
		return fmt.Sprintf("./modules/%s-%s", base, shortHash(abs)), nil
	default:
		return mod.Source, nil
	}
}

func packerDeploymentSource(mod config.Module) string {
	if sourcereader.IsRemotePath(mod.Source) {
		_, subDir := getter.SourceDirSubdir(mod.Source)
		return filepath.Join(string(mod.ID), subDir)
	}
	return string(mod.ID)
}

// Returns first 4 characters of md5 sum in hex form
func shortHash(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])[:4]
}

func copyEmbeddedModules(base string) error {
	r := sourcereader.EmbeddedSourceReader{}
	for _, src := range []string{"modules", "community/modules"} {
		dst := filepath.Join(base, "modules/embedded", src)
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		if err := r.CopyDir(src, dst); err != nil {
			return err
		}

	}
	return nil
}

func copySource(deploymentPath string, deploymentGroups *[]config.DeploymentGroup) error {
	for iGrp := range *deploymentGroups {
		grp := &(*deploymentGroups)[iGrp]
		basePath := filepath.Join(deploymentPath, string(grp.Name))

		var copyEmbedded = false
		for iMod := range grp.Modules {
			mod := &grp.Modules[iMod]
			deplSource, err := DeploymentSource(*mod)
			if err != nil {
				return err
			}

			if mod.Kind == config.TerraformKind {
				// some terraform modules do not require copying
				if sourcereader.IsEmbeddedPath(mod.Source) {
					copyEmbedded = true
					continue // all embedded terraform modules fill be copied at once
				}
				if sourcereader.IsRemotePath(mod.Source) {
					continue // will be downloaded by terraform
				}
			}

			/* Copy source files */
			var src, dst string

			if sourcereader.IsRemotePath(mod.Source) && mod.Kind == config.PackerKind {
				src, _ = getter.SourceDirSubdir(mod.Source)
				dst = filepath.Join(basePath, string(mod.ID))
			} else {
				src = mod.Source
				dst = filepath.Join(basePath, deplSource)
			}
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			reader := sourcereader.Factory(src)
			if err := reader.GetModule(src, dst); err != nil {
				return fmt.Errorf("failed to get module from %s to %s: %v", src, dst, err)
			}
			// remove .git directory if one exists; we do not want submodule
			// git history in deployment directory
			if err := os.RemoveAll(filepath.Join(dst, ".git")); err != nil {
				return err
			}
		}
		if copyEmbedded {
			if err := copyEmbeddedModules(basePath); err != nil {
				return fmt.Errorf("failed to copy embedded modules: %v", err)
			}
		}

	}
	return nil
}

// Determines if overwrite is allowed
func isOverwriteAllowed(depDir string, overwritingConfig *config.Blueprint, overwriteFlag bool) bool {
	if !overwriteFlag {
		return false
	}

	files, err := os.ReadDir(depDir)
	if err != nil {
		return false
	}

	// build list of previous and current deployment groups
	var prevGroups []string
	for _, f := range files {
		if f.IsDir() && f.Name() != HiddenGhpcDirName {
			prevGroups = append(prevGroups, f.Name())
		}
	}

	var curGroups []string
	for _, group := range overwritingConfig.DeploymentGroups {
		curGroups = append(curGroups, string(group.Name))
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
	ghpcDir := filepath.Join(depDir, HiddenGhpcDirName)
	artifactsDir := filepath.Join(ghpcDir, ArtifactsDirName)
	gitignoreFile := filepath.Join(depDir, ".gitignore")

	// create deployment directory
	if err := deploymentio.CreateDirectory(depDir); err != nil {
		if !overwrite {
			return &OverwriteDeniedError{err}
		}

		// Confirm we have a previously written deployment dir before overwriting.
		if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
			return fmt.Errorf(
				"while trying to update the deployment directory at %s, the '.ghpc/' dir could not be found", depDir)
		}
	} else {
		if err := deploymentio.CreateDirectory(ghpcDir); err != nil {
			return fmt.Errorf("failed to create directory at %s: err=%w", ghpcDir, err)
		}

		if err := deploymentio.CopyFromFS(templatesFS, gitignoreTemplate, gitignoreFile); err != nil {
			return fmt.Errorf("failed to copy template.gitignore file to %s: err=%w", gitignoreFile, err)
		}
	}

	if err := prepArtifactsDir(artifactsDir); err != nil {
		return err
	}

	// remove any existing backups of deployment group
	prevGroupDir := filepath.Join(ghpcDir, prevDeploymentGroupDirName)
	os.RemoveAll(prevGroupDir)
	if err := os.MkdirAll(prevGroupDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory to save previous deployment groups at %s: %w", prevGroupDir, err)
	}

	// create new backup of deployment group directory
	files, err := os.ReadDir(depDir)
	if err != nil {
		return fmt.Errorf("Error trying to read directories in %s, %w", depDir, err)
	}
	for _, f := range files {
		if !f.IsDir() || f.Name() == HiddenGhpcDirName {
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

func prepArtifactsDir(artifactsDir string) error {
	// cleanup previous artifacts on every write
	if err := os.RemoveAll(artifactsDir); err != nil {
		return fmt.Errorf(
			"error while removing the artifacts directory at %s; %s", artifactsDir, err.Error())
	}

	if err := os.MkdirAll(artifactsDir, 0700); err != nil {
		return err
	}

	artifactsWarningFile := path.Join(artifactsDir, artifactsWarningFilename)
	f, err := os.Create(artifactsWarningFile)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(artifactsWarning)
	if err != nil {
		return err
	}
	return nil
}

func writeExpandedBlueprint(depDir string, dc config.DeploymentConfig) error {
	artifactsDir := filepath.Join(depDir, HiddenGhpcDirName, ArtifactsDirName)
	blueprintFile := filepath.Join(artifactsDir, expandedBlueprintName)
	return dc.ExportBlueprint(blueprintFile)
}

func writeDestroyInstructions(w io.Writer, dc config.DeploymentConfig, deploymentDir string) {
	packerManifests := []string{}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Destroying infrastructure when no longer needed")
	fmt.Fprintln(w, "===============================================")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Automated")
	fmt.Fprintln(w, "---------")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "ghpc destroy %s\n", deploymentDir)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Advanced / Manual")
	fmt.Fprintln(w, "-----------------")
	fmt.Fprintln(w, "Infrastructure should be destroyed in reverse order of creation:")
	fmt.Fprintln(w)
	for grpIdx := len(dc.Config.DeploymentGroups) - 1; grpIdx >= 0; grpIdx-- {
		grp := dc.Config.DeploymentGroups[grpIdx]
		grpPath := filepath.Join(deploymentDir, string(grp.Name))
		if grp.Kind() == config.TerraformKind {
			fmt.Fprintf(w, "terraform -chdir=%s destroy\n", grpPath)
		}
		if grp.Kind() == config.PackerKind {
			packerManifests = append(packerManifests, filepath.Join(grpPath, string(grp.Modules[0].ID), "packer-manifest.json"))

		}
	}

	WritePackerDestroyInstructions(w, packerManifests)
}

// WritePackerDestroyInstructions prints our best effort guidance to the user on
// deleting images produced by Packer; must improve definition of Packer outputs
func WritePackerDestroyInstructions(w io.Writer, manifests []string) {
	if len(manifests) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Please browse to the Cloud Console to remove VM images produced by Packer.\n")
	fmt.Fprintln(w, "If this file is present, the names of images can be read from it:")
	fmt.Fprintln(w)
	for _, manifest := range manifests {
		fmt.Fprintln(w, manifest)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "https://console.cloud.google.com/compute/images")
}
