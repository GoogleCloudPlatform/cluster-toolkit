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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/imagebuilder"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/safetext/yamltemplate"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"gopkg.in/yaml.v2"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	defaultClusterQueue = "default-queue"
	defaultLocalQueue   = "multislice-queue"

	defaultPathwaysProxyImage  = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest"
	defaultPathwaysServerImage = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest"

	// kueueAPIVersion is the GVR version used for Kueue resources.
	kueueAPIVersion = "v1beta2"
)

func NewGKEOrchestrator() *GKEOrchestrator {
	return &GKEOrchestrator{
		executor:                 &DefaultExecutor{},
		machineTypeClient:        &DefaultMachineTypeClient{},
		acceleratorToMachineType: make(map[string]string),
		machineCapCache:          make(map[string]MachineTypeCap),
		topologyCache:            make(map[string]string),
		dynamicSlicingCache:      make(map[string]bool),
		staticSlicingCache:       make(map[string]bool),
	}
}

func (g *GKEOrchestrator) SetExecutor(e Executor) {
	g.executor = e
}

func (g *GKEOrchestrator) SetDynamicClient(c dynamic.Interface) {
	g.dynClient = c
}

func (g *GKEOrchestrator) SetKubeClient(c KubeClient) {
	g.kubeClient = c
}

// SubmitJob submits a job to the GKE cluster. It processes the job definition,
// creates the required Kubernetes manifests (JobSet), and applies them to the cluster.
func (g *GKEOrchestrator) SubmitJob(job orchestrator.JobDefinition) error {
	logging.Info("Starting gcluster job submit workflow...")

	sm := &StorageManager{orchestrator: g}
	if err := sm.ValidateMounts(job.RawMounts); err != nil {
		return err
	}

	var err error
	err = g.initializeJobSubmission(&job)
	if err != nil {
		return err
	}

	if err := g.fetchClusterState(&job); err != nil {
		return err
	}

	profile, isDynamicSlicing, isStaticSlicing, err := g.resolveHardwareRequirements(&job)
	if err != nil {
		return err
	}
	if err := g.validateJobConflicts(job.WorkloadName, job.ClusterName, job.ClusterLocation, job.ProjectID); err != nil {
		return err
	}

	fullImageName, err := g.BuildContainerImage(job)
	if err != nil {
		return err
	}

	if err := g.generateAndSubmitManifests(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing); err != nil {
		return err
	}

	if job.DryRunManifest == "" {
		g.printConsoleLinks(job)
	}

	if job.AwaitJobCompletion && job.DryRunManifest == "" {
		err = g.awaitJobCompletion(job.WorkloadName, job.ClusterName, job.ClusterLocation, job.ProjectID, job.Timeout)
		if err != nil {
			return err
		}
	}
	logging.Info("gcluster job submit workflow completed.")

	return nil
}

// ListJobs retrieves a list of jobs in the GKE cluster.
// It filters jobs based on the provided ListOptions.
func (g *GKEOrchestrator) ListJobs(opts orchestrator.ListOptions) ([]orchestrator.JobStatus, error) {
	logging.Info("Listing jobs in cluster '%s'...", opts.ClusterName)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return nil, err
	}

	if _, err := g.getDynamicClient(); err != nil {
		return nil, err
	}

	list, err := g.kubeClient.ListJobSets("gcluster.google.com/workload")
	if err != nil {
		return nil, fmt.Errorf("failed to list jobsets across all namespaces: %w", err)
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

// CancelJob deletes a job from the GKE cluster by name.
// Jobs are filtered via cluster name and location provided through CancelOptions.
func (g *GKEOrchestrator) CancelJob(name string, opts orchestrator.CancelOptions) error {
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return err
	}

	if _, err := g.getDynamicClient(); err != nil {
		return fmt.Errorf("failed to initialize k8s client: %w", err)
	}

	// Find the job to get its namespace
	foundNamespace, err := g.kubeClient.GetJobNamespace(name)
	if err != nil {
		return err
	}

	status, err := g.getJobSetStatus(name, foundNamespace)
	actionVerb := "Cancel"
	if err == nil && (status == "Completed" || status == "Failed") {
		actionVerb = "Cleanup"
		logging.Info("Cleaning up resources for the '%s' job '%s' in cluster '%s'...", status, name, opts.ClusterName)
	} else {
		logging.Info("Canceling job '%s' in cluster '%s'...", name, opts.ClusterName)
	}

	err = g.kubeClient.DeleteJobSet(foundNamespace, name)
	if err != nil {
		return fmt.Errorf("%s operation failed for %s in namespace %s: %w", strings.ToLower(actionVerb), name, foundNamespace, err)
	}
	logging.Info("%s operation on Job '%s' completed successfully.", actionVerb, name)
	return nil
}

