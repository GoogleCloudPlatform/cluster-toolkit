// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License and limitations under the License.

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

// Validate checks if the variables specified in the rule match the provided regex pattern.
// This function focuses on the predicate and uses IterateRuleTargets from targets.go to resolve targets.
func (r *RegexValidator) Validate(
	bp config.Blueprint,
	mod config.Module,
	rule modulereader.ValidationRule,
	group config.Group,
	modIdx int) error {

	// Extract pattern
	patternRaw, ok := rule.Inputs["pattern"].(string)
	if !ok || patternRaw == "" {
		return config.BpError{
			Err: fmt.Errorf(
				"validation rule for module %q is missing a string 'pattern' in inputs", mod.ID),
			Path: config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source,
		}
	}

	// compile regex
	re, err := regexp.Compile(patternRaw)
	if err != nil {
		return config.BpError{
			Err:  fmt.Errorf("failed to compile regex for module %q: %v", mod.ID, err),
			Path: config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source,
		}
	}

	// helper: validate flattened cty.Values against regex, returning first error
	validateValues := func(values []cty.Value, path config.Path) error {
		for _, val := range values {
			if val.Type() != cty.String {
				continue
			}
			if !re.MatchString(val.AsString()) {
				msg := rule.ErrorMessage
				if msg == "" {
					msg = fmt.Sprintf("value %q does not match pattern %q", val.AsString(), patternRaw)
				}
				return config.BpError{Err: fmt.Errorf("%s", msg), Path: path}
			}
		}
		return nil
	}

	// iterate targets using shared logic
	err = IterateRuleTargets(bp, mod, rule, group, modIdx, func(t Target) error {
		return validateValues(t.Values, t.Path)
	})
	return err
}

type AllowedEnumValidator struct{}

// normalizeAllowed converts the 'allowed' input (either []string or []interface{}) into a standard string slice.
func (v *AllowedEnumValidator) normalizeAllowed(allowedRaw interface{}) ([]string, error) {
	var allowedList []string
	switch t := allowedRaw.(type) {
	case []string:
		allowedList = t
	case []interface{}:
		for _, e := range t {
			allowedList = append(allowedList, fmt.Sprintf("%v", e))
		}
	default:
		return nil, fmt.Errorf("'allowed' must be a list of strings")
	}
	if len(allowedList) == 0 {
		return nil, fmt.Errorf("'allowed' list must be non-empty")
	}
	return allowedList, nil
}

// checkValues iterates through cty.Values to ensure they exist within the allowed set, handling nulls and casing.
func (v *AllowedEnumValidator) checkValues(values []cty.Value, path config.Path, allowedSet map[string]struct{}, allowedList []string, caseSensitive bool, allowNull bool, errMsg string) error {
	for _, val := range values {
		if val.IsNull() {
			if allowNull {
				continue
			}
			msg := errMsg
			if msg == "" {
				msg = fmt.Sprintf("null value is not allowed; allowed values: %v", allowedList)
			}
			return config.BpError{Err: fmt.Errorf("%s", msg), Path: path}
		}

		if val.Type() != cty.String {
			continue
		}

		str := val.AsString()
		key := str
		if !caseSensitive {
			key = strings.ToLower(str)
		}

		if _, ok := allowedSet[key]; !ok {
			msg := errMsg
			if msg == "" {
				msg = fmt.Sprintf("invalid value %q; allowed values: %v", str, allowedList)
			}
			return config.BpError{Err: fmt.Errorf("%s", msg), Path: path}
		}
	}
	return nil
}

// Ensures that user-provided module settings conform to a predefined list of allowed values (enums).
func (v *AllowedEnumValidator) Validate(
	bp config.Blueprint,
	mod config.Module,
	rule modulereader.ValidationRule,
	group config.Group,
	modIdx int) error {

	modPath := config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source

	// 1. Parse Metadata Inputs (flags)
	caseSensitive, err := parseBoolInput(rule.Inputs, "case_sensitive", true)
	if err != nil {
		return config.BpError{
			Err:  fmt.Errorf("validation rule for module %q: %v", mod.ID, err),
			Path: modPath,
		}
	}

	allowNull, err := parseBoolInput(rule.Inputs, "allow_null", false)
	if err != nil {
		return config.BpError{
			Err:  fmt.Errorf("validation rule for module %q: %v", mod.ID, err),
			Path: modPath,
		}
	}

	// 2. Normalize the 'allowed' list
	allowedRaw, ok := rule.Inputs["allowed"]
	if !ok {
		return config.BpError{
			Err:  fmt.Errorf("validation rule for module %q is missing an 'allowed' list", mod.ID),
			Path: modPath,
		}
	}

	allowedList, err := v.normalizeAllowed(allowedRaw)
	if err != nil {
		return config.BpError{
			Err:  fmt.Errorf("validation rule for module %q: %v", mod.ID, err),
			Path: modPath,
		}
	}

	// 3. Build the lookup set
	allowedSet := make(map[string]struct{}, len(allowedList))
	for _, s := range allowedList {
		key := s
		if !caseSensitive {
			key = strings.ToLower(s)
		}
		allowedSet[key] = struct{}{}
	}

	// 4. Iterate and validate user-provided values
	return IterateRuleTargets(bp, mod, rule, group, modIdx, func(t Target) error {
		return v.checkValues(t.Values, t.Path, allowedSet, allowedList, caseSensitive, allowNull, rule.ErrorMessage)
	})
}

