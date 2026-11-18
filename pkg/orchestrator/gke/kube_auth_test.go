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
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "sigs.k8s.io/yaml"

	"hpc-toolkit/pkg/shell"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestMain(m *testing.M) {
	_ = os.Unsetenv(refreshCredentialsEnv)
	os.Exit(m.Run())
}

var errLoadKubeconfigForTest = errors.New("kubeconfig unreadable")

const (
	testProject  = "my-project"
	testLocation = "us-central1"
	testCluster  = "my-cluster"
	testContext  = "gke_my-project_us-central1_my-cluster"
	testDNSHost  = "gke-abc123-123456789012.us-central1.gke.goog"
	testPublicIP = "203.0.113.10"
	testCAPEM    = "-----BEGIN CERTIFICATE-----\nfakeca\n-----END CERTIFICATE-----\n"

	getCredsIP  = "gcloud container clusters get-credentials my-cluster --location us-central1 --project my-project"
	getCredsDNS = getCredsIP + " --dns-endpoint"
	probeCmd    = "kubectl --context " + testContext + " get --raw /version --request-timeout=5s"
	setNSCmd    = "kubectl config set-context " + testContext + " --namespace=team-ns"
)

// exactExecutor is a strict, order-recording executor. Unlike MockExecutor it
// matches the full command string, which matters here because the IP-mode
// get-credentials command is a strict prefix of the DNS-mode one.
type exactExecutor struct {
	responses map[string][]shell.CommandResult
	calls     []string
	callCount map[string]int
}

func newExactExecutor(responses map[string][]shell.CommandResult) *exactExecutor {
	return &exactExecutor{responses: responses, callCount: map[string]int{}}
}

func (e *exactExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	key := name + " " + strings.Join(args, " ")
	e.calls = append(e.calls, key)
	results, ok := e.responses[key]
	if !ok {
		return shell.CommandResult{ExitCode: 1, Stderr: "exactExecutor: unexpected command: " + key}
	}
	idx := e.callCount[key]
	if idx >= len(results) {
		idx = len(results) - 1
	}
	e.callCount[key]++
	return results[idx]
}

func (e *exactExecutor) ExecuteCommandWithTimeout(_ time.Duration, name string, args ...string) shell.CommandResult {
	return e.ExecuteCommand(name, args...)
}

func (e *exactExecutor) ExecuteCommandStream(name string, args ...string) error { return nil }

func (e *exactExecutor) count(key string) int { return e.callCount[key] }

func testClusterDesc(allowDNS, publicIP bool) gkeCluster {
	desc := gkeCluster{
		Endpoint:   testPublicIP,
		MasterAuth: &gkeMasterAuth{ClusterCaCertificate: base64.StdEncoding.EncodeToString([]byte(testCAPEM))},
		ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
			DnsEndpointConfig: &dnsEndpointConfig{AllowExternalTraffic: allowDNS, Endpoint: testDNSHost},
			IPEndpointsConfig: &ipEndpointsConfig{EnablePublicEndpoint: publicIP, PublicEndpoint: testPublicIP, PrivateEndpoint: "172.16.0.34"},
		},
	}
	return desc
}

func kubeconfigWith(server string, caData []byte, exec *clientcmdapi.ExecConfig, current string) *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[testContext] = &clientcmdapi.Cluster{Server: server, CertificateAuthorityData: caData}
	authInfo := &clientcmdapi.AuthInfo{Exec: exec}
	cfg.AuthInfos[testContext] = authInfo
	cfg.Contexts[testContext] = &clientcmdapi.Context{Cluster: testContext, AuthInfo: testContext, Namespace: "team-ns"}
	cfg.CurrentContext = current
	return cfg
}

func gkeExec() *clientcmdapi.ExecConfig {
	return &clientcmdapi.ExecConfig{
		Command:         "gke-gcloud-auth-plugin",
		APIVersion:      "client.authentication.k8s.io/v1beta1",
		InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode,
	}
}

func staticLoader(cfg *clientcmdapi.Config) k8s.KubeconfigLoader {
	return func() (*clientcmdapi.Config, error) { return cfg, nil }
}

func TestKnownEndpoints(t *testing.T) {
	desc := testClusterDesc(true, true)
	desc.PrivateClusterConfig = &gkePrivateClusterConfig{PublicEndpoint: testPublicIP, PrivateEndpoint: "172.16.0.34"}

	got := desc.knownEndpoints()

	want := map[string]endpointMode{
		"https://" + testDNSHost:  endpointModeDNS,
		"https://" + testPublicIP: endpointModeIP,
		"https://172.16.0.34":     endpointModeIP,
	}
	if len(got) != len(want) {
		t.Fatalf("knownEndpoints() = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("knownEndpoints()[%q] = %v, want %v", k, got[k], v)
		}
	}
}

func TestKnownEndpoints_EmptyDescribe(t *testing.T) {
	if got := (gkeCluster{}).knownEndpoints(); len(got) != 0 {
		t.Errorf("knownEndpoints() on empty describe = %v, want empty", got)
	}
}

func TestKnownEndpoints_DNSOnlyClusterPreservesDNSMode(t *testing.T) {
	desc := gkeCluster{
		Endpoint:             testDNSHost,
		PrivateClusterConfig: &gkePrivateClusterConfig{PublicEndpoint: testPublicIP, PrivateEndpoint: "172.16.0.34"},
		ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
			DnsEndpointConfig: &dnsEndpointConfig{AllowExternalTraffic: true, Endpoint: testDNSHost},
			IPEndpointsConfig: &ipEndpointsConfig{Enabled: new(false), PublicEndpoint: testPublicIP, PrivateEndpoint: "172.16.0.34"},
		},
	}
	got := desc.knownEndpoints()
	if len(got) != 1 || got["https://"+testDNSHost] != endpointModeDNS {
		t.Fatalf("knownEndpoints() for DNS-only cluster = %v, want only https://%s -> DNS", got, testDNSHost)
	}

	// Even when Enabled is omitted (legacy response), DNS wins when Endpoint mirrors the DNS host.
	desc.ControlPlaneEndpointsConfig.IPEndpointsConfig.Enabled = nil
	if got := desc.knownEndpoints(); got["https://"+testDNSHost] != endpointModeDNS {
		t.Fatalf("knownEndpoints() with mirrored Endpoint = %v, want https://%s -> DNS", got, testDNSHost)
	}
}

