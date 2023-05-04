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

package config

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/validators"

	"github.com/pkg/errors"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

const (
	validationWarningMsg = "Validation failures were treated as a warning, continuing to create blueprint."
	validationErrorMsg   = "validation failed due to the issues listed above"
	funcErrorMsgTemplate = "validator %s failed"
)

// InvalidSettingError signifies a problem with the supplied setting name in a
// module definition.
type InvalidSettingError struct {
	cause string
}

func (err *InvalidSettingError) Error() string {
	return fmt.Sprintf("invalid setting provided to a module, cause: %v", err.cause)
}

// validate is the top-level function for running the validation suite.
func (dc DeploymentConfig) validate() {
	// Drop the flags for log to improve readability only for running the validation suite
	log.SetFlags(0)

	if err := dc.validateVars(); err != nil {
		log.Fatal(err)
	}

	// variables should be validated before running validators
	if err := dc.executeValidators(); err != nil {
		log.Fatal(err)
	}

	if err := dc.validateModules(); err != nil {
		log.Fatal(err)
	}
	if err := dc.validateModuleSettings(); err != nil {
		log.Fatal(err)
	}

	// Set it back to the initial value
	log.SetFlags(log.LstdFlags)
}

// performs validation of global variables
func (dc DeploymentConfig) executeValidators() error {
	var errored, warned bool
	implementedValidators := dc.getValidators()

	if dc.Config.ValidationLevel == validationIgnore {
		return nil
	}

	for _, validator := range dc.Config.Validators {
		if validator.Skip {
			continue
		}

		f, ok := implementedValidators[validator.Validator]
		if !ok {
			errored = true
			log.Printf("%s is not an implemented validator", validator.Validator)
			continue
		}

		if err := f(validator); err != nil {
			var prefix string
			switch dc.Config.ValidationLevel {
			case validationWarning:
				warned = true
				prefix = "warning: "
			default:
				errored = true
				prefix = "error: "
			}
			log.Print(prefix, err)
			log.Println()

			// do not bother running further validators if project ID could not be found
			if validator.Validator == testProjectExistsName.String() {
				break
			}
		}

	}

	if warned || errored {
		log.Println("One or more blueprint validators has failed. See messages above for suggested")
		log.Println("actions. General troubleshooting guidance and instructions for configuring")
		log.Println("validators are shown below.")
		log.Println("")
		log.Println("- https://goo.gle/hpc-toolkit-troubleshooting")
		log.Println("- https://goo.gle/hpc-toolkit-validation")
		log.Println("")
		log.Println("Validators can be silenced or treated as warnings or errors:")
		log.Println("")
		log.Println("- https://goo.gle/hpc-toolkit-validation-levels")
		log.Println("")
	}

	if warned {
		log.Println(validationWarningMsg)
		log.Println("")
	}

	if errored {
		return fmt.Errorf(validationErrorMsg)
	}
	return nil
}

// validateVars checks the global variables for viable types
func (dc DeploymentConfig) validateVars() error {
	vars := dc.Config.Vars
	nilErr := "deployment variable %s was not set"

	// Check type of labels (if they are defined)
	if labels, ok := vars["labels"]; ok {
		if _, ok := labels.(map[string]interface{}); !ok {
			return errors.New("vars.labels must be a map")
		}
	}

	// Check for any nil values
	for key, val := range vars {
		if val == nil {
			return fmt.Errorf(nilErr, key)
		}
	}

	return nil
}

func module2String(c Module) string {
	cBytes, _ := yaml.Marshal(&c)
	return string(cBytes)
}

func validateModule(c Module) error {
	if c.ID == "" {
		return fmt.Errorf("%s\n%s", errorMessages["emptyID"], module2String(c))
	}
	if c.Source == "" {
		return fmt.Errorf("%s\n%s", errorMessages["emptySource"], module2String(c))
	}
	if !modulereader.IsValidKind(c.Kind) {
		return fmt.Errorf("%s\n%s", errorMessages["wrongKind"], module2String(c))
	}
	return nil
}

func hasIllegalChars(name string) bool {
	return !regexp.MustCompile(`^[\w\+]+(\s*)[\w-\+\.]+$`).MatchString(name)
}

func validateOutputs(mod Module, modInfo modulereader.ModuleInfo) error {

	// Only get the map if needed
	var outputsMap map[string]modulereader.OutputInfo
	if len(mod.Outputs) > 0 {
		outputsMap = modInfo.GetOutputsAsMap()
	}

	// Ensure output exists in the underlying modules
	for _, output := range mod.Outputs {
		if _, ok := outputsMap[output.Name]; !ok {
			return fmt.Errorf("%s, module: %s output: %s",
				errorMessages["invalidOutput"], mod.ID, output.Name)
		}
	}
	return nil
}

