// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package config manages and updates the ghpc input config
package config

import (
	"fmt"
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"

	"hpc-toolkit/pkg/resreader"
	"hpc-toolkit/pkg/sourcereader"
)

const expectedVarFormat = "$(vars.var_name) or $(resource_id.var_name)"

var errorMessages = map[string]string{
	// general
	"notImplemented": "not yet implemented",
	// config
	"fileLoadError":      "failed to read the input yaml",
	"yamlUnmarshalError": "failed to unmarshal the yaml config",
	"yamlMarshalError":   "failed to marshal the yaml config",
	"fileSaveError":      "failed to write the expanded yaml",
	// expand
	"missingSetting":    "a required setting is missing from a resource",
	"globalLabelType":   "global labels are not a map",
	"settingsLabelType": "labels in resources settings are not a map",
	"invalidVar":        "invalid variable definition in",
	"varNotFound":       "Could not find source of variable",
	"noOutput":          "Output not found for a variable",
	// validator
	"emptyID":         "a resource id cannot be empty",
	"emptySource":     "a resource source cannot be empty",
	"wrongKind":       "a resource kind is invalid",
	"extraSetting":    "a setting was added that is not found in the resource",
	"mixedResourcees": "mixing resources of differing kinds in a resource group is not supported",
	"duplicateGroup":  "group names must be unique",
	"duplicateID":     "resource IDs must be unique",
	"emptyGroupName":  "group name must be set for each resource group",
	"illegalChars":    "invalid character(s) found in group name",
}

// ResourceGroup defines a group of Resource that are all executed together
type ResourceGroup struct {
	Name             string           `yaml:"group"`
	TerraformBackend TerraformBackend `yaml:"terraform_backend"`
	Resources        []Resource
}

func (g ResourceGroup) getResourceByID(resID string) Resource {
	for i := range g.Resources {
		res := g.Resources[i]
		if g.Resources[i].ID == resID {
			return res
		}
	}
	return Resource{}
}

// TerraformBackend defines the configuration for the terraform state backend
type TerraformBackend struct {
	Type          string
	Configuration map[string]interface{}
}

// HasKind checks to see if a resource group contains any resources of the given
// kind. Note that a resourceGroup should never have more than one kind, this
// function is used in the validation step to ensure that is true.
func (g ResourceGroup) HasKind(kind string) bool {
	for _, res := range g.Resources {
		if res.Kind == kind {
			return true
		}
	}
	return false
}

// Resource stores YAML definition of a resource
type Resource struct {
	Source           string
	Kind             string
	ID               string
	ResourceName     string
	Use              []string
	WrapSettingsWith map[string][]string
	Settings         map[string]interface{}
}

// getSetSettings returns a slice of explicitly set settings at a given point.
func (r Resource) getSetSettings() []string {
	setSettings := make([]string, len(r.Settings))
	i := 0
	for setting := range r.Settings {
		setSettings[i] = setting
		i++
	}
	return setSettings
}

// createWrapSettingsWith ensures WrapSettingsWith field is not nil, if it is
// a new map is created.
func (r *Resource) createWrapSettingsWith() {
	if r.WrapSettingsWith == nil {
		r.WrapSettingsWith = make(map[string][]string)
	}
}

// YamlConfig stores the contents on the User YAML
type YamlConfig struct {
	BlueprintName            string `yaml:"blueprint_name"`
	Vars                     map[string]interface{}
	ResourceGroups           []ResourceGroup  `yaml:"resource_groups"`
	TerraformBackendDefaults TerraformBackend `yaml:"terraform_backend_defaults"`
}

// BlueprintConfig is a container for the imported YAML data and supporting data for
// creating the blueprint from it
type BlueprintConfig struct {
	Config YamlConfig
	// Indexed by Resource Group name and Resource Source
	ResourcesInfo map[string]map[string]resreader.ResourceInfo
	// Maps resource ID to group index
	ResourceToGroup map[string]int
	expanded        bool
}

// ExpandConfig expands the yaml config in place
func (bc *BlueprintConfig) ExpandConfig() {
	bc.setResourcesInfo()
	bc.validateConfig()
	bc.expand()
	bc.validate()
	bc.expanded = true
}

// NewBlueprintConfig is a constructor for BlueprintConfig
func NewBlueprintConfig(configFilename string) BlueprintConfig {
	newBlueprintConfig := BlueprintConfig{
		Config: importYamlConfig(configFilename),
	}
	return newBlueprintConfig
}

