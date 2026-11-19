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
	"slices"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeDiscoveryLister struct {
	resources map[schema.GroupVersionResource][]unstructured.Unstructured
	errs      map[schema.GroupVersionResource]error
	called    []schema.GroupVersionResource
}

func (f *fakeDiscoveryLister) ListResources(gvr schema.GroupVersionResource, _, _, _ string) ([]unstructured.Unstructured, error) {
	f.called = append(f.called, gvr)
	if err := f.errs[gvr]; err != nil {
		return nil, err
	}
	return f.resources[gvr], nil
}

type fakeAPIGroupChecker struct {
	groups map[string]bool
	err    error
	called []string
}

func (f *fakeAPIGroupChecker) HasAPIGroupVersion(gv string) (bool, error) {
	f.called = append(f.called, gv)
	if f.err != nil {
		return false, f.err
	}
	return f.groups[gv], nil
}

func makeNode(name string, labels map[string]string, alloc map[string]interface{}, ready bool) unstructured.Unstructured {
	readyStatus := "False"
	if ready {
		readyStatus = "True"
	}
	return unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name":   name,
				"labels": stringMapToAny(labels),
			},
			"status": map[string]interface{}{
				"allocatable": alloc,
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": readyStatus,
					},
				},
			},
		},
	}
}

func stringMapToAny(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func makeTPUSlice(name, tpuType, topology, state string, labels map[string]string) unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": TPUSliceAPIGroupVersion,
		"kind":       "Slice",
		"metadata": map[string]interface{}{
			"name":   name,
			"labels": stringMapToAny(labels),
		},
		"spec": map[string]interface{}{
			"type":     tpuType,
			"topology": topology,
		},
		"status": map[string]interface{}{},
	}
	if state != "" {
		obj["status"].(map[string]interface{})["state"] = state
	}
	return unstructured.Unstructured{Object: obj}
}

func makePartitionTree(name, tpuType, rootTopo string, subTopos []interface{}) unstructured.Unstructured {
	return unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": TPUSliceAPIGroupVersion,
			"kind":       "PartitionTree",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"type":       tpuType,
				"topology":   rootTopo,
				"partitions": subTopos,
			},
		},
	}
}

