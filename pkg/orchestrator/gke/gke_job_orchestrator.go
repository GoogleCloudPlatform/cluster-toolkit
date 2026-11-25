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
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/imagebuilder"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"gopkg.in/yaml.v2"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	// maxLogRequests caps the number of pods from which gcluster fetches logs in the terminal before
	// redirecting to Cloud Logging, and serves as the floor for kubectl's --max-log-requests flag.
	maxLogRequests = 10

	// jobSetInfoJSONPath reads terminalState, the Suspended condition status, and the first replicated job's
	// name on line 1 (tab-separated), followed by "<replicas> <parallelism>\t<container names>" per replicated job.
	jobSetInfoJSONPath = `{.status.terminalState}{"\t"}{.status.conditions[?(@.type=="Suspended")].status}{"\t"}{.spec.replicatedJobs[0].name}{"\n"}{range .spec.replicatedJobs[*]}{.replicas}{" "}{.template.spec.parallelism}{"\t"}{.template.spec.template.spec.initContainers[*].name}{" "}{.template.spec.template.spec.containers[*].name}{"\n"}{end}`

	// podContainersJSONPath prints one line per pod with all container names for pod and stream counting.
	podContainersJSONPath = `{range .items[*]}{.spec.initContainers[*].name}{" "}{.spec.containers[*].name}{" "}{.spec.ephemeralContainers[*].name}{"\n"}{end}`

	// spsEndpointFormat is the address SPS clients use to reach an SPS instance's resource manager,
	// formatted with the instance's workload name.
	spsEndpointFormat = "%[1]s-pathways-head-0-0.%[1]s:29001"

	// unscheduledPodsJSONPath prints one tab-separated line per pod with name, phase, and PodScheduled condition status/reason/message.
	unscheduledPodsJSONPath = `{range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\t"}{.status.conditions[?(@.type=="PodScheduled")].status}{"\t"}{.status.conditions[?(@.type=="PodScheduled")].reason}{"\t"}{.status.conditions[?(@.type=="PodScheduled")].message}{"\n"}{end}`
)

// logsRetryInterval is the wait between waitForJobPods and fetchLogsWithRetry attempts; tests shorten it.
var logsRetryInterval = 5 * time.Second

// directAwaitSleep sleeps between awaitWorkloadDirect poll attempts; tests override it to avoid real-time waits.
var directAwaitSleep = time.Sleep

// logsMaxAttempts bounds both wait loops: 12 attempts 5s apart (~1 minute plus kubectl latency).
const logsMaxAttempts = 12

func NewGKEOrchestrator() *GKEOrchestrator {
	return &GKEOrchestrator{
		executor:          &DefaultExecutor{},
		machineTypeClient: &DefaultMachineTypeClient{},
		machineCapCache:   make(map[string]MachineTypeCap),
		submissionSession: newSubmissionSession(),
	}
}

func (g *GKEOrchestrator) SetExecutor(e Executor) {
	g.executor = e
}

func (g *GKEOrchestrator) SetDynamicClient(c dynamic.Interface) {
	g.inventory = nil
	g.dynClient = c
	g.dynClientManaged = false
	g.dynClientContext = ""
	if d, isDefault := g.kubeClient.(*DefaultKubeClient); !isDefault || d == nil {
		g.kubeClient = nil
	}
	if c == nil {
		g.kubeClient = nil
		return
	}
	g.syncKubeClient()
}

func (g *GKEOrchestrator) SetKubeClient(c KubeClient) {
	g.inventory = nil
	if d, ok := c.(*DefaultKubeClient); ok && d == nil {
		c = nil
	}
	if c == nil {
		g.kubeClient = nil
		g.dynClient = nil
		g.dynClientManaged = false
		g.dynClientContext = ""
		return
	}
	if d, ok := c.(*DefaultKubeClient); ok {
		g.dynClient = d.dynClient
		g.dynClientManaged = false
		g.dynClientContext = ""
	} else {
		g.resetManagedDynamicClient()
	}
	g.kubeClient = c
}

func (g *GKEOrchestrator) resetStorageCaches() {
	g.inspectedPVCs = make(map[string]pvcInspectResult)
	g.nfsAuthChecked = false
	g.jobSetAPIProbed = false
	g.jobSetAPIPresent = false
	g.jobSetAPIContext = ""
}

func (g *GKEOrchestrator) resetSubmissionSession() {
	g.submissionSession = newSubmissionSession()
}

// SubmitJob submits a job to the cluster. It processes the job definition,
// creates the required Kubernetes manifests (JobSet, or standalone batch/v1 Indexed Job when JobSet is absent),
// and applies them to the cluster.
func (g *GKEOrchestrator) SubmitJob(job orchestrator.JobDefinition) error {
	job.GKENamespace = strings.TrimSpace(job.GKENamespace)
	job.Pathways.RunSPSWorkload = strings.TrimSpace(job.Pathways.RunSPSWorkload)
	g.namespace = job.GKENamespace
	g.resetSubmissionSession()
	logging.Info("Starting gcluster job submit workflow...")
	g.gkeCustomTemplatesPath = job.GkeCustomTemplatesPath

	sm := &StorageManager{orchestrator: g}
	if err := g.validateSubmitLocalPreflight(sm, job); err != nil {
		return err
	}
	if job.Pathways.RunSPSWorkload != "" {
		return g.runSPSWorkload(job)
	}

	if err := g.prepareAndValidateSubmission(&job); err != nil {
		return err
	}

	profile, isDynamicSlicing, isStaticSlicing, err := g.resolveHardwareRequirements(&job)
	if err != nil {
		return err
	}
	if err := g.validatePostHardwareChecks(sm, &job, profile); err != nil {
		return err
	}

	fullImageName, err := g.BuildContainerImage(job)
	if err != nil {
		return err
	}

	if err := g.generateAndSubmitManifests(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing); err != nil {
		return err
	}

	if err := g.finishJobSubmission(job); err != nil {
		return err
	}
	logging.Info("gcluster job submit workflow completed.")

	return nil
}

func (g *GKEOrchestrator) prepareAndValidateSubmission(job *orchestrator.JobDefinition) error {
	if err := g.initializeJobSubmission(job); err != nil {
		return err
	}
	return g.fetchClusterState(job)
}

func (g *GKEOrchestrator) validatePostHardwareChecks(sm *StorageManager, job *orchestrator.JobDefinition, profile JobProfile) error {
	if err := g.validateTier3Workload(job, profile.IsTPU); err != nil {
		return err
	}
	if err := sm.ValidateClusterMounts(*job); err != nil {
		return err
	}
	if err := sm.RunStorageProfilePreflight(job.RawMounts, job.ProjectID, job.ClusterLocation, job.DryRunManifest != ""); err != nil {
		return err
	}
	return g.pinTelemetryAndValidate(job, profile.IsTPU)
}

// workloadTemplateName selects the JobSet workload template assumed before cluster add-on resolution.
func workloadTemplateName(job orchestrator.JobDefinition) string {
	if job.IsPathwaysJob || job.Pathways.RunSPSWorkload != "" {
		return k8s.PathwaysJobSetTemplate
	}
	return k8s.JobSetTemplate
}

func (g *GKEOrchestrator) resolveWorkloadTemplateName(job orchestrator.JobDefinition) string {
	if job.IsPathwaysJob || job.Pathways.RunSPSWorkload != "" {
		return k8s.PathwaysJobSetTemplate
	}
	if !g.jobSetEnabled() {
		return k8s.JobTemplate
	}
	return k8s.JobSetTemplate
}

func (g *GKEOrchestrator) validateResolvedCustomTemplate(job orchestrator.JobDefinition) error {
	if g.gkeCustomTemplatesPath == "" || g.isEnvTruthy(disableJobSetEnvVar) {
		return nil
	}
	resolved := g.resolveWorkloadTemplateName(job)
	if resolved == workloadTemplateName(job) {
		return nil
	}
	return k8s.ValidateCustomWorkloadTemplates(g.gkeCustomTemplatesPath, resolved)
}

// pinTelemetryAndValidate runs the cheap name-conflict check, then resolves the ML Diagnostics collector before
// the image build, so a missing TPU node image cache or an RBAC denial fails fast and PrepareManifestOptions sees
// the resolved image as explicit, and finally validates the MTC configuration.
func (g *GKEOrchestrator) pinTelemetryAndValidate(job *orchestrator.JobDefinition, isTPU bool) error {
	if err := g.validateJobConflicts(*job); err != nil {
		return err
	}
	telemetryImg, _, err := g.resolveTelemetryCollector(*job, isTPU)
	if err != nil {
		return err
	}
	job.MLDiagnosticsImage = telemetryImg
	return g.validateMTCConfig(job)
}

// validateSubmitLocalPreflight runs the checks that need no cluster access or image build: GKE-only flag
// rejection in TypeK8s, Tier-3 flag compatibility when GCLUSTER_DISABLE_JOBSET is set, mount syntax,
// restart bounds, ML Diagnostics flags, Pathways image resolution, and custom template parsing.
func (g *GKEOrchestrator) validateSubmitLocalPreflight(sm *StorageManager, job orchestrator.JobDefinition) error {
	if g.mode == orchestrator.TypeK8s {
		if err := orchestrator.ValidateK8sSubmitJob(job); err != nil {
			return err
		}
	} else if err := validateGKETPUNumNodes(job); err != nil {
		return err
	}
	if !g.jobSetEnabled() {
		if err := g.validateTier3FlagsWithWarn(&job, false); err != nil {
			return err
		}
	}
	if err := sm.validateJobMounts(job.RawMounts, job.IsPathwaysJob, job.Pathways.ColocatedPythonSidecarImage != ""); err != nil {
		return err
	}
	if err := g.validateTelemetryFlags(job); err != nil {
		return err
	}
	if job.IsPathwaysJob && job.Pathways.RunSPSWorkload == "" {
		if _, _, _, err := resolvePathwaysImages(job.Pathways); err != nil {
			return err
		}
	}
	if err := k8s.ValidateNonNegativeInt32("--restarts", job.MaxRestarts); err != nil {
		return err
	}
	return k8s.ValidateCustomWorkloadTemplates(g.gkeCustomTemplatesPath, g.resolveWorkloadTemplateName(job))
}

func (g *GKEOrchestrator) finishJobSubmission(job orchestrator.JobDefinition) error {
	if job.DryRunManifest != "" {
		return nil
	}
	g.printConsoleLinks(job)
	if job.AwaitJobCompletion {
		return g.awaitJobCompletion(job.WorkloadName, job.ClusterName, job.ClusterLocation, job.ProjectID, job.Timeout, strings.TrimSpace(job.KueueQueueName) != "")
	}
	return nil
}

// ListJobs retrieves a list of jobs in the GKE cluster.
// It filters jobs based on the provided ListOptions.
func (g *GKEOrchestrator) ListJobs(opts orchestrator.ListOptions) ([]orchestrator.JobStatus, error) {
	g.namespace = opts.GKENamespace
	logging.Info("Listing jobs in cluster '%s'...", opts.ClusterName)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return nil, err
	}

	if _, err := g.getDynamicClient(); err != nil {
		return nil, err
	}

	ns, err := g.getCurrentNamespace(opts.ClusterName, opts.ClusterLocation, opts.ProjectID)
	if err != nil {
		return nil, err
	}

	list, err := g.listWorkloadsWithFallback(ns)
	if err != nil {
		return nil, err
	}

	var filteredJobs []orchestrator.JobStatus
	for _, job := range list {
		if opts.NameContains != "" && !strings.Contains(job.Name, opts.NameContains) {
			continue
		}

		if opts.Status != "" && !strings.EqualFold(job.Status, opts.Status) {
			continue
		}

		filteredJobs = append(filteredJobs, job)
	}

	return filteredJobs, nil
}

func (g *GKEOrchestrator) listWorkloadsWithFallback(ns string) ([]orchestrator.JobStatus, error) {
	if !g.jobSetEnabled() {
		return g.listStandaloneBatchJobs(ns)
	}
	list, err := g.kubeClient.ListJobSets(ns, workloadLabel)
	if err != nil {
		if !g.isJobSetAPIAbsent(err) {
			return nil, fmt.Errorf("failed to list jobsets in namespace %s: %w", ns, err)
		}
		logging.Info("JobSet API unavailable in namespace %q; listing standalone batch/v1 Jobs.", ns)
		return g.listStandaloneBatchJobs(ns)
	}
	batchJobs, batchErr := g.listStandaloneBatchJobs(ns)
	if batchErr != nil {
		if k8s.IsForbiddenError(batchErr) {
			logging.Info("Skipping standalone batch/v1 Job listing in namespace %q due to RBAC permissions: %v", ns, batchErr)
		} else {
			logging.Warn("Unable to list standalone batch/v1 Jobs in namespace %q: %v", ns, batchErr)
		}
		return list, nil
	}
	seen := make(map[string]bool, len(list))
	for _, j := range list {
		seen[j.Name] = true
	}
	for _, bj := range batchJobs {
		if !seen[bj.Name] {
			list = append(list, bj)
		}
	}
	return list, nil
}

func (g *GKEOrchestrator) listStandaloneBatchJobs(ns string) ([]orchestrator.JobStatus, error) {
	items, err := g.kubeClient.ListResources(jobGVR, ns, workloadLabel+",!jobset.sigs.k8s.io/jobset-name", "")
	if err != nil {
		return nil, fmt.Errorf("failed to list batch/v1 jobs in namespace %s: %w", ns, err)
	}
	var jobs []orchestrator.JobStatus
	for i := range items {
		item := items[i]
		if isJobSetOwnedJob(&item) || item.GetLabels()[workloadLabel] != item.GetName() {
			continue
		}
		statusStr, completionTime := parseBatchJobStatus(item.Object)
		jobs = append(jobs, orchestrator.JobStatus{
			Name:           item.GetName(),
			Status:         statusStr,
			CreationTime:   item.GetCreationTimestamp().Format(time.RFC3339),
			CompletionTime: completionTime,
		})
	}
	return jobs, nil
}

func isJobSetOwnedJob(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	if _, hasJS := obj.GetLabels()["jobset.sigs.k8s.io/jobset-name"]; hasJS {
		return true
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind == "JobSet" {
			return true
		}
	}
	return false
}

func parseBatchJobStatus(obj map[string]interface{}) (string, string) {
	statusStr := "Unknown"
	completionTime := ""

	statusMap, hasStatus := obj["status"].(map[string]interface{})
	if hasStatus {
		if compTime, ok := statusMap["completionTime"].(string); ok && compTime != "" {
			completionTime = compTime
		}
		if conditions, ok := statusMap["conditions"].([]interface{}); ok {
			parseConditions(conditions, &statusStr, &completionTime)
		}
	}
	if statusStr != "Unknown" {
		return statusStr, completionTime
	}
	if specMap, ok := obj["spec"].(map[string]interface{}); ok {
		if suspend, ok := specMap["suspend"].(bool); ok && suspend {
			return "Suspended", completionTime
		}
	}
	if !hasStatus {
		return "Pending", completionTime
	}
	return parseBatchJobActiveCounters(obj, statusMap, completionTime)
}

