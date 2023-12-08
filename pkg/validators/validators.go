// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
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
	"golang.org/x/exp/slices"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

const enableAPImsg = "%[1]s: can be enabled at https://console.cloud.google.com/apis/library/%[1]s?project=%[2]s"
const projectError = "project ID %s does not exist or your credentials do not have permission to access it"
const regionError = "region %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneError = "zone %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneInRegionError = "zone %s is not in region %s in project ID %s or your credentials do not have permissions to access it"
const computeDisabledError = "Compute Engine API has not been used in project"
const computeDisabledMsg = "the Compute Engine API must be enabled in project %s to validate blueprint global variables"
const serviceDisabledMsg = "the Service Usage API must be enabled in project %s to validate that all APIs needed by the blueprint are enabled"
const unusedModuleMsg = "module %q uses module %q, but matching setting and outputs were not found. This may be because the value is set explicitly or set by a prior used module"

func handleClientError(e error) error {
	if strings.Contains(e.Error(), "could not find default credentials") {
		return hint(
			fmt.Errorf("could not find application default credentials"),
			"load application default credentials following instructions at https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/README.md#supplying-cloud-credentials-to-terraform")
	}
	return e
}

// TODO: use HintError trait once its implemented
func hint(err error, h string) error {
	return fmt.Errorf("%w\n%s", err, h)
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
		var herr *googleapi.Error
		if !errors.As(err, &herr) {
			return fmt.Errorf("unhandled error: %s", err)
		}
		ok, reason, metadata := getErrorReason(*herr)
		if !ok {
			return fmt.Errorf("unhandled error: %s", err)
		}
		switch reason {
		case "SERVICE_DISABLED":
			return hint(
				fmt.Errorf(serviceDisabledMsg, projectID),
				fmt.Sprintf(enableAPImsg, "serviceusage.googleapis.com", projectID))
		case "SERVICE_CONFIG_NOT_FOUND_OR_PERMISSION_DENIED":
			return fmt.Errorf("service %s does not exist in project %s", metadata["services"], projectID)
		case "USER_PROJECT_DENIED":
			return fmt.Errorf(projectError, projectID)
		case "SU_MISSING_NAMES":
			// occurs if API list is empty and 0 APIs to validate
			return nil
		default:
			return fmt.Errorf("unhandled error: %s", err)
		}
	}

	errs := config.Errors{}
	for _, service := range resp.Services {
		if service.State == "DISABLED" {
			errs.Add(hint(
				fmt.Errorf("%s: service is disabled in project %s", service.Config.Name, projectID),
				fmt.Sprintf(enableAPImsg, service.Config.Name, projectID)))
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
		if strings.Contains(err.Error(), computeDisabledError) {
			errs := config.Errors{}
			return errs.
				Add(hint(
					fmt.Errorf(computeDisabledMsg, projectID),
					fmt.Sprintf(enableAPImsg, "serviceusage.googleapis.com", projectID))).
				Add(hint(
					fmt.Errorf(serviceDisabledMsg, projectID),
					fmt.Sprintf(enableAPImsg, "serviceusage.googleapis.com", projectID)))
		}
		return fmt.Errorf(projectError, projectID)
	}

	return nil
}

