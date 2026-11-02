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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const spacer = "========================================================"

// inspectStdout is where `gcluster job inspect --sps` prints its summary; tests replace it.
var inspectStdout io.Writer = os.Stdout

type inspectWriter struct {
	writer io.Writer
	orc    *GKEOrchestrator
}

// runAndLog pins kubectl to the chosen context before logging so the report shows the command that ran.
func (w *inspectWriter) runAndLog(description string, command string, args ...string) {
	if command == "kubectl" {
		args = withKubeContext(w.orc.kubeContext, args)
	}
	outputStr := fmt.Sprintf("Description: %s\nCommand: %s %v\n", description, command, args)
	_, _ = fmt.Fprint(w.writer, outputStr)

	res := w.orc.executor.ExecuteCommand(command, args...)
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
// With opts.SPS it only prints a summary of the SPS instance opts.WorkloadName to the terminal.
func (g *GKEOrchestrator) InspectCluster(opts orchestrator.InspectOptions) error {
	if opts.SPS && opts.WorkloadName == "" {
		return fmt.Errorf("--sps requires --name")
	}
	g.namespace = opts.GKENamespace
	// 1. Setup Kubectl (Critical, fail fast)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return fmt.Errorf("failed to configure kubectl: %w", err)
	}

	// Resolve namespace context
	targetNamespace, err := g.getCurrentNamespace(opts.ClusterName, opts.ClusterLocation, opts.ProjectID)
	if err != nil {
		return err
	}

	if opts.SPS {
		summary, err := g.inspectSPSInstance(opts.WorkloadName, targetNamespace)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(inspectStdout, formatSPSSummary(summary))
		return nil
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

	var outputTarget io.Writer = file
	if opts.Show {
		outputTarget = io.MultiWriter(file, os.Stdout)
	}

	// Initial header in log
	_, _ = fmt.Fprintf(outputTarget, "==================\nGCLUSTER INSPECT OUTPUT:\n==================\n\n")
	g.logSPSSummaryIfInstance(outputTarget, opts.WorkloadName, targetNamespace)

	writer := &inspectWriter{
		writer: outputTarget,
		orc:    g,
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
	g.logNodeCounts(outputTarget)

	// --- 4. Kueue & JobSet Resources ---
	writer.runAndLog("Kueue: ClusterQueue Details", "kubectl", "describe", "ClusterQueue")
	writer.runAndLog("Kueue: LocalQueue Details", "kubectl", "describe", "LocalQueue", "-A")
	writer.runAndLog("Kueue: ResourceFlavor Details", "kubectl", "describe", "ResourceFlavor")
	writer.runAndLog("Kueue: Kueue Deployment Details", "kubectl", "describe", "Deployment", "kueue-controller-manager", "-n", "kueue-system")
	writer.runAndLog("Kueue: Kueue Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/kueue-controller-manager", "-n", "kueue-system", "-c", "manager", "--tail=100")

	writer.runAndLog("JobSet: Deployment Details", "kubectl", "describe", "Deployment", "jobset-controller-manager", "-n", "jobset-system")
	writer.runAndLog("JobSet: JobSet Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/jobset-controller-manager", "-n", "jobset-system", "-c", "manager", "--tail=100")

	// --- 5. Slice Controller (Dynamic Slicing) ---
	cResult := g.kubectl("get", "crd", "topologies.kueue.x-k8s.io")
	if cResult.ExitCode == 0 {
		writer.runAndLog("Slice Controller: Deployment Details", "kubectl", "describe", "deployment", "slice-controller-controller-manager", "-n", "slice-controller-system")
		writer.runAndLog("Slice Controller: Logs (tail 100)", "kubectl", "logs", "deployment/slice-controller-controller-manager", "-n", "slice-controller-system", "-c", "manager", "--tail=100")
	}

	// --- 6. Workloads ---
	g.logWorkloadList(outputTarget, "EVERYTHING", "", targetNamespace)
	g.logWorkloadList(outputTarget, "QUEUED", "", targetNamespace)
	g.logWorkloadList(outputTarget, "RUNNING", "", targetNamespace)

	g.inspectWorkload(writer, opts.WorkloadName, targetNamespace)

	// --- 7. Console Links ---
	logConsoleLinks(outputTarget, opts, targetNamespace)

	logging.Info("Cluster inspection report saved to %s", filePath)
	return nil
}

func (g *GKEOrchestrator) inspectWorkload(writer *inspectWriter, workloadName, workloadNamespace string) {
	if workloadName == "" {
		return
	}

	g.logWorkloadList(writer.writer, "EVERYTHING", workloadName, workloadNamespace)

	writer.runAndLog(fmt.Sprintf("JobSet: Config for %s", workloadName), "kubectl", "describe", "jobsets", workloadName, "-n", workloadNamespace)

	targetWorkload := fmt.Sprintf("jobset-%s", workloadName)
	if g.needsDynamicClientInit() {
		_, _ = g.getDynamicClient()
	}
	if g.kubeClient != nil {
		if tw, err := g.findTargetWorkload(workloadNamespace, workloadName, 2*time.Second); err == nil {
			targetWorkload = tw
		}
	}
	writer.runAndLog(fmt.Sprintf("Kueue: Workload config for %s", workloadName), "kubectl", "describe", "workloads", targetWorkload, "-n", workloadNamespace)

}

func (g *GKEOrchestrator) logNodeCounts(w io.Writer) {
	desc := "Kubectl: Node count analysis"
	_, _ = fmt.Fprintf(w, "Description: %s\n", desc)

	nodeList, err := g.fetchNodeList()
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

func (g *GKEOrchestrator) fetchNodeList() (*kubernetesNodeList, error) {
	res := g.kubectl("get", "nodes", "-o", "json")
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

func (g *GKEOrchestrator) logWorkloadList(w io.Writer, filterStatus string, filterWorkload string, namespace string) {
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
	res := g.kubectl(args...)
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

// spsInstanceSummary is what `gcluster job inspect --name <instance> --sps` prints.
type spsInstanceSummary struct {
	Name          string
	Namespace     string
	Status        string
	InstanceType  string // pathways-rm --instance_type, e.g. tpuv6e:2x4
	InstanceCount int    // pathways-rm --instance_count
	ReadySlices   string // e.g. "1/1"; empty until pathways-rm logs or JobSet status reports ready slices
	ServerImage   string
	SidecarImage  string // empty when the workers run no colocated Python sidecar
	SidecarName   string // sidecar container name on worker pods
	SidecarPython string // from running sidecar container or image tag
	SidecarJAX    string // from running sidecar container or image tag
}

type spsContainerView struct {
	Name  string   `json:"name"`
	Image string   `json:"image"`
	Args  []string `json:"args"`
}

type spsPodSpecView struct {
	InitContainers []spsContainerView `json:"initContainers"`
	Containers     []spsContainerView `json:"containers"`
}

type spsJobSetView struct {
	Spec struct {
		Suspend        bool `json:"suspend"`
		ReplicatedJobs []struct {
			Name     string `json:"name"`
			Replicas int    `json:"replicas"`
			Template struct {
				Spec struct {
					Template struct {
						Spec spsPodSpecView `json:"spec"`
					} `json:"template"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"replicatedJobs"`
	} `json:"spec"`
	Status struct {
		TerminalState string `json:"terminalState"`
		Conditions    []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
		ReplicatedJobsStatus []struct {
			Name  string `json:"name"`
			Ready int    `json:"ready"`
		} `json:"replicatedJobsStatus"`
	} `json:"status"`
}

var (
	spsReadySlicesRe = regexp.MustCompile(`(\d+)/(\d+) Pathways Slices Now Ready`)
	jaxVersionRe     = regexp.MustCompile(`(?i)jax[-_]?v?(\d+\.\d+(?:\.\d+)?)`)
	pythonVersionRe  = regexp.MustCompile(`(?i)(?:python|py)[-_]?(3\.\d+)`)
)

const spsSidecarVersionScript = "import jax, jaxlib, sys; print(f'{sys.version_info.major}.{sys.version_info.minor},{jax.__version__},{jaxlib.__version__}')"

// inspectSPSInstance reads the JobSet name in namespace ns and summarizes it as an SPS instance.
// It works for instances deployed with --pathways-deploy-sps and with the SPS deploy script.
func (g *GKEOrchestrator) inspectSPSInstance(name, ns string) (spsInstanceSummary, error) {
	res := g.kubectl("get", "jobset", name, "-n", ns, "-o", "json")
	if res.ExitCode != 0 {
		return spsInstanceSummary{}, fmt.Errorf("failed to read JobSet %q in namespace %q: %s", name, ns, strings.TrimSpace(res.Stderr))
	}
	summary, err := parseSPSInstance(name, ns, []byte(res.Stdout))
	if err != nil {
		return spsInstanceSummary{}, err
	}
	logs := g.kubectl("logs", "-n", ns, "-l", "jobset.sigs.k8s.io/jobset-name="+name+",jobset.sigs.k8s.io/replicatedjob-name=pathways-head",
		"-c", "pathways-rm", "--tail=-1")
	if logs.ExitCode == 0 {
		if ready := lastReadySlices(logs.Stdout); ready != "" {
			summary.ReadySlices = ready
		}
	}
	g.probeSPSSidecarVersions(name, ns, &summary)
	return summary, nil
}

// probeSPSSidecarVersions queries a running worker's sidecar container for its installed
// Python, JAX, and jaxlib versions (as pathwaysutils get_sidecar_versions does), falling back
// to the image-tag versions when no worker pod is running or pods/exec is not permitted.
func (g *GKEOrchestrator) probeSPSSidecarVersions(name, ns string, s *spsInstanceSummary) {
	if s.SidecarImage == "" {
		return
	}
	pods := g.kubectl("get", "pods", "-n", ns,
		"-l", "jobset.sigs.k8s.io/jobset-name="+name,
		"--field-selector=status.phase=Running",
		"-o", "jsonpath={.items[*].metadata.name}")
	if pods.ExitCode != 0 {
		return
	}
	workerPod := firstWorkerPod(name, pods.Stdout)
	if workerPod == "" {
		return
	}
	sidecar := s.SidecarName
	if sidecar == "" {
		sidecar = "colocated-python-sidecar"
	}
	res := g.kubectl("exec", "-n", ns, "pod/"+workerPod, "-c", sidecar, "--", "python3", "-c", spsSidecarVersionScript)
	if res.ExitCode == 0 {
		applyProbedVersions(res.Stdout, s)
	}
}

func firstWorkerPod(jobsetName, podsStdout string) string {
	headPrefix := jobsetName + "-pathways-head-"
	for _, pod := range strings.Fields(podsStdout) {
		if !strings.HasPrefix(pod, headPrefix) {
			return pod
		}
	}
	return ""
}

func applyProbedVersions(stdout string, s *spsInstanceSummary) {
	parts := strings.Split(strings.TrimSpace(stdout), ",")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return
	}
	s.SidecarPython = strings.TrimSpace(parts[0])
	jax := strings.TrimSpace(parts[1])
	if jaxlib := strings.TrimSpace(parts[2]); jaxlib != "" {
		jax = fmt.Sprintf("%s (jaxlib %s)", jax, jaxlib)
	}
	s.SidecarJAX = jax
}

// logSPSSummaryIfInstance adds the SPS summary to the top of the full inspect report when name is an SPS instance.
func (g *GKEOrchestrator) logSPSSummaryIfInstance(w io.Writer, name, ns string) {
	if name == "" {
		return
	}
	summary, err := g.inspectSPSInstance(name, ns)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "SPS instance summary:\n%s\n%s\n\n", formatSPSSummary(summary), spacer)
}

func parseSPSInstance(name, ns string, data []byte) (spsInstanceSummary, error) {
	var js spsJobSetView
	if err := json.Unmarshal(data, &js); err != nil {
		return spsInstanceSummary{}, fmt.Errorf("failed to parse JobSet %q: %w", name, err)
	}
	summary := spsInstanceSummary{Name: name, Namespace: ns}
	var head, worker *spsPodSpecView
	var workerReplicas int
	for i := range js.Spec.ReplicatedJobs {
		rj := &js.Spec.ReplicatedJobs[i]
		switch rj.Name {
		case "pathways-head":
			head = &rj.Template.Spec.Template.Spec
		case "worker", "pathways-worker":
			worker = &rj.Template.Spec.Template.Spec
			workerReplicas = rj.Replicas
		}
	}
	if head == nil || worker == nil {
		return spsInstanceSummary{}, fmt.Errorf("JobSet %q is not an SPS instance: it needs a pathways-head job and a worker job", name)
	}
	if err := parseSPSHead(name, head, &summary); err != nil {
		return spsInstanceSummary{}, err
	}
	parseSPSWorker(worker, workerReplicas, &summary)
	summary.Status = spsInstanceStatus(&js)
	summary.ReadySlices = statusReadySlices(&js, summary.InstanceCount)
	return summary, nil
}

func parseSPSWorker(worker *spsPodSpecView, replicas int, summary *spsInstanceSummary) {
	if summary.InstanceCount == 0 && replicas > 0 {
		summary.InstanceCount = replicas
	}
	for _, c := range append(append([]spsContainerView{}, worker.InitContainers...), worker.Containers...) {
		switch c.Name {
		case "pathways-worker":
			if c.Image != "" {
				summary.ServerImage = c.Image
			}
		case "colocated-python-sidecar", "external-sidecar":
			summary.SidecarImage = c.Image
			summary.SidecarName = c.Name
			summary.SidecarPython, summary.SidecarJAX = imageVersions(c.Image)
		}
	}
}

func statusReadySlices(js *spsJobSetView, instanceCount int) string {
	if js.Status.TerminalState != "" || instanceCount <= 0 {
		return ""
	}
	for _, rj := range js.Status.ReplicatedJobsStatus {
		if (rj.Name == "worker" || rj.Name == "pathways-worker") && rj.Ready > 0 {
			return fmt.Sprintf("%d/%d", rj.Ready, instanceCount)
		}
	}
	return ""
}

// parseSPSHead checks that pathways-head runs only the resource manager and reads the slice shape from it.
func parseSPSHead(name string, head *spsPodSpecView, summary *spsInstanceSummary) error {
	var rm *spsContainerView
	for _, c := range append(append([]spsContainerView{}, head.InitContainers...), head.Containers...) {
		switch {
		case c.Name == "pathways-rm":
			c := c
			rm = &c
		case c.Name == "pathways-proxy" || strings.HasPrefix(c.Name, "workload-container") || c.Name == "user-workload":
			return fmt.Errorf("JobSet %q is not an SPS instance: its pathways-head runs %s, so it is a regular or headless Pathways job", name, c.Name)
		}
	}
	if rm == nil {
		return fmt.Errorf("JobSet %q is not an SPS instance: its pathways-head does not run pathways-rm", name)
	}
	summary.ServerImage = rm.Image
	parseRMArgs(rm.Args, summary)
	return nil
}

func parseRMArgs(args []string, summary *spsInstanceSummary) {
	for i := 0; i < len(args); i++ {
		key, value, ok := strings.Cut(args[i], "=")
		if !ok && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		} else if !ok {
			continue
		}
		switch strings.TrimLeft(key, "-") {
		case "instance_type":
			summary.InstanceType = value
		case "instance_count":
			summary.InstanceCount, _ = strconv.Atoi(value)
		}
	}
}

func spsInstanceStatus(js *spsJobSetView) string {
	if js.Status.TerminalState != "" {
		return js.Status.TerminalState
	}
	for _, c := range js.Status.Conditions {
		if (c.Type == "Completed" || c.Type == "Failed") && c.Status == "True" {
			return c.Type
		}
		if c.Type == "Suspended" && c.Status == "True" {
			return "Suspended (waiting for Kueue admission)"
		}
	}
	if js.Spec.Suspend {
		return "Suspended (waiting for Kueue admission)"
	}
	for _, rj := range js.Status.ReplicatedJobsStatus {
		if rj.Name == "pathways-head" && rj.Ready > 0 {
			return "Running"
		}
	}
	return "Starting"
}

// lastReadySlices returns the most recent "N/M" from pathways-rm's "N/M Pathways Slices Now Ready" log line.
func lastReadySlices(logs string) string {
	matches := spsReadySlicesRe.FindAllStringSubmatch(logs, -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	return last[1] + "/" + last[2]
}

// imageVersions reads the Python and JAX versions from an image tag such as
// sidecar:20260831-maxtext-v0.2.4-jax0.11.1-pwutils. Missing versions are returned empty.
func imageVersions(image string) (python, jax string) {
	tag, _, _ := strings.Cut(image, "@")
	i := strings.LastIndex(tag, ":")
	if i < 0 || strings.Contains(tag[i:], "/") {
		return "", ""
	}
	tag = tag[i+1:]
	if m := pythonVersionRe.FindStringSubmatch(tag); m != nil {
		python = m[1]
	}
	if m := jaxVersionRe.FindStringSubmatch(tag); m != nil {
		jax = m[1]
	}
	return python, jax
}

func formatSPSSummary(s spsInstanceSummary) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	row := func(k, v string) { _, _ = fmt.Fprintf(tw, "%s:\t%s\n", k, v) }
	row("Name", fmt.Sprintf("%s (namespace: %s)", s.Name, s.Namespace))
	row("Status", s.Status)
	row("Endpoint", fmt.Sprintf(spsEndpointFormat, s.Name))
	ready := "readiness not logged yet"
	if s.ReadySlices != "" {
		ready = s.ReadySlices + " ready"
	}
	row("Slices", fmt.Sprintf("%d x %s (%s)", s.InstanceCount, s.InstanceType, ready))
	row("Server image", s.ServerImage)
	if s.SidecarImage == "" {
		row("Sidecar image", "none")
	} else {
		python, jax := s.SidecarPython, s.SidecarJAX
		if python == "" && jax == "" {
			python, jax = imageVersions(s.SidecarImage)
		}
		orUnknown := func(v string) string {
			if v == "" {
				return "unknown (not in image tag)"
			}
			return v
		}
		row("Sidecar image", s.SidecarImage)
		row("Sidecar Python", orUnknown(python))
		row("Sidecar JAX", orUnknown(jax))
	}
	_ = tw.Flush()
	return b.String()
}
