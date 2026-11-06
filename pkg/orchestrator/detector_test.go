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

package orchestrator_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	_ "hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type recordingRunner struct {
	calls  [][]string
	result shell.CommandResult
}

func (r *recordingRunner) ExecuteCommandWithTimeout(timeout time.Duration, name string, args ...string) shell.CommandResult {
	r.calls = append(r.calls, append([]string{name, timeout.String()}, args...))
	return r.result
}

func buildSingleContextConfig(ctxName, clusterName, server string, ai *clientcmdapi.AuthInfo) *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[clusterName] = &clientcmdapi.Cluster{Server: server}
	cfg.AuthInfos["user"] = ai
	cfg.Contexts[ctxName] = &clientcmdapi.Context{Cluster: clusterName, AuthInfo: "user"}
	cfg.CurrentContext = ctxName
	return cfg
}

func TestRegisteredTypes_IncludesGKEAndK8s(t *testing.T) {
	types := orchestrator.RegisteredTypes()
	for _, want := range []orchestrator.Type{orchestrator.TypeGKE, orchestrator.TypeK8s} {
		if !slices.Contains(types, want) {
			t.Errorf("RegisteredTypes() = %v, missing %q", types, want)
		}
	}
}

func TestDetect_FourStepPrecedenceMatrix(t *testing.T) {
	tests := []struct {
		name           string
		env            map[string]string
		cfg            *clientcmdapi.Config
		clusterName    string
		location       string
		projectID      string
		versionResult  shell.CommandResult
		wantType       orchestrator.Type
		wantContext    string
		wantCluster    string
		wantSubprocess int
	}{
		{
			name:        "Step 1 GCLUSTER_ORCHESTRATOR=gke overrides kubeadm client-cert context with zero subprocesses",
			env:         map[string]string{"GCLUSTER_ORCHESTRATOR": "gke"},
			cfg:         buildSingleContextConfig("kubernetes-admin@kubernetes", "kubernetes", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert")}),
			wantType:    orchestrator.TypeGKE,
			wantContext: "kubernetes-admin@kubernetes",
			wantCluster: "kubernetes",
		},
		{
			name:        "Step 1 GCLUSTER_ORCHESTRATOR=k8s overrides gke-gcloud-auth-plugin context with zero subprocesses",
			env:         map[string]string{"GCLUSTER_ORCHESTRATOR": "k8s"},
			cfg:         buildSingleContextConfig("gke_proj_us-central1_clus", "gke_proj_us-central1_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}),
			wantType:    orchestrator.TypeK8s,
			wantContext: "gke_proj_us-central1_clus",
			wantCluster: "gke_proj_us-central1_clus",
		},
		{
			name:        "Step 2 POSIX gke-gcloud-auth-plugin selects TypeGKE offline",
			cfg:         buildSingleContextConfig("gke_proj_us-central1_clus", "gke_proj_us-central1_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "/usr/bin/gke-gcloud-auth-plugin"}}),
			clusterName: "clus",
			location:    "us-central1",
			projectID:   "proj",
			wantType:    orchestrator.TypeGKE,
			wantContext: "gke_proj_us-central1_clus",
			wantCluster: "clus",
		},
		{
			name:        "Step 2 Windows backslash gke-gcloud-auth-plugin.exe and zonal-to-regional context match",
			cfg:         buildSingleContextConfig("gke_proj_us-central1_clus", "gke_proj_us-central1_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: `C:\Program Files\Google\Cloud SDK\gke-gcloud-auth-plugin.EXE`}}),
			clusterName: "clus",
			location:    "us-central1-a",
			projectID:   "proj",
			wantType:    orchestrator.TypeGKE,
			wantContext: "gke_proj_us-central1_clus",
			wantCluster: "clus",
		},
		{
			name:        "Step 3 static bearer token targeting *.gke.goog endpoint selects TypeGKE offline",
			cfg:         buildSingleContextConfig("dns-ctx", "dns-cluster", "https://gke-123456.us-central1.gke.goog:443/", &clientcmdapi.AuthInfo{Token: "test-bearer-token"}),
			wantType:    orchestrator.TypeGKE,
			wantContext: "dns-ctx",
			wantCluster: "dns-cluster",
		},
		{
			name: "Step 4 static bearer token on IP endpoint with -gke. serverVersion selects TypeGKE via pinned kubectl",
			cfg:  buildSingleContextConfig("static-gke-ctx", "static-gke", "https://34.118.224.10", &clientcmdapi.AuthInfo{Token: "test-bearer-token"}),
			versionResult: shell.CommandResult{
				ExitCode: 0,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.0"},"serverVersion":{"gitVersion":"v1.32.2-gke.1182000"}}`,
			},
			wantType:       orchestrator.TypeGKE,
			wantContext:    "static-gke-ctx",
			wantCluster:    "static-gke",
			wantSubprocess: 1,
		},
		{
			name: "Step 4 Google Cloud SDK -gke. clientVersion against kubeadm serverVersion stays TypeK8s",
			cfg:  buildSingleContextConfig("token-k8s-ctx", "token-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"}),
			versionResult: shell.CommandResult{
				ExitCode: 0,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.1-gke.1200"},"serverVersion":{"gitVersion":"v1.32.13"}}`,
			},
			wantType:       orchestrator.TypeK8s,
			wantContext:    "token-k8s-ctx",
			wantCluster:    "token-k8s",
			wantSubprocess: 1,
		},
		{
			name:        "Step 4 skipped under GCLUSTER_AIR_GAPPED=true (zero subprocesses)",
			env:         map[string]string{"GCLUSTER_AIR_GAPPED": "true"},
			cfg:         buildSingleContextConfig("airgap-token-ctx", "airgap-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"}),
			wantType:    orchestrator.TypeK8s,
			wantContext: "airgap-token-ctx",
			wantCluster: "airgap-k8s",
		},
		{
			name:        "kubeadm X.509 client-cert context selects TypeK8s with zero subprocesses even when --project/--location passed",
			cfg:         buildSingleContextConfig("kubernetes-admin@kubernetes", "kubernetes", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert-pem")}),
			projectID:   "ar-build-proj",
			location:    "us-central1",
			wantType:    orchestrator.TypeK8s,
			wantContext: "kubernetes-admin@kubernetes",
			wantCluster: "kubernetes",
		},
		{
			name:        "fresh workstation with empty kubeconfig and GKE flags selects TypeGKE for get-credentials bootstrap with zero subprocesses",
			cfg:         clientcmdapi.NewConfig(),
			clusterName: "fresh-gke",
			location:    "us-central1",
			projectID:   "my-proj",
			wantType:    orchestrator.TypeGKE,
			wantContext: "",
			wantCluster: "fresh-gke",
		},
		{
			name:        "no kubeconfig and no GKE flags selects TypeK8s with empty context and zero subprocesses",
			cfg:         clientcmdapi.NewConfig(),
			wantType:    orchestrator.TypeK8s,
			wantContext: "",
			wantCluster: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{result: tc.versionResult}
			det := &orchestrator.Detector{
				Getenv:           func(k string) string { return tc.env[k] },
				KubeconfigLoader: func() (*clientcmdapi.Config, error) { return tc.cfg, nil },
				Runner:           runner,
			}
			got, err := det.Detect(tc.clusterName, tc.location, tc.projectID)
			if err != nil {
				t.Fatalf("Detect() unexpected error: %v", err)
			}
			if got.Type != tc.wantType || got.KubeContext != tc.wantContext || got.ClusterName != tc.wantCluster {
				t.Errorf("Detect() = %+v, want Type=%q KubeContext=%q ClusterName=%q", got, tc.wantType, tc.wantContext, tc.wantCluster)
			}
			if len(runner.calls) != tc.wantSubprocess {
				t.Fatalf("subprocess calls = %v (len %d), want %d", runner.calls, len(runner.calls), tc.wantSubprocess)
			}
			if tc.wantSubprocess == 1 {
				joined := strings.Join(runner.calls[0], " ")
				if !strings.Contains(joined, "3s") || !strings.Contains(joined, "--context "+tc.wantContext) || !strings.Contains(joined, "--request-timeout=2s") {
					t.Errorf("version probe call = %q, want 3s process timeout, explicit --context %s, and --request-timeout=2s", joined, tc.wantContext)
				}
			}
		})
	}
}

func TestDetect_HonorsDefaultEnvironmentAirGappedState(t *testing.T) {
	t.Setenv(orchestrator.OrchestratorEnvVar, "")
	t.Setenv(shell.AirGappedEnvVar, "")
	origAirGap := shell.IsKnownAirGapped()
	t.Cleanup(func() { shell.SetDefaultAirGappedState(origAirGap) })
	shell.SetDefaultAirGapped()

	cfg := buildSingleContextConfig("airgap-default-ctx", "airgap-default", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"})
	runner := &recordingRunner{}
	det := &orchestrator.Detector{
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return cfg, nil },
		Runner:           runner,
	}
	got, err := det.Detect("", "", "")
	if err != nil || got.Type != orchestrator.TypeK8s || len(runner.calls) != 0 {
		t.Fatalf("Detect() with SetDefaultAirGapped = (%+v, %v), calls=%v; want TypeK8s with 0 subprocesses", got, err, runner.calls)
	}
}
