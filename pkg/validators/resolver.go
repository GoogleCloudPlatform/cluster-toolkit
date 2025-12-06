// Copyright 2025 "Google LLC"
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
	"fmt"
	"hpc-toolkit/pkg/config"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// regexes and tokenRe assumed defined in other file or duplicate them here
var (
	varsRefExactRegex  = regexp.MustCompile(`^\$\(vars\.([a-zA-Z0-9_]+)\)$`)
	varsRefSearchRegex = regexp.MustCompile(`\$\(\s*vars\.([a-zA-Z0-9_]+)\s*\)`)
	varDotRefRegex     = regexp.MustCompile(`\bvar\.([A-Za-z0-9_]+)\b`)
	tokenRe            = regexp.MustCompile(`[A-Za-z0-9]+`)
)

func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}

func tryParseNumber(s string) (float64, bool) {
	if parsed, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return parsed, true
	}
	return 0, false
}

func resolveRefGeneric(bp config.Blueprint, re *regexp.Regexp, s, settingName string) (float64, bool, error) {
	if matches := re.FindAllStringSubmatch(s, -1); len(matches) > 0 {
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			name := m[1]
			if !bp.Vars.Has(name) {
				continue
			}
			refVal := bp.Vars.Get(name)
			if !refVal.IsKnown() || refVal.IsNull() {
				continue
			}
			switch refVal.Type() {
			case cty.Number:
				f, _ := refVal.AsBigFloat().Float64()
				return f, true, nil
			case cty.String:
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(refVal.AsString()), 64); err == nil {
					return parsed, true, nil
				}
				return 0, true, fmt.Errorf("variable %q referenced by %q must be a number or numeric string", name, settingName)
			default:
				return 0, true, fmt.Errorf("variable %q referenced by %q must be a number", name, settingName)
			}
		}
		return 0, false, nil
	}
	return 0, false, nil
}

func resolveExactVarsRef(bp config.Blueprint, s, settingName string) (float64, bool, error) {
	if m := varsRefExactRegex.FindStringSubmatch(s); len(m) == 2 {
		name := m[1]
		if !bp.Vars.Has(name) {
			return 0, false, nil
		}
		refVal := bp.Vars.Get(name)
		if !refVal.IsKnown() || refVal.IsNull() {
			return 0, false, nil
		}
		if refVal.Type() == cty.Number {
			f, _ := refVal.AsBigFloat().Float64()
			return f, true, nil
		}
		if refVal.Type() == cty.String {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(refVal.AsString()), 64); err == nil {
				return parsed, true, nil
			}
			return 0, true, fmt.Errorf("variable %q referenced by %q must be a number or numeric string", name, settingName)
		}
		return 0, true, fmt.Errorf("variable %q referenced by %q must be a number", name, settingName)
	}
	return 0, false, nil
}

func resolveNumericFromString(bp config.Blueprint, s, settingName string) (float64, bool, error) {
	if val, ok, err := resolveExactVarsRef(bp, s, settingName); ok || err != nil {
		return val, ok, err
	}
	if val, ok, err := resolveRefGeneric(bp, varsRefSearchRegex, s, settingName); ok || err != nil {
		return val, ok, err
	}
	if val, ok, err := resolveRefGeneric(bp, varDotRefRegex, s, settingName); ok || err != nil {
		return val, ok, err
	}
	if parsed, ok := tryParseNumber(s); ok {
		return parsed, true, nil
	}
	return 0, false, nil
}

func resolveNumericFromValue(bp config.Blueprint, v cty.Value, settingName string) (float64, bool, error) {
	if !v.IsKnown() || v.IsNull() {
		return 0, false, nil
	}
	if v.Type() == cty.Number {
		f, _ := v.AsBigFloat().Float64()
		return f, true, nil
	}
	if v.Type() == cty.String {
		s := stripQuotes(strings.TrimSpace(v.AsString()))
		return resolveNumericFromString(bp, s, settingName)
	}
	repr := stripQuotes(strings.TrimSpace(fmt.Sprint(v)))
	if repr != "" {
		if val, ok, err := resolveNumericFromString(bp, repr, settingName); ok || err != nil {
			return val, ok, err
		}
	}
	return 0, true, fmt.Errorf("setting %q must be a number or a string that can be resolved to a number", settingName)
}

