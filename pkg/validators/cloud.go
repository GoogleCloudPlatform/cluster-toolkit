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
	"context"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"regexp"
	"strings"

	"golang.org/x/exp/maps"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

var reservationNameRegex = regexp.MustCompile(`^projects/([^/]+)/reservations/([^/]+)$`)
var resKeyRegex = regexp.MustCompile(`^(.*_)?reservation(_name)?$`)

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
		err := validateMachineTypeInZone(s, projectID, globalZone, explicitMachineType, validatorName)

		// Catch the sentinel and return nil so the deployment proceeds
		if errors.Is(err, errSoftWarning) {
			return nil
		}
		return err
	}

	return validateSettingsInModules(bp, globalZone, projectID, "machine_type", "machine type", validatorName, func(z, name string, vName string) error {
		return validateMachineTypeInZone(s, projectID, z, name, vName)
	})
}

// findReservationInOtherZones searches for a reservation by name across zones
// in the project.
func findReservationInOtherZones(s *compute.Service, projectID string, name string) ([]string, error) {
	aggList, err := s.Reservations.AggregatedList(projectID).Do()
	if err != nil {
		return nil, err
	}

	foundInZones := []string{}
	for _, scopedList := range aggList.Items {
		for _, res := range scopedList.Reservations {
			if res.Name == name {
				// res.Zone is a full URL, extract just the name (e.g., "us-central1-a")
				parts := strings.Split(res.Zone, "/")
				foundInZones = append(foundInZones, parts[len(parts)-1])
			}
		}
	}
	return foundInZones, nil
}

// TestReservationExists checks if a reservation exists in a project and zone.
func TestReservationExists(ctx context.Context, reservationProjectID string, zone string, reservationName string, deploymentProjectID string) error {
	if reservationName == "" {
		return nil
	}

	s, err := compute.NewService(ctx)
	if err != nil {
		return handleClientError(err)
	}

	// 1. Direct check: Try to Get the specific reservation
	_, err = s.Reservations.Get(reservationProjectID, zone, reservationName).Do()
	if err == nil {
		return nil // Success
	}

	// 2. Access Check: If we can't even reach the project/API, issue soft warning
	if msg, isSoft := getSoftWarningMessage(err, "test_reservation_exists", reservationProjectID, "Compute Engine API", "compute.reservations.get"); isSoft {
		fmt.Println(msg)
		return nil // Skip and continue
	}

	// 3. Diagnostic Search: The reservation was not in the expected zone (404).
	// We try to find where it actually is.
	foundInZones, aggErr := findReservationInOtherZones(s, reservationProjectID, reservationName)

	if aggErr != nil {
		// If Discovery fails (403/400) and it's a SHARED project, we must skip
		// because we can't prove the user has a typo; we just can't list resources.
		if reservationProjectID != deploymentProjectID {
			fmt.Printf("\n[!] WARNING: Shared reservation %q was not found in zone %q.\n", reservationName, zone)
			fmt.Printf("    Discovery in other zones of project %q failed due to restricted permissions: %v\n", reservationProjectID, aggErr)
			fmt.Printf("    Skipping this check as consumption may still be possible.\n")
			return nil
		}

		// For Local Project: If List fails, we report the original 404 but note the permission issue.
		var gerr *googleapi.Error
		if errors.As(aggErr, &gerr) && (gerr.Code == 403 || gerr.Code == 400) {
			return fmt.Errorf("reservation %q not found in zone %q (Note: identity lacks permission to search other zones)", reservationName, zone)
		}
		return fmt.Errorf("reservation %q not found in project %q and zone %q", reservationName, reservationProjectID, zone)
	}

	// 4. Resource Found Discovery: If we found it elsewhere, provide a Hard Failure with Hint.
	if len(foundInZones) > 0 {
		zonesList := strings.Join(foundInZones, ", ")
		return config.HintError{
			Err: fmt.Errorf("reservation %q exists in project %q, but in zone(s) [%s] instead of %q",
				reservationName, reservationProjectID, zonesList, zone),
			Hint: fmt.Sprintf("Change the zone in your blueprint to one of [%s], or use a reservation that is located in zone %q.",
				zonesList, zone),
		}
	}

	// 5. Not Found Anywhere: Hard Failure
	return fmt.Errorf("reservation %q was not found in any zone of project %q", reservationName, reservationProjectID)
}

func testReservationExists(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{"project_id", "zone", "reservation_name"}); err != nil {
		return err
	}
	inputMap, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}

	// The primary project defined in the blueprint vars
	deploymentProjectID := inputMap["project_id"]
	zone := inputMap["zone"]
	resInput := inputMap["reservation_name"]

	if resInput == "" {
		return nil
	}

	// Handle hierarchical formats
	resInput = strings.Split(resInput, "/reservationBlocks/")[0]

	// Determine if it's a Shared Reservation path or a simple name
	matches := reservationNameRegex.FindStringSubmatch(resInput)
	reservationProjectID := deploymentProjectID
	targetName := resInput

	if len(matches) == 3 {
		// The input is a full resource path, indicating a shared reservation.
		// Use the project ID extracted from the path instead of the deployment project.
		reservationProjectID = matches[1]
		targetName = matches[2]
	}

	// Pass both the owner project and the deployment project
	ctx := context.Background()
	return TestReservationExists(ctx, reservationProjectID, zone, targetName, deploymentProjectID)
}

func testDiskTypeInZoneAvailability(bp config.Blueprint, inputs config.Dict) error {
	// 1. Determine if the validator was explicitly added to the blueprint YAML
	const validatorName = "test_disk_type_in_zone"
	required := []string{"project_id", "zone"}
	if isValidatorExplicit(bp, validatorName) {
		required = append(required, "disk_type")
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

	projectID, globalZone, explicitDiskType := m["project_id"], m["zone"], m["disk_type"]

	if explicitDiskType != "" {
		// When explicitly called, we MUST validate the zone provided in the inputs
		if err := TestZoneExists(projectID, globalZone); err != nil {
			return err
		}
		err := validateDiskTypeInZone(s, projectID, globalZone, explicitDiskType, validatorName)

		// Catch the sentinel and return nil so the deployment proceeds
		if errors.Is(err, errSoftWarning) {
			return nil
		}
		return err
	}

	return validateSettingsInModules(bp, globalZone, projectID, "disk_type", "disk type", validatorName, func(z, name string, vName string) error {
		return validateDiskTypeInZone(s, projectID, z, name, vName)
	})
}
