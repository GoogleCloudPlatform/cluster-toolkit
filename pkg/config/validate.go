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
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
)

const (
	validationWarningMsg = "Validation failures were treated as a warning, continuing to create blueprint."
	validationErrorMsg   = "validation failed due to the issues listed above"
	funcErrorMsgTemplate = "validator %s failed"
	maxLabels            = 64
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
func (dc DeploymentConfig) validate() error {
	// variables should be validated before running validators
	if err := dc.executeValidators(); err != nil {
		return err
	}
	if err := dc.validateModules(); err != nil {
		return err
	}
	if err := dc.validateModuleSettings(); err != nil {
		return err
	}
	return nil
}

// performs validation of global variables
func (dc DeploymentConfig) executeValidators() error {
	var errored, warned bool
	implementedValidators := dc.getValidators()

	if dc.Config.ValidationLevel == ValidationIgnore {
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
			case ValidationWarning:
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
	if vars.Has("labels") {
		labels := vars.Get("labels")
		ty := labels.Type()
		if !ty.IsObjectType() && !ty.IsMapType() {
			return errors.New("vars.labels must be a map of strings")
		}
		if labels.LengthInt() > maxLabels {
			// GCP resources cannot have more than 64 labels, so enforce this upper bound here
			// to do some early validation. Modules may add more labels, leading to potential
			// deployment failures.
			return errors.New("vars.labels cannot have more than 64 labels")
		}
		for labelName, v := range labels.AsValueMap() {
			if v.Type() != cty.String {
				return errors.New("vars.labels must be a map of strings")
			}
			labelValue := v.AsString()

			// Check that label names are valid
			if !isValidLabelName(labelName) {
				return errors.Errorf("%s: '%s: %s'",
					errorMessages["labelNameReqs"], labelName, labelValue)
			}
			// Check that label values are valid
			if !isValidLabelValue(labelValue) {
				return errors.Errorf("%s: '%s: %s'",
					errorMessages["labelValueReqs"], labelName, labelValue)
			}
		}
	}

	// Check for any nil values
	for key, val := range vars.Items() {
		if val.IsNull() {
			return fmt.Errorf(nilErr, key)
		}
	}

	err := cty.Walk(vars.AsObject(), func(p cty.Path, v cty.Value) (bool, error) {
		if e, is := IsExpressionValue(v); is {
			return false, fmt.Errorf("can not use expressions in vars block, got %#v", e.makeYamlExpressionValue().AsString())
		}
		return true, nil
	})
	if err != nil {
		return err
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
	if !IsValidModuleKind(c.Kind.String()) {
		return fmt.Errorf("%s\n%s", errorMessages["wrongKind"], module2String(c))
	}
	return nil
}

func validateOutputs(mod Module) error {
	modInfo := mod.InfoOrDie()
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
	return dc.Config.WalkModules(func(m *Module) error {
		if err := validateModule(*m); err != nil {
			return err
		}
		return validateOutputs(*m)
	})
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

	for k := range mod.Settings.Items() {
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
			info, err := modulereader.GetModuleInfo(mod.Source, mod.Kind.String())
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

func (dc *DeploymentConfig) testApisEnabled(c validatorConfig) error {
	if err := c.check(testApisEnabledName, []string{}); err != nil {
		return err
	}

	pv := dc.Config.Vars.Get("project_id")
	if pv.Type() != cty.String {
		return fmt.Errorf("the deployment variable `project_id` is either not set or is not a string")
	}

	apis := map[string]bool{}
	dc.Config.WalkModules(func(m *Module) error {
		for _, api := range m.InfoOrDie().RequiredApis {
			apis[api] = true
		}
		return nil
	})

	if err := validators.TestApisEnabled(pv.AsString(), maps.Keys(apis)); err != nil {
		log.Println(err)
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
	m, err := evalValidatorInputsAsStrings(c.Inputs, dc.Config)
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	if err = validators.TestProjectExists(m["project_id"]); err != nil {
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
	m, err := evalValidatorInputsAsStrings(c.Inputs, dc.Config)
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	if err = validators.TestRegionExists(m["project_id"], m["region"]); err != nil {
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
	m, err := evalValidatorInputsAsStrings(c.Inputs, dc.Config)
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	if err = validators.TestZoneExists(m["project_id"], m["zone"]); err != nil {
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
	m, err := evalValidatorInputsAsStrings(c.Inputs, dc.Config)
	if err != nil {
		log.Print(funcErrorMsg)
		return err
	}

	if err = validators.TestZoneInRegion(m["project_id"], m["zone"], m["region"]); err != nil {
		log.Print(err)
		return fmt.Errorf(funcErrorMsg)
	}
	return nil
}

func (dc *DeploymentConfig) testModuleNotUsed(c validatorConfig) error {
	if err := c.check(testModuleNotUsedName, []string{}); err != nil {
		return err
	}

	acc := map[string][]string{}
	dc.Config.WalkModules(func(m *Module) error {
		ids := m.listUnusedModules()
		sids := make([]string, len(ids))
		for i, id := range ids {
			sids[i] = string(id)
		}
		acc[string(m.ID)] = sids
		return nil
	})

	if err := validators.TestModuleNotUsed(acc); err != nil {
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

// Helper function to evaluate validator inputs and make sure that all values are strings.
func evalValidatorInputsAsStrings(inputs Dict, bp Blueprint) (map[string]string, error) {
	ev, err := inputs.Eval(bp)
	if err != nil {
		return nil, err
	}
	ms := map[string]string{}
	for k, v := range ev.Items() {
		if v.Type() != cty.String {
			return nil, fmt.Errorf("validator inputs must be strings, %s is a %s", k, v.Type())
		}
		ms[k] = v.AsString()
	}
	return ms, nil
}