func TestEvaluateExistingContext(t *testing.T) {
	caBytes := []byte(testCAPEM)
	desc := testClusterDesc(true, true)

	tests := []struct {
		name        string
		cfg         *clientcmdapi.Config
		desc        gkeCluster
		wantReuse   bool
		wantContext string
		wantMode    endpointMode
		wantReason  string
	}{
		{
			name:      "DNS-mode context with exec plugin and no CA data is reusable",
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:      "IP-mode context with matching CA is reusable",
			cfg:       kubeconfigWith("https://"+testPublicIP, caBytes, gkeExec(), testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeIP,
		},
		{
			name:      "trailing slash on server is tolerated",
			cfg:       kubeconfigWith("https://"+testPublicIP+"/", caBytes, gkeExec(), testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeIP,
		},
		{
			name:      "explicit default port :443 on server is tolerated",
			cfg:       kubeconfigWith("https://"+testPublicIP+":443", caBytes, gkeExec(), testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeIP,
		},
		{
			name:      "explicit :443 with trailing slash on DNS server is tolerated",
			cfg:       kubeconfigWith("https://"+testDNSHost+":443/", nil, gkeExec(), testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:       "non-default port on server is not reusable",
			cfg:        kubeconfigWith("https://"+testPublicIP+":8443", caBytes, gkeExec(), testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "does not match any endpoint",
		},
		{
			name:       "missing context is not reusable",
			cfg:        clientcmdapi.NewConfig(),
			desc:       desc,
			wantReuse:  false,
			wantReason: "no kubeconfig context",
		},
		{
			name:       "server not among cluster endpoints is not reusable (recreated or foreign cluster)",
			cfg:        kubeconfigWith("https://10.0.0.1", caBytes, gkeExec(), testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "does not match any endpoint",
		},
		{
			name:       "IP-mode context with CA mismatch is not reusable",
			cfg:        kubeconfigWith("https://"+testPublicIP, []byte("other-ca"), gkeExec(), testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "certificate authority",
		},
		{
			name: "IP-mode context with CA referenced by file path is not reusable",
			cfg: func() *clientcmdapi.Config {
				c := kubeconfigWith("https://"+testPublicIP, nil, gkeExec(), testContext)
				c.Clusters[testContext].CertificateAuthority = "/tmp/ca.crt"
				return c
			}(),
			desc:       desc,
			wantReuse:  false,
			wantReason: "certificate authority",
		},
		{
			name:       "IP-mode context when describe carries no CA fails closed",
			cfg:        kubeconfigWith("https://"+testPublicIP, caBytes, gkeExec(), testContext),
			desc:       func() gkeCluster { d := testClusterDesc(true, true); d.MasterAuth = nil; return d }(),
			wantReuse:  false,
			wantReason: "certificate authority",
		},
		{
			name: "static token auth is not reusable",
			cfg: func() *clientcmdapi.Config {
				c := kubeconfigWith("https://"+testDNSHost, nil, nil, testContext)
				c.AuthInfos[testContext].Token = "abc"
				return c
			}(),
			desc:       desc,
			wantReuse:  false,
			wantReason: "gke-gcloud-auth-plugin",
		},
		{
			name:       "exec plugin other than gke-gcloud-auth-plugin is not reusable",
			cfg:        kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "aws"}, testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "gke-gcloud-auth-plugin",
		},
		{
			name:      "exec plugin given by absolute path is accepted",
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "/usr/lib/google-cloud-sdk/bin/gke-gcloud-auth-plugin", APIVersion: "client.authentication.k8s.io/v1beta1", InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode}, testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:      "exec plugin with Windows .exe suffix is accepted",
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin.exe", APIVersion: "client.authentication.k8s.io/v1beta1", InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode}, testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:      "exec plugin with uppercase .EXE suffix and backslash Windows install path is accepted",
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: `C:\Program Files\Google\Cloud SDK\google-cloud-sdk\bin\GKE-GCLOUD-AUTH-PLUGIN.EXE`, APIVersion: "client.authentication.k8s.io/v1beta1", InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode}, testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:      "exec plugin with v1 apiVersion and Never interactiveMode is accepted",
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin", APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: clientcmdapi.NeverExecInteractiveMode}, testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:       "v1 exec plugin missing interactiveMode is not reusable",
			cfg:        kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin", APIVersion: "client.authentication.k8s.io/v1"}, testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "exec interactiveMode",
		},
		{
			name:       "Always exec interactiveMode is not reusable in non-TTY subprocesses",
			cfg:        kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin", APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: clientcmdapi.AlwaysExecInteractiveMode}, testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "exec interactiveMode",
		},
		{
			name:       "invalid exec interactiveMode is not reusable",
			cfg:        kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin", APIVersion: "client.authentication.k8s.io/v1beta1", InteractiveMode: "Bogus"}, testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "exec interactiveMode",
		},
		{
			name: "combined static token and exec plugin is not reusable",
			cfg: func() *clientcmdapi.Config {
				c := kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext)
				c.AuthInfos[testContext].Token = "stale-static-token"
				return c
			}(),
			desc:       desc,
			wantReuse:  false,
			wantReason: "static credentials with exec",
		},
		{
			name: "combined client certificate and exec plugin is not reusable",
			cfg: func() *clientcmdapi.Config {
				c := kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext)
				c.AuthInfos[testContext].ClientCertificateData = []byte("cert")
				return c
			}(),
			desc:       desc,
			wantReuse:  false,
			wantReason: "static credentials with exec",
		},
		{
			name: "combined auth-provider and exec plugin is not reusable",
			cfg: func() *clientcmdapi.Config {
				c := kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext)
				c.AuthInfos[testContext].AuthProvider = &clientcmdapi.AuthProviderConfig{Name: "gcp"}
				return c
			}(),
			desc:       desc,
			wantReuse:  false,
			wantReason: "auth-provider and exec",
		},
		{
			name: "context referencing missing cluster entry is not reusable",
			cfg: func() *clientcmdapi.Config {
				c := kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext)
				delete(c.Clusters, testContext)
				return c
			}(),
			desc:       desc,
			wantReuse:  false,
			wantReason: "references unknown cluster entry",
		},
		{
			name:       "legacy v1alpha1 exec plugin is not reusable",
			cfg:        kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin", APIVersion: "client.authentication.k8s.io/v1alpha1", InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode}, testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "unsupported exec apiVersion",
		},
		{
			name:       "empty exec apiVersion is not reusable",
			cfg:        kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin", InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode}, testContext),
			desc:       desc,
			wantReuse:  false,
			wantReason: "unsupported exec apiVersion",
		},
		{
			name: "DNS-only cluster with Endpoint mirroring DNS host and no CA data is reusable",
			cfg:  kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext),
			desc: gkeCluster{
				Endpoint: testDNSHost,
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{AllowExternalTraffic: true, Endpoint: testDNSHost},
					IPEndpointsConfig: &ipEndpointsConfig{Enabled: new(false)},
				},
			},
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name: "renamed GKE context alias referencing canonical cluster entry is reusable",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.Clusters[testContext] = &clientcmdapi.Cluster{Server: "https://" + testDNSHost}
				c.AuthInfos[testContext] = &clientcmdapi.AuthInfo{Exec: gkeExec()}
				c.Contexts["prod-gpu"] = &clientcmdapi.Context{Cluster: testContext, AuthInfo: testContext, Namespace: "team-ns"}
				return c
			}(),
			desc:        desc,
			wantReuse:   true,
			wantContext: "prod-gpu",
			wantMode:    endpointModeDNS,
		},
		{
			name: "legacy context named after the cluster is found",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.Clusters[testCluster] = &clientcmdapi.Cluster{Server: "https://" + testDNSHost}
				c.AuthInfos["u"] = &clientcmdapi.AuthInfo{Exec: gkeExec()}
				c.Contexts[testCluster] = &clientcmdapi.Context{Cluster: testCluster, AuthInfo: "u"}
				return c
			}(),
			desc:        desc,
			wantReuse:   true,
			wantContext: testCluster,
			wantMode:    endpointModeDNS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateExistingContext(tt.cfg, testCluster, testLocation, testProject, tt.desc)
			if got.reusable() != tt.wantReuse {
				t.Fatalf("reusable() = %v, want %v (reason=%q)", got.reusable(), tt.wantReuse, got.reason)
			}
			if tt.wantReuse {
				wantCtx := tt.wantContext
				if wantCtx == "" {
					wantCtx = testContext
				}
				if got.contextName != wantCtx || got.mode != tt.wantMode {
					t.Errorf("check = (context=%q, mode=%v), want (%q, %v)", got.contextName, got.mode, wantCtx, tt.wantMode)
				}
			}
			if !tt.wantReuse && !strings.Contains(got.reason, tt.wantReason) {
				t.Errorf("reason = %q, want substring %q", got.reason, tt.wantReason)
			}
		})
	}
}

