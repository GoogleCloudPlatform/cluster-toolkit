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

package orchestrator

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// EnvVar forces job orchestrator selection (for example, "gke" or "k8s") when set.
const EnvVar = "GCLUSTER_ORCHESTRATOR"

// Detector resolves the target job orchestrator without mutating kubeconfig or invoking cloud CLIs.
type Detector struct {
	Getenv           func(string) string
	KubeconfigLoader k8s.KubeconfigLoader
	Runner           CommandRunner
	Providers        []ProviderSpec
}

// Detect evaluates GCLUSTER_ORCHESTRATOR, registered provider probes, and generic Kubernetes kubeconfig fallback.
func (d *Detector) Detect(clusterName, location, projectID string) (Detection, error) {
	getenv := d.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	providers := d.Providers
	if providers == nil {
		providers = snapshotProviders()
	}

	if rawOverride := strings.ToLower(strings.TrimSpace(getenv(EnvVar))); rawOverride != "" {
		return d.detectFromOverride(Type(rawOverride), clusterName, location, projectID, providers)
	}

	cfg, loadErr := d.loadKubeconfig()
	probeCfg := cfg
	if loadErr != nil {
		probeCfg = nil
	}
	input := ProbeInput{
		ClusterName: clusterName,
		Location:    location,
		ProjectID:   projectID,
		Config:      probeCfg,
		AirGapped:   shell.IsAirGappedEnv(getenv),
		Getenv:      getenv,
		Runner:      d.runner(),
	}
	for _, spec := range providers {
		if spec.Probe == nil {
			continue
		}
		if det, ok := spec.Probe(input); ok {
			return det, nil
		}
	}
	if loadErr != nil {
		return Detection{}, fmt.Errorf("failed to load kubeconfig: %w", loadErr)
	}

	ctxName, _, resolvedCluster, ok := k8s.ResolveContext(cfg, clusterName)
	if !ok {
		return unresolvedDetection(TypeK8s, false, clusterName, cfg, "no matching kubeconfig context found"), nil
	}
	return Detection{
		Type:        TypeK8s,
		KubeContext: ctxName,
		ClusterName: resolvedCluster,
		Reason:      fmt.Sprintf("kubeconfig context %q", ctxName),
	}, nil
}

func (d *Detector) detectFromOverride(target Type, clusterName, location, projectID string, providers []ProviderSpec) (Detection, error) {
	supported := supportedOverrideTypes(providers)
	if !slices.Contains(supported, target) {
		return Detection{}, fmt.Errorf("invalid %s=%q; supported values: %s", EnvVar, target, formatQuotedTypes(supported))
	}
	cfg, loadErr := d.loadKubeconfig()
	if target == TypeK8s && loadErr != nil {
		return Detection{}, fmt.Errorf("failed to load kubeconfig: %w", loadErr)
	}
	if loadErr != nil {
		cfg = nil
	}
	reason := fmt.Sprintf("forced by %s=%s", EnvVar, target)
	ctxName, resolvedCluster, resolved := resolveOverrideContext(cfg, target, clusterName, location, projectID, providers)
	if !resolved {
		return unresolvedDetection(target, true, clusterName, cfg, reason), nil
	}
	return Detection{
		Type:        target,
		Forced:      true,
		KubeContext: ctxName,
		ClusterName: resolvedCluster,
		Reason:      reason,
	}, nil
}

func resolveOverrideContext(cfg *clientcmdapi.Config, target Type, clusterName, location, projectID string, providers []ProviderSpec) (string, string, bool) {
	for _, spec := range providers {
		if spec.Type == target && spec.ResolveContext != nil {
			ctxName, _, resolvedCluster, ok := spec.ResolveContext(cfg, clusterName, location, projectID)
			return ctxName, resolvedCluster, ok
		}
	}
	ctxName, _, resolvedCluster, ok := k8s.ResolveContext(cfg, clusterName)
	return ctxName, resolvedCluster, ok
}

func unresolvedDetection(target Type, forced bool, clusterName string, cfg *clientcmdapi.Config, reason string) Detection {
	var dangling string
	if strings.TrimSpace(clusterName) == "" {
		dangling = k8s.DanglingCurrentContext(cfg)
	}
	return Detection{
		Type:            target,
		Forced:          forced,
		ClusterName:     clusterName,
		DanglingContext: dangling,
		Reason:          reason,
	}
}

func supportedOverrideTypes(providers []ProviderSpec) []Type {
	types := make([]Type, 0, len(providers)+1)
	types = append(types, TypeK8s)
	for _, spec := range providers {
		if spec.Type != "" {
			types = append(types, spec.Type)
		}
	}
	slices.Sort(types)
	return slices.Compact(types)
}

func formatQuotedTypes(types []Type) string {
	quoted := make([]string, len(types))
	for i, t := range types {
		quoted[i] = fmt.Sprintf("%q", t)
	}
	return strings.Join(quoted, ", ")
}

func (d *Detector) loadKubeconfig() (*clientcmdapi.Config, error) {
	if d.KubeconfigLoader != nil {
		return d.KubeconfigLoader()
	}
	return k8s.DefaultKubeconfigLoader()
}

func (d *Detector) runner() CommandRunner {
	if d.Runner != nil {
		return d.Runner
	}
	return shell.ExecuteCommandWithTimeout
}
