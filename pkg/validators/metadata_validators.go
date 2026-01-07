// Copyright 2025 Google LLC
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
