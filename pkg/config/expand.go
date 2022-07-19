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
	"hpc-toolkit/pkg/sourcereader"
)

const (
	blueprintLabel    string = "ghpc_blueprint"
	deploymentLabel   string = "ghpc_deployment"
	roleLabel         string = "ghpc_role"
	simpleVariableExp string = `^\$\((.*)\)$`
	anyVariableExp    string = `\$\((.*)\)`
	literalExp        string = `^\(\((.*)\)\)$`
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
) {
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
			} else if !isAlreadySet {
				// If input is not a list, set value if not already set and continue
				mod.Settings[settingName] = modVarName
				changedSettings[settingName] = true
			}
		}
	}
}

// applyUseModules applies variables from modules listed in the "use" field
// when/if applicable
func (dc *DeploymentConfig) applyUseModules() error {
	for iGrp := range dc.Config.DeploymentGroups {
		group := &dc.Config.DeploymentGroups[iGrp]
		for iMod := range group.Modules {
			mod := &group.Modules[iMod]
			modInfo := dc.ModulesInfo[group.Name][mod.Source]
			modInputs := getModuleInputMap(modInfo.Inputs)
			changedSettings := make(map[string]bool)
			for _, useModID := range mod.Use {
				useMod := group.getModuleByID(useModID)
				useInfo := dc.ModulesInfo[group.Name][useMod.Source]
				if useMod.ID == "" {
					return fmt.Errorf("could not find module %s used by %s in group %s",
						useModID, mod.ID, group.Name)
				}
				useModule(mod, useMod, modInputs, useInfo.Outputs, changedSettings)
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

	// Break up variable into source and value
	varComponents := strings.SplitN(contents[1], ".", 2)
	if len(varComponents) != 2 {
		return "", fmt.Errorf("%s %s, expected format: %s",
			errorMessages["invalidVar"], context.varString, expectedVarFormat)
	}
	varSource := varComponents[0]
	varValue := varComponents[1]

	if varSource == "vars" { // Global variable
		// Verify global variable exists
		if _, ok := context.blueprint.Vars[varValue]; !ok {
			return "", fmt.Errorf("%s: %s is not a deployment variable",
				errorMessages["varNotFound"], context.varString)
		}
		return fmt.Sprintf("((var.%s))", varValue), nil
	}

	// Module variable
	// Verify module exists
	refGrpIndex, ok := modToGrp[varSource]
	if !ok {
		return "", fmt.Errorf("%s: module %s was not found",
			errorMessages["varNotFound"], varSource)
	}
	if refGrpIndex != context.groupIndex {
		return "", fmt.Errorf("%s: module %s was defined in group %d and called from group %d",
			errorMessages["varInAnotherGroup"], varSource, refGrpIndex, context.groupIndex)
	}

	// Get the module info
	refGrp := context.blueprint.DeploymentGroups[refGrpIndex]
	refModIndex := -1
	for i := range refGrp.Modules {
		if refGrp.Modules[i].ID == varSource {
			refModIndex = i
			break
		}
	}
	if refModIndex == -1 {
		log.Fatalf("Could not find module referenced by variable %s",
			context.varString)
	}
	refMod := refGrp.Modules[refModIndex]
	reader := sourcereader.Factory(refMod.Source)
	modInfo, err := reader.GetModuleInfo(refMod.Source, refMod.Kind)
	if err != nil {
		log.Fatalf(
			"failed to get info for module at %s while expanding variables: %e",
			refMod.Source, err)
	}

	// Verify output exists in module
	found := false
	for _, output := range modInfo.Outputs {
		if output.Name == varValue {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("%s: module %s did not have output %s",
			errorMessages["noOutput"], refMod.ID, varValue)
	}
	return fmt.Sprintf("((module.%s.%s))", varSource, varValue), nil
}

func expandVariable(
	context varContext,
	modToGrp map[string]int) (string, error) {
	return "", fmt.Errorf("%s: expandVariable", errorMessages["notImplemented"])
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
		for iMod := range grp.Modules {
			context := varContext{
				groupIndex: iGrp,
				modIndex:   iMod,
				blueprint:  dc.Config,
			}
			err := updateVariables(
				context,
				dc.Config.DeploymentGroups[iGrp].Modules[iMod].Settings,
				dc.ModuleToGroup)
			if err != nil {
				log.Fatalf("expandVariables: %v", err)
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

	if projectIDExists {
		v := validatorConfig{
			Validator: testProjectExistsName.String(),
			Inputs: map[string]interface{}{
				"project_id": "$(vars.project_id)",
			},
		}
		dc.Config.Validators = append(dc.Config.Validators, v)
	}

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
