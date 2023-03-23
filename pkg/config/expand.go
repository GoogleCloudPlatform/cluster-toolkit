// Copyright 2022 Google LLC
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

package config

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"path/filepath"

	"hpc-toolkit/pkg/modulereader"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	blueprintLabel  string = "ghpc_blueprint"
	deploymentLabel string = "ghpc_deployment"
	roleLabel       string = "ghpc_role"
)

var (
	// Checks if a variable exists only as a substring, ex:
	// Matches: "a$(vars.example)", "word $(vars.example)", "word$(vars.example)", "$(vars.example)"
	// Doesn't match: "\$(vars.example)", "no variable in this string"
	anyVariableExp    *regexp.Regexp = regexp.MustCompile(`(^|[^\\])\$\((.*?)\)`)
	literalExp        *regexp.Regexp = regexp.MustCompile(`^\(\((.*)\)\)$`)
	simpleVariableExp *regexp.Regexp = regexp.MustCompile(`^\$\((.*)\)$`)
	// the greediness and non-greediness of expression below is important
	// consume all whitespace at beginning and end
	// consume only up to first period to get variable source
	// consume only up to whitespace to get variable name
	literalSplitExp       *regexp.Regexp = regexp.MustCompile(`^\(\([[:space:]]*(.*?)\.(.*?)[[:space:]]*\)\)$`)
	deploymentVariableExp *regexp.Regexp = regexp.MustCompile(`^\$\(vars\.(.*)\)$`)
)

// expand expands variables and strings in the yaml config. Used directly by
// ExpandConfig for the create and expand commands.
func (dc *DeploymentConfig) expand() {
	dc.addSettingsToModules()
	if err := dc.addMetadataToModules(); err != nil {
		log.Printf("could not determine required APIs: %v", err)
	}

	if err := dc.expandBackends(); err != nil {
		log.Fatalf("failed to apply default backend to deployment groups: %v", err)
	}

	if err := dc.addDefaultValidators(); err != nil {
		log.Fatalf(
			"failed to update validators when expanding the config: %v", err)
	}

	if err := dc.combineLabels(); err != nil {
		log.Fatalf(
			"failed to update module labels when expanding the config: %v", err)
	}

	if err := dc.applyUseModules(); err != nil {
		log.Fatalf(
			"failed to apply \"use\" modules when expanding the config: %v", err)
	}

	if err := dc.applyGlobalVariables(); err != nil {
		log.Fatalf(
			"failed to apply deployment variables in modules when expanding the config: %v",
			err)
	}
	if err := dc.expandVariables(); err != nil {
		log.Fatalf("failed to expand variables: %v", err)
	}
}

func (dc *DeploymentConfig) addSettingsToModules() {
	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod, mod := range grp.Modules {
			if mod.Settings == nil {
				dc.Config.DeploymentGroups[iGrp].Modules[iMod].Settings =
					make(map[string]interface{})
			}
		}
	}
}

func (dc *DeploymentConfig) addMetadataToModules() error {
	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod, mod := range grp.Modules {
			if mod.RequiredApis == nil {
				_, projectIDExists := dc.Config.Vars["project_id"].(string)
				if !projectIDExists {
					return fmt.Errorf("global variable project_id must be defined")
				}

				// handle possibility that ModulesInfo does not have this module in it
				// this occurs in unit testing because they do not run dc.ExpandConfig()
				// and dc.setModulesInfo()
				requiredAPIs := dc.ModulesInfo[grp.Name][mod.Source].RequiredApis
				if requiredAPIs == nil {
					requiredAPIs = []string{}
				}
				dc.Config.DeploymentGroups[iGrp].Modules[iMod].RequiredApis = map[string][]string{
					"$(vars.project_id)": requiredAPIs,
				}
			}
		}
	}
	return nil
}