// ImportYamlConfig imports the blueprint configuration provided.
func importYamlConfig(yamlConfigFilename string) YamlConfig {
	yamlConfigText, err := ioutil.ReadFile(yamlConfigFilename)
	if err != nil {
		log.Fatalf("%s, filename=%s: %v",
			errorMessages["fileLoadError"], yamlConfigFilename, err)
	}

	var yamlConfig YamlConfig
	err = yaml.UnmarshalStrict(yamlConfigText, &yamlConfig)

	if err != nil {
		log.Fatalf("%s filename=%s: %v",
			errorMessages["yamlUnmarshalError"], yamlConfigFilename, err)
	}

	// Ensure Vars is not a nil map if not set by the user
	if len(yamlConfig.Vars) == 0 {
		yamlConfig.Vars = make(map[string]interface{})
	}

	return yamlConfig
}

// ExportYamlConfig exports the internal representation of a blueprint config
func (bc BlueprintConfig) ExportYamlConfig(outputFilename string) []byte {
	d, err := yaml.Marshal(&bc.Config)
	if err != nil {
		log.Fatalf("%s: %v", errorMessages["yamlMarshalError"], err)
	}
	if outputFilename == "" {
		return d
	}
	err = ioutil.WriteFile(outputFilename, d, 0644)
	if err != nil {
		log.Fatalf("%s, Filename: %s",
			errorMessages["fileSaveError"], outputFilename)
	}
	return nil
}

func createResourceInfo(
	resourceGroup ResourceGroup) map[string]resreader.ResourceInfo {
	resInfo := make(map[string]resreader.ResourceInfo)
	for _, res := range resourceGroup.Resources {
		if _, exists := resInfo[res.Source]; !exists {
			reader := sourcereader.Factory(res.Source)
			ri, err := reader.GetResourceInfo(res.Source, res.Kind)
			if err != nil {
				log.Fatalf(
					"failed to get info for resource at %s while setting bc.ResourcesInfo: %e",
					res.Source, err)
			}
			resInfo[res.Source] = ri
		}
	}
	return resInfo
}

// setResourcesInfo populates needed information from resources.
func (bc *BlueprintConfig) setResourcesInfo() {
	bc.ResourcesInfo = make(map[string]map[string]resreader.ResourceInfo)
	for _, grp := range bc.Config.ResourceGroups {
		bc.ResourcesInfo[grp.Name] = createResourceInfo(grp)
	}
}

func validateGroupName(name string, usedNames map[string]bool) {
	if name == "" {
		log.Fatal(errorMessages["emptyGroupName"])
	}
	if hasIllegalChars(name) {
		log.Fatalf("%s %s", errorMessages["illegalChars"], name)
	}
	if _, ok := usedNames[name]; ok {
		log.Fatalf(
			"%s: %s used more than once", errorMessages["duplicateGroup"], name)
	}
	usedNames[name] = true
}

// checkResourceAndGroupNames checks and imports resource and resource group IDs
// and names respectively.
func checkResourceAndGroupNames(
	resGroups []ResourceGroup) (map[string]int, error) {
	resourceToGroup := make(map[string]int)
	groupNames := make(map[string]bool)
	for iGrp, grp := range resGroups {
		validateGroupName(grp.Name, groupNames)
		var groupKind string
		for _, res := range grp.Resources {
			// Verify no duplicate resource names
			if _, ok := resourceToGroup[res.ID]; ok {
				return resourceToGroup, fmt.Errorf(
					"%s: %s used more than once", errorMessages["duplicateID"], res.ID)
			}
			resourceToGroup[res.ID] = iGrp

			// Verify Resource Kind matches group Kind
			if groupKind == "" {
				groupKind = res.Kind
			} else if groupKind != res.Kind {
				return resourceToGroup, fmt.Errorf(
					"%s: resource group %s, got: %s, wanted: %s",
					errorMessages["mixedResources"],
					grp.Name, groupKind, res.Kind)
			}
		}
	}
	return resourceToGroup, nil
}

// checkUsedResourceNames verifies that any used resources have valid names and
// are in the correct group
func checkUsedResourceNames(
	resGroups []ResourceGroup, idToGroup map[string]int) error {
	for iGrp, grp := range resGroups {
		for _, res := range grp.Resources {
			for _, usedRes := range res.Use {
				// Check if resource even exists
				if _, ok := idToGroup[usedRes]; !ok {
					return fmt.Errorf("used resource ID %s does not exist", usedRes)
				}
				// Ensure resource is from the correct group
				if idToGroup[usedRes] != iGrp {
					return fmt.Errorf(
						"used resource ID %s not found in this Resource Group", usedRes)
				}
			}
		}
	}
	return nil
}

// validateConfig runs a set of simple early checks on the imported input YAML
func (bc *BlueprintConfig) validateConfig() {
	resourceToGroup, err := checkResourceAndGroupNames(bc.Config.ResourceGroups)
	if err != nil {
		log.Fatal(err)
	}
	bc.ResourceToGroup = resourceToGroup
	if err = checkUsedResourceNames(
		bc.Config.ResourceGroups, bc.ResourceToGroup); err != nil {
		log.Fatal(err)
	}
}
