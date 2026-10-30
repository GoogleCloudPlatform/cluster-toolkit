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

package config

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"hpc-toolkit/pkg/subblockmap"

	"github.com/zclconf/go-cty/cty"
	"google.golang.org/api/googleapi"
)

const (
	blk1 = "my-res-block-0001"
	blk2 = "my-res-block-0002"
)

func subPath(block string, n int) string {
	return fmt.Sprintf("my-res/reservationBlocks/%s/reservationSubBlocks/%s-subblock-%04d", block, block, n)
}

func sub(block string, n int, inUse bool) subblockmap.SubBlock {
	s := subblockmap.SubBlock{Block: block, Name: fmt.Sprintf("%s-subblock-%04d", block, n), Path: subPath(block, n), Count: 18, Healthy: true}
	if inUse {
		s.InUse = 18
	}
	return s
}

// fakeLister replaces the Compute Engine API. It returns the number of calls.
func fakeLister(t *testing.T, err error, subs ...subblockmap.SubBlock) *int {
	t.Helper()
	calls := new(int)
	orig := listReservationSubBlocks
	listReservationSubBlocks = func(subblockmap.Reservation) ([]subblockmap.SubBlock, error) {
		*calls++
		return subs, err
	}
	t.Cleanup(func() { listReservationSubBlocks = orig })
	return calls
}

// fakeIO records confirmations and saves.
type fakeIO struct {
	answer    bool
	questions []string
	saved     map[string]map[string]string
}

func (f *fakeIO) io() *SubblockIO {
	return &SubblockIO{
		File: "deployment.yaml",
		Confirm: func(q string) (bool, error) {
			f.questions = append(f.questions, q)
			return f.answer, nil
		},
		Save: func(v string, m map[string]string) (string, error) {
			if f.saved == nil {
				f.saved = map[string]map[string]string{}
			}
			f.saved[v] = m
			return "deployment.yaml", nil
		},
	}
}

func objOf(m map[string]string) cty.Value {
	if len(m) == 0 {
		return cty.EmptyObjectVal
	}
	attrs := map[string]cty.Value{}
	for k, v := range m {
		attrs[k] = cty.StringVal(v)
	}
	return cty.ObjectVal(attrs)
}

func pinBlueprint(saved map[string]string, extraVars map[string]cty.Value) Blueprint {
	vars := map[string]cty.Value{
		"project_id":            cty.StringVal("consumer"),
		"zone":                  cty.StringVal("us-central1-b"),
		"reservation":           cty.StringVal("my-res"),
		"reservation_subblocks": objOf(saved),
	}
	for k, v := range extraVars {
		vars[k] = v
	}
	return Blueprint{Vars: NewDict(vars)}
}

// pinModule is a gke-node-pool module with pin_to_subblocks: true whose map comes
// from mapSetting (normally $(vars.reservation_subblocks)).
func pinModule(id string, n int, mapSetting cty.Value) *Module {
	first := map[string]cty.Value{
		"name":             GlobalRef("reservation").AsValue(),
		pinToSubblocksAttr: cty.True,
	}
	if mapSetting != cty.NilVal {
		first[reservationSubblocksAttr] = mapSetting
	}
	settings := map[string]cty.Value{
		"zones": cty.TupleVal([]cty.Value{GlobalRef("zone").AsValue()}),
		reservationAffinitySetting: cty.ObjectVal(map[string]cty.Value{
			"consume_reservation_type": cty.StringVal("SPECIFIC_RESERVATION"),
			"specific_reservations":    cty.TupleVal([]cty.Value{cty.ObjectVal(first)}),
		}),
	}
	if n > 0 {
		settings["num_node_pools"] = cty.NumberIntVal(int64(n))
	}
	return &Module{ID: ModuleID(id), Settings: NewDict(settings)}
}

func varRef(name string) cty.Value { return GlobalRef(name).AsValue() }

func varMap(t *testing.T, bp Blueprint, name string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range bp.Vars.Get(name).AsValueMap() {
		out[k] = v.AsString()
	}
	return out
}

func keys(m map[string]string) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

func TestFirstDeployProposesConfirmsAndSaves(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, true), sub(blk1, 2, false), sub(blk1, 3, false), sub(blk2, 1, false))
	bp := pinBlueprint(nil, nil)
	f := &fakeIO{answer: true}
	bp.SetSubblockIO(f.io())
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 2, varRef("reservation_subblocks"))}); err != nil {
		t.Fatal(err)
	}
	if len(f.questions) != 1 || keys(f.saved["reservation_subblocks"]) != "01-0002,01-0003" {
		t.Errorf("questions=%v saved=%v", f.questions, f.saved)
	}
	if keys(varMap(t, bp, "reservation_subblocks")) != "01-0002,01-0003" {
		t.Errorf("Vars not updated: %v", varMap(t, bp, "reservation_subblocks"))
	}
}

