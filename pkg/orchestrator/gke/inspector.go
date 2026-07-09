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
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Structs for parsing kubectl get nodes -o json
type kubernetesNodeList struct {
	Items []kubernetesNode `json:"items"`
}

type kubernetesNode struct {
	Metadata kubernetesNodeMetadata `json:"metadata"`
	Status   kubernetesNodeStatus   `json:"status"`
}

type kubernetesNodeMetadata struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

type kubernetesNodeStatus struct {
	Conditions []kubernetesNodeCondition `json:"conditions"`
}

type kubernetesNodeCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

const spacer = "========================================================"

// InspectCluster runs diagnostic checks on the GKE cluster and writes them to a log file.
func (g *GKEOrchestrator) InspectCluster(opts orchestrator.InspectOptions) error {
	// 1. Setup Kubectl (Critical, fail fast)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return fmt.Errorf("failed to configure kubectl: %w", err)
	}

	// 2. Create log file (Critical, fail fast)
	timestamp := time.Now().Format("20060102-150405")
	fileName := fmt.Sprintf("gcluster-inspect-%s-%s.log", opts.ClusterName, timestamp)
	filePath := filepath.Join(".", fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create log file %s: %w", filePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	// Initial header in log
	_, _ = fmt.Fprintf(file, "==================\nGCLUSTER INSPECT OUTPUT:\n==================\n\n")

	// Helper for running commands and logging them
	runAndLog := func(description string, command string, args ...string) {
		outputStr := fmt.Sprintf("Description: %s\nCommand: %s %v\n", description, command, args)
		if opts.Show {
			fmt.Print(outputStr)
		}
		_, _ = fmt.Fprint(file, outputStr)

		res := g.executor.ExecuteCommand(command, args...)
		if res.ExitCode != 0 {
			errStr := fmt.Sprintf("Error (%d):\n%s\n", res.ExitCode, res.Stderr)
			if opts.Show {
				fmt.Print(errStr)
			}
			_, _ = fmt.Fprint(file, errStr)
		} else {
			outStr := fmt.Sprintf("Output:\n%s\n", res.Stdout)
			if opts.Show {
				fmt.Print(outStr)
			}
			_, _ = fmt.Fprint(file, outStr)
		}
		divider := fmt.Sprintf("\n%s\n\n", spacer)
		if opts.Show {
			fmt.Print(divider)
		}
		_, _ = fmt.Fprint(file, divider)
	}

	// --- 1. Local Setup ---
	runAndLog("Local Setup: gcloud version", "gcloud", "version")
	runAndLog("Local Setup: Active gcloud configuration", "gcloud", "config", "list")

	// --- 2. GKE Infrastructure ---
	runAndLog("GKE: Cluster Details", "gcloud", "container", "clusters", "describe", opts.ClusterName, "--location", opts.ClusterLocation, "--project", opts.ProjectID, "--format=yaml")
	runAndLog("GKE: Node Pool Details", "gcloud", "container", "node-pools", "list", "--cluster", opts.ClusterName, "--location", opts.ClusterLocation, "--project", opts.ProjectID)

	// ConfigMaps (graceful handle if not present)
	metadataCM := fmt.Sprintf("%s-metadata", opts.ClusterName)
	resourcesCM := fmt.Sprintf("%s-resources", opts.ClusterName)
	runAndLog("GKE: Cluster Metadata ConfigMap Details", "kubectl", "get", "configmap", metadataCM, "-n", "default", "-o", "yaml")
	runAndLog("GKE: Cluster Resources ConfigMap Details", "kubectl", "get", "configmap", resourcesCM, "-n", "default", "-o", "yaml")

	// --- 3. Node Status ---
	runAndLog("Kubectl: All Nodes", "kubectl", "get", "nodes", "-o", "wide")

	// Count nodes per pool (healthy and total)
	logNodeCounts(file, g.executor, opts.Show)

	// --- 4. Kueue & JobSet Resources ---
	runAndLog("Kueue: ClusterQueue Details", "kubectl", "describe", "ClusterQueue")
	runAndLog("Kueue: LocalQueue Details", "kubectl", "describe", "LocalQueue", "-A")
	runAndLog("Kueue: ResourceFlavor Details", "kubectl", "describe", "ResourceFlavor")
	runAndLog("Kueue: Kueue Deployment Details", "kubectl", "describe", "Deployment", "kueue-controller-manager", "-n", "kueue-system")
	runAndLog("Kueue: Kueue Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/kueue-controller-manager", "-n", "kueue-system", "-c", "manager", "--tail=100")

	runAndLog("JobSet: Deployment Details", "kubectl", "describe", "Deployment", "jobset-controller-manager", "-n", "jobset-system")
	runAndLog("JobSet: JobSet Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/jobset-controller-manager", "-n", "jobset-system", "-c", "manager", "--tail=100")

	// --- 5. Slice Controller (Dynamic Slicing) ---
	cResult := g.executor.ExecuteCommand("kubectl", "get", "crd", "topologies.kueue.x-k8s.io")
	if cResult.ExitCode == 0 {
		runAndLog("Slice Controller: Deployment Details", "kubectl", "describe", "deployment", "slice-controller-controller-manager", "-n", "slice-controller-system")
		runAndLog("Slice Controller: Logs (tail 100)", "kubectl", "logs", "deployment/slice-controller-controller-manager", "-n", "slice-controller-system", "-c", "manager", "--tail=100")
	}

	// --- 6. Workloads ---
	runAndLog("Kubectl: All Workloads", "kubectl", "get", "workloads", "-A")

	if opts.WorkloadName != "" {
		workloadNamespace := "default"
		if g.kubeClient != nil {
			ns, err := g.kubeClient.GetJobNamespace(opts.WorkloadName)
			if err == nil {
				workloadNamespace = ns
			} else {
				logging.Warn("Failed to auto-discover namespace for workload %s, defaulting to 'default': %v", opts.WorkloadName, err)
			}
		}
		runAndLog(fmt.Sprintf("JobSet: Config for %s", opts.WorkloadName), "kubectl", "describe", "jobsets", opts.WorkloadName, "-n", workloadNamespace)
		runAndLog(fmt.Sprintf("Kueue: Workload config for %s", opts.WorkloadName), "kubectl", "describe", "workloads", fmt.Sprintf("jobset-%s", opts.WorkloadName), "-n", workloadNamespace)
	}

	// --- 7. Console Links ---
	logConsoleLinks(file, opts)

	logging.Info("Cluster inspection report saved to %s", fileName)
	return nil
}