func (dc *DeploymentConfig) expandBackends() error {
	// 1. DEFAULT: use TerraformBackend configuration (if supplied) in each
	//    resource group
	// 2. If top-level TerraformBackendDefaults is defined, insert that
	//    backend into resource groups which have no explicit
	//    TerraformBackend
	// 3. In all cases, add a prefix for GCS backends if one is not defined
	blueprint := &dc.Config
	if blueprint.TerraformBackendDefaults.Type != "" {
		for i := range blueprint.DeploymentGroups {
			grp := &blueprint.DeploymentGroups[i]
			if grp.TerraformBackend.Type == "" {
				grp.TerraformBackend.Type = blueprint.TerraformBackendDefaults.Type
				grp.TerraformBackend.Configuration = make(map[string]interface{})
				for k, v := range blueprint.TerraformBackendDefaults.Configuration {
					grp.TerraformBackend.Configuration[k] = v
				}
			}
			if grp.TerraformBackend.Type == "gcs" && grp.TerraformBackend.Configuration["prefix"] == nil {
				prefix := blueprint.BlueprintName
				if deployment, err := blueprint.DeploymentName(); err == nil {
					prefix += "/" + deployment
				}
				prefix += "/" + grp.Name
				grp.TerraformBackend.Configuration["prefix"] = prefix
			}
		}
	}
	return nil
}

// always use explicit group even for intragroup references
func getModuleVarName(groupID string, modID string, varName string) string {
	return fmt.Sprintf("$(%s.%s.%s)", groupID, modID, varName)
}

func getModuleInputMap(inputs []modulereader.VarInfo) map[string]string {
	modInputs := make(map[string]string)
	for _, input := range inputs {
		modInputs[input.Name] = input.Type
	}
	return modInputs
}

// initialize a Toolkit setting that corresponds to a module input of type list
// create new list if unset, append if already set, error if value not a list
func (mod *Module) addListValue(settingName string, value string) error {
	_, found := mod.Settings[settingName]
	if !found {
		mod.Settings[settingName] = []interface{}{}
		mod.createWrapSettingsWith()
		mod.WrapSettingsWith[settingName] = []string{"flatten([", "])"}
	}
	currentValue, ok := mod.Settings[settingName].([]interface{})
	if ok {
		mod.Settings[settingName] = append(currentValue, value)
		return nil
	}
	return fmt.Errorf("%s: module %s, setting %s",
		errorMessages["appendToNonList"], mod.ID, settingName)
}

// useModule matches input variables in a "using" module to output values
// from a "used" module. It may be used iteratively to successively apply used
// modules in order of precedence. New input variables are added to the using
// module as Toolkit variable references (in same format as a blueprint). If
// the input variable already has a setting, it is ignored, unless the value is
// a list, in which case output values are appended and flattened using HCL.
//
//	mod: "using" module as defined above
//	useMod: "used" module as defined above
//	useModGroupID: deployment group ID to which useMod belongs
//	modInputs: input variables as defined by the using module code
//	useOutputs: output values as defined by the used module code
//	settingsToIgnore: a list of module settings not to modify for any reason;
//	 typical usage will be to leave explicit blueprint settings unmodified
//
// returns: a list of variable names that were used during this function call
func useModule(
	mod *Module,
	useMod Module,
	useModGroupID string,
	modInputs []modulereader.VarInfo,
	useOutputs []modulereader.VarInfo,
	settingsToIgnore []string,
) ([]string, error) {
	usedVars := []string{}
	modInputsMap := getModuleInputMap(modInputs)
	for _, useOutput := range useOutputs {
		settingName := useOutput.Name

		// explicitly ignore these settings (typically those in blueprint)
		if slices.Contains(settingsToIgnore, settingName) {
			continue
		}

		// Skip settings that do not have matching module inputs
		inputType, ok := modInputsMap[settingName]
		if !ok {
			continue
		}

		// skip settings that are not of list type, but already have a value
		// these were probably added by a previous call to this function
		_, alreadySet := mod.Settings[settingName]
		isList := strings.HasPrefix(inputType, "list")
		if alreadySet && !isList {
			continue
		}

		modVarName := getModuleVarName(useModGroupID, useMod.ID, settingName)
		if !isList {
			mod.Settings[settingName] = modVarName
			usedVars = append(usedVars, settingName)
		} else {
			if err := mod.addListValue(settingName, modVarName); err != nil {
				return nil, err
			}
			usedVars = append(usedVars, settingName)
		}
	}
	return usedVars, nil
}

