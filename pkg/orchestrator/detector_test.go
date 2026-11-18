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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/gke"
	"hpc-toolkit/pkg/shell"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type recordingRunner struct {
	calls  [][]string
	result shell.CommandResult
}

func (r *recordingRunner) run(timeout time.Duration, name string, args ...string) shell.CommandResult {
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
	gke.RegisterProviders()
	types := orchestrator.RegisteredTypes()
	for _, want := range []orchestrator.Type{orchestrator.TypeGKE, orchestrator.TypeK8s} {
		if !slices.Contains(types, want) {
			t.Errorf("RegisteredTypes() = %v, missing %q", types, want)
		}
	}
}

func TestDetect_FourStepPrecedenceMatrix(t *testing.T) {
	gke.RegisterProviders()
	tests := []struct {
		name             string
		env              map[string]string
		cfg              *clientcmdapi.Config
		clusterName      string
		location         string
		projectID        string
		versionResult    shell.CommandResult
		wantType         orchestrator.Type
		wantForced       bool
		wantContext      string
		wantCluster      string
		wantReasonSubstr string
		wantSubprocess   int
	}{
		{
			name:             "step1_env_override_gke",
			env:              map[string]string{"GCLUSTER_ORCHESTRATOR": "gke"},
			cfg:              buildSingleContextConfig("kubernetes-admin@kubernetes", "kubernetes", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert")}),
			wantType:         orchestrator.TypeGKE,
			wantForced:       true,
			wantContext:      "kubernetes-admin@kubernetes",
			wantCluster:      "kubernetes",
			wantReasonSubstr: "forced by GCLUSTER_ORCHESTRATOR=gke",
		},
		{
			name: "step1_env_override_gke_resolves_canonical_gke_context",
			env:  map[string]string{"GCLUSTER_ORCHESTRATOR": "gke"},
			cfg: func() *clientcmdapi.Config {
				c := buildSingleContextConfig("gke_proj_us-central1_clus", "gke_proj_us-central1_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Token: "tok"})
				c.CurrentContext = ""
				return c
			}(),
			clusterName:      "clus",
			location:         "us-central1-a",
			projectID:        "proj",
			wantType:         orchestrator.TypeGKE,
			wantForced:       true,
			wantContext:      "gke_proj_us-central1_clus",
			wantCluster:      "clus",
			wantReasonSubstr: "forced by GCLUSTER_ORCHESTRATOR=gke",
		},
		{
			name:             "step1_env_override_k8s",
			env:              map[string]string{"GCLUSTER_ORCHESTRATOR": "k8s"},
			cfg:              buildSingleContextConfig("gke_proj_us-central1_clus", "gke_proj_us-central1_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}),
			wantType:         orchestrator.TypeK8s,
			wantForced:       true,
			wantContext:      "gke_proj_us-central1_clus",
			wantCluster:      "gke_proj_us-central1_clus",
			wantReasonSubstr: "forced by GCLUSTER_ORCHESTRATOR=k8s",
		},
		{
			name:             "step2_posix_gke_auth_plugin_on_non_canonical_alias",
			cfg:              buildSingleContextConfig("prod-gpu-alias", "prod-gpu-cluster", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "/usr/bin/gke-gcloud-auth-plugin"}}),
			clusterName:      "prod-gpu-cluster",
			location:         "us-central1",
			projectID:        "proj",
			wantType:         orchestrator.TypeGKE,
			wantContext:      "prod-gpu-alias",
			wantCluster:      "prod-gpu-cluster",
			wantReasonSubstr: "authenticates via gke-gcloud-auth-plugin",
		},
		{
			name:             "step2_windows_gke_auth_plugin_exe_on_non_canonical_alias",
			cfg:              buildSingleContextConfig("win-gpu-alias", "win-gpu-cluster", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: `C:\Program Files\Google\Cloud SDK\gke-gcloud-auth-plugin.EXE`}}),
			wantType:         orchestrator.TypeGKE,
			wantContext:      "win-gpu-alias",
			wantCluster:      "win-gpu-cluster",
			wantReasonSubstr: "authenticates via gke-gcloud-auth-plugin",
		},
		{
			name:             "step2_windows_gke_auth_plugin_exe_and_zonal_to_regional",
			cfg:              buildSingleContextConfig("gke_proj_us-central1_clus", "gke_proj_us-central1_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: `C:\Program Files\Google\Cloud SDK\gke-gcloud-auth-plugin.EXE`}}),
			clusterName:      "clus",
			location:         "us-central1-a",
			projectID:        "proj",
			wantType:         orchestrator.TypeGKE,
			wantContext:      "gke_proj_us-central1_clus",
			wantCluster:      "clus",
			wantReasonSubstr: "authenticates via gke-gcloud-auth-plugin",
		},
		{
			name:             "step2_legacy_gcp_auth_provider_selects_gke_offline",
			cfg:              buildSingleContextConfig("legacy-ctx", "legacy-clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{AuthProvider: &clientcmdapi.AuthProviderConfig{Name: "gcp"}}),
			clusterName:      "legacy-clus",
			location:         "us-central1",
			projectID:        "proj",
			wantType:         orchestrator.TypeGKE,
			wantContext:      "legacy-ctx",
			wantCluster:      "legacy-clus",
			wantReasonSubstr: "uses legacy gcp auth-provider",
		},
		{
			name:             "step2_canonical_gke_context_with_non_canonical_cluster_entry_selects_gke_offline",
			cfg:              buildSingleContextConfig("gke_proj_us-central1_my_clus", "custom-cluster-entry", "https://34.118.224.10", &clientcmdapi.AuthInfo{Token: "expired-oauth"}),
			clusterName:      "my_clus",
			location:         "us-central1",
			projectID:        "proj",
			wantType:         orchestrator.TypeGKE,
			wantContext:      "gke_proj_us-central1_my_clus",
			wantCluster:      "my_clus",
			wantReasonSubstr: "matches gke_* naming",
		},
		{
			name:             "step2_renamed_context_with_canonical_gke_cluster_entry",
			cfg:              buildSingleContextConfig("prod-alias", "gke_proj_us-central1_my_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Token: "expired-oauth"}),
			wantType:         orchestrator.TypeGKE,
			wantContext:      "prod-alias",
			wantCluster:      "gke_proj_us-central1_my_clus",
			wantReasonSubstr: "matches gke_* naming",
		},
		{
			name:             "step2_refresh_credentials_env_with_gke_flags_forces_gke",
			env:              map[string]string{"GCLUSTER_REFRESH_CREDENTIALS": "1"},
			cfg:              buildSingleContextConfig("custom-gke-alias", "custom-gke-clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Token: "expired-oauth"}),
			clusterName:      "custom-gke-alias",
			location:         "us-central1",
			projectID:        "proj",
			wantType:         orchestrator.TypeGKE,
			wantContext:      "custom-gke-alias",
			wantCluster:      "custom-gke-alias",
			wantReasonSubstr: "credential refresh requested via GCLUSTER_REFRESH_CREDENTIALS",
		},
		{
			name:             "step2_refresh_credentials_env_without_gke_flags_does_not_hijack_k8s",
			env:              map[string]string{"GCLUSTER_REFRESH_CREDENTIALS": "1"},
			cfg:              buildSingleContextConfig("onprem-ctx", "onprem-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert-pem")}),
			wantType:         orchestrator.TypeK8s,
			wantContext:      "onprem-ctx",
			wantCluster:      "onprem-k8s",
			wantReasonSubstr: `kubeconfig context "onprem-ctx"`,
		},
		{
			name:             "step3_gke_goog_dns_endpoint_with_trailing_dot",
			cfg:              buildSingleContextConfig("dns-ctx", "dns-cluster", "https://gke-123456.us-central1.gke.goog.:443/", &clientcmdapi.AuthInfo{Token: "test-bearer-token"}),
			wantType:         orchestrator.TypeGKE,
			wantContext:      "dns-ctx",
			wantCluster:      "dns-cluster",
			wantReasonSubstr: "targets GKE endpoint gke-123456.us-central1.gke.goog",
		},
		{
			name: "step4_static_token_ip_endpoint_with_gke_server_version",
			cfg:  buildSingleContextConfig("static-gke-ctx", "static-gke", "https://34.118.224.10", &clientcmdapi.AuthInfo{Token: "test-bearer-token"}),
			versionResult: shell.CommandResult{
				ExitCode: 0,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.0"},"serverVersion":{"gitVersion":"v1.32.2-gke.1182000"}}`,
			},
			wantType:         orchestrator.TypeGKE,
			wantContext:      "static-gke-ctx",
			wantCluster:      "static-gke",
			wantReasonSubstr: "reported GKE version v1.32.2-gke.1182000",
			wantSubprocess:   1,
		},
		{
			name: "step4_cloud_sdk_client_version_against_kubeadm_server_stays_k8s",
			cfg:  buildSingleContextConfig("token-k8s-ctx", "token-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"}),
			versionResult: shell.CommandResult{
				ExitCode: 0,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.1-gke.1200"},"serverVersion":{"gitVersion":"v1.32.13"}}`,
			},
			wantType:         orchestrator.TypeK8s,
			wantContext:      "token-k8s-ctx",
			wantCluster:      "token-k8s",
			wantReasonSubstr: `kubeconfig context "token-k8s-ctx"`,
			wantSubprocess:   1,
		},
		{
			name: "step4_unreachable_server_nil_server_version_with_gke_client_version_stays_k8s",
			cfg:  buildSingleContextConfig("unreachable-k8s-ctx", "unreachable-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"}),
			versionResult: shell.CommandResult{
				ExitCode: 1,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.1-gke.1200"}}`,
			},
			wantType:       orchestrator.TypeK8s,
			wantContext:    "unreachable-k8s-ctx",
			wantCluster:    "unreachable-k8s",
			wantSubprocess: 1,
		},
		{
			name: "step4_malformed_json_stdout_stays_k8s",
			cfg:  buildSingleContextConfig("badjson-k8s-ctx", "badjson-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"}),
			versionResult: shell.CommandResult{
				ExitCode: 1,
				Stdout:   `not-json`,
			},
			wantType:       orchestrator.TypeK8s,
			wantContext:    "badjson-k8s-ctx",
			wantCluster:    "badjson-k8s",
			wantSubprocess: 1,
		},
		{
			name:        "step4_skipped_for_non_gcloud_exec_plugin_eks_aws",
			cfg:         buildSingleContextConfig("eks-ctx", "arn:aws:eks:us-west-2:123:cluster/prod", "https://abc.gr7.us-west-2.eks.amazonaws.com", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "aws"}}),
			wantType:    orchestrator.TypeK8s,
			wantContext: "eks-ctx",
			wantCluster: "arn:aws:eks:us-west-2:123:cluster/prod",
		},
		{
			name: "step4_windows_gcloud_cmd_exec_helper_probes_server_version",
			cfg:  buildSingleContextConfig("gcloud-cmd-ctx", "gcloud-cmd-clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: `C:\cloud-sdk\bin\gcloud.cmd`}}),
			versionResult: shell.CommandResult{
				ExitCode: 0,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.0"},"serverVersion":{"gitVersion":"v1.32.2-gke.1182000"}}`,
			},
			wantType:         orchestrator.TypeGKE,
			wantContext:      "gcloud-cmd-ctx",
			wantCluster:      "gcloud-cmd-clus",
			wantReasonSubstr: "reported GKE version v1.32.2-gke.1182000",
			wantSubprocess:   1,
		},
		{
			name:        "step4_skipped_under_air_gapped_env",
			env:         map[string]string{"GCLUSTER_AIR_GAPPED": "true"},
			cfg:         buildSingleContextConfig("airgap-token-ctx", "airgap-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"}),
			wantType:    orchestrator.TypeK8s,
			wantContext: "airgap-token-ctx",
			wantCluster: "airgap-k8s",
		},
		{
			name:        "kubeadm_client_cert_with_explicit_project_and_location_stays_k8s",
			cfg:         buildSingleContextConfig("kubernetes-admin@kubernetes", "kubernetes", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert-pem")}),
			projectID:   "ar-build-proj",
			location:    "us-central1",
			wantType:    orchestrator.TypeK8s,
			wantContext: "kubernetes-admin@kubernetes",
			wantCluster: "kubernetes",
		},
		{
			name:        "gkemulticloud_aws_connect_gateway_with_gke_auth_plugin_stays_k8s",
			env:         map[string]string{"GCLUSTER_REFRESH_CREDENTIALS": "1"},
			cfg:         buildSingleContextConfig("gke_aws_my-proj_us-west-2_eks-prod", "gke_aws_my-proj_us-west-2_eks-prod", "https://us-west-2-connectgateway.googleapis.com/v1/projects/123/locations/us-west-2/gkeMultiCloudClusters/eks-prod", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}),
			projectID:   "my-proj",
			location:    "us-west-2",
			wantType:    orchestrator.TypeK8s,
			wantContext: "gke_aws_my-proj_us-west-2_eks-prod",
			wantCluster: "gke_aws_my-proj_us-west-2_eks-prod",
		},
		{
			name: "gkemulticloud_azure_private_endpoint_gcloud_print_access_token_stays_k8s",
			cfg: buildSingleContextConfig("gke_azure_my-proj_eastus_aks-prod", "gke_azure_my-proj_eastus_aks-prod", "https://10.0.0.10:443", &clientcmdapi.AuthInfo{
				Exec: &clientcmdapi.ExecConfig{Command: "gcloud", Args: []string{"container", "azure", "clusters", "print-access-token", "aks-prod"}},
			}),
			versionResult: shell.CommandResult{
				ExitCode: 0,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.0"},"serverVersion":{"gitVersion":"v1.32.2-gke.1182000"}}`,
			},
			wantType:    orchestrator.TypeK8s,
			wantContext: "gke_azure_my-proj_eastus_aks-prod",
			wantCluster: "gke_azure_my-proj_eastus_aks-prod",
		},
		{
			name: "renamed_multicloud_private_endpoint_gcloud_print_access_token_stays_k8s",
			cfg: buildSingleContextConfig("aks-private-alias", "aks-private-clus", "https://10.0.0.10:443", &clientcmdapi.AuthInfo{
				Exec: &clientcmdapi.ExecConfig{Command: "gcloud", Args: []string{"container", "azure", "clusters", "print-access-token", "aks-prod"}},
			}),
			versionResult: shell.CommandResult{
				ExitCode: 0,
				Stdout:   `{"clientVersion":{"gitVersion":"v1.32.0"},"serverVersion":{"gitVersion":"v1.32.2-gke.1182000"}}`,
			},
			wantType:    orchestrator.TypeK8s,
			wantContext: "aks-private-alias",
			wantCluster: "aks-private-clus",
		},
		{
			name:        "gke_attached_mtls_connect_gateway_with_legacy_gcp_auth_provider_stays_k8s",
			cfg:         buildSingleContextConfig("gke_attached_my-proj_us-west2_onprem", "gke_attached_my-proj_us-west2_onprem", "https://connectgateway.mtls.googleapis.com/v1/projects/123/locations/us-west2/gkeAttachedClusters/onprem", &clientcmdapi.AuthInfo{AuthProvider: &clientcmdapi.AuthProviderConfig{Name: "gcp"}}),
			wantType:    orchestrator.TypeK8s,
			wantContext: "gke_attached_my-proj_us-west2_onprem",
			wantCluster: "gke_attached_my-proj_us-west2_onprem",
		},
		{
			name:        "fleet_connectgateway_context_with_gke_auth_plugin_stays_k8s",
			cfg:         buildSingleContextConfig("connectgateway_my-proj_global_member-1", "connectgateway_my-proj_global_member-1", "https://connectgateway.googleapis.com/v1/projects/123/locations/global/memberships/member-1", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}),
			wantType:    orchestrator.TypeK8s,
			wantContext: "connectgateway_my-proj_global_member-1",
			wantCluster: "connectgateway_my-proj_global_member-1",
		},
		{
			name: "gdce_edgecontainer_and_use_edge_cloud_stay_k8s",
			cfg: buildSingleContextConfig("edge-alias", "edgecontainer_my-proj_us-central1_edge-1", "https://192.168.1.10:443", &clientcmdapi.AuthInfo{
				Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin", Args: []string{"--use_edge_cloud"}},
			}),
			wantType:    orchestrator.TypeK8s,
			wantContext: "edge-alias",
			wantCluster: "edgecontainer_my-proj_us-central1_edge-1",
		},
		{
			name: "userless_kubectl_proxy_context_with_project_and_location_stays_k8s",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.Clusters["local-proxy"] = &clientcmdapi.Cluster{Server: "http://127.0.0.1:8001"}
				c.Contexts["local-proxy"] = &clientcmdapi.Context{Cluster: "local-proxy"}
				c.CurrentContext = "local-proxy"
				return c
			}(),
			env:         map[string]string{"GCLUSTER_AIR_GAPPED": "true"},
			clusterName: "local-proxy",
			projectID:   "saved-gke-proj",
			location:    "us-central1",
			wantType:    orchestrator.TypeK8s,
			wantContext: "local-proxy",
			wantCluster: "local-proxy",
		},
		{
			name:             "fresh_workstation_empty_kubeconfig_with_gke_flags_bootstraps_gke",
			cfg:              clientcmdapi.NewConfig(),
			clusterName:      "fresh-gke",
			location:         "us-central1",
			projectID:        "my-proj",
			wantType:         orchestrator.TypeGKE,
			wantContext:      "",
			wantCluster:      "fresh-gke",
			wantReasonSubstr: "no existing kubeconfig context",
		},
		{
			name:             "empty_kubeconfig_and_zero_flags_returns_empty_k8s_detection",
			cfg:              clientcmdapi.NewConfig(),
			wantType:         orchestrator.TypeK8s,
			wantContext:      "",
			wantCluster:      "",
			wantReasonSubstr: "no matching kubeconfig context found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{result: tc.versionResult}
			det := &orchestrator.Detector{
				Getenv:           func(k string) string { return tc.env[k] },
				KubeconfigLoader: func() (*clientcmdapi.Config, error) { return tc.cfg, nil },
				Runner:           runner.run,
			}
			got, err := det.Detect(tc.clusterName, tc.location, tc.projectID)
			if err != nil {
				t.Fatalf("Detect(%q, %q, %q) err = %v, want nil", tc.clusterName, tc.location, tc.projectID, err)
			}
			if got.Type != tc.wantType || got.Forced != tc.wantForced || got.KubeContext != tc.wantContext || got.ClusterName != tc.wantCluster {
				t.Errorf("Detect(%q, %q, %q) = %+v, want Type=%q Forced=%v KubeContext=%q ClusterName=%q", tc.clusterName, tc.location, tc.projectID, got, tc.wantType, tc.wantForced, tc.wantContext, tc.wantCluster)
			}
			if tc.wantReasonSubstr != "" && !strings.Contains(got.Reason, tc.wantReasonSubstr) {
				t.Errorf("Detect(%q, %q, %q) Reason = %q, want substring %q", tc.clusterName, tc.location, tc.projectID, got.Reason, tc.wantReasonSubstr)
			}
			if len(runner.calls) != tc.wantSubprocess {
				t.Fatalf("Detect(%q, %q, %q) subprocess calls = %v (len %d), want %d", tc.clusterName, tc.location, tc.projectID, runner.calls, len(runner.calls), tc.wantSubprocess)
			}
			if tc.wantSubprocess == 1 {
				wantCall := []string{"kubectl", "3s", "--context", tc.wantContext, "version", "-o", "json", "--request-timeout=2s"}
				if !slices.Equal(runner.calls[0], wantCall) {
					t.Errorf("Detect(%q, %q, %q) probe call = %v, want %v", tc.clusterName, tc.location, tc.projectID, runner.calls[0], wantCall)
				}
			}
		})
	}
}

