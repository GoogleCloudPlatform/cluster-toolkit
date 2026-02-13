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
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"strings"

	"github.com/zclconf/go-cty/cty"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

// errSoftWarning is a private sentinel error used to signal that a soft warning
// was triggered and discovery should stop immediately to avoid console spam.
var errSoftWarning = errors.New("abort")

// getSoftWarningMessage checks if a Google Cloud API error represents a permission issue (403)
// or a disabled API (400). When these occur, it prints a warning to the console
// and returns true, signaling the validator to "skip" the check rather than failing the deployment.
func getSoftWarningMessage(err error, validatorName, projectID, apiName, permission string) (string, bool) {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		// 403 is always a Soft Warning (Permission)
		is403 := gerr.Code == 403

		// 400 is ONLY a Soft Warning if it's about the API not being enabled/used.
		// If it's an "Invalid Value" (like your custom machine error), it should be a Hard Failure.
		isAPIOff := gerr.Code == 400 && (strings.Contains(strings.ToLower(gerr.Message), "not enabled") ||
			strings.Contains(strings.ToLower(gerr.Message), "not been used"))

		if is403 || isAPIOff {
			msg := fmt.Sprintf("\n[!] WARNING (%d): validator %q for project %q. Identity lacks permissions. Skipping check.\n", gerr.Code, validatorName, projectID)
			msg += fmt.Sprintf("    Hint: Ensure %s is enabled and check IAM permissions (%s).\n", apiName, permission)
			return msg, true
		}
	}
	return "", false
}

// 1. Helper for cty resolution
func resolveStringSetting(bp config.Blueprint, val cty.Value) string {
	v := val
	if resolved, err := bp.Eval(v); err == nil {
		v = resolved
	}
	if v != cty.NilVal && !v.IsNull() && v.Type() == cty.String {
		return v.AsString()
	}
	return ""
}

// extractZones converts a cty.Value (String, List, Set, or Tuple) into a string slice.
// This helper removes complex type-checking branches from the main resolution logic.
func extractZones(val cty.Value) ([]string, error) {
	if val.IsNull() || !val.IsKnown() {
		return nil, nil
	}

	var zones []string
	// evaluateAndFlatten handles single strings, lists, and tuples for us
	for _, v := range evaluateAndFlatten(val) {
		if v.Type() == cty.String {
			zones = append(zones, v.AsString())
		} else {
			// Reuse toolkit standard error generation for non-string types
			_, err := inputsAsStrings(config.NewDict(map[string]cty.Value{"zone": v}))
			return nil, err
		}
	}
	return zones, nil
}

// resolveZones identifies all target zones for a module by scanning its settings.
// It implements a priority system to ensure that specific module placements
// (like 'gpu_zones' or 'zones') take precedence over the inherited singular 'zone' variable.
func resolveZones(blueprint config.Blueprint, module *config.Module, globalZone string) ([]string, error) {
	plural, singular := make(map[string]bool), make(map[string]bool)

	for key, val := range module.Settings.Items() {
		// Identify settings ending in 'zone' or 'zones'.
		// strings.HasSuffix also matches the words "zone" and "zones" themselves.
		isPlural := strings.HasSuffix(key, "zones")
		if !isPlural && !strings.HasSuffix(key, "zone") {
			continue
		}

		resolved, err := blueprint.Eval(val)
		if err != nil {
			continue
		}

		// Normalize the value (handling single strings vs. lists) via extractZones
		// and categorize the result based on the key suffix.
		zones, err := extractZones(resolved)
		if err != nil {
			return nil, err // Return type error immediately
		}

		for _, z := range zones {
			if isPlural {
				plural[z] = true
			} else {
				singular[z] = true
			}
		}
	}

	// Priority: 1. Plural zone list, 2. Singular zone override, 3. Global default
	source := plural
	if len(plural) == 0 {
		source = singular
	}
	if len(source) == 0 {
		return []string{globalZone}, nil
	}

	zones := make([]string, 0, len(source))
	for z := range source {
		zones = append(zones, z)
	}
	return zones, nil
}

