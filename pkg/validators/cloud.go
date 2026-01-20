// Copyright 2023 "Google LLC"
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
	"context"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

func getErrorReason(err googleapi.Error) (string, map[string]interface{}) {
	for _, d := range err.Details {
		m, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		if reason, ok := m["reason"].(string); ok {
			return reason, m["metadata"].(map[string]interface{})
		}
	}
	return "", nil
}

func newDisabledServiceError(title string, name string, pid string) error {
	return config.HintError{
		Hint: fmt.Sprintf("%s can be enabled at https://console.cloud.google.com/apis/library/%s?project=%s", title, name, pid),
		Err:  fmt.Errorf("%s service is disabled in project %s", title, pid)}
}

func handleServiceUsageError(err error, pid string) error {
	if err == nil {
		return nil
	}

	var herr *googleapi.Error
	if !errors.As(err, &herr) {
		return fmt.Errorf("unhandled error: %s", err)
	}

	reason, metadata := getErrorReason(*herr)
	switch reason {
	case "SERVICE_DISABLED":
		return newDisabledServiceError("Service Usage API", "serviceusage.googleapis.com", pid)
	case "SERVICE_CONFIG_NOT_FOUND_OR_PERMISSION_DENIED":
		return fmt.Errorf("service %s does not exist in project %s", metadata["services"], pid)
	case "USER_PROJECT_DENIED":
		return projectError(pid)
	case "SU_MISSING_NAMES":
		return nil // occurs if API list is empty and 0 APIs to validate
	}
	return fmt.Errorf("unhandled error: %s", herr)
}

func isValidatorExplicit(bp config.Blueprint, validatorName string) bool {
	for _, v := range bp.Validators {
		if v.Validator == validatorName {
			return true
		}
	}
	return false
}

// TestApisEnabled tests whether APIs are enabled in given project
func TestApisEnabled(projectID string, requiredAPIs []string) error {
	// can return immediately if there are 0 APIs to test
	if len(requiredAPIs) == 0 {
		return nil
	}

	ctx := context.Background()

	s, err := serviceusage.NewService(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		return handleClientError(err)
	}

	prefix := "projects/" + projectID
	var serviceNames []string
	for _, api := range requiredAPIs {
		serviceNames = append(serviceNames, prefix+"/services/"+api)
	}

	resp, err := s.Services.BatchGet(prefix).Names(serviceNames...).Do()
	if err != nil {
		return handleServiceUsageError(err, projectID)
	}
	errs := config.Errors{}
	for _, service := range resp.Services {
		if service.State == "DISABLED" {
			errs.Add(newDisabledServiceError(service.Config.Title, service.Config.Name, projectID))
		}
	}
	return errs.OrNil()
}

// TestProjectExists whether projectID exists / is accessible with credentials
func TestProjectExists(projectID string) error {
	ctx := context.Background()
	s, err := compute.NewService(ctx)
	if err != nil {
		err = handleClientError(err)
		return err
	}
	_, err = s.Projects.Get(projectID).Fields().Do()
	if err != nil {
		if strings.Contains(err.Error(), "Compute Engine API has not been used in project") {
			return newDisabledServiceError("Compute Engine API", "compute.googleapis.com", projectID)
		}
		return projectError(projectID)
	}

	return nil
}

func getRegion(projectID string, region string) (*compute.Region, error) {
	ctx := context.Background()
	s, err := compute.NewService(ctx)
	if err != nil {
		err = handleClientError(err)
		return nil, err
	}
	return s.Regions.Get(projectID, region).Do()
}

// TestRegionExists whether region exists / is accessible with credentials
func TestRegionExists(projectID string, region string) error {
	_, err := getRegion(projectID, region)
	if err != nil {
		return fmt.Errorf(regionError, region, projectID)
	}
	return nil
}

func getZone(projectID string, zone string) (*compute.Zone, error) {
	ctx := context.Background()
	s, err := compute.NewService(ctx)
	if err != nil {
		err = handleClientError(err)
		return nil, err
	}
	return s.Zones.Get(projectID, zone).Do()
}

// TestZoneExists whether zone exists / is accessible with credentials
func TestZoneExists(projectID string, zone string) error {
	_, err := getZone(projectID, zone)
	if err != nil {
		return fmt.Errorf(zoneError, zone, projectID)
	}
	return nil
}

// TestZoneInRegion whether zone is in region
func TestZoneInRegion(projectID string, zone string, region string) error {
	regionObject, err := getRegion(projectID, region)
	if err != nil {
		return fmt.Errorf(regionError, region, projectID)
	}
	zoneObject, err := getZone(projectID, zone)
	if err != nil {
		return fmt.Errorf(zoneError, zone, projectID)
	}

	if zoneObject.Region != regionObject.SelfLink {
		return fmt.Errorf(zoneInRegionError, zone, region, projectID)
	}

	return nil
}

func testApisEnabled(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{"project_id"}); err != nil {
		return err
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}
	apis := map[string]bool{}
	bp.WalkModulesSafe(func(_ config.ModulePath, m *config.Module) {
		services := m.InfoOrDie().Metadata.Spec.Requirements.Services
		for _, api := range services {
			apis[api] = true
		}
	})
	return TestApisEnabled(m["project_id"], maps.Keys(apis))
}

