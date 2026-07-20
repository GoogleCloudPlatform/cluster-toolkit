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
	"strings"
	"text/tabwriter"
	"time"
)

const spacer = "========================================================"

type inspectWriter struct {
	writer   io.Writer
	executor Executor
}

func (w *inspectWriter) runAndLog(description string, command string, args ...string) {
	outputStr := fmt.Sprintf("Description: %s\nCommand: %s %v\n", description, command, args)
	_, _ = fmt.Fprint(w.writer, outputStr)

	res := w.executor.ExecuteCommand(command, args...)
	if res.ExitCode != 0 {
		errStr := fmt.Sprintf("Error (%d):\n%s\n", res.ExitCode, res.Stderr)
		_, _ = fmt.Fprint(w.writer, errStr)
	} else {
		outStr := fmt.Sprintf("Output:\n%s\n", res.Stdout)
		_, _ = fmt.Fprint(w.writer, outStr)
	}
	divider := fmt.Sprintf("\n%s\n\n", spacer)
	_, _ = fmt.Fprint(w.writer, divider)
}

// InspectCluster runs diagnostic checks on the GKE cluster and writes them to a log file.
func (g *GKEOrchestrator) InspectCluster(opts orchestrator.InspectOptions) error {
	// 1. Setup Kubectl (Critical, fail fast)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return fmt.Errorf("failed to configure kubectl: %w", err)
	}

	// 2. Create log file (Critical, fail fast)
	filePath := opts.OutputPath
	if filePath == "" {
		timestamp := time.Now().UTC().Format("20060102-150405")
		fileName := fmt.Sprintf("gcluster-inspect-%s-%s.log", opts.ClusterName, timestamp)
		filePath = filepath.Join(".", fileName)
	}
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create log file %s: %w", filePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	// Resolve namespace context
	targetNamespace, err := g.getCurrentNamespace()
	if err != nil {
		logging.Warn("Failed to resolve current namespace: %v. Defaulting to 'default'", err)
		targetNamespace = "default"
	}

	var outputTarget io.Writer = file
	if opts.Show {
		outputTarget = io.MultiWriter(file, os.Stdout)
	}

	// Initial header in log
	_, _ = fmt.Fprintf(outputTarget, "==================\nGCLUSTER INSPECT OUTPUT:\n==================\n\n")

	writer := &inspectWriter{
		writer:   outputTarget,
		executor: g.executor,
	}

	// --- 1. Local Setup ---
	writer.runAndLog("Local Setup: gcloud version", "gcloud", "version")
	writer.runAndLog("Local Setup: Active gcloud configuration", "gcloud", "config", "list")

	// --- 2. GKE Infrastructure ---
	writer.runAndLog("GKE: Cluster Details", "gcloud", "container", "clusters", "describe", opts.ClusterName, "--location", opts.ClusterLocation, "--project", opts.ProjectID, "--format=yaml")
	writer.runAndLog("GKE: Node Pool Details", "gcloud", "container", "node-pools", "list", "--cluster", opts.ClusterName, "--location", opts.ClusterLocation, "--project", opts.ProjectID)

	// ConfigMaps (graceful handle if not present)
	metadataCM := fmt.Sprintf("%s-metadata", opts.ClusterName)
	resourcesCM := fmt.Sprintf("%s-resources", opts.ClusterName)
	writer.runAndLog("GKE: Cluster Metadata ConfigMap Details", "kubectl", "get", "configmap", metadataCM, "-n", targetNamespace, "-o", "yaml")
	writer.runAndLog("GKE: Cluster Resources ConfigMap Details", "kubectl", "get", "configmap", resourcesCM, "-n", targetNamespace, "-o", "yaml")

	// --- 3. Node Status ---
	writer.runAndLog("Kubectl: All Nodes", "kubectl", "get", "nodes", "-o", "wide")

	// Count nodes per pool (healthy and total)
	logNodeCounts(outputTarget, g.executor)

	// --- 4. Kueue & JobSet Resources ---
	writer.runAndLog("Kueue: ClusterQueue Details", "kubectl", "describe", "ClusterQueue")
	writer.runAndLog("Kueue: LocalQueue Details", "kubectl", "describe", "LocalQueue", "-A")
	writer.runAndLog("Kueue: ResourceFlavor Details", "kubectl", "describe", "ResourceFlavor")
	writer.runAndLog("Kueue: Kueue Deployment Details", "kubectl", "describe", "Deployment", "kueue-controller-manager", "-n", "kueue-system")
	writer.runAndLog("Kueue: Kueue Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/kueue-controller-manager", "-n", "kueue-system", "-c", "manager", "--tail=100")

	writer.runAndLog("JobSet: Deployment Details", "kubectl", "describe", "Deployment", "jobset-controller-manager", "-n", "jobset-system")
	writer.runAndLog("JobSet: JobSet Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/jobset-controller-manager", "-n", "jobset-system", "-c", "manager", "--tail=100")

	// --- 5. Slice Controller (Dynamic Slicing) ---
	cResult := g.executor.ExecuteCommand("kubectl", "get", "crd", "topologies.kueue.x-k8s.io")
	if cResult.ExitCode == 0 {
		writer.runAndLog("Slice Controller: Deployment Details", "kubectl", "describe", "deployment", "slice-controller-controller-manager", "-n", "slice-controller-system")
		writer.runAndLog("Slice Controller: Logs (tail 100)", "kubectl", "logs", "deployment/slice-controller-controller-manager", "-n", "slice-controller-system", "-c", "manager", "--tail=100")
	}

	// --- 6. Workloads ---
	logWorkloadList(outputTarget, g.executor, "EVERYTHING", "", targetNamespace)
	logWorkloadList(outputTarget, g.executor, "QUEUED", "", targetNamespace)
	logWorkloadList(outputTarget, g.executor, "RUNNING", "", targetNamespace)

	workloadNamespace := g.inspectWorkload(writer, opts.WorkloadName)

	// --- 7. Console Links ---
	logConsoleLinks(outputTarget, opts, workloadNamespace)

	logging.Info("Cluster inspection report saved to %s", filePath)
	return nil
}

