// Copyright 2026 "Google LLC"
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
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"regexp"
	"sort"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

var resKeyRegex = regexp.MustCompile(`^(.*_)?reservation(_name)?$`)

var newComputeService = func(ctx context.Context) (*compute.Service, error) {
	return compute.NewService(ctx)
}

func getErrorReason(err googleapi.Error) (string, map[string]interface{}) {
	for _, d := range err.Details {
		m, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		if reason, ok := m["reason"].(string); ok {
			return reason, m["metadata"].(map[string]interface{})
		}
	}
	return "", nil
}

func newDisabledServiceError(title string, name string, pid string) error {
	return config.HintError{
		Hint: fmt.Sprintf("%s can be enabled at https://console.cloud.google.com/apis/library/%s?project=%s", title, name, pid),
		Err:  fmt.Errorf("%s service is disabled in project %s", title, pid)}
}

func handleServiceUsageError(err error, pid string) error {
	if err == nil {
		return nil
	}

	var herr *googleapi.Error
	if !errors.As(err, &herr) {
		return fmt.Errorf("unhandled error: %s", err)
	}

	reason, metadata := getErrorReason(*herr)
	switch reason {
	case "SERVICE_DISABLED":
		return newDisabledServiceError("Service Usage API", "serviceusage.googleapis.com", pid)
	case "SERVICE_CONFIG_NOT_FOUND_OR_PERMISSION_DENIED":
		return fmt.Errorf("service %s does not exist in project %s", metadata["services"], pid)
	case "USER_PROJECT_DENIED":
		return projectError(pid)
	case "SU_MISSING_NAMES":
		return nil // occurs if API list is empty and 0 APIs to validate
	}
	return fmt.Errorf("unhandled error: %s", herr)
}

