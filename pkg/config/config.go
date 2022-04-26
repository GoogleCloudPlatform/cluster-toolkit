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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyJson "github.com/zclconf/go-cty/cty/json"
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
	"missingSetting":    "a required setting is missing from a module",
	"globalLabelType":   "global labels are not a map",
	"settingsLabelType": "labels in module settings are not a map",
	"invalidVar":        "invalid variable definition in",
	"varNotFound":       "Could not find source of variable",
	"varInAnotherGroup": "References to other groups are not yet supported",
	"noOutput":          "Output not found for a variable",
	// validator
	"emptyID":        "a module id cannot be empty",
	"emptySource":    "a module source cannot be empty",
	"wrongKind":      "a module kind is invalid",
	"extraSetting":   "a setting was added that is not found in the module",
	"mixedModules":   "mixing modules of differing kinds in a deployment group is not supported",
	"duplicateGroup": "group names must be unique",
	"duplicateID":    "module IDs must be unique",
	"emptyGroupName": "group name must be set for each deployment group",
	"illegalChars":   "invalid character(s) found in group name",
	"invalidOutput":  "requested output was not found in the module",
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

type validatorName int64

const (
	// Undefined will be default and potentially throw errors if used
	Undefined validatorName = iota
	testProjectExistsName
	testRegionExistsName
	testZoneExistsName
	testZoneInRegionName
)

// this enum will be used to control how fatal validator failures will be
// treated during blueprint creation
const (
	validationError int = iota
	validationWarning
	validationIgnore
)

func isValidValidationLevel(level int) bool {
	return !(level > validationIgnore || level < validationError)
}

// SetValidationLevel allows command-line tools to set the validation level
func (bc *BlueprintConfig) SetValidationLevel(level string) error {
	switch level {
	case "ERROR":
		bc.Config.ValidationLevel = validationError
	case "WARNING":
		bc.Config.ValidationLevel = validationWarning
	case "IGNORE":
		bc.Config.ValidationLevel = validationIgnore
	default:
		return fmt.Errorf("invalid validation level (\"ERROR\", \"WARNING\", \"IGNORE\")")
	}

	return nil
}

func (v validatorName) String() string {
	switch v {
	case testProjectExistsName:
		return "test_project_exists"
	case testRegionExistsName:
		return "test_region_exists"
	case testZoneExistsName:
		return "test_zone_exists"
	case testZoneInRegionName:
		return "test_zone_in_region"
	default:
		return "unknown_validator"
	}
}

type validatorConfig struct {
	Validator string
	Inputs    map[string]interface{}
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
	Outputs          []string `yaml:"outputs,omitempty"`
	Settings         map[string]interface{}
}

// createWrapSettingsWith ensures WrapSettingsWith field is not nil, if it is
// a new map is created.
func (r *Resource) createWrapSettingsWith() {
	if r.WrapSettingsWith == nil {
		r.WrapSettingsWith = make(map[string][]string)
	}
}

// YamlConfig stores the contents on the User YAML
// omitempty on validation_level ensures that expand will not expose the setting
// unless it has been set to a non-default value; the implementation as an
// integer is primarily for internal purposes even if it can be set in blueprint
type YamlConfig struct {
	BlueprintName            string `yaml:"blueprint_name"`
	Validators               []validatorConfig
	ValidationLevel          int `yaml:"validation_level,omitempty"`
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

	if len(yamlConfig.Vars) == 0 {
		yamlConfig.Vars = make(map[string]interface{})
	}

	// if the validation level has been explicitly set to an invalid value
	// in YAML blueprint then silently default to validationError
	if !isValidValidationLevel(yamlConfig.ValidationLevel) {
		yamlConfig.ValidationLevel = validationError
	}

	return yamlConfig
}

