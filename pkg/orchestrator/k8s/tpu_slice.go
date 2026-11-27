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

package k8s

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TPU slice resource, label, annotation, finalizer, and scheduler constants used across
// slice.tpu.google.com discovery, manifest rendering, validation, diagnostics, and teardown.
const (
	TPUSliceResource                = "slices.slice.tpu.google.com"
	TPUSliceLabelPrefix             = "tpu.google.com/"
	TPUSliceNameLabel               = "tpu.google.com/slice-name"
	TPUSliceSchedulerNameLabel      = "slice-scheduler.tpu.google.com/slice-name"
	TPUSlicePrefixLabel             = "tpu.google.com/slice-prefix"
	TPUSliceSchedulerPrefixLabel    = "slice-scheduler.tpu.google.com/slice-prefix"
	TPUSliceOwnerNameLabel          = "tpu.google.com/owner-name"
	TPUSliceOwnerNamespaceLabel     = "tpu.google.com/owner-namespace"
	TPUSliceOwnerKindLabel          = "tpu.google.com/owner-kind"
	TPUSliceSchedulerOwnerNameLabel = "slice-scheduler.tpu.google.com/owner-name"
	TPUSliceSchedulerOwnerNSLabel   = "slice-scheduler.tpu.google.com/owner-namespace"
	TPUSliceSchedulerOwnerKindLabel = "slice-scheduler.tpu.google.com/owner-kind"
	TPUSliceLegacyFinalizer         = "tpu.google.com/delete-slice-cr"
	TPUSliceSchedulerFinalizer      = "slice-scheduler.tpu.google.com/delete-slices"
	TPUSliceTypeLabel               = "tpu.google.com/type"
	TPUSliceTopologyLabel           = "tpu.google.com/topology"
	TPUSlicePartitionLabelPrefix    = "tpu.google.com/partition-id-"
	TPUNumSlicesAnnotation          = "tpu.google.com/num-slices"
	TPUSliceGroupNameAnnotation     = "tpu.google.com/slice-group-name"
	TPUSliceSchedulerName           = "slice-scheduler"

	// MaxSliceEvents caps the number of recent Slice events surfaced in inspect and logs output.
	MaxSliceEvents = 20
)

// IsSchedulable reports whether the TPU slice is Ready, not terminating, and either has
// SliceNodesLabeled=True or does not expose the SliceNodesLabeled condition.
func (s SliceInfo) IsSchedulable() bool {
	return s.Ready && !s.Terminating && (!s.NodesLabeledKnown || s.NodesLabeled)
}

// listTPUSlices queries slice.tpu.google.com/v1alpha1 (spec.{type,topology,partitionIds} and
// status.conditions Ready) and falls back to v1beta1 strictly on NotFound/NoMatch.
func listTPUSlices(lister ResourceLister, hw TPUHardwareResolver) ([]SliceInfo, bool, error) {
	items, err := lister.ListResources(TPUSliceGVR, "", "", "")
	if err != nil && IsNotFoundOrNoMatchError(err) {
		items, err = lister.ListResources(TPUSliceV1Beta1GVR, "", "", "")
	}
	if err != nil {
		return nil, false, err
	}
	result := make([]SliceInfo, 0, len(items))
	for i := range items {
		result = append(result, parseTPUSlice(items[i], hw))
	}
	sortSlices(result)
	return result, true, nil
}

// ListTPUSlicesByOwner lists cluster-scoped slice.tpu.google.com Slice resources owned by ownerName
// (scoped to ownerNamespace when both ownerNamespace and a slice's owner-namespace label are non-empty),
// falling back from v1alpha1 to v1beta1 strictly on NotFound/NoMatch.
// It matches both JobSet-owned slices (owner-name=<ownerName>) and per-Job lifecycle slices created for
// standard JobSet child Jobs (owner-name=<ownerName>-main-job-<index>), across legacy tpu.google.com/*
// and slice-scheduler.tpu.google.com/* metadata keys.
func ListTPUSlicesByOwner(lister ResourceLister, ownerNamespace, ownerName string, hw TPUHardwareResolver) ([]SliceInfo, error) {
	if lister == nil {
		return nil, errListerRequired
	}
	ownerName = strings.TrimSpace(ownerName)
	if ownerName == "" {
		return nil, nil
	}
	if hw == nil {
		hw = noopHardwareResolver{}
	}
	allSlices, _, err := listTPUSlices(lister, hw)
	if err != nil {
		return nil, err
	}
	return FilterOwnedTPUSlices(allSlices, ownerNamespace, ownerName), nil
}

