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
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
	resourcemanagerpb "google.golang.org/genproto/googleapis/cloud/resourcemanager/v3"
)

// TestProjectExists whether projectID exists / is accessible with credentials
func TestProjectExists(projectID string) error {
	ctx := context.Background()
	c, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	req := &resourcemanagerpb.GetProjectRequest{
		Name: "projects/" + projectID,
	}

	resp, err := c.GetProject(ctx, req)
	if err != nil {
		return err
	}
	// TODO: Use resp.
	_ = resp

	return nil
}

// TestRegionExists whether region exists / is accessible with credentials
func TestRegionExists(region string) error {
	ctx := context.Background()
	c, err := compute.NewRegionsRESTClient(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	req := &computepb.GetRegionRequest{
		Project: "",
		Region:  region,
	}
	resp, err := c.Get(ctx, req)
	if err != nil {
		return err
	}
	// TODO: Use resp.
	_ = resp
	return nil
}

// TestZoneExists whether zone exists / is accessible with credentials
func TestZoneExists(zone string) error {
	ctx := context.Background()
	c, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	req := &computepb.GetZoneRequest{
		Project: "",
		Zone:    zone,
	}
	resp, err := c.Get(ctx, req)
	if err != nil {
		return err
	}
	_ = resp

	return nil
}

// TestZoneInRegion whether zone is in region
func TestZoneInRegion(zone string, region string) error {
	ctx := context.Background()
	c, err := compute.NewZonesRESTClient(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	req := &computepb.GetZoneRequest{
		Project: "",
		Zone:    zone,
	}
	resp, err := c.Get(ctx, req)
	if err != nil {
		return err
	}
	if *resp.Region != region {
		return fmt.Errorf("zone %s is not in region %s", zone, region)
	}

	return nil
}
