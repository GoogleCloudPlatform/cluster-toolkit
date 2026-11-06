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
	"strings"
	"time"

	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// OrchestratorEnvVar forces Layer-1 orchestrator selection ("gke" or "k8s") when set.
const OrchestratorEnvVar = "GCLUSTER_ORCHESTRATOR"

type defaultCommandRunner struct{}

func (defaultCommandRunner) ExecuteCommandWithTimeout(timeout time.Duration, name string, args ...string) shell.CommandResult {
	return shell.ExecuteCommandWithTimeout(timeout, name, args...)
}

// Detector resolves the target Layer-1 orchestrator without mutating kubeconfig or invoking cloud CLIs.
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

	if rawOverride := strings.ToLower(strings.TrimSpace(getenv(OrchestratorEnvVar))); rawOverride != "" {
		return d.detectFromOverride(Type(rawOverride), clusterName, location, projectID, providers)
	}

	cfg, _ := d.loadKubeconfig()
	input := ProbeInput{
		ClusterName: clusterName,
		Location:    location,
		ProjectID:   projectID,
		Config:      cfg,
		AirGapped:   shell.IsAirGappedEnv(d.Getenv),
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

	ctxName, _, resolvedCluster, ok := k8s.ResolveContext(cfg, clusterName)
	if !ok {
		return Detection{
			Type:        TypeK8s,
			KubeContext: "",
			ClusterName: clusterName,
			Reason:      "no matching kubeconfig context found",
		}, nil
	}
	return Detection{
		Type:        TypeK8s,
		KubeContext: ctxName,
		ClusterName: resolvedCluster,
		Reason:      fmt.Sprintf("kubeconfig context %q selected for Kubernetes mode", ctxName),
	}, nil
}

func (d *Detector) detectFromOverride(target Type, clusterName, location, projectID string, providers []ProviderSpec) (Detection, error) {
	if target != TypeGKE && target != TypeK8s {
		return Detection{}, fmt.Errorf("invalid %s=%q; supported values are %q and %q", OrchestratorEnvVar, target, TypeGKE, TypeK8s)
	}
	cfg, _ := d.loadKubeconfig()
	var ctxName, resolvedCluster string
	resolved := false
	for _, spec := range providers {
		if spec.Type == target && spec.ResolveContext != nil {
			ctxName, _, resolvedCluster, resolved = spec.ResolveContext(cfg, clusterName, location, projectID)
			break
		}
	}
	if !resolved {
		ctxName, _, resolvedCluster, resolved = k8s.ResolveContext(cfg, clusterName)
		if !resolved {
			resolvedCluster = clusterName
		}
	}
	return Detection{
		Type:        target,
		KubeContext: ctxName,
		ClusterName: resolvedCluster,
		Reason:      fmt.Sprintf("forced by %s=%s", OrchestratorEnvVar, target),
	}, nil
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
	return defaultCommandRunner{}
}
