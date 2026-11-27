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
	"hpc-toolkit/pkg/shell"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	k8syaml "sigs.k8s.io/yaml"
)

const (
	spacer = "========================================================"

	slicesResource             = "slices.accelerator.gke.io"
	topologiesResource         = "topologies.kueue.x-k8s.io"
	kueueWorkloadsResource     = "workloads.kueue.x-k8s.io"
	kueueClusterQueuesResource = "clusterqueues.kueue.x-k8s.io"
	compositePodGroupsResource = "compositepodgroups.scheduling.k8s.io"
	podGroupsResource          = "podgroups.scheduling.k8s.io"

	sliceControllerNamespace  = "slice-controller-system"
	sliceControllerDeployment = "slice-controller-controller-manager"

	tpuSchedulerSystemNamespace = "tpu-scheduler-system"
	tpuSchedulerLegacyNamespace = "kube-system"
	tpuSchedulerDeployment      = "tpu-scheduler"
	tpuSchedulerContainer       = "kube-scheduler"
	tpuSchedulerPodLabel        = "component=tpu-scheduler"

	tpuWorkloadControllerNamespace  = "tpu-workload-controller-system"
	tpuWorkloadControllerDeployment = "tpu-workload-controller-controller-manager"
	tpuWorkloadControllerContainer  = "manager"

	wasControllerLogTail = "--tail=500"

	tpuAcceleratorLabel         = "cloud.google.com/gke-tpu-accelerator"
	gceTopologyBlockLabel       = "cloud.google.com/gce-topology-block"
	tpuSuperpodIDLabel          = "cloud.google.com/gke-tpu-superpod-id"
	tpuPartitionLabelPrefix     = "cloud.google.com/gke-tpu-partition-"
	tpuSliceLabel               = "cloud.google.com/gke-tpu-slice"
	tpuSliceLabelPrefix         = "cloud.google.com/gke-tpu-slice-"
	lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"
)

var (
	pemPrivateKeyRe          = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	pemPrivateKeyTruncatedRe = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----(?:\r?\n[A-Za-z0-9+/= \t]+)*`)
	authBearerRe             = regexp.MustCompile(`(?i)(\bauthorization\s*[:=]\s*bearer\s+)[^\s"',;\]]+`)
	standaloneBearerRe       = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]{16,}`)
	googleOAuthTokenRe       = regexp.MustCompile(`\bya29\.[A-Za-z0-9._~+/=-]+`)
	googleIAMTokenRe         = regexp.MustCompile(`\biam-[A-Za-z0-9._~+/=-]{8,}`)
	jwtTokenRe               = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.eyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]*`)
	awsAccessKeyRe           = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	basicAuthURLRe           = regexp.MustCompile(`\b(https?://)([^:@/\s]+):([^@/\s]+)@`)
	sensitiveFlagEqRe        = regexp.MustCompile(`(?i)(--[a-z0-9._-]*(?:token|secret|password|passwd|credential|api[-_]?key|access[-_]?key|private[-_]?key|auth[-_]?key)[a-z0-9._-]*=)([^\s"',;\]]+)`)
	sensitiveFlagSpaceRe     = regexp.MustCompile(`(?i)(--[a-z0-9._-]*(?:token|secret|password|passwd|credential|api[-_]?key|access[-_]?key|private[-_]?key|auth[-_]?key)[a-z0-9._-]*[ \t]+)([^\s"',;\]-][^\s"',;\]]*)`)
	sensitiveFlagBareRe      = regexp.MustCompile(`(?i)^--[a-z0-9._-]*(?:token|secret|password|passwd|credential|api[-_]?key|access[-_]?key|private[-_]?key|auth[-_]?key)[a-z0-9._-]*$`)
	sensitiveKeyNameRe       = regexp.MustCompile(`(?i)(?:token|secret|password|passwd|credential|api[-_]?key|private[-_]?key|access[-_]?key|client[-_]?key)`)
)

type inspectWriter struct {
	writer io.Writer
	orc    *GKEOrchestrator
}

func hasYAMLOutputArg(args []string) bool {
	for i := 0; i < len(args); i++ {
		if args[i] == "-o=yaml" || args[i] == "--output=yaml" {
			return true
		}
		if (args[i] == "-o" || args[i] == "--output") && i+1 < len(args) && args[i+1] == "yaml" {
			return true
		}
	}
	return false
}

func formatCommandOutput(command string, args []string, stdout string) string {
	if command == "kubectl" && hasYAMLOutputArg(args) {
		return sanitizeK8sYAML(stdout)
	}
	return redactSensitiveText(stdout)
}

// runAndLog pins kubectl to the chosen context before logging so the report shows the command that ran.
func (w *inspectWriter) runAndLog(description string, command string, args ...string) shell.CommandResult {
	if command == "kubectl" {
		args = withKubeContext(w.orc.kubeContext, args)
	}
	outputStr := fmt.Sprintf("Description: %s\nCommand: %s %v\n", description, command, args)
	_, _ = fmt.Fprint(w.writer, outputStr)

	res := w.orc.executor.ExecuteCommand(command, args...)
	if res.ExitCode != 0 {
		errStr := fmt.Sprintf("Error (%d):\n%s\n", res.ExitCode, redactSensitiveText(res.Stderr))
		_, _ = fmt.Fprint(w.writer, errStr)
	} else {
		outStr := fmt.Sprintf("Output:\n%s\n", formatCommandOutput(command, args, res.Stdout))
		_, _ = fmt.Fprint(w.writer, outStr)
	}
	divider := fmt.Sprintf("\n%s\n\n", spacer)
	_, _ = fmt.Fprint(w.writer, divider)
	return res
}

// runAndLogOptional runs the command and logs the section only when it succeeds with non-empty output.
// Optional WAS/Slice components and --previous container logs use this so absent resources are skipped cleanly.
func (w *inspectWriter) runAndLogOptional(description string, command string, args ...string) (shell.CommandResult, bool) {
	if command == "kubectl" {
		args = withKubeContext(w.orc.kubeContext, args)
	}
	res := w.orc.executor.ExecuteCommand(command, args...)
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return res, false
	}
	_, _ = fmt.Fprintf(w.writer, "Description: %s\nCommand: %s %v\nOutput:\n%s\n\n%s\n\n",
		description, command, args, formatCommandOutput(command, args, res.Stdout), spacer)
	return res, true
}

func redactSensitiveText(text string) string {
	if text == "" {
		return text
	}
	s := pemPrivateKeyRe.ReplaceAllString(text, "[REDACTED_PRIVATE_KEY]")
	s = pemPrivateKeyTruncatedRe.ReplaceAllString(s, "[REDACTED_PRIVATE_KEY]")
	s = redactDescribeBlocks(s)
	s = authBearerRe.ReplaceAllString(s, "${1}[REDACTED]")
	s = standaloneBearerRe.ReplaceAllString(s, "${1}[REDACTED]")
	s = googleOAuthTokenRe.ReplaceAllString(s, "[REDACTED]")
	s = googleIAMTokenRe.ReplaceAllString(s, "[REDACTED]")
	s = jwtTokenRe.ReplaceAllString(s, "[REDACTED]")
	s = awsAccessKeyRe.ReplaceAllString(s, "[REDACTED]")
	s = basicAuthURLRe.ReplaceAllString(s, "${1}${2}:[REDACTED]@")
	s = sensitiveFlagEqRe.ReplaceAllString(s, "${1}[REDACTED]")
	s = sensitiveFlagSpaceRe.ReplaceAllString(s, "${1}[REDACTED]")
	return s
}

