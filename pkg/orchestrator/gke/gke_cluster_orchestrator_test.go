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
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"strings"
	"testing"
)

func TestListEnvironments(t *testing.T) {
	mockResponses := map[string][]shell.CommandResult{
		"gcloud container clusters list --project test-project": {
			{ExitCode: 0, Stdout: `[{"name": "cluster-1", "location": "us-central1-a", "status": "RUNNING"}, {"name": "cluster-2", "location": "us-east1-b", "status": "STOPPED"}]`},
		},
	}
	orc := &GKEOrchestrator{executor: NewMockExecutor(mockResponses)}

	envs, err := orc.ListEnvironments(orchestrator.ListOptions{ProjectID: "test-project"})
	if err != nil {
		t.Fatalf("ListEnvironments failed: %v", err)
	}

	if len(envs) != 2 {
		t.Errorf("Expected 2 environments, got %d", len(envs))
	}

	if envs[0].Name != "cluster-1" || envs[0].Status != "RUNNING" {
		t.Errorf("Unexpected environment status: %+v", envs[0])
	}
}

func TestGetClusterInfo(t *testing.T) {
	mockResponses := map[string][]shell.CommandResult{
		"gcloud container clusters describe cluster-1 --location=us-central1-a --project test-project": {
			{ExitCode: 0, Stdout: `{"name": "cluster-1", "location": "us-central1-a", "nodePools": [{"name": "pool-1", "config": {"machineType": "n2-standard-4"}, "count": 2, "status": "RUNNING"}]}`},
		},
	}
	orc := &GKEOrchestrator{executor: NewMockExecutor(mockResponses)}

	info, err := orc.GetClusterInfo("cluster-1", orchestrator.ListOptions{ClusterLocation: "us-central1-a", ProjectID: "test-project"})
	if err != nil {
		t.Fatalf("GetClusterInfo failed: %v", err)
	}

	if !strings.Contains(info, "NodePool: pool-1") {
		t.Errorf("Info missing node pool name")
	}
	if !strings.Contains(info, "MachineType: n2-standard-4") {
		t.Errorf("Info missing machine type")
	}
}

func TestDescribeEnvironment(t *testing.T) {
	mockResponses := map[string][]shell.CommandResult{
		"gcloud container clusters describe cluster-1 --location=us-central1-a --project test-project": {
			{ExitCode: 0, Stdout: "yaml output of describe"},
		},
	}
	orc := &GKEOrchestrator{executor: NewMockExecutor(mockResponses)}

	desc, err := orc.DescribeEnvironment("cluster-1", orchestrator.ListOptions{ClusterLocation: "us-central1-a", ProjectID: "test-project"})
	if err != nil {
		t.Fatalf("DescribeEnvironment failed: %v", err)
	}

	if desc != "yaml output of describe" {
		t.Errorf("Expected 'yaml output of describe', got %q", desc)
	}
}

func TestListVolumes(t *testing.T) {
	mockResponses := map[string][]shell.CommandResult{
		"kubectl get pvc": {
			{ExitCode: 0, Stdout: `{"items": [{"metadata": {"name": "pvc-1"}, "spec": {"storageClassName": "standard"}} ]}`},
		},
	}
	orc := &GKEOrchestrator{executor: NewMockExecutor(mockResponses)}

	vols, err := orc.ListVolumes(orchestrator.ListOptions{})
	if err != nil {
		t.Fatalf("ListVolumes failed: %v", err)
	}

	if len(vols) != 1 {
		t.Errorf("Expected 1 volume, got %d", len(vols))
	}

	if vols[0].Name != "pvc-1" || vols[0].Type != "standard" {
		t.Errorf("Unexpected volume status: %+v", vols[0])
	}
}

func TestListVolumes_PinsNamedCluster(t *testing.T) {
	const pvcCmd = "kubectl --context " + testContext + " get pvc --all-namespaces -l ghpc_role=file-system -o json"
	const describeCmd = "gcloud container clusters describe " + testCluster + " --location " + testLocation + " --project " + testProject + " --format=json"
	exec := newExactExecutor(map[string][]shell.CommandResult{
		describeCmd: {{ExitCode: 0, Stdout: `{"controlPlaneEndpointsConfig":{"dnsEndpointConfig":{"allowExternalTraffic":true,"endpoint":"` + testDNSHost + `"}}}`}},
		probeCmd:    {{ExitCode: 0}},
		pvcCmd:      {{ExitCode: 0, Stdout: `{"items": [{"metadata": {"name": "pvc-1"}}]}`}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

	vols, err := orc.ListVolumes(orchestrator.ListOptions{ClusterName: testCluster, ClusterLocation: testLocation, ProjectID: testProject})
	if err != nil {
		t.Fatalf("ListVolumes failed: %v", err)
	}
	if len(vols) != 1 || vols[0].Name != "pvc-1" {
		t.Errorf("unexpected volumes: %+v", vols)
	}
	if exec.count(pvcCmd) != 1 {
		t.Errorf("get pvc was not pinned to %q; calls=%v", testContext, exec.calls)
	}
}

func TestListVolumes_ClusterWithoutLocationFails(t *testing.T) {
	orc := newTestGKEOrchestrator(newExactExecutor(nil))
	if _, err := orc.ListVolumes(orchestrator.ListOptions{ClusterName: testCluster}); err == nil {
		t.Fatal("expected error when --cluster is set without --location")
	}
}
