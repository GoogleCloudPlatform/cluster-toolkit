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

package job

import (
	"hpc-toolkit/pkg/orchestrator"
	"strings"
	"testing"
)

func TestInspectCmd_Success(t *testing.T) {
	setupSubmitTestEnv(t)
	var gotOpts orchestrator.InspectOptions
	inspectCalled := false
	mockOrc := &mockOrchestrator{
		inspectClusterFunc: func(opts orchestrator.InspectOptions) error {
			inspectCalled = true
			gotOpts = opts
			return nil
		},
	}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return mockOrc
	}

	_, err := executeCommand(JobCmd,
		"inspect",
		"--cluster", "test-cluster",
		"--location", "us-central1-a",
		"--project", "test-project",
		"--name", "test-workload",
		"--show",
		"--output", "/tmp/custom.log",
	)
	if err != nil {
		t.Fatalf("executeCommand(inspect) error = %v, want nil", err)
	}
	if !inspectCalled {
		t.Fatalf("InspectCluster called = false, want true")
	}
	wantOpts := orchestrator.InspectOptions{
		ClusterName:     "test-cluster",
		ClusterLocation: "us-central1-a",
		ProjectID:       "test-project",
		WorkloadName:    "test-workload",
		OutputPath:      "/tmp/custom.log",
		Show:            true,
	}
	if gotOpts != wantOpts {
		t.Errorf("InspectOptions = %+v, want %+v", gotOpts, wantOpts)
	}
}

func TestInspectCmd_NameShorthand(t *testing.T) {
	t.Run("gke_flags", func(t *testing.T) {
		setupSubmitTestEnv(t)
		var gotOpts orchestrator.InspectOptions
		inspectCalled := false
		mockOrc := &mockOrchestrator{
			inspectClusterFunc: func(opts orchestrator.InspectOptions) error {
				inspectCalled = true
				gotOpts = opts
				return nil
			},
		}
		gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

		if _, err := executeCommand(JobCmd,
			"inspect",
			"--cluster", "test-cluster",
			"--location", "us-central1-a",
			"--project", "test-project",
			"-n", "short-workload",
			"-s",
		); err != nil {
			t.Fatalf("unexpected error using -n shorthand: %v", err)
		}
		if !inspectCalled || gotOpts.WorkloadName != "short-workload" || !gotOpts.Show {
			t.Fatalf("InspectOptions = %+v (called=%v), want WorkloadName=short-workload Show=true", gotOpts, inspectCalled)
		}
	})

	t.Run("type_k8s_context", func(t *testing.T) {
		setupSubmitTestEnv(t)
		kcPath := writeTestKubeconfigFile(t, "admin@onprem-k8s", "onprem-k8s", "https://10.128.0.68:6443")
		t.Setenv("KUBECONFIG", kcPath)

		var gotOpts orchestrator.InspectOptions
		inspectCalled := false
		mockOrc := &mockOrchestrator{
			inspectClusterFunc: func(opts orchestrator.InspectOptions) error {
				inspectCalled = true
				gotOpts = opts
				return nil
			},
		}
		gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

		if _, err := executeCommand(JobCmd, "inspect", "-n", "short-k8s-workload", "-s"); err != nil {
			t.Fatalf("unexpected error using -n shorthand in TypeK8s mode: %v", err)
		}
		if !inspectCalled || gotOpts.ClusterName != "onprem-k8s" || gotOpts.WorkloadName != "short-k8s-workload" || !gotOpts.Show {
			t.Fatalf("InspectOptions = %+v (called=%v), want ClusterName=onprem-k8s WorkloadName=short-k8s-workload Show=true", gotOpts, inspectCalled)
		}
	})
}

func TestInspectCmd_SPS(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantShow   bool
		wantErr    string
	}{
		{name: "filter_pathways_sps", args: []string{"--name", "sps-v6e", "--filter", "pathways-sps"}},
		{name: "filter_pathways_sps_with_output_and_show", args: []string{"--name", "sps-v6e", "--filter=pathways-sps", "--output", "/tmp/x.log", "--show"}, wantOutput: "/tmp/x.log", wantShow: true},
		{name: "requires_name_flag", args: []string{"--filter=pathways-sps"}, wantErr: "--filter=pathways-sps requires --name"},
		{name: "rejects_unknown_filter", args: []string{"--name", "sps-v6e", "--filter=unknown"}, wantErr: "invalid value for --filter: unknown. Allowed values are: pathways-sps"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			var gotOpts orchestrator.InspectOptions
			inspectCalled := false
			mockOrc := &mockOrchestrator{
				inspectClusterFunc: func(opts orchestrator.InspectOptions) error {
					inspectCalled = true
					gotOpts = opts
					return nil
				},
			}
			gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }
			baseArgs := []string{"inspect", "--cluster", "test-cluster", "--location", "us-central1-a", "--project", "test-project"}
			_, err := executeCommand(JobCmd, append(baseArgs, tc.args...)...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got err = %v, want %q", err, tc.wantErr)
				}
				if inspectCalled {
					t.Errorf("InspectCluster called, want the flag error first")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotOpts.Filter != "pathways-sps" || gotOpts.WorkloadName != "sps-v6e" ||
				gotOpts.OutputPath != tc.wantOutput || gotOpts.Show != tc.wantShow {
				t.Errorf("InspectOptions = %+v, want Filter=pathways-sps WorkloadName=sps-v6e OutputPath=%q Show=%v",
					gotOpts, tc.wantOutput, tc.wantShow)
			}
		})
	}
}