// redactDescribeBlocks scrubs literal env values in `kubectl describe` Environment: blocks
// while preserving env names, Secret/ConfigMap references, and downward API references,
// and redacts values following standalone sensitive CLI flags in multi-line Args: blocks.
func redactDescribeBlocks(text string) string {
	lines := strings.Split(text, "\n")
	inEnvBlock := false
	envHeaderIndent := 0
	envEntryIndent := -1
	redactingContinuation := false
	redactNextArgIndent := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		if redactNextArgIndent >= 0 {
			expectedIndent := redactNextArgIndent
			redactNextArgIndent = -1
			if trimmed != "" && indent == expectedIndent && !strings.HasPrefix(trimmed, "-") && !strings.HasSuffix(trimmed, ":") {
				lines[i] = line[:indent] + "[REDACTED]"
				continue
			}
		}

		if inEnvBlock {
			if trimmed == "" || indent <= envHeaderIndent {
				inEnvBlock = false
				envEntryIndent = -1
				redactingContinuation = false
			} else {
				if envEntryIndent < 0 {
					envEntryIndent = indent
				}
				if indent > envEntryIndent {
					if redactingContinuation {
						lines[i] = line[:indent] + "[REDACTED]"
					}
					continue
				}
				redactingContinuation = false
				if colonIdx := strings.Index(trimmed, ":"); colonIdx > 0 {
					key := trimmed[:colonIdx]
					val := strings.TrimSpace(trimmed[colonIdx+1:])
					if val != "" &&
						!strings.HasPrefix(val, "<set to ") &&
						!(strings.HasPrefix(val, "(") && strings.HasSuffix(val, ")")) {
						lines[i] = line[:indent] + key + ": [REDACTED]"
						redactingContinuation = true
					}
				} else {
					lines[i] = line[:indent] + "[REDACTED]"
				}
				continue
			}
		}

		if trimmed == "Environment:" {
			inEnvBlock = true
			envHeaderIndent = indent
			envEntryIndent = -1
			redactingContinuation = false
			continue
		}

		if sensitiveFlagBareRe.MatchString(trimmed) {
			redactNextArgIndent = indent
		}
	}
	return strings.Join(lines, "\n")
}

func sanitizeK8sYAML(rawYAML string) string {
	if strings.TrimSpace(rawYAML) == "" {
		return rawYAML
	}
	var obj map[string]interface{}
	if err := k8syaml.Unmarshal([]byte(rawYAML), &obj); err != nil || obj == nil {
		return redactSensitiveText(rawYAML)
	}
	_, hasAPIVersion := obj["apiVersion"]
	_, hasKind := obj["kind"]
	_, hasItems := obj["items"]
	_, hasMetadata := obj["metadata"]
	if !hasAPIVersion && !hasKind && !hasItems && !hasMetadata {
		return redactSensitiveText(rawYAML)
	}
	sanitizeK8sValue(obj)
	out, err := k8syaml.Marshal(obj)
	if err != nil {
		return redactSensitiveText(rawYAML)
	}
	return string(out)
}

func sanitizeK8sValue(v interface{}) interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		if metadata, ok := typed["metadata"].(map[string]interface{}); ok {
			delete(metadata, "managedFields")
			if anns, ok := metadata["annotations"].(map[string]interface{}); ok {
				delete(anns, lastAppliedConfigAnnotation)
				if len(anns) == 0 {
					delete(metadata, "annotations")
				}
			}
		}
		if kind, ok := typed["kind"].(string); ok {
			switch {
			case strings.EqualFold(kind, "Secret"):
				for _, field := range []string{"data", "stringData"} {
					if secMap, ok := typed[field].(map[string]interface{}); ok {
						for k := range secMap {
							secMap[k] = "[REDACTED]"
						}
					}
				}
			case strings.EqualFold(kind, "ConfigMap"):
				for _, field := range []string{"data", "binaryData"} {
					if cmMap, ok := typed[field].(map[string]interface{}); ok {
						for k := range cmMap {
							if sensitiveKeyNameRe.MatchString(k) {
								cmMap[k] = "[REDACTED]"
							}
						}
					}
				}
			}
		}
		for k, val := range typed {
			switch k {
			case "env":
				if envList, ok := val.([]interface{}); ok {
					for _, elem := range envList {
						if envMap, ok := elem.(map[string]interface{}); ok {
							if envVal, hasVal := envMap["value"]; hasVal && envVal != nil && fmt.Sprint(envVal) != "" {
								envMap["value"] = "[REDACTED]"
							}
						}
					}
				}
				typed[k] = sanitizeK8sValue(val)
			case "envFrom":
				if envFromList, ok := val.([]interface{}); ok && len(envFromList) > 0 {
					typed[k] = []interface{}{"[REDACTED]"}
				}
			default:
				typed[k] = sanitizeK8sValue(val)
			}
		}
		return typed
	case []interface{}:
		for i, elem := range typed {
			typed[i] = sanitizeK8sValue(elem)
		}
		return typed
	case string:
		return redactSensitiveText(typed)
	default:
		return v
	}
}

// InspectCluster runs diagnostic checks on the GKE cluster and writes them to a log file.
// With opts.Filter == "pathways-sps" it writes only the SPS instance summary for opts.WorkloadName.
func (g *GKEOrchestrator) InspectCluster(opts orchestrator.InspectOptions) error {
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

	if opts.Filter == "pathways-sps" {
		return g.inspectSPSInstanceToLog(opts, targetNamespace)
	}

	// 2. Create log file (Critical, fail fast)
	file, outputTarget, filePath, err := openInspectOutput(opts, ".log")
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

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

	// Count nodes per pool (healthy and total) and render TPU node summary from the same node list.
	nodeList := g.logNodeCounts(outputTarget)
	logTPUNodeSummary(outputTarget, nodeList)

	apiState := g.discoverInspectAPIs()

	// --- 4. Kueue & JobSet Resources ---
	if apiState.hasKueue {
		writer.runAndLog("Kueue: ClusterQueue Details", "kubectl", "describe", "ClusterQueue")
		writer.runAndLog("Kueue: LocalQueue Details", "kubectl", "describe", "LocalQueue", "-A")
		writer.runAndLog("Kueue: ResourceFlavor Details", "kubectl", "describe", "ResourceFlavor")
		writer.runAndLog("Kueue: Kueue Deployment Details", "kubectl", "describe", "Deployment", "kueue-controller-manager", "-n", "kueue-system")
		writer.runAndLog("Kueue: Kueue Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/kueue-controller-manager", "-n", "kueue-system", "-c", "manager", "--tail=100")
	}

	writer.runAndLog("JobSet: Deployment Details", "kubectl", "describe", "Deployment", "jobset-controller-manager", "-n", "jobset-system")
	writer.runAndLog("JobSet: JobSet Controller Manager Logs (tail 100)", "kubectl", "logs", "deployment/jobset-controller-manager", "-n", "jobset-system", "-c", "manager", "--tail=100")

	// --- 5. Slice Controller & WAS (Dynamic Slicing / Workload-Aware Scheduling) ---
	wasState := g.inspectSliceAndWAS(writer, apiState)

	// --- 6. Workloads ---
	if apiState.hasKueue {
		g.logWorkloadList(outputTarget, "EVERYTHING", "", targetNamespace)
		g.logWorkloadList(outputTarget, "QUEUED", "", targetNamespace)
		g.logWorkloadList(outputTarget, "RUNNING", "", targetNamespace)
	}

	g.inspectWorkload(writer, opts.WorkloadName, targetNamespace, wasState)

	// --- 7. Console Links ---
	logConsoleLinks(outputTarget, opts, targetNamespace)

	logging.Info("Cluster inspection report saved to %s", filePath)
	return nil
}