func TestParseLabelSchemeAndEnvOverride(t *testing.T) {
	tests := []struct {
		raw     string
		want    LabelScheme
		wantErr bool
	}{
		{raw: "", want: SchemeAuto},
		{raw: "  ", want: SchemeAuto},
		{raw: "tpu-slice", want: SchemeTPUSlice},
		{raw: " TPU-SLICE ", want: SchemeTPUSlice},
		{raw: "gke", want: SchemeGKE},
		{raw: "generic", want: SchemeGeneric},
		{raw: "unknown", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParseLabelScheme(tc.raw)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseLabelScheme(%q) err=%v, wantErr=%v", tc.raw, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("ParseLabelScheme(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}

	env := func(val string) func(string) string {
		return func(k string) string {
			if k == LabelSchemeEnvVar {
				return val
			}
			return ""
		}
	}
	// Flag takes precedence over env (even if env is invalid).
	got, err := ResolveLabelSchemeOverride("tpu-slice", env("invalid"))
	if err != nil || got != SchemeTPUSlice {
		t.Fatalf("flag precedence failed: got (%q, %v), want (tpu-slice, nil)", got, err)
	}
	// Empty flag reads env via os.Getenv when getenv is nil.
	t.Setenv(LabelSchemeEnvVar, "gke")
	got, err = ResolveLabelSchemeOverride("", nil)
	if err != nil || got != SchemeGKE {
		t.Fatalf("env fallback failed: got (%q, %v), want (gke, nil)", got, err)
	}
	// Invalid env with empty flag fails fast.
	if _, err := ResolveLabelSchemeOverride("", env("bogus")); err == nil || !strings.Contains(err.Error(), "tpu-slice, gke, generic") {
		t.Fatalf("expected allowed-values error for invalid %s, got: %v", LabelSchemeEnvVar, err)
	}
}

func TestNormalizeTPUTypeAndInferMachine(t *testing.T) {
	tests := []struct {
		rawType     string
		rawMachine  string
		chips       int
		wantType    string
		wantMachine string
	}{
		{rawType: "tpu-v5p-slice", rawMachine: "", chips: 4, wantType: "v5p", wantMachine: "ct5p-hightpu-4t"},
		{rawType: "", rawMachine: "ct5p-hightpu-4t", chips: 4, wantType: "v5p", wantMachine: "ct5p-hightpu-4t"},
		{rawType: "v5p-8", rawMachine: "v5p", chips: 4, wantType: "v5p", wantMachine: "ct5p-hightpu-4t"},
		{rawType: "", rawMachine: "tpu-v5p-slice", chips: 4, wantType: "v5p", wantMachine: "ct5p-hightpu-4t"},
		{rawType: "v5e", rawMachine: "ct5lp-hightpu-8t", chips: 8, wantType: "v5e", wantMachine: "ct5lp-hightpu-8t"},
		{rawType: "tpu-v5-lite-podslice", rawMachine: "", chips: 1, wantType: "v5e", wantMachine: "ct5lp-hightpu-1t"},
		{rawType: "v5litepod-8", rawMachine: "", chips: 3, wantType: "v5e", wantMachine: "ct5lp-hightpu-4t"},
		{rawType: "tpu-v6e-slice", rawMachine: "", chips: 8, wantType: "v6e", wantMachine: "ct6e-standard-8t"},
		{rawType: "", rawMachine: "ct6e-standard-1t", chips: 1, wantType: "v6e", wantMachine: "ct6e-standard-1t"},
		{rawType: "tpu7x", rawMachine: "", chips: 4, wantType: "tpu7x", wantMachine: "tpu7x-standard-4t"},
		{rawType: "tpu-7x", rawMachine: "", chips: 3, wantType: "tpu7x", wantMachine: "tpu7x-standard-4t"},
		{rawType: "ironwood", rawMachine: "", chips: 0, wantType: "tpu7x", wantMachine: "tpu7x-standard-4t"},
		{rawType: "", rawMachine: "ironwood", chips: 4, wantType: "tpu7x", wantMachine: "tpu7x-standard-4t"},
		{rawType: "tpu-v4-podslice", rawMachine: "", chips: 4, wantType: "v4", wantMachine: "ct4p-hightpu-4t"},
		{rawType: "v4-8", rawMachine: "", chips: 4, wantType: "v4", wantMachine: "ct4p-hightpu-4t"},
		{rawType: "v5p", rawMachine: "tpu-metal-host", chips: 4, wantType: "v5p", wantMachine: "tpu-metal-host"},
		{rawType: "v5p", rawMachine: "custom-tpu-metal", chips: 4, wantType: "v5p", wantMachine: "custom-tpu-metal"},
		{rawType: "n2-standard-8", rawMachine: "", chips: 0, wantType: "n2-standard-8", wantMachine: ""},
		{rawType: "", rawMachine: "", chips: 0, wantType: "", wantMachine: ""},
	}
	for _, tc := range tests {
		gotType, gotMachine := InferTPUTypeAndMachine(tc.rawType, tc.rawMachine, tc.chips)
		if gotType != tc.wantType || gotMachine != tc.wantMachine {
			t.Errorf("InferTPUTypeAndMachine(%q, %q, %d) = (%q, %q), want (%q, %q)",
				tc.rawType, tc.rawMachine, tc.chips, gotType, gotMachine, tc.wantType, tc.wantMachine)
		}
	}
}

func TestDiscoverInventory_TPUSliceMixedPoolDeterministic(t *testing.T) {
	cpuNode := makeNode("ctrl-0", map[string]string{
		InstanceTypeLabel:     "n2-standard-8",
		ControlPlaneRoleLabel: "",
	}, map[string]interface{}{
		"cpu":    "8",
		"memory": "32Gi",
	}, true)
	tpuNode0 := makeNode("tpu-0", map[string]string{
		InstanceTypeLabel:                  "ct5p-hightpu-4t",
		TPUSliceNameLabel:                  "slice-v5p-0",
		TPUSliceTypeLabel:                  "v5p",
		TPUSliceTopologyLabel:              "2x2x2",
		TPUSlicePartitionLabelPrefix + "0": "p0",
	}, map[string]interface{}{
		"cpu":            "208",
		"memory":         "440Gi",
		"google.com/tpu": "4",
	}, true)
	tpuNode1 := makeNode("tpu-1", map[string]string{
		InstanceTypeLabel:     "ct5p-hightpu-4t",
		TPUSliceNameLabel:     "slice-v5p-0",
		TPUSliceTypeLabel:     "v5p",
		TPUSliceTopologyLabel: "2x2x2",
	}, map[string]interface{}{
		"cpu":            "208",
		"memory":         "440Gi",
		"google.com/tpu": "4",
	}, true)

	sliceObj := makeTPUSlice("slice-v5p-0", "v5p", "2x2x2", "READY", map[string]string{
		TPUSliceOwnerNameLabel: "job-1",
		TPUSliceManagedByLabel: "slice-scheduler",
	})
	treeObj := makePartitionTree("pt-v5p", "v5p", "2x2x2", []interface{}{
		map[string]interface{}{
			"topology": "2x2x2",
			"children": []interface{}{
				map[string]interface{}{"topology": "2x2x1"},
			},
		},
	})

	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
	var firstSummary []string

	for run := 0; run < 25; run++ {
		nodes := []unstructured.Unstructured{cpuNode, tpuNode0, tpuNode1}
		if run%2 == 1 {
			nodes = []unstructured.Unstructured{tpuNode1, cpuNode, tpuNode0}
		}
		lister := &fakeDiscoveryLister{
			resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:             nodes,
				TPUSliceGVR:         {sliceObj},
				TPUPartitionTreeGVR: {treeObj},
			},
		}
		inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", run, err)
		}
		assertTPUSliceMixedInventory(t, inv)
		summary := inv.SummaryLines()
		if run == 0 {
			firstSummary = summary
			inv.LogSummary()
		} else if !slices.Equal(summary, firstSummary) {
			t.Fatalf("run %d: non-deterministic summary:\ngot:  %v\nwant: %v", run, summary, firstSummary)
		}
	}
}

func assertTPUSliceMixedInventory(t *testing.T, inv *Inventory) {
	t.Helper()
	assertTPUSlicePoolCaps(t, inv)
	assertTPUSliceTPU(t, inv)
	assertTPUSliceCRDs(t, inv)
}

