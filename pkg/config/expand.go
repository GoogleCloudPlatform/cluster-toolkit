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

	"golang.org/x/exp/slices"
)

const (
	blueprintLabel        string = "ghpc_blueprint"
	deploymentLabel       string = "ghpc_deployment"
	roleLabel             string = "ghpc_role"
	simpleVariableExp     string = `^\$\((.*)\)$`
	deploymentVariableExp string = `^\$\(vars\.(.*)\)$`
	// Checks if a variable exists only as a substring, ex:
	// Matches: "a$(vars.example)", "word $(vars.example)", "word$(vars.example)", "$(vars.example)"
	// Doesn't match: "\$(vars.example)", "no variable in this string"
	anyVariableExp string = `(^|[^\\])\$\((.*?)\)`
	literalExp     string = `^\(\((.*)\)\)$`
	// the greediness and non-greediness of expression below is important
	// consume all whitespace at beginning and end
	// consume only up to first period to get variable source
	// consume only up to whitespace to get variable name
	literalSplitExp string = `^\(\([[:space:]]*(.*?)\.(.*?)[[:space:]]*\)\)$`
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
	dc.expandVariables()
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
				DeploymentName := blueprint.Vars["deployment_name"]
				prefix := blueprint.BlueprintName
				if DeploymentName != nil {
					prefix += "/" + DeploymentName.(string)
				}
				prefix += "/" + grp.Name
				grp.TerraformBackend.Configuration["prefix"] = prefix
			}
		}
	}
	return nil
}

func getModuleVarName(modID string, varName string) string {
	return fmt.Sprintf("$(%s.%s)", modID, varName)
}

func getModuleInputMap(inputs []modulereader.VarInfo) map[string]string {
	modInputs := make(map[string]string)
	for _, input := range inputs {
		modInputs[input.Name] = input.Type
	}
	return modInputs
}

