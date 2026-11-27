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

package gke

import (
	"encoding/json"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "sigs.k8s.io/yaml"
)

func parseRenderedJobSet(t *testing.T, manifest string) jobSetValidationSpec {
	t.Helper()
	var js jobSetValidationSpec
	if err := k8syaml.Unmarshal([]byte(manifest), &js); err != nil {
		t.Fatalf("unmarshal rendered JobSet: %v\n%s", err, manifest)
	}
	if len(js.Spec.ReplicatedJobs) == 0 {
		t.Fatalf("rendered JobSet missing replicatedJobs:\n%s", manifest)
	}
	return js
}

func makeTPUSlicePipelineOrchestrator(tpuType, machineType string) (*GKEOrchestrator, *MockExecutor) {
	mockExec := NewMockExecutor(nil)
	orc := newTestGKEOrchestrator(mockExec)
	orc.mode = orchestrator.TypeK8s
	orc.namespace = "ml-ns"
	orc.kubeContext = "tpu-slice-ctx"
	orc.addons = addonPlan{resolved: true, jobSetEnabled: true, kueueEnabled: false}
	orc.inventory = &k8s.Inventory{
		LabelScheme:        k8s.SchemeTPUSlice,
		TPUSliceAPIPresent: true,
		PoolNames:          []string{"tpu-pool"},
		Pools: map[string]k8s.PoolCapacity{
			"tpu-pool": {
				MachineType:         machineType,
				IsTPU:               true,
				TPUType:             tpuType,
				TPUsPerNode:         4,
				AllocatableCPUMilli: 240000,
				MemMiBPerNode:       1024 * 1024,
				ReadyNodes:          4,
				Topologies:          []string{"2x2x1", "2x2x2"},
			},
		},
		TPU: &k8s.TPUInventory{
			TPUType:            tpuType,
			MachineType:        machineType,
			ChipsPerNode:       4,
			Topology:           "2x2x2",
			NodesPerSlice:      2,
			DefaultSliceName:   "other-team-2x2x2",
			Topologies:         []string{"2x2x1", "2x2x2"},
			HasPartitionLabels: true,
		},
		Slices: []k8s.SliceInfo{
			{Name: "other-team-2x2x2", TPUType: tpuType, MachineType: machineType, Topology: "2x2x2", Ready: true, State: "READY", NodesPerSlice: 2, ChipsPerNode: 4},
			{Name: "node-only-slice", TPUType: tpuType, MachineType: machineType, Topology: "", Ready: true, State: "READY", NodesPerSlice: 2, ChipsPerNode: 4},
			{Name: "terminating-slice", TPUType: tpuType, MachineType: machineType, Topology: "2x2x1", Ready: false, Terminating: true, State: "Terminating", NodesPerSlice: 1, ChipsPerNode: 4},
		},
	}
	return orc, mockExec
}

func renderTPUSliceJobSetForTest(t *testing.T, orc *GKEOrchestrator, job *orchestrator.JobDefinition) jobSetValidationSpec {
	t.Helper()
	profile, isDyn, isStatic, err := orc.resolveHardwareRequirements(job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements() err = %v", err)
	}
	opts, err := orc.PrepareManifestOptions(*job, "registry.local/jax:v1", profile, isDyn, isStatic)
	if err != nil {
		t.Fatalf("PrepareManifestOptions() err = %v", err)
	}
	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest() err = %v", err)
	}
	return parseRenderedJobSet(t, manifest)
}

func mustMarshalEventList(t *testing.T, events ...corev1.Event) string {
	t.Helper()
	b, err := json.Marshal(corev1.EventList{Items: events})
	if err != nil {
		t.Fatalf("marshal EventList: %v", err)
	}
	return string(b)
}

func assertDynamicPodTPUSliceContract(t *testing.T, podTmpl corev1.PodTemplateSpec, wantType, wantTopo string) {
	t.Helper()
	if podTmpl.Labels[k8s.TPUSliceTypeLabel] != wantType || podTmpl.Labels[k8s.TPUSliceTopologyLabel] != wantTopo {
		t.Errorf("pod labels = %v, want type=%s topology=%s", podTmpl.Labels, wantType, wantTopo)
	}
	for _, forbid := range []string{k8s.TPUSliceNameLabel, k8s.TPUSlicePrefixLabel} {
		if _, found := podTmpl.Labels[forbid]; found {
			t.Errorf("dynamic JobSet pod must never set %s: %v", forbid, podTmpl.Labels)
		}
	}
	if podTmpl.Spec.SchedulerName != k8s.TPUSliceSchedulerName {
		t.Errorf("pod schedulerName = %q, want %q", podTmpl.Spec.SchedulerName, k8s.TPUSliceSchedulerName)
	}
	if _, hasSel := podTmpl.Spec.NodeSelector[k8s.TPUSliceNameLabel]; hasSel {
		t.Errorf("fresh dynamic slice must omit nodeSelector[%s]: %v", k8s.TPUSliceNameLabel, podTmpl.Spec.NodeSelector)
	}
	if len(podTmpl.Spec.Containers[0].Ports) != 1 || podTmpl.Spec.Containers[0].Ports[0].ContainerPort != 8471 {
		t.Errorf("TPU container ports = %+v, want containerPort 8471", podTmpl.Spec.Containers[0].Ports)
	}
}

func TestTPUSlice_FreshSubsliceUsesJobSetPrefixAndSliceScheduler(t *testing.T) {
	orc, mockExec := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	job := orchestrator.JobDefinition{
		WorkloadName:   "fresh-subslice",
		ComputeType:    "tpu7x",
		Topology:       "2x2x1",
		NumSlices:      1,
		CommandToRun:   "python3 train.py",
		NodeConstraint: map[string]string{"topology.kubernetes.io/zone": "zone-a"},
	}
	js := renderTPUSliceJobSetForTest(t, orc, &job)
	if job.NodesPerSlice != 1 {
		t.Fatalf("NodesPerSlice = %d, want 1 for 2x2x1", job.NodesPerSlice)
	}
	assertZeroGCloudOrKueueSlicingCalls(t, mockExec.calls)

	if js.Labels[k8s.TPUSlicePrefixLabel] != "fresh-subslice" {
		t.Errorf("JobSet metadata.labels[%s] = %q, want fresh-subslice", k8s.TPUSlicePrefixLabel, js.Labels[k8s.TPUSlicePrefixLabel])
	}
	if _, hasExcl := js.Annotations["alpha.jobset.sigs.k8s.io/exclusive-topology"]; hasExcl {
		t.Errorf("dynamic tpu-slice JobSet must omit exclusive-topology annotation: %v", js.Annotations)
	}
	podTmpl := js.Spec.ReplicatedJobs[0].Template.Spec.Template
	assertDynamicPodTPUSliceContract(t, podTmpl, "tpu7x", "2x2x1")
	for _, forbid := range []string{k8s.TPUNumSlicesAnnotation, k8s.TPUSliceGroupNameAnnotation} {
		if _, found := podTmpl.Annotations[forbid]; found {
			t.Errorf("single-slice dynamic JobSet must omit gang annotation %s: %v", forbid, podTmpl.Annotations)
		}
	}
	if podTmpl.Spec.NodeSelector["topology.kubernetes.io/zone"] != "zone-a" {
		t.Errorf("ordinary --node-constraint must be preserved: %v", podTmpl.Spec.NodeSelector)
	}
}

func TestTPUSlice_PrefixLengthGuardAndCustomSchedulerOverride(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	exact := strings.Repeat("a", maxTPUSlicePrefixLen)
	if got, err := orc.buildTPUSliceWorkloadLabels(orchestrator.JobDefinition{WorkloadName: exact}, true); err != nil || !strings.Contains(got, exact) {
		t.Fatalf("buildTPUSliceWorkloadLabels(35 chars) = (%q, %v), want valid prefix", got, err)
	}
	tooLong := strings.Repeat("a", maxTPUSlicePrefixLen+1)
	if _, err := orc.buildTPUSliceWorkloadLabels(orchestrator.JobDefinition{WorkloadName: tooLong}, true); err == nil || !strings.Contains(err.Error(), "35-character limit") {
		t.Fatalf("buildTPUSliceWorkloadLabels(36 chars) err = %v, want 35-character limit error", err)
	}

	job := orchestrator.JobDefinition{
		WorkloadName: "custom-sched",
		ComputeType:  "tpu7x",
		Topology:     "2x2x1",
		NumSlices:    1,
		GKEScheduler: "my-scheduler",
		CommandToRun: "python3 train.py",
	}
	js := renderTPUSliceJobSetForTest(t, orc, &job)
	if got := js.Spec.ReplicatedJobs[0].Template.Spec.Template.Spec.SchedulerName; got != "my-scheduler" {
		t.Errorf("explicit --gke-scheduler = %q, want my-scheduler", got)
	}
}

func TestTPUSlice_ParallelContainersDeclaresTPUCommPortOnce(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	job := orchestrator.JobDefinition{
		WorkloadName: "parallel-tpu7x",
		ComputeType:  "tpu7x",
		Topology:     "2x2x2",
		NumSlices:    1,
		CommandToRun: "python3 train.py",
	}
	profile, isDyn, isStatic, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements() err = %v", err)
	}
	opts, err := orc.PrepareManifestOptions(job, "registry.local/jax:v1", profile, isDyn, isStatic)
	if err != nil {
		t.Fatalf("PrepareManifestOptions() err = %v", err)
	}
	opts.ParallelContainers = 2
	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest() err = %v", err)
	}
	js := parseRenderedJobSet(t, manifest)
	containers := js.Spec.ReplicatedJobs[0].Template.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("len(containers) = %d, want 2 when ParallelContainers=2", len(containers))
	}
	if len(containers[0].Ports) != 1 || containers[0].Ports[0].ContainerPort != 8471 || containers[0].Ports[0].Name != "tpu-comm" {
		t.Errorf("container[0] ports = %+v, want single port 8471 (tpu-comm)", containers[0].Ports)
	}
	if len(containers[1].Ports) != 0 {
		t.Errorf("container[1] must not duplicate port 8471 / tpu-comm, got %+v", containers[1].Ports)
	}
}

func TestTPUSlice_MissingTopologyErrorsSingleAndMultiGen(t *testing.T) {
	t.Run("single_gen_suggests_ready_slice_and_shapes", func(t *testing.T) {
		orc, mockExec := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
		job := orchestrator.JobDefinition{WorkloadName: "no-topo-job", ComputeType: "tpu7x", NumSlices: 1}
		_, _, _, err := orc.resolveHardwareRequirements(&job)
		if err == nil {
			t.Fatalf("resolveHardwareRequirements() = nil, want --topology required error")
		}
		for _, want := range []string{
			"--topology is required to provision a new slice",
			"Cluster has tpu7x nodes with 4 chips each",
			"shapes present: 2x2x1, 2x2x2",
			"--node-constraint " + k8s.TPUSliceNameLabel + "=other-team-2x2x2",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want substring %q", err.Error(), want)
			}
		}
		if len(mockExec.calls) != 0 {
			t.Errorf("expected zero cluster calls, got %v", mockExec.calls)
		}
	})

	t.Run("multi_gen_scopes_chips_shapes_and_slice_to_requested_type", func(t *testing.T) {
		orc, _ := makeTPUSlicePipelineOrchestrator("v6e", "ct6e-standard-8t")
		orc.inventory.Pools["tpu-pool"] = k8s.PoolCapacity{
			MachineType: "ct6e-standard-8t", IsTPU: true, TPUType: "v6e", TPUsPerNode: 8, ReadyNodes: 4, Topologies: []string{"2x4"},
		}
		orc.inventory.Pools["tpu7x-pool"] = k8s.PoolCapacity{
			MachineType: "tpu7x-standard-4t", IsTPU: true, TPUType: "tpu7x", TPUsPerNode: 4, ReadyNodes: 2, Topologies: []string{"2x2x1"},
		}
		orc.inventory.PoolNames = []string{"tpu-pool", "tpu7x-pool"}
		orc.inventory.TPU = &k8s.TPUInventory{TPUType: "v6e", MachineType: "ct6e-standard-8t", ChipsPerNode: 8, Topologies: []string{"2x4"}}
		orc.inventory.Slices = []k8s.SliceInfo{
			{Name: "v6e-ready-slice", TPUType: "v6e", MachineType: "ct6e-standard-8t", Topology: "2x4", Ready: true, NodesPerSlice: 1, ChipsPerNode: 8},
		}

		job := orchestrator.JobDefinition{WorkloadName: "tpu7x-no-topo", ComputeType: "tpu7x", NumSlices: 1}
		_, _, _, err := orc.resolveHardwareRequirements(&job)
		if err == nil {
			t.Fatalf("resolveHardwareRequirements() = nil, want --topology error")
		}
		for _, want := range []string{"Cluster has tpu7x nodes with 4 chips each", "shapes present: 2x2x1", "--node-constraint " + k8s.TPUSliceNameLabel + "=<slice-name>"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("multi-gen error = %q, want %q", err.Error(), want)
			}
		}
		if strings.Contains(err.Error(), "v6e-ready-slice") {
			t.Errorf("multi-gen error must not suggest incompatible v6e slice: %q", err.Error())
		}
	})

	t.Run("static_reuse_with_underivable_topology_does_not_tell_user_to_pass_slice_constraint", func(t *testing.T) {
		orc, _ := makeTPUSlicePipelineOrchestrator("tpu-custom", "bm-tpu-custom")
		orc.inventory.Pools["tpu-pool"] = k8s.PoolCapacity{MachineType: "bm-tpu-custom", IsTPU: true, TPUType: "tpu-custom", TPUsPerNode: 2, ReadyNodes: 2}
		orc.inventory.Slices = []k8s.SliceInfo{
			{Name: "empty-topo-slice", TPUType: "tpu-custom", MachineType: "bm-tpu-custom", Topology: "", Ready: true, NodesPerSlice: 0, ChipsPerNode: 2},
		}
		job := orchestrator.JobDefinition{
			WorkloadName:   "static-empty-topo",
			ComputeType:    "bm-tpu-custom",
			NumSlices:      1,
			NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "empty-topo-slice"},
		}
		_, _, _, err := orc.resolveHardwareRequirements(&job)
		if err == nil || !strings.Contains(err.Error(), "--topology is required for TPU machine type") {
			t.Fatalf("static reuse with empty topology err = %v, want standard --topology error", err)
		}
		if strings.Contains(err.Error(), "To reuse an existing slice") {
			t.Errorf("static reuse error must not tell user to pass --node-constraint again: %q", err.Error())
		}
	})
}