func assertTPUSlicePoolCaps(t *testing.T, inv *Inventory) {
	t.Helper()
	if inv.LabelScheme != SchemeTPUSlice || !inv.TPUSliceAPIPresent || inv.ExplicitMode {
		t.Fatalf("unexpected header state: scheme=%q tpuSliceAPI=%v explicit=%v", inv.LabelScheme, inv.TPUSliceAPIPresent, inv.ExplicitMode)
	}
	if wantPools := []string{"ct5p-hightpu-4t", "n2-standard-8"}; !slices.Equal(inv.PoolNames, wantPools) {
		t.Fatalf("PoolNames = %v, want %v", inv.PoolNames, wantPools)
	}
	tpuCap := inv.DefaultPoolCap(true)
	if tpuCap == nil || tpuCap.MachineType != "ct5p-hightpu-4t" || tpuCap.CPUsPerNode != 208 || tpuCap.MemMiBPerNode != 440*1024 {
		t.Fatalf("TPUPoolCap = %+v, want ct5p-hightpu-4t 208 vCPU / %d MiB", tpuCap, 440*1024)
	}
	cpuCap := inv.DefaultPoolCap(false)
	if cpuCap == nil || cpuCap.MachineType != "n2-standard-8" || cpuCap.CPUsPerNode != 8 || cpuCap.MemMiBPerNode != 32*1024 {
		t.Fatalf("CPUPoolCap = %+v, want n2-standard-8 8 vCPU / %d MiB", cpuCap, 32*1024)
	}
}

func assertTPUSliceTPU(t *testing.T, inv *Inventory) {
	t.Helper()
	if inv.TPU == nil || inv.TPU.TPUType != "v5p" || inv.TPU.Topology != "2x2x2" || inv.TPU.NodesPerSlice != 2 {
		t.Fatalf("unexpected TPU inventory: %+v", inv.TPU)
	}
	if inv.TPU.ChipsPerNode != 4 || inv.TPU.DefaultSliceName != "slice-v5p-0" || !inv.TPU.HasPartitionLabels {
		t.Fatalf("unexpected TPU slice/partition metadata: %+v", inv.TPU)
	}
	if !slices.Equal(inv.TPU.Topologies, []string{"2x2x1", "2x2x2"}) {
		t.Fatalf("TPU.Topologies = %v, want [2x2x1 2x2x2]", inv.TPU.Topologies)
	}
}

func assertTPUSliceCRDs(t *testing.T, inv *Inventory) {
	t.Helper()
	if len(inv.Slices) != 1 || !inv.Slices[0].FromCRD || inv.Slices[0].MachineType != "ct5p-hightpu-4t" {
		t.Fatalf("unexpected Slices (expected deduplicated CRD+node merge): %+v", inv.Slices)
	}
	if len(inv.PartitionTrees) != 1 || inv.PartitionTrees[0].RootTopology != "2x2x2" {
		t.Fatalf("unexpected PartitionTrees: %+v", inv.PartitionTrees)
	}
}

func TestDiscoverInventory_ExplicitMode_403Forbidden(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("cannot list nodes"))

	tests := []struct {
		name           string
		tpuSliceAPI    bool
		override       LabelScheme
		wantScheme     LabelScheme
		wantTPUSlice   bool
		wantCRDQueries bool
	}{
		{
			name:           "403 with TPU slice API present auto-detects SchemeTPUSlice",
			tpuSliceAPI:    true,
			wantScheme:     SchemeTPUSlice,
			wantTPUSlice:   true,
			wantCRDQueries: true,
		},
		{
			name:           "403 with TPU slice API absent falls back to SchemeGeneric",
			tpuSliceAPI:    false,
			wantScheme:     SchemeGeneric,
			wantTPUSlice:   false,
			wantCRDQueries: false,
		},
		{
			name:           "403 with TPU slice API absent honors --k8s-label-scheme=tpu-slice",
			tpuSliceAPI:    false,
			override:       SchemeTPUSlice,
			wantScheme:     SchemeTPUSlice,
			wantTPUSlice:   false,
			wantCRDQueries: true,
		},
		{
			name:           "403 with TPU slice API present honors --k8s-label-scheme=generic and skips CRDs",
			tpuSliceAPI:    true,
			override:       SchemeGeneric,
			wantScheme:     SchemeGeneric,
			wantTPUSlice:   true,
			wantCRDQueries: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lister := &fakeDiscoveryLister{
				errs: map[schema.GroupVersionResource]error{
					NodeGVR:             forbidden,
					TPUSliceGVR:         forbidden,
					TPUPartitionTreeGVR: forbidden,
				},
			}
			checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: tc.tpuSliceAPI}}

			inv, err := DiscoverInventory(lister, checker, DiscoverOptions{LabelSchemeOverride: tc.override})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !inv.ExplicitMode {
				t.Fatalf("expected ExplicitMode=true")
			}
			if inv.LabelScheme != tc.wantScheme {
				t.Errorf("LabelScheme = %q, want %q", inv.LabelScheme, tc.wantScheme)
			}
			if inv.TPUSliceAPIPresent != tc.wantTPUSlice {
				t.Errorf("TPUSliceAPIPresent = %v, want %v", inv.TPUSliceAPIPresent, tc.wantTPUSlice)
			}
			queriedCRDs := slices.Contains(lister.called, TPUSliceGVR)
			if queriedCRDs != tc.wantCRDQueries {
				t.Errorf("queriedCRDs = %v, want %v (called=%v)", queriedCRDs, tc.wantCRDQueries, lister.called)
			}
			lines := inv.SummaryLines()
			if len(lines) == 0 || !strings.Contains(lines[0], "explicit mode") {
				t.Errorf("unexpected explicit-mode summary: %v", lines)
			}
		})
	}
}

