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
			capFromPool, err := machineTypeCapFromPool(pool, "")
			if err != nil || len(capFromPool.Accelerators) != 1 || capFromPool.Accelerators[0].Count != 1 {
				t.Errorf("machineTypeCapFromPool(%q) = (%+v, %v), want 1 chip", mt, capFromPool, err)
			}
			orc := &GKEOrchestrator{
				mode: orchestrator.TypeK8s,
				submissionSession: submissionSession{
					inventory: &k8s.Inventory{TPU: &k8s.TPUInventory{}},
				},
			}
			capFromInv := orc.machineTypeCapFromTPUInventory(mt)
			if len(capFromInv.Accelerators) != 1 || capFromInv.Accelerators[0].Count != 1 {
				t.Errorf("machineTypeCapFromTPUInventory(%q) = %+v, want 1 chip", mt, capFromInv)
			}
			fallbackCap := fallbackK8sMachineTypeCap(mt, "")
			if len(fallbackCap.Accelerators) != 1 || fallbackCap.Accelerators[0].Count != 1 {
				t.Errorf("fallbackK8sMachineTypeCap(%q) = %+v, want 1 chip", mt, fallbackCap)
			}
			if got := orc.resolveK8sTPUChipsPerNode(mt, k8s.PoolCapacity{}, false, 0); got != 1 {
				t.Errorf("resolveK8sTPUChipsPerNode(%q) = %d, want 1", mt, got)
			}
		})
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
