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

var (
	// exact anchored form used previously
	varsRefExactRegex = regexp.MustCompile(`^\$\(vars\.([a-zA-Z0-9_]+)\)$`)
	// liberal search for any $(vars.NAME) occurrences inside a string
	varsRefSearchRegex = regexp.MustCompile(`\$\(\s*vars\.([a-zA-Z0-9_]+)\s*\)`)
)

// resolveNumericFromValue attempts to extract a numeric value from a cty.Value.
// Returns (value, present, error).
// - present == false means value is missing/unknown/unresolvable (conservative => skip).
// - present == true with error means the value is present but type-invalid.
func resolveNumericFromValue(bp config.Blueprint, v cty.Value, settingName string) (float64, bool, error) {
	if !v.IsKnown() || v.IsNull() {
		return 0, false, nil
	}
	if v.Type() == cty.Number {
		f, _ := v.AsBigFloat().Float64()
		return f, true, nil
	}
	if v.Type() == cty.String {
		s := strings.TrimSpace(v.AsString())

		// strip surrounding single/double quotes if present (YAML quoting)
		if len(s) >= 2 {
			if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
				s = s[1 : len(s)-1]
				s = strings.TrimSpace(s)
			}
		}

		// 1) Exact anchored $(vars.NAME)
		if m := varsRefExactRegex.FindStringSubmatch(s); len(m) == 2 {
			varName := m[1]
			if !bp.Vars.Has(varName) {
				return 0, false, nil
			}
			refVal := bp.Vars.Get(varName)
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
				return 0, true, fmt.Errorf("variable %q referenced by %q must be a number or numeric string", varName, settingName)
			}
			return 0, true, fmt.Errorf("variable %q referenced by %q must be a number", varName, settingName)
		}

		// 2) Search for any $(vars.NAME) inside the string (handles quoting/templating nuances)
		if matches := varsRefSearchRegex.FindAllStringSubmatch(s, -1); len(matches) > 0 {
			for _, m := range matches {
				if len(m) < 2 {
					continue
				}
				varName := m[1]
				if !bp.Vars.Has(varName) {
					continue
				}
				refVal := bp.Vars.Get(varName)
				if !refVal.IsKnown() || refVal.IsNull() {
					continue
				}
				if refVal.Type() == cty.Number {
					f, _ := refVal.AsBigFloat().Float64()
					return f, true, nil
				}
				if refVal.Type() == cty.String {
					if parsed, err := strconv.ParseFloat(strings.TrimSpace(refVal.AsString()), 64); err == nil {
						return parsed, true, nil
					}
					// referenced var present but not numeric -> surface type error
					return 0, true, fmt.Errorf("variable %q referenced by %q must be a number or numeric string", varName, settingName)
				}
				// if referenced var is non-numeric type, surface error
				return 0, true, fmt.Errorf("variable %q referenced by %q must be a number", varName, settingName)
			}
			// we found references but none yielded a numeric value -> treat as not present
			return 0, false, nil
		}

		// 3) plain numeric string like "2"
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return parsed, true, nil
		}

		// not numeric and no $(vars...) pattern -> treat as not-present
		return 0, false, nil
	}
	// other types -> type error in context of numeric setting
	return 0, true, fmt.Errorf("setting %q must be a number or a string that can be resolved to a number", settingName)
}

// discoverFallbackCandidates returns candidate global var names derived from modID
// by scanning bp.Vars for keys that start with the module prefix and ranking them.
// This avoids a hardcoded suffix/token list and adapts to actual variables present.
func discoverFallbackCandidates(bp config.Blueprint, modID string, forDynamic bool) []string {
	// derive prefix from modID by trimming common suffixes
	prefix := modID
	for _, suf := range []string{"_nodeset", "-nodeset", "_nodes", "-nodes"} {
		if strings.HasSuffix(prefix, suf) {
			prefix = strings.TrimSuffix(prefix, suf)
			break
		}
	}
	prefix = strings.ToLower(prefix)
	if prefix == "" {
		return nil
	}

	type kv struct {
		k string
		s int
	}
	candidates := make([]kv, 0)

	// Inspect blueprint variables. Prefer keys that start with prefix_, then keys containing prefix,
	// then other numeric vars (so we don't hardcode tokens like "cluster"/"size").
	for k, v := range bp.Vars.Items() {
		// attempt to resolve each var value to a number (transitively) using resolveNumericFromValue
		if val, present, err := resolveNumericFromValue(bp, v, k); err == nil && present {
			// present and numeric candidate
			score := 1
			low := strings.ToLower(k)
			if strings.HasPrefix(low, prefix+"_") || strings.HasPrefix(low, prefix+"-") {
				score += 100
			} else if strings.Contains(low, prefix) {
				score += 50
			}
			// small tie-breaker: smaller numeric values are not necessarily better; we don't use value here.
			_ = val // keep for potential future heuristics
			candidates = append(candidates, kv{k, score})
		}
	}

	// sort by score desc
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].s > candidates[j].s })

	out := make([]string, 0, len(candidates))
	for _, e := range candidates {
		out = append(out, e.k)
	}
	return out
}

// resolveModuleNumericSetting attempts resolution for a numeric setting for a module.
// Order:
// 1) explicit module setting (items[name]) with support for $(vars.NAME) indirection
// 2) fallback: discover existing bp.Vars candidates with module prefix (discoverFallbackCandidates)
// Returns (value, found, error).
func resolveModuleNumericSetting(bp config.Blueprint, mod config.Module, items map[string]cty.Value, name string, forDynamic bool) (float64, bool, error) {
	// 1) explicit module setting first
	if items != nil {
		if v, ok := items[name]; ok {
			val, present, err := resolveNumericFromValue(bp, v, name)
			if err != nil {
				// present but invalid type -> surface
				return 0, true, err
			}
			if present {
				return val, true, nil
			}
			// present but not resolvable -> fall through to fallback
		}
	}

	// 2) fallback candidates discovered from bp.Vars
	modIDStr := string(mod.ID)
	for _, cand := range discoverFallbackCandidates(bp, modIDStr, forDynamic) {
		if bp.Vars.Has(cand) {
			refVal := bp.Vars.Get(cand)
			if !refVal.IsKnown() || refVal.IsNull() {
				continue
			}
			// accept numeric or parsable numeric string
			if refVal.Type() == cty.Number {
				f, _ := refVal.AsBigFloat().Float64()
				return f, true, nil
			}
			if refVal.Type() == cty.String {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(refVal.AsString()), 64); err == nil {
					return parsed, true, nil
				}
			}
		}
	}

	return 0, false, nil
}
