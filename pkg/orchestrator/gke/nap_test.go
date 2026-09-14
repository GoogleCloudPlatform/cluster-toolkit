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
				Zone:    "us-central1-a",
				Name:    "my-res",
			},
		},
		{
			input: "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/reservations/my-res/reservationBlocks/block-1/reservationSubBlocks/subblock-2",
			want: parsedReservation{
				Project:  "my-project",
				Zone:     "us-central1-a",
				Name:     "my-res",
				Block:    "block-1",
				Subblock: "subblock-2",
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
		{
			input: "projects/my-project/zones/us-central1-a/reservations/",
			want: parsedReservation{
				Project: "my-project",
				Zone:    "us-central1-a",
				Name:    "",
			},
		},
		{
			input: "projects/my-project/zones/us-central1-a/reservations",
			want: parsedReservation{
				Project: "my-project",
				Zone:    "us-central1-a",
				Name:    "",
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
			name: "Existing node pool with PROVISION_ONLY policy is skipped for static workload",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
			},
			nodePools: []gkeJobNodePool{
				{
					Name: "dynamic-tpu7x-pool",
					Config: gkeNodePoolConfig{
						MachineType: "tpu7x-standard-4t",
						Labels: map[string]string{
							"cloud.google.com/gke-tpu-topology": "2x2x2",
						},
					},
					PlacementPolicy: &gkePlacementPolicy{
						PolicyName:              "dynamic-slicing-policy",
						AcceleratorTopologyMode: "PROVISION_ONLY",
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
				},
			},
			wantPolicy: "tpu7x-16-2x2x2-placement-policy",
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
					{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2"}}`},
				},
			},
			wantPolicy: "tpu7x-16-2x2x2-placement-policy",
		},
		{
			name: "Canonical policy describe returns incompatible topology, falls back to regional list",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x4"}}`},
				},
				"gcloud compute resource-policies list --project=my-project --filter=region:( us-central1 ) AND workloadPolicy.acceleratorTopology=2x2x2 AND workloadPolicy.type=HIGH_THROUGHPUT --format=value(name,workloadPolicy.acceleratorTopologyMode)": {
					{ExitCode: 0, Stdout: "valid-regional-2x2x2-policy\n"},
				},
			},
			wantPolicy: "valid-regional-2x2x2-policy",
		},
		{
			name: "Canonical policy describe returns PROVISION_ONLY mode, falls back to regional list and skips non-static modes",
			job: &orchestrator.JobDefinition{
				MachineType:     "tpu7x-standard-4t",
				Topology:        "2x2x2",
				NodesPerSlice:   2,
				ClusterLocation: "us-central1-c",
				ProjectID:       "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute resource-policies describe tpu7x-16-2x2x2-placement-policy --region=us-central1 --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"tpu7x-16-2x2x2-placement-policy","region":"us-central1","workloadPolicy":{"type":"HIGH_THROUGHPUT","acceleratorTopology":"2x2x2","acceleratorTopologyMode":"PROVISION_ONLY"}}`},
				},
				"gcloud compute resource-policies list --project=my-project --filter=region:( us-central1 ) AND workloadPolicy.acceleratorTopology=2x2x2 AND workloadPolicy.type=HIGH_THROUGHPUT --format=value(name,workloadPolicy.acceleratorTopologyMode)": {
					{ExitCode: 0, Stdout: "dynamic-candidate PROVISION_ONLY\nfuture-flavor FUTURE_MODE\nstatic-regional-2x2x2-policy AUTO_CONNECT\n"},
				},
			},
			wantPolicy: "static-regional-2x2x2-policy",
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
				"gcloud compute resource-policies list --project=my-project --filter=region:( us-central1 ) AND workloadPolicy.acceleratorTopology=2x2x2 AND workloadPolicy.type=HIGH_THROUGHPUT --format=value(name,workloadPolicy.acceleratorTopologyMode)": {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			g := newTestGKEOrchestrator(mockExecutor)
			g.projectID = tt.job.ProjectID
			g.clusterDesc.NodePools = tt.nodePools

			policy, err := g.resolveTPUWorkloadPolicy(tt.job.MachineType, tt.job.Topology, tt.job.ClusterLocation, tt.job.ProjectID, tt.job.DryRunManifest != "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveTPUWorkloadPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && policy != tt.wantPolicy {
				t.Errorf("resolveTPUWorkloadPolicy() = %q, want %q", policy, tt.wantPolicy)
			}
		})
	}
}