func TestTPUSlice_ShorthandPreservedOnSchemeTPUSliceWithJobSet(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("v6e", "ct6e-standard-4t")
	orc.inventory.TPU.Topologies = []string{"2x2", "2x4"}
	orc.inventory.Pools["tpu-pool"] = k8s.PoolCapacity{
		MachineType:         "ct6e-standard-4t",
		IsTPU:               true,
		TPUType:             "v6e",
		TPUsPerNode:         4,
		AllocatableCPUMilli: 120000,
		MemMiBPerNode:       512 * 1024,
		ReadyNodes:          4,
		Topologies:          []string{"2x2", "2x4"},
	}
	job := orchestrator.JobDefinition{
		WorkloadName: "v6e-shorthand",
		ComputeType:  "v6e-8",
		NumSlices:    1,
		CommandToRun: "python3 train.py",
	}
	js := renderTPUSliceJobSetForTest(t, orc, &job)
	if job.Topology != "2x4" || job.NodesPerSlice != 2 {
		t.Fatalf("v6e-8 resolved (topology=%q, nodes=%d), want (2x4, 2)", job.Topology, job.NodesPerSlice)
	}
	if js.Labels[k8s.TPUSlicePrefixLabel] != "v6e-shorthand" {
		t.Errorf("JobSet metadata.labels = %v, want %s=v6e-shorthand", js.Labels, k8s.TPUSlicePrefixLabel)
	}
	podTmpl := js.Spec.ReplicatedJobs[0].Template.Spec.Template
	if podTmpl.Labels[k8s.TPUSliceTopologyLabel] != "2x4" || podTmpl.Labels[k8s.TPUSliceTypeLabel] != "v6e" {
		t.Errorf("v6e-8 pod labels = %v, want type=v6e topology=2x4", podTmpl.Labels)
	}
	if podTmpl.Spec.SchedulerName != k8s.TPUSliceSchedulerName {
		t.Errorf("v6e-8 schedulerName = %q, want %q", podTmpl.Spec.SchedulerName, k8s.TPUSliceSchedulerName)
	}
}

func TestTPUSlice_StaticReuseSingleSlice(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	reuseJob := orchestrator.JobDefinition{
		WorkloadName:   "reuse-job",
		NumSlices:      1,
		CommandToRun:   "python3 train.py",
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "other-team-2x2x2"},
	}
	js := renderTPUSliceJobSetForTest(t, orc, &reuseJob)
	if reuseJob.Topology != "2x2x2" || reuseJob.NodesPerSlice != 2 {
		t.Fatalf("reuse resolved (topology=%q, nodes=%d), want (2x2x2, 2)", reuseJob.Topology, reuseJob.NodesPerSlice)
	}
	if _, hasPrefix := js.Labels[k8s.TPUSlicePrefixLabel]; hasPrefix {
		t.Errorf("static reuse must omit JobSet label %s: %v", k8s.TPUSlicePrefixLabel, js.Labels)
	}
	if got := js.Annotations["alpha.jobset.sigs.k8s.io/exclusive-topology"]; got != k8s.TPUSliceNameLabel {
		t.Errorf("multi-host static reuse exclusive-topology = %q, want %q", got, k8s.TPUSliceNameLabel)
	}
	podTmpl := js.Spec.ReplicatedJobs[0].Template.Spec.Template
	if podTmpl.Spec.NodeSelector[k8s.TPUSliceNameLabel] != "other-team-2x2x2" || podTmpl.Spec.SchedulerName != "" {
		t.Errorf("static reuse nodeSelector=%v scheduler=%q, want pin + default scheduler", podTmpl.Spec.NodeSelector, podTmpl.Spec.SchedulerName)
	}
	if podTmpl.Labels[k8s.TPUSliceTypeLabel] != "tpu7x" || podTmpl.Labels[k8s.TPUSliceTopologyLabel] != "2x2x2" {
		t.Errorf("static reuse pod labels = %v, want type=tpu7x topology=2x2x2", podTmpl.Labels)
	}
	if _, hasName := podTmpl.Labels[k8s.TPUSliceNameLabel]; hasName {
		t.Errorf("static reuse pod labels must omit %s: %v", k8s.TPUSliceNameLabel, podTmpl.Labels)
	}

	nodeOnlyJob := orchestrator.JobDefinition{
		WorkloadName:   "node-only-reuse",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "node-only-slice"},
	}
	if _, _, _, err := orc.resolveHardwareRequirements(&nodeOnlyJob); err != nil {
		t.Fatalf("node-only reuse err = %v", err)
	}
	if nodeOnlyJob.Topology != "2x2x2" || nodeOnlyJob.NodesPerSlice != 2 {
		t.Errorf("node-only reuse resolved (topology=%q, nodes=%d), want (2x2x2, 2)", nodeOnlyJob.Topology, nodeOnlyJob.NodesPerSlice)
	}
}

func TestTPUSlice_StaticReuseMultiSliceCustomType(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu-custom", "bm-host")
	orc.inventory.ExplicitMode = true
	orc.inventory.Pools = nil
	orc.inventory.PoolNames = nil
	orc.inventory.TPU = nil
	orc.inventory.Slices = []k8s.SliceInfo{
		{Name: "slice-a", TPUType: "tpu-custom", MachineType: "bm-host", Topology: "2x2x2", Ready: true, NodesPerSlice: 2, ChipsPerNode: 4},
		{Name: "slice-b", TPUType: "tpu-custom", MachineType: "bm-host", Topology: "2x2x2", Ready: true, NodesPerSlice: 2, ChipsPerNode: 4},
	}
	job := orchestrator.JobDefinition{
		WorkloadName:   "multi-static-bm",
		ComputeType:    "bm-host",
		Topology:       "2x2x2",
		NumSlices:      2,
		CommandToRun:   "python3 train.py",
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "slice-a|slice-b"},
	}
	js := renderTPUSliceJobSetForTest(t, orc, &job)
	if _, hasPrefix := js.Labels[k8s.TPUSlicePrefixLabel]; hasPrefix {
		t.Errorf("multi-slice static reuse must omit %s: %v", k8s.TPUSlicePrefixLabel, js.Labels)
	}
	if got := js.Annotations["alpha.jobset.sigs.k8s.io/exclusive-topology"]; got != k8s.TPUSliceNameLabel {
		t.Errorf("multi-slice static exclusive-topology = %q, want %q", got, k8s.TPUSliceNameLabel)
	}
	podTmpl := js.Spec.ReplicatedJobs[0].Template.Spec.Template
	if podTmpl.Labels[k8s.TPUSliceTypeLabel] != "tpu-custom" || podTmpl.Labels[k8s.TPUSliceTopologyLabel] != "2x2x2" {
		t.Errorf("multi-slice static pod labels = %v, want type=tpu-custom topology=2x2x2", podTmpl.Labels)
	}
	if podTmpl.Spec.SchedulerName != "" || len(podTmpl.Annotations) != 0 {
		t.Errorf("multi-slice static must use default scheduler and omit gang annotations: sched=%q ann=%v", podTmpl.Spec.SchedulerName, podTmpl.Annotations)
	}
	if podTmpl.Spec.Affinity == nil || podTmpl.Spec.Affinity.NodeAffinity == nil {
		t.Fatalf("multi-slice static reuse must render nodeAffinity for slice-a|slice-b")
	}
}

func TestTPUSlice_StaticReuseRejections(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	cases := []struct {
		name      string
		job       orchestrator.JobDefinition
		wantParts []string
	}{
		{
			name: "conflicting_topology",
			job: orchestrator.JobDefinition{
				WorkloadName: "conflict-job", Topology: "2x2x1", NumSlices: 1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "other-team-2x2x2"},
			},
			wantParts: []string{"2x2x1", "2x2x2"},
		},
		{
			name: "node_only_topology_mismatch",
			job: orchestrator.JobDefinition{
				WorkloadName: "node-only-mismatch", Topology: "2x2x1", NumSlices: 1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "node-only-slice"},
			},
			wantParts: []string{"does not match slice"},
		},
		{
			name: "terminating_slice",
			job: orchestrator.JobDefinition{
				WorkloadName: "term-reuse", NumSlices: 1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "terminating-slice"},
			},
			wantParts: []string{"terminating"},
		},
		{
			name: "multi_slice_insufficient_static_slices",
			job: orchestrator.JobDefinition{
				WorkloadName: "multi-static", NumSlices: 2,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "other-team-2x2x2"},
			},
			wantParts: []string{"--num-slices=2 requires at least 2 distinct READY slices", "omit --node-constraint"},
		},
		{
			name: "pipe_delimited_unknown_slice_rejected",
			job: orchestrator.JobDefinition{
				WorkloadName: "pipe-static", NumSlices: 2,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "other-team-2x2x2|missing-slice"},
			},
			wantParts: []string{"missing-slice", "does not exist"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := tc.job
			_, _, _, err := orc.resolveHardwareRequirements(&j)
			if err == nil {
				t.Fatalf("resolveHardwareRequirements() = nil, want error containing %v", tc.wantParts)
			}
			for _, part := range tc.wantParts {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("err = %q, want substring %q", err.Error(), part)
				}
			}
		})
	}
}

func TestTPUSlice_StaticReuseRejectsShorthandMismatchAndDerivesV5pNodes(t *testing.T) {
	v6eOrc, _ := makeTPUSlicePipelineOrchestrator("v6e", "ct6e-standard-4t")
	v6eOrc.inventory.Slices = []k8s.SliceInfo{
		{Name: "small-2x2", TPUType: "v6e", MachineType: "ct6e-standard-4t", Topology: "2x2", Ready: true, State: "READY", NodesPerSlice: 1, ChipsPerNode: 4},
	}
	mismatch := orchestrator.JobDefinition{
		WorkloadName:   "v6e-8-vs-2x2",
		ComputeType:    "v6e-8",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "small-2x2"},
	}
	if _, _, _, err := v6eOrc.resolveHardwareRequirements(&mismatch); err == nil || !strings.Contains(err.Error(), "2x4") || !strings.Contains(err.Error(), "2x2") {
		t.Errorf("v6e-8 against 2x2 slice err = %v, want mismatch showing 2x4 vs 2x2", err)
	}

	for _, tc := range []struct {
		nodes int
		want  string
	}{
		{nodes: 1, want: "2x2x1"},
		{nodes: 2, want: "2x2x2"},
	} {
		got := deriveSliceTopologyFromNodes(
			&orchestrator.JobDefinition{MachineType: "ct5p-hightpu-4t", ComputeType: "v5p"},
			k8s.SliceInfo{TPUType: "v5p", MachineType: "ct5p-hightpu-4t", NodesPerSlice: tc.nodes, ChipsPerNode: 4},
		)
		if got != tc.want {
			t.Errorf("deriveSliceTopologyFromNodes(v5p, %d nodes) = %q, want %q", tc.nodes, got, tc.want)
		}
	}
}

func TestTPUSlice_MultiSliceGangAnnotations(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	job := orchestrator.JobDefinition{
		WorkloadName: "gang-train",
		ComputeType:  "tpu7x",
		Topology:     "2x2x1",
		NumSlices:    2,
		CommandToRun: "python3 train.py",
	}
	js := renderTPUSliceJobSetForTest(t, orc, &job)
	if js.Labels[k8s.TPUSlicePrefixLabel] != "gang-train" {
		t.Errorf("multi-slice JobSet label %s = %q, want gang-train", k8s.TPUSlicePrefixLabel, js.Labels[k8s.TPUSlicePrefixLabel])
	}
	if jobLabels := js.Spec.ReplicatedJobs[0].Template.Labels; jobLabels[k8s.TPUSliceTypeLabel] != "tpu7x" || jobLabels[k8s.TPUSliceTopologyLabel] != "2x2x1" {
		t.Errorf("multi-slice replicatedJob template.metadata.labels = %v, want type=tpu7x topology=2x2x1", jobLabels)
	}
	podTmpl := js.Spec.ReplicatedJobs[0].Template.Spec.Template
	if _, hasName := podTmpl.Labels[k8s.TPUSliceNameLabel]; hasName {
		t.Errorf("multi-slice pod must never emit slice-name: %v", podTmpl.Labels)
	}
	if _, hasPrefix := podTmpl.Labels[k8s.TPUSlicePrefixLabel]; hasPrefix {
		t.Errorf("multi-slice pod must not emit slice-prefix: %v", podTmpl.Labels)
	}
	if podTmpl.Annotations[k8s.TPUNumSlicesAnnotation] != "2" || podTmpl.Annotations[k8s.TPUSliceGroupNameAnnotation] != "gang-train" {
		t.Errorf("multi-slice annotations = %v, want num-slices=2 and slice-group-name=gang-train", podTmpl.Annotations)
	}
	if podTmpl.Spec.SchedulerName != k8s.TPUSliceSchedulerName {
		t.Errorf("multi-slice schedulerName = %q, want %q", podTmpl.Spec.SchedulerName, k8s.TPUSliceSchedulerName)
	}
	if _, hasSel := podTmpl.Spec.NodeSelector[k8s.TPUSliceNameLabel]; hasSel {
		t.Errorf("multi-slice fresh provisioning must omit nodeSelector[%s]", k8s.TPUSliceNameLabel)
	}

	// Pathways, non-TPU, single-slice, whitespace name, and static-slice jobs must never receive dynamic gang annotations.
	for _, tc := range []struct {
		name      string
		job       orchestrator.JobDefinition
		isTPU     bool
		numSlices int
	}{
		{"pathways", orchestrator.JobDefinition{WorkloadName: "pw-job", IsPathwaysJob: true}, true, 2},
		{"static_slice", orchestrator.JobDefinition{WorkloadName: "st-job", NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "slice-a"}}, true, 2},
		{"non_tpu", orchestrator.JobDefinition{WorkloadName: "cpu-job"}, false, 2},
		{"single_slice", orchestrator.JobDefinition{WorkloadName: "single-job"}, true, 1},
		{"whitespace_name", orchestrator.JobDefinition{WorkloadName: "   "}, true, 2},
	} {
		dynamic := orc.isDynamicTPUSliceWorkload(tc.job, tc.isTPU)
		if got := buildTPUSliceGangAnnotations(tc.job.WorkloadName, tc.numSlices, dynamic); got != nil {
			t.Errorf("buildTPUSliceGangAnnotations(%s, dynamic=%v) = %v, want nil", tc.name, dynamic, got)
		}
	}
}

func TestTPUSlice_ResolveTypeLabelEdgeCases(t *testing.T) {
	customOrc, _ := makeTPUSlicePipelineOrchestrator("v6e", "ct6e-standard-4t")
	customPool := k8s.PoolCapacity{MachineType: "bm-host", IsTPU: true, TPUType: "tpu-custom", TPUsPerNode: 4}
	customOrc.inventory.Pools["custom-pool"] = customPool
	customOrc.inventory.PoolNames = append(customOrc.inventory.PoolNames, "custom-pool")
	if got := customOrc.resolveTPUSliceTypeLabel(orchestrator.JobDefinition{MachineType: "bm-host"}, JobProfile{K8sPool: &customPool}); got != "tpu-custom" {
		t.Errorf("resolveTPUSliceTypeLabel(bm-host via K8sPool) = %q, want tpu-custom", got)
	}
	if got := customOrc.resolveTPUSliceTypeLabel(orchestrator.JobDefinition{MachineType: "bm-host"}, JobProfile{}); got != "tpu-custom" {
		t.Errorf("resolveTPUSliceTypeLabel(bm-host via pool lookup) = %q, want tpu-custom", got)
	}
	if got := customOrc.resolveTPUSliceTypeLabel(orchestrator.JobDefinition{MachineType: "tpu7x-standard-4t"}, JobProfile{}); got != "tpu7x" {
		t.Errorf("resolveTPUSliceTypeLabel(explicit MachineType, empty ComputeType) = %q, want tpu7x", got)
	}
}

