// Copyright 2026 Google LLC
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

func TestRangeValidator_Numeric(t *testing.T) {
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
							"single_value": cty.NumberIntVal(7),
						}),
					},
				},
			},
		},
	}

	validator := RangeValidator{}
	t.Run("passes_on_valid_value", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'single_value' value is too high",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"single_value"},
				"min":  2,
				"max":  10,
			},
		}
		if err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("fails_on_value_below_min", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"single_value": cty.NumberIntVal(1),
		})

		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'single_value' value is too low",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"single_value"},
				"min":  2,
				"max":  10,
			},
		}

		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "value is too low") {
			t.Fatalf("expected error message to contain 'value is too low', got: %q", err.Error())
		}
	})

	t.Run("fails_on_value_above_max", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"single_value": cty.NumberIntVal(14),
		})

		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'single_value' value is too high",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"single_value"},
				"min":  2,
				"max":  10,
			},
		}

		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "value is too high") {
			t.Fatalf("expected error message to contain 'value is too high', got: %q", err.Error())
		}
	})

	t.Run("passes_on_valid_list_values", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"list_values": cty.ListVal([]cty.Value{
				cty.NumberIntVal(100),
				cty.NumberIntVal(150),
				cty.NumberIntVal(200),
			}),
		})
		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'list_values' contains values that are too low",
			Inputs: map[string]interface{}{
				"vars":         []interface{}{"list_values"},
				"min":          100,
				"max":          200,
				"length_check": false,
			},
		}
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("fails_on_list_values_below_min", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"list_values": cty.ListVal([]cty.Value{
				cty.NumberIntVal(90),
				cty.NumberIntVal(150),
			}),
		})

		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'list_values' contains values that are too low",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"list_values"},
				"min":  100,
				"max":  200,
			},
		}

		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "too low") {
			t.Fatalf("expected error message to contain 'too low', got: %q", err.Error())
		}
	})

	t.Run("fails_on_list_values_above_max", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"list_values": cty.ListVal([]cty.Value{
				cty.NumberIntVal(148),
				cty.NumberIntVal(4675),
			}),
		})

		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'list_values' contains values that are too high",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"list_values"},
				"min":  100,
				"max":  200,
			},
		}

		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "too high") {
			t.Fatalf("expected error message to contain 'too high', got: %q", err.Error())
		}
	})

	t.Run("fails_on_float_value", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"float_var": cty.NumberFloatVal(2.5),
		})
		rule := modulereader.ValidationRule{
			Validator: "range",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"float_var"},
				"min":  1,
				"max":  5,
			},
		}
		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected failure for non-integer numeric value")
		}
		if !strings.Contains(err.Error(), "range validator only supports integer numbers") {
			t.Fatalf("expected error message to contain 'range validator only supports integer numbers', got: %q", err.Error())
		}
	})
}

func TestRangeValidator_Length(t *testing.T) {
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
							"list_strings": cty.ListVal([]cty.Value{
								cty.StringVal("net-a"),
								cty.StringVal("net-b"),
								cty.StringVal("net-c"),
							}),
						}),
					},
				},
			},
		},
	}

	validator := RangeValidator{}

	t.Run("passes_on_valid_list_length", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'list_strings' list length is too low",
			Inputs: map[string]interface{}{
				"vars":         []interface{}{"list_strings"},
				"min":          2,
				"max":          4,
				"length_check": true,
			},
		}
		if err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("fails_on_list_length_below_min", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"list_strings": cty.ListVal([]cty.Value{
				cty.StringVal("net-a"),
			}),
		})
		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'list_strings' list length is too low",
			Inputs: map[string]interface{}{
				"vars":         []interface{}{"list_strings"},
				"min":          2,
				"max":          4,
				"length_check": true,
			},
		}

		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "too low") {
			t.Fatalf("expected error message to contain 'too low', got: %q", err.Error())
		}
	})

	t.Run("fails_on_list_length_above_max", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"list_strings": cty.ListVal([]cty.Value{
				cty.StringVal("net-a"),
				cty.StringVal("net-b"),
				cty.StringVal("net-c"),
				cty.StringVal("net-d"),
				cty.StringVal("net-e"),
				cty.StringVal("net-f"),
			}),
		})
		rule := modulereader.ValidationRule{
			Validator:    "range",
			ErrorMessage: "'list_strings' list length is too high",
			Inputs: map[string]interface{}{
				"vars":         []interface{}{"list_strings"},
				"min":          2,
				"max":          4,
				"length_check": true,
			},
		}

		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "too high") {
			t.Fatalf("expected error message to contain 'too high', got: %q", err.Error())
		}
	})
}

func TestExclusiveValidator(t *testing.T) {
	baseBP := config.Blueprint{
		BlueprintName: "test-bp",
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:       "test-module",
						Source:   "test/module",
						Settings: config.NewDict(map[string]cty.Value{}),
					},
				},
			},
		},
	}

	validator := ExclusiveValidator{}

	t.Run("passes_on_empty_exclusion", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator:    "exclusion",
			ErrorMessage: "Only one of 'var_a' or 'var_b' should be present.",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"var_a", "var_b"},
			},
		}
		if err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
	t.Run("passes_on_valid_exclusion", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"var_a": cty.StringVal("one"),
		})
		rule := modulereader.ValidationRule{
			Validator:    "exclusion",
			ErrorMessage: "Only one of 'var_a' or 'var_b' should be set.",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"var_a", "var_b"},
			},
		}
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
	t.Run("fails_on_multiple_set_variables", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"var_a": cty.StringVal("one"),
			"var_b": cty.NumberIntVal(2),
		})
		rule := modulereader.ValidationRule{
			Validator:    "exclusion",
			ErrorMessage: "Only one of 'var_a' or 'var_b' should be set.",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"var_a", "var_b"},
			},
		}
		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "Only one of 'var_a' or 'var_b' should be set.") {
			t.Fatalf("unexpected error message: %q", err.Error())
		}
	})
	t.Run("passes_on_zero_or_empty_variables", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"var_a": cty.StringVal(""),
			"var_b": cty.NumberIntVal(0),
			"var_c": cty.False,
			"var_d": cty.ListValEmpty(cty.String),
		})
		rule := modulereader.ValidationRule{
			Validator:    "exclusion",
			ErrorMessage: "Only one variable should be set.",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"var_a", "var_b", "var_c", "var_d"},
			},
		}
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
	t.Run("passes_with_one_set_variable_and_others_empty", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"var_a": cty.StringVal("one"),
			"var_b": cty.NumberIntVal(0),
			"var_c": cty.False,
		})
		rule := modulereader.ValidationRule{
			Validator:    "exclusion",
			ErrorMessage: "Only one variable should be set.",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"var_a", "var_b", "var_c"},
			},
		}
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
	t.Run("fails_on_non_empty_map_and _other_set_variable", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"var_a": cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("value")}),
			"var_b": cty.NumberIntVal(2),
		})
		rule := modulereader.ValidationRule{
			Validator:    "exclusion",
			ErrorMessage: "Only one of 'var_a' or 'var_b' should be set.",
			Inputs: map[string]interface{}{
				"vars": []interface{}{"var_a", "var_b"},
			},
		}
		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "Only one of 'var_a' or 'var_b' should be set.") {
			t.Fatalf("unexpected error message: %q", err.Error())
		}
	})
}
