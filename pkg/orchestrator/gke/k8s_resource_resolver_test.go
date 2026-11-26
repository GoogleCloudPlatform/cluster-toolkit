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

package gke

import (
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
)

func TestResolveK8sHardwareRequirements_TPUSlicePreflightOrderingWithoutTopology(t *testing.T) {
	baseInv := &k8s.Inventory{
		LabelScheme: k8s.SchemeTPUSlice,
		TPU:         &k8s.TPUInventory{TPUType: "tpu7x", MachineType: "tpu7x-standard-4t", ChipsPerNode: 4},
		Slices: []k8s.SliceInfo{
			{Name: "creating-slice", TPUType: "tpu7x", Ready: false, State: "CREATING"},
			{Name: "ready-slice", TPUType: "tpu7x", Topology: "2x2x1", Ready: true, NodesPerSlice: 1, ChipsPerNode: 4},
		},
	}
	explicitInv := &k8s.Inventory{
		LabelScheme:               k8s.SchemeTPUSlice,
		ExplicitMode:              true,
		SliceInventoryUnavailable: true,
	}

	tests := []struct {
		name          string
		inv           *k8s.Inventory
		jobSetEnabled bool
		job           orchestrator.JobDefinition
		wantErr       string
		forbidErr     string
	}{
		{
			name:          "jobset_unnamed_tpu_without_topology_reports_slice_requirement",
			inv:           baseInv,
			jobSetEnabled: true,
			job:           orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 1},
			wantErr:       k8s.TPUSliceNameLabel,
			forbidErr:     "--topology is required",
		},
		{
			name:          "jobset_autodiscovered_tpu_without_topology_reports_slice_requirement",
			inv:           baseInv,
			jobSetEnabled: true,
			job:           orchestrator.JobDefinition{NumSlices: 1},
			wantErr:       k8s.TPUSliceNameLabel,
			forbidErr:     "--topology is required",
		},
		{
			name:          "tier3_unnamed_tpu_without_topology_reports_slice_requirement",
			inv:           baseInv,
			jobSetEnabled: false,
			job:           orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 1},
			wantErr:       "standalone batch/v1 Indexed Job without JobSet cannot dynamically provision a new TPU slice",
			forbidErr:     "--topology is required",
		},
		{
			name:          "explicit_mode_jobset_unnamed_tpu_without_topology_reports_slice_requirement",
			inv:           explicitInv,
			jobSetEnabled: true,
			job:           orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 1},
			wantErr:       k8s.TPUSliceNameLabel,
			forbidErr:     "--topology is required",
		},
		{
			name:          "explicit_mode_tier3_unnamed_tpu_without_topology_reports_slice_requirement",
			inv:           explicitInv,
			jobSetEnabled: false,
			job:           orchestrator.JobDefinition{ComputeType: "tpu7x", NumSlices: 1},
			wantErr:       "standalone batch/v1 Indexed Job without JobSet cannot dynamically provision a new TPU slice",
			forbidErr:     "--topology is required",
		},
		{
			name:          "jobset_creating_slice_without_topology_reports_not_ready",
			inv:           baseInv,
			jobSetEnabled: true,
			job: orchestrator.JobDefinition{
				ComputeType:    "tpu7x",
				NumSlices:      1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "creating-slice"},
			},
			wantErr:   `is not READY (state="CREATING")`,
			forbidErr: "--topology is required",
		},
		{
			name:          "tier3_creating_slice_without_topology_reports_not_ready",
			inv:           baseInv,
			jobSetEnabled: false,
			job: orchestrator.JobDefinition{
				ComputeType:    "tpu7x",
				NumSlices:      1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "creating-slice"},
			},
			wantErr:   `is not READY (state="CREATING")`,
			forbidErr: "--topology is required",
		},
		{
			name:          "jobset_missing_slice_without_topology_reports_not_exist",
			inv:           baseInv,
			jobSetEnabled: true,
			job: orchestrator.JobDefinition{
				ComputeType:    "tpu7x",
				NumSlices:      1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "ghost-slice"},
			},
			wantErr:   `TPU slice "ghost-slice" does not exist`,
			forbidErr: "--topology is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orc := &GKEOrchestrator{
				mode: orchestrator.TypeK8s,
				submissionSession: submissionSession{
					inventory: tc.inv,
					addons:    addonPlan{resolved: true, jobSetEnabled: tc.jobSetEnabled},
				},
			}
			job := tc.job
			_, _, _, err := orc.resolveK8sHardwareRequirements(&job)
			if err == nil {
				t.Fatalf("resolveK8sHardwareRequirements() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("resolveK8sHardwareRequirements() err = %q, want substring %q", err.Error(), tc.wantErr)
			}
			if tc.forbidErr != "" && strings.Contains(err.Error(), tc.forbidErr) {
				t.Errorf("resolveK8sHardwareRequirements() err = %q, must not contain %q", err.Error(), tc.forbidErr)
			}
		})
	}
}

