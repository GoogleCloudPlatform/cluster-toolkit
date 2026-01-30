// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validators

import (
	"fmt"
	"strings"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"

	"github.com/zclconf/go-cty/cty"
)

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
	return cty.NilVal, false
}

// evaluateAndFlatten converts a cty.Value into a slice of cty.Value elements.
// If it's a list/tuple, returns elements; otherwise returns single-item slice.
func evaluateAndFlatten(val cty.Value) []cty.Value {
	var values []cty.Value
	if val.Type().IsListType() || val.Type().IsTupleType() {
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			values = append(values, v)
		}
	} else {
		values = append(values, val)
	}
	return values
}

// getModuleSettingValues retrieves a cty.Value from module settings using a dot-separated path,
// evaluates expressions via bp.Eval and returns flattened slice + path for errors.
func getModuleSettingValues(bp config.Blueprint, group config.Group, modIdx int, mod config.Module, settingName string) ([]cty.Value, config.Path, error) {
	var nilPath config.Path

	// Determine canonical path for this module.setting in the blueprint.
	groupIndex := bp.GroupIndex(group.Name)
	path := config.Root.Groups.At(groupIndex).Modules.At(modIdx).Settings.Dot(settingName)

	// If YAML context exists and the path is not present in the user's YAML,
	// treat the setting as absent so validators skip it (honoring optional:true).

	if bp.YamlCtx != nil {
		if _, ok := bp.YamlCtx.Pos(path); !ok {
			return nil, nilPath, fmt.Errorf("setting %q not present in blueprint YAML for module %q", settingName, mod.ID)
		}
	}

	val, ok := getNestedValue(mod.Settings, settingName)
	if !ok {
		return nil, nilPath, fmt.Errorf("setting %q not present in blueprint YAML for module %q", settingName, mod.ID)
	}

	if evaledVal, err := bp.Eval(val); err == nil {
		val = evaledVal
	}
	values := evaluateAndFlatten(val)

	return values, path, nil
}