// applyUseModules applies variables from modules listed in the "use" field
// when/if applicable
func (dc *DeploymentConfig) applyUseModules() error {
	for iGrp := range dc.Config.DeploymentGroups {
		group := &dc.Config.DeploymentGroups[iGrp]
		grpModsInfo := dc.ModulesInfo[group.Name]
		for iMod := range group.Modules {
			fromMod := &group.Modules[iMod]
			fromModInfo := grpModsInfo[fromMod.Source]
			settingsInBlueprint := maps.Keys(fromMod.Settings)
			for _, toModID := range fromMod.Use {
				// turn the raw string into a modReference struct
				// which was previously validated by checkUsedModuleNames
				// this will enable us to get structs about the module being
				// used and search it for outputs that match inputs in the
				// current module (the iterator)
				modRef, err := identifyModuleByReference(toModID, *group)
				if err != nil {
					return err
				}

				// to get the module struct, we first needs its group
				toGroup, err := dc.getGroupByID(modRef.toGroupID)
				if err != nil {
					return err
				}

				// this module contains information about the target module that
				// was specified by the user in the blueprint
				toMod, err := toGroup.getModuleByID(modRef.toModuleID)
				if err != nil {
					return err
				}

				// Packer modules cannot be used because they do not have a
				// native concept of outputs. Without this, the validator
				// that checks for matching inputs will always trigger
				if toMod.Kind == "packer" {
					return fmt.Errorf("%s: %s", errorMessages["cannotUsePacker"], toMod.ID)
				}

				// this struct contains the underlying module implementation,
				// not just what the user specified in blueprint. e.g. module
				// input variables and output values
				// this line should probably be tested for success and unit
				// tested but it our unit test infrastructure does not support
				// running dc.setModulesInfo() on our test configurations
				toModInfo := dc.ModulesInfo[toGroup.Name][toMod.Source]
				usedVars, err := useModule(fromMod, toMod, modRef.toGroupID,
					fromModInfo.Inputs, toModInfo.Outputs, settingsInBlueprint)
				if err != nil {
					return err
				}
				connection := ModConnection{
					ref:             modRef,
					kind:            useConnection,
					sharedVariables: usedVars,
				}
				dc.moduleConnections = append(dc.moduleConnections, connection)
			}
		}
	}
	return nil
}

func (dc DeploymentConfig) moduleHasInput(
	depGroup string, source string, inputName string) bool {
	for _, input := range dc.ModulesInfo[depGroup][source].Inputs {
		if input.Name == inputName {
			return true
		}
	}
	return false
}

// Returns enclosing directory of source directory.
func getRole(source string) string {
	role := filepath.Base(filepath.Dir(source))
	// Returned by base if containing directory was not explicit
	invalidRoles := []string{"..", ".", "/"}
	for _, ir := range invalidRoles {
		if role == ir {
			return "other"
		}
	}
	return role
}

func toStringInterfaceMap(i interface{}) (map[string]interface{}, error) {
	var ret map[string]interface{}
	switch val := i.(type) {
	case map[string]interface{}:
		ret = val
	case map[interface{}]interface{}:
		ret = make(map[string]interface{})
		for k, v := range val {
			ret[k.(string)] = v
		}
	default:
		return ret, fmt.Errorf(
			"invalid type of interface{}, expected a map with keys of string or interface{} got %T",
			i,
		)
	}
	return ret, nil
}

// combineLabels sets defaults for labels based on other variables and merges
// the global labels defined in Vars with module setting labels. It also
// determines the role and sets it for each module independently.
func (dc *DeploymentConfig) combineLabels() error {
	defaultLabels := map[string]interface{}{
		blueprintLabel:  dc.Config.BlueprintName,
		deploymentLabel: dc.Config.Vars["deployment_name"],
	}
	labels := "labels"
	var globalLabels map[string]interface{}

	// Add defaults to global labels if they don't already exist
	if _, exists := dc.Config.Vars[labels]; !exists {
		dc.Config.Vars[labels] = defaultLabels
	}

	// Cast global labels so we can index into them
	globalLabels, err := toStringInterfaceMap(dc.Config.Vars[labels])
	if err != nil {
		return fmt.Errorf(
			"%s: found %T",
			errorMessages["globalLabelType"],
			dc.Config.Vars[labels])
	}

	// Add both default labels if they don't already exist
	mergeInLabels(globalLabels, defaultLabels)

	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod := range grp.Modules {
			if err := combineModuleLabels(dc, iGrp, iMod); err != nil {
				return err
			}
		}
	}
	dc.Config.Vars[labels] = globalLabels
	return nil
}