func TestEvaluateExistingContext_YAMLExecInteractiveModeDefaults(t *testing.T) {
	desc := testClusterDesc(true, true)
	load := func(apiVersion string) *clientcmdapi.Config {
		t.Helper()
		raw := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %s
  cluster:
    server: https://%s
contexts:
- name: %s
  context:
    cluster: %s
    user: %s
users:
- name: %s
  user:
    exec:
      apiVersion: %s
      command: gke-gcloud-auth-plugin
`, testContext, testDNSHost, testContext, testContext, testContext, testContext, apiVersion)
		cfg, err := clientcmd.Load([]byte(raw))
		if err != nil {
			t.Fatalf("clientcmd.Load(%s) error = %v", apiVersion, err)
		}
		return cfg
	}

	// client-go defaults omitted interactiveMode to IfAvailable for v1beta1 YAML, but not for v1 YAML.
	if got := evaluateExistingContext(load("client.authentication.k8s.io/v1beta1"), testCluster, testLocation, testProject, desc); !got.reusable() {
		t.Fatalf("v1beta1 YAML without interactiveMode should be reusable after client-go defaulting; got reason=%q", got.reason)
	}
	if got := evaluateExistingContext(load("client.authentication.k8s.io/v1"), testCluster, testLocation, testProject, desc); got.reusable() || !strings.Contains(got.reason, "interactiveMode") {
		t.Fatalf("v1 YAML without interactiveMode must not be reusable; got reusable=%v reason=%q", got.reusable(), got.reason)
	}
}

func TestClassifyProbeFailure(t *testing.T) {
	tests := []struct {
		stderr string
		want   probeOutcome
	}{
		{"Unable to connect to the server: dial tcp 203.0.113.10:443: i/o timeout", probeConnectivity},
		{"Unable to connect to the server: context deadline exceeded", probeConnectivity},
		{"Unable to connect to the server: dial tcp 203.0.113.10:443: connect: connection refused", probeConnectivity},
		{"Unable to connect to the server: dial tcp: lookup gke-abc.gke.goog: no such host", probeConnectivity},
		{"Unable to connect to the server: net/http: TLS handshake timeout", probeConnectivity},
		{"Unable to connect to the server: tls: failed to verify certificate: x509: certificate signed by unknown authority", probeConnectivity},
		{"error: the server responded with the status code 431 but did not return more information", probeConnectivity},
		{"Request Header Fields Too Large", probeConnectivity},
		// Forward proxies answer for unreachable upstreams with gateway errors rather than a raw dial failure.
		{"error: the server responded with the status code 502 but did not return more information", probeConnectivity},
		{"Error from server (BadGateway): Bad Gateway", probeConnectivity},
		{"error: the server responded with the status code 503 but did not return more information", probeConnectivity},
		{"Error from server (ServiceUnavailable): Service Unavailable", probeConnectivity},
		{"error: the server responded with the status code 504 but did not return more information", probeConnectivity},
		{"Error from server (Timeout): Gateway Timeout", probeConnectivity},
		{"error: You must be logged in to the server (Unauthorized)", probeAuth},
		{"error: You must be logged in to the server (the server has asked for the client to provide credentials)", probeAuth},
		{"Unable to connect to the server: getting credentials: exec: executable gke-gcloud-auth-plugin not found", probeAuth},
		{"Unable to connect to the server: getting credentials: exec: executable gke-gcloud-auth-plugin failed with exit code 1", probeAuth},
		{"could not get token: oauth2: cannot fetch token: 400 Bad Request", probeAuth},
		{"Error from server (Forbidden): forbidden: User cannot get path \"/version\"", probeForbidden},
		// GKE's DNS endpoint returns HTTP 403 when external traffic is disabled, IAM denies clusters.connect, or VPC-SC blocks access.
		{"Error from server (Forbidden): control_plane_endpoints_config.dns_endpoint_config.allow_external_traffic is disabled, external access via DNS endpoint is not allowed", probeConnectivity},
		{"Error from server (Forbidden): Permission 'container.googleapis.com/clusters.connect' denied on resource '//container.googleapis.com/projects/p/locations/l/clusters/c'", probeConnectivity},
		{"Error from server (Forbidden): Request is prohibited by organization's policy. vpcServiceControlsUniqueIdentifier: abc123", probeConnectivity},
		// A forward proxy refusing the CONNECT is a connectivity problem, not an apiserver RBAC rejection.
		{"Unable to connect to the server: proxyconnect tcp: dial tcp 203.0.113.10:443: proxy returned status 403 Forbidden", probeConnectivity},
		// Incidental substrings must not be misread as connectivity failures.
		{"mock error: unexpected command: kubectl --context c get --raw /version --request-timeout=5s", probeUnknown},
		{"Error from server (NotFound): pods \"worker-431-tls\" not found", probeUnknown},
		{"something completely different", probeUnknown},
		{"", probeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.stderr, func(t *testing.T) {
			if got := classifyProbeFailure(tt.stderr); got != tt.want {
				t.Errorf("classifyProbeFailure(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}

func TestConfigureKubectl_ReusesWorkingContext(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd: {{ExitCode: 0, Stdout: `{"major":"1"}`}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count(getCredsIP)+exec.count(getCredsDNS) != 0 {
		t.Errorf("get-credentials must not run when the existing context is valid and reachable; calls=%v", exec.calls)
	}
	if exec.count(probeCmd) != 1 {
		t.Errorf("probe count = %d, want 1; calls=%v", exec.count(probeCmd), exec.calls)
	}
}

// Reuse must not write to kubeconfig: current-context stays wherever the user left it.
func TestConfigureKubectl_ReuseDoesNotSwitchCurrentContext(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd: {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), "some-other-context"))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if orc.kubeContext != testContext {
		t.Errorf("kubeContext = %q, want %q", orc.kubeContext, testContext)
	}
	if got := []string{probeCmd}; !slices.Equal(exec.calls, got) {
		t.Errorf("calls = %v, want only the probe (no use-context, no get-credentials)", exec.calls)
	}
}

func TestConfigureKubectl_ReuseFallsBackToOtherEndpointOnConnectivityFailure(t *testing.T) {
	// Existing context is DNS mode but DNS is blocked for this client
	// (e.g. HTTP 431 behind a proxy). Expect regeneration in IP mode.
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd:   {{ExitCode: 1, Stderr: "Request Header Fields Too Large"}, {ExitCode: 0}},
		getCredsIP: {{ExitCode: 0}},
		setNSCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.kubeClient = &MockKubeClient{Namespace: "team-ns"}
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count(getCredsIP) != 1 || exec.count(getCredsDNS) != 0 {
		t.Errorf("expected exactly one IP-mode get-credentials and no DNS-mode; calls=%v", exec.calls)
	}
	if exec.count(probeCmd) != 2 {
		t.Errorf("probe count = %d, want 2; calls=%v", exec.count(probeCmd), exec.calls)
	}
}

func TestConfigureKubectl_ReuseIPContextDemotesIPAndTriesDNSFirst(t *testing.T) {
	// Existing context is IP mode (which is also the cluster's policy default),
	// and probing it times out. endpointModeOrder must swap [IP, DNS] -> [DNS, IP]
	// so get-credentials --dns-endpoint is tried before IP mode.
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd:    {{ExitCode: 1, Stderr: "Unable to connect to the server: dial tcp 203.0.113.10:443: i/o timeout"}, {ExitCode: 0}},
		getCredsDNS: {{ExitCode: 0}},
		setNSCmd:    {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.kubeClient = &MockKubeClient{Namespace: "team-ns"}
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testPublicIP, []byte(testCAPEM), gkeExec(), testContext))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	wantOrder := []string{probeCmd, getCredsDNS, setNSCmd, probeCmd}
	if len(exec.calls) != len(wantOrder) {
		t.Fatalf("calls = %v, want %v", exec.calls, wantOrder)
	}
	for i := range wantOrder {
		if exec.calls[i] != wantOrder[i] {
			t.Errorf("call[%d] = %q, want %q", i, exec.calls[i], wantOrder[i])
		}
	}
}

func TestConfigureKubectl_ReuseUnknownProbeFailureReusesWithWarning(t *testing.T) {
	// An unrecognized probe failure on a structurally valid existing context must
	// not trigger get-credentials (which would overwrite a DNS context with IP).
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd: {{ExitCode: 1, Stderr: "mock error: unexpected command"}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), "some-other-context"))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v, want nil on unknown probe failure", err)
	}
	if orc.kubeContext != testContext {
		t.Errorf("kubeContext = %q, want the reused context %q", orc.kubeContext, testContext)
	}
	if exec.count(getCredsIP)+exec.count(getCredsDNS) != 0 {
		t.Errorf("get-credentials must not run when probe outcome is unknown on reuse path; calls=%v", exec.calls)
	}
}

func TestConfigureKubectl_ReuseAuthFailureFailsFastWithoutRegenerating(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd: {{ExitCode: 1, Stderr: "error: You must be logged in to the server (Unauthorized)"}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

	err := orc.configureKubectl(testCluster, testLocation, testProject)
	if err == nil {
		t.Fatal("configureKubectl() expected error on auth failure, got nil")
	}
	if !strings.Contains(err.Error(), "gcloud auth application-default login") {
		t.Errorf("error should guide the user to re-authenticate, got: %v", err)
	}
	if exec.count(getCredsIP)+exec.count(getCredsDNS) != 0 {
		t.Errorf("get-credentials must not run on an auth failure; calls=%v", exec.calls)
	}
}

func TestConfigureKubectl_NoContext_DefaultIPThenFallsBackToDNS(t *testing.T) {
	// Cluster has both endpoints and the default is IP, but only the DNS
	// endpoint is reachable from this client (e.g. a VM without external egress).
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP:  {{ExitCode: 0}},
		getCredsDNS: {{ExitCode: 0}},
		probeCmd:    {{ExitCode: 1, Stderr: "Unable to connect to the server: dial tcp 203.0.113.10:443: i/o timeout"}, {ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	wantOrder := []string{getCredsIP, probeCmd, getCredsDNS, probeCmd}
	if len(exec.calls) != len(wantOrder) {
		t.Fatalf("calls = %v, want %v", exec.calls, wantOrder)
	}
	for i := range wantOrder {
		if exec.calls[i] != wantOrder[i] {
			t.Errorf("call[%d] = %q, want %q", i, exec.calls[i], wantOrder[i])
		}
	}
}

func TestConfigureKubectl_NoContext_PolicyDefaultDNSWhenNoPublicIP(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsDNS: {{ExitCode: 0}},
		probeCmd:    {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, false)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count(getCredsDNS) != 1 || exec.count(getCredsIP) != 0 {
		t.Errorf("expected DNS-mode get-credentials only; calls=%v", exec.calls)
	}
}

func TestConfigureKubectl_NoContext_BothEndpointsUnreachableErrors(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP:  {{ExitCode: 0}},
		getCredsDNS: {{ExitCode: 0}},
		probeCmd:    {{ExitCode: 1, Stderr: "dial tcp 203.0.113.10:443: i/o timeout"}, {ExitCode: 1, Stderr: "Request Header Fields Too Large"}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	err := orc.configureKubectl(testCluster, testLocation, testProject)
	if err == nil {
		t.Fatal("expected error when both endpoints are unreachable")
	}
	for _, want := range []string{"i/o timeout", "Request Header Fields Too Large", "IP endpoint", "DNS endpoint"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err.Error(), want)
		}
	}
	if exec.count(getCredsIP) != 1 || exec.count(getCredsDNS) != 1 {
		t.Errorf("expected one attempt per endpoint mode; calls=%v", exec.calls)
	}
}

func TestConfigureKubectl_NoContext_DNSNotAllowedDoesNotFallBack(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 1, Stderr: "dial tcp 203.0.113.10:443: i/o timeout"}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(false, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	err := orc.configureKubectl(testCluster, testLocation, testProject)
	if err == nil {
		t.Fatal("expected error when the only available endpoint is unreachable")
	}
	if exec.count(getCredsDNS) != 0 {
		t.Errorf("must not attempt DNS mode when external DNS traffic is disallowed; calls=%v", exec.calls)
	}
	if !strings.Contains(err.Error(), "authorized networks") {
		t.Errorf("error should mention authorized networks remediation, got: %v", err)
	}
}

func TestConfigureKubectl_UnknownProbeFailureProceedsWithWarning(t *testing.T) {
	// Unknown probe failures must not block submission; downstream kubectl
	// calls will surface the real error with full context.
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 1, Stderr: "mock error: unexpected command"}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v, want nil on unknown probe failure", err)
	}
	if exec.count(getCredsDNS) != 0 {
		t.Errorf("must not flip endpoint on an unknown failure; calls=%v", exec.calls)
	}
}

func TestConfigureKubectl_ForbiddenProbeIsTreatedAsReachable(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd: {{ExitCode: 1, Stderr: `Error from server (Forbidden): forbidden: User cannot get path "/version"`}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count(getCredsIP)+exec.count(getCredsDNS) != 0 {
		t.Errorf("403 on /version means the endpoint is reachable; must reuse; calls=%v", exec.calls)
	}
}

func TestConfigureKubectl_RefreshEnvForcesRegeneration(t *testing.T) {
	t.Setenv(refreshCredentialsEnv, "1")
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
		setNSCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.kubeClient = &MockKubeClient{Namespace: "team-ns"}
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count(getCredsIP) != 1 {
		t.Errorf("%s=1 must force get-credentials even when a valid context exists; calls=%v", refreshCredentialsEnv, exec.calls)
	}
}

func TestConfigureKubectl_RefreshEnvFalseValuesDoNotForce(t *testing.T) {
	for _, v := range []string{"", "0", "false", "FALSE", "no", "off", "OFF"} {
		t.Run("value="+v, func(t *testing.T) {
			t.Setenv(refreshCredentialsEnv, v)
			exec := newExactExecutor(map[string][]shell.CommandResult{probeCmd: {{ExitCode: 0}}})
			orc := newTestGKEOrchestrator(exec)
			orc.clusterDesc = testClusterDesc(true, true)
			orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

			if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
				t.Fatalf("configureKubectl() error = %v", err)
			}
			if exec.count(getCredsIP)+exec.count(getCredsDNS) != 0 {
				t.Errorf("value %q must not force regeneration; calls=%v", v, exec.calls)
			}
		})
	}
}

func TestConfigureKubectl_KubeconfigLoadErrorFallsBackToRegeneration(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) { return nil, errLoadKubeconfigForTest }

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v; an unreadable kubeconfig should fall back to get-credentials", err)
	}
	if exec.count(getCredsIP) != 1 {
		t.Errorf("expected get-credentials after kubeconfig load failure; calls=%v", exec.calls)
	}
}

// The namespace restored after get-credentials is the one already in kubeconfig.
func TestConfigureKubectl_RegenerationPreservesNamespace(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
		setNSCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	// Context exists with namespace team-ns but points at an unknown server, so it is regenerated.
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://198.51.100.9", nil, gkeExec(), testContext))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count(setNSCmd) != 1 {
		t.Errorf("namespace must be restored after get-credentials; calls=%v", exec.calls)
	}
}

// A one-off --gke-namespace must not be written into the user's kubeconfig context.
func TestConfigureKubectl_RegenerationDoesNotPersistFlagNamespace(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.namespace = "flag-ns" // as seeded from --gke-namespace by every entry point
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	for _, c := range exec.calls {
		if strings.Contains(c, "set-context") {
			t.Errorf("flag namespace leaked into kubeconfig: %q", c)
		}
	}
}

func TestConfigureKubectl_GetCredentialsFailureIsReturned(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 1, Stderr: "ERROR: (gcloud.container.clusters.get-credentials) ResponseError: code=403"}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	err := orc.configureKubectl(testCluster, testLocation, testProject)
	if err == nil || !strings.Contains(err.Error(), "failed to get GKE cluster credentials") {
		t.Fatalf("configureKubectl() error = %v, want get-credentials failure", err)
	}
	if exec.count(probeCmd) != 0 {
		t.Errorf("must not probe after get-credentials failed; calls=%v", exec.calls)
	}
}

func TestShouldUseDNSEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		clusterDesc  gkeCluster
		wantEndpoint bool
	}{
		{
			name:         "Nil ControlPlaneEndpointsConfig returns false",
			clusterDesc:  gkeCluster{},
			wantEndpoint: false,
		},
		{
			name: "Nil DnsEndpointConfig returns false",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{},
			},
			wantEndpoint: false,
		},
		{
			name: "DnsEndpointConfig external traffic disallowed returns false",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{
						AllowExternalTraffic: false,
					},
				},
			},
			wantEndpoint: false,
		},
		{
			name: "DnsEndpointConfig external traffic allowed with nil IPEndpointsConfig returns true",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{
						AllowExternalTraffic: true,
					},
				},
			},
			wantEndpoint: true,
		},
		{
			name: "Public IP endpoint enabled bypasses DNS endpoint to prevent HTTP 431 header issues",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{
						AllowExternalTraffic: true,
					},
					IPEndpointsConfig: &ipEndpointsConfig{
						EnablePublicEndpoint: true,
					},
				},
			},
			wantEndpoint: false,
		},
		{
			name: "Public IP endpoint disabled uses DNS endpoint for external connectivity",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{
						AllowExternalTraffic: true,
					},
					IPEndpointsConfig: &ipEndpointsConfig{
						EnablePublicEndpoint: false,
					},
				},
			},
			wantEndpoint: true,
		},
		{
			name: "IP endpoints explicitly disabled uses DNS endpoint even if EnablePublicEndpoint is true",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{
						AllowExternalTraffic: true,
					},
					IPEndpointsConfig: &ipEndpointsConfig{
						Enabled:              new(false),
						EnablePublicEndpoint: true,
					},
				},
			},
			wantEndpoint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseDNSEndpoint(tt.clusterDesc.ControlPlaneEndpointsConfig)
			if got != tt.wantEndpoint {
				t.Errorf("shouldUseDNSEndpoint() = %v, want %v", got, tt.wantEndpoint)
			}
		})
	}
}

func TestEndpointModeOrder_DNSOnlyOmitsIPFallback(t *testing.T) {
	orc := &GKEOrchestrator{
		clusterDesc: gkeCluster{
			Endpoint: testDNSHost,
			ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
				DnsEndpointConfig: &dnsEndpointConfig{AllowExternalTraffic: true, Endpoint: testDNSHost},
				IPEndpointsConfig: &ipEndpointsConfig{Enabled: new(false)},
			},
		},
	}
	if got := orc.endpointModeOrder(endpointModeUnknown); !slices.Equal(got, []endpointMode{endpointModeDNS}) {
		t.Fatalf("endpointModeOrder(DNS-only) = %v, want [DNS]", got)
	}
	if got := orc.endpointModeOrder(endpointModeDNS); !slices.Equal(got, []endpointMode{endpointModeDNS}) {
		t.Fatalf("endpointModeOrder(DNS-only, avoid=DNS) = %v, want [DNS]", got)
	}

	// When both IP endpoints and external DNS are disabled, endpointModeOrder returns empty and regenerateKubeContext fails fast.
	exec := newExactExecutor(nil)
	disabledOrc := newTestGKEOrchestrator(exec)
	disabledOrc.clusterDesc = gkeCluster{
		ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
			DnsEndpointConfig: &dnsEndpointConfig{AllowExternalTraffic: false, Endpoint: testDNSHost},
			IPEndpointsConfig: &ipEndpointsConfig{Enabled: new(false)},
		},
	}
	if got := disabledOrc.endpointModeOrder(endpointModeUnknown); len(got) != 0 {
		t.Fatalf("endpointModeOrder(all-disabled) = %v, want empty", got)
	}
	err := disabledOrc.regenerateKubeContext(testCluster, testLocation, testProject, endpointModeUnknown)
	if err == nil || !strings.Contains(err.Error(), "no usable control-plane endpoint") || len(exec.calls) != 0 {
		t.Fatalf("regenerateKubeContext(all-disabled) = (%v, calls=%v), want fast-fail with 0 calls", err, exec.calls)
	}
}

func TestConfigureKubectl_DisabledExternalDNSFallsBackToIP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
	}{
		{name: "allow_external_traffic disabled", stderr: "Error from server (Forbidden): control_plane_endpoints_config.dns_endpoint_config.allow_external_traffic is disabled"},
		{name: "IAM clusters.connect denied", stderr: "Error from server (Forbidden): Permission 'container.googleapis.com/clusters.connect' denied on resource"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newExactExecutor(map[string][]shell.CommandResult{
				probeCmd:   {{ExitCode: 1, Stderr: tc.stderr}, {ExitCode: 0}},
				getCredsIP: {{ExitCode: 0}},
				setNSCmd:   {{ExitCode: 0}},
			})
			orc := newTestGKEOrchestrator(exec)
			orc.clusterDesc = testClusterDesc(false, true)
			orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))

			if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
				t.Fatalf("configureKubectl() error = %v; calls=%v", err, exec.calls)
			}
			wantCalls := []string{probeCmd, getCredsIP, setNSCmd, probeCmd}
			if orc.kubeContext != testContext || !slices.Equal(exec.calls, wantCalls) {
				t.Fatalf("kubeContext=%q calls=%v, want (%q, %v)", orc.kubeContext, exec.calls, testContext, wantCalls)
			}
		})
	}
}

func TestConfigureKubectl_DNSOnlyReuseAndFailureMessage(t *testing.T) {
	dnsOnlyDesc := gkeCluster{
		Endpoint: testDNSHost,
		ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
			DnsEndpointConfig: &dnsEndpointConfig{AllowExternalTraffic: true, Endpoint: testDNSHost},
			IPEndpointsConfig: &ipEndpointsConfig{Enabled: new(false)},
		},
	}

	// End-to-end reuse on a DNS-only cluster with no CA data.
	reuseExec := newExactExecutor(map[string][]shell.CommandResult{probeCmd: {{ExitCode: 0}}})
	reuseOrc := newTestGKEOrchestrator(reuseExec)
	reuseOrc.clusterDesc = dnsOnlyDesc
	reuseOrc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext))
	if err := reuseOrc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl(DNS-only reuse) error = %v", err)
	}
	if reuseOrc.kubeContext != testContext || !slices.Equal(reuseExec.calls, []string{probeCmd}) {
		t.Fatalf("DNS-only reuse got kubeContext=%q calls=%v, want (%q, [%q])", reuseOrc.kubeContext, reuseExec.calls, testContext, probeCmd)
	}

	// Failed DNS-only cluster omits IP-only "authorized networks" hint and points to DNS/IAM remediation.
	failExec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsDNS: {{ExitCode: 0}},
		probeCmd:    {{ExitCode: 1, Stderr: "Error from server (Forbidden): Permission 'container.googleapis.com/clusters.connect' denied"}},
	})
	failOrc := newTestGKEOrchestrator(failExec)
	failOrc.clusterDesc = dnsOnlyDesc
	failOrc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())
	err := failOrc.configureKubectl(testCluster, testLocation, testProject)
	if err == nil || strings.Contains(err.Error(), "authorized networks") || !strings.Contains(err.Error(), "container.clusters.connect") {
		t.Fatalf("DNS-only failure err = %v, want DNS/IAM hint without authorized networks", err)
	}
	if failExec.count(getCredsIP) != 0 || failExec.count(getCredsDNS) != 1 {
		t.Fatalf("DNS-only failure must not fall back to IP; calls=%v", failExec.calls)
	}
}

func TestRefreshGKEAuth_DNSEndpoint(t *testing.T) {
	tests := []struct {
		name          string
		clusterDesc   gkeCluster
		mockResponses map[string][]shell.CommandResult
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name: "Appends --dns-endpoint when shouldUseDNSEndpoint is true",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{
						AllowExternalTraffic: true,
					},
					IPEndpointsConfig: &ipEndpointsConfig{
						EnablePublicEndpoint: false,
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud container clusters get-credentials my-cluster --location us-central1-a --project my-project --dns-endpoint": {
					{ExitCode: 0, Stdout: "kubeconfig entry generated"},
				},
			},
			wantErr: false,
		},
		{
			name: "Omits --dns-endpoint when public IP endpoint is available",
			clusterDesc: gkeCluster{
				ControlPlaneEndpointsConfig: &controlPlaneEndpointsConfig{
					DnsEndpointConfig: &dnsEndpointConfig{
						AllowExternalTraffic: true,
					},
					IPEndpointsConfig: &ipEndpointsConfig{
						EnablePublicEndpoint: true,
					},
				},
			},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud container clusters get-credentials my-cluster --location us-central1-a --project my-project": {
					{ExitCode: 0, Stdout: "kubeconfig entry generated"},
				},
			},
			wantErr: false,
		},
		{
			name:        "Returns timeout error when get-credentials times out",
			clusterDesc: gkeCluster{},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud container clusters get-credentials my-cluster --location us-central1-a --project my-project": {
					{ExitCode: 124, Err: context.DeadlineExceeded},
				},
			},
			wantErr:       true,
			wantErrSubstr: "timed out after",
		},
		{
			name:        "Returns execution error when gcloud fails to start",
			clusterDesc: gkeCluster{},
			mockResponses: map[string][]shell.CommandResult{
				"gcloud container clusters get-credentials my-cluster --location us-central1-a --project my-project": {
					{ExitCode: -1, Err: fmt.Errorf("executable file not found in $PATH")},
				},
			},
			wantErr:       true,
			wantErrSubstr: "failed to execute gcloud: executable file not found in $PATH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := NewMockExecutor(tt.mockResponses)
			orc := newTestGKEOrchestrator(mockExec)
			orc.clusterDesc = tt.clusterDesc
			err := orc.refreshGKEAuth("my-cluster", "us-central1-a", "my-project", shouldUseDNSEndpoint(tt.clusterDesc.ControlPlaneEndpointsConfig))
			if (err != nil) != tt.wantErr {
				t.Errorf("refreshGKEAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrSubstr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr)) {
				t.Errorf("refreshGKEAuth() error = %v, want substring %q", err, tt.wantErrSubstr)
			}
		})
	}
}

func TestKubectlCommandLine(t *testing.T) {
	const path = `C:\Users\Jane Doe\.gcluster\generated\jobset.yaml`
	args := []string{"apply", "-f", path}
	tests := []struct {
		name, kubeContext, want string
	}{
		{name: "no context", want: `kubectl apply -f "` + path + `"`},
		{name: "pinned context", kubeContext: testContext, want: `kubectl --context ` + testContext + ` apply -f "` + path + `"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := kubectlCommandLine(tc.kubeContext, args...); got != tc.want {
				t.Errorf("kubectlCommandLine() = %s, want %s", got, tc.want)
			}
			if !slices.Equal(args, []string{"apply", "-f", path}) {
				t.Errorf("kubectlCommandLine() modified its arguments: %q", args)
			}
		})
	}
}

