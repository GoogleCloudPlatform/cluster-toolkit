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
	"bytes"
	"crypto/md5"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/deploymentio"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

const (
	hiddenGhpcDirName          = ".ghpc"
	prevDeploymentGroupDirName = "previous_deployment_groups"
	gitignoreTemplate          = "deployment.gitignore.tmpl"
	deploymentMetadataName     = "deployment_metadata.yaml"
)

const intergroupWarning string = `
WARNING: this deployment group requires outputs from previous groups!
This is an advanced feature under active development. The automatically generated
instructions for executing terraform or packer below will not work as shown.

`

// ModuleWriter interface for writing modules to a deployment
type ModuleWriter interface {
	getNumModules() int
	addNumModules(int)
	writeDeploymentGroup(
		dc config.DeploymentConfig,
		grpIdx int,
		deployDir string,
	) (groupMetadata, error)
	restoreState(deploymentDir string) error
}

type deploymentMetadata struct {
	DeploymentMetadata []groupMetadata `yaml:"deployment_metadata"`
}

type groupMetadata struct {
	Name             string
	DeploymentInputs []string `yaml:"deployment_inputs"`
	IntergroupInputs []string `yaml:"intergroup_inputs"`
	Outputs          []string
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
func WriteDeployment(dc config.DeploymentConfig, outputDir string, overwriteFlag bool) error {
	deploymentName, err := dc.Config.DeploymentName()
	if err != nil {
		return err
	}
	deploymentDir := filepath.Join(outputDir, deploymentName)

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

	metadata := deploymentMetadata{
		DeploymentMetadata: []groupMetadata{},
	}
	for grpIdx, grp := range dc.Config.DeploymentGroups {
		writer, ok := kinds[grp.Kind.String()]
		if !ok {
			return fmt.Errorf(
				"invalid kind in deployment group %s, got '%s'", grp.Name, grp.Kind)
		}

		gmd, err := writer.writeDeploymentGroup(dc, grpIdx, deploymentDir)
		if err != nil {
			return fmt.Errorf("error writing deployment group %s: %w", grp.Name, err)
		}
		metadata.DeploymentMetadata = append(metadata.DeploymentMetadata, gmd)
	}

	if err := writeDeploymentMetadata(deploymentDir, metadata); err != nil {
		return err
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
//     => <mod.ID>
//   - embedded (source starts with "modules" or "comunity/modules")
//     => ./modules/embedded/<source>
//   - other
//     => ./modules/<basename(source)>-<hash(abs(source))>
func deploymentSource(mod config.Module) (string, error) {
	if sourcereader.IsGitPath(mod.Source) {
		return mod.Source, nil
	}
	if mod.Kind == config.PackerKind {
		return mod.ID, nil
	}
	if mod.Kind != config.TerraformKind {
		return "", fmt.Errorf("unexpected module kind %#v", mod.Kind)
	}

	if sourcereader.IsEmbeddedPath(mod.Source) {
		return "./modules/" + filepath.Join("embedded", mod.Source), nil
	}
	if !sourcereader.IsLocalPath(mod.Source) {
		return "", fmt.Errorf("unuexpected module source %s", mod.Source)
	}

	abs, err := filepath.Abs(mod.Source)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %#v: %v", mod.Source, err)
	}
	base := filepath.Base(mod.Source)
	return fmt.Sprintf("./modules/%s-%s", base, shortHash(abs)), nil
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
		basePath := filepath.Join(deploymentPath, grp.Name)

		var copyEmbedded = false
		for iMod := range grp.Modules {
			mod := &grp.Modules[iMod]
			ds, err := deploymentSource(*mod)
			if err != nil {
				return err
			}
			mod.DeploymentSource = ds

			if sourcereader.IsGitPath(mod.Source) {
				continue // do not download
			}
			factory(mod.Kind.String()).addNumModules(1)
			if sourcereader.IsEmbeddedPath(mod.Source) && mod.Kind == config.TerraformKind {
				copyEmbedded = true
				continue // all embedded terraform modules fill be copied at once
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
		}
		if copyEmbedded {
			if err := copyEmbeddedModules(basePath); err != nil {
				return fmt.Errorf("failed to copy embedded modules: %v", err)
			}
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

	// clean up old dirs
	prevGroupDir := filepath.Join(ghpcDir, prevDeploymentGroupDirName)
	os.RemoveAll(prevGroupDir)
	if err := os.MkdirAll(prevGroupDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory to save previous deployment groups at %s: %w", prevGroupDir, err)
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

func writeDeploymentMetadata(depDir string, metadata deploymentMetadata) error {
	ghpcDir := filepath.Join(depDir, hiddenGhpcDirName)
	if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
		return fmt.Errorf(
			"while trying to update the deployment directory at %s, the '.ghpc/' dir could not be found", depDir)
	}

	metadataFile := filepath.Join(ghpcDir, deploymentMetadataName)
	f, err := os.OpenFile(metadataFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	buf.WriteString(config.YamlLicense)
	buf.WriteString("\n")
	encoder := yaml.NewEncoder(&buf)
	defer encoder.Close()
	encoder.SetIndent(2)
	if err := encoder.Encode(metadata); err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}

	return nil
}

func findIntergroupVariables(group config.DeploymentGroup, graph map[string][]config.ModConnection) []modulereader.VarInfo {
	// var integroupVars []modulereader.VarInfo
	var intergroupVars []modulereader.VarInfo

	for _, mod := range group.Modules {
		if connections, ok := graph[mod.ID]; ok {
			for _, conn := range connections {
				if conn.IsIntergroup() {
					for _, v := range conn.GetSharedVariables() {
						vinfo := modulereader.VarInfo{
							Name:        v,
							Type:        getHclType(cty.DynamicPseudoType),
							Description: fmt.Sprintf("Toolkit automatically generated variable: %s", v),
							Default:     nil,
							Required:    true,
						}
						intergroupVars = append(intergroupVars, vinfo)
					}
				}
			}
		}
	}
	return intergroupVars
}
