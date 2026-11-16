// Copyright 2026 Google LLC
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

package subblockmap

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"gopkg.in/yaml.v3"
)

const res = "my-res"

var (
	b1  = res + "-block-0001"
	b2  = res + "-block-0002"
	rsv = Reservation{Project: "owner", Zone: "us-central1-a", Name: res}
)

type fakeLister struct {
	blocks   map[string][]*compute.ReservationSubBlock
	err      error
	projects []string
}

func (f *fakeLister) Blocks(_ context.Context, project, _, _ string) ([]string, error) {
	f.projects = append(f.projects, project)
	if f.err != nil {
		return nil, f.err
	}
	var names []string
	for n := range f.blocks {
		names = append(names, n)
	}
	return names, nil
}

func (f *fakeLister) SubBlocks(_ context.Context, project, _, _, block string) ([]*compute.ReservationSubBlock, error) {
	f.projects = append(f.projects, project)
	if f.err != nil {
		return nil, f.err
	}
	return f.blocks[block], nil
}

func sb(block string, n int, status, health string) *compute.ReservationSubBlock {
	return &compute.ReservationSubBlock{
		Name: fmt.Sprintf("%s-subblock-%04d", block, n), Count: 18, Status: status,
		HealthInfo: &compute.ReservationSubBlockHealthInfo{HealthStatus: health},
	}
}

func ok(block string, n int) *compute.ReservationSubBlock { return sb(block, n, "READY", "HEALTHY") }

func path(block string, n int) string {
	return fmt.Sprintf("%s/reservationBlocks/%s/reservationSubBlocks/%s-subblock-%04d", res, block, block, n)
}

func list(t *testing.T, l Lister) func() ([]SubBlock, error) {
	t.Helper()
	return func() ([]SubBlock, error) { return ListSubBlocks(context.Background(), l, rsv) }
}

func noList(t *testing.T) func() ([]SubBlock, error) {
	return func() ([]SubBlock, error) {
		t.Error("sub-blocks must not be listed when the previous map already has enough entries")
		return nil, nil
	}
}

func TestKey(t *testing.T) {
	for _, tc := range []struct{ block, sub, want string }{
		{"r-block-0002", "r-block-0002-subblock-0001", "02-0001"},
		{"r-block-0013", "r-block-0013-subblock-0018", "13-0018"},
	} {
		got, err := Key(tc.block, tc.sub)
		if err != nil || got != tc.want {
			t.Errorf("Key(%q, %q) = %q, %v; want %q", tc.block, tc.sub, got, err, tc.want)
		}
	}
	if _, err := Key("r-block-0100", "r-block-0100-subblock-0001"); err == nil {
		t.Error("block number above 99 must be an error")
	}
	if _, err := Key("rack", "rack-a"); err == nil {
		t.Error("unexpected names must be an error")
	}
}

func used(block string, n int) *compute.ReservationSubBlock {
	s := ok(block, n)
	s.InUseCount = 18
	return s
}

func keysOf(m map[string]string) string {
	return strings.Join(sortedKeys(m), ",")
}

func TestFirstDeployTakesLowestFreeHealthy(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{
		b1: {used(b1, 1), sb(b1, 2, "READY", "DEGRADED"), ok(b1, 3), ok(b1, 4)},
		b2: {ok(b2, 1)},
	}}
	sel, err := Select(nil, 3, res, nil, list(t, l))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"01-0003": path(b1, 3), "01-0004": path(b1, 4), "02-0001": path(b2, 1)}
	if !reflect.DeepEqual(sel.Map, want) || keysOf(sel.Map) != strings.Join(sel.Added, ",") {
		t.Errorf("map=%v added=%v", sel.Map, sel.Added)
	}
	for _, p := range l.projects {
		if p != "owner" {
			t.Errorf("queried project %q, want owner", p)
		}
	}
}

func TestSameNKeepsPreviousWithoutListing(t *testing.T) {
	prev := map[string]string{"01-0001": path(b1, 1), "01-0002": path(b1, 2)}
	sel, err := Select(prev, 2, res, nil, noList(t))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sel.Map, prev) || len(sel.Added)+len(sel.Removed) != 0 {
		t.Errorf("unexpected selection %+v", sel)
	}
}