type inspectAPIState struct {
	hasKueue              bool
	hasTopologies         bool
	hasSlices             bool
	hasCompositePodGroups bool
	hasPodGroups          bool
	slicesDiscoveryFailed bool
	schedDiscoveryFailed  bool
}

type inspectWASState struct {
	hasKueue               bool
	hasSlices              bool
	hasTopologies          bool
	hasCompositePodGroups  bool
	hasPodGroups           bool
	slicesYAML             string
	compositePodGroupsYAML string
	podGroupsYAML          string
	workloadPodsJSON       string
}

type wasResourceItem struct {
	raw        map[string]interface{}
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Metadata   struct {
		Name            string                  `json:"name"`
		Namespace       string                  `json:"namespace,omitempty"`
		Labels          map[string]string       `json:"labels,omitempty"`
		Annotations     map[string]string       `json:"annotations,omitempty"`
		OwnerReferences []kueueWorkloadOwnerRef `json:"ownerReferences,omitempty"`
	} `json:"metadata"`
	Spec struct {
		ParentCompositePodGroupName string `json:"parentCompositePodGroupName,omitempty"`
		WorkloadRef                 *struct {
			WorkloadName string `json:"workloadName,omitempty"`
		} `json:"workloadRef,omitempty"`
		Topology     string   `json:"topology,omitempty"`
		Type         string   `json:"type,omitempty"`
		PartitionIDs []string `json:"partitionIds,omitempty"`
	} `json:"spec,omitempty"`
	Status map[string]interface{} `json:"status,omitempty"`
}

type wasResourceList struct {
	Items []wasResourceItem `json:"items"`
}

type wasPodItem struct {
	Metadata struct {
		Name            string                  `json:"name"`
		Namespace       string                  `json:"namespace,omitempty"`
		Annotations     map[string]string       `json:"annotations,omitempty"`
		OwnerReferences []kueueWorkloadOwnerRef `json:"ownerReferences,omitempty"`
	} `json:"metadata"`
	Spec struct {
		SchedulingGroup *struct {
			PodGroupName string `json:"podGroupName,omitempty"`
		} `json:"schedulingGroup,omitempty"`
	} `json:"spec"`
}

type wasPodList struct {
	Items []wasPodItem `json:"items"`
}

func (g *GKEOrchestrator) discoverAPIResources(apiGroup string) (map[string]bool, bool) {
	res := g.kubectl("api-resources", "--api-group="+apiGroup, "-o", "name")
	if res.ExitCode != 0 {
		return nil, false
	}
	found := make(map[string]bool)
	for _, line := range strings.Split(res.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		found[name] = true
		if !strings.Contains(name, ".") && apiGroup != "" {
			found[name+"."+apiGroup] = true
		}
	}
	return found, true
}

func (g *GKEOrchestrator) crdExists(crdNames ...string) bool {
	for _, name := range crdNames {
		if g.kubectl("get", "crd", name).ExitCode == 0 {
			return true
		}
	}
	return false
}

func (g *GKEOrchestrator) discoverInspectAPIs() inspectAPIState {
	var state inspectAPIState

	kueueRes, kueueOK := g.discoverAPIResources("kueue.x-k8s.io")
	if kueueOK {
		state.hasTopologies = kueueRes[topologiesResource]
		state.hasKueue = kueueRes[kueueWorkloadsResource] || kueueRes[kueueClusterQueuesResource]
	} else {
		state.hasTopologies = g.crdExists(topologiesResource)
		state.hasKueue = true
	}

	accelRes, accelOK := g.discoverAPIResources("accelerator.gke.io")
	if accelOK {
		state.hasSlices = accelRes[slicesResource]
	} else if g.crdExists(slicesResource) {
		state.hasSlices = true
	} else {
		state.slicesDiscoveryFailed = true
	}

	schedRes, schedOK := g.discoverAPIResources("scheduling.k8s.io")
	if schedOK {
		state.hasCompositePodGroups = schedRes[compositePodGroupsResource]
		state.hasPodGroups = schedRes[podGroupsResource]
	} else {
		state.schedDiscoveryFailed = true
	}

	return state
}

func logTPUNodeSummary(w io.Writer, nodeList *kubernetesNodeList) {
	if nodeList == nil {
		return
	}
	var tpuNodes []kubernetesNode
	for _, node := range nodeList.Items {
		if strings.TrimSpace(node.Metadata.Labels[tpuAcceleratorLabel]) != "" {
			tpuNodes = append(tpuNodes, node)
		}
	}
	if len(tpuNodes) == 0 {
		return
	}
	sort.Slice(tpuNodes, func(i, j int) bool {
		return tpuNodes[i].Metadata.Name < tpuNodes[j].Metadata.Name
	})

	var sb strings.Builder
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tREADY\tTAINTS\tACCELERATOR\tTOPOLOGY\tBLOCK\tPARTITIONS\tSLICES")
	for _, node := range tpuNodes {
		labels := node.Metadata.Labels
		ready := "Unknown"
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" {
				ready = cond.Status
				break
			}
		}
		accel := labelOrNone(labels[tpuAcceleratorLabel])
		topo := labelOrNone(labels[tpuTopologyLabel])
		block := formatTPUNodeBlock(labels)
		taints := formatNodeTaints(node.Spec.Taints)
		partitions := formatTPUPrefixLabels(labels, tpuPartitionLabelPrefix)
		slices := formatTPUSliceLabels(labels)

		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			node.Metadata.Name, ready, taints, accel, topo, block, partitions, slices)
	}
	_ = tw.Flush()

	_, _ = fmt.Fprintf(w, "Description: Kubectl: TPU Node Summary\nOutput:\n%s\n%s\n\n", sb.String(), spacer)
}

func labelOrNone(v string) string {
	if strings.TrimSpace(v) == "" {
		return "<none>"
	}
	return strings.TrimSpace(v)
}

func formatNodeTaints(taints []kubernetesNodeTaint) string {
	if len(taints) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(taints))
	for _, t := range taints {
		if t.Key == "" {
			continue
		}
		entry := t.Key
		if t.Value != "" {
			entry += "=" + t.Value
		}
		if t.Effect != "" {
			entry += ":" + t.Effect
		}
		parts = append(parts, entry)
	}
	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ",")
}

func formatTPUNodeBlock(labels map[string]string) string {
	block := strings.TrimSpace(labels[gceTopologyBlockLabel])
	superpod := strings.TrimSpace(labels[tpuSuperpodIDLabel])
	switch {
	case block != "" && superpod != "":
		return fmt.Sprintf("%s(superpod=%s)", block, superpod)
	case block != "":
		return block
	case superpod != "":
		return fmt.Sprintf("superpod=%s", superpod)
	default:
		return "<none>"
	}
}