// FilterOwnedTPUSlices returns the subset of slices owned by workloadName (scoped to ownerNamespace
// when both ownerNamespace and a slice's OwnerNamespace are set).
func FilterOwnedTPUSlices(all []SliceInfo, ownerNamespace, workloadName string) []SliceInfo {
	workloadName = strings.TrimSpace(workloadName)
	if workloadName == "" {
		return nil
	}
	ownerNamespace = strings.TrimSpace(ownerNamespace)
	var matched []SliceInfo
	for _, s := range all {
		if sliceOwnedByWorkload(s, ownerNamespace, workloadName) {
			matched = append(matched, s)
		}
	}
	return matched
}

func sliceOwnedByWorkload(s SliceInfo, ownerNamespace, workloadName string) bool {
	if ownerNamespace != "" && s.OwnerNamespace != "" && s.OwnerNamespace != ownerNamespace {
		return false
	}
	if s.OwnerName == workloadName && (s.OwnerKind == "" || strings.EqualFold(s.OwnerKind, "JobSet")) {
		return true
	}
	return isJobSetChildOwner(s.OwnerName, s.OwnerKind, workloadName)
}

// isJobSetChildOwner matches legacy per-Job slice-provisioner ownership where each JobSet child Job
// (<workloadName>-main-job-<replicaIndex>) owns its own Slice CR rather than the parent JobSet.
func isJobSetChildOwner(ownerName, ownerKind, workloadName string) bool {
	if ownerKind != "" && !strings.EqualFold(ownerKind, "Job") {
		return false
	}
	rest, ok := strings.CutPrefix(ownerName, workloadName+"-main-job-")
	if !ok || rest == "" {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return false
		}
	}
	return true
}

// ParseTPUSliceListJSON parses the JSON output of `kubectl get slices -o json` into sorted SliceInfo entries.
func ParseTPUSliceListJSON(raw string, hw TPUHardwareResolver) ([]SliceInfo, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	if hw == nil {
		hw = noopHardwareResolver{}
	}
	var payload *struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Slice list JSON: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("empty or null Slice list JSON")
	}
	result := make([]SliceInfo, 0, len(payload.Items))
	for _, item := range payload.Items {
		if len(item) == 0 {
			continue
		}
		parsed := parseTPUSlice(unstructured.Unstructured{Object: item}, hw)
		if strings.TrimSpace(parsed.Name) == "" {
			continue
		}
		result = append(result, parsed)
	}
	sortSlices(result)
	return result, nil
}

// ParseTPUSliceJSON parses the JSON output of `kubectl get slice <name> -o json` into a SliceInfo entry.
func ParseTPUSliceJSON(raw string, hw TPUHardwareResolver) (SliceInfo, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return SliceInfo{}, fmt.Errorf("empty Slice JSON")
	}
	if hw == nil {
		hw = noopHardwareResolver{}
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return SliceInfo{}, fmt.Errorf("failed to parse Slice JSON: %w", err)
	}
	if len(obj) == 0 {
		return SliceInfo{}, fmt.Errorf("empty or null Slice JSON")
	}
	parsed := parseTPUSlice(unstructured.Unstructured{Object: obj}, hw)
	if strings.TrimSpace(parsed.Name) == "" {
		return SliceInfo{}, fmt.Errorf("slice JSON missing metadata.name")
	}
	return parsed, nil
}

// FormatSliceReadiness renders concise human-readable readiness status for discovered TPU Slice CRs,
// highlighting terminating slices, empty spec.partitionIds (capacity / gang wait), and Ready=False / SliceNodesLabeled=False states.
func FormatSliceReadiness(sliceList []SliceInfo) string {
	if len(sliceList) == 0 {
		return ""
	}
	lines := make([]string, 0, len(sliceList))
	for _, s := range sliceList {
		lines = append(lines, formatSingleSliceReadiness(s))
	}
	return strings.Join(lines, "\n")
}

func formatSliceNodesLabeledNotReady(s SliceInfo) string {
	if !s.NodesLabeledKnown || s.NodesLabeled {
		return ""
	}
	detail := "SliceNodesLabeled=False"
	if s.NodesLabeledReason != "" {
		detail += " reason=" + s.NodesLabeledReason
	}
	return detail
}