func (g *GKEOrchestrator) inspectWorkload(writer *inspectWriter, workloadName string) string {
	workloadNamespace := "default"
	if workloadName == "" {
		return workloadNamespace
	}

	ns, err := g.getJobNamespace(workloadName)
	if err == nil {
		workloadNamespace = ns
	} else {
		logging.Warn("Failed to auto-discover namespace for workload %s, defaulting to 'default': %v", workloadName, err)
	}

	logWorkloadList(writer.writer, g.executor, "EVERYTHING", workloadName, workloadNamespace)

	writer.runAndLog(fmt.Sprintf("JobSet: Config for %s", workloadName), "kubectl", "describe", "jobsets", workloadName, "-n", workloadNamespace)

	targetWorkload := fmt.Sprintf("jobset-%s", workloadName)
	if g.kubeClient != nil {
		if tw, err := g.findTargetWorkload(workloadNamespace, workloadName); err == nil {
			targetWorkload = tw
		}
	}
	writer.runAndLog(fmt.Sprintf("Kueue: Workload config for %s", workloadName), "kubectl", "describe", "workloads", targetWorkload, "-n", workloadNamespace)

	return workloadNamespace
}

func logNodeCounts(w io.Writer, exec Executor) {
	desc := "Kubectl: Node count analysis"
	_, _ = fmt.Fprintf(w, "Description: %s\n", desc)

	nodeList, err := fetchNodeList(exec)
	if err != nil {
		errStr := fmt.Sprintf("Error fetching nodes for analysis: %v\n", err)
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

func logConsoleLinks(w io.Writer, opts orchestrator.InspectOptions, workloadNamespace string) {
	desc := "Cloud Console Links"
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
			url:  fmt.Sprintf("https://console.cloud.google.com/kubernetes/workload/details/%s/%s/%s/%s?project=%s", opts.ClusterLocation, opts.ClusterName, workloadNamespace, opts.WorkloadName, opts.ProjectID),
		}
		links = append(links, workloadLink)
	}

	outputStr := "Output:\n"
	for _, l := range links {
		outputStr += fmt.Sprintf("Link Description: %s\nLink: %s\n\n", l.desc, l.url)
	}

	_, _ = fmt.Fprint(w, outputStr)
	_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
}