func parseBatchJobActiveCounters(obj, statusMap map[string]interface{}, completionTime string) (string, string) {
	active, _, _ := unstructured.NestedInt64(statusMap, "active")
	if active > 0 {
		return "Running", completionTime
	}
	succeeded, _, _ := unstructured.NestedInt64(statusMap, "succeeded")
	completions, hasCompletions, _ := unstructured.NestedInt64(obj, "spec", "completions")
	if succeeded > 0 && (!hasCompletions || completions <= 1 || succeeded >= completions) {
		return "Succeeded", completionTime
	}
	if failed, _, _ := unstructured.NestedInt64(statusMap, "failed"); failed > 0 {
		return "Failed", completionTime
	}
	if succeeded > 0 {
		return "Running", completionTime
	}
	return "Pending", completionTime
}

// GetJobLogs fetches the logs for a specific job in the GKE cluster.
func (g *GKEOrchestrator) GetJobLogs(name string, opts orchestrator.LogsOptions) (string, error) {
	g.namespace = opts.GKENamespace
	logging.Info("Fetching logs for job '%s' in cluster '%s'...", name, opts.ClusterName)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return "", err
	}

	foundNamespace, err := g.getCurrentNamespace(opts.ClusterName, opts.ClusterLocation, opts.ProjectID)
	if err != nil {
		return "", err
	}

	podCount, streamCount, jsInfo, err := g.waitForJobPods(foundNamespace, name)
	if err != nil {
		return "", err
	}
	if podCount == 0 {
		return "Job exists but has no live logs available (it may have finished or failed to start pods)", nil
	}

	// Pods appear in batches (and after the leader with InOrder startup), so size decisions on the spec.
	jsInfo = g.ensureJobSetInfo(foundNamespace, name, jsInfo)
	livePods := podCount
	podCount = max(podCount, jsInfo.expectedPods)
	streamCount = scaleExpectedStreams(livePods, streamCount, podCount, jsInfo.expectedStreams)
	if opts.MainOnly == nil && podCount > 5 {
		g.logMainOnlyDefault(name, foundNamespace, opts, podCount, streamCount, jsInfo.isBatchJob)
	}
	selector, mainOnly := g.resolveLogsSelectorForWorkload(name, opts.MainOnly, podCount, jsInfo.firstJob, jsInfo.isBatchJob)
	if podCount > maxLogRequests && !mainOnly {
		return "", g.maxLogRequestsError(name, foundNamespace, opts, podCount, streamCount, jsInfo.isBatchJob)
	}

	allContainers := !mainOnly
	if opts.Follow {
		return "", g.followJobLogs(foundNamespace, name, selector, allContainers, streamCount, jsInfo.isBatchJob)
	}
	res, err := g.fetchLogsWithRetryForWorkload(foundNamespace, name, selector, allContainers, streamCount, -1, jsInfo.isBatchJob)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Stdout) == "" {
		if schedMsg := g.checkUnscheduledPods(foundNamespace, name, jsInfo.isBatchJob); schedMsg != "" {
			return fmt.Sprintf("Job pods are waiting to be scheduled (no container logs available yet):\n%s", schedMsg), nil
		}
		return "Job exists but has no live logs available (it may have finished or failed to start pods)", nil
	}
	return res.Stdout, nil
}

func (g *GKEOrchestrator) followJobLogs(ns, name, selector string, allContainers bool, streamCount int, isBatchJob bool) error {
	logging.Info("Streaming logs for job '%s'...", name)
	res, err := g.fetchLogsWithRetryForWorkload(ns, name, selector, allContainers, streamCount, 1, isBatchJob)
	if err != nil {
		return err
	}
	if strings.TrimSpace(res.Stdout) == "" {
		if schedMsg := g.checkUnscheduledPods(ns, name, isBatchJob); schedMsg != "" {
			logging.Warn("Job pods are waiting to be scheduled (no container logs available yet):\n%s", schedMsg)
		}
	}
	if _, fresh, err := g.getJobPodStreams(ns, selector); err == nil {
		streamCount = max(streamCount, fresh) // pods created while the probe waited
	}
	args := g.buildKubectlLogsArgs(ns, selector, allContainers, true, streamCount, -1)
	return g.kubectlStream(args...)
}

// buildKubectlLogsArgs leaves the container to kubectl unless allContainers is set. kubectl (1.24+) then reads the
// pod's kubectl.kubernetes.io/default-container, else spec.containers[0]; native sidecars are never picked.
func (g *GKEOrchestrator) buildKubectlLogsArgs(ns, selector string, allContainers, follow bool, streamCount, tail int) []string {
	args := []string{"logs", "-n", ns, "-l", selector}
	if allContainers {
		args = append(args, "--all-containers")
	}
	if follow {
		args = append(args, "-f")
	}
	maxReqs := maxLogRequests
	if allContainers && streamCount > maxReqs {
		maxReqs = streamCount
	}
	args = append(args, fmt.Sprintf("--max-log-requests=%d", maxReqs), fmt.Sprintf("--tail=%d", tail))
	return args
}

func (g *GKEOrchestrator) fetchLogsWithRetryForWorkload(ns, jobsetName, selector string, allContainers bool, streamCount, tail int, isBatchJob bool) (shell.CommandResult, error) {
	var res shell.CommandResult
	cmdArgs := g.buildKubectlLogsArgs(ns, selector, allContainers, false, streamCount, tail)
	for i := 0; i < logsMaxAttempts; i++ {
		if i > 0 {
			time.Sleep(logsRetryInterval)
		}
		res = g.kubectl(cmdArgs...)
		if res.ExitCode == 0 {
			return res, nil
		}

		if strings.Contains(res.Stderr, "is waiting to start") {
			if i == 0 {
				if schedMsg := g.checkUnscheduledPods(ns, jobsetName, isBatchJob); schedMsg != "" {
					logging.Warn("Job pods are waiting to be scheduled:\n%s", schedMsg)
				} else {
					logging.Info("Job containers are waiting to start (likely pulling images). Waiting...")
				}
			}
			continue
		}

		return res, fmt.Errorf("failed to get logs: %s\n%s", res.Stderr, res.Stdout)
	}

	if schedMsg := g.checkUnscheduledPods(ns, jobsetName, isBatchJob); schedMsg != "" {
		return res, fmt.Errorf("timed out waiting for job to start; unscheduled pods:\n%s\nlatest error: %s\n%s", schedMsg, res.Stderr, res.Stdout)
	}
	if warnEvents := g.checkWorkloadWarningEvents(ns, jobsetName, isBatchJob); warnEvents != "" {
		return res, fmt.Errorf("timed out waiting for job to start; %s reported warning events:\n%s\nlatest error: %s\n%s", workloadKindLabel(isBatchJob), warnEvents, res.Stderr, res.Stdout)
	}
	return res, fmt.Errorf("timed out waiting for job to start; latest error: %s\n%s", res.Stderr, res.Stdout)
}

func (g *GKEOrchestrator) checkUnscheduledPods(ns, jobsetName string, isBatchJob bool) string {
	podSelector := workloadPodSelector(jobsetName, isBatchJob)
	res := g.kubectl("get", "pods", "-n", ns, "-l", podSelector, "-o", "jsonpath="+unscheduledPodsJSONPath)
	if res.ExitCode != 0 {
		return ""
	}
	var unscheduled []string
	jobPods := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		podName := parts[0]
		scheduledStatus := parts[2]
		if scheduledStatus == "False" {
			jobPods["pod/"+podName] = true
			var reason, msg string
			if len(parts) > 3 {
				reason = parts[3]
			}
			if len(parts) > 4 {
				msg = parts[4]
			}
			if reason != "" || msg != "" {
				unscheduled = append(unscheduled, fmt.Sprintf("%s (%s: %s)", podName, reason, msg))
			} else {
				unscheduled = append(unscheduled, fmt.Sprintf("%s (PodScheduled=False)", podName))
			}
		}
	}
	if len(unscheduled) == 0 {
		return ""
	}
	if podEvents := g.checkPodWarningEvents(ns, jobPods); podEvents != "" {
		unscheduled = append(unscheduled, podEvents)
	}
	return strings.Join(unscheduled, "\n")
}

// waitForJobPods polls until the JobSet (or fallback batch/v1 Job) has at least one pod, or returns 0 pods if already finished.
// If it queries the workload while waiting, it returns the parsed jobSetInfo so the caller does not re-query it.
func (g *GKEOrchestrator) waitForJobPods(ns, jobsetName string) (int, int, jobSetInfo, error) {
	info := jobSetInfo{isBatchJob: !g.jobSetEnabled()}
	selector := workloadPodSelector(jobsetName, info.isBatchJob)
	for i := 0; i < logsMaxAttempts; i++ {
		if i > 0 {
			time.Sleep(logsRetryInterval)
		}
		pods, streams, done, err := g.pollJobPodsOnce(ns, jobsetName, &selector, &info)
		if err != nil || done {
			return pods, streams, info, err
		}
		if i == 0 {
			logging.Info("Waiting for job '%s' pods to be created...", jobsetName)
		}
	}
	if warnEvents := g.checkWorkloadWarningEvents(ns, jobsetName, info.isBatchJob); warnEvents != "" {
		return 0, 0, jobSetInfo{}, fmt.Errorf("timed out waiting for job to start; %s reported warning events:\n%s", workloadKindLabel(info.isBatchJob), warnEvents)
	}
	if info.suspended {
		return 0, 0, jobSetInfo{}, fmt.Errorf("timed out waiting for job '%s' to start: job is still queued (%s suspended, waiting for Kueue admission); re-run once admitted or check gcluster job inspect", jobsetName, workloadKindLabel(info.isBatchJob))
	}
	return 0, 0, jobSetInfo{}, fmt.Errorf("timed out waiting for job to start: no pods created for job '%s'", jobsetName)
}

func workloadKindLabel(isBatchJob bool) string {
	if isBatchJob {
		return "Job"
	}
	return "JobSet"
}

func workloadPodSelector(name string, isBatchJob bool) string {
	if isBatchJob {
		return fmt.Sprintf("%s=%s,batch.kubernetes.io/job-name=%s", workloadLabel, name, name)
	}
	return fmt.Sprintf("jobset.sigs.k8s.io/jobset-name=%s", name)
}

func (g *GKEOrchestrator) pollJobPodsOnce(ns, jobsetName string, selector *string, info *jobSetInfo) (int, int, bool, error) {
	pods, streams, err := g.getJobPodStreams(ns, *selector)
	if err != nil || pods > 0 {
		return pods, streams, true, err
	}
	queried, err := g.getJobSetInfo(ns, jobsetName)
	if err != nil {
		return 0, 0, true, err
	}
	*info = queried
	if info.isBatchJob {
		batchSel := workloadPodSelector(jobsetName, true)
		if *selector != batchSel {
			*selector = batchSel
			if pods, streams, err := g.getJobPodStreams(ns, *selector); err != nil || pods > 0 {
				return pods, streams, true, err
			}
		}
	}
	return 0, 0, info.terminal(), nil
}

func (g *GKEOrchestrator) getJobSetInfo(ns, jobsetName string) (jobSetInfo, error) {
	if !g.jobSetEnabled() {
		return g.getBatchJobInfo(ns, jobsetName)
	}
	res := g.kubectl("get", "jobsets.jobset.x-k8s.io", jobsetName, "-n", ns, "-o", "jsonpath="+jobSetInfoJSONPath, kubectlReadTimeout)
	if res.ExitCode != 0 {
		if rawErr := kubectlOutputError(res); rawErr != nil && g.isJobSetAPIAbsent(rawErr) {
			return g.getBatchJobInfo(ns, jobsetName)
		}
		return jobSetInfo{}, kubectlCommandError(fmt.Sprintf("failed to read status of job '%s'", jobsetName), res)
	}
	info := parseJobSetInfo(res.Stdout)
	info.queried = true
	return info, nil
}

func (g *GKEOrchestrator) getBatchJobInfo(ns, name string) (jobSetInfo, error) {
	res := g.kubectl("get", "job", name, "-n", ns, "-o", "json", kubectlReadTimeout)
	if res.ExitCode != 0 {
		if rawErr := kubectlOutputError(res); rawErr != nil && k8s.IsNotFoundOrNoMatchError(rawErr) {
			return jobSetInfo{}, fmt.Errorf("job '%s' not found in namespace '%s' (run 'gcluster job list' or check --gke-namespace)", name, ns)
		}
		return jobSetInfo{}, kubectlCommandError(fmt.Sprintf("failed to read status of job '%s'", name), res)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return jobSetInfo{}, fmt.Errorf("job '%s' not found in namespace '%s' (run 'gcluster job list' or check --gke-namespace)", name, ns)
	}
	var jobObj batchv1.Job
	if err := json.Unmarshal([]byte(res.Stdout), &jobObj); err != nil {
		return jobSetInfo{}, fmt.Errorf("failed to parse batch/v1 Job '%s': %w", name, err)
	}
	if jobObj.Labels[workloadLabel] != name {
		return jobSetInfo{}, fmt.Errorf("job '%s' not found in namespace '%s' (run 'gcluster job list' or check --gke-namespace)", name, ns)
	}
	return parseBatchJobInfo(jobObj), nil
}

func parseBatchJobInfo(jobObj batchv1.Job) jobSetInfo {
	expectedPods := 1
	if jobObj.Spec.Parallelism != nil && *jobObj.Spec.Parallelism >= 0 {
		expectedPods = int(*jobObj.Spec.Parallelism)
	} else if jobObj.Spec.Completions != nil && *jobObj.Spec.Completions >= 0 {
		expectedPods = int(*jobObj.Spec.Completions)
	}
	containers := len(jobObj.Spec.Template.Spec.InitContainers) + len(jobObj.Spec.Template.Spec.Containers)
	info := jobSetInfo{
		queried:         true,
		isBatchJob:      true,
		suspended:       jobObj.Spec.Suspend != nil && *jobObj.Spec.Suspend,
		expectedPods:    expectedPods,
		expectedStreams: expectedPods * containers,
	}
	for _, c := range jobObj.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case batchv1.JobComplete, "Succeeded":
			info.terminalState = "Completed"
		case batchv1.JobFailed:
			info.terminalState = "Failed"
		case batchv1.JobSuspended:
			info.suspended = true
		}
	}
	return info
}

func (g *GKEOrchestrator) ensureJobSetInfo(ns, jobsetName string, info jobSetInfo) jobSetInfo {
	if info.queried {
		return info
	}
	queried, err := g.getJobSetInfo(ns, jobsetName)
	if err != nil {
		logging.Warn("Failed to query JobSet: %v", err)
		return info
	}
	return queried
}

// parseJobSetInfo parses the output of jobSetInfoJSONPath. Each line after the tab-separated header is one
// replicated job; a missing replicas or parallelism field defaults to 1 (the API default), so a blank line
// still counts as one pod, while an explicit 0 counts as zero.
func parseJobSetInfo(stdout string) jobSetInfo {
	stdout = strings.ReplaceAll(stdout, "\r\n", "\n")
	header, rest, _ := strings.Cut(stdout, "\n")
	var info jobSetInfo
	if parts := strings.Split(header, "\t"); len(parts) == 3 {
		info.terminalState = strings.TrimSpace(parts[0])
		info.suspended = strings.Contains(parts[1], "True")
		info.firstJob = strings.TrimSpace(parts[2])
	}
	rest = strings.TrimRight(rest, "\n")
	if rest == "" {
		return info
	}
	for _, line := range strings.Split(rest, "\n") {
		countsPart, containersPart, _ := strings.Cut(line, "\t")
		pods := 1
		for _, f := range strings.Fields(countsPart) {
			if n, err := strconv.Atoi(f); err == nil && n >= 0 {
				pods *= n
			}
		}
		info.expectedPods += pods
		if containers := len(strings.Fields(containersPart)); containers > 0 {
			info.expectedStreams += pods * containers
		}
	}
	return info
}

