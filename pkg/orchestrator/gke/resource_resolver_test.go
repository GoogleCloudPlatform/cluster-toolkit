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
	"hpc-toolkit/pkg/shell"
	"testing"
)

func TestResolveMachineName(t *testing.T) {
	tests := []struct {
		name            string
		acceleratorType string
		wantMachineName string
	}{
		{
			name:            "Direct shorthand mapping",
			acceleratorType: "l4-1",
			wantMachineName: "g2-standard-12",
		},
		{
			name:            "TPU shorthand mapping",
			acceleratorType: "v5p-1",
			wantMachineName: "ct5p-hightpu-1t",
		},
		{
			name:            "Unknown type falls back to input",
			acceleratorType: "unknown-machine",
			wantMachineName: "unknown-machine",
		},
		{
			name:            "GKE label string resolution for unknown shorthand",
			acceleratorType: "nvidia-l4",
			wantMachineName: "nvidia-l4", // Default fallthrough if neither matches
		},
		{
			name:            "TPU7 shorthand mapping",
			acceleratorType: "tpu7",
			wantMachineName: "tpu7-standard-1t",
		},
		{
			name:            "TPU7x shorthand mapping",
			acceleratorType: "tpu7x",
			wantMachineName: "tpu7x-standard-4t",
		},
	}

	g := &GKEOrchestrator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.resolveMachineName(tt.acceleratorType)
			if got != tt.wantMachineName {
				t.Errorf("resolveMachineName() = %v, want %v", got, tt.wantMachineName)
			}
		})
	}
}

func TestFetchMachineCapacity_AllZonesFail(t *testing.T) {
	mockResponses := map[string][]shell.CommandResult{
		"gcloud compute machine-types describe tpu7 --zone=europe-west2-a": {
			{ExitCode: 1, Stderr: "resource not found"},
		},
		"gcloud compute machine-types describe tpu7 --zone=europe-west2-c": {
			{ExitCode: 1, Stderr: "resource not found"},
		},
		"gcloud compute machine-types describe tpu7 --zone=europe-west2-b": {
			{ExitCode: 1, Stderr: "resource not found"},
		},
	}
	mockExec := NewMockExecutor(mockResponses)
	g := &GKEOrchestrator{executor: mockExec}
	g.clusterZones = []string{"europe-west2-a", "europe-west2-c", "europe-west2-b"}

	_, err := g.FetchMachineCapacity("tpu7", "europe-west2")

	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	expectedErrStr := "failed to fetch machine capacity for tpu7: tried in all candidate zones [europe-west2-a europe-west2-c europe-west2-b] but did not find machine type in any of them"
	if err.Error() != expectedErrStr {
		t.Errorf("Expected error %q, got %q", expectedErrStr, err.Error())
	}
}