// validateModules ensures parameters set in modules are set correctly.
func (dc DeploymentConfig) validateModules() error {
	for _, grp := range dc.Config.DeploymentGroups {
		for _, mod := range grp.Modules {
			if err := validateModule(mod); err != nil {
				return err
			}
			modInfo := dc.ModulesInfo[grp.Name][mod.Source]
			if err := validateOutputs(mod, modInfo); err != nil {
				return err
			}
		}
	}
	return nil
}

type moduleVariables struct {
	Inputs  map[string]bool
	Outputs map[string]bool
}

func validateSettings(
	mod Module,
	info modulereader.ModuleInfo) error {

	var cVars = moduleVariables{
		Inputs:  map[string]bool{},
		Outputs: map[string]bool{},
	}

	for _, input := range info.Inputs {
		cVars.Inputs[input.Name] = input.Required
	}

	for k := range mod.Settings {
		errData := fmt.Sprintf("Module ID: %s Setting: %s", mod.ID, k)
		// Setting name included a period
		// The user was likely trying to set a subfield which is not supported.
		// HCL does not support periods in variables names either:
		// https://hcl.readthedocs.io/en/latest/language_design.html#language-keywords-and-identifiers
		if strings.Contains(k, ".") {
			return &InvalidSettingError{
				fmt.Sprintf("%s\n%s", errorMessages["settingWithPeriod"], errData),
			}
		}
		// Setting includes invalid characters
		if !regexp.MustCompile(`^[a-zA-Z-_][a-zA-Z0-9-_]*$`).MatchString(k) {
			return &InvalidSettingError{
				fmt.Sprintf("%s\n%s", errorMessages["settingInvalidChar"], errData),
			}
		}
		// Module not found
		if _, ok := cVars.Inputs[k]; !ok {
			return &InvalidSettingError{
				fmt.Sprintf("%s\n%s", errorMessages["extraSetting"], errData),
			}
		}

	}
	return nil
}

// validateModuleSettings verifies that no additional settings are provided
// that don't have a counterpart variable in the module
func (dc DeploymentConfig) validateModuleSettings() error {
	for _, grp := range dc.Config.DeploymentGroups {
		for _, mod := range grp.Modules {
			info, err := modulereader.GetModuleInfo(mod.Source, mod.Kind)
			if err != nil {
				errStr := "failed to get info for module at %s while validating module settings"
				return errors.Wrapf(err, errStr, mod.Source)
			}
			if err = validateSettings(mod, info); err != nil {
				errStr := "found an issue while validating settings for module at %s"
				return errors.Wrapf(err, errStr, mod.Source)
			}
		}
	}
	return nil
}

func (dc *DeploymentConfig) getValidators() map[string]func(validatorConfig) error {
	allValidators := map[string]func(validatorConfig) error{
		testApisEnabledName.String():               dc.testApisEnabled,
		testProjectExistsName.String():             dc.testProjectExists,
		testRegionExistsName.String():              dc.testRegionExists,
		testZoneExistsName.String():                dc.testZoneExists,
		testZoneInRegionName.String():              dc.testZoneInRegion,
		testModuleNotUsedName.String():             dc.testModuleNotUsed,
		testDeploymentVariableNotUsedName.String(): dc.testDeploymentVariableNotUsed,
	}
	return allValidators
}

// The expected use case of this function is to merge blueprint requirements
// that are maps from project_id to string slices containing APIs or IAM roles
// required for provisioning. It will remove duplicate elements and ensure that
// the output is sorted for easy visual and automatic comparison.
// Solution: merge []string of new[key] into []string of base[key], removing
// duplicate elements and sorting the result
func mergeBlueprintRequirements(base map[string][]string, new map[string][]string) map[string][]string {
	dest := make(map[string][]string)
	maps.Copy(dest, base)

	// sort each value in dest in-place to ensure output is sorted when new map
	// does not contain all keys in base
	for _, v := range dest {
		slices.Sort(v)
	}

	for newProject, newRequirements := range new {
		// this code is safe even if dest[newProject] has not yet been populated
		dest[newProject] = append(dest[newProject], newRequirements...)
		slices.Sort(dest[newProject])
		dest[newProject] = slices.Compact(dest[newProject])
	}
	return dest
}

func (dc *DeploymentConfig) testApisEnabled(c validatorConfig) error {
	if err := c.check(testApisEnabledName, []string{}); err != nil {
		return err
	}

	requiredApis := make(map[string][]string)
	for _, grp := range dc.Config.DeploymentGroups {
		for _, mod := range grp.Modules {
			requiredApis = mergeBlueprintRequirements(requiredApis, mod.RequiredApis)
		}
	}

	var errored bool
	for pid, apis := range requiredApis {
		var project string
		if IsLiteralVariable(pid) {
			project, _ = dc.getStringValue(pid)
		} else {
			project = pid
		}
		err := validators.TestApisEnabled(project, apis)
		if err != nil {
			log.Println(err)
			errored = true
		}
	}

	if errored {
		return fmt.Errorf(funcErrorMsgTemplate, testApisEnabledName.String())
	}
	return nil
}