func TestDiscoverInventory_ExplicitMode_SliceTakesPrecedenceOverOtherFamilyTree(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("forbidden"))
	v5pSlice := makeTPUSlice("slice-v5p-ready", "v5p", "2x2x2", "READY", nil)
	v6eTree := makePartitionTree("pt-v6e", "v6e", "4x4", []interface{}{"2x2"})

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			TPUSliceGVR:         {v5pSlice},
			TPUPartitionTreeGVR: {v6eTree},
		},
		errs: map[schema.GroupVersionResource]error{NodeGVR: forbidden},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}

	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.TPU == nil || inv.TPU.DefaultSliceName != "slice-v5p-ready" || inv.TPU.TPUType != "v5p" {
		t.Fatalf("expected Ready v5p slice to anchor TPUInventory over v6e PartitionTree, got %+v", inv.TPU)
	}
	if !slices.Equal(inv.TPU.Topologies, []string{"2x2x2"}) {
		t.Fatalf("TPU.Topologies = %v, want [2x2x2] (no cross-family v6e topologies)", inv.TPU.Topologies)
	}
	if lines := inv.SummaryLines(); len(lines) < 2 || !strings.Contains(lines[1], "slice-v5p-ready") {
		t.Errorf("expected explicit-mode TPU summary line, got %v", lines)
	}
}

func TestDiscoverInventory_GKEClusterSkipsTPUSliceCRDs(t *testing.T) {
	gkeNode := makeNode("gke-tpu-0", map[string]string{
		GKENodePoolLabel:       "tpu-pool",
		GKETPUAcceleratorLabel: "tpu-v5p-slice",
		GKETPUTopologyLabel:    "2x2x2",
		InstanceTypeLabel:      "ct5p-hightpu-4t",
	}, map[string]interface{}{
		"cpu":            "208",
		"memory":         "440Gi",
		"google.com/tpu": "4",
	}, true)
	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {gkeNode},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: false}}

	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.LabelScheme != SchemeGKE || inv.TPUSliceAPIPresent {
		t.Fatalf("unexpected GKE discovery state: scheme=%q tpuSliceAPI=%v", inv.LabelScheme, inv.TPUSliceAPIPresent)
	}
	if inv.TPU == nil || inv.TPU.TPUType != "v5p" || inv.TPU.NodesPerSlice != 2 || inv.TPU.Topology != "2x2x2" {
		t.Fatalf("unexpected GKE TPU inventory: %+v", inv.TPU)
	}
	for _, gvr := range lister.called {
		if gvr == TPUSliceGVR || gvr == TPUPartitionTreeGVR {
			t.Errorf("GKE cluster should not query TPU Slice CRD %v", gvr)
		}
	}
}

func TestDiscoverInventory_CPUPoolSelection(t *testing.T) {
	ctrlNode := makeNode("ctrl", map[string]string{
		InstanceTypeLabel:     "n2-standard-8",
		ControlPlaneRoleLabel: "",
	}, map[string]interface{}{"cpu": "7910m", "memory": "32Gi"}, true)
	workerSmall := makeNode("worker-0", map[string]string{
		InstanceTypeLabel: "n2-standard-32",
	}, map[string]interface{}{"cpu": "31900m", "memory": "120Gi"}, true)
	workerLarge := makeNode("worker-1", map[string]string{
		InstanceTypeLabel: "n2-standard-32",
	}, map[string]interface{}{"cpu": "32", "memory": "128Gi"}, true)
	gpuNode := makeNode("gpu-0", map[string]string{
		BetaInstanceTypeLabel: "g2-standard-48",
	}, map[string]interface{}{"cpu": "48", "memory": "192Gi", "nvidia.com/gpu": "4"}, true)

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {ctrlNode, workerLarge, workerSmall, gpuNode},
		},
		errs: map[schema.GroupVersionResource]error{
			TPUSliceGVR:         apierrors.NewNotFound(schema.GroupResource{Resource: "slices"}, ""),
			TPUPartitionTreeGVR: apierrors.NewNotFound(schema.GroupResource{Resource: "partitiontrees"}, ""),
		},
	}

	inv, err := DiscoverInventory(lister, nil, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.LabelScheme != SchemeGeneric {
		t.Fatalf("LabelScheme = %q, want generic", inv.LabelScheme)
	}
	if inv.CPUPoolCap == nil || inv.CPUPoolCap.MachineType != "n2-standard-32" {
		t.Fatalf("expected non-control-plane CPU pool n2-standard-32, got %+v", inv.CPUPoolCap)
	}
	if inv.CPUPoolCap.CPUsPerNode != 31 || inv.CPUPoolCap.MemMiBPerNode != 120*1024 {
		t.Errorf("CPUPoolCap = (%d vCPU, %d MiB), want (31, %d)", inv.CPUPoolCap.CPUsPerNode, inv.CPUPoolCap.MemMiBPerNode, 120*1024)
	}
	if inv.Pools["n2-standard-8"].CPUsPerNode != 7 {
		t.Errorf("ctrl node 7910m cpu = %d, want 7 (floored)", inv.Pools["n2-standard-8"].CPUsPerNode)
	}
	if !inv.Pools["g2-standard-48"].IsGPU || inv.Pools["g2-standard-48"].GPUsPerNode != 4 {
		t.Errorf("unexpected GPU pool: %+v", inv.Pools["g2-standard-48"])
	}
	if inv.DefaultPoolCap(true) != nil {
		t.Errorf("expected nil TPUPoolCap on non-TPU cluster, got %+v", inv.DefaultPoolCap(true))
	}
}