func combineModuleLabels(dc *DeploymentConfig, iGrp int, iMod int) error {
	grp := &dc.Config.DeploymentGroups[iGrp]
	mod := &grp.Modules[iMod]
	mod.createWrapSettingsWith()
	labels := "labels"

	// previously expanded blueprint, user written BPs do not use `WrapSettingsWith`
	if _, ok := mod.WrapSettingsWith[labels]; ok {
		return nil // Do nothing
	}

	// Check if labels are set for this module
	if !dc.moduleHasInput(grp.Name, mod.Source, labels) {
		return nil
	}

	var modLabels map[string]interface{}
	var err error

	if _, exists := mod.Settings[labels]; !exists {
		modLabels = map[string]interface{}{}
	} else {
		// Cast into map so we can index into them
		modLabels, err = toStringInterfaceMap(mod.Settings[labels])
		if err != nil {
			return fmt.Errorf("%s, Module %s, labels type: %T",
				errorMessages["settingsLabelType"], mod.ID, mod.Settings[labels])
		}
	}
	// Add the role (e.g. compute, network, etc)
	if _, exists := modLabels[roleLabel]; !exists {
		modLabels[roleLabel] = getRole(mod.Source)
	}

	if mod.Kind == "terraform" {
		// Terraform module labels to be expressed as
		// `merge(var.labels, { ghpc_role=..., **settings.labels })`
		mod.WrapSettingsWith[labels] = []string{"merge(", ")"}
		mod.Settings[labels] = []interface{}{"((var.labels))", modLabels}
	} else if mod.Kind == "packer" {
		mergeInLabels(modLabels, dc.Config.Vars[labels].(map[string]interface{}))
		mod.Settings[labels] = modLabels
	}
	return nil
}

func mergeInLabels[V interface{}](to map[string]V, from map[string]V) {
	for k, v := range from {
		if _, exists := to[k]; !exists {
			to[k] = v
		}
	}
}

func applyGlobalVarsInGroup(
	deploymentGroup DeploymentGroup,
	modInfo map[string]modulereader.ModuleInfo,
	globalVars map[string]interface{}) error {
	for _, mod := range deploymentGroup.Modules {
		for _, input := range modInfo[mod.Source].Inputs {

			// Module setting exists? Nothing more needs to be done.
			if _, ok := mod.Settings[input.Name]; ok {
				continue
			}

			// If it's not set, is there a global we can use?
			if _, ok := globalVars[input.Name]; ok {
				mod.Settings[input.Name] = fmt.Sprintf("((var.%s))", input.Name)
				continue
			}

			if input.Required {
				// It's not explicitly set, and not global is set
				// Fail if no default has been set
				return fmt.Errorf("%s: Module ID: %s Setting: %s",
					errorMessages["missingSetting"], mod.ID, input.Name)
			}
			// Default exists, the module will handle it
		}
	}
	return nil
}

func updateGlobalVarTypes(vars map[string]interface{}) error {
	for k, v := range vars {
		val, err := updateVariableType(v, varContext{})
		if err != nil {
			return fmt.Errorf("error setting type for deployment variable %s: %v", k, err)
		}
		vars[k] = val
	}
	return nil
}

// applyGlobalVariables takes any variables defined at the global level and
// applies them to module settings if not already set.
func (dc *DeploymentConfig) applyGlobalVariables() error {
	// Update global variable types to match
	if err := updateGlobalVarTypes(dc.Config.Vars); err != nil {
		return err
	}

	for _, grp := range dc.Config.DeploymentGroups {
		err := applyGlobalVarsInGroup(
			grp, dc.ModulesInfo[grp.Name], dc.Config.Vars)
		if err != nil {
			return err
		}
	}
	return nil
}

type varContext struct {
	varString  string
	groupIndex int
	modIndex   int
	blueprint  Blueprint
}

type reference interface {
	validate(Blueprint) error
	IsIntergroup() bool
	String() string
	FromModuleID() string
	ToModuleID() string
}

/*
A module reference is made by the use keyword and is subject to IGC constraints
of references (ordering, explicitness). It has the following fields:
  - toModuleID: the target module ID
  - fromModuleID: the source module ID
  - toGroupID: the deployment group in which the module is *expected* to be found
  - fromGroupID: the deployment group from which the reference is made
  - explicit: a boolean value indicating whether the user made a reference that
    explicitly identified toGroupID rather than inferring it using fromGroupID
*/
type modReference struct {
	toModuleID   string
	fromModuleID string
	toGroupID    string
	fromGroupID  string
	explicit     bool
}

func (ref modReference) String() string {
	return ref.toGroupID + "." + ref.toModuleID
}

func (ref modReference) IsIntergroup() bool {
	return ref.toGroupID == ref.fromGroupID
}

func (ref modReference) FromModuleID() string {
	return ref.fromModuleID
}

func (ref modReference) ToModuleID() string {
	return ref.toModuleID
}