func (g *GKEOrchestrator) getJobPodStreams(ns, selector string) (int, int, error) {
	res := g.kubectl("get", "pods", "-n", ns, "-l", selector, "-o", "jsonpath="+podContainersJSONPath)
	if res.ExitCode != 0 {
		return 0, 0, fmt.Errorf("failed to query pods: %s", res.Stderr)
	}
	stdout := strings.TrimSpace(res.Stdout)
	if stdout == "" {
		return 0, 0, nil
	}
	pods, streams := 0, 0
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pods++
		streams += len(fields)
	}
	return pods, streams, nil
}

// resolveLogsSelectorForWorkload returns the pod selector and whether logs are main-only. Main-only selects rank-0 of the
// first replicated job (for JobSets) or rank-0 completion index (for fallback batch/v1 Indexed Jobs).
func (g *GKEOrchestrator) resolveLogsSelectorForWorkload(name string, optsMainOnly *bool, podCount int, firstJob string, isBatchJob bool) (string, bool) {
	mainOnly := podCount > 5
	if optsMainOnly != nil {
		mainOnly = *optsMainOnly
	}
	selector := workloadPodSelector(name, isBatchJob)
	if isBatchJob {
		if mainOnly {
			return selector + ",batch.kubernetes.io/job-completion-index=0", true
		}
		return selector, false
	}

	if mainOnly {
		if firstJob != "" {
			selector += ",jobset.sigs.k8s.io/replicatedjob-name=" + firstJob
		} else {
			logging.Info("Falling back to the rank-0 pod of every replicated job in '%s'.", name)
		}
		selector = fmt.Sprintf("%s,jobset.sigs.k8s.io/job-index=0,batch.kubernetes.io/job-completion-index=0", selector)
	}

	return selector, mainOnly
}

