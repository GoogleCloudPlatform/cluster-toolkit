// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestFindContextByCluster(t *testing.T) {
	t.Parallel()
	cfg := clientcmdapi.NewConfig()
	cfg.Contexts[""] = &clientcmdapi.Context{Cluster: "shared-cluster"}
	cfg.Contexts["   "] = &clientcmdapi.Context{Cluster: "shared-cluster"}
	cfg.Contexts["alpha-ctx"] = &clientcmdapi.Context{Cluster: " shared-cluster "}
	cfg.Contexts["zeta-ctx"] = &clientcmdapi.Context{Cluster: " shared-cluster "}
	cfg.Contexts["exact-name"] = &clientcmdapi.Context{Cluster: "other-cluster"}
	cfg.Contexts["alpha-exact"] = &clientcmdapi.Context{Cluster: "exact-name"}

	cfg.CurrentContext = "alpha-exact"
	if name, ctx, ok := FindContextByCluster(cfg, " exact-name "); !ok || name != "exact-name" || ctx.Cluster != "other-cluster" {
		t.Errorf("exact context name should win over current-context and cluster entry match: got (%q, %+v, %v)", name, ctx, ok)
	}
	cfg.CurrentContext = ""
	if name, _, ok := FindContextByCluster(cfg, "shared-cluster"); !ok || name != "alpha-ctx" {
		t.Errorf("sorted cluster-entry fallback should skip blank context names: got (%q, %v), want (alpha-ctx, true)", name, ok)
	}
	cfg.CurrentContext = "zeta-ctx"
	if name, _, ok := FindContextByCluster(cfg, "shared-cluster"); !ok || name != "zeta-ctx" {
		t.Errorf("active current-context matching cluster should win over alphabetical fallback: got (%q, %v), want (zeta-ctx, true)", name, ok)
	}
	cfg.Contexts["nil-ctx"] = nil
	cfg.Contexts[" shared-cluster "] = &clientcmdapi.Context{Cluster: ""}
	cfg.CurrentContext = " shared-cluster "
	if name, _, ok := FindContextByCluster(cfg, "shared-cluster"); !ok || name != "alpha-ctx" {
		t.Errorf("padded current-context with blank cluster must not preempt real cluster match: got (%q, %v), want (alpha-ctx, true)", name, ok)
	}
	for _, tc := range []struct {
		name string
		cfg  *clientcmdapi.Config
		arg  string
	}{
		{"nil entry", cfg, "nil-ctx"},
		{"missing", cfg, "missing"},
		{"nil config", nil, "exact-name"},
		{"whitespace", cfg, "   "},
	} {
		if _, _, ok := FindContextByCluster(tc.cfg, tc.arg); ok {
			t.Errorf("FindContextByCluster(%s) = true, want false", tc.name)
		}
	}
}

func TestResolveCurrentContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		cfg         *clientcmdapi.Config
		wantOK      bool
		wantCtx     string
		wantCluster string
	}{
		{name: "nil config", cfg: nil},
		{name: "empty current-context", cfg: clientcmdapi.NewConfig()},
		{
			name: "whitespace-only current-context",
			cfg: &clientcmdapi.Config{
				CurrentContext: "   ",
				Contexts:       map[string]*clientcmdapi.Context{"   ": {Cluster: "prod-cluster"}},
			},
		},
		{
			name: "dangling current-context",
			cfg: &clientcmdapi.Config{
				CurrentContext: "dangling",
				Contexts:       map[string]*clientcmdapi.Context{"prod-ctx": {Cluster: "prod-cluster"}},
			},
		},
		{
			name: "valid current-context",
			cfg: &clientcmdapi.Config{
				CurrentContext: "prod-ctx",
				Contexts:       map[string]*clientcmdapi.Context{"prod-ctx": {Cluster: "prod-cluster"}},
			},
			wantOK:      true,
			wantCtx:     "prod-ctx",
			wantCluster: "prod-cluster",
		},
		{
			name: "blank cluster falls back to current-context",
			cfg: &clientcmdapi.Config{
				CurrentContext: "nameless-cluster-ctx",
				Contexts:       map[string]*clientcmdapi.Context{"nameless-cluster-ctx": {Cluster: "   "}},
			},
			wantOK:      true,
			wantCtx:     "nameless-cluster-ctx",
			wantCluster: "nameless-cluster-ctx",
		},
		{
			name: "padded current-context preserves verbatim key and trims fallback cluster",
			cfg: &clientcmdapi.Config{
				CurrentContext: " padded-ctx ",
				Contexts:       map[string]*clientcmdapi.Context{" padded-ctx ": {Cluster: "   "}},
			},
			wantOK:      true,
			wantCtx:     " padded-ctx ",
			wantCluster: "padded-ctx",
		},
		{
			name: "padded kubeCtx.Cluster is trimmed",
			cfg: &clientcmdapi.Config{
				CurrentContext: "padded-cluster-ctx",
				Contexts:       map[string]*clientcmdapi.Context{"padded-cluster-ctx": {Cluster: " trimmed-cluster "}},
			},
			wantOK:      true,
			wantCtx:     "padded-cluster-ctx",
			wantCluster: "trimmed-cluster",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCtx, gotEntry, gotCluster, gotOK := ResolveCurrentContext(tc.cfg)
			if gotOK != tc.wantOK || gotCtx != tc.wantCtx || gotCluster != tc.wantCluster || (tc.wantOK && gotEntry == nil) {
				t.Fatalf("ResolveCurrentContext() = (%q, %v, %q, %v), want (%q, non-nil=%v, %q, %v)",
					gotCtx, gotEntry != nil, gotCluster, gotOK, tc.wantCtx, tc.wantOK, tc.wantCluster, tc.wantOK)
			}
		})
	}
}

func TestResolveContext(t *testing.T) {
	t.Parallel()
	cfg := clientcmdapi.NewConfig()
	prodCtx := &clientcmdapi.Context{Cluster: "prod-cluster", Namespace: "prod-ns"}
	otherCtx := &clientcmdapi.Context{Cluster: "other-cluster", Namespace: "other-ns"}
	cfg.Contexts["prod-ctx"] = prodCtx
	cfg.Contexts["other-ctx"] = otherCtx
	cfg.CurrentContext = "prod-ctx"

	tests := []struct {
		clusterName string
		wantName    string
		wantCtx     *clientcmdapi.Context
		wantCluster string
		wantOK      bool
	}{
		{clusterName: "", wantName: "prod-ctx", wantCtx: prodCtx, wantCluster: "prod-cluster", wantOK: true},
		{clusterName: "prod-cluster", wantName: "prod-ctx", wantCtx: prodCtx, wantCluster: "prod-cluster", wantOK: true},
		{clusterName: "other-ctx", wantName: "other-ctx", wantCtx: otherCtx, wantCluster: "other-ctx", wantOK: true},
		{clusterName: "nonexistent", wantOK: false},
	}
	for _, tc := range tests {
		name, gotCtx, clusterName, ok := ResolveContext(cfg, tc.clusterName)
		if ok != tc.wantOK || name != tc.wantName || gotCtx != tc.wantCtx || clusterName != tc.wantCluster {
			t.Errorf("ResolveContext(cfg, %q) = (%q, %v, %q, %v), want (%q, %v, %q, %v)",
				tc.clusterName, name, gotCtx, clusterName, ok, tc.wantName, tc.wantCtx, tc.wantCluster, tc.wantOK)
		}
	}
}

func TestDanglingCurrentContext(t *testing.T) {
	t.Parallel()
	if got := DanglingCurrentContext(nil); got != "" {
		t.Errorf("DanglingCurrentContext(nil) = %q, want empty", got)
	}
	cfg := clientcmdapi.NewConfig()
	if got := DanglingCurrentContext(cfg); got != "" {
		t.Errorf("DanglingCurrentContext(unset) = %q, want empty", got)
	}
	cfg.CurrentContext = "   "
	if got := DanglingCurrentContext(cfg); got != "" {
		t.Errorf("DanglingCurrentContext(whitespace) = %q, want empty", got)
	}
	cfg.CurrentContext = "stale-ctx"
	if got := DanglingCurrentContext(cfg); got != "stale-ctx" {
		t.Errorf("DanglingCurrentContext(dangling) = %q, want %q", got, "stale-ctx")
	}
	cfg.Contexts["stale-ctx"] = &clientcmdapi.Context{Cluster: "c"}
	if got := DanglingCurrentContext(cfg); got != "" {
		t.Errorf("DanglingCurrentContext(valid) = %q, want empty", got)
	}
}

