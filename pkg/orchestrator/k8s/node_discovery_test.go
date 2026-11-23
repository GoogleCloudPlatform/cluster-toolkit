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
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeDiscoveryLister struct {
	byGVR  map[schema.GroupVersionResource][]unstructured.Unstructured
	errGVR map[schema.GroupVersionResource]error
	calls  []schema.GroupVersionResource
}

func (f *fakeDiscoveryLister) ListResources(gvr schema.GroupVersionResource, _, _, _ string) ([]unstructured.Unstructured, error) {
	f.calls = append(f.calls, gvr)
	if err, ok := f.errGVR[gvr]; ok && err != nil {
		return nil, err
	}
	return f.byGVR[gvr], nil
}

type fakeAPIGroupChecker struct {
	groups map[string]bool
	err    error
	calls  []string
}

func (f *fakeAPIGroupChecker) HasAPIGroupVersion(groupVersion string) (bool, error) {
	f.calls = append(f.calls, groupVersion)
	if f.err != nil {
		return false, f.err
	}
	return f.groups[groupVersion], nil
}

// testHardwareResolver simulates a provider-specific GCE TPUHardwareResolver for k8s unit tests.
type testHardwareResolver struct{}

func (testHardwareResolver) IsTPUMachine(m string) bool {
	s := strings.ToLower(strings.TrimSpace(m))
	return strings.HasPrefix(s, "ct") || strings.HasPrefix(s, "tpu7x")
}

var testTPUNormTable = []struct {
	canonical string
	tokens    []string
}{
	{"tpu7x", []string{"tpu7x", "ironwood"}},
	{"v6e", []string{"ct6e", "v6e"}},
	{"v5p", []string{"ct5p", "v5p"}},
	{"v5e", []string{"ct5lp", "ct5l", "v5e", "v5lite"}},
	{"v4", []string{"ct4p", "tpu-v4"}},
}

func (testHardwareResolver) NormalizeTPUType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "tpu7" || strings.HasPrefix(s, "tpu7-") {
		return "tpu7x"
	}
	for _, row := range testTPUNormTable {
		for _, tok := range row.tokens {
			if strings.Contains(s, tok) {
				return row.canonical
			}
		}
	}
	return s
}

func (r testHardwareResolver) InferTPUMachine(tpuType, rawMachine string, chips int) string {
	trimmed := strings.TrimSpace(rawMachine)
	if trimmed != "" && !isTestTPUShorthand(trimmed, tpuType) {
		return trimmed
	}
	return testDefaultTPUMachine(tpuType, chips)
}

func isTestTPUShorthand(raw, _ string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "tpu7", "tpu7x", "ironwood", "v6e", "v5p", "v5e", "v4", "tpu-v5p-slice":
		return true
	default:
		return false
	}
}

func testDefaultTPUMachine(tpuType string, chips int) string {
	switch tpuType {
	case "tpu7x":
		if chips == 1 {
			return "tpu7x-standard-1t"
		}
		return "tpu7x-standard-4t"
	case "v6e":
		return fmt.Sprintf("ct6e-standard-%dt", testNormChips(chips))
	case "v5p":
		return "ct5p-hightpu-4t"
	case "v5e":
		return fmt.Sprintf("ct5lp-hightpu-%dt", testNormChips(chips))
	default:
		return ""
	}
}

func testNormChips(chips int) int {
	if chips == 1 || chips == 8 {
		return chips
	}
	return 4
}

func (testHardwareResolver) ChipsPerNode(machineType string) int {
	switch strings.ToLower(strings.TrimSpace(machineType)) {
	case "tpu7x-standard-1t", "ct6e-standard-1t", "ct5lp-hightpu-1t":
		return 1
	case "tpu7x-standard-4t", "ct6e-standard-4t", "ct5p-hightpu-4t", "ct5lp-hightpu-4t", "ct4p-hightpu-4t":
		return 4
	case "ct6e-standard-8t", "ct5lp-hightpu-8t":
		return 8
	default:
		return 0
	}
}

func makeNode(name string, labels map[string]string, alloc map[string]interface{}, ready bool) unstructured.Unstructured {
	return makeNodeWithSpec(name, labels, alloc, ready, false, nil)
}

func makeNodeWithSpec(name string, labels map[string]string, alloc map[string]interface{}, ready, unschedulable bool, taints []interface{}) unstructured.Unstructured {
	condStatus := "False"
	if ready {
		condStatus = "True"
	}
	spec := map[string]interface{}{}
	if unschedulable {
		spec["unschedulable"] = true
	}
	if len(taints) > 0 {
		spec["taints"] = taints
	}
	return unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name":   name,
				"labels": toAnyMap(labels),
			},
			"spec": spec,
			"status": map[string]interface{}{
				"allocatable": alloc,
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": condStatus},
				},
			},
		},
	}
}

func makeSliceCRD(name, ns, tpuType, topology, readyStatus, reason string, partIDs []interface{}) unstructured.Unstructured {
	spec := map[string]interface{}{
		"type":     tpuType,
		"topology": topology,
	}
	if len(partIDs) > 0 {
		spec["partitionIds"] = partIDs
	}
	metaObj := map[string]interface{}{"name": name}
	if ns != "" {
		metaObj["namespace"] = ns
	}
	return unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "slice.tpu.google.com/v1alpha1",
			"kind":       "Slice",
			"metadata":   metaObj,
			"spec":       spec,
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": readyStatus,
						"reason": reason,
					},
				},
			},
		},
	}
}

func toAnyMap(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func TestParseAndResolveLabelScheme(t *testing.T) {
	t.Parallel()
	cases := []struct {
		env           string
		want          LabelScheme
		wantErrSubstr string
	}{
		{"", SchemeAuto, ""},
		{"tpu-slice", SchemeTPUSlice, ""},
		{"  GKE ", SchemeGKE, ""},
		{"generic", SchemeGeneric, ""},
		{"bogus", "", LabelSchemeEnvVar},
	}
	for _, tc := range cases {
		got, err := ResolveLabelSchemeOverride(func(k string) string {
			if k == LabelSchemeEnvVar {
				return tc.env
			}
			return ""
		})
		if tc.wantErrSubstr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr) || strings.Contains(err.Error(), "--") {
				t.Errorf("ResolveLabelSchemeOverride(%q) err=%v want %q without CLI flag hints", tc.env, err, tc.wantErrSubstr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ResolveLabelSchemeOverride(%q) = (%q, %v), want (%q, nil)", tc.env, got, err, tc.want)
		}
	}
	if _, err := ParseLabelScheme("invalid"); err == nil || !strings.Contains(err.Error(), "invalid label scheme") || strings.Contains(err.Error(), LabelSchemeEnvVar) {
		t.Errorf("ParseLabelScheme(invalid) = %v, want source-neutral error", err)
	}
}

func TestDiscoverInventory_RealCRDSchemasAndV1Beta1Fallback(t *testing.T) {
	t.Parallel()
	noMatchSlice := &meta.NoResourceMatchError{PartialResource: TPUSliceGVR}
	noMatchTree := &meta.NoResourceMatchError{PartialResource: TPUPartitionTreeGVR}

	v1beta1Slice := makeSliceCRD("slice-ready", "", "tpu7x", "2x2x2", "True", "READY", []interface{}{"pod2-r2-2x2x2-5"})
	v1beta1Tree := unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "tree-v1beta1"},
			"status": map[string]interface{}{
				"acceleratorType": "tpu7x",
				"root": map[string]interface{}{
					"topology":    "4x4x4",
					"partitionId": "pod-4x4x4",
					"childPartitions": []interface{}{
						map[string]interface{}{
							"topology":    "2x2x2",
							"partitionId": "pod-2x2x2",
							"childPartitions": []interface{}{
								map[string]interface{}{"partitionId": "pod-2x2x1-0"},
							},
						},
					},
				},
			},
		},
	}
	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:                    {makeNode("n1", map[string]string{TPUSliceTypeLabel: "tpu7x"}, map[string]interface{}{"cpu": "240", "memory": "1000Gi", "google.com/tpu": "4"}, true)},
			TPUSliceV1Beta1GVR:         {v1beta1Slice},
			TPUPartitionTreeV1Beta1GVR: {v1beta1Tree},
		},
		errGVR: map[schema.GroupVersionResource]error{
			TPUSliceGVR:         noMatchSlice,
			TPUPartitionTreeGVR: noMatchTree,
		},
	}
	api := &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}
	inv, err := DiscoverInventory(lister, api, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if inv.LabelScheme != SchemeTPUSlice || !inv.TPUSliceAPIPresent {
		t.Fatalf("got scheme=%q api=%v, want tpu-slice/true", inv.LabelScheme, inv.TPUSliceAPIPresent)
	}
	if len(inv.Slices) != 1 || !inv.Slices[0].Ready || inv.Slices[0].State != "READY" || len(inv.Slices[0].PartitionIDs) != 1 {
		t.Fatalf("unexpected v1beta1 slice: %+v", inv.Slices)
	}
	if len(inv.PartitionTrees) != 1 || inv.PartitionTrees[0].RootTopology != "4x4x4" {
		t.Fatalf("unexpected v1beta1 partition tree: %+v", inv.PartitionTrees)
	}
	wantTopos := []string{"2x2x1", "2x2x2", "4x4x4"}
	if !slices.Equal(inv.PartitionTrees[0].SupportedTopologies, wantTopos) {
		t.Errorf("SupportedTopologies = %v, want %v", inv.PartitionTrees[0].SupportedTopologies, wantTopos)
	}
	if !slices.Equal(api.calls, []string{"slice.tpu.google.com"}) {
		t.Errorf("API probe calls = %v, want group-level [slice.tpu.google.com]", api.calls)
	}
}