func (g *GKEOrchestrator) generateAndSubmitManifests(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) error {
	return g.submitWorkloadTemplate(g.resolveWorkloadTemplateName(job), job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
}

// submitWorkloadTemplate renders and applies the manifest for tmplName. Every workload template has an explicit
// case; an unknown name is an internal error rather than a silent fallback to the JobSet path.
func (g *GKEOrchestrator) submitWorkloadTemplate(tmplName string, job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) error {
	switch tmplName {
	case k8s.PathwaysJobSetTemplate:
		manifestContent, err := g.GeneratePathwaysManifest(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
		if err != nil {
			return err
		}
		return g.ApplyManifest(manifestContent, job.DryRunManifest, job.WorkloadName)
	case k8s.JobSetTemplate:
		manifestOpts, err := g.PrepareManifestOptions(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
		if err != nil {
			return err
		}
		return g.generateAndApplyManifest(manifestOpts, profile, job.DryRunManifest)
	case k8s.JobTemplate:
		manifestOpts, err := g.PrepareManifestOptions(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
		if err != nil {
			return err
		}
		return g.generateAndApplyJobManifest(manifestOpts, profile, job.DryRunManifest, job.ClusterName, job.ProjectID)
	default:
		return fmt.Errorf("internal error: no manifest generator is wired for workload template %q", tmplName)
	}
}

func getCloudConsoleLogsURL(projectID, location, clusterName, namespace, podNamePrefix string) string {
	query := fmt.Sprintf(`resource.type="k8s_container"
resource.labels.project_id="%s"
resource.labels.location="%s"
resource.labels.cluster_name="%s"
resource.labels.namespace_name="%s"
resource.labels.pod_name:"%s-"
severity>=DEFAULT`, projectID, location, clusterName, namespace, podNamePrefix)

	encodedFilter := url.QueryEscape(query)
	return fmt.Sprintf("https://console.cloud.google.com/logs/query;query=%s;storageScope=project;duration=P1D?project=%s",
		encodedFilter, projectID)
}

func (g *GKEOrchestrator) k8sJobCLIFlags(clusterName, ns string) string {
	var parts []string
	if c := cmp.Or(strings.TrimSpace(clusterName), strings.TrimSpace(g.k8sCluster), strings.TrimSpace(g.kubeContext)); c != "" {
		parts = append(parts, "--cluster", c)
	}
	if n := cmp.Or(strings.TrimSpace(ns), strings.TrimSpace(g.namespace)); n != "" {
		parts = append(parts, "--gke-namespace", n)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func (g *GKEOrchestrator) printConsoleLinks(job orchestrator.JobDefinition) {
	if g.mode == orchestrator.TypeK8s {
		flags := g.k8sJobCLIFlags(job.ClusterName, job.GKENamespace)
		logging.Info("Workload '%s' submitted. Run 'gcluster job logs %s%s' to stream logs or 'gcluster job inspect --name %s%s' for diagnostics.",
			job.WorkloadName, job.WorkloadName, flags, job.WorkloadName, flags)
		return
	}
	jobName := job.WorkloadName + "-main-job-0"
	if !g.jobSetEnabled() {
		jobName = job.WorkloadName
	} else if job.IsPathwaysJob {
		jobName = job.WorkloadName + "-pathways-head-0"
	}

	ns, err := g.getCurrentNamespace(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		// Non-critical path (printing informational links).
		// Fallback to 'default' to avoid failing the command output if namespace cannot be determined.
		ns = "default"
	}

	gkeLink := fmt.Sprintf("https://console.cloud.google.com/kubernetes/job/%s/%s/%s/%s/details?project=%s",
		job.ClusterLocation, job.ClusterName, ns, jobName, job.ProjectID)

	logging.Info("Follow your workload details here: %s", gkeLink)

	logsLink := getCloudConsoleLogsURL(job.ProjectID, job.ClusterLocation, job.ClusterName, ns, jobName)
	logging.Info("View your workload logs in real-time here: %s or use gcluster job logs [job-name] to view logs using kubectl", logsLink)

	if job.Pathways.DeploySPS {
		logging.Info("SPS instance %s submitted. Endpoint: %s", job.WorkloadName, fmt.Sprintf(spsEndpointFormat, job.WorkloadName))
	}
}

func (g *GKEOrchestrator) validateJobConflicts(job orchestrator.JobDefinition) error {
	if job.DryRunManifest != "" {
		return nil
	}
	status, err := g.getJobStatus(job.WorkloadName)
	if err != nil {
		return err
	}
	if status != "" {
		ns := cmp.Or(strings.TrimSpace(g.namespace), strings.TrimSpace(job.GKENamespace))
		hint := g.formatCancelHintInNamespace(job.WorkloadName, job.ClusterName, job.ClusterLocation, job.ProjectID, ns)
		if status == orphanedTier3ServiceStatus {
			return fmt.Errorf("a leftover headless Service named '%s' from a previous job already exists in namespace '%s'. You can remove it using '%s' or resubmit this workload with a different name using '--name'", job.WorkloadName, ns, hint)
		}
		return fmt.Errorf("job with name '%s' already exists in state '%s'. You can cancel the existing job using '%s' or resubmit this workload with a different name using '--name'", job.WorkloadName, status, hint)
	}
	return nil
}

// ApplyManifest validates the workload manifest and either saves it to outputManifestPath or applies it to the
// cluster in g.namespace. SubmitJob resolves g.namespace (configureClusterEnvironment -> getCurrentNamespace) before
// reaching this point; when it is empty, kubectl applies into the pinned context's namespace, which is the same value
// getCurrentNamespace would resolve.
func (g *GKEOrchestrator) ApplyManifest(manifestContent, outputManifestPath, workloadName string) error {
	if err := ValidateJobSetManifest(manifestContent); err != nil {
		return err
	}
	if outputManifestPath != "" {
		logging.Info("Saving GKE manifest to %s", outputManifestPath)
		if err := os.WriteFile(outputManifestPath, []byte(manifestContent), 0644); err != nil {
			return fmt.Errorf("failed to write GKE manifest to file %s: %w", outputManifestPath, err)
		}
		logging.Info("GKE manifest saved successfully.")
	} else {
		// Submit will fail if a job with the same name already exists. The embedded workload templates render
		// metadata.namespace from the same resolved namespace, but a custom template extracted before that
		// existed does not, so pass -n as well: kubectl then places unnamespaced objects in the resolved
		// namespace and rejects any object that declares a different one, instead of silently using the
		// kubeconfig default.
		logging.Info("Applying GKE manifest to cluster...")
		err := g.applyManifests([]byte(manifestContent), workloadName+".yaml", g.namespace)
		if err != nil {
			return fmt.Errorf("failed to apply GKE manifest: %w", err)
		}
		if err := g.verifyStorageGateways(manifestContent, workloadName); err != nil {
			return err
		}
		logging.Info("GKE workload deployed successfully.")
	}
	return nil
}

func (g *GKEOrchestrator) populateClusterMetadata(job *orchestrator.JobDefinition) error {

	if job.IsPathwaysJob {
		if job.Pathways.HeadNodePool != "" {
			g.resolvedHeadNodePool = job.Pathways.HeadNodePool
		} else {
			g.resolvedHeadNodePool = g.autoDetectCPUNodePool()
		}
		if g.resolvedHeadNodePool == "" {
			return fmt.Errorf("failed to auto-detect a suitable CPU node pool (expected 'cpu-np' or 'pathways-np') for Pathways head job. You can explicitly specify your head node pool name using the --pathways-head-np flag")
		}
		if err := g.validatePathwaysHeadNodePool(g.resolvedHeadNodePool, job.ClusterName); err != nil {
			return err
		}
		job.Pathways.HeadNodePool = g.resolvedHeadNodePool
	}

	capacity, nodePoolSAs, err := g.calculateClusterCapacity(g.clusterDesc, job.ClusterLocation)
	if err != nil {
		return err
	}
	g.capacity = capacity
	g.nodePoolSAs = nodePoolSAs
	logging.Info("Calculated cluster capacity: %+v", g.capacity)

	return nil
}

func (g *GKEOrchestrator) autoDetectCPUNodePool() string {
	var fallback string
	for _, np := range g.clusterDesc.NodePools {
		if (np.Name == "cpu-np" || np.Name == "pathways-np") && len(np.Config.Accelerators) == 0 && !config.IsTPU(np.Config.MachineType) && !g.isSystemPool(np) {
			if g.getNodeCount(np) > 0 {
				return np.Name
			}
			if fallback == "" {
				fallback = np.Name
			}
		}
	}
	return fallback
}

func (g *GKEOrchestrator) validatePathwaysHeadNodePool(poolName, clusterName string) error {
	if len(g.clusterDesc.NodePools) == 0 {
		return nil
	}
	var availablePools []string
	for _, np := range g.clusterDesc.NodePools {
		if g.isSystemPool(np) {
			if np.Name == poolName {
				return fmt.Errorf("Pathways head node pool '%s' in cluster '%s' is a GKE system node pool. Specify a non-system CPU node pool using --pathways-head-np", poolName, clusterName)
			}
			continue
		}
		if len(np.Config.Accelerators) == 0 && !config.IsTPU(np.Config.MachineType) && g.getNodeCount(np) > 0 {
			availablePools = append(availablePools, np.Name)
		}
		if np.Name == poolName {
			if len(np.Config.Accelerators) > 0 || config.IsTPU(np.Config.MachineType) {
				return fmt.Errorf("Pathways head node pool '%s' in cluster '%s' is not a CPU node pool. The Pathways head pod must run on a CPU-only node pool", poolName, clusterName)
			}
			if g.getNodeCount(np) == 0 {
				return fmt.Errorf("Pathways head node pool '%s' in cluster '%s' has 0 nodes and autoscaling is disabled (or max node count is 0), so the Pathways head pod cannot be scheduled. Enable autoscaling or resize node pool '%s', or specify a different node pool with --pathways-head-np", poolName, clusterName, poolName)
			}
			return nil
		}
	}
	if len(availablePools) > 0 {
		return fmt.Errorf("Pathways head node pool '%s' not found in cluster '%s' (available node pools: %s). Specify an existing CPU node pool using --pathways-head-np", poolName, clusterName, strings.Join(availablePools, ", "))
	}
	return fmt.Errorf("Pathways head node pool '%s' not found in cluster '%s'. Specify an existing CPU node pool using --pathways-head-np", poolName, clusterName)
}

func (g *GKEOrchestrator) isSystemPool(np gkeJobNodePool) bool {
	for _, taint := range np.Config.Taints {
		if taint.Key == k8s.GKESystemTaintKey && taint.Value == "true" {
			return true
		}
	}
	return false
}

func (g *GKEOrchestrator) initializeJobSubmission(job *orchestrator.JobDefinition) error {
	if err := g.validateMTCJobFlags(job); err != nil {
		return err
	}
	if g.mode == orchestrator.TypeK8s {
		return g.initializeK8sJobSubmission(job)
	}
	if err := g.populateClusterMetadata(job); err != nil {
		return err
	}

	logging.Info("Configuring kubectl for GKE cluster '%s'...", job.ClusterName)
	if err := g.configureKubectl(job.ClusterName, job.ClusterLocation, job.ProjectID); err != nil {
		return err
	}

	if job.DryRunManifest == "" {
		if err := g.ValidateClusterState(job); err != nil {
			return err
		}
	} else if g.isEnvTruthy(disableJobSetEnvVar) || g.isEnvTruthy(disableKueueEnvVar) || g.isAirGapped() {
		if err := g.resolveClusterAddons(job, true); err != nil {
			return err
		}
	}
	if err := g.validateTier3Flags(job); err != nil {
		return err
	}
	if err := g.validateResolvedCustomTemplate(*job); err != nil {
		return err
	}

	return g.configureClusterEnvironment(job)
}

func (g *GKEOrchestrator) initializeK8sJobSubmission(job *orchestrator.JobDefinition) error {
	logging.Info("Configuring kubectl for Kubernetes cluster '%s'...", job.ClusterName)
	if err := g.configureKubectl(job.ClusterName, job.ClusterLocation, job.ProjectID); err != nil {
		return err
	}
	if _, err := g.DiscoverK8sInventory(); err != nil {
		return err
	}

	if job.DryRunManifest == "" {
		if err := g.ValidateClusterState(job); err != nil {
			return err
		}
	} else {
		if err := g.resolveClusterAddons(job, true); err != nil {
			return err
		}
	}
	if err := g.validateTier3Flags(job); err != nil {
		return err
	}
	if err := g.validateResolvedCustomTemplate(*job); err != nil {
		return err
	}

	return g.configureClusterEnvironment(job)
}

func (g *GKEOrchestrator) calculateClusterCapacity(clusterDesc gkeCluster, location string) (ClusterCapacity, []string, error) {
	var totalCPUs int
	var totalMemoryMb int
	var totalGPUs int
	var totalTPUs int
	var nodePoolSAs []string
	flavors := make(map[string]FlavorCapacity)

	g.machineTypeToThreadsPerCore = make(map[string]string)
	for _, np := range clusterDesc.NodePools {
		if g.isSystemPool(np) {
			continue
		}
		if np.Config.AdvancedMachineFeatures != nil {
			g.machineTypeToThreadsPerCore[np.Config.MachineType] = np.Config.AdvancedMachineFeatures.ThreadsPerCore
		}
		cpus, memMb, gpus, tpus, flavor, nodeLabels, sa, err := g.processNodePoolCapacity(np, location)
		if err != nil {
			return ClusterCapacity{}, nil, fmt.Errorf("failed to determine capacity for node pool '%s': %w. This may cause job scheduling to fail due to inaccurate cluster capacity calculations. Please verify that the cluster is accessible and that you have correct permissions.", np.Name, err)
		}

		totalCPUs += cpus
		totalMemoryMb += memMb
		totalGPUs += gpus
		totalTPUs += tpus
		if sa != "" {
			nodePoolSAs = append(nodePoolSAs, sa)
		}

		fc := flavors[flavor]
		fc.CPUs += cpus
		fc.MemoryGi += memMb / 1024
		fc.GPUs += gpus
		fc.TPUs += tpus
		fc.NodeLabels = nodeLabels
		flavors[flavor] = fc
	}

	if err := g.populateNAPFlavors(flavors); err != nil {
		return ClusterCapacity{}, nil, fmt.Errorf("failed to populate GKE Node Auto-Provisioning (NAP) flavors: %w", err)
	}

	capacity := ClusterCapacity{
		CPUs:     totalCPUs,
		MemoryGi: totalMemoryMb / 1024,
		GPUs:     totalGPUs,
		TPUs:     totalTPUs,
		Flavors:  flavors,
	}
	return capacity, nodePoolSAs, nil
}

// isSharedCoreMachineType mirrors local.machine_shared_core in
// modules/compute/gke-node-pool/threads_per_core_calc.tf: shared-core machine
// types (e.g. e2-micro, e2-small, e2-medium, f1-micro, g1-small) have two
// dash-separated parts (<family>-<tier>) and only expose 1 allocatable vCPU in
// GKE even when guestCpus is 2.
func isSharedCoreMachineType(machineType string) bool {
	trimmed := strings.TrimSpace(machineType)
	if trimmed == "" {
		return false
	}
	return len(strings.Split(trimmed, "-")) <= 2
}

func (g *GKEOrchestrator) getThreadsPerCore(machineType string) string {
	if tpc, ok := g.machineTypeToThreadsPerCore[machineType]; ok {
		return tpc
	}
	for _, np := range g.clusterDesc.NodePools {
		if g.isSystemPool(np) {
			continue
		}
		if strings.EqualFold(np.Config.MachineType, machineType) && np.Config.AdvancedMachineFeatures != nil {
			return np.Config.AdvancedMachineFeatures.ThreadsPerCore
		}
	}
	return ""
}

func (g *GKEOrchestrator) getEffectiveCPUs(machineType string, guestCpus int) int {
	if guestCpus <= 0 {
		return 0
	}
	if isSharedCoreMachineType(machineType) {
		return 1
	}
	if g.getThreadsPerCore(machineType) == "1" && !strings.HasPrefix(machineType, "t2a") && !strings.HasPrefix(machineType, "t2d") {
		for k, cap := range g.machineCapCache {
			if strings.HasPrefix(k, machineType+":") && cap.GuestCpus > 0 && guestCpus == cap.GuestCpus/2 && cap.GuestCpus != guestCpus {
				return guestCpus
			}
		}
		return guestCpus / 2
	}
	return guestCpus
}

func (g *GKEOrchestrator) processNodePoolCapacity(np gkeJobNodePool, location string) (cpus, memMb, gpus, tpus int, flavor string, nodeLabels map[string]string, sa string, err error) {
	sa = strings.TrimSpace(np.Config.ServiceAccount)
	if sa == "default" {
		sa = ""
	}

	nodeCount := g.getNodeCount(np)
	nodeLabels = make(map[string]string)

	if nodeCount == 0 {
		return 0, 0, 0, 0, "flavor-default", nodeLabels, sa, nil
	}

	cap, err := g.FetchMachineCapabilities(np.Config.MachineType, location)
	if err != nil {
		return 0, 0, 0, 0, "flavor-default", nodeLabels, sa, err
	}

	cpus = g.getEffectiveCPUs(np.Config.MachineType, cap.GuestCpus) * nodeCount
	memMb = cap.MemoryMb * nodeCount

	flavor = "flavor-default"
	if np.Name == g.resolvedHeadNodePool {
		flavor = "pathways-flavor"
	}

	accGpus, accTpus, accFlavor, accLabels, err := g.resolveAccelerators(np, cap, nodeCount)
	if err != nil {
		return 0, 0, 0, 0, "flavor-default", nodeLabels, sa, err
	}

	gpus = accGpus
	tpus = accTpus
	nodeLabels = accLabels
	if accFlavor != "flavor-default" {
		flavor = accFlavor
	}

	if flavor == "pathways-flavor" {
		nodeLabels["cloud.google.com/gke-nodepool"] = np.Name
	}

	return cpus, memMb, gpus, tpus, flavor, nodeLabels, sa, nil
}

func (g *GKEOrchestrator) resolveAccelerators(np gkeJobNodePool, cap MachineTypeCap, nodeCount int) (gpus, tpus int, flavor string, nodeLabels map[string]string, err error) {
	if len(np.Config.Accelerators) > 0 {
		gpus, tpus, flavor, nodeLabels, err = g.processAccelerators(np.Config.Accelerators, nodeCount, np.Config.MachineType)
		if err != nil {
			return 0, 0, "flavor-default", nil, fmt.Errorf("in node pool %s: %w", np.Name, err)
		}
		for _, acc := range np.Config.Accelerators {
			if g.acceleratorToMachineType == nil {
				g.acceleratorToMachineType = make(map[string]string)
			}
			g.acceleratorToMachineType[strings.ToLower(acc.AcceleratorType)] = np.Config.MachineType
		}
	} else if len(cap.Accelerators) > 0 {
		gpus, tpus, flavor, nodeLabels = g.processCapabilitiesAccelerators(cap, nodeCount)
		accType := cap.Accelerators[0].Type
		if g.acceleratorToMachineType == nil {
			g.acceleratorToMachineType = make(map[string]string)
		}
		g.acceleratorToMachineType[strings.ToLower(accType)] = np.Config.MachineType
	} else {
		return 0, 0, "flavor-default", make(map[string]string), nil
	}

	return gpus, tpus, flavor, nodeLabels, nil
}

func (g *GKEOrchestrator) processCapabilitiesAccelerators(cap MachineTypeCap, nodeCount int) (gpus, tpus int, flavor string, nodeLabels map[string]string) {
	nodeLabels = make(map[string]string)
	if len(cap.Accelerators) == 0 {
		return 0, 0, "flavor-default", nodeLabels
	}
	count := cap.Accelerators[0].Count
	accType := cap.Accelerators[0].Type
	flavor = "flavor-" + strings.ToLower(accType)
	if strings.Contains(strings.ToLower(accType), "tpu") {
		tpus = count * nodeCount
		nodeLabels["cloud.google.com/gke-tpu-accelerator"] = accType
	} else {
		gpus = count * nodeCount
		nodeLabels["cloud.google.com/gke-accelerator"] = accType
	}
	return gpus, tpus, flavor, nodeLabels
}

func (g *GKEOrchestrator) getNodeCount(np gkeJobNodePool) int {
	numZones := len(g.clusterZones)
	if numZones == 0 {
		numZones = 1
	}

	nodeCount := np.InitialNodeCount * numZones
	if np.Autoscaling.Enabled {
		var maxNodes int
		if np.Autoscaling.TotalMaxNodeCount > 0 {
			maxNodes = np.Autoscaling.TotalMaxNodeCount
		} else {
			maxNodes = np.Autoscaling.MaxNodeCount * numZones
		}

		if maxNodes > nodeCount {
			nodeCount = maxNodes
		}
	}
	return nodeCount
}

func (g *GKEOrchestrator) processAccelerators(accelerators []gkeAccelerator, nodeCount int, machineType string) (gpus, tpus int, flavor string, nodeLabels map[string]string, err error) {
	nodeLabels = make(map[string]string)
	flavor = "flavor-default"

	for _, acc := range accelerators {
		count64, err := acc.AcceleratorCount.Int64()
		if err != nil {
			return 0, 0, "flavor-default", nodeLabels, fmt.Errorf("invalid acceleratorCount %q: %w", acc.AcceleratorCount, err)
		}
		count := int(count64)
		if strings.Contains(strings.ToLower(acc.AcceleratorType), "tpu") {
			tpus += count * nodeCount
			flavor = "flavor-" + strings.ToLower(acc.AcceleratorType)
			nodeLabels["cloud.google.com/gke-tpu-accelerator"] = acc.AcceleratorType
		} else if strings.Contains(strings.ToLower(acc.AcceleratorType), "nvidia") || strings.Contains(strings.ToLower(acc.AcceleratorType), "gpu") {
			gpus += count * nodeCount
			flavor = "flavor-" + strings.ToLower(acc.AcceleratorType)
			nodeLabels["cloud.google.com/gke-accelerator"] = acc.AcceleratorType
		}
	}
	return gpus, tpus, flavor, nodeLabels, nil
}

func (g *GKEOrchestrator) queryAllMachineTypes() ([]string, error) {
	var unique []string
	uniqueMap := make(map[string]bool)
	for _, np := range g.clusterDesc.NodePools {
		if np.Config.MachineType != "" {
			uniqueMap[np.Config.MachineType] = true
		}
	}
	for k := range uniqueMap {
		unique = append(unique, k)
	}
	return unique, nil
}

func (g *GKEOrchestrator) resolveTopology(job *orchestrator.JobDefinition) (string, bool, error) {

	if g.topologyCache == nil {
		g.topologyCache = make(map[string]string)
	}

	cacheKey := job.Topology + ":" + job.MachineType
	if val, ok := g.topologyCache[cacheKey]; ok {
		isDyn, err := g.verifyDynamicSlicingActive(ManifestOptions{
			ClusterName:     job.ClusterName,
			ClusterLocation: job.ClusterLocation,
			ComputeType:     job.ComputeType,
			MachineType:     job.MachineType,
			Topology:        job.Topology,
		})
		if err != nil {
			if isDyn {
				return "", true, err
			}
			logging.Warn("Failed to verify if dynamic slicing is active: %v. Assuming not active.", err)
		}
		return val, isDyn, nil
	}

	top, dynSlicing, err := g.resolveDynamicSlicingTopology(job)
	if err != nil {
		return "", dynSlicing, err
	}
	if dynSlicing {
		g.topologyCache[cacheKey] = top
		return top, dynSlicing, nil
	}

	if job.Topology == "" {
		topo, err := config.ResolveTopologyForChips(job.ComputeType, job.MachineType)
		if err == nil {
			logging.Info("Auto-resolved topology %s for shorthand %s", topo, job.ComputeType)
			job.Topology = topo
		} else {
			if !strings.Contains(err.Error(), "invalid accelerator type format") {
				return "", false, err
			}
		}
	}

	logging.Info("Auto-discovering Topology for %s...", job.MachineType)
	accelLabel := g.GenerateGKENodeSelectorLabel(job.MachineType)
	output, err := g.queryDiscoveredTopologies(accelLabel, job.MachineType)
	if err != nil {
		return "", false, err
	}

	topologies := g.parseTopologies(output)

	parts := strings.Split(job.ComputeType, "-")
	var requestedChips int
	if len(parts) == 2 {
		// If parts[1] is not a valid integer, Atoi returns 0.
		// We ignore the error and fall back to resolving without size restriction.
		requestedChips, _ = strconv.Atoi(parts[1])
	}
	res, err := g.selectTopology(job.Topology, topologies, job.MachineType, requestedChips)
	if err == nil {
		g.topologyCache[cacheKey] = res
	}
	return res, false, err
}

func calculateChipsFromTopology(topology string) int {
	parts := strings.Split(topology, "x")
	chips := 1
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		if n > 0 {
			chips *= n
		}
	}
	return chips
}

func (g *GKEOrchestrator) selectTopology(requested string, topologies map[string]bool, accelType string, requestedChips int) (string, error) {
	if len(topologies) == 0 {
		if requested != "" {
			logging.Info("Warning: No active topologies discovered from Kueue or Nodes. Fast-tracking provided topology: %s", requested)
			return requested, nil
		}
		return "", nil
	}

	if requested != "" {
		if err := g.validateRequestedTopology(requested, topologies, accelType); err != nil {
			return "", err
		}
		return requested, nil
	}

	if requestedChips > 0 {
		for t := range topologies {
			chips := calculateChipsFromTopology(t)
			if chips == requestedChips {
				logging.Info("Auto-discovered Topology matching requested size (%d chips): %s", requestedChips, t)
				return t, nil
			}
		}
		return "", fmt.Errorf("no discovered topologies match the requested size of %d chips", requestedChips)
	}

	uniqueTops := make([]string, 0, len(topologies))
	for t := range topologies {
		uniqueTops = append(uniqueTops, t)
	}

	if len(uniqueTops) == 1 {
		logging.Info("Auto-discovered Topology: %s", uniqueTops[0])
		return uniqueTops[0], nil
	}

	logging.Info("Warning: Multiple Topologies found (%v). Defaulting to the first one: %s", uniqueTops, uniqueTops[0])
	return uniqueTops[0], nil
}

func (g *GKEOrchestrator) validateRequestedTopology(requested string, topologies map[string]bool, accelType string) error {
	if topologies[requested] {
		logging.Info("Validated provided Topology: %s", requested)
		return nil
	}

	if g.napEnabled {
		logging.Info("NAP is enabled. Allowing requested topology %s which differs from currently discovered limits (may trigger new node pool creation).", requested)
		return nil
	}

	for t := range topologies {
		fit, err := config.CheckTopologyContainment(requested, t, accelType)
		if err != nil {
			return fmt.Errorf("failed to check topology containment: %w", err)
		}
		if fit {
			if !g.hasSlicingTopologies() {
				var valid []string
				for t := range topologies {
					valid = append(valid, t)
				}
				slices.Sort(valid)
				return fmt.Errorf("requested topology %s fits inside discovered limits but no slicing labels found. It must match discovered limits exactly: %v", requested, valid)
			}
			logging.Info("Validated provided Topology: %s", requested)
			return nil
		}
	}

	var valid []string
	for t := range topologies {
		valid = append(valid, t)
	}
	slices.Sort(valid)
	return fmt.Errorf("requested topology %s is not valid for cluster. It must match or fit inside discovered limits: %v", requested, valid)
}

func (g *GKEOrchestrator) resolveDynamicSlicingTopology(job *orchestrator.JobDefinition) (string, bool, error) {
	active, err := g.verifyDynamicSlicingActive(ManifestOptions{
		ClusterName:     job.ClusterName,
		ClusterLocation: job.ClusterLocation,
		ComputeType:     job.ComputeType,
		MachineType:     job.MachineType,
		Topology:        job.Topology,
	})
	if err != nil {
		if active {
			return "", true, err
		}
		logging.Warn("Failed to verify if dynamic slicing is active: %v. Assuming not active.", err)
	}
	if active {
		logging.Info("Dynamic-slicing detected. Skipping strict physical state queries for topology.")
		if job.Topology != "" {
			if err := config.Validate3DTopology(job.Topology, job.MachineType, true); err != nil {
				return "", true, err
			}
			logging.Info("Validated provided Topology (Dynamic-Slicing): %s", job.Topology)
			return job.Topology, true, nil
		}
		return "", true, nil
	}
	return "", false, nil
}

func (g *GKEOrchestrator) parseTopologies(output string) map[string]bool {
	topologies := make(map[string]bool)
	for _, top := range strings.Split(output, "\n") {
		top = strings.TrimSpace(top)
		if top != "" {
			topologies[top] = true
		}
	}
	return topologies
}

func (g *GKEOrchestrator) queryDiscoveredTopologies(accelLabel string, machineType string) (string, error) {
	selector := fmt.Sprintf("cloud.google.com/gke-tpu-accelerator=%s", accelLabel)

	var nodePoolTopologies []string
	for _, np := range g.clusterDesc.NodePools {
		if strings.EqualFold(np.Config.MachineType, machineType) {
			if np.PlacementPolicy != nil && np.PlacementPolicy.TpuTopology != "" {
				nodePoolTopologies = append(nodePoolTopologies, np.PlacementPolicy.TpuTopology)
			}
		}
	}

	var output string
	if g.kueueEnabled() {
		res := g.kubectl("get", "resourceflavors.kueue.x-k8s.io", "-o", "jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}", "-l", selector)
		output = strings.TrimSpace(res.Stdout)
	}

	if output == "" {
		res := g.kubectl("get", "nodes", "-o", "jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}", "-l", selector)
		if res.ExitCode != 0 {
			if rawErr := kubectlOutputError(res); rawErr != nil && k8s.IsForbiddenError(rawErr) {
				logging.Warn("Insufficient RBAC permissions to query Nodes for topology (403 Forbidden); falling back to node pool metadata topologies.")
				return strings.Join(nodePoolTopologies, "\n"), nil
			}
			return "", kubectlCommandError("failed to query Nodes for topology", res)
		}
		output = strings.TrimSpace(res.Stdout)
	}

	if len(nodePoolTopologies) > 0 {
		output = strings.Join(nodePoolTopologies, "\n") + "\n" + output
	}
	return strings.TrimSpace(output), nil
}

func (g *GKEOrchestrator) resolveBuildTargetNamespace(explicitNS, clusterName, location, projectID string, dryRun bool) (string, error) {
	if ns := strings.TrimSpace(explicitNS); ns != "" {
		return ns, nil
	}
	ns, err := g.getCurrentNamespace(clusterName, location, projectID)
	if err != nil {
		if dryRun {
			return "default", nil
		}
		return "", fmt.Errorf("failed to resolve target namespace for image build: %w", err)
	}
	return ns, nil
}

func (g *GKEOrchestrator) BuildContainerImage(job orchestrator.JobDefinition) (string, error) {
	if job.Pathways.Headless || job.Pathways.DeploySPS {
		return "", nil
	}
	if job.DryRunManifest != "" {
		if job.BaseImage != "" {
			logging.Info("[Dry Run] Skipping Crane build, generating predicted URI...")
			targetNS, err := g.resolveBuildTargetNamespace(job.GKENamespace, job.ClusterName, job.ClusterLocation, job.ProjectID, true)
			if err != nil {
				return "", err
			}
			return imagebuilder.GenerateImageName(job.ProjectID, job.ClusterLocation, targetNS, job.WorkloadName, job.BaseImage, "")
		}
		if job.ImageName != "" {
			logging.Info("[Dry Run] Using pre-existing container image: %s", job.ImageName)
			return job.ImageName, nil
		}
	}

	if job.BaseImage != "" {
		logging.Info("Building container image using Crane (Go implementation) on top of %s...", job.BaseImage)

		targetNS, err := g.resolveBuildTargetNamespace(job.GKENamespace, job.ClusterName, job.ClusterLocation, job.ProjectID, false)
		if err != nil {
			return "", err
		}

		ignorePatterns := []string{
			".git", ".terraform", ".ghpc", ".ansible", "vendor", "bin", "pkg", "node_modules", "*.log", "tmp/", ".DS_Store", "__pycache__",
		}

		ignoreMatcher, err := imagebuilder.ReadDockerignorePatterns(job.BuildContext, ignorePatterns)
		if err != nil {
			return "", fmt.Errorf("failed to read .dockerignore patterns: %w", err)
		}

		fullImageName, err := imagebuilder.BuildContainerImageFromBaseImage(
			job.ProjectID,
			job.ClusterLocation,
			targetNS,
			job.WorkloadName,
			job.BaseImage,
			job.BuildContext,
			job.Platform,
			ignoreMatcher,
		)
		if err != nil {
			return "", fmt.Errorf("crane-based image build failed: %w", err)
		}
		logging.Info("Built image will be available at: %s", fullImageName)
		return fullImageName, nil
	} else if job.ImageName != "" {
		logging.Info("Using pre-existing container image: %s", job.ImageName)
		return job.ImageName, nil
	}
	return "", fmt.Errorf("either --image or --base-image must be provided")
}

func (g *GKEOrchestrator) generateAndApplyManifest(opts ManifestOptions, profile JobProfile, outputManifestPath string) error {
	logging.Info("Generating GKE manifest...")
	gkeManifestContent, err := g.GenerateGKEManifest(opts, profile)
	if err != nil {
		return fmt.Errorf("failed to generate GKE manifest: %w", err)
	}

	return g.ApplyManifest(gkeManifestContent, outputManifestPath, opts.WorkloadName)
}

func (g *GKEOrchestrator) generateAndApplyJobManifest(opts ManifestOptions, profile JobProfile, outputManifestPath, clusterName, projectID string) error {
	logging.Info("Generating standalone batch/v1 Indexed Job manifest...")
	jobManifestContent, err := g.GenerateJobManifest(opts, profile)
	if err != nil {
		return fmt.Errorf("failed to generate batch/v1 Job manifest: %w", err)
	}

	resolvedProject := cmp.Or(strings.TrimSpace(projectID), strings.TrimSpace(g.projectID))
	if err := g.ApplyManifest(jobManifestContent, outputManifestPath, opts.WorkloadName); err != nil {
		if outputManifestPath == "" {
			g.rollbackPartialBatchJobService(opts.WorkloadName, clusterName, opts.ClusterLocation, resolvedProject)
		}
		return err
	}
	if outputManifestPath == "" {
		g.attachBatchJobServiceOwnerRefWithTarget(opts.WorkloadName, clusterName, opts.ClusterLocation, resolvedProject)
	}
	return nil
}

// resolveTier3ClientAndNamespace lazily prepares the Kubernetes client and target namespace for
// post-apply Tier-3 headless Service ownership/rollback helpers without reading host kubeconfig when unconfigured.
func (g *GKEOrchestrator) resolveTier3ClientAndNamespace(clusterName, location, projectID string) (KubeClient, string) {
	kc := g.usableKubeClient()
	if kc == nil {
		return nil, ""
	}
	ns := strings.TrimSpace(g.namespace)
	if ns == "" && strings.TrimSpace(g.kubeContext) != "" {
		resolved, err := g.getCurrentNamespace(clusterName, location, projectID)
		if err == nil {
			ns = strings.TrimSpace(resolved)
			if ns != "" {
				g.namespace = ns
			}
		}
	}
	return kc, ns
}

// rollbackPartialBatchJobService removes a gcluster-owned headless Service left behind if kubectl apply
// created the Service before batch/v1 Job admission failed. Never touches the Service if the Job exists
// or if Job lookup fails with a non-NotFound error.
func (g *GKEOrchestrator) rollbackPartialBatchJobService(name, clusterName, location, projectID string) {
	if name == "" {
		return
	}
	kc, ns := g.resolveTier3ClientAndNamespace(clusterName, location, projectID)
	if kc == nil || ns == "" {
		return
	}
	_, err := kc.GetResource(jobGVR, ns, name)
	if err == nil {
		return
	}
	if !apierrors.IsNotFound(err) {
		logging.Warn("Could not verify whether batch/v1 Job %q exists in namespace %q after apply failure (%v); a headless Service may remain. Run '%s' if cleanup is needed.",
			name, ns, err, g.formatCancelHintInNamespace(name, clusterName, location, projectID, ns))
		return
	}
	if _, delErr := g.deleteOwnedFallbackService(ns, name); delErr != nil {
		logging.Warn("Failed to clean up headless Service %q in namespace %q after partial batch/v1 Job apply (%v); run '%s' to remove it.",
			name, ns, delErr, g.formatCancelHintInNamespace(name, clusterName, location, projectID, ns))
	}
}

// attachBatchJobServiceOwnerRefWithTarget binds the Tier-3 headless Service to its batch/v1 Job via metadata.ownerReferences
// (with controller=true and without blockOwnerDeletion to avoid jobs/finalizers RBAC requirements), so Kubernetes
// garbage-collects the Service when ttlSecondsAfterFinished deletes the Job and prevents stale DNS selectors.
func (g *GKEOrchestrator) attachBatchJobServiceOwnerRefWithTarget(name, clusterName, location, projectID string) {
	if name == "" {
		return
	}
	_, ns := g.resolveTier3ClientAndNamespace(clusterName, location, projectID)
	if g.dynClient == nil || ns == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	jobUID, ok := g.lookupOwnedBatchJobUID(ctx, ns, name)
	if !ok {
		return
	}
	svcObj, ok := g.lookupOwnedServiceForOwnerRef(ctx, ns, name)
	if !ok {
		return
	}
	patchBytes, conflictOwner := buildOwnerRefJSONPatch(svcObj.GetOwnerReferences(), name, jobUID)
	if conflictOwner != "" {
		logging.Warn("Headless Service %q in namespace %q is already controlled by %s; skipping Job ownerReference patch. Run '%s' to clean up the Service after the Job expires.",
			name, ns, conflictOwner, g.formatCancelHintInNamespace(name, clusterName, location, projectID, ns))
		return
	}
	if len(patchBytes) == 0 {
		return
	}
	if _, err := g.dynClient.Resource(serviceGVR).Namespace(ns).Patch(ctx, name, k8stypes.JSONPatchType, patchBytes, metav1.PatchOptions{}); err != nil {
		logging.Warn("Could not attach Job ownerReference to headless Service %q in namespace %q (%v); run '%s' to clean up the Service after the Job expires.", name, ns, err, g.formatCancelHintInNamespace(name, clusterName, location, projectID, ns))
	}
}

func buildOwnerRefJSONPatch(existing []metav1.OwnerReference, name, jobUID string) ([]byte, string) {
	for _, ref := range existing {
		if string(ref.UID) == jobUID {
			return nil, ""
		}
		if ref.Controller != nil && *ref.Controller {
			return nil, fmt.Sprintf("%s/%s (uid %s)", ref.Kind, ref.Name, ref.UID)
		}
	}
	ownerRef := metav1.OwnerReference{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       name,
		UID:        k8stypes.UID(jobUID),
		Controller: new(true),
	}
	type jsonPatchOp struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	patchOp := jsonPatchOp{
		Op:    "add",
		Path:  "/metadata/ownerReferences",
		Value: []metav1.OwnerReference{ownerRef},
	}
	if len(existing) > 0 {
		patchOp.Path = "/metadata/ownerReferences/-"
		patchOp.Value = ownerRef
	}
	patchBytes, err := json.Marshal([]jsonPatchOp{patchOp})
	if err != nil {
		return nil, ""
	}
	return patchBytes, ""
}

func (g *GKEOrchestrator) formatCancelHintInNamespace(name, clusterName, location, projectID, ns string) string {
	if g.mode == orchestrator.TypeK8s {
		return fmt.Sprintf("gcluster job cancel %s%s", name, g.k8sJobCLIFlags(clusterName, ns))
	}
	parts := []string{"gcluster", "job", "cancel", name}
	if c := strings.TrimSpace(clusterName); c != "" {
		parts = append(parts, "--cluster", c)
	}
	if l := strings.TrimSpace(location); l != "" {
		parts = append(parts, "--location", l)
	}
	if p := strings.TrimSpace(projectID); p != "" {
		parts = append(parts, "--project", p)
	}
	if trimmedNS := strings.TrimSpace(ns); trimmedNS != "" {
		parts = append(parts, "--gke-namespace", trimmedNS)
	}
	return strings.Join(parts, " ")
}

func (g *GKEOrchestrator) lookupOwnedBatchJobUID(ctx context.Context, ns, name string) (string, bool) {
	jobObj, err := g.dynClient.Resource(jobGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		logging.Warn("Could not read batch/v1 Job %q UID to attach ownerReferences to headless Service %q: %v", name, name, err)
		return "", false
	}
	if jobObj == nil || jobObj.GetLabels()[workloadLabel] != name || jobObj.GetUID() == "" {
		return "", false
	}
	return string(jobObj.GetUID()), true
}

func (g *GKEOrchestrator) lookupOwnedServiceForOwnerRef(ctx context.Context, ns, name string) (*unstructured.Unstructured, bool) {
	svcObj, err := g.dynClient.Resource(serviceGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !k8s.IsNotFoundOrNoMatchError(err) {
			logging.Warn("Could not inspect headless Service %q before attaching ownerReferences: %v", name, err)
		}
		return nil, false
	}
	if svcObj == nil || svcObj.GetLabels()[workloadLabel] != name || isJobSetOwnedJob(svcObj) {
		return nil, false
	}
	return svcObj, true
}

// TODO: Make this a dynamic lookup using cloud.google.com/gke-tpu-accelerator & cloud.google.com/gke-accelerator
func (g *GKEOrchestrator) GenerateGKENodeSelectorLabel(acceleratorType string) string {
	resolvedLower := strings.ToLower(acceleratorType)

	parts := strings.Split(resolvedLower, "-")

	// Try matching first two parts (e.g., "g2-standard")
	if len(parts) >= 2 {
		family := parts[0] + "-" + parts[1]
		if label, ok := config.GetMachineMappings().MachineFamilyToLabelMap[family]; ok {
			return label
		}
	}

	// Try matching first part (e.g., "v6e")
	if len(parts) >= 1 {
		if label, ok := config.GetMachineMappings().MachineFamilyToLabelMap[parts[0]]; ok {
			return label
		}
	}

	return acceleratorType
}

func (g *GKEOrchestrator) determineIfCPUMachine(job *orchestrator.JobDefinition) (bool, int, error) {
	if _, exists := config.AcceleratorShorthandMap[job.MachineType]; exists {
		return false, 0, nil
	}

	for _, realMachine := range config.AcceleratorShorthandMap {
		if job.MachineType == realMachine {
			return false, 0, nil
		}
	}

	cap, err := g.FetchMachineCapabilities(job.MachineType, job.ClusterLocation)
	if err != nil {
		return false, 0, fmt.Errorf("failed to fetch machine capabilities for %s: %w", job.MachineType, err)
	}

	if len(cap.Accelerators) > 0 {
		return false, 0, nil
	}

	logging.Info("Dynamically determined %s is a CPU-only machine during manifest preparation", job.MachineType)
	return true, g.getEffectiveCPUs(job.MachineType, cap.GuestCpus), nil
}

func parseJobStatus(obj map[string]interface{}) (statusStr, completionTime string) {
	statusStr = "Unknown"
	completionTime = ""

	// Determine base status from spec.suspend
	if specMap, ok := obj["spec"].(map[string]interface{}); ok {
		if suspend, ok := specMap["suspend"].(bool); ok {
			if suspend {
				statusStr = "Suspended"
			} else {
				statusStr = "Running"
			}
		}
	}

	statusMap, ok := obj["status"].(map[string]interface{})
	if !ok {
		return
	}

	if compTime, ok := statusMap["completionTime"].(string); ok && compTime != "" {
		completionTime = compTime
	}

	if conditions, ok := statusMap["conditions"].([]interface{}); ok {
		parseConditions(conditions, &statusStr, &completionTime)
	}

	return
}

func parseConditions(conditions []interface{}, statusStr *string, completionTime *string) {
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		if condStatus == "True" {
			switch condType {
			case "Completed", "JobSetCompleted", "Succeeded", "Complete":
				*statusStr = "Succeeded"
				if *completionTime == "" {
					if transitionTime, ok := cond["lastTransitionTime"].(string); ok {
						*completionTime = transitionTime
					}
				}
			case "Failed", "JobSetFailed":
				*statusStr = "Failed"
				if *completionTime == "" {
					if transitionTime, ok := cond["lastTransitionTime"].(string); ok {
						*completionTime = transitionTime
					}
				}
			case "Suspended", "JobSetSuspended":
				if *statusStr != "Succeeded" && *statusStr != "Failed" {
					*statusStr = "Suspended"
				}
			}
		}
	}
}

func (g *GKEOrchestrator) getKueueWorkloadStatus(client dynamic.Interface, ns string, uid string) (string, error) {
	wlList, err := listKueueWorkloads(client, ns, uid)
	if err != nil {
		return "", err
	}
	if len(wlList.Items) > 0 {
		wlObj := wlList.Items[0]
		return g.parseKueueWorkloadStatus(wlObj.Object), nil
	}
	return "", nil
}

func (g *GKEOrchestrator) getPodAggregatedStatus(client dynamic.Interface, ns string, workloadName string) (string, error) {
	gvrPod := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("gcluster.google.com/workload=%s", workloadName),
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	podList, err := client.Resource(gvrPod).Namespace(ns).List(ctx, listOpts)
	if err != nil {
		return "", err
	}

	if len(podList.Items) == 0 {
		return "Pending", nil
	}

	allPending := true
	atLeastOneRunning := false

	for _, p := range podList.Items {
		podStatus, ok := p.Object["status"].(map[string]interface{})
		if !ok {
			continue
		}
		phase, _ := podStatus["phase"].(string)
		if phase == "Running" {
			atLeastOneRunning = true
			allPending = false
			break
		}
		if phase != "Pending" {
			allPending = false
		}
	}

	if allPending {
		return "Pending", nil
	}
	if atLeastOneRunning {
		return "Running", nil
	}
	return "Running", nil
}

const orphanedTier3ServiceStatus = "orphaned-service"

func (g *GKEOrchestrator) getJobStatus(name string) (string, error) {
	client, err := g.getDynamicClient()
	if err != nil {
		return "", err
	}
	if g.jobSetEnabled() {
		status, noMatch, err := g.lookupJobSetConflict(client, name, true)
		if err != nil || status != "" {
			return status, err
		}
		if status, err := g.lookupBatchJobConflict(client, name, noMatch); err != nil || status != "" {
			return status, err
		}
		return g.lookupServiceConflict(client, name, !noMatch)
	}
	if status, err := g.lookupBatchJobConflict(client, name, true); err != nil || status != "" {
		return status, err
	}
	if status, _, err := g.lookupJobSetConflict(client, name, false); err != nil || status != "" {
		return status, err
	}
	return g.lookupServiceConflict(client, name, false)
}

func (g *GKEOrchestrator) lookupJobSetConflict(client dynamic.Interface, name string, strict bool) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	if g.namespace != "" && (!strict || g.mode == orchestrator.TypeK8s) {
		return g.lookupNamespacedJobSetConflict(ctx, client, g.namespace, name, strict)
	}
	list, err := client.Resource(jobSetGVR).Namespace("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	})
	if err != nil {
		if k8s.IsNotFoundOrNoMatchError(err) {
			return "", true, nil
		}
		if k8s.IsForbiddenError(err) {
			if strict && g.namespace != "" {
				return g.lookupNamespacedJobSetConflict(ctx, client, g.namespace, name, true)
			}
			if !strict {
				return "", false, nil
			}
		}
		return "", false, fmt.Errorf("failed to search for jobset %s across namespaces: %w", name, err)
	}
	if len(list.Items) > 1 {
		return "", false, fmt.Errorf("found multiple jobsets named %s in different namespaces; this is not currently supported", name)
	}
	if len(list.Items) == 0 {
		return "", false, nil
	}
	st, err := g.resolveWorkloadObjectStatus(client, name, list.Items[0], false)
	return st, false, err
}

func (g *GKEOrchestrator) lookupNamespacedJobSetConflict(ctx context.Context, client dynamic.Interface, ns, name string, strict bool) (string, bool, error) {
	obj, err := client.Resource(jobSetGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8s.IsNotFoundOrNoMatchError(err) {
			return "", k8s.IsNoMatchError(err), nil
		}
		if !strict && k8s.IsForbiddenError(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to check existing jobset %s in namespace %s: %w", name, ns, err)
	}
	if obj == nil {
		return "", false, nil
	}
	st, err := g.resolveWorkloadObjectStatus(client, name, *obj, false)
	return st, false, err
}

func (g *GKEOrchestrator) lookupBatchJobConflict(client dynamic.Interface, name string, strict bool) (string, error) {
	if g.namespace == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	return g.lookupNamespacedBatchJobConflict(ctx, client, g.namespace, name, strict)
}

func (g *GKEOrchestrator) lookupNamespacedBatchJobConflict(ctx context.Context, client dynamic.Interface, ns, name string, strict bool) (string, error) {
	obj, err := client.Resource(jobGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8s.IsNotFoundOrNoMatchError(err) || (!strict && k8s.IsForbiddenError(err)) {
			return "", nil
		}
		return "", fmt.Errorf("failed to check existing batch/v1 job %s in namespace %s: %w", name, ns, err)
	}
	if obj == nil {
		return "", nil
	}
	if isJobSetOwnedJob(obj) || obj.GetLabels()[workloadLabel] != name {
		if strict {
			return "", fmt.Errorf("a batch/v1 Job named %q already exists in namespace %q (not managed by gcluster); resubmit with a different --name", name, ns)
		}
		return "", nil
	}
	return g.resolveWorkloadObjectStatus(client, name, *obj, true)
}

func (g *GKEOrchestrator) lookupServiceConflict(client dynamic.Interface, name string, checkOwnedOrphan bool) (string, error) {
	if g.namespace == "" || name == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	svc, err := client.Resource(serviceGVR).Namespace(g.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8s.IsNotFoundOrNoMatchError(err) || k8s.IsForbiddenError(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to check existing service %s in namespace %s: %w", name, g.namespace, err)
	}
	if svc == nil {
		return "", nil
	}
	if svc.GetLabels()[workloadLabel] == name && !isJobSetOwnedJob(svc) {
		if checkOwnedOrphan {
			return orphanedTier3ServiceStatus, nil
		}
		return "", nil
	}
	if !checkOwnedOrphan {
		return "", fmt.Errorf("a Kubernetes Service named %q already exists in namespace %q (not managed by gcluster); resubmit with a different --name", name, g.namespace)
	}
	return "", nil
}

func (g *GKEOrchestrator) resolveWorkloadObjectStatus(client dynamic.Interface, name string, obj unstructured.Unstructured, isBatchJob bool) (string, error) {
	ns := obj.GetNamespace()
	metadata, ok := obj.Object["metadata"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed to get JobSet metadata")
	}
	uid, _ := metadata["uid"].(string)

	if g.kueueEnabled() && uid != "" {
		wlStatus, err := g.getKueueWorkloadStatus(client, ns, uid)
		if err == nil && (wlStatus == "QuotaReserved" || wlStatus == "Evicted") {
			return wlStatus, nil
		}
	}

	var status string
	if isBatchJob {
		status, _ = parseBatchJobStatus(obj.Object)
	} else {
		status, _ = parseJobStatus(obj.Object)
	}

	if status == "Running" {
		podStatus, err := g.getPodAggregatedStatus(client, ns, name)
		if err != nil {
			return status, nil
		}
		return podStatus, nil
	}

	return status, nil
}

func (g *GKEOrchestrator) parseKueueWorkloadStatus(obj map[string]interface{}) string {
	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return "Unknown"
	}
	conditions, ok := status["conditions"].([]interface{})
	if !ok || len(conditions) == 0 {
		return "Unknown"
	}

	var latestCondition map[string]interface{}
	var latestTime string

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condStatus, _ := cond["status"].(string)
		if condStatus != "True" {
			continue
		}
		transitionTime, _ := cond["lastTransitionTime"].(string)
		if latestTime == "" || transitionTime > latestTime {
			latestTime = transitionTime
			latestCondition = cond
		}
	}

	if latestCondition != nil {
		cType, _ := latestCondition["type"].(string)
		return cType
	}

	return "Unknown"
}

func (g *GKEOrchestrator) generatePodFailurePolicy(exitCodes []int, containerNames ...string) (string, error) {
	if len(exitCodes) == 0 {
		return "", nil
	}

	validCodes, err := dedupeValidExitCodes(exitCodes)
	if err != nil || len(validCodes) == 0 {
		return "", err
	}

	policy := map[string]interface{}{
		"rules": buildPodFailureRules(validCodes, containerNames),
	}
	b, err := yaml.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func dedupeValidExitCodes(exitCodes []int) ([]int, error) {
	var validCodes []int
	seen := make(map[int]bool, len(exitCodes))
	for _, code := range exitCodes {
		if code < 1 || code > 255 {
			return nil, fmt.Errorf("invalid exit code %d in restart-on-exit-codes: exit codes must be between 1 and 255", code)
		}
		if !seen[code] {
			seen[code] = true
			validCodes = append(validCodes, code)
		}
	}
	// batch/v1 declares onExitCodes.values as +listType=set: the API server rejects unordered or duplicate
	// lists ("must be ordered"), so sorting here is required, not cosmetic.
	slices.Sort(validCodes)
	return validCodes, nil
}

func buildPodFailureRules(validCodes []int, containerNames []string) []map[string]interface{} {
	if len(containerNames) == 0 {
		containerNames = []string{""} // one pod-wide rule
	}
	rules := make([]map[string]interface{}, 0, len(containerNames))
	for _, cname := range containerNames {
		onExitCodes := map[string]interface{}{"operator": "NotIn", "values": validCodes}
		if cname != "" {
			onExitCodes["containerName"] = cname
		}
		rules = append(rules, map[string]interface{}{"action": "FailJob", "onExitCodes": onExitCodes})
	}
	return rules
}

func (g *GKEOrchestrator) generateImagePullSecrets(secrets string) string {
	if secrets == "" {
		return ""
	}
	parts := strings.Split(secrets, ",")
	var secretList []map[string]string
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			secretList = append(secretList, map[string]string{"name": s})
		}
	}
	if len(secretList) == 0 {
		return ""
	}
	b, _ := yaml.Marshal(secretList)
	return string(b)
}

func (g *GKEOrchestrator) syncKubeClient() {
	if g.kubeClient == nil {
		g.kubeClient = &DefaultKubeClient{
			dynClient:        g.dynClient,
			kubeconfigLoader: g.kubeconfigLoader,
			jobSetEnabled:    g.jobSetEnabled,
		}
	} else if defaultClient, ok := g.kubeClient.(*DefaultKubeClient); ok {
		defaultClient.dynClient = g.dynClient
		defaultClient.jobSetEnabled = g.jobSetEnabled
		if g.kubeconfigLoader != nil {
			defaultClient.kubeconfigLoader = g.kubeconfigLoader
		}
	}
}

func (g *GKEOrchestrator) needsDynamicClientInit() bool {
	if g.kubeClient == nil {
		return true
	}
	if defaultClient, ok := g.kubeClient.(*DefaultKubeClient); ok {
		return defaultClient.dynClient == nil || (g.dynClientManaged && g.dynClientContext != g.kubeContext)
	}
	return false
}

func (g *GKEOrchestrator) resetManagedDynamicClient() {
	if !g.dynClientManaged {
		return
	}
	g.dynClient = nil
	g.dynClientManaged = false
	g.dynClientContext = ""
	g.syncKubeClient()
}

// getDynamicClient builds the client for the context set by configureKubectl and caches it,
// rebuilding automatically if a managed client's pinned kubeContext changes.
func (g *GKEOrchestrator) getDynamicClient() (dynamic.Interface, error) {
	if g.dynClient != nil && (!g.dynClientManaged || g.dynClientContext == g.kubeContext) {
		g.syncKubeClient()
		return g.dynClient, nil
	}
	g.resetManagedDynamicClient()
	config, err := g.restConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	g.dynClient = dynClient
	g.dynClientManaged = true
	g.dynClientContext = g.kubeContext
	g.syncKubeClient()
	return g.dynClient, nil
}

// restConfig builds a client-go config for the pinned context from the same kubeconfig
// loader the rest of the orchestrator uses; an empty context means current-context.
func (g *GKEOrchestrator) restConfig() (*rest.Config, error) {
	cfg, err := g.loadKubeconfig()
	if err != nil {
		return nil, err
	}
	var configAccess clientcmd.ConfigAccess
	if g.kubeconfigLoader == nil {
		configAccess = clientcmd.NewDefaultPathOptions()
	}
	return clientcmd.NewNonInteractiveClientConfig(*cfg, g.kubeContext, &clientcmd.ConfigOverrides{}, configAccess).ClientConfig()
}

func (g *GKEOrchestrator) awaitJobCompletion(workloadName, clusterName, clusterLocation, projectID, timeout string, queued bool) error {
	logging.Info("Waiting for job '%s' to complete...", workloadName)

	if _, err := g.getDynamicClient(); err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}

	ns, err := g.getCurrentNamespace(clusterName, clusterLocation, projectID)
	if err != nil {
		return fmt.Errorf("failed to get current namespace: %w", err)
	}

	jobConsoleLink := ""
	if g.mode != orchestrator.TypeK8s {
		jobConsoleLink = fmt.Sprintf("https://console.cloud.google.com/kubernetes/workload/gke/%s/%s/details/%s?project=%s",
			clusterLocation, clusterName, workloadName, projectID)
	}

	if !g.kueueEnabled() || !queued {
		return g.awaitWorkloadDirect(workloadName, clusterName, ns, timeout, jobConsoleLink)
	}

	targetWorkloadName, err := g.findTargetWorkload(ns, workloadName, 60*time.Second)
	if err != nil {
		return err
	}

	err = g.waitWorkloadFinished(targetWorkloadName, ns, timeout, jobConsoleLink, workloadName)
	if err != nil {
		return err
	}

	logging.Info("Job '%s' has finished. Checking final status...", workloadName)

	status, err := g.getFinalWorkloadStatus(workloadName, ns)
	if err != nil {
		return err
	}

	return g.reportWorkloadTerminal(workloadName, clusterName, ns, jobConsoleLink, status)
}

const maxDirectAwaitTimeout = 168 * time.Hour

func parseDirectAwaitTimeout(timeout string) (time.Duration, error) {
	trimmed := strings.TrimSpace(timeout)
	if trimmed == "" || trimmed == "-1s" {
		return maxDirectAwaitTimeout, nil
	}
	dur, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid --timeout value %q: %w", timeout, err)
	}
	if dur < 0 {
		return maxDirectAwaitTimeout, nil
	}
	return dur, nil
}

func (g *GKEOrchestrator) readDirectWorkloadInfo(ns, workloadName string) (jobSetInfo, error) {
	if !g.jobSetEnabled() {
		return g.getBatchJobInfo(ns, workloadName)
	}
	return g.getJobSetInfo(ns, workloadName)
}

func (g *GKEOrchestrator) getFinalWorkloadStatus(workloadName, ns string) (string, error) {
	if !g.jobSetEnabled() {
		info, err := g.getBatchJobInfo(ns, workloadName)
		if err != nil {
			return "", err
		}
		if info.terminalState != "" {
			return info.terminalState, nil
		}
		if info.suspended {
			return "Suspended", nil
		}
		return "Running", nil
	}
	return g.getJobSetStatus(workloadName, ns)
}

func isPermanentAwaitError(err error) bool {
	if err == nil {
		return false
	}
	return k8s.IsForbiddenError(err) || k8s.IsNotFoundOrNoMatchError(err) || strings.Contains(strings.ToLower(err.Error()), "not found in namespace")
}

func (g *GKEOrchestrator) awaitWorkloadDirect(workloadName, clusterName, ns, timeout, jobConsoleLink string) error {
	dur, err := parseDirectAwaitTimeout(timeout)
	if err != nil {
		return err
	}
	pollInterval := calculatePollInterval(dur)
	deadline := time.Now().Add(dur)
	warnedTransient := false
	for {
		info, err := g.readDirectWorkloadInfo(ns, workloadName)
		if err != nil {
			if isPermanentAwaitError(err) || dur == 0 || time.Now().After(deadline) {
				return err
			}
			if !warnedTransient {
				logging.Warn("Transient error polling job '%s' status (%v); retrying until timeout...", workloadName, err)
				warnedTransient = true
			}
			directAwaitSleep(pollInterval)
			continue
		}
		if info.terminal() {
			return g.reportWorkloadTerminal(workloadName, clusterName, ns, jobConsoleLink, info.terminalState)
		}
		if dur == 0 || time.Now().After(deadline) {
			return g.formatDirectTimeoutError(ns, workloadName, info.isBatchJob)
		}
		directAwaitSleep(pollInterval)
	}
}

func (g *GKEOrchestrator) reportWorkloadTerminal(workloadName, clusterName, ns, jobConsoleLink, terminalState string) error {
	if terminalState != "Completed" {
		if jobConsoleLink != "" {
			logging.Error("Job '%s' finished with status '%s'. Check details in the Cloud Console: %s", workloadName, terminalState, jobConsoleLink)
		} else {
			logging.Error("Job '%s' finished with status '%s'. Run 'gcluster job inspect --name %s%s' for details.", workloadName, terminalState, workloadName, g.k8sJobCLIFlags(clusterName, ns))
		}
		return fmt.Errorf("job completed unsuccessfully with status: %s", terminalState)
	}
	if jobConsoleLink != "" {
		logging.Info("Job '%s' completed successfully. View details in the Cloud Console: %s", workloadName, jobConsoleLink)
	} else {
		logging.Info("Job '%s' completed successfully.", workloadName)
	}
	return nil
}

func (g *GKEOrchestrator) formatDirectTimeoutError(ns, workloadName string, isBatchJob bool) error {
	if warnEvents := g.checkWorkloadWarningEvents(ns, workloadName, isBatchJob); warnEvents != "" {
		return fmt.Errorf("job timed out\nWorkload warning events:\n%s", warnEvents)
	}
	return fmt.Errorf("job timed out")
}

func (g *GKEOrchestrator) getJobSetStatus(workloadName, ns string) (string, error) {
	statusRes := g.kubectl("get", "jobset", workloadName, "-n", ns, "-o", "json")
	if statusRes.ExitCode != 0 {
		return "", fmt.Errorf("failed to get final job status: %s\n%s", statusRes.Stderr, statusRes.Stdout)
	}

	var jsStatus JobSetStatus
	if err := json.Unmarshal([]byte(statusRes.Stdout), &jsStatus); err != nil {
		return "", fmt.Errorf("failed to parse jobset status JSON: %w", err)
	}

	if terminal := getTerminalJobSetCondition(jsStatus.Status.Conditions); terminal != "" {
		return terminal, nil
	}

	if latest := getLatestActiveJobSetCondition(jsStatus.Status.Conditions); latest != "" {
		return latest, nil
	}

	if jsStatus.Spec.Suspend {
		return "Suspended", nil
	}

	return "Running", nil
}

func getTerminalJobSetCondition(conditions []JobSetCondition) string {
	for _, cond := range conditions {
		if cond.Status == "True" {
			switch cond.Type {
			case "Completed", "JobSetCompleted", "Succeeded":
				return "Completed"
			case "Failed", "JobSetFailed":
				return "Failed"
			}
		}
	}
	return ""
}

func getLatestActiveJobSetCondition(conditions []JobSetCondition) string {
	var latestCondition JobSetCondition
	var latestTime time.Time

	for _, cond := range conditions {
		if cond.Status != "True" || cond.LastTransitionTime == "" {
			continue
		}
		transitionTime, err := time.Parse(time.RFC3339, cond.LastTransitionTime)
		if err != nil {
			continue
		}
		if transitionTime.After(latestTime) {
			latestTime = transitionTime
			latestCondition = cond
		}
	}
	return latestCondition.Type
}

func calculatePollInterval(timeout time.Duration) time.Duration {
	pollInterval := 2 * time.Second
	if timeout < pollInterval {
		pollInterval = timeout / 2
		if pollInterval < 100*time.Millisecond {
			pollInterval = 100 * time.Millisecond
		}
	}
	return pollInterval
}

func (g *GKEOrchestrator) findTargetWorkload(ns, workloadName string, timeout time.Duration) (string, error) {
	return g.findTargetWorkloadForKind(ns, workloadName, timeout, !g.jobSetEnabled())
}

func (g *GKEOrchestrator) findTargetWorkloadForKind(ns, workloadName string, timeout time.Duration, isBatchJob bool) (string, error) {
	if g.needsDynamicClientInit() {
		if _, err := g.getDynamicClient(); err != nil {
			return "", fmt.Errorf("kubernetes client is not initialized: %w", err)
		}
	}
	if g.kubeClient == nil {
		return "", fmt.Errorf("kubernetes client is not initialized")
	}

	kindLower, kindTitle := "jobset", "JobSet"
	if isBatchJob {
		kindLower, kindTitle = "job", "Job"
	}
	logging.Info("Discovering Kueue workload for %s '%s'...", kindLower, workloadName)
	pollInterval := calculatePollInterval(timeout)
	deadline := time.Now().Add(timeout)

	var lastErr error
	for {
		matchedWorkloads, err := g.listKueueWorkloadsForKind(ns, workloadName, isBatchJob)
		lastErr = err
		if err == nil && len(matchedWorkloads) > 0 {
			targetWorkloadName := matchedWorkloads[len(matchedWorkloads)-1]
			if targetWorkloadName != "" {
				return targetWorkloadName, nil
			}
		}

		if timeout <= 0 || time.Now().After(deadline) {
			break
		}
		directAwaitSleep(pollInterval)
	}

	warnEvents := g.checkWorkloadWarningEvents(ns, workloadName, isBatchJob)
	return "", formatMissingKueueWorkloadError(kindLower, kindTitle, workloadName, warnEvents, timeout, lastErr)
}

func formatMissingKueueWorkloadError(kindLower, kindTitle, workloadName, warnEvents string, timeout time.Duration, lastErr error) error {
	var warnSuffix string
	if warnEvents != "" {
		warnSuffix = fmt.Sprintf("\n%s warning events:\n%s", kindTitle, warnEvents)
	}
	if lastErr != nil {
		return fmt.Errorf("failed to find Kueue workload for %s %s: %w%s", kindLower, workloadName, lastErr, warnSuffix)
	}
	if timeout <= 0 {
		return fmt.Errorf("failed to find Kueue workload for %s %s%s", kindLower, workloadName, warnSuffix)
	}
	return fmt.Errorf("failed to find Kueue workload for %s %s (timed out waiting for Kueue to create workload)%s", kindLower, workloadName, warnSuffix)
}

func (g *GKEOrchestrator) listKueueWorkloadsForKind(ns, workloadName string, isBatchJob bool) ([]string, error) {
	if isBatchJob {
		if d, ok := g.kubeClient.(*DefaultKubeClient); ok {
			return d.listWorkloadsForBatchJob(ns, workloadName)
		}
	}
	return g.kubeClient.ListWorkloads(ns, workloadName)
}

func (g *GKEOrchestrator) checkJobSetWarningEvents(ns, workloadName string) string {
	return g.checkWorkloadWarningEvents(ns, workloadName, false)
}

func (g *GKEOrchestrator) checkWorkloadWarningEvents(ns, workloadName string, isBatchJob bool) string {
	if workloadName == "" {
		return ""
	}
	involvedKind := workloadKindLabel(isBatchJob)
	podSelector := workloadPodSelector(workloadName, isBatchJob)
	var events []string
	res := g.kubectl("get", "events", "-n", ns,
		fmt.Sprintf("--field-selector=involvedObject.name=%s,involvedObject.kind=%s,type=Warning", workloadName, involvedKind),
		"--request-timeout=10s",
		"--no-headers")
	if res.ExitCode == 0 && strings.TrimSpace(res.Stdout) != "" {
		events = append(events, strings.TrimSpace(res.Stdout))
	}
	podsRes := g.kubectl("get", "pods", "-n", ns, "-l", podSelector, "-o", "name")
	if podsRes.ExitCode == 0 {
		jobPods := make(map[string]bool)
		for _, line := range strings.Split(strings.TrimSpace(podsRes.Stdout), "\n") {
			if name := strings.TrimSpace(line); name != "" {
				jobPods[name] = true
			}
		}
		if podEvents := g.checkPodWarningEvents(ns, jobPods); podEvents != "" {
			events = append(events, podEvents)
		}
	}
	return strings.Join(events, "\n")
}

func (g *GKEOrchestrator) checkPodWarningEvents(ns string, jobPods map[string]bool) string {
	if len(jobPods) == 0 {
		return ""
	}
	res := g.kubectl("get", "events", "-n", ns,
		"--field-selector=involvedObject.kind=Pod",
		"--request-timeout=10s",
		"--no-headers")
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return ""
	}
	var matched []string
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 4 || !jobPods[fields[3]] {
			continue
		}
		if fields[1] == "Warning" || fields[2] == "NotTriggerScaleUp" || fields[2] == "FailedScheduling" {
			matched = append(matched, line)
		}
	}
	return strings.Join(matched, "\n")
}

