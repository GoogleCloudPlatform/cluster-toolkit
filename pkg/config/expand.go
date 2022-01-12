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

package config

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"path/filepath"

	"hpc-toolkit/pkg/resreader"
)

const (
	blueprintLabel    string = "ghpc_blueprint"
	deploymentLabel   string = "ghpc_deployment"
	roleLabel         string = "ghpc_role"
	simpleVariableExp string = `^\$\((.*)\)$`
	anyVariableExp    string = `\$\((.*)\)`
)

// expand expands variables and strings in the yaml config. Used directly by
// ExpandConfig for the create and expand commands.
func (bc *BlueprintConfig) expand() {
	bc.addSettingsToResources()
	if err := bc.combineLabels(); err != nil {
		log.Fatalf(
			"failed to update resources labels when expanding the config: %v", err)
	}

	if err := bc.applyUseResources(); err != nil {
		log.Fatalf(
			"failed to apply \"use\" resources when expanding the config: %v", err)
	}

	if err := bc.applyGlobalVariables(); err != nil {
		log.Fatalf(
			"failed to apply global variables in resources when expanding the config: %v",
			err)
	}
	bc.expandVariables()
}

func (bc *BlueprintConfig) addSettingsToResources() {
	for iGrp, grp := range bc.Config.ResourceGroups {
		for iRes, res := range grp.Resources {
			if res.Settings == nil {
				bc.Config.ResourceGroups[iGrp].Resources[iRes].Settings =
					make(map[string]interface{})
			}
		}
	}
}

func getResourceVarName(resID string, varName string) string {
	return fmt.Sprintf("$(%s.%s)", resID, varName)
}

func stringSliceContains(slice []string, value string) bool {
	for _, elem := range slice {
		if elem == value {
			return true
		}
	}
	return false
}

func useResource(
	res *Resource,
	useRes Resource,
	info map[string]resreader.ResourceInfo,
	hardSettings []string,
) {
	resInfo := info[res.Source]
	useInfo := info[useRes.Source]

	for _, useOutput := range useInfo.Outputs {
		// Skip if setting is already set
		if stringSliceContains(hardSettings, useOutput.Name) {
			continue
		}

		for _, resInput := range resInfo.Inputs {
			if useOutput.Name == resInput.Name {
				resVarName := getResourceVarName(useRes.ID, useOutput.Name)
				isInputList := strings.HasPrefix(resInput.Type, "list")
				_, isAlreadySet := res.Settings[resInput.Name]
				// If input is not a list, set value if not already set and continue
				if !isInputList {
					if !isAlreadySet {
						res.Settings[resInput.Name] = resVarName
					}
					continue
				}
				if !isAlreadySet {
					// Input is a list, create an outer list for it
					res.Settings[resInput.Name] = []interface{}{}
				}
				// Append value list to the outer list
				res.Settings[resInput.Name] = append(
					res.Settings[resInput.Name].([]interface{}), resVarName)
				res.createWrapSettingsWith()
				res.WrapSettingsWith[resInput.Name] = []string{"flatten(", ")"}
			}
		}
	}
}

// applyUseResources applies variables from resources listed in the "use" field
// when/if applicable
func (bc *BlueprintConfig) applyUseResources() error {
	for iGrp := range bc.Config.ResourceGroups {
		group := &bc.Config.ResourceGroups[iGrp]
		for iRes := range group.Resources {
			res := &group.Resources[iRes]
			// Determine which settings are already set before starting
			hardSettings := res.getSetSettings()
			for _, useResID := range res.Use {
				useRes := group.getResourceByID(useResID)
				if useRes.ID == "" {
					return fmt.Errorf("could not find resource %s used by %s in group %s",
						useResID, res.ID, group.Name)
				}
				useResource(res, useRes, bc.ResourcesInfo[group.Name], hardSettings)
			}
		}
	}
	return nil
}