func TestTPUSlice_HybridClusterOmitsGKENodePoolExclusiveTopology(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	orc.inventory.Pools["cpu-gke"] = k8s.PoolCapacity{NodePoolName: "gke-cpu-pool", MachineType: "n2-standard-8"}
	orc.inventory.PoolNames = append(orc.inventory.PoolNames, "cpu-gke")
	job := orchestrator.JobDefinition{
		WorkloadName: "hybrid-2x2x2",
		ComputeType:  "tpu7x",
		Topology:     "2x2x2",
		NumSlices:    1,
		CommandToRun: "python3 train.py",
	}
	js := renderTPUSliceJobSetForTest(t, orc, &job)
	if got := js.Annotations["alpha.jobset.sigs.k8s.io/exclusive-topology"]; got != "" {
		t.Errorf("SchemeTPUSlice dynamic multi-host TPU JobSet must omit exclusive-topology, got %q", got)
	}
}

func TestTPUSlice_StaleCustomTemplatesRejectedWithHint(t *testing.T) {
	const podWithSchedAndPort = `{{- define "pod_template" -}}
metadata:
  labels:
    gcluster.google.com/workload: {{.WorkloadName}}
{{- if .PodLabels }}
{{reindent 4 (StructuralData .PodLabels)}}
{{- end }}
spec:
  schedulerName: {{.SchedulerName}}
  restartPolicy: Never
  containers:
  {{- range .Containers }}
  - name: {{ .Name }}
    image: {{ $.FullImageName }}
    ports:
    - containerPort: 8471
    command:
    {{- range $.Command }}
    - {{ printf "%q" . }}
    {{- end }}
  {{- end }}
{{- end -}}`

	cases := []struct {
		name        string
		fileName    string
		content     string
		numSlice    int
		staticSlice string
		wantSub     string
	}{
		{
			name:     "missing_workload_labels_slice_prefix",
			fileName: "base.tmpl",
			content:  "{{- define \"workload_labels\" -}}\ngcluster.google.com/workload: {{.WorkloadName}}\n{{- end -}}",
			numSlice: 1,
			wantSub:  "JobSet metadata.labels is missing required TPU slice label " + k8s.TPUSlicePrefixLabel,
		},
		{
			name:     "forbidden_slice_name_on_dynamic_jobset_labels",
			fileName: "base.tmpl",
			content:  "{{- define \"workload_labels\" -}}\ngcluster.google.com/workload: {{.WorkloadName}}\ntpu.google.com/slice-prefix: {{.WorkloadName}}\ntpu.google.com/slice-name: bad-static\n{{- end -}}",
			numSlice: 1,
			wantSub:  "JobSet metadata.labels must not set " + k8s.TPUSliceNameLabel,
		},
		{
			name:        "forbidden_slice_prefix_on_static_jobset_labels",
			fileName:    "base.tmpl",
			content:     "{{- define \"workload_labels\" -}}\ngcluster.google.com/workload: {{.WorkloadName}}\ntpu.google.com/slice-prefix: unwanted\n{{- end -}}",
			numSlice:    1,
			staticSlice: "other-team-2x2x2",
			wantSub:     "JobSet metadata.labels must not set " + k8s.TPUSlicePrefixLabel,
		},
		{
			name:        "forbidden_scheduler_slice_prefix_on_static_jobset_labels",
			fileName:    "base.tmpl",
			content:     "{{- define \"workload_labels\" -}}\ngcluster.google.com/workload: {{.WorkloadName}}\nslice-scheduler.tpu.google.com/slice-prefix: unwanted\n{{- end -}}",
			numSlice:    1,
			staticSlice: "other-team-2x2x2",
			wantSub:     "JobSet metadata.labels must not set " + k8s.TPUSliceSchedulerPrefixLabel,
		},
		{
			name:     "missing_scheduler_name",
			fileName: "base.tmpl",
			content:  strings.ReplaceAll(podWithSchedAndPort, "  schedulerName: {{.SchedulerName}}\n", ""),
			numSlice: 1,
			wantSub:  `missing required schedulerName "slice-scheduler"`,
		},
		{
			name:     "missing_gang_annotations",
			fileName: "base.tmpl",
			content:  podWithSchedAndPort,
			numSlice: 2,
			wantSub:  "missing required TPU slice gang annotation",
		},
		{
			name:     "missing_pod_labels",
			fileName: "base.tmpl",
			content:  strings.ReplaceAll(podWithSchedAndPort, "{{- if .PodLabels }}\n{{reindent 4 (StructuralData .PodLabels)}}\n{{- end }}\n", ""),
			numSlice: 1,
			wantSub:  "missing required TPU slice pod label",
		},
		{
			name:     "missing_comm_port_8471",
			fileName: "base.tmpl",
			content:  strings.ReplaceAll(podWithSchedAndPort, "    ports:\n    - containerPort: 8471\n", ""),
			numSlice: 1,
			wantSub:  "missing required TPU communication containerPort 8471",
		},
		{
			name:     "forbidden_slice_name_on_dynamic_pod_template",
			fileName: "base.tmpl",
			content:  strings.Replace(podWithSchedAndPort, "    gcluster.google.com/workload: {{.WorkloadName}}", "    gcluster.google.com/workload: {{.WorkloadName}}\n    tpu.google.com/slice-name: stale", 1),
			numSlice: 1,
			wantSub:  "must not set pod label " + k8s.TPUSliceNameLabel,
		},
		{
			name:        "forbidden_slice_prefix_on_static_pod_template",
			fileName:    "base.tmpl",
			content:     strings.Replace(podWithSchedAndPort, "    gcluster.google.com/workload: {{.WorkloadName}}", "    gcluster.google.com/workload: {{.WorkloadName}}\n    tpu.google.com/slice-prefix: stale", 1),
			numSlice:    1,
			staticSlice: "other-team-2x2x2",
			wantSub:     "must not set pod label " + k8s.TPUSlicePrefixLabel,
		},
		{
			name:     "forbidden_scheduler_slice_prefix_on_pod_template",
			fileName: "base.tmpl",
			content:  strings.Replace(podWithSchedAndPort, "    gcluster.google.com/workload: {{.WorkloadName}}", "    gcluster.google.com/workload: {{.WorkloadName}}\n    slice-scheduler.tpu.google.com/slice-prefix: stale", 1),
			numSlice: 1,
			wantSub:  "must not set pod label " + k8s.TPUSliceSchedulerPrefixLabel,
		},
		{
			name:     "forbidden_slice_name_node_selector_on_dynamic_pod_template",
			fileName: "base.tmpl",
			content:  strings.Replace(podWithSchedAndPort, "  restartPolicy: Never", "  nodeSelector:\n    tpu.google.com/slice-name: \"\"\n  restartPolicy: Never", 1),
			numSlice: 1,
			wantSub:  "must not target " + k8s.TPUSliceNameLabel + " in nodeSelector or nodeAffinity",
		},
		{
			name:     "forbidden_slice_name_node_affinity_on_dynamic_pod_template",
			fileName: "base.tmpl",
			content:  strings.Replace(podWithSchedAndPort, "  restartPolicy: Never", "  affinity:\n    nodeAffinity:\n      requiredDuringSchedulingIgnoredDuringExecution:\n        nodeSelectorTerms:\n        - matchExpressions:\n          - key: tpu.google.com/slice-name\n            operator: In\n            values:\n            - s1\n  restartPolicy: Never", 1),
			numSlice: 1,
			wantSub:  "must not target " + k8s.TPUSliceNameLabel + " in nodeSelector or nodeAffinity",
		},
		{
			name:     "forbidden_exclusive_topology_on_dynamic_jobset",
			fileName: "jobset.tmpl",
			content: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: {{.WorkloadName}}
  labels:
{{(StructuralData (.RenderBlock "workload_labels" 4))}}
  annotations:
    alpha.jobset.sigs.k8s.io/exclusive-topology: cloud.google.com/gke-nodepool
spec:
  replicatedJobs:
    - name: main-job
      replicas: {{.NumSlices}}
      template:
        spec:
          completionMode: Indexed
          parallelism: {{.NodesPerSlice}}
          completions: {{.NodesPerSlice}}
          template:
{{(StructuralData (.RenderBlock "pod_template" 12))}}`,
			numSlice: 1,
			wantSub:  "must not set alpha.jobset.sigs.k8s.io/exclusive-topology",
		},
		{
			name:     "forbidden_replicated_job_exclusive_topology_on_dynamic_jobset",
			fileName: "jobset.tmpl",
			content: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: {{.WorkloadName}}
  labels:
{{(StructuralData (.RenderBlock "workload_labels" 4))}}
spec:
  replicatedJobs:
    - name: main-job
      replicas: {{.NumSlices}}
      template:
        metadata:
          annotations:
            alpha.jobset.sigs.k8s.io/exclusive-topology: tpu.google.com/slice-name
        spec:
          completionMode: Indexed
          parallelism: {{.NodesPerSlice}}
          completions: {{.NodesPerSlice}}
          template:
{{(StructuralData (.RenderBlock "pod_template" 12))}}`,
			numSlice: 1,
			wantSub:  "template.metadata.annotations must not set alpha.jobset.sigs.k8s.io/exclusive-topology",
		},
		{
			name:     "missing_replicated_job_template_labels_on_dynamic_jobset",
			fileName: "jobset.tmpl",
			content: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: {{.WorkloadName}}
  labels:
{{(StructuralData (.RenderBlock "workload_labels" 4))}}
spec:
  replicatedJobs:
    - name: main-job
      replicas: {{.NumSlices}}
      template:
        spec:
          completionMode: Indexed
          parallelism: {{.NodesPerSlice}}
          completions: {{.NodesPerSlice}}
          template:
{{(StructuralData (.RenderBlock "pod_template" 12))}}`,
			numSlice: 1,
			wantSub:  "template.metadata.labels is missing required TPU slice label",
		},
		{
			name:     "forbidden_slice_name_on_replicated_job_template_labels",
			fileName: "jobset.tmpl",
			content: `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: {{.WorkloadName}}
  labels:
{{(StructuralData (.RenderBlock "workload_labels" 4))}}
spec:
  replicatedJobs:
    - name: main-job
      replicas: {{.NumSlices}}
      template:
        metadata:
          labels:
            tpu.google.com/type: tpu7x
            tpu.google.com/topology: 2x2x2
            slice-scheduler.tpu.google.com/slice-name: stolen-slice
        spec:
          completionMode: Indexed
          parallelism: {{.NodesPerSlice}}
          completions: {{.NodesPerSlice}}
          template:
{{(StructuralData (.RenderBlock "pod_template" 12))}}`,
			numSlice: 1,
			wantSub:  "template.metadata.labels must not set " + k8s.TPUSliceSchedulerNameLabel,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
			tmplDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmplDir, tc.fileName), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			orc.gkeCustomTemplatesPath = tmplDir
			job := orchestrator.JobDefinition{
				WorkloadName: "stale-tmpl-job",
				ComputeType:  "tpu7x",
				Topology:     "2x2x2",
				NumSlices:    tc.numSlice,
				CommandToRun: "python3 train.py",
			}
			if tc.staticSlice != "" {
				job.NodeConstraint = map[string]string{k8s.TPUSliceNameLabel: tc.staticSlice}
			}
			profile, isDyn, isStatic, err := orc.resolveHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveHardwareRequirements() err = %v", err)
			}
			opts, err := orc.PrepareManifestOptions(job, "registry.local/jax:v1", profile, isDyn, isStatic)
			if err != nil {
				t.Fatalf("PrepareManifestOptions() err = %v", err)
			}
			_, err = orc.GenerateGKEManifest(opts, profile)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) || !strings.Contains(err.Error(), "gke-template-extract") {
				t.Fatalf("GenerateGKEManifest(%s) err = %v, want %q + gke-template-extract hint", tc.name, err, tc.wantSub)
			}
		})
	}
}

func TestInspectClusterAndWorkload_TPUSliceObservability(t *testing.T) {
	t.Run("dynamic_slices_match_legacy_child_and_jobset_owners_and_filter_namespace", func(t *testing.T) {
		sliceListJSON := `{"items":[` +
			`{"metadata":{"name":"my-job-main-job-0","labels":{"tpu.google.com/owner-name":"my-job","tpu.google.com/owner-namespace":"ml","tpu.google.com/owner-kind":"JobSet"}},"spec":{"type":"tpu7x","topology":"2x2x1"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"NotCreated","message":"waiting on capacity"}]}},` +
			`{"metadata":{"name":"my-job-main-job-1","labels":{"tpu.google.com/owner-name":"my-job-main-job-1","tpu.google.com/owner-namespace":"ml","tpu.google.com/owner-kind":"Job"}},"spec":{"type":"tpu7x","topology":"2x2x1","partitionIds":["p1"]},"status":{"conditions":[{"type":"Ready","status":"False","reason":"Provisioning","message":"calibrating links"},{"type":"SliceNodesLabeled","status":"False","reason":"LabelingNodes"}]}},` +
			`{"metadata":{"name":"other-ns-slice","labels":{"tpu.google.com/owner-name":"my-job","tpu.google.com/owner-namespace":"other-ns"}},"spec":{"type":"tpu7x","topology":"2x2x1"}}` +
			`]}`
		eventsJSON := mustMarshalEventList(t,
			corev1.Event{InvolvedObject: corev1.ObjectReference{Name: "other-slice"}, Type: "Warning", Reason: "Failed", Message: "Unrelated event"},
			corev1.Event{InvolvedObject: corev1.ObjectReference{Name: "my-job-main-job-0"}, Reason: "Provisioning", Message: "Waiting for gang capacity"},
		)
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get --raw /apis/" + k8s.TPUSliceAPIGroupVersion:                                                 {{ExitCode: 0, Stdout: "{}"}},
			"kubectl describe jobsets my-job -n ml":                                                                  {{ExitCode: 0, Stdout: "jobset-details"}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout:                                 {{ExitCode: 0, Stdout: sliceListJSON}},
			"kubectl get " + k8s.TPUSliceResource + " -o wide -- my-job-main-job-0 my-job-main-job-1":                {{ExitCode: 0, Stdout: "NAME\nmy-job-main-job-0\nmy-job-main-job-1\n"}},
			"kubectl describe " + k8s.TPUSliceResource + " -- my-job-main-job-0 my-job-main-job-1":                   {{ExitCode: 0, Stdout: "slice-describe"}},
			"kubectl get events -n default --field-selector=involvedObject.kind=Slice -o json " + kubectlReadTimeout: {{ExitCode: 0, Stdout: eventsJSON}},
			"kubectl get pods -l jobset.sigs.k8s.io/jobset-name=my-job -n ml -o wide":                                {{ExitCode: 0, Stdout: "pods-wide"}},
			"kubectl describe pods -l jobset.sigs.k8s.io/jobset-name=my-job -n ml":                                   {{ExitCode: 0, Stdout: "pods-desc"}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"

		var buf strings.Builder
		w := &inspectWriter{writer: &buf, orc: orc}
		if !orc.hasTPUSliceAPI() {
			t.Fatalf("first hasTPUSliceAPI() = false, want true")
		}
		orc.inspectWorkload(w, "my-job", "ml", false, true)
		out := buf.String()
		for _, want := range []string{
			"TPU Slices: Provisioned slices for my-job",
			"my-job-main-job-0",
			"my-job-main-job-1",
			"waiting for TPU capacity / gang completion",
			"Ready=False reason=NotCreated: waiting on capacity",
			"SliceNodesLabeled=False reason=LabelingNodes",
			"Waiting for gang capacity",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("inspectWorkload output missing %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "Unrelated event") || strings.Contains(out, "other-ns-slice") {
			t.Errorf("inspectWorkload leaked unrelated event or other-namespace slice:\n%s", out)
		}
		probeCount := 0
		for _, call := range mockExec.calls {
			if strings.Contains(call, "/apis/"+k8s.TPUSliceAPIGroupVersion) {
				probeCount++
			}
		}
		if probeCount != 1 {
			t.Errorf("expected 1 cached API probe call, got %d (%v)", probeCount, mockExec.calls)
		}
	})

	t.Run("static_slice_reuse_uses_list_cache_and_filters_invalid_names", func(t *testing.T) {
		sliceListJSON := `{"items":[{"metadata":{"name":"static-r13"},"spec":{"type":"tpu7x","topology":"2x2x2","partitionIds":["p1"]},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY"}]}}]}`
		jobSetJSON := `{"spec":{"replicatedJobs":[{"name":"main-job","template":{"spec":{"template":{"spec":{"affinity":{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"tpu.google.com/slice-name","operator":"In","values":["static-r13","static-r14","--bad-flag"]}]}]}}}}}}}}]}}`
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get --raw /apis/" + k8s.TPUSliceAPIGroupVersion:                                                 {{ExitCode: 0, Stdout: "{}"}},
			"kubectl describe jobsets static-job -n ml":                                                              {{ExitCode: 0, Stdout: "jobset-details"}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout:                                 {{ExitCode: 0, Stdout: sliceListJSON}},
			"kubectl get jobsets.jobset.x-k8s.io -n ml -o json " + kubectlReadTimeout + " -- static-job":             {{ExitCode: 0, Stdout: jobSetJSON}},
			"kubectl describe " + k8s.TPUSliceResource + " -- static-r13 static-r14":                                 {{ExitCode: 0, Stdout: "static-slice-details"}},
			"kubectl get events -n default --field-selector=involvedObject.kind=Slice -o json " + kubectlReadTimeout: {{ExitCode: 0, Stdout: `{"items":[]}`}},
			"kubectl get pods -l jobset.sigs.k8s.io/jobset-name=static-job -n ml":                                    {{ExitCode: 0, Stdout: "pods-wide"}},
			"kubectl describe pods -l jobset.sigs.k8s.io/jobset-name=static-job -n ml":                               {{ExitCode: 0, Stdout: "pods-desc"}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"

		var buf strings.Builder
		w := &inspectWriter{writer: &buf, orc: orc}
		orc.inspectWorkload(w, "static-job", "ml", false, true)
		out := buf.String()
		if !strings.Contains(out, "TPU Slice: Static slice(s) static-r13, static-r14 for static-job") || !strings.Contains(out, "Slice static-r13") {
			t.Errorf("inspectWorkload(static affinity) unexpected output:\n%s", out)
		}
		if strings.Contains(out, "--bad-flag") {
			t.Errorf("inspectWorkload leaked invalid slice name --bad-flag:\n%s", out)
		}
	})
}

func TestFetchWorkloadSlices_BatchJobStaticLookupAndIndividualFallback(t *testing.T) {
	t.Run("batch_job_skips_cluster_list_and_uses_targeted_get", func(t *testing.T) {
		sliceJSON := `{"metadata":{"name":"static-job-slice"},"spec":{"type":"tpu7x","topology":"2x2x2","partitionIds":["p0"]},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY"}]}}`
		batchJobJSON := `{"spec":{"template":{"spec":{"nodeSelector":{"tpu.google.com/slice-name":"static-job-slice"}}}}}`
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get job -n ml -o json " + kubectlReadTimeout + " -- shared-name":                         {{ExitCode: 0, Stdout: batchJobJSON}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- static-job-slice": {{ExitCode: 0, Stdout: sliceJSON}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"

		gotSlices, staticNames := orc.fetchWorkloadSlices("ml", "shared-name", true)
		if len(staticNames) != 1 || staticNames[0] != "static-job-slice" || len(gotSlices) != 1 || gotSlices[0].Name != "static-job-slice" {
			t.Fatalf("fetchWorkloadSlices(isBatchJob=true) = (%+v, %v), want static-job-slice only", gotSlices, staticNames)
		}
		for _, call := range mockExec.calls {
			if strings.HasSuffix(call, "get "+k8s.TPUSliceResource+" -o json "+kubectlReadTimeout) {
				t.Errorf("batch/v1 Job must not run cluster-wide Slice LIST: %v", mockExec.calls)
			}
		}
	})

	t.Run("batch_job_non_tpu_makes_zero_slice_calls", func(t *testing.T) {
		cpuJobJSON := `{"spec":{"template":{"spec":{"nodeSelector":{"kubernetes.io/os":"linux"}}}}}`
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get job -n ml -o json " + kubectlReadTimeout + " -- cpu-job": {{ExitCode: 0, Stdout: cpuJobJSON}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"

		gotSlices, staticNames := orc.fetchWorkloadSlices("ml", "cpu-job", true)
		if len(gotSlices) != 0 || len(staticNames) != 0 {
			t.Fatalf("fetchWorkloadSlices(non-TPU batch Job) = (%+v, %v), want nil, nil", gotSlices, staticNames)
		}
		if len(mockExec.calls) != 1 {
			t.Errorf("non-TPU batch Job must issue zero Slice API calls, got %v", mockExec.calls)
		}
	})
}

func TestFetchWorkloadSlices_IndividualFallbackAndForbiddenFiltering(t *testing.T) {
	t.Run("list_failure_falls_back_to_individual_get_and_short_circuits_on_403", func(t *testing.T) {
		sliceJSON := `{"metadata":{"name":"s1"},"spec":{"type":"tpu7x","topology":"2x2x1","partitionIds":["p1"]},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY"}]}}`
		jsJSON := `{"spec":{"replicatedJobs":[{"name":"main-job","template":{"spec":{"template":{"spec":{"affinity":{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"tpu.google.com/slice-name","operator":"In","values":["s1","s2","s3"]}]}]}}}}}}}}]}}`
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout:                   {{ExitCode: 1, Stderr: "list denied"}},
			"kubectl get jobsets.jobset.x-k8s.io -n ml -o json " + kubectlReadTimeout + " -- fallback": {{ExitCode: 0, Stdout: jsJSON}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- s1":        {{ExitCode: 0, Stdout: sliceJSON}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- s2":        {{ExitCode: 1, Stderr: "Error from server (Forbidden): slices is forbidden"}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"

		gotSlices, staticNames := orc.fetchWorkloadSlices("ml", "fallback", false)
		if len(staticNames) != 3 || len(gotSlices) != 1 || gotSlices[0].Name != "s1" {
			t.Fatalf("fetchWorkloadSlices(individual fallback) = (%+v, %v), want [s1] + 3 names", gotSlices, staticNames)
		}
		for _, call := range mockExec.calls {
			if strings.HasSuffix(call, "-- s3") {
				t.Errorf("fetchSlicesIndividually must short-circuit on 403 Forbidden and skip s3: %v", mockExec.calls)
			}
		}
	})

	t.Run("individual_get_ignores_non_rbac_forbidden_strings", func(t *testing.T) {
		s2JSON := `{"metadata":{"name":"s2"},"spec":{"type":"tpu7x","topology":"2x2x1"}}`
		s4JSON := `{"metadata":{"name":"s4"},"spec":{"type":"tpu7x","topology":"2x2x1"}}`
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- forbidden-slice": {{ExitCode: 1, Stderr: `Error from server (NotFound): slices.slice.tpu.google.com "forbidden-slice" not found`}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- s2":              {{ExitCode: 0, Stdout: s2JSON}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- spawn-fail":      {{ExitCode: -1, Stderr: "403 Forbidden"}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- proxy-html":      {{ExitCode: 1, Stderr: "<html><body>403 Forbidden</body></html>"}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout + " -- s4":              {{ExitCode: 0, Stdout: s4JSON}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		got := orc.fetchSlicesIndividually([]string{"forbidden-slice", "s2", "spawn-fail", "proxy-html", "s4"})
		if len(got) != 2 || got[0].Name != "s2" || got[1].Name != "s4" {
			t.Fatalf("fetchSlicesIndividually(non-RBAC errors) = %+v, want [s2, s4]", got)
		}
	})
}

func TestInspectCluster_TPUSliceSectionAndGKEShortCircuit(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "inspect.txt")
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl version --client":                                               {{ExitCode: 0, Stdout: "v1.32"}},
		"kubectl config current-context":                                         {{ExitCode: 0, Stdout: testContext}},
		"kubectl get --raw /version --request-timeout=5s":                        {{ExitCode: 0, Stdout: `{"gitVersion":"v1.32.0"}`}},
		"kubectl get --raw /apis/" + k8s.TPUSliceAPIGroupVersion:                 {{ExitCode: 0, Stdout: "{}"}},
		"kubectl get " + k8s.TPUSliceResource + " -o wide":                       {{ExitCode: 0, Stdout: "NAME\nslice-1\n"}},
		"kubectl get --raw /apis/kueue.x-k8s.io":                                 {{ExitCode: 1, Stderr: "NotFound"}},
		"kubectl get --raw /apis/jobset.x-k8s.io":                                {{ExitCode: 0, Stdout: "{}"}},
		"kubectl get nodes -o wide":                                              {{ExitCode: 0, Stdout: "node-1"}},
		"kubectl describe nodes":                                                 {{ExitCode: 0, Stdout: "node-desc"}},
		"kubectl describe Deployment jobset-controller-manager -n jobset-system": {{ExitCode: 0, Stdout: "js-dep"}},
		"kubectl logs deployment/jobset-controller-manager -n jobset-system":     {{ExitCode: 0, Stdout: "js-logs"}},
		"kubectl get jobsets":                                                    {{ExitCode: 0, Stdout: "js-list"}},
		"kubectl get jobs":                                                       {{ExitCode: 0, Stdout: "jobs-list"}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.mode = orchestrator.TypeK8s
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://10.0.0.1:6443", nil, nil, testContext))
	if err := orc.InspectCluster(orchestrator.InspectOptions{ClusterName: testContext, OutputPath: outFile}); err != nil {
		t.Fatalf("InspectCluster() err = %v", err)
	}
	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "TPU Slices: All Slices") || !strings.Contains(string(raw), "slice-1") {
		t.Errorf("InspectCluster report missing TPU Slices section:\n%s", string(raw))
	}

	gkeExec := NewMockExecutor(nil)
	gkeOrc := newTestGKEOrchestrator(gkeExec)
	gkeOrc.mode = orchestrator.TypeGKE
	gkeOrc.kubeContext = "gke_p_z_c"
	if gkeOrc.hasTPUSliceAPI() || len(gkeExec.calls) != 0 {
		t.Errorf("GKE hasTPUSliceAPI() = true or ran calls %v, want false with 0 calls", gkeExec.calls)
	}
	k8sNegExec := NewMockExecutor(nil)
	k8sNeg := newTestGKEOrchestrator(k8sNegExec)
	k8sNeg.mode = orchestrator.TypeK8s
	k8sNeg.kubeContext = "generic-ctx"
	k8sNeg.inventory = &k8s.Inventory{LabelScheme: k8s.SchemeGeneric, TPUSliceAPIPresent: false}
	if k8sNeg.hasTPUSliceAPI() || len(k8sNegExec.calls) != 0 {
		t.Errorf("negative inventory hasTPUSliceAPI() must return false with 0 calls, got %v", k8sNegExec.calls)
	}
}

func TestGetJobLogs_TPUSliceReadinessDiagnostics(t *testing.T) {
	t.Run("unscheduled_pods_surface_slice_capacity_wait_and_events", func(t *testing.T) {
		sliceJSON := `{"items":[{"metadata":{"name":"wait-job-main-job-0","labels":{"tpu.google.com/owner-name":"wait-job","tpu.google.com/owner-namespace":"ml"}},"spec":{"type":"tpu7x","topology":"2x2x2"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"NotCreated","message":"waiting for partition"}]}}]}`
		eventsJSON := mustMarshalEventList(t, corev1.Event{
			InvolvedObject: corev1.ObjectReference{Name: "wait-job-main-job-0"},
			Type:           "Normal",
			Reason:         "Scheduling",
			Message:        "Waiting for slice partition",
		})
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get --raw /apis/" + k8s.TPUSliceAPIGroupVersion:                                                 {{ExitCode: 0, Stdout: "{}"}},
			"kubectl get pods -n ml -l jobset.sigs.k8s.io/jobset-name=wait-job -o jsonpath=":                         {{ExitCode: 0, Stdout: "wait-job-0\tPending\tFalse\tUnschedulable\t0/2 nodes match\n"}},
			"kubectl get events -n ml --field-selector=involvedObject.kind=Pod":                                      {{ExitCode: 0, Stdout: ""}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout:                                 {{ExitCode: 0, Stdout: sliceJSON}},
			"kubectl get events -n default --field-selector=involvedObject.kind=Slice -o json " + kubectlReadTimeout: {{ExitCode: 0, Stdout: eventsJSON}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"

		msg := orc.checkUnscheduledPods("ml", "wait-job", false)
		for _, want := range []string{"wait-job-0 (Unschedulable:", "waiting for TPU capacity / gang completion", "Ready=False reason=NotCreated: waiting for partition", "Waiting for slice partition"} {
			if !strings.Contains(msg, want) {
				t.Errorf("checkUnscheduledPods() missing %q:\n%s", want, msg)
			}
		}
	})

	t.Run("trailing_tabs_pending_pod_and_terminal_failed_pod", func(t *testing.T) {
		pendingSliceJSON := `{"items":[{"metadata":{"name":"mix-job-main-job-0","labels":{"tpu.google.com/owner-name":"mix-job"}},"spec":{"type":"tpu7x","topology":"2x2x2"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"NotCreated"}]}}]}`
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get --raw /apis/" + k8s.TPUSliceAPIGroupVersion:                                                 {{ExitCode: 0, Stdout: "{}"}},
			"kubectl get pods -n ml -l jobset.sigs.k8s.io/jobset-name=mix-job -o jsonpath=":                          {{ExitCode: 0, Stdout: "mix-job-0\tRunning\tTrue\t\t\nmix-job-1\tPending\t\t\t\n"}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout:                                 {{ExitCode: 0, Stdout: pendingSliceJSON}},
			"kubectl get events -n default --field-selector=involvedObject.kind=Slice -o json " + kubectlReadTimeout: {{ExitCode: 0, Stdout: `{"items":[]}`}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"

		if msg := orc.checkUnscheduledPods("ml", "mix-job", false); !strings.Contains(msg, "mix-job-main-job-0") {
			t.Errorf("mixed Running + Pending(empty PodScheduled) must surface pending slice diagnostics, got %q", msg)
		}

		// Terminal Failed pod with unset PodScheduled must NOT query slices or emit bogus waiting diagnostics.
		failExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get pods -n ml -l jobset.sigs.k8s.io/jobset-name=fail-job -o jsonpath=": {{ExitCode: 0, Stdout: "fail-job-0\tFailed\t\t\t\n"}},
		})
		failOrc := newTestGKEOrchestrator(failExec)
		failOrc.mode = orchestrator.TypeK8s
		failOrc.kubeContext = "tpu-slice-ctx"
		if msg := failOrc.checkUnscheduledPods("ml", "fail-job", false); msg != "" {
			t.Errorf("checkUnscheduledPods(Failed pod with unset PodScheduled) = %q, want empty", msg)
		}
		if len(failExec.calls) != 1 {
			t.Errorf("terminal Failed pod must issue 0 slice queries, got %v", failExec.calls)
		}

		// Scheduled pod pulling an image (Pending + PodScheduled=True) must NOT query slices or emit unscheduled warnings.
		pullExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get pods -n ml -l jobset.sigs.k8s.io/jobset-name=pull-job -o jsonpath=": {{ExitCode: 0, Stdout: "pull-job-0\tPending\tTrue\t\t\n"}},
		})
		pullOrc := newTestGKEOrchestrator(pullExec)
		pullOrc.mode = orchestrator.TypeK8s
		pullOrc.kubeContext = "tpu-slice-ctx"
		if msg := pullOrc.checkUnscheduledPods("ml", "pull-job", false); msg != "" {
			t.Errorf("checkUnscheduledPods(Pending + PodScheduled=True) = %q, want empty", msg)
		}
		if len(pullExec.calls) != 1 {
			t.Errorf("scheduled image-pulling pod must issue 0 slice queries, got %v", pullExec.calls)
		}
	})

	t.Run("wait_for_job_pods_logs_and_times_out_with_pending_slice_diagnostics", func(t *testing.T) {
		origInterval := logsRetryInterval
		logsRetryInterval = time.Millisecond
		t.Cleanup(func() { logsRetryInterval = origInterval })

		sliceJSON := `{"items":[{"metadata":{"name":"pending-job-main-job-0","labels":{"tpu.google.com/owner-name":"pending-job-main-job-0","tpu.google.com/owner-namespace":"ml","tpu.google.com/owner-kind":"Job"}},"spec":{"type":"tpu7x","topology":"2x2x2"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"NotCreated"}]}}]}`
		emptyPods := make([]shell.CommandResult, logsMaxAttempts)
		jobSetPolls := make([]shell.CommandResult, logsMaxAttempts)
		for i := range jobSetPolls {
			jobSetPolls[i] = shell.CommandResult{ExitCode: 0, Stdout: "\tFalse\tmain-job\n1 2\tworkload-container\n"}
		}
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get pods -n ml -l jobset.sigs.k8s.io/jobset-name=pending-job": emptyPods,
			"kubectl get jobsets.jobset.x-k8s.io pending-job -n ml":                jobSetPolls,
			"kubectl get --raw /apis/" + k8s.TPUSliceAPIGroupVersion:               {{ExitCode: 0, Stdout: "{}"}},
			"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout: {
				{ExitCode: 0, Stdout: sliceJSON},
				{ExitCode: 0, Stdout: sliceJSON},
			},
			"kubectl get events -n default --field-selector=involvedObject.kind=Slice -o json " + kubectlReadTimeout: {
				{ExitCode: 0, Stdout: `{"items":[]}`},
				{ExitCode: 0, Stdout: `{"items":[]}`},
			},
		})
		readLogs, restore := logging.CaptureForTesting(t)
		defer restore()

		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = "tpu-slice-ctx"
		orc.addons = addonPlan{resolved: true, jobSetEnabled: true}

		_, _, _, err := orc.waitForJobPods("ml", "pending-job")
		infoLogs, errLogs := readLogs()
		restore()

		if err == nil || !strings.Contains(err.Error(), "TPU slice provisioning is still pending") || !strings.Contains(err.Error(), "pending-job-main-job-0") {
			t.Fatalf("waitForJobPods() err = %v, want pending slice timeout error", err)
		}
		if !strings.Contains(infoLogs+errLogs, "Waiting for job 'pending-job' TPU slice(s) to become ready") {
			t.Errorf("expected initial pending-slice wait log, got:\n%s\n%s", infoLogs, errLogs)
		}
	})
}

func TestFormatPendingSliceDiagnostics_AllReadyReturnsEmpty(t *testing.T) {
	readySlicesJSON := `{"items":[` +
		`{"metadata":{"name":"ready-job-main-job-0","labels":{"tpu.google.com/owner-name":"ready-job","tpu.google.com/owner-namespace":"ml"}},"spec":{"type":"tpu7x","topology":"2x2x2","partitionIds":["p0"]},"status":{"conditions":[{"type":"Ready","status":"True","reason":"SliceReady"}]}},` +
		`{"metadata":{"name":"ready-job-main-job-1","labels":{"tpu.google.com/owner-name":"ready-job","tpu.google.com/owner-namespace":"ml"}},"spec":{"type":"tpu7x","topology":"2x2x2","partitionIds":["p1"]},"status":{"conditions":[{"type":"Ready","status":"True","reason":"SliceReady"}]}}` +
		`]}`
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get --raw /apis/" + k8s.TPUSliceAPIGroupVersion:                 {{ExitCode: 0, Stdout: "{}"}},
		"kubectl get " + k8s.TPUSliceResource + " -o json " + kubectlReadTimeout: {{ExitCode: 0, Stdout: readySlicesJSON}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.mode = orchestrator.TypeK8s
	orc.kubeContext = "tpu-slice-ctx"

	if msg := orc.formatPendingSliceDiagnostics("ml", "ready-job", false); msg != "" {
		t.Errorf("formatPendingSliceDiagnostics(all slices Ready) = %q, want empty", msg)
	}
	if len(mockExec.calls) != 2 {
		t.Errorf("formatPendingSliceDiagnostics(all slices Ready) calls = %v, want only API probe + slice list (no events query)", mockExec.calls)
	}
}

type pollingSliceKubeClient struct {
	MockKubeClient
	t          *testing.T
	listSlices func(gvr schema.GroupVersionResource, labelSelector string) ([]unstructured.Unstructured, error)
}

func (p *pollingSliceKubeClient) ListResources(gvr schema.GroupVersionResource, namespace, labelSelector, fieldSelector string) ([]unstructured.Unstructured, error) {
	if gvr == k8s.TPUSliceGVR || gvr == k8s.TPUSliceV1Beta1GVR {
		if p.t != nil && namespace != "" {
			p.t.Errorf("cluster-scoped Slice ListResources called with non-empty namespace %q", namespace)
		}
		return p.listSlices(gvr, labelSelector)
	}
	return p.MockKubeClient.ListResources(gvr, namespace, labelSelector, fieldSelector)
}

func makeOwnedSliceUnstructured(name, ns, owner string) unstructured.Unstructured {
	labels := map[string]interface{}{k8s.TPUSliceOwnerNameLabel: owner}
	if ns != "" {
		labels[k8s.TPUSliceOwnerNamespaceLabel] = ns
	}
	return unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   name,
			"labels": labels,
		},
		"spec": map[string]interface{}{"type": "tpu7x", "topology": "2x2x2"},
	}}
}

func TestCancelJob_TPUSliceTeardownWait(t *testing.T) {
	t.Run("end_to_end_cancel_waits_for_owned_slices_and_isolates_namespaces", func(t *testing.T) {
		polls := 0
		var sleeps []time.Duration
		otherNsSlice := makeOwnedSliceUnstructured("other-ns-dyn-job-0", "other-ns", "dyn-job")
		dynJS := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "jobset.x-k8s.io/v1alpha2",
			"kind":       "JobSet",
			"metadata": map[string]interface{}{
				"name":      "dyn-job",
				"namespace": "ml",
				"labels":    map[string]interface{}{k8s.TPUSlicePrefixLabel: "dyn-job"},
			},
		}}
		client := &pollingSliceKubeClient{
			MockKubeClient: MockKubeClient{
				Namespace: "ml",
				Objects:   map[string][]unstructured.Unstructured{"jobsets": {dynJS}},
			},
			t: t,
			listSlices: func(_ schema.GroupVersionResource, labelSelector string) ([]unstructured.Unstructured, error) {
				if labelSelector != "" {
					t.Errorf("labelSelector = %q, want empty", labelSelector)
				}
				polls++
				if polls == 1 {
					return []unstructured.Unstructured{
						makeOwnedSliceUnstructured("dyn-job-main-job-0", "ml", "dyn-job-main-job-0"),
						makeOwnedSliceUnstructured("dyn-job-main-job-1", "ml", "dyn-job-main-job-1"),
						otherNsSlice,
					}, nil
				}
				if polls == 2 {
					return []unstructured.Unstructured{
						makeOwnedSliceUnstructured("dyn-job-main-job-1", "ml", "dyn-job-main-job-1"),
						otherNsSlice,
					}, nil
				}
				return []unstructured.Unstructured{otherNsSlice}, nil
			},
		}
		readLogs, restore := logging.CaptureForTesting(t)
		defer restore()

		mockExec := NewMockExecutor(nil)
		orc := newTestGKEOrchestrator(mockExec)
		orc.mode = orchestrator.TypeK8s
		orc.kubeContext = testContext
		orc.k8sCluster = testContext
		orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://10.0.0.1:6443", nil, nil, testContext))
		orc.kubeClient = client
		orc.sliceTeardownTimeout = 120 * time.Second
		orc.sliceTeardownInterval = 5 * time.Second
		orc.sleepFunc = func(d time.Duration) { sleeps = append(sleeps, d) }

		if err := orc.CancelJob("dyn-job", orchestrator.CancelOptions{ClusterName: testContext, GKENamespace: "ml"}); err != nil {
			t.Fatalf("CancelJob() err = %v", err)
		}
		infoLogs, errLogs := readLogs()
		restore()

		if polls != 3 || len(sleeps) != 2 {
			t.Fatalf("polls=%d sleeps=%v, want 3 polls and 2 sleeps", polls, sleeps)
		}
		if len(mockExec.calls) != 0 {
			t.Errorf("CancelJob must run zero subprocesses during slice teardown wait, got %v", mockExec.calls)
		}
		if len(client.Deleted["slices"]) != 0 {
			t.Errorf("gcluster must never delete Slice CRs directly: %v", client.Deleted)
		}
		if strings.Count(infoLogs, "Waiting for TPU slice controller to release optical links...") != 2 {
			t.Errorf("expected 2 wait logs (initial 2 slices + shrunken 1 slice), got:\n%s", infoLogs)
		}
		if !strings.Contains(infoLogs+errLogs, "released successfully") {
			t.Errorf("expected release confirmation log, got:\n%s\n%s", infoLogs, errLogs)
		}
	})

	t.Run("static_reuse_zero_owned_slices_returns_immediately", func(t *testing.T) {
		sleeps := 0
		client := &pollingSliceKubeClient{
			MockKubeClient: MockKubeClient{Namespace: "ml"},
			t:              t,
			listSlices:     func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) { return nil, nil },
		}
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.kubeClient = client
		orc.sleepFunc = func(time.Duration) { sleeps++ }

		orc.waitForOwnedSlicesTeardown("ml", "static-job")
		if sleeps != 0 {
			t.Errorf("sleeps = %d, want 0 when no owned slices exist", sleeps)
		}
	})
}

