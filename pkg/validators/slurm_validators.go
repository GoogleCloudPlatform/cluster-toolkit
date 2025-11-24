
package validators

import (
	"fmt"
	"hpc-toolkit/pkg/config"
	"regexp"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

var slurmClusterNameRegex = regexp.MustCompile(`^[a-z](?:[a-z0-9]{0,9})$`)

// checkSlurmNodeCount validates Slurm nodeset modules have at least one node.
// Delegates numeric resolution to resolveModuleNumericSetting.
func checkSlurmNodeCount(bp config.Blueprint) error {
	var errs []string

	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			// identify nodeset modules by source path containing "nodeset"
			if !strings.Contains(strings.ToLower(mod.Source), "nodeset") {
				continue
			}
			items := mod.Settings.Items()

			staticVal, staticPresent, err := resolveModuleNumericSetting(bp, mod, items, "node_count_static", false)
			if err != nil {
				errs = append(errs, fmt.Sprintf("module %q: %v", mod.ID, err))
				continue
			}
			dynVal, dynPresent, err := resolveModuleNumericSetting(bp, mod, items, "node_count_dynamic_max", true)
			if err != nil {
				errs = append(errs, fmt.Sprintf("module %q: %v", mod.ID, err))
				continue
			}

			// If neither setting is present, skip (conservative).
			if !staticPresent && !dynPresent {
				continue
			}

			// If both present and both <= 0 => error.
			if staticPresent && dynPresent {
				if staticVal <= 0 && dynVal <= 0 {
					errs = append(errs, fmt.Sprintf("in nodeset module %q, at least one of 'node_count_static' or 'node_count_dynamic_max' must be greater than 0", mod.ID))
				}
				continue
			}

			// If only static present, ensure it's > 0
			if staticPresent && !dynPresent {
				if staticVal <= 0 {
					errs = append(errs, fmt.Sprintf("in nodeset module %q, 'node_count_static' must be greater than 0", mod.ID))
				}
				continue
			}

			// If only dynamic present, ensure it's > 0
			if dynPresent && !staticPresent {
				if dynVal <= 0 {
					errs = append(errs, fmt.Sprintf("in nodeset module %q, 'node_count_dynamic_max' must be greater than 0", mod.ID))
				}
				continue
			}
		}
	}

	if len(errs) > 0 {
		var sb strings.Builder
		sb.WriteString("One or more nodeset modules have invalid node counts:")
		for _, e := range errs {
			sb.WriteString("\n  - ")
			sb.WriteString(e)
		}
		return fmt.Errorf("%s", sb.String())
	}
	return nil
}

// provisioningState holds the detected provisioning settings for a given prefix.
type provisioningState struct {
	reservation  string
	resPresent   bool
	spot         bool
	spotPresent  bool
	dws          bool
	dwsPresent   bool
	observedKeys []string
}

// formatProvisioningSettings creates a multi-line string of provisioning settings.
func formatProvisioningSettings(s *provisioningState, listOnlyActive bool) string {
	var settings []string

	// Find and format reservation settings
	if s.resPresent {
		if !listOnlyActive || (listOnlyActive && strings.TrimSpace(s.reservation) != "") {
			for _, k := range s.observedKeys {
				if strings.HasSuffix(strings.ToLower(k), "_reservation_name") || strings.ToLower(k) == "reservation_name" {
					settings = append(settings, fmt.Sprintf("%s=%q", k, s.reservation))
				}
			}
		}
	}

	// Find and format spot VM settings
	if s.spotPresent {
		if !listOnlyActive || (listOnlyActive && s.spot) {
			for _, k := range s.observedKeys {
				if strings.HasSuffix(strings.ToLower(k), "_enable_spot_vm") || strings.ToLower(k) == "enable_spot_vm" {
					settings = append(settings, fmt.Sprintf("%s=%t", k, s.spot))
				}
			}
		}
	}

	// Find and format DWS flex settings
	if s.dwsPresent {
		if !listOnlyActive || (listOnlyActive && s.dws) {
			for _, k := range s.observedKeys {
				if strings.HasSuffix(strings.ToLower(k), "_dws_flex_enabled") || strings.ToLower(k) == "dws_flex_enabled" {
					settings = append(settings, fmt.Sprintf("%s=%t", k, s.dws))
				}
			}
		}
	}

	var details strings.Builder
	for _, setting := range settings {
		details.WriteString(fmt.Sprintf("\n  - %s", setting))
	}
	return details.String()
}