func TestDiscoverInventory_SharedMachineTypesIsolated(t *testing.T) {
	cpuN1 := makeNode("cpu-n1", map[string]string{InstanceTypeLabel: "n1-standard-16"}, map[string]interface{}{"cpu": "16", "memory": "60Gi"}, true)
	gpuN1 := makeNode("gpu-n1", map[string]string{InstanceTypeLabel: "n1-standard-16"}, map[string]interface{}{"cpu": "16", "memory": "60Gi", "nvidia.com/gpu": "2"}, true)
	ctrlN2 := makeNode("ctrl-n2", map[string]string{InstanceTypeLabel: "n2-standard-8", ControlPlaneRoleLabel: ""}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, true)
	unreadyWorkerN2 := makeNode("worker-n2", map[string]string{InstanceTypeLabel: "n2-standard-8"}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, false)

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {gpuN1, cpuN1, ctrlN2, unreadyWorkerN2},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: false}}

	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.CPUPoolCap == nil || inv.CPUPoolCap.MachineType != "n1-standard-16" || inv.CPUPoolCap.IsGPU {
		t.Fatalf("GPU pool must not poison CPUPoolCap: got %+v", inv.CPUPoolCap)
	}
	if got := inv.Pools["n2-standard-8/cpu"]; got.ReadyNodes != 0 || got.TotalNodes != 1 {
		t.Errorf("worker pool n2-standard-8/cpu = (%d ready / %d total), want (0/1) without control-plane inflation", got.ReadyNodes, got.TotalNodes)
	}
	if got := inv.Pools["n2-standard-8/control-plane"]; got.ReadyNodes != 1 || got.TotalNodes != 1 {
		t.Errorf("control-plane pool n2-standard-8/control-plane = (%d ready / %d total), want (1/1)", got.ReadyNodes, got.TotalNodes)
	}
}

func TestDiscoverInventory_CordonedNodeNotReady(t *testing.T) {
	cordoned := makeNode("tpu-cordoned", map[string]string{
		InstanceTypeLabel:     "ct5p-hightpu-4t",
		TPUSliceNameLabel:     "cordoned-slice",
		TPUSliceTypeLabel:     "v5p",
		TPUSliceTopologyLabel: "2x2x1",
	}, map[string]interface{}{"cpu": "208", "memory": "440Gi", "google.com/tpu": "4"}, true)
	cordoned.Object["spec"] = map[string]interface{}{"unschedulable": true}

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: {cordoned}},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: false}}

	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.ReadyNodes != 0 || inv.TPU.ReadyTPUNodes != 0 || inv.TPU.DefaultSliceName != "" {
		t.Fatalf("cordoned node must not be ready or populate DefaultSliceName: ready=%d tpu=%+v", inv.ReadyNodes, inv.TPU)
	}
	if len(inv.Slices) != 1 || inv.Slices[0].Ready {
		t.Fatalf("expected cordoned node-derived slice Ready=false, got %+v", inv.Slices)
	}
}

func TestDiscoverInventory_NonReadyAndConditionReadySlices(t *testing.T) {
	creatingSlice := makeTPUSlice("slice-creating", "v6e", "2x4", "CREATING", nil)
	condFalse := makeTPUSlice("slice-cond-false", "v6e", "2x4", "", nil)
	condFalse.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "False"},
		},
	}
	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     nil,
			TPUSliceGVR: {creatingSlice, condFalse},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.TPU == nil || inv.TPU.DefaultSliceName != "" || inv.TPU.TPUType != "v6e" || inv.TPU.Topology != "2x4" {
		t.Fatalf("expected non-ready slices to not populate DefaultSliceName: %+v", inv.TPU)
	}
	if lines := inv.SummaryLines(); len(lines) < 2 || !strings.Contains(lines[1], "<none>") {
		t.Errorf("expected <none> slice label in summary, got %v", lines)
	}
}