func TestCancelJob_SkipsSlicePollForNonDynamicJobSets(t *testing.T) {
	makeJS := func(name string, labels map[string]interface{}, finalizers []interface{}, podSpec map[string]interface{}, podLabels map[string]interface{}) unstructured.Unstructured {
		meta := map[string]interface{}{"name": name, "namespace": "ml"}
		if labels != nil {
			meta["labels"] = labels
		}
		if finalizers != nil {
			meta["finalizers"] = finalizers
		}
		tmpl := map[string]interface{}{"spec": podSpec}
		if podLabels != nil {
			tmpl["metadata"] = map[string]interface{}{"labels": podLabels}
		}
		return unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "jobset.x-k8s.io/v1alpha2",
			"kind":       "JobSet",
			"metadata":   meta,
			"spec": map[string]interface{}{
				"replicatedJobs": []interface{}{
					map[string]interface{}{
						"name":     "main-job",
						"template": map[string]interface{}{"spec": map[string]interface{}{"template": tmpl}},
					},
				},
			},
		}}
	}

	cases := []struct {
		name      string
		js        unstructured.Unstructured
		wantPolls int
	}{
		{
			name:      "cpu_jobset_skips_slice_poll_and_403_warning",
			js:        makeJS("cpu-job", map[string]interface{}{"gcluster.google.com/workload": "cpu-job"}, nil, map[string]interface{}{}, nil),
			wantPolls: 0,
		},
		{
			name: "static_reuse_jobset_skips_slice_poll",
			js: makeJS("static-job", map[string]interface{}{"gcluster.google.com/workload": "static-job"}, nil,
				map[string]interface{}{"nodeSelector": map[string]interface{}{k8s.TPUSliceNameLabel: "ready-slice"}},
				map[string]interface{}{k8s.TPUSliceTypeLabel: "tpu7x"}),
			wantPolls: 0,
		},
		{
			name: "static_reuse_with_slice_scheduler_skips_slice_poll",
			js: makeJS("static-sched-job", map[string]interface{}{"gcluster.google.com/workload": "static-sched-job"}, nil,
				map[string]interface{}{
					"schedulerName": k8s.TPUSliceSchedulerName,
					"nodeSelector":  map[string]interface{}{k8s.TPUSliceNameLabel: "ready-slice"},
				},
				map[string]interface{}{k8s.TPUSliceTypeLabel: "tpu7x"}),
			wantPolls: 0,
		},
		{
			name:      "dynamic_slice_prefix_jobset_polls",
			js:        makeJS("dyn-prefix", map[string]interface{}{k8s.TPUSlicePrefixLabel: "dyn-prefix"}, nil, map[string]interface{}{}, nil),
			wantPolls: 1,
		},
		{
			name:      "finalizer_claimed_jobset_polls",
			js:        makeJS("dyn-fin", nil, []interface{}{k8s.TPUSliceSchedulerFinalizer}, map[string]interface{}{}, nil),
			wantPolls: 1,
		},
		{
			name: "legacy_dynamic_jobset_without_static_selector_polls",
			js: makeJS("dyn-legacy", nil, nil,
				map[string]interface{}{"schedulerName": k8s.TPUSliceSchedulerName},
				map[string]interface{}{k8s.TPUSliceTypeLabel: "tpu7x"}),
			wantPolls: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			polls := 0
			client := &pollingSliceKubeClient{
				MockKubeClient: MockKubeClient{
					Namespace: "ml",
					Objects:   map[string][]unstructured.Unstructured{"jobsets": {tc.js}},
				},
				t: t,
				listSlices: func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) {
					polls++
					if tc.wantPolls == 0 {
						return nil, apierrors.NewForbidden(k8s.TPUSliceGVR.GroupResource(), "", errors.New("denied"))
					}
					return nil, nil
				},
			}
			readLogs, restore := logging.CaptureForTesting(t)
			defer restore()

			orc := newTestGKEOrchestrator(NewMockExecutor(nil))
			orc.mode = orchestrator.TypeK8s
			orc.kubeContext = testContext
			orc.k8sCluster = testContext
			orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://10.0.0.1:6443", nil, nil, testContext))
			orc.kubeClient = client

			if err := orc.CancelJob(tc.js.GetName(), orchestrator.CancelOptions{ClusterName: testContext, GKENamespace: "ml"}); err != nil {
				t.Fatalf("CancelJob(%s) err = %v", tc.name, err)
			}
			infoLogs, errLogs := readLogs()
			restore()

			if polls != tc.wantPolls {
				t.Errorf("%s: slice polls = %d, want %d", tc.name, polls, tc.wantPolls)
			}
			if tc.wantPolls == 0 && strings.Contains(infoLogs+errLogs, "Insufficient RBAC permissions") {
				t.Errorf("%s: unexpected 403 RBAC warning:\n%s\n%s", tc.name, infoLogs, errLogs)
			}
		})
	}
}

