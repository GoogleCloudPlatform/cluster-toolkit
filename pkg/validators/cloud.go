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

// checkMachineType checks if a machine type exists in a specific zone using an existing service client.
func validateMachineTypeInZone(s *compute.Service, projectID string, zone string, machineType string) error {
	_, err := s.MachineTypes.Get(projectID, zone, machineType).Do()

	// SUCCESS PATH: The machine type exists in the specified zone.
	if err == nil {
		return nil
	}

	// 2. SOFT WARNING PATH: Check for Permission (403) or Disabled API (400)
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == 403 || gerr.Code == 400 {
			// Print the warning to the console and return nil to "skip"
			fmt.Printf("Warning: validator \"test_all_machine_types\" skipped: could not access Compute Engine API for project %q (%v)\n", projectID, gerr.Message)
			fmt.Println("Check IAM permissions (compute.machineTypes.get) and ensure the API is enabled to use this validator.")
			return nil
		}
	}

	return fmt.Errorf("machine type %q is not available in zone %q for project %q", machineType, zone, projectID)
}

func testMachineTypeAvailability(bp config.Blueprint, inputs config.Dict) error {
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}
	projectID := m["project_id"]
	globalZone := m["zone"]

	if projectID == "" {
		return fmt.Errorf("machine type validation requires a project_id")
	}

	// 0. Create Service
	ctx := context.Background()
	s, err := compute.NewService(ctx)
	if err != nil {
		return handleClientError(err)
	}

	errs := config.Errors{}

	// 2. Walk through every module in the blueprint
	bp.WalkModulesSafe(func(path config.ModulePath, mod *config.Module) {
		targetZone := globalZone
		zVal := mod.Settings.Get("zone")

		if resolvedZone, err := bp.Eval(zVal); err == nil {
			zVal = resolvedZone
		}

		if zVal != cty.NilVal && !zVal.IsNull() && zVal.Type() == cty.String {
			targetZone = zVal.AsString()
		}

		if targetZone == "" {
			return
		}

		// 2. Iterate over settings to find any that look like a machine type
		for key, val := range mod.Settings.Items() {
			// Catch "machine_type", "manager_machine_type", "system_node_pool_machine_type", etc.
			if key != "machine_type" && !strings.HasSuffix(key, "_machine_type") {
				continue
			}

			// Validate the value
			mtVal := val
			if resolved, err := bp.Eval(mtVal); err == nil {
				mtVal = resolved
			}

			if mtVal == cty.NilVal || mtVal.IsNull() || mtVal.Type() != cty.String {
				continue
			}
			machineType := mtVal.AsString()

			// 3. Run the validation
			if err := validateMachineTypeInZone(s, projectID, targetZone, machineType); err != nil {
				// We add the key to the error message so the user knows WHICH setting failed
				errs.Add(fmt.Errorf("In module '%s' setting '%s': %w", mod.ID, key, err))
			}
		}
	})

	return errs.OrNil()
}