func TestMatchingMapIsUsedWithoutPromptAndChecked(t *testing.T) {
	calls := fakeLister(t, nil, sub(blk1, 1, true), sub(blk1, 2, true))
	saved := map[string]string{"01-0001": subPath(blk1, 1), "01-0002": subPath(blk1, 2)}
	bp := pinBlueprint(saved, nil)
	f := &fakeIO{answer: true}
	bp.SetSubblockIO(f.io())
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 2, varRef("reservation_subblocks"))}); err != nil {
		t.Fatal(err)
	}
	if len(f.questions) != 0 || f.saved != nil || *calls != 1 {
		t.Errorf("questions=%v saved=%v calls=%d; want no prompt, no save, one read-only check", f.questions, f.saved, *calls)
	}
}

func TestGrowAndShrink(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, true), sub(blk1, 2, false), sub(blk1, 3, true))
	saved := map[string]string{"01-0001": subPath(blk1, 1), "01-0003": subPath(blk1, 3)}

	bp := pinBlueprint(saved, nil)
	f := &fakeIO{answer: true}
	bp.SetSubblockIO(f.io())
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 3, varRef("reservation_subblocks"))}); err != nil {
		t.Fatal(err)
	}
	if keys(f.saved["reservation_subblocks"]) != "01-0001,01-0002,01-0003" {
		t.Errorf("grow saved %v", f.saved)
	}

	bp = pinBlueprint(saved, nil)
	f = &fakeIO{answer: true}
	bp.SetSubblockIO(f.io())
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 1, varRef("reservation_subblocks"))}); err != nil {
		t.Fatal(err)
	}
	if keys(f.saved["reservation_subblocks"]) != "01-0001" {
		t.Errorf("shrink saved %v, want the highest key dropped", f.saved)
	}
}

func TestNoStopsWithoutSaving(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, false))
	bp := pinBlueprint(nil, nil)
	f := &fakeIO{answer: false}
	bp.SetSubblockIO(f.io())
	err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 1, varRef("reservation_subblocks"))})
	if err == nil || !strings.Contains(err.Error(), "not saved") || f.saved != nil {
		t.Errorf("err=%v saved=%v", err, f.saved)
	}
}

func TestExpandIsReadOnly(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, false))
	bp := pinBlueprint(nil, nil) // no SubblockIO, like gcluster expand
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 1, varRef("reservation_subblocks"))}); err != nil {
		t.Fatal(err)
	}
	if keys(varMap(t, bp, "reservation_subblocks")) != "01-0001" {
		t.Error("expand should show the proposal in its output")
	}
}

func TestLiteralMapMismatchStops(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, false), sub(blk1, 2, false))
	bp := pinBlueprint(nil, nil)
	bp.SetSubblockIO((&fakeIO{answer: true}).io())
	lit := objOf(map[string]string{"01-0001": subPath(blk1, 1)})
	err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 2, lit)})
	if err == nil || !strings.Contains(err.Error(), "comes from a variable") {
		t.Errorf("want literal-map error, got %v", err)
	}
}

func TestMissingSettingStopsWithInstructions(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, false))
	bp := pinBlueprint(nil, nil)
	err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 1, cty.NilVal)})
	if err == nil || !strings.Contains(err.Error(), "reservation_subblocks: $(vars.reservation_subblocks)") {
		t.Errorf("want instructions, got %v", err)
	}
}

func TestTwoModulesGetDifferentSubBlocks(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, false), sub(blk1, 2, false), sub(blk1, 3, false))
	bp := pinBlueprint(nil, map[string]cty.Value{"train_subblocks": cty.EmptyObjectVal, "infer_subblocks": cty.EmptyObjectVal})
	f := &fakeIO{answer: true}
	bp.SetSubblockIO(f.io())
	err := bp.resolvePinnedModules([]*Module{
		pinModule("train", 2, varRef("train_subblocks")),
		pinModule("infer", 1, varRef("infer_subblocks")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys(f.saved["train_subblocks"]) != "01-0001,01-0002" || keys(f.saved["infer_subblocks"]) != "01-0003" {
		t.Errorf("saved %v", f.saved)
	}
}

func TestSameSubBlockInTwoMapsIsError(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, true))
	m := cty.ObjectVal(map[string]cty.Value{"01-0001": cty.StringVal(subPath(blk1, 1))})
	bp := pinBlueprint(nil, map[string]cty.Value{"a": m, "b": m})
	err := bp.resolvePinnedModules([]*Module{pinModule("train", 1, varRef("a")), pinModule("infer", 1, varRef("b"))})
	if err == nil || !strings.Contains(err.Error(), `"train" and "infer"`) {
		t.Errorf("want duplicate error, got %v", err)
	}
}

