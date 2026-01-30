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

func TestIsVarSet(t *testing.T) {
	tests := []struct {
		name string
		val  []cty.Value
		want bool
	}{
		{"null", []cty.Value{cty.NilVal}, false},
		{"empty string", []cty.Value{cty.StringVal("")}, false},
		{"valid string", []cty.Value{cty.StringVal("ok")}, true},
		{"bool false", []cty.Value{cty.BoolVal(false)}, false},
		{"bool true", []cty.Value{cty.BoolVal(true)}, true},
		{"num 0", []cty.Value{cty.NumberIntVal(0)}, false},
		{"num positive", []cty.Value{cty.NumberIntVal(5)}, true},
		{"empty list", []cty.Value{cty.ListValEmpty(cty.String)}, false},
		{"populated list", []cty.Value{cty.ListVal([]cty.Value{cty.StringVal("a")})}, true},
		{"empty object", []cty.Value{cty.EmptyObjectVal}, false},
		{"populated object", []cty.Value{cty.ObjectVal(map[string]cty.Value{"a": cty.True})}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVarSet(tt.val); got != tt.want {
				t.Errorf("isVarSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertToCty(t *testing.T) {
	val := convertToCty(nil)
	if !val.IsNull() {
		t.Error("convertToCty(nil) should return NilVal")
	}

	val = convertToCty("test")
	if val.AsString() != "test" {
		t.Error("convertToCty(string) failed")
	}

	// Test Slice
	sliceInput := []interface{}{"a", 1}
	val = convertToCty(sliceInput)
	if !val.Type().IsTupleType() || val.LengthInt() != 2 {
		t.Errorf("convertToCty(slice) failed, got %s", val.GoString())
	}

	// Test Map
	mapInput := map[string]interface{}{"key": true}
	val = convertToCty(mapInput)
	if !val.Type().IsObjectType() || !val.GetAttr("key").True() {
		t.Error("convertToCty(map) failed")
	}
}

func TestValuesMatch(t *testing.T) {
	if !ValuesMatch([]cty.Value{cty.NilVal}, []cty.Value{cty.BoolVal(false)}) {
		t.Error("ValuesMatch: null should equal false")
	}
	if !ValuesMatch([]cty.Value{cty.NumberIntVal(10)}, []cty.Value{cty.NumberIntVal(10)}) {
		t.Error("ValuesMatch: numbers should match")
	}
	vList1 := []cty.Value{cty.TupleVal([]cty.Value{cty.StringVal("a")})}
	vList2 := []cty.Value{cty.TupleVal([]cty.Value{cty.StringVal("a")})}
	if !ValuesMatch(vList1, vList2) {
		t.Error("ValuesMatch: identical lists should match")
	}
}

func TestConditionalValidator_Triggers(t *testing.T) {
	baseBP := config.Blueprint{
		BlueprintName: "test-bp",
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{ID: "test-module", Source: "test/module", Settings: config.NewDict(map[string]cty.Value{})},
				},
			},
		},
	}
	validator := ConditionalValidator{}

	t.Run("fails_on_missing_trigger_input_in_rule", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator: "conditional",
			Inputs:    map[string]interface{}{"dependent": "foo"},
		}
		err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0)
		if err == nil || !strings.Contains(err.Error(), "missing 'trigger'") {
			t.Fatalf("expected error for missing 'trigger' input, got: %v", err)
		}
	})

	t.Run("handles_null_equals_false_for_trigger_value", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator: "conditional",
			Inputs: map[string]interface{}{
				"trigger":       "missing_var",
				"trigger_value": false,
				"dependent":     "dep",
				"optional":      true,
			},
		}
		err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0)
		if err == nil || !strings.Contains(err.Error(), "variable \"dep\" is required") {
			t.Fatal("expected dependent check to trigger because null matches false")
		}
	})

	t.Run("passes_when_trigger_missing_in_blueprint_and_optional", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator: "conditional",
			Inputs: map[string]interface{}{
				"trigger":   "missing_var",
				"dependent": "dep_var",
				"optional":  true,
			},
		}
		if err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0); err != nil {
			t.Fatalf("unexpected error for missing optional trigger: %v", err)
		}
	})

	t.Run("fails_when_trigger_missing_in_blueprint_and_not_optional", func(t *testing.T) {
		rule := modulereader.ValidationRule{
			Validator: "conditional",
			Inputs: map[string]interface{}{
				"trigger":   "missing_var",
				"dependent": "dep_var",
				"optional":  false,
			},
		}
		err := validator.Validate(baseBP, baseBP.Groups[0].Modules[0], rule, baseBP.Groups[0], 0)
		if err == nil || !strings.Contains(err.Error(), "setting \"missing_var\" not found") {
			t.Fatalf("expected error for missing required trigger, got: %v", err)
		}
	})

	t.Run("skips_when_trigger_is_false_and_no_trigger_value_provided", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"trigger_var": cty.BoolVal(false),
		})
		rule := modulereader.ValidationRule{
			Validator: "conditional",
			Inputs: map[string]interface{}{
				"trigger":   "trigger_var",
				"dependent": "dep_var",
			},
		}
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err != nil {
			t.Fatalf("validation should have skipped, but got error: %v", err)
		}
	})
}

func TestConditionalValidator_Dependents(t *testing.T) {
	baseBP := config.Blueprint{
		BlueprintName: "test-bp",
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{ID: "test-module", Source: "test/module", Settings: config.NewDict(map[string]cty.Value{})},
				},
			},
		},
	}
	validator := ConditionalValidator{}

	t.Run("fails_when_trigger_is_true_and_dependent_is_missing", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"trigger_var": cty.BoolVal(true),
		})
		rule := modulereader.ValidationRule{
			Validator:    "conditional",
			ErrorMessage: "DEP_REQUIRED",
			Inputs: map[string]interface{}{
				"trigger":   "trigger_var",
				"dependent": "dep_var",
			},
		}
		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil || !strings.Contains(err.Error(), "DEP_REQUIRED") {
			t.Fatalf("expected custom error message, got: %v", err)
		}
	})

	t.Run("trigger_value_match_triggers_dependent_check", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"machine": cty.StringVal("a3-megagpu"),
		})
		rule := modulereader.ValidationRule{
			Validator: "conditional",
			Inputs: map[string]interface{}{
				"trigger":       "machine",
				"trigger_value": "a3-megagpu",
				"dependent":     "gpu_count",
			},
		}
		if err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0); err == nil {
			t.Fatal("expected error: trigger matched, so dependent is required")
		}
	})

	t.Run("dependent_value_mismatch_shows_formatted_error", func(t *testing.T) {
		bp := baseBP
		bp.Groups[0].Modules[0].Settings = config.NewDict(map[string]cty.Value{
			"trigger": cty.True,
			"dep":     cty.NumberIntVal(10),
		})
		rule := modulereader.ValidationRule{
			Validator: "conditional",
			Inputs: map[string]interface{}{
				"trigger":         "trigger",
				"dependent":       "dep",
				"dependent_value": 20,
			},
		}
		err := validator.Validate(bp, bp.Groups[0].Modules[0], rule, bp.Groups[0], 0)
		if err == nil {
			t.Fatal("expected error for value mismatch")
		}
		if !strings.Contains(err.Error(), "expected: '20', got: '10'") {
			t.Fatalf("error message format incorrect: %v", err)
		}
	})
}
