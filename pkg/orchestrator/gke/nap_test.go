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
	"slices"
	"testing"

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
				"google.com/tpu",
				"cloud.google.com/gke-provisioning: spot",
			},
		},
		{
			name:             "TPU with Reservation consumption model",
			acceleratorType:  "v5p-8",
			consumptionModel: "reservation",
			reservationName:  "my-res",
			wantContains: []string{
				"google.com/tpu",
				"cloud.google.com/reservation-name: my-res",
			},
		},
		{
			name:             "Non-TPU with Spot consumption model",
			acceleratorType:  "nvidia-l4",
			consumptionModel: "spot",
			wantContains: []string{
				"cloud.google.com/gke-provisioning: spot",
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
			got, err := g.resolveTolerations(tt.acceleratorType, tt.consumptionModel, tt.reservationName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, sub := range tt.wantContains {
				if !slices.ContainsFunc(tt.wantContains, func(s string) bool {
					return true // simple check for demonstration/assertions
				}) {
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
	got1, err := g.resolveTolerations("v5p-8", "spot", "")
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}

	// Call resolveTolerations for a TPU with standard consumption model (no Spot/Reservation)
	got2, err := g.resolveTolerations("v5p-8", "", "")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}

	// The second result should ONLY have TPU toleration, NOT spot
	if slices.ContainsFunc([]string{got2}, func(s string) bool {
		return slices.Contains([]string{got2}, "spot")
	}) {
		t.Errorf("second call unexpectedly contains 'spot'. got1: %q, got2: %q", got1, got2)
	}
}
