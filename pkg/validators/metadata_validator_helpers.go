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

// getNestedValueEval retrieves a cty.Value from a Dict using a dot-separated path,
// evaluating intermediate expression values against the blueprint when encountered.
func getNestedValueEval(bp config.Blueprint, d config.Dict, path string) (cty.Value, bool) {
	parts := strings.Split(path, ".")
	currentVal := d.AsObject()

	for i, part := range parts {
		if _, isExpr := config.IsExpressionValue(currentVal); isExpr {
			if evaledVal, err := bp.Eval(currentVal); err == nil {
				currentVal = evaledVal
			}
		}
		if currentVal == cty.NilVal || !currentVal.IsKnown() || currentVal.IsNull() {
			return cty.NilVal, false
		}
		currentVal, _ = currentVal.Unmark()
		if !currentVal.Type().IsObjectType() || !currentVal.Type().HasAttribute(part) {
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

// getNestedValue retrieves a cty.Value from a Dict using a dot-separated path.
func getNestedValue(d config.Dict, path string) (cty.Value, bool) {
	return getNestedValueEval(config.Blueprint{}, d, path)
}

func resolveExplicitSettingPath(bp config.Blueprint, groupIndex int, modIdx int, mod config.Module, settingName string, hasSetting bool) (config.Path, bool) {
	settingsPath := config.Root.Groups.At(groupIndex).Modules.At(modIdx).Settings
	path := settingsPath.Dot(settingName)
	if bp.YamlCtx == nil {
		return path, hasSetting
	}
	if _, ok := bp.YamlCtx.Pos(path); ok {
		return path, true
	}
	if !hasSetting {
		return path, false
	}
	parts := strings.Split(settingName, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		prefixPath := settingsPath.Dot(strings.Join(parts[:i], "."))
		if _, ok := bp.YamlCtx.Pos(prefixPath); ok {
			return prefixPath, true
		}
	}
	return path, mod.Settings.Has(parts[0]) && len(config.IsProductOfModuleUse(mod.Settings.Get(parts[0]))) > 0
}

// evaluateAndFlatten converts a cty.Value into a slice of cty.Value elements.
// If it is a list, tuple, or set, returns its elements; otherwise returns a single-item slice.
func evaluateAndFlatten(val cty.Value) []cty.Value {
	if val == cty.NilVal || !val.IsKnown() || val.IsNull() {
		return []cty.Value{val}
	}
	val, _ = val.Unmark()
	if val.Type().IsListType() || val.Type().IsTupleType() || val.Type().IsSetType() {
		var values []cty.Value
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			if v.IsKnown() {
				v, _ = v.UnmarkDeep()
			}
			values = append(values, v)
		}
		return values
	}
	val, _ = val.UnmarkDeep()
	return []cty.Value{val}
}

// getModuleSettingValues retrieves a cty.Value from module settings using a dot-separated path,
// evaluates expressions via bp.Eval and returns flattened slice + path for errors.
func getModuleSettingValues(bp config.Blueprint, group config.Group, modIdx int, mod config.Module, settingName string) ([]cty.Value, config.Path, error) {
	var nilPath config.Path

	groupIndex := bp.GroupIndex(group.Name)
	val, hasSetting := getNestedValueEval(bp, mod.Settings, settingName)
	path, isExplicitSetting := resolveExplicitSettingPath(bp, groupIndex, modIdx, mod, settingName, hasSetting)

	// If the setting is omitted from the settings block, check if it's implicitly bound
	// to a global variable with the exact same name declared in the vars block.
	if !isExplicitSetting {
		if varVal, ok := getNestedValueEval(bp, bp.Vars, settingName); ok {
			if evaledVal, err := bp.Eval(varVal); err == nil {
				varVal = evaledVal
			}
			return evaluateAndFlatten(varVal), config.Root.Vars.Dot(settingName), nil
		}
		return nil, nilPath, fmt.Errorf("setting %q not present in blueprint YAML for module %q", settingName, mod.ID)
	}

	if !hasSetting {
		return nil, nilPath, fmt.Errorf("setting %q not present in blueprint YAML for module %q", settingName, mod.ID)
	}

	if evaledVal, err := bp.Eval(val); err == nil {
		val = evaledVal
	}
	return evaluateAndFlatten(val), path, nil
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
	optional, _ := parseBoolInput(rule.Inputs, "optional", true)

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

func isSingleValSet(val cty.Value) bool {
	if val == cty.NilVal || val.IsNull() {
		return false
	}
	if !val.IsKnown() {
		_, isExpr := config.IsExpressionValue(val)
		return isExpr || len(config.IsProductOfModuleUse(val)) > 0
	}
	val, _ = val.Unmark()
	switch val.Type() {
	case cty.String:
		return val.AsString() != ""
	case cty.Number:
		return val.AsBigFloat().Sign() > 0
	case cty.Bool:
		return val.True()
	default:
		// For lists, maps, and sets, consider them "not set" if they are empty.
		return val.LengthInt() > 0
	}
}

// isVarSet returns true if the value is known (or a dynamic expression/module use reference), non-null,
// and non-empty (positive number, non-empty string, true bool, or non-empty collection).
func isVarSet(values []cty.Value) bool {
	if len(values) == 0 {
		return false
	}
	for _, val := range values {
		if !isSingleValSet(val) {
			return false
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

// ValuesMatch compares two slices of cty.Value for equality, treating "unset" values as equal.
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
		if isOriginalSet != isExpectedSet || !originalVal.IsKnown() || !expectedVal.IsKnown() {
			return false
		}
		origUnmarked, _ := originalVal.UnmarkDeep()
		expUnmarked, _ := expectedVal.UnmarkDeep()
		if !origUnmarked.Equals(expUnmarked).True() {
			return false
		}
	}
	return true
}

// formatValue formats a cty.Value slice for display in error messages.
func formatValue(vals []cty.Value) string {
	if len(vals) == 0 || vals[0] == cty.NilVal || vals[0].IsNull() {
		return "null"
	}
	if _, isExpr := config.IsExpressionValue(vals[0]); !isExpr && !vals[0].IsKnown() {
		return "unknown"
	}
	return string(config.TokensForValue(vals[0]).Bytes())
}
