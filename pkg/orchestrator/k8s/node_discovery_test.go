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
	"errors"
	"slices"
	"strings"
	"testing"

	"hpc-toolkit/pkg/shell"

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

func makeBloomSlice(name, tpuType, topology, state string, labels map[string]string) unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": BloomSliceAPIGroupVersion,
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
			"apiVersion": BloomSliceAPIGroupVersion,
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

func resetAirGapState(t *testing.T) {
	t.Helper()
	t.Setenv(shell.AirGappedEnvVar, "")
	shell.SetDefaultAirGapped(false)
	t.Cleanup(func() { shell.SetDefaultAirGapped(false) })
}

func TestParseLabelSchemeAndEnvOverride(t *testing.T) {
	tests := []struct {
		raw     string
		want    LabelScheme
		wantErr bool
	}{
		{raw: "", want: SchemeAuto},
		{raw: "  ", want: SchemeAuto},
		{raw: "bloom", want: SchemeBloom},
		{raw: " BLOOM ", want: SchemeBloom},
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
	got, err := ResolveLabelSchemeOverride("bloom", env("invalid"))
	if err != nil || got != SchemeBloom {
		t.Fatalf("flag precedence failed: got (%q, %v), want (bloom, nil)", got, err)
	}
	// Empty flag reads env via os.Getenv when getenv is nil.
	t.Setenv(LabelSchemeEnvVar, "gke")
	got, err = ResolveLabelSchemeOverride("", nil)
	if err != nil || got != SchemeGKE {
		t.Fatalf("env fallback failed: got (%q, %v), want (gke, nil)", got, err)
	}
	// Invalid env with empty flag fails fast.
	if _, err := ResolveLabelSchemeOverride("", env("bogus")); err == nil {
		t.Fatalf("expected error for invalid %s", LabelSchemeEnvVar)
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
		{rawType: "v5e", rawMachine: "ct5lp-hightpu-8t", chips: 8, wantType: "v5e", wantMachine: "ct5lp-hightpu-8t"},
		{rawType: "tpu-v5-lite-podslice", rawMachine: "", chips: 1, wantType: "v5e", wantMachine: "ct5lp-hightpu-1t"},
		{rawType: "v5litepod-8", rawMachine: "", chips: 3, wantType: "v5e", wantMachine: "ct5lp-hightpu-4t"},
		{rawType: "tpu-v6e-slice", rawMachine: "", chips: 8, wantType: "v6e", wantMachine: "ct6e-standard-8t"},
		{rawType: "", rawMachine: "ct6e-standard-1t", chips: 1, wantType: "v6e", wantMachine: "ct6e-standard-1t"},
		{rawType: "tpu7x", rawMachine: "", chips: 4, wantType: "tpu7x", wantMachine: "tpu7x-standard-4t"},
		{rawType: "ironwood", rawMachine: "", chips: 0, wantType: "tpu7x", wantMachine: "tpu7x-standard-4t"},
		{rawType: "", rawMachine: "ironwood", chips: 4, wantType: "tpu7x", wantMachine: "tpu7x-standard-4t"},
		{rawType: "tpu-v4-podslice", rawMachine: "", chips: 4, wantType: "v4", wantMachine: "ct4p-hightpu-4t"},
		{rawType: "v4-8", rawMachine: "", chips: 4, wantType: "v4", wantMachine: "ct4p-hightpu-4t"},
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

func TestDiscoverInventory_BloomMixedPoolDeterministic(t *testing.T) {
	resetAirGapState(t)

	cpuNode := makeNode("ctrl-0", map[string]string{
		InstanceTypeLabel:     "n2-standard-8",
		ControlPlaneRoleLabel: "",
	}, map[string]interface{}{
		"cpu":    "8",
		"memory": "32Gi",
	}, true)
	tpuNode0 := makeNode("tpu-0", map[string]string{
		InstanceTypeLabel:               "ct5p-hightpu-4t",
		BloomSliceNameLabel:             "slice-v5p-0",
		BloomTPUTypeLabel:               "v5p",
		BloomTopologyLabel:              "2x2x2",
		BloomPartitionLabelPrefix + "0": "p0",
	}, map[string]interface{}{
		"cpu":            "208",
		"memory":         "440Gi",
		"google.com/tpu": "4",
	}, true)
	tpuNode1 := makeNode("tpu-1", map[string]string{
		InstanceTypeLabel:   "ct5p-hightpu-4t",
		BloomSliceNameLabel: "slice-v5p-0",
		BloomTPUTypeLabel:   "v5p",
		BloomTopologyLabel:  "2x2x2",
	}, map[string]interface{}{
		"cpu":            "208",
		"memory":         "440Gi",
		"google.com/tpu": "4",
	}, true)

	sliceObj := makeBloomSlice("slice-v5p-0", "v5p", "2x2x2", "READY", map[string]string{
		BloomOwnerNameLabel: "job-1",
		BloomManagedByLabel: "slice-scheduler",
	})
	treeObj := makePartitionTree("pt-v5p", "v5p", "2x2x2", []interface{}{
		map[string]interface{}{
			"topology": "2x2x2",
			"children": []interface{}{
				map[string]interface{}{"topology": "2x2x1"},
			},
		},
	})

	checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: true}}
	var firstSummary []string

	for run := 0; run < 25; run++ {
		nodes := []unstructured.Unstructured{cpuNode, tpuNode0, tpuNode1}
		if run%2 == 1 {
			nodes = []unstructured.Unstructured{tpuNode1, cpuNode, tpuNode0}
		}
		lister := &fakeDiscoveryLister{
			resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:               nodes,
				BloomSliceGVR:         {sliceObj},
				BloomPartitionTreeGVR: {treeObj},
			},
		}
		inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", run, err)
		}
		assertBloomMixedInventory(t, inv)
		summary := inv.SummaryLines()
		if run == 0 {
			firstSummary = summary
		} else if !slices.Equal(summary, firstSummary) {
			t.Fatalf("run %d: non-deterministic summary:\ngot:  %v\nwant: %v", run, summary, firstSummary)
		}
	}
	if !shell.IsKnownAirGapped() {
		t.Errorf("expected SchemeBloom detection to enable default air-gapped state")
	}
}