func (g *GKEOrchestrator) waitWorkloadFinished(targetWorkloadName, ns, timeout, jobConsoleLink, workloadName string) error {
	logging.Info("Waiting for Kueue workload '%s' to be Finished...", targetWorkloadName)
	waitRes := g.kubectl("wait", "--for=condition=Finished",
		"workload", targetWorkloadName, "-n", ns, "--timeout="+timeout)

	if waitRes.ExitCode != 0 {
		isBatchJob := !g.jobSetEnabled()
		warnEvents := g.checkWorkloadWarningEvents(ns, workloadName, isBatchJob)
		var warnSuffix string
		if warnEvents != "" {
			kind := workloadKindLabel(isBatchJob)
			logging.Error("%s '%s' reported warning events:\n%s", kind, workloadName, warnEvents)
			warnSuffix = fmt.Sprintf("\n%s warning events:\n%s", kind, warnEvents)
		}
		if strings.Contains(waitRes.Stderr, "timed out waiting") || strings.Contains(waitRes.Stdout, "timed out waiting") {
			if jobConsoleLink != "" {
				logging.Error("Timed out waiting for job '%s' to finish. Check its status in the Cloud Console: %s", workloadName, jobConsoleLink)
			} else {
				logging.Error("Timed out waiting for job '%s' to finish. Run 'gcluster job inspect --name %s%s' for details.", workloadName, workloadName, g.k8sJobCLIFlags("", ns))
			}
			return fmt.Errorf("job timed out%s", warnSuffix)
		}
		return fmt.Errorf("error waiting for job completion: %s\n%s%s", waitRes.Stderr, waitRes.Stdout, warnSuffix)
	}
	return nil
}