func TestValidateConsumptionForStaticCluster(t *testing.T) {
	tests := []struct {
		name        string
		napEnabled  bool
		napLimits   map[string]int64
		nodePools   []gkeJobNodePool
		job         orchestrator.JobDefinition
		wantErr     bool
		expectedErr string
	}{
		{
			name:       "Static Cluster - Explicit on-demand flag fails",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "on-demand",
			},
			wantErr:     true,
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning \"on-demand\", --gke-nap-reservation \"\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
		},
		{
			name:       "Static Cluster - Empty string consumption model",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "",
			},
			wantErr: false,
		},
		{
			name:       "Static Cluster - Consumption model flag set to spot",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning \"spot\", --gke-nap-reservation \"\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
		},
		{
			name:       "Static Cluster - Reservation name flag set",
			napEnabled: false,
			job: orchestrator.JobDefinition{
				GKENAPProvisioning: "on-demand",
				GKENAPReservation:  "my-res",
			},
			wantErr:     true,
			expectedErr: "GKE NAP provisioning options (--gke-nap-provisioning \"on-demand\", --gke-nap-reservation \"my-res\") are only supported on GKE clusters with Node Auto-Provisioning (NAP) enabled",
		},
		{
			name:       "NAP Cluster - Machine type in NAP limits",
			napEnabled: true,
			napLimits: map[string]int64{
				"tpu-v6e-slice": 100,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "ct6e-standard-8t", // TPU
				GKENAPProvisioning: "spot",
			},
			wantErr: false,
		},
		{
			name:       "NAP Cluster - Machine type not in limits, but matches static pool",
			napEnabled: true,
			napLimits:  map[string]int64{},
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "n2-standard-4",
						Labels: map[string]string{
							"cloud.google.com/gke-provisioning": "spot",
						},
					},
				},
			},
			job: orchestrator.JobDefinition{
				ComputeType:        "n2-standard-4",
				MachineType:        "n2-standard-4",
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
		},
		{
			name:       "NAP Cluster - Machine type not in limits, and mismatches static pool",
			napEnabled: true,
			napLimits:  map[string]int64{},
			nodePools: []gkeJobNodePool{
				{
					Config: gkeNodePoolConfig{
						MachineType: "n2-standard-4",
						Labels: map[string]string{
							"cloud.google.com/gke-provisioning": "standard",
						},
					},
				},
			},
			job: orchestrator.JobDefinition{
				ComputeType:        "n2-standard-4",
				MachineType:        "n2-standard-4",
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
		},
		{
			name:       "NAP Cluster - Machine type covered by generic TPU limit fallback",
			napEnabled: true,
			napLimits: map[string]int64{
				"google.com/tpu": 100,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "ct6e-standard-8t",
				GKENAPProvisioning: "spot",
			},
			wantErr: false,
		},
		{
			name:       "NAP Cluster - Machine type with unknown GPU accelerator fails fast",
			napEnabled: true,
			napLimits:  map[string]int64{},
			job: orchestrator.JobDefinition{
				MachineType:        "my-unknown-gpu-machine",
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "unknown accelerator label: \"unknown-gpu\"",
		},
		{
			name:       "NAP Cluster - TPU: Specific limit configured, requesting different TPU (Should Fail)",
			napEnabled: true,
			napLimits: map[string]int64{
				"tpu-v6e-slice":  8,
				"google.com/tpu": 8,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "ct5lp-hightpu-4t", // TPU v5e (tpu-v5-lite-podslice)
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
		},
		{
			name:       "NAP Cluster - GPU: Specific limit configured, requesting different GPU (Should Fail)",
			napEnabled: true,
			napLimits: map[string]int64{
				"nvidia-h100-mega-80gb": 8,
				"nvidia.com/gpu":        8,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "g2-standard-12", // L4 GPU (nvidia-l4)
				GKENAPProvisioning: "spot",
			},
			wantErr:     true,
			expectedErr: "is not configured within your cluster's Node Auto-Provisioning (NAP) limits",
		},
		{
			name:       "NAP Cluster - GPU: Generic limit only, requesting GPU (Should Pass)",
			napEnabled: true,
			napLimits: map[string]int64{
				"nvidia.com/gpu": 8,
			},
			job: orchestrator.JobDefinition{
				MachineType:        "g2-standard-12",
				GKENAPProvisioning: "spot",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orc := newTestGKEOrchestrator(nil)
			orc.napEnabled = tt.napEnabled
			orc.napLimits = tt.napLimits
			orc.clusterDesc.NodePools = tt.nodePools
			orc.machineCapCache = map[string]MachineTypeCap{
				"n2-standard-4:": {
					GuestCpus: 4,
					MemoryMb:  16000,
				},
				"my-unknown-gpu-machine:": {
					GuestCpus: 8,
					MemoryMb:  32000,
					Accelerators: []struct {
						Count int    `json:"guestAcceleratorCount"`
						Type  string `json:"guestAcceleratorType"`
					}{
						{
							Count: 1,
							Type:  "unknown-gpu",
						},
					},
				},
				"g2-standard-12:": {
					GuestCpus: 12,
					MemoryMb:  48000,
					Accelerators: []struct {
						Count int    `json:"guestAcceleratorCount"`
						Type  string `json:"guestAcceleratorType"`
					}{
						{
							Count: 1,
							Type:  "nvidia-l4",
						},
					},
				},
			}

			err := orc.validateConsumptionForStaticCluster(&tt.job)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("expected error containing %q, got: %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateNAPReservation(t *testing.T) {
	tests := []struct {
		name          string
		job           *orchestrator.JobDefinition
		mockResponses map[string][]shell.CommandResult
		wantErr       bool
	}{
		{
			name: "Valid reservation matching machine type succeeds",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "my-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1-a",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations describe my-res --zone=us-central1-a --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"my-res","specificReservation":{"instanceProperties":{"machineType":"ct5p-hightpu-4t"}}}`},
				},
			},
			wantErr: false,
		},
		{
			name: "Reservation in full HTTPS URL with zone parsed and validated",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "https://www.googleapis.com/compute/v1/projects/my-owner/zones/us-central1-b/reservations/shared-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1-a",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations describe shared-res --zone=us-central1-b --project=my-owner --format=json": {
					{ExitCode: 0, Stdout: `{"name":"shared-res","specificReservation":{"instanceProperties":{"machineType":"ct5p-hightpu-4t"}}}`},
				},
			},
			wantErr: false,
		},
		{
			name: "Reservation not found (404) returns error",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "missing-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1-a",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations describe missing-res --zone=us-central1-a --project=my-project --format=json": {
					{ExitCode: 1, Stderr: "ERROR: (gcloud.compute.reservations.describe) The resource 'projects/my-project/zones/us-central1-a/reservations/missing-res' was not found"},
				},
			},
			wantErr: true,
		},
		{
			name: "Reservation machine type mismatch returns error",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "wrong-type-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1-a",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations describe wrong-type-res --zone=us-central1-a --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{"name":"wrong-type-res","specificReservation":{"instanceProperties":{"machineType":"a3-highgpu-8g"}}}`},
				},
			},
			wantErr: true,
		},
		{
			name: "Reservation 403 Forbidden degrades gracefully with warning",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "forbidden-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1-a",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations describe forbidden-res --zone=us-central1-a --project=my-project --format=json": {
					{ExitCode: 1, Stderr: "ERROR: (gcloud.compute.reservations.describe) 403 Forbidden: Required 'compute.reservations.get' permission"},
				},
			},
			wantErr: false,
		},
		{
			name: "Reservation check skipped on dry-run",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "dry-run-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1-a",
				ProjectID:          "my-project",
				DryRunManifest:     "/tmp/job.yaml",
			},
			wantErr: false,
		},
		{
			name: "Regional cluster reservation matching region and partial URI machine type succeeds",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "reg-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations list": {
					{ExitCode: 0, Stdout: `[{"zone":"projects/my-project/zones/us-central1-a","specificReservation":{"instanceProperties":{"machineType":"zones/us-central1-a/machineTypes/ct5p-hightpu-4t"}}}]`},
				},
			},
			wantErr: false,
		},
		{
			name: "Regional cluster reservation found in different region returns error",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "reg-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations list": {
					{ExitCode: 0, Stdout: `[{"zone":"europe-west4-a","specificReservation":{"instanceProperties":{"machineType":"ct5p-hightpu-4t"}}}]`},
				},
			},
			wantErr: true,
		},
		{
			name: "Reservation URI with region locality mismatch returns error",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "projects/my-project/zones/europe-west4-a/reservations/my-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1",
				ProjectID:          "my-project",
			},
			wantErr: true,
		},
		{
			name: "Zonal reservation with malformed JSON returns error",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "corrupt-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1-a",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations describe corrupt-res --zone=us-central1-a --project=my-project --format=json": {
					{ExitCode: 0, Stdout: `{invalid-json`},
				},
			},
			wantErr: true,
		},
		{
			name: "Regional cluster multi-zone reservation with first zone mismatch and second zone match succeeds",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "reg-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations list": {
					{ExitCode: 0, Stdout: `[
						{"zone":"projects/my-project/zones/us-central1-a","specificReservation":{"instanceProperties":{"machineType":"a3-highgpu-8g"}}},
						{"zone":"projects/my-project/zones/us-central1-b","specificReservation":{"instanceProperties":{"machineType":"ct5p-hightpu-4t"}}}
					]`},
				},
			},
			wantErr: false,
		},
		{
			name: "Regional cluster multi-zone reservation with all zones mismatched returns error",
			job: &orchestrator.JobDefinition{
				GKENAPProvisioning: "reservation",
				GKENAPReservation:  "reg-res",
				MachineType:        "ct5p-hightpu-4t",
				ClusterLocation:    "us-central1",
				ProjectID:          "my-project",
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud compute reservations list": {
					{ExitCode: 0, Stdout: `[
						{"zone":"projects/my-project/zones/us-central1-a","specificReservation":{"instanceProperties":{"machineType":"a3-highgpu-8g"}}},
						{"zone":"projects/my-project/zones/us-central1-b","specificReservation":{"instanceProperties":{"machineType":"n1-standard-4"}}}
					]`},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExecutor := NewMockExecutor(tt.mockResponses)
			g := newTestGKEOrchestrator(mockExecutor)
			g.projectID = tt.job.ProjectID

			err := g.validateNAPReservation(tt.job)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateNAPReservation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