func TestGrowKeepsPreviousEvenIfBusyOrUnhealthy(t *testing.T) {
	// The previous sub-blocks are in use by this deployment's own pools, and one is unhealthy.
	prev := map[string]string{"01-0001": path(b1, 1), "01-0002": path(b1, 2)}
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {
		used(b1, 1), {Name: fmt.Sprintf("%s-subblock-%04d", b1, 2), InUseCount: 18, Status: "READY",
			HealthInfo: &compute.ReservationSubBlockHealthInfo{HealthStatus: "DEGRADED"}}, ok(b1, 3), ok(b1, 4)}}}
	sel, err := Select(prev, 3, res, nil, list(t, l))
	if err != nil {
		t.Fatal(err)
	}
	if keysOf(sel.Map) != "01-0001,01-0002,01-0003" || strings.Join(sel.Added, ",") != "01-0003" || len(sel.Warnings) != 1 {
		t.Errorf("unexpected selection %+v", sel)
	}
}

func TestShrinkDropsHighestKeys(t *testing.T) {
	prev := map[string]string{"01-0001": path(b1, 1), "01-0002": path(b1, 2), "02-0001": path(b2, 1)}
	sel, err := Select(prev, 1, res, nil, noList(t))
	if err != nil {
		t.Fatal(err)
	}
	if keysOf(sel.Map) != "01-0001" || strings.Join(sel.Removed, ",") != "01-0002,02-0001" {
		t.Errorf("unexpected selection %+v", sel)
	}
}

func TestNotEnoughFreeSubBlocksIsError(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {used(b1, 1), ok(b1, 2)}}}
	if _, err := Select(nil, 2, res, nil, list(t, l)); err == nil || !strings.Contains(err.Error(), "1 available") {
		t.Errorf("want not-enough error, got %v", err)
	}
}

func TestPreviousOfOtherReservationIsDropped(t *testing.T) {
	prev := map[string]string{"01-0001": "old-res/reservationBlocks/old-res-block-0001/reservationSubBlocks/old-res-block-0001-subblock-0001"}
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {ok(b1, 1)}}}
	sel, err := Select(prev, 1, res, nil, list(t, l))
	if err != nil {
		t.Fatal(err)
	}
	if sel.Map["01-0001"] != path(b1, 1) || len(sel.Warnings) != 1 || strings.Join(sel.Removed, ",") != "01-0001" {
		t.Errorf("unexpected selection %+v", sel)
	}
}

// Three clusters share one reservation of 10 sub-blocks. Each cluster's previous map
// comes from its own deployment file.
func TestThreeClustersShareReservation(t *testing.T) {
	state := map[int]bool{} // sub-block number -> in use
	lister := func() *fakeLister {
		var subs []*compute.ReservationSubBlock
		for n := 1; n <= 10; n++ {
			if state[n] {
				subs = append(subs, used(b1, n))
			} else {
				subs = append(subs, ok(b1, n))
			}
		}
		return &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: subs}}
	}
	deploy := func(prev map[string]string, n int) map[string]string {
		t.Helper()
		sel, err := Select(prev, n, res, nil, list(t, lister()))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range sel.Map {
			var num int
			if _, err := fmt.Sscanf(p[strings.LastIndex(p, "-")+1:], "%d", &num); err != nil {
				t.Fatal(err)
			}
			state[num] = true
		}
		return sel.Map
	}

	c1 := deploy(nil, 2)
	c2 := deploy(nil, 7)
	if keysOf(c1) != "01-0001,01-0002" || keysOf(c2) != "01-0003,01-0004,01-0005,01-0006,01-0007,01-0008,01-0009" {
		t.Fatalf("c1=%v c2=%v", keysOf(c1), keysOf(c2))
	}
	state[1], state[2] = false, false // cluster 1 destroyed
	c3 := deploy(nil, 3)
	if keysOf(c3) != "01-0001,01-0002,01-0010" {
		t.Errorf("cluster 3 got %v, want 01-0001,01-0002,01-0010", keysOf(c3))
	}
	// Re-creating cluster 2 keeps its sub-blocks.
	again := deploy(c2, 7)
	if !reflect.DeepEqual(again, c2) {
		t.Errorf("cluster 2 moved: %v -> %v", keysOf(c2), keysOf(again))
	}
}

