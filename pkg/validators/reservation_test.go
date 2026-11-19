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

	"github.com/zclconf/go-cty/cty"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	. "gopkg.in/check.v1"
)

type mockResEnv struct {
	queried      []string
	serviceInits int
	onlyFuture   map[string]bool
	notFound     map[string]bool
	aggStatus    int
	aggZone      string
}

func (e *mockResEnv) reset() {
	e.queried = nil
	e.serviceInits = 0
	e.onlyFuture = map[string]bool{}
	e.notFound = map[string]bool{}
	e.aggStatus = 0
	e.aggZone = ""
}

func (e *mockResEnv) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "/aggregated/") {
		if e.aggStatus != 0 {
			w.WriteHeader(e.aggStatus)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"forbidden"}}`))
			return
		}
		if e.aggZone != "" && strings.HasSuffix(r.URL.Path, "/reservations") {
			_, _ = w.Write([]byte(`{"items":{"zones/` + e.aggZone + `":{"reservations":[{"name":"wrong-zone-res","zone":"zones/` + e.aggZone + `"}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":{}}`))
		return
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) >= 7 && parts[1] == "projects" && parts[3] == "zones" {
		kind, name := parts[5], parts[6]
		e.queried = append(e.queried, parts[2]+"/"+parts[4]+"/"+kind+"/"+name)
		if e.notFound[name] || (e.onlyFuture[name] && kind == "reservations") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
			return
		}
	}
	_, _ = w.Write([]byte(`{"name":"ok"}`))
}

func withMockReservationServer(fn func(env *mockResEnv)) {
	env := &mockResEnv{onlyFuture: map[string]bool{}, notFound: map[string]bool{}}
	ts := httptest.NewServer(http.HandlerFunc(env.serveHTTP))
	defer ts.Close()
	oldCreator := newComputeService
	defer func() { newComputeService = oldCreator }()
	newComputeService = func(ctx context.Context) (*compute.Service, error) {
		env.serviceInits++
		return compute.NewService(ctx, option.WithEndpoint(ts.URL), option.WithHTTPClient(ts.Client()))
	}
	fn(env)
}

func testMod(id string, settings map[string]cty.Value) config.Module {
	return config.Module{ID: config.ModuleID(id), Settings: config.NewDict(settings)}
}

func testBP(vars map[string]cty.Value, mods ...config.Module) config.Blueprint {
	bp := config.Blueprint{Vars: config.NewDict(vars)}
	if len(mods) > 0 {
		bp.Groups = []config.Group{{Modules: mods}}
	}
	return bp
}

func specResEntry(name, project string) cty.Value {
	m := map[string]cty.Value{"name": cty.StringVal(name)}
	if project != "" {
		m["project"] = cty.StringVal(project)
	}
	return cty.ObjectVal(m)
}

func affinityVal(consumeType string, entries ...cty.Value) cty.Value {
	return cty.ObjectVal(map[string]cty.Value{
		"consume_reservation_type": cty.StringVal(consumeType),
		"specific_reservations":    cty.TupleVal(entries),
	})
}

func assertHintError(c *C, err error, pattern string) {
	c.Assert(err, NotNil)
	_, ok := err.(config.HintError)
	c.Assert(ok, Equals, true)
	c.Assert(err.Error(), Matches, pattern)
}

func stdInputs() config.Dict {
	return config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a")})
}

func (s *MySuite) TestParseReservationURI(c *C) {
	cases := []struct {
		in, proj, zone, name string
		wantErr              bool
	}{
		{in: "my-res", name: "my-res"},
		{in: "my-res/ReservationBlocks/b1/reservationSubBlocks/s2", name: "my-res"},
		{in: "\u0130-res/reservationBlocks/b1", name: "\u0130-res"},
		{in: "projects/p1/reservations/r1", proj: "p1", name: "r1"},
		{in: "projects/p1/futureReservations/fr1", proj: "p1", name: "fr1"},
		{in: "projects/p1/zones/z1/reservations/r1", proj: "p1", zone: "z1", name: "r1"},
		{in: "https://www.googleapis.com/compute/v1/projects/p1/zones/z1/reservations/r1/", proj: "p1", zone: "z1", name: "r1"},
		{in: "HTTPS://COMPUTE.GOOGLEAPIS.COM/compute/beta/projects/p1/zones/z1/futureReservations/fr1", proj: "p1", zone: "z1", name: "fr1"},
		{in: "http://compute.googleapis.com/projects/p1/zones/z1/reservations/r1/reservationBlocks/b1", proj: "p1", zone: "z1", name: "r1"},
		{in: "/compute/v1/projects/p1/reservations/r1", proj: "p1", name: "r1"},
		{in: "foo/bar", wantErr: true},
		{in: "/reservationBlocks/b1", wantErr: true},
		{in: "compute/v1/my-res", wantErr: true},
		{in: "reservationBlocks/b1", wantErr: true},
		{in: "my-res/reservationBlocks/", wantErr: true},
		{in: "my-res/reservationBlocks//b1", wantErr: true},
		{in: "projects//reservations/r", wantErr: true},
		{in: "projects/p1/other/r1", wantErr: true},
	}
	for _, tc := range cases {
		proj, zone, name, err := parseReservationURI(tc.in)
		if tc.wantErr {
			assertHintError(c, err, ".*malformed reservation.*")
			continue
		}
		c.Assert(err, IsNil)
		c.Assert([]string{proj, zone, name}, DeepEquals, []string{tc.proj, tc.zone, tc.name})
	}
}

func (s *MySuite) TestDefaultsReservationAttachment(c *C) {
	base := map[string]cty.Value{"project_id": cty.StringVal("p"), "zone": cty.StringVal("z")}
	gkeAff := affinityVal("SPECIFIC_RESERVATION", specResEntry("gke-res", ""))
	cases := []struct {
		bp                    config.Blueprint
		wantCount, wantInputs int
	}{
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("p"), "zone": cty.StringVal("z"), "reservation_name": cty.StringVal("r1")}), 1, 3},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("p"), "zone": cty.StringVal("z"), "cpu_reservation": cty.StringVal("r1"), "gpu_reservation": cty.StringVal("r2")}), 1, 2},
		{testBP(base, testMod("gke", map[string]cty.Value{"reservation_affinity": gkeAff})), 1, 2},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("p")}, testMod("slurm", map[string]cty.Value{"reservation_name": cty.StringVal("r1")})), 1, 1},
		{testBP(base, testMod("bad", map[string]cty.Value{"reservation_name": cty.StringVal("projects/bad")})), 1, 2},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("p"), "zone": cty.StringVal("z"), "reservation_name": cty.StringVal("NO_RESERVATION")}), 0, 0},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("p"), "zone": cty.StringVal("z"), "reservation_name": cty.StringVal("ANY_RESERVATION")}), 0, 0},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("p"), "zone": cty.StringVal("z"), "reservation_name": cty.NullVal(cty.String), "enable_reservation": cty.True, "gpu_reservation": cty.DynamicVal}), 0, 0},
		{testBP(base, testMod("spot", map[string]cty.Value{"spot": cty.True, "reservation_affinity": gkeAff})), 0, 0},
		{testBP(base, testMod("flex", map[string]cty.Value{"dws_flex": cty.ObjectVal(map[string]cty.Value{"max_run": cty.NumberIntVal(10)}), "reservation_name": cty.StringVal("r1")})), 0, 0},
		{testBP(map[string]cty.Value{"reservation_name": cty.StringVal("r1")}), 0, 0},
	}
	for _, tc := range cases {
		var matched []config.Validator
		for _, v := range defaults(tc.bp) {
			if v.Validator == testReservationExistsName {
				matched = append(matched, v)
			}
		}
		c.Assert(len(matched), Equals, tc.wantCount)
		if tc.wantCount == 1 {
			c.Assert(len(matched[0].Inputs.Items()), Equals, tc.wantInputs)
		}
	}
}