/*
This function performs only the most rudimentary conversion of an input
string into a modReference struct as defined above. An input string consists of
1 or 2 fields separated by periods. An error will be returned if there are not
1 or 2 fields or if either field is the empty string. This function does not
ensure the existence of the module!
*/
func identifyModuleByReference(yamlReference string, dg DeploymentGroup) (modReference, error) {
	// struct defaults: empty strings and false booleans
	var ref modReference
	// intra-group references length 1 and inter-group references length 2
	modComponents := strings.Split(yamlReference, ".")
	switch len(modComponents) {
	case 1:
		ref.toModuleID = modComponents[0]
		ref.toGroupID = dg.Name
		ref.fromGroupID = dg.Name
	case 2:
		ref.toGroupID = modComponents[0]
		ref.toModuleID = modComponents[1]
		ref.fromGroupID = dg.Name
		ref.explicit = true
	}

	// should consider more sophisticated definition of valid values here.
	// for now check that no fields are the empty string; due to the default
	// zero values for strings in the "ref" struct, this will also cover the
	// case that modComponents has wrong # of fields
	if ref.toModuleID == "" || ref.toGroupID == "" || ref.fromGroupID == "" {
		return ref, fmt.Errorf("%s: %s, expected %s",
			errorMessages["invalidMod"], yamlReference, expectedModFormat)
	}

	return ref, nil
}

/*
A variable reference has the following fields
  - Name: the name of the module output or deployment variable
  - toModuleID: the target module ID or "vars" if referring to a deployment variable
  - fromModuleID: the source module ID
  - toGroupID: the deployment group in which the module is *expected* to be found
  - fromGroupID: the deployment group from which the reference is made
  - explicit: a boolean value indicating whether the user made a reference that
    explicitly identified toGroupID rather than inferring it using fromGroupID
*/
type varReference struct {
	name         string
	toModuleID   string
	fromModuleID string
	toGroupID    string
	fromGroupID  string
	explicit     bool
}

func (ref varReference) String() string {
	return ref.toGroupID + "." + ref.toModuleID + "." + ref.name
}

func (ref varReference) IsIntergroup() bool {
	return ref.toGroupID == ref.fromGroupID
}

func (ref varReference) FromModuleID() string {
	return ref.fromModuleID
}

func (ref varReference) ToModuleID() string {
	return ref.toModuleID
}

/*
This function performs only the most rudimentary conversion of an input
string into a varReference struct as defined above. An input string consists of
2 or 3 fields separated by periods. An error will be returned if there are not
2 or 3 fields, or if any field is the empty string. This function does not
ensure the existence of the reference!
*/
func identifySimpleVariable(yamlReference string, dg DeploymentGroup) (varReference, error) {
	varComponents := strings.Split(yamlReference, ".")

	// struct defaults: empty strings and false booleans
	var ref varReference
	ref.fromGroupID = dg.Name

	// intra-group references length 2 and inter-group references length 3
	switch len(varComponents) {
	case 2:
		ref.toModuleID = varComponents[0]
		ref.name = varComponents[1]

		if ref.toModuleID == "vars" {
			ref.toGroupID = "deployment"
		} else {
			ref.toGroupID = dg.Name
		}
	case 3:
		ref.toGroupID = varComponents[0]
		ref.toModuleID = varComponents[1]
		ref.name = varComponents[2]
		ref.explicit = true
	}

	// should consider more sophisticated definition of valid values here.
	// for now check that source and name are not empty strings; due to the
	// default zero values for strings in the "ref" struct, this will also
	// cover the case that varComponents has wrong # of fields
	if ref.fromGroupID == "" || ref.toGroupID == "" || ref.toModuleID == "" || ref.name == "" {
		return varReference{}, fmt.Errorf("%s %s, expected format: %s",
			errorMessages["invalidVar"], yamlReference, expectedVarFormat)
	}
	return ref, nil
}