// checkResourceInZones implements the "OR" logic: passes if found in at least one valid zone.
func checkResourceInZones(projectID string, zones []string, globalZone, resourceLabel, resourceName string, validatorName string, validateFn func(string, string, string) error) (bool, error) {
	var attempted []string
	for _, z := range zones {
		if z == "" {
			continue
		}

		if z != globalZone {
			if err := TestZoneExists(projectID, z); err != nil {
				// Check if the zone-check error is actually a permission issue (403)
				if msg, isSoft := getSoftWarningMessage(err, validatorName, projectID, "Compute Engine API", "compute.zones.get"); isSoft {
					fmt.Println(msg)
					return true, errSoftWarning // Trigger the abort sentinel
				}
				// If it's a real typo (not a 403), return it as a Hard Failure
				return false, err
			}
		}

		attempted = append(attempted, z)
		err := validateFn(z, resourceName, validatorName)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, errSoftWarning) {
			return true, errSoftWarning
		}
	}

	if len(attempted) > 0 {
		return false, fmt.Errorf(config.ErrMsgResourceInAnyZone, resourceLabel, resourceName, strings.Join(attempted, ", "), projectID)
	}
	return true, nil
}

// validateSettingsInModules  walks through every module in the blueprint,
// identifies settings that match a specific suffix (e.g., "machine_type"),
// and validates them against the zones where that module is allowed to reside.
func validateSettingsInModules(blueprint config.Blueprint, globalZone, projectID, suffix, resourceLabel string, validatorName string, validateResource func(zone string, name string, vName string) error) error {
	validationErrors := config.Errors{}
	// Anti-Spam Logic: This flag is set if we encounter an environmental issue
	// (like a 403 Permission Denied). It allows us to stop making slow API calls
	// and stop printing repetitive warnings for the rest of the blueprint walk.
	var aborted bool

	blueprint.WalkModulesSafe(func(path config.ModulePath, module *config.Module) {
		if aborted {
			return
		}

		// Identify which zones this module is targeting.
		// This handles singular overrides, plural lists, and global defaults.
		// Handle the new error return from resolveZones
		targetZones, err := resolveZones(blueprint, module, globalZone)
		if err != nil {
			validationErrors.Add(fmt.Errorf("in module %q: %w", module.ID, err))
			return // Skip discovery for this module due to type error
		}
		for key, val := range module.Settings.Items() {
			if aborted || !strings.HasSuffix(key, suffix) {
				continue
			}

			resourceName := resolveStringSetting(blueprint, val)
			if resourceName == "" {
				continue
			}

			found, err := checkResourceInZones(projectID, targetZones, globalZone, resourceLabel, resourceName, validatorName, validateResource)
			// If we hit the private sentinel error (403/400), set the abort flag.
			if errors.Is(err, errSoftWarning) {
				aborted = true
				return
			}
			if !found && err != nil {
				validationErrors.Add(fmt.Errorf("in module %q setting %q: %w", module.ID, key, err))
			}
		}
	})
	return validationErrors.OrNil()
}

// validateMachineTypeInZone calls the Compute Engine API to verify if a specific
// machine type is available in the given zone and project.
func validateMachineTypeInZone(s *compute.Service, projectID, zone, machineType string, validatorName string) error {
	_, err := s.MachineTypes.Get(projectID, zone, machineType).Do()

	// Case 1: Success - The machine type exists
	if err == nil {
		return nil
	}

	// Case 2: Environmental Issue - API disabled or permissions missing (Soft Warning)
	if msg, isSoft := getSoftWarningMessage(err, validatorName, projectID, "Compute Engine API", "compute.machineTypes.get"); isSoft {
		fmt.Println(msg)
		return errSoftWarning
	}

	// Case 3: Validation Failure - The machine type is genuinely invalid or unavailable
	return fmt.Errorf(config.ErrMsgResourceInZone, "machine type", machineType, zone, projectID)
}

// validateDiskTypeInZone calls the Compute Engine API to verify if a specific
// disk type is available in the given zone and project.
func validateDiskTypeInZone(s *compute.Service, projectID, zone, diskType string, validatorName string) error {
	_, err := s.DiskTypes.Get(projectID, zone, diskType).Do()

	// Case 1: Success - The disk type exists
	if err == nil {
		return nil
	}

	// Case 2: Environmental Issue - API disabled or permissions missing (Soft Warning)
	if msg, isSoft := getSoftWarningMessage(err, validatorName, projectID, "Compute Engine API", "compute.diskTypes.get"); isSoft {
		fmt.Println(msg)
		return errSoftWarning
	}

	// Case 3: Validation Failure - The disk type is genuinely invalid or unavailable
	return fmt.Errorf(config.ErrMsgResourceInZone, "disk type", diskType, zone, projectID)
}