func TestDiscoverInventory_V1Alpha1PartitionTreeLiveSchema(t *testing.T) {
	t.Parallel()
	// Matches live partition.tpu.google.com/v1alpha1 schema on TPU slice clusters.
	liveTree := unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":   "pod1-r1-4x4x4-1",
				"labels": map[string]interface{}{TPUSliceTypeLabel: "tpu7x"},
			},
			"status": map[string]interface{}{
				"partitionId":       "pod1-r1-4x4x4-1",
				"partitionTypeInfo": "IRONWOOD_4_4_4",
				"childPartitions": []interface{}{
					map[string]interface{}{
						"partitionId":       "pod1-r1-2x4x4-1",
						"partitionTypeInfo": "IRONWOOD_2_4_4",
						"childPartitions": []interface{}{
							map[string]interface{}{
								"partitionId":       "pod1-r1-2x2x1-1",
								"partitionTypeInfo": "IRONWOOD_2_2_1",
							},
						},
					},
				},
			},
		},
	}
	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:             {makeNode("n1", map[string]string{TPUSliceTypeLabel: "tpu7x"}, map[string]interface{}{"google.com/tpu": "4"}, true)},
			TPUPartitionTreeGVR: {liveTree},
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if len(inv.PartitionTrees) != 1 || inv.PartitionTrees[0].RootTopology != "4x4x4" || inv.PartitionTrees[0].TPUType != "tpu7x" {
		t.Fatalf("unexpected v1alpha1 partition tree: %+v", inv.PartitionTrees)
	}
	want := []string{"2x2x1", "2x4x4", "4x4x4"}
	if !slices.Equal(inv.PartitionTrees[0].SupportedTopologies, want) {
		t.Errorf("SupportedTopologies = %v, want %v", inv.PartitionTrees[0].SupportedTopologies, want)
	}
	if inv.TPU == nil || inv.TPU.Topology != "4x4x4" {
		t.Errorf("expected fallback topology from PartitionTree.RootTopology=4x4x4, got %+v", inv.TPU)
	}
}

func TestDiscoverInventory_PartitionTreeDoesNotSetTPUSliceAPIPresent(t *testing.T) {
	t.Parallel()
	// GKE dynamic slicing clusters may serve partition.tpu.google.com without slice.tpu.google.com.
	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {makeNode("n1", map[string]string{InstanceTypeLabel: "n2-standard-8"}, map[string]interface{}{"cpu": "8"}, true)},
			TPUPartitionTreeGVR: {
				{Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "gke-part-tree"},
					"status":   map[string]interface{}{"acceleratorType": "v6e", "root": map[string]interface{}{"topology": "2x2"}},
				}},
			},
		},
		errGVR: map[schema.GroupVersionResource]error{
			TPUSliceGVR:        apierrors.NewNotFound(TPUSliceGVR.GroupResource(), ""),
			TPUSliceV1Beta1GVR: apierrors.NewNotFound(TPUSliceV1Beta1GVR.GroupResource(), ""),
		},
	}
	inv, err := DiscoverInventory(lister, nil, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if inv.TPUSliceAPIPresent || inv.LabelScheme != SchemeGeneric || inv.TPU != nil || len(inv.PartitionTrees) != 0 {
		t.Fatalf("generic cluster without Slice API must not fabricate TPU/PartitionTree inventory: api=%v scheme=%q TPU=%+v trees=%+v", inv.TPUSliceAPIPresent, inv.LabelScheme, inv.TPU, inv.PartitionTrees)
	}
	if slices.Contains(lister.calls, TPUPartitionTreeGVR) || slices.Contains(lister.calls, TPUPartitionTreeV1Beta1GVR) {
		t.Errorf("must skip PartitionTree listing after Slice NotFound on generic cluster; calls=%v", lister.calls)
	}
}

func TestDiscoverInventory_FailClosedWhenAutoSchemeUnresolvable(t *testing.T) {
	t.Parallel()
	forbiddenNodes := apierrors.NewForbidden(NodeGVR.GroupResource(), "", errors.New("forbidden"))
	forbiddenSlices := apierrors.NewForbidden(TPUSliceGVR.GroupResource(), "", errors.New("forbidden"))
	probeErr := errors.New("apiserver 503 unavailable")

	// Probe failed + Slice 403 -> must fail closed and suggest GCLUSTER_LABEL_SCHEME.
	lister := &fakeDiscoveryLister{
		errGVR: map[schema.GroupVersionResource]error{
			NodeGVR:     forbiddenNodes,
			TPUSliceGVR: forbiddenSlices,
		},
	}
	_, err := DiscoverInventory(lister, &fakeAPIGroupChecker{err: probeErr}, DiscoverOptions{})
	if err == nil || !errors.Is(err, probeErr) || !errors.Is(err, forbiddenSlices) || !strings.Contains(err.Error(), LabelSchemeEnvVar) {
		t.Fatalf("expected fail-closed error wrapping both probe and slice errors, got: %v", err)
	}
	// v1beta1 must NOT be queried when v1alpha1 returned 403 (fallback is NotFound/NoMatch only).
	if slices.Contains(lister.calls, TPUSliceV1Beta1GVR) {
		t.Errorf("unexpected v1beta1 fallback after 403 on v1alpha1: %v", lister.calls)
	}

	// apiChecker == nil + Slice 403 -> also fails closed.
	_, err = DiscoverInventory(lister, nil, DiscoverOptions{})
	if err == nil || !errors.Is(err, forbiddenSlices) {
		t.Fatalf("expected fail-closed error when apiChecker is nil and slices are 403, got: %v", err)
	}

	// Explicit override bypasses the probe and succeeds cleanly even when nodes/slices are 403.
	api := &fakeAPIGroupChecker{err: probeErr}
	inv, err := DiscoverInventory(lister, api, DiscoverOptions{LabelSchemeOverride: SchemeTPUSlice})
	if err != nil {
		t.Fatalf("explicit override should succeed: %v", err)
	}
	if len(api.calls) != 0 {
		t.Errorf("expected probe to be skipped on explicit override, got calls=%v", api.calls)
	}
	if !inv.ExplicitMode || inv.LabelScheme != SchemeTPUSlice || !inv.SliceInventoryUnavailable {
		t.Errorf("unexpected explicit inventory: %+v", inv)
	}
}