func TestResolveK8sHardwareRequirements_NonSliceSchemeAllowsSliceConstraint(t *testing.T) {
	for _, scheme := range []k8s.LabelScheme{k8s.SchemeGeneric, k8s.SchemeGKE} {
		t.Run(string(scheme), func(t *testing.T) {
			orc := &GKEOrchestrator{
				mode: orchestrator.TypeK8s,
				submissionSession: submissionSession{
					inventory: &k8s.Inventory{
						LabelScheme:               scheme,
						SliceInventoryUnavailable: false,
					},
					addons: addonPlan{resolved: true, jobSetEnabled: true},
				},
			}
			job := orchestrator.JobDefinition{
				ComputeType:    "tpu7x",
				Topology:       "2x2x1",
				NumSlices:      1,
				NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "my-slice"},
			}
			profile, _, _, err := orc.resolveK8sHardwareRequirements(&job)
			if err != nil {
				t.Fatalf("resolveK8sHardwareRequirements(%s) unexpected err = %v", scheme, err)
			}
			if !profile.IsTPU || job.Topology != "2x2x1" || job.NodesPerSlice != 1 {
				t.Errorf("got (IsTPU=%v, Topology=%q, NodesPerSlice=%d), want (true, 2x2x1, 1)", profile.IsTPU, job.Topology, job.NodesPerSlice)
			}
		})
	}
}

func TestResolveFallbackTPUChips_CaseInsensitiveSingleChip(t *testing.T) {
	for _, mt := range []string{"CT6E-STANDARD-1T", "TPU7X-STANDARD-1T", "tpu7x-1"} {
		t.Run(mt, func(t *testing.T) {
			pool := k8s.PoolCapacity{MachineType: mt, IsTPU: true, TPUsPerNode: 0}
			_, _, _, tpuLim, err := limitsFromK8sPool(pool)
			if err != nil || tpuLim != "1" {
				t.Errorf("limitsFromK8sPool(%q) = (%q, %v), want (\"1\", nil)", mt, tpuLim, err)
			}
			orc := &GKEOrchestrator{
				mode: orchestrator.TypeK8s,
				submissionSession: submissionSession{
					inventory: &k8s.Inventory{TPU: &k8s.TPUInventory{}},
				},
			}
			if got := orc.resolveK8sTPUChipsPerNode(mt, k8s.PoolCapacity{}, false, 0); got != 1 {
				t.Errorf("resolveK8sTPUChipsPerNode(%q) = %d, want 1", mt, got)
			}
		})
	}
}