// GetJobLogs fetches the logs for a specific job in the GKE cluster.
func (g *GKEOrchestrator) GetJobLogs(name string, opts orchestrator.LogsOptions) (string, error) {
	logging.Info("Fetching logs for job '%s' in cluster '%s'...", name, opts.ClusterName)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return "", err
	}

	foundNamespace, err := g.getJobNamespace(name)
	if err != nil {
		return "", err
	}

	// Retry loop for pulling logs, especially to handle ImagePullBackOff/waiting states
	maxRetries := 12 // 12 * 5s = 1 minute timeout
	var res shell.CommandResult
	for i := 0; i < maxRetries; i++ {
		res = g.executor.ExecuteCommand("kubectl", "logs", "-n", foundNamespace, "-l", fmt.Sprintf("jobset.sigs.k8s.io/jobset-name=%s", name), "--all-containers", "--max-log-requests=50")
		if res.ExitCode == 0 {
			break
		}

		if strings.Contains(res.Stderr, "is waiting to start") {
			if i == 0 {
				logging.Info("Job containers are waiting to start (likely pulling images). Waiting...")
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return "", fmt.Errorf("failed to get logs: %s\n%s", res.Stderr, res.Stdout)
	}

	if res.ExitCode != 0 {
		return "", fmt.Errorf("timed out waiting for job to start; latest error: %s\n%s", res.Stderr, res.Stdout)
	}

	if opts.Follow {
		logging.Info("Streaming logs for job '%s'...", name)
		err := g.executor.ExecuteCommandStream("kubectl", "logs", "-n", foundNamespace, "-l", fmt.Sprintf("jobset.sigs.k8s.io/jobset-name=%s", name), "--all-containers", "-f", "--max-log-requests=10")
		return "", err
	}

	if strings.TrimSpace(res.Stdout) == "" {
		return "Job exists but has no live logs available (it may have finished or failed to start pods)", nil
	}

	return res.Stdout, nil
}

func (g *GKEOrchestrator) generateAndSubmitManifests(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) error {
	if job.IsPathwaysJob {
		manifestContent, err := g.GeneratePathwaysManifest(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
		if err != nil {
			return err
		}
		return g.ApplyManifest(manifestContent, job.DryRunManifest, job.WorkloadName)
	}

	manifestOpts, err := g.PrepareManifestOptions(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		return err
	}
	return g.generateAndApplyManifest(manifestOpts, profile, job.DryRunManifest)
}

func (g *GKEOrchestrator) printConsoleLinks(job orchestrator.JobDefinition) {
	jobName := job.WorkloadName + "-main-job-0"
	if job.IsPathwaysJob {
		jobName = job.WorkloadName + "-pathways-head-0"
	}
	gkeLink := fmt.Sprintf("https://console.cloud.google.com/kubernetes/job/%s/%s/default/%s/details?project=%s",
		job.ClusterLocation, job.ClusterName, jobName, job.ProjectID)

	logging.Info("Follow your workload details here: %s", gkeLink)

	logFilter := fmt.Sprintf(`resource.type="k8s_container"
resource.labels.project_id="%s"
resource.labels.location="%s"
resource.labels.cluster_name="%s"
resource.labels.namespace_name="default"
resource.labels.pod_name:"%s-"
severity>=DEFAULT`, job.ProjectID, job.ClusterLocation, job.ClusterName, jobName)

	encodedFilter := url.QueryEscape(logFilter)
	logsLink := fmt.Sprintf("https://console.cloud.google.com/logs/query;query=%s;storageScope=project;duration=P1D?project=%s",
		encodedFilter, job.ProjectID)

	logging.Info("View your workload logs in real-time here: %s or use gcluster job logs [job-name] to view logs using kubectl", logsLink)
}

func (g *GKEOrchestrator) validateJobConflicts(workloadName string, clusterName string, clusterLocation string, projectID string) error {
	status, err := g.getJobStatus(workloadName)
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("job with name '%s' already exists in state '%s'. You can cancel the existing job using 'gcluster job cancel %s --cluster %s --location %s --project %s' or resubmit this workload with a different name using '--name'", workloadName, status, workloadName, clusterName, clusterLocation, projectID)
	}
	return nil
}

func (g *GKEOrchestrator) GeneratePathwaysManifest(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) (string, error) {
	// Set default values for Pathways-specific fields if not provided
	if job.Pathways.ProxyServerImage == "" {
		job.Pathways.ProxyServerImage = defaultPathwaysProxyImage
	}
	if job.Pathways.ServerImage == "" {
		job.Pathways.ServerImage = defaultPathwaysServerImage
	}
	if job.Pathways.WorkerImage == "" {
		// WorkerImage defaults to ServerImage if not explicitly set
		job.Pathways.WorkerImage = job.Pathways.ServerImage
	}

	tmpl, err := yamltemplate.New("pathways_jobset.tmpl").ParseFS(templatesFS, "templates/pathways_jobset.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse pathways jobset template: %w", err)
	}

	opts, err := g.PrepareManifestOptions(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		return "", err
	}

	opts.Pathways = job.Pathways

	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(opts, profile)
	var resStr string
	if err == nil {
		resStr, err = g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit, 14)
		if err != nil {
			return "", err
		}
	} else {
		logging.Warn("Warning: failed to calculate resource limits for Pathways job: %v", err)
	}

	cmdSlice := []string{"/bin/bash", "-c", opts.CommandToRun}
	isTPU := tpuLimit != ""
	isGPU := gpuLimit != ""
	data := g.prepareJobSetTemplateData(opts, cmdSlice, resStr, isTPU, isGPU)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute pathways jobset template: %w", err)
	}

	return assembleManifest(buf.String(), opts.AdditionalManifests), nil
}

func (g *GKEOrchestrator) ApplyManifest(manifestContent, outputManifestPath, workloadName string) error {
	if outputManifestPath != "" {
		logging.Info("Saving GKE manifest to %s", outputManifestPath)
		if err := os.WriteFile(outputManifestPath, []byte(manifestContent), 0644); err != nil {
			return fmt.Errorf("failed to write GKE manifest to file %s: %w", outputManifestPath, err)
		}
		logging.Info("GKE manifest saved successfully.")
	} else {
		// Submit will fail if a job with the same name already exists.
		logging.Info("Applying GKE manifest to cluster...")
		err := g.applyManifests([]byte(manifestContent), workloadName+".yaml")
		if err != nil {
			return fmt.Errorf("failed to apply GKE manifest: %w", err)
		}
		logging.Info("GKE workload deployed successfully.")
	}
	return nil
}