func TestDiscoverInventory_SkipsProbeOnPureGKEAndIsolatesHybridOverride(t *testing.T) {
	t.Parallel()
	pureGKENode := makeNode("gke-1", map[string]string{
		InstanceTypeLabel:      "ct5p-hightpu-4t",
		GKENodePoolLabel:       "tpu-pool",
		GKETPUAcceleratorLabel: "tpu-v5p-slice",
		GKETPUTopologyLabel:    "2x2x2",
	}, map[string]interface{}{"cpu": "200", "memory": "400Gi", "google.com/tpu": "4"}, true)

	api := &fakeAPIGroupChecker{err: errors.New("should not be probed on pure GKE")}
	lister := &fakeDiscoveryLister{byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: {pureGKENode}}}
	inv, err := DiscoverInventory(lister, api, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("pure GKE discovery failed: %v", err)
	}
	if inv.LabelScheme != SchemeGKE || len(api.calls) != 0 {
		t.Fatalf("got scheme=%q apiCalls=%v, want gke and 0 probe calls", inv.LabelScheme, api.calls)
	}

	// Hybrid node carrying both GKE and tpu.google.com labels under forced gke override.
	hybridNode := makeNode("hybrid-1", map[string]string{
		InstanceTypeLabel:                  "ct5p-hightpu-4t",
		GKENodePoolLabel:                   "tpu-pool",
		GKETPUAcceleratorLabel:             "tpu-v5p-slice",
		GKETPUTopologyLabel:                "2x2x2",
		TPUSliceTypeLabel:                  "tpu7x",
		TPUSliceTopologyLabel:              "4x4x4",
		TPUSliceNameLabel:                  "legacy-slice",
		TPUSlicePartitionLabelPrefix + "x": "p1",
	}, map[string]interface{}{"google.com/tpu": "4"}, true)
	listerHybrid := &fakeDiscoveryLister{byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: {hybridNode}}}
	invForcedGKE, err := DiscoverInventory(listerHybrid, api, DiscoverOptions{
		LabelSchemeOverride: SchemeGKE,
		HardwareResolver:    testHardwareResolver{},
	})
	if err != nil {
		t.Fatalf("forced GKE hybrid discovery failed: %v", err)
	}
	if invForcedGKE.TPU == nil || invForcedGKE.TPU.TPUType != "v5p" || invForcedGKE.TPU.Topology != "2x2x2" {
		t.Errorf("forced GKE should prefer GKE TPU labels, got %+v", invForcedGKE.TPU)
	}
	if invForcedGKE.TPU.HasPartitionLabels || len(invForcedGKE.Slices) != 0 || len(invForcedGKE.TPUPoolCap.SliceNames) != 0 {
		t.Errorf("forced GKE must omit slice/partition metadata: tpu=%+v slices=%+v pool=%+v", invForcedGKE.TPU, invForcedGKE.Slices, invForcedGKE.TPUPoolCap)
	}
}

func TestDiscoverInventory_PoolAggregationGPUsAndMilliCPU(t *testing.T) {
	t.Parallel()
	ctrlNode := makeNode("ctrl", map[string]string{ControlPlaneRoleLabel: ""}, map[string]interface{}{"cpu": "500m", "memory": "2Gi"}, true)
	defaultPoolNode := makeNode("sys-cpu", map[string]string{
		InstanceTypeLabel: "e2-standard-8",
		GKENodePoolLabel:  "default-pool",
	}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, true)
	workerCPU := makeNode("worker-cpu", map[string]string{
		InstanceTypeLabel: "e2-standard-4",
		GKENodePoolLabel:  "cpu-workers",
	}, map[string]interface{}{"cpu": "500m", "memory": "4Gi"}, true)
	unreadyWorkerCPU := makeNode("worker-cpu-bad", map[string]string{
		InstanceTypeLabel: "e2-standard-4",
		GKENodePoolLabel:  "cpu-workers",
	}, map[string]interface{}{"cpu": "100m", "memory": "1Gi"}, false)
	migNode := makeNode("gpu-mig", map[string]string{
		InstanceTypeLabel:     "a3-highgpu-8g",
		GKENodePoolLabel:      "mig-pool",
		NvidiaGPUProductLabel: "NVIDIA-H100-80GB",
	}, map[string]interface{}{"cpu": "100", "memory": "500Gi", "nvidia.com/gpu": "0", "nvidia.com/mig-1g.10gb": "2"}, true)
	amdNode := makeNode("gpu-amd", map[string]string{
		InstanceTypeLabel: "custom-amd",
		GKENodePoolLabel:  "amd-pool",
	}, map[string]interface{}{"cpu": "64", "amd.com/gpu": "4"}, true)
	intelNode := makeNode("gpu-intel", map[string]string{
		InstanceTypeLabel: "custom-intel",
		GKENodePoolLabel:  "intel-pool",
	}, map[string]interface{}{"cpu": "32", "gpu.intel.com/i915": "2"}, true)

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {ctrlNode, defaultPoolNode, workerCPU, unreadyWorkerCPU, migNode, amdNode, intelNode},
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{}, DiscoverOptions{LabelSchemeOverride: SchemeGeneric})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	// Worker CPU pool beats default-pool and control-plane, and prefers ready-node 500m over unready 100m.
	if inv.CPUPoolCap == nil || inv.CPUPoolCap.NodePoolName != "cpu-workers" || inv.CPUPoolCap.AllocatableCPUMilli != 500 {
		t.Fatalf("unexpected CPUPoolCap: %+v", inv.CPUPoolCap)
	}
	// Unknown machineType on ctrlNode must stay empty (not synthetic "k8s-node").
	if pool := inv.Pools["k8s-node"]; pool.MachineType != "" {
		t.Errorf("synthetic MachineType leaked: %q", pool.MachineType)
	}
	summary := strings.Join(inv.SummaryLines(), "\n")
	for _, want := range []string{"500m vCPU", "GPU pool: a3-highgpu-8g", "pool=mig-pool", "GPU pool: custom-amd", "GPU pool: custom-intel"} {
		if !strings.Contains(summary, want) {
			t.Errorf("SummaryLines missing %q:\n%s", want, summary)
		}
	}
	for _, bad := range []string{"<none>", "<unknown>", "(s)"} {
		if strings.Contains(summary, bad) {
			t.Errorf("SummaryLines must not contain %q:\n%s", bad, summary)
		}
	}
}

func TestDiscoverInventory_SliceEnrichmentReadinessAndPrimaryIsolation(t *testing.T) {
	t.Parallel()
	// Primary TPU pool is worker tpu7x (4 chips); also have control-plane TPU node and a v5p slice.
	ctrlTPU := makeNode("ctrl-tpu", map[string]string{
		ControlPlaneRoleLabel: "",
		TPUSliceTypeLabel:     "v5p",
	}, map[string]interface{}{"google.com/tpu": "4"}, true)
	tpu1 := makeNode("tpu-1", map[string]string{
		TPUSliceTypeLabel: "tpu7x",
		TPUSliceNameLabel: "partial-slice",
	}, map[string]interface{}{"google.com/tpu": "4"}, true)
	tpu2Unready := makeNode("tpu-2", map[string]string{
		TPUSliceTypeLabel: "tpu7x",
		TPUSliceNameLabel: "partial-slice",
	}, map[string]interface{}{"google.com/tpu": "4"}, false)
	tpu3 := makeNode("tpu-3", map[string]string{
		TPUSliceTypeLabel: "tpu7x",
		TPUSliceNameLabel: "healthy-slice",
	}, map[string]interface{}{"google.com/tpu": "4"}, true)
	tpu4 := makeNode("tpu-4", map[string]string{
		TPUSliceTypeLabel: "tpu7x",
		TPUSliceNameLabel: "healthy-slice",
	}, map[string]interface{}{"google.com/tpu": "4"}, true)

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {ctrlTPU, tpu1, tpu2Unready, tpu3, tpu4},
			TPUSliceGVR: {
				makeSliceCRD("a-other-family", "", "v5p", "2x2x1", "True", "READY", nil),
				makeSliceCRD("partial-slice", "", "tpu7x", "2x2x2", "True", "READY", nil),
				makeSliceCRD("healthy-slice", "", "tpu7x", "2x2x2", "True", "READY", nil),
			},
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	// Worker tpu7x pool beats control-plane v5p pool, and TotalTPUNodes isolates to tpu7x (4 nodes, not 5).
	if inv.TPU == nil || inv.TPU.TPUType != "tpu7x" || inv.TPU.TotalTPUNodes != 4 || inv.TPU.ReadyTPUNodes != 3 {
		t.Fatalf("unexpected primary TPU isolation: %+v", inv.TPU)
	}
	// partial-slice has 1/2 ready nodes so it must be marked not ready; healthy-slice is picked instead.
	if inv.TPU.DefaultSliceName != "healthy-slice" {
		t.Errorf("DefaultSliceName = %q, want healthy-slice", inv.TPU.DefaultSliceName)
	}
}