func TestDiscoverInventory_ConditionReadyAnd1x1Slices(t *testing.T) {
	creatingSlice := makeTPUSlice("slice-creating", "tpu7x", "2x2x2", "CREATING", nil)
	condReady := makeTPUSlice("slice-cond", "tpu7x", "2x2x2", "", nil)
	condReady.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
		"nodes": []interface{}{"n0", "n1"},
	}
	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     nil,
			TPUSliceGVR: {creatingSlice, condReady},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.TPU == nil || inv.TPU.DefaultSliceName != "slice-cond" || inv.TPU.NodesPerSlice != 2 {
		t.Fatalf("expected condition-ready slice as default: %+v", inv.TPU)
	}

	oneByOne := makeTPUSlice("slice-1x1", "v6e", "1x1", "READY", nil)
	lister1x1 := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     nil,
			TPUSliceGVR: {oneByOne},
		},
	}
	inv1x1, err := DiscoverInventory(lister1x1, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv1x1.TPU == nil || inv1x1.TPU.MachineType != "ct6e-standard-1t" || inv1x1.TPU.ChipsPerNode != 1 || inv1x1.TPU.NodesPerSlice != 1 {
		t.Fatalf("expected 1x1 v6e slice to infer (ct6e-standard-1t, 1 chip, 1 node), got %+v", inv1x1.TPU)
	}
}

func TestDiscoverInventory_NodeDerivedSliceAndFallbackPoolKey(t *testing.T) {
	nodeOnly := makeNode("tpu-node", map[string]string{
		TPUSliceNameLabel:     "node-slice-0",
		TPUSliceTypeLabel:     "v5p",
		TPUSliceTopologyLabel: "2x2x1",
	}, map[string]interface{}{"google.com/tpu": "4", "cpu": "100", "memory": "200Gi"}, true)
	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: {nodeOnly}},
		errs: map[schema.GroupVersionResource]error{
			TPUSliceGVR:         errors.New(`the server doesn't have a resource type "slices"`),
			TPUPartitionTreeGVR: errors.New(`the server doesn't have a resource type "partitiontrees"`),
		},
	}
	inv, err := DiscoverInventory(lister, nil, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Slices) != 1 || inv.Slices[0].FromCRD || inv.Slices[0].Name != "node-slice-0" || !inv.Slices[0].Ready {
		t.Fatalf("expected node-derived slice, got %+v", inv.Slices)
	}

	// Fallback nodepool key ("tpu-pool") with PartitionTree must keep TPUPoolCap non-nil.
	fallbackTPUNode := makeNode("tpu-fallback", map[string]string{
		GKENodePoolLabel: "tpu-pool",
	}, map[string]interface{}{"google.com/tpu": "4", "cpu": "100", "memory": "200Gi"}, true)
	treeObj := makePartitionTree("pt-v5p", "v5p", "2x2x2", nil)
	listerFB := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:             {fallbackTPUNode},
			TPUPartitionTreeGVR: {treeObj},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
	invFB, err := DiscoverInventory(listerFB, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invFB.TPUPoolCap == nil || invFB.TPUPoolCap.CPUsPerNode != 100 {
		t.Fatalf("expected non-nil TPUPoolCap for fallback pool key, got %+v", invFB.TPUPoolCap)
	}
}

func TestDiscoverInventory_ErrorsAndNilSafety(t *testing.T) {
	if _, err := DiscoverInventory(nil, nil, DiscoverOptions{}); err == nil || !strings.Contains(err.Error(), "lister is required") {
		t.Errorf("expected lister-required error for nil lister, got: %v", err)
	}
	if _, err := DiscoverInventory(&fakeDiscoveryLister{}, nil, DiscoverOptions{LabelSchemeOverride: "bad"}); err == nil || !strings.Contains(err.Error(), "invalid --k8s-label-scheme") {
		t.Errorf("expected invalid label-scheme error, got: %v", err)
	}

	// Non-403 error listing nodes must propagate (not enter explicit mode).
	lister := &fakeDiscoveryLister{
		errs: map[schema.GroupVersionResource]error{
			NodeGVR: errors.New("dial tcp: connection refused"),
		},
	}
	if _, err := DiscoverInventory(lister, nil, DiscoverOptions{}); err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected non-403 list nodes error to propagate, got: %v", err)
	}

	// Nil receiver helpers must not panic.
	var nilInv *Inventory
	if nilInv.DefaultPoolCap(true) != nil || nilInv.SummaryLines() != nil {
		t.Errorf("expected nil helpers for nil *Inventory")
	}
}

func TestDiscoverInventory_ProbeErrorFallsBackToCRDs(t *testing.T) {
	// Transient API group probe error falls back to CRD listing when non-GKE; if CRDs return 503, degrades to generic.
	lister503 := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: nil},
		errs: map[schema.GroupVersionResource]error{
			TPUSliceGVR:         errors.New("503 service unavailable"),
			TPUPartitionTreeGVR: errors.New("503 service unavailable"),
		},
	}
	errChecker := &fakeAPIGroupChecker{err: errors.New("timeout")}
	inv, err := DiscoverInventory(lister503, errChecker, DiscoverOptions{})
	if err != nil || inv.LabelScheme != SchemeGeneric {
		t.Fatalf("expected graceful fallback on apiChecker + CRD 503 error, got (%+v, %v)", inv, err)
	}
	if len(errChecker.called) != 1 || !slices.Contains(lister503.called, TPUSliceGVR) {
		t.Errorf("expected 1 apiChecker probe and CRD fallback query, got checker=%v lister=%v", errChecker.called, lister503.called)
	}

	// Transient API group probe error falls back to CRD listing and still detects SchemeTPUSlice if CRDs succeed.
	listerCRD := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     nil,
			TPUSliceGVR: {makeTPUSlice("s0", "v5p", "2x2x2", "READY", nil)},
		},
	}
	invFallback, err := DiscoverInventory(listerCRD, &fakeAPIGroupChecker{err: errors.New("timeout")}, DiscoverOptions{})
	if err != nil || invFallback.LabelScheme != SchemeTPUSlice || !invFallback.TPUSliceAPIPresent {
		t.Fatalf("expected CRD fallback after apiChecker timeout to detect SchemeTPUSlice, got (%+v, %v)", invFallback, err)
	}
}

