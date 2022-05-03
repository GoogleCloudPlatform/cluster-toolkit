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
	"fmt"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
)

const projectError = "project ID %s does not exist or your credentials do not have permission to access it"
const regionError = "region %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneError = "zone %s is not available in project ID %s or your credentials do not have permission to access it"
const zoneInRegionError = "zone %s is not in region %s in project ID %s or your credentials do not have permissions to access it"

// TestProjectExists whether projectID exists / is accessible with credentials
func TestProjectExists(projectID string) error {
	ctx := context.Background()
	c, err := compute.NewProjectsRESTClient(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	req := &computepb.GetProjectRequest{
		Project: projectID,
	}

	_, err = c.Get(ctx, req)
	if err != nil {
		return fmt.Errorf(projectError, projectID)
	}

	return nil
}

func getRegion(projectID string, region string) (*computepb.Region, error) {
	ctx := context.Background()
	c, err := compute.NewRegionsRESTClient(ctx)
	if err != nil {
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