func TestDiscoverInventory_SingleChipBackfillAndBareMetalChips(t *testing.T) {
	t.Parallel()
	// CRD without topology guesses 4 chips (tpu7x-standard-4t); node omits instance-type and backfills 1x1x1.
	nodeBackfill := makeNode("bm-1", map[string]string{
		TPUSliceTypeLabel:     "tpu7x",
		TPUSliceTopologyLabel: "1x1x1",
		TPUSliceNameLabel:     "single-chip-slice",
	}, map[string]interface{}{"cpu": "64"}, true)
	// Custom bare-metal host with no allocatable TPU must not default ChipsPerNode to 4.
	bmUnknown := makeNode("bm-2", map[string]string{
		InstanceTypeLabel: "my-baremetal-host",
		TPUSliceNameLabel: "bm-slice",
	}, map[string]interface{}{"cpu": "64"}, true)

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     {nodeBackfill, bmUnknown},
			TPUSliceGVR: {makeSliceCRD("single-chip-slice", "", "tpu7x", "", "True", "READY", nil)},
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	byName := map[string]SliceInfo{}
	for _, s := range inv.Slices {
		byName[s.Name] = s
	}
	if got := byName["single-chip-slice"]; got.ChipsPerNode != 1 || got.MachineType != "tpu7x-standard-1t" || got.NodesPerSlice != 1 {
		t.Errorf("single-chip backfill failed: %+v", got)
	}
	if got := byName["bm-slice"]; got.ChipsPerNode != 0 || got.MachineType != "my-baremetal-host" || got.TPUType != "" {
		t.Errorf("bare-metal host must not default ChipsPerNode to 4 or pollute TPUType: %+v", got)
	}
}

func TestDiscoverInventory_RestartingPoolChipsBeforeSliceMatch(t *testing.T) {
	t.Parallel()
	// Restarting ct6e-standard-8t pool reports 0 allocatable TPUs; must derive 8 chips from MachineType
	// before matching slices so a 4-chip ct6e-standard-4t slice is NOT selected.
	restartingNode := makeNode("v6e-8t-node", map[string]string{
		InstanceTypeLabel: "ct6e-standard-8t",
		TPUSliceTypeLabel: "v6e",
	}, map[string]interface{}{"cpu": "120"}, true)
	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {restartingNode},
			TPUSliceGVR: {
				makeSliceCRD("wrong-4t-slice", "", "v6e", "2x2", "True", "READY", nil),
			},
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if inv.TPU == nil || inv.TPU.ChipsPerNode != 8 || inv.TPU.DefaultSliceName != "" {
		t.Fatalf("expected 8-chip restarting pool to reject 4-chip slice, got %+v", inv.TPU)
	}
}

func TestDiscoverInventory_NilListerAndNilSummary(t *testing.T) {
	t.Parallel()
	if _, err := DiscoverInventory(nil, nil, DiscoverOptions{}); err == nil || !strings.Contains(err.Error(), "Kubernetes resource lister") {
		t.Fatalf("expected capitalized Kubernetes lister error, got %v", err)
	}
	var nilInv *Inventory
	if nilInv.SummaryLines() != nil {
		t.Errorf("nil SummaryLines should be nil")
	}
	nilInv.LogSummary()
}

func TestSummaryLines_TPUAndExplicitModes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		inv         *Inventory
		wantParts   []string
		forbidParts []string
	}{
		{
			name: "explicit with unavailable slices and no API probe",
			inv:  &Inventory{LabelScheme: SchemeTPUSlice, ExplicitMode: true, SliceInventoryUnavailable: true},
			wantParts: []string{
				"Kubernetes explicit mode active (label scheme: tpu-slice).",
				"slice inventory unavailable",
			},
			forbidParts: []string{"TPU slice API present", "TPU slice API detected"},
		},
		{
			name: "explicit with ready slice and detected TPU slice API",
			inv: &Inventory{
				LabelScheme:        SchemeTPUSlice,
				ExplicitMode:       true,
				TPUSliceAPIPresent: true,
				Slices:             []SliceInfo{{Name: "s1", Ready: true}},
				TPU:                &TPUInventory{DefaultSliceName: "s1"},
			},
			wantParts: []string{"Kubernetes explicit mode active (label scheme: tpu-slice, TPU slice API detected).", "Default slice: s1."},
		},
		{
			name: "healthy GKE TPU pool does not report missing slice",
			inv: &Inventory{
				LabelScheme: SchemeGKE,
				TotalNodes:  4,
				ReadyNodes:  4,
				TPUPoolCap:  &PoolCapacity{NodePoolName: "tpu-v6e-pool", MachineType: "ct6e-standard-4t"},
				TPU: &TPUInventory{
					TPUType:       "v6e",
					MachineType:   "ct6e-standard-4t",
					TotalTPUNodes: 4,
					ReadyTPUNodes: 4,
					ChipsPerNode:  4,
					Topology:      "2x2",
					NodesPerSlice: 1,
				},
			},
			wantParts:   []string{"TPU pool: ct6e-standard-4t", "4/4 ready nodes", "type=v6e", "pool=tpu-v6e-pool"},
			forbidParts: []string{"no active slice", "no ready slice", "machine=ct6e-standard-4t"},
		},
		{
			name: "tpu pool with ready slice and distinct node pool",
			inv: &Inventory{
				LabelScheme: SchemeTPUSlice,
				TotalNodes:  2,
				ReadyNodes:  2,
				TPUPoolCap:  &PoolCapacity{NodePoolName: "tpu-pool-a", MachineType: "tpu7x-standard-4t"},
				TPU: &TPUInventory{
					TPUType:          "tpu7x",
					MachineType:      "tpu7x-standard-4t",
					TotalTPUNodes:    2,
					ReadyTPUNodes:    2,
					ChipsPerNode:     4,
					Topology:         "2x2x2",
					DefaultSliceName: "s1",
				},
			},
			wantParts: []string{"tpu7x", "pool=tpu-pool-a", "2/2 ready Kubernetes nodes", "Default slice: s1", "2/2 ready nodes"},
		},
		{
			name: "tpu pool with unready slice",
			inv: &Inventory{
				LabelScheme: SchemeTPUSlice,
				TotalNodes:  1,
				ReadyNodes:  1,
				Slices:      []SliceInfo{{Name: "s-unready", Ready: false}},
				TPU:         &TPUInventory{TPUType: "tpu7x", TotalTPUNodes: 1, ReadyTPUNodes: 1},
			},
			wantParts: []string{"1/1 ready Kubernetes node", "no ready slice"},
		},
		{
			name: "tpu pool with unavailable slice inventory",
			inv: &Inventory{
				LabelScheme:               SchemeTPUSlice,
				SliceInventoryUnavailable: true,
				TotalNodes:                1,
				ReadyNodes:                1,
				TPU:                       &TPUInventory{TPUType: "tpu7x", TotalTPUNodes: 1, ReadyTPUNodes: 1},
			},
			wantParts: []string{"slice inventory unavailable"},
		},
		{
			name: "tpu pool with no active slice",
			inv: &Inventory{
				LabelScheme: SchemeTPUSlice,
				TotalNodes:  1,
				ReadyNodes:  1,
				TPU:         &TPUInventory{TPUType: "tpu7x", TotalTPUNodes: 1, ReadyTPUNodes: 1},
			},
			wantParts: []string{"no active slice"},
		},
	}
	for _, tc := range cases {
		joined := strings.Join(tc.inv.SummaryLines(), "\n")
		tc.inv.LogSummary()
		for _, bad := range append([]string{"<none>", "<unknown>", "(s)"}, tc.forbidParts...) {
			if strings.Contains(joined, bad) {
				t.Errorf("%s: unexpected %q in summary:\n%s", tc.name, bad, joined)
			}
		}
		for _, want := range tc.wantParts {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: missing %q in summary:\n%s", tc.name, want, joined)
			}
		}
	}
}