func useModule(
	mod *Module,
	useMod Module,
	modInputs map[string]string,
	useOutputs []modulereader.VarInfo,
	changedSettings map[string]bool,
) (usedVars []string) {
	usedVars = []string{}
	for _, useOutput := range useOutputs {
		settingName := useOutput.Name
		_, isAlreadySet := mod.Settings[settingName]
		_, hasChanged := changedSettings[settingName]

		// Skip settings explicitly defined by users
		if isAlreadySet && !hasChanged {
			continue
		}

		// This output corresponds to an input that was not explicitly set by the user
		if inputType, ok := modInputs[settingName]; ok {
			modVarName := getModuleVarName(useMod.ID, settingName)
			isInputList := strings.HasPrefix(inputType, "list")
			if isInputList {
				if !isAlreadySet {
					// Input is a list, create an outer list for it
					mod.Settings[settingName] = []interface{}{}
					changedSettings[settingName] = true
					mod.createWrapSettingsWith()
					mod.WrapSettingsWith[settingName] = []string{"flatten(", ")"}
				}
				// Append value list to the outer list
				mod.Settings[settingName] = append(
					mod.Settings[settingName].([]interface{}), modVarName)
				usedVars = append(usedVars, settingName)
			} else if !isAlreadySet {
				// If input is not a list, set value if not already set and continue
				mod.Settings[settingName] = modVarName
				changedSettings[settingName] = true
				usedVars = append(usedVars, settingName)
			}
		}
	}
	return
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
			fromModInputs := getModuleInputMap(fromModInfo.Inputs)
			changedSettings := make(map[string]bool)
			for _, toModID := range fromMod.Use {
				toMod := group.getModuleByID(toModID)
				toModInfo := dc.ModulesInfo[group.Name][toMod.Source]
				if toMod.ID == "" {
					return fmt.Errorf("could not find module %s used by %s in group %s",
						toModID, fromMod.ID, group.Name)
				}
				usedVars := useModule(fromMod, toMod, fromModInputs, toModInfo.Outputs, changedSettings)
				connection := ModConnection{
					toID:            toModID,
					fromID:          fromMod.ID,
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
	if _, exists := globalLabels[blueprintLabel]; !exists {
		globalLabels[blueprintLabel] = defaultLabels[blueprintLabel]
	}
	if _, exists := globalLabels[deploymentLabel]; !exists {
		globalLabels[deploymentLabel] = defaultLabels[deploymentLabel]
	}

	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod, mod := range grp.Modules {
			// Check if labels are set for this module
			if !dc.moduleHasInput(grp.Name, mod.Source, labels) {
				continue
			}

			var modLabels map[string]interface{}
			var ok bool
			// If labels aren't already set, prefill them with globals
			if _, exists := mod.Settings[labels]; !exists {
				modLabels = make(map[string]interface{})
			} else {
				// Cast into map so we can index into them
				modLabels, ok = mod.Settings[labels].(map[string]interface{})

				if !ok {
					return fmt.Errorf("%s, Module %s, labels type: %T",
						errorMessages["settingsLabelType"], mod.ID, mod.Settings[labels])
				}
			}

			// Add the role (e.g. compute, network, etc)
			if _, exists := modLabels[roleLabel]; !exists {
				modLabels[roleLabel] = getRole(mod.Source)
			}
			dc.Config.DeploymentGroups[iGrp].Modules[iMod].Settings[labels] =
				modLabels
		}
	}
	dc.Config.Vars[labels] = globalLabels
	return nil
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
		val, err := updateVariableType(v, varContext{}, make(map[string]int))
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

/*
A module reference is made by the use keyword and is subject to IGC constraints
of references (ordering, explicitness). It has the following fields:
  - ID: a module ID
  - ToGroupID: the deployment group in which the module is *expected* to be found
  - FromGroupID: the deployment group from which the reference is made
  - Explicit: a boolean value indicating whether the user made a reference that
    explicitly identified ToGroupID rather than inferring it using FromGroupID
*/
type modReference struct {
	ID          string
	ToGroupID   string
	FromGroupID string
	Explicit    bool
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
		ref.ID = modComponents[0]
		ref.ToGroupID = dg.Name
		ref.FromGroupID = dg.Name
	case 2:
		ref.ToGroupID = modComponents[0]
		ref.ID = modComponents[1]
		ref.FromGroupID = dg.Name
		ref.Explicit = true
	}

	// should consider more sophisticated definition of valid values here.
	// for now check that no fields are the empty string; due to the default
	// zero values for strings in the "ref" struct, this will also cover the
	// case that modComponents has wrong # of fields
	if ref.ID == "" || ref.ToGroupID == "" || ref.FromGroupID == "" {
		return ref, fmt.Errorf("%s: %s, expected %s",
			errorMessages["invalidMod"], yamlReference, expectedModFormat)
	}

	return ref, nil
}

/*
A variable reference has the following fields
  - ID: a module ID or "vars" if referring to a deployment variable
  - Name: the name of the module output or deployment variable
  - ToGroupID: the deployment group in which the module is *expected* to be found
  - FromGroupID: the deployment group from which the reference is made
  - Explicit: a boolean value indicating whether the user made a reference that
    explicitly identified ToGroupID rather than inferring it using FromGroupID
*/
type varReference struct {
	ID          string
	ToGroupID   string
	FromGroupID string
	Name        string
	Explicit    bool
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
	ref.FromGroupID = dg.Name

	// intra-group references length 2 and inter-group references length 3
	switch len(varComponents) {
	case 2:
		ref.ID = varComponents[0]
		ref.Name = varComponents[1]

		if ref.ID == "vars" {
			ref.ToGroupID = "deployment"
		} else {
			ref.ToGroupID = dg.Name
		}
	case 3:
		ref.ToGroupID = varComponents[0]
		ref.ID = varComponents[1]
		ref.Name = varComponents[2]
		ref.Explicit = true
	}

	// should consider more sophisticated definition of valid values here.
	// for now check that source and name are not empty strings; due to the
	// default zero values for strings in the "ref" struct, this will also
	// cover the case that varComponents has wrong # of fields
	if ref.FromGroupID == "" || ref.ToGroupID == "" || ref.ID == "" || ref.Name == "" {
		return varReference{}, fmt.Errorf("%s %s, expected format: %s",
			errorMessages["invalidVar"], yamlReference, expectedVarFormat)
	}
	return ref, nil
}

func (ref *modReference) validate(depGroups []DeploymentGroup, modToGrp map[string]int) error {
	callingModuleGroupIndex := slices.IndexFunc(depGroups, func(d DeploymentGroup) bool { return d.Name == ref.FromGroupID })
	if callingModuleGroupIndex == -1 {
		return fmt.Errorf("%s: %s", errorMessages["groupNotFound"], ref.FromGroupID)
	}

	targetModuleGroupIndex, ok := modToGrp[ref.ID]
	if !ok {
		return fmt.Errorf("%s: module %s was not found",
			errorMessages["varNotFound"], ref.ID)
	}
	targetModuleGroupName := depGroups[targetModuleGroupIndex].Name

	// Ensure module is from the correct group
	isInterGroupReference := callingModuleGroupIndex != targetModuleGroupIndex
	isRefToLaterGroup := targetModuleGroupIndex > callingModuleGroupIndex
	isCorrectToGroup := ref.ToGroupID == targetModuleGroupName

	if isInterGroupReference {
		if isRefToLaterGroup {
			return fmt.Errorf("%s: %s is in a later group",
				errorMessages["intergroupOrder"], ref.ID)
		}

		if !ref.Explicit {
			return fmt.Errorf("%s: %s must specify a group ID before the module ID",
				errorMessages["intergroupImplicit"], ref.ID)
		}
	}

	// at this point, the reference may be intergroup or intragroup. now we
	// only care about correctness of target group ID. better to order this
	// error after enforcing explicitness of intergroup references
	if !isCorrectToGroup {
		return fmt.Errorf("%s: %s.%s",
			errorMessages["referenceWrongGroup"], ref.ToGroupID, ref.ID)
	}

	return nil
}

// this function validates every field within a varReference struct and that
// the reference must be to the same or earlier group.
// ref.GroupID: this group must exist or be the value "deployment"
// ref.ID: must be an existing module ID or "vars" (if groupID is "deployment")
// ref.Name: must match a module output name or deployment variable name
// ref.ExplicitInterGroup: intergroup references must explicitly identify the
// target group ID and intragroup references cannot have an incorrect explicit
// group ID
func (ref *varReference) validate(depGroups []DeploymentGroup, vars map[string]interface{}, modToGrp map[string]int) error {
	// simplest case to evaluate is a deployment variable's existence
	if ref.ToGroupID == "deployment" {
		if ref.ID == "vars" {
			if _, ok := vars[ref.Name]; !ok {
				return fmt.Errorf("%s: %s is not a deployment variable",
					errorMessages["varNotFound"], ref.Name)
			}
			return nil
		}
		return fmt.Errorf("%s: %s", errorMessages["invalidDeploymentRef"], ref.ID)
	}

	targetModuleGroupIndex, ok := modToGrp[ref.ID]
	if !ok {
		return fmt.Errorf("%s: module %s was not found",
			errorMessages["varNotFound"], ref.ID)
	}
	targetModuleGroup := depGroups[targetModuleGroupIndex]

	callingModuleGroupIndex := slices.IndexFunc(depGroups, func(d DeploymentGroup) bool { return d.Name == ref.FromGroupID })
	if callingModuleGroupIndex == -1 {
		return fmt.Errorf("%s: %s", errorMessages["groupNotFound"], ref.FromGroupID)
	}

	// at this point, we know the target module exists. now record whether it
	// is intergroup and whether it comes in a (disallowed) later group
	isInterGroupReference := targetModuleGroupIndex != callingModuleGroupIndex
	isRefToLaterGroup := targetModuleGroupIndex > callingModuleGroupIndex
	isCorrectToGroup := ref.ToGroupID == targetModuleGroup.Name

	// intergroup references must be explicit about group and refer to an earlier group;
	if isInterGroupReference {
		if isRefToLaterGroup {
			return fmt.Errorf("%s: %s is in the later group %s",
				errorMessages["intergroupOrder"], ref.ID, ref.ToGroupID)
		}

		if !ref.Explicit {
			return fmt.Errorf("%s: %s must specify the group ID %s before the module ID",
				errorMessages["intergroupImplicit"], ref.ID, ref.ToGroupID)
		}
	}

	// at this point, the reference may be intergroup or intragroup. now we
	// only care about correctness of target group ID. better to order this
	// error after enforcing explicitness of intergroup references
	if !isCorrectToGroup {
		return fmt.Errorf("%s: %s.%s should be %s.%s",
			errorMessages["referenceWrongGroup"], ref.ToGroupID, ref.ID, targetModuleGroup.Name, ref.ID)
	}

	// at this point, we have a valid intragroup or intergroup references to a
	// module. must now determine whether the output value actually exists in
	// the module.
	refModIndex := slices.IndexFunc(targetModuleGroup.Modules, func(m Module) bool { return m.ID == ref.ID })
	if refModIndex == -1 {
		log.Fatalf("Could not find module %s", ref.ID)
	}
	refMod := targetModuleGroup.Modules[refModIndex]
	modInfo, err := modulereader.GetModuleInfo(refMod.Source, refMod.Kind)
	if err != nil {
		log.Fatalf(
			"failed to get info for module at %s while expanding variables: %e",
			refMod.Source, err)
	}
	found := slices.ContainsFunc(modInfo.Outputs, func(o modulereader.VarInfo) bool { return o.Name == ref.Name })
	if !found {
		return fmt.Errorf("%s: module %s did not have output %s",
			errorMessages["noOutput"], refMod.ID, ref.Name)
	}

	return nil
}

// Needs DeploymentGroups, variable string, current group,
func expandSimpleVariable(
	context varContext,
	modToGrp map[string]int) (string, error) {

	// Get variable contents
	re := regexp.MustCompile(simpleVariableExp)
	contents := re.FindStringSubmatch(context.varString)
	if len(contents) != 2 { // Should always be (match, contents) here
		err := fmt.Errorf("%s %s, failed to extract contents: %v",
			errorMessages["invalidVar"], context.varString, contents)
		return "", err
	}

	callingGroup := context.blueprint.DeploymentGroups[context.groupIndex]
	refStr := contents[1]

	ref, err := identifySimpleVariable(refStr, callingGroup)
	if err != nil {
		return "", err
	}

	err = ref.validate(context.blueprint.DeploymentGroups, context.blueprint.Vars, modToGrp)
	if err != nil {
		return "", err
	}

	var expandedVariable string
	if ref.ToGroupID == "deployment" {
		expandedVariable = fmt.Sprintf("((var.%s))", ref.Name)
	} else {
		if ref.FromGroupID != ref.ToGroupID {
			// TODO: expandedVariable = fmt.Sprintf("((var.%s_%s))", ref.Name, ref.ID)
			return "", fmt.Errorf("%s: %s is an intergroup reference",
				errorMessages["varInAnotherGroup"], context.varString)
		}
		expandedVariable = fmt.Sprintf("((module.%s.%s))", ref.ID, ref.Name)
	}
	return expandedVariable, nil
}

func expandVariable(
	context varContext,
	modToGrp map[string]int) (string, error) {
	re := regexp.MustCompile(anyVariableExp)
	matchall := re.FindAllString(context.varString, -1)
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
	matched, err := regexp.MatchString(deploymentVariableExp, str)
	if err != nil {
		log.Fatalf("isDeploymentVariable(%s): %v", str, err)
	}
	return matched
}

// isSimpleVariable checks if the entire string is just a single variable
func isSimpleVariable(str string) bool {
	matched, err := regexp.MatchString(simpleVariableExp, str)
	if err != nil {
		log.Fatalf("isSimpleVariable(%s): %v", str, err)
	}
	return matched
}

// hasVariable checks to see if any variable exists in a string
func hasVariable(str string) bool {
	matched, err := regexp.MatchString(anyVariableExp, str)
	if err != nil {
		log.Fatalf("hasVariable(%s): %v", str, err)
	}
	return matched
}

func handleVariable(
	prim interface{},
	context varContext,
	modToGrp map[string]int) (interface{}, error) {
	switch val := prim.(type) {
	case string:
		context.varString = val
		if hasVariable(val) {
			if isSimpleVariable(val) {
				return expandSimpleVariable(context, modToGrp)
			}
			return expandVariable(context, modToGrp)
		}
		return val, nil
	default:
		return val, nil
	}
}

func updateVariableType(
	value interface{},
	context varContext,
	modToGrp map[string]int) (interface{}, error) {
	var err error
	switch typedValue := value.(type) {
	case []interface{}:
		interfaceSlice := value.([]interface{})
		{
			for i := 0; i < len(interfaceSlice); i++ {
				interfaceSlice[i], err = updateVariableType(
					interfaceSlice[i], context, modToGrp)
				if err != nil {
					return interfaceSlice, err
				}
			}
		}
		return typedValue, err
	case map[string]interface{}:
		retMap := map[string]interface{}{}
		for k, v := range typedValue {
			retMap[k], err = updateVariableType(v, context, modToGrp)
			if err != nil {
				return retMap, err
			}
		}
		return retMap, err
	case map[interface{}]interface{}:
		retMap := map[string]interface{}{}
		for k, v := range typedValue {
			retMap[k.(string)], err = updateVariableType(v, context, modToGrp)
			if err != nil {
				return retMap, err
			}
		}
		return retMap, err
	default:
		return handleVariable(value, context, modToGrp)
	}
}

func updateVariables(
	context varContext,
	interfaceMap map[string]interface{},
	modToGrp map[string]int) error {
	for key, value := range interfaceMap {
		updatedVal, err := updateVariableType(value, context, modToGrp)
		if err != nil {
			return err
		}
		interfaceMap[key] = updatedVal
	}
	return nil
}

// expandVariables recurses through the data structures in the yaml config and
// expands all variables
func (dc *DeploymentConfig) expandVariables() {
	for _, validator := range dc.Config.Validators {
		err := updateVariables(varContext{blueprint: dc.Config}, validator.Inputs, make(map[string]int))
		if err != nil {
			log.Fatalf("expandVariables: %v", err)
		}
	}

	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod, mod := range grp.Modules {
			context := varContext{
				groupIndex: iGrp,
				modIndex:   iMod,
				blueprint:  dc.Config,
			}
			err := updateVariables(
				context,
				mod.Settings,
				dc.ModuleToGroup)
			if err != nil {
				log.Fatalf("expandVariables: %v", err)
			}

			// ensure that variable references to projects in required APIs are expanded
			for projectID, requiredAPIs := range mod.RequiredApis {
				if isDeploymentVariable(projectID) {
					s, err := handleVariable(projectID, varContext{blueprint: dc.Config}, make(map[string]int))
					if err != nil {
						log.Fatalf("expandVariables: %v", err)
					}
					mod.RequiredApis[s.(string)] = slices.Clone(requiredAPIs)
					delete(mod.RequiredApis, projectID)
				}
			}
		}
	}
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

	// always add the project ID validator first; remaining validators can only
	// succeed if credentials can access the project
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

	dc.Config.Validators = append(dc.Config.Validators, validatorConfig{
		Validator: testModuleNotUsedName.String(),
		Inputs:    map[string]interface{}{},
	})

	return nil
}