func (g *GKEOrchestrator) addTopologyLabel(nodeSelector map[string]string, schedOpts SchedulingOptions, isGPU bool, isCPUMachine bool) error {
	if schedOpts.Topology != "" {
		if isGPU || isCPUMachine {
			return fmt.Errorf("topology is not allowed for GPU and CPU jobs")
		}
		if !schedOpts.IsDynamicSlicing && !schedOpts.IsStaticSlicing {
			_, hasFallback := schedOpts.NodeAffinityLabels[tpuTopologyLabel]
			if !hasFallback {
				nodeSelector[tpuTopologyLabel] = schedOpts.Topology
			}
		}
	}
	return nil
}

func injectProvisioningLabels(nodeSelector map[string]string, provisioning string, reservation string) {
	switch strings.ToLower(provisioning) {
	case "spot":
		nodeSelector["cloud.google.com/gke-provisioning"] = "spot"
	case "on-demand":
		nodeSelector["cloud.google.com/gke-provisioning"] = "standard"
	case "reservation":
		res := parseReservationURI(reservation)
		if res.Name != "" {
			nodeSelector["cloud.google.com/reservation-name"] = res.Name
			nodeSelector["cloud.google.com/reservation-affinity"] = "specific"
		}
		if res.Project != "" {
			nodeSelector["cloud.google.com/reservation-project"] = res.Project
		}
		if res.Block != "" {
			nodeSelector["cloud.google.com/reservation-blocks"] = res.Block
		}
		if res.Subblock != "" {
			nodeSelector["cloud.google.com/reservation-subblocks"] = res.Subblock
		}
	}
}