func TestKubectlHelpers_PinContext(t *testing.T) {
	rec := &streamRecorder{exactExecutor: *newExactExecutor(map[string][]shell.CommandResult{
		"kubectl --context " + testContext + " get pods":   {{ExitCode: 0}},
		"kubectl --context " + testContext + " exec pod/p": {{ExitCode: 0}},
	})}
	g := &GKEOrchestrator{executor: rec, kubeContext: testContext}

	if res := g.kubectl("get", "pods"); res.ExitCode != 0 {
		t.Errorf("kubectl call was not pinned; calls=%v", rec.calls)
	}
	if res := g.kubectlWithTimeout(time.Second, "exec", "pod/p"); res.ExitCode != 0 {
		t.Errorf("kubectlWithTimeout call was not pinned; calls=%v", rec.calls)
	}
	if err := g.kubectlStream("logs", "-f"); err != nil {
		t.Fatal(err)
	}
	want := "kubectl --context " + testContext + " logs -f"
	if len(rec.streamed) != 1 || rec.streamed[0] != want {
		t.Errorf("streamed = %v, want [%q]", rec.streamed, want)
	}
}

type streamRecorder struct {
	exactExecutor
	streamed []string
}

func (s *streamRecorder) ExecuteCommandStream(name string, args ...string) error {
	s.streamed = append(s.streamed, name+" "+strings.Join(args, " "))
	return nil
}

