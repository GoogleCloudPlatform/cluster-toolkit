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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type recordingSliceLister struct {
	listFunc func(gvr schema.GroupVersionResource, namespace, labelSelector, fieldSelector string) ([]unstructured.Unstructured, error)
}

func (r *recordingSliceLister) ListResources(gvr schema.GroupVersionResource, namespace, labelSelector, fieldSelector string) ([]unstructured.Unstructured, error) {
	return r.listFunc(gvr, namespace, labelSelector, fieldSelector)
}

func TestSliceInfo_IsSchedulable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    SliceInfo
		want bool
	}{
		{name: "ready_no_label_condition", s: SliceInfo{Ready: true}, want: true},
		{name: "ready_and_labeled", s: SliceInfo{Ready: true, NodesLabeledKnown: true, NodesLabeled: true}, want: true},
		{name: "ready_but_unlabeled", s: SliceInfo{Ready: true, NodesLabeledKnown: true, NodesLabeled: false}, want: false},
		{name: "ready_but_terminating", s: SliceInfo{Ready: true, Terminating: true}, want: false},
		{name: "not_ready", s: SliceInfo{Ready: false, NodesLabeledKnown: true, NodesLabeled: true}, want: false},
	}
	for _, tc := range cases {
		if got := tc.s.IsSchedulable(); got != tc.want {
			t.Errorf("%s: IsSchedulable() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSelectDefaultSlice_SkipsTerminatingAndUnlabeled(t *testing.T) {
	t.Parallel()
	tpu := &TPUInventory{TPUType: "tpu7x"}
	slicesIn := []SliceInfo{
		{Name: "s-term", TPUType: "tpu7x", Ready: true, Terminating: true},
		{Name: "s-unlabeled", TPUType: "tpu7x", Ready: true, NodesLabeledKnown: true, NodesLabeled: false},
		{Name: "s-ready", TPUType: "tpu7x", Topology: "2x2x2", NodesPerSlice: 2, Ready: true, NodesLabeledKnown: true, NodesLabeled: true},
	}
	inv := &Inventory{Slices: slicesIn}
	applyDefaultSlice(inv, tpu)
	if tpu.DefaultSliceName != "s-ready" || tpu.Topology != "2x2x2" {
		t.Fatalf("applyDefaultSlice = (%q, %q), want (s-ready, 2x2x2)", tpu.DefaultSliceName, tpu.Topology)
	}

	// Only an unlabeled slice present: hardware metadata backfills, but DefaultSliceName stays empty.
	tpuOnlyUnlabeled := &TPUInventory{}
	applyDefaultSlice(&Inventory{Slices: []SliceInfo{
		{Name: "s-term", TPUType: "v6e", Ready: false, Terminating: true},
		{Name: "s-unlabeled", TPUType: "tpu7x", Topology: "2x2x1", Ready: true, NodesLabeledKnown: true, NodesLabeled: false},
	}}, tpuOnlyUnlabeled)
	if tpuOnlyUnlabeled.DefaultSliceName != "" || tpuOnlyUnlabeled.TPUType != "tpu7x" {
		t.Errorf("unlabeled-only backfill = %+v, want empty DefaultSliceName + TPUType=tpu7x", tpuOnlyUnlabeled)
	}
}

func TestListTPUSlicesByOwner(t *testing.T) {
	t.Parallel()
	noMatchAlpha := &meta.NoResourceMatchError{PartialResource: TPUSliceGVR}
	betaSlice := unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "my-job-main-job-0",
				"labels": map[string]interface{}{
					TPUSliceOwnerNameLabel:      "my-job",
					TPUSliceOwnerNamespaceLabel: "ml",
					TPUSliceOwnerKindLabel:      "JobSet",
				},
			},
			"spec": map[string]interface{}{
				"type":     "tpu7x",
				"topology": "2x2x1",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "False", "reason": "NotCreated", "message": "waiting on slice controller"},
					map[string]interface{}{"type": "SliceNodesLabeled", "status": "False", "reason": "LabelingNodes"},
				},
			},
		},
	}
	otherNsSlice := unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "other-ns-slice",
				"labels": map[string]interface{}{
					TPUSliceOwnerNameLabel:      "my-job",
					TPUSliceOwnerNamespaceLabel: "other-ns",
				},
			},
			"spec": map[string]interface{}{"type": "tpu7x", "topology": "2x2x1"},
		},
	}
	type listCall struct {
		gvr      schema.GroupVersionResource
		selector string
	}
	var calls []listCall
	lister := &recordingSliceLister{
		listFunc: func(gvr schema.GroupVersionResource, _, labelSelector, _ string) ([]unstructured.Unstructured, error) {
			calls = append(calls, listCall{gvr: gvr, selector: labelSelector})
			if gvr == TPUSliceGVR {
				return nil, noMatchAlpha
			}
			return []unstructured.Unstructured{betaSlice, otherNsSlice}, nil
		},
	}

	got, err := ListTPUSlicesByOwner(lister, "ml", "my-job", nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("ListTPUSlicesByOwner() = (%+v, %v), want 1 slice", got, err)
	}
	wantCalls := []listCall{{TPUSliceGVR, ""}, {TPUSliceV1Beta1GVR, ""}}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("ListResources calls = %+v, want %+v", calls, wantCalls)
	}
	wantSlice := SliceInfo{
		Name:               "my-job-main-job-0",
		TPUType:            "tpu7x",
		Topology:           "2x2x1",
		State:              "NotCreated",
		Ready:              false,
		ReadyMessage:       "waiting on slice controller",
		NodesLabeled:       false,
		NodesLabeledKnown:  true,
		NodesLabeledReason: "LabelingNodes",
		OwnerName:          "my-job",
		OwnerNamespace:     "ml",
		OwnerKind:          "JobSet",
	}
	if !reflect.DeepEqual(got[0], wantSlice) {
		t.Errorf("parsed SliceInfo = %+v, want %+v", got[0], wantSlice)
	}
}