func formatTPUPrefixLabels(labels map[string]string, prefix string) string {
	ids := make(map[string]string)
	states := make(map[string]string)
	topos := make(map[string]bool)
	var extras []string

	for k, v := range labels {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		switch {
		case strings.HasSuffix(rest, "-id"):
			topo := strings.TrimSuffix(rest, "-id")
			ids[topo] = v
			topos[topo] = true
		case strings.HasSuffix(rest, "-state"):
			topo := strings.TrimSuffix(rest, "-state")
			states[topo] = v
			topos[topo] = true
		default:
			extras = append(extras, fmt.Sprintf("%s=%s", rest, v))
		}
	}

	var sortedTopos []string
	for t := range topos {
		sortedTopos = append(sortedTopos, t)
	}
	sort.Strings(sortedTopos)
	sort.Strings(extras)

	var entries []string
	for _, topo := range sortedTopos {
		id := ids[topo]
		st := states[topo]
		switch {
		case id != "" && st != "":
			entries = append(entries, fmt.Sprintf("%s:%s(%s)", topo, id, st))
		case id != "":
			entries = append(entries, fmt.Sprintf("%s:%s", topo, id))
		case st != "":
			entries = append(entries, fmt.Sprintf("%s:(%s)", topo, st))
		}
	}
	entries = append(entries, extras...)
	if len(entries) == 0 {
		return "<none>"
	}
	return strings.Join(entries, ",")
}

func formatTPUSliceLabels(labels map[string]string) string {
	var entries []string
	if v := strings.TrimSpace(labels[tpuSliceLabel]); v != "" {
		entries = append(entries, v)
	}
	if prefixed := formatTPUPrefixLabels(labels, tpuSliceLabelPrefix); prefixed != "<none>" {
		entries = append(entries, prefixed)
	}
	if len(entries) == 0 {
		return "<none>"
	}
	return strings.Join(entries, ",")
}

func (g *GKEOrchestrator) inspectSliceAndWAS(writer *inspectWriter, apiState inspectAPIState) inspectWASState {
	state := inspectWASState{
		hasKueue:              apiState.hasKueue,
		hasSlices:             apiState.hasSlices,
		hasTopologies:         apiState.hasTopologies,
		hasCompositePodGroups: apiState.hasCompositePodGroups,
		hasPodGroups:          apiState.hasPodGroups,
	}

	if state.hasSlices {
		res := writer.runAndLog("Slice Controller: Slices (YAML)", "kubectl", "get", slicesResource, "-o", "yaml")
		if res.ExitCode == 0 {
			state.slicesYAML = res.Stdout
		}
	} else if apiState.slicesDiscoveryFailed {
		if res, ok := writer.runAndLogOptional("Slice Controller: Slices (YAML)", "kubectl", "get", slicesResource, "-o", "yaml"); ok {
			state.hasSlices = true
			state.slicesYAML = res.Stdout
		}
	}

	if state.hasSlices || state.hasTopologies {
		g.inspectSliceController(writer)
	}

	if state.hasCompositePodGroups {
		res := writer.runAndLog("WAS: CompositePodGroups (YAML)", "kubectl", "get", compositePodGroupsResource, "-A", "-o", "yaml")
		if res.ExitCode == 0 {
			state.compositePodGroupsYAML = res.Stdout
		}
	} else if apiState.schedDiscoveryFailed {
		if res, ok := writer.runAndLogOptional("WAS: CompositePodGroups (YAML)", "kubectl", "get", compositePodGroupsResource, "-A", "-o", "yaml"); ok {
			state.hasCompositePodGroups = true
			state.compositePodGroupsYAML = res.Stdout
		}
	}

	if state.hasPodGroups {
		res := writer.runAndLog("WAS: PodGroups (YAML)", "kubectl", "get", podGroupsResource, "-A", "-o", "yaml")
		if res.ExitCode == 0 {
			state.podGroupsYAML = res.Stdout
		}
	} else if apiState.schedDiscoveryFailed {
		if res, ok := writer.runAndLogOptional("WAS: PodGroups (YAML)", "kubectl", "get", podGroupsResource, "-A", "-o", "yaml"); ok {
			state.hasPodGroups = true
			state.podGroupsYAML = res.Stdout
		}
	}

	if state.hasSlices || state.hasCompositePodGroups || state.hasPodGroups {
		g.inspectWASControllers(writer)
	}
	return state
}

func (g *GKEOrchestrator) inspectSliceController(writer *inspectWriter) {
	writer.runAndLogOptional("Slice Controller: Deployment Details", "kubectl", "describe", "deployment", sliceControllerDeployment, "-n", sliceControllerNamespace)
	writer.runAndLogOptional("Slice Controller: Logs (tail 500)", "kubectl", "logs", "deployment/"+sliceControllerDeployment, "-n", sliceControllerNamespace, "-c", "manager", wasControllerLogTail)
	writer.runAndLogOptional("Slice Controller: Previous Logs (tail 500)", "kubectl", "logs", "deployment/"+sliceControllerDeployment, "-n", sliceControllerNamespace, "-c", "manager", "--previous", wasControllerLogTail)
}

func (g *GKEOrchestrator) inspectWASControllers(writer *inspectWriter) {
	g.inspectTPUScheduler(writer)
	g.inspectTPUWorkloadController(writer)
}

func (g *GKEOrchestrator) discoverTPUSchedulerNamespaces() []string {
	res := g.kubectl("get", "pods", "-A", "-l", tpuSchedulerPodLabel, "-o", `jsonpath={range .items[*]}{.metadata.namespace}{"\n"}{end}`)
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return nil
	}
	seen := make(map[string]bool)
	var namespaces []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		ns := strings.TrimSpace(line)
		if ns != "" && !seen[ns] {
			seen[ns] = true
			namespaces = append(namespaces, ns)
		}
	}
	sort.Strings(namespaces)
	return namespaces
}

func (g *GKEOrchestrator) inspectTPUScheduler(writer *inspectWriter) {
	namespaces := g.discoverTPUSchedulerNamespaces()
	if len(namespaces) > 0 {
		for _, ns := range namespaces {
			g.inspectTPUSchedulerInNamespace(writer, ns)
		}
		return
	}
	for _, ns := range []string{tpuSchedulerSystemNamespace, tpuSchedulerLegacyNamespace} {
		if g.inspectTPUSchedulerInNamespace(writer, ns) {
			return
		}
	}
}

func (g *GKEOrchestrator) inspectTPUSchedulerInNamespace(writer *inspectWriter, ns string) bool {
	_, depOK := writer.runAndLogOptional("WAS: TPU Scheduler Deployment Details", "kubectl", "describe", "deployment", tpuSchedulerDeployment, "-n", ns)
	_, curOK := writer.runAndLogOptional("WAS: TPU Scheduler Logs (tail 500)", "kubectl", "logs", "deployment/"+tpuSchedulerDeployment, "-n", ns, "-c", tpuSchedulerContainer, wasControllerLogTail)
	_, prevOK := writer.runAndLogOptional("WAS: TPU Scheduler Previous Logs (tail 500)", "kubectl", "logs", "deployment/"+tpuSchedulerDeployment, "-n", ns, "-c", tpuSchedulerContainer, "--previous", wasControllerLogTail)
	podCurOK := false
	podPrevOK := false
	if !curOK && !prevOK {
		_, podCurOK = writer.runAndLogOptional("WAS: TPU Scheduler Logs (tail 500)", "kubectl", "logs", "-l", tpuSchedulerPodLabel, "-n", ns, "-c", tpuSchedulerContainer, wasControllerLogTail)
		_, podPrevOK = writer.runAndLogOptional("WAS: TPU Scheduler Previous Logs (tail 500)", "kubectl", "logs", "-l", tpuSchedulerPodLabel, "-n", ns, "-c", tpuSchedulerContainer, "--previous", wasControllerLogTail)
	}
	return depOK || curOK || prevOK || podCurOK || podPrevOK
}