func assertBloomMixedInventory(t *testing.T, inv *Inventory) {
	t.Helper()
	assertBloomPoolCaps(t, inv)
	assertBloomTPU(t, inv)
	assertBloomCRDs(t, inv)
}

func assertBloomPoolCaps(t *testing.T, inv *Inventory) {
	t.Helper()
	if inv.LabelScheme != SchemeBloom || !inv.BloomAPIPresent || inv.ExplicitMode {
		t.Fatalf("unexpected header state: scheme=%q bloomAPI=%v explicit=%v", inv.LabelScheme, inv.BloomAPIPresent, inv.ExplicitMode)
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

func assertBloomTPU(t *testing.T, inv *Inventory) {
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

func assertBloomCRDs(t *testing.T, inv *Inventory) {
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
		name          string
		bloomAPI      bool
		override      LabelScheme
		wantScheme    LabelScheme
		wantAirGapped bool
		wantBloomAPI  bool
	}{
		{
			name:          "403 with Bloom API present auto-detects SchemeBloom",
			bloomAPI:      true,
			wantScheme:    SchemeBloom,
			wantAirGapped: true,
			wantBloomAPI:  true,
		},
		{
			name:          "403 with Bloom API absent falls back to SchemeGeneric",
			bloomAPI:      false,
			wantScheme:    SchemeGeneric,
			wantAirGapped: false,
			wantBloomAPI:  false,
		},
		{
			name:          "403 with Bloom API absent honors --k8s-label-scheme=bloom",
			bloomAPI:      false,
			override:      SchemeBloom,
			wantScheme:    SchemeBloom,
			wantAirGapped: true,
			wantBloomAPI:  false,
		},
		{
			name:          "403 with Bloom API present honors --k8s-label-scheme=generic",
			bloomAPI:      true,
			override:      SchemeGeneric,
			wantScheme:    SchemeGeneric,
			wantAirGapped: false,
			wantBloomAPI:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetAirGapState(t)
			lister := &fakeDiscoveryLister{
				errs: map[schema.GroupVersionResource]error{
					NodeGVR:               forbidden,
					BloomSliceGVR:         forbidden,
					BloomPartitionTreeGVR: forbidden,
				},
			}
			checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: tc.bloomAPI}}

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
			if inv.BloomAPIPresent != tc.wantBloomAPI {
				t.Errorf("BloomAPIPresent = %v, want %v", inv.BloomAPIPresent, tc.wantBloomAPI)
			}
			if shell.IsKnownAirGapped() != tc.wantAirGapped {
				t.Errorf("IsKnownAirGapped() = %v, want %v", shell.IsKnownAirGapped(), tc.wantAirGapped)
			}
			lines := inv.SummaryLines()
			if len(lines) == 0 || !strings.Contains(lines[0], "explicit mode") {
				t.Errorf("unexpected explicit-mode summary: %v", lines)
			}
		})
	}
}