func (s *MySuite) TestReservationDiscoveryRoutingAndDeduplication(c *C) {
	unknownRef := config.ModuleRef("host", "project_id").AsValue()
	varRes := config.GlobalRef("reservation_name").AsValue()
	baseVars := map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a")}
	cases := []struct {
		bp           config.Blueprint
		in           config.Dict
		future, want []string
	}{
		{testBP(baseVars,
			testMod("m1", map[string]cty.Value{"zone": cty.StringVal("us-east1-b"), "backup_zone": cty.StringVal("bad"), "timezone": cty.StringVal("UTC"), "reservation_name": cty.StringVal("r1")}),
			testMod("m2", map[string]cty.Value{"zones": cty.SetVal([]cty.Value{cty.StringVal("us-west1-a"), cty.StringVal("us-west1-b")}), "gpu_reservation": cty.StringVal("r2")}),
			testMod("m3", map[string]cty.Value{"zones": cty.ListValEmpty(cty.String), "reservation_name": cty.StringVal("r3")})),
			stdInputs(), nil,
			[]string{"proj-a/us-east1-b/reservations/r1", "proj-a/us-west1-a/reservations/r2", "proj-a/us-west1-b/reservations/r2", "proj-a/us-central1-a/reservations/r3"}},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "unknown_res": cty.StringVal("unk-shared")},
			testMod("gke1", map[string]cty.Value{"reservation_affinity": affinityVal("SPECIFIC_RESERVATION",
				specResEntry("fut-r/reservationBlocks/b1", ""), specResEntry("shared-r", "host-proj"),
				cty.ObjectVal(map[string]cty.Value{"name": config.GlobalRef("unknown_res").AsValue(), "project": unknownRef}))}),
			testMod("gke2", map[string]cty.Value{"reservation_name": cty.StringVal("projects/host-proj/zones/us-central1-a/reservations/shared-r")})),
			stdInputs(), []string{"fut-r"},
			[]string{"proj-a/us-central1-a/reservations/fut-r", "proj-a/us-central1-a/futureReservations/fut-r", "host-proj/us-central1-a/reservations/shared-r"}},
		{testBP(baseVars, testMod("unk-z", map[string]cty.Value{
			"zones": cty.TupleVal([]cty.Value{cty.StringVal("us-central1-a"), unknownRef}), "zone": cty.StringVal("us-east1-b"),
			"reservation_name": cty.StringVal("skip-short"), "gpu_reservation": cty.StringVal("projects/proj-a/zones/us-west4-a/reservations/link-r")})),
			stdInputs(), nil, []string{"proj-a/us-west4-a/reservations/link-r"}},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "reservation_name": cty.StringVal("shared-var"), "other_reservation": cty.StringVal("foo")},
			testMod("spot", map[string]cty.Value{"spot": cty.True, "reservation": varRes, "gpu_reservation": cty.StringVal("projects/proj-b/reservations/foo"), "reservation_affinity": affinityVal("SPECIFIC_RESERVATION", specResEntry("foo", "proj-b"))}),
			testMod("od", map[string]cty.Value{"zone": cty.StringVal("us-west1-b"), "reservation_name": varRes}),
			testMod("proj-override", map[string]cty.Value{"project_id": cty.StringVal("proj-c"), "reservation_name": cty.StringVal("c-res")})),
			stdInputs(), nil, []string{"proj-a/us-west1-b/reservations/shared-var", "proj-c/us-central1-a/reservations/c-res", "proj-a/us-central1-a/reservations/foo"}},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "reservation_name": cty.StringVal("projects/shared-p/reservations/r1"), "gpu_reservation": cty.StringVal("blk-r/reservationBlocks/b1")},
			testMod("east", map[string]cty.Value{"zone": cty.StringVal("us-east4-a"), "reservation_name": varRes}),
			testMod("blk-spot", map[string]cty.Value{"spot": cty.True, "reservation": config.GlobalRef("gpu_reservation").AsValue()})),
			stdInputs(), nil, []string{"shared-p/us-east4-a/reservations/r1"}},
		{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "reservation_name": cty.StringVal("shared-r"), "gpu_reservation": cty.StringVal("spot-shared"),
			"aff": affinityVal("SPECIFIC_RESERVATION", cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("dyn-aff"), "project": unknownRef})), "dyn_aff_reservation": cty.StringVal("dyn-aff")},
			testMod("c-mod", map[string]cty.Value{"project_id": cty.StringVal("proj-c"), "zone": cty.StringVal("us-east4-a"), "reservation_name": varRes}),
			testMod("spot-aff", map[string]cty.Value{"spot": cty.True, "reservation_affinity": affinityVal("SPECIFIC_RESERVATION", cty.ObjectVal(map[string]cty.Value{"name": config.GlobalRef("gpu_reservation").AsValue(), "project": cty.StringVal("host-proj")}))}),
			testMod("var-aff", map[string]cty.Value{"reservation_affinity": config.GlobalRef("aff").AsValue()})),
			stdInputs(), nil, []string{"proj-c/us-east4-a/reservations/shared-r"}},
		{config.Blueprint{
			Vars: config.NewDict(baseVars),
			Groups: []config.Group{{Modules: []config.Module{
				testMod("spot", map[string]cty.Value{"spot": cty.True, "reservation_affinity": affinityVal("SPECIFIC_RESERVATION", specResEntry("shared-r", "wrong-proj"))}),
				testMod("act", map[string]cty.Value{"reservation_affinity": affinityVal("SPECIFIC_RESERVATION", specResEntry("shared-r", "owner-proj"))}),
			}}},
			Validators: []config.Validator{{Validator: testReservationExistsName}}},
			config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "reservation_name": cty.StringVal("shared-r")}), nil, []string{"owner-proj/us-central1-a/reservations/shared-r"}},
		{config.Blueprint{
			Vars:       config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "gpu_reservation": cty.StringVal("ignored")}),
			Groups:     []config.Group{{Modules: []config.Module{testMod("m", map[string]cty.Value{"reservation_name": cty.StringVal("ignored-m")})}}},
			Validators: []config.Validator{{Validator: testReservationExistsName}}},
			config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "reservation_name": cty.NullVal(cty.String)}), nil, nil},
	}
	withMockReservationServer(func(env *mockResEnv) {
		for _, tc := range cases {
			env.reset()
			for _, f := range tc.future {
				env.onlyFuture[f] = true
			}
			c.Assert(testReservationExists(tc.bp, tc.in), IsNil)
			c.Assert(env.queried, DeepEquals, tc.want)
			if len(tc.want) > 0 {
				c.Assert(env.serviceInits, Equals, 1)
			}
		}
	})
}