// Later calls must hit the configured cluster even after current-context changes.
func TestConfigureKubectl_PinsReusedContextAgainstCurrentContextChange(t *testing.T) {
	const applyCmd = "kubectl --context " + testContext + " apply -f job.yaml"
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd: {{ExitCode: 0}},
		applyCmd: {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	cfg := kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), testContext)
	orc.kubeconfigLoader = staticLoader(cfg)

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if orc.kubeContext != testContext {
		t.Fatalf("kubeContext = %q, want %q", orc.kubeContext, testContext)
	}

	cfg.CurrentContext = "gke_other-project_us-central1_shared-cluster"
	if res := orc.kubectl("apply", "-f", "job.yaml"); res.ExitCode != 0 {
		t.Errorf("apply was not pinned to %q; calls=%v", testContext, exec.calls)
	}
}

func TestConfigureKubectl_PinsRegeneratedContext(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if orc.kubeContext != testContext {
		t.Errorf("kubeContext = %q, want %q", orc.kubeContext, testContext)
	}
}

func TestConfigureKubectl_FailureClearsPinnedContext(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 1, Stderr: "ERROR: permission denied"}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.kubeContext = "stale-context"
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err == nil {
		t.Fatal("configureKubectl() error = nil, want error")
	}
	if orc.kubeContext != "" {
		t.Errorf("kubeContext = %q, want empty after failure", orc.kubeContext)
	}
}

func TestRestConfig_UsesPinnedContext(t *testing.T) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["current"] = &clientcmdapi.Cluster{Server: "https://current.example.com"}
	cfg.Clusters["pinned"] = &clientcmdapi.Cluster{Server: "https://pinned.example.com"}
	cfg.AuthInfos["u"] = &clientcmdapi.AuthInfo{Token: "t"}
	cfg.Contexts["current"] = &clientcmdapi.Context{Cluster: "current", AuthInfo: "u"}
	cfg.Contexts["pinned"] = &clientcmdapi.Context{Cluster: "pinned", AuthInfo: "u"}
	cfg.CurrentContext = "current"

	tests := []struct {
		kubeContext string
		wantHost    string
	}{
		{kubeContext: "pinned", wantHost: "https://pinned.example.com"},
		{kubeContext: "", wantHost: "https://current.example.com"},
	}
	for _, tt := range tests {
		g := &GKEOrchestrator{kubeconfigLoader: staticLoader(cfg), kubeContext: tt.kubeContext}
		rc, err := g.restConfig()
		if err != nil {
			t.Fatalf("restConfig() with kubeContext %q error = %v", tt.kubeContext, err)
		}
		if rc.Host != tt.wantHost {
			t.Errorf("restConfig() with kubeContext %q Host = %q, want %q", tt.kubeContext, rc.Host, tt.wantHost)
		}
	}
}

// With a pinned context the namespace comes from that context, not from a cluster-name lookup.
func TestGetCurrentNamespace_PrefersPinnedContext(t *testing.T) {
	cfg := kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), "other") // testContext has namespace team-ns
	orc := newTestGKEOrchestrator(newExactExecutor(nil))
	orc.kubeClient = &MockKubeClient{Namespace: "from-kube-client"}
	orc.kubeconfigLoader = staticLoader(cfg)

	orc.kubeContext = testContext
	if ns, err := orc.getCurrentNamespace("", "", ""); err != nil || ns != "team-ns" {
		t.Errorf("getCurrentNamespace() = %q, %v; want team-ns from pinned context", ns, err)
	}

	orc.namespace, orc.kubeContext = "", "missing-context"
	if ns, err := orc.getCurrentNamespace(testCluster, testLocation, testProject); err != nil || ns != "from-kube-client" {
		t.Errorf("getCurrentNamespace() = %q, %v; want fallback to kube client when the pinned context is absent", ns, err)
	}
}

func auditFileForUnpinnedKubectl(relPath string, src []byte) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, src, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s: failed to parse Go source: %v", relPath, err)}
	}
	var violations []string
	for _, decl := range file.Decls {
		funcName := ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcName = fn.Name.Name
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if msg := checkKubectlCall(relPath, funcName, call); msg != "" {
				pos := fset.Position(call.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d %s", relPath, pos.Line, msg))
			}
			return true
		})
	}
	return violations
}

func checkKubectlCall(relPath, funcName string, call *ast.CallExpr) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if sel.Sel.Name == "kubectlIn" {
			return checkKubectlInLiteral(call)
		}
		cmdIdx, isExec := selectorCmdArgIndex(sel)
		if cmdIdx >= 0 && isKubectlLiteralArg(call, cmdIdx) {
			return validateKubectlInvocation(relPath, funcName, call, cmdIdx, isExec)
		}
		return ""
	}
	if cmdIdx := identKubectlCmdArgIndex(call); cmdIdx >= 0 {
		return validateKubectlInvocation(relPath, funcName, call, cmdIdx, false)
	}
	return ""
}

func checkKubectlInLiteral(call *ast.CallExpr) string {
	if len(call.Args) > 0 {
		if val, isLit := stringLitValue(call.Args[0]); isLit && strings.TrimSpace(val) == "" {
			return "calls kubectlIn with an empty/whitespace context"
		}
	}
	return ""
}

func isKubectlLiteralArg(call *ast.CallExpr, cmdIdx int) bool {
	if len(call.Args) <= cmdIdx {
		return false
	}
	cmd, isLit := stringLitValue(call.Args[cmdIdx])
	return isLit && cmd == "kubectl"
}

func selectorCmdArgIndex(sel *ast.SelectorExpr) (int, bool) {
	if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "exec" {
		switch sel.Sel.Name {
		case "Command":
			return 0, true
		case "CommandContext":
			return 1, true
		}
	}
	switch sel.Sel.Name {
	case "ExecuteCommand", "ExecuteCommandStream", "NewCommand":
		return 0, false
	case "ExecuteCommandWithTimeout", "NewCommandWithTimeout", "Runner":
		return 1, false
	default:
		return -1, false
	}
}

func identKubectlCmdArgIndex(call *ast.CallExpr) int {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name == "append" {
		return -1
	}
	for _, idx := range []int{0, 1} {
		if len(call.Args) >= idx+2 && isKubectlLiteralArg(call, idx) {
			return idx
		}
	}
	return -1
}

func isAllowedKubeAuthWrapper(funcName string) bool {
	switch funcName {
	case "kubectl", "kubectlWithTimeout", "kubectlIn", "kubectlStream":
		return true
	default:
		return false
	}
}

func isAllowedRestoreNamespaceCall(call *ast.CallExpr, cmdIdx int) bool {
	if len(call.Args) < cmdIdx+4 {
		return false
	}
	subcmd, ok1 := stringLitValue(call.Args[cmdIdx+1])
	action, ok2 := stringLitValue(call.Args[cmdIdx+2])
	if !ok1 || !ok2 || subcmd != "config" || action != "set-context" {
		return false
	}
	if target, isLit := stringLitValue(call.Args[cmdIdx+3]); isLit {
		trimmed := strings.TrimSpace(target)
		return trimmed != "" && trimmed != "--current"
	}
	return true
}

