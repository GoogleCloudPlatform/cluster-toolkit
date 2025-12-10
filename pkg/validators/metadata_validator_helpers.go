// Copyright 2025 "Google LLC"
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

// getBlueprintValues retrieves a cty.Value from blueprint variables by name,
// evaluates it and returns the flattened values and the path to use for errors.
func getBlueprintValues(bp config.Blueprint, varName string) ([]cty.Value, config.Path, error) {
	var nilPath config.Path

	if !bp.Vars.Has(varName) {
		return nil, nilPath, fmt.Errorf("variable %q not found in blueprint vars", varName)
	}
	val := bp.Vars.Get(varName)
	if evaledVal, err := bp.Eval(val); err == nil {
		val = evaledVal
	}
	values := evaluateAndFlatten(val)
	return values, config.Root.Vars.Dot(varName), nil
}

// getModuleSettingValues retrieves a cty.Value from module settings using a dot-separated path,
// evaluates expressions via bp.Eval and returns flattened slice + path for errors.
func getModuleSettingValues(bp config.Blueprint, group config.Group, modIdx int, mod config.Module, settingName string) ([]cty.Value, config.Path, error) {
	var nilPath config.Path

	val, found := getNestedValue(mod.Settings, settingName)
	if !found {
		return nil, nilPath, fmt.Errorf("setting %q not found in module %q settings", settingName, mod.ID)
	}
	if evaledVal, err := bp.Eval(val); err == nil {
		val = evaledVal
	}
	values := evaluateAndFlatten(val)

	groupIndex := bp.GroupIndex(group.Name)
	path := config.Root.Groups.At(groupIndex).Modules.At(modIdx).Settings.Dot(settingName)

	return values, path, nil
}

// valuesEqualBlueprint returns true when the provided flattened module values are
// equal to the flattened blueprint var with the same name. Comparison is only done
// for string values; non-comparable types return false.
func valuesEqualBlueprint(bp config.Blueprint, varName string, moduleValues []cty.Value) bool {
	if !bp.Vars.Has(varName) {
		return false
	}
	bpVal := bp.Vars.Get(varName)
	if evaledBp, err := bp.Eval(bpVal); err == nil {
		bpVal = evaledBp
	}
	bpVals := evaluateAndFlatten(bpVal)

	if len(bpVals) != len(moduleValues) {
		return false
	}
	for i := range bpVals {
		if bpVals[i].Type() != cty.String || moduleValues[i].Type() != cty.String {
			return false
		}
		if bpVals[i].AsString() != moduleValues[i].AsString() {
			return false
		}
	}
	return true
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

// Target represents a resolved target (module-setting or blueprint var) for validation.
type Target struct {
	Name        string
	Values      []cty.Value
	Path        config.Path
	IsBlueprint bool // true if came from blueprint vars, false if module.settings
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

// processVarsAsBlueprint processes a list of names as blueprint vars.
func processVarsAsBlueprint(bp config.Blueprint, list []string, optional bool, handler func(Target) error) error {
	for _, name := range list {
		values, path, err := getBlueprintValues(bp, name)
		if err != nil {
			if optional {
				continue
			}
			// legacy behavior: skip when missing blueprint var even if optional == false
			continue
		}
		if err := handler(Target{Name: name, Values: values, Path: path, IsBlueprint: true}); err != nil {
			return err
		}
	}
	return nil
}

// processVarsPreferModuleSetting prefers module.setting for each name; skips if module.setting absent.
func processVarsPreferModuleSetting(bp config.Blueprint, mod config.Module, group config.Group, modIdx int, list []string, handler func(Target) error) error {
	for _, vname := range list {
		if val, ok := getNestedValue(mod.Settings, vname); ok {
			if evaled, err := bp.Eval(val); err == nil {
				val = evaled
			}
			values := evaluateAndFlatten(val)
			path := config.Root.Groups.At(bp.GroupIndex(group.Name)).Modules.At(modIdx).Settings.Dot(vname)
			if err := handler(Target{Name: vname, Values: values, Path: path, IsBlueprint: false}); err != nil {
				return err
			}
		}
		// else: skip (do not fallback to blueprint var)
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

	varsList, _ := parseStringList(rule.Inputs["vars"])
	settingsList, _ := parseStringList(rule.Inputs["settings"])
	scope, _ := rule.Inputs["scope"].(string)
	optional := true
	if v, ok := rule.Inputs["optional"]; ok {
		if b, ok := v.(bool); ok {
			optional = b
		}
	}

	switch scope {
	case "module":
		if len(settingsList) > 0 {
			return processModuleSettings(bp, mod, group, modIdx, settingsList, optional, handler)
		}
		return processModuleSettings(bp, mod, group, modIdx, varsList, optional, handler)

	case "blueprint":
		return processVarsAsBlueprint(bp, varsList, optional, handler)

	default:
		if len(settingsList) > 0 {
			if err := processModuleSettings(bp, mod, group, modIdx, settingsList, optional, handler); err != nil {
				return err
			}
		}
		return processVarsPreferModuleSetting(bp, mod, group, modIdx, varsList, handler)
	}
}