func TestResolveK8sHardwareRequirements_MixedClusterAutoDiscovery(t *testing.T) {
	orc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"a3-highgpu-8g", "n2-standard-32"},
				Pools: map[string]k8s.PoolCapacity{
					"a3-highgpu-8g":  {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 8, GPUType: "nvidia-h100-80gb", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 2},
					"n2-standard-32": {MachineType: "n2-standard-32", AllocatableCPUMilli: 32000, ReadyNodes: 4},
				},
			},
		},
	}
	job := orchestrator.JobDefinition{NumSlices: 1}
	_, _, _, err := orc.resolveK8sHardwareRequirements(&job)
	if err == nil || !strings.Contains(err.Error(), "--compute-type is required; discovered:") ||
		!strings.Contains(err.Error(), "a3-highgpu-8g") || !strings.Contains(err.Error(), "n2-standard-32") {
		t.Fatalf("mixed GPU+CPU auto-discovery err = %v, want ambiguous discovered list", err)
	}

	ctrlAndGPU := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"ctrl", "gpu"},
				Pools: map[string]k8s.PoolCapacity{
					"ctrl": {MachineType: "e2-standard-4", IsControlPlane: true, AllocatableCPUMilli: 4000, ReadyNodes: 1},
					"gpu":  {MachineType: "g2-standard-12", IsGPU: true, GPUsPerNode: 1, GPUType: "nvidia-l4", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 2},
				},
			},
		},
	}
	gpuJob := orchestrator.JobDefinition{NumSlices: 1}
	if prof, _, _, err := ctrlAndGPU.resolveK8sHardwareRequirements(&gpuJob); err != nil || prof.IsCPUMachine || prof.CapacityCount != 1 {
		t.Fatalf("control-plane + single GPU worker = (prof=%+v, err=%v), want 1 GPU", prof, err)
	}
}

func TestResolveK8sHardwareRequirements_TwoGPUPoolsAmbiguity(t *testing.T) {
	orc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"a3-highgpu-8g", "g2-standard-12"},
				Pools: map[string]k8s.PoolCapacity{
					"a3-highgpu-8g":  {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 8, GPUType: "nvidia-h100-80gb", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 2},
					"g2-standard-12": {MachineType: "g2-standard-12", IsGPU: true, GPUsPerNode: 1, GPUType: "nvidia-l4", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 2},
				},
			},
		},
	}
	omitted := orchestrator.JobDefinition{NumSlices: 1}
	if _, _, _, err := orc.resolveK8sHardwareRequirements(&omitted); err == nil || !strings.Contains(err.Error(), "--compute-type is required; discovered:") ||
		!strings.Contains(err.Error(), "nvidia-l4") || !strings.Contains(err.Error(), "nvidia-h100-80gb") {
		t.Fatalf("two-GPU omitted compute-type err = %v, want ambiguous discovered list", err)
	}
	byRes := orchestrator.JobDefinition{ComputeType: "nvidia.com/gpu", NumSlices: 1}
	if _, _, _, err := orc.resolveK8sHardwareRequirements(&byRes); err == nil || !strings.Contains(err.Error(), "--compute-type is required") ||
		!strings.Contains(err.Error(), "nvidia-l4") || !strings.Contains(err.Error(), "nvidia-h100-80gb") {
		t.Fatalf("two-GPU nvidia.com/gpu query err = %v, want ambiguous discovered list", err)
	}
	specific := orchestrator.JobDefinition{ComputeType: "nvidia-l4", NumSlices: 1}
	if prof, _, _, err := orc.resolveK8sHardwareRequirements(&specific); err != nil || prof.CapacityCount != 1 {
		t.Fatalf("specific nvidia-l4 query = (prof=%+v, err=%v), want 1 GPU", prof, err)
	}
}

