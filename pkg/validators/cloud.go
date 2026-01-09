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

// handleAPIError catches specific Google API errors (403, 400) to provide
// actionable hard errors that block deployment.
func handleAPIError(err error, resourceName string, projectID string) error {
	if err == nil {
		return nil
	}

	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		switch gerr.Code {
		case 403:
			// Project-level/IAM issue: fail hard.
			return fmt.Errorf("insufficient permissions to verify %s in project %q (or project does not exist): %w", resourceName, projectID, err)
		case 404:
			// Resource not found: return raw error to trigger calling function's diagnostics.
			return err
		}
	}

	return err
}

// TestReservationExists checks if a reservation exists in a project and zone.
// It uses TestProjectExists for diagnosis if the specific reservation is missing.
func TestReservationExists(projectID string, zone string, reservationName string) error {
	if reservationName == "" {
		return nil
	}

	ctx := context.Background()
	s, err := compute.NewService(ctx)
	if err != nil {
		return handleClientError(err)
	}

	// 1. Direct check: Try to get the specific reservation in the specific zone (Fast Path)
	_, err = s.Reservations.Get(projectID, zone, reservationName).Do()
	if err == nil {
		return nil // Success: Found exactly where expected
	}

	// ERROR PATH: Handle Hard Failures (403 Permission / 400 Disabled API)
	// We only reach this point if err != nil.
	apiErr := handleAPIError(err, fmt.Sprintf("reservation %q", reservationName), projectID)

	var gerr *googleapi.Error
	if errors.As(apiErr, &gerr) && gerr.Code != 404 {
		return apiErr
	}

	// 2. Diagnostic Path: The Get failed.
	// Before searching other zones, check if the project itself is reachable.
	// Reusing TestProjectExists provides the standardized error/hint for missing projects.
	if pErr := TestProjectExists(projectID); pErr != nil {
		return pErr
	}

	// 3. Project is valid. Let's find out if the reservation exists in another zone.
	aggList, aggErr := s.Reservations.AggregatedList(projectID).Do()
	if aggErr != nil {
		// If listing fails after project exists, it's likely a permission/API issue.
		return fmt.Errorf("reservation %q not found in project %q and zone %q", reservationName, projectID, zone)
	}

	foundInZones := []string{}
	for _, scopedList := range aggList.Items {
		for _, res := range scopedList.Reservations {
			if res.Name == reservationName {
				// res.Zone is a full URL, extract just the name (e.g., "us-central1-a")
				parts := strings.Split(res.Zone, "/")
				foundInZones = append(foundInZones, parts[len(parts)-1])
			}
		}
	}

	// 4. Return helpful zone hint or general not-found
	if len(foundInZones) > 0 {
		return config.HintError{
			Err: fmt.Errorf("reservation %q exists in project %q, but in zone(s) %s instead of %q",
				reservationName, projectID, strings.Join(foundInZones, ", "), zone),
			Hint: fmt.Sprintf("The blueprint is configured for %q. Change your zone or use a reservation located in %q.", zone, zone),
		}
	}

	// Found project, but name exists in NO zone.
	return fmt.Errorf("reservation %q was not found in any zone of project %q", reservationName, projectID)
}

// // Wrapper for the blueprint validator logic
func testReservationExists(bp config.Blueprint, inputs config.Dict) error {
	// 1. Get inputs
	if err := checkInputs(inputs, []string{"project_id", "zone", "reservation_name"}); err != nil {
		return err
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}

	projectID := m["project_id"]
	zone := m["zone"]
	resInput := m["reservation_name"]

	if resInput == "" {
		return nil // Skip validation if no reservation is provided
	}

	// Handle hierarchical formats (e.g., <name>/reservationBlocks/<block>)
	// by stripping the block suffix.
	resInput = strings.Split(resInput, "/reservationBlocks/")[0]

	// 2. Determine if it's a Shared Reservation (Resource Path) or Local (Simple Name)
	// Regex matches: projects/{PROJECT}/reservations/{NAME}
	matches := reservationNameRegex.FindStringSubmatch(resInput)

	targetProject := projectID
	targetName := resInput

	if len(matches) == 3 {
		// It's a shared reservation path
		targetProject = matches[1]
		targetName = matches[2]
	}
	return TestReservationExists(targetProject, zone, targetName)
}