func TestListTPUSlicesByOwner_InputValidation(t *testing.T) {
	t.Parallel()
	lister := &recordingSliceLister{
		listFunc: func(schema.GroupVersionResource, string, string, string) ([]unstructured.Unstructured, error) {
			return nil, nil
		},
	}
	if _, err := ListTPUSlicesByOwner(nil, "ml", "my-job", nil); err == nil {
		t.Errorf("ListTPUSlicesByOwner(nil lister) = nil, want error")
	}
	if emptyGot, err := ListTPUSlicesByOwner(lister, "ml", "   ", nil); err != nil || len(emptyGot) != 0 {
		t.Errorf("ListTPUSlicesByOwner(blank owner) = (%+v, %v), want (nil, nil)", emptyGot, err)
	}
}

func TestListTPUSlicesByOwner_DirectAlphaAndForbidden(t *testing.T) {
	t.Parallel()
	alphaSlice := unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "alpha-job-main-job-0",
				"labels": map[string]interface{}{
					TPUSliceSchedulerOwnerNameLabel: "alpha-job",
					TPUSliceSchedulerOwnerNSLabel:   "ml",
					TPUSliceSchedulerOwnerKindLabel: "JobSet",
				},
			},
			"spec": map[string]interface{}{"type": "tpu7x", "topology": "2x2x2"},
		},
	}
	alphaCalls := 0
	alphaLister := &recordingSliceLister{
		listFunc: func(gvr schema.GroupVersionResource, _, labelSelector, _ string) ([]unstructured.Unstructured, error) {
			alphaCalls++
			if gvr != TPUSliceGVR || labelSelector != "" {
				t.Errorf("unexpected call (%v, %q)", gvr, labelSelector)
			}
			return []unstructured.Unstructured{alphaSlice}, nil
		},
	}
	if got, err := ListTPUSlicesByOwner(alphaLister, "ml", "alpha-job", nil); err != nil || len(got) != 1 || alphaCalls != 1 {
		t.Fatalf("ListTPUSlicesByOwner(v1alpha1) = (%+v, %v, calls=%d), want 1 slice in 1 call", got, err, alphaCalls)
	}

	forbErr := apierrors.NewForbidden(TPUSliceGVR.GroupResource(), "", errors.New("denied"))
	calls := 0
	forbLister := &recordingSliceLister{
		listFunc: func(schema.GroupVersionResource, string, string, string) ([]unstructured.Unstructured, error) {
			calls++
			return nil, forbErr
		},
	}
	if _, err := ListTPUSlicesByOwner(forbLister, "ml", "my-job", nil); !errors.Is(err, forbErr) || calls != 1 {
		t.Errorf("ListTPUSlicesByOwner(403) = (%v, calls=%d), want 403 in 1 call", err, calls)
	}
}