func (g *GKEOrchestrator) populateClusterMetadata(job *orchestrator.JobDefinition) error {
	projectID, err := g.getProjectID(job.ProjectID)
	if err != nil {
		return err
	}
	job.ProjectID = projectID
	g.projectID = projectID

	logging.Info("Fetching GKE cluster metadata for '%s'...", job.ClusterName)
	res := g.executor.ExecuteCommand("gcloud", "container", "clusters", "describe", job.ClusterName,
		"--location", job.ClusterLocation,
		"--project", job.ProjectID,
		"--format=json")
	if res.ExitCode != 0 {
		if strings.Contains(res.Stderr, "403") || strings.Contains(strings.ToLower(res.Stderr), "permission denied") {
			return fmt.Errorf("your account lacks the required permission to access cluster '%s' in project '%s'. Please ask your project administrator to grant you the Kubernetes Engine Viewer role (roles/container.viewer)", job.ClusterName, job.ProjectID)
		}
		// If the user specified a zone (location with 3 components, e.g. us-central1-a), try to fallback to the region
		if len(strings.Split(job.ClusterLocation, "-")) == 3 {
			region := shell.ExtractRegion(job.ClusterLocation)
			logging.Info("Failed to find cluster in zone %s. Trying fallback to region %s...", job.ClusterLocation, region)
			fallbackRes := g.executor.ExecuteCommand("gcloud", "container", "clusters", "describe", job.ClusterName,
				"--location", region,
				"--project", job.ProjectID,
				"--format=json")
			if fallbackRes.ExitCode == 0 {
				logging.Warn("Cluster '%s' is a regional cluster in '%s'. Found it by falling back from zone '%s'. "+
					"Note: This does NOT restrict your job to '%s'. To run specifically in '%s', "+
					"please use the '--node-constraint topology.kubernetes.io/zone=%s' flag.",
					job.ClusterName, region, job.ClusterLocation, job.ClusterLocation, job.ClusterLocation, job.ClusterLocation)
				job.ClusterLocation = region
				res = fallbackRes
			} else {
				return fmt.Errorf("failed to describe GKE cluster %s in zone %s and fallback region %s: %s", job.ClusterName, job.ClusterLocation, region, res.Stderr)
			}
		} else {
			return fmt.Errorf("failed to describe GKE cluster %s: %s", job.ClusterName, res.Stderr)
		}
	}

	var clusterDesc gkeCluster
	if err := json.Unmarshal([]byte(res.Stdout), &clusterDesc); err != nil {
		return fmt.Errorf("failed to parse GKE cluster description: %w", err)
	}

	g.clusterZones = clusterDesc.Locations
	g.clusterDesc = clusterDesc

	g.napEnabled = clusterDesc.Autoscaling.EnableNodeAutoprovisioning
	g.napLimits = parseNAPLimits(clusterDesc.Autoscaling)

	capacity, nodePoolSAs, err := g.calculateClusterCapacity(clusterDesc, job.ClusterLocation)
	if err != nil {
		return err
	}
	g.capacity = capacity
	g.nodePoolSAs = nodePoolSAs
	logging.Info("Calculated cluster capacity: %+v", g.capacity)

	if job.IsPathwaysJob {
		if job.Pathways.HeadNodePool != "" {
			g.resolvedHeadNodePool = job.Pathways.HeadNodePool
		} else {
			g.resolvedHeadNodePool = g.autoDetectCPUNodePool()
		}
		if g.resolvedHeadNodePool == "" {
			return fmt.Errorf("failed to auto-detect a suitable CPU node pool (expected 'cpu-np' or 'pathways-np') for Pathways head job. You can explicitly specify your head node pool name using the --pathways-head-np flag")
		}
		job.Pathways.HeadNodePool = g.resolvedHeadNodePool
	}

	return nil
}

func (g *GKEOrchestrator) autoDetectCPUNodePool() string {
	for _, np := range g.clusterDesc.NodePools {
		if (np.Name == "cpu-np" || np.Name == "pathways-np") && len(np.Config.Accelerators) == 0 && !g.isSystemPool(np) {
			return np.Name
		}
	}
	return ""
}

func (g *GKEOrchestrator) isSystemPool(np gkeJobNodePool) bool {
	for _, taint := range np.Config.Taints {
		if taint.Key == "components.gke.io/gke-managed-components" && taint.Value == "true" {
			return true
		}
	}
	return false
}