func TestWaitForOwnedSlicesTeardown_ErrorsAndTimeout(t *testing.T) {
	t.Run("transient_error_on_zero_slice_job_does_not_log_false_release", func(t *testing.T) {
		polls, sleeps := 0, 0
		client := &pollingSliceKubeClient{
			MockKubeClient: MockKubeClient{Namespace: "ml"},
			t:              t,
			listSlices: func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) {
				polls++
				if polls == 1 {
					return nil, errors.New("503 service unavailable")
				}
				return nil, nil
			},
		}
		readLogs, restore := logging.CaptureForTesting(t)
		defer restore()

		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.kubeClient = client
		orc.sliceTeardownTimeout = 30 * time.Second
		orc.sliceTeardownInterval = 5 * time.Second
		orc.sleepFunc = func(time.Duration) { sleeps++ }

		orc.waitForOwnedSlicesTeardown("ml", "flaky-job")
		infoLogs, errLogs := readLogs()
		restore()

		if polls != 2 || sleeps != 1 {
			t.Fatalf("polls=%d sleeps=%d, want 2 polls and 1 sleep", polls, sleeps)
		}
		if !strings.Contains(infoLogs+errLogs, "Transient error polling TPU Slice teardown") {
			t.Errorf("missing transient warning:\n%s\n%s", infoLogs, errLogs)
		}
		if strings.Contains(infoLogs+errLogs, "released successfully") {
			t.Errorf("must not log 'released successfully' when 0 slices were ever observed:\n%s\n%s", infoLogs, errLogs)
		}
	})

	t.Run("persistent_transient_error_times_out_cleanly", func(t *testing.T) {
		var totalSlept time.Duration
		client := &pollingSliceKubeClient{
			MockKubeClient: MockKubeClient{Namespace: "ml"},
			t:              t,
			listSlices: func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) {
				return nil, errors.New("503 down")
			},
		}
		readLogs, restore := logging.CaptureForTesting(t)
		defer restore()

		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.kubeClient = client
		orc.sliceTeardownTimeout = 15 * time.Second
		orc.sliceTeardownInterval = 5 * time.Second
		orc.sleepFunc = func(d time.Duration) { totalSlept += d }

		orc.waitForOwnedSlicesTeardown("ml", "down-job")
		infoLogs, errLogs := readLogs()
		restore()

		if totalSlept != 15*time.Second || !strings.Contains(infoLogs+errLogs, "Timed out after 15s polling TPU Slice teardown") {
			t.Errorf("slept=%v logs=%q/%q, want 15s timeout warning", totalSlept, infoLogs, errLogs)
		}
	})

	t.Run("times_out_after_120s_throttles_logs_and_omits_final_waiting_line", func(t *testing.T) {
		var totalSlept time.Duration
		client := &pollingSliceKubeClient{
			MockKubeClient: MockKubeClient{Namespace: "ml"},
			t:              t,
			listSlices: func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) {
				return []unstructured.Unstructured{makeOwnedSliceUnstructured("stuck-slice", "ml", "stuck-job")}, nil
			},
		}
		readLogs, restore := logging.CaptureForTesting(t)
		defer restore()

		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.kubeClient = client
		orc.sliceTeardownTimeout = 120 * time.Second
		orc.sliceTeardownInterval = 5 * time.Second
		orc.sleepFunc = func(d time.Duration) { totalSlept += d }

		orc.waitForOwnedSlicesTeardown("ml", "stuck-job")
		infoLogs, errLogs := readLogs()
		restore()

		if totalSlept != 120*time.Second {
			t.Errorf("totalSlept = %v, want 120s", totalSlept)
		}
		waitLines := strings.Count(infoLogs, "Waiting for TPU slice controller to release optical links...")
		if waitLines != 4 {
			t.Errorf("expected exactly 4 throttled wait logs (0s, 30s, 60s, 90s; omitting 120s), got %d:\n%s", waitLines, infoLogs)
		}
		if !strings.Contains(infoLogs+errLogs, "Timed out after 2m0s waiting for TPU Slice(s) (stuck-slice)") {
			t.Errorf("expected timeout warning, got:\n%s\n%s", infoLogs, errLogs)
		}
	})
}

