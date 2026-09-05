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
	"hpc-toolkit/pkg/shell"

	corev1 "k8s.io/api/core/v1"
)

func TestResolveReservationTolerations(t *testing.T) {
	tests := []struct {
		name            string
		machineType     string
		reservationName string
		nodePools       []gkeJobNodePool
		wantTolerations []corev1.Toleration
	}{
		{
			name:            "Reservation only, no matching node pools (NAP case)",
			machineType:     "a3-highgpu-8g",
			reservationName: "projects/my-project/reservations/my-res-1",
			nodePools:       nil,
			wantTolerations: []corev1.Toleration{
				{
					Key:      "cloud.google.com/reservation-name",
					Operator: corev1.TolerationOpEqual,
					Value:    "my-res-1",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name:            "Reservation with matching node pool with additional taints",
			machineType:     "a3-highgpu-8g",
			reservationName: "my-res-2",
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "a3-highgpu-8g",
						Labels: map[string]string{
							"cloud.google.com/reservation-name": "my-res-2",
						},
						Taints: []gkeTaint{
							{
								Key:    "my-custom-taint",
								Value:  "custom-value",
								Effect: "NoSchedule",
							},
						},
					},
				},
			},
			wantTolerations: []corev1.Toleration{
				{
					Key:      "cloud.google.com/reservation-name",
					Operator: corev1.TolerationOpEqual,
					Value:    "my-res-2",
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      "my-custom-taint",
					Operator: corev1.TolerationOpEqual,
					Value:    "custom-value",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name:            "Reservation with matching node pool that duplicates reservation taint",
			machineType:     "a3-highgpu-8g",
			reservationName: "my-res-3",
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "a3-highgpu-8g",
						Labels: map[string]string{
							"cloud.google.com/reservation-name": "my-res-3",
						},
						Taints: []gkeTaint{
							{
								Key:    "cloud.google.com/reservation-name",
								Value:  "my-res-3",
								Effect: "NoSchedule",
							},
							{
								Key:    "another-taint",
								Value:  "value",
								Effect: "NoSchedule",
							},
						},
					},
				},
			},
			wantTolerations: []corev1.Toleration{
				{
					Key:      "cloud.google.com/reservation-name",
					Operator: corev1.TolerationOpEqual,
					Value:    "my-res-3",
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      "another-taint",
					Operator: corev1.TolerationOpEqual,
					Value:    "value",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name:            "Reservation with matching node pool using full URI",
			machineType:     "a3-highgpu-8g",
			reservationName: "projects/my-project/reservations/my-res-4",
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "a3-highgpu-8g",
						Labels: map[string]string{
							"cloud.google.com/reservation-name": "my-res-4",
						},
						Taints: []gkeTaint{
							{
								Key:    "cloud.google.com/reservation-name",
								Value:  "my-res-4",
								Effect: "NoSchedule",
							},
						},
					},
				},
			},
			wantTolerations: []corev1.Toleration{
				{
					Key:      "cloud.google.com/reservation-name",
					Operator: corev1.TolerationOpEqual,
					Value:    "my-res-4",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name:            "Reservation with matching node pool, case-insensitive reservation name",
			machineType:     "a3-highgpu-8g",
			reservationName: "My-ReS-5",
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "a3-highgpu-8g",
						Labels: map[string]string{
							"cloud.google.com/reservation-name": "my-res-5",
						},
						Taints: []gkeTaint{
							{
								Key:    "cloud.google.com/reservation-name",
								Value:  "my-res-5",
								Effect: "NoSchedule",
							},
						},
					},
				},
			},
			wantTolerations: []corev1.Toleration{
				{
					Key:      "cloud.google.com/reservation-name",
					Operator: corev1.TolerationOpEqual,
					Value:    "my-res-5",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name:            "Reservation with matching node pool, converts GKE NO_SCHEDULE taint to Kubernetes NoSchedule",
			machineType:     "a3-highgpu-8g",
			reservationName: "my-res-6",
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "a3-highgpu-8g",
						Labels: map[string]string{
							"cloud.google.com/reservation-name": "my-res-6",
						},
						Taints: []gkeTaint{
							{
								Key:    "cloud.google.com/reservation-name",
								Value:  "my-res-6",
								Effect: "NO_SCHEDULE",
							},
							{
								Key:    "custom-taint-prefer",
								Value:  "prefer-val",
								Effect: "PREFER_NO_SCHEDULE",
							},
							{
								Key:    "custom-taint-execute",
								Value:  "execute-val",
								Effect: "NO_EXECUTE",
							},
						},
					},
				},
			},
			wantTolerations: []corev1.Toleration{
				{
					Key:      "cloud.google.com/reservation-name",
					Operator: corev1.TolerationOpEqual,
					Value:    "my-res-6",
					Effect:   corev1.TaintEffectNoSchedule,
				},
				{
					Key:      "custom-taint-prefer",
					Operator: corev1.TolerationOpEqual,
					Value:    "prefer-val",
					Effect:   corev1.TaintEffectPreferNoSchedule,
				},
				{
					Key:      "custom-taint-execute",
					Operator: corev1.TolerationOpEqual,
					Value:    "execute-val",
					Effect:   corev1.TaintEffectNoExecute,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GKEOrchestrator{
				clusterDesc: gkeCluster{
					NodePools: tt.nodePools,
				},
			}
			got := g.resolveReservationTolerations(tt.machineType, tt.reservationName)
			if len(got) != len(tt.wantTolerations) {
				t.Fatalf("expected %d tolerations, got %d", len(tt.wantTolerations), len(got))
			}
			for i, wt := range tt.wantTolerations {
				gt := got[i]
				if gt.Key != wt.Key || gt.Operator != wt.Operator || gt.Value != wt.Value || gt.Effect != wt.Effect {
					t.Errorf("toleration %d mismatch: got %+v, want %+v", i, gt, wt)
				}
			}
		})
	}
}