func (g *GKEOrchestrator) initializeJobSubmission(job *orchestrator.JobDefinition) error {
	if err := g.populateClusterMetadata(job); err != nil {
		return err
	}

	logging.Info("Configuring kubectl for GKE cluster '%s'...", job.ClusterName)
	if err := g.configureKubectl(job.ClusterName, job.ClusterLocation, job.ProjectID); err != nil {
		return err
	}

	// Centralized Cluster Validation (Skip for dry-runs to avoid cluster mutations)
	if job.DryRunManifest == "" {
		if err := g.ValidateClusterState(job); err != nil {
			return err
		}
	}

	if err := g.configureClusterEnvironment(job); err != nil {
		return err
	}

	return nil
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

func (g *GKEOrchestrator) getEffectiveCPUs(machineType string, guestCpus int) int {
	if tpc, ok := g.machineTypeToThreadsPerCore[machineType]; ok && tpc == "1" && !strings.HasPrefix(machineType, "t2a") {
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
	if len(np.Config.Accelerators) > 0 {
		var err error
		gpus, tpus, flavor, nodeLabels, err = g.processAccelerators(np.Config.Accelerators, nodeCount, np.Config.MachineType)
		if err != nil {
			return 0, 0, 0, 0, "flavor-default", nodeLabels, sa, fmt.Errorf("in node pool %s: %w", np.Name, err)
		}
	}

	if len(np.Config.Accelerators) == 0 && len(cap.Accelerators) > 0 {
		count := cap.Accelerators[0].Count
		accType := cap.Accelerators[0].Type
		if strings.Contains(strings.ToLower(accType), "tpu") {
			tpus += count * nodeCount
			flavor = "flavor-" + strings.ToLower(accType)
			nodeLabels["cloud.google.com/gke-tpu-accelerator"] = accType
		} else {
			gpus += count * nodeCount
			flavor = "flavor-" + strings.ToLower(accType)
			nodeLabels["cloud.google.com/gke-accelerator"] = accType
		}
		if g.acceleratorToMachineType == nil {
			g.acceleratorToMachineType = make(map[string]string)
		}
		g.acceleratorToMachineType[strings.ToLower(accType)] = np.Config.MachineType
	}

	return cpus, memMb, gpus, tpus, flavor, nodeLabels, sa, nil
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
		if g.acceleratorToMachineType == nil {
			g.acceleratorToMachineType = make(map[string]string)
		}
		g.acceleratorToMachineType[strings.ToLower(acc.AcceleratorType)] = machineType
	}
	return gpus, tpus, flavor, nodeLabels, nil
}

func (g *GKEOrchestrator) configureClusterEnvironment(job *orchestrator.JobDefinition) error {
	localQueue, err := g.resolveKueueQueue(job.KueueQueueName)
	if err != nil {
		logging.Info("Warning: Failed to auto-discover Kueue Queue Name: %v. Falling back to default-queue.", err)
		localQueue = "default-queue"
	}
	job.KueueQueueName = localQueue

	if job.DryRunManifest == "" {
		if err := g.EnsureResourceFlavors(); err != nil {
			logging.Info("Warning: Failed to ensure ResourceFlavors: %v", err)
		}

		exists, err := g.checkLocalQueueExists(localQueue)
		if err != nil {
			logging.Info("Warning: Failed to check if LocalQueue exists: %v", err)
		}
		if !exists {
			promptMsg := fmt.Sprintf("LocalQueue '%s' does not exist. Do you want gcluster to create default Kueue resources (ClusterQueue and LocalQueue) with calculated cluster capacity?", localQueue)
			if shell.PromptYesNo(promptMsg) {
				if err := g.createDefaultQueues(localQueue); err != nil {
					logging.Info("Warning: Failed to create default queues: %v. Workload might remain suspended.", err)
				}
			} else {
				return fmt.Errorf("LocalQueue '%s' does not exist and user declined to create default queues. Please create one manually or specify an existing queue using --queue flag", localQueue)
			}
		}

		if job.IsPathwaysJob {
			if err := g.ensureClusterQueueCoverage(localQueue); err != nil {
				logging.Info("Warning: Could not automatically update ClusterQueue: %v. Workload might remain suspended.", err)
			}
		}
	}

	return nil
}

func (g *GKEOrchestrator) checkLocalQueueExists(name string) (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "localqueue", name, "-n", "default")
	if res.ExitCode == 0 {
		return true, nil
	}
	if strings.Contains(res.Stderr, "NotFound") || strings.Contains(res.Stderr, "not found") {
		return false, nil
	}
	return false, fmt.Errorf("failed to check localqueue status: %s", res.Stderr)
}

func (g *GKEOrchestrator) createDefaultQueues(localQueueName string) error {
	logging.Info("Creating default ClusterQueue and LocalQueue...")

	// Render and apply ClusterQueue
	clusterQueueBytes, err := g.renderClusterQueue(defaultClusterQueue)
	if err != nil {
		return fmt.Errorf("failed to render clusterqueue: %w", err)
	}
	if err := g.applyManifests(clusterQueueBytes, "cluster-queue.yaml"); err != nil {
		return fmt.Errorf("failed to apply clusterqueue: %w", err)
	}

	// Render and apply LocalQueue
	localQueueTmpl, err := yamltemplate.New("local_queue.tmpl").ParseFS(templatesFS, "templates/local_queue.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse local_queue.tmpl: %w", err)
	}
	var localQueueBuf bytes.Buffer
	if err := localQueueTmpl.Execute(&localQueueBuf, struct {
		Namespace        string
		LocalQueueName   string
		ClusterQueueName string
	}{"default", localQueueName, defaultClusterQueue}); err != nil {
		return fmt.Errorf("failed to execute local_queue.tmpl template: %w", err)
	}

	if err := g.applyManifests(localQueueBuf.Bytes(), "local-queue.yaml"); err != nil {
		return fmt.Errorf("failed to apply localqueue: %w", err)
	}

	logging.Info("Default queues created successfully.")
	return nil
}

func (g *GKEOrchestrator) ensureClusterQueueCoverage(localQueueName string) error {
	cqName, err := g.getClusterQueueName(localQueueName)
	if err != nil {
		return err
	}

	hasCoverage, isEmpty, err := g.checkClusterQueueCoverage(cqName)
	if err != nil {
		return err
	}

	if hasCoverage {
		logging.Info("Kueue ClusterQueue '%s' already covers CPU and Memory.", cqName)
		return nil
	}

	if isEmpty {
		logging.Info("ClusterQueue '%s' is empty. Applying calculated capacity...", cqName)
		clusterQueueBytes, err := g.renderClusterQueue(cqName)
		if err != nil {
			return fmt.Errorf("failed to render clusterqueue with new capacity: %w", err)
		}
		if err := g.applyManifests(clusterQueueBytes, "cluster-queue.yaml"); err != nil {
			return fmt.Errorf("failed to apply clusterqueue with new capacity: %w", err)
		}
		return nil
	}

	return fmt.Errorf("clusterQueue '%s' does not cover required resources (CPU and Memory). Please configure it manually to include quotas for 'cpu' and 'memory' resources.", cqName)
}

func (g *GKEOrchestrator) getClusterQueueName(localQueueName string) (string, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "localqueue", localQueueName, "-n", "default", "-o", "jsonpath={.spec.clusterQueue}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to find clusterqueue for %s: %s", localQueueName, res.Stderr)
	}
	cqName := strings.TrimSpace(res.Stdout)
	if cqName == "" {
		cqName = localQueueName
	}
	return cqName, nil
}

