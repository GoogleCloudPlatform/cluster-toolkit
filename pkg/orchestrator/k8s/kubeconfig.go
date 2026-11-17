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

// FindContextByCluster resolves a kubeconfig context for name by checking, in order:
// an exact context-name match, current-context if its cluster matches name, and the
// first matching context.cluster entry in sorted context-name order.
func FindContextByCluster(cfg *clientcmdapi.Config, name string) (string, *clientcmdapi.Context, bool) {
	name = strings.TrimSpace(name)
	if cfg == nil || name == "" {
		return "", nil, false
	}
	if kubeCtx := cfg.Contexts[name]; kubeCtx != nil {
		return name, kubeCtx, true
	}
	if curName, curCtx, _, ok := ResolveCurrentContext(cfg); ok && strings.TrimSpace(curCtx.Cluster) == name {
		return curName, curCtx, true
	}
	for _, ctxName := range slices.Sorted(maps.Keys(cfg.Contexts)) {
		if strings.TrimSpace(ctxName) == "" {
			continue
		}
		kubeCtx := cfg.Contexts[ctxName]
		if kubeCtx != nil && strings.TrimSpace(kubeCtx.Cluster) == name {
			return ctxName, kubeCtx, true
		}
	}
	return "", nil, false
}

// ResolveCurrentContext returns the active context, its entry, and its target cluster name.
func ResolveCurrentContext(cfg *clientcmdapi.Config) (string, *clientcmdapi.Context, string, bool) {
	if cfg == nil || strings.TrimSpace(cfg.CurrentContext) == "" {
		return "", nil, "", false
	}
	kubeCtx := cfg.Contexts[cfg.CurrentContext]
	if kubeCtx == nil {
		return "", nil, "", false
	}
	clusterName := strings.TrimSpace(kubeCtx.Cluster)
	if clusterName == "" {
		clusterName = strings.TrimSpace(cfg.CurrentContext)
	}
	return cfg.CurrentContext, kubeCtx, clusterName, true
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
	if base == "." || base == ".." || base == "/" {
		return ""
	}
	return strings.TrimSuffix(base, ".exe")
}

// ContextNamespace returns kubeCtx's trimmed namespace, or "default" when unset or blank.
func ContextNamespace(kubeCtx *clientcmdapi.Context) string {
	if kubeCtx != nil {
		if ns := strings.TrimSpace(kubeCtx.Namespace); ns != "" {
			return ns
		}
	}
	return "default"
}

// WithContext prepends --context <kubeContext> to args when kubeContext is non-blank,
// preserving kubeContext verbatim so kubectl and client-go resolve the same map key.
func WithContext(kubeContext string, args []string) []string {
	if strings.TrimSpace(kubeContext) == "" {
		return args
	}
	return append([]string{"--context", kubeContext}, args...)
}