func formatSingleSliceReadiness(s SliceInfo) string {
	var metaParts []string
	if s.TPUType != "" {
		metaParts = append(metaParts, "type="+s.TPUType)
	}
	if s.Topology != "" {
		metaParts = append(metaParts, "topology="+s.Topology)
	}
	header := fmt.Sprintf("Slice %s", s.Name)
	if len(metaParts) > 0 {
		header += " (" + strings.Join(metaParts, ", ") + ")"
	}
	if s.Terminating || strings.EqualFold(s.State, "Terminating") {
		return fmt.Sprintf("%s: Terminating", header)
	}
	if s.Ready {
		state := firstNonEmpty(s.State, "Ready")
		if labelDetail := formatSliceNodesLabeledNotReady(s); labelDetail != "" {
			return fmt.Sprintf("%s: Ready (%s); %s", header, state, labelDetail)
		}
		return fmt.Sprintf("%s: Ready (%s)", header, state)
	}
	var details []string
	if len(s.PartitionIDs) == 0 {
		details = append(details, "waiting for TPU capacity / gang completion (spec.partitionIds is empty)")
	}
	readyDetail := "Ready=False"
	if s.State != "" {
		readyDetail += " reason=" + s.State
	}
	if s.ReadyMessage != "" {
		readyDetail += ": " + s.ReadyMessage
	}
	details = append(details, readyDetail)
	if labelDetail := formatSliceNodesLabeledNotReady(s); labelDetail != "" {
		details = append(details, labelDetail)
	}
	return fmt.Sprintf("%s: %s", header, strings.Join(details, "; "))
}

func parseTPUSlice(obj unstructured.Unstructured, hw TPUHardwareResolver) SliceInfo {
	labels := obj.GetLabels()
	rawType := firstNonEmpty(
		nestedString(obj.Object, "spec", "type"),
		labels[TPUSliceTypeLabel],
	)
	topology := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		nestedString(obj.Object, "spec", "topology"),
		labels[TPUSliceTopologyLabel],
	)))
	ready, reason, readyMsg, _ := findCondition(obj.Object, "Ready")
	nodesLabeled, nodesLabeledReason, _, nodesLabeledKnown := findCondition(obj.Object, "SliceNodesLabeled")
	terminating := obj.GetDeletionTimestamp() != nil
	if terminating {
		ready = false
		reason = "Terminating"
		readyMsg = ""
	}
	partitionIDs, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "partitionIds")

	totalChips, _ := parseTopologyChipCount(topology)
	chipsHint := 0
	if totalChips == 1 {
		chipsHint = 1
	}
	tpuType := hw.NormalizeTPUType(rawType)
	var machineType string
	if !isAmbiguousSliceHostSize(hw, tpuType, totalChips) {
		machineType = hw.InferTPUMachine(tpuType, "", chipsHint)
	}
	chips := hw.ChipsPerNode(machineType)
	if chips == 0 && chipsHint > 0 {
		chips = chipsHint
	}

	annotations := obj.GetAnnotations()
	ownerName := firstNonEmpty(
		labels[TPUSliceSchedulerOwnerNameLabel],
		labels[TPUSliceOwnerNameLabel],
		annotations[TPUSliceSchedulerOwnerNameLabel],
		annotations[TPUSliceOwnerNameLabel],
	)
	ownerNamespace := firstNonEmpty(
		labels[TPUSliceSchedulerOwnerNSLabel],
		labels[TPUSliceOwnerNamespaceLabel],
		annotations[TPUSliceSchedulerOwnerNSLabel],
		annotations[TPUSliceOwnerNamespaceLabel],
	)
	ownerKind := firstNonEmpty(
		labels[TPUSliceSchedulerOwnerKindLabel],
		labels[TPUSliceOwnerKindLabel],
		annotations[TPUSliceSchedulerOwnerKindLabel],
		annotations[TPUSliceOwnerKindLabel],
	)

	return SliceInfo{
		Name:               obj.GetName(),
		Namespace:          obj.GetNamespace(),
		TPUType:            tpuType,
		MachineType:        machineType,
		Topology:           topology,
		State:              reason,
		Ready:              ready,
		Terminating:        terminating,
		ReadyMessage:       readyMsg,
		NodesLabeled:       nodesLabeled,
		NodesLabeledKnown:  nodesLabeledKnown,
		NodesLabeledReason: nodesLabeledReason,
		OwnerName:          ownerName,
		OwnerNamespace:     ownerNamespace,
		OwnerKind:          ownerKind,
		NodesPerSlice:      calculateNodesFromTopology(topology, chips),
		ChipsPerNode:       chips,
		PartitionIDs:       partitionIDs,
	}
}