func (g *GKEOrchestrator) checkClusterQueueCoverage(cqName string) (bool, bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "clusterqueue", cqName, "-o", "json")
	if res.ExitCode != 0 {
		return false, false, fmt.Errorf("failed to get clusterqueue %s: %s", cqName, res.Stderr)
	}

	var cq map[string]interface{}
	if err := json.Unmarshal([]byte(res.Stdout), &cq); err != nil {
		return false, false, err
	}

	spec, ok := cq["spec"].(map[string]interface{})
	if !ok {
		return false, true, nil
	}
	rgList, ok := spec["resourceGroups"].([]interface{})
	if !ok || len(rgList) == 0 {
		return false, true, nil
	}

	return g.hasRequiredResources(rgList), false, nil
}

func (g *GKEOrchestrator) hasRequiredResources(rgList []interface{}) bool {
	hasCPU := false
	hasMem := false
	for _, rgItem := range rgList {
		rg, ok := rgItem.(map[string]interface{})
		if !ok {
			continue
		}
		if covered, ok := rg["coveredResources"].([]interface{}); ok {
			for _, r := range covered {
				if rStr, ok := r.(string); ok {
					if rStr == "cpu" {
						hasCPU = true
					}
					if rStr == "memory" {
						hasMem = true
					}
				}
			}
		}
	}

	return hasCPU && hasMem
}

func (g *GKEOrchestrator) getProjectID(initialProjectID string) (string, error) {
	if initialProjectID != "" {
		return initialProjectID, nil
	}

	res := g.executor.ExecuteCommand("gcloud", "config", "get-value", "project")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to get GCP project ID from gcloud config: %s", res.Stderr)
	}
	projectID := strings.TrimSpace(res.Stdout)
	if projectID == "" {
		return "", fmt.Errorf("GCP project ID is empty. Please provide it via --project flag or configure gcloud CLI.")
	}
	logging.Info("Using GCP Project ID inferred from gcloud config: %s", projectID)
	return projectID, nil
}

func (g *GKEOrchestrator) resolveKueueQueue(requestedQueueName string) (string, error) {
	if requestedQueueName != "" {
		logging.Info("Using provided Kueue LocalQueue: %s", requestedQueueName)
		return requestedQueueName, nil
	}

	res := g.executor.ExecuteCommand("kubectl", "get", "localqueue", "-n", "default", "-o", "jsonpath={.items[*].metadata.name}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to query LocalQueues: %s", res.Stderr)
	}

	output := strings.TrimSpace(res.Stdout)
	if output == "" {
		logging.Info("No LocalQueues found. Defaulting to '%s'.", defaultLocalQueue)
		return defaultLocalQueue, nil
	}

	queues := strings.Fields(output)
	if len(queues) == 1 {
		logging.Info("Auto-discovered Kueue LocalQueue: %s", queues[0])
		return queues[0], nil
	}

	return "", fmt.Errorf("multiple LocalQueues found (%v). Please specify which one to use using --queue flag", queues)
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
	if !topologies[requested] {
		contained := false
		for t := range topologies {
			fit, err := config.CheckTopologyContainment(requested, t, accelType)
			if err != nil {
				return fmt.Errorf("failed to check topology containment: %w", err)
			}
			if fit {
				contained = true
				break
			}
		}
		if !contained {
			var valid []string
			for t := range topologies {
				valid = append(valid, t)
			}
			return fmt.Errorf("requested topology %s is not valid for cluster. It must match or fit inside discovered limits: %v", requested, valid)
		}
	}
	logging.Info("Validated provided Topology: %s", requested)
	return nil
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

	res := g.executor.ExecuteCommand("kubectl", "get", "resourceflavors.kueue.x-k8s.io", "-o", "jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}", "-l", selector)
	output := strings.TrimSpace(res.Stdout)

	if output == "" {
		res = g.executor.ExecuteCommand("kubectl", "get", "nodes", "-o", "jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}", "-l", selector)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("failed to query Nodes for topology: %s", res.Stderr)
		}
		output = strings.TrimSpace(res.Stdout)
	}

	if len(nodePoolTopologies) > 0 {
		output = strings.Join(nodePoolTopologies, "\n") + "\n" + output
	}
	return strings.TrimSpace(output), nil
}

