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
)

const (
	validationWarningMsg = "Validation failures were treated as a warning, continuing to create blueprint."
	validationErrorMsg   = "validation failed due to the issues listed above"
	funcErrorMsgTemplate = "validator %s failed"
	maxLabels            = 64
)

// validate is the top-level function for running the validation suite.
func (dc DeploymentConfig) validate() error {
	errs := MultiError{}
	return errs.
		Add(dc.executeValidators()).
		Add(dc.validateModules()).OrNil()
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
	return nil
}

func validateModule(p modulePath, m Module) error {
	errs := MultiError{}
	if m.ID == "" {
		errs.Add(BpError{p.ID, fmt.Errorf(errorMessages["emptyID"])})
	}
	if m.Source == "" {
		errs.Add(BpError{p.Source, fmt.Errorf(errorMessages["emptySource"])})
	}
	if !IsValidModuleKind(m.Kind.String()) {
		errs.Add(BpError{p.Kind, fmt.Errorf(errorMessages["wrongKind"])})
	}

	info, err := modulereader.GetModuleInfo(m.Source, m.Kind.kind)
	if err != nil { // Can not proceed with other validations
		return errs.Add(err)
	}

	errs.Add(validateSettings(p, m, info))
	errs.Add(validateOutputs(p, m, info))
	return errs.OrNil()
}

func validateOutputs(p modulePath, mod Module, info modulereader.ModuleInfo) error {
	errs := MultiError{}
	outputs := info.GetOutputsAsMap()

	// Ensure output exists in the underlying modules
	for io, output := range mod.Outputs {
		if _, ok := outputs[output.Name]; !ok {
			err := fmt.Errorf("%s, module: %s output: %s", errorMessages["invalidOutput"], mod.ID, output.Name)
			errs.Add(BpError{p.Outputs.At(io), err})
		}
	}
	return errs.OrNil()
}

// validateModules ensures parameters set in modules are set correctly.
func (dc DeploymentConfig) validateModules() error {
	errs := MultiError{}
	dc.Config.WalkModules(func(p modulePath, m *Module) error {
		errs.Add(validateModule(p, *m))
		return nil
	})
	return errs.OrNil()
}

type moduleVariables struct {
	Inputs  map[string]bool
	Outputs map[string]bool
}

func validateSettings(
	p modulePath,
	mod Module,
	info modulereader.ModuleInfo) error {

	var cVars = moduleVariables{
		Inputs:  map[string]bool{},
		Outputs: map[string]bool{},
	}

	for _, input := range info.Inputs {
		cVars.Inputs[input.Name] = input.Required
	}
	errs := MultiError{}
	for k := range mod.Settings.Items() {
		sp := p.Settings.Dot(k)
		// Setting name included a period
		// The user was likely trying to set a subfield which is not supported.
		// HCL does not support periods in variables names either:
		// https://hcl.readthedocs.io/en/latest/language_design.html#language-keywords-and-identifiers
		if strings.Contains(k, ".") {
			errs.Add(BpError{sp, errors.New(errorMessages["settingWithPeriod"])})
			continue // do not perform other validations
		}
		// Setting includes invalid characters
		if !regexp.MustCompile(`^[a-zA-Z-_][a-zA-Z0-9-_]*$`).MatchString(k) {
			errs.Add(BpError{sp, errors.New(errorMessages["settingInvalidChar"])})
			continue // do not perform other validations
		}
		// Setting not found
		if _, ok := cVars.Inputs[k]; !ok {
			errs.Add(BpError{sp, errors.New(errorMessages["extraSetting"])})
			continue // do not perform other validations
		}

	}
	return errs.OrNil()
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
	dc.Config.WalkModules(func(_ modulePath, m *Module) error {
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
	dc.Config.WalkModules(func(_ modulePath, m *Module) error {
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