// RangeValidator implements the RuleValidator interface for the 'range' validation type.
type RangeValidator struct{}

// checkBounds validates a single integer value against the optional minimum and maximum bounds.
func (r *RangeValidator) checkBounds(value int, min *int, max *int, customErrMsg string, path config.Path) error {
	if min != nil && value < *min {
		msg := customErrMsg
		if msg == "" {
			msg = fmt.Sprintf("value %d is less than the minimum allowed value of %d", value, *min)
		}
		return config.BpError{Err: fmt.Errorf("%s", msg), Path: path}
	}
	if max != nil && value > *max {
		msg := customErrMsg
		if msg == "" {
			msg = fmt.Sprintf("value %d is greater than the maximum allowed value of %d", value, *max)
		}
		return config.BpError{Err: fmt.Errorf("%s", msg), Path: path}
	}
	return nil
}

// validateTarget applies range validation to a list of cty.Values.
func (r *RangeValidator) validateTarget(
	values []cty.Value,
	path config.Path,
	min *int,
	max *int,
	lengthCheck bool,
	customErrMsg string) error {
	if lengthCheck {
		return r.checkBounds(len(values), min, max, customErrMsg, path)
	}

	for _, val := range values {
		if val.IsNull() || !val.IsKnown() {
			continue
		}
		if val.Type() == cty.Number {
			f, _ := val.AsBigFloat().Float64()
			if f != float64(int64(f)) {
				return config.BpError{
					Err:  fmt.Errorf("range validator only supports integer numbers, not %v", f),
					Path: path,
				}
			}
			if err := r.checkBounds(int(f), min, max, customErrMsg, path); err != nil {
				return err
			}
		} else {
			return config.BpError{
				Err:  fmt.Errorf("range validator only supports numbers, not %s", val.Type().FriendlyName()),
				Path: path,
			}
		}
	}
	return nil
}

// Validate checks if the variables specified in the rule fall within the specified numeric range or length constraints.
func (r *RangeValidator) Validate(
	bp config.Blueprint,
	mod config.Module,
	rule modulereader.ValidationRule,
	group config.Group,
	modIdx int) error {

	modPath := config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source

	min, err := parseIntInput(rule.Inputs, "min")
	if err != nil {
		return config.BpError{Err: fmt.Errorf("validation rule for module %q: %v", mod.ID, err), Path: modPath}
	}

	max, err := parseIntInput(rule.Inputs, "max")
	if err != nil {
		return config.BpError{Err: fmt.Errorf("validation rule for module %q: %v", mod.ID, err), Path: modPath}
	}

	if min == nil && max == nil {
		return config.BpError{
			Err:  fmt.Errorf("range validator for module %q must have at least one of 'min' or 'max' defined", mod.ID),
			Path: modPath,
		}
	}

	if min != nil && max != nil && *max < *min {
		return config.BpError{
			Err:  fmt.Errorf("range validator for module %q must have 'min' less than or equal to 'max' defined", mod.ID),
			Path: modPath,
		}
	}

	checkListLength, err := parseBoolInput(rule.Inputs, "length_check", false)
	if err != nil {
		return config.BpError{Err: fmt.Errorf("validation rule for module %q: %v", mod.ID, err), Path: modPath}
	}

	return IterateRuleTargets(bp, mod, rule, group, modIdx, func(t Target) error {
		return r.validateTarget(t.Values, t.Path, min, max, checkListLength, rule.ErrorMessage)
	})
}

// ExclusiveValidator implements the RuleValidator interface for the 'exclusive' validation type.
type ExclusiveValidator struct{}

// isVarSet returns true if the value is known, non-null, and non-empty (positive number, non-empty string, true bool, or non-empty collection).
func (e *ExclusiveValidator) isVarSet(val cty.Value) bool {
	if val.IsNull() || !val.IsKnown() {
		return false
	}
	switch val.Type() {
	case cty.String:
		return val.AsString() != ""
	case cty.Number:
		return val.AsBigFloat().Sign() != 0
	case cty.Bool:
		return val.True()
	default:
		// For lists, maps, and sets, consider them "set" if they are not empty.
		return val.LengthInt() > 0
	}
}

// Validate returns an error if more than one of the variables specified in the rule are "set" within the module configuration.
func (e *ExclusiveValidator) Validate(
	bp config.Blueprint,
	mod config.Module,
	rule modulereader.ValidationRule,
	group config.Group,
	modIdx int) error {
	var setVarNames []string
	handler := func(t Target) error {
		for _, val := range t.Values {
			if e.isVarSet(val) {
				setVarNames = append(setVarNames, t.Name)
			}
		}
		return nil
	}
	if err := IterateRuleTargets(bp, mod, rule, group, modIdx, handler); err != nil {
		return err
	}
	if len(setVarNames) > 1 {
		modPath := config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Source
		return config.BpError{Err: fmt.Errorf("%s: the following are set: %s", rule.ErrorMessage, strings.Join(setVarNames, ", ")), Path: modPath}
	}
	return nil
}
