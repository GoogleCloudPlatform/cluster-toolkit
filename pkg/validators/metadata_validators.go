// Copyright 2025 Google LLC
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

package validators

import (
	"fmt"
	"regexp"
	"strings"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"

	"github.com/zclconf/go-cty/cty"
)

// RegexValidator implements the Validator interface for 'regex' type.
type RegexValidator struct{}

// getNestedValue retrieves a cty.Value from a Dict using a dot-separated path.
func getNestedValue(d config.Dict, path string) (cty.Value, bool) {
	parts := strings.Split(path, ".")
	currentVal := d.AsObject()

	for i, part := range parts {
		if !currentVal.Type().IsObjectType() && !currentVal.Type().IsMapType() {
			return cty.NilVal, false
		}

		if !currentVal.Type().HasAttribute(part) {
			return cty.NilVal, false
		}

		val := currentVal.GetAttr(part)
		if i == len(parts)-1 {
			return val, true
		}
		currentVal = val
	}
	return cty.NilVal, false // Should not be reached
}

// getValuesToValidate retrieves the values for a given variable name from the module settings or blueprint variables.
// It handles nested variables and lists of strings.
func getValuesToValidate(bp config.Blueprint, mod config.Module, varName string) ([]cty.Value, config.Path, error) {
	var val cty.Value
	var found bool
	var path config.Path

	if val, found = getNestedValue(mod.Settings, varName); found {
		path = config.Root.Groups.At(bp.GroupIndex(bp.ModuleGroupOrDie(mod.ID).Name)).Modules.At(bp.ModuleGroupOrDie(mod.ID).ModuleIndex(mod.ID)).Settings.Dot(varName)
	} else if bp.Vars.Has(varName) {
		val = bp.Vars.Get(varName)
		path = config.Root.Vars.Dot(varName)
		found = true
	}

	if !found {
		return nil, path, fmt.Errorf("variable %q not found in module %q settings or blueprint vars", varName, mod.ID)
	}

	// Try to evaluate the value if it's an expression
	evaledVal, err := bp.Eval(val)
	if err == nil {
		val = evaledVal
	}

	var values []cty.Value
	if val.Type().IsListType() || val.Type().IsTupleType() {
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			values = append(values, v)
		}
	} else {
		values = append(values, val)
	}
	return values, path, nil
}

// Validate checks if the variables specified in the rule match the provided regex pattern.
func (v *RegexValidator) Validate(
	bp config.Blueprint,
	mod config.Module,
	rule modulereader.ValidationRule,
	group config.Group,
	modIdx int) error {

	// Extract pattern from inputs
	pattern, ok := rule.Inputs["pattern"].(string)
	if !ok {
		return config.BpError{
			Err: fmt.Errorf(
				"validation rule for module %q has a non-string pattern", mod.ID),
			Path: config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source,
		}
	}

	// Extract vars from inputs
	varsInterface, ok := rule.Inputs["vars"].([]interface{})
	if !ok {
		return config.BpError{
			Err: fmt.Errorf(
				"validation rule for module %q has a malformed 'vars' field", mod.ID),
			Path: config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source,
		}
	}

	var vars []string
	for _, v := range varsInterface {
		s, ok := v.(string)
		if !ok {
			return config.BpError{
				Err: fmt.Errorf(
					"validation rule for module %q has a non-string value in 'vars'", mod.ID),
				Path: config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source,
			}
		}
		vars = append(vars, s)
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return config.BpError{
			Err: fmt.Errorf(
				"failed to compile regex for module %q", mod.ID),
			Path: config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source,
		}
	}

	for _, varName := range vars {
		values, path, err := getValuesToValidate(bp, mod, varName)
		if err != nil {
			continue
		}

		for _, val := range values {
			if val.Type() != cty.String {
				continue
			}
			strVal := val.AsString()

			if !re.MatchString(strVal) {
				return config.BpError{
					Err:  fmt.Errorf("%s", rule.ErrorMessage),
					Path: path,
				}
			}
		}
	}
	return nil
}