func TestResolveTolerations(t *testing.T) {
	tests := []struct {
		name             string
		acceleratorType  string
		consumptionModel string
		reservationName  string
		nodePools        []gkeJobNodePool
		wantContains     []string
	}{
		{
			name:             "TPU with Spot consumption model",
			acceleratorType:  "v5p-8",
			consumptionModel: "spot",
			wantContains: []string{
				"key: google.com/tpu",
				"key: cloud.google.com/gke-provisioning",
				"value: spot",
			},
		},
		{
			name:             "TPU with Reservation consumption model",
			acceleratorType:  "v5p-8",
			consumptionModel: "reservation",
			reservationName:  "my-res",
			wantContains: []string{
				"key: google.com/tpu",
				"key: cloud.google.com/reservation-name",
				"value: my-res",
			},
		},
		{
			name:             "Non-TPU with Spot consumption model",
			acceleratorType:  "nvidia-l4",
			consumptionModel: "spot",
			wantContains: []string{
				"key: cloud.google.com/gke-provisioning",
				"value: spot",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GKEOrchestrator{
				clusterDesc: gkeCluster{
					NodePools: tt.nodePools,
				},
			}
			got, err := g.resolveTolerations(tt.acceleratorType, tt.consumptionModel, tt.reservationName, 16)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, sub := range tt.wantContains {
				if !strings.Contains(got, sub) {
					t.Errorf("expected output to contain %q, got %q", sub, got)
				}
			}
		})
	}
}

func TestResolveTolerationsDoesNotMutateSharedArray(t *testing.T) {
	// Verify that multiple calls to resolveTolerations do not mutate the underlying array returned by GetTolerations.
	g := &GKEOrchestrator{}

	// Call resolveTolerations for a TPU with Spot (which appends "spot")
	got1, err := g.resolveTolerations("v5p-8", "spot", "", 16)
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}

	// Call resolveTolerations for a TPU with standard consumption model (no Spot/Reservation)
	got2, err := g.resolveTolerations("v5p-8", "", "", 16)
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}

	// The second result should ONLY have TPU toleration, NOT spot
	if strings.Contains(got2, "spot") {
		t.Errorf("second call unexpectedly contains 'spot'. got1: %q, got2: %q", got1, got2)
	}
}