func TestDiscoverInventory_PoolKeyDisambiguationAndSystemTaints(t *testing.T) {
	t.Parallel()
	// Two pools with identical machineType but distinct GKE node pools and GPU models must not merge.
	gpuL4 := makeNode("gpu-l4", map[string]string{
		InstanceTypeLabel:   "g2-standard-8",
		GKENodePoolLabel:    "pool-l4",
		GKEAcceleratorLabel: "nvidia-l4",
	}, map[string]interface{}{"cpu": "8", "nvidia.com/gpu": "1"}, true)
	gpuH100 := makeNode("gpu-h100", map[string]string{
		InstanceTypeLabel:   "g2-standard-8",
		GKENodePoolLabel:    "pool-h100",
		GKEAcceleratorLabel: "nvidia-h100",
	}, map[string]interface{}{"cpu": "8", "nvidia.com/gpu": "1"}, true)

	// System-tainted CPU node (CriticalAddonsOnly) with more CPUs must lose to worker CPU node.
	sysTainted := makeNodeWithSpec("sys-tainted", map[string]string{
		InstanceTypeLabel: "n2-standard-32",
		GKENodePoolLabel:  "addons-pool",
	}, map[string]interface{}{"cpu": "32"}, true, false, []interface{}{
		map[string]interface{}{"key": "CriticalAddonsOnly", "value": "true", "effect": "NoSchedule"},
	})
	worker := makeNode("worker-1", map[string]string{
		InstanceTypeLabel: "n2-standard-4",
		GKENodePoolLabel:  "worker-pool",
	}, map[string]interface{}{"cpu": "4"}, true)

	// Two PartitionTrees out of order verify deterministic sorting.
	treeB := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "partition.tpu.google.com/v1beta1",
		"kind":       "PartitionTree",
		"metadata":   map[string]interface{}{"name": "tree-b"},
		"status": map[string]interface{}{
			"acceleratorType": "v6e",
			"root":            map[string]interface{}{"topology": "4x4", "partitionId": "p-b"},
		},
	}}
	treeA := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "partition.tpu.google.com/v1beta1",
		"kind":       "PartitionTree",
		"metadata":   map[string]interface{}{"name": "tree-a"},
		"status": map[string]interface{}{
			"acceleratorType": "tpu7x",
			"root":            map[string]interface{}{"topology": "2x2x2", "partitionId": "p-a"},
		},
	}}

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:                    {gpuL4, gpuH100, sysTainted, worker},
			TPUPartitionTreeV1Beta1GVR: {treeB, treeA},
		},
		errGVR: map[schema.GroupVersionResource]error{
			TPUPartitionTreeGVR: apierrors.NewNotFound(schema.GroupResource{Group: "partition.tpu.google.com", Resource: "partitiontrees"}, ""),
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{
		LabelSchemeOverride: SchemeTPUSlice,
		HardwareResolver:    testHardwareResolver{},
	})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if len(inv.Pools) != 4 {
		t.Fatalf("expected 4 distinct pools (2 GPU + 2 CPU), got %d: %v", len(inv.Pools), inv.PoolNames)
	}
	if inv.CPUPoolCap == nil || inv.CPUPoolCap.NodePoolName != "worker-pool" {
		t.Errorf("expected worker-pool ahead of CriticalAddonsOnly tainted pool, got %+v", inv.CPUPoolCap)
	}
	if len(inv.PartitionTrees) != 2 || inv.PartitionTrees[0].Name != "tree-a" || inv.PartitionTrees[1].Name != "tree-b" {
		t.Errorf("expected sorted PartitionTrees [tree-a tree-b], got %+v", inv.PartitionTrees)
	}
}

func TestDiscoverInventory_SelfReviewEdgeCases(t *testing.T) {
	t.Parallel()
	// Restarting 8t nodes (no allocatable google.com/tpu) enriching a 4x4 v6e CRD update ChipsPerNode to 8
	// (NodesPerSlice=2) and keep slice Ready; secondary 1t pool and control-plane TPU node are excluded from primary counts.
	node8tA := makeNode("v6e-8t-1", map[string]string{
		InstanceTypeLabel: "ct6e-standard-8t",
		TPUSliceTypeLabel: "v6e",
		TPUSliceNameLabel: "slice-8t",
	}, map[string]interface{}{"cpu": "120"}, true)
	node8tB := makeNode("v6e-8t-2", map[string]string{
		InstanceTypeLabel: "ct6e-standard-8t",
		TPUSliceTypeLabel: "v6e",
		TPUSliceNameLabel: "slice-8t",
	}, map[string]interface{}{"cpu": "120"}, true)
	node1t := makeNode("v6e-1t-1", map[string]string{
		InstanceTypeLabel: "ct6e-standard-1t",
		TPUSliceTypeLabel: "v6e",
	}, map[string]interface{}{"cpu": "24", "google.com/tpu": "1"}, true)
	ctrlTPU := makeNode("ctrl-tpu", map[string]string{
		InstanceTypeLabel:     "ct6e-standard-8t",
		TPUSliceTypeLabel:     "v6e",
		ControlPlaneRoleLabel: "",
	}, map[string]interface{}{"cpu": "120", "google.com/tpu": "8"}, true)

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     {node8tA, node8tB, node1t, ctrlTPU},
			TPUSliceGVR: {makeSliceCRD("slice-8t", "", "v6e", "4x4", "True", "READY", nil)},
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if inv.TPU == nil || inv.TPU.DefaultSliceName != "slice-8t" || inv.TPU.ChipsPerNode != 8 || inv.TPU.NodesPerSlice != 2 {
		t.Fatalf("restarting 8t slice enrichment failed: %+v", inv.TPU)
	}
	if inv.TPU.TotalTPUNodes != 2 || inv.TPU.ReadyTPUNodes != 2 {
		t.Errorf("primary TPU node counts leaked secondary/control-plane pools: %d/%d", inv.TPU.ReadyTPUNodes, inv.TPU.TotalTPUNodes)
	}
}

func TestDiscoverInventory_UnreadyFallbackAndFailClosedSkipsTrees(t *testing.T) {
	t.Parallel()
	// Unready slice must not override PartitionTree.RootTopology, and mismatched slices print "no active slice".
	ptMultiPrefix := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "partition.tpu.google.com/v1alpha1",
		"kind":       "PartitionTree",
		"metadata":   map[string]interface{}{"name": "v6e-tree"},
		"status": map[string]interface{}{
			"partitionId":       "v6e-root",
			"partitionTypeInfo": "TPU_V6E_2_4",
		},
	}}
	lister2 := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {makeNode("v6e-8t-only", map[string]string{
				InstanceTypeLabel: "ct6e-standard-8t",
				TPUSliceTypeLabel: "v6e",
			}, map[string]interface{}{"cpu": "120", "google.com/tpu": "8"}, true)},
			TPUSliceGVR:         {makeSliceCRD("other-4t-slice", "", "v6e", "2x2", "False", "NotCreated", nil)},
			TPUPartitionTreeGVR: {ptMultiPrefix},
		},
	}
	inv2, err := DiscoverInventory(lister2, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory(2) failed: %v", err)
	}
	if inv2.TPU == nil || inv2.TPU.Topology != "2x4" {
		t.Errorf("expected fallback topology 2x4 from TPU_V6E_2_4 PartitionTree, got %+v", inv2.TPU)
	}
	if joined := strings.Join(inv2.SummaryLines(), "\n"); !strings.Contains(joined, "no active slice") {
		t.Errorf("expected 'no active slice' when only mismatched-shape slice exists, got:\n%s", joined)
	}

	// Auto fail-closed must return before calling listTPUPartitionTrees.
	lister3 := &fakeDiscoveryLister{
		errGVR: map[schema.GroupVersionResource]error{
			NodeGVR:     apierrors.NewForbidden(NodeGVR.GroupResource(), "", errors.New("forbidden")),
			TPUSliceGVR: apierrors.NewForbidden(TPUSliceGVR.GroupResource(), "", errors.New("forbidden")),
		},
	}
	if _, err := DiscoverInventory(lister3, &fakeAPIGroupChecker{err: errors.New("probe 403")}, DiscoverOptions{}); err == nil {
		t.Fatalf("expected fail-closed error")
	}
	for _, gvr := range lister3.calls {
		if gvr == TPUPartitionTreeGVR || gvr == TPUPartitionTreeV1Beta1GVR {
			t.Errorf("unexpected PartitionTree call %v after Slice fail-closed error", gvr)
		}
	}
}