func isAmbiguousSliceHostSize(hw TPUHardwareResolver, tpuType string, totalChips int) bool {
	if tpuType == "" {
		return false
	}
	if totalChips == 0 {
		return hasMultipleHostChipSizes(hw, tpuType)
	}
	if totalChips != 8 {
		return false
	}
	return hw.ChipsPerNode(hw.InferTPUMachine(tpuType, "", 8)) == 8 &&
		hw.ChipsPerNode(hw.InferTPUMachine(tpuType, "", 4)) == 4
}

func hasMultipleHostChipSizes(hw TPUHardwareResolver, tpuType string) bool {
	seen := 0
	for _, hint := range []int{1, 4, 8} {
		c := hw.ChipsPerNode(hw.InferTPUMachine(tpuType, "", hint))
		if c == 0 {
			continue
		}
		if seen != 0 && seen != c {
			return true
		}
		seen = c
	}
	return false
}

func sortSlices(sliceList []SliceInfo) {
	slices.SortFunc(sliceList, func(a, b SliceInfo) int {
		return cmp.Or(
			strings.Compare(a.Name, b.Name),
			strings.Compare(a.Namespace, b.Namespace),
		)
	})
}

// FilterSliceEventsJSON parses a core/v1 EventList JSON payload, filters events targeting the named
// slice.tpu.google.com Slice CRs, sorts chronologically, caps to the latest MaxSliceEvents, and formats
// each event onto a single line.
func FilterSliceEventsJSON(raw string, wanted map[string]bool) string {
	var list corev1.EventList
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return ""
	}
	var matched []corev1.Event
	for _, ev := range list.Items {
		if isTPUSliceEventObject(ev.InvolvedObject) && wanted[strings.TrimSpace(ev.InvolvedObject.Name)] {
			matched = append(matched, ev)
		}
	}
	if len(matched) == 0 {
		return ""
	}
	slices.SortStableFunc(matched, compareSliceEvents)
	if len(matched) > MaxSliceEvents {
		matched = matched[len(matched)-MaxSliceEvents:]
	}
	lines := make([]string, 0, len(matched))
	for _, ev := range matched {
		lines = append(lines, formatSliceEvent(ev))
	}
	return strings.Join(lines, "\n")
}

func isTPUSliceEventObject(ref corev1.ObjectReference) bool {
	if kind := strings.TrimSpace(ref.Kind); kind != "" && kind != "Slice" {
		return false
	}
	if apiVer := strings.TrimSpace(ref.APIVersion); apiVer != "" &&
		apiVer != TPUSliceAPIGroupVersion &&
		!strings.HasPrefix(apiVer, TPUSliceAPIGroupVersion+"/") {
		return false
	}
	return true
}

func effectiveEventTime(ev corev1.Event) time.Time {
	if ev.Series != nil && !ev.Series.LastObservedTime.IsZero() {
		return ev.Series.LastObservedTime.Time
	}
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	if !ev.FirstTimestamp.IsZero() {
		return ev.FirstTimestamp.Time
	}
	return ev.CreationTimestamp.Time
}

func compareSliceEvents(a, b corev1.Event) int {
	return cmp.Or(
		effectiveEventTime(a).Compare(effectiveEventTime(b)),
		strings.Compare(a.InvolvedObject.Name, b.InvolvedObject.Name),
		strings.Compare(a.Name, b.Name),
	)
}

func formatSliceEvent(ev corev1.Event) string {
	header := strings.TrimSpace(ev.InvolvedObject.Name)
	if evType := strings.TrimSpace(ev.Type); evType != "" {
		header += " [" + evType + "]"
	}
	if reason := strings.TrimSpace(ev.Reason); reason != "" {
		header += " " + reason
	}
	if msg := normalizeEventMessage(ev.Message); msg != "" {
		return header + ": " + msg
	}
	return header
}

func normalizeEventMessage(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\r", "\n")
	return strings.ReplaceAll(trimmed, "\n", " | ")
}