func TestDiscoverInventory_SliceNodeEnrichmentRecomputesNodesPerSlice(t *testing.T) {
	t.Run("single visible 8-chip v6e node recomputes 4x4 NodesPerSlice=2 via topology math", func(t *testing.T) {
		// Only 1 of 2 nodes is visible, proving NodesPerSlice=2 comes from CalculateAcceleratorNodes(ct6e-standard-8t, 4x4, 8)
		// rather than raw observed node count (1) or stale 4-chip default (4).
		sliceObj := makeTPUSlice("slice-v6e-8t", "v6e", "4x4", "READY", nil)
		n0 := makeNode("v6e-0", map[string]string{
			InstanceTypeLabel:     "ct6e-standard-8t",
			TPUSliceNameLabel:     "slice-v6e-8t",
			TPUSliceTypeLabel:     "v6e",
			TPUSliceTopologyLabel: "4x4",
		}, map[string]interface{}{"cpu": "180", "memory": "720Gi", "google.com/tpu": "8"}, true)

		lister := &fakeDiscoveryLister{
			resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:     {n0},
				TPUSliceGVR: {sliceObj},
			},
		}
		checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
		inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inv.TPU == nil || inv.TPU.MachineType != "ct6e-standard-8t" || inv.TPU.ChipsPerNode != 8 || inv.TPU.NodesPerSlice != 2 {
			t.Fatalf("expected recomputed NodesPerSlice=2 for 8-chip v6e 4x4 slice, got %+v", inv.TPU)
		}
	})

	t.Run("partial repairing 4x4x4 v5p slice keeps NodesPerSlice=16 and enriches duplicate-name namespaces", func(t *testing.T) {
		sA := makeTPUSlice("shared-slice", "v5p", "4x4x4", "READY", nil)
		sA.SetNamespace("ns-a")
		sB := makeTPUSlice("shared-slice", "v5p", "4x4x4", "READY", nil)
		sB.SetNamespace("ns-b")
		oneOfSixteen := makeNode("v5p-0", map[string]string{
			InstanceTypeLabel:     "ct5p-hightpu-4t",
			TPUSliceNameLabel:     "shared-slice",
			TPUSliceTypeLabel:     "v5p",
			TPUSliceTopologyLabel: "4x4x4",
		}, map[string]interface{}{"cpu": "208", "memory": "440Gi", "google.com/tpu": "4"}, true)

		lister := &fakeDiscoveryLister{
			resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:     {oneOfSixteen},
				TPUSliceGVR: {sB, sA},
			},
		}
		checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
		inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(inv.Slices) != 2 || inv.Slices[0].Namespace != "ns-a" || inv.Slices[1].Namespace != "ns-b" {
			t.Fatalf("expected 2 sorted namespaced slices, got %+v", inv.Slices)
		}
		for _, s := range inv.Slices {
			if s.NodesPerSlice != 16 || s.ChipsPerNode != 4 {
				t.Errorf("slice %s/%s = (nodes=%d, chips=%d), want (16, 4)", s.Namespace, s.Name, s.NodesPerSlice, s.ChipsPerNode)
			}
		}
	})
}

func TestDiscoverInventory_HeterogeneousTPUFamilyDoesNotGraftSlice(t *testing.T) {
	v5pNode := makeNode("v5p-0", map[string]string{
		InstanceTypeLabel:     "ct5p-hightpu-4t",
		TPUSliceTypeLabel:     "v5p",
		TPUSliceTopologyLabel: "2x2x2",
	}, map[string]interface{}{"cpu": "208", "memory": "440Gi", "google.com/tpu": "4"}, true)
	v6eSlice := makeTPUSlice("slice-v6e-only", "v6e", "2x2", "READY", nil)

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     {v5pNode},
			TPUSliceGVR: {v6eSlice},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.TPU == nil || inv.TPU.TPUType != "v5p" || inv.TPU.DefaultSliceName != "" || inv.TPU.Topology != "2x2x2" {
		t.Fatalf("mismatched v6e slice must not graft onto v5p TPUInventory: %+v", inv.TPU)
	}
	if !slices.Equal(inv.TPU.Topologies, []string{"2x2x2"}) {
		t.Fatalf("TPU.Topologies = %v, want [2x2x2] (no cross-family 2x2)", inv.TPU.Topologies)
	}
}