func TestDiscoverInventory_CordonedNodeAnd1x1SliceCRD(t *testing.T) {
	t.Parallel()
	cordoned := makeNodeWithSpec("cpu-cordoned", map[string]string{
		InstanceTypeLabel: "n2-standard-8",
	}, map[string]interface{}{"cpu": "8", "memory": "32Gi"}, true, true, nil)

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     {cordoned},
			TPUSliceGVR: {makeSliceCRD("v6e-1x1-slice", "", "v6e", "1x1", "True", "READY", nil)},
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if inv.TotalNodes != 1 || inv.ReadyNodes != 0 {
		t.Errorf("cordoned node must be unready: got %d/%d ready", inv.ReadyNodes, inv.TotalNodes)
	}
	if inv.CPUPoolCap == nil || inv.CPUPoolCap.ReadyNodes != 0 || inv.CPUPoolCap.TotalNodes != 1 {
		t.Errorf("expected CPUPoolCap 0/1 ready for cordoned node, got %+v", inv.CPUPoolCap)
	}
	if inv.TPU == nil || inv.TPU.DefaultSliceName != "v6e-1x1-slice" || inv.TPU.MachineType != "ct6e-standard-1t" || inv.TPU.ChipsPerNode != 1 || inv.TPU.NodesPerSlice != 1 {
		t.Errorf("1x1 slice CRD inference failed: %+v", inv.TPU)
	}
}

func TestDiscoverInventory_ExplicitModeSliceVsPartitionTreePrecedence(t *testing.T) {
	t.Parallel()
	node403 := apierrors.NewForbidden(NodeGVR.GroupResource(), "", errors.New("nodes forbidden"))
	tree := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "partition.tpu.google.com/v1beta1",
		"kind":       "PartitionTree",
		"metadata":   map[string]interface{}{"name": "tpu7x-tree"},
		"status": map[string]interface{}{
			"acceleratorType": "tpu7x",
			"root": map[string]interface{}{
				"topology":    "4x4x4",
				"partitionId": "root-4x4x4",
				"childPartitions": []interface{}{
					map[string]interface{}{"topology": "2x2x1", "partitionId": "child-2x2x1"},
				},
			},
		},
	}}

	// Case 1: Ready Slice anchors TPUInventory ahead of PartitionTree while preserving tree topologies.
	listerReady := &fakeDiscoveryLister{
		errGVR: map[schema.GroupVersionResource]error{NodeGVR: node403},
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			TPUSliceGVR:         {makeSliceCRD("ready-2x2x2", "", "tpu7x", "2x2x2", "True", "READY", nil)},
			TPUPartitionTreeGVR: {tree},
		},
	}
	inv, err := DiscoverInventory(listerReady, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("explicit ready-slice discovery failed: %v", err)
	}
	assertExplicitSlicePrecedence(t, inv, "ready-2x2x2", "2x2x2", 2)
	if !slices.Contains(inv.TPU.Topologies, "4x4x4") || !slices.Contains(inv.TPU.Topologies, "2x2x1") {
		t.Errorf("expected PartitionTree topologies merged, got %v", inv.TPU.Topologies)
	}

	// Case 2: Only unready Slice exists -> falls back to PartitionTree.RootTopology.
	listerUnready := &fakeDiscoveryLister{
		errGVR: map[schema.GroupVersionResource]error{NodeGVR: node403},
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			TPUSliceGVR:         {makeSliceCRD("unready-2x2x1", "", "tpu7x", "2x2x1", "False", "NotCreated", nil)},
			TPUPartitionTreeGVR: {tree},
		},
	}
	inv2, err := DiscoverInventory(listerUnready, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("explicit fallback discovery failed: %v", err)
	}
	assertExplicitSlicePrecedence(t, inv2, "", "4x4x4", 16)
}

func assertExplicitSlicePrecedence(t *testing.T, inv *Inventory, wantSlice, wantTopo string, wantNodes int) {
	t.Helper()
	if !inv.ExplicitMode || inv.TPU == nil {
		t.Fatalf("expected explicit mode with TPU inventory, got %+v", inv)
	}
	if inv.TPU.DefaultSliceName != wantSlice || inv.TPU.Topology != wantTopo || inv.TPU.NodesPerSlice != wantNodes {
		t.Errorf("got (slice=%q, topo=%q, nodes=%d), want (%q, %q, %d)",
			inv.TPU.DefaultSliceName, inv.TPU.Topology, inv.TPU.NodesPerSlice, wantSlice, wantTopo, wantNodes)
	}
}

func TestDiscoverInventory_MixedPoolDeterminism25Runs(t *testing.T) {
	t.Parallel()
	nodes := []unstructured.Unstructured{
		makeNode("ctrl-1", map[string]string{InstanceTypeLabel: "n2-standard-8", ControlPlaneRoleLabel: ""}, map[string]interface{}{"cpu": "8"}, true),
		makeNode("cpu-1", map[string]string{InstanceTypeLabel: "n2-standard-8", GKENodePoolLabel: "worker-cpu"}, map[string]interface{}{"cpu": "8"}, true),
		makeNode("gpu-l4", map[string]string{InstanceTypeLabel: "g2-standard-8", GKENodePoolLabel: "pool-l4", GKEAcceleratorLabel: "nvidia-l4"}, map[string]interface{}{"cpu": "8", "nvidia.com/gpu": "1"}, true),
		makeNode("gpu-h100", map[string]string{InstanceTypeLabel: "g2-standard-8", GKENodePoolLabel: "pool-h100", GKEAcceleratorLabel: "nvidia-h100"}, map[string]interface{}{"cpu": "8", "nvidia.com/gpu": "1"}, true),
		makeNode("tpu-1", map[string]string{InstanceTypeLabel: "tpu7x-standard-4t", TPUSliceTypeLabel: "tpu7x", TPUSliceNameLabel: "slice-b"}, map[string]interface{}{"cpu": "240", "google.com/tpu": "4"}, true),
		makeNode("tpu-2", map[string]string{InstanceTypeLabel: "tpu7x-standard-4t", TPUSliceTypeLabel: "tpu7x", TPUSliceNameLabel: "slice-a"}, map[string]interface{}{"cpu": "240", "google.com/tpu": "4"}, true),
	}
	sliceList := []unstructured.Unstructured{
		makeSliceCRD("slice-b", "", "tpu7x", "2x2x1", "True", "READY", nil),
		makeSliceCRD("slice-a", "", "tpu7x", "2x2x1", "True", "READY", nil),
	}
	trees := []unstructured.Unstructured{
		{Object: map[string]interface{}{
			"apiVersion": "partition.tpu.google.com/v1beta1",
			"kind":       "PartitionTree",
			"metadata":   map[string]interface{}{"name": "tree-z"},
			"status":     map[string]interface{}{"acceleratorType": "tpu7x", "root": map[string]interface{}{"topology": "4x4x4"}},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "partition.tpu.google.com/v1beta1",
			"kind":       "PartitionTree",
			"metadata":   map[string]interface{}{"name": "tree-a"},
			"status":     map[string]interface{}{"acceleratorType": "tpu7x", "root": map[string]interface{}{"topology": "2x2x2"}},
		}},
	}

	var baseline *Inventory
	var baseSummary []string
	for i := 0; i < 25; i++ {
		lister := &fakeDiscoveryLister{
			byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
				NodeGVR:             nodes,
				TPUSliceGVR:         sliceList,
				TPUPartitionTreeGVR: trees,
			},
		}
		inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
		if err != nil {
			t.Fatalf("run %d failed: %v", i, err)
		}
		summary := inv.SummaryLines()
		if i == 0 {
			baseline = inv
			baseSummary = summary
			continue
		}
		if !reflect.DeepEqual(inv, baseline) || !slices.Equal(summary, baseSummary) {
			t.Fatalf("run %d diverged from baseline:\ngot:  %+v\nwant: %+v", i, inv, baseline)
		}
	}
}