func TestWaitForOwnedSlicesTeardown_FatalAndSchemeOverride(t *testing.T) {
	t.Run("forbidden_warns_and_nomatch_or_notfound_is_silent", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			err      error
			wantWarn bool
		}{
			{name: "forbidden", err: apierrors.NewForbidden(k8s.TPUSliceGVR.GroupResource(), "", errors.New("denied")), wantWarn: true},
			{name: "nomatch", err: &meta.NoResourceMatchError{PartialResource: k8s.TPUSliceGVR}, wantWarn: false},
			{name: "notfound", err: apierrors.NewNotFound(k8s.TPUSliceGVR.GroupResource(), ""), wantWarn: false},
		} {
			sleeps := 0
			client := &pollingSliceKubeClient{
				MockKubeClient: MockKubeClient{Namespace: "ml"},
				t:              t,
				listSlices:     func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) { return nil, tc.err },
			}
			readLogs, restore := logging.CaptureForTesting(t)
			orc := newTestGKEOrchestrator(NewMockExecutor(nil))
			orc.mode = orchestrator.TypeK8s
			orc.kubeClient = client
			orc.sleepFunc = func(time.Duration) { sleeps++ }
			orc.waitForOwnedSlicesTeardown("ml", "err-job")
			infoLogs, errLogs := readLogs()
			restore()

			if sleeps != 0 {
				t.Errorf("%s: sleeps = %d, want 0", tc.name, sleeps)
			}
			if tc.wantWarn {
				if !strings.Contains(infoLogs+errLogs, "Insufficient RBAC permissions") {
					t.Errorf("%s: expected RBAC warning, got (%q / %q)", tc.name, infoLogs, errLogs)
				}
			} else if infoLogs != "" || errLogs != "" {
				t.Errorf("%s: expected silent return, got (%q / %q)", tc.name, infoLogs, errLogs)
			}
		}
	})

	t.Run("generic_scheme_override_skips_teardown_polling", func(t *testing.T) {
		t.Setenv(k8s.LabelSchemeEnvVar, "generic")
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.kubeClient = &MockKubeClient{Namespace: "ml"}
		if orc.canPollTPUSliceTeardown("any-job") {
			t.Errorf("canPollTPUSliceTeardown() = true under GCLUSTER_LABEL_SCHEME=generic, want false")
		}
	})
}

func TestSubmitJob_TypeK8sMultiSliceDryRun(t *testing.T) {
	orc, mockExec := makeTPUSliceK8sSubmitOrchestrator(t, false)
	outPath := filepath.Join(t.TempDir(), "multislice.yaml")
	err := orc.SubmitJob(orchestrator.JobDefinition{
		WorkloadName:          "ms-job",
		ClusterName:           "tpu-slice-cluster",
		GKENamespace:          "ml-ns",
		Topology:              "2x2x1",
		ImageName:             "registry.local/jax:v1",
		CommandToRun:          "python3 train.py",
		NumSlices:             2,
		UseParallelContainers: true,
		DryRunManifest:        outPath,
	})
	if err != nil {
		t.Fatalf("SubmitJob(multi-slice) err = %v", err)
	}
	assertZeroGCloudOrKueueSlicingCalls(t, mockExec.calls)
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	js := parseRenderedJobSet(t, string(raw))
	if js.Labels[k8s.TPUSlicePrefixLabel] != "ms-job" {
		t.Errorf("multi-slice JobSet labels = %v, want %s=ms-job", js.Labels, k8s.TPUSlicePrefixLabel)
	}
	podTmpl := js.Spec.ReplicatedJobs[0].Template.Spec.Template
	if podTmpl.Labels[k8s.TPUSliceTopologyLabel] != "2x2x1" || podTmpl.Labels[k8s.TPUSliceTypeLabel] != "tpu7x" {
		t.Errorf("multi-slice pod labels = %v, want type=tpu7x topology=2x2x1", podTmpl.Labels)
	}
	if _, hasPrefix := podTmpl.Labels[k8s.TPUSlicePrefixLabel]; hasPrefix {
		t.Errorf("multi-slice pod must not set %s: %v", k8s.TPUSlicePrefixLabel, podTmpl.Labels)
	}
	if podTmpl.Spec.SchedulerName != k8s.TPUSliceSchedulerName {
		t.Errorf("multi-slice schedulerName = %q, want %q", podTmpl.Spec.SchedulerName, k8s.TPUSliceSchedulerName)
	}
	if podTmpl.Annotations[k8s.TPUNumSlicesAnnotation] != "2" || podTmpl.Annotations[k8s.TPUSliceGroupNameAnnotation] != "ms-job" {
		t.Errorf("multi-slice annotations = %v, want num-slices=2 slice-group-name=ms-job", podTmpl.Annotations)
	}
	if len(podTmpl.Spec.Containers) != 1 || len(podTmpl.Spec.Containers[0].Ports) != 1 || podTmpl.Spec.Containers[0].Ports[0].ContainerPort != 8471 {
		t.Errorf("expected 1 container with port 8471 in TypeK8s mode, got %+v", podTmpl.Spec.Containers)
	}
}

func TestCancelJob_GetForbiddenStillPollsOwnedSlices(t *testing.T) {
	polls := 0
	client := &pollingSliceKubeClient{
		MockKubeClient: MockKubeClient{
			Namespace: "ml",
			GetErr:    apierrors.NewForbidden(schema.GroupResource{Group: "jobset.x-k8s.io", Resource: "jobsets"}, "dyn-job", errors.New("denied")),
		},
		t: t,
		listSlices: func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) {
			polls++
			return nil, nil
		},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.mode = orchestrator.TypeK8s
	orc.kubeContext = testContext
	orc.k8sCluster = testContext
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://10.0.0.1:6443", nil, nil, testContext))
	orc.kubeClient = client
	if err := orc.CancelJob("dyn-job", orchestrator.CancelOptions{ClusterName: testContext, GKENamespace: "ml"}); err != nil {
		t.Fatalf("CancelJob(get 403) err = %v", err)
	}
	if polls != 1 {
		t.Errorf("CancelJob(get 403) slice polls = %d, want 1", polls)
	}
}

func TestTPUSlice_BlankSliceNameConstraintRejected(t *testing.T) {
	for _, rawVal := range []string{"", "   "} {
		orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
		orc.inventory.Pools["cpu"] = k8s.PoolCapacity{MachineType: "n2-standard-8", AllocatableCPUMilli: 8000}
		orc.inventory.PoolNames = append(orc.inventory.PoolNames, "cpu")
		for _, ct := range []string{"tpu7x", "bm-host", "n2-standard-8"} {
			job := orchestrator.JobDefinition{
				WorkloadName:   "blank-slice",
				ComputeType:    ct,
				Topology:       "2x2x2",
				NumSlices:      1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: rawVal},
			}
			_, _, _, err := orc.resolveHardwareRequirements(&job)
			if err == nil || !strings.Contains(err.Error(), "value must not be empty") {
				t.Errorf("resolveHardwareRequirements(ct=%q, slice=%q) err = %v, want empty value error", ct, rawVal, err)
			}
		}
	}
}

func TestTPUSlice_ExplicitModeAndMissingSliceCustomMachineType(t *testing.T) {
	t.Run("explicit_mode_slice_inventory_unavailable_custom_machine_resolves_as_tpu", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.addons = addonPlan{resolved: true, jobSetEnabled: true}
		orc.inventory = &k8s.Inventory{
			LabelScheme:               k8s.SchemeTPUSlice,
			TPUSliceAPIPresent:        true,
			ExplicitMode:              true,
			SliceInventoryUnavailable: true,
		}
		job := orchestrator.JobDefinition{
			WorkloadName:   "bm-explicit",
			ComputeType:    "custom-bm-tpu",
			Topology:       "2x2x2",
			NumSlices:      1,
			NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "my-slice"},
		}
		profile, _, _, err := orc.resolveHardwareRequirements(&job)
		if err != nil {
			t.Fatalf("resolveHardwareRequirements(ExplicitMode custom-bm-tpu) err = %v", err)
		}
		if !profile.IsTPU {
			t.Errorf("profile.IsTPU = false, want true for custom-bm-tpu with slice-name constraint")
		}
	})

	t.Run("missing_slice_with_custom_machine_fails_instead_of_cpu_fallback", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.addons = addonPlan{resolved: true, jobSetEnabled: true}
		orc.inventory = &k8s.Inventory{
			LabelScheme:        k8s.SchemeTPUSlice,
			TPUSliceAPIPresent: true,
			Slices:             []k8s.SliceInfo{{Name: "real-slice", TPUType: "tpu-custom", Topology: "2x2x2", Ready: true, State: "READY"}},
		}
		job := orchestrator.JobDefinition{
			WorkloadName:   "bm-missing",
			ComputeType:    "custom-bm-tpu",
			NumSlices:      1,
			NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "missing-slice"},
		}
		_, _, _, err := orc.resolveHardwareRequirements(&job)
		if err == nil || !strings.Contains(err.Error(), `TPU slice "missing-slice" does not exist`) {
			t.Fatalf("resolveHardwareRequirements(missing slice custom-bm-tpu) err = %v, want missing slice error", err)
		}
	})
}

