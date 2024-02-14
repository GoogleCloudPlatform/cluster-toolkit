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
	ExpandedBlueprintName      = "expanded_blueprint.yaml"
	prevDeploymentGroupDirName = "previous_deployment_groups"
	gitignoreTemplate          = "deployment.gitignore.tmpl"
	artifactsWarningFilename   = "DO_NOT_MODIFY_THIS_DIRECTORY"
)

func HiddenGhpcDir(deplDir string) string {
	return filepath.Join(filepath.Clean(deplDir), HiddenGhpcDirName)
}

func ArtifactsDir(deplDir string) string {
	return filepath.Join(HiddenGhpcDir(deplDir), ArtifactsDirName)
}

// ModuleWriter interface for writing modules to a deployment
type ModuleWriter interface {
	writeDeploymentGroup(
		dc config.DeploymentConfig,
		grpIdx int,
		groupPath string,
		instructionsFile io.Writer,
	) error
	restoreState(deploymentDir string) error
	kind() config.ModuleKind
}

var kinds = map[config.ModuleKind]ModuleWriter{
	config.TerraformKind: new(TFWriter),
	config.PackerKind:    new(PackerWriter),
}

//go:embed *.tmpl
var templatesFS embed.FS

// WriteDeployment writes a deployment directory using modules defined the environment blueprint.
func WriteDeployment(dc config.DeploymentConfig, deploymentDir string) error {
	if err := prepDepDir(deploymentDir); err != nil {
		return err
	}

	instructions, err := os.Create(InstructionsPath(deploymentDir))
	if err != nil {
		return err
	}
	defer instructions.Close()
	fmt.Fprintln(instructions, "Advanced Deployment Instructions")
	fmt.Fprintln(instructions, "================================")

	for ig := range dc.Config.DeploymentGroups {
		if err := writeGroup(deploymentDir, dc, ig, instructions); err != nil {
			return err
		}
	}

	writeDestroyInstructions(instructions, dc, deploymentDir)

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

func writeGroup(deplPath string, dc config.DeploymentConfig, gIdx int, instructions io.Writer) error {
	g := dc.Config.DeploymentGroups[gIdx]
	gPath, err := createGroupDir(deplPath, g)
	if err != nil {
		return err
	}

	if err := copyGroupSources(gPath, g); err != nil {
		return err
	}

	writer, ok := kinds[g.Kind()]
	if !ok {
		return fmt.Errorf("invalid kind in deployment group %q, got %q", g.Name, g.Kind())
	}

	if err := writer.writeDeploymentGroup(dc, gIdx, gPath, instructions); err != nil {
		return fmt.Errorf("error writing deployment group %s: %w", g.Name, err)
	}
	return nil
}

// InstructionsPath returns the path to the instructions file for a deployment
func InstructionsPath(deploymentDir string) string {
	return filepath.Join(deploymentDir, "instructions.txt")
}

func createGroupDir(deplPath string, g config.DeploymentGroup) (string, error) {
	gPath := filepath.Join(deplPath, string(g.Name))
	// Create the deployment group directory if not already created.
	if _, err := os.Stat(gPath); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(gPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory at %s for deployment group %s: err=%w", gPath, g.Name, err)
		}
	}
	return gPath, nil
}

// DeploymentSource returns module source within deployment group
// Rules are following:
//   - remote source
//     = terraform => <mod.Source>
//     = packer    => <mod.ID>/<package_subdir>
//   - packer
//     => <mod.ID>
//   - embedded (source starts with "modules" or "community/modules")
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

func copyGroupSources(gPath string, g config.DeploymentGroup) error {
	var copyEmbedded = false
	for iMod := range g.Modules {
		mod := &g.Modules[iMod]
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
			dst = filepath.Join(gPath, string(mod.ID))
		} else {
			src = mod.Source
			dst = filepath.Join(gPath, deplSource)
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		reader := sourcereader.Factory(src)
		if err := reader.GetModule(src, dst); err != nil {
			return fmt.Errorf("failed to get module from %s to %s: %w", src, dst, err)
		}
		// remove .git directory if one exists; we do not want submodule
		// git history in deployment directory
		if err := os.RemoveAll(filepath.Join(dst, ".git")); err != nil {
			return err
		}
	}
	if copyEmbedded {
		if err := copyEmbeddedModules(gPath); err != nil {
			return fmt.Errorf("failed to copy embedded modules: %w", err)
		}
	}

	return nil
}

// Prepares a deployment directory to be written to.
func prepDepDir(depDir string) error {
	deploymentio := deploymentio.GetDeploymentioLocal()
	ghpcDir := HiddenGhpcDir(depDir)

	// create deployment directory
	if err := deploymentio.CreateDirectory(depDir); err != nil {
		// Confirm we have a previously written deployment dir before overwriting.
		if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
			return fmt.Errorf("while trying to update the deployment directory at %s, the '.ghpc/' dir could not be found", depDir)
		}
	} else {
		if err := deploymentio.CreateDirectory(ghpcDir); err != nil {
			return fmt.Errorf("failed to create directory at %s: err=%w", ghpcDir, err)
		}

		gitignoreFile := filepath.Join(depDir, ".gitignore")
		if err := deploymentio.CopyFromFS(templatesFS, gitignoreTemplate, gitignoreFile); err != nil {
			return fmt.Errorf("failed to copy template.gitignore file to %s: err=%w", gitignoreFile, err)
		}
	}

	if err := prepArtifactsDir(ArtifactsDir(depDir)); err != nil {
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
		return fmt.Errorf("error trying to read directories in %s, %w", depDir, err)
	}
	for _, f := range files {
		if !f.IsDir() || f.Name() == HiddenGhpcDirName {
			continue
		}
		src := filepath.Join(depDir, f.Name())
		dest := filepath.Join(prevGroupDir, f.Name())
		if err := os.Rename(src, dest); err != nil {
			return fmt.Errorf("error while moving previous deployment groups: failed on %s: %w", f.Name(), err)
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
	return dc.ExportBlueprint(filepath.Join(ArtifactsDir(depDir), ExpandedBlueprintName))
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