func TestFilterOwnedTPUSlices_NamespaceAndLifecycle(t *testing.T) {
	t.Parallel()
	all := []SliceInfo{
		{Name: "s-js", OwnerName: "train", OwnerNamespace: "ns-a", OwnerKind: "JobSet"},
		{Name: "s-child-0", OwnerName: "train-main-job-0", OwnerNamespace: "ns-a", OwnerKind: "Job"},
		{Name: "s-child-other-ns", OwnerName: "train-main-job-1", OwnerNamespace: "ns-b", OwnerKind: "Job"},
		{Name: "s-prefix-collision", OwnerName: "train-v2-main-job-0", OwnerNamespace: "ns-a", OwnerKind: "Job"},
		{Name: "s-nested-collision", OwnerName: "train-main-job-main-job-0", OwnerNamespace: "ns-a", OwnerKind: "Job"},
		{Name: "s-direct-sts", OwnerName: "train", OwnerNamespace: "ns-a", OwnerKind: "StatefulSet"},
		{Name: "s-direct-pod", OwnerName: "train", OwnerNamespace: "ns-a", OwnerKind: "Pod"},
		{Name: "s-wrong-kind", OwnerName: "train-main-job-2", OwnerNamespace: "ns-a", OwnerKind: "StatefulSet"},
		{Name: "s-unscoped", OwnerName: "train"},
	}
	got := FilterOwnedTPUSlices(all, "ns-a", "train")
	if len(got) != 3 || got[0].Name != "s-js" || got[1].Name != "s-child-0" || got[2].Name != "s-unscoped" {
		t.Fatalf("FilterOwnedTPUSlices(ns-a, train) = %+v, want [s-js, s-child-0, s-unscoped]", got)
	}
	if empty := FilterOwnedTPUSlices(all, "ns-a", "  "); len(empty) != 0 {
		t.Errorf("FilterOwnedTPUSlices(blank) = %+v, want empty", empty)
	}
}

func TestFilterOwnedTPUSlices_AnnotationOwnerIsolation(t *testing.T) {
	t.Parallel()
	longOwner := strings.Repeat("w", 70)
	annotJSON := `{"metadata":{"name":"s-annot","labels":{"slice-scheduler.tpu.google.com/owner-namespace":"ns-a","slice-scheduler.tpu.google.com/owner-kind":"JobSet"},"annotations":{"slice-scheduler.tpu.google.com/owner-name":"` + longOwner + `"}},"spec":{"type":"tpu7x","topology":"2x2x1"}}`
	parsed, err := ParseTPUSliceJSON(annotJSON, nil)
	if err != nil || parsed.OwnerName != longOwner || parsed.OwnerNamespace != "ns-a" || parsed.OwnerKind != "JobSet" {
		t.Fatalf("ParseTPUSliceJSON(annotation owner-name) = (%+v, %v)", parsed, err)
	}
	if matched := FilterOwnedTPUSlices([]SliceInfo{parsed}, "ns-a", longOwner); len(matched) != 1 {
		t.Errorf("FilterOwnedTPUSlices(ns-a, longOwner) = %+v, want 1 match", matched)
	}
	if matched := FilterOwnedTPUSlices([]SliceInfo{parsed}, "ns-b", longOwner); len(matched) != 0 {
		t.Errorf("FilterOwnedTPUSlices(ns-b, longOwner) = %+v, want 0 matches", matched)
	}

	allAnnotJSON := `{"metadata":{"name":"s-all-annot","annotations":{"tpu.google.com/owner-name":"train","tpu.google.com/owner-namespace":"ns-a","tpu.google.com/owner-kind":"JobSet"}},"spec":{"type":"tpu7x","topology":"2x2x1"}}`
	parsedAnnot, err := ParseTPUSliceJSON(allAnnotJSON, nil)
	if err != nil || parsedAnnot.OwnerName != "train" || parsedAnnot.OwnerNamespace != "ns-a" || parsedAnnot.OwnerKind != "JobSet" {
		t.Fatalf("ParseTPUSliceJSON(all-annotation owner) = (%+v, %v)", parsedAnnot, err)
	}
	if matched := FilterOwnedTPUSlices([]SliceInfo{parsedAnnot}, "ns-a", "train"); len(matched) != 1 {
		t.Errorf("FilterOwnedTPUSlices(ns-a, train) = %+v, want 1 match", matched)
	}
	if matched := FilterOwnedTPUSlices([]SliceInfo{parsedAnnot}, "ns-b", "train"); len(matched) != 0 {
		t.Errorf("FilterOwnedTPUSlices(ns-b, train) = %+v, want 0 matches (cross-namespace isolation)", matched)
	}
}