func TestUnexpectedSubBlockNameIsError(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {{Name: "rack-a", Status: "READY"}}}}
	if _, err := Select(nil, 1, res, nil, list(t, l)); err == nil || !strings.Contains(err.Error(), "cannot build a key") {
		t.Errorf("want key error, got %v", err)
	}
}

func TestZeroPoolsIsError(t *testing.T) {
	if _, err := Select(nil, 0, res, nil, noList(t)); err == nil {
		t.Error("num_node_pools 0 must be an error")
	}
}

func TestPermissionErrorGivesHint(t *testing.T) {
	l := &fakeLister{err: &googleapi.Error{Code: 403, Message: "Required 'compute.reservationBlocks.list' permission"}}
	_, err := Select(nil, 1, res, nil, list(t, l))
	if err == nil || !strings.Contains(err.Error(), "hint:") || !strings.Contains(err.Error(), "roles/compute.viewer") || !strings.Contains(err.Error(), `"owner"`) {
		t.Errorf("want permission hint, got %v", err)
	}
}

func TestOtherErrorHasNoHint(t *testing.T) {
	l := &fakeLister{err: &googleapi.Error{Code: 404, Message: "not found"}}
	_, err := Select(nil, 1, res, nil, list(t, l))
	if err == nil || strings.Contains(err.Error(), "hint:") || !strings.Contains(err.Error(), `reservation "my-res"`) {
		t.Errorf("want plain error naming the reservation, got %v", err)
	}
}

func TestNoBlocksIsError(t *testing.T) {
	_, err := Select(nil, 1, res, nil, list(t, &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{}}))
	if err == nil || !strings.Contains(err.Error(), "no blocks") {
		t.Errorf("want no-blocks error, got %v", err)
	}
}

// Block-only dense reservations (blocks without sub-blocks) get a clear error
// instead of "0 available".
func TestBlocksWithoutSubBlocksIsError(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {}, b2: nil}}
	_, err := Select(nil, 1, res, nil, list(t, l))
	if err == nil || !strings.Contains(err.Error(), "has 2 blocks but no sub-blocks") || strings.Contains(err.Error(), "available") {
		t.Errorf("want blocks-without-sub-blocks error, got %v", err)
	}
}

func TestExcludeSkipsOtherModulesSubBlocks(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {ok(b1, 1), ok(b1, 2), ok(b1, 3)}}}
	sel, err := Select(nil, 2, res, map[string]bool{path(b1, 1): true}, list(t, l))
	if err != nil {
		t.Fatal(err)
	}
	if keysOf(sel.Map) != "01-0002,01-0003" {
		t.Errorf("got %v, want 01-0002,01-0003", keysOf(sel.Map))
	}
}

const deploymentText = `# Example deployment
terraform_backend_defaults:
  type: gcs

vars:
  project_id: my-project  # project
  reservation: my-res
%s
  zone: us-central1-a

# trailing comment
`

func TestUpdateVarKeepsRestOfFile(t *testing.T) {
	want := map[string]string{"01-0001": path(b1, 1), "02-0001": path(b2, 1)}
	for name, block := range map[string]string{
		"absent":   "",
		"empty":    "  reservation_subblocks: {}  # filled by gcluster",
		"existing": "  reservation_subblocks:\n    \"01-0009\": " + path(b1, 9),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := UpdateVar(fmt.Sprintf(deploymentText, block), "reservation_subblocks", want)
			if err != nil {
				t.Fatal(err)
			}
			for _, keep := range []string{"# Example deployment", "# project", "# trailing comment", "type: gcs", "zone: us-central1-a"} {
				if !strings.Contains(out, keep) {
					t.Errorf("lost %q:\n%s", keep, out)
				}
			}
			var doc struct {
				Vars map[string]interface{} `yaml:"vars"`
			}
			if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			got, _ := doc.Vars["reservation_subblocks"].(map[string]interface{})
			if len(got) != 2 || got["01-0001"] != path(b1, 1) || got["02-0001"] != path(b2, 1) || doc.Vars["zone"] != "us-central1-a" {
				t.Errorf("unexpected vars %v\n%s", doc.Vars, out)
			}
		})
	}
}