// ExportYamlConfig exports the internal representation of a blueprint config
func (bc BlueprintConfig) ExportYamlConfig(outputFilename string) ([]byte, error) {
	d, err := yaml.Marshal(&bc.Config)
	if err != nil {
		return d, fmt.Errorf("%s: %w", errorMessages["yamlMarshalError"], err)
	}
	if outputFilename == "" {
		return d, nil
	}
	err = ioutil.WriteFile(outputFilename, d, 0644)
	if err != nil {
		// hitting this error writing yaml
		return d, fmt.Errorf("%s, Filename: %s: %w",
			errorMessages["fileSaveError"], outputFilename, err)
	}
	return nil, nil
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
					"failed to get info for module at %s while setting bc.ResourcesInfo: %e",
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
					"%s: deployment group %s, got: %s, wanted: %s",
					errorMessages["mixedModule"],
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
					return fmt.Errorf("used module ID %s does not exist", usedRes)
				}
				// Ensure resource is from the correct group
				if idToGroup[usedRes] != iGrp {
					return fmt.Errorf(
						"used module ID %s not found in this Deployment Group", usedRes)
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

// SetCLIVariables sets the variables at CLI
func (bc *BlueprintConfig) SetCLIVariables(cliVariables []string) error {
	for _, cliVar := range cliVariables {
		arr := strings.SplitN(cliVar, "=", 2)

		if len(arr) != 2 {
			return fmt.Errorf("invalid format: '%s' should follow the 'name=value' format", cliVar)
		}

		key, value := arr[0], arr[1]
		bc.Config.Vars[key] = value
	}

	return nil
}

// IsLiteralVariable returns true if string matches variable ((ctx.name))
func IsLiteralVariable(str string) bool {
	match, err := regexp.MatchString(literalExp, str)
	if err != nil {
		log.Fatalf("Failed checking if variable is a literal: %v", err)
	}
	return match
}

// IdentifyLiteralVariable returns
// string: variable source (e.g. global "vars" or module "modname")
// string: variable name (e.g. "project_id")
// bool: true/false reflecting success
func IdentifyLiteralVariable(str string) (string, string, bool) {
	re := regexp.MustCompile(literalSplitExp)
	contents := re.FindStringSubmatch(str)
	if len(contents) != 3 {
		return "", "", false
	}

	return contents[1], contents[2], true
}

// HandleLiteralVariable is exported for use in reswriter as well
func HandleLiteralVariable(str string) string {
	re := regexp.MustCompile(literalExp)
	contents := re.FindStringSubmatch(str)
	if len(contents) != 2 {
		log.Fatalf("Incorrectly formatted literal variable: %s", str)
	}

	return strings.TrimSpace(contents[1])
}

// ConvertToCty convert interface directly to a cty.Value
func ConvertToCty(val interface{}) (cty.Value, error) {
	// Convert to JSON bytes
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return cty.Value{}, err
	}

	// Unmarshal JSON into cty
	simpleJSON := ctyJson.SimpleJSONValue{}
	simpleJSON.UnmarshalJSON(jsonBytes)
	return simpleJSON.Value, nil
}

// ConvertMapToCty convert an interface map to a map of cty.Values
func ConvertMapToCty(iMap map[string]interface{}) (map[string]cty.Value, error) {
	cMap := make(map[string]cty.Value)
	for k, v := range iMap {
		convertedVal, err := ConvertToCty(v)
		if err != nil {
			return cMap, err
		}
		cMap[k] = convertedVal
	}
	return cMap, nil
}

// ResolveGlobalVariables given a map of strings to cty.Value types, will examine
// all cty.Values that are of type cty.String. If they are literal global variables,
// then they are replaced by the cty.Value of the corresponding entry in
// yc.Vars. All other cty.Values are unmodified.
// ERROR: if conversion from yc.Vars to map[string]cty.Value fails
// ERROR: if (somehow) the cty.String cannot be converted to a Go string
// ERROR: rely on HCL TraverseAbs to bubble up "diagnostics" when the global variable
//        being resolved does not exist in yc.Vars
func (yc *YamlConfig) ResolveGlobalVariables(ctyMap map[string]cty.Value) error {
	ctyVars, err := ConvertMapToCty(yc.Vars)
	if err != nil {
		return fmt.Errorf("could not convert global variables to cty map")
	}
	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{"var": cty.ObjectVal(ctyVars)},
	}
	for key, val := range ctyMap {
		if val.Type() == cty.String {
			var valString string
			if err := gocty.FromCtyValue(val, &valString); err != nil {
				return err
			}
			ctx, varName, found := IdentifyLiteralVariable(valString)
			// only attempt resolution on global literal variables
			// leave all other strings alone (including non-global)
			if found && ctx == "var" {
				varTraversal := hcl.Traversal{
					hcl.TraverseRoot{Name: ctx},
					hcl.TraverseAttr{Name: varName},
				}
				newVal, diags := varTraversal.TraverseAbs(evalCtx)
				if diags.HasErrors() {
					return diags
				}
				ctyMap[key] = newVal
			}
		}
	}
	return nil
}

// DeploymentNameError signifies a problem with the blueprint deployment name.
type DeploymentNameError struct {
	cause string
}

func (err *DeploymentNameError) Error() string {
	return fmt.Sprintf("deployment_name must be a string and cannot be empty, cause: %v", err.cause)
}

// DeploymentName returns the deployment_name from the config and does approperate checks.
func (yc *YamlConfig) DeploymentName() (string, error) {
	nameInterface, found := yc.Vars["deployment_name"]
	if !found {
		return "", &DeploymentNameError{"deployment_name variable not defined."}
	}

	deploymentName, ok := nameInterface.(string)
	if !ok {
		return "", &DeploymentNameError{"deployment_name was not of type string."}
	}

	if len(deploymentName) == 0 {
		return "", &DeploymentNameError{"deployment_name was an empty string."}
	}
	return deploymentName, nil
}
