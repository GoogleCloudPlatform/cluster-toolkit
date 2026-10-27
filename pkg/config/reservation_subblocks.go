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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strings"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/subblockmap"

	"github.com/zclconf/go-cty/cty"
	"google.golang.org/api/googleapi"
)

const (
	reservationAffinitySetting = "reservation_affinity"
	reservationSubblocksAttr   = "reservation_subblocks"
	pinToSubblocksAttr         = "pin_to_subblocks"
	// mockReservationSubblocksEnv holds a JSON list of "BLOCK/SUBBLOCK" names, each
	// optionally followed by "=used", "=degraded" or "=unhealthy", used instead of the Compute
	// Engine API (for tests).
	mockReservationSubblocksEnv = "GHPC_MOCK_RESERVATION_SUBBLOCKS"
)

// SubblockIO lets gcluster ask the user and save the sub-block map. It is set by
// the command (create/deploy). Without it (gcluster expand), proposals are only
// shown and nothing is saved.
type SubblockIO struct {
	// Confirm asks a yes/no question. An error stops the expansion.
	Confirm func(question string) (bool, error)
	// Save writes `varName: m` under vars: of the user's deployment file (or
	// blueprint) and returns the file that was changed.
	Save func(varName string, m map[string]string) (string, error)
	// File is the file Save writes to, for messages.
	File string
}

// SetSubblockIO enables saving sub-block maps chosen by gcluster.
func (bp *Blueprint) SetSubblockIO(io *SubblockIO) { bp.subblockIO = io }

// listReservationSubBlocks lists a reservation's sub-blocks. Overridden in tests.
var listReservationSubBlocks = func(r subblockmap.Reservation) ([]subblockmap.SubBlock, error) {
	if mock := os.Getenv(mockReservationSubblocksEnv); mock != "" {
		return mockSubBlocks(r, mock)
	}
	ctx := context.Background()
	l, err := subblockmap.NewComputeLister(ctx)
	if err != nil {
		return nil, err
	}
	return subblockmap.ListSubBlocks(ctx, l, r)
}

func mockSubBlocks(r subblockmap.Reservation, mock string) ([]subblockmap.SubBlock, error) {
	if mock == "forbidden" {
		return nil, &googleapi.Error{Code: http.StatusForbidden, Message: "mock: permission denied"}
	}
	var names []string
	if err := json.Unmarshal([]byte(mock), &names); err != nil {
		return nil, fmt.Errorf("%s: %w", mockReservationSubblocksEnv, err)
	}
	var out []subblockmap.SubBlock
	for _, n := range names {
		n, state, _ := strings.Cut(n, "=")
		block, sub, ok := strings.Cut(n, "/")
		if !ok {
			return nil, fmt.Errorf("%s: %q must be BLOCK/SUBBLOCK", mockReservationSubblocksEnv, n)
		}
		s := subblockmap.SubBlock{
			Block: block, Name: sub, Count: 18, Healthy: state != "unhealthy", Degraded: state == "degraded",
			Path: fmt.Sprintf("%s/reservationBlocks/%s/reservationSubBlocks/%s", r.Name, block, sub),
		}
		if state == "used" {
			s.InUse = 18
		}
		out = append(out, s)
	}
	return out, nil
}

// supportsSubblockPinning reports whether a reservation_affinity input type accepts
// specific_reservations[*].pin_to_subblocks and reservation_subblocks (gke-node-pool).
func supportsSubblockPinning(t cty.Type) bool {
	if !t.IsObjectType() || !t.HasAttribute("specific_reservations") {
		return false
	}
	sr := t.AttributeType("specific_reservations")
	if !sr.IsListType() {
		return false
	}
	et := sr.ElementType()
	return et.IsObjectType() && et.HasAttribute(pinToSubblocksAttr) && et.HasAttribute(reservationSubblocksAttr)
}

// pinnedModule is a gke-node-pool module with pin_to_subblocks: true.
type pinnedModule struct {
	mod         *Module
	reservation subblockmap.Reservation // project and zone may be empty if unknown
	n           int                     // num_node_pools
	current     map[string]string       // map from the blueprint / deployment file
	varName     string                  // variable the map comes from; "" if not a plain variable reference
	hasSetting  bool                    // the module sets reservation_subblocks at all
}

