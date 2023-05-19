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
	"log"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	serviceusage "cloud.google.com/go/serviceusage/apiv1"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/option"
	serviceusagepb "google.golang.org/genproto/googleapis/api/serviceusage/v1"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
)

const enableAPImsg = "%[1]s: can be enabled at https://console.cloud.google.com/apis/library/%[1]s?project=%[2]s"
const projectError = "project ID %s does not exist or your credentials do not have permission to access it"
const regionError = "region %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneError = "zone %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneInRegionError = "zone %s is not in region %s in project ID %s or your credentials do not have permissions to access it"
const computeDisabledError = "Compute Engine API has not been used in project"
const computeDisabledMsg = "the Compute Engine API must be enabled in project %s to validate blueprint global variables"
const serviceDisabledMsg = "the Service Usage API must be enabled in project %s to validate that all APIs needed by the blueprint are enabled"
const unusedModuleMsg = "module %s uses module %s, but matching setting and outputs were not found. This may be because the value is set explicitly or set by a prior used module"
const unusedModuleError = "One or more used modules could not have their settings and outputs linked."
const unusedDeploymentVariableMsg = "the deployment variable \"%s\" was not used in this blueprint"
const unusedDeploymentVariableError = "one or more deployment variables was not used by any modules"

func handleClientError(e error) error {
	if strings.Contains(e.Error(), "could not find default credentials") {
		log.Println("load application default credentials following instructions at https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/README.md#supplying-cloud-credentials-to-terraform")
		return fmt.Errorf("could not find application default credentials")

	}
	return e
}

// TestDeploymentVariablesNotUsed errors if there are any unused deployment
// variables and prints any to the output for the user
func TestDeploymentVariablesNotUsed(unusedVariables []string) error {
	for _, v := range unusedVariables {
		log.Printf(unusedDeploymentVariableMsg, v)
	}

	if len(unusedVariables) > 0 {
		return fmt.Errorf(unusedDeploymentVariableError)
	}

	return nil
}

// TestModuleNotUsed validates that all modules referenced in the "use" field
// of the blueprint are actually used, i.e. the outputs and settings are
// connected.
func TestModuleNotUsed(unusedModules map[string][]string) error {
	any := false
	for mod, unusedMods := range unusedModules {
		for _, unusedMod := range unusedMods {
			log.Printf(unusedModuleMsg, mod, unusedMod)
			any = true
		}
	}

	if any {
		return fmt.Errorf(unusedModuleError)
	}

	return nil
}

// TestApisEnabled tests whether APIs are enabled in given project
func TestApisEnabled(projectID string, requiredAPIs []string) error {
	// can return immediately if there are 0 APIs to test
	if len(requiredAPIs) == 0 {
		return nil
	}

	ctx := context.Background()

	c, err := serviceusage.NewClient(ctx, option.WithQuotaProject(projectID))
	if err != nil {
		err = handleClientError(err)
		return err
	}
	defer c.Close()

	prefix := "projects/" + projectID
	var serviceNames []string
	for _, api := range requiredAPIs {
		serviceNames = append(serviceNames, prefix+"/services/"+api)
	}

	req := &serviceusagepb.BatchGetServicesRequest{
		Parent: prefix,
		Names:  serviceNames,
	}
	resp, err := c.BatchGetServices(ctx, req)
	if err != nil {
		var ae *apierror.APIError
		if errors.As(err, &ae) {
			switch reason := ae.Reason(); reason {
			case "SERVICE_DISABLED":
				log.Printf(enableAPImsg, "serviceusage.googleapis.com", projectID)
				return fmt.Errorf(serviceDisabledMsg, projectID)
			case "SERVICE_CONFIG_NOT_FOUND_OR_PERMISSION_DENIED":
				return fmt.Errorf("service %s does not exist in project %s", ae.Metadata()["services"], projectID)
			case "USER_PROJECT_DENIED":
				return fmt.Errorf(projectError, projectID)
			case "SU_MISSING_NAMES":
				// occurs if API list is empty and 0 APIs to validate
				return nil
			default:
				return fmt.Errorf("unhandled error: %s", err)
			}
		}
	}

	var errored bool
	for _, service := range resp.Services {
		if service.State.String() == "DISABLED" {
			errored = true
			log.Printf("%s: service is disabled in project %s", service.Config.Name, projectID)
			log.Printf(enableAPImsg, service.Config.Name, projectID)
		}
	}
	if errored {
		return fmt.Errorf("one or more required APIs are disabled in project %s, please enable them as instructed above", projectID)
	}
	return nil
}

// TestProjectExists whether projectID exists / is accessible with credentials
func TestProjectExists(projectID string) error {
	ctx := context.Background()
	c, err := compute.NewProjectsRESTClient(ctx)
	if err != nil {
		err = handleClientError(err)
		return err
	}
	defer c.Close()

	req := &computepb.GetProjectRequest{
		Project: projectID,
	}

	_, err = c.Get(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), computeDisabledError) {
			log.Printf(computeDisabledMsg, projectID)
			log.Printf(serviceDisabledMsg, projectID)
			log.Printf(enableAPImsg, "serviceusage.googleapis.com", projectID)
			return fmt.Errorf(enableAPImsg, "compute.googleapis.com", projectID)
		}
		return fmt.Errorf(projectError, projectID)
	}

	return nil
}

func getRegion(projectID string, region string) (*computepb.Region, error) {
	ctx := context.Background()
	c, err := compute.NewRegionsRESTClient(ctx)
	if err != nil {
		err = handleClientError(err)
		return nil, err
	}
	defer c.Close()

	req := &computepb.GetRegionRequest{
		Project: projectID,
		Region:  region,
	}
	regionObject, err := c.Get(ctx, req)
	if err != nil {
		return nil, err
	}

	return regionObject, nil
}

// TestRegionExists whether region exists / is accessible with credentials
func TestRegionExists(projectID string, region string) error {
	_, err := getRegion(projectID, region)
	if err != nil {
		return fmt.Errorf(regionError, region, projectID)
	}
	return nil
}

func getZone(projectID string, zone string) (*computepb.Zone, error) {
	ctx := context.Background()
	c, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		err = handleClientError(err)
		return nil, err
	}
	defer c.Close()

	req := &computepb.GetZoneRequest{
		Project: projectID,
		Zone:    zone,
	}
	zoneObject, err := c.Get(ctx, req)
	if err != nil {
		return nil, err
	}

	return zoneObject, nil
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

	if *zoneObject.Region != *regionObject.SelfLink {
		return fmt.Errorf(zoneInRegionError, zone, region, projectID)
	}

	return nil
}