func isValidatorExplicit(bp config.Blueprint, validatorName string) bool {
	for _, v := range bp.Validators {
		if v.Validator == validatorName {
			return true
		}
	}
	return false
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
		return handleServiceUsageError(err, projectID)
	}
	errs := config.Errors{}
	for _, service := range resp.Services {
		if service.State == "DISABLED" {
			errs.Add(newDisabledServiceError(service.Config.Title, service.Config.Name, projectID))
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
		if strings.Contains(err.Error(), "Compute Engine API has not been used in project") {
			return newDisabledServiceError("Compute Engine API", "compute.googleapis.com", projectID)
		}
		return projectError(projectID)
	}

	return nil
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

func testApisEnabled(bp config.Blueprint, inputs config.Dict) error {
	if err := checkInputs(inputs, []string{"project_id"}); err != nil {
		return err
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}
	apis := map[string]bool{}
	bp.WalkModulesSafe(func(_ config.ModulePath, m *config.Module) {
		services := m.InfoOrDie().Metadata.Spec.Requirements.Services
		for _, api := range services {
			apis[api] = true
		}
	})
	return TestApisEnabled(m["project_id"], maps.Keys(apis))
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

// Helper interface to treat Standard and Future reservations generically
type zoneResource interface {
	GetName() string
	GetZone() string
}

// Wrapper for compute.Reservation
type stdRes struct{ *compute.Reservation }

func (r stdRes) GetName() string { return r.Name }
func (r stdRes) GetZone() string { return r.Zone }

// Wrapper for compute.FutureReservation
type futRes struct{ *compute.FutureReservation }

func (r futRes) GetName() string { return r.Name }
func (r futRes) GetZone() string { return r.Zone }

func extractZonesFromItems[T any](items map[string]T, name string, extractor func(T) []zoneResource) []string {
	foundInZones := []string{}
	for _, scopedList := range items {
		for _, res := range extractor(scopedList) {
			if res.GetName() == name {
				parts := strings.Split(res.GetZone(), "/")
				foundInZones = append(foundInZones, parts[len(parts)-1])
			}
		}
	}
	return foundInZones
}

func findReservationInOtherZones(ctx context.Context, s *compute.Service, projectID string, name string) ([]string, error) {
	// 1. Search Standard Zonal Reservations
	aggList, err := s.Reservations.AggregatedList(projectID).Context(ctx).Do()
	if err == nil {
		found := extractZonesFromItems(aggList.Items, name, func(l compute.ReservationsScopedList) []zoneResource {
			res := make([]zoneResource, len(l.Reservations))
			for i, r := range l.Reservations {
				res[i] = stdRes{r}
			}
			return res
		})
		if len(found) > 0 {
			return found, nil
		}
	}

	// 2. Search Future Reservations (Early return if Standard found, otherwise search here)
	fAggList, fErr := s.FutureReservations.AggregatedList(projectID).Context(ctx).Do()
	if fErr == nil {
		found := extractZonesFromItems(fAggList.Items, name, func(l compute.FutureReservationsScopedList) []zoneResource {
			res := make([]zoneResource, len(l.FutureReservations))
			for i, r := range l.FutureReservations {
				res[i] = futRes{r}
			}
			return res
		})
		if len(found) > 0 {
			return found, nil
		}
	}

	// If discovery failed and we found nothing, preserve wrapped API errors.
	if err != nil || fErr != nil {
		return nil, fmt.Errorf("failed to list reservations: %w", errors.Join(err, fErr))
	}

	return []string{}, nil
}

// TestReservationExists checks if a reservation exists in a project and zone.
func TestReservationExists(ctx context.Context, reservationProjectID string, zone string, reservationName string, deploymentProjectID string) error {
	if reservationName == "" {
		return nil
	}
	s, err := newComputeService(ctx)
	if err != nil {
		return handleClientError(err)
	}
	return testReservationExistsWithService(ctx, s, reservationProjectID, zone, reservationName, deploymentProjectID)
}

func testReservationExistsWithService(ctx context.Context, s *compute.Service, reservationProjectID, zone, reservationName, deploymentProjectID string) error {
	if reservationName == "" {
		return nil
	}

	// 1. Direct check: Try Standard Zonal Reservation
	_, err := s.Reservations.Get(reservationProjectID, zone, reservationName).Context(ctx).Do()
	if err == nil {
		return nil
	}

	// 2. Fallback: Try Future Reservation (Required for Blackwell/A4 hardware)
	_, fErr := s.FutureReservations.Get(reservationProjectID, zone, reservationName).Context(ctx).Do()
	if fErr == nil {
		return nil
	}

	// 3. Access Check: If both failed, check for metadata blindness (403/400).
	if msg, isSoft := getSoftWarningMessage(err, "test_reservation_exists", reservationProjectID, "Compute Engine API", "compute.reservations.get"); isSoft {
		fmt.Println(msg)
		return nil
	}
	if msg, isSoft := getSoftWarningMessage(fErr, "test_reservation_exists", reservationProjectID, "Compute Engine API", "compute.futureReservations.get"); isSoft {
		fmt.Println(msg)
		return nil
	}

	// 4. Diagnostic Search: The reservation was not in the expected zone (404).
	foundInZones, aggErr := findReservationInOtherZones(ctx, s, reservationProjectID, reservationName)
	if aggErr != nil {
		if reservationProjectID != deploymentProjectID {
			fmt.Printf("\n[!] WARNING: Shared reservation %q was not found in zone %q.\n", reservationName, zone)
			fmt.Printf("    Discovery in other zones of project %q failed due to restricted permissions: %v\n", reservationProjectID, aggErr)
			fmt.Printf("    Skipping this check as consumption may still be possible.\n")
			return nil
		}
		var gerr *googleapi.Error
		if errors.As(aggErr, &gerr) && (gerr.Code == 403 || gerr.Code == 400) {
			return fmt.Errorf("reservation %q not found in zone %q (Note: identity lacks permission to search other zones)", reservationName, zone)
		}
		return fmt.Errorf("reservation %q not found in project %q and zone %q", reservationName, reservationProjectID, zone)
	}

	// 5. Resource Found Discovery: Provide Hint
	if len(foundInZones) > 0 {
		zonesList := strings.Join(foundInZones, ", ")
		return config.HintError{
			Err: fmt.Errorf("reservation %q exists in project %q, but in zone(s) [%s] instead of %q",
				reservationName, reservationProjectID, zonesList, zone),
			Hint: fmt.Sprintf("Change the zone in your blueprint to one of [%s], or use a reservation that is located in zone %q.",
				zonesList, zone),
		}
	}

	// 6. Not Found Anywhere: Hard Failure
	return fmt.Errorf("reservation %q was not found in any zone of project %q", reservationName, reservationProjectID)
}

type reservationTarget struct {
	ownerProject string
	zone         string
	name         string
	source       string
}

var skipReservationBoolSettings = []string{
	"spot", "enable_spot_vm", "preemptible", "enable_flex_start", "enable_queued_provisioning",
}

var computeAPIPrefixes = []string{
	"https://www.googleapis.com/compute/v1/", "https://www.googleapis.com/compute/beta/",
	"http://www.googleapis.com/compute/v1/", "http://www.googleapis.com/compute/beta/",
	"https://compute.googleapis.com/compute/v1/", "https://compute.googleapis.com/compute/beta/",
	"http://compute.googleapis.com/compute/v1/", "http://compute.googleapis.com/compute/beta/",
	"https://compute.googleapis.com/", "http://compute.googleapis.com/", "//compute.googleapis.com/",
	"www.googleapis.com/compute/v1/", "www.googleapis.com/compute/beta/",
	"compute.googleapis.com/compute/v1/", "compute.googleapis.com/compute/beta/",
	"compute.googleapis.com/", "/compute/v1/", "/compute/beta/", "compute/v1/", "compute/beta/",
}

func directGlobalRefName(val cty.Value) string {
	if val == cty.NilVal {
		return ""
	}
	e, ok := config.IsExpressionValue(val)
	if !ok {
		return ""
	}
	refs := e.References()
	if len(refs) != 1 || !refs[0].GlobalVar || strings.TrimSpace(string(e.Tokenize().Bytes())) != refs[0].String() {
		return ""
	}
	return refs[0].Name
}

func evalUnmarked(bp config.Blueprint, val cty.Value) cty.Value {
	if val == cty.NilVal {
		return cty.NilVal
	}
	if v, err := bp.Eval(val); err == nil && v != cty.NilVal {
		u, _ := v.Unmark()
		return u
	}
	limit := len(bp.Vars.Keys())
	for i := 0; i <= limit; i++ {
		name := directGlobalRefName(val)
		if name == "" || !bp.Vars.Has(name) {
			break
		}
		val = bp.Vars.Get(name)
	}
	u, _ := val.Unmark()
	return u
}

// evalTrimmedString evaluates val into (trimmedString, known).
func evalTrimmedString(bp config.Blueprint, val cty.Value) (string, bool) {
	v := evalUnmarked(bp, val)
	if v == cty.NilVal || !v.IsKnown() || v.IsNull() || v.Type() != cty.String {
		return "", v == cty.NilVal || v.IsKnown()
	}
	return strings.TrimSpace(v.AsString()), true
}

func resolveProject(bp config.Blueprint, val cty.Value, fallback string) string {
	s, known := evalTrimmedString(bp, val)
	if !known || s != "" {
		return s
	}
	return fallback
}

func asObjectMap(val cty.Value) (map[string]cty.Value, bool) {
	if val == cty.NilVal {
		return nil, false
	}
	u, _ := val.Unmark()
	if !u.IsKnown() || u.IsNull() || (!u.Type().IsObjectType() && !u.Type().IsMapType()) {
		return nil, false
	}
	if u.LengthInt() == 0 {
		return map[string]cty.Value{}, true
	}
	return u.AsValueMap(), true
}

func asSequenceSlice(val cty.Value) []cty.Value {
	if val == cty.NilVal {
		return nil
	}
	u, _ := val.Unmark()
	if !u.IsKnown() || u.IsNull() || (!u.Type().IsTupleType() && !u.Type().IsListType() && !u.Type().IsSetType()) {
		return nil
	}
	var out []cty.Value
	if u.Length().IsKnown() { // cty.Set length is unknown when it contains an unknown element
		out = make([]cty.Value, 0, u.LengthInt())
	}
	for it := u.ElementIterator(); it.Next(); {
		_, elem := it.Element()
		out = append(out, elem)
	}
	return out
}

func evalBoolSetting(bp config.Blueprint, val cty.Value, want bool) bool {
	v := evalUnmarked(bp, val)
	return v != cty.NilVal && v.IsKnown() && !v.IsNull() && v.Type() == cty.Bool && v.True() == want
}

func isDwsFlexActive(bp config.Blueprint, settings config.Dict) bool {
	m, ok := asObjectMap(evalUnmarked(bp, settings.Get("dws_flex")))
	if !ok || len(m) == 0 {
		return false
	}
	en, exists := m["enabled"]
	v := evalUnmarked(bp, en)
	return !exists || v == cty.NilVal || v.IsNull() || (v.IsKnown() && v.Type() == cty.Bool && v.True())
}

func shouldSkipModuleReservations(bp config.Blueprint, m config.Module) bool {
	for _, k := range skipReservationBoolSettings {
		if evalBoolSetting(bp, m.Settings.Get(k), true) {
			return true
		}
	}
	pm, _ := evalTrimmedString(bp, m.Settings.Get("provisioning_model"))
	return strings.EqualFold(pm, "SPOT") ||
		evalBoolSetting(bp, m.Settings.Get("is_reservation_active"), false) ||
		isDwsFlexActive(bp, m.Settings)
}

func isIgnoredReservation(s string) bool {
	return s == "" || s == "NO_RESERVATION" || s == "ANY_RESERVATION"
}

func trimComputeAPIPrefix(s string) string {
	for _, p := range computeAPIPrefixes {
		if len(s) > len(p) && strings.EqualFold(s[:len(p)], p) && strings.HasPrefix(s[len(p):], "projects/") {
			return s[len(p):]
		}
	}
	return s
}

func isReservationCollection(seg string) bool {
	return seg == "reservations" || seg == "futureReservations"
}

// cleanReservationRef trims API prefixes and block suffixes, returning (cleaned, ok).
func cleanReservationRef(raw string) (string, bool) {
	s := trimComputeAPIPrefix(strings.TrimRight(strings.TrimSpace(raw), "/"))
	parts := strings.Split(s, "/")
	cut := len(parts)
	for i, seg := range parts {
		if seg == "" {
			return "", false
		}
		if cut == len(parts) && strings.EqualFold(seg, "reservationBlocks") {
			cut = i
		}
	}
	if cut == 0 || cut == len(parts)-1 {
		return "", false
	}
	return strings.Join(parts[:cut], "/"), true
}

// parseReservationURI parses raw into (project, zone, name, err).
func parseReservationURI(raw string) (string, string, string, error) {
	s, ok := cleanReservationRef(raw)
	if ok && !strings.Contains(s, "/") {
		return "", "", s, nil
	}
	if ok {
		p := strings.Split(s, "/")
		if len(p) == 4 && p[0] == "projects" && isReservationCollection(p[2]) {
			return p[1], "", p[3], nil
		}
		if len(p) == 6 && p[0] == "projects" && p[2] == "zones" && isReservationCollection(p[4]) {
			return p[1], p[3], p[5], nil
		}
	}
	return "", "", "", config.HintError{
		Err:  fmt.Errorf("malformed reservation value %q", raw),
		Hint: "Expected a short name ('my-res'), 'projects/<project>/reservations/<name>', or a full zone reservation URL.",
	}
}

func wrapWithSource(source string, err error) error {
	if err == nil || source == "" || strings.Contains(err.Error(), source) {
		return err
	}
	if he, ok := err.(config.HintError); ok {
		return config.HintError{Err: fmt.Errorf("%s: %w", source, he.Err), Hint: he.Hint}
	}
	return fmt.Errorf("%s: %w", source, err)
}

func claimReservation(claimed map[string]bool, val cty.Value, raw, ownerProj, name string) {
	if claimed == nil {
		return
	}
	if ref := directGlobalRefName(val); ref != "" {
		claimed["var."+ref] = true
	}
	if raw == "" {
		return
	}
	switch {
	case name == "":
		claimed[raw] = true
	case ownerProj == "":
		claimed["*/"+name] = true
	default:
		claimed[ownerProj+"/"+name] = true
	}
}

func isClaimed(claimed map[string]bool, raw, ownerProj, name string) bool {
	return claimed[raw] || claimed["*/"+name] || (ownerProj != "" && claimed[ownerProj+"/"+name])
}

// parseAffinityEntry evaluates one specific_reservations entry into (target, ok, err).
func parseAffinityEntry(bp config.Blueprint, modID config.ModuleID, elem cty.Value, active bool, defaultProj string, claimed map[string]bool) (reservationTarget, bool, error) {
	m, ok := asObjectMap(evalUnmarked(bp, elem))
	if !ok {
		return reservationTarget{}, false, nil
	}
	nameVal := m["name"]
	raw, _ := evalTrimmedString(bp, nameVal)
	if raw == "" {
		claimReservation(claimed, nameVal, "", "", "")
		return reservationTarget{}, false, nil
	}
	uriProj, zone, name, err := parseReservationURI(raw)
	if err != nil {
		claimReservation(claimed, nameVal, raw, "", "")
		if active {
			return reservationTarget{}, false, wrapWithSource(fmt.Sprintf("module %q", modID), err)
		}
		return reservationTarget{}, false, nil
	}
	if active && uriProj != "" {
		return reservationTarget{}, false, config.HintError{
			Err:  fmt.Errorf("module %q: specific_reservations name %q must not contain a project path", modID, raw),
			Hint: "Specify the reservation short name (or block path '<res>/reservationBlocks/<block>') in 'name', and set 'project' on the specific_reservations entry for shared reservations.",
		}
	}
	ownerProj := uriProj
	if pVal, hasProj := m["project"]; hasProj {
		ownerProj = resolveProject(bp, pVal, defaultProj)
	} else if ownerProj == "" {
		ownerProj = defaultProj
	}
	res := reservationTarget{ownerProject: ownerProj, zone: zone, name: name}
	claimReservation(claimed, nameVal, raw, res.ownerProject, res.name)
	return res, true, nil
}

func scanAffinityReservations(bp config.Blueprint, m config.Module, moduleActive bool, defaultProj string, claimed map[string]bool) ([]reservationTarget, error) {
	aff, ok := asObjectMap(evalUnmarked(bp, m.Settings.Get("reservation_affinity")))
	if !ok {
		return nil, nil
	}
	cType, _ := evalTrimmedString(bp, aff["consume_reservation_type"])
	active := moduleActive && cType == "SPECIFIC_RESERVATION"
	var out []reservationTarget
	for _, elem := range asSequenceSlice(evalUnmarked(bp, aff["specific_reservations"])) {
		res, ok, err := parseAffinityEntry(bp, m.ID, elem, active, defaultProj, claimed)
		if err != nil {
			return nil, err
		}
		if ok && active {
			out = append(out, res)
		}
	}
	return out, nil
}

func scanModuleReservations(bp config.Blueprint, m config.Module, deploymentProjectID string, claimed map[string]bool) ([]reservationTarget, error) {
	active := !shouldSkipModuleReservations(bp, m)
	modProj := resolveProject(bp, m.Settings.Get("project_id"), deploymentProjectID)
	out, err := scanAffinityReservations(bp, m, active, modProj, claimed)
	if err != nil {
		return nil, err
	}
	keys := m.Settings.Keys()
	sort.Strings(keys)
	for _, k := range keys {
		if !resKeyRegex.MatchString(k) {
			continue
		}
		v := m.Settings.Get(k)
		raw, _ := evalTrimmedString(bp, v)
		if isIgnoredReservation(raw) {
			claimReservation(claimed, v, "", "", "")
			continue
		}
		uriProj, zone, name, err := parseReservationURI(raw)
		if err != nil {
			claimReservation(claimed, v, raw, "", "")
			if !active {
				continue
			}
			return nil, wrapWithSource(fmt.Sprintf("module %q setting %q", m.ID, k), err)
		}
		proj := modProj
		if uriProj != "" {
			proj = uriProj
		}
		claimReservation(claimed, v, raw, proj, name)
		if active {
			out = append(out, reservationTarget{ownerProject: proj, zone: zone, name: name})
		}
	}
	return out, nil
}

func findReservationOwnerProject(bp config.Blueprint, reservationName string) string {
	for _, grp := range bp.Groups {
		for _, m := range grp.Modules {
			modProj := resolveProject(bp, m.Settings.Get("project_id"), "")
			aff, _ := scanAffinityReservations(bp, m, !shouldSkipModuleReservations(bp, m), modProj, nil)
			for _, r := range aff {
				if r.name == reservationName && r.ownerProject != "" {
					return r.ownerProject
				}
			}
		}
	}
	return ""
}

func blueprintHasModuleReservation(bp config.Blueprint) bool {
	for _, grp := range bp.Groups {
		for _, m := range grp.Modules {
			res, err := scanModuleReservations(bp, m, "", nil)
			if err != nil || len(res) > 0 {
				return true
			}
		}
	}
	return false
}

func activeTopLevelReservationVars(bp config.Blueprint) []string {
	var keys []string
	for k, v := range bp.Vars.Items() {
		s, _ := evalTrimmedString(bp, v)
		if resKeyRegex.MatchString(k) && !isIgnoredReservation(s) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func defaultReservationValidators(bp config.Blueprint) []config.Validator {
	if !bp.Vars.Has("project_id") {
		return nil
	}
	vars := activeTopLevelReservationVars(bp)
	hasMod := blueprintHasModuleReservation(bp)
	if len(vars) == 0 && !hasMod {
		return nil
	}
	inputs := config.Dict{}.With("project_id", config.GlobalRef("project_id").AsValue())
	if bp.Vars.Has("zone") {
		inputs = inputs.With("zone", config.GlobalRef("zone").AsValue())
	}
	if len(vars) == 1 && !hasMod {
		inputs = inputs.With("reservation_name", config.GlobalRef(vars[0]).AsValue())
	}
	return []config.Validator{{Validator: testReservationExistsName, Inputs: inputs}}
}

func validateReservationInputs(bp config.Blueprint, inputs config.Dict) error {
	required := []string{"project_id"}
	if isValidatorExplicit(bp, testReservationExistsName) {
		required = []string{"project_id", "reservation_name", "zone"}
	} else {
		if bp.Vars.Has("zone") || inputs.Has("zone") {
			required = append(required, "zone")
		}
		if inputs.Has("reservation_name") {
			required = append(required, "reservation_name")
		}
	}
	if err := checkInputs(inputs, required); err != nil {
		return err
	}
	for k, v := range inputs.Items() {
		u := evalUnmarked(bp, v)
		if u != cty.NilVal && u.IsKnown() && !u.IsNull() && u.Type() != cty.String {
			return fmt.Errorf("input %q must be a string, got %s", k, u.Type().FriendlyName())
		}
	}
	return nil
}

// resolveModuleZones resolves module zones into (zones, known, err).
func resolveModuleZones(bp config.Blueprint, m config.Module, globalZone string, globalZoneKnown bool) ([]string, bool, error) {
	for _, key := range []string{"zones", "zone"} {
		v := evalUnmarked(bp, m.Settings.Get(key))
		if v == cty.NilVal {
			continue
		}
		u, _ := v.UnmarkDeep()
		if !u.IsWhollyKnown() {
			return []string{""}, false, nil
		}
		zs, err := extractZones(u)
		if err != nil {
			return nil, false, fmt.Errorf("module %q setting %q: %w", m.ID, key, err)
		}
		if len(zs) > 0 {
			return zs, true, nil
		}
	}
	return []string{globalZone}, globalZoneKnown, nil
}

func appendReservationTarget(out []reservationTarget, r reservationTarget, z string, zoneKnown bool, source string) ([]reservationTarget, error) {
	if r.ownerProject == "" {
		return out, nil
	}
	targetZone := z
	if r.zone != "" {
		if z != "" && r.zone != z {
			return nil, wrapWithSource(source, config.HintError{
				Err:  fmt.Errorf("reservation %q is in zone %q, which conflicts with configured zone %q", r.name, r.zone, z),
				Hint: fmt.Sprintf("Use a reservation located in zone %q or update the configured zone to %q.", z, r.zone),
			})
		}
		targetZone = r.zone
	}
	if targetZone == "" {
		if !zoneKnown {
			return out, nil
		}
		return nil, wrapWithSource(source, config.HintError{
			Err:  fmt.Errorf("reservation %q has no zone specified", r.name),
			Hint: "Set 'vars.zone', configure 'zone'/'zones' on the module consuming the reservation, or provide a full reservation URL containing '/zones/<zone>/'.",
		})
	}
	return append(out, reservationTarget{ownerProject: r.ownerProject, zone: targetZone, name: r.name, source: source}), nil
}

func appendRawTarget(bp config.Blueprint, out []reservationTarget, raw, source, deploymentProj, globalZone string, zoneKnown bool, claimed map[string]bool) ([]reservationTarget, error) {
	if isIgnoredReservation(raw) {
		return out, nil
	}
	uriProj, uriZone, name, err := parseReservationURI(raw)
	if err != nil {
		return nil, wrapWithSource(source, err)
	}
	ownerProj := uriProj
	if ownerProj == "" {
		ownerProj = findReservationOwnerProject(bp, name)
	}
	if ownerProj == "" {
		ownerProj = deploymentProj
	}
	if claimed != nil && isClaimed(claimed, raw, ownerProj, name) {
		return out, nil
	}
	return appendReservationTarget(out, reservationTarget{ownerProject: ownerProj, zone: uriZone, name: name}, globalZone, zoneKnown, source)
}

func collectModuleTargets(bp config.Blueprint, deploymentProj, globalZone string, zoneKnown bool, claimed map[string]bool) ([]reservationTarget, error) {
	var targets []reservationTarget
	for _, grp := range bp.Groups {
		for _, m := range grp.Modules {
			mRes, err := scanModuleReservations(bp, m, deploymentProj, claimed)
			if err != nil {
				return nil, err
			}
			if len(mRes) == 0 {
				continue
			}
			zs, mZoneKnown, err := resolveModuleZones(bp, m, globalZone, zoneKnown)
			if err != nil {
				return nil, err
			}
			src := fmt.Sprintf("module %q", m.ID)
			for _, r := range mRes {
				for _, z := range zs {
					if targets, err = appendReservationTarget(targets, r, z, mZoneKnown, src); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return targets, nil
}

func collectReservationTargets(bp config.Blueprint, deploymentProj, globalZone string, zoneKnown bool, resInput string) ([]reservationTarget, error) {
	if isValidatorExplicit(bp, testReservationExistsName) {
		return appendRawTarget(bp, nil, resInput, "", deploymentProj, globalZone, zoneKnown, nil)
	}
	claimed := make(map[string]bool)
	targets, err := collectModuleTargets(bp, deploymentProj, globalZone, zoneKnown, claimed)
	if err != nil {
		return nil, err
	}
	seenInput := false
	for _, k := range activeTopLevelReservationVars(bp) {
		s, _ := evalTrimmedString(bp, bp.Vars.Get(k))
		seenInput = seenInput || s == resInput
		if claimed["var."+k] {
			continue
		}
		if targets, err = appendRawTarget(bp, targets, s, fmt.Sprintf("vars.%s", k), deploymentProj, globalZone, zoneKnown, claimed); err != nil {
			return nil, err
		}
	}
	if !isIgnoredReservation(resInput) && !seenInput {
		return appendRawTarget(bp, targets, resInput, "", deploymentProj, globalZone, zoneKnown, claimed)
	}
	return targets, nil
}

func testReservationExists(bp config.Blueprint, inputs config.Dict) error {
	if err := validateReservationInputs(bp, inputs); err != nil {
		return err
	}
	deploymentProjectID, _ := evalTrimmedString(bp, inputs.Get("project_id"))
	globalZone, zoneKnown := evalTrimmedString(bp, inputs.Get("zone"))
	resInput, _ := evalTrimmedString(bp, inputs.Get("reservation_name"))

	targets, err := collectReservationTargets(bp, deploymentProjectID, globalZone, zoneKnown, resInput)
	if err != nil || len(targets) == 0 {
		return err
	}

	ctx := context.Background()
	s, err := newComputeService(ctx)
	if err != nil {
		return handleClientError(err)
	}
	checked := make(map[[3]string]bool, len(targets))
	errs := config.Errors{}
	for _, t := range targets {
		key := [3]string{t.ownerProject, t.zone, t.name}
		if t.name == "" || checked[key] {
			continue
		}
		checked[key] = true
		if err := testReservationExistsWithService(ctx, s, t.ownerProject, t.zone, t.name, deploymentProjectID); err != nil {
			_ = errs.Add(wrapWithSource(t.source, err))
		}
	}
	return errs.OrNil()
}

// testResourceInZoneAvailability is a generic helper that validates a resource type
// (like machine_type or disk_type) across a blueprint's modules or via an explicit validator.
func testResourceInZoneAvailability(
	bp config.Blueprint,
	inputs config.Dict,
	validatorName string,
	settingSuffix string,
	resourceLabel string,
	validateFn func(s *compute.Service, projectID, zone, name, vName string) error,
) error {
	// 1. Determine if the validator was explicitly added to the blueprint YAML
	required := []string{"project_id", "zone"}
	if isValidatorExplicit(bp, validatorName) {
		required = append(required, settingSuffix)
	}

	if err := checkInputs(inputs, required); err != nil {
		return err
	}

	// Initialize Compute API service
	s, err := compute.NewService(context.Background())
	if err != nil {
		return handleClientError(err)
	}
	m, err := inputsAsStrings(inputs)
	if err != nil {
		return err
	}

	projectID, globalZone, explicitValue := m["project_id"], m["zone"], m[settingSuffix]

	// 2. Handle Explicit case: User manually defined the validator in the YAML
	if explicitValue != "" {
		if err := TestZoneExists(projectID, globalZone); err != nil {
			return err
		}
		err := validateFn(s, projectID, globalZone, explicitValue, validatorName)
		if errors.Is(err, errSoftWarning) {
			return nil
		}
		return err
	}

	// 3. Case: Implicitly check all modules for this setting
	return validateSettingsInModules(bp, globalZone, projectID, settingSuffix, resourceLabel, validatorName, func(z, name string, vName string) error {
		return validateFn(s, projectID, z, name, vName)
	})
}

// testMachineTypeInZoneAvailability automatically validates machine types in modules
func testMachineTypeInZoneAvailability(bp config.Blueprint, inputs config.Dict) error {
	return testResourceInZoneAvailability(bp, inputs, "test_machine_type_in_zone", "machine_type", "machine type", validateMachineTypeInZone)
}

// testDiskTypeInZoneAvailability automatically validates disk types in modules
func testDiskTypeInZoneAvailability(bp config.Blueprint, inputs config.Dict) error {
	return testResourceInZoneAvailability(bp, inputs, "test_disk_type_in_zone", "disk_type", "disk type", validateDiskTypeInZone)
}
