// Copyright 2026 Google LLC
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
	"hpc-toolkit/pkg/config"
	"net/http"
	"net/http/httptest"
	"testing"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *MySuite) TestCheckInputs(c *C) {
	dummy := cty.NullVal(cty.String)

	{ // OK: Inputs is equal to required inputs without regard to ordering
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy})
		c.Check(checkInputs(i, []string{"in0", "in1"}), IsNil)
		c.Check(checkInputs(i, []string{"in1", "in0"}), IsNil)
	}

	{ // FAIL: inputs are a proper subset of required inputs
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy})
		err := checkInputs(i, []string{"in0", "in1", "in2"})
		c.Check(err, NotNil)
	}

	{ // FAIL: inputs intersect with required inputs but are not a proper subset
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy,
			"in3": dummy})
		err := checkInputs(i, []string{"in0", "in1", "in2"})
		c.Check(err, NotNil)
	}

	{ // FAIL inputs are a proper superset of required inputs
		i := config.NewDict(map[string]cty.Value{
			"in0": dummy,
			"in1": dummy,
			"in2": dummy,
			"in3": dummy})
		err := checkInputs(i, []string{"in0", "in1", "in2"})
		c.Check(err, ErrorMatches, "only 3 inputs \\[in0 in1 in2\\] should be provided")
	}
}

func (s *MySuite) TestDefaultValidators(c *C) {
	unusedMods := config.Validator{Validator: "test_module_not_used"}
	unusedVars := config.Validator{Validator: "test_deployment_variable_not_used"}

	prjInp := config.Dict{}.With("project_id", config.GlobalRef("project_id").AsValue())
	regInp := prjInp.With("region", config.GlobalRef("region").AsValue())
	zoneInp := prjInp.With("zone", config.GlobalRef("zone").AsValue())
	regZoneInp := regInp.With("zone", config.GlobalRef("zone").AsValue())

	projectExists := config.Validator{
		Validator: "test_project_exists", Inputs: prjInp}
	apisEnabled := config.Validator{
		Validator: "test_apis_enabled", Inputs: prjInp}
	regionExists := config.Validator{
		Validator: testRegionExistsName, Inputs: regInp}
	zoneExists := config.Validator{
		Validator: testZoneExistsName, Inputs: zoneInp}
	zoneInRegion := config.Validator{
		Validator: testZoneInRegionName, Inputs: regZoneInp}
	machineTypeInZone := config.Validator{
		Validator: "test_machine_type_in_zone", Inputs: zoneInp}
	diskTypeInZone := config.Validator{
		Validator: testDiskTypeInZone, Inputs: zoneInp}
	resInp := zoneInp.With("reservation_name", config.GlobalRef("reservation_name").AsValue())
	resExists := config.Validator{
		Validator: testReservationExistsName, Inputs: resInp}

	myResInp := zoneInp.With("reservation_name", config.GlobalRef("my_reservation").AsValue())
	myResExists := config.Validator{
		Validator: testReservationExistsName, Inputs: myResInp}

	{
		bp := config.Blueprint{}
		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b"))}
		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, projectExists, apisEnabled})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("region", cty.StringVal("narnia"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, projectExists, apisEnabled, regionExists})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("zone", cty.StringVal("danger"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, projectExists, apisEnabled, zoneExists, machineTypeInZone, diskTypeInZone})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("region", cty.StringVal("narnia")).
			With("zone", cty.StringVal("danger"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, projectExists, apisEnabled, regionExists, zoneExists, machineTypeInZone, diskTypeInZone, zoneInRegion})
	}
	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("zone", cty.StringVal("danger")).
			With("reservation_name", cty.StringVal("my-res"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, projectExists, apisEnabled, zoneExists, machineTypeInZone, diskTypeInZone, resExists})
	}

	{
		bp := config.Blueprint{Vars: config.Dict{}.
			With("project_id", cty.StringVal("f00b")).
			With("zone", cty.StringVal("danger")).
			With("my_reservation", cty.StringVal("my-res"))}

		c.Check(defaults(bp), DeepEquals, []config.Validator{
			unusedMods, unusedVars, projectExists, apisEnabled, zoneExists, machineTypeInZone, diskTypeInZone, myResExists})
	}
}

