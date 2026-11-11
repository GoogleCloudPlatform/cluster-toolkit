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

package gke

import (
	"fmt"
	"strings"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
)

func init() {
	newWithDetection := func(det orchestrator.Detection) orchestrator.JobOrchestrator {
		g := NewGKEOrchestrator()
		g.mode = det.Type
		if det.Type == orchestrator.TypeK8s {
			g.kubeContext = det.KubeContext
		}
		return g
	}
	orchestrator.RegisterProvider(orchestrator.ProviderSpec{
		Type: orchestrator.TypeGKE,
		Prerequisites: []orchestrator.PrereqKind{
			orchestrator.PrereqGCloudSDK,
			orchestrator.PrereqGCloudAuth,
			orchestrator.PrereqADC,
			orchestrator.PrereqKubectl,
			orchestrator.PrereqGKEAuthPlugin,
			orchestrator.PrereqGCPProject,
		},
		ResolveContext: resolveGKEContext,
		Probe:          detectGKE,
		Factory:        newWithDetection,
	})
	// Temporary bridge until pure-Kubernetes domain files (cancel.go, kueue.go, infra_manager.go,
	// manifest_generator.go, scheduling.go) migrate to pkg/orchestrator/k8s in the follow-up PR.
	orchestrator.RegisterProvider(orchestrator.ProviderSpec{
		Type:          orchestrator.TypeK8s,
		Prerequisites: []orchestrator.PrereqKind{orchestrator.PrereqKubectl},
		Factory:       newWithDetection,
	})
}

func (g *GKEOrchestrator) configureK8sKubectl(clusterName string) error {
	if g.kubeContext != "" {
		return nil
	}
	cfg, err := g.loadKubeconfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	if name, _, _, ok := k8s.ResolveContext(cfg, clusterName); ok {
		g.kubeContext = name
		return nil
	}
	if strings.TrimSpace(clusterName) != "" {
		return fmt.Errorf("kubeconfig context or cluster %q was not found; check 'kubectl config get-contexts'", clusterName)
	}
	return fmt.Errorf("no current-context configured in kubeconfig; run 'kubectl config use-context <context>' or pass --cluster <context>")
}

func (g *GKEOrchestrator) logsConsoleURL(projectID, location, clusterName, namespace, name string) string {
	if g.mode == orchestrator.TypeK8s {
		return ""
	}
	return getCloudConsoleLogsURL(projectID, location, clusterName, namespace, name)
}

func (g *GKEOrchestrator) maxLogRequestsError(name, namespace, selector string, podCount int, consoleURL string) error {
	if consoleURL == "" {
		cmdLine := kubectlCommandLine(g.kubeContext, "logs", "-n", namespace, "-l", selector, fmt.Sprintf("--max-log-requests=%d", podCount))
		return fmt.Errorf("job '%s' has %d pods matching logs query, which exceeds the max fetch limit (%d). Omit --main-only=false to fetch only the main pod's logs, or stream all logs directly via '%s'", name, podCount, maxLogRequests, cmdLine)
	}
	return fmt.Errorf("job '%s' has %d pods matching logs query, which exceeds the max fetch limit (%d). Please view logs directly in the Google Cloud Console:\n%s", name, podCount, maxLogRequests, consoleURL)
}

func (g *GKEOrchestrator) k8sJobCLIFlags(explicitNS, clusterName, location, projectID string) string {
	ns := strings.TrimSpace(explicitNS)
	if ns == "" {
		ns, _ = g.getCurrentNamespace(clusterName, location, projectID)
	}
	return k8sCLIFlags(clusterName, ns)
}

func k8sCLIFlags(clusterName, namespace string) string {
	var b strings.Builder
	if c := strings.TrimSpace(clusterName); c != "" {
		b.WriteString(" --cluster ")
		b.WriteString(c)
	}
	if ns := strings.TrimSpace(namespace); ns != "" && ns != "default" {
		b.WriteString(" --gke-namespace ")
		b.WriteString(ns)
	}
	return b.String()
}