func validateKubectlInvocation(relPath, funcName string, call *ast.CallExpr, cmdIdx int, isExec bool) string {
	if isExec {
		if isPinnedWithContextArg(relPath, call, cmdIdx) {
			return ""
		}
		return "invokes kubectl via os/exec without pinned context args"
	}
	if relPath == "gke/kube_auth.go" && funcName == "restoreNamespaceContext" && isAllowedRestoreNamespaceCall(call, cmdIdx) {
		return ""
	}
	allowPinned := !strings.HasPrefix(relPath, "gke/") || (relPath == "gke/kube_auth.go" && isAllowedKubeAuthWrapper(funcName))
	if allowPinned && isPinnedWithContextArg(relPath, call, cmdIdx) {
		return ""
	}
	return "runs kubectl without the pinned context; use g.kubectl / g.kubectlWithTimeout / g.kubectlIn / g.kubectlStream (or k8s.WithContext outside pkg/orchestrator/gke)"
}

func isWithContextFun(relPath string, fun ast.Expr) bool {
	if path.Dir(relPath) == "k8s" {
		ident, ok := fun.(*ast.Ident)
		return ok && ident.Name == "WithContext"
	}
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WithContext" {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	return ok && pkgIdent.Name == "k8s"
}

func isPinnedWithContextArg(relPath string, call *ast.CallExpr, cmdIdx int) bool {
	if !call.Ellipsis.IsValid() || len(call.Args) != cmdIdx+2 {
		return false
	}
	argCall, ok := call.Args[cmdIdx+1].(*ast.CallExpr)
	if !ok || len(argCall.Args) != 2 || !isWithContextFun(relPath, argCall.Fun) {
		return false
	}
	if ctxLit, isLit := stringLitValue(argCall.Args[0]); isLit && strings.TrimSpace(ctxLit) == "" {
		return false
	}
	return true
}

func stringLitValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	val, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return val, true
}

// Guards against new kubectl calls that bypass the pinned context, whatever the executor variable is called,
// and ensures kubectlIn is never invoked with an empty context literal.
func TestNoUnpinnedKubectlCalls(t *testing.T) {
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel("..", p)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, v := range auditFileForUnpinnedKubectl(filepath.ToSlash(rel), src) {
			t.Error(v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuditFileForUnpinnedKubectl(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		relPath     string
		src         string
		wantSubstrs []string
	}{
		{
			name:    "flags direct ExecuteCommand, Stream, Timeout with comma, and shell.NewCommand",
			relPath: "gke/sample.go",
			src: `package gke
func f() {
	g.executor.ExecuteCommand("kubectl", "get", "pods")
	g.executor.ExecuteCommandStream("kubectl", "logs", "-f")
	g.executor.ExecuteCommandWithTimeout(max(a, b), "kubectl", "get", "pods")
	shell.NewCommand("kubectl", "get", "pods")
	shell.NewCommandWithTimeout(t, "kubectl", "get", "pods")
}`,
			wantSubstrs: []string{
				"gke/sample.go:3 runs kubectl without the pinned context",
				"gke/sample.go:4 runs kubectl without the pinned context",
				"gke/sample.go:5 runs kubectl without the pinned context",
				"gke/sample.go:6 runs kubectl without the pinned context",
				"gke/sample.go:7 runs kubectl without the pinned context",
			},
		},
		{
			name:    "flags empty raw-backtick and escaped-whitespace kubectlIn",
			relPath: "gke/sample.go",
			src:     "package gke\nfunc f() {\n\tg.kubectlIn(\"\", \"get\")\n\tg.kubectlIn(` `, \"get\")\n\tg.kubectlIn(\"\\t\\n\", \"get\")\n}\n",
			wantSubstrs: []string{
				"gke/sample.go:3 calls kubectlIn with an empty/whitespace context",
				"gke/sample.go:4 calls kubectlIn with an empty/whitespace context",
				"gke/sample.go:5 calls kubectlIn with an empty/whitespace context",
			},
		},
		{
			name:    "enforces strict kube_auth.go wrapper and restoreNamespaceContext allowlists",
			relPath: "gke/kube_auth.go",
			src: `package gke
func kubectl(args ...string) { g.executor.ExecuteCommand("kubectl", k8s.WithContext(g.kubeContext, args)...) }
func restoreNamespaceContext() {
	g.executor.ExecuteCommand("kubectl", "config", "set-context", ctx, "--namespace=ns")
	g.executor.ExecuteCommand("kubectl", "config", "set-context", "--current", "--namespace=ns")
	g.executor.ExecuteCommand("kubectl", "get", "pods")
}
func probeContext() { g.executor.ExecuteCommand("kubectl", k8s.WithContext(ctx, args)...) }`,
			wantSubstrs: []string{
				"gke/kube_auth.go:5 runs kubectl without the pinned context",
				"gke/kube_auth.go:6 runs kubectl without the pinned context",
				"gke/kube_auth.go:8 runs kubectl without the pinned context",
			},
		},
		{
			name:    "allows bare WithContext inside package k8s and rejects blank context literal",
			relPath: "k8s/client.go",
			src: `package k8s
func f() {
	exec.ExecuteCommand("kubectl", WithContext(ctx, args)...)
	exec.ExecuteCommand("kubectl", WithContext("   ", args)...)
}`,
			wantSubstrs: []string{
				"k8s/client.go:4 runs kubectl without the pinned context",
			},
		},
		{
			name:    "audits bare runner and .Runner calls in orchestrator root package",
			relPath: "detector.go",
			src: `package orchestrator
func f() {
	runner(t, "kubectl", k8s.WithContext(ctx, args)...)
	in.Runner(t, "kubectl", k8s.WithContext(ctx, args)...)
	runner(t, "kubectl", "version")
	in.Runner(t, "kubectl", k8s.WithContext("", args)...)
}`,
			wantSubstrs: []string{
				"detector.go:5 runs kubectl without the pinned context",
				"detector.go:6 runs kubectl without the pinned context",
			},
		},
		{
			name:    "requires inline k8s.WithContext for os/exec in sps_client.go",
			relPath: "gke/sps_client.go",
			src: `package gke
func startPortForward() { exec.CommandContext(ctx, "kubectl", k8s.WithContext(g.kubeContext, args)...) }
func keepPortForwardAlive() { exec.CommandContext(ctx, "kubectl", args...) }
func other() { exec.CommandContext(ctx, "kubectl", "port-forward") }`,
			wantSubstrs: []string{
				"gke/sps_client.go:3 invokes kubectl via os/exec without pinned context args",
				"gke/sps_client.go:4 invokes kubectl via os/exec without pinned context args",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := auditFileForUnpinnedKubectl(tc.relPath, []byte(tc.src))
			if len(got) != len(tc.wantSubstrs) {
				t.Fatalf("auditFileForUnpinnedKubectl(%s) returned %d violations (%v), want %d (%v)", tc.relPath, len(got), got, len(tc.wantSubstrs), tc.wantSubstrs)
			}
			for i, want := range tc.wantSubstrs {
				if !strings.Contains(got[i], want) {
					t.Errorf("violation[%d] = %q, want substring %q", i, got[i], want)
				}
			}
		})
	}
}

func TestKubectlIn_RefusesEmptyContextAndPinsNonEmpty(t *testing.T) {
	execErr := errors.New("exec: kubectl: executable file not found in $PATH")
	exec := newExactExecutor(map[string][]shell.CommandResult{
		"kubectl --context pinned-ctx get pods":                                  {{ExitCode: 0, Stdout: "ok"}},
		"kubectl --context exec-err-ctx get --raw /version --request-timeout=5s": {{ExitCode: -1, Err: execErr}},
	})
	orc := newTestGKEOrchestrator(exec)

	res := orc.kubectlIn("   ", "get", "pods")
	if res.ExitCode == 0 || len(exec.calls) != 0 || !strings.Contains(res.Stderr, "refusing to run kubectl without an explicit --context") {
		t.Fatalf("kubectlIn(empty) = %+v (calls=%v), want explicit --context error with 0 subprocesses", res, exec.calls)
	}

	outcome, stderr := orc.probeContext("   ")
	if outcome != probeInvalid || len(exec.calls) != 0 || !strings.Contains(stderr, "refusing to run kubectl without an explicit --context") {
		t.Fatalf("probeContext(whitespace) = (%v, %q), calls=%v; want (probeInvalid, refusal, 0 calls)", outcome, stderr, exec.calls)
	}

	res = orc.kubectlIn("pinned-ctx", "get", "pods")
	if res.ExitCode != 0 || len(exec.calls) != 1 || exec.calls[0] != "kubectl --context pinned-ctx get pods" {
		t.Fatalf("kubectlIn(pinned-ctx) = %+v (calls=%v)", res, exec.calls)
	}

	outcome, stderr = orc.probeContext("exec-err-ctx")
	if outcome != probeUnknown || !strings.Contains(stderr, "executable file not found") {
		t.Fatalf("probeContext(exec-err) = (%v, %q), want probeUnknown surfacing res.Err when Stderr is empty", outcome, stderr)
	}
}

func TestConfigureKubectl_BlankContextNameIsNotReused(t *testing.T) {
	if (kubeContextCheck{}).reusable() || (kubeContextCheck{contextName: "   "}).reusable() {
		t.Fatal("expected blank/whitespace kubeContextCheck.reusable() to be false")
	}
	caBytes := []byte(testCAPEM)
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[testCluster] = &clientcmdapi.Cluster{Server: "https://" + testPublicIP, CertificateAuthorityData: caBytes}
	cfg.AuthInfos["u"] = &clientcmdapi.AuthInfo{Exec: gkeExec()}
	cfg.Contexts["   "] = &clientcmdapi.Context{Cluster: testCluster, AuthInfo: "u"}

	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(cfg)

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if orc.kubeContext != testContext {
		t.Fatalf("kubeContext = %q, want %q (blank context key must not be reused)", orc.kubeContext, testContext)
	}
	if exec.count(getCredsIP) != 1 || exec.count(probeCmd) != 1 {
		t.Fatalf("expected get-credentials + pinned probe, got calls=%v", exec.calls)
	}
}

func TestRefreshCredentials_ZonalDoesNotCopySiblingRegionalNamespace(t *testing.T) {
	const (
		zonalLoc   = "us-central1-a"
		zonalCtx   = "gke_my-project_us-central1-a_my-cluster"
		getZonalIP = "gcloud container clusters get-credentials my-cluster --location us-central1-a --project my-project"
		probeZonal = "kubectl --context " + zonalCtx + " get --raw /version --request-timeout=5s"
	)
	// Kubeconfig holds a regional cluster with the same short name and custom namespace "team-ns".
	cfg := kubeconfigWith("https://198.51.100.99", []byte("other-ca"), gkeExec(), testContext)

	exec := newExactExecutor(map[string][]shell.CommandResult{
		getZonalIP: {{ExitCode: 0}},
		probeZonal: {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(false, true)
	orc.kubeconfigLoader = staticLoader(cfg)

	if err := orc.configureKubectl(testCluster, zonalLoc, testProject); err != nil {
		t.Fatalf("configureKubectl(zonal) error = %v; calls=%v", err, exec.calls)
	}
	for _, call := range exec.calls {
		if strings.Contains(call, "set-context") {
			t.Fatalf("unexpected namespace carry-over from sibling regional cluster onto zonal context: calls=%v", exec.calls)
		}
	}
	if orc.kubeContext != zonalCtx {
		t.Fatalf("kubeContext = %q, want %q", orc.kubeContext, zonalCtx)
	}
}

func TestRefreshCredentials_BareNameDoesNotCopyAndRenamedAliasPreservesNamespace(t *testing.T) {
	// Case 1: An unrelated bare-name context (e.g. local/on-prem cluster with the same short name)
	// must NOT leak its namespace onto a newly created GKE context.
	bareCfg := clientcmdapi.NewConfig()
	bareCfg.Clusters[testCluster] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:6443"}
	bareCfg.Contexts[testCluster] = &clientcmdapi.Context{Cluster: testCluster, Namespace: "dev-local"}

	execBare := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
	})
	orcBare := newTestGKEOrchestrator(execBare)
	orcBare.clusterDesc = testClusterDesc(false, true)
	orcBare.kubeconfigLoader = staticLoader(bareCfg)
	if err := orcBare.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl(bare-name collision) error = %v; calls=%v", err, execBare.calls)
	}
	for _, call := range execBare.calls {
		if strings.Contains(call, "set-context") {
			t.Fatalf("unexpected namespace carry-over from unrelated bare-name context: calls=%v", execBare.calls)
		}
	}

	// Case 2: A renamed GKE context alias (Context.Cluster == canonical gke_<proj>_<loc>_<clus>)
	// MUST preserve its custom namespace when credentials are refreshed.
	aliasCfg := clientcmdapi.NewConfig()
	aliasCfg.Clusters[testContext] = &clientcmdapi.Cluster{Server: "https://198.51.100.9"} // forces regeneration
	aliasCfg.AuthInfos[testContext] = &clientcmdapi.AuthInfo{Exec: gkeExec()}
	aliasCfg.Contexts["prod-gpu"] = &clientcmdapi.Context{Cluster: testContext, AuthInfo: testContext, Namespace: "team-ns"}

	execAlias := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
		setNSCmd:   {{ExitCode: 0}},
	})
	orcAlias := newTestGKEOrchestrator(execAlias)
	orcAlias.clusterDesc = testClusterDesc(false, true)
	orcAlias.kubeconfigLoader = staticLoader(aliasCfg)
	if err := orcAlias.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl(renamed GKE alias) error = %v; calls=%v", err, execAlias.calls)
	}
	if execAlias.count(setNSCmd) != 1 {
		t.Fatalf("expected namespace preservation from renamed GKE context alias, got calls=%v", execAlias.calls)
	}
}