func TestDiscoverInventory_GKEClusterSkipsBloomCRDs(t *testing.T) {
	resetAirGapState(t)

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
	checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: false}}

	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.LabelScheme != SchemeGKE || inv.BloomAPIPresent || shell.IsKnownAirGapped() {
		t.Fatalf("unexpected GKE discovery state: scheme=%q bloomAPI=%v airgap=%v", inv.LabelScheme, inv.BloomAPIPresent, shell.IsKnownAirGapped())
	}
	if inv.TPU == nil || inv.TPU.TPUType != "v5p" || inv.TPU.NodesPerSlice != 2 || inv.TPU.Topology != "2x2x2" {
		t.Fatalf("unexpected GKE TPU inventory: %+v", inv.TPU)
	}
	for _, gvr := range lister.called {
		if gvr == BloomSliceGVR || gvr == BloomPartitionTreeGVR {
			t.Errorf("GKE cluster should not query Bloom CRD %v", gvr)
		}
	}
}

func TestDiscoverInventory_CPUPoolSelectionAndEdgeCases(t *testing.T) {
	resetAirGapState(t)

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
			BloomSliceGVR:         apierrors.NewNotFound(schema.GroupResource{Resource: "slices"}, ""),
			BloomPartitionTreeGVR: apierrors.NewNotFound(schema.GroupResource{Resource: "partitiontrees"}, ""),
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
	// Fractional CPU allocatable floors to whole schedulable vCPUs and takes min across nodes in pool.
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

func TestDiscoverInventory_NonReadyAndConditionReadySlices(t *testing.T) {
	t.Run("non-ready CREATING slice does not populate DefaultSliceName", func(t *testing.T) {
		resetAirGapState(t)
		creatingSlice := makeBloomSlice("slice-creating", "v6e", "2x4", "CREATING", nil)
		lister := &fakeDiscoveryLister{
			resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:       nil,
				BloomSliceGVR: {creatingSlice},
			},
		}
		checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: true}}
		inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inv.TPU == nil || inv.TPU.DefaultSliceName != "" || inv.TPU.TPUType != "v6e" || inv.TPU.Topology != "2x4" {
			t.Fatalf("expected non-ready slice to not populate DefaultSliceName: %+v", inv.TPU)
		}
	})

	t.Run("condition-ready slice becomes DefaultSliceName over CREATING slice", func(t *testing.T) {
		resetAirGapState(t)
		creatingSlice := makeBloomSlice("slice-creating", "tpu7x", "2x2x2", "CREATING", nil)
		condReady := makeBloomSlice("slice-cond", "tpu7x", "2x2x2", "", nil)
		condReady.Object["status"] = map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
			"nodes": []interface{}{"n0", "n1"},
		}
		lister := &fakeDiscoveryLister{
			resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:       nil,
				BloomSliceGVR: {creatingSlice, condReady},
			},
		}
		checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: true}}
		inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inv.TPU == nil || inv.TPU.DefaultSliceName != "slice-cond" || inv.TPU.NodesPerSlice != 2 {
			t.Fatalf("expected condition-ready slice as default: %+v", inv.TPU)
		}
	})
}