func (g *GKEOrchestrator) inspectTPUWorkloadController(writer *inspectWriter) {
	writer.runAndLogOptional("WAS: TPU Workload Controller Deployment Details", "kubectl", "describe", "deployment", tpuWorkloadControllerDeployment, "-n", tpuWorkloadControllerNamespace)
	writer.runAndLogOptional("WAS: TPU Workload Controller Logs (tail 500)", "kubectl", "logs", "deployment/"+tpuWorkloadControllerDeployment, "-n", tpuWorkloadControllerNamespace, "-c", tpuWorkloadControllerContainer, wasControllerLogTail)
	writer.runAndLogOptional("WAS: TPU Workload Controller Previous Logs (tail 500)", "kubectl", "logs", "deployment/"+tpuWorkloadControllerDeployment, "-n", tpuWorkloadControllerNamespace, "-c", tpuWorkloadControllerContainer, "--previous", wasControllerLogTail)
}

func (g *GKEOrchestrator) inspectWorkload(writer *inspectWriter, workloadName, workloadNamespace string, wasState ...inspectWASState) {
	if workloadName == "" {
		return
	}

	hasKueue := true
	if len(wasState) > 0 {
		hasKueue = wasState[0].hasKueue
	}

	if hasKueue {
		g.logWorkloadList(writer.writer, "EVERYTHING", workloadName, workloadNamespace)
	}

	writer.runAndLog(fmt.Sprintf("JobSet: Config for %s", workloadName), "kubectl", "describe", "jobsets", workloadName, "-n", workloadNamespace)

	var targetWorkload string
	if hasKueue {
		targetWorkload = fmt.Sprintf("jobset-%s", workloadName)
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

	podSelector := fmt.Sprintf("jobset.sigs.k8s.io/jobset-name=%s", workloadName)
	writer.runAndLog(fmt.Sprintf("Pods: Status for %s", workloadName), "kubectl", "get", "pods", "-l", podSelector, "-n", workloadNamespace, "-o", "wide")
	writer.runAndLog(fmt.Sprintf("Pods: Config and Events for %s", workloadName), "kubectl", "describe", "pods", "-l", podSelector, "-n", workloadNamespace)

	if len(wasState) > 0 {
		g.inspectWorkloadWAS(writer, workloadName, workloadNamespace, targetWorkload, wasState[0])
	}
}

func (g *GKEOrchestrator) fetchWorkloadPodsJSON(workloadName, workloadNamespace string) string {
	if workloadName == "" {
		return ""
	}
	selectors := []string{
		fmt.Sprintf("jobset.sigs.k8s.io/jobset-name=%s", workloadName),
		fmt.Sprintf("gcluster.google.com/workload=%s", workloadName),
	}
	for _, sel := range selectors {
		args := []string{"get", "pods"}
		if workloadNamespace != "" {
			args = append(args, "-n", workloadNamespace)
		}
		args = append(args, "-l", sel, "-o", "json")
		res := g.kubectl(args...)
		if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
			continue
		}
		var podList wasPodList
		if err := json.Unmarshal([]byte(res.Stdout), &podList); err == nil && len(podList.Items) > 0 {
			return res.Stdout
		}
	}
	return ""
}