func (g *GKEOrchestrator) buildNodeSelectorWithProfile(schedOpts SchedulingOptions, job orchestrator.JobDefinition, profile JobProfile) (string, error) {
	nodeSelector := make(map[string]string)
	existing, err := getNodeSelector(schedOpts)
	if err != nil {
		return "", err
	}
	for k, v := range existing {
		nodeSelector[k] = v
	}

	// Inject unified consumption options
	injectProvisioningLabels(nodeSelector, job.GKENAPProvisioning, job.GKENAPReservation)

	accelLabel, isTPU, isGPU, err := g.resolveNodeSelectorAccelerator(job, profile)
	if err != nil {
		return "", err
	}
	if err := g.applyPlatformNodeSelectorLabels(nodeSelector, schedOpts, profile, accelLabel, job.MachineType, isTPU, isGPU, profile.IsCPUMachine); err != nil {
		return "", err
	}

	if len(nodeSelector) == 0 {
		return "", nil
	}
	b, err := yaml.Marshal(nodeSelector)
	if err != nil {
		return "", fmt.Errorf("failed to marshal nodeSelector: %w", err)
	}
	return indentYaml(string(b), 16), nil
}

func (g *GKEOrchestrator) resolveNodeSelectorAccelerator(job orchestrator.JobDefinition, profile JobProfile) (accelLabel string, isTPU, isGPU bool, err error) {
	if g.mode == orchestrator.TypeK8s {
		isTPU = profile.IsTPU || (!profile.IsCPUMachine && profile.K8sPool == nil && defaultTPUHardwareResolver.IsTPUMachine(job.MachineType))
		isGPU = !profile.IsCPUMachine && !isTPU
		if !profile.IsCPUMachine {
			accelLabel = g.resolveK8sGKEAcceleratorLabel(job.MachineType, profile, isTPU)
		}
		return accelLabel, isTPU, isGPU, nil
	}
	cap, err := g.FetchMachineCapabilities(job.MachineType, job.ClusterLocation)
	if err != nil {
		return "", false, false, fmt.Errorf("failed to fetch machine capabilities: %w", err)
	}
	if len(cap.Accelerators) > 0 {
		accelLabel = cap.Accelerators[0].Type
	}
	isTPU = !profile.IsCPUMachine && config.IsTPU(job.MachineType)
	isGPU = !profile.IsCPUMachine && !isTPU
	return accelLabel, isTPU, isGPU, nil
}