func TestResolveK8sHardwareRequirements_SharedMachineTypeAndRestartingGPUAmbiguity(t *testing.T) {
	// Shared MachineType with distinct GPU types (e.g., n1-standard-8 T4 vs V100) must be flagged ambiguous.
	sharedMT := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGKE,
				PoolNames:   []string{"n1-t4", "n1-v100"},
				Pools: map[string]k8s.PoolCapacity{
					"n1-t4":   {MachineType: "n1-standard-8", IsGPU: true, GPUsPerNode: 1, GPUType: "nvidia-tesla-t4", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 2},
					"n1-v100": {MachineType: "n1-standard-8", IsGPU: true, GPUsPerNode: 1, GPUType: "nvidia-tesla-v100", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 2},
				},
			},
		},
	}
	for _, ct := range []string{"", "nvidia.com/gpu", "n1-standard-8"} {
		job := orchestrator.JobDefinition{ComputeType: ct, NumSlices: 1}
		if _, _, _, err := sharedMT.resolveK8sHardwareRequirements(&job); err == nil ||
			!strings.Contains(err.Error(), "nvidia-tesla-t4") || !strings.Contains(err.Error(), "nvidia-tesla-v100") {
			t.Errorf("shared-MT compute-type=%q err = %v, want ambiguity listing both T4 and V100", ct, err)
		}
	}

	// Heterogeneous MIG profiles sharing MachineType + GPUType must include MIG resource names in display labels.
	migOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"a3-mig1", "a3-mig3"},
				Pools: map[string]k8s.PoolCapacity{
					"a3-mig1": {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 7, GPUType: "NVIDIA-H100-80GB-HBM3", GPUResourceName: "nvidia.com/mig-1g.10gb", ReadyNodes: 2},
					"a3-mig3": {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 2, GPUType: "NVIDIA-H100-80GB-HBM3", GPUResourceName: "nvidia.com/mig-3g.40gb", ReadyNodes: 2},
				},
			},
		},
	}
	migByMT := orchestrator.JobDefinition{ComputeType: "a3-highgpu-8g", NumSlices: 1}
	if _, _, _, err := migOrc.resolveK8sHardwareRequirements(&migByMT); err == nil ||
		!strings.Contains(err.Error(), "nvidia.com/mig-1g.10gb") || !strings.Contains(err.Error(), "nvidia.com/mig-3g.40gb") {
		t.Errorf("MIG shared-MT query err = %v, want both MIG resource names listed", err)
	}
	migSpecific := orchestrator.JobDefinition{ComputeType: "nvidia.com/mig-3g.40gb", NumSlices: 1}
	if prof, _, _, err := migOrc.resolveK8sHardwareRequirements(&migSpecific); err != nil || prof.CapacityCount != 2 {
		t.Errorf("specific MIG query = (prof=%+v, err=%v), want 2 MIG instances", prof, err)
	}

	// Single GPU pool + one restarting 0-GPU node must still auto-discover cleanly when --compute-type is omitted.
	restartingOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGeneric,
				PoolNames:   []string{"a-restarting-0gpu", "b-healthy-8gpu"},
				Pools: map[string]k8s.PoolCapacity{
					"a-restarting-0gpu": {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 0, GPUType: "nvidia-h100-80gb", ReadyNodes: 1},
					"b-healthy-8gpu":    {MachineType: "a3-highgpu-8g", IsGPU: true, GPUsPerNode: 8, GPUType: "nvidia-h100-80gb", GPUResourceName: "nvidia.com/gpu", ReadyNodes: 7},
				},
			},
		},
	}
	autoJob := orchestrator.JobDefinition{NumSlices: 1}
	if prof, _, _, err := restartingOrc.resolveK8sHardwareRequirements(&autoJob); err != nil || prof.CapacityCount != 8 {
		t.Errorf("single GPU pool with restarting 0-GPU node = (prof=%+v, err=%v), want 8 GPUs", prof, err)
	}
}