func (s *MySuite) TestReservationSkipGuardsAndErrors(c *C) {
	baseVars := map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "pm_spot": cty.StringVal("spot")}
	withMockReservationServer(func(env *mockResEnv) {
		skipCases := []struct {
			settings  map[string]cty.Value
			wantQuery bool
		}{
			{map[string]cty.Value{"spot": cty.True}, false},
			{map[string]cty.Value{"enable_spot_vm": cty.True}, false},
			{map[string]cty.Value{"preemptible": cty.True}, false},
			{map[string]cty.Value{"enable_flex_start": cty.True}, false},
			{map[string]cty.Value{"enable_queued_provisioning": cty.True}, false},
			{map[string]cty.Value{"provisioning_model": cty.StringVal("SPOT")}, false},
			{map[string]cty.Value{"provisioning_model": config.GlobalRef("pm_spot").AsValue()}, false},
			{map[string]cty.Value{"provisioning_model": cty.StringVal("RESERVATION_BOUND")}, true},
			{map[string]cty.Value{"provisioning_model": cty.StringVal("STANDARD")}, true},
			{map[string]cty.Value{"is_reservation_active": cty.False}, false},
			{map[string]cty.Value{"dws_flex": cty.ObjectVal(map[string]cty.Value{"max_run": cty.NumberIntVal(1)})}, false},
			{map[string]cty.Value{"dws_flex": cty.ObjectVal(map[string]cty.Value{"enabled": cty.NullVal(cty.Bool)})}, false},
			{map[string]cty.Value{"dws_flex": cty.ObjectVal(map[string]cty.Value{"enabled": cty.True})}, false},
			{map[string]cty.Value{"dws_flex": cty.EmptyObjectVal}, true},
			{map[string]cty.Value{"dws_flex": cty.ObjectVal(map[string]cty.Value{"enabled": cty.False})}, true},
			{map[string]cty.Value{"dws_flex": cty.ObjectVal(map[string]cty.Value{"enabled": cty.UnknownVal(cty.Bool)})}, true},
		}
		for _, sc := range skipCases {
			env.reset()
			sc.settings["reservation_name"] = cty.StringVal("guard-res")
			c.Assert(testReservationExists(testBP(baseVars, testMod("m", sc.settings)), stdInputs()), IsNil)
			c.Assert(len(env.queried) > 0, Equals, sc.wantQuery)
		}

		errCases := []struct {
			bp                config.Blueprint
			in                config.Dict
			notFound, pattern string
			wantHint          bool
		}{
			{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a")}, testMod("reg-pool", map[string]cty.Value{"reservation_name": cty.StringVal("r1")})), config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("proj-a")}), "", `module "reg-pool": .*no zone specified.*`, true},
			{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.NullVal(cty.String), "reservation_name": cty.StringVal("r1")}), config.NewDict(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.NullVal(cty.String)}), "", `vars\.reservation_name: .*no zone specified.*`, true},
			{testBP(baseVars, testMod("m-mis", map[string]cty.Value{"reservation_name": cty.StringVal("projects/proj-a/zones/us-west1-b/reservations/r1")})), stdInputs(), "", `module "m-mis": .*conflicts with configured zone.*`, true},
			{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "reservation_name": cty.StringVal("projects/proj-a/zones/us-west1-b/reservations/r1")}), stdInputs(), "", `vars\.reservation_name: .*conflicts with configured zone.*`, true},
			{testBP(baseVars, testMod("gke-bad", map[string]cty.Value{"reservation_affinity": affinityVal("SPECIFIC_RESERVATION", specResEntry("projects/p/reservations/r1", ""))})), stdInputs(), "", `module "gke-bad": .*must not contain a project path.*`, true},
			{testBP(baseVars, testMod("gke-404", map[string]cty.Value{"reservation_name": cty.StringVal("missing-m")})), stdInputs(), "missing-m", `module "gke-404": .*missing-m.*not found.*`, false},
			{testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "gpu_reservation": cty.StringVal("missing-v")}), stdInputs(), "missing-v", `vars\.gpu_reservation: .*missing-v.*not found.*`, false},
		}
		for _, ec := range errCases {
			env.reset()
			if ec.notFound != "" {
				env.notFound[ec.notFound] = true
			}
			err := testReservationExists(ec.bp, ec.in)
			if ec.wantHint {
				assertHintError(c, err, ec.pattern)
			} else {
				c.Assert(err, ErrorMatches, ec.pattern)
			}
		}

		env.reset()
		env.notFound["wrong-zone-res"] = true
		env.aggZone = "us-east1-b"
		assertHintError(c, testReservationExists(testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "reservation_name": cty.StringVal("wrong-zone-res")}), stdInputs()), `vars\.reservation_name: .*exists in project "proj-a", but in zone\(s\) \[us-east1-b\].*`)
		env.reset()
		env.notFound["denied-res"] = true
		env.aggStatus = http.StatusForbidden
		c.Assert(TestReservationExists(context.Background(), "shared-proj", "us-central1-a", "denied-res", "proj-a"), IsNil)
		c.Assert(TestReservationExists(context.Background(), "proj-a", "us-central1-a", "denied-res", "proj-a"), ErrorMatches, `.*lacks permission to search other zones.*`)
		env.reset()
		env.notFound = map[string]bool{"miss-a": true, "miss-b": true}
		multiErr := testReservationExists(testBP(map[string]cty.Value{"project_id": cty.StringVal("proj-a"), "zone": cty.StringVal("us-central1-a"), "gpu_reservation": cty.StringVal("miss-b")}, testMod("mod-a", map[string]cty.Value{"reservation_name": cty.StringVal("miss-a")})), stdInputs())
		c.Assert(multiErr, ErrorMatches, `(?s).*module "mod-a": .*miss-a.*vars\.gpu_reservation: .*miss-b.*`)
	})

	affR1 := affinityVal("SPECIFIC_RESERVATION", specResEntry("r1", ""))
	ownerBP := testBP(baseVars,
		testMod("spot", map[string]cty.Value{"spot": cty.True, "project_id": cty.StringVal("spot-proj"), "reservation_affinity": affR1}),
		testMod("unk", map[string]cty.Value{"project_id": config.ModuleRef("h", "p").AsValue(), "reservation_affinity": affR1}),
		testMod("nullp", map[string]cty.Value{"project_id": cty.NullVal(cty.String), "reservation_affinity": affR1}),
		testMod("m", map[string]cty.Value{"project_id": cty.StringVal("mod-proj"), "reservation_affinity": affR1}))
	c.Assert(findReservationOwnerProject(ownerBP, "r1"), Equals, "mod-proj")
	c.Assert(evaluateAndFlatten(cty.NilVal), DeepEquals, []cty.Value{cty.NilVal})
	c.Assert(ValuesMatch(evaluateAndFlatten(cty.NilVal), evaluateAndFlatten(cty.NullVal(cty.String))), Equals, true)
	unkMarked := cty.UnknownVal(cty.String).Mark("m")
	c.Assert(len(evaluateAndFlatten(cty.SetVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}))), Equals, 2)
	flatTup := evaluateAndFlatten(cty.TupleVal([]cty.Value{cty.StringVal("a").Mark("m"), unkMarked}))
	c.Assert(!flatTup[0].IsMarked() && flatTup[1].IsMarked(), Equals, true)
	_, ok := asObjectMap(cty.NullVal(cty.EmptyObject).Mark("m"))
	c.Assert(ok, Equals, false)
	c.Assert(len(asSequenceSlice(cty.SetVal([]cty.Value{cty.StringVal("a"), unkMarked}).Mark("m"))), Equals, 2)
	knownSeq := asSequenceSlice(cty.ListVal([]cty.Value{cty.StringVal("a").Mark("m"), cty.StringVal("b")}))
	c.Assert(len(knownSeq) == 2 && cap(knownSeq) == 2, Equals, true)
	_, err := inputsAsStrings(config.NewDict(map[string]cty.Value{"k": cty.NullVal(cty.String)}))
	c.Assert(err, ErrorMatches, `.*known non-null string.*`)
	c.Assert(testReservationExists(testBP(baseVars), config.NewDict(map[string]cty.Value{"project_id": cty.True, "zone": cty.StringVal("z")})), ErrorMatches, `.*must be a string.*`)
}