func TestPoolSelectionTieBreakers(t *testing.T) {
	readySmall := PoolCapacity{MachineType: "b", ReadyNodes: 1, CPUsPerNode: 8}
	unreadyLarge := PoolCapacity{MachineType: "a", ReadyNodes: 0, CPUsPerNode: 32}
	readyLarge := PoolCapacity{MachineType: "z", ReadyNodes: 1, CPUsPerNode: 32}
	readySmallA := PoolCapacity{MachineType: "a", ReadyNodes: 1, CPUsPerNode: 8}

	for _, tc := range []struct {
		name  string
		a     PoolCapacity
		aCtrl bool
		b     PoolCapacity
		bCtrl bool
	}{
		{"ready over unready", readySmall, false, unreadyLarge, false},
		{"larger over smaller", readyLarge, false, readySmall, false},
		{"lexical tie-breaker", readySmallA, false, readySmall, false},
		{"worker over control-plane", readySmall, false, readyLarge, true},
	} {
		if !isBetterCPUPool(tc.a, tc.aCtrl, &tc.b, tc.bCtrl) || isBetterCPUPool(tc.b, tc.bCtrl, &tc.a, tc.aCtrl) {
			t.Errorf("CPU tie-breaker %q failed symmetry", tc.name)
		}
	}

	tpuReady1 := PoolCapacity{MachineType: "tpu7x-standard-4t", ReadyNodes: 1, TotalNodes: 1}
	tpuUnready4 := PoolCapacity{MachineType: "ct5p-hightpu-4t", ReadyNodes: 0, TotalNodes: 4}
	tpuReady4 := PoolCapacity{MachineType: "tpu7x-standard-4t", ReadyNodes: 2, TotalNodes: 4}
	tpuReady2A := PoolCapacity{MachineType: "ct5p-hightpu-4t", ReadyNodes: 2, TotalNodes: 2}
	tpuReady2Z := PoolCapacity{MachineType: "tpu7x-standard-4t", ReadyNodes: 2, TotalNodes: 2}

	for _, tc := range []struct {
		name string
		a, b PoolCapacity
	}{
		{"ready over unready", tpuReady1, tpuUnready4},
		{"larger over smaller", tpuReady4, tpuReady2A},
		{"lexical tie-breaker", tpuReady2A, tpuReady2Z},
	} {
		if !isBetterTPUPool(tc.a, &tc.b) || isBetterTPUPool(tc.b, &tc.a) {
			t.Errorf("TPU tie-breaker %q failed symmetry", tc.name)
		}
	}
}

func TestQuantityAndNestedIntParsing(t *testing.T) {
	alloc := map[string]interface{}{
		"int64":   int64(8),
		"jsonNum": json.Number("16"),
		"neg":     "-4",
		"bad":     "not-a-qty",
		"blank":   "   ",
		"badType": []string{"x"},
	}
	for k, want := range map[string]int64{"int64": 8, "jsonNum": 16} {
		if q, ok := parseQuantityField(alloc, k); !ok || q.Value() != want {
			t.Errorf("parseQuantityField(%q) = (%v, %v), want (%d, true)", k, q, ok, want)
		}
	}
	for _, k := range []string{"neg", "bad", "blank", "badType", "missing"} {
		if _, ok := parseQuantityField(alloc, k); ok {
			t.Errorf("parseQuantityField(%q) unexpectedly succeeded", k)
		}
	}

	obj := map[string]interface{}{
		"i64":     int64(4),
		"i":       int(2),
		"f64":     float64(6),
		"jsonNum": json.Number("12"),
		"badType": true,
	}
	for k, want := range map[string]int{"i64": 4, "i": 2, "f64": 6, "jsonNum": 12} {
		if n, ok := nestedInt(obj, k); !ok || n != want {
			t.Errorf("nestedInt(%q) = (%d, %v), want (%d, true)", k, n, ok, want)
		}
	}
	if _, ok := nestedInt(obj, "badType"); ok {
		t.Errorf("nestedInt(bool) unexpectedly succeeded")
	}
}

func TestDiscoverInventory_NodeOrderIndependenceAndCRDNumNodes(t *testing.T) {
	nA := makeNode("n-a", map[string]string{InstanceTypeLabel: "n2-standard-8", GKENodePoolLabel: "z-pool"}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, true)
	nB := makeNode("n-b", map[string]string{InstanceTypeLabel: "n2-standard-8", GKENodePoolLabel: "a-pool"}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, true)
	sliceWithNum := makeTPUSlice("s-num", "v5p", "2x2x2", "READY", nil)
	sliceWithNum.Object["status"].(map[string]interface{})["numNodes"] = int64(2)

	for _, order := range [][]unstructured.Unstructured{{nA, nB}, {nB, nA}} {
		lister := &fakeDiscoveryLister{
			resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:     order,
				TPUSliceGVR: {sliceWithNum},
			},
			errs: map[schema.GroupVersionResource]error{
				TPUPartitionTreeGVR: errors.New("transient apiserver 503"),
			},
		}
		checker := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
		inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inv.Pools["n2-standard-8"].NodePoolName != "a-pool" {
			t.Errorf("NodePoolName = %q, want deterministic min a-pool", inv.Pools["n2-standard-8"].NodePoolName)
		}
		if len(inv.Slices) != 1 || inv.Slices[0].NodesPerSlice != 2 {
			t.Errorf("expected status.numNodes=2 (int64) parsed, got %+v", inv.Slices)
		}
	}
}