// checkSlurmProvisioning ensures exactly one provisioning model is chosen per prefix.
func checkSlurmProvisioning(bp config.Blueprint) error {
	prefixes := map[string]*provisioningState{}
	getState := func(prefix string) *provisioningState {
		if s, ok := prefixes[prefix]; ok {
			return s
		}
		s := &provisioningState{}
		prefixes[prefix] = s
		return s
	}

	scanMap := func(items map[string]cty.Value) {
		for k, v := range items {
			if v.IsNull() || !v.IsKnown() {
				continue
			}
			low := strings.ToLower(k)
			// reservation_name
			if low == "reservation_name" || strings.HasSuffix(low, "_reservation_name") {
				prefix := ""
				if low != "reservation_name" {
					prefix = strings.TrimSuffix(k, "_reservation_name")
				}
				s := getState(prefix)
				if v.Type() == cty.String {
					s.reservation = v.AsString()
					s.resPresent = true
					s.observedKeys = append(s.observedKeys, k)
				}
				continue
			}
			// enable_spot_vm
			if low == "enable_spot_vm" || strings.HasSuffix(low, "_enable_spot_vm") {
				prefix := ""
				if low != "enable_spot_vm" {
					prefix = strings.TrimSuffix(k, "_enable_spot_vm")
				}
				s := getState(prefix)
				if v.Type() == cty.Bool {
					s.spot = v.True()
					s.spotPresent = true
					s.observedKeys = append(s.observedKeys, k)
				}
				continue
			}
			// dws_flex_enabled
			if low == "dws_flex_enabled" || strings.HasSuffix(low, "_dws_flex_enabled") {
				prefix := ""
				if low != "dws_flex_enabled" {
					prefix = strings.TrimSuffix(k, "_dws_flex_enabled")
				}
				s := getState(prefix)
				if v.Type() == cty.Bool {
					s.dws = v.True()
					s.dwsPresent = true
					s.observedKeys = append(s.observedKeys, k)
				}
				continue
			}
		}
	}

	// scan blueprint vars and module settings
	scanMap(bp.Vars.Items())
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			if mod.Settings.Items() != nil {
				scanMap(mod.Settings.Items())
			}
		}
	}

	if len(prefixes) == 0 {
		return nil
	}

	var msgs []string
	for prefix, s := range prefixes {
		if !s.resPresent && !s.spotPresent && !s.dwsPresent {
			continue
		}
		selectedCount := 0
		if s.resPresent && strings.TrimSpace(s.reservation) != "" {
			selectedCount++
		}
		if s.spotPresent && s.spot {
			selectedCount++
		}
		if s.dwsPresent && s.dws {
			selectedCount++
		}

		label := prefix
		if label == "" {
			label = "<root>"
		}

		if selectedCount == 0 {
			details := formatProvisioningSettings(s, false)
			msgs = append(msgs, fmt.Sprintf(
				"no provisioning model selected for prefix %q.\nFound settings:%s\nPlease enable one provisioning model (e.g. set a reservation_name, or set enable_spot_vm to true).",
				label, details))
			continue
		}

		if selectedCount > 1 {
			details := formatProvisioningSettings(s, true)
			msgs = append(msgs, fmt.Sprintf(
				"provisioning conflict for prefix %q: multiple models selected.\nConflicting settings:%s\nPlease choose only one.",
				label, details))
			continue
		}
	}

	if len(msgs) > 0 {
		return fmt.Errorf("%s", strings.Join(msgs, "\n"))
	}
	return nil
}

// checkSlurmClusterName ensures slurm_cluster_name (if present) follows regex.
func checkSlurmClusterName(bp config.Blueprint) error {
	// blueprint-level var first
	if bp.Vars.Has("slurm_cluster_name") {
		v := bp.Vars.Get("slurm_cluster_name")
		if !v.IsNull() && v.IsKnown() {
			if v.Type() != cty.String {
				return fmt.Errorf("variable 'slurm_cluster_name' must be a string")
			}
			name := v.AsString()
			if !slurmClusterNameRegex.MatchString(name) {
				return fmt.Errorf("variable 'slurm_cluster_name' ('%s') must match regex '^[a-z](?:[a-z0-9]{0,9})$' (lowercase, 1-10 chars, alpha first, no hyphens)", name)
			}
			return nil
		}
	}
	// fall back to scanning module settings (unchanged)
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			for k, v := range mod.Settings.Items() {
				low := strings.ToLower(k)
				if !strings.Contains(low, "slurm_cluster_name") {
					continue
				}
				if v.IsNull() || !v.IsKnown() {
					continue
				}
				if v.Type() != cty.String {
					return fmt.Errorf("variable '%s' must be a string", k)
				}
				name := v.AsString()
				if !slurmClusterNameRegex.MatchString(name) {
					return fmt.Errorf("variable '%s' ('%s') must match regex '^[a-z](?:[a-z0-9]{0,9})$' (lowercase, 1-10 chars, alpha first, no hyphens)", k, name)
				}
				return nil
			}
		}
	}
	return nil
}
