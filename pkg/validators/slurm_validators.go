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
	"strings"

	"github.com/zclconf/go-cty/cty"
)

var slurmClusterNameRegex = regexp.MustCompile(`^[a-z](?:[a-z0-9]{0,9})$`)

// Validates Slurm nodeset modules have at least one node.
// Delegates numeric resolution to resolveModuleNumericSetting.
func checkSlurmNodeCount(bp config.Blueprint) error {
	var all []string
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			// identify nodeset modules by source path containing "nodeset"
			if !strings.Contains(strings.ToLower(mod.Source), "nodeset") {
				continue
			}
			errs := validateNodeCountsForModule(bp, mod)
			all = append(all, errs...)
		}
	}
	if len(all) > 0 {
		var sb strings.Builder
		sb.WriteString("One or more nodeset modules have invalid node counts:")
		for _, e := range all {
			sb.WriteString("\n  - ")
			sb.WriteString(e)
		}
		return fmt.Errorf("%s", sb.String())
	}
	return nil
}

// Validates node counts for a single module and returns per-module errors.
func validateNodeCountsForModule(bp config.Blueprint, mod config.Module) []string {
	var errs []string

	items := mod.Settings.Items()
	staticVal, staticPresent, err := resolveModuleNumericSetting(bp, mod, items, "node_count_static")
	if err != nil {
		errs = append(errs, fmt.Sprintf("module %q: %v", mod.ID, err))
		return errs
	}
	dynVal, dynPresent, err := resolveModuleNumericSetting(bp, mod, items, "node_count_dynamic_max")
	if err != nil {
		errs = append(errs, fmt.Sprintf("module %q: %v", mod.ID, err))
		return errs
	}

	// Simple explicit checks for the three meaningful cases.
	switch {
	case staticPresent && !dynPresent:
		// only static present
		if staticVal <= 0 {
			return []string{fmt.Sprintf("in nodeset module %q, 'node_count_static' must be greater than 0", mod.ID)}
		}
	case !staticPresent && dynPresent:
		// only dynamic present
		if dynVal <= 0 {
			return []string{fmt.Sprintf("in nodeset module %q, 'node_count_dynamic_max' must be greater than 0", mod.ID)}
		}
	case staticPresent && dynPresent:
		// both present
		if staticVal <= 0 && dynVal <= 0 {
			return []string{fmt.Sprintf("in nodeset module %q, at least one of 'node_count_static' or 'node_count_dynamic_max' must be greater than 0", mod.ID)}
		}
	}
	// neither present, or validation passed
	return nil
}

// provisioningState holds the detected provisioning settings for a given prefix.
type provisioningState struct {
	reservation    string
	hasReservation bool
	spot           bool
	spotPresent    bool
	dws            bool
	dwsPresent     bool
	observedKeys   []string
}

// Creates a multi-line string of provisioning settings.
func formatProvisioningSettings(s *provisioningState, listOnlyActive bool) string {
	var out []string
	for _, k := range s.observedKeys {
		if line, ok := formatSettingLine(k, s, listOnlyActive); ok {
			out = append(out, line)
		}
	}
	var b strings.Builder
	for _, line := range out {
		b.WriteString("\n  - ")
		b.WriteString(line)
	}
	return b.String()
}

// formatSettingLine inspects a single observed key and formats it if applicable.
// Returns formatted string and true when formatted, otherwise ("", false).
func formatSettingLine(k string, s *provisioningState, listOnlyActive bool) (string, bool) {
	low := strings.ToLower(k)
	switch {
	case low == "reservation_name" || strings.HasSuffix(low, "_reservation_name"):
		if !listOnlyActive || strings.TrimSpace(s.reservation) != "" {
			return fmt.Sprintf("%s=%q", k, s.reservation), true
		}
	case low == "enable_spot_vm" || strings.HasSuffix(low, "_enable_spot_vm"):
		if !listOnlyActive || s.spot {
			return fmt.Sprintf("%s=%t", k, s.spot), true
		}
	case low == "dws_flex_enabled" || strings.HasSuffix(low, "_dws_flex_enabled"):
		if !listOnlyActive || s.dws {
			return fmt.Sprintf("%s=%t", k, s.dws), true
		}
	}
	return "", false
}

// provKeyDef defines a provisioning key and how to map it into state.
type provKeyDef struct {
	baseName string
	handler  func(s *provisioningState, v cty.Value, key string)
}

func provisioningKeyDefs() []provKeyDef {
	return []provKeyDef{
		{
			baseName: "reservation_name",
			handler: func(s *provisioningState, v cty.Value, key string) {
				if v.Type() == cty.String {
					s.reservation = v.AsString()
					s.hasReservation = true
					s.observedKeys = append(s.observedKeys, key)
				}
			},
		},
		{
			baseName: "enable_spot_vm",
			handler: func(s *provisioningState, v cty.Value, key string) {
				if v.Type() == cty.Bool {
					s.spot = v.True()
					s.spotPresent = true
					s.observedKeys = append(s.observedKeys, key)
				}
			},
		},
		{
			baseName: "dws_flex_enabled",
			handler: func(s *provisioningState, v cty.Value, key string) {
				if v.Type() == cty.Bool {
					s.dws = v.True()
					s.dwsPresent = true
					s.observedKeys = append(s.observedKeys, key)
				}
			},
		},
	}
}