func TestResolveK8sHardwareRequirements_NodeDerivedSliceTypoAndTopologyDerivation(t *testing.T) {
	// Nodes readable (ExplicitMode=false) + Slice CRD 403 (SliceInventoryUnavailable=true):
	// typo slice name must be rejected rather than silently accepted.
	typoOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme:               k8s.SchemeTPUSlice,
				ExplicitMode:              false,
				SliceInventoryUnavailable: true,
				Slices: []k8s.SliceInfo{
					{Name: "real-slice", TPUType: "tpu7x", MachineType: "tpu7x-standard-4t", Topology: "2x2x1", ChipsPerNode: 4, NodesPerSlice: 1, Ready: true},
				},
			},
			addons: addonPlan{resolved: true, jobSetEnabled: true},
		},
	}
	typoJob := orchestrator.JobDefinition{
		ComputeType:    "tpu7x-standard-4t",
		Topology:       "2x2x1",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "reel-slice-typo"},
	}
	if _, _, _, err := typoOrc.resolveK8sHardwareRequirements(&typoJob); err == nil ||
		!strings.Contains(err.Error(), `TPU slice "reel-slice-typo" does not exist`) ||
		!strings.Contains(err.Error(), "Slice objects could not be listed") {
		t.Fatalf("typo slice with nodes-200/slices-403 err = %v, want missing slice with Slice-API note", err)
	}

	// deriveSliceTopologyFromNodes must work for tpu7x (including 1t and multi-host) and ct5l-.
	for _, tc := range []struct {
		name    string
		tpuType string
		mt      string
		chips   int
		nodes   int
		want    string
	}{
		{"tpu7x_1t_single_node", "tpu7x", "tpu7x-standard-1t", 1, 1, "1x1x1"},
		{"tpu7x_4t_1_node", "tpu7x", "tpu7x-standard-4t", 4, 1, "2x2x1"},
		{"tpu7x_4t_2_nodes", "tpu7x", "tpu7x-standard-4t", 4, 2, "2x2x2"},
		{"tpu7x_4t_4_nodes", "tpu7x", "tpu7x-standard-4t", 4, 4, "2x2x4"},
		{"ct5l_4t_2_nodes", "v5e", "ct5l-hightpu-4t", 4, 2, "2x4"},
		{"v6e_4t_2_nodes", "v6e", "ct6e-standard-4t", 4, 2, "2x4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveSliceTopologyFromNodes(
				&orchestrator.JobDefinition{MachineType: tc.mt},
				k8s.SliceInfo{TPUType: tc.tpuType, MachineType: tc.mt, ChipsPerNode: tc.chips, NodesPerSlice: tc.nodes},
			)
			if got != tc.want {
				t.Errorf("deriveSliceTopologyFromNodes(%s, %d nodes x %d chips) = %q, want %q", tc.mt, tc.nodes, tc.chips, got, tc.want)
			}
		})
	}

	// --compute-type v5e-8 on a ct5l-hightpu-8t pool must resolve topology 2x4 cleanly.
	ct5lOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{
				LabelScheme: k8s.SchemeGKE,
				PoolNames:   []string{"ct5l-8t"},
				Pools: map[string]k8s.PoolCapacity{
					"ct5l-8t": {MachineType: "ct5l-hightpu-8t", IsTPU: true, TPUType: "v5e", TPUsPerNode: 8, GKETPUAcceleratorLabel: "tpu-v5-lite-device", ReadyNodes: 1},
				},
			},
			addons: addonPlan{resolved: true, jobSetEnabled: true},
		},
	}
	v5eJob := orchestrator.JobDefinition{ComputeType: "v5e-8", NumSlices: 1}
	prof, _, _, err := ct5lOrc.resolveK8sHardwareRequirements(&v5eJob)
	if err != nil {
		t.Fatalf("resolveK8sHardwareRequirements(v5e-8 on ct5l-hightpu-8t) err = %v", err)
	}
	if !prof.IsTPU || v5eJob.MachineType != "ct5l-hightpu-8t" || v5eJob.Topology != "2x4" || v5eJob.NodesPerSlice != 1 {
		t.Errorf("v5e-8 on ct5l-hightpu-8t = (IsTPU=%v mt=%q topo=%q nodes=%d), want (true, ct5l-hightpu-8t, 2x4, 1)", prof.IsTPU, v5eJob.MachineType, v5eJob.Topology, v5eJob.NodesPerSlice)
	}
}