func TestFormatSliceReadiness(t *testing.T) {
	t.Parallel()
	if got := FormatSliceReadiness(nil); got != "" {
		t.Errorf("FormatSliceReadiness(nil) = %q, want empty", got)
	}
	pending := SliceInfo{
		Name:               "my-job-main-job-0",
		TPUType:            "tpu7x",
		Topology:           "2x2x1",
		State:              "NotCreated",
		Ready:              false,
		ReadyMessage:       "waiting on slice controller",
		NodesLabeled:       false,
		NodesLabeledKnown:  true,
		NodesLabeledReason: "LabelingNodes",
	}
	formatted := FormatSliceReadiness([]SliceInfo{pending})
	for _, want := range []string{"my-job-main-job-0", "waiting for TPU capacity / gang completion", "Ready=False reason=NotCreated: waiting on slice controller", "SliceNodesLabeled=False reason=LabelingNodes"} {
		if !strings.Contains(formatted, want) {
			t.Errorf("FormatSliceReadiness missing %q:\n%s", want, formatted)
		}
	}

	withPart := pending
	withPart.PartitionIDs = []string{"p-0"}
	if out := FormatSliceReadiness([]SliceInfo{withPart}); strings.Contains(out, "spec.partitionIds is empty") || !strings.Contains(out, "Ready=False reason=NotCreated") {
		t.Errorf("FormatSliceReadiness(with partitionIds) = %q", out)
	}
	readyOut := FormatSliceReadiness([]SliceInfo{{Name: "s-ready", TPUType: "tpu7x", Topology: "2x2x2", Ready: true, State: "READY", NodesLabeledKnown: true, NodesLabeled: true}})
	if readyOut != "Slice s-ready (type=tpu7x, topology=2x2x2): Ready (READY)" {
		t.Errorf("FormatSliceReadiness(ready) = %q", readyOut)
	}
	readyEmptyState := FormatSliceReadiness([]SliceInfo{{Name: "s-plain", Ready: true}})
	if readyEmptyState != "Slice s-plain: Ready (Ready)" {
		t.Errorf("FormatSliceReadiness(ready empty state) = %q", readyEmptyState)
	}
	readyUnlabeled := FormatSliceReadiness([]SliceInfo{{Name: "s-lbl", TPUType: "tpu7x", Topology: "2x2x1", Ready: true, State: "READY", NodesLabeledKnown: true, NodesLabeled: false, NodesLabeledReason: "LabelingNodes"}})
	if !strings.Contains(readyUnlabeled, "Ready (READY); SliceNodesLabeled=False reason=LabelingNodes") {
		t.Errorf("FormatSliceReadiness(ready + unlabeled) = %q", readyUnlabeled)
	}
	termOut := FormatSliceReadiness([]SliceInfo{{Name: "s-term", TPUType: "tpu7x", Topology: "2x2x1", Ready: false, Terminating: true, State: "Terminating"}})
	if termOut != "Slice s-term (type=tpu7x, topology=2x2x1): Terminating" {
		t.Errorf("FormatSliceReadiness(terminating) = %q, want clean Terminating without capacity wait", termOut)
	}
}