// scanItemsIntoPrefixes scans the provided items map and fills the prefixes map.
func scanItemsIntoPrefixes(items map[string]cty.Value, prefixes map[string]*provisioningState, defs []provKeyDef) {
	getState := func(prefix string) *provisioningState {
		if s, ok := prefixes[prefix]; ok {
			return s
		}
		s := &provisioningState{}
		prefixes[prefix] = s
		return s
	}
	for k, v := range items {
		if v.IsNull() || !v.IsKnown() {
			continue
		}
		low := strings.ToLower(k)
		for _, d := range defs {
			if low == d.baseName || strings.HasSuffix(low, "_"+d.baseName) {
				prefix := ""
				if low != d.baseName {
					prefix = strings.TrimSuffix(k, "_"+d.baseName)
				}
				s := getState(prefix)
				d.handler(s, v, k)
				break
			}
		}
	}
}

// evaluateProvisioningPrefix inspects a single prefix state and returns a multi-line message if invalid.
func evaluateProvisioningPrefix(prefix string, s *provisioningState) string {
	if !s.hasReservation && !s.spotPresent && !s.dwsPresent {
		return ""
	}
	selectedCount := 0
	selectedList := []string{}
	if s.hasReservation && strings.TrimSpace(s.reservation) != "" {
		selectedCount++
		selectedList = append(selectedList, "reservation")
	}
	if s.spotPresent && s.spot {
		selectedCount++
		selectedList = append(selectedList, "spot_vm")
	}
	if s.dwsPresent && s.dws {
		selectedCount++
		selectedList = append(selectedList, "dws_flex")
	}

	label := prefix
	if label == "" {
		label = "<root>"
	}

	if selectedCount == 0 {
		details := formatProvisioningSettings(s, false)
		required := []string{
			fmt.Sprintf("%s_reservation_name (non-empty string)", prefix),
			fmt.Sprintf("%s_enable_spot_vm (boolean true)", prefix),
			fmt.Sprintf("%s_dws_flex_enabled (boolean true)", prefix),
		}
		return buildProvisioningMessage("Provisioning model not selected", label, details, required)
	}

	if selectedCount > 1 {
		details := formatProvisioningSettings(s, true)
		required := []string{
			fmt.Sprintf("%s_reservation_name (non-empty string)", prefix),
			fmt.Sprintf("%s_enable_spot_vm (boolean true)", prefix),
			fmt.Sprintf("%s_dws_flex_enabled (boolean true)", prefix),
		}
		title := fmt.Sprintf("Provisioning conflict (selected: %v)", selectedList)
		// Example removed to reduce message complexity as requested.
		return buildProvisioningMessage(title, label, details, required)
	}
	return ""
}

func buildProvisioningMessage(title, label, details string, required []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s for prefix %q.\n", title, label))
	if details != "" {
		b.WriteString("Observed variables:\n")
		// details already includes newline-prefixed list entries
		b.WriteString(details)
		b.WriteString("\n")
	}
	if len(required) > 0 {
		b.WriteString("\nRequired: choose exactly one of:\n")
		for _, r := range required {
			b.WriteString(fmt.Sprintf("  - %s\n", r))
		}
	}
	return b.String()
}

func checkSlurmProvisioning(bp config.Blueprint) error {
	prefixes := map[string]*provisioningState{}
	defs := provisioningKeyDefs()

	// scan blueprint-level vars and module-level settings
	scanItemsIntoPrefixes(bp.Vars.Items(), prefixes, defs)
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			if mod.Settings.Items() != nil {
				scanItemsIntoPrefixes(mod.Settings.Items(), prefixes, defs)
			}
		}
	}

	if len(prefixes) == 0 {
		return nil
	}

	var msgs []string
	for prefix, s := range prefixes {
		if msg := evaluateProvisioningPrefix(prefix, s); msg != "" {
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("%s", strings.Join(msgs, "\n\n"))
	}
	return nil
}

func validateSlurmClusterNameValue(v cty.Value, varName string) error {
	// Assume caller ensures v is known & non-null; still check types here.
	if v.Type() != cty.String {
		return fmt.Errorf("variable '%s' must be a string", varName)
	}
	name := v.AsString()
	if !slurmClusterNameRegex.MatchString(name) {
		return fmt.Errorf("variable '%s' ('%s') must match regex '^[a-z](?:[a-z0-9]{0,9})$' (lowercase, 1-10 chars, alpha first, no hyphens)", varName, name)
	}
	return nil
}

// checkSlurmClusterName ensures slurm_cluster_name follows regex.
func checkSlurmClusterName(bp config.Blueprint) error {
	// blueprint-level var first
	if bp.Vars.Has("slurm_cluster_name") {
		v := bp.Vars.Get("slurm_cluster_name")
		if !v.IsNull() && v.IsKnown() {
			if err := validateSlurmClusterNameValue(v, "slurm_cluster_name"); err != nil {
				return err
			}
			return nil
		}
	}
	// fall back to scanning module settings
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			for k, v := range mod.Settings.Items() {
				low := strings.ToLower(k)
				if !(low == "slurm_cluster_name" || strings.HasSuffix(low, "_slurm_cluster_name")) {
					continue
				}
				if v.IsNull() || !v.IsKnown() {
					continue
				}
				if err := validateSlurmClusterNameValue(v, k); err != nil {
					return err
				}
				return nil
			}
		}
	}
	return nil
}