func (g *GKEOrchestrator) resolveK8sGKEAcceleratorLabel(machineType string, profile JobProfile, isTPU bool) string {
	if isTPU {
		return g.resolveK8sGKETPUAcceleratorLabel(machineType, profile)
	}
	if profile.K8sPool != nil && profile.K8sPool.IsGPU {
		return strings.TrimSpace(profile.K8sPool.GKEAcceleratorLabel)
	}
	mapped := g.GenerateGKENodeSelectorLabel(machineType)
	if mapped == "" || strings.Contains(mapped, "/") {
		return ""
	}
	if config.ValidGPUAccelerators[strings.ToLower(mapped)] || !strings.EqualFold(mapped, strings.TrimSpace(machineType)) {
		return mapped
	}
	return ""
}

// resolveK8sGKETPUAcceleratorLabel prefers the observed pool label and falls back to the static machine map only
// when no pool matched. GenerateGKENodeSelectorLabel echoes unmapped input (e.g. "ct5l-hightpu-8t", "bm-host"),
// so a raw machine type is never emitted as the accelerator label.
func (g *GKEOrchestrator) resolveK8sGKETPUAcceleratorLabel(machineType string, profile JobProfile) string {
	if profile.K8sPool != nil && profile.K8sPool.IsTPU {
		if obs := strings.TrimSpace(profile.K8sPool.GKETPUAcceleratorLabel); obs != "" && !strings.Contains(obs, "/") {
			return obs
		}
		return ""
	}
	mapped := g.GenerateGKENodeSelectorLabel(machineType)
	if mapped == "" || strings.Contains(mapped, "/") || strings.EqualFold(mapped, strings.TrimSpace(machineType)) {
		return ""
	}
	return mapped
}

func (g *GKEOrchestrator) applyPlatformNodeSelectorLabels(nodeSelector map[string]string, schedOpts SchedulingOptions, profile JobProfile, accelLabel, machineType string, isTPU, isGPU, isCPUMachine bool) error {
	if g.mode != orchestrator.TypeK8s || g.labelScheme() == k8s.SchemeGKE {
		return g.applyGKEPlatformNodeSelectorLabels(nodeSelector, schedOpts, profile, accelLabel, machineType, isTPU, isGPU, isCPUMachine)
	}
	if schedOpts.Topology != "" && (isGPU || isCPUMachine) {
		return fmt.Errorf("topology is not allowed for GPU and CPU jobs")
	}
	if g.labelScheme() == k8s.SchemeGeneric || isGPU || isCPUMachine {
		g.applyGenericObservedNodeSelector(nodeSelector, schedOpts.NodeAffinityLabels, profile, machineType, isGPU)
	}
	return nil
}

func (g *GKEOrchestrator) applyGKEPlatformNodeSelectorLabels(nodeSelector map[string]string, schedOpts SchedulingOptions, profile JobProfile, accelLabel, machineType string, isTPU, isGPU, isCPUMachine bool) error {
	if accelLabel != "" && !isCPUMachine {
		if isTPU {
			nodeSelector["cloud.google.com/gke-tpu-accelerator"] = accelLabel
		} else {
			nodeSelector["cloud.google.com/gke-accelerator"] = accelLabel
		}
	} else if g.mode == orchestrator.TypeK8s && (isGPU || isTPU) {
		g.applyGenericObservedNodeSelector(nodeSelector, schedOpts.NodeAffinityLabels, profile, machineType, isGPU)
	}
	return g.addTopologyLabel(nodeSelector, schedOpts, isGPU, isCPUMachine)
}

func (g *GKEOrchestrator) applyGenericObservedNodeSelector(nodeSelector, affinityLabels map[string]string, profile JobProfile, machineType string, isGPU bool) {
	var pool k8s.PoolCapacity
	if profile.K8sPool != nil {
		pool = *profile.K8sPool
	} else if strings.TrimSpace(machineType) != "" {
		foundPool, ok := g.findK8sPoolForRequest("", machineType)
		if !ok {
			return
		}
		pool = foundPool
	} else {
		return
	}
	setIfUnconstrained := func(key, val string) {
		if val == "" {
			return
		}
		if _, inSel := nodeSelector[key]; inSel {
			return
		}
		if _, inAff := affinityLabels[key]; inAff {
			return
		}
		nodeSelector[key] = val
	}
	setIfUnconstrained(k8s.InstanceTypeLabel, pool.InstanceTypeLabel)
	if isGPU {
		setIfUnconstrained(k8s.NvidiaGPUProductLabel, pool.GPUProductLabel)
	}
}

func (g *GKEOrchestrator) buildAffinity(schedOpts SchedulingOptions) (string, error) {
	affinity, err := GetAffinity(schedOpts)
	if err != nil {
		return "", err
	}
	if affinity != nil {
		b, err := k8syaml.Marshal(affinity)
		if err != nil {
			return "", fmt.Errorf("failed to marshal affinity: %w", err)
		}
		return indentYaml(string(b), 16), nil
	}
	return "", nil
}

func (g *GKEOrchestrator) buildTopologyAnnotation(topology string, machineType string, numSlices int, nodesPerSlice int, isSubSlicing bool) string {
	if g.mode == orchestrator.TypeK8s && g.labelScheme() != k8s.SchemeGKE {
		return ""
	}
	var topologyAnnotation map[string]string
	if isSubSlicing {
		topologyAnnotation = GetTopologyAnnotation(topology, machineType, numSlices, nodesPerSlice)
	} else if topology != "" {
		topologyAnnotation = map[string]string{
			"cloud.google.com/gke-tpu-slice-topology": topology,
		}
	}
	if len(topologyAnnotation) > 0 {
		b, err := yaml.Marshal(topologyAnnotation)
		if err == nil {
			return indentYaml(string(b), 16)
		}
	}
	return ""
}

// DeleteJobSet deletes a JobSet resource in the specified namespace.
func (d *DefaultKubeClient) DeleteJobSet(namespace string, name string) error {
	if d.dynClient == nil {
		return fmt.Errorf("kubernetes dynamic client is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	return d.dynClient.Resource(jobSetGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (d *DefaultKubeClient) resource(gvr schema.GroupVersionResource, namespace string) (dynamic.ResourceInterface, error) {
	if d.dynClient == nil {
		return nil, fmt.Errorf("kubernetes dynamic client is not initialized")
	}
	if namespace == "" {
		return d.dynClient.Resource(gvr), nil
	}
	return d.dynClient.Resource(gvr).Namespace(namespace), nil
}

const defaultKubeClientTimeout = 30 * time.Second

// ListResources returns the raw objects of gvr in namespace matching labelSelector and fieldSelector.
func (d *DefaultKubeClient) ListResources(gvr schema.GroupVersionResource, namespace, labelSelector, fieldSelector string) ([]unstructured.Unstructured, error) {
	r, err := d.resource(gvr, namespace)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	list, err := r.List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// GetResource returns the raw object of gvr named name.
func (d *DefaultKubeClient) GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	r, err := d.resource(gvr, namespace)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	return r.Get(ctx, name, metav1.GetOptions{})
}

// DeleteResource deletes the object of gvr named name, only if it still matches pre.
func (d *DefaultKubeClient) DeleteResource(gvr schema.GroupVersionResource, namespace, name string, pre *metav1.Preconditions) error {
	r, err := d.resource(gvr, namespace)
	if err != nil {
		return err
	}
	opts := metav1.DeleteOptions{Preconditions: pre}
	if gvr == jobGVR {
		bg := metav1.DeletePropagationBackground
		opts.PropagationPolicy = &bg
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	return r.Delete(ctx, name, opts)
}

// ListWorkloads lists matching Kueue workloads in the specified namespace.
func (d *DefaultKubeClient) ListWorkloads(namespace string, workloadName string) ([]string, error) {
	return d.listWorkloadsWithUIDResolver(namespace, func() (string, error) {
		return d.resolveWorkloadOwnerUID(namespace, workloadName)
	})
}

func (d *DefaultKubeClient) listWorkloadsForBatchJob(namespace string, workloadName string) ([]string, error) {
	return d.listWorkloadsWithUIDResolver(namespace, func() (string, error) {
		return d.resolveBatchJobOwnerUID(namespace, workloadName, nil)
	})
}

func (d *DefaultKubeClient) listWorkloadsWithUIDResolver(namespace string, resolveUID func() (string, error)) ([]string, error) {
	if d.dynClient == nil {
		return nil, fmt.Errorf("kubernetes dynamic client is not initialized")
	}
	uid, err := resolveUID()
	if err != nil {
		return nil, err
	}

	workloadList, err := listKueueWorkloads(d.dynClient, namespace, uid)
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads in namespace %s: %w", namespace, err)
	}

	var matchedWorkloads []string
	for _, item := range workloadList.Items {
		matchedWorkloads = append(matchedWorkloads, item.GetName())
	}
	return matchedWorkloads, nil
}

func (d *DefaultKubeClient) isJobSetDisabled() bool {
	if d.jobSetEnabled != nil {
		return !d.jobSetEnabled()
	}
	v, _ := shell.ParseBoolEnv(os.Getenv(disableJobSetEnvVar))
	return v
}

func (d *DefaultKubeClient) resolveWorkloadOwnerUID(namespace, workloadName string) (string, error) {
	if d.isJobSetDisabled() {
		return d.resolveBatchJobOwnerUID(namespace, workloadName, nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	jobset, err := d.dynClient.Resource(jobSetGVR).Namespace(namespace).Get(ctx, workloadName, metav1.GetOptions{})
	if err == nil {
		uid := string(jobset.GetUID())
		if uid == "" {
			return "", fmt.Errorf("jobset %s has no UID", workloadName)
		}
		return uid, nil
	}
	if !k8s.IsNotFoundOrNoMatchError(err) {
		return "", fmt.Errorf("failed to get jobset %s in namespace %s: %w", workloadName, namespace, err)
	}
	return d.resolveBatchJobOwnerUID(namespace, workloadName, err)
}

func (d *DefaultKubeClient) resolveBatchJobOwnerUID(namespace, workloadName string, jobSetErr error) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	jobObj, jobErr := d.dynClient.Resource(jobGVR).Namespace(namespace).Get(ctx, workloadName, metav1.GetOptions{})
	if jobErr != nil {
		if jobSetErr != nil && k8s.IsNotFoundOrNoMatchError(jobErr) {
			return "", fmt.Errorf("failed to get jobset %s in namespace %s: %w", workloadName, namespace, jobSetErr)
		}
		return "", fmt.Errorf("failed to get batch/v1 job %s in namespace %s: %w", workloadName, namespace, jobErr)
	}
	if jobObj.GetLabels()[workloadLabel] != workloadName {
		if jobSetErr != nil {
			return "", fmt.Errorf("failed to get jobset %s in namespace %s: %w", workloadName, namespace, jobSetErr)
		}
		return "", fmt.Errorf("batch/v1 job %s in namespace %s is not managed by gcluster", workloadName, namespace)
	}
	uid := string(jobObj.GetUID())
	if uid == "" {
		return "", fmt.Errorf("job %s has no UID", workloadName)
	}
	return uid, nil
}

// ListJobSets retrieves job statuses for JobSets matching the given label selector in the namespace.
func (d *DefaultKubeClient) ListJobSets(namespace string, labelSelector string) ([]orchestrator.JobStatus, error) {
	if d.dynClient == nil {
		return nil, fmt.Errorf("kubernetes dynamic client is not initialized")
	}
	gvr := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	ctx, cancel := context.WithTimeout(context.Background(), defaultKubeClientTimeout)
	defer cancel()
	list, err := d.dynClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		return nil, err
	}

	var jobs []orchestrator.JobStatus
	for _, item := range list.Items {
		name := item.GetName()
		creationParams := item.GetCreationTimestamp()
		creationTime := creationParams.Format(time.RFC3339)

		statusStr, completionTime := parseJobStatus(item.Object)

		jobs = append(jobs, orchestrator.JobStatus{
			Name:           name,
			Status:         statusStr,
			CreationTime:   creationTime,
			CompletionTime: completionTime,
		})
	}

	return jobs, nil
}

func (d *DefaultExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	return shell.ExecuteCommand(name, args...)
}

func (d *DefaultExecutor) ExecuteCommandWithTimeout(timeout time.Duration, name string, args ...string) shell.CommandResult {
	return shell.ExecuteCommandWithTimeout(timeout, name, args...)
}

func (d *DefaultExecutor) ExecuteCommandStream(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GetCurrentNamespace resolves the target Kubernetes namespace from kubeconfig for the specified cluster.
func (d *DefaultKubeClient) GetCurrentNamespace(clusterName, location, projectID string) (string, error) {
	loader := d.kubeconfigLoader
	if loader == nil {
		loader = k8s.DefaultKubeconfigLoader
	}
	config, err := loader()
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w. You can explicitly specify the namespace using the --gke-namespace flag", err)
	}

	// Standard GKE context naming convention (with zone-to-region fallback) first, then bare cluster name or current-context fallback.
	_, kubeCtx, _, ok := resolveGKEContext(config, clusterName, location, projectID)
	if !ok {
		trimmedCluster := strings.TrimSpace(clusterName)
		if trimmedCluster == "" {
			if config != nil && strings.TrimSpace(config.CurrentContext) != "" {
				return "", fmt.Errorf("current-context %q was not found in kubeconfig and no cluster specified. You can explicitly specify the namespace using the --gke-namespace flag", config.CurrentContext)
			}
			return "", fmt.Errorf("no current-context set in kubeconfig and no cluster specified. You can explicitly specify the namespace using the --gke-namespace flag")
		}
		return "", fmt.Errorf("no matching context found for cluster %q in kubeconfig. You can explicitly specify the namespace using the --gke-namespace flag", trimmedCluster)
	}
	return k8s.ContextNamespace(kubeCtx), nil
}