func TestParseTPUSliceListJSON(t *testing.T) {
	t.Parallel()
	listJSON := `{"items":[null,{"metadata":{"name":"s2","deletionTimestamp":"2026-11-25T00:00:00Z"},"spec":{"type":"tpu7x","topology":"2x2x1"},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY","message":"synced"}]}},{"metadata":{"name":"s1"},"spec":{"type":"v6e","topology":"2x2","partitionIds":["p0"]},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY"}]}}]}`
	list, err := ParseTPUSliceListJSON(listJSON, testHardwareResolver{})
	if err != nil || len(list) != 2 {
		t.Fatalf("ParseTPUSliceListJSON = (%+v, %v), want 2 sorted non-null slices", list, err)
	}
	if list[0].Name != "s1" || !list[0].Ready || list[0].MachineType == "" {
		t.Errorf("list[0] = %+v, want sorted ready s1 with resolved MachineType", list[0])
	}
	if !list[1].Terminating || list[1].Ready || list[1].State != "Terminating" {
		t.Errorf("list[1] (terminating) = %+v, want Terminating=true Ready=false State=Terminating", list[1])
	}
	if list[1].Name != "s2" || list[1].ReadyMessage != "" {
		t.Errorf("list[1] name/message = (%q, %q), want (s2, '')", list[1].Name, list[1].ReadyMessage)
	}
}

func TestParseTPUSliceListJSON_EdgeCases(t *testing.T) {
	t.Parallel()
	if emptyList, err := ParseTPUSliceListJSON("  ", nil); err != nil || len(emptyList) != 0 {
		t.Errorf("ParseTPUSliceListJSON(empty) = (%+v, %v), want nil, nil", emptyList, err)
	}
	for _, bad := range []string{"{bad", "null"} {
		if _, err := ParseTPUSliceListJSON(bad, nil); err == nil {
			t.Errorf("ParseTPUSliceListJSON(%q) = nil, want error", bad)
		}
	}
	if unnamed, err := ParseTPUSliceListJSON(`{"items":[{"metadata":{}},{"metadata":{"name":"s-ok"}}]}`, nil); err != nil || len(unnamed) != 1 || unnamed[0].Name != "s-ok" {
		t.Errorf("ParseTPUSliceListJSON(unnamed item) = (%+v, %v), want [s-ok]", unnamed, err)
	}
}

func TestParseTPUSliceJSON(t *testing.T) {
	t.Parallel()
	singleJSON := `{"metadata":{"name":"s3"},"spec":{"type":"tpu7x","topology":"2x2x2","partitionIds":["p1"]},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY"}]}}`
	single, err := ParseTPUSliceJSON(singleJSON, nil)
	if err != nil || single.Name != "s3" || single.TPUType != "tpu7x" || !single.Ready {
		t.Fatalf("ParseTPUSliceJSON = (%+v, %v), want ready tpu7x slice s3", single, err)
	}
	for _, bad := range []string{"", "null", "{}", "{bad", `{"metadata":{}}`, `{"kind":"Status","status":"Failure"}`} {
		if _, err := ParseTPUSliceJSON(bad, nil); err == nil {
			t.Errorf("ParseTPUSliceJSON(%q) = nil, want error", bad)
		}
	}
}