func (g *GKEOrchestrator) BuildContainerImage(job orchestrator.JobDefinition) (string, error) {
	if job.DryRunManifest != "" {
		if job.BaseImage != "" {
			logging.Info("[Dry Run] Skipping Crane build, generating predicted URI...")
			return imagebuilder.GenerateImageName(job.ProjectID, job.ClusterLocation)
		}
		if job.ImageName != "" {
			logging.Info("[Dry Run] Using pre-existing container image: %s", job.ImageName)
			return job.ImageName, nil
		}
	}

	if job.BaseImage != "" {
		logging.Info("Building container image using Crane (Go implementation) on top of %s...", job.BaseImage)

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

func (g *GKEOrchestrator) configureKubectl(clusterName, clusterLocation, projectID string) error {
	credsRes := g.executor.ExecuteCommand("gcloud", "container", "clusters", "get-credentials", clusterName, "--location", clusterLocation, "--project", projectID)
	if credsRes.ExitCode != 0 {
		if strings.Contains(strings.ToLower(credsRes.Stderr), "multiple") || strings.Contains(strings.ToLower(credsRes.Stderr), "ambiguous") {
			return fmt.Errorf("found multiple GKE clusters named %s. Please specify the exact Zone using --location to disambiguate.", clusterName)
		}
		return fmt.Errorf("failed to get GKE cluster credentials: %s\n%s", credsRes.Stderr, credsRes.Stdout)
	}
	return nil
}

func (g *GKEOrchestrator) generateAndApplyManifest(opts ManifestOptions, profile JobProfile, outputManifestPath string) error {
	logging.Info("Generating GKE manifest...")
	gkeManifestContent, err := g.GenerateGKEManifest(opts, profile)
	if err != nil {
		return fmt.Errorf("failed to generate GKE manifest: %w", err)
	}

	return g.ApplyManifest(gkeManifestContent, outputManifestPath, opts.WorkloadName)
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

func (g *GKEOrchestrator) prepareJobSetTemplateData(opts ManifestOptions, command []string, resourcesYAML string, isTPU, isGPU bool) jobSetTemplateData {
	exclusiveTopology := ""
	if !opts.IsDynamicSlicing {
		exclusiveTopology = "alpha.jobset.sigs.k8s.io/exclusive-topology: cloud.google.com/gke-nodepool"
	}

	workerBackoffLimit := 0
	if opts.Pathways.ElasticSlices > 0 {
		workerBackoffLimit = opts.Pathways.MaxSliceRestarts * opts.NodesPerSlice
	} else {
		workerBackoffLimit = opts.NodesPerSlice * 4
	}

	var proxyArgsList []string
	if opts.Pathways.ProxyArgs != "" {
		proxyArgsList = strings.Fields(opts.Pathways.ProxyArgs)
	}
	var serverArgsList []string
	if opts.Pathways.ServerArgs != "" {
		serverArgsList = strings.Fields(opts.Pathways.ServerArgs)
	}
	var workerArgsList []string
	if opts.Pathways.WorkerArgs != "" {
		workerArgsList = strings.Fields(opts.Pathways.WorkerArgs)
	}

	var containers []ContainerData
	if opts.ParallelContainers > 1 {
		for i := 0; i < opts.ParallelContainers; i++ {
			containers = append(containers, ContainerData{
				Name:          fmt.Sprintf("workload-container-%d", i+1),
				ResourcesYAML: resourcesYAML,
			})
		}
	} else {
		containers = append(containers, ContainerData{
			Name:          "workload-container",
			ResourcesYAML: resourcesYAML,
		})
	}

	return jobSetTemplateData{
		WorkloadName:                  opts.WorkloadName,
		ClusterName:                   opts.ClusterName,
		Containers:                    containers,
		ProjectID:                     opts.ProjectID,
		KueueQueueName:                opts.KueueQueueName,
		TtlSecondsAfterFinished:       opts.TtlSecondsAfterFinished,
		TerminationGracePeriodSeconds: opts.TerminationGracePeriodSeconds,
		MaxRestarts:                   opts.MaxRestarts,
		NumSlices:                     opts.NumSlices,
		NodesPerSlice:                 opts.NodesPerSlice,
		WorkerBackoffLimit:            workerBackoffLimit,
		ProxyArgsList:                 proxyArgsList,
		ServerArgsList:                serverArgsList,
		WorkerArgsList:                workerArgsList,
		PathwaysInstanceType:          opts.PathwaysInstanceType,
		CommandToRun:                  opts.CommandToRun,
		ResourcesString:               resourcesYAML,
		FullImageName:                 opts.FullImageName,
		Command:                       command,
		ResourcesYAML:                 resourcesYAML,
		AcceleratorTypeLabel:          g.GenerateGKENodeSelectorLabel(opts.ComputeType),
		NodeSelector:                  opts.NodeSelector,
		Affinity:                      opts.Affinity,
		PodFailurePolicy:              opts.PodFailurePolicy,
		ImagePullSecrets:              opts.ImagePullSecrets,
		ServiceAccountName:            opts.ServiceAccountName,
		TopologyAnnotation:            opts.TopologyAnnotation,
		SchedulerName:                 opts.SchedulerName,
		SchedulingGates:               opts.SchedulingGates,
		Tolerations:                   opts.Tolerations,
		PriorityClassName:             opts.PriorityClassName,
		VolumesYAML:                   opts.VolumesYAML,
		VolumeMountsYAML:              opts.VolumeMountsYAML,
		GCSFuseEnabled:                opts.GCSFuseEnabled,
		HostNetworkEnabled:            isTPU || isGPU,
		Pathways:                      opts.Pathways,
		ExclusiveTopologyAnnotation:   exclusiveTopology,
		Verbose:                       opts.Verbose,
		Env:                           sortedEnvVars(opts.Env),
		PathwaysProxyEnv:              sortedEnvVars(opts.Pathways.ProxyEnv),
		PathwaysServerEnv:             sortedEnvVars(opts.Pathways.ServerEnv),
		PathwaysWorkerEnv:             sortedEnvVars(opts.Pathways.WorkerEnv),
		IsTPU:                         isTPU,
		IsGPU:                         isGPU,
	}
}

func sortedEnvVars(envMap map[string]string) []EnvVar {
	if len(envMap) == 0 {
		return nil
	}
	envKeys := make([]string, 0, len(envMap))
	for k := range envMap {
		envKeys = append(envKeys, k)
	}
	slices.Sort(envKeys)
	res := make([]EnvVar, len(envKeys))
	for i, k := range envKeys {
		res[i] = EnvVar{Name: k, Value: envMap[k]}
	}
	return res
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
		cond := c.(map[string]interface{})
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		if condStatus == "True" {
			switch condType {
			case "Completed", "Succeeded":
				*statusStr = "Succeeded"
				if *completionTime == "" {
					if transitionTime, ok := cond["lastTransitionTime"].(string); ok {
						*completionTime = transitionTime
					}
				}
			case "Failed":
				*statusStr = "Failed"
				if *completionTime == "" {
					if transitionTime, ok := cond["lastTransitionTime"].(string); ok {
						*completionTime = transitionTime
					}
				}
			case "Suspended":
				*statusStr = "Suspended"
			}
		}
	}
}

func (g *GKEOrchestrator) getCurrentNamespace() (string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	ns, _, err := kubeConfig.Namespace()
	if err != nil || ns == "" {
		return "default", nil
	}
	return ns, nil
}

func (g *GKEOrchestrator) getKueueWorkloadStatus(client dynamic.Interface, ns string, uid string) (string, error) {
	gvrWl := schema.GroupVersionResource{Group: "kueue.x-k8s.io", Version: kueueAPIVersion, Resource: "workloads"}
	listOptsWl := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kueue.x-k8s.io/job-uid=%s", uid),
	}
	wlList, err := client.Resource(gvrWl).Namespace(ns).List(context.TODO(), listOptsWl)
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
	podList, err := client.Resource(gvrPod).Namespace(ns).List(context.TODO(), listOpts)
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

func (g *GKEOrchestrator) getJobStatus(name string) (string, error) {
	client, err := g.getDynamicClient()
	if err != nil {
		return "", err
	}
	gvr := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}

	optsSelector := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	}
	list, err := client.Resource(gvr).Namespace("").List(context.TODO(), optsSelector)
	if err != nil {
		return "", fmt.Errorf("failed to search for jobset %s across namespaces: %w", name, err)
	}

	if len(list.Items) == 0 {
		return "", nil
	}

	if len(list.Items) > 1 {
		return "", fmt.Errorf("found multiple jobsets named %s in different namespaces; this is not currently supported", name)
	}

	obj := list.Items[0]
	ns := obj.GetNamespace()

	metadata, ok := obj.Object["metadata"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed to get JobSet metadata")
	}
	uid, _ := metadata["uid"].(string)

	wlStatus, err := g.getKueueWorkloadStatus(client, ns, uid)
	if err == nil && (wlStatus == "QuotaReserved" || wlStatus == "Evicted") {
		return wlStatus, nil
	}

	status, _ := parseJobStatus(obj.Object)

	if status == "Running" {
		podStatus, err := g.getPodAggregatedStatus(client, ns, name)
		if err != nil {
			return status, nil // Fall back to JobSet status
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

func (g *GKEOrchestrator) generatePodFailurePolicy(exitCodes []int) (string, error) {
	if len(exitCodes) == 0 {
		return "", nil
	}

	var validCodes []int
	for _, code := range exitCodes {
		if code == 0 {
			logging.Info("Warning: Exit code 0 (success) cannot be used in PodFailurePolicy. Ignoring it.")
			continue
		}
		validCodes = append(validCodes, code)
	}

	if len(validCodes) == 0 {
		return "", nil
	}

	policy := map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"action": "FailJob",
				"onExitCodes": map[string]interface{}{
					"operator": "NotIn",
					"values":   validCodes,
				},
			},
		},
	}
	b, err := yaml.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(b), nil
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

func (g *GKEOrchestrator) getJobNamespace(name string) (string, error) {
	if g.kubeClient == nil {
		_, err := g.getDynamicClient()
		if err != nil {
			return "", fmt.Errorf("failed to get dynamic client: %w", err)
		}
	}
	return g.kubeClient.GetJobNamespace(name)
}

func (g *GKEOrchestrator) getDynamicClient() (dynamic.Interface, error) {
	if g.dynClient != nil {
		return g.dynClient, nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	g.dynClient, err = dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	if g.kubeClient == nil {
		g.kubeClient = &DefaultKubeClient{dynClient: g.dynClient}
	}
	return g.dynClient, nil
}

func (g *GKEOrchestrator) awaitJobCompletion(workloadName, clusterName, clusterLocation, projectID, timeout string) error {
	logging.Info("Waiting for job '%s' to complete...", workloadName)

	if g.kubeClient == nil {
		_, err := g.getDynamicClient() // ensure kubeClient is initialized
		if err != nil {
			return fmt.Errorf("failed to get dynamic client: %w", err)
		}
	}

	ns, err := g.kubeClient.GetJobNamespace(workloadName)
	if err != nil {
		return fmt.Errorf("failed to get job namespace: %w", err)
	}

	jobConsoleLink := fmt.Sprintf("https://console.cloud.google.com/kubernetes/workload/gke/%s/%s/details/%s?project=%s",
		clusterLocation, clusterName, workloadName, projectID)

	targetWorkloadName, err := g.findTargetWorkload(ns, workloadName)
	if err != nil {
		return err
	}

	err = g.waitWorkloadFinished(targetWorkloadName, ns, timeout, jobConsoleLink, workloadName)
	if err != nil {
		return err
	}

	logging.Info("Job '%s' has finished. Checking final status...", workloadName)

	status, err := g.getJobSetStatus(workloadName, ns)
	if err != nil {
		return err
	}

	if status != "Completed" {
		logging.Error("Job '%s' finished with status '%s'. Check details in the Cloud Console: %s", workloadName, status, jobConsoleLink)
		return fmt.Errorf("job completed unsuccessfully with status: %s", status)
	}

	logging.Info("Job '%s' completed successfully. View details in the Cloud Console: %s", workloadName, jobConsoleLink)
	return nil
}

func (g *GKEOrchestrator) getJobSetStatus(workloadName, ns string) (string, error) {
	statusRes := g.executor.ExecuteCommand("kubectl", "get", "jobset", workloadName, "-n", ns, "-o", "json")
	if statusRes.ExitCode != 0 {
		return "", fmt.Errorf("failed to get final job status: %s\n%s", statusRes.Stderr, statusRes.Stdout)
	}

	var jsStatus JobSetStatus
	if err := json.Unmarshal([]byte(statusRes.Stdout), &jsStatus); err != nil {
		return "", fmt.Errorf("failed to parse jobset status JSON: %w", err)
	}

	var latestCondition JobSetCondition
	var latestTime time.Time

	for _, cond := range jsStatus.Status.Conditions {
		if cond.LastTransitionTime == "" {
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

	if latestCondition.Type == "" {
		return "", fmt.Errorf("no valid conditions found for jobset %s", workloadName)
	}

	return latestCondition.Type, nil
}

func (g *GKEOrchestrator) findTargetWorkload(ns, workloadName string) (string, error) {
	matchedWorkloads, err := g.kubeClient.ListWorkloads(ns, workloadName)
	if err != nil {
		return "", fmt.Errorf("failed to list workloads: %w", err)
	}

	var targetWorkloadName string
	if len(matchedWorkloads) > 0 {
		targetWorkloadName = matchedWorkloads[len(matchedWorkloads)-1]
	}

	if targetWorkloadName == "" {
		return "", fmt.Errorf("failed to find Kueue workload for jobset %s", workloadName)
	}
	return targetWorkloadName, nil
}

func (g *GKEOrchestrator) waitWorkloadFinished(targetWorkloadName, ns, timeout, jobConsoleLink, workloadName string) error {
	logging.Info("Waiting for Kueue workload '%s' to be Finished...", targetWorkloadName)
	waitRes := g.executor.ExecuteCommand("kubectl", "wait", "--for=condition=Finished",
		"workload", targetWorkloadName, "-n", ns, "--timeout="+timeout)

	if waitRes.ExitCode != 0 {
		if strings.Contains(waitRes.Stderr, "timed out waiting") || strings.Contains(waitRes.Stdout, "timed out waiting") {
			logging.Error("Timed out waiting for job '%s' to finish. Check its status in the Cloud Console: %s", workloadName, jobConsoleLink)
			return fmt.Errorf("job timed out")
		}
		return fmt.Errorf("error waiting for job completion: %s\n%s", waitRes.Stderr, waitRes.Stdout)
	}
	return nil
}

func (g *GKEOrchestrator) addAcceleratorLabel(nodeSelector map[string]string, accelLabel string, isCPUMachine bool, machineType string) {
	if accelLabel != "" && !isCPUMachine {
		if config.IsTPU(machineType) {
			nodeSelector["cloud.google.com/gke-tpu-accelerator"] = accelLabel
		} else {
			nodeSelector["cloud.google.com/gke-accelerator"] = accelLabel
		}
	}
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

func (g *GKEOrchestrator) buildNodeSelector(schedOpts SchedulingOptions, job orchestrator.JobDefinition, isCPUMachine bool) (string, error) {
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

	cap, err := g.FetchMachineCapabilities(job.MachineType, job.ClusterLocation)
	if err != nil {
		return "", fmt.Errorf("failed to fetch machine capabilities: %w", err)
	}

	var accelLabel string
	if len(cap.Accelerators) > 0 {
		accelLabel = cap.Accelerators[0].Type
	}

	isGPU := !isCPUMachine && !config.IsTPU(job.MachineType)

	g.addAcceleratorLabel(nodeSelector, accelLabel, isCPUMachine, job.MachineType)

	if err := g.addTopologyLabel(nodeSelector, schedOpts, isGPU, isCPUMachine); err != nil {
		return "", err
	}

	if len(nodeSelector) > 0 {
		b, err := yaml.Marshal(nodeSelector)
		if err != nil {
			return "", fmt.Errorf("failed to marshal nodeSelector: %w", err)
		}
		return indentYaml(string(b), 16), nil
	}
	return "", nil
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

func (d *DefaultKubeClient) GetJobNamespace(workloadName string) (string, error) {
	gvr := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	optsSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("gcluster.google.com/workload=%s", workloadName),
	}
	list, err := d.dynClient.Resource(gvr).Namespace("").List(context.TODO(), optsSelector)
	if err != nil {
		return "", fmt.Errorf("failed to search for jobset %s across namespaces: %w", workloadName, err)
	}

	if len(list.Items) == 1 {
		return list.Items[0].GetNamespace(), nil
	} else if len(list.Items) > 1 {
		return "", fmt.Errorf("found multiple jobsets named %s in different namespaces; this is not currently supported. Please ensure job names are unique across the cluster", workloadName)
	}
	return "", fmt.Errorf("jobset %s not found in any namespace", workloadName)
}

func (d *DefaultKubeClient) DeleteJobSet(namespace string, name string) error {
	gvr := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	return d.dynClient.Resource(gvr).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (d *DefaultKubeClient) ListWorkloads(namespace string, workloadName string) ([]string, error) {
	// First, retrieve the JobSet to get its UID
	jobsetGVR := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	jobset, err := d.dynClient.Resource(jobsetGVR).Namespace(namespace).Get(context.TODO(), workloadName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get jobset %s in namespace %s: %w", workloadName, namespace, err)
	}

	uid := string(jobset.GetUID())
	if uid == "" {
		return nil, fmt.Errorf("jobset %s has no UID", workloadName)
	}

	// Now list workloads using the UID label selector
	workloadGVR := schema.GroupVersionResource{Group: "kueue.x-k8s.io", Version: kueueAPIVersion, Resource: "workloads"}
	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kueue.x-k8s.io/job-uid=%s", uid),
	}
	workloadList, err := d.dynClient.Resource(workloadGVR).Namespace(namespace).List(context.TODO(), listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads in namespace %s: %w", namespace, err)
	}

	var matchedWorkloads []string
	for _, item := range workloadList.Items {
		matchedWorkloads = append(matchedWorkloads, item.GetName())
	}
	return matchedWorkloads, nil
}

func (d *DefaultKubeClient) ListJobSets(labelSelector string) ([]orchestrator.JobStatus, error) {
	gvr := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	list, err := d.dynClient.Resource(gvr).Namespace("").List(context.Background(), metav1.ListOptions{
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

func (d *DefaultExecutor) ExecuteCommandStream(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