func (dc *DeploymentConfig) testProjectExists(c validatorConfig) error {
	funcName := testProjectExistsName.String()
	funcErrorMsg := fmt.Sprintf(funcErrorMsgTemplate, funcName)

	if err := c.check(testProjectExistsName, []string{"project_id"}); err != nil {
		return err
	}

	projectID, err := dc.getStringValue(c.Inputs["project_id"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	// err is nil or an error
	err = validators.TestProjectExists(projectID)
	if err != nil {
		log.Print(err)
		return fmt.Errorf(funcErrorMsg)
	}
	return nil
}

func (dc *DeploymentConfig) testRegionExists(c validatorConfig) error {
	funcName := testRegionExistsName.String()
	funcErrorMsg := fmt.Sprintf(funcErrorMsgTemplate, funcName)

	if err := c.check(testRegionExistsName, []string{"project_id", "region"}); err != nil {
		return err
	}
	projectID, err := dc.getStringValue(c.Inputs["project_id"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}
	region, err := dc.getStringValue(c.Inputs["region"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	// err is nil or an error
	err = validators.TestRegionExists(projectID, region)
	if err != nil {
		log.Print(err)
		return fmt.Errorf(funcErrorMsg)
	}
	return nil
}

func (dc *DeploymentConfig) testZoneExists(c validatorConfig) error {
	funcName := testZoneExistsName.String()
	funcErrorMsg := fmt.Sprintf(funcErrorMsgTemplate, funcName)

	if err := c.check(testZoneExistsName, []string{"project_id", "zone"}); err != nil {
		return err
	}

	projectID, err := dc.getStringValue(c.Inputs["project_id"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}
	zone, err := dc.getStringValue(c.Inputs["zone"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	// err is nil or an error
	err = validators.TestZoneExists(projectID, zone)
	if err != nil {
		log.Print(err)
		return fmt.Errorf(funcErrorMsg)
	}
	return nil
}

func (dc *DeploymentConfig) testZoneInRegion(c validatorConfig) error {
	funcName := testZoneInRegionName.String()
	funcErrorMsg := fmt.Sprintf(funcErrorMsgTemplate, funcName)

	if err := c.check(testZoneInRegionName, []string{"project_id", "region", "zone"}); err != nil {
		return err
	}

	projectID, err := dc.getStringValue(c.Inputs["project_id"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}
	zone, err := dc.getStringValue(c.Inputs["zone"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}
	region, err := dc.getStringValue(c.Inputs["region"])
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	// err is nil or an error
	err = validators.TestZoneInRegion(projectID, zone, region)
	if err != nil {
		log.Print(err)
		return fmt.Errorf(funcErrorMsg)
	}
	return nil
}

func (dc *DeploymentConfig) testModuleNotUsed(c validatorConfig) error {
	if err := c.check(testModuleNotUsedName, []string{}); err != nil {
		return err
	}

	if err := validators.TestModuleNotUsed(dc.listUnusedModules()); err != nil {
		log.Print(err)
		return fmt.Errorf(funcErrorMsgTemplate, testModuleNotUsedName.String())
	}
	return nil
}

func (dc *DeploymentConfig) testDeploymentVariableNotUsed(c validatorConfig) error {
	if err := c.check(testDeploymentVariableNotUsedName, []string{}); err != nil {
		return err
	}

	if err := validators.TestDeploymentVariablesNotUsed(dc.listUnusedDeploymentVariables()); err != nil {
		log.Print(err)
		return fmt.Errorf(funcErrorMsgTemplate, testDeploymentVariableNotUsedName.String())
	}
	return nil
}

// return the actual value of a global variable specified by the literal
// variable inputReference in form ((var.project_id))
// if it is a literal global variable defined as a string, return value as string
// in all other cases, return empty string and error
func (dc *DeploymentConfig) getStringValue(inputReference interface{}) (string, error) {
	varRef, ok := inputReference.(string)
	if !ok {
		return "", fmt.Errorf("the value %s cannot be cast to a string", inputReference)
	}

	if IsLiteralVariable(varRef) {
		varSlice := strings.Split(HandleLiteralVariable(varRef), ".")
		varSrc := varSlice[0]
		varName := varSlice[1]

		// because expand has already run, the global variable should have been
		// checked for existence. handle if user has explicitly passed
		// ((var.does_not_exit)) or ((not_a_varsrc.not_a_var))
		if varSrc == "var" {
			if val, ok := dc.Config.Vars[varName]; ok {
				valString, ok := val.(string)
				if ok {
					return valString, nil
				}
				return "", fmt.Errorf("the deployment variable %s is not a string", inputReference)
			}
		}
	}
	return "", fmt.Errorf("the value %s is not a deployment variable or was not defined", inputReference)
}