func TestFilterSliceEventsJSON_ChronologicalCapAndAPIVersionFilter(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	var events []corev1.Event
	for i := 0; i < 20; i++ {
		events = append(events, corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: fmt.Sprintf("ev-%02d", i)},
			InvolvedObject: corev1.ObjectReference{APIVersion: TPUSliceAPIGroupVersion + "/v1alpha1", Kind: "Slice", Name: "slice-b"},
			Type:           "Normal",
			Reason:         "Scheduling",
			LastTimestamp:  metav1.NewTime(base.Add(time.Duration(i) * time.Second)),
			Message:        fmt.Sprintf("older-%02d", i),
		})
	}
	events = append(events,
		corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-other-group"},
			InvolvedObject: corev1.ObjectReference{APIVersion: "other.io/v1", Kind: "Slice", Name: "slice-a"},
			Type:           "Warning",
			Reason:         "WrongGroup",
			LastTimestamp:  metav1.NewTime(base.Add(200 * time.Second)),
			Message:        "must be filtered out",
		},
		corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-other-kind"},
			InvolvedObject: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Name: "slice-a"},
			Type:           "Warning",
			Reason:         "WrongKind",
			LastTimestamp:  metav1.NewTime(base.Add(201 * time.Second)),
			Message:        "pod must be filtered out",
		},
		corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-newest-series"},
			InvolvedObject: corev1.ObjectReference{APIVersion: TPUSliceAPIGroupVersion + "/v1beta1", Kind: "Slice", Name: "slice-a"},
			Type:           "Warning",
			Reason:         "CapacityWait",
			Series:         &corev1.EventSeries{LastObservedTime: metav1.NewMicroTime(base.Add(100 * time.Second))},
			Message:        "line1\nline2\r\nline3",
		},
		corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev-mid-empty-apiversion"},
			InvolvedObject: corev1.ObjectReference{Name: "slice-a"},
			Type:           "Normal",
			Reason:         "Queued",
			EventTime:      metav1.NewMicroTime(base.Add(50 * time.Second)),
			Message:        "queued in gang",
		},
	)
	raw, err := json.Marshal(corev1.EventList{Items: events})
	if err != nil {
		t.Fatal(err)
	}
	out := FilterSliceEventsJSON(string(raw), map[string]bool{"slice-a": true, "slice-b": true})
	lines := strings.Split(out, "\n")
	if len(lines) != MaxSliceEvents {
		t.Fatalf("len(lines) = %d, want %d:\n%s", len(lines), MaxSliceEvents, out)
	}
	for _, forbid := range []string{"older-00", "older-01", "WrongGroup", "WrongKind"} {
		if strings.Contains(out, forbid) {
			t.Errorf("FilterSliceEventsJSON unexpectedly included %q:\n%s", forbid, out)
		}
	}
	if !strings.HasSuffix(lines[len(lines)-1], "slice-a [Warning] CapacityWait: line1 | line2 | line3") {
		t.Errorf("last event = %q, want newest multi-line slice-a event", lines[len(lines)-1])
	}
	if !strings.Contains(lines[len(lines)-2], "slice-a [Normal] Queued: queued in gang") {
		t.Errorf("second-to-last event = %q, want empty-apiVersion slice-a event", lines[len(lines)-2])
	}
}

func TestParseTPUSliceJSON_OmittedTopologyMultiHostFamilies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		raw         string
		wantMachine string
		wantChips   int
	}{
		{
			name:        "tpu7x_omitted_topology",
			raw:         `{"metadata":{"name":"s-7x"},"spec":{"type":"tpu7x"}}`,
			wantMachine: "",
			wantChips:   0,
		},
		{
			name:        "v6e_omitted_topology",
			raw:         `{"metadata":{"name":"s-6e"},"spec":{"type":"v6e"}}`,
			wantMachine: "",
			wantChips:   0,
		},
		{
			name:        "v5e_omitted_topology",
			raw:         `{"metadata":{"name":"s-5e"},"spec":{"type":"v5e"}}`,
			wantMachine: "",
			wantChips:   0,
		},
		{
			name:        "v5p_omitted_topology_single_host_size",
			raw:         `{"metadata":{"name":"s-5p"},"spec":{"type":"v5p"}}`,
			wantMachine: "ct5p-hightpu-4t",
			wantChips:   4,
		},
		{
			name:        "v6e_ambiguous_8_chips",
			raw:         `{"metadata":{"name":"s-6e-8"},"spec":{"type":"v6e","topology":"2x4"}}`,
			wantMachine: "",
			wantChips:   0,
		},
	}
	for _, tc := range cases {
		got, err := ParseTPUSliceJSON(tc.raw, testHardwareResolver{})
		if err != nil {
			t.Fatalf("%s: ParseTPUSliceJSON err = %v", tc.name, err)
		}
		if got.MachineType != tc.wantMachine || got.ChipsPerNode != tc.wantChips {
			t.Errorf("%s: got (MachineType=%q, ChipsPerNode=%d), want (%q, %d)",
				tc.name, got.MachineType, got.ChipsPerNode, tc.wantMachine, tc.wantChips)
		}
	}
}
