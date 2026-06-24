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
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
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

func (s *MySuite) TestFindReservationOwnerProject(c *C) {
	// Case 1: No reservation_affinity in blueprint
	{
		bp := config.Blueprint{
			Groups: []config.Group{
				{
					Modules: []config.Module{
						{
							ID:       "m1",
							Settings: config.NewDict(nil),
						},
					},
				},
			},
		}
		proj := findReservationOwnerProject(bp, "my-res")
		c.Check(proj, Equals, "")
	}

	// Case 2: reservation_affinity present but type is not SPECIFIC_RESERVATION
	{
		bp := config.Blueprint{
			Groups: []config.Group{
				{
					Modules: []config.Module{
						{
							ID: "m1",
							Settings: config.NewDict(map[string]cty.Value{
								"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
									"consume_reservation_type": cty.StringVal("ANY_RESERVATION"),
								}),
							}),
						},
					},
				},
			},
		}
		proj := findReservationOwnerProject(bp, "my-res")
		c.Check(proj, Equals, "")
	}

	// Case 3: SPECIFIC_RESERVATION present, but name doesn't match
	{
		bp := config.Blueprint{
			Groups: []config.Group{
				{
					Modules: []config.Module{
						{
							ID: "m1",
							Settings: config.NewDict(map[string]cty.Value{
								"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
									"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
									"specific_reservations": cty.ListVal([]cty.Value{
										cty.ObjectVal(map[string]cty.Value{
											"name":    cty.StringVal("other-res"),
											"project": cty.StringVal("owner-proj"),
										}),
									}),
								}),
							}),
						},
					},
				},
			},
		}
		proj := findReservationOwnerProject(bp, "my-res")
		c.Check(proj, Equals, "")
	}

	// Case 4: SPECIFIC_RESERVATION present, name matches, project is set
	{
		bp := config.Blueprint{
			Groups: []config.Group{
				{
					Modules: []config.Module{
						{
							ID: "m1",
							Settings: config.NewDict(map[string]cty.Value{
								"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
									"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
									"specific_reservations": cty.ListVal([]cty.Value{
										cty.ObjectVal(map[string]cty.Value{
											"name":    cty.StringVal("my-res"),
											"project": cty.StringVal("owner-proj"),
										}),
									}),
								}),
							}),
						},
					},
				},
			},
		}
		proj := findReservationOwnerProject(bp, "my-res")
		c.Check(proj, Equals, "owner-proj")
	}

	// Case 5: SPECIFIC_RESERVATION present, name matches, project is NOT set (optional field)
	{
		bp := config.Blueprint{
			Groups: []config.Group{
				{
					Modules: []config.Module{
						{
							ID: "m1",
							Settings: config.NewDict(map[string]cty.Value{
								"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
									"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
									"specific_reservations": cty.ListVal([]cty.Value{
										cty.ObjectVal(map[string]cty.Value{
											"name":    cty.StringVal("my-res"),
											"project": cty.NullVal(cty.String),
										}),
									}),
								}),
							}),
						},
					},
				},
			},
		}
		proj := findReservationOwnerProject(bp, "my-res")
		c.Check(proj, Equals, "")
	}
}