func TestUpdateVarNeedsVars(t *testing.T) {
	if _, err := UpdateVar("terraform_backend_defaults: {}\n", "x", map[string]string{"a": "b"}); err == nil {
		t.Error("missing vars block must be an error")
	}
}

func TestDiff(t *testing.T) {
	d := Diff("a\nb\nc\n", "a\nB\nc\n", "old", "new")
	if !strings.Contains(d, "-b\n") || !strings.Contains(d, "+B\n") || !strings.HasPrefix(d, "--- old\n+++ new\n") {
		t.Errorf("unexpected diff:\n%s", d)
	}
	if Diff("same\n", "same\n", "a", "b") != "" {
		t.Error("identical input must give an empty diff")
	}
}

func degraded(block string, n int) *compute.ReservationSubBlock {
	return sb(block, n, "READY", "DEGRADED")
}

func TestListSubBlocksHealth(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {
		ok(b1, 1), degraded(b1, 2), sb(b1, 3, "READY", "HEALTH_STATUS_UNSPECIFIED"), sb(b1, 4, "CREATING", "DEGRADED"),
	}}}
	subs, err := ListSubBlocks(context.Background(), l, rsv)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ healthy, degraded bool }{{true, false}, {true, true}, {false, false}, {false, true}}
	for i, w := range want {
		if subs[i].Healthy != w.healthy || subs[i].Degraded != w.degraded {
			t.Errorf("sub-block %d: Healthy=%v Degraded=%v, want %v %v", i+1, subs[i].Healthy, subs[i].Degraded, w.healthy, w.degraded)
		}
	}
}

func TestDegradedIsUsedAfterHealthyWithWarning(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {degraded(b1, 1), ok(b1, 2)}}}

	// One needed: the HEALTHY sub-block is taken although its key is higher.
	sel, err := Select(nil, 1, res, nil, list(t, l))
	if err != nil {
		t.Fatal(err)
	}
	if keysOf(sel.Map) != "01-0002" || len(sel.Warnings) != 0 {
		t.Errorf("unexpected selection %+v", sel)
	}

	// Two needed: the DEGRADED one is added too, with a warning.
	sel, err = Select(nil, 2, res, nil, list(t, l))
	if err != nil {
		t.Fatal(err)
	}
	if keysOf(sel.Map) != "01-0001,01-0002" || strings.Join(sel.Added, ",") != "01-0001,01-0002" ||
		len(sel.Warnings) != 1 || !strings.Contains(sel.Warnings[0], "01-0001") || !strings.Contains(sel.Warnings[0], "DEGRADED") {
		t.Errorf("unexpected selection %+v", sel)
	}
}

func TestOnlyDegradedSubBlockIsAccepted(t *testing.T) {
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {degraded(b1, 1)}}}
	sel, err := Select(nil, 1, res, nil, list(t, l))
	if err != nil {
		t.Fatal(err)
	}
	if sel.Map["01-0001"] != path(b1, 1) || len(sel.Warnings) != 1 {
		t.Errorf("unexpected selection %+v", sel)
	}
}

func TestBusyDegradedSubBlockIsNotAvailable(t *testing.T) {
	busy := degraded(b1, 1)
	busy.InUseCount = 1
	l := &fakeLister{blocks: map[string][]*compute.ReservationSubBlock{b1: {busy}}}
	if _, err := Select(nil, 1, res, nil, list(t, l)); err == nil || !strings.Contains(err.Error(), "0 available") {
		t.Errorf("want not-enough error, got %v", err)
	}
}