// Helper to create a mock compute service for unit tests
func mockComputeService(handler http.HandlerFunc) *compute.Service {
	ts := httptest.NewServer(handler)
	s, _ := compute.NewService(context.Background(),
		option.WithEndpoint(ts.URL),
		option.WithHTTPClient(ts.Client()))
	return s
}

func (s *MySuite) TestValidateMachineTypeInZone(c *C) {
	const validatorName = "test_machine_type_in_zone"
	// Case 1: Success (200 OK)
	{
		svc := mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"name": "c2-standard-60"}`))
		})
		err := validateMachineTypeInZone(svc, "proj", "zone", "mt", validatorName)
		c.Check(err, IsNil)
	}

	// Case 2: Soft Warning (403 Forbidden)
	{
		svc := mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error": {"code": 403, "message": "Denied"}}`))
		})
		err := validateMachineTypeInZone(svc, "proj", "zone", "mt", validatorName)
		// FIXED: Replaced errors.Is with direct comparison
		c.Check(err == errSoftWarning, Equals, true)
	}

	// Case 4: Hard Failure (404 Not Found)
	{
		svc := mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		err := validateMachineTypeInZone(svc, "proj", "zone", "mt", validatorName)
		c.Check(err, NotNil)
		c.Check(err == errSoftWarning, Equals, false)
	}
}

func (s *MySuite) TestResolveZonesAndOverride(c *C) {
	bp := config.Blueprint{}

	// Case 1: Plural list (*_zones) takes priority over singular zone (*_zone)
	{
		mod := &config.Module{
			ID: "m1",
			Settings: config.NewDict(map[string]cty.Value{
				"zone":      cty.StringVal("ignore-me"),
				"gpu_zones": cty.StringVal("use-me"),
			}),
		}
		res, err := resolveZones(bp, mod, "global")
		c.Check(err, IsNil)
		c.Check(res, DeepEquals, []string{"use-me"})
	}

	// Case 2: Singular override works when no plural list is present
	{
		mod := &config.Module{
			ID: "m2",
			Settings: config.NewDict(map[string]cty.Value{
				"compute_zone": cty.StringVal("override"),
			}),
		}
		res, err := resolveZones(bp, mod, "global")
		c.Check(err, IsNil)
		c.Check(res, DeepEquals, []string{"override"})
	}

	// Case 3: Fallback to global default when no module settings exist
	{
		mod := &config.Module{ID: "m3", Settings: config.NewDict(nil)}
		res, err := resolveZones(bp, mod, "default-zone")
		c.Check(err, IsNil)
		c.Check(res, DeepEquals, []string{"default-zone"})
	}

	// Case 4: Type Error in zones list (Hard Failure)
	// This verifies that unquoted numbers trigger the official toolkit error
	{
		mod := &config.Module{
			ID: "m4",
			Settings: config.NewDict(map[string]cty.Value{
				"zones": cty.ListVal([]cty.Value{cty.NumberIntVal(10)}),
			}),
		}
		res, err := resolveZones(bp, mod, "global")
		c.Check(res, IsNil)
		c.Check(err, NotNil)
		// Verifies the error message matches the standard toolkit type error
		c.Check(err.Error(), Matches, ".*must be strings.*")
	}
}

func (s *MySuite) TestValidateDiskTypeInZone(c *C) {
	const validatorName = "test_disk_type_in_zone"
	// Case 1: Success (200 OK)
	{
		svc := mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"name": "pd-balanced"}`))
		})
		err := validateDiskTypeInZone(svc, "proj", "zone", "pd-balanced", validatorName)
		c.Check(err, IsNil)
	}

	// Case 2: Soft Warning (403 Forbidden)
	{
		svc := mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error": {"code": 403, "message": "Denied"}}`))
		})
		err := validateDiskTypeInZone(svc, "proj", "zone", "pd-balanced", validatorName)
		c.Check(err == errSoftWarning, Equals, true)
	}

	// Case 3: Hard Failure (404 Not Found)
	{
		svc := mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		err := validateDiskTypeInZone(svc, "proj", "zone", "invalid-disk", validatorName)
		c.Check(err, NotNil)
		c.Check(err == errSoftWarning, Equals, false)

		c.Check(err.Error(), Matches, ".*disk type.*invalid-disk.*not available.*")
	}
}