func getErrorReason(err googleapi.Error) (bool, string, map[string]interface{}) {
	for _, d := range err.Details {
		m, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		if reason, ok := m["reason"].(string); ok {
			return true, reason, m["metadata"].(map[string]interface{})
		}
	}
	return false, "", nil
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

const (
	testApisEnabledName               = "test_apis_enabled"
	testProjectExistsName             = "test_project_exists"
	testRegionExistsName              = "test_region_exists"
	testZoneExistsName                = "test_zone_exists"
	testZoneInRegionName              = "test_zone_in_region"
	testModuleNotUsedName             = "test_module_not_used"
	testDeploymentVariableNotUsedName = "test_deployment_variable_not_used"
	testResourceRequirementsName      = "test_resource_requirements"
)

func implementations() map[string]func(config.Blueprint, config.Dict) error {
	return map[string]func(config.Blueprint, config.Dict) error{
		testApisEnabledName:               testApisEnabled,
		testProjectExistsName:             testProjectExists,
		testRegionExistsName:              testRegionExists,
		testZoneExistsName:                testZoneExists,
		testZoneInRegionName:              testZoneInRegion,
		testModuleNotUsedName:             testModuleNotUsed,
		testDeploymentVariableNotUsedName: testDeploymentVariableNotUsed,
		testResourceRequirementsName:      testResourceRequirements,
	}
}

// ValidatorError is an error wrapper for errors that occurred during validation
type ValidatorError struct {
	Validator string
	Err       error
}

func (e ValidatorError) Unwrap() error {
	return e.Err
}

func (e ValidatorError) Error() string {
	return fmt.Sprintf("validator %q failed:\n%v", e.Validator, e.Err)
}

// Execute runs all validators on the blueprint
func Execute(bp config.Blueprint) error {
	if bp.ValidationLevel == config.ValidationIgnore {
		return nil
	}
	impl := implementations()
	errs := config.Errors{}
	for iv, v := range validators(bp) {
		p := config.Root.Validators.At(iv)
		if v.Skip {
			continue
		}

		f, ok := impl[v.Validator]
		if !ok {
			errs.At(p.Validator, fmt.Errorf("unknown validator %q", v.Validator))
			continue
		}

		inp, err := v.Inputs.Eval(bp)
		if err != nil {
			errs.At(p.Inputs, err)
			continue
		}

		if err := f(bp, inp); err != nil {
			errs.Add(ValidatorError{v.Validator, err})
			// do not bother running further validators if project ID could not be found
			if v.Validator == "test_project_exists" {
				break
			}
		}
	}
	return errs.OrNil()
}

func checkInputs(inputs config.Dict, required []string) error {
	errs := config.Errors{}
	for _, inp := range required {
		if !inputs.Has(inp) {
			errs.Add(fmt.Errorf("a required input %q was not provided", inp))
		}
	}

	if errs.Any() {
		return errs
	}

	// ensure that no extra inputs were provided by comparing length
	if len(required) != len(inputs.Items()) {
		errStr := "only %v inputs %s should be provided"
		return fmt.Errorf(errStr, len(required), required)
	}

	return nil
}

func testApisEnabled(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{}); err != nil {
		return err
	}
	p, err := bp.ProjectID()
	if err != nil {
		return err
	}
	apis := map[string]bool{}
	bp.WalkModules(func(m *config.Module) error {
		services := m.InfoOrDie().Metadata.Spec.Requirements.Services
		for _, api := range services {
			apis[api] = true
		}
		return nil
	})
	return TestApisEnabled(p, maps.Keys(apis))
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

func testModuleNotUsed(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{}); err != nil {
		return err
	}
	errs := config.Errors{}
	for ig, g := range bp.DeploymentGroups {
		for im, m := range g.Modules {
			ums := m.ListUnusedModules()
			p := config.Root.Groups.At(ig).Modules.At(im).Use

			for iu, u := range m.Use {
				if slices.Contains(ums, u) {
					errs.At(p.At(iu), fmt.Errorf(unusedModuleMsg, m.ID, u))
				}
			}
		}
	}

	return errs.OrNil()
}

func testDeploymentVariableNotUsed(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{}); err != nil {
		return err
	}
	errs := config.Errors{}
	for _, v := range bp.ListUnusedVariables() {
		errs.At(
			config.Root.Vars.Dot(v),
			fmt.Errorf("the variable %q was not used in this blueprint", v))
	}
	return errs.OrNil()
}

// Helper function to sure that all input values are strings.
func inputsAsStrings(inputs config.Dict) (map[string]string, error) {
	ms := map[string]string{}
	for k, v := range inputs.Items() {
		if v.Type() != cty.String {
			return nil, fmt.Errorf("validator inputs must be strings, %s is a %s", k, v.Type())
		}
		ms[k] = v.AsString()
	}
	return ms, nil
}

// Creates a list of default validators for the given blueprint,
// inspect the blueprint for global variables that exist and add an appropriate validators.
func defaults(bp config.Blueprint) []config.Validator {
	projectIDExists := bp.Vars.Has("project_id")
	projectRef := config.GlobalRef("project_id").AsExpression().AsValue()

	regionExists := bp.Vars.Has("region")
	regionRef := config.GlobalRef("region").AsExpression().AsValue()

	zoneExists := bp.Vars.Has("zone")
	zoneRef := config.GlobalRef("zone").AsExpression().AsValue()

	defaults := []config.Validator{
		{Validator: testModuleNotUsedName},
		{Validator: testDeploymentVariableNotUsedName}}

	// always add the project ID validator before subsequent validators that can
	// only succeed if credentials can access the project. If the project ID
	// validator fails, all remaining validators are not executed.
	if projectIDExists {
		defaults = append(defaults, config.Validator{
			Validator: testProjectExistsName,
			Inputs:    config.NewDict(map[string]cty.Value{"project_id": projectRef}),
		})
	}

	// it is safe to run this validator even if vars.project_id is undefined;
	// it will likely fail but will do so helpfully to the user
	defaults = append(defaults,
		config.Validator{Validator: testApisEnabledName})

	if projectIDExists && regionExists {
		defaults = append(defaults, config.Validator{
			Validator: testRegionExistsName,
			Inputs: config.NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"region":     regionRef,
			},
			)})
	}

	if projectIDExists && zoneExists {
		defaults = append(defaults, config.Validator{
			Validator: testZoneExistsName,
			Inputs: config.NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"zone":       zoneRef,
			}),
		})
	}

	if projectIDExists && regionExists && zoneExists {
		defaults = append(defaults, config.Validator{
			Validator: testZoneInRegionName,
			Inputs: config.NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"region":     regionRef,
				"zone":       zoneRef,
			}),
		})
	}
	return defaults
}

// Returns a list of validators for the given blueprint with any default validators appended.
func validators(bp config.Blueprint) []config.Validator {
	used := map[string]bool{}
	for _, v := range bp.Validators {
		used[v.Validator] = true
	}
	vs := append([]config.Validator{}, bp.Validators...) // clone
	for _, v := range defaults(bp) {
		if !used[v.Validator] {
			vs = append(vs, v)
		}
	}
	return vs
}