func TestDiscoverInventory_MultiPoolTopologiesIntelGPUAndOverflow(t *testing.T) {
	t.Parallel()
	// Two matching primary ct6e-standard-4t GKE pools with different topologies (2x4 and 4x4)
	// must contribute BOTH topologies to inv.TPU.Topologies.
	poolA := makeNode("v6e-a", map[string]string{
		InstanceTypeLabel:      "ct6e-standard-4t",
		GKENodePoolLabel:       "pool-a",
		GKETPUAcceleratorLabel: "tpu-v6e-slice",
		GKETPUTopologyLabel:    "2x4",
	}, map[string]interface{}{"cpu": "120", "google.com/tpu": "4"}, true)
	poolB := makeNode("v6e-b", map[string]string{
		InstanceTypeLabel:      "ct6e-standard-4t",
		GKENodePoolLabel:       "pool-b",
		GKETPUAcceleratorLabel: "tpu-v6e-slice",
		GKETPUTopologyLabel:    "4x4",
	}, map[string]interface{}{"cpu": "120", "google.com/tpu": "4"}, true)
	// Intel GPU node with both whole-device i915 and millicores must report 1 GPU (not 1001).
	intelNode := makeNode("intel-1", map[string]string{
		InstanceTypeLabel: "n2-standard-8",
		GKENodePoolLabel:  "intel-pool",
	}, map[string]interface{}{
		"cpu":                           "8",
		"gpu.intel.com/i915":            "1",
		"gpu.intel.com/millicores":      "1000",
		"gpu.intel.com/i915_monitoring": "1",
	}, true)

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {poolA, poolB, intelNode},
		},
	}
	inv, err := DiscoverInventory(lister, nil, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if inv.TPU == nil || !slices.Equal(inv.TPU.Topologies, []string{"2x4", "4x4"}) {
		t.Errorf("expected both primary pool topologies [2x4 4x4], got %+v", inv.TPU)
	}
	foundIntel := false
	for _, p := range inv.Pools {
		if p.NodePoolName == "intel-pool" {
			foundIntel = true
			if !p.IsGPU || p.GPUsPerNode != 1 || p.GPUType != "gpu.intel.com/i915" {
				t.Errorf("intel GPU pool = %+v, want 1 GPU of gpu.intel.com/i915", p)
			}
		}
	}
	if !foundIntel {
		t.Errorf("intel-pool missing from discovered pools: %v", inv.PoolNames)
	}
	if chips, ok := parseTopologyChipCount("999999999x999999999x999999999"); ok || chips != 0 {
		t.Errorf("parseTopologyChipCount overflow = (%d, %v), want (0, false)", chips, ok)
	}
}

func TestDiscoverInventory_PartitionTree403NoFallbackAndCordonedTPUSlice(t *testing.T) {
	t.Parallel()
	// Cordoned TPU slice node (Ready=True, unschedulable=true) must mark its slice not ready.
	cordonedTPU := makeNodeWithSpec("tpu-cordoned", map[string]string{
		InstanceTypeLabel: "tpu7x-standard-4t",
		TPUSliceTypeLabel: "tpu7x",
		TPUSliceNameLabel: "cordoned-slice",
	}, map[string]interface{}{"cpu": "240", "google.com/tpu": "4"}, true, true, nil)

	lister := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     {cordonedTPU},
			TPUSliceGVR: {makeSliceCRD("cordoned-slice", "", "tpu7x", "2x2x1", "True", "READY", nil)},
		},
		errGVR: map[schema.GroupVersionResource]error{
			TPUPartitionTreeGVR: apierrors.NewForbidden(TPUPartitionTreeGVR.GroupResource(), "", errors.New("forbidden")),
		},
	}
	inv, err := DiscoverInventory(lister, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if slices.Contains(lister.calls, TPUPartitionTreeV1Beta1GVR) {
		t.Errorf("PartitionTree 403 must not fall back to v1beta1; calls=%v", lister.calls)
	}
	if inv.TPU == nil || inv.TPU.DefaultSliceName != "" || inv.TPU.ReadyTPUNodes != 0 {
		t.Errorf("cordoned TPU slice node must not be selected as ready default slice: %+v", inv.TPU)
	}
	if len(inv.Slices) != 1 || inv.Slices[0].Ready {
		t.Errorf("expected cordoned-slice Ready=false, got %+v", inv.Slices)
	}

	// Untyped primary TPU node with topology must keep its topology in inv.TPU.Topologies
	// even after applyDefaultSlice backfills tpu.TPUType and tpu.MachineType from a Slice CRD.
	untypedNode := makeNode("untyped-tpu", map[string]string{
		TPUSliceTopologyLabel: "2x2",
	}, map[string]interface{}{"cpu": "64", "google.com/tpu": "4"}, true)
	listerUntyped := &fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     {untypedNode},
			TPUSliceGVR: {makeSliceCRD("v5e-slice", "", "v5e", "2x4", "False", "NotCreated", nil)},
		},
	}
	invUntyped, err := DiscoverInventory(listerUntyped, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("untyped primary pool discovery failed: %v", err)
	}
	if invUntyped.TPU == nil || !slices.Equal(invUntyped.TPU.Topologies, []string{"2x2", "2x4"}) || invUntyped.TPU.Topology != "2x2" {
		t.Errorf("untyped primary pool topology lost after applyDefaultSlice: %+v", invUntyped.TPU)
	}
}

func TestDiscoverInventory_NonRBACNodeErrorFailsFast(t *testing.T) {
	t.Parallel()
	errs := []error{
		errors.New("dial tcp 10.0.0.1:443: connect: connection refused"),
		&url.Error{Op: "Get", URL: "https://10.0.0.1/api/v1/nodes", Err: errors.New("403 Forbidden")},
	}
	for _, nodeErr := range errs {
		lister := &fakeDiscoveryLister{errGVR: map[schema.GroupVersionResource]error{NodeGVR: nodeErr}}
		inv, err := DiscoverInventory(lister, nil, DiscoverOptions{})
		if err == nil || inv != nil {
			t.Fatalf("DiscoverInventory(nodeErr=%v) = (%+v, %v), want error without ExplicitMode", nodeErr, inv, err)
		}
	}
}

func TestDiscoverInventory_IntelGPUMetadataOnlyAndFallbackKeys(t *testing.T) {
	t.Parallel()
	metadataOnly := makeNode("intel-meta", map[string]string{
		InstanceTypeLabel: "n2-standard-8",
		GKENodePoolLabel:  "intel-meta-pool",
	}, map[string]interface{}{
		"cpu":                           "8",
		"gpu.intel.com/millicores":      "1000",
		"gpu.intel.com/i915_monitoring": "1",
		"gpu.intel.com/memory.max":      "16Gi",
		"gpu.intel.com/tiles":           "2",
	}, true)
	customKey := makeNode("intel-custom", map[string]string{
		InstanceTypeLabel: "n2-standard-8",
		GKENodePoolLabel:  "intel-custom-pool",
	}, map[string]interface{}{
		"cpu":                      "8",
		"gpu.intel.com/millicores": "1000",
		"gpu.intel.com/tiles":      "2",
		"gpu.intel.com/custom-gpu": "2",
	}, true)

	inv, err := DiscoverInventory(&fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{NodeGVR: {metadataOnly, customKey}},
	}, nil, DiscoverOptions{})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	var foundMeta, foundCustom bool
	for _, p := range inv.Pools {
		switch p.NodePoolName {
		case "intel-meta-pool":
			foundMeta = true
			if p.IsGPU || p.GPUsPerNode != 0 {
				t.Errorf("metadata-only Intel pool misclassified as GPU: %+v", p)
			}
		case "intel-custom-pool":
			foundCustom = true
			if !p.IsGPU || p.GPUsPerNode != 2 || p.GPUType != "gpu.intel.com/custom-gpu" {
				t.Errorf("custom Intel GPU pool = %+v, want 2 of gpu.intel.com/custom-gpu", p)
			}
		}
	}
	if !foundMeta || !foundCustom {
		t.Errorf("missing expected Intel pools (foundMeta=%v foundCustom=%v): %v", foundMeta, foundCustom, inv.PoolNames)
	}
}