func logNodeCounts(w io.Writer, exec Executor, show bool) {
	desc := "Kubectl: Node count analysis"
	if show {
		fmt.Printf("Description: %s\n", desc)
	}
	_, _ = fmt.Fprintf(w, "Description: %s\n", desc)

	nodeList, err := fetchNodeList(exec)
	if err != nil {
		errStr := fmt.Sprintf("Error fetching nodes for analysis: %v\n", err)
		if show {
			fmt.Print(errStr)
		}
		_, _ = fmt.Fprint(w, errStr)
		_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
		return
	}

	totalNodesPerPool, healthyNodesPerPool := countNodes(nodeList)

	// Sort keys for deterministic output
	var pools []string
	for k := range totalNodesPerPool {
		pools = append(pools, k)
	}
	sort.Strings(pools)

	outputStr := "Output:\n"
	outputStr += "Node Pool Node Counts:\n"
	for _, pool := range pools {
		outputStr += fmt.Sprintf("  - %s: %d total\n", pool, totalNodesPerPool[pool])
	}
	outputStr += "Healthy Node Counts Per Node Pool:\n"
	for _, pool := range pools {
		outputStr += fmt.Sprintf("  - %s: %d healthy\n", pool, healthyNodesPerPool[pool])
	}

	if show {
		fmt.Print(outputStr)
	}
	_, _ = fmt.Fprint(w, outputStr)
	_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
}

func fetchNodeList(exec Executor) (*kubernetesNodeList, error) {
	res := exec.ExecuteCommand("kubectl", "get", "nodes", "-o", "json")
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("kubectl get nodes failed (%d): %s", res.ExitCode, res.Stderr)
	}

	var nodeList kubernetesNodeList
	if err := json.Unmarshal([]byte(res.Stdout), &nodeList); err != nil {
		return nil, fmt.Errorf("failed to parse node JSON: %w", err)
	}
	return &nodeList, nil
}

func countNodes(nodeList *kubernetesNodeList) (map[string]int, map[string]int) {
	totalNodesPerPool := make(map[string]int)
	healthyNodesPerPool := make(map[string]int)

	for _, node := range nodeList.Items {
		nodepool := node.Metadata.Labels["cloud.google.com/gke-nodepool"]
		if nodepool == "" {
			nodepool = "unknown"
		}
		totalNodesPerPool[nodepool]++

		isReady := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				isReady = true
				break
			}
		}
		if isReady {
			healthyNodesPerPool[nodepool]++
		}
	}
	return totalNodesPerPool, healthyNodesPerPool
}

func logConsoleLinks(w io.Writer, opts orchestrator.InspectOptions) {
	desc := "Cloud Console Links"
	if opts.Show {
		fmt.Printf("Description: %s\n", desc)
	}
	_, _ = fmt.Fprintf(w, "Description: %s\n", desc)

	links := []struct {
		desc string
		url  string
	}{
		{
			desc: "Cloud Console for GKE Cluster",
			url:  fmt.Sprintf("https://console.cloud.google.com/kubernetes/clusters/details/%s/%s/details?project=%s", opts.ClusterLocation, opts.ClusterName, opts.ProjectID),
		},
		{
			desc: "Cloud Console for GKE Workloads Overview",
			url:  fmt.Sprintf("https://console.cloud.google.com/kubernetes/workload/overview?project=%s&pageState=((gke%%2F%s%%2F%s))", opts.ProjectID, opts.ClusterLocation, opts.ClusterName),
		},
		{
			desc: "Cloud Console for IAM Permissions",
			url:  fmt.Sprintf("https://console.cloud.google.com/iam-admin/iam?project=%s", opts.ProjectID),
		},
		{
			desc: "Cloud Console for Quotas",
			url:  fmt.Sprintf("https://console.cloud.google.com/iam-admin/quotas?project=%s", opts.ProjectID),
		},
	}

	if opts.WorkloadName != "" {
		workloadLink := struct {
			desc string
			url  string
		}{
			desc: fmt.Sprintf("Cloud Console for workload %s", opts.WorkloadName),
			url:  fmt.Sprintf("https://console.cloud.google.com/kubernetes/service/%s/%s/default/%s/details?project=%s", opts.ClusterLocation, opts.ClusterName, opts.WorkloadName, opts.ProjectID),
		}
		links = append(links, workloadLink)
	}

	outputStr := "Output:\n"
	for _, l := range links {
		outputStr += fmt.Sprintf("Link Description: %s\nLink: %s\n\n", l.desc, l.url)
	}

	if opts.Show {
		fmt.Print(outputStr)
	}
	_, _ = fmt.Fprint(w, outputStr)
	_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
}