func TestSavedSubBlockMissingPromptsAndUnhealthyWarns(t *testing.T) {
	unhealthy := sub(blk1, 1, true)
	unhealthy.Healthy = false
	fakeLister(t, nil, unhealthy) // 01-0002 is gone
	saved := map[string]string{"01-0001": subPath(blk1, 1), "01-0002": subPath(blk1, 2)}

	bp := pinBlueprint(saved, nil)
	f := &fakeIO{answer: false}
	bp.SetSubblockIO(f.io())
	err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 2, varRef("reservation_subblocks"))})
	if err == nil || len(f.questions) != 1 || !strings.Contains(f.questions[0], "01-0002") || f.saved != nil {
		t.Errorf("err=%v questions=%v saved=%v", err, f.questions, f.saved)
	}

	bp = pinBlueprint(saved, nil)
	f = &fakeIO{answer: true}
	bp.SetSubblockIO(f.io())
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 2, varRef("reservation_subblocks"))}); err != nil || f.saved != nil {
		t.Errorf("continue: err=%v saved=%v", err, f.saved)
	}
}

func TestPermissionErrors(t *testing.T) {
	forbidden := &googleapi.Error{Code: http.StatusForbidden, Message: "denied"}

	fakeLister(t, forbidden)
	bp := pinBlueprint(nil, nil)
	bp.SetSubblockIO((&fakeIO{answer: true}).io())
	err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 1, varRef("reservation_subblocks"))})
	if err == nil || !strings.Contains(err.Error(), "To deploy now: add the missing entries to reservation_subblocks") {
		t.Errorf("proposal without permission: got %v", err)
	}

	saved := map[string]string{"01-0001": subPath(blk1, 1)}
	bp = pinBlueprint(saved, nil)
	bp.SetSubblockIO((&fakeIO{answer: true}).io())
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 1, varRef("reservation_subblocks"))}); err != nil {
		t.Errorf("complete map without permission must work, got %v", err)
	}
}

func TestNumNodePoolsFromDeploymentVariable(t *testing.T) {
	fakeLister(t, nil, sub(blk1, 1, false), sub(blk1, 2, false))
	bp := pinBlueprint(nil, map[string]cty.Value{"num_node_pools": cty.NumberIntVal(2)})
	f := &fakeIO{answer: true}
	bp.SetSubblockIO(f.io())
	if err := bp.resolvePinnedModules([]*Module{pinModule("gpu-sb", 0, varRef("reservation_subblocks"))}); err != nil {
		t.Fatal(err)
	}
	if len(f.saved["reservation_subblocks"]) != 2 {
		t.Errorf("saved %v, want 2 entries from vars.num_node_pools", f.saved)
	}
}

func TestPinOffIsUntouched(t *testing.T) {
	calls := fakeLister(t, errors.New("must not be called"))
	bp := pinBlueprint(nil, nil)
	m := pinModule("gpu-sb", 1, varRef("reservation_subblocks"))
	ra := m.Settings.Get(reservationAffinitySetting).AsValueMap()
	first := ra["specific_reservations"].Index(cty.NumberIntVal(0)).AsValueMap()
	first[pinToSubblocksAttr] = cty.False
	ra["specific_reservations"] = cty.TupleVal([]cty.Value{cty.ObjectVal(first)})
	m.Settings = m.Settings.With(reservationAffinitySetting, cty.ObjectVal(ra))
	if err := bp.resolvePinnedModules([]*Module{m}); err != nil || *calls != 0 {
		t.Errorf("err=%v calls=%d", err, *calls)
	}
}

func TestSupportsSubblockPinning(t *testing.T) {
	gkeType := cty.Object(map[string]cty.Type{
		"consume_reservation_type": cty.String,
		"specific_reservations": cty.List(cty.Object(map[string]cty.Type{
			"name": cty.String, "project": cty.String,
			reservationSubblocksAttr: cty.Map(cty.String), pinToSubblocksAttr: cty.Bool,
		})),
	})
	otherType := cty.Object(map[string]cty.Type{
		"consume_reservation_type": cty.String,
		"specific_reservations":    cty.List(cty.Object(map[string]cty.Type{"name": cty.String})),
	})
	if !supportsSubblockPinning(gkeType) || supportsSubblockPinning(otherType) || supportsSubblockPinning(cty.String) {
		t.Error("supportsSubblockPinning gave the wrong answer")
	}
}

func TestMockSubBlocks(t *testing.T) {
	subs, err := mockSubBlocks(subblockmap.Reservation{Name: "my-res"},
		`["my-res-block-0001/my-res-block-0001-subblock-0003", "my-res-block-0001/my-res-block-0001-subblock-0004=used", "my-res-block-0001/my-res-block-0001-subblock-0005=unhealthy"]`)
	if err != nil || len(subs) != 3 || subs[0].Path != subPath(blk1, 3) || !subs[0].Free() || subs[1].Free() || subs[2].Healthy {
		t.Errorf("mockSubBlocks = %+v, %v", subs, err)
	}
	if _, err := mockSubBlocks(subblockmap.Reservation{Name: "my-res"}, "forbidden"); !subblockmap.IsPermissionError(err) {
		t.Errorf("forbidden mock: %v", err)
	}
}