func (ref modReference) validate(bp Blueprint) error {
	callingModuleGroupIndex := slices.IndexFunc(bp.DeploymentGroups, func(d DeploymentGroup) bool { return d.Name == ref.fromGroupID })
	if callingModuleGroupIndex == -1 {
		return fmt.Errorf("%s: %s", errorMessages["groupNotFound"], ref.fromGroupID)
	}

	targetModuleGroupIndex, err := modToGrp(bp.DeploymentGroups, ref.toModuleID)
	if err != nil {
		return err
	}
	targetModuleGroupName := bp.DeploymentGroups[targetModuleGroupIndex].Name

	// Ensure module is from the correct group
	isInterGroupReference := callingModuleGroupIndex != targetModuleGroupIndex
	isRefToLaterGroup := targetModuleGroupIndex > callingModuleGroupIndex
	isCorrectToGroup := ref.toGroupID == targetModuleGroupName

	if isInterGroupReference {
		if isRefToLaterGroup {
			return fmt.Errorf("%s: %s is in a later group",
				errorMessages["intergroupOrder"], ref.toModuleID)
		}

		if !ref.explicit {
			return fmt.Errorf("%s: %s must specify a group ID before the module ID",
				errorMessages["intergroupImplicit"], ref.toModuleID)
		}
	}

	// at this point, the reference may be intergroup or intragroup. now we
	// only care about correctness of target group ID. better to order this
	// error after enforcing explicitness of intergroup references
	if !isCorrectToGroup {
		return fmt.Errorf("%s: %s.%s",
			errorMessages["referenceWrongGroup"], ref.toGroupID, ref.toModuleID)
	}

	return nil
}

// this function validates every field within a varReference struct and that
// the reference must be to the same or earlier group.
// ref.GroupID: this group must exist or be the value "deployment"
// ref.ID: must be an existing module ID or "vars" (if groupID is "deployment")
// ref.name: must match a module output name or deployment variable name
// ref.explicitInterGroup: intergroup references must explicitly identify the
// target group ID and intragroup references cannot have an incorrect explicit
// group ID
func (ref *varReference) validate(bp Blueprint) error {
	// simplest case to evaluate is a deployment variable's existence
	if ref.toGroupID == "deployment" {
		if ref.toModuleID == "vars" {
			if _, ok := bp.Vars[ref.name]; !ok {
				return fmt.Errorf("%s: %s is not a deployment variable",
					errorMessages["varNotFound"], ref.name)
			}
			return nil
		}
		return fmt.Errorf("%s: %s", errorMessages["invalidDeploymentRef"], ref)
	}

	targetModuleGroupIndex, err := modToGrp(bp.DeploymentGroups, ref.toModuleID)
	if err != nil {
		return err
	}
	targetModuleGroup := bp.DeploymentGroups[targetModuleGroupIndex]

	callingModuleGroupIndex := slices.IndexFunc(bp.DeploymentGroups, func(d DeploymentGroup) bool { return d.Name == ref.fromGroupID })
	if callingModuleGroupIndex == -1 {
		return fmt.Errorf("%s: %s", errorMessages["groupNotFound"], ref.fromGroupID)
	}

	// at this point, we know the target module exists. now record whether it
	// is intergroup and whether it comes in a (disallowed) later group
	isInterGroupReference := targetModuleGroupIndex != callingModuleGroupIndex
	isRefToLaterGroup := targetModuleGroupIndex > callingModuleGroupIndex
	isCorrectToGroup := ref.toGroupID == targetModuleGroup.Name

	// intergroup references must be explicit about group and refer to an earlier group;
	if isInterGroupReference {
		if isRefToLaterGroup {
			return fmt.Errorf("%s: %s is in the later group %s",
				errorMessages["intergroupOrder"], ref.toModuleID, ref.toGroupID)
		}

		if !ref.explicit {
			return fmt.Errorf("%s: %s must specify the group ID %s before the module ID",
				errorMessages["intergroupImplicit"], ref.toModuleID, ref.toGroupID)
		}
	}

	// at this point, the reference may be intergroup or intragroup. now we
	// only care about correctness of target group ID. better to order this
	// error after enforcing explicitness of intergroup references
	if !isCorrectToGroup {
		return fmt.Errorf("%s: %s.%s should be %s.%s",
			errorMessages["referenceWrongGroup"], ref.toGroupID, ref.toModuleID, targetModuleGroup.Name, ref.toModuleID)
	}

	// at this point, we have a valid intragroup or intergroup references to a
	// module. must now determine whether the output value actually exists in
	// the module.
	refModIndex := slices.IndexFunc(targetModuleGroup.Modules, func(m Module) bool { return m.ID == ref.toModuleID })
	if refModIndex == -1 {
		log.Fatalf("Could not find module %s", ref.toModuleID)
	}
	refMod := targetModuleGroup.Modules[refModIndex]
	modInfo, err := modulereader.GetModuleInfo(refMod.Source, refMod.Kind)
	if err != nil {
		log.Fatalf(
			"failed to get info for module at %s while expanding variables: %e",
			refMod.Source, err)
	}
	found := slices.ContainsFunc(modInfo.Outputs, func(o modulereader.VarInfo) bool { return o.Name == ref.name })
	if !found {
		return fmt.Errorf("%s: module %s did not have output %s",
			errorMessages["noOutput"], refMod.ID, ref.name)
	}

	return nil
}

