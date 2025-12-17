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

	val, _ := getNestedValue(mod.Settings, settingName)

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