func uniqueResourceNames(items []wasResourceItem) []string {
	seen := make(map[string]bool)
	var names []string
	for _, item := range items {
		name := strings.TrimSpace(item.Metadata.Name)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (g *GKEOrchestrator) inspectWorkloadWAS(writer *inspectWriter, workloadName, workloadNamespace, targetWorkload string, wasState inspectWASState) {
	if !wasState.hasSlices && !wasState.hasCompositePodGroups && !wasState.hasPodGroups {
		return
	}
	if wasState.workloadPodsJSON == "" {
		wasState.workloadPodsJSON = g.fetchWorkloadPodsJSON(workloadName, workloadNamespace)
	}

	ownedCPGs, ownedPGs, ownedSlices := filterOwnedWASResources(workloadName, workloadNamespace, targetWorkload, wasState)

	if wasState.hasCompositePodGroups {
		if names := uniqueResourceNames(ownedCPGs); len(names) > 0 {
			args := append([]string{"describe", compositePodGroupsResource}, names...)
			args = append(args, "-n", workloadNamespace)
			writer.runAndLogOptional(fmt.Sprintf("WAS: CompositePodGroups for %s", workloadName), "kubectl", args...)
		}
	}
	if wasState.hasPodGroups {
		if names := uniqueResourceNames(ownedPGs); len(names) > 0 {
			args := append([]string{"describe", podGroupsResource}, names...)
			args = append(args, "-n", workloadNamespace)
			writer.runAndLogOptional(fmt.Sprintf("WAS: PodGroups for %s", workloadName), "kubectl", args...)
		}
	}
	if wasState.hasSlices {
		if names := uniqueResourceNames(ownedSlices); len(names) > 0 {
			args := append([]string{"describe", slicesResource}, names...)
			writer.runAndLogOptional(fmt.Sprintf("WAS: Slices for %s", workloadName), "kubectl", args...)
		}
	}
	logOwnedWASResources(writer.writer, workloadName, ownedCPGs, ownedPGs, ownedSlices)
}

func logOwnedWASResources(w io.Writer, workloadName string, ownedCPGs, ownedPGs, ownedSlices []wasResourceItem) {
	if len(ownedCPGs) == 0 && len(ownedPGs) == 0 && len(ownedSlices) == 0 {
		return
	}

	var allOwned []wasResourceItem
	allOwned = append(allOwned, ownedCPGs...)
	allOwned = append(allOwned, ownedPGs...)
	allOwned = append(allOwned, ownedSlices...)

	var rawObjects []interface{}
	for _, item := range allOwned {
		if item.raw != nil {
			rawObjects = append(rawObjects, sanitizeK8sValue(item.raw))
		} else {
			rawObjects = append(rawObjects, item)
		}
	}

	yamlBytes, err := k8syaml.Marshal(rawObjects)
	if err != nil {
		return
	}

	desc := fmt.Sprintf("WAS: Owned Resources for %s", workloadName)
	_, _ = fmt.Fprintf(w, "Description: %s\nOutput:\n%s\n%s\n\n", desc, redactSensitiveText(string(yamlBytes)), spacer)
}

func filterOwnedWASResources(workloadName, workloadNamespace, targetWorkload string, wasState inspectWASState) (cpgs, pgs, slices []wasResourceItem) {
	if workloadName == "" {
		return nil, nil, nil
	}

	ownedCPGs := make(map[string]bool)
	ownedPGs := make(map[string]bool)
	ownedSliceNames := make(map[string]bool)
	seedPodNames := make(map[string]bool)
	seedOwnerNames := make(map[string]bool)

	seedFromWorkloadPods(wasState.workloadPodsJSON, workloadNamespace, ownedCPGs, ownedPGs, ownedSliceNames, seedPodNames, seedOwnerNames)

	allCPGs := filterItemsByNamespace(parseWASResourceItems(wasState.compositePodGroupsYAML), workloadNamespace)
	allPGs := filterItemsByNamespace(parseWASResourceItems(wasState.podGroupsYAML), workloadNamespace)

	for _, item := range allCPGs {
		if item.Metadata.Name == "" || hasConflictingWorkloadOwner(item, workloadName, targetWorkload) {
			continue
		}
		if ownedCPGs[item.Metadata.Name] || isDirectlyOwnedByWorkload(item, workloadName, targetWorkload, seedPodNames, seedOwnerNames) {
			ownedCPGs[item.Metadata.Name] = true
		}
	}
	for _, item := range allPGs {
		if item.Metadata.Name == "" || hasConflictingWorkloadOwner(item, workloadName, targetWorkload) {
			continue
		}
		if ownedPGs[item.Metadata.Name] || isDirectlyOwnedByWorkload(item, workloadName, targetWorkload, seedPodNames, seedOwnerNames) {
			ownedPGs[item.Metadata.Name] = true
		}
	}

	// Walk spec.parentCompositePodGroupName (and CompositePodGroup ownerReferences) both upward and downward to fixed point.
	for changed := true; changed; {
		changed = false
		for _, item := range allPGs {
			if !ownedPGs[item.Metadata.Name] {
				continue
			}
			for _, parent := range parentCPGNames(item) {
				if !ownedCPGs[parent] {
					ownedCPGs[parent] = true
					changed = true
				}
			}
		}
		for _, item := range allCPGs {
			if !ownedCPGs[item.Metadata.Name] {
				continue
			}
			for _, parent := range parentCPGNames(item) {
				if !ownedCPGs[parent] {
					ownedCPGs[parent] = true
					changed = true
				}
			}
		}
		for _, item := range allCPGs {
			if item.Metadata.Name == "" || ownedCPGs[item.Metadata.Name] || hasConflictingWorkloadOwner(item, workloadName, targetWorkload) {
				continue
			}
			for _, parent := range parentCPGNames(item) {
				if ownedCPGs[parent] {
					ownedCPGs[item.Metadata.Name] = true
					changed = true
					break
				}
			}
		}
		for _, item := range allPGs {
			if item.Metadata.Name == "" || ownedPGs[item.Metadata.Name] || hasConflictingWorkloadOwner(item, workloadName, targetWorkload) {
				continue
			}
			for _, parent := range parentCPGNames(item) {
				if ownedCPGs[parent] {
					ownedPGs[item.Metadata.Name] = true
					changed = true
					break
				}
			}
		}
	}

	for _, item := range allCPGs {
		if ownedCPGs[item.Metadata.Name] {
			cpgs = append(cpgs, item)
			recordSliceAnnotationRefs(item.Metadata.Annotations, ownedSliceNames)
		}
	}
	for _, item := range allPGs {
		if ownedPGs[item.Metadata.Name] {
			pgs = append(pgs, item)
			recordSliceAnnotationRefs(item.Metadata.Annotations, ownedSliceNames)
		}
	}

	existingCPGs := make(map[string]bool, len(allCPGs))
	for _, item := range allCPGs {
		if item.Metadata.Name != "" {
			existingCPGs[item.Metadata.Name] = true
		}
	}
	existingPGs := make(map[string]bool, len(allPGs))
	for _, item := range allPGs {
		if item.Metadata.Name != "" {
			existingPGs[item.Metadata.Name] = true
		}
	}
	for _, item := range parseWASResourceItems(wasState.slicesYAML) {
		if isSliceOwnedByWorkload(item, workloadName, workloadNamespace, targetWorkload, ownedCPGs, ownedPGs, ownedSliceNames, existingCPGs, existingPGs) {
			slices = append(slices, item)
		}
	}

	return cpgs, pgs, slices
}

func seedFromWorkloadPods(podsJSON, workloadNamespace string, ownedCPGs, ownedPGs, ownedSliceNames, seedPodNames, seedOwnerNames map[string]bool) {
	if strings.TrimSpace(podsJSON) == "" {
		return
	}
	var podList wasPodList
	if err := json.Unmarshal([]byte(podsJSON), &podList); err != nil {
		return
	}
	for _, pod := range podList.Items {
		if workloadNamespace != "" && pod.Metadata.Namespace != "" && pod.Metadata.Namespace != workloadNamespace {
			continue
		}
		if pod.Metadata.Name != "" {
			seedPodNames[pod.Metadata.Name] = true
		}
		for _, ref := range pod.Metadata.OwnerReferences {
			if name := strings.TrimSpace(ref.Name); name != "" {
				seedOwnerNames[name] = true
			}
		}
		if pod.Spec.SchedulingGroup != nil {
			if pgName := strings.TrimSpace(pod.Spec.SchedulingGroup.PodGroupName); pgName != "" {
				ownedPGs[pgName] = true
			}
		}
		if ann := pod.Metadata.Annotations["tpuplugin.cloud.google.com/pod-group-names"]; ann != "" {
			for _, pgName := range strings.Split(ann, ",") {
				if trimmed := strings.TrimSpace(pgName); trimmed != "" {
					ownedPGs[trimmed] = true
				}
			}
		}
		if parentCPG := strings.TrimSpace(pod.Metadata.Annotations["tpuplugin.cloud.google.com/parent-composite-pod-group-name"]); parentCPG != "" {
			ownedCPGs[parentCPG] = true
		}
		recordSliceAnnotationRefs(pod.Metadata.Annotations, ownedSliceNames)
	}
}

func filterItemsByNamespace(items []wasResourceItem, workloadNamespace string) []wasResourceItem {
	if workloadNamespace == "" {
		return items
	}
	filtered := make([]wasResourceItem, 0, len(items))
	for _, item := range items {
		if item.Metadata.Namespace != "" && item.Metadata.Namespace != workloadNamespace {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func parseWASResourceItems(rawYAML string) []wasResourceItem {
	if strings.TrimSpace(rawYAML) == "" {
		return nil
	}
	data := []byte(rawYAML)
	var list wasResourceList
	if err := k8syaml.Unmarshal(data, &list); err == nil && len(list.Items) > 0 {
		var rawList struct {
			Items []map[string]interface{} `json:"items"`
		}
		if err := k8syaml.Unmarshal(data, &rawList); err == nil && len(rawList.Items) == len(list.Items) {
			for i := range list.Items {
				list.Items[i].raw = rawList.Items[i]
			}
		}
		return list.Items
	}
	var single wasResourceItem
	if err := k8syaml.Unmarshal(data, &single); err == nil && single.Metadata.Name != "" {
		var rawSingle map[string]interface{}
		if err := k8syaml.Unmarshal(data, &rawSingle); err == nil {
			single.raw = rawSingle
		}
		return []wasResourceItem{single}
	}
	return nil
}

func matchesExactWorkload(candidate, workloadName, targetWorkload string) bool {
	if candidate == "" {
		return false
	}
	if candidate == workloadName {
		return true
	}
	return targetWorkload != "" && candidate == targetWorkload
}

func parseInitOwnerRef(ann string) (kind, name string, ok bool) {
	trimmed := strings.TrimSpace(ann)
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	switch len(parts) {
	case 4:
		if parts[2] != "" && parts[3] != "" {
			return parts[2], parts[3], true
		}
	case 3:
		if parts[1] != "" && parts[2] != "" {
			return parts[1], parts[2], true
		}
	}
	return "", "", false
}

func parentCPGNames(item wasResourceItem) []string {
	var parents []string
	seen := make(map[string]bool)
	add := func(name string) {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			parents = append(parents, trimmed)
		}
	}
	add(item.Spec.ParentCompositePodGroupName)
	add(item.Metadata.Annotations["tpuplugin.cloud.google.com/parent-composite-pod-group-name"])
	for _, ref := range item.Metadata.OwnerReferences {
		if strings.EqualFold(ref.Kind, "CompositePodGroup") {
			add(ref.Name)
		}
	}
	return parents
}

func recordSliceAnnotationRefs(annotations map[string]string, ownedSliceNames map[string]bool) {
	if sliceAnn := annotations["tpuplugin.cloud.google.com/slice-name"]; sliceAnn != "" {
		for _, sName := range strings.Split(sliceAnn, ",") {
			if trimmed := strings.TrimSpace(sName); trimmed != "" {
				ownedSliceNames[trimmed] = true
			}
		}
	}
}

func hasConflictingWorkloadOwner(item wasResourceItem, workloadName, targetWorkload string) bool {
	if jsLabel := strings.TrimSpace(item.Metadata.Labels["jobset.sigs.k8s.io/jobset-name"]); jsLabel != "" {
		return jsLabel != workloadName
	}
	if wlLabel := strings.TrimSpace(item.Metadata.Labels["gcluster.google.com/workload"]); wlLabel != "" {
		return wlLabel != workloadName
	}
	for _, ref := range item.Metadata.OwnerReferences {
		if strings.EqualFold(ref.Kind, "JobSet") && ref.Name != "" {
			return ref.Name != workloadName
		}
	}
	if kind, name, ok := parseInitOwnerRef(item.Metadata.Annotations["tpuplugin.cloud.google.com/init-owner-ref"]); ok {
		if strings.EqualFold(kind, "JobSet") {
			return name != workloadName
		}
	}
	return false
}

func isDirectlyOwnedByWorkload(item wasResourceItem, workloadName, targetWorkload string, seedPodNames, seedOwnerNames map[string]bool) bool {
	if jsLabel := strings.TrimSpace(item.Metadata.Labels["jobset.sigs.k8s.io/jobset-name"]); jsLabel != "" {
		return jsLabel == workloadName
	}
	if wlLabel := strings.TrimSpace(item.Metadata.Labels["gcluster.google.com/workload"]); wlLabel != "" {
		return wlLabel == workloadName
	}
	for _, ref := range item.Metadata.OwnerReferences {
		if strings.EqualFold(ref.Kind, "JobSet") && ref.Name != "" {
			return ref.Name == workloadName
		}
	}
	hasExplicitOwner := len(item.Metadata.OwnerReferences) > 0
	for _, ref := range item.Metadata.OwnerReferences {
		if matchesExactWorkload(ref.Name, workloadName, targetWorkload) {
			return true
		}
		if strings.EqualFold(ref.Kind, "Pod") && seedPodNames[ref.Name] {
			return true
		}
		if seedOwnerNames[ref.Name] {
			return true
		}
	}
	if kind, name, ok := parseInitOwnerRef(item.Metadata.Annotations["tpuplugin.cloud.google.com/init-owner-ref"]); ok {
		if strings.EqualFold(kind, "JobSet") {
			return name == workloadName
		}
		if matchesExactWorkload(name, workloadName, targetWorkload) {
			return true
		}
		if strings.EqualFold(kind, "Pod") && seedPodNames[name] {
			return true
		}
		if seedOwnerNames[name] {
			return true
		}
		hasExplicitOwner = true
	}
	if item.Spec.WorkloadRef != nil {
		if wlRef := strings.TrimSpace(item.Spec.WorkloadRef.WorkloadName); wlRef != "" {
			if matchesExactWorkload(wlRef, workloadName, targetWorkload) || seedPodNames[wlRef] || seedOwnerNames[wlRef] {
				return true
			}
			hasExplicitOwner = true
		}
	}
	hasOwnerLabel := false
	for _, k := range []string{
		"batch.kubernetes.io/job-name",
		"job-name",
		"kueue.x-k8s.io/job-name",
		"tpuplugin.cloud.google.com/owner-name",
	} {
		if v := strings.TrimSpace(item.Metadata.Labels[k]); v != "" {
			hasOwnerLabel = true
			if matchesExactWorkload(v, workloadName, targetWorkload) || seedOwnerNames[v] {
				return true
			}
		}
	}
	if hasOwnerLabel || hasExplicitOwner {
		return false
	}
	return matchesExactWorkload(item.Metadata.Name, workloadName, targetWorkload)
}

func isSliceOwnedByWorkload(item wasResourceItem, workloadName, workloadNamespace, targetWorkload string, ownedCPGs, ownedPGs, ownedSliceNames, existingCPGs, existingPGs map[string]bool) bool {
	if ownedSliceNames[item.Metadata.Name] {
		return true
	}
	if ownerRefAnn := strings.TrimSpace(item.Metadata.Annotations["tpuplugin.cloud.google.com/slice-owner-reference"]); ownerRefAnn != "" {
		parts := strings.Split(ownerRefAnn, "/")
		switch len(parts) {
		case 3:
			ownerKind, ownerNS, ownerName := parts[0], parts[1], parts[2]
			if workloadNamespace != "" && ownerNS != "" && ownerNS != workloadNamespace {
				return false
			}
			switch strings.ToLower(ownerKind) {
			case "compositepodgroup":
				return ownedCPGs[ownerName] || (!existingCPGs[ownerName] && matchesExactWorkload(ownerName, workloadName, targetWorkload))
			case "podgroup":
				return ownedPGs[ownerName] || (!existingPGs[ownerName] && matchesExactWorkload(ownerName, workloadName, targetWorkload))
			default:
				return matchesExactWorkload(ownerName, workloadName, targetWorkload)
			}
		case 2:
			ownerNS, ownerName := parts[0], parts[1]
			if workloadNamespace != "" && ownerNS != "" && ownerNS != workloadNamespace {
				return false
			}
			return ownedCPGs[ownerName] || ownedPGs[ownerName] || (!existingCPGs[ownerName] && !existingPGs[ownerName] && matchesExactWorkload(ownerName, workloadName, targetWorkload))
		case 1:
			ownerName := parts[0]
			return ownedCPGs[ownerName] || ownedPGs[ownerName] || (!existingCPGs[ownerName] && !existingPGs[ownerName] && matchesExactWorkload(ownerName, workloadName, targetWorkload))
		default:
			return false
		}
	}
	// Unowned Slices entering delayed deletion keep their creator's GenerateName prefix
	// after slice-owner-reference is cleared; never attribute them to a workload.
	if strings.TrimSpace(item.Metadata.Annotations["tpuplugin.cloud.google.com/delayed-slice-deletion-time"]) != "" {
		return false
	}
	ownerNS := item.Metadata.Labels["tpuplugin.cloud.google.com/owner-namespace"]
	if ownerNS == "" {
		ownerNS = item.Metadata.Labels["kueue.gke.io/tpu-slice-used-by-namespace"]
	}
	if workloadNamespace != "" && ownerNS != "" && ownerNS != workloadNamespace {
		return false
	}
	hasOwnerLabel := false
	for _, k := range []string{"tpuplugin.cloud.google.com/owner-name", "kueue.gke.io/tpu-slice-used-by-workload"} {
		if v := strings.TrimSpace(item.Metadata.Labels[k]); v != "" {
			hasOwnerLabel = true
			if ownedCPGs[v] || ownedPGs[v] || matchesExactWorkload(v, workloadName, targetWorkload) {
				return true
			}
		}
	}
	if hasOwnerLabel {
		return false
	}
	if len(item.Metadata.OwnerReferences) > 0 {
		for _, ref := range item.Metadata.OwnerReferences {
			switch strings.ToLower(ref.Kind) {
			case "jobset":
				return ref.Name == workloadName
			case "compositepodgroup":
				if ownedCPGs[ref.Name] {
					return true
				}
			case "podgroup":
				if ownedPGs[ref.Name] {
					return true
				}
			default:
				if matchesExactWorkload(ref.Name, workloadName, targetWorkload) {
					return true
				}
			}
		}
		return false
	}
	return matchesExactWorkload(item.Metadata.Name, workloadName, targetWorkload)
}

func (g *GKEOrchestrator) logNodeCounts(w io.Writer) *kubernetesNodeList {
	desc := "Kubectl: Node count analysis"
	_, _ = fmt.Fprintf(w, "Description: %s\n", desc)

	nodeList, err := g.fetchNodeList()
	if err != nil {
		errStr := fmt.Sprintf("Error fetching nodes for analysis: %v\n", err)
		_, _ = fmt.Fprint(w, errStr)
		_, _ = fmt.Fprintf(w, "\n%s\n\n", spacer)
		return nil
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
	return nodeList
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

var (
	spsReadySlicesRe = regexp.MustCompile(`(\d+)/(\d+) Pathways Slices Now Ready`)
	jaxVersionRe     = regexp.MustCompile(`(?i)jax[-_]?v?(\d+\.\d+(?:\.\d+)?)`)
	pythonVersionRe  = regexp.MustCompile(`(?i)(?:python|py)[-_]?(3\.\d+)`)
)

const spsSidecarVersionScript = "import jax, jaxlib, sys; print(f'{sys.version_info.major}.{sys.version_info.minor},{jax.__version__},{jaxlib.__version__}')"

func openInspectOutput(opts orchestrator.InspectOptions, defaultSuffix string) (*os.File, io.Writer, string, error) {
	filePath := opts.OutputPath
	if filePath == "" {
		timestamp := time.Now().UTC().Format("20060102-150405")
		fileName := fmt.Sprintf("gcluster-inspect-%s-%s%s", opts.ClusterName, timestamp, defaultSuffix)
		filePath = filepath.Join(".", fileName)
	}
	file, err := os.Create(filePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to create log file %s: %w", filePath, err)
	}
	var outputTarget io.Writer = file
	if opts.Show {
		outputTarget = io.MultiWriter(file, os.Stdout)
	}
	return file, outputTarget, filePath, nil
}

func (g *GKEOrchestrator) inspectSPSInstanceToLog(opts orchestrator.InspectOptions, targetNamespace string) error {
	summary, err := g.inspectSPSInstance(opts.WorkloadName, targetNamespace)
	if err != nil {
		return err
	}
	file, outputTarget, filePath, err := openInspectOutput(opts, "-sps.log")
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	_, _ = fmt.Fprint(outputTarget, formatSPSSummary(summary))
	logging.Info("Cluster inspection report saved to %s", filePath)
	return nil
}

// inspectSPSInstance reads the JobSet name in namespace ns and summarizes it as an SPS instance.
// It works for instances deployed with --pathways-deploy-sps and with the SPS deploy script.
func (g *GKEOrchestrator) inspectSPSInstance(name, ns string) (spsInstanceSummary, error) {
	res := g.kubectl("get", "jobset", name, "-n", ns, "-o", "json", kubectlReadTimeout)
	if res.ExitCode != 0 {
		return spsInstanceSummary{}, fmt.Errorf("failed to read JobSet %q in namespace %q: %s", name, ns, strings.TrimSpace(res.Stderr))
	}
	summary, err := parseSPSInstance(name, ns, []byte(res.Stdout))
	if err != nil {
		return spsInstanceSummary{}, err
	}
	if summary.Status == "Running" {
		logs := g.kubectl("logs", "-n", ns, "-l", "jobset.sigs.k8s.io/jobset-name="+name+",jobset.sigs.k8s.io/replicatedjob-name=pathways-head",
			"-c", "pathways-rm", "--tail=-1", kubectlReadTimeout)
		if logs.ExitCode == 0 {
			if ready := lastReadySlices(logs.Stdout); ready != "" {
				summary.ReadySlices = ready
			}
		}
		g.probeSPSSidecarVersions(name, ns, &summary)
	}
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
		"-l", "jobset.sigs.k8s.io/jobset-name="+name+",jobset.sigs.k8s.io/replicatedjob-name!=pathways-head",
		"--field-selector=status.phase=Running",
		"-o", "jsonpath={.items[*].metadata.name}",
		kubectlReadTimeout)
	if pods.ExitCode != 0 {
		return
	}
	workerPods := strings.Fields(pods.Stdout)
	for _, pod := range workerPods[:min(len(workerPods), 3)] {
		res := g.kubectlWithTimeout(10*time.Second, "exec", "-n", ns, "pod/"+pod, "-c", s.SidecarName, "--", "python3", "-c", spsSidecarVersionScript)
		if res.ExitCode == 0 && applyProbedVersions(res.Stdout, s) {
			return
		}
	}
}

func applyProbedVersions(stdout string, s *spsInstanceSummary) bool {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	parts := strings.Split(strings.TrimSpace(lines[len(lines)-1]), ",")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return false
	}
	s.SidecarPython = strings.TrimSpace(parts[0])
	jax := strings.TrimSpace(parts[1])
	if jaxlib := strings.TrimSpace(parts[2]); jaxlib != "" {
		jax = fmt.Sprintf("%s (jaxlib %s)", jax, jaxlib)
	}
	s.SidecarJAX = jax
	return true
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
	if summary.Status == "Running" {
		summary.ReadySlices = statusReadySlices(&js, summary.InstanceCount)
	}
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
	if instanceCount <= 0 {
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
	var rm spsContainerView
	var foundRM bool
	for _, c := range append(append([]spsContainerView{}, head.InitContainers...), head.Containers...) {
		switch {
		case c.Name == "pathways-rm":
			rm = c
			foundRM = true
		case c.Name == "pathways-proxy" || strings.HasPrefix(c.Name, "workload-container") || c.Name == "user-workload":
			return fmt.Errorf("JobSet %q is not an SPS instance: its pathways-head runs %s, so it is a regular or headless Pathways job", name, c.Name)
		}
	}
	if !foundRM {
		return fmt.Errorf("JobSet %q is not an SPS instance: its pathways-head does not run pathways-rm", name)
	}
	summary.ServerImage = rm.Image
	parseRMArgs(rm.Args, summary)
	return nil
}

func parseRMArgs(args []string, summary *spsInstanceSummary) {
	summary.InstanceType, summary.InstanceCount, _ = parsePathwaysRMArgs(args)
}

func parsePathwaysRMArgs(args []string) (instanceType string, instanceCount int, gcsLocation string) {
	for i := 0; i < len(args); i++ {
		key, value, ok := strings.Cut(args[i], "=")
		if !ok && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		} else if !ok {
			continue
		}
		switch strings.TrimLeft(key, "-") {
		case "gcs_scratch_location":
			gcsLocation = value
		case "instance_type":
			instanceType = value
		case "instance_count":
			instanceCount, _ = strconv.Atoi(value)
		}
	}
	return instanceType, instanceCount, gcsLocation
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
		orUnknown := func(v string) string {
			if v == "" {
				return "unknown (not in image tag)"
			}
			return v
		}
		row("Sidecar image", s.SidecarImage)
		row("Sidecar Python", orUnknown(s.SidecarPython))
		row("Sidecar JAX", orUnknown(s.SidecarJAX))
	}
	_ = tw.Flush()
	return b.String()
}
