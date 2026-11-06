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
	"sync"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
)

var registerProvidersOnce sync.Once

func init() {
	RegisterProviders()
}

// RegisterProviders registers the GKE and generic Kubernetes orchestrator providers once.
func RegisterProviders() {
	registerProvidersOnce.Do(func() {
		newWithDetection := func(det orchestrator.Detection) orchestrator.JobOrchestrator {
			g := NewGKEOrchestrator()
			g.mode = det.Type
			if det.Type == orchestrator.TypeK8s {
				g.kubeContext = det.KubeContext
				g.k8sCluster = strings.TrimSpace(det.ClusterName)
			}
			return g
		}
		orchestrator.RegisterProvider(orchestrator.ProviderSpec{
			Type:           orchestrator.TypeGKE,
			Prerequisites:  []orchestrator.PrereqKind{orchestrator.PrereqGKE},
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
	})
}

func (g *GKEOrchestrator) configureK8sKubectl(clusterName string) error {
	trimmed := strings.TrimSpace(clusterName)
	if strings.TrimSpace(g.kubeContext) != "" && (trimmed == "" || trimmed == g.k8sCluster || trimmed == g.kubeContext) {
		return nil
	}
	cfg, err := g.loadKubeconfig()
	if err != nil {
		g.kubeContext = ""
		g.k8sCluster = ""
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	if name, _, resolvedCluster, ok := k8s.ResolveContext(cfg, trimmed); ok {
		g.kubeContext = name
		if trimmed != "" {
			g.k8sCluster = trimmed
		} else {
			g.k8sCluster = resolvedCluster
		}
		return nil
	}
	g.kubeContext = ""
	g.k8sCluster = ""
	if trimmed != "" {
		return fmt.Errorf("kubeconfig context or cluster %q was not found; check 'kubectl config get-contexts'", trimmed)
	}
	if dangling := k8s.DanglingCurrentContext(cfg); dangling != "" {
		return fmt.Errorf("kubeconfig current-context %q was not found; run 'kubectl config get-contexts' or pass --cluster <context>", dangling)
	}
	return fmt.Errorf("no current-context configured in kubeconfig; run 'kubectl config use-context <context>' or pass --cluster <context>")
}

func (g *GKEOrchestrator) verifyK8sClusterReachable() error {
	switch outcome, stderr := g.probeContext(g.kubeContext); outcome {
	case probeOK, probeForbidden:
		return nil
	case probeAuth:
		return fmt.Errorf("authentication to Kubernetes context %q failed: %s; refresh or update your KUBECONFIG credentials", g.kubeContext, firstLine(stderr))
	case probeConnectivity:
		return fmt.Errorf("kubeconfig context %q is unreachable: %s; verify cluster connectivity and KUBECONFIG server/CA settings", g.kubeContext, firstLine(stderr))
	case probeUnknown:
		logging.Warn("Could not verify Kubernetes context %q (%s). Proceeding with inspection.", g.kubeContext, firstLine(stderr))
		return nil
	default:
		return fmt.Errorf("cannot verify Kubernetes context %q: %s", g.kubeContext, firstLine(stderr))
	}
}

func scaleExpectedStreams(livePods, liveStreams, expectedPods, expectedStreams int) int {
	streams := max(liveStreams, expectedStreams)
	if expectedStreams == 0 && livePods > 0 && expectedPods > livePods {
		perPod := max(1, (liveStreams+livePods-1)/livePods)
		streams = max(streams, expectedPods*perPod)
	}
	return max(expectedPods, streams)
}

func (g *GKEOrchestrator) allPodsLogsCommand(namespace, name string, maxReqs int, follow bool) string {
	selector := fmt.Sprintf("jobset.sigs.k8s.io/jobset-name=%s", name)
	args := []string{"logs", "-n", namespace, "-l", selector, "--all-containers", "--prefix"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, fmt.Sprintf("--max-log-requests=%d", maxReqs), "--tail=-1")
	return kubectlCommandLine(g.kubeContext, args...)
}

func (g *GKEOrchestrator) logMainOnlyDefault(name, namespace string, opts orchestrator.LogsOptions, podCount, maxReqs int) {
	if podCount <= maxLogRequests {
		logging.Info("Job has %d pods (> 5). Defaulting to --main-only logs. To fetch logs from all pods, run with --main-only=false.", podCount)
		return
	}
	if g.mode == orchestrator.TypeK8s {
		cmdLine := g.allPodsLogsCommand(namespace, name, maxReqs, opts.Follow)
		logging.Info("Job has %d pods (> %d). Defaulting to --main-only logs. To view logs from all pods, run '%s'.", podCount, maxLogRequests, cmdLine)
		return
	}
	consoleURL := getCloudConsoleLogsURL(opts.ProjectID, opts.ClusterLocation, opts.ClusterName, namespace, name)
	logging.Info("Job has %d pods (> %d). Defaulting to --main-only logs. To view logs from all pods, use the Google Cloud Console:\n%s", podCount, maxLogRequests, consoleURL)
}

func (g *GKEOrchestrator) maxLogRequestsError(name, namespace string, opts orchestrator.LogsOptions, podCount, maxReqs int) error {
	if g.mode == orchestrator.TypeK8s {
		cmdLine := g.allPodsLogsCommand(namespace, name, maxReqs, opts.Follow)
		verb := "fetch"
		if opts.Follow {
			verb = "stream"
		}
		return fmt.Errorf("job %q has %d pods matching logs query, which exceeds the max fetch limit (%d). Omit --main-only=false to %s only the main pod's logs, or %s all logs directly via '%s'", name, podCount, maxLogRequests, verb, verb, cmdLine)
	}
	consoleURL := getCloudConsoleLogsURL(opts.ProjectID, opts.ClusterLocation, opts.ClusterName, namespace, name)
	return fmt.Errorf("job '%s' has %d pods matching logs query, which exceeds the max fetch limit (%d). Please view logs directly in the Google Cloud Console:\n%s", name, podCount, maxLogRequests, consoleURL)
}