// Needs DeploymentGroups, variable string, current group,
func expandSimpleVariable(context varContext) (string, error) {
	// Get variable contents
	contents := simpleVariableExp.FindStringSubmatch(context.varString)
	if len(contents) != 2 { // Should always be (match, contents) here
		err := fmt.Errorf("%s %s, failed to extract contents: %v",
			errorMessages["invalidVar"], context.varString, contents)
		return "", err
	}

	callingGroup := context.blueprint.DeploymentGroups[context.groupIndex]
	refStr := contents[1]

	varRef, err := identifySimpleVariable(refStr, callingGroup)
	if err != nil {
		return "", err
	}

	if err := varRef.validate(context.blueprint); err != nil {
		return "", err
	}

	var expandedVariable string
	switch varRef.toGroupID {
	case "deployment":
		// deployment variables
		expandedVariable = fmt.Sprintf("((var.%s))", varRef.name)
	case varRef.fromGroupID:
		// intragroup reference can make direct reference to module output
		expandedVariable = fmt.Sprintf("((module.%s.%s))", varRef.toModuleID, varRef.name)
	default:

		// intergroup reference; begin by finding the target module in blueprint
		toGrpIdx := slices.IndexFunc(
			context.blueprint.DeploymentGroups,
			func(g DeploymentGroup) bool { return g.Name == varRef.toGroupID })

		if toGrpIdx == -1 {
			return "", fmt.Errorf("invalid group reference: %s", varRef.toGroupID)
		}
		toGrp := context.blueprint.DeploymentGroups[toGrpIdx]
		toModIdx := slices.IndexFunc(toGrp.Modules, func(m Module) bool { return m.ID == varRef.toModuleID })
		if toModIdx == -1 {
			return "", fmt.Errorf("%s: %s", errorMessages["invalidMod"], varRef.toModuleID)
		}
		toMod := toGrp.Modules[toModIdx]

		// ensure that the target module outputs the value in the root module
		// state and not just internally within its deployment group
		if !slices.Contains(toMod.Outputs, varRef.name) {
			toMod.Outputs = append(toMod.Outputs, varRef.name)
		}

		// TODO: expandedVariable = fmt.Sprintf("((var.%s_%s))", ref.name, ref.ID)
		return "", fmt.Errorf("%s: %s is an intergroup reference",
			errorMessages["varInAnotherGroup"], context.varString)
	}
	return expandedVariable, nil
}

func expandVariable(context varContext) (string, error) {
	matchall := anyVariableExp.FindAllString(context.varString, -1)
	errHint := ""
	for _, element := range matchall {
		// the regex match will include the first matching character
		// this might be (1) "^" or (2) any character EXCEPT "\"
		// if (2), we have to remove the first character from the match
		firstChars := element[0:2]
		if firstChars != "$(" {
			element = strings.Replace(element, element[0:1], "", 1)
		}
		errHint += "\\" + element + " will be rendered as " + element + "\n"
	}
	return "", fmt.Errorf("%s \n%s",
		errorMessages["varWithinStrings"], errHint)
}

// isDeploymentVariable checks if the entire string is just a single deployment variable
func isDeploymentVariable(str string) bool {
	return deploymentVariableExp.MatchString(str)
}

// isSimpleVariable checks if the entire string is just a single variable
func isSimpleVariable(str string) bool {
	return simpleVariableExp.MatchString(str)
}

// hasVariable checks to see if any variable exists in a string
func hasVariable(str string) bool {
	return anyVariableExp.MatchString(str)
}

func handleVariable(prim interface{}, context varContext) (interface{}, error) {
	switch val := prim.(type) {
	case string:
		context.varString = val
		if hasVariable(val) {
			if isSimpleVariable(val) {
				return expandSimpleVariable(context)
			}
			return expandVariable(context)
		}
		return val, nil
	default:
		return val, nil
	}
}