func TestParseReservationURI(t *testing.T) {
	tests := []struct {
		input string
		want  parsedReservation
	}{
		{
			input: "my-res",
			want: parsedReservation{
				Name: "my-res",
			},
		},
		{
			input: "projects/my-project/reservations/my-res",
			want: parsedReservation{
				Project: "my-project",
				Name:    "my-res",
			},
		},
		{
			input: "projects/my-project/reservations/my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2",
			want: parsedReservation{
				Project:  "my-project",
				Name:     "my-res",
				Block:    "block-1",
				Subblock: "subblock-2",
			},
		},
		{
			input: "my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2",
			want: parsedReservation{
				Name:     "my-res",
				Block:    "block-1",
				Subblock: "subblock-2",
			},
		},
		{
			input: "nvidia-gb300-1elqwl23xva0f/reservationBlocks/nvidia-gb300-1elqwl23xva0f-block-0001/reservationSubBlocks/nvidia-gb300-1elqwl23xva0f-block-0001-subblock-0002",
			want: parsedReservation{
				Name:     "nvidia-gb300-1elqwl23xva0f",
				Block:    "nvidia-gb300-1elqwl23xva0f-block-0001",
				Subblock: "nvidia-gb300-1elqwl23xva0f-block-0001-subblock-0002",
			},
		},
		{
			input: "projects/my-project/reservations/my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2/",
			want: parsedReservation{
				Project:  "my-project",
				Name:     "my-res",
				Block:    "block-1",
				Subblock: "subblock-2",
			},
		},
		{
			input: "projects/my-project/reservations/my-res/reservationBlocks/block-1",
			want: parsedReservation{
				Project: "my-project",
				Name:    "my-res",
				Block:   "block-1",
			},
		},
		{
			input: "projects/123456789/reservations/my-res",
			want: parsedReservation{
				Project: "123456789",
				Name:    "my-res",
			},
		},
		{
			input: "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/reservations/my-res",
			want: parsedReservation{
				Project: "my-project",
				Name:    "my-res",
			},
		},
		{
			input: "folders/my-folder/my-res",
			want: parsedReservation{
				Name: "my-res",
			},
		},
		{
			input: "projects/My-Project/reservations/My-Res/reservationBlocks/Block-1/reservationSubBlocks/Subblock-2",
			want: parsedReservation{
				Project:  "my-project",
				Name:     "my-res",
				Block:    "block-1",
				Subblock: "subblock-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseReservationURI(tt.input)
			if got != tt.want {
				t.Errorf("parseReservationURI(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveTPUWorkloadPolicy(t *testing.T) {
	tests := []struct {
		name          string
		job           *orchestrator.JobDefinition
		nodePools     []gkeJobNodePool
		mockResponses map[string][]shell.CommandResult
		wantPolicy    string
		wantErr       bool
	}{
		{
			name: "Placement policy already specified on job is preserved",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				PlacementPolicy: "user-specified-policy",
			},
			wantPolicy: "user-specified-policy",
		},
		{
			name: "Discovered from existing matching node pool",
			job: &orchestrator.JobDefinition{
				MachineType:   "tpu7x-standard-4t",
				Topology:      "2x2x2",
				NodesPerSlice: 2,
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "nap-tpu7x-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
						Labels: map[string]string{
							"cloud.google.com/gke-tpu-topology": "2x2x2",
						},
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName: "projects/my-project/regions/us-central1/resourcePolicies/existing-nodepool-policy",
					},
				},
			},
			wantPolicy: "existing-nodepool-policy",
		},
		{
			name: "Discovered from describe of canonical policy",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy"}`},
				},
			},
			wantPolicy: "tpu7x-16-2x2x2-placement-policy",
		},
		{
			name: "Permission denied on describe falls back to canonical policy name without error",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy": {
					{ExitCode: 1, Stderr: "ERROR: (gcloud.compute.resource-policies.describe) Some requests did not succeed: - Required 'compute.resourcePolicies.get' permission for '...'"},
				},
			},
			wantPolicy: "tpu7x-16-2x2x2-placement-policy",
		},
		{
			name: "Discovered from regional list when canonical name differs",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy": {
					{ExitCode: 1, Stderr: "ERROR: not found"},
				},
				"gcloud compute resource-policies list --project=my-project --filter=region:( us-central1 ) AND workloadPolicy.acceleratorTopology=2x2x2 AND workloadPolicy.type=HIGH_THROUGHPUT --format=value(name)": {
					{ExitCode: 0, Stdout: "custom-regional-2x2x2-policy\n"},
				},
			},
			wantPolicy: "custom-regional-2x2x2-policy",
		},
		{
			name: "Auto-creates policy when not found and not dry-run",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				DryRunManifest:  "",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy": {
					{ExitCode: 1, Stderr: "ERROR: not found"},
				},
				"gcloud compute resource-policies list": {
					{ExitCode: 0, Stdout: ""},
				},
				"gcloud compute resource-policies create workload-policy tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=my-project --type=HIGH_THROUGHPUT --accelerator-topology=2x2x2": {
					{ExitCode: 0, Stdout: "Created [https://www.googleapis.com/...].\n"},
				},
			},
			wantPolicy: "tpu7x-16-2x2x2-placement-policy",
		},
		{
			name: "Dry-run does not call create command",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
				DryRunManifest:  "/tmp/job.yaml",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy": {
					{ExitCode: 1, Stderr: "ERROR: not found"},
				},
				"gcloud compute resource-policies list": {
					{ExitCode: 0, Stdout: ""},
				},
			},
			wantPolicy: "tpu7x-16-2x2x2-placement-policy",
		},
		{
			name: "Single-node slice does not assign workload placement policy",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x1",
				NodesPerSlice:   1,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
			},
			wantPolicy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			g := newTestGKEOrchestrator(mockExecutor)
			g.projectID = tt.job.ProjectID
			g.clusterDesc.NodePools = tt.nodePools

			err := g.resolveTPU7xWorkloadPolicyIfRequired(tt.job, true, false)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveTPU7xWorkloadPolicyIfRequired() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.job.PlacementPolicy != tt.wantPolicy {
				t.Errorf("resolveTPU7xWorkloadPolicyIfRequired() placementPolicy = %q, want %q", tt.job.PlacementPolicy, tt.wantPolicy)
			}
		})
	}
}

