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

package engine

import (
	"fmt"
	"sort"

	"hpc-toolkit/pkg/intent/ast"
	"hpc-toolkit/pkg/modulereader"

	"github.com/agext/levenshtein"
)

// ValidateModuleSettings ensures that all keys in the module's settings map
// correspond to valid inputs defined in the target module's variables.tf file.
func ValidateModuleSettings(module ast.ModuleSpec, toolkitPath string) error {
	// If the module lacks a source, there's no variables.tf to inspect.
	// E.g., virtual modules or stubs.
	if module.Source == "" {
		return nil
	}

	// Assuming all modules in this compiler's context are terraform.
	fullPath := resolveModuleSourcePath(module.Source, toolkitPath)

	modInfo, err := modulereader.GetModuleInfo(fullPath, "terraform")
	if err != nil {
		return fmt.Errorf("failed to read module source %q: %w", fullPath, err)
	}

	validInputs := make(map[string]bool)
	for _, input := range modInfo.Inputs {
		validInputs[input.Name] = true
	}

	var unknown []string
	for key := range module.Settings {
		if !validInputs[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		accepted := sortedKeys(validInputs)
		key := unknown[0]
		best, minDist := "", maxSettingHintDist+1
		for _, cand := range accepted {
			if d := levenshtein.Distance(key, cand, nil); d < minDist {
				best, minDist = cand, d
			}
		}
		target := fmt.Sprintf("module %q", module.ID)
		if module.PoolName != "" {
			target = fmt.Sprintf("pool %q", module.PoolName)
		} else if module.FeatureInstanceID != "" {
			target = fmt.Sprintf("feature %q", module.FeatureInstanceID)
		}
		if minDist <= maxSettingHintDist {
			return fmt.Errorf("unknown setting %q for %s (did you mean %q?)", key, target, best)
		}
		return fmt.Errorf("unknown setting %q for %s", key, target)
	}

	return nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