// resolveReservationSubblocks handles every gke-node-pool module whose reservation
// has pin_to_subblocks: true. The sub-block map is user configuration: the module's
// reservation_subblocks refers to a deployment variable, e.g.
// $(vars.reservation_subblocks). When the map has num_node_pools entries it is used
// as is. Otherwise gcluster proposes a new map (keep saved entries; add free, healthy
// sub-blocks with the lowest keys, or drop the highest keys), prints it and, after
// confirmation, saves it to the user's file and uses it.
func (bp *Blueprint) resolveReservationSubblocks() error {
	var mods []*Module
	bp.WalkModulesSafe(func(_ ModulePath, m *Module) {
		if !m.Settings.Has(reservationAffinitySetting) {
			return
		}
		for _, input := range m.InfoOrDie().Inputs {
			if input.Name == reservationAffinitySetting && supportsSubblockPinning(input.Type) {
				mods = append(mods, m)
				return
			}
		}
	})
	return bp.resolvePinnedModules(mods)
}

// resolvePinnedModules is resolveReservationSubblocks for modules whose
// reservation_affinity input supports pinning.
func (bp *Blueprint) resolvePinnedModules(mods []*Module) error {
	var pinned []*pinnedModule
	var errs []error
	for _, m := range mods {
		pm, err := bp.pinnedModule(m)
		if err != nil {
			errs = append(errs, err)
		} else if pm != nil {
			pinned = append(pinned, pm)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if len(pinned) == 0 {
		return nil
	}

	owner, err := subblockOwners(pinned)
	if err != nil {
		return err
	}

	for _, pm := range pinned {
		exclude := map[string]bool{}
		for p, o := range owner {
			if o != pm.mod.ID {
				exclude[p] = true
			}
		}
		proposed, err := bp.resolvePinnedModule(pm, exclude)
		if err != nil {
			return fmt.Errorf("module %q: %w", pm.mod.ID, err)
		}
		for _, p := range proposed {
			owner[p] = pm.mod.ID
		}
	}
	return nil
}

// subblockOwners maps each sub-block in the modules' maps to its module. A sub-block
// can be used by only one module.
func subblockOwners(pinned []*pinnedModule) (map[string]ModuleID, error) {
	owner := map[string]ModuleID{}
	for _, pm := range pinned {
		for _, k := range sortedKeys(pm.current) {
			p := pm.current[k]
			if o, ok := owner[p]; ok && o != pm.mod.ID {
				return nil, fmt.Errorf("sub-block %s is used by modules %q and %q; remove it from one map", p, o, pm.mod.ID)
			}
			owner[p] = pm.mod.ID
		}
	}
	return owner, nil
}

// pinnedModule returns the module's sub-block settings, or nil if pinning is off.
func (bp *Blueprint) pinnedModule(m *Module) (*pinnedModule, error) {
	if !m.Settings.Has(reservationAffinitySetting) {
		return nil, nil
	}
	raw := m.Settings.Get(reservationAffinitySetting)
	ev, ok := attemptEvalModuleInput(raw, *bp)
	if !ok {
		return nil, nil // e.g. uses a module output; the module rejects pin_to_subblocks with an empty map
	}
	first, ok := firstSpecificReservation(ev, true)
	if !ok {
		return nil, nil
	}
	if pin, ok := knownBoolAttr(first, pinToSubblocksAttr); !ok || !pin {
		return nil, nil
	}

	pm := &pinnedModule{mod: m, current: stringMapAttr(first, reservationSubblocksAttr)}
	var err error
	if pm.reservation, err = bp.pinnedReservation(m, first); err != nil {
		return nil, fmt.Errorf("module %q: %w", m.ID, err)
	}
	if pm.n, err = bp.numNodePools(m); err != nil {
		return nil, fmt.Errorf("module %q: %w", m.ID, err)
	}
	pm.varName, pm.hasSetting = mapSource(raw)
	if !pm.hasSetting {
		return nil, fmt.Errorf(`module %q has pin_to_subblocks: true but no reservation_subblocks setting. Add these lines to the blueprint and run again:
  vars:
    reservation_subblocks: {}
  in module %q, under reservation_affinity.specific_reservations[0]:
    reservation_subblocks: $(vars.reservation_subblocks)`, m.ID, m.ID)
	}
	return pm, nil
}

// pinnedReservation returns the reservation of a pinned module. Project and zone
// are empty if they aren't known.
func (bp *Blueprint) pinnedReservation(m *Module, first cty.Value) (subblockmap.Reservation, error) {
	name, _ := knownStringAttr(first, "name")
	if name == "" || strings.Contains(name, "/") {
		return subblockmap.Reservation{}, fmt.Errorf("%s requires reservation_affinity.specific_reservations[0].name to be the bare reservation name, got %q",
			pinToSubblocksAttr, name)
	}
	project, _ := knownStringAttr(first, "project")
	if project == "" {
		project = extractProject(m, *bp)
	}
	zone, err := extractSingleZone(m, *bp)
	if err != nil {
		return subblockmap.Reservation{}, err
	}
	return subblockmap.Reservation{Project: project, Zone: zone, Name: name}, nil
}

// mapSource inspects the unevaluated reservation_subblocks setting. varName is the
// variable it refers to if it is exactly $(vars.X), so gcluster can save to X.
func mapSource(raw cty.Value) (varName string, hasSetting bool) {
	first, ok := firstSpecificReservation(raw, false)
	if !ok || !first.Type().HasAttribute(reservationSubblocksAttr) {
		return "", false
	}
	v := first.GetAttr(reservationSubblocksAttr)
	if v.IsNull() {
		return "", false
	}
	expr, isExpr := IsExpressionValue(v)
	if !isExpr {
		return "", true
	}
	refs := expr.References()
	if len(refs) == 1 && refs[0].GlobalVar && strings.TrimSpace(string(expr.Tokenize().Bytes())) == "var."+refs[0].Name {
		return refs[0].Name, true
	}
	return "", true
}

// stringMapAttr returns the known string entries of a map or object attribute.
func stringMapAttr(obj cty.Value, attr string) map[string]string {
	out := map[string]string{}
	if !obj.Type().HasAttribute(attr) {
		return out
	}
	v := obj.GetAttr(attr)
	if v.IsNull() || !v.IsKnown() || (!v.Type().IsObjectType() && !v.Type().IsMapType()) {
		return out
	}
	for k, e := range v.AsValueMap() {
		if e.IsKnown() && !e.IsNull() && e.Type() == cty.String {
			out[k] = e.AsString()
		}
	}
	return out
}

// resolvePinnedModule uses, or proposes and saves, the module's map. It returns
// newly proposed sub-block paths.
func (bp *Blueprint) resolvePinnedModule(pm *pinnedModule, exclude map[string]bool) ([]string, error) {
	r := pm.reservation
	list := func() ([]subblockmap.SubBlock, error) {
		if r.Project == "" || r.Zone == "" {
			return nil, fmt.Errorf("%s needs a known project (specific_reservations[0].project or project_id) and zone (zones or zone)", pinToSubblocksAttr)
		}
		return listReservationSubBlocks(r)
	}

	if len(pm.current) == pm.n {
		printSubblockMap(pm.mod.ID, pm.current, nil, nil, "using saved sub-block map")
		return nil, bp.checkSavedSubblocks(pm, list)
	}

	if pm.varName == "" {
		return nil, fmt.Errorf("reservation_subblocks has %d entries but num_node_pools is %d, and gcluster can only update a map that comes from a variable "+
			"(e.g. $(vars.reservation_subblocks)). Edit the map, or refer to a variable", len(pm.current), pm.n)
	}

	sel, err := subblockmap.Select(pm.current, pm.n, r.Name, exclude, list)
	if err != nil {
		if subblockmap.IsPermissionError(err) {
			return nil, fmt.Errorf("cannot list sub-blocks of reservation %q in project %q (needs %s, e.g. roles/compute.viewer).\n"+
				"To deploy now: add the missing entries to %s in the deployment file (see the gke-node-pool README for the commands that generate them)",
				r.Name, r.Project, subblockmap.RequiredPermissions, pm.varName)
		}
		return nil, err
	}
	for _, w := range sel.Warnings {
		logging.Info("WARNING: module %q: %s", pm.mod.ID, w)
	}
	removed := map[string]string{}
	for _, k := range sel.Removed {
		removed[k] = pm.current[k]
	}
	printSubblockMap(pm.mod.ID, sel.Map, sel.Added, removed, "proposed sub-block map"+changeSummary(len(sel.Added), len(sel.Removed), len(sel.Kept)))

	if err := bp.saveProposal(pm, sel.Map); err != nil {
		return nil, err
	}

	attrs := map[string]cty.Value{}
	for k, v := range sel.Map {
		attrs[k] = cty.StringVal(v)
	}
	bp.Vars = bp.Vars.With(pm.varName, cty.ObjectVal(attrs))

	var proposed []string
	for _, k := range sel.Added {
		proposed = append(proposed, sel.Map[k])
	}
	return proposed, nil
}

// saveProposal asks the user and saves the proposed map. Without a SubblockIO
// (gcluster expand) it only warns that nothing is saved.
func (bp *Blueprint) saveProposal(pm *pinnedModule, m map[string]string) error {
	if bp.subblockIO == nil {
		logging.Info("WARNING: module %q: the proposed sub-block map is NOT saved; gcluster expand only shows it. Run gcluster create or deploy to review and save it.", pm.mod.ID)
		return nil
	}
	ok, err := bp.subblockIO.Confirm(fmt.Sprintf("Save these sub-blocks for module %q to %s?", pm.mod.ID, bp.subblockIO.File))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("sub-blocks not saved, so nothing was changed. Run again and confirm, or set %s yourself (YAML above)", pm.varName)
	}
	_, err = bp.subblockIO.Save(pm.varName, m)
	return err
}

// checkSavedSubblocks warns about saved sub-blocks that are unhealthy or no longer
// in the reservation. The map itself is never changed here.
func (bp *Blueprint) checkSavedSubblocks(pm *pinnedModule, list func() ([]subblockmap.SubBlock, error)) error {
	subs, err := list()
	if err != nil {
		logging.Info("module %q: could not check the saved sub-blocks (%v); using the saved sub-block map as is", pm.mod.ID, firstLine(err))
		return nil
	}
	byPath := map[string]subblockmap.SubBlock{}
	for _, s := range subs {
		byPath[s.Path] = s
	}
	var missing []string
	for _, k := range sortedKeys(pm.current) {
		s, ok := byPath[pm.current[k]]
		switch {
		case !ok:
			missing = append(missing, k)
			logging.Info("WARNING: module %q: saved sub-block (%q: %s) is no longer in reservation %q", pm.mod.ID, k, pm.current[k], pm.reservation.Name)
		case !s.Healthy:
			logging.Info("WARNING: module %q: saved sub-block (%q: %s) is not healthy", pm.mod.ID, k, pm.current[k])
		case s.Degraded:
			logging.Info("WARNING: module %q: saved sub-block (%q: %s) is DEGRADED: some of its hosts may be unhealthy, so fewer nodes may be created than requested", pm.mod.ID, k, pm.current[k])
		}
	}
	if len(missing) == 0 || bp.subblockIO == nil {
		return nil
	}
	ok, err := bp.subblockIO.Confirm(fmt.Sprintf("Continue with the saved sub-block map for module %q? (no new nodes can be created for %v until the entries are fixed)", pm.mod.ID, missing))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("stopped. To move a node pool to another sub-block, delete its entry from %s and run again (gcluster proposes a replacement); "+
			"to retire it, delete the entry and lower num_node_pools", pm.varName)
	}
	return nil
}