func TestExecPluginBaseName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{name: "POSIX binary path", cmd: "/usr/local/bin/custom-auth-plugin", want: "custom-auth-plugin"},
		{name: "Windows backslash and uppercase .EXE", cmd: `C:\Program Files\Auth\Custom-Auth-Plugin.EXE`, want: "custom-auth-plugin"},
		{name: "bare binary name", cmd: "kubelogin", want: "kubelogin"},
		{name: "empty command", cmd: "   ", want: ""},
		{name: "dot command", cmd: ".", want: ""},
		{name: "parent dir command", cmd: "..", want: ""},
		{name: "trailing parent dir command", cmd: "/usr/bin/..", want: ""},
		{name: "root slash command", cmd: "/", want: ""},
		{name: "Windows drive root command", cmd: `C:\`, want: ""},
		{name: "Windows drive dot-exe command", cmd: `C:.exe`, want: ""},
		{name: "bare .exe extension", cmd: ".exe", want: ""},
		{name: "double-dot .exe command", cmd: "..exe", want: ""},
		{name: "triple-dot .exe command", cmd: "...exe", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			auth := &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: tc.cmd}}
			if got := ExecPluginBaseName(auth); got != tc.want {
				t.Errorf("ExecPluginBaseName(%q) = %q, want %q", tc.cmd, got, tc.want)
			}
		})
	}
	if got := ExecPluginBaseName(nil); got != "" {
		t.Errorf("ExecPluginBaseName(nil) = %q, want empty", got)
	}
	if got := ExecPluginBaseName(&clientcmdapi.AuthInfo{}); got != "" {
		t.Errorf("ExecPluginBaseName(AuthInfo{Exec: nil}) = %q, want empty", got)
	}
}

func TestClusterServerHostname(t *testing.T) {
	t.Parallel()
	tests := []struct {
		server string
		want   string
	}{
		{server: "https://API.K8S.Example.COM:6443/", want: "api.k8s.example.com"},
		{server: "https://gke-123456.us-central1.gke.goog.:443/", want: "gke-123456.us-central1.gke.goog"},
		{server: "//my-cluster.us-central1.gke.goog:443", want: "my-cluster.us-central1.gke.goog"},
		{server: "my-cluster.us-central1.gke.goog:443", want: "my-cluster.us-central1.gke.goog"},
		{server: "https://[fd12:3456::1]:6443", want: "fd12:3456::1"},
		{server: "https://10.128.0.68:6443", want: "10.128.0.68"},
		{server: "://invalid", want: ""},
		{server: "", want: ""},
	}
	for _, tc := range tests {
		if got := ClusterServerHostname(&clientcmdapi.Cluster{Server: tc.server}); got != tc.want {
			t.Errorf("ClusterServerHostname(%q) = %q, want %q", tc.server, got, tc.want)
		}
	}
	if got := ClusterServerHostname(nil); got != "" {
		t.Errorf("ClusterServerHostname(nil) = %q, want empty", got)
	}
}

func TestHasClientCertificateAuth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		auth *clientcmdapi.AuthInfo
		want bool
	}{
		{
			name: "cert_data_only",
			auth: &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert-pem")},
			want: true,
		},
		{
			name: "cert_file_only",
			auth: &clientcmdapi.AuthInfo{ClientCertificate: "/etc/kubernetes/admin.crt"},
			want: true,
		},
		{
			name: "empty_auth_info",
			auth: &clientcmdapi.AuthInfo{},
			want: false,
		},
		{
			name: "bearer_token_only",
			auth: &clientcmdapi.AuthInfo{Token: "static-bearer-token"},
			want: false,
		},
		{
			name: "cert_plus_token",
			auth: &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert"), Token: "static-bearer-token"},
			want: false,
		},
		{
			name: "cert_plus_token_file",
			auth: &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert"), TokenFile: "/var/run/secrets/token"},
			want: false,
		},
		{
			name: "cert_plus_exec",
			auth: &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert"), Exec: &clientcmdapi.ExecConfig{Command: "plugin"}},
			want: false,
		},
		{
			name: "cert_plus_auth_provider",
			auth: &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert"), AuthProvider: &clientcmdapi.AuthProviderConfig{Name: "gcp"}},
			want: false,
		},
		{
			name: "nil_auth",
			auth: nil,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasClientCertificateAuth(tc.auth); got != tc.want {
				t.Errorf("HasClientCertificateAuth(%+v) = %v, want %v", tc.auth, got, tc.want)
			}
		})
	}
}

func TestWithContextAndContextNamespace(t *testing.T) {
	args := []string{"get", "pods"}
	if got := WithContext("", args); !slices.Equal(got, args) {
		t.Errorf("WithContext empty = %v, want %v", got, args)
	}
	if got := WithContext("   ", args); !slices.Equal(got, args) {
		t.Errorf("WithContext whitespace = %v, want %v", got, args)
	}
	if got, want := WithContext("my-ctx", args), []string{"--context", "my-ctx", "get", "pods"}; !slices.Equal(got, want) {
		t.Errorf("WithContext(my-ctx) = %v, want %v", got, want)
	}
	if got, want := WithContext(" my-ctx ", args), []string{"--context", " my-ctx ", "get", "pods"}; !slices.Equal(got, want) {
		t.Errorf("WithContext(' my-ctx ') = %v, want %v (must preserve exact context key)", got, want)
	}
	if got := ContextNamespace(nil); got != "default" {
		t.Errorf("ContextNamespace(nil) = %q, want default", got)
	}
	if got := ContextNamespace(&clientcmdapi.Context{Namespace: "   "}); got != "default" {
		t.Errorf("ContextNamespace(blank) = %q, want default", got)
	}
	if got := ContextNamespace(&clientcmdapi.Context{Namespace: " team-a "}); got != "team-a" {
		t.Errorf("ContextNamespace(team-a) = %q, want team-a", got)
	}
}

func TestDefaultKubeconfigLoader(t *testing.T) {
	kcPath := filepath.Join(t.TempDir(), "kubeconfig")
	raw := clientcmdapi.NewConfig()
	raw.Clusters["c"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:6443"}
	raw.Contexts["ctx"] = &clientcmdapi.Context{Cluster: "c"}
	raw.CurrentContext = "ctx"
	data, err := clientcmd.Write(*raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kcPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kcPath)

	loaded, err := DefaultKubeconfigLoader()
	if err != nil || loaded.CurrentContext != "ctx" {
		t.Fatalf("DefaultKubeconfigLoader() = (%+v, %v), want CurrentContext=ctx", loaded, err)
	}

	badPath := filepath.Join(t.TempDir(), "bad-kubeconfig")
	if err := os.WriteFile(badPath, []byte(":::not-valid-yaml\n\t["), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", badPath)
	if _, err := DefaultKubeconfigLoader(); err == nil {
		t.Fatalf("DefaultKubeconfigLoader() error = nil on malformed kubeconfig, want error")
	}
}

func TestOIDCAuthProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["oidc-k8s"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:6443"}
	cfg.AuthInfos["oidc-user"] = &clientcmdapi.AuthInfo{
		AuthProvider: &clientcmdapi.AuthProviderConfig{
			Name: "oidc",
			Config: map[string]string{
				"idp-issuer-url": "https://dex.example.com",
				"client-id":      "k8s-cli",
				"id-token":       "header.payload.sig",
			},
		},
	}
	cfg.Contexts["oidc-ctx"] = &clientcmdapi.Context{Cluster: "oidc-k8s", AuthInfo: "oidc-user"}
	cfg.CurrentContext = "oidc-ctx"

	restCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, "oidc-ctx", &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		t.Fatalf("ClientConfig(oidc) error = %v", err)
	}
	if _, err := rest.TransportFor(restCfg); err != nil {
		t.Fatalf("rest.TransportFor(oidc) error = %v, want registered oidc auth-provider", err)
	}
}
