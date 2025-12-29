// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package validators

import (
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestRegexValidator(t *testing.T) {
	// Base fake blueprint/module used by subtests
	baseBP := config.Blueprint{
		BlueprintName: "test-bp",
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "test-module",
						Source: "test/module",
						Settings: config.NewDict(map[string]cty.Value{
							"name": cty.StringVal("Invalid-Name"),
						}),
					},
				},
			},
		},
	}

	validator := RegexValidator{}

	t.Run("fails_on_invalid_name", func(t *testing.T) {
		// rule targeting module.setting "name" via vars:
		rule := modulereader.ValidationRule{
			Validator:    "regex",
			ErrorMessage: "'name' must be lowercase and start with a letter.",
			Inputs: map[string]interface{}{
				"vars":    []interface{}{"name"},
				"pattern": "^[a-z]([-a-z0-9]*[a-z0-9])?$",
			},
		}

		err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "must be lowercase") {
			t.Fatalf("expected error message to contain 'must be lowercase', got: %q", err.Error())
		}
	})

	t.Run("passes_on_valid_name", func(t *testing.T) {
		// valid module.setting value
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"name": cty.StringVal("validname"),
		})

		rule := modulereader.ValidationRule{
			Validator:    "regex",
			ErrorMessage: "'name' must be lowercase and start with a letter.",
			Inputs: map[string]interface{}{
				"vars":    []interface{}{"name"},
				"pattern": "^[a-z]([-a-z0-9]*[a-z0-9])?$",
			},
		}

		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("module_setting_equal_to_blueprint_var_is_validated", func(t *testing.T) {
		// when module.setting equals blueprint var, module-scoped validators should still validate it
		bp := baseBP
		// set blueprint var and module setting to the same INVALID value
		bp.Vars = config.NewDict(map[string]cty.Value{
			"name": cty.StringVal("Invalid-Name"),
		})
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"name": cty.StringVal("Invalid-Name"),
		})

		// module-scoped rule (uses vars: to indicate module.setting names)
		rule := modulereader.ValidationRule{
			Validator:    "regex",
			ErrorMessage: "'name' must be lowercase and start with a letter.",
			Inputs: map[string]interface{}{
				"vars":    []interface{}{"name"},
				"pattern": "^[a-z]([-a-z0-9]*[a-z0-9])?$",
			},
		}

		// Expect an error because the module setting (even though equal to bp var) is validated.
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err == nil {
			t.Fatalf("expected validation error when module.setting equals blueprint var, got nil")
		}
	})

	t.Run("list_values_are_validated_elementwise", func(t *testing.T) {
		// module.setting is a list; validator should validate each element
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"tags": cty.ListVal([]cty.Value{
				cty.StringVal("good"),
				cty.StringVal("BadTag"),
			}),
		})

		rule := modulereader.ValidationRule{
			Validator:    "regex",
			ErrorMessage: "'tags' must be lowercase-only.",
			Inputs: map[string]interface{}{
				"vars":    []interface{}{"tags"},
				"pattern": "^[a-z]+$",
			},
		}

		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error for list containing invalid element, got nil")
		}
		if !strings.Contains(err.Error(), "must be lowercase-only") {
			t.Fatalf("unexpected error message: %q", err.Error())
		}
	})
}