func (s *MySuite) TestReservationExistsValidatorWithSharedReservation(c *C) {
	var capturedProject string
	var capturedZone string
	var capturedName string

	// 1. Mock the compute service
	oldCreator := newComputeService
	defer func() { newComputeService = oldCreator }()
	newComputeService = func(ctx context.Context) (*compute.Service, error) {
		return mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			// URL format: /projects/{project}/zones/{zone}/reservations/{reservation}
			path := r.URL.Path
			parts := strings.Split(path, "/")
			if len(parts) >= 7 && parts[1] == "projects" && parts[3] == "zones" && parts[5] == "reservations" {
				capturedProject = parts[2]
				capturedZone = parts[4]
				capturedName = parts[6]
			}
			// Return a dummy reservation to simulate success
			_, _ = w.Write([]byte(`{"name": "my-shared-res", "status": "READY"}`))
		}), nil
	}

	// 2. Set up a blueprint with a shared reservation in a module's reservation_affinity
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id":       cty.StringVal("consumer-proj"),
			"zone":             cty.StringVal("us-central1-a"),
			"reservation_name": cty.StringVal("my-shared-res"),
		}),
		Groups: []config.Group{
			{
				Modules: []config.Module{
					{
						ID: "gke_node_pool",
						Settings: config.NewDict(map[string]cty.Value{
							"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
								"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
								"specific_reservations": cty.ListVal([]cty.Value{
									cty.ObjectVal(map[string]cty.Value{
										"name":    cty.StringVal("my-shared-res"),
										"project": cty.StringVal("owner-proj"),
									}),
								}),
							}),
						}),
					},
				},
			},
		},
	}

	// 3. Run the validator
	inputs := config.NewDict(map[string]cty.Value{
		"project_id":       cty.StringVal("consumer-proj"),
		"zone":             cty.StringVal("us-central1-a"),
		"reservation_name": cty.StringVal("my-shared-res"),
	})

	err := testReservationExists(bp, inputs)

	// 4. Assertions
	c.Check(err, IsNil)                            // Should succeed because mock returned 200 OK
	c.Check(capturedProject, Equals, "owner-proj") // CRITICAL: Must be owner-proj, not consumer-proj!
	c.Check(capturedZone, Equals, "us-central1-a")
	c.Check(capturedName, Equals, "my-shared-res")
}

func (s *MySuite) TestReservationExistsValidatorWithSharedReservation_PermissionDenied(c *C) {
	// 1. Mock the compute service to return 403 Forbidden
	oldCreator := newComputeService
	defer func() { newComputeService = oldCreator }()
	newComputeService = func(ctx context.Context) (*compute.Service, error) {
		return mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error": {"code": 403, "message": "Identity lacks permission to get reservation"}}`))
		}), nil
	}

	// 2. Set up a blueprint with a shared reservation in a module's reservation_affinity
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id":       cty.StringVal("consumer-proj"),
			"zone":             cty.StringVal("us-central1-a"),
			"reservation_name": cty.StringVal("my-shared-res"),
		}),
		Groups: []config.Group{
			{
				Modules: []config.Module{
					{
						ID: "gke_node_pool",
						Settings: config.NewDict(map[string]cty.Value{
							"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
								"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
								"specific_reservations": cty.ListVal([]cty.Value{
									cty.ObjectVal(map[string]cty.Value{
										"name":    cty.StringVal("my-shared-res"),
										"project": cty.StringVal("owner-proj"),
									}),
								}),
							}),
						}),
					},
				},
			},
		},
	}

	// 3. Run the validator
	inputs := config.NewDict(map[string]cty.Value{
		"project_id":       cty.StringVal("consumer-proj"),
		"zone":             cty.StringVal("us-central1-a"),
		"reservation_name": cty.StringVal("my-shared-res"),
	})

	err := testReservationExists(bp, inputs)

	// 4. Assertion: Should succeed (return nil) despite 403, because it's handled as a soft warning
	c.Check(err, IsNil)
}

