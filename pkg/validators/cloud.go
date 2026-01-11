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
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
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

// TestIAMPolicyBindingExists checks for a specific IAM binding, resolving project numbers automatically.
func TestIAMPolicyBindingExists(hostProjectID string, serviceProjectID string, role string) error {
	ctx := context.Background()
	s, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return handleClientError(err)
	}

	// 1. Resolve Service Project Number to construct the member string
	// This makes it generic so you don't need to hardcode the project number.
	project, err := s.Projects.Get(serviceProjectID).Do()
	if err != nil {
		return fmt.Errorf("failed to get service project %s: %v", serviceProjectID, handleClientError(err))
	}
	member := fmt.Sprintf("serviceAccount:%d-compute@developer.gserviceaccount.com", project.ProjectNumber)

	// 2. Fetch IAM policy for the Host Project
	policy, err := s.Projects.GetIamPolicy(hostProjectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for host project %s: %v", hostProjectID, handleClientError(err))
	}

	// 3. Verify the binding exists
	for _, binding := range policy.Bindings {
		if binding.Role == role {
			for _, m := range binding.Members {
				if m == member {
					return nil // Success: Binding confirmed
				}
			}
		}
	}

	return fmt.Errorf("IAM binding for role %q and member %q not found in project %q", role, member, hostProjectID)
}

// testIAMPolicyBindingExists extracts inputs and only runs if a shared reservation is detected
func testIAMPolicyBindingExists(bp config.Blueprint, inputs config.Dict) error {
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}

	resName := m["reservation_name"]

	// Handle conditional execution: Only run if it's a shared reservation path.
	// Shared reservation format: "projects/<project-id>/reservations/<name>"
	// Normal reservation format: "reservation-name"
	if !strings.HasPrefix(resName, "projects/") {
		fmt.Printf("skipping IAM binding check: %q is not a shared reservation path", resName)
		return nil // Success: Skip validation for local reservations
	}

	// 1. Extract Host Project ID from the shared reservation path
	re := regexp.MustCompile(`^projects/([^/]+)/`)
	matches := re.FindStringSubmatch(resName)
	if len(matches) <= 1 {
		fmt.Printf("skipping IAM binding check: could not parse project ID from %q", resName)
		return nil
	}
	hostProjectID := matches[1]

	// 2. Extract other required fields
	serviceProjectID := m["service_project_id"]
	role := m["role"]

	if serviceProjectID == "" || role == "" {
		return fmt.Errorf("validator %s requires 'service_project_id' and 'role'", "test_iam_policy_binding_exists")
	}

	// 3. Invoke core validation logic
	return TestIAMPolicyBindingExists(hostProjectID, serviceProjectID, role)
}

// getFloat64 safely converts a cty.Value to a float64, handling marks and strings
func getFloat64(v cty.Value) (float64, bool) {
	unmarked, _ := v.Unmark()
	if unmarked.IsNull() || !unmarked.IsKnown() {
		return 0, false
	}

	if unmarked.Type() == cty.Number {
		f, _ := unmarked.AsBigFloat().Float64()
		return f, true
	}

	if unmarked.Type() == cty.String {
		f, err := strconv.ParseFloat(unmarked.AsString(), 64)
		if err == nil {
			return f, true
		}
	}

	return 0, false
}

// TestQuotaAvailability queries GCP regional quotas and compares them against requested amounts.
func TestQuotaAvailability(projectID string, region string, requested map[string]float64) error {
	ctx := context.Background()
	s, err := compute.NewService(ctx)
	if err != nil {
		return handleClientError(err)
	}

	reg, err := s.Regions.Get(projectID, region).Do()
	if err != nil {
		return fmt.Errorf("failed to get quotas for region %s: %v", region, err)
	}

	for metric, reqValue := range requested {
		for _, q := range reg.Quotas {
			if q.Metric == metric {
				available := q.Limit - q.Usage
				if reqValue > available {
					return fmt.Errorf("Insufficient quota for %s in %s. Requested: %.0f, Available: %.0f (Limit: %.0f, Usage: %.0f)",
						metric, region, reqValue, available, q.Limit, q.Usage)
				}
				break
			}
		}
	}
	return nil
}

// testQuotaAvailability aggregates resources based on validator inputs and blueprint modules.
func testQuotaAvailability(bp config.Blueprint, inputs config.Dict) error {
	projectIDVal := inputs.Get("project_id")
	regionVal := inputs.Get("region")

	if projectIDVal.IsNull() || regionVal.IsNull() {
		return fmt.Errorf("validator %s requires 'project_id' and 'region'", "test_quota_availability")
	}

	unmarkedPID, _ := projectIDVal.Unmark()
	unmarkedReg, _ := regionVal.Unmark()
	projectID := unmarkedPID.AsString()
	region := unmarkedReg.AsString()

	// Extract overrides from validator inputs
	unmarkedMT, _ := inputs.Get("machine_type").Unmark()
	inputMT := ""
	if !unmarkedMT.IsNull() && unmarkedMT.Type() == cty.String {
		inputMT = unmarkedMT.AsString()
	}

	inputCount, _ := getFloat64(inputs.Get("cluster_size"))
	inputDiskSize, _ := getFloat64(inputs.Get("disk_size_gb"))

	requested := make(map[string]float64)
	ctx := context.Background()
	s, err := compute.NewService(ctx)
	if err != nil {
		return handleClientError(err)
	}

	bp.WalkModulesSafe(func(_ config.ModulePath, mod *config.Module) {
		settings := mod.Settings.Items()

		// 1. Instance Count
		count := 1.0
		foundCount := false
		for _, key := range []string{"cluster_size", "node_count", "instance_count", "count"} {
			if val, ok := settings[key]; ok {
				if f, ok := getFloat64(val); ok && f > 0 {
					count = f
					foundCount = true
					break
				}
			}
		}
		if !foundCount && inputCount > 0 {
			count = inputCount
		}

		// 2. Machine Type (CPUs/GPUs) - Priority: Input > Module
		mtName := inputMT
		if mtName == "" {
			if mtVal, ok := settings["machine_type"]; ok && !mtVal.IsNull() {
				u, _ := mtVal.Unmark()
				if u.Type() == cty.String {
					mtName = u.AsString()
				}
			}
		}

		if mtName != "" {
			zone := region + "-a"
			mt, err := s.MachineTypes.Get(projectID, zone, mtName).Do()
			if err == nil {
				requested["CPUS"] += float64(mt.GuestCpus) * count
				for _, acc := range mt.Accelerators {
					metric := strings.ToUpper(acc.GuestAcceleratorType) + "_GPUS"
					requested[metric] += float64(acc.GuestAcceleratorCount) * count
				}
			}
		}

		// 3. Disks - Priority: Input > Module
		moduleDiskSize := 0.0
		for _, key := range []string{"disk_size_gb", "boot_disk_size_gb"} {
			if val, ok := settings[key]; ok {
				if f, ok := getFloat64(val); ok {
					moduleDiskSize += f
				}
			}
		}
		if moduleDiskSize == 0 && inputDiskSize > 0 {
			moduleDiskSize = inputDiskSize
		}
		requested["DISKS_TOTAL_GB"] += moduleDiskSize * count
	})

	return TestQuotaAvailability(projectID, region, requested)
}