func TestDiscoverInventory_Day2TPUPoolRankingAndTerminatingObjects(t *testing.T) {
	t.Parallel()
	// Healthy 4/4 TPU pool must beat a mostly-drained 1/8 TPU pool.
	degraded := PoolCapacity{MachineType: "ct6e-standard-4t", TotalNodes: 8, ReadyNodes: 1}
	healthy := PoolCapacity{MachineType: "ct6e-standard-8t", TotalNodes: 4, ReadyNodes: 4}
	if !isBetterTPUPool("pool-healthy", healthy, false, "pool-degraded", &degraded, false) {
		t.Errorf("expected healthy 4/4 TPU pool to beat degraded 1/8 TPU pool")
	}

	// Terminating Node, Slice, and PartitionTree (metadata.deletionTimestamp set) must not be ready/active.
	const deletionTS = "2026-11-23T00:00:00Z"
	termNode := makeNode("tpu-term", map[string]string{
		InstanceTypeLabel: "tpu7x-standard-4t",
		TPUSliceTypeLabel: "tpu7x",
	}, map[string]interface{}{"cpu": "240", "google.com/tpu": "4"}, true)
	termNode.Object["metadata"].(map[string]interface{})["deletionTimestamp"] = deletionTS

	termSlice := makeSliceCRD("slice-term", "", "tpu7x", "2x2x2", "True", "READY", nil)
	termSlice.Object["metadata"].(map[string]interface{})["deletionTimestamp"] = deletionTS

	termTree := unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "tree-term", "deletionTimestamp": deletionTS},
		"status":   map[string]interface{}{"partitionId": "pod1_r1_4x4x4_1"},
	}}
	liveTree := unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "tree-live"},
		"status":   map[string]interface{}{"partitionId": "pod1_r1_4x4x4_1"},
	}}

	inv, err := DiscoverInventory(&fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:             {termNode},
			TPUSliceGVR:         {termSlice},
			TPUPartitionTreeGVR: {termTree, liveTree},
		},
	}, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory failed: %v", err)
	}
	if inv.ReadyNodes != 0 || inv.TPU == nil || inv.TPU.ReadyTPUNodes != 0 || inv.TPU.DefaultSliceName != "" {
		t.Errorf("terminating node/slice must not be ready or default: ReadyNodes=%d TPU=%+v", inv.ReadyNodes, inv.TPU)
	}
	if len(inv.Slices) != 1 || inv.Slices[0].Ready {
		t.Errorf("terminating slice must have Ready=false, got %+v", inv.Slices)
	}
	if len(inv.PartitionTrees) != 1 || inv.PartitionTrees[0].Name != "tree-live" || inv.PartitionTrees[0].RootTopology != "4x4x4" {
		t.Errorf("expected only live underscore PartitionTree with 4x4x4, got %+v", inv.PartitionTrees)
	}
}

func TestDiscoverInventory_MultiShape8tPoolAndSingleChipPartitionTree(t *testing.T) {
	t.Parallel()
	// 1. ct6e-standard-8t pool (without node slice-name labels) must reject a 2x2 slice (4 chips)
	// while selecting a ready 2x4 v6e Slice CRD and computing NodesPerSlice=1 using 8 chips/node.
	node8t := makeNode("v6e-8t-0", map[string]string{
		InstanceTypeLabel: "ct6e-standard-8t",
		TPUSliceTypeLabel: "v6e",
	}, map[string]interface{}{"cpu": "120", "google.com/tpu": "8"}, true)
	inv8t, err := DiscoverInventory(&fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {node8t},
			TPUSliceGVR: {
				makeSliceCRD("a-slice-2x2", "", "v6e", "2x2", "True", "READY", nil),
				makeSliceCRD("b-slice-2x4", "", "v6e", "2x4", "True", "READY", nil),
			},
		},
	}, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil {
		t.Fatalf("DiscoverInventory(8t) failed: %v", err)
	}
	if inv8t.TPU == nil || inv8t.TPU.DefaultSliceName != "b-slice-2x4" || inv8t.TPU.ChipsPerNode != 8 || inv8t.TPU.NodesPerSlice != 1 {
		t.Fatalf("8t pool slice matching = %+v, want DefaultSliceName=b-slice-2x4 ChipsPerNode=8 NodesPerSlice=1", inv8t.TPU)
	}

	// 2. Explicit-mode 1x1 and 1x1x1 PartitionTrees (no nodes, no slices) must infer 1t host shapes.
	forbiddenNodes := apierrors.NewForbidden(NodeGVR.GroupResource(), "", errors.New("forbidden"))
	for _, tc := range []struct {
		tpuType, topo, wantMachine string
	}{
		{"v6e", "1x1", "ct6e-standard-1t"},
		{"tpu7x", "1x1x1", "tpu7x-standard-1t"},
	} {
		pt := unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "pt-" + tc.tpuType},
			"status":   map[string]interface{}{"acceleratorType": tc.tpuType, "root": map[string]interface{}{"topology": tc.topo}},
		}}
		invPT, err := DiscoverInventory(&fakeDiscoveryLister{
			byGVR:  map[schema.GroupVersionResource][]unstructured.Unstructured{TPUPartitionTreeGVR: {pt}},
			errGVR: map[schema.GroupVersionResource]error{NodeGVR: forbiddenNodes},
		}, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
		if err != nil {
			t.Fatalf("DiscoverInventory(pt %s) failed: %v", tc.tpuType, err)
		}
		if invPT.TPU == nil || invPT.TPU.MachineType != tc.wantMachine || invPT.TPU.ChipsPerNode != 1 || invPT.TPU.NodesPerSlice != 1 {
			t.Errorf("single-chip PartitionTree(%s/%s) = %+v, want machine=%s chips=1 nodes=1", tc.tpuType, tc.topo, invPT.TPU, tc.wantMachine)
		}
	}
}

func TestDiscoverInventory_SingleHost1tAnd8tSliceRejection(t *testing.T) {
	t.Parallel()
	// Single-host 8t pool must reject multi-host 4x4 while keeping 2x4.
	node8t := makeNode("v6e-8t-0", map[string]string{
		InstanceTypeLabel: "ct6e-standard-8t",
		TPUSliceTypeLabel: "v6e",
	}, map[string]interface{}{"cpu": "120", "google.com/tpu": "8"}, true)
	inv8t, err := DiscoverInventory(&fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR: {node8t},
			TPUSliceGVR: {
				makeSliceCRD("a-slice-4x4", "", "v6e", "4x4", "True", "READY", nil),
				makeSliceCRD("b-slice-2x4", "", "v6e", "2x4", "True", "READY", nil),
			},
		},
	}, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil || inv8t.TPU == nil || inv8t.TPU.DefaultSliceName != "b-slice-2x4" {
		t.Fatalf("8t pool = (%+v, %v), want b-slice-2x4 (rejecting 4x4)", inv8t, err)
	}

	// Single-chip 1t pool must reject multi-chip 2x4.
	node1t := makeNode("v6e-1t-0", map[string]string{
		InstanceTypeLabel: "ct6e-standard-1t",
		TPUSliceTypeLabel: "v6e",
	}, map[string]interface{}{"cpu": "24", "google.com/tpu": "1"}, true)
	inv1t, err := DiscoverInventory(&fakeDiscoveryLister{
		byGVR: map[schema.GroupVersionResource][]unstructured.Unstructured{
			NodeGVR:     {node1t},
			TPUSliceGVR: {makeSliceCRD("slice-2x4", "", "v6e", "2x4", "True", "READY", nil)},
		},
	}, &fakeAPIGroupChecker{groups: map[string]bool{TPUSliceAPIGroupVersion: true}}, DiscoverOptions{HardwareResolver: testHardwareResolver{}})
	if err != nil || inv1t.TPU == nil || inv1t.TPU.DefaultSliceName != "" {
		t.Fatalf("1t pool = (%+v, %v), want empty DefaultSliceName (rejecting 2x4)", inv1t, err)
	}
}