func TestBuildNodeSelector_PlacementPolicy(t *testing.T) {
	g := newTestGKEOrchestrator(nil)
	g.machineCapCache["tpu7x-standard-4t:"] = MachineTypeCap{}
	g.machineCapCache["a3-highgpu-8g:"] = MachineTypeCap{}

	// 1. TPU workload uses cloud.google.com/placement-policy-name
	tpuJob := orchestrator.JobDefinition{
		MachineType: "tpu7x-standard-4t",
	}
	tpuOpts := SchedulingOptions{
		PlacementPolicy: "tpu7x-16-2x2x2-placement-policy",
		IsTPU:           true,
	}
	tpuSelectorStr, err := g.buildNodeSelector(tpuOpts, tpuJob, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(tpuSelectorStr, "cloud.google.com/placement-policy-name: tpu7x-16-2x2x2-placement-policy") {
		t.Errorf("Expected cloud.google.com/placement-policy-name in %s", tpuSelectorStr)
	}
	if strings.Contains(tpuSelectorStr, "cloud.google.com/gke-placement-group") {
		t.Errorf("Did not expect cloud.google.com/gke-placement-group in %s", tpuSelectorStr)
	}

	// 2. Non-TPU workload uses cloud.google.com/gke-placement-group
	nonTpuJob := orchestrator.JobDefinition{
		MachineType: "a3-highgpu-8g",
	}
	nonTpuOpts := SchedulingOptions{
		PlacementPolicy: "compact-placement-group",
	}
	nonTpuSelectorStr, err := g.buildNodeSelector(nonTpuOpts, nonTpuJob, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(nonTpuSelectorStr, "cloud.google.com/gke-placement-group: compact-placement-group") {
		t.Errorf("Expected cloud.google.com/gke-placement-group in %s", nonTpuSelectorStr)
	}
	if strings.Contains(nonTpuSelectorStr, "cloud.google.com/placement-policy-name") {
		t.Errorf("Did not expect cloud.google.com/placement-policy-name in %s", nonTpuSelectorStr)
	}
}