func TestResolveGKEContext_ZonalFallbackAndCurrentContext(t *testing.T) {
	assertResolved := func(label string, cfg *clientcmdapi.Config, cluster, loc, proj, wantCtx, wantNS, wantCluster string) {
		t.Helper()
		ctxName, kubeCtx, clus, ok := resolveGKEContext(cfg, cluster, loc, proj)
		if !ok || kubeCtx == nil || ctxName != wantCtx || kubeCtx.Namespace != wantNS || clus != wantCluster {
			t.Fatalf("resolveGKEContext(%s) = (%q, %+v, %q, %v), want (%q, ns=%q, %q, true)", label, ctxName, kubeCtx, clus, ok, wantCtx, wantNS, wantCluster)
		}
	}

	if _, _, _, ok := resolveGKEContext(nil, "reg-clus", "us-central1", "my-proj"); ok {
		t.Fatalf("expected nil config with explicit cluster to return false")
	}

	cfg := clientcmdapi.NewConfig()
	cfg.Contexts["gke_my-proj_us-central1_reg-clus"] = &clientcmdapi.Context{
		Cluster:   "gke_my-proj_us-central1_reg-clus",
		Namespace: "reg-ns",
	}
	// Competing non-gcloud context with Cluster == "reg-clus" ensures regional gcloud name wins over FindContextByCluster.
	cfg.Contexts["aaa-competing-ctx"] = &clientcmdapi.Context{Cluster: "reg-clus", Namespace: "wrong-ns"}
	cfg.Contexts["active-ctx"] = &clientcmdapi.Context{Cluster: "active-clus", Namespace: "active-ns"}
	cfg.CurrentContext = "active-ctx"

	assertResolved("zonal->regional", cfg, " reg-clus ", " us-central1-b ", " my-proj ", "gke_my-proj_us-central1_reg-clus", "reg-ns", "reg-clus")

	// Exact zonal gcloud context takes precedence over regional fallback when both exist.
	cfg.Contexts["gke_my-proj_us-central1-b_reg-clus"] = &clientcmdapi.Context{
		Cluster:   "gke_my-proj_us-central1-b_reg-clus",
		Namespace: "zonal-ns",
	}
	assertResolved("exact zonal over regional", cfg, "reg-clus", "us-central1-b", "my-proj", "gke_my-proj_us-central1-b_reg-clus", "zonal-ns", "reg-clus")
	assertResolved("current-context", cfg, "", "", "", "active-ctx", "active-ns", "active-clus")

	if _, _, _, ok := resolveGKEContext(cfg, "missing-clus", "us-central1", "my-proj"); ok {
		t.Fatalf("expected missing explicit cluster not to fall back to current-context")
	}

	// Renamed GKE context alias (Context.Cluster == canonical gke_<proj>_<loc>_<clus>) wins over bare-name collision.
	aliasCfg := clientcmdapi.NewConfig()
	aliasCfg.Contexts["prod-gpu"] = &clientcmdapi.Context{Cluster: "gke_my-proj_us-central1_reg-clus", Namespace: "alias-ns"}
	aliasCfg.Contexts["reg-clus"] = &clientcmdapi.Context{Cluster: "reg-clus", Namespace: "local-ns"}
	assertResolved("renamed GKE alias", aliasCfg, "reg-clus", "us-central1", "my-proj", "prod-gpu", "alias-ns", "reg-clus")
}

func TestDefaultKubeClient_GetCurrentNamespaceErrors(t *testing.T) {
	t.Parallel()
	loadSentinel := errors.New("simulated kubeconfig read error")
	danglingCfg := clientcmdapi.NewConfig()
	danglingCfg.CurrentContext = "deleted-ctx "
	whitespaceCurCfg := clientcmdapi.NewConfig()
	whitespaceCurCfg.CurrentContext = "   "

	for _, tc := range []struct {
		name        string
		loader      k8s.KubeconfigLoader
		cluster     string
		loc         string
		proj        string
		wantSubstr  string
		wantWrapped error
	}{
		{name: "nil cfg", loader: staticLoader(nil), wantSubstr: "no current-context set in kubeconfig"},
		{name: "load error", loader: func() (*clientcmdapi.Config, error) { return nil, loadSentinel }, cluster: "my-clus", loc: "us-central1", proj: "my-proj", wantSubstr: "--gke-namespace", wantWrapped: loadSentinel},
		{name: "empty current-context", loader: staticLoader(clientcmdapi.NewConfig()), wantSubstr: "no current-context set in kubeconfig"},
		{name: "missing explicit cluster", loader: staticLoader(clientcmdapi.NewConfig()), cluster: " missing-clus ", loc: "us-central1", proj: "my-proj", wantSubstr: `cluster "missing-clus"`},
		{name: "dangling current-context", loader: staticLoader(danglingCfg), wantSubstr: `current-context "deleted-ctx " was not found in kubeconfig`},
		{name: "whitespace current-context", loader: staticLoader(whitespaceCurCfg), wantSubstr: "no current-context set in kubeconfig"},
	} {
		kc := &DefaultKubeClient{kubeconfigLoader: tc.loader}
		_, err := kc.GetCurrentNamespace(tc.cluster, tc.loc, tc.proj)
		if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) || (tc.wantWrapped != nil && !errors.Is(err, tc.wantWrapped)) {
			t.Fatalf("%s: GetCurrentNamespace() = %v, want substring %q (wrapped=%v)", tc.name, err, tc.wantSubstr, tc.wantWrapped)
		}
	}

	happyCurCfg := clientcmdapi.NewConfig()
	happyCurCfg.Contexts["active-ctx"] = &clientcmdapi.Context{Cluster: "active-clus", Namespace: " active-ns "}
	happyCurCfg.CurrentContext = "active-ctx"
	kcHappyCur := &DefaultKubeClient{kubeconfigLoader: staticLoader(happyCurCfg)}
	if ns, err := kcHappyCur.GetCurrentNamespace("", "", ""); err != nil || ns != "active-ns" {
		t.Fatalf("GetCurrentNamespace(happy current-context) = (%q, %v), want (active-ns, nil)", ns, err)
	}
	assertOrchestratorNamespaceLoaderPropagation(t)
}

