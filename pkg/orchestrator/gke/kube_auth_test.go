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
	"encoding/pem"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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
	_ = os.Unsetenv(shell.AirGappedEnvVar)
	shell.SetDefaultAirGapped(false)
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
	timeouts  []time.Duration
	callCount map[string]int
}

func newExactExecutor(responses map[string][]shell.CommandResult) *exactExecutor {
	return &exactExecutor{responses: responses, callCount: map[string]int{}}
}

func (e *exactExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	return e.execute(0, name, args...)
}

func (e *exactExecutor) ExecuteCommandWithTimeout(timeout time.Duration, name string, args ...string) shell.CommandResult {
	return e.execute(timeout, name, args...)
}

func (e *exactExecutor) execute(timeout time.Duration, name string, args ...string) shell.CommandResult {
	key := name + " " + strings.Join(args, " ")
	e.calls = append(e.calls, key)
	e.timeouts = append(e.timeouts, timeout)
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
		{"The connection to the server 203.0.113.10:443 was refused - did you specify the right host or port?", probeConnectivity},
		{"connect: connection refused", probeConnectivity},
		{"Unable to connect to the server: dial tcp: lookup gke-abc.gke.goog: no such host", probeConnectivity},
		{"Unable to connect to the server: net/http: TLS handshake timeout", probeConnectivity},
		{"Unable to connect to the server: tls: failed to verify certificate: x509: certificate signed by unknown authority", probeConnectivity},
		{"Error from server: the server responded with the status code 431 but did not return more information", probeConnectivity},
		{"Request Header Fields Too Large", probeConnectivity},
		// Forward proxies and gateways answer for unreachable upstreams with HTTP 502/503/504 errors rather than a raw dial failure.
		{"Error from server (InternalError): an error on the server (\"<html><body>502 Bad Gateway</body></html>\") has prevented the request from succeeding", probeConnectivity},
		{"Error from server (ServiceUnavailable): the server is currently unable to handle the request", probeConnectivity},
		{"upstream service unavailable", probeConnectivity},
		{"Error from server (Timeout): the server was unable to return a response in the time allotted, but may still be processing the request", probeConnectivity},
		{"upstream gateway timeout", probeConnectivity},
		// Realistic multi-marker kubectl 401/exec outputs plus isolated single-marker cases so each probeAuthMarkers entry is pinned.
		{"error: You must be logged in to the server (Unauthorized)", probeAuth},
		{"error: You must be logged in to the server (the server has asked for the client to provide credentials)", probeAuth},
		{"Unable to connect to the server: getting credentials: exec: executable gke-gcloud-auth-plugin not found", probeAuth},
		{"Unable to connect to the server: getting credentials: exec: executable gke-gcloud-auth-plugin failed with exit code 1", probeAuth},
		{"Error from server: unauthorized", probeAuth},
		{"error: You must be logged in to the server", probeAuth},
		{"the server has asked for the client to provide credentials", probeAuth},
		{"getting credentials: exec: executable kubelogin failed with exit code 1", probeAuth},
		{"exec: executable gke-gcloud-auth-plugin failed with exit code 1", probeAuth},
		{"oauth2: cannot fetch token: 400 Bad Request", probeAuth},
		{`Unable to connect to the server: failed to refresh token: oauth2: "invalid_grant"`, probeAuth},
		{"Unable to connect to the server: No valid id-token, and cannot refresh without refresh-token", probeAuth},
		{"Unable to connect to the server: token response did not contain an id_token", probeAuth},
		{"Unable to connect to the server: could not persist new tokens: permission denied", probeAuth},
		{"error: The azure auth plugin has been removed.", probeAuth},
		{`error: No Auth Provider found for name "openstack"`, probeAuth},
		{`error: exec plugin: invalid apiVersion "client.authentication.k8s.io/v1alpha1"`, probeAuth},
		{"Unable to connect to the server: failed to refresh token: Post \"https://idp.example.com/token\": dial tcp 192.0.2.10:443: i/o timeout", probeConnectivity},
		{"could not get token from provider", probeAuth},
		{"google: could not find default credentials (application default credentials)", probeAuth},
		{"oauth2/google: status code 400: reauthentication required", probeAuth},
		{"error: unable to read certificate-authority /missing/ca.crt for local-k8s due to open /missing/ca.crt: no such file or directory", probeInvalid},
		{"error: unable to read client-cert /missing/client.crt for admin due to open /missing/client.crt: no such file or directory", probeInvalid},
		{"error: unable to read token file /missing/token: open /missing/token: no such file or directory", probeInvalid},
		{`error: error loading config file "/tmp/corrupt.yaml": yaml: line 2: did not find expected key`, probeInvalid},
		{"Error from server (Forbidden): forbidden: User cannot get path \"/version\"", probeForbidden},
		// GKE's DNS endpoint returns HTTP 403 when external traffic is disabled, IAM denies clusters.connect, or VPC-SC blocks access.
		{"Error from server (Forbidden): control_plane_endpoints_config.dns_endpoint_config.allow_external_traffic is disabled, external access via DNS endpoint is not allowed", probeConnectivity},
		{"Error from server (Forbidden): Permission 'container.googleapis.com/clusters.connect' denied on resource '//container.googleapis.com/projects/p/locations/l/clusters/c'", probeConnectivity},
		{"Error from server (Forbidden): Request is prohibited by organization's policy", probeConnectivity},
		{"Error from server (Forbidden): vpcServiceControlsUniqueIdentifier: abc123", probeConnectivity},
		// Forward proxy HTTP CONNECT rejection and TCP proxy dial failure are connectivity problems, not apiserver RBAC rejections.
		{"Unable to connect to the server: Forbidden", probeConnectivity},
		{"Unable to connect to the server: proxyconnect tcp: dial tcp 192.0.2.1:8080: i/o timeout", probeConnectivity},
		// Incidental substrings must not be misread as connectivity failures.
		{"mock error: unexpected command: kubectl --context c get --raw /version --request-timeout=5s", probeUnknown},
		{"Error from server (NotFound): pods \"worker-431-tls\" not found", probeUnknown},
		{"Error from server: admission webhook was refused", probeUnknown},
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

func TestRestConfig_PersistsRotatedOIDCTokensToKubeconfig(t *testing.T) {
	freshPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":4102444800}`))
	freshIDToken := "eyJhbGciOiJub25lIn0." + freshPayload + ".sig"

	var idpURL string
	idpSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/.well-known/openid-configuration" {
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"token_endpoint":%q}`, idpURL, idpURL+"/token")
			return
		}
		_, _ = fmt.Fprintf(w, `{"access_token":"acc-2","token_type":"Bearer","refresh_token":"rotated-rt-2","id_token":%q}`, freshIDToken)
	}))
	t.Cleanup(idpSrv.Close)
	idpURL = idpSrv.URL

	var gotBearer atomic.Value
	apiSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBearer.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"jobset.x-k8s.io/v1alpha2","kind":"JobSetList","items":[]}`))
	}))
	t.Cleanup(apiSrv.Close)

	idpCAPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: idpSrv.Certificate().Raw})
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["oidc-clus"] = &clientcmdapi.Cluster{Server: apiSrv.URL, InsecureSkipTLSVerify: true}
	cfg.AuthInfos["oidc-user"] = &clientcmdapi.AuthInfo{
		AuthProvider: &clientcmdapi.AuthProviderConfig{
			Name: "oidc",
			Config: map[string]string{
				"idp-issuer-url":                 idpSrv.URL,
				"client-id":                      "test-client",
				"refresh-token":                  "single-use-rt-1",
				"idp-certificate-authority-data": base64.StdEncoding.EncodeToString(idpCAPEM),
			},
		},
	}
	cfg.Contexts["oidc-ctx"] = &clientcmdapi.Context{Cluster: "oidc-clus", AuthInfo: "oidc-user", Namespace: "ml-ns"}
	cfg.CurrentContext = "oidc-ctx"

	raw, err := clientcmd.Write(*cfg)
	if err != nil {
		t.Fatal(err)
	}
	kcFile := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(kcFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kcFile)

	rawOrc, err := orchestrator.NewJobOrchestrator(orchestrator.Detection{
		Type:        orchestrator.TypeK8s,
		KubeContext: "oidc-ctx",
		ClusterName: "oidc-clus",
	})
	if err != nil {
		t.Fatalf("NewJobOrchestrator error = %v", err)
	}
	orc := rawOrc.(*GKEOrchestrator)
	orc.SetExecutor(NewMockExecutor(nil))

	if _, err := orc.ListJobs(orchestrator.ListOptions{ClusterName: "oidc-clus"}); err != nil {
		t.Fatalf("ListJobs(OIDC) error = %v", err)
	}
	if got, _ := gotBearer.Load().(string); got != "Bearer "+freshIDToken {
		t.Errorf("API Authorization header = %q, want %q", got, "Bearer "+freshIDToken)
	}

	persisted, err := clientcmd.LoadFromFile(kcFile)
	if err != nil {
		t.Fatalf("LoadFromFile(%s): %v", kcFile, err)
	}
	persistedCfg := persisted.AuthInfos["oidc-user"].AuthProvider.Config
	if persistedCfg["refresh-token"] != "rotated-rt-2" || persistedCfg["id-token"] != freshIDToken {
		t.Fatalf("persisted OIDC config = %v, want rotated refresh-token=\"rotated-rt-2\" and refreshed id-token", persistedCfg)
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
	case "kubectl", "kubectlWithTimeout", "kubectlIn", "kubectlStream", "probeGKEServerVersion":
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
func probeGKEServerVersion() {
	runner(t, "kubectl", k8s.WithContext(ctx, args)...)
	runner(t, "kubectl", "version")
	runner(t, "kubectl", k8s.WithContext("", args)...)
}
func restoreNamespaceContext() {
	g.executor.ExecuteCommand("kubectl", "config", "set-context", ctx, "--namespace=ns")
	g.executor.ExecuteCommand("kubectl", "config", "set-context", "--current", "--namespace=ns")
	g.executor.ExecuteCommand("kubectl", "get", "pods")
}
func probeContext() { g.executor.ExecuteCommand("kubectl", k8s.WithContext(ctx, args)...) }`,
			wantSubstrs: []string{
				"gke/kube_auth.go:5 runs kubectl without the pinned context",
				"gke/kube_auth.go:6 runs kubectl without the pinned context",
				"gke/kube_auth.go:10 runs kubectl without the pinned context",
				"gke/kube_auth.go:11 runs kubectl without the pinned context",
				"gke/kube_auth.go:13 runs kubectl without the pinned context",
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
		"kubectl --context pinned-ctx get pods":                                     {{ExitCode: 0, Stdout: "ok"}},
		"kubectl --context exec-err-ctx get --raw /version --request-timeout=5s":    {{ExitCode: -1, Err: execErr}},
		"kubectl --context hung-silent-ctx get --raw /version --request-timeout=5s": {{ExitCode: -1, Err: context.DeadlineExceeded}},
		"kubectl --context hung-plugin-ctx get --raw /version --request-timeout=5s": {{ExitCode: -1, Stderr: "fetching token from STS...\nwaiting on local callback port 8085", Err: context.DeadlineExceeded}},
		"kubectl --context hung-reauth-ctx get --raw /version --request-timeout=5s": {{ExitCode: -1, Stderr: "plugin starting\nreauthentication prompt waiting on stdin", Err: context.DeadlineExceeded}},
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
	if exec.timeouts[0] != shell.DefaultLocalCommandTimeout {
		t.Fatalf("kubectlIn timeout = %v, want %v", exec.timeouts[0], shell.DefaultLocalCommandTimeout)
	}

	outcome, stderr = orc.probeContext("exec-err-ctx")
	if outcome != probeUnknown || !strings.Contains(stderr, "executable file not found") {
		t.Fatalf("probeContext(exec-err) = (%v, %q), want probeUnknown surfacing res.Err when Stderr is empty", outcome, stderr)
	}
	assertProbeContextTimeoutOutcomes(t, orc)
}

func assertProbeContextTimeoutOutcomes(t *testing.T, orc *GKEOrchestrator) {
	t.Helper()
	if outcome, stderr := orc.probeContext("hung-silent-ctx"); outcome != probeConnectivity || !strings.HasPrefix(firstLine(stderr), "timed out after") {
		t.Fatalf("probeContext(hung-silent) = (%v, %q), want (probeConnectivity, firstLine timed out after...)", outcome, stderr)
	}
	outcome, stderr := orc.probeContext("hung-plugin-ctx")
	if outcome != probeConnectivity || !strings.HasPrefix(firstLine(stderr), "timed out after") || !strings.Contains(firstLine(stderr), "fetching token from STS...") {
		t.Fatalf("probeContext(hung-plugin) = (%v, %q), want (probeConnectivity, firstLine prefixed with timed out after + plugin line 1)", outcome, stderr)
	}
	outcome, stderr = orc.probeContext("hung-reauth-ctx")
	if outcome != probeAuth || !strings.HasPrefix(firstLine(stderr), "timed out after") {
		t.Fatalf("probeContext(hung-reauth) = (%v, %q), want (probeAuth, firstLine prefixed with timed out after)", outcome, stderr)
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

func newTypeK8sInspectMockExecutor() *MockExecutor {
	okDescribe := shell.CommandResult{ExitCode: 0, Stdout: "ok"}
	okWorkloads := shell.CommandResult{ExitCode: 0, Stdout: `{"items":[]}`}
	return NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get --raw /version":                {{ExitCode: 0, Stdout: `{"major":"1","minor":"32"}`}},
		"kubectl get --raw /apis/kueue.x-k8s.io":    {{ExitCode: 0, Stdout: "{}"}},
		"kubectl get nodes -o wide":                 {{ExitCode: 0, Stdout: "NAME STATUS\n"}},
		"kubectl get nodes -o json":                 {{ExitCode: 0, Stdout: `{"items":[]}`}},
		"kubectl describe ":                         {okDescribe, okDescribe, okDescribe, okDescribe, okDescribe},
		"kubectl logs ":                             {{ExitCode: 0, Stdout: "controller ok"}, {ExitCode: 0, Stdout: "controller ok"}},
		"kubectl get crd topologies.kueue.x-k8s.io": {{ExitCode: 1, Stderr: "NotFound"}},
		"kubectl get workloads ":                    {okWorkloads, okWorkloads, okWorkloads},
	})
}

func TestTypeK8s_ZeroGCloudAcrossLifecycle(t *testing.T) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["local-k8s"] = &clientcmdapi.Cluster{Server: "https://10.128.0.68:6443"}
	cfg.Contexts["k8s-ctx"] = &clientcmdapi.Context{Cluster: "local-k8s", Namespace: "ml-ns"}
	cfg.CurrentContext = "k8s-ctx"

	rawOrc, err := orchestrator.NewJobOrchestrator(orchestrator.Detection{
		Type:        orchestrator.TypeK8s,
		KubeContext: "k8s-ctx",
		ClusterName: "local-k8s",
	})
	if err != nil {
		t.Fatalf("NewJobOrchestrator(TypeK8s) error = %v", err)
	}
	orc := rawOrc.(*GKEOrchestrator)
	orc.kubeconfigLoader = staticLoader(cfg)

	mockExec := newTypeK8sInspectMockExecutor()
	orc.SetExecutor(mockExec)

	submitErr := orc.SubmitJob(orchestrator.JobDefinition{
		WorkloadName:           "blocked-job",
		ImageName:              "busybox:latest",
		CommandToRun:           "echo hi",
		GKENamespace:           "should-not-mutate",
		GkeCustomTemplatesPath: "/should-not-mutate",
	})
	wantSubmitErr := orchestrator.UnsupportedK8sSubmitError().Error()
	if submitErr == nil || submitErr.Error() != wantSubmitErr {
		t.Fatalf("SubmitJob(TypeK8s) error = %v, want %q", submitErr, wantSubmitErr)
	}
	if orc.namespace != "" || orc.gkeCustomTemplatesPath != "" {
		t.Fatalf("SubmitJob(TypeK8s) mutated state: namespace=%q customTemplatesPath=%q, want both empty", orc.namespace, orc.gkeCustomTemplatesPath)
	}
	if len(mockExec.calls) != 0 {
		t.Fatalf("SubmitJob(TypeK8s) calls = %v, want 0 subprocess calls", mockExec.calls)
	}

	if loc, err := orc.Initialize("local-k8s", "", ""); err != nil || loc != "" {
		t.Fatalf("Initialize(TypeK8s) = (%q, %v), want ('', nil)", loc, err)
	}
	if err := orc.configureKubectl("local-k8s", "", ""); err != nil || orc.kubeContext != "k8s-ctx" {
		t.Fatalf("configureKubectl(TypeK8s) = (%v, %q), want (nil, \"k8s-ctx\")", err, orc.kubeContext)
	}
	outPath := filepath.Join(t.TempDir(), "inspect.log")
	if err := orc.InspectCluster(orchestrator.InspectOptions{ClusterName: "local-k8s", OutputPath: outPath}); err != nil {
		t.Fatalf("InspectCluster(TypeK8s) error = %v", err)
	}
	assertTypeK8sInspectClean(t, mockExec, outPath)
	mockExec.assertAllKubectlPinned(t, "k8s-ctx")

	assertInspectSanitizesClusterFilename(t, orc)
}

func assertTypeK8sInspectClean(t *testing.T, mockExec *MockExecutor, outPath string) {
	t.Helper()
	for _, call := range mockExec.calls {
		if strings.HasPrefix(call, "gcloud") {
			t.Errorf("unexpected gcloud invocation in TypeK8s mode: %q", call)
		}
		if strings.Contains(call, "local-k8s-metadata") || strings.Contains(call, "local-k8s-resources") {
			t.Errorf("TypeK8s InspectCluster queried GKE-only blueprint ConfigMap: %q", call)
		}
	}
	if len(mockExec.calls) == 0 {
		t.Fatalf("expected kubectl diagnostic commands during InspectCluster")
	}
	inspectBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", outPath, err)
	}
	if strings.Contains(string(inspectBytes), "console.cloud.google.com") {
		t.Errorf("TypeK8s InspectCluster output must not emit broken Google Cloud Console URLs")
	}
	if strings.Contains(string(inspectBytes), "mock error:") {
		t.Errorf("TypeK8s InspectCluster output contained unmocked command failure:\n%s", string(inspectBytes))
	}
}

func assertInspectSanitizesClusterFilename(t *testing.T, orc *GKEOrchestrator) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	const eksARN = "arn:aws:eks:us-west-2:123456789012:cluster/prod-ml"
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[eksARN] = &clientcmdapi.Cluster{Server: "https://10.128.0.68:6443"}
	cfg.Contexts[eksARN] = &clientcmdapi.Context{Cluster: eksARN, Namespace: "ml-ns"}
	orc.kubeconfigLoader = staticLoader(cfg)
	orc.kubeContext = eksARN
	orc.k8sCluster = eksARN
	orc.SetExecutor(newTypeK8sInspectMockExecutor())

	if err := orc.InspectCluster(orchestrator.InspectOptions{ClusterName: eksARN}); err != nil {
		t.Fatalf("InspectCluster(EKS ARN default output) error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(tmpDir, "gcluster-inspect-arn-aws-eks-us-west-2-123456789012-cluster-prod-ml-*.log"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("sanitized inspect output matches = %v (err=%v), want 1 file", matches, err)
	}
	inspectBytes, err := os.ReadFile(matches[0])
	if err != nil || strings.Contains(string(inspectBytes), "mock error:") {
		t.Fatalf("sanitized inspect output err=%v or contained mock error:\n%s", err, string(inspectBytes))
	}
	for _, degenerate := range []string{"", "../..", ":::", "___"} {
		if got := sanitizeInspectFileSegment(degenerate); got != "cluster" {
			t.Errorf("sanitizeInspectFileSegment(%q) = %q, want \"cluster\"", degenerate, got)
		}
	}
}

func TestTypeK8s_ConfigureKubectlUnseeded(t *testing.T) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["local-k8s"] = &clientcmdapi.Cluster{Server: "https://10.128.0.68:6443"}
	cfg.Contexts["k8s-ctx"] = &clientcmdapi.Context{Cluster: "local-k8s", Namespace: "ml-ns"}
	cfg.CurrentContext = "k8s-ctx"

	orcUnseeded := newTestGKEOrchestrator(NewMockExecutor(nil))
	orcUnseeded.mode = orchestrator.TypeK8s
	orcUnseeded.kubeconfigLoader = staticLoader(cfg)
	if err := orcUnseeded.configureKubectl("local-k8s", "", ""); err != nil || orcUnseeded.kubeContext != "k8s-ctx" {
		t.Errorf("configureKubectl(unseeded TypeK8s) = (%v, %q), want (nil, \"k8s-ctx\")", err, orcUnseeded.kubeContext)
	}

	orcMissing := newTestGKEOrchestrator(NewMockExecutor(nil))
	orcMissing.mode = orchestrator.TypeK8s
	orcMissing.kubeconfigLoader = staticLoader(cfg)
	if err := orcMissing.configureKubectl("unknown-cluster", "", ""); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Errorf("configureKubectl(unknown-cluster) = %v, want context not found error", err)
	}

	orcEmpty := newTestGKEOrchestrator(NewMockExecutor(nil))
	orcEmpty.mode = orchestrator.TypeK8s
	orcEmpty.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())
	if err := orcEmpty.configureKubectl("", "", ""); err == nil || !strings.Contains(err.Error(), "no current-context configured") {
		t.Errorf("configureKubectl(empty kubeconfig) = %v, want no current-context configured error", err)
	}

	danglingCfg := clientcmdapi.NewConfig()
	danglingCfg.CurrentContext = "deleted-ctx"
	orcDangling := newTestGKEOrchestrator(NewMockExecutor(nil))
	orcDangling.mode = orchestrator.TypeK8s
	orcDangling.kubeconfigLoader = staticLoader(danglingCfg)
	if err := orcDangling.configureKubectl("", "", ""); err == nil || !strings.Contains(err.Error(), `"deleted-ctx"`) || !strings.Contains(err.Error(), "was not found") {
		t.Errorf("configureKubectl(dangling current-context) = %v, want error naming \"deleted-ctx\"", err)
	}

	orcBadLoader := newTestGKEOrchestrator(NewMockExecutor(nil))
	orcBadLoader.mode = orchestrator.TypeK8s
	orcBadLoader.kubeconfigLoader = func() (*clientcmdapi.Config, error) { return nil, errors.New("load fault") }
	if err := orcBadLoader.configureKubectl("local-k8s", "", ""); err == nil || !strings.Contains(err.Error(), "load fault") {
		t.Errorf("configureKubectl(bad loader) = %v, want load fault error", err)
	}
}

func TestTypeK8s_ConfigureKubectlReResolutionSyncsDynamicClient(t *testing.T) {
	var hitsA, hitsB atomic.Int32
	emptyListBody := `{"apiVersion":"jobset.x-k8s.io/v1alpha2","kind":"JobSetList","items":[]}`
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hitsA.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emptyListBody))
	}))
	t.Cleanup(srvA.Close)
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hitsB.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emptyListBody))
	}))
	t.Cleanup(srvB.Close)

	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["cluster-a"] = &clientcmdapi.Cluster{Server: srvA.URL}
	cfg.Clusters["cluster-b"] = &clientcmdapi.Cluster{Server: srvB.URL}
	cfg.Contexts["ctx-a"] = &clientcmdapi.Context{Cluster: "cluster-a", Namespace: "ns-a"}
	cfg.Contexts["ctx-b"] = &clientcmdapi.Context{Cluster: "cluster-b", Namespace: "ns-b"}

	rawOrc, err := orchestrator.NewJobOrchestrator(orchestrator.Detection{
		Type:        orchestrator.TypeK8s,
		KubeContext: "ctx-a",
		ClusterName: "cluster-a",
	})
	if err != nil {
		t.Fatalf("NewJobOrchestrator error = %v", err)
	}
	orc := rawOrc.(*GKEOrchestrator)
	orc.SetExecutor(NewMockExecutor(nil))
	orc.kubeconfigLoader = staticLoader(cfg)

	if _, err := orc.ListJobs(orchestrator.ListOptions{ClusterName: "cluster-a"}); err != nil {
		t.Fatalf("ListJobs(cluster-a) error = %v", err)
	}
	if hitsA.Load() != 1 || hitsB.Load() != 0 {
		t.Fatalf("hits after cluster-a = (%d, %d), want (1, 0)", hitsA.Load(), hitsB.Load())
	}

	if _, err := orc.ListJobs(orchestrator.ListOptions{ClusterName: "cluster-b"}); err != nil {
		t.Fatalf("ListJobs(cluster-b) error = %v", err)
	}
	if hitsA.Load() != 1 || hitsB.Load() != 1 {
		t.Fatalf("hits after direct ListJobs(cluster-b) = (%d, %d), want (1, 1)", hitsA.Load(), hitsB.Load())
	}
	kc := orc.usableKubeClient()
	if kc == nil {
		t.Fatal("usableKubeClient() after context switch = nil, want DefaultKubeClient bound to ctx-b")
	}
	if _, err := kc.ListJobSets("ns-b", ""); err != nil {
		t.Fatalf("usableKubeClient().ListJobSets(ns-b) error = %v", err)
	}
	if hitsA.Load() != 1 || hitsB.Load() != 2 {
		t.Fatalf("hits after cluster-b usableKubeClient = (%d, %d), want (1, 2)", hitsA.Load(), hitsB.Load())
	}
	assertUnseededClusterAndErrorInvalidation(t, cfg)
	assertSameContextGKERegenerateInvalidatesDynamicClient(t, srvA.URL, srvB.URL, &hitsA, &hitsB)
	assertInjectedDynamicClientPreserved(t, orc)
}

func assertSameContextGKERegenerateInvalidatesDynamicClient(t *testing.T, urlA, urlB string, hitsA, hitsB *atomic.Int32) {
	t.Helper()
	activeURL := urlA
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		setNSCmd:   {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.mode = orchestrator.TypeGKE
	orc.kubeClient = nil
	orc.clusterDesc = testClusterDesc(false, true)
	orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) {
		c := clientcmdapi.NewConfig()
		c.Clusters[testContext] = &clientcmdapi.Cluster{Server: activeURL}
		c.Contexts[testContext] = &clientcmdapi.Context{Cluster: testContext, Namespace: "team-ns"}
		return c, nil
	}
	orc.kubeContext = testContext

	beforeA, beforeB := hitsA.Load(), hitsB.Load()
	if kc := orc.usableKubeClient(); kc == nil {
		t.Fatal("initial GKE usableKubeClient() = nil")
	} else if _, err := kc.ListJobSets("team-ns", ""); err != nil {
		t.Fatalf("initial GKE ListJobSets error = %v", err)
	}
	if hitsA.Load() != beforeA+1 || hitsB.Load() != beforeB {
		t.Fatalf("hits before same-context regenerate = (%d, %d), want (%d, %d)", hitsA.Load(), hitsB.Load(), beforeA+1, beforeB)
	}

	activeURL = urlB
	if err := orc.regenerateKubeContext(testCluster, testLocation, testProject, endpointModeUnknown); err != nil {
		t.Fatalf("regenerateKubeContext error = %v", err)
	}
	if _, err := orc.getDynamicClient(); err != nil {
		t.Fatalf("getDynamicClient after same-context regenerate error = %v", err)
	}
	if _, err := orc.kubeClient.ListJobSets("team-ns", ""); err != nil {
		t.Fatalf("ListJobSets after same-context regenerate error = %v", err)
	}
	if hitsA.Load() != beforeA+1 || hitsB.Load() != beforeB+1 {
		t.Fatalf("hits after same-context regenerate = (%d, %d), want (%d, %d)", hitsA.Load(), hitsB.Load(), beforeA+1, beforeB+1)
	}
}

func assertUnseededClusterAndErrorInvalidation(t *testing.T, cfg *clientcmdapi.Config) {
	t.Helper()
	cfgWithCurrent := cfg.DeepCopy()
	cfgWithCurrent.CurrentContext = "ctx-a"

	rawOrc, err := orchestrator.NewJobOrchestrator(orchestrator.Detection{
		Type:        orchestrator.TypeK8s,
		KubeContext: "ctx-a",
	})
	if err != nil {
		t.Fatalf("NewJobOrchestrator(empty ClusterName) error = %v", err)
	}
	orc := rawOrc.(*GKEOrchestrator)
	orc.SetExecutor(NewMockExecutor(nil))
	orc.kubeconfigLoader = staticLoader(cfgWithCurrent)

	if err := orc.configureKubectl("cluster-b", "", ""); err != nil || orc.kubeContext != "ctx-b" || orc.k8sCluster != "cluster-b" {
		t.Fatalf("configureKubectl(cluster-b with empty initial k8sCluster) = (%v, %q, %q), want (nil, ctx-b, cluster-b)", err, orc.kubeContext, orc.k8sCluster)
	}
	if orc.usableKubeClient() == nil {
		t.Fatal("usableKubeClient() after cluster-b resolution = nil, want initialized client")
	}

	if err := orc.configureKubectl("unknown-cluster", "", ""); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("configureKubectl(unknown-cluster) = %v, want error containing \"was not found\"", err)
	}
	if orc.dynClient != nil {
		t.Fatalf("dynClient after failed configureKubectl = %v, want nil", orc.dynClient)
	}
	if orc.kubeContext != "" || orc.k8sCluster != "" {
		t.Fatalf("after failed configureKubectl: kubeContext=%q k8sCluster=%q, want both empty", orc.kubeContext, orc.k8sCluster)
	}
	if kc := orc.usableKubeClient(); kc != nil {
		t.Fatalf("usableKubeClient() after failed configureKubectl = %v, want nil (must not fall back to CurrentContext)", kc)
	}
}

func assertInjectedDynamicClientPreserved(t *testing.T, orc *GKEOrchestrator) {
	t.Helper()
	// SetDynamicClient marks dynClient unmanaged so test doubles survive context switches.
	injected := &mockDynamicClient{}
	orc.SetDynamicClient(injected)
	if err := orc.configureKubectl("cluster-a", "", ""); err != nil {
		t.Fatalf("configureKubectl(cluster-a) with injected client error = %v", err)
	}
	gotDyn, err := orc.getDynamicClient()
	if err != nil || gotDyn != injected {
		t.Fatalf("getDynamicClient() with injected client = (%v, %v), want (%v, nil)", gotDyn, err, injected)
	}
}

func assertStorageCachesReset(t *testing.T, orc *GKEOrchestrator, step string) {
	t.Helper()
	if len(orc.inspectedPVCs) != 0 || orc.nfsAuthChecked {
		t.Fatalf("after %s: inspectedPVCs=%v nfsAuthChecked=%v, want empty and false", step, orc.inspectedPVCs, orc.nfsAuthChecked)
	}
}

func TestTypeK8s_ConfigureKubectlResetsStorageCaches(t *testing.T) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["cluster-a"] = &clientcmdapi.Cluster{Server: "https://10.0.0.1:6443"}
	cfg.Clusters["cluster-b"] = &clientcmdapi.Cluster{Server: "https://10.0.0.2:6443"}
	cfg.Contexts["ctx-a"] = &clientcmdapi.Context{Cluster: "cluster-a", Namespace: "ns-a"}
	cfg.Contexts["ctx-b"] = &clientcmdapi.Context{Cluster: "cluster-b", Namespace: "ns-b"}

	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.mode = orchestrator.TypeK8s
	orc.kubeconfigLoader = staticLoader(cfg)
	orc.kubeContext = "ctx-a"
	orc.k8sCluster = "cluster-a"

	seedCaches := func() {
		orc.inspectedPVCs = map[string]pvcInspectResult{"ns-a/stale": {volumeName: "pv-old"}}
		orc.nfsAuthChecked = true
	}

	// 1. Context switch ("cluster-a" -> "cluster-b") clears storage caches.
	seedCaches()
	if err := orc.configureKubectl("cluster-b", "", ""); err != nil || orc.kubeContext != "ctx-b" {
		t.Fatalf("configureKubectl(cluster-b) = (%v, %q), want (nil, ctx-b)", err, orc.kubeContext)
	}
	assertStorageCachesReset(t, orc, "context switch")

	// 2. Same-context short-circuit reuse ("cluster-b" -> "cluster-b") also clears storage caches.
	seedCaches()
	if err := orc.configureKubectl("cluster-b", "", ""); err != nil || orc.kubeContext != "ctx-b" {
		t.Fatalf("configureKubectl(same cluster-b) = (%v, %q), want (nil, ctx-b)", err, orc.kubeContext)
	}
	assertStorageCachesReset(t, orc, "same-context reuse")

	// 3. Failed context lookup ("unknown-cluster") clears storage caches alongside kubeContext.
	seedCaches()
	if err := orc.configureKubectl("unknown-cluster", "", ""); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("configureKubectl(unknown-cluster) = %v, want error containing \"was not found\"", err)
	}
	assertStorageCachesReset(t, orc, "failed lookup")

	// 4. Kubeconfig loader failure also clears storage caches before configureK8sKubectl returns.
	orc.kubeconfigLoader = func() (*clientcmdapi.Config, error) { return nil, errors.New("load fault") }
	seedCaches()
	if err := orc.configureKubectl("cluster-a", "", ""); err == nil || !strings.Contains(err.Error(), "load fault") {
		t.Fatalf("configureKubectl(load fault) = %v, want load fault error", err)
	}
	assertStorageCachesReset(t, orc, "kubeconfig load fault")
}

func TestTypeK8s_LogsAllPodsHintsAndBatchedStreamScaling(t *testing.T) {
	twelvePods := strings.Repeat("workload-container sidecar-a sidecar-b\n", 12)
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get pods": {
			{ExitCode: 0, Stdout: twelvePods},
			{ExitCode: 0, Stdout: twelvePods},
			{ExitCode: 0, Stdout: twelvePods},
			{ExitCode: 0, Stdout: twelvePods},
		},
		"kubectl get jobsets": {
			{ExitCode: 0, Stdout: "\tFalse\tmain-job\n12 1\n"},
			{ExitCode: 0, Stdout: "\tFalse\tmain-job\n12 1\n"},
			{ExitCode: 0, Stdout: "\tFalse\tmain-job\n12 1\n"},
			{ExitCode: 0, Stdout: "\tFalse\tmain-job\n12 1\n"},
		},
		"kubectl logs": {
			{ExitCode: 0, Stdout: "rank-0 log line\n"},
			{ExitCode: 0, Stdout: "rank-0 log line\n"},
		},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.mode = orchestrator.TypeK8s
	orc.kubeContext = "k8s ctx"
	orc.k8sCluster = "local-k8s"
	orc.kubeClient = &MockKubeClient{Namespace: "ml-ns"}

	falseVal := false
	_, err := orc.GetJobLogs("big-job", orchestrator.LogsOptions{ClusterName: "local-k8s", MainOnly: &falseVal})
	wantSnapCmd := `kubectl --context "k8s ctx" logs -n ml-ns -l jobset.sigs.k8s.io/jobset-name=big-job --all-containers --prefix --max-log-requests=36 --tail=-1`
	if err == nil || strings.Contains(err.Error(), "console.cloud.google.com") || !strings.Contains(err.Error(), `"big-job"`) || !strings.Contains(err.Error(), wantSnapCmd) {
		t.Fatalf("GetJobLogs(MainOnly=false, Follow=false) = %v, want %q without Cloud Console URL", err, wantSnapCmd)
	}

	_, err = orc.GetJobLogs("big-job", orchestrator.LogsOptions{ClusterName: "local-k8s", MainOnly: &falseVal, Follow: true})
	wantFollowCmd := `kubectl --context "k8s ctx" logs -n ml-ns -l jobset.sigs.k8s.io/jobset-name=big-job --all-containers --prefix -f --max-log-requests=36 --tail=-1`
	if err == nil || strings.Contains(err.Error(), "console.cloud.google.com") || !strings.Contains(err.Error(), wantFollowCmd) {
		t.Fatalf("GetJobLogs(MainOnly=false, Follow=true) = %v, want %q", err, wantFollowCmd)
	}

	assertTypeK8sDefaultLogsInfoOutput(t, orc, wantSnapCmd, wantFollowCmd)
	assertMainOnlyDefaultSixPodsAndGKEConsoleHints(t)
	assertTypeK8sBatchedSlowStartStreamScaling(t)
	mockExec.assertAllKubectlPinned(t, "k8s ctx")
}

func assertMainOnlyDefaultSixPodsAndGKEConsoleHints(t *testing.T) {
	t.Helper()
	sixPods := strings.Repeat("workload-container\n", 6)
	elevenPods := strings.Repeat("workload-container\n", 11)
	exec6 := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get pods":    {{ExitCode: 0, Stdout: sixPods}},
		"kubectl get jobsets": {{ExitCode: 0, Stdout: "\tFalse\tmain-job\n6 1\tworkload-container\n"}},
		"kubectl logs":        {{ExitCode: 0, Stdout: "ok\n"}},
	})
	orc6 := newTestGKEOrchestrator(exec6)
	orc6.mode = orchestrator.TypeK8s
	orc6.kubeContext = "k8s-ctx"
	orc6.k8sCluster = "local-k8s"
	orc6.kubeClient = &MockKubeClient{Namespace: "ml-ns"}
	snap6, restore6 := logging.CaptureForTesting(t)
	defer restore6()
	_, err := orc6.GetJobLogs("six-pod-job", orchestrator.LogsOptions{ClusterName: "local-k8s"})
	info6, _ := snap6()
	restore6()
	if err != nil || !strings.Contains(info6, "--main-only=false") {
		t.Fatalf("6-pod MainOnly=nil GetJobLogs = (%v, %q), want --main-only=false hint", err, info6)
	}

	exec11 := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get --raw /version": {{ExitCode: 0, Stdout: `{"gitVersion":"v1.32.2-gke.1182000"}`}},
		"kubectl get pods":           {{ExitCode: 0, Stdout: elevenPods}},
		"kubectl get jobsets":        {{ExitCode: 0, Stdout: "\tFalse\tmain-job\n11 1\tworkload-container\n"}},
		"kubectl logs":               {{ExitCode: 0, Stdout: "ok\n"}},
	})
	orcGKE := newTestGKEOrchestrator(exec11)
	orcGKE.mode = orchestrator.TypeGKE
	orcGKE.clusterDesc = testClusterDesc(false, true)
	orcGKE.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testPublicIP, []byte(testCAPEM), gkeExec(), testContext))
	orcGKE.kubeClient = &MockKubeClient{Namespace: "ml-ns"}
	snapGKE, restoreGKE := logging.CaptureForTesting(t)
	defer restoreGKE()
	_, err = orcGKE.GetJobLogs("eleven-pod-job", orchestrator.LogsOptions{
		ClusterName:     testCluster,
		ClusterLocation: testLocation,
		ProjectID:       testProject,
	})
	infoGKE, _ := snapGKE()
	restoreGKE()
	if err != nil || !strings.Contains(infoGKE, "console.cloud.google.com") {
		t.Fatalf("11-pod GKE MainOnly=nil GetJobLogs = (%v, %q), want Cloud Console URL hint", err, infoGKE)
	}
}

func assertTypeK8sDefaultLogsInfoOutput(t *testing.T, orc *GKEOrchestrator, wantSnapCmd, wantFollowCmd string) {
	t.Helper()
	snapLogs, restoreLogs := logging.CaptureForTesting(t)
	defer restoreLogs()
	if _, err := orc.GetJobLogs("big-job", orchestrator.LogsOptions{ClusterName: "local-k8s"}); err != nil {
		t.Fatalf("GetJobLogs(MainOnly=nil, Follow=false) error = %v", err)
	}
	if _, err := orc.GetJobLogs("big-job", orchestrator.LogsOptions{ClusterName: "local-k8s", Follow: true}); err != nil {
		t.Fatalf("GetJobLogs(MainOnly=nil, Follow=true) error = %v", err)
	}
	infoOut, _ := snapLogs()
	for _, want := range []string{wantSnapCmd, wantFollowCmd} {
		if !strings.Contains(infoOut, want) {
			t.Errorf("captured info logs missing %q; got:\n%s", want, infoOut)
		}
	}
	if strings.Contains(infoOut, "console.cloud.google.com") {
		t.Errorf("captured info logs contained Cloud Console URL:\n%s", infoOut)
	}
}

func assertTypeK8sBatchedSlowStartStreamScaling(t *testing.T) {
	t.Helper()
	// Verify both JobSet container counting and live-pod ceiling fallback when only 2 of 12 pods are scheduled.
	if streams := scaleExpectedStreams(2, 6, 12, 0); streams != 36 {
		t.Errorf("scaleExpectedStreams(2, 6, 12, 0) = %d, want 36", streams)
	}
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get pods": {
			{ExitCode: 0, Stdout: "c1 c2 c3\nc1 c2 c3\n"},
		},
		"kubectl get jobsets": {
			{ExitCode: 0, Stdout: "\tFalse\tpathways-head\n1 1\tc1 c2\n1 11\tc1 c2 c3\n"},
		},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.mode = orchestrator.TypeK8s
	orc.kubeContext = "k8s-ctx"
	orc.k8sCluster = "local-k8s"
	orc.kubeClient = &MockKubeClient{Namespace: "ml-ns"}

	falseVal := false
	_, err := orc.GetJobLogs("batched-job", orchestrator.LogsOptions{ClusterName: "local-k8s", MainOnly: &falseVal, Follow: true})
	want := "kubectl --context k8s-ctx logs -n ml-ns -l jobset.sigs.k8s.io/jobset-name=batched-job --all-containers --prefix -f --max-log-requests=35 --tail=-1"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("GetJobLogs(batched slow-start) = %v, want %q", err, want)
	}
}

func TestTypeK8s_CancelJobPinsContextAndZeroGCloud(t *testing.T) {
	var deletedPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deletedPath.Store(r.URL.Path)
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Success"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/jobsets/job-1") {
			_, _ = w.Write([]byte(`{"apiVersion":"jobset.x-k8s.io/v1alpha2","kind":"JobSet","metadata":{"name":"job-1","namespace":"ml-ns"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"List","items":[]}`))
	}))
	t.Cleanup(srv.Close)

	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["local-k8s"] = &clientcmdapi.Cluster{Server: srv.URL}
	cfg.Contexts["k8s-ctx"] = &clientcmdapi.Context{Cluster: "local-k8s", Namespace: "ml-ns"}

	rawOrc, err := orchestrator.NewJobOrchestrator(orchestrator.Detection{
		Type:        orchestrator.TypeK8s,
		KubeContext: "k8s-ctx",
		ClusterName: "local-k8s",
	})
	if err != nil {
		t.Fatalf("NewJobOrchestrator error = %v", err)
	}
	orc := rawOrc.(*GKEOrchestrator)
	orc.kubeconfigLoader = staticLoader(cfg)
	mockExec := NewMockExecutor(nil)
	orc.SetExecutor(mockExec)

	if err := orc.CancelJob("job-1", orchestrator.CancelOptions{ClusterName: "local-k8s"}); err != nil {
		t.Fatalf("CancelJob(TypeK8s) error = %v, want nil", err)
	}
	if got, _ := deletedPath.Load().(string); got != "/apis/jobset.x-k8s.io/v1alpha2/namespaces/ml-ns/jobsets/job-1" {
		t.Errorf("DELETE path = %q, want /apis/jobset.x-k8s.io/v1alpha2/namespaces/ml-ns/jobsets/job-1", got)
	}
	if len(mockExec.calls) != 0 {
		t.Errorf("CancelJob(TypeK8s) unexpected subprocesses = %v, want 0", mockExec.calls)
	}
}

func TestTypeK8s_InspectClusterFailsFastOnUnreachableOrUnauthorizedContext(t *testing.T) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["local-k8s"] = &clientcmdapi.Cluster{Server: "https://10.128.0.68:6443"}
	cfg.Contexts["k8s-ctx"] = &clientcmdapi.Context{Cluster: "local-k8s", Namespace: "ml-ns"}

	for _, tc := range []struct {
		name       string
		stderr     string
		wantSubstr string
	}{
		{
			name:       "unreachable_endpoint",
			stderr:     "Unable to connect to the server: dial tcp 10.128.0.68:6443: i/o timeout",
			wantSubstr: "unreachable",
		},
		{
			name:       "connection_refused_kubectl_rewrite",
			stderr:     "The connection to the server 127.0.0.1:6443 was refused - did you specify the right host or port?",
			wantSubstr: "unreachable",
		},
		{
			name:       "service_unavailable_503",
			stderr:     "Error from server (ServiceUnavailable): the server is currently unable to handle the request",
			wantSubstr: "unreachable",
		},
		{
			name:       "expired_credentials",
			stderr:     "error: You must be logged in to the server (Unauthorized)",
			wantSubstr: "authentication to Kubernetes context",
		},
		{
			name:       "expired_oidc_id_token",
			stderr:     "Unable to connect to the server: No valid id-token, and cannot refresh without refresh-token",
			wantSubstr: "authentication to Kubernetes context",
		},
		{
			name:       "missing_certificate_authority_file",
			stderr:     "error: unable to read certificate-authority /missing/ca.crt for local-k8s due to open /missing/ca.crt: no such file or directory",
			wantSubstr: "cannot verify Kubernetes context",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				"kubectl get --raw /version": {{ExitCode: 1, Stderr: tc.stderr}},
			})
			orc := newTestGKEOrchestrator(mockExec)
			orc.mode = orchestrator.TypeK8s
			orc.kubeconfigLoader = staticLoader(cfg)
			orc.kubeContext = "k8s-ctx"
			orc.k8sCluster = "local-k8s"
			outPath := filepath.Join(t.TempDir(), "must-not-exist.log")

			err := orc.InspectCluster(orchestrator.InspectOptions{ClusterName: "local-k8s", OutputPath: outPath})
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("InspectCluster(%s) = %v, want error containing %q", tc.name, err, tc.wantSubstr)
			}
			if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
				t.Errorf("InspectCluster(%s) created %s despite pre-flight failure", tc.name, outPath)
			}
			if len(mockExec.calls) != 1 {
				t.Errorf("InspectCluster(%s) subprocess calls = %v, want single /version probe", tc.name, mockExec.calls)
			}
		})
	}

	assertTypeK8sInspectProceedsOnForbiddenAndUnknownProbe(t, cfg)
}

func assertTypeK8sInspectProceedsOnForbiddenAndUnknownProbe(t *testing.T, cfg *clientcmdapi.Config) {
	t.Helper()
	for _, tc := range []struct {
		name     string
		stderr   string
		wantWarn bool
	}{
		{
			name:   "rbac_forbidden_on_version_proceeds",
			stderr: `Error from server (Forbidden): forbidden: User "dev" cannot get path "/version"`,
		},
		{
			name:     "unknown_probe_error_warns_and_proceeds",
			stderr:   "custom apiserver banner error",
			wantWarn: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockExec := newTypeK8sInspectMockExecutor()
			mockExec.responses["kubectl get --raw /version"] = []shell.CommandResult{{ExitCode: 1, Stderr: tc.stderr}}
			orc := newTestGKEOrchestrator(mockExec)
			orc.mode = orchestrator.TypeK8s
			orc.kubeconfigLoader = staticLoader(cfg)
			orc.kubeContext = "k8s-ctx"
			orc.k8sCluster = "local-k8s"
			outPath := filepath.Join(t.TempDir(), "inspect-ok.log")

			snapLogs, restoreLogs := logging.CaptureForTesting(t)
			defer restoreLogs()
			if err := orc.InspectCluster(orchestrator.InspectOptions{ClusterName: "local-k8s", OutputPath: outPath}); err != nil {
				t.Fatalf("InspectCluster(%s) error = %v, want nil", tc.name, err)
			}
			_, warnErrOut := snapLogs()
			if tc.wantWarn != strings.Contains(warnErrOut, "Could not verify Kubernetes context") {
				t.Errorf("InspectCluster(%s) warn output = %q, wantWarn=%v", tc.name, warnErrOut, tc.wantWarn)
			}
			if _, statErr := os.Stat(outPath); statErr != nil {
				t.Errorf("InspectCluster(%s) did not create %s: %v", tc.name, outPath, statErr)
			}
		})
	}
}

func TestDetectGKE_IncompleteContextSelfHealing(t *testing.T) {
	cfg := clientcmdapi.NewConfig()
	cfg.Contexts["half-written-ctx"] = &clientcmdapi.Context{Cluster: "missing-cluster", AuthInfo: "missing-user"}
	cfg.Clusters["empty-server-clus"] = &clientcmdapi.Cluster{Server: "   "}
	cfg.Clusters["valid-server-clus"] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
	cfg.AuthInfos["valid-user"] = &clientcmdapi.AuthInfo{Token: "tok"}
	cfg.Contexts["empty-server-ctx"] = &clientcmdapi.Context{Cluster: "empty-server-clus", AuthInfo: "valid-user"}
	cfg.Contexts["missing-user-ctx"] = &clientcmdapi.Context{Cluster: "valid-server-clus", AuthInfo: "missing-user"}
	cfg.CurrentContext = "half-written-ctx"

	runnerCalls := 0
	runner := func(_ time.Duration, _ string, _ ...string) shell.CommandResult {
		runnerCalls++
		return shell.CommandResult{ExitCode: 1}
	}

	if det, ok := detectGKE(orchestrator.ProbeInput{Config: cfg, Runner: runner}); ok || runnerCalls != 0 {
		t.Fatalf("detectGKE(incomplete context without GKE flags) = (%+v, %v), runnerCalls=%d, want (_, false, 0)", det, ok, runnerCalls)
	}
	for _, ctxName := range []string{"half-written-ctx", "empty-server-ctx", "missing-user-ctx"} {
		det, ok := detectGKE(orchestrator.ProbeInput{
			Config:      cfg,
			ClusterName: ctxName,
			Location:    "us-central1-a",
			ProjectID:   "my-proj",
			Runner:      runner,
		})
		if !ok || det.Type != orchestrator.TypeGKE || det.KubeContext != ctxName || runnerCalls != 0 {
			t.Fatalf("detectGKE(%s with GKE flags) = (%+v, %v), runnerCalls=%d, want (TypeGKE, %s, true, 0)", ctxName, det, ok, runnerCalls, ctxName)
		}
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
