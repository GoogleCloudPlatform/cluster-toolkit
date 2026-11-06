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
	"maps"
	"path"
	"slices"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// DefaultKubeconfigLoader loads the merged kubeconfig using client-go's standard KUBECONFIG / ~/.kube/config rules.
func DefaultKubeconfigLoader() (*clientcmdapi.Config, error) {
	return clientcmd.NewDefaultClientConfigLoadingRules().Load()
}

// FindContextByCluster resolves a kubeconfig context matching name, preferring an exact context name match,
// then cfg.CurrentContext when its cluster matches name, before scanning context cluster references in sorted order.
func FindContextByCluster(cfg *clientcmdapi.Config, name string) (string, *clientcmdapi.Context, bool) {
	if cfg == nil || name == "" {
		return "", nil, false
	}
	if kubeCtx, ok := cfg.Contexts[name]; ok && kubeCtx != nil {
		return name, kubeCtx, true
	}
	if curName, curCtx, curCluster, ok := ResolveCurrentContext(cfg); ok && curCluster == name {
		return curName, curCtx, true
	}
	for _, ctxName := range slices.Sorted(maps.Keys(cfg.Contexts)) {
		kubeCtx := cfg.Contexts[ctxName]
		if kubeCtx != nil && kubeCtx.Cluster == name {
			return ctxName, kubeCtx, true
		}
	}
	return "", nil, false
}

// ResolveCurrentContext resolves cfg.CurrentContext and returns its context name, context entry, and cluster name.
func ResolveCurrentContext(cfg *clientcmdapi.Config) (string, *clientcmdapi.Context, string, bool) {
	if cfg == nil || cfg.CurrentContext == "" {
		return "", nil, "", false
	}
	kubeCtx := cfg.Contexts[cfg.CurrentContext]
	if kubeCtx == nil {
		return "", nil, "", false
	}
	clusterName := kubeCtx.Cluster
	if clusterName == "" {
		clusterName = cfg.CurrentContext
	}
	return cfg.CurrentContext, kubeCtx, clusterName, true
}

// ResolveContext resolves clusterName via FindContextByCluster when non-empty, or falls back to ResolveCurrentContext.
func ResolveContext(cfg *clientcmdapi.Config, clusterName string) (string, *clientcmdapi.Context, string, bool) {
	if trimmed := strings.TrimSpace(clusterName); trimmed != "" {
		name, kubeCtx, ok := FindContextByCluster(cfg, trimmed)
		if !ok {
			return "", nil, "", false
		}
		return name, kubeCtx, trimmed, true
	}
	return ResolveCurrentContext(cfg)
}

// ExecPluginBaseName returns the normalized lowercase executable basename (without .exe) of auth's exec credential plugin.
func ExecPluginBaseName(auth *clientcmdapi.AuthInfo) string {
	if auth == nil || auth.Exec == nil {
		return ""
	}
	// Normalize Windows backslashes before path.Base so Windows kubeconfigs parse identically on Unix/WSL.
	cmdPath := strings.ReplaceAll(strings.TrimSpace(auth.Exec.Command), `\`, "/")
	if cmdPath == "" {
		return ""
	}
	base := strings.ToLower(path.Base(cmdPath))
	if base == "." || base == "/" {
		return ""
	}
	return strings.TrimSuffix(base, ".exe")
}

// WithContext prepends --context <kubeContext> to args when kubeContext is non-empty.
func WithContext(kubeContext string, args []string) []string {
	if kubeContext == "" {
		return args
	}
	return append([]string{"--context", kubeContext}, args...)
}