func testProjectExists(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{"project_id"}); err != nil {
		return err
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}
	return TestProjectExists(m["project_id"])
}

func testRegionExists(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{"project_id", "region"}); err != nil {
		return err
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}
	return TestRegionExists(m["project_id"], m["region"])
}

func testZoneExists(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{"project_id", "zone"}); err != nil {
		return err
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}
	return TestZoneExists(m["project_id"], m["zone"])
}

func testZoneInRegion(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{"project_id", "region", "zone"}); err != nil {
		return err
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}
	return TestZoneInRegion(m["project_id"], m["zone"], m["region"])
}

// errSoftWarning is a private sentinel error used to signal that a soft warning
// was triggered and discovery should stop immediately to avoid console spam.
var errSoftWarning = errors.New("abort")

// handleSoftWarning checks if a Google Cloud API error represents a permission issue (403)
// or a disabled API (400). When these occur, it prints a warning to the console
// and returns true, signaling the validator to "skip" the check rather than failing the deployment.
func handleSoftWarning(err error, validatorName, projectID, apiName, permission string) bool {
	var gerr *googleapi.Error
	// 403 = Forbidden (Permission missing), 400 = Bad Request (API often disabled)
	if errors.As(err, &gerr) && (gerr.Code == 403 || gerr.Code == 400) {
		fmt.Printf("\n[!] WARNING (%d): validator %q for project %q. Identity lacks permissions to verify the resource. Skipping this check.\n", gerr.Code, validatorName, projectID)
		fmt.Printf("    Hint: It is possible that the %s is disabled or you do not have IAM permissions (%s).\n Please ensure the API is enabled and check your permissions.\n\n", apiName, permission)
		return true
	}
	return false
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

func validateSettingsInModules(
	blueprint config.Blueprint,
	globalZone string,
	projectID string,
	suffix string,
	validateResource func(zone string, name string) error,
) error {
	validationErrors := config.Errors{}
	// Anti-Spam Logic: The aborted flag ensures that once a soft warning is hit,
	// we stop processing any more modules or settings.
	var aborted bool

	blueprint.WalkModulesSafe(func(path config.ModulePath, module *config.Module) {
		// Exit immediately if we hit a soft warning in a previous module
		if aborted {
			return
		}
		targetZone := globalZone
		moduleZone := resolveStringSetting(blueprint, module.Settings.Get("zone"))

		// If module overrides global zone, validate the override as a hard error
		if moduleZone != "" && moduleZone != globalZone {
			targetZone = moduleZone
			if err := TestZoneExists(projectID, targetZone); err != nil {
				validationErrors.Add(fmt.Errorf("in module %q: %w", module.ID, err))
				return
			}
		}

		if targetZone == "" {
			return
		}

		for key, val := range module.Settings.Items() {
			if aborted || !strings.HasSuffix(key, suffix) {
				continue
			}

			if resourceName := resolveStringSetting(blueprint, val); resourceName != "" {
				err := validateResource(targetZone, resourceName)

				// Short-circuit on soft warning to prevent duplicate console spam.
				if errors.Is(err, errSoftWarning) {
					aborted = true
					return
				}

				if err != nil {
					validationErrors.Add(fmt.Errorf("in module %q setting %q: %w", module.ID, key, err))
				}
			}
		}
	})

	return validationErrors.OrNil()
}

// validateMachineTypeInZone calls the Compute Engine API to verify if a specific
// machine type is available in the given zone and project.
func validateMachineTypeInZone(s *compute.Service, projectID, zone, machineType string) error {
	_, err := s.MachineTypes.Get(projectID, zone, machineType).Do()

	// Case 1: Success - The machine type exists
	if err == nil {
		return nil
	}

	// Case 2: Environmental Issue - API disabled or permissions missing (Soft Warning)
	if handleSoftWarning(err, "test_machine_type_in_zone", projectID, "Compute Engine API", "compute.machineTypes.get") {
		return errSoftWarning
	}

	// Case 3: Validation Failure - The machine type is genuinely invalid or unavailable
	return fmt.Errorf("machine type %q is not available in zone %q for project %q", machineType, zone, projectID)
}

func testMachineTypeInZoneAvailability(bp config.Blueprint, inputs config.Dict) error {
	// 1. Determine if the validator was explicitly added to the blueprint YAML
	const validatorName = "test_machine_type_in_zone"
	required := []string{"project_id", "zone"}
	if isValidatorExplicit(bp, validatorName) {
		required = append(required, "machine_type")
	}

	if err := checkInputs(inputs, required); err != nil {
		return err
	}

	s, err := compute.NewService(context.Background())
	if err != nil {
		return handleClientError(err)
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}

	projectID, globalZone, explicitMachineType := m["project_id"], m["zone"], m["machine_type"]

	if explicitMachineType != "" {
		// When explicitly called, we MUST validate the zone provided in the inputs
		if err := TestZoneExists(projectID, globalZone); err != nil {
			return err
		}
		err := validateMachineTypeInZone(s, projectID, globalZone, explicitMachineType)

		// Catch the sentinel and return nil so the deployment proceeds
		if errors.Is(err, errSoftWarning) {
			return nil
		}
		return err
	}

	return validateSettingsInModules(bp, globalZone, projectID, "machine_type", func(z, name string) error {
		return validateMachineTypeInZone(s, projectID, z, name)
	})
}
