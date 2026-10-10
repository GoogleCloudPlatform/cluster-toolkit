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
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/shell"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

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
	useCtxCmd   = "kubectl config use-context " + testContext
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
	return &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}
}

func staticLoader(cfg *clientcmdapi.Config) kubeconfigLoader {
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

func TestEvaluateExistingContext(t *testing.T) {
	caBytes := []byte(testCAPEM)
	desc := testClusterDesc(true, true)

	tests := []struct {
		name       string
		cfg        *clientcmdapi.Config
		desc       gkeCluster
		wantReuse  bool
		wantMode   endpointMode
		wantReason string
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
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "/usr/lib/google-cloud-sdk/bin/gke-gcloud-auth-plugin"}, testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:      "exec plugin with Windows .exe suffix is accepted",
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin.exe"}, testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
		{
			name:      "exec plugin with uppercase .EXE suffix and backslash Windows install path is accepted",
			cfg:       kubeconfigWith("https://"+testDNSHost, nil, &clientcmdapi.ExecConfig{Command: `C:\Program Files\Google\Cloud SDK\google-cloud-sdk\bin\GKE-GCLOUD-AUTH-PLUGIN.EXE`}, testContext),
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
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
			desc:      desc,
			wantReuse: true,
			wantMode:  endpointModeDNS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateExistingContext(tt.cfg, testCluster, testLocation, testProject, tt.desc)
			if got.reusable() != tt.wantReuse {
				t.Fatalf("reusable() = %v, want %v (reason=%q)", got.reusable(), tt.wantReuse, got.reason)
			}
			if tt.wantReuse && got.mode != tt.wantMode {
				t.Errorf("mode = %v, want %v", got.mode, tt.wantMode)
			}
			if !tt.wantReuse && !strings.Contains(got.reason, tt.wantReason) {
				t.Errorf("reason = %q, want substring %q", got.reason, tt.wantReason)
			}
		})
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
		{"Unable to connect to the server: getting credentials: exec: executable gke-gcloud-auth-plugin not found", probeAuth},
		{"Unable to connect to the server: getting credentials: exec: executable gke-gcloud-auth-plugin failed with exit code 1", probeAuth},
		{"could not get token: oauth2: cannot fetch token: 400 Bad Request", probeAuth},
		{"Error from server (Forbidden): forbidden: User cannot get path \"/version\"", probeForbidden},
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
	if exec.count(useCtxCmd) != 0 {
		t.Errorf("use-context must not run when the reused context is already current; calls=%v", exec.calls)
	}
	if exec.count(probeCmd) != 1 {
		t.Errorf("probe count = %d, want 1; calls=%v", exec.count(probeCmd), exec.calls)
	}
}

func TestConfigureKubectl_ReuseSwitchesCurrentContext(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd:  {{ExitCode: 0}},
		useCtxCmd: {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), "some-other-context"))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count(useCtxCmd) != 1 {
		t.Errorf("expected current-context to be switched to the reused context; calls=%v", exec.calls)
	}
	if exec.count(getCredsIP)+exec.count(getCredsDNS) != 0 {
		t.Errorf("get-credentials must not run on the reuse path; calls=%v", exec.calls)
	}
}

func TestConfigureKubectl_ReuseFallsBackToOtherEndpointOnConnectivityFailure(t *testing.T) {
	// Existing context is DNS mode but DNS is blocked for this client
	// (e.g. HTTP 431 behind a proxy). Expect regeneration in IP mode.
	exec := newExactExecutor(map[string][]shell.CommandResult{
		probeCmd:   {{ExitCode: 1, Stderr: "Request Header Fields Too Large"}, {ExitCode: 0}},
		getCredsIP: {{ExitCode: 0}},
		"kubectl config set-context --current --namespace=team-ns": {{ExitCode: 0}},
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
		"kubectl config set-context --current --namespace=team-ns": {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.kubeClient = &MockKubeClient{Namespace: "team-ns"}
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testPublicIP, []byte(testCAPEM), gkeExec(), testContext))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	wantOrder := []string{probeCmd, getCredsDNS, "kubectl config set-context --current --namespace=team-ns", probeCmd}
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
		probeCmd:  {{ExitCode: 1, Stderr: "mock error: unexpected command"}},
		useCtxCmd: {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://"+testDNSHost, nil, gkeExec(), "some-other-context"))

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v, want nil on unknown probe failure", err)
	}
	if exec.count(useCtxCmd) != 1 {
		t.Errorf("expected current-context to be switched to the reused context; calls=%v", exec.calls)
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
		"kubectl config set-context --current --namespace=team-ns": {{ExitCode: 0}},
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
	for _, v := range []string{"", "0", "false", "FALSE", "no"} {
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

func TestConfigureKubectl_RegenerationPreservesNamespace(t *testing.T) {
	exec := newExactExecutor(map[string][]shell.CommandResult{
		getCredsIP: {{ExitCode: 0}},
		probeCmd:   {{ExitCode: 0}},
		"kubectl config set-context --current --namespace=team-ns": {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(exec)
	orc.kubeClient = &MockKubeClient{Namespace: "team-ns"}
	orc.clusterDesc = testClusterDesc(true, true)
	orc.kubeconfigLoader = staticLoader(clientcmdapi.NewConfig())

	if err := orc.configureKubectl(testCluster, testLocation, testProject); err != nil {
		t.Fatalf("configureKubectl() error = %v", err)
	}
	if exec.count("kubectl config set-context --current --namespace=team-ns") != 1 {
		t.Errorf("namespace must be restored after get-credentials; calls=%v", exec.calls)
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

func TestCheckClusterConnectivity(t *testing.T) {
	const cmd = "kubectl get --raw /version --request-timeout=5s"
	tests := []struct {
		name    string
		result  shell.CommandResult
		wantErr string
	}{
		{name: "reachable", result: shell.CommandResult{ExitCode: 0}},
		{name: "forbidden is still connected", result: shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden): forbidden"}},
		{name: "auth failure", result: shell.CommandResult{ExitCode: 1, Stderr: "error: You must be logged in to the server (Unauthorized)"}, wantErr: "authentication to GKE cluster"},
		{name: "unreachable", result: shell.CommandResult{ExitCode: 1, Stderr: "dial tcp 10.0.0.1:443: i/o timeout"}, wantErr: "authorized networks"},
		{name: "proxy gateway error is a connectivity failure", result: shell.CommandResult{ExitCode: 1, Stderr: "error: the server responded with the status code 502 but did not return more information"}, wantErr: "authorized networks"},
		// Mirrors configureKubectl: unrecognised output must not block submission with a misleading network error.
		{name: "unrecognised output is not fatal", result: shell.CommandResult{ExitCode: 1, Stderr: "something completely different"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := newExactExecutor(map[string][]shell.CommandResult{cmd: {tt.result}})
			g := &GKEOrchestrator{executor: exec}
			err := g.checkClusterConnectivity(testCluster)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkClusterConnectivity() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkClusterConnectivity() error = %v, want containing %q", err, tt.wantErr)
			}
			if exec.count(cmd) != 1 {
				t.Errorf("probe calls = %d, want 1; calls=%v", exec.count(cmd), exec.calls)
			}
		})
	}
}
