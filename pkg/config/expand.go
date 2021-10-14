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

// combineLabels sets defaults for labels based on other variables and merges
// the global labels defined in Vars with resource setting labels. It also
// determines the role and sets it for each resource independently.
func (bc *BlueprintConfig) combineLabels() error {
	defaultLabels := map[interface{}]interface{}{
		blueprintLabel:  bc.Config.BlueprintName,
		deploymentLabel: getDeploymentName(bc.Config.Vars),
	}
	labels := "labels"
	var globalLabels map[interface{}]interface{}

	// Add defaults to global labels if they don't already exist
	if _, exists := bc.Config.Vars[labels]; !exists {
		bc.Config.Vars[labels] = defaultLabels
	}

	// Cast global labels so we can index into them
	globalLabels, ok := bc.Config.Vars[labels].(map[interface{}]interface{})
	if !ok {
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
			if input.Required == true {
				// Exists? Continue
				if _, ok := res.Settings[input.Name]; ok {
					continue
				}

				// Exists at top level? Update and Continue
				if _, ok := globalVars[input.Name]; ok {
					res.Settings[input.Name] = fmt.Sprintf("((var.%s))", input.Name)
				} else {
					return fmt.Errorf("%s: Resource.ID: %s Setting: %s",
						errorMessages["missingSetting"], res.ID, input.Name)
				}
			}
		}
	}
	return nil
}

// applyGlobalVariables takes any variables defined at the global level and
// applies them to resources settings if not already set.
func (bc *BlueprintConfig) applyGlobalVariables() error {
	var err error
	for _, grp := range bc.Config.ResourceGroups {
		err = applyGlobalVarsInGroup(
			grp, bc.ResourcesInfo[grp.Name], bc.Config.Vars)
		if err != nil {
			break
		}
	}
	return err
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
	resInfo := resreader.Factory(refRes.Kind).GetInfo(refRes.Source)

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
	switch prim.(type) {
	case string:
		str := prim.(string)
		context.varString = str
		if hasVariable(str) {
			if isSimpleVariable(str) {
				return expandSimpleVariable(context, resToGrp)
			}
			return expandVariable(context, resToGrp)
		}
		return prim, nil
	default:
		return prim, nil
	}
}

func updateVariableType(
	value interface{},
	context varContext,
	resToGrp map[string]int) (interface{}, error) {
	var err error
	switch value.(type) {
	case []interface{}:
		interfaceSlice := value.([]interface{})
		{
			for i := 0; i < len(interfaceSlice); i++ {
				interfaceSlice[i], err = updateVariableType(
					interfaceSlice[i], context, resToGrp)
				if err != nil {
					break
				}
			}
		}
		return interfaceSlice, err
	case map[interface{}]interface{}:
		interfaceMap := value.(map[interface{}]interface{})
		retMap := map[interface{}]interface{}{}
		for k, v := range interfaceMap {
			retMap[k], err = updateVariableType(v, context, resToGrp)
			if err != nil {
				break
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
	var err error
	for key, value := range interfaceMap {
		interfaceMap[key], err = updateVariableType(value, context, resToGrp)
		if err != nil {
			break
		}
	}
	return err
}

// handlePrimitives recurses through the data structures in the yaml config and
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