// parseStringList normalizes an input that may be a single string, []interface{} or nil into []string.
func parseStringList(v interface{}) ([]string, bool) {
	if v == nil {
		return nil, false
	}
	switch vv := v.(type) {
	case string:
		return []string{vv}, true
	case []interface{}:
		out := make([]string, 0, len(vv))
		for _, e := range vv {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	case []string:
		return vv, true
	default:
		return nil, false
	}
}

// parseString normalizes an input that may be a single string or a list containing a single string into a string.
func parseString(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch vv := v.(type) {
	case string:
		return vv, true
	case []interface{}:
		if len(vv) == 1 {
			s, ok := vv[0].(string)
			return s, ok
		}
	case []string:
		if len(vv) == 1 {
			return vv[0], true
		}
	}
	return "", false
}

// Target represents a resolved target (module-setting or blueprint var) for validation.
type Target struct {
	Name        string
	Values      []cty.Value
	Path        config.Path
	IsBlueprint bool // true if came from blueprint vars, false if module.settings
}

// parseIntInput parses an integer from the input map and returns a pointer.
// It returns nil if the key is not found.
func parseIntInput(inputs map[string]interface{}, key string) (*int, error) {
	v, ok := inputs[key]
	if !ok {
		return nil, nil
	}

	var val int
	switch t := v.(type) {
	case int:
		val = t
	case float64:
		val = int(t)
	default:
		return nil, fmt.Errorf("'%s' must be an integer, not %T", key, v)
	}
	return &val, nil
}

// processModuleSettings processes a list of names interpreted as module settings.
func processModuleSettings(bp config.Blueprint, mod config.Module, group config.Group, modIdx int, list []string, optional bool, handler func(Target) error) error {
	for _, s := range list {
		values, path, err := getModuleSettingValues(bp, group, modIdx, mod, s)
		if err != nil {
			if optional {
				continue
			}
			missingPath := config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Settings.Dot(s)
			return config.BpError{
				Err:  fmt.Errorf("setting %q not found in module %q settings", s, mod.ID),
				Path: missingPath,
			}
		}
		if err := handler(Target{Name: s, Values: values, Path: path, IsBlueprint: false}); err != nil {
			return err
		}
	}
	return nil
}

// IterateRuleTargets resolves vars/settings from a validation rule according to scope and optional semantics,
// and calls the provided handler for each resolved Target. The handler may return an error to stop iteration.
func IterateRuleTargets(
	bp config.Blueprint,
	mod config.Module,
	rule modulereader.ValidationRule,
	group config.Group,
	modIdx int,
	handler func(Target) error,
) error {

	// Only support `vars:` in module metadata validators (each treated as a module.setting name).
	// `settings:` support has been removed to enforce a single convention.
	varsList, _ := parseStringList(rule.Inputs["vars"])
	optional := true
	if v, ok := rule.Inputs["optional"]; ok {
		if b, ok := v.(bool); ok {
			optional = b
		}
	}

	// Interpret each var name as module.settings.<name>
	return processModuleSettings(bp, mod, group, modIdx, varsList, optional, handler)
}

// parseBoolInput retrieves a boolean value from a map by key.
// If the key is missing, it returns the defaultVal.
// If the key is present but not a boolean, it returns an error.
func parseBoolInput(inputs map[string]interface{}, key string, defaultVal bool) (bool, error) {
	v, ok := inputs[key]
	if !ok {
		return defaultVal, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("'%s' must be a boolean, not %T", key, v)
	}
	return b, nil
}

// isVarSet checks if the value is "truthy": non-null, known, and not "empty" (false, 0, empty string/list/map).
func isVarSet(values []cty.Value) bool {
	if len(values) == 0 {
		return false
	}
	for _, val := range values {
		if val.IsNull() || !val.IsKnown() {
			return false
		}
		switch val.Type() {
		case cty.String:
			if val.AsString() == "" {
				return false
			}
		case cty.Number:
			if val.AsBigFloat().Sign() <= 0 {
				return false
			}
		case cty.Bool:
			if val.False() {
				return false
			}
		default:
			// For lists, maps, and sets, consider them "not set" if they are empty.
			if val.LengthInt() == 0 {
				return false
			}
		}
	}
	return true
}

// convertToCty converts a primitive Go interface{} to a cty.Value.
// Returns cty.Value if input was not nil, otherwise cty.NilVal.
func convertToCty(in interface{}) cty.Value {
	if in == nil {
		return cty.NilVal
	}
	switch v := in.(type) {
	case bool:
		return cty.BoolVal(v)
	case int:
		return cty.NumberIntVal(int64(v))
	case float64:
		return cty.NumberFloatVal(v)
	case string:
		return cty.StringVal(v)
	case []interface{}:
		if len(v) == 0 {
			return cty.EmptyTupleVal
		}
		vals := make([]cty.Value, len(v))
		for i, val := range v {
			vals[i] = convertToCty(val)
		}
		return cty.TupleVal(vals)
	case map[string]interface{}:
		if len(v) == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value)
		for k, val := range v {
			vals[k] = convertToCty(val)
		}
		return cty.ObjectVal(vals)
	default:
		return cty.NilVal
	}
}

// ValuesMatch compares two slices of cty.Value for equality.
func ValuesMatch(original []cty.Value, expected []cty.Value) bool {
	if len(original) != len(expected) {
		return false
	}
	for i := range original {
		originalVal := original[i]
		expectedVal := expected[i]

		isOriginalSet := isVarSet([]cty.Value{originalVal})
		isExpectedSet := isVarSet([]cty.Value{expectedVal})
		if !isOriginalSet && !isExpectedSet {
			continue
		}
		if !originalVal.Equals(expectedVal).True() {
			return false
		}
	}
	return true
}

// formatValue formats a cty.Value slice for display in error messages.
func formatValue(vals []cty.Value) string {
	if len(vals) == 0 {
		return "null"
	}
	val := vals[0]
	if val.IsNull() {
		return "null"
	}
	return string(config.TokensForValue(val).Bytes())
}