func (bc BlueprintConfig) resourceHasInput(
	resGroup string, source string, inputName string) bool {
	for _, input := range bc.ResourcesInfo[resGroup][source].Inputs {
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

func getDeploymentName(vars map[string]interface{}) string {
	deployName, exists := vars["deployment_name"]
	if exists {
		return deployName.(string)
	}
	return "undefined"
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
// the global labels defined in Vars with resource setting labels. It also
// determines the role and sets it for each resource independently.
func (bc *BlueprintConfig) combineLabels() error {
	defaultLabels := map[string]interface{}{
		blueprintLabel:  bc.Config.BlueprintName,
		deploymentLabel: getDeploymentName(bc.Config.Vars),
	}
	labels := "labels"
	var globalLabels map[string]interface{}

	// Add defaults to global labels if they don't already exist
	if _, exists := bc.Config.Vars[labels]; !exists {
		bc.Config.Vars[labels] = defaultLabels
	}

	// Cast global labels so we can index into them
	globalLabels, err := toStringInterfaceMap(bc.Config.Vars[labels])
	if err != nil {
		return fmt.Errorf(
			"%s: found %T",
			errorMessages["globalLabelType"],
			bc.Config.Vars[labels])
	}

	// Add both default labels if they don't already exist
	if _, exists := globalLabels[blueprintLabel]; !exists {
		globalLabels[blueprintLabel] = defaultLabels[blueprintLabel]
	}
	if _, exists := globalLabels[deploymentLabel]; !exists {
		globalLabels[deploymentLabel] = defaultLabels[deploymentLabel]
	}

	for iGrp, grp := range bc.Config.ResourceGroups {
		for iRes, res := range grp.Resources {
			// Check if labels are set for this resource
			if !bc.resourceHasInput(grp.Name, res.Source, labels) {
				continue
			}

			var resLabels map[interface{}]interface{}
			var ok bool
			// If labels aren't already set, prefill them with globals
			if _, exists := res.Settings[labels]; !exists {
				resLabels = make(map[interface{}]interface{})
			} else {
				// Cast into map so we can index into them
				resLabels, ok = res.Settings[labels].(map[interface{}]interface{})

				if !ok {
					return fmt.Errorf("%s, Resource %s, labels type: %T",
						errorMessages["settingsLabelType"], res.ID, res.Settings[labels])
				}
			}

			// Add the role (e.g. compute, network, etc)
			if _, exists := resLabels[roleLabel]; !exists {
				resLabels[roleLabel] = getRole(res.Source)
			}
			bc.Config.ResourceGroups[iGrp].Resources[iRes].Settings[labels] =
				resLabels
		}
	}
	bc.Config.Vars[labels] = globalLabels
	return nil
}

func applyGlobalVarsInGroup(
	resourceGroup ResourceGroup,
	resInfo map[string]resreader.ResourceInfo,
	globalVars map[string]interface{}) error {
	for _, res := range resourceGroup.Resources {
		for _, input := range resInfo[res.Source].Inputs {

			// Resource setting exists? Nothing more needs to be done.
			if _, ok := res.Settings[input.Name]; ok {
				continue
			}

			// If it's not set, is there a global we can use?
			if _, ok := globalVars[input.Name]; ok {
				res.Settings[input.Name] = fmt.Sprintf("((var.%s))", input.Name)
				continue
			}

			if input.Required {
				// It's not explicitly set, and not global is set
				// Fail if no default has been set
				return fmt.Errorf("%s: Resource.ID: %s Setting: %s",
					errorMessages["missingSetting"], res.ID, input.Name)
			}
			// Default exists, the resource will handle it
		}
	}
	return nil
}

func updateGlobalVarTypes(vars map[string]interface{}) error {
	for k, v := range vars {
		val, err := updateVariableType(v, varContext{}, make(map[string]int))
		if err != nil {
			return fmt.Errorf("error setting type for global variabl %s: %v", k, err)
		}
		vars[k] = val
	}
	return nil
}

// applyGlobalVariables takes any variables defined at the global level and
// applies them to resources settings if not already set.
func (bc *BlueprintConfig) applyGlobalVariables() error {
	// Update global variable types to match
	if err := updateGlobalVarTypes(bc.Config.Vars); err != nil {
		return err
	}

	for _, grp := range bc.Config.ResourceGroups {
		err := applyGlobalVarsInGroup(
			grp, bc.ResourcesInfo[grp.Name], bc.Config.Vars)
		if err != nil {
			return err
		}
	}
	return nil
}

type varContext struct {
	varString  string
	groupIndex int
	resIndex   int
	yamlConfig YamlConfig
}

// Needs ResourceGroups, variable string, current group,
func expandSimpleVariable(
	context varContext,
	resToGrp map[string]int) (string, error) {

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
		if _, ok := context.yamlConfig.Vars[varValue]; !ok {
			return "", fmt.Errorf("%s: %s is not a global variable",
				errorMessages["varNotFound"], context.varString)
		}
		return fmt.Sprintf("((var.%s))", varValue), nil
	}

	// Resource variable
	// Verify resource exists
	refGrpIndex, ok := resToGrp[varSource]
	if !ok {
		return "", fmt.Errorf("%s: resource %s was not found",
			errorMessages["varNotFound"], varSource)
	}
	if refGrpIndex != context.groupIndex {
		log.Fatalf("Unimplemented: references to other groups are not yet supported")
	}

	// Get the resource info
	refGrp := context.yamlConfig.ResourceGroups[refGrpIndex]
	refResIndex := -1
	for i := range refGrp.Resources {
		if refGrp.Resources[i].ID == varSource {
			refResIndex = i
			break
		}
	}
	if refResIndex == -1 {
		log.Fatalf("Could not find resource referenced by variable %s",
			context.varString)
	}
	refRes := refGrp.Resources[refResIndex]
	reader := resreader.Factory(refRes.Kind)
	resInfo, err := reader.GetInfo(refRes.Source)
	if err != nil {
		log.Fatalf(
			"failed to get info for resource at %s while expanding variables: %e",
			refRes.Source, err)
	}

	// Verify output exists in resource
	found := false
	for _, output := range resInfo.Outputs {
		if output.Name == varValue {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("%s: resource %s did not have output %s",
			errorMessages["noOutput"], refRes.ID, varValue)
	}
	return fmt.Sprintf("((module.%s.%s))", varSource, varValue), nil
}

func expandVariable(
	context varContext,
	resToGrp map[string]int) (string, error) {
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
	resToGrp map[string]int) (interface{}, error) {
	switch val := prim.(type) {
	case string:
		context.varString = val
		if hasVariable(val) {
			if isSimpleVariable(val) {
				return expandSimpleVariable(context, resToGrp)
			}
			return expandVariable(context, resToGrp)
		}
		return val, nil
	default:
		return val, nil
	}
}

func updateVariableType(
	value interface{},
	context varContext,
	resToGrp map[string]int) (interface{}, error) {
	var err error
	switch typedValue := value.(type) {
	case []interface{}:
		interfaceSlice := value.([]interface{})
		{
			for i := 0; i < len(interfaceSlice); i++ {
				interfaceSlice[i], err = updateVariableType(
					interfaceSlice[i], context, resToGrp)
				if err != nil {
					return interfaceSlice, err
				}
			}
		}
		return typedValue, err
	case map[string]interface{}:
		retMap := map[string]interface{}{}
		for k, v := range typedValue {
			retMap[k], err = updateVariableType(v, context, resToGrp)
			if err != nil {
				return retMap, err
			}
		}
		return retMap, err
	case map[interface{}]interface{}:
		retMap := map[string]interface{}{}
		for k, v := range typedValue {
			retMap[k.(string)], err = updateVariableType(v, context, resToGrp)
			if err != nil {
				return retMap, err
			}
		}
		return retMap, err
	default:
		return handleVariable(value, context, resToGrp)
	}
}

func updateVariables(
	context varContext,
	interfaceMap map[string]interface{},
	resToGrp map[string]int) error {
	for key, value := range interfaceMap {
		updatedVal, err := updateVariableType(value, context, resToGrp)
		if err != nil {
			return err
		}
		interfaceMap[key] = updatedVal
	}
	return nil
}

// expandVariables recurses through the data structures in the yaml config and
// expands all variables
func (bc *BlueprintConfig) expandVariables() {
	for iGrp, grp := range bc.Config.ResourceGroups {
		for iRes := range grp.Resources {
			context := varContext{
				groupIndex: iGrp,
				resIndex:   iRes,
				yamlConfig: bc.Config,
			}
			err := updateVariables(
				context,
				bc.Config.ResourceGroups[iGrp].Resources[iRes].Settings,
				bc.ResourceToGroup)
			if err != nil {
				log.Fatalf("expandVariables: %v", err)
			}
		}
	}
}