func (s *MySuite) TestReservationExistsValidatorWithSharedReservation_NotFound(c *C) {
	// 1. Mock the compute service
	oldCreator := newComputeService
	defer func() { newComputeService = oldCreator }()
	newComputeService = func(ctx context.Context) (*compute.Service, error) {
		return mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if strings.Contains(path, "aggregated") {
				// Discovery/List succeeds but returns empty list of reservations
				_, _ = w.Write([]byte(`{"items": {}}`))
				return
			}
			// Direct check fails with 404
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": {"code": 404, "message": "Reservation not found"}}`))
		}), nil
	}

	// 2. Set up a blueprint with a shared reservation in a module's reservation_affinity
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id":       cty.StringVal("consumer-proj"),
			"zone":             cty.StringVal("us-central1-a"),
			"reservation_name": cty.StringVal("my-shared-res"),
		}),
		Groups: []config.Group{
			{
				Modules: []config.Module{
					{
						ID: "gke_node_pool",
						Settings: config.NewDict(map[string]cty.Value{
							"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
								"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
								"specific_reservations": cty.ListVal([]cty.Value{
									cty.ObjectVal(map[string]cty.Value{
										"name":    cty.StringVal("my-shared-res"),
										"project": cty.StringVal("owner-proj"),
									}),
								}),
							}),
						}),
					},
				},
			},
		},
	}

	// 3. Run the validator
	inputs := config.NewDict(map[string]cty.Value{
		"project_id":       cty.StringVal("consumer-proj"),
		"zone":             cty.StringVal("us-central1-a"),
		"reservation_name": cty.StringVal("my-shared-res"),
	})

	err := testReservationExists(bp, inputs)

	// 4. Assertion: Should FAIL (return error) because it's 404 and not found anywhere
	c.Assert(err, NotNil) // Use Assert to stop execution if nil, avoiding panic on next line
	c.Check(err.Error(), Matches, ".*was not found in any zone of project.*")
}

func (s *MySuite) TestReservationExistsValidatorWithSharedReservation_UnknownAndMarked(c *C) {
	// 1. Mock the compute service to return success ONLY for the resolved owner-proj
	oldCreator := newComputeService
	defer func() { newComputeService = oldCreator }()
	apiCalled := false
	newComputeService = func(ctx context.Context) (*compute.Service, error) {
		return mockComputeService(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			// Verify the API call is routed to the owner project resolved from the marked setting
			if strings.Contains(path, "owner-proj") && strings.Contains(path, "us-central1-a") && strings.Contains(path, "my-shared-res") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"name": "my-shared-res", "status": "READY"}`))
				apiCalled = true
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}), nil
	}

	// 2. Create a marked reservation_affinity value (simulating sensitive or module-passed values)
	markedAffinity := cty.ObjectVal(map[string]cty.Value{
		"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
		"specific_reservations": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"name":    cty.StringVal("my-shared-res"),
				"project": cty.StringVal("owner-proj"),
			}),
		}),
	}).Mark("sensitive-metadata") // Mark the entire object

	// 3. Set up a blueprint containing:
	//    - A module with a MARKED reservation_affinity
	//    - A module with an UNKNOWN reservation_affinity (simulating unresolved module outputs)
	//    - A module with a list containing an UNKNOWN reservation element
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id":       cty.StringVal("consumer-proj"),
			"zone":             cty.StringVal("us-central1-a"),
			"reservation_name": cty.StringVal("my-shared-res"),
		}),
		Groups: []config.Group{
			{
				Modules: []config.Module{
					{
						ID: "module_with_marked_affinity",
						Settings: config.NewDict(map[string]cty.Value{
							"reservation_affinity": markedAffinity,
						}),
					},
					{
						ID: "module_with_unknown_affinity",
						Settings: config.NewDict(map[string]cty.Value{
							"reservation_affinity": cty.UnknownVal(cty.Object(map[string]cty.Type{
								"consume_reservation_type": cty.String,
							})),
						}),
					},
					{
						ID: "module_with_unknown_element",
						Settings: config.NewDict(map[string]cty.Value{
							"reservation_affinity": cty.ObjectVal(map[string]cty.Value{
								"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
								"specific_reservations": cty.ListVal([]cty.Value{
									cty.UnknownVal(cty.Object(map[string]cty.Type{
										"name":    cty.String,
										"project": cty.String,
									})),
								}),
							}),
						}),
					},
				},
			},
		},
	}

	// 4. Run the validator
	inputs := config.NewDict(map[string]cty.Value{
		"project_id":       cty.StringVal("consumer-proj"),
		"zone":             cty.StringVal("us-central1-a"),
		"reservation_name": cty.StringVal("my-shared-res"),
	})

	// Execution should NOT panic. It should safely skip unknown values,
	// unmark the marked value, resolve "owner-proj", and succeed.
	err := testReservationExists(bp, inputs)

	// 5. Assertions
	c.Assert(err, IsNil)
	c.Check(apiCalled, Equals, true)
}
