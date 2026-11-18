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

func TestWithContextAndDefaultLoader(t *testing.T) {
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