func TestDetect_MultiFileKubeconfigPartialLoadErrorFailsFast(t *testing.T) {
	gke.RegisterProviders()
	dir := t.TempDir()
	corruptPath := filepath.Join(dir, "prod-corrupt.yaml")
	if err := os.WriteFile(corruptPath, []byte("clusters: [unclosed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	validCfg := buildSingleContextConfig("dev-k8s-ctx", "dev-k8s", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert")})
	validRaw, err := clientcmd.Write(*validCfg)
	if err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(dir, "dev-valid.yaml")
	if err := os.WriteFile(validPath, validRaw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", corruptPath+string(os.PathListSeparator)+validPath)

	det := &orchestrator.Detector{
		Getenv: func(string) string { return "" },
		Runner: (&recordingRunner{}).run,
	}
	if got, err := det.Detect("", "", ""); err == nil || !strings.Contains(err.Error(), "failed to load kubeconfig") {
		t.Fatalf("Detect() with corrupt+valid KUBECONFIG = (%+v, %v), want failed to load kubeconfig error", got, err)
	}

	detForcedK8s := &orchestrator.Detector{
		Getenv: func(k string) string {
			if k == orchestrator.EnvVar {
				return "k8s"
			}
			return ""
		},
		Runner: (&recordingRunner{}).run,
	}
	if got, err := detForcedK8s.Detect("", "", ""); err == nil || !strings.Contains(err.Error(), "failed to load kubeconfig") {
		t.Fatalf("Detect() with %s=k8s and corrupt+valid KUBECONFIG = (%+v, %v), want failed to load kubeconfig error", orchestrator.EnvVar, got, err)
	}

	// Flag-only GKE bootstrap still succeeds so gcloud get-credentials can run or report cleanly.
	gotGKE, err := det.Detect("fresh-gke", "us-central1", "my-proj")
	if err != nil || gotGKE.Type != orchestrator.TypeGKE || gotGKE.KubeContext != "" {
		t.Fatalf("Detect(flag-only GKE) with corrupt+valid KUBECONFIG = (%+v, %v), want TypeGKE bootstrap with empty KubeContext", gotGKE, err)
	}
	assertCorruptMultiFileGKEDoesNotSwallowLoadError(t, dir, corruptPath, det)
}

