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
	"regexp"
	"strings"

	"hpc-toolkit/pkg/modulereader"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
)

const maxLabels = 64

func validateGlobalLabels(vars Dict) error {
	if !vars.Has("labels") {
		return nil
	}
	p := Root.Vars.Dot("labels")
	labels := vars.Get("labels")
	ty := labels.Type()

	if !ty.IsObjectType() && !ty.IsMapType() {
		return BpError{
			p, errors.New("vars.labels must be a map of strings")} // skip further validation
	}
	errs := Errors{}
	if labels.LengthInt() > maxLabels {
		// GCP resources cannot have more than 64 labels, so enforce this upper bound here
		// to do some early validation. Modules may add more labels, leading to potential
		// deployment failures.
		errs.At(p, errors.New("vars.labels cannot have more than 64 labels"))
	}
	for k, v := range labels.AsValueMap() {
		vp := p.Cty(cty.Path{}.IndexString(k))
		if v.Type() != cty.String {
			errs.At(vp, errors.New("vars.labels must be a map of strings"))
			continue
		}
		s := v.AsString()

		// Check that label names are valid
		if !isValidLabelName(k) {
			errs.At(vp, errors.Errorf("%s: '%s: %s'", errMsgLabelNameReqs, k, s))
		}
		// Check that label values are valid
		if !isValidLabelValue(s) {
			errs.At(vp, errors.Errorf("%s: '%s: %s'", errMsgLabelValueReqs, k, s))
		}
	}
	return errs.OrNil()
}

// validateVars checks the global variables for viable types
func validateVars(vars Dict) error {
	errs := Errors{}
	errs.Add(validateGlobalLabels(vars))
	// Check for any nil values
	for key, val := range vars.Items() {
		if val.IsNull() {
			errs.At(Root.Vars.Dot(key), fmt.Errorf("deployment variable %s was not set", key))
		}
	}
	return errs.OrNil()
}

func validateModule(p modulePath, m Module, bp Blueprint) error {
	// Source/Kind validations are required to pass to perform other validations
	if m.Source == "" {
		return BpError{p.Source, EmptyModuleSource}
	}
	if err := checkMovedModule(m.Source); err != nil {
		return BpError{p.Source, err}
	}
	if !IsValidModuleKind(m.Kind.String()) {
		return BpError{p.Kind, InvalidModuleKind}
	}
	info, err := modulereader.GetModuleInfo(m.Source, m.Kind.kind)
	if err != nil {
		return BpError{p.Source, err}
	}

	errs := Errors{}
	if m.ID == "" {
		errs.At(p.ID, EmptyModuleID)
	}
	if m.ID == "vars" { // invalid module ID
		errs.At(p.ID, errors.New("module id cannot be 'vars'"))
	}
	return errs.
		Add(validateSettings(p, m, info)).
		Add(validateOutputs(p, m, info)).
		Add(validateModuleUseReferences(p, m, bp)).
		Add(validateModuleSettingReferences(p, m, bp)).
		OrNil()
}

func validateOutputs(p modulePath, mod Module, info modulereader.ModuleInfo) error {
	errs := Errors{}
	outputs := info.GetOutputsAsMap()

	// Ensure output exists in the underlying modules
	for io, output := range mod.Outputs {
		if _, ok := outputs[output.Name]; !ok {
			err := fmt.Errorf("%s, module: %s output: %s", errMsgInvalidOutput, mod.ID, output.Name)
			errs.At(p.Outputs.At(io), err)
		}
	}
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
	errs := Errors{}
	for k := range mod.Settings.Items() {
		sp := p.Settings.Dot(k)
		// Setting name included a period
		// The user was likely trying to set a subfield which is not supported.
		// HCL does not support periods in variables names either:
		// https://hcl.readthedocs.io/en/latest/language_design.html#language-keywords-and-identifiers
		if strings.Contains(k, ".") {
			errs.At(sp, ModuleSettingWithPeriod)
			continue // do not perform other validations
		}
		// Setting includes invalid characters
		if !regexp.MustCompile(`^[a-zA-Z-_][a-zA-Z0-9-_]*$`).MatchString(k) {
			errs.At(sp, ModuleSettingInvalidChar)
			continue // do not perform other validations
		}
		// Setting not found
		if _, ok := cVars.Inputs[k]; !ok {
			errs.At(sp, UnknownModuleSetting)
			continue // do not perform other validations
		}

	}
	return errs.OrNil()
}