func TestResolveAndValidateTPUSlices_Tier3DuplicatePipeAndProfileIsTPU(t *testing.T) {
	inv := &k8s.Inventory{
		LabelScheme: k8s.SchemeTPUSlice,
		Slices: []k8s.SliceInfo{
			{Name: "ready-slice", TPUType: "tpu7x", Topology: "2x2x1", Ready: true, NodesPerSlice: 1, ChipsPerNode: 4},
			{Name: "ready-slice-2", TPUType: "tpu7x", Topology: "2x2x1", Ready: true, NodesPerSlice: 1, ChipsPerNode: 4},
		},
	}
	orc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: inv,
			addons:    addonPlan{resolved: true, jobSetEnabled: false},
		},
	}

	dupJob := orchestrator.JobDefinition{
		ComputeType:    "tpu7x-standard-4t",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "ready-slice|ready-slice"},
	}
	if _, _, _, err := orc.resolveK8sHardwareRequirements(&dupJob); err != nil {
		t.Fatalf("duplicate pipe-delimited single slice should deduplicate via parseAffinityValues, got err = %v", err)
	}
	if dupJob.Topology != "2x2x1" || dupJob.NodesPerSlice != 1 {
		t.Errorf("dupJob resolved (Topology=%q, NodesPerSlice=%d), want (2x2x1, 1)", dupJob.Topology, dupJob.NodesPerSlice)
	}

	multiJob := orchestrator.JobDefinition{
		ComputeType:    "tpu7x-standard-4t",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "ready-slice|ready-slice-2"},
	}
	if _, _, _, err := orc.resolveK8sHardwareRequirements(&multiJob); err == nil || !strings.Contains(err.Error(), "cannot target multiple TPU slices") {
		t.Fatalf("distinct pipe-delimited slices in Tier-3 err = %v, want multi-slice rejection", err)
	}

	emptyElemJob := orchestrator.JobDefinition{
		ComputeType:    "tpu7x-standard-4t",
		NumSlices:      1,
		NodeConstraint: map[string]string{k8s.TPUSliceNameLabel: "ready-slice|"},
	}
	if _, _, _, err := orc.resolveK8sHardwareRequirements(&emptyElemJob); err == nil || !strings.Contains(err.Error(), "empty element") {
		t.Fatalf("trailing pipe slice constraint err = %v, want empty element error", err)
	}

	// Verify validatePostHardwareChecks honors profile.IsTPU directly without re-scanning pools.
	gkeSchemeOrc := &GKEOrchestrator{
		mode: orchestrator.TypeK8s,
		submissionSession: submissionSession{
			inventory: &k8s.Inventory{LabelScheme: k8s.SchemeGKE},
			addons:    addonPlan{resolved: true, jobSetEnabled: false},
		},
	}
	customHostJob := orchestrator.JobDefinition{
		MachineType:    "bm-host",
		NodesPerSlice:  2,
		NumSlices:      1,
		DryRunManifest: "/tmp/out.yaml",
	}
	if err := gkeSchemeOrc.validatePostHardwareChecks(&StorageManager{orchestrator: gkeSchemeOrc}, &customHostJob, JobProfile{IsTPU: true}); err == nil || !strings.Contains(err.Error(), k8s.GKENodePoolLabel) {
		t.Errorf("validatePostHardwareChecks(IsTPU=true on bm-host) err = %v, want GKE nodepool pinning error", err)
	}
	if err := gkeSchemeOrc.validatePostHardwareChecks(&StorageManager{orchestrator: gkeSchemeOrc}, &customHostJob, JobProfile{IsTPU: false}); err != nil {
		t.Errorf("validatePostHardwareChecks(IsTPU=false on bm-host) unexpected err = %v", err)
	}
}