func updateVariableType(value interface{}, context varContext) (interface{}, error) {
	var err error
	switch typedValue := value.(type) {
	case []interface{}:
		interfaceSlice := value.([]interface{})
		{
			for i := 0; i < len(interfaceSlice); i++ {
				interfaceSlice[i], err = updateVariableType(interfaceSlice[i], context)
				if err != nil {
					return interfaceSlice, err
				}
			}
		}
		return typedValue, err
	case map[string]interface{}:
		retMap := map[string]interface{}{}
		for k, v := range typedValue {
			retMap[k], err = updateVariableType(v, context)
			if err != nil {
				return retMap, err
			}
		}
		return retMap, err
	case map[interface{}]interface{}:
		retMap := map[string]interface{}{}
		for k, v := range typedValue {
			retMap[k.(string)], err = updateVariableType(v, context)
			if err != nil {
				return retMap, err
			}
		}
		return retMap, err
	default:
		return handleVariable(value, context)
	}
}

func updateVariables(context varContext, interfaceMap map[string]interface{}) error {
	for key, value := range interfaceMap {
		updatedVal, err := updateVariableType(value, context)
		if err != nil {
			return err
		}
		interfaceMap[key] = updatedVal
	}
	return nil
}

// expandVariables recurses through the data structures in the yaml config and
// expands all variables
func (dc *DeploymentConfig) expandVariables() error {
	for _, validator := range dc.Config.Validators {
		err := updateVariables(varContext{blueprint: dc.Config}, validator.Inputs)
		if err != nil {
			return err

		}
	}

	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod, mod := range grp.Modules {
			context := varContext{
				groupIndex: iGrp,
				modIndex:   iMod,
				blueprint:  dc.Config,
			}
			err := updateVariables(context, mod.Settings)
			if err != nil {
				return err
			}

			// ensure that variable references to projects in required APIs are expanded
			for projectID, requiredAPIs := range mod.RequiredApis {
				if isDeploymentVariable(projectID) {
					s, err := handleVariable(projectID, varContext{blueprint: dc.Config})
					if err != nil {
						return err
					}
					mod.RequiredApis[s.(string)] = slices.Clone(requiredAPIs)
					delete(mod.RequiredApis, projectID)
				}
			}
		}
	}
	return nil
}

// this function adds default validators to the blueprint if none have been
// defined. default validators are only added for global variables that exist
func (dc *DeploymentConfig) addDefaultValidators() error {
	if dc.Config.Validators != nil {
		return nil
	}
	dc.Config.Validators = []validatorConfig{}

	_, projectIDExists := dc.Config.Vars["project_id"]
	_, regionExists := dc.Config.Vars["region"]
	_, zoneExists := dc.Config.Vars["zone"]

	dc.Config.Validators = append(dc.Config.Validators, validatorConfig{
		Validator: testModuleNotUsedName.String(),
		Inputs:    map[string]interface{}{},
	})

	// always add the project ID validator before subsequent validators that can
	// only succeed if credentials can access the project. If the project ID
	// validator fails, all remaining validators are not executed.
	if projectIDExists {
		v := validatorConfig{
			Validator: testProjectExistsName.String(),
			Inputs: map[string]interface{}{
				"project_id": "$(vars.project_id)",
			},
		}
		dc.Config.Validators = append(dc.Config.Validators, v)
	}

	// it is safe to run this validator even if vars.project_id is undefined;
	// it will likely fail but will do so helpfully to the user
	dc.Config.Validators = append(dc.Config.Validators, validatorConfig{
		Validator: "test_apis_enabled",
		Inputs:    map[string]interface{}{},
	})

	if projectIDExists && regionExists {
		v := validatorConfig{
			Validator: testRegionExistsName.String(),
			Inputs: map[string]interface{}{
				"project_id": "$(vars.project_id)",
				"region":     "$(vars.region)",
			},
		}
		dc.Config.Validators = append(dc.Config.Validators, v)

	}

	if projectIDExists && zoneExists {
		v := validatorConfig{
			Validator: testZoneExistsName.String(),
			Inputs: map[string]interface{}{
				"project_id": "$(vars.project_id)",
				"zone":       "$(vars.zone)",
			},
		}
		dc.Config.Validators = append(dc.Config.Validators, v)
	}

	if projectIDExists && regionExists && zoneExists {
		v := validatorConfig{
			Validator: testZoneInRegionName.String(),
			Inputs: map[string]interface{}{
				"project_id": "$(vars.project_id)",
				"region":     "$(vars.region)",
				"zone":       "$(vars.zone)",
			},
		}
		dc.Config.Validators = append(dc.Config.Validators, v)
	}

	return nil
}
