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

func TestAllowedEnumValidator(t *testing.T) {
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
							"network_routing_mode": cty.StringVal("GLOBAL"),
						}),
					},
				},
			},
		},
	}

	validator := AllowedEnumValidator{}

	t.Run("validates_network_routing_mode_from_metadata_example", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"network_routing_mode": cty.StringVal("INVALID_MODE"),
		})

		rule := modulereader.ValidationRule{
			Validator:    "allowed_enum",
			ErrorMessage: "'network_routing_mode' must be GLOBAL or REGIONAL.",
			Inputs: map[string]interface{}{
				"vars":    []interface{}{"network_routing_mode"},
				"allowed": []interface{}{"GLOBAL", "REGIONAL"},
			},
		}

		// 1. Test failure with invalid value
		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatal("expected error for invalid routing mode, got nil")
		}
		if !strings.Contains(err.Error(), "'network_routing_mode' must be GLOBAL or REGIONAL.") {
			t.Fatalf("unexpected error message: %q", err.Error())
		}

		// 2. Test failure with lowercase value (default case_sensitive is true)
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"network_routing_mode": cty.StringVal("global"),
		})
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err == nil {
			t.Fatal("expected error for lowercase 'global' due to default case sensitivity, got nil")
		}

		// 3. Test success with correct uppercase value
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"network_routing_mode": cty.StringVal("GLOBAL"),
		})
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected error for valid routing mode 'GLOBAL': %v", err)
		}

		// 4. Test success with correct uppercase value 'REGIONAL'
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"network_routing_mode": cty.StringVal("REGIONAL"),
		})
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected error for valid routing mode 'REGIONAL': %v", err)
		}
	})

	t.Run("handles_explicit_case_insensitive_matching", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"tier": cty.StringVal("basic_ssd"),
		})

		rule := modulereader.ValidationRule{
			Validator: "allowed_enum",
			Inputs: map[string]interface{}{
				"vars":           []interface{}{"tier"},
				"allowed":        []interface{}{"BASIC_SSD"},
				"case_sensitive": false,
			},
		}

		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("expected case-insensitive match to pass, got: %v", err)
		}
	})

	t.Run("fails_on_null_when_allow_null_is_false", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"tier": cty.NullVal(cty.String),
		})

		rule := modulereader.ValidationRule{
			Validator: "allowed_enum",
			Inputs: map[string]interface{}{
				"vars":       []interface{}{"tier"},
				"allowed":    []interface{}{"BASIC_SSD"},
				"allow_null": false,
			},
		}

		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err == nil {
			t.Fatal("expected error for null value when allow_null is false, got nil")
		}
	})

	t.Run("passes_on_null_when_allow_null_is_true", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"tier": cty.NullVal(cty.String),
		})

		rule := modulereader.ValidationRule{
			Validator: "allowed_enum",
			Inputs: map[string]interface{}{
				"vars":       []interface{}{"tier"},
				"allowed":    []interface{}{"BASIC_SSD"},
				"allow_null": true,
			},
		}

		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected error for null value when allow_null is true: %v", err)
		}
	})

	t.Run("fails_with_missing_allowed_input", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator: "allowed_enum",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"tier"},
			},
		}

		err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0)
		if err == nil {
			t.Fatal("expected error for missing 'allowed' input, got nil")
		}
		if !strings.Contains(err.Error(), "missing an 'allowed' list") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}