func TestDiscoverInventory_NodeDerivedSlice(t *testing.T) {
	resetAirGapState(t)
	nodeOnly := makeNode("tpu-node", map[string]string{
		BloomSliceNameLabel: "node-slice-0",
		BloomTPUTypeLabel:   "v5p",
		BloomTopologyLabel:  "2x2x1",
	}, map[string]interface{}{"google.com/tpu": "4", "cpu": "100", "memory": "200Gi"}, true)
	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: {nodeOnly}},
		errs: map[schema.GroupVersionResource]error{
			BloomSliceGVR:         errors.New(`the server doesn't have a resource type "slices"`),
			BloomPartitionTreeGVR: errors.New(`the server doesn't have a resource type "partitiontrees"`),
		},
	}
	inv, err := DiscoverInventory(lister, nil, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Slices) != 1 || inv.Slices[0].FromCRD || inv.Slices[0].Name != "node-slice-0" || !inv.Slices[0].Ready {
		t.Fatalf("expected node-derived slice, got %+v", inv.Slices)
	}
}

func TestDiscoverInventory_ErrorsAndNilSafety(t *testing.T) {
	resetAirGapState(t)

	if _, err := DiscoverInventory(nil, nil, DiscoverOptions{}); err == nil {
		t.Errorf("expected error for nil lister")
	}
	if _, err := DiscoverInventory(&fakeDiscoveryLister{}, nil, DiscoverOptions{LabelSchemeOverride: "bad"}); err == nil {
		t.Errorf("expected error for invalid LabelSchemeOverride")
	}

	// Non-403 error listing nodes must propagate (not enter explicit mode).
	lister := &fakeDiscoveryLister{
		errs: map[schema.GroupVersionResource]error{
			NodeGVR: errors.New("dial tcp: connection refused"),
		},
	}
	if _, err := DiscoverInventory(lister, nil, DiscoverOptions{}); err == nil {
		t.Errorf("expected non-403 list nodes error to propagate")
	}

	// API group probe error degrades cleanly with warning.
	listerOK := &fakeDiscoveryLister{resources: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: nil}}
	errChecker := &fakeAPIGroupChecker{err: errors.New("timeout")}
	inv, err := DiscoverInventory(listerOK, errChecker, DiscoverOptions{})
	if err != nil || inv.LabelScheme != SchemeGeneric {
		t.Fatalf("expected graceful fallback on apiChecker error, got (%+v, %v)", inv, err)
	}

	// Nil receiver helpers must not panic.
	var nilInv *Inventory
	if nilInv.DefaultPoolCap(true) != nil || nilInv.SummaryLines() != nil {
		t.Errorf("expected nil helpers for nil *Inventory")
	}
}

func TestDiscoverInventory_SliceNodeEnrichmentRecomputesNodesPerSlice(t *testing.T) {
	resetAirGapState(t)

	// Slice CRD omits status.nodes and names generic family "v6e" with 4x4 (16 chips).
	// Real nodes are 8-chip ct6e-standard-8t hosts, so NodesPerSlice must be recomputed to 2 (not stale 4).
	sliceObj := makeBloomSlice("slice-v6e-8t", "v6e", "4x4", "READY", nil)
	n0 := makeNode("v6e-0", map[string]string{
		InstanceTypeLabel:   "ct6e-standard-8t",
		BloomSliceNameLabel: "slice-v6e-8t",
		BloomTPUTypeLabel:   "v6e",
		BloomTopologyLabel:  "4x4",
	}, map[string]interface{}{"cpu": "180", "memory": "720Gi", "google.com/tpu": "8"}, true)
	n1 := makeNode("v6e-1", map[string]string{
		InstanceTypeLabel:   "ct6e-standard-8t",
		BloomSliceNameLabel: "slice-v6e-8t",
		BloomTPUTypeLabel:   "v6e",
		BloomTopologyLabel:  "4x4",
	}, map[string]interface{}{"cpu": "180", "memory": "720Gi", "google.com/tpu": "8"}, true)

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:       {n0, n1},
			BloomSliceGVR: {sliceObj},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: true}}
	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.TPU == nil || inv.TPU.MachineType != "ct6e-standard-8t" || inv.TPU.ChipsPerNode != 8 || inv.TPU.NodesPerSlice != 2 {
		t.Fatalf("expected recomputed NodesPerSlice=2 for 8-chip v6e 4x4 slice, got %+v", inv.TPU)
	}
}