func assertCorruptMultiFileGKEDoesNotSwallowLoadError(t *testing.T, dir, corruptPath string, det *orchestrator.Detector) {
	t.Helper()
	gkeFallback := buildSingleContextConfig("gke_proj_us-central1_clus", "gke_proj_us-central1_clus", "https://34.118.224.10", &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}})
	gkeRaw, err := clientcmd.Write(*gkeFallback)
	if err != nil {
		t.Fatal(err)
	}
	gkePath := filepath.Join(dir, "fallback-gke.yaml")
	if err := os.WriteFile(gkePath, gkeRaw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", corruptPath+string(os.PathListSeparator)+gkePath)
	if got, err := det.Detect("", "", ""); err == nil || !strings.Contains(err.Error(), "failed to load kubeconfig") {
		t.Fatalf("Detect() with corrupt+fallback-GKE KUBECONFIG = (%+v, %v), want failed to load kubeconfig error", got, err)
	}
}

func TestDetect_HonorsDefaultEnvironmentAirGappedState(t *testing.T) {
	gke.RegisterProviders()
	t.Setenv(orchestrator.EnvVar, "")
	t.Setenv(shell.AirGappedEnvVar, "")
	origAirGap := shell.IsKnownAirGapped()
	t.Cleanup(func() { shell.SetDefaultAirGapped(origAirGap) })
	shell.SetDefaultAirGapped(true)

	cfg := buildSingleContextConfig("airgap-default-ctx", "airgap-default", "https://10.128.0.68:6443", &clientcmdapi.AuthInfo{Token: "sa-jwt"})
	runner := &recordingRunner{}
	det := &orchestrator.Detector{
		KubeconfigLoader: func() (*clientcmdapi.Config, error) { return cfg, nil },
		Runner:           runner.run,
	}
	got, err := det.Detect("", "", "")
	if err != nil || got.Type != orchestrator.TypeK8s || len(runner.calls) != 0 {
		t.Fatalf("Detect(\"\", \"\", \"\") with SetDefaultAirGapped = (%+v, %v), calls=%v, want TypeK8s with 0 subprocesses", got, err, runner.calls)
	}
}