func assertOrchestratorNamespaceLoaderPropagation(t *testing.T) {
	t.Helper()
	const ctxKey = "gke_my-proj_us-central1_my-clus"
	orcCfg := clientcmdapi.NewConfig()
	orcCfg.Contexts[ctxKey] = &clientcmdapi.Context{
		Cluster:   ctxKey,
		Namespace: " loader-ns ",
	}
	pinnedOnlyCfg := clientcmdapi.NewConfig()
	pinnedOnlyCfg.Contexts["pinned-only-ctx"] = &clientcmdapi.Context{
		Cluster:   "unrelated-cluster",
		Namespace: " loader-ns ",
	}

	for _, tc := range []struct {
		name string
		orc  *GKEOrchestrator
	}{
		{name: "pre-seeded DefaultKubeClient", orc: &GKEOrchestrator{kubeClient: &DefaultKubeClient{}, kubeconfigLoader: staticLoader(orcCfg)}},
		{name: "nil kubeClient", orc: &GKEOrchestrator{kubeconfigLoader: staticLoader(orcCfg)}},
		{name: "pre-injected DefaultKubeClient loader preserved", orc: &GKEOrchestrator{kubeClient: &DefaultKubeClient{kubeconfigLoader: staticLoader(orcCfg)}}},
		{name: "pinned context trims namespace", orc: &GKEOrchestrator{kubeContext: "pinned-only-ctx", kubeClient: &MockKubeClient{Namespace: "wrong-fallback"}, kubeconfigLoader: staticLoader(pinnedOnlyCfg)}},
	} {
		if ns, err := tc.orc.getCurrentNamespace("my-clus", "us-central1", "my-proj"); err != nil || ns != "loader-ns" {
			t.Fatalf("%s: getCurrentNamespace = (%q, %v), want (loader-ns, nil)", tc.name, ns, err)
		}
	}

	// pinnedContextNamespace returns ("", false) on nil config or loader error.
	if ns, ok := (&GKEOrchestrator{kubeContext: "pinned", kubeconfigLoader: staticLoader(nil)}).pinnedContextNamespace(); ok || ns != "" {
		t.Fatalf("pinnedContextNamespace(nil cfg) = (%q, %v), want (\"\", false)", ns, ok)
	}
	if ns, ok := (&GKEOrchestrator{kubeContext: "pinned", kubeconfigLoader: func() (*clientcmdapi.Config, error) { return nil, errLoadKubeconfigForTest }}).pinnedContextNamespace(); ok || ns != "" {
		t.Fatalf("pinnedContextNamespace(load err) = (%q, %v), want (\"\", false)", ns, ok)
	}
}

func TestRestoreNamespaceContext(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		setNSCmd: {
			{ExitCode: 0},
			{ExitCode: 1, Stderr: "permission denied"},
		},
	})
	orc := newTestGKEOrchestrator(exec)

	for _, noop := range []string{"", "   ", " default "} {
		if err := orc.restoreNamespaceContext(testContext, noop); err != nil {
			t.Fatalf("restoreNamespaceContext(%q) unexpected error: %v", noop, err)
		}
	}
	if len(exec.calls) != 0 {
		t.Fatalf("expected 0 kubectl calls for blank/default namespaces, got %v", exec.calls)
	}

	if err := orc.restoreNamespaceContext(testContext, "  team-ns  "); err != nil {
		t.Fatalf("restoreNamespaceContext(padded) error = %v", err)
	}
	if err := orc.restoreNamespaceContext(testContext, "team-ns"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("restoreNamespaceContext(error) = %v, want permission denied", err)
	}
	if exec.count(setNSCmd) != 2 {
		t.Fatalf("expected 2 set-context calls, got calls=%v", exec.calls)
	}
}

func TestImagePullSecrets_NamespacePrecedence(t *testing.T) {
	setupMockMachineConfig(t)
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}
	kc := &MockKubeClient{
		Namespace:      "must-not-use-getcurrentnamespace",
		ListNamespaces: map[string]string{},
		Objects: map[string][]unstructured.Unstructured{
			"secrets": {makeDockerConfigSecret("reg-sec", "registry.local")},
		},
	}
	orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get pv":  {{ExitCode: 0, Stdout: ""}},
		"kubectl get pvc": {{ExitCode: 0, Stdout: ""}},
	}))
	orc.machineCapCache = testMachineCapCache()
	orc.kubeClient = kc
	orc.kubeContext = "pinned-ctx"
	orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) {
		cfg := clientcmdapi.NewConfig()
		cfg.Contexts["pinned-ctx"] = &clientcmdapi.Context{Namespace: "ctx-ns"}
		return cfg, nil
	}

	baseJob := orchestrator.JobDefinition{
		WorkloadName:    "ns-job",
		CommandToRun:    "echo hi",
		ComputeType:     "n2-standard-4",
		MachineType:     "n2-standard-4",
		ClusterLocation: "us-central1-a",
		NumSlices:       1,
		NodesPerSlice:   1,
	}

	if _, err := orc.PrepareManifestOptions(baseJob, "registry.local/app:v1", profile, false, false); err != nil || kc.ListNamespaces["secrets"] != "ctx-ns" {
		t.Fatalf("got ns=%q err=%v, want ctx-ns", kc.ListNamespaces["secrets"], err)
	}

	// SubmitJob seeds g.namespace from --gke-namespace before any manifest work.
	orc.namespace = "flag-ns"
	if _, err := orc.PrepareManifestOptions(baseJob, "registry.local/app:v1", profile, false, false); err != nil || kc.ListNamespaces["secrets"] != "flag-ns" {
		t.Fatalf("got ns=%q err=%v, want flag-ns", kc.ListNamespaces["secrets"], err)
	}
}

func TestImagePullSecrets_FallbackEscapingAndLazyInit(t *testing.T) {
	setupMockMachineConfig(t)
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}
	orc := newCachedTestOrchestrator()
	orc.kubeClient = &MockKubeClient{
		ListErrs: map[string]error{
			"secrets": apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("forbidden")),
		},
	}
	job := orchestrator.JobDefinition{
		WorkloadName:    "forbidden-sec",
		CommandToRun:    "echo hi",
		ComputeType:     "n2-standard-4",
		MachineType:     "n2-standard-4",
		ClusterLocation: "us-central1-a",
		NumSlices:       1,
		NodesPerSlice:   1,
	}
	opts, err := orc.PrepareManifestOptions(job, "registry.local/app:v1", profile, false, false)
	if err != nil || opts.ImagePullSecrets != "" {
		t.Fatalf("expected clean fallback on 403 Forbidden, got opts.ImagePullSecrets=%q err=%v", opts.ImagePullSecrets, err)
	}

	job.ImagePullSecrets = "sec\n  hostPID: true"
	opts, err = orc.PrepareManifestOptions(job, "registry.local/app:v1", profile, false, false)
	if err != nil {
		t.Fatalf("unexpected error escaping secret name: %v", err)
	}
	assertJobSetEscapedPullSecret(t, orc, opts, profile, "sec\n  hostPID: true")

	lazyOrc := &GKEOrchestrator{
		kubeClient:       &DefaultKubeClient{},
		kubeContext:      "missing-ctx-in-kubeconfig",
		machineCapCache:  testMachineCapCache(),
		kubeconfigLoader: func() (*clientcmdapi.Config, error) { return clientcmdapi.NewConfig(), nil },
	}
	job.ImagePullSecrets = ""
	opts, err = lazyOrc.PrepareManifestOptions(job, "registry.local/app:v1", profile, false, false)
	if err != nil || opts.ImagePullSecrets != "" {
		t.Fatalf("expected lazy init error to degrade gracefully, got opts.ImagePullSecrets=%q err=%v", opts.ImagePullSecrets, err)
	}
}

func assertJobSetEscapedPullSecret(t *testing.T, orc *GKEOrchestrator, opts ManifestOptions, profile JobProfile, wantSecret string) {
	t.Helper()
	cpuResp := shell.CommandResult{ExitCode: 0, Stdout: `{"guestCpus": 4, "memoryMb": 16384}`}
	orc.executor = NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-4 --zone=us-central1-a --format=json": {cpuResp},
	})
	orc.clusterZones = []string{"us-central1-a"}
	manifest, err := orc.GenerateGKEManifest(opts, profile)
	if err != nil {
		t.Fatalf("GenerateGKEManifest failed: %v", err)
	}
	var js jobSetValidationSpec
	if err := k8syaml.Unmarshal([]byte(manifest), &js); err != nil {
		t.Fatalf("failed to unmarshal JobSet YAML: %v\n%s", err, manifest)
	}
	podSpec := js.Spec.ReplicatedJobs[0].Template.Spec.Template.Spec
	if podSpec.HostPID {
		t.Fatalf("ImagePullSecrets newline injected hostPID=true into PodSpec!")
	}
	if len(podSpec.ImagePullSecrets) != 1 || podSpec.ImagePullSecrets[0].Name != wantSecret {
		t.Errorf("unexpected escaped ImagePullSecrets: %+v", podSpec.ImagePullSecrets)
	}
}

func TestImagePullSecrets_NamespaceResolutionFailureFailsFast(t *testing.T) {
	setupMockMachineConfig(t)
	profile := JobProfile{IsCPUMachine: true, CapacityCount: 4}
	mockClient := &MockKubeClient{
		ExplicitEmpty: true,
		Err:           errors.New("no matching context found for cluster clus-1 in kubeconfig. You can explicitly specify the namespace using the --gke-namespace flag"),
		Objects: map[string][]unstructured.Unstructured{
			"secrets": {makeDockerConfigSecret("ns-sec", "registry.local")},
		},
	}
	// A usable API client whose context resolution fails: discovery must not probe "default".
	orc := newCachedTestOrchestrator()
	orc.kubeClient = mockClient
	job := orchestrator.JobDefinition{
		WorkloadName:    "ns-fail-job",
		CommandToRun:    "echo hi",
		ComputeType:     "n2-standard-4",
		MachineType:     "n2-standard-4",
		ClusterName:     "clus-1",
		ClusterLocation: "us-central1-a",
		NumSlices:       1,
		NodesPerSlice:   1,
	}
	_, err := orc.PrepareManifestOptions(job, "registry.local/app:v1", profile, false, false)
	if err == nil || !strings.Contains(err.Error(), "imagePullSecrets discovery") || !strings.Contains(err.Error(), "--gke-namespace") {
		t.Fatalf("expected namespace resolution error naming --gke-namespace, got: %v", err)
	}

	orc.namespace = "tenant-ns" // seeded from --gke-namespace by SubmitJob
	opts, err := orc.PrepareManifestOptions(job, "registry.local/app:v1", profile, false, false)
	if err != nil || !strings.Contains(opts.ImagePullSecrets, "ns-sec") {
		t.Fatalf("explicit --gke-namespace must bypass resolution and discover ns-sec: opts=%q err=%v", opts.ImagePullSecrets, err)
	}
}