func TestTPUSlice_IdleBareMetalPoolBackfilledTypeAndMissingTopoHint(t *testing.T) {
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.mode = orchestrator.TypeK8s
	orc.addons = addonPlan{resolved: true, jobSetEnabled: true}
	orc.inventory = &k8s.Inventory{
		LabelScheme:        k8s.SchemeTPUSlice,
		TPUSliceAPIPresent: true,
		TPU: &k8s.TPUInventory{
			TPUType:      "tpu-custom",
			MachineType:  "bm-host",
			ChipsPerNode: 4,
			Topologies:   []string{"2x2x2", "2x2x4"},
		},
		PoolNames: []string{"bm-host"},
		Pools: map[string]k8s.PoolCapacity{
			"bm-host": {MachineType: "bm-host", IsTPU: true, TPUType: "", TPUsPerNode: 4, TotalNodes: 4, ReadyNodes: 4},
		},
		Slices: []k8s.SliceInfo{
			{Name: "unlabeled-custom", TPUType: "tpu-custom", ChipsPerNode: 4, Ready: true, NodesLabeledKnown: true, NodesLabeled: false},
			{Name: "wrong-chips-custom", TPUType: "tpu-custom", ChipsPerNode: 1, Ready: true, NodesLabeledKnown: true, NodesLabeled: true},
			{Name: "ready-custom-4t", TPUType: "tpu-custom", ChipsPerNode: 4, Ready: true, NodesLabeledKnown: true, NodesLabeled: true},
		},
	}

	dynJob := orchestrator.JobDefinition{
		WorkloadName: "bm-dyn",
		ComputeType:  "bm-host",
		Topology:     "2x2x2",
		NumSlices:    1,
		CommandToRun: "python3 train.py",
	}
	js := renderTPUSliceJobSetForTest(t, orc, &dynJob)
	if got := js.Spec.ReplicatedJobs[0].Template.Spec.Template.Labels[k8s.TPUSliceTypeLabel]; got != "tpu-custom" {
		t.Errorf("idle bare-metal pool tpu.google.com/type = %q, want tpu-custom (from PartitionTree-backfilled inv.TPU)", got)
	}

	noTopo := orchestrator.JobDefinition{WorkloadName: "bm-notopo", ComputeType: "bm-host", NumSlices: 1}
	_, _, _, err := orc.resolveHardwareRequirements(&noTopo)
	if err == nil {
		t.Fatalf("expected missing --topology error for bm-host")
	}
	for _, want := range []string{"tpu-custom nodes with 4 chips each", "2x2x2, 2x2x4", k8s.TPUSliceNameLabel + "=ready-custom-4t"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing --topology error missing %q:\n%s", want, err.Error())
		}
	}

	// Same-gen mixed chip-count: 1t pool without topologies must not bleed 4t primary topologies or 4t slice hints.
	orc.inventory.Pools["bm-1t"] = k8s.PoolCapacity{MachineType: "bm-1t", IsTPU: true, TPUType: "tpu-custom", TPUsPerNode: 1, TotalNodes: 2, ReadyNodes: 2}
	orc.inventory.PoolNames = append(orc.inventory.PoolNames, "bm-1t")
	noTopo1t := orchestrator.JobDefinition{WorkloadName: "bm-1t-notopo", ComputeType: "bm-1t", NumSlices: 1}
	_, _, _, err1t := orc.resolveHardwareRequirements(&noTopo1t)
	if err1t == nil || strings.Contains(err1t.Error(), "2x2x2") || !strings.Contains(err1t.Error(), k8s.TPUSliceNameLabel+"=wrong-chips-custom") {
		t.Errorf("1t mixed-chip missing --topology err = %v, want no 4t shapes and 1t slice hint", err1t)
	}
}

func TestTPUSlice_ExplicitModeNonGCETPUTypes(t *testing.T) {
	for _, ct := range []string{"tpu-custom-a", "tpu-custom-b", "tpu0-custom", "ct0x-standard-4t"} {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.addons = addonPlan{resolved: true, jobSetEnabled: true}
		orc.inventory = &k8s.Inventory{
			LabelScheme:        k8s.SchemeTPUSlice,
			TPUSliceAPIPresent: true,
			ExplicitMode:       true,
		}
		job := orchestrator.JobDefinition{
			WorkloadName: "exp-" + ct,
			ComputeType:  ct,
			Topology:     "2x2x2",
			NumSlices:    1,
		}
		if !orc.shouldBypassMissingKueue(&job, kueueAddonProbe{state: addonStateMissing}) {
			t.Errorf("shouldBypassMissingKueue(%s) = false, want true on SchemeTPUSlice ExplicitMode", ct)
		}
		profile, _, _, err := orc.resolveHardwareRequirements(&job)
		if err != nil {
			t.Fatalf("resolveHardwareRequirements(%s ExplicitMode) err = %v", ct, err)
		}
		if !profile.IsTPU || job.NodesPerSlice != 2 {
			t.Errorf("%s ExplicitMode = (IsTPU=%v, NodesPerSlice=%d), want (true, 2)", ct, profile.IsTPU, job.NodesPerSlice)
		}
	}

	// Non-TPU machine names containing "tpu" or starting with "ct" must NOT be misclassified as TPU in ExplicitMode.
	for _, tc := range []struct {
		computeType string
		wantCPU     bool
	}{
		{"output-worker", true},
		{"output-gpu-worker", false},
		{"ct-worker", true},
		{"ct-standard-16", true},
	} {
		orc := newTestGKEOrchestrator(NewMockExecutor(nil))
		orc.mode = orchestrator.TypeK8s
		orc.addons = addonPlan{resolved: true, jobSetEnabled: true}
		orc.inventory = &k8s.Inventory{
			LabelScheme:        k8s.SchemeTPUSlice,
			TPUSliceAPIPresent: true,
			ExplicitMode:       true,
		}
		job := orchestrator.JobDefinition{
			WorkloadName:      "exp-non-tpu",
			ComputeType:       tc.computeType,
			NumNodesSpecified: true,
			NodesPerSlice:     2,
			NumSlices:         1,
		}
		if orc.shouldBypassMissingKueue(&job, kueueAddonProbe{state: addonStateMissing}) {
			t.Errorf("shouldBypassMissingKueue(%s) = true, want false for non-TPU machine in ExplicitMode", tc.computeType)
		}
		profile, _, _, err := orc.resolveHardwareRequirements(&job)
		if err != nil {
			t.Fatalf("resolveHardwareRequirements(%s ExplicitMode --num-nodes 2) err = %v", tc.computeType, err)
		}
		if profile.IsTPU || profile.IsCPUMachine != tc.wantCPU {
			t.Errorf("%s ExplicitMode profile = %+v, want IsTPU=false IsCPUMachine=%v", tc.computeType, profile, tc.wantCPU)
		}
	}
}

func TestTPUSlice_StaticReuseRejectsUnlabeledNodesAndProvesNodeCountAuthority(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	orc.inventory.Slices = append(orc.inventory.Slices,
		k8s.SliceInfo{Name: "unlabeled-reason", TPUType: "tpu7x", MachineType: "tpu7x-standard-4t", Topology: "2x2x2", Ready: true, State: "READY", NodesLabeledKnown: true, NodesLabeled: false, NodesLabeledReason: "LabelingNodes", NodesPerSlice: 2, ChipsPerNode: 4},
		k8s.SliceInfo{Name: "unlabeled-default", TPUType: "tpu7x", MachineType: "tpu7x-standard-4t", Topology: "2x2x2", Ready: true, State: "READY", NodesLabeledKnown: true, NodesLabeled: false, NodesPerSlice: 2, ChipsPerNode: 4},
		k8s.SliceInfo{Name: "auth-nodes-4", TPUType: "tpu7x", MachineType: "tpu7x-standard-4t", Topology: "2x2x2", Ready: true, State: "READY", NodesPerSlice: 4, ChipsPerNode: 4},
	)

	for _, tc := range []struct {
		slice, wantState string
	}{
		{"unlabeled-reason", "LabelingNodes"},
		{"unlabeled-default", "SliceNodesLabeled=False"},
	} {
		job := orchestrator.JobDefinition{
			WorkloadName:   "unlabeled-reuse",
			NumSlices:      1,
			NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: tc.slice},
		}
		if _, _, _, err := orc.resolveHardwareRequirements(&job); err == nil || !strings.Contains(err.Error(), tc.wantState) {
			t.Errorf("static reuse(%s) err = %v, want rejection containing %q", tc.slice, err, tc.wantState)
		}
	}

	// SliceInfo.NodesPerSlice (4) differs from 2x2x2/4-chips topology math (2) and must be authoritative.
	authJob := orchestrator.JobDefinition{
		WorkloadName:   "auth-reuse",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "auth-nodes-4"},
	}
	if _, _, _, err := orc.resolveHardwareRequirements(&authJob); err != nil {
		t.Fatalf("resolveHardwareRequirements(auth-nodes-4) err = %v", err)
	}
	if authJob.NodesPerSlice != 4 {
		t.Errorf("authJob.NodesPerSlice = %d, want authoritative SliceInfo.NodesPerSlice=4", authJob.NodesPerSlice)
	}

	// Multi-slice static reuse rejects slices with mismatched NodesPerSlice.
	mismatchJob := orchestrator.JobDefinition{
		WorkloadName:   "mismatch-reuse",
		NumSlices:      2,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "other-team-2x2x2|auth-nodes-4"},
	}
	if _, _, _, err := orc.resolveHardwareRequirements(&mismatchJob); err == nil || !strings.Contains(err.Error(), "does not match slice") {
		t.Errorf("multi-slice mismatched NodesPerSlice err = %v, want node-count mismatch error", err)
	}
}

func TestHasTPUSliceAPI_RespectsEnvSchemeOverride(t *testing.T) {
	for _, tc := range []struct {
		scheme string
		want   bool
	}{
		{scheme: "generic", want: false},
		{scheme: "gke", want: false},
		{scheme: "tpu-slice", want: true},
	} {
		t.Run(tc.scheme, func(t *testing.T) {
			t.Setenv(k8s.LabelSchemeEnvVar, tc.scheme)
			mockExec := NewMockExecutor(nil)
			orc := newTestGKEOrchestrator(mockExec)
			orc.mode = orchestrator.TypeK8s
			orc.kubeContext = "tpu-slice-ctx"
			if got := orc.hasTPUSliceAPI(); got != tc.want || len(mockExec.calls) != 0 {
				t.Errorf("hasTPUSliceAPI(override=%s) = (%v, calls=%v), want (%v, 0 calls)", tc.scheme, got, mockExec.calls, tc.want)
			}
		})
	}
}

func TestTPUSlice_LongWorkloadNameFailsBeforeImageBuild(t *testing.T) {
	orc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	longJob := orchestrator.JobDefinition{
		WorkloadName: strings.Repeat("a", maxTPUSlicePrefixLen+1),
		ComputeType:  "tpu7x",
		Topology:     "2x2x2",
		NumSlices:    1,
	}
	_, _, _, err := orc.resolveHardwareRequirements(&longJob)
	if err == nil || !strings.Contains(err.Error(), "35-character limit") {
		t.Fatalf("resolveHardwareRequirements(36-char dynamic workload) err = %v, want pre-image-build 35-char error", err)
	}
}

func TestCancelJob_TypeGKE403NeverPollsTPUSlices(t *testing.T) {
	polls := 0
	client := &pollingSliceKubeClient{
		MockKubeClient: MockKubeClient{
			Namespace: "default",
			GetErr:    apierrors.NewForbidden(schema.GroupResource{Group: "jobset.x-k8s.io", Resource: "jobsets"}, "gke-job", errors.New("denied")),
		},
		t: t,
		listSlices: func(schema.GroupVersionResource, string) ([]unstructured.Unstructured, error) {
			polls++
			return nil, nil
		},
	}
	orc := cancelTestOrchestrator(client)
	orc.mode = orchestrator.TypeGKE
	if err := orc.CancelJob("gke-job", cancelOpts); err != nil {
		t.Fatalf("CancelJob(TypeGKE 403) err = %v", err)
	}
	if polls != 0 {
		t.Errorf("CancelJob(TypeGKE 403) slice polls = %d, want 0", polls)
	}
}

func TestTPUSlice_ForbidsSchedulerSliceNameLabel(t *testing.T) {
	contract := tpuSliceManifestContract{
		workloadLabelsYAML: k8s.TPUSlicePrefixLabel + ": dyn-job",
		podLabelsYAML:      k8s.TPUSliceTypeLabel + ": tpu7x\n" + k8s.TPUSliceTopologyLabel + ": 2x2x1",
		commPortEnabled:    true,
		dynamicTPUSlice:    true,
		schedulerName:      k8s.TPUSliceSchedulerName,
	}
	jobSetWithSchedSliceName := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: dyn-job
  labels:
    tpu.google.com/slice-prefix: dyn-job
    slice-scheduler.tpu.google.com/slice-name: stolen-slice
spec:
  replicatedJobs:
  - name: main-job
    template:
      spec:
        template:
          metadata:
            labels:
              tpu.google.com/type: tpu7x
              tpu.google.com/topology: 2x2x1
          spec:
            schedulerName: slice-scheduler
            containers:
            - name: workload-container
              ports:
              - containerPort: 8471
`
	if err := validateTPUSliceManifest(jobSetWithSchedSliceName, contract, "hint"); err == nil || !strings.Contains(err.Error(), k8s.TPUSliceSchedulerNameLabel) {
		t.Errorf("validateTPUSliceManifest(JobSet scheduler slice-name) err = %v, want %s error", err, k8s.TPUSliceSchedulerNameLabel)
	}

	podWithSchedSliceName := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: dyn-job
  labels:
    tpu.google.com/slice-prefix: dyn-job
spec:
  replicatedJobs:
  - name: main-job
    template:
      metadata:
        labels:
          tpu.google.com/type: tpu7x
          tpu.google.com/topology: 2x2x1
      spec:
        template:
          metadata:
            labels:
              tpu.google.com/type: tpu7x
              tpu.google.com/topology: 2x2x1
              slice-scheduler.tpu.google.com/slice-name: stolen-slice
          spec:
            schedulerName: slice-scheduler
            containers:
            - name: workload-container
              ports:
              - containerPort: 8471
`
	if err := validateTPUSliceManifest(podWithSchedSliceName, contract, "hint"); err == nil || !strings.Contains(err.Error(), k8s.TPUSliceSchedulerNameLabel) {
		t.Errorf("validateTPUSliceManifest(Pod scheduler slice-name) err = %v, want %s error", err, k8s.TPUSliceSchedulerNameLabel)
	}
}