func TestDiscoverInventory_HeterogeneousTPUFamilyDoesNotGraftSlice(t *testing.T) {
	resetAirGapState(t)

	// Primary TPU pool is v5p (2x2x2) with no v5p Slice CRD; an unrelated v6e Slice CRD exists.
	v5pNode := makeNode("v5p-0", map[string]string{
		InstanceTypeLabel:  "ct5p-hightpu-4t",
		BloomTPUTypeLabel:  "v5p",
		BloomTopologyLabel: "2x2x2",
	}, map[string]interface{}{"cpu": "208", "memory": "440Gi", "google.com/tpu": "4"}, true)
	v6eSlice := makeBloomSlice("slice-v6e-only", "v6e", "2x2", "READY", nil)

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:       {v5pNode},
			BloomSliceGVR: {v6eSlice},
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: true}}
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

func TestPoolSelectionTieBreakersAndCRDResilience(t *testing.T) {
	// Direct tie-breaker assertions resist condition inversions in isBetterCPUPool / isBetterTPUPool.
	if !isBetterCPUPool(PoolCapacity{ReadyNodes: 1, CPUsPerNode: 8}, false, &PoolCapacity{ReadyNodes: 0, CPUsPerNode: 32}, false) {
		t.Errorf("ready CPU pool should beat unready CPU pool")
	}
	if !isBetterCPUPool(PoolCapacity{ReadyNodes: 1, CPUsPerNode: 32}, false, &PoolCapacity{ReadyNodes: 1, CPUsPerNode: 8}, false) {
		t.Errorf("larger CPU pool should beat smaller CPU pool")
	}
	if !isBetterCPUPool(PoolCapacity{MachineType: "a-pool", ReadyNodes: 1, CPUsPerNode: 8}, false, &PoolCapacity{MachineType: "z-pool", ReadyNodes: 1, CPUsPerNode: 8}, false) {
		t.Errorf("lexical tie-breaker should pick a-pool over z-pool")
	}
	if !isBetterTPUPool(PoolCapacity{ReadyNodes: 1, TotalNodes: 1}, &PoolCapacity{ReadyNodes: 0, TotalNodes: 4}) {
		t.Errorf("ready TPU pool should beat unready TPU pool")
	}
	if !isBetterTPUPool(PoolCapacity{ReadyNodes: 2, TotalNodes: 4}, &PoolCapacity{ReadyNodes: 2, TotalNodes: 2}) {
		t.Errorf("larger TPU pool should beat smaller TPU pool")
	}
	if !isBetterTPUPool(PoolCapacity{MachineType: "ct5p-hightpu-4t", ReadyNodes: 2, TotalNodes: 2}, &PoolCapacity{MachineType: "tpu7x-standard-4t", ReadyNodes: 2, TotalNodes: 2}) {
		t.Errorf("lexical tie-breaker should pick ct5p-hightpu-4t over tpu7x-standard-4t")
	}
}

func TestDiscoverInventory_NodeOrderIndependenceAndCRDNumNodes(t *testing.T) {
	resetAirGapState(t)

	// Two nodes sharing machine type n2-standard-8 with different GKE nodepool names: must pick min lexicographically regardless of order.
	nA := makeNode("n-a", map[string]string{InstanceTypeLabel: "n2-standard-8", GKENodePoolLabel: "z-pool"}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, true)
	nB := makeNode("n-b", map[string]string{InstanceTypeLabel: "n2-standard-8", GKENodePoolLabel: "a-pool"}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, true)
	sliceWithNum := makeBloomSlice("s-num", "v5p", "2x2x2", "READY", nil)
	sliceWithNum.Object["status"].(map[string]interface{})["numNodes"] = "2"

	lister := &fakeDiscoveryLister{
		resources: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:       {nA, nB},
			BloomSliceGVR: {sliceWithNum},
		},
		errs: map[schema.GroupVersionResource]error{
			BloomPartitionTreeGVR: errors.New("transient apiserver 503"),
		},
	}
	checker := &fakeAPIGroupChecker{groups: map[string]bool{BloomSliceAPIGroupVersion: true}}
	inv, err := DiscoverInventory(lister, checker, DiscoverOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.Pools["n2-standard-8"].NodePoolName != "a-pool" {
		t.Errorf("NodePoolName = %q, want deterministic min a-pool", inv.Pools["n2-standard-8"].NodePoolName)
	}
	if len(inv.Slices) != 1 || inv.Slices[0].NodesPerSlice != 2 {
		t.Errorf("expected spec/status numNodes=2 parsed, got %+v", inv.Slices)
	}
}