func extractRefsFromRepr(repr string) []string {
	if repr == "" {
		return nil
	}
	seen := make(map[string]struct{})
	for _, m := range varsRefSearchRegex.FindAllStringSubmatch(repr, -1) {
		if len(m) >= 2 {
			seen[m[1]] = struct{}{}
		}
	}
	for _, m := range varDotRefRegex.FindAllStringSubmatch(repr, -1) {
		if len(m) >= 2 {
			seen[m[1]] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}

func collectExplicitRefs(bp config.Blueprint, modID string) []string {
	primary := strings.ToLower(tokenRe.FindString(modID))
	set := map[string]struct{}{}

	addFromRepr := func(repr string) {
		if repr == "" {
			return
		}
		for _, ref := range extractRefsFromRepr(repr) {
			low := strings.ToLower(ref)
			if primary != "" && !(strings.Contains(low, primary) || strings.Contains(strings.ToLower(modID), low)) {
				continue
			}
			set[ref] = struct{}{}
		}
	}

	for _, v := range bp.Vars.Items() {
		if v.IsNull() {
			continue
		}
		addFromRepr(fmt.Sprint(v))
	}

	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			for _, v := range mod.Settings.Items() {
				addFromRepr(fmt.Sprint(v))
			}
		}
	}

	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func filterNumericRefs(bp config.Blueprint, refs []string, modID string) []string {
	if len(refs) == 0 {
		return nil
	}
	primary := strings.ToLower(tokenRe.FindString(modID))

	var prefixed []string
	var others []string
	for _, ref := range refs {
		if !bp.Vars.Has(ref) {
			continue
		}
		refVal := bp.Vars.Get(ref)
		if _, present, err := resolveNumericFromValue(bp, refVal, ref); err != nil || !present {
			continue
		}
		low := strings.ToLower(ref)
		if primary != "" && (low == primary || strings.HasPrefix(low, primary+"_") || strings.HasPrefix(low, primary+"-")) {
			prefixed = append(prefixed, ref)
		} else {
			others = append(others, ref)
		}
	}

	sort.SliceStable(prefixed, func(i, j int) bool {
		if len(prefixed[i]) == len(prefixed[j]) {
			return prefixed[i] < prefixed[j]
		}
		return len(prefixed[i]) < len(prefixed[j])
	})
	sort.Strings(others)

	out := append(prefixed, others...)
	if len(out) == 0 {
		return nil
	}
	return out
}

func discoverFallbackCandidatesImpl(bp config.Blueprint, modID string) []string {
	refs := collectExplicitRefs(bp, modID)
	return filterNumericRefs(bp, refs, modID)
}

func resolveModuleNumericSetting(bp config.Blueprint, mod config.Module, items map[string]cty.Value, name string) (float64, bool, error) {
	// 1) explicit module setting first
	if items != nil {
		if v, ok := items[name]; ok {
			val, present, err := resolveNumericFromValue(bp, v, name)
			if err != nil {
				// present but invalid type -> propagate
				return 0, true, err
			}
			if present {
				return val, true, nil
			}
			// present but not resolvable -> fall through to fallback
		}
	}

	// 2) fallback candidates discovered from explicit references in bp or module settings
	for _, cand := range discoverFallbackCandidatesImpl(bp, string(mod.ID)) {
		if bp.Vars.Has(cand) {
			refVal := bp.Vars.Get(cand)
			if val, present, _ := resolveNumericFromValue(bp, refVal, cand); present {
				return val, true, nil
			}
		}
	}

	return 0, false, nil
}