func TestTPUSlice_MissingTopologyErrorGatingAndMultiSliceHint(t *testing.T) {
	t.Setenv(k8s.LabelSchemeEnvVar, string(k8s.SchemeTPUSlice))
	nilInvOrc := newTestGKEOrchestrator(NewMockExecutor(nil))
	nilInvOrc.mode = orchestrator.TypeK8s
	nilInvOrc.addons = addonPlan{resolved: true, jobSetEnabled: true}
	errNil := nilInvOrc.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x-standard-4t", MachineType: "tpu7x-standard-4t", NumSlices: 1})
	if strings.Contains(errNil.Error(), "Cluster has") || !strings.Contains(errNil.Error(), k8s.TPUSliceNameLabel+"=<slice-name>") {
		t.Errorf("nil-inventory missing topology err = %q, want no 'Cluster has' and <slice-name> hint", errNil)
	}

	v6eOrc, _ := makeTPUSlicePipelineOrchestrator("v6e", "ct6e-standard-4t")
	v6eOrc.inventory.Slices = []k8s.SliceInfo{{Name: "v6e-ready", TPUType: "v6e", ChipsPerNode: 4, Ready: true}}
	errUnmatched := v6eOrc.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x-standard-4t", MachineType: "tpu7x-standard-4t", NumSlices: 1})
	if strings.Contains(errUnmatched.Error(), "Cluster has") || strings.Contains(errUnmatched.Error(), "v6e-ready") {
		t.Errorf("unmatched-family missing topology err = %q, want no 'Cluster has' and no v6e-ready hint", errUnmatched)
	}

	multiOrc, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	multiOrc.inventory.Slices = []k8s.SliceInfo{
		{Name: "slice-a", TPUType: "tpu7x", ChipsPerNode: 4, Topology: "2x2x1", NodesPerSlice: 1, Ready: true},
		{Name: "slice-b", TPUType: "tpu7x", ChipsPerNode: 4, Topology: "2x2x1", NodesPerSlice: 1, Ready: true},
	}
	errMulti := multiOrc.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 2})
	if !strings.Contains(errMulti.Error(), k8s.TPUSliceNameLabel+"=slice-a|slice-b") {
		t.Errorf("2-slice missing topology err = %q, want slice-a|slice-b hint", errMulti)
	}
	errThree := multiOrc.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 3})
	if !strings.Contains(errThree.Error(), k8s.TPUSliceNameLabel+"=<slice-1>|...|<slice-3>") {
		t.Errorf("3-slice missing topology err = %q, want <slice-1>|...|<slice-3> hint", errThree)
	}
	multiOrc.inventory.Slices = multiOrc.inventory.Slices[:1]
	errTwoPlaceholder := multiOrc.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 2})
	if !strings.Contains(errTwoPlaceholder.Error(), k8s.TPUSliceNameLabel+"=<slice-1>|<slice-2>") {
		t.Errorf("2-slice with 1 ready slice err = %q, want <slice-1>|<slice-2> hint", errTwoPlaceholder)
	}
}

func TestTPUSlice_OmittedTopologyMultiHostSliceAccepts1tAnd8t(t *testing.T) {
	s6e, err := k8s.ParseTPUSliceJSON(`{"metadata":{"name":"s-6e-8t"},"spec":{"type":"v6e"},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY"}]}}`, defaultTPUHardwareResolver)
	if err != nil {
		t.Fatalf("ParseTPUSliceJSON(v6e) err = %v", err)
	}
	s7x, err := k8s.ParseTPUSliceJSON(`{"metadata":{"name":"s-7x-1t"},"spec":{"type":"tpu7x"},"status":{"conditions":[{"type":"Ready","status":"True","reason":"READY"}]}}`, defaultTPUHardwareResolver)
	if err != nil {
		t.Fatalf("ParseTPUSliceJSON(tpu7x) err = %v", err)
	}
	s6e.NodesPerSlice = 1
	s7x.NodesPerSlice = 1

	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.mode = orchestrator.TypeK8s
	orc.addons = addonPlan{resolved: true, jobSetEnabled: true}
	orc.inventory = &k8s.Inventory{
		LabelScheme:        k8s.SchemeTPUSlice,
		TPUSliceAPIPresent: true,
		Slices:             []k8s.SliceInfo{s6e, s7x},
	}

	job8t := orchestrator.JobDefinition{
		WorkloadName:   "job-v6e-8t",
		ComputeType:    "ct6e-standard-8t",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-6e-8t"},
	}
	if _, _, _, err := orc.resolveHardwareRequirements(&job8t); err != nil || job8t.Topology != "2x4" {
		t.Fatalf("resolveHardwareRequirements(ct6e-standard-8t on omitted-topo slice) = (topo=%q, err=%v), want (2x4, nil)", job8t.Topology, err)
	}

	job1t := orchestrator.JobDefinition{
		WorkloadName:   "job-7x-1t",
		ComputeType:    "tpu7x-standard-1t",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "s-7x-1t"},
	}
	if _, _, _, err := orc.resolveHardwareRequirements(&job1t); err != nil || job1t.Topology != "1x1x1" {
		t.Fatalf("resolveHardwareRequirements(tpu7x-standard-1t on omitted-topo slice) = (topo=%q, err=%v), want (1x1x1, nil)", job1t.Topology, err)
	}
}

func TestTPUSlice_ForbidsSliceNameOnStaticReuseWorkloadMetadata(t *testing.T) {
	staticContract := tpuSliceManifestContract{
		podLabelsYAML:   k8s.TPUSliceTypeLabel + ": tpu7x\n" + k8s.TPUSliceTopologyLabel + ": 2x2x1",
		commPortEnabled: true,
		dynamicTPUSlice: false,
	}
	for _, key := range []string{k8s.TPUSliceNameLabel, k8s.TPUSliceSchedulerNameLabel} {
		jsManifest := fmt.Sprintf(`apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: static-job
  labels:
    %s: shared-slice
spec:
  replicatedJobs:
  - name: main-job
    template:
      spec:
        template:
          metadata:
            labels:
              tpu.google.com/type: tpu7x
              tpu.google.com/topology: 2x2x1
          spec:
            containers:
            - name: workload-container
              ports:
              - containerPort: 8471
`, key)
		if err := validateTPUSliceManifest(jsManifest, staticContract, "hint"); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("static JobSet with metadata.labels[%s] err = %v, want rejection", key, err)
		}

		batchJobManifest := fmt.Sprintf(`apiVersion: batch/v1
kind: Job
metadata:
  name: static-batch
  labels:
    %s: shared-slice
spec:
  template:
    metadata:
      labels:
        tpu.google.com/type: tpu7x
        tpu.google.com/topology: 2x2x1
    spec:
      containers:
      - name: workload-container
        ports:
        - containerPort: 8471
`, key)
		if err := validateTPUSliceManifest(batchJobManifest, staticContract, "hint"); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("static batch Job with metadata.labels[%s] err = %v, want rejection", key, err)
		}
	}
}

func TestTPUSlice_MissingTopologyErrorChipMismatchAndShapeGrouping(t *testing.T) {
	v6e4t, _ := makeTPUSlicePipelineOrchestrator("v6e", "ct6e-standard-4t")
	v6e4t.inventory.TPU.Topologies = []string{"2x2", "2x4"}
	v6e4t.inventory.Slices = []k8s.SliceInfo{{Name: "v6e-4t-slice", TPUType: "v6e", ChipsPerNode: 4, Topology: "2x2", Ready: true}}
	err8t := v6e4t.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "ct6e-standard-8t", MachineType: "ct6e-standard-8t", NumSlices: 1})
	if strings.Contains(err8t.Error(), "Cluster has") || strings.Contains(err8t.Error(), "2x2") || strings.Contains(err8t.Error(), "v6e-4t-slice") {
		t.Errorf("ct6e-standard-8t on 4t cluster err = %q, want no Cluster has / 4t shapes / 4t slice", err8t)
	}

	unlabeledBM := newTestGKEOrchestrator(NewMockExecutor(nil))
	unlabeledBM.mode = orchestrator.TypeK8s
	unlabeledBM.addons = addonPlan{resolved: true, jobSetEnabled: true}
	unlabeledBM.inventory = &k8s.Inventory{
		LabelScheme:        k8s.SchemeTPUSlice,
		TPUSliceAPIPresent: true,
		TPU:                &k8s.TPUInventory{TPUType: "", MachineType: "bm-host", ChipsPerNode: 4, Topologies: []string{"2x2"}},
		PoolNames:          []string{"bm-host"},
		Pools:              map[string]k8s.PoolCapacity{"bm-host": {MachineType: "bm-host", IsTPU: true, TPUsPerNode: 4, ReadyNodes: 2}},
	}
	errUnlabeled := unlabeledBM.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x-standard-4t", MachineType: "tpu7x-standard-4t", NumSlices: 1})
	if strings.Contains(errUnlabeled.Error(), "Cluster has") {
		t.Errorf("tpu7x on unlabeled bm-host err = %q, want no Cluster has", errUnlabeled)
	}

	mixedShapes, _ := makeTPUSlicePipelineOrchestrator("tpu7x", "tpu7x-standard-4t")
	mixedShapes.inventory.Slices = []k8s.SliceInfo{
		{Name: "s-small", TPUType: "tpu7x", ChipsPerNode: 4, Topology: "2x2x1", NodesPerSlice: 1, Ready: true},
		{Name: "s-big-1", TPUType: "tpu7x", ChipsPerNode: 4, Topology: "2x2x2", NodesPerSlice: 2, Ready: true},
		{Name: "s-big-2", TPUType: "tpu7x", ChipsPerNode: 4, Topology: "2x2x2", NodesPerSlice: 2, Ready: true},
	}
	errGrouped := mixedShapes.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 2})
	if !strings.Contains(errGrouped.Error(), k8s.TPUSliceNameLabel+"=s-big-1|s-big-2") {
		t.Errorf("mixed-shape 2-slice err = %q, want compatible pair s-big-1|s-big-2", errGrouped)
	}
	mixedShapes.inventory.Slices = mixedShapes.inventory.Slices[:2]
	errIncompat := mixedShapes.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 2})
	if !strings.Contains(errIncompat.Error(), k8s.TPUSliceNameLabel+"=<slice-1>|<slice-2>") {
		t.Errorf("incompatible-shape 2-slice err = %q, want placeholder <slice-1>|<slice-2>", errIncompat)
	}

	mixedShapes.inventory.Slices = []k8s.SliceInfo{
		{Name: "s-unknown-1", TPUType: "tpu7x", ChipsPerNode: 4, Ready: true},
		{Name: "s-unknown-2", TPUType: "tpu7x", ChipsPerNode: 4, Ready: true},
	}
	errUnknown := mixedShapes.missingTPUSliceTopologyError(&orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 2})
	if !strings.Contains(errUnknown.Error(), k8s.TPUSliceNameLabel+"=<slice-1>|<slice-2>") {
		t.Errorf("unknown-shape 2-slice err = %q, want placeholder <slice-1>|<slice-2>", errUnknown)
	}
}

func TestTPUSlice_ShorthandPreserves8tWhenInventoryHasUnknownChips(t *testing.T) {
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.mode = orchestrator.TypeK8s
	orc.addons = addonPlan{resolved: true, jobSetEnabled: true}
	orc.inventory = &k8s.Inventory{
		LabelScheme:        k8s.SchemeTPUSlice,
		TPUSliceAPIPresent: true,
		TPU:                &k8s.TPUInventory{TPUType: "v6e", MachineType: "", ChipsPerNode: 0},
	}
	if got := orc.resolveK8sTPUShorthandFromPools("v6e-8", "ct6e-standard-8t"); got != "ct6e-standard-8t" {
		t.Errorf("resolveK8sTPUShorthandFromPools(v6e-8, unknown inv chips) = %q, want ct6e-standard-8t", got)
	}
}

func TestTPUSlice_RejectsEmptyStringForbiddenLabelsAndAnnotations(t *testing.T) {
	contract := tpuSliceManifestContract{
		workloadLabelsYAML: k8s.TPUSlicePrefixLabel + ": dyn-job",
		podLabelsYAML:      k8s.TPUSliceTypeLabel + ": tpu7x\n" + k8s.TPUSliceTopologyLabel + ": 2x2x1",
		commPortEnabled:    true,
		dynamicTPUSlice:    true,
		schedulerName:      k8s.TPUSliceSchedulerName,
	}
	emptySliceNameLabel := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: dyn-job
  labels:
    tpu.google.com/slice-prefix: dyn-job
    tpu.google.com/slice-name: ""
spec:
  replicatedJobs:
  - name: main-job
    template:
      metadata:
        labels:
          tpu.google.com/type: tpu7x
          tpu.google.com/topology: 2x2x1
      spec:
        template:
          metadata:
            labels:
              tpu.google.com/type: tpu7x
              tpu.google.com/topology: 2x2x1
          spec:
            schedulerName: slice-scheduler
            containers:
            - name: workload-container
              ports:
              - containerPort: 8471
`
	if err := validateTPUSliceManifest(emptySliceNameLabel, contract, "hint"); err == nil || !strings.Contains(err.Error(), k8s.TPUSliceNameLabel) {
		t.Errorf("empty-value slice-name label err = %v, want rejection", err)
	}

	emptyExclusiveAnnot := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: dyn-job
  labels:
    tpu.google.com/slice-prefix: dyn-job
  annotations:
    alpha.jobset.sigs.k8s.io/exclusive-topology: ""
spec:
  replicatedJobs:
  - name: main-job
    template:
      metadata:
        labels:
          tpu.google.com/type: tpu7x
          tpu.google.com/topology: 2x2x1
      spec:
        template:
          metadata:
            labels:
              tpu.google.com/type: tpu7x
              tpu.google.com/topology: 2x2x1
          spec:
            schedulerName: slice-scheduler
            containers:
            - name: workload-container
              ports:
              - containerPort: 8471
`
	if err := validateTPUSliceManifest(emptyExclusiveAnnot, contract, "hint"); err == nil || !strings.Contains(err.Error(), "exclusive-topology") {
		t.Errorf("empty-value exclusive-topology annotation err = %v, want rejection", err)
	}
}