// printSubblockMap prints the map as YAML in one log entry. New entries are marked
// with a comment; removed entries are listed below the map.
func printSubblockMap(id ModuleID, m map[string]string, added []string, removed map[string]string, title string) {
	isNew := map[string]bool{}
	for _, k := range added {
		isNew[k] = true
	}
	lines := subblockmap.RenderVar(reservationSubblocksAttr, m, "  ")
	keys := sortedKeys(m)
	for i, k := range keys {
		if isNew[k] {
			lines[i+1] = strings.TrimSuffix(lines[i+1], "\n") + "  # new\n"
		}
	}
	out := fmt.Sprintf("module %q: %s:\n%s", id, title, strings.Join(lines, ""))
	if len(removed) > 0 {
		out += "  removed (their node pools will be deleted):\n"
		for _, k := range sortedKeys(removed) {
			out += fmt.Sprintf("    %q: %s\n", k, removed[k])
		}
	}
	logging.Info("%s", strings.TrimSuffix(out, "\n"))
}

// changeSummary returns e.g. " (1 new, 2 unchanged)", leaving out zero counts.
func changeSummary(added, removed, unchanged int) string {
	var parts []string
	for _, p := range []struct {
		n    int
		what string
	}{{added, "new"}, {removed, "removed"}, {unchanged, "unchanged"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.what))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func firstLine(err error) string {
	s, _, _ := strings.Cut(err.Error(), "\n")
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// numNodePools returns num_node_pools from the module settings, or from the
// deployment variable of the same name (which gcluster passes to the module when
// the module doesn't set it), or 1 (the module default).
func (bp *Blueprint) numNodePools(m *Module) (int, error) {
	var v cty.Value
	switch {
	case m.Settings.Has("num_node_pools"):
		v = m.Settings.Get("num_node_pools")
	case bp.Vars.Has("num_node_pools"):
		v = bp.Vars.Get("num_node_pools")
	default:
		return 1, nil
	}
	v, ok := attemptEvalModuleInput(v, *bp)
	if !ok || !v.IsKnown() || v.IsNull() || v.Type() != cty.Number {
		return 0, fmt.Errorf("%s requires a known number in num_node_pools", pinToSubblocksAttr)
	}
	bf := v.AsBigFloat()
	if !bf.IsInt() {
		return 0, fmt.Errorf("num_node_pools must be a whole number, got %s", bf.Text('g', -1))
	}
	i, acc := bf.Int64()
	if acc != big.Exact {
		return 0, errors.New("num_node_pools is out of range")
	}
	return int(i), nil
}

// firstSpecificReservation returns reservation_affinity.specific_reservations[0].
// With known=false it also accepts an unevaluated setting whose leaves may be
// expressions.
func firstSpecificReservation(v cty.Value, known bool) (cty.Value, bool) {
	usable := func(v cty.Value) bool { return !v.IsMarked() && (!known || v.IsKnown()) && !v.IsNull() }
	if !usable(v) || !v.Type().IsObjectType() || !v.Type().HasAttribute("specific_reservations") {
		return cty.NilVal, false
	}
	sr := v.GetAttr("specific_reservations")
	if !usable(sr) || (!sr.Type().IsListType() && !sr.Type().IsTupleType()) || sr.LengthInt() == 0 {
		return cty.NilVal, false
	}
	first := sr.Index(cty.NumberIntVal(0))
	if !usable(first) || !first.Type().IsObjectType() {
		return cty.NilVal, false
	}
	return first, true
}

func knownStringAttr(obj cty.Value, attr string) (string, bool) {
	if !obj.Type().HasAttribute(attr) {
		return "", false
	}
	v := obj.GetAttr(attr)
	if v.IsMarked() || !v.IsKnown() || v.IsNull() || v.Type() != cty.String {
		return "", false
	}
	return v.AsString(), true
}

func knownBoolAttr(obj cty.Value, attr string) (bool, bool) {
	if !obj.Type().HasAttribute(attr) {
		return false, false
	}
	v := obj.GetAttr(attr)
	if v.IsMarked() || !v.IsKnown() || v.IsNull() || v.Type() != cty.Bool {
		return false, false
	}
	return v.True(), true
}

// extractSingleZone returns the module's zone: the single entry of its zones
// setting, or zone settings and variables as for machine_configs.
func extractSingleZone(m *Module, bp Blueprint) (string, error) {
	if m.Settings.Has("zones") {
		if v, ok := attemptEvalModuleInput(m.Settings.Get("zones"), bp); ok && !v.IsNull() && v.IsKnown() && (v.Type().IsListType() || v.Type().IsTupleType()) {
			if v.LengthInt() != 1 {
				return "", fmt.Errorf("%s requires exactly one zone in zones, got %d", pinToSubblocksAttr, v.LengthInt())
			}
			z := v.Index(cty.NumberIntVal(0))
			if z.IsKnown() && !z.IsNull() && z.Type() == cty.String {
				return z.AsString(), nil
			}
		}
	}
	return extractZone(m, bp), nil
}
