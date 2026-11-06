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
	cfg.Contexts["alpha-ctx"] = &clientcmdapi.Context{Cluster: "shared-cluster"}
	cfg.Contexts["zeta-ctx"] = &clientcmdapi.Context{Cluster: "shared-cluster"}
	cfg.Contexts["exact-name"] = &clientcmdapi.Context{Cluster: "other-cluster"}
	cfg.Contexts["alpha-exact"] = &clientcmdapi.Context{Cluster: "exact-name"}

	if name, ctx, ok := FindContextByCluster(cfg, "exact-name"); !ok || name != "exact-name" || ctx.Cluster != "other-cluster" {
		t.Errorf("exact context name should win over cluster entry match: got (%q, %+v, %v)", name, ctx, ok)
	}
	if name, _, ok := FindContextByCluster(cfg, "shared-cluster"); !ok || name != "alpha-ctx" {
		t.Errorf("sorted cluster-entry fallback = (%q, %v), want (alpha-ctx, true)", name, ok)
	}
	if _, _, ok := FindContextByCluster(cfg, "missing"); ok {
		t.Errorf("expected missing cluster to return false")
	}
	if _, _, ok := FindContextByCluster(nil, "exact-name"); ok {
		t.Errorf("expected nil config to return false")
	}
	if _, _, ok := FindContextByCluster(cfg, ""); ok {
		t.Errorf("expected empty name to return false")
	}
}

func TestResolveCurrentContext(t *testing.T) {
	t.Parallel()
	cfg := clientcmdapi.NewConfig()
	cfg.Contexts["prod-ctx"] = &clientcmdapi.Context{Cluster: "prod-cluster"}
	cfg.CurrentContext = "prod-ctx"

	name, kubeCtx, clusterName, ok := ResolveCurrentContext(cfg)
	if !ok || name != "prod-ctx" || kubeCtx == nil || clusterName != "prod-cluster" {
		t.Fatalf("ResolveCurrentContext() = (%q, %v, %q, %v), want (prod-ctx, non-nil, prod-cluster, true)", name, kubeCtx, clusterName, ok)
	}

	cfg.Contexts["nameless-cluster-ctx"] = &clientcmdapi.Context{}
	cfg.CurrentContext = "nameless-cluster-ctx"
	if name, _, clusterName, ok := ResolveCurrentContext(cfg); !ok || name != "nameless-cluster-ctx" || clusterName != "nameless-cluster-ctx" {
		t.Errorf("expected fallback clusterName = contextName when Context.Cluster is empty, got (%q, %q, %v)", name, clusterName, ok)
	}

	cfg.CurrentContext = "dangling"
	if _, _, _, ok := ResolveCurrentContext(cfg); ok {
		t.Errorf("expected dangling current-context to return false")
	}
	if _, _, _, ok := ResolveCurrentContext(nil); ok {
		t.Errorf("expected nil config to return false")
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
}

func TestWithContextAndDefaultLoader(t *testing.T) {
	args := []string{"get", "pods"}
	if got := WithContext("", args); !slices.Equal(got, args) {
		t.Errorf("WithContext empty = %v, want %v", got, args)
	}
	if got, want := WithContext("my-ctx", args), []string{"--context", "my-ctx", "get", "pods"}; !slices.Equal(got, want) {
		t.Errorf("WithContext(my-ctx) = %v, want %v", got, want)
	}

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
}