func logWorkloadList(w io.Writer, exec Executor, filterStatus string, filterWorkload string, namespace string) {
	desc := fmt.Sprintf("Kubectl: List Jobs with filter-by-status=%s", filterStatus)
	if filterWorkload != "" {
		desc += fmt.Sprintf(" with filter-by-job=%s", filterWorkload)
	}

	_, _ = fmt.Fprintf(w, "Description: %s\n", desc)

	args := []string{"get", "workloads"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	} else {
		args = append(args, "-A")
	}
	args = append(args, "-o", "json")
	res := exec.ExecuteCommand("kubectl", args...)
	if res.ExitCode != 0 {
		errStr := fmt.Sprintf("Error listing workloads (%d):\n%s\n", res.ExitCode, res.Stderr)
		_, _ = fmt.Fprint(w, errStr)
		_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
		return
	}

	var wlList kueueWorkloadList
	if err := json.Unmarshal([]byte(res.Stdout), &wlList); err != nil {
		errStr := fmt.Sprintf("Error parsing workloads JSON: %v\n", err)
		_, _ = fmt.Fprint(w, errStr)
		_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
		return
	}

	filtered := filterWorkloads(wlList.Items, filterStatus, filterWorkload)
	renderWorkloadsTable(w, filtered)
}

func filterWorkloads(items []kueueWorkload, filterStatus string, filterWorkload string) []kueueWorkload {
	var filtered []kueueWorkload
	for _, wl := range items {
		// Filter by workload name
		if filterWorkload != "" {
			jobsetName := wl.Metadata.Name
			if len(wl.Metadata.OwnerReferences) > 0 {
				jobsetName = wl.Metadata.OwnerReferences[0].Name
			}
			if !strings.Contains(jobsetName, filterWorkload) {
				continue
			}
		}

		// Filter by status
		if filterStatus == "QUEUED" && !isWorkloadQueued(wl) {
			continue
		}
		if filterStatus == "RUNNING" && !isWorkloadRunning(wl) {
			continue
		}

		filtered = append(filtered, wl)
	}
	return filtered
}

func getWorkloadStats(wl kueueWorkload) (int, string) {
	running := 0
	if wl.Status.Admission != nil && len(wl.Status.Admission.PodSetAssignments) > 0 {
		running = wl.Status.Admission.PodSetAssignments[len(wl.Status.Admission.PodSetAssignments)-1].Count
	}
	status := ""
	if len(wl.Status.Conditions) > 0 {
		status = wl.Status.Conditions[len(wl.Status.Conditions)-1].Type
	}
	return running, status
}

func isWorkloadQueued(wl kueueWorkload) bool {
	running, status := getWorkloadStats(wl)
	statusMatch := strings.Contains(status, "Admitted") || strings.Contains(status, "Evicted") || strings.Contains(status, "QuotaReserved")
	return statusMatch && running == 0
}

func isWorkloadRunning(wl kueueWorkload) bool {
	running, status := getWorkloadStats(wl)
	statusMatch := strings.Contains(status, "Admitted") || strings.Contains(status, "Evicted")
	return statusMatch && running > 0
}

func renderWorkloadsTable(w io.Writer, filtered []kueueWorkload) {
	var sb strings.Builder
	sb.WriteString("Output:\n")

	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Jobset Name\tCreated Time\tPriority\tTPU VMs Needed\tTPU VMs Running/Ran\tTPU VMs Done\tStatus\tStatus Message\tStatus Time")

	for _, wl := range filtered {
		jobsetName := wl.Metadata.Name
		if len(wl.Metadata.OwnerReferences) > 0 {
			jobsetName = wl.Metadata.OwnerReferences[0].Name
		}

		createdTime := wl.Metadata.CreationTimestamp
		priority := wl.Spec.PriorityClassName
		if priority == "" {
			priority = "<none>"
		}

		needed := 0
		if len(wl.Spec.PodSets) > 0 {
			needed = wl.Spec.PodSets[0].Count
		}

		running := 0
		if wl.Status.Admission != nil && len(wl.Status.Admission.PodSetAssignments) > 0 {
			running = wl.Status.Admission.PodSetAssignments[len(wl.Status.Admission.PodSetAssignments)-1].Count
		}

		done := 0
		if len(wl.Status.ReclaimablePods) > 0 {
			done = wl.Status.ReclaimablePods[0].Count
		}

		status := "<none>"
		msg := "<none>"
		statusTime := "<none>"
		if len(wl.Status.Conditions) > 0 {
			cond := wl.Status.Conditions[len(wl.Status.Conditions)-1]
			status = cond.Type
			msg = cond.Message
			statusTime = cond.LastTransitionTime
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%s\t%s\n",
			jobsetName, createdTime, priority, needed, running, done, status, msg, statusTime)
	}
	_ = tw.Flush()

	outStr := sb.String()
	_, _ = fmt.Fprint(w, outStr)
	_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
}
