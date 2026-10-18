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
	"hpc-toolkit/pkg/logging"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// Keep in sync with the jobset.version default in modules/management/kubectl-apply/variables.tf (no 'v' prefix there).
const defaultJobSetVersion = "v0.12.0"

func (g *GKEOrchestrator) checkAndInstallJobSetCRD() error {
	if installed, err := g.isJobSetCRDInstalled(); err != nil {
		return err
	} else if installed {
		logging.Info("JobSet CRD found. Verifying Webhook health...")
		cmdEndpoints := g.kubectl("get", "endpointslice", "-l", "kubernetes.io/service-name=jobset-webhook-service", "-n", "jobset-system", "-o", "json")
		if cmdEndpoints.ExitCode == 0 {
			var eps k8sEndpointSliceList
			if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err != nil {
				logging.Warn("Failed to parse JobSet endpointslice JSON: %v", err)
			} else if eps.HasReadyEndpoint() {
				logging.Info("JobSet Webhook is healthy.")
				return nil
			}
		} else if strings.Contains(strings.ToLower(cmdEndpoints.Stderr), "forbidden") {
			logging.Warn("Insufficient RBAC permissions to read JobSet webhook endpoints (403 Forbidden). Assuming JobSet is healthy in shared cluster.")
			return nil
		}
		logging.Info("JobSet Webhook endpoints not found. Proceeding with re-installation/fix...")
	}

	jobSetManifestsURL := fmt.Sprintf("https://github.com/kubernetes-sigs/jobset/releases/download/%s/manifests.yaml", defaultJobSetVersion)
	return g.installJobSetCRD(jobSetManifestsURL)
}

func (g *GKEOrchestrator) installJobSetCRD(jobSetManifestsURL string) error {
	logging.Info("Installing/Fixing JobSet CRD and Webhook...")

	manifestBytes, err := g.downloadManifests(jobSetManifestsURL)
	if err != nil {
		return err
	}

	cleanedManifests, err := g.cleanJobSetManifests(manifestBytes)
	if err != nil {
		return err
	}

	if err := g.applyManifests(cleanedManifests, "jobset.yaml"); err != nil {
		return err
	}

	logging.Info("JobSet components applied successfully.")

	return g.waitForJobSetWebhook()
}

type k8sEndpointSliceList struct {
	Items []struct {
		Endpoints []struct {
			Addresses  []string `json:"addresses"`
			Conditions struct {
				Ready bool `json:"ready"`
			} `json:"conditions"`
		} `json:"endpoints"`
	} `json:"items"`
}

func (eps *k8sEndpointSliceList) HasReadyEndpoint() bool {
	for _, item := range eps.Items {
		for _, ep := range item.Endpoints {
			if ep.Conditions.Ready && len(ep.Addresses) > 0 {
				return true
			}
		}
	}
	return false
}

func (g *GKEOrchestrator) waitForJobSetWebhook() error {
	logging.Info("Waiting for JobSet webhook service to be ready...")
	res := g.kubectl("rollout", "status", "deployment/jobset-controller-manager", "-n", "jobset-system", "--timeout=600s")
	if res.ExitCode != 0 {
		return fmt.Errorf("jobset controller manager failed to become ready: %s\n%s", res.Stderr, res.Stdout)
	}

	logging.Info("Verifying JobSet webhook service endpoints...")
	for i := 0; i < 40; i++ {
		cmdEndpoints := g.kubectl("get", "endpointslice", "-l", "kubernetes.io/service-name=jobset-webhook-service", "-n", "jobset-system", "-o", "json")
		if cmdEndpoints.ExitCode == 0 {
			var eps k8sEndpointSliceList
			if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err != nil {
				return fmt.Errorf("failed to unmarshal jobset endpointslice json: %w", err)
			}
			if eps.HasReadyEndpoint() {
				logging.Info("JobSet webhook service endpoints are available.")
				return nil
			}
		}
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("timed out waiting for jobset-webhook-service endpoints to be available")
}

func (g *GKEOrchestrator) isJobSetCRDInstalled() (bool, error) {
	res := g.kubectl("get", "crd", "jobsets.jobset.x-k8s.io")
	if res.ExitCode == 0 {
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr + " " + res.Stdout)
	if strings.Contains(errStr, "not found") || strings.Contains(errStr, "notfound") {
		logging.Info("JobSet CRD not found.")
		return false, nil
	}
	if strings.Contains(errStr, "forbidden") {
		logging.Warn("Insufficient RBAC permissions to read JobSet CRD (403 Forbidden). Assuming JobSet is installed in shared cluster.")
		return true, nil
	}
	return false, fmt.Errorf("failed to check for JobSet CRD: %s\n%s", res.Stderr, res.Stdout)
}

func (g *GKEOrchestrator) getHTTPClient() HTTPClient {
	g.httpOnce.Do(func() {
		if g.httpClient == nil {
			g.httpClient = &http.Client{Timeout: 30 * time.Second}
		}
	})
	return g.httpClient
}

func (g *GKEOrchestrator) downloadManifests(url string) ([]byte, error) {
	logging.Info("Downloading manifests from %s", url)
	resp, err := g.getHTTPClient().Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download manifests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download manifests: received status code %d", resp.StatusCode)
	}

	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests: %w", err)
	}
	return manifestBytes, nil
}

func (g *GKEOrchestrator) cleanJobSetManifests(manifestBytes []byte) ([]byte, error) {
	logging.Info("Cleaning JobSet manifests (removing description fields)...")
	return g.cleanAndProcessManifests(manifestBytes, func(data map[interface{}]interface{}) {
		g.injectTolerationsAndLabels(data)
	})
}

func (g *GKEOrchestrator) cleanAndProcessManifests(manifestBytes []byte, processFn func(map[interface{}]interface{})) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(manifestBytes))
	var cleanedManifests bytes.Buffer

	for {
		var doc interface{}
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode YAML document: %w", err)
		}

		if doc == nil {
			continue
		}

		if data, ok := doc.(map[interface{}]interface{}); ok {
			g.removeDescriptionFields(data)
			if processFn != nil {
				processFn(data)
			}
			cleanedBytes, err := yaml.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal cleaned YAML: %w", err)
			}
			cleanedManifests.Write(cleanedBytes)
			cleanedManifests.WriteString("---\n")
		} else {
			cleanedBytes, err := yaml.Marshal(doc)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal YAML document: %w", err)
			}
			cleanedManifests.Write(cleanedBytes)
			cleanedManifests.WriteString("---\n")
		}
	}
	return cleanedManifests.Bytes(), nil
}

func (g *GKEOrchestrator) injectTolerationsAndLabels(data map[interface{}]interface{}) {
	kind, ok := data["kind"].(string)
	if !ok || kind != "Deployment" {
		return
	}

	meta, ok := data["metadata"].(map[interface{}]interface{})
	if !ok {
		return
	}
	name, ok := meta["name"].(string)
	if !ok || (name != "jobset-controller-manager" && name != "jobset-controller") {
		return
	}

	spec, ok := data["spec"].(map[interface{}]interface{})
	if !ok {
		return
	}
	template, ok := spec["template"].(map[interface{}]interface{})
	if !ok {
		return
	}
	podSpec, ok := template["spec"].(map[interface{}]interface{})
	if !ok {
		return
	}

	tolerations := []interface{}{
		map[interface{}]interface{}{
			"key":      "nvidia.com/gpu",
			"operator": "Exists",
			"effect":   "NoSchedule",
		},
		map[interface{}]interface{}{
			"key":      "components.gke.io/gke-managed-components",
			"operator": "Exists",
			"effect":   "NoSchedule",
		},
	}

	if existingTolerations, ok := podSpec["tolerations"].([]interface{}); ok {
		podSpec["tolerations"] = append(existingTolerations, tolerations...)
	} else {
		podSpec["tolerations"] = tolerations
	}

	if podMeta, ok := template["metadata"].(map[interface{}]interface{}); ok {
		labels, ok := podMeta["labels"].(map[interface{}]interface{})
		if !ok {
			labels = make(map[interface{}]interface{})
			podMeta["labels"] = labels
		}
		labels["app.kubernetes.io/instance"] = "jobset"
		labels["app.kubernetes.io/name"] = "jobset"
		labels["control-plane"] = "controller-manager"
		labels["app.kubernetes.io/component"] = "controller-manager"
	}
}

func (g *GKEOrchestrator) applyManifests(manifests []byte, filename string) error {
	logging.Info("Applying manifests for %s...", filename)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, ".gcluster", "generated")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for generated manifests at %q. Please check your file system permissions for this path: %w", stateDir, err)
	}

	filePath := filepath.Join(stateDir, filename)
	if err := os.WriteFile(filePath, manifests, 0644); err != nil {
		return fmt.Errorf("failed to write manifests to %s: %w", filePath, err)
	}
	logging.Info("Manifests saved to %s", filePath)

	res := g.kubectl("apply", "-f", filePath)
	if res.ExitCode != 0 {
		return fmt.Errorf("kubectl apply failed with exit code %d: %s\n%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	logging.Info("Manifests applied successfully.")
	return nil
}

func (g *GKEOrchestrator) removeDescriptionFields(data map[interface{}]interface{}) {
	for key, value := range data {
		if key == "description" {
			delete(data, key)
			continue
		}
		if subMap, ok := value.(map[interface{}]interface{}); ok {
			g.removeDescriptionFields(subMap)
		} else if subList, ok := value.([]interface{}); ok {
			for _, item := range subList {
				if itemMap, ok := item.(map[interface{}]interface{}); ok {
					g.removeDescriptionFields(itemMap)
				}
			}
		}
	}
}

// ValidateClusterState runs all cluster-specific validations to fail early on invalid state.
func (g *GKEOrchestrator) ValidateClusterState(job *orchestrator.JobDefinition) error {
	validators := []func() error{
		func() error {
			return g.validateTargetNamespaceExists(job.ClusterName, job.ClusterLocation, job.ProjectID)
		},
		func() error { return g.CheckAndInstallKueue("", job.ClusterName, job.ClusterLocation) },
		g.checkAndInstallJobSetCRD,
	}

	if job.PriorityClassName != "" {
		validators = append(validators, func() error {
			return g.ensureAndValidatePriorityClass(job.PriorityClassName)
		})
	}

	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (g *GKEOrchestrator) getKubeClient() KubeClient {
	g.syncKubeClient()
	return g.kubeClient
}

// getCurrentNamespace is the single source of truth for the target namespace.
// It returns the cached namespace (seeded from --gke-namespace at every
// orchestrator entry point) or, if unset, resolves and caches the namespace of
// the pinned kubeconfig context, falling back to a lookup by cluster name.
func (g *GKEOrchestrator) getCurrentNamespace(clusterName, location, projectID string) (string, error) {
	if g.namespace != "" {
		return g.namespace, nil
	}

	if ns, ok := g.pinnedContextNamespace(); ok {
		g.namespace = ns
		return ns, nil
	}
	ns, err := g.getKubeClient().GetCurrentNamespace(clusterName, location, projectID)
	if err != nil {
		return "", err
	}
	g.namespace = ns
	return ns, nil
}

// pinnedContextNamespace reads the namespace of g.kubeContext from kubeconfig.
func (g *GKEOrchestrator) pinnedContextNamespace() (string, bool) {
	if g.kubeContext == "" {
		return "", false
	}
	cfg, err := g.loadKubeconfig()
	if err != nil {
		return "", false
	}
	kubeCtx := cfg.Contexts[g.kubeContext]
	if kubeCtx == nil {
		return "", false
	}
	if kubeCtx.Namespace == "" {
		return "default", true
	}
	return kubeCtx.Namespace, true
}

func (g *GKEOrchestrator) validateTargetNamespaceExists(clusterName, location, projectID string) error {
	ns, err := g.getCurrentNamespace(clusterName, location, projectID)
	if err != nil {
		return err
	}

	if ns == "" {
		return fmt.Errorf("target namespace cannot be empty for GKE cluster %q. Please pass --gke-namespace, or set a default namespace in your kubeconfig context (e.g., 'kubectl config set-context --current --namespace=<namespace>')", clusterName)
	}

	client, err := g.getDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to initialize Kubernetes client for namespace validation: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.Resource(namespaceGVR).Get(ctx, ns, metav1.GetOptions{})
	switch {
	case err == nil:
		return nil
	case apierrors.IsNotFound(err):
		return fmt.Errorf("target namespace %q does not exist on GKE cluster %q. Please create the namespace first (e.g., 'kubectl create namespace %s')", ns, clusterName, ns)
	case isForbiddenError(err):
		logging.Warn("Insufficient RBAC permissions to verify existence of namespace %q on cluster %q (403 Forbidden). Proceeding with job submission...", ns, clusterName)
		return nil
	default:
		return fmt.Errorf("failed to verify existence of namespace %q on cluster %q: %w", ns, clusterName, err)
	}
}

// Initialize fetches GKE cluster metadata and resolves the cluster location,
// handling regional fallback if necessary.
func (g *GKEOrchestrator) Initialize(clusterName, location, projectID string) (string, error) {
	g.projectID = projectID

	logging.Info("Fetching GKE cluster metadata for '%s'...", clusterName)
	res := g.executor.ExecuteCommandWithTimeout(shell.DefaultCloudAPITimeout, "gcloud", "container", "clusters", "describe", clusterName,
		"--location", location,
		"--project", g.projectID,
		"--format=json")

	timeoutMsg := fmt.Sprintf("timed out after %v while trying to reach GKE cluster '%s' in '%s'. Please check your network connection", shell.DefaultCloudAPITimeout, clusterName, location)
	if err := shell.HandleExecError(res, "gcloud", timeoutMsg); err != nil {
		return "", err
	}

	if res.ExitCode != 0 {
		if strings.Contains(res.Stderr, "403") || strings.Contains(strings.ToLower(res.Stderr), "permission denied") {
			return "", fmt.Errorf("your account lacks the required permission to access cluster '%s' in project '%s'. Please ask your project administrator to grant you the Kubernetes Engine Viewer role (roles/container.viewer)", clusterName, g.projectID)
		}
		// If the user specified a zone (location with 3 components, e.g. us-central1-a), try to fallback to the region
		if len(strings.Split(location, "-")) == 3 {
			region := shell.ExtractRegion(location)
			logging.Info("Failed to find cluster in zone %s. Trying fallback to region %s...", location, region)
			fallbackRes := g.executor.ExecuteCommandWithTimeout(shell.DefaultCloudAPITimeout, "gcloud", "container", "clusters", "describe", clusterName,
				"--location", region,
				"--project", g.projectID,
				"--format=json")

			fallbackTimeoutMsg := fmt.Sprintf("timed out after %v while trying to reach GKE cluster '%s' in fallback region '%s'. Please check your network connection", shell.DefaultCloudAPITimeout, clusterName, region)
			if err := shell.HandleExecError(fallbackRes, "gcloud", fallbackTimeoutMsg); err != nil {
				return "", err
			}

			if fallbackRes.ExitCode == 0 {
				logging.Warn("Cluster '%s' is a regional cluster in '%s'. Found it by falling back from zone '%s'. "+
					"Note: This does NOT restrict your job to '%s'. To run specifically in '%s', "+
					"please use the '--node-constraint topology.kubernetes.io/zone=%s' flag.",
					clusterName, region, location, location, location, location)
				location = region
				res = fallbackRes
			} else {
				return "", fmt.Errorf("failed to describe GKE cluster %s in zone %s and fallback region %s: %s", clusterName, location, region, res.Stderr)
			}
		} else {
			return "", fmt.Errorf("failed to describe GKE cluster %s: %s", clusterName, res.Stderr)
		}
	}

	var clusterDesc gkeCluster
	if err := json.Unmarshal([]byte(res.Stdout), &clusterDesc); err != nil {
		return "", fmt.Errorf("failed to parse GKE cluster description: %w", err)
	}

	g.clusterZones = clusterDesc.Locations
	g.clusterDesc = clusterDesc

	g.napEnabled = clusterDesc.Autoscaling.EnableNodeAutoprovisioning
	g.napLimits = parseNAPLimits(clusterDesc.Autoscaling)

	return location, nil
}

func (g *GKEOrchestrator) configureClusterEnvironment(job *orchestrator.JobDefinition) error {
	ns, err := g.getCurrentNamespace(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		return err
	}

	explicitQueue := job.KueueQueueName != ""
	localQueue, discovered, err := g.resolveKueueQueue(job.KueueQueueName, ns)
	if err != nil {
		if job.DryRunManifest == "" {
			return err
		}
		logging.Info("Warning: Failed to auto-discover Kueue Queue Name: %v. Falling back to %s for dry-run.", err, defaultLocalQueue)
		localQueue = defaultLocalQueue
	}
	job.KueueQueueName = localQueue

	if job.DryRunManifest != "" {
		return nil
	}
	if err := g.ensureLocalQueueExists(localQueue, ns, discovered, explicitQueue); err != nil {
		return err
	}
	if job.IsPathwaysJob {
		return g.ensureClusterQueueCoverage(localQueue, ns)
	}
	return nil
}

// ensureLocalQueueExists verifies that the specified LocalQueue exists in the namespace, prompting to create it if missing.
func (g *GKEOrchestrator) ensureLocalQueueExists(localQueue, ns string, discovered, explicitQueue bool) error {
	if discovered {
		return nil
	}
	if explicitQueue {
		exists, err := g.checkLocalQueueExists(localQueue, ns)
		if err != nil {
			return fmt.Errorf("failed to check if LocalQueue exists: %w", err)
		}
		if exists {
			return nil
		}
	}
	var availableStr string
	if explicitQueue {
		if availableQueues := g.listLocalQueues(ns); len(availableQueues) > 0 {
			availableStr = fmt.Sprintf(" Available LocalQueues in namespace '%s': %s.", ns, strings.Join(availableQueues, ", "))
		}
	}
	promptMsg := fmt.Sprintf("LocalQueue '%s' does not exist in namespace '%s'.%s Do you want gcluster to create default Kueue resources (ClusterQueue and LocalQueue) with calculated cluster capacity?", localQueue, ns, availableStr)
	if !shell.PromptYesNo(promptMsg) {
		return fmt.Errorf("LocalQueue '%s' does not exist in namespace '%s' and user declined to create default queues.%s Please create one manually or specify an existing queue using --queue flag", localQueue, ns, availableStr)
	}
	return g.createDefaultQueues(localQueue, ns)
}

// isForbiddenError returns true if the error indicates a 403 Forbidden RBAC permission error.
func isForbiddenError(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsForbidden(err) || strings.Contains(strings.ToLower(err.Error()), "forbidden")
}

func (g *GKEOrchestrator) verifyCheckpointConfigurationCR(job *orchestrator.JobDefinition, docRemediationMsg string) error {
	client, err := g.getDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to initialize dynamic client to verify CheckpointConfiguration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checkpointConfigList, err := client.Resource(checkpointConfigurationGVR).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("the CheckpointConfiguration CustomResourceDefinition (CRD) is not registered on the cluster. %s: %w", docRemediationMsg, err)
		}
		if isForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to verify CheckpointConfiguration resources (403 Forbidden). Assuming CheckpointConfiguration is configured in shared cluster and proceeding with job submission.")
			return nil
		}
		return fmt.Errorf("failed to verify CheckpointConfiguration resource: %w", err)
	}
	if len(checkpointConfigList.Items) == 0 {
		return fmt.Errorf("Multi-Tier Checkpointing (MTC) requires a CheckpointConfiguration resource to be deployed on the cluster. With Cluster Toolkit, set checkpoint_configuration on the kubectl-apply module. %s", docRemediationMsg)
	}

	return g.validateCheckpointConfigurationCompatibility(client, &checkpointConfigList.Items[0], job)
}

// mtcCandidateNodePools returns the node pools pinned with --node-constraint cloud.google.com/gke-nodepool,
// or else the non-system node pools with the job's machine type.
func (g *GKEOrchestrator) mtcCandidateNodePools(nodePoolConstraint, machineType string) []*gkeJobNodePool {
	var pinned []string
	if nodePoolConstraint != "" {
		for _, name := range strings.Split(nodePoolConstraint, "|") {
			pinned = append(pinned, strings.TrimSpace(name))
		}
	}
	var pools []*gkeJobNodePool
	for i := range g.clusterDesc.NodePools {
		np := &g.clusterDesc.NodePools[i]
		match := slices.Contains(pinned, np.Name)
		if len(pinned) == 0 {
			match = machineType != "" && !g.isSystemPool(*np) && strings.EqualFold(np.Config.MachineType, machineType)
		}
		if match {
			pools = append(pools, np)
		}
	}
	return pools
}

// hasAccelerators reports whether the node pool or machine type has accelerators attached.
func (g *GKEOrchestrator) hasAccelerators(np *gkeJobNodePool, machineType, location string) bool {
	if np != nil && len(np.Config.Accelerators) > 0 {
		return true
	}
	if machineType == "" {
		return false
	}
	cap, err := g.FetchMachineCapabilities(machineType, location)
	return err == nil && len(cap.Accelerators) > 0
}

func tolerationMatchesTaint(tol map[string]interface{}, taint gkeTaint) bool {
	effect, _ := tol["effect"].(string)
	if effect != "" && effect != taint.Effect {
		return false
	}
	op, _ := tol["operator"].(string)
	key, _ := tol["key"].(string)
	val, _ := tol["value"].(string)

	switch op {
	case "Exists":
		return key == "" || key == taint.Key
	case "", "Equal":
		return key == taint.Key && val == taint.Value
	default:
		return false
	}
}

func tolerationsCoverTaint(rawTolerations []interface{}, taint gkeTaint) bool {
	for _, item := range rawTolerations {
		if tol, ok := item.(map[string]interface{}); ok && tolerationMatchesTaint(tol, taint) {
			return true
		}
	}
	return false
}

func nodeSelectorMatchesPool(crName string, nodeSelector map[string]string, np *gkeJobNodePool) error {
	if want := nodeSelector[nodePoolLabel]; want != "" && want != np.Name {
		return fmt.Errorf("CheckpointConfiguration %q targets %s=%q in spec.nodeSelector, which does not match the job's node pool %q; the multitier-driver DaemonSet will not run on the job's nodes", crName, nodePoolLabel, want, np.Name)
	}
	for k, v := range nodeSelector {
		if actual, ok := np.Config.Labels[k]; ok && actual != v {
			return fmt.Errorf("CheckpointConfiguration %q targets %s=%q in spec.nodeSelector, which does not match label %s=%q on node pool %q; the multitier-driver DaemonSet will not run on the job's nodes", crName, k, v, k, actual, np.Name)
		}
	}
	return nil
}

// checkpointConfigCoversPool returns an error if the CheckpointConfiguration's nodeSelector or
// tolerations keep the multitier driver off the node pool (np is nil when no pool is known).
func (g *GKEOrchestrator) checkpointConfigCoversPool(crName string, nodeSelector map[string]string, tolerations []interface{}, np *gkeJobNodePool, machineType, location string) error {
	var taints []gkeTaint
	if np != nil {
		machineType = np.Config.MachineType
		if err := nodeSelectorMatchesPool(crName, nodeSelector, np); err != nil {
			return err
		}
		for _, t := range np.Config.Taints {
			if t.Effect == "NoSchedule" || t.Effect == "NoExecute" {
				taints = append(taints, t)
			}
		}
	}
	// GKE taints accelerator nodes without listing the taint in the node pool config;
	// as elsewhere in gcluster, a non-TPU machine with accelerators is a GPU machine.
	if config.IsTPU(machineType) {
		taints = append(taints, gkeTaint{Key: "google.com/tpu", Value: "present", Effect: "NoSchedule"})
	} else if g.hasAccelerators(np, machineType, location) {
		taints = append(taints, gkeTaint{Key: "nvidia.com/gpu", Value: "present", Effect: "NoSchedule"})
	}
	for _, t := range taints {
		if !tolerationsCoverTaint(tolerations, t) {
			return fmt.Errorf("CheckpointConfiguration %q is missing a toleration for taint \"%s=%s:%s\" on the target nodes; the multitier-driver DaemonSet will not schedule on those nodes. Add a matching toleration to your CheckpointConfiguration (e.g. under checkpoint_configuration.tolerations in your blueprint) and redeploy", crName, t.Key, t.Value, t.Effect)
		}
	}
	return nil
}

func (g *GKEOrchestrator) validateCheckpointConfigurationCompatibility(client dynamic.Interface, cr *unstructured.Unstructured, job *orchestrator.JobDefinition) error {
	crName := cr.GetName()
	machineType := job.MachineType
	if machineType == "" && job.ComputeType != "" {
		// Best effort: resolveHardwareRequirements reports compute types that do not resolve.
		machineType, _ = g.resolveJobMachineType(job.ComputeType)
	}
	nodeSelector, _, _ := unstructured.NestedStringMap(cr.Object, "spec", "nodeSelector")
	tolerations, _, _ := unstructured.NestedSlice(cr.Object, "spec", "tolerations")

	if instType := nodeSelector["node.kubernetes.io/instance-type"]; instType != "" && machineType != "" && !strings.EqualFold(instType, machineType) {
		return fmt.Errorf("CheckpointConfiguration %q targets node.kubernetes.io/instance-type=%q in spec.nodeSelector, which does not match the job's machine type %q; the multitier-driver DaemonSet will not run on the job's nodes. Update checkpoint_configuration.node_selector in your blueprint and redeploy", crName, instType, machineType)
	}

	// The job can land on any candidate pool, so one pool the driver covers is enough.
	pools := g.mtcCandidateNodePools(job.NodeConstraint[nodePoolLabel], machineType)
	var err error
	if len(pools) == 0 {
		err = g.checkpointConfigCoversPool(crName, nodeSelector, tolerations, nil, machineType, job.ClusterLocation)
	}
	for _, np := range pools {
		if err = g.checkpointConfigCoversPool(crName, nodeSelector, tolerations, np, machineType, job.ClusterLocation); err == nil {
			break
		}
	}
	if err != nil {
		return err
	}

	uid := string(cr.GetUID())
	if uid == "" {
		return nil
	}
	canScaleFromZero := g.napEnabled || slices.ContainsFunc(pools, func(np *gkeJobNodePool) bool {
		return np.Autoscaling.Enabled && np.Autoscaling.MinNodeCount == 0
	})
	return waitForMTCDaemonSetReady(client, crName, uid, canScaleFromZero)
}

var (
	mtcDaemonSetPollInterval = 5 * time.Second
	mtcDaemonSetPollTimeout  = 2 * time.Minute
)

// daemonSetOwnedByOtherCR reports whether ds has a CheckpointConfiguration ownerReference with a different UID.
func daemonSetOwnedByOtherCR(ds *unstructured.Unstructured, uid string) bool {
	for _, ref := range ds.GetOwnerReferences() {
		if ref.Kind == "CheckpointConfiguration" && string(ref.UID) != "" && string(ref.UID) != uid {
			return true
		}
	}
	return false
}

// lookupMTCDaemonSet finds the multitier driver DaemonSet of the CheckpointConfiguration with the given UID.
func lookupMTCDaemonSet(ctx context.Context, client dynamic.Interface, uid string) (*unstructured.Unstructured, error) {
	dsClient := client.Resource(daemonsetGVR).Namespace(mtcDriverNamespace)
	for _, name := range []string{"multitier-driver-" + uid, "multitier-driver"} {
		ds, err := dsClient.Get(ctx, name, metav1.GetOptions{})
		if !apierrors.IsNotFound(err) {
			return ds, err
		}
	}
	dsList, err := dsClient.List(ctx, metav1.ListOptions{LabelSelector: "k8s-app=high-scale-checkpointing"})
	if err != nil {
		return nil, err
	}
	for i := range dsList.Items {
		if dsList.Items[i].GetDeletionTimestamp() == nil && !daemonSetOwnedByOtherCR(&dsList.Items[i], uid) {
			return &dsList.Items[i], nil
		}
	}
	return nil, apierrors.NewNotFound(daemonsetGVR.GroupResource(), "multitier-driver-"+uid)
}

// mtcDaemonSetNotReady returns why the multitier driver DaemonSet is not ready, or nil if it is.
func mtcDaemonSetNotReady(ds *unstructured.Unstructured, crName string, canScaleFromZero bool) error {
	gen, _, _ := unstructured.NestedInt64(ds.Object, "metadata", "generation")
	obsGen, _, _ := unstructured.NestedInt64(ds.Object, "status", "observedGeneration")
	desired, _, _ := unstructured.NestedInt64(ds.Object, "status", "desiredNumberScheduled")
	ready, _, _ := unstructured.NestedInt64(ds.Object, "status", "numberReady")
	updated, hasUpdated, _ := unstructured.NestedInt64(ds.Object, "status", "updatedNumberScheduled")
	switch {
	case obsGen < gen: // Also true while the status is not populated yet.
		return fmt.Errorf("the MTC CSI driver DaemonSet %s/%s is still reconciling generation %d (observedGeneration=%d)", mtcDriverNamespace, ds.GetName(), gen, obsGen)
	case desired == 0 && canScaleFromZero:
		return nil
	case desired == 0:
		return fmt.Errorf("the MTC CSI driver DaemonSet %s/%s has 0 desired pods (desiredNumberScheduled=0); verify that CheckpointConfiguration %q nodeSelector and tolerations match your target nodes", mtcDriverNamespace, ds.GetName(), crName)
	case ready < desired || (hasUpdated && updated < desired):
		return fmt.Errorf("the MTC CSI driver DaemonSet %s/%s has %d of %d pods ready; wait for the driver pods to become Ready or check 'kubectl describe pods -n %s' (common causes: missing bucket IAM permissions or disabled Hierarchical Namespace on the checkpoint bucket)", mtcDriverNamespace, ds.GetName(), ready, desired, mtcDriverNamespace)
	}
	return nil
}

// waitForMTCDaemonSetReady polls until all multitier driver pods are ready. As in the other
// gcluster wait loops, a 403 skips the check and any other error is retried until the deadline.
func waitForMTCDaemonSetReady(client dynamic.Interface, crName, uid string, canScaleFromZero bool) error {
	dsName := "multitier-driver-" + uid
	deadline := time.Now().Add(mtcDaemonSetPollTimeout)
	loggedWait := false
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ds, err := lookupMTCDaemonSet(ctx, client, uid)
		cancel()

		var lastErr error
		switch {
		case isForbiddenError(err):
			logging.Warn("Insufficient RBAC permissions to verify MTC driver DaemonSet %s/%s (403 Forbidden). Proceeding with job submission.", mtcDriverNamespace, dsName)
			return nil
		case apierrors.IsNotFound(err):
			lastErr = fmt.Errorf("the MTC CSI driver DaemonSet %q was not found in namespace %q for CheckpointConfiguration %q (uid %s); verify that the HighScaleCheckpointing controller is healthy (`kubectl get daemonset -n %s`)", dsName, mtcDriverNamespace, crName, uid, mtcDriverNamespace)
		case err != nil:
			lastErr = fmt.Errorf("failed to verify MTC driver DaemonSet %s/%s: %w", mtcDriverNamespace, dsName, err)
		default:
			if lastErr = mtcDaemonSetNotReady(ds, crName, canScaleFromZero); lastErr == nil {
				return nil
			}
		}

		if time.Now().Add(mtcDaemonSetPollInterval).After(deadline) {
			return lastErr
		}
		if !loggedWait {
			logging.Info("Waiting up to %s for MTC driver DaemonSet %s/%s to become ready...", mtcDaemonSetPollTimeout, mtcDriverNamespace, dsName)
			loggedWait = true
		}
		time.Sleep(mtcDaemonSetPollInterval)
	}
}

// The GKE-managed MTC driver runs as this Kubernetes service account.
const (
	mtcDriverNamespace = "gke-managed-checkpointing"
	mtcDriverKSA       = "gke-checkpointing-multitier-node"
	// gcpServiceAccountAnnotation makes GKE authenticate a Kubernetes service
	// account as the annotated Google service account.
	gcpServiceAccountAnnotation = "iam.gke.io/gcp-service-account"
)

// legacyMTCDriverGSA returns the GCP service account that older gcluster
// versions annotated on the MTC driver's Kubernetes service account, or "" if
// the annotation is absent or cannot be read.
func (g *GKEOrchestrator) legacyMTCDriverGSA() string {
	client, err := g.getDynamicClient()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ksa, err := client.Resource(serviceAccountGVR).Namespace(mtcDriverNamespace).Get(ctx, mtcDriverKSA, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return ksa.GetAnnotations()[gcpServiceAccountAnnotation]
}

// validateMTCJobFlags checks the MTC flags; SubmitJob runs it before any cluster call.
func (g *GKEOrchestrator) validateMTCJobFlags(job *orchestrator.JobDefinition) error {
	if !job.GKEMTCEnabled {
		if job.GKEMTCRamdiskDirectory != "" {
			return fmt.Errorf("--gke-mtc-ramdisk-dir requires --gke-mtc-enabled")
		}
		return nil
	}
	if strings.TrimSpace(job.GKEMTCRamdiskDirectory) == "" {
		return fmt.Errorf("ramdisk directory path (--gke-mtc-ramdisk-dir) cannot be empty when Multi-Tier Checkpointing (MTC) is enabled (e.g. --gke-mtc-ramdisk-dir=/tmp/mtc_checkpoints)")
	}
	sm := &StorageManager{orchestrator: g}
	return sm.ValidateRamdiskDir(job.GKEMTCRamdiskDirectory, job.RawMounts, job.IsPathwaysJob, job.Pathways.ColocatedPythonSidecarImage != "")
}

func (g *GKEOrchestrator) validateMTCConfig(job *orchestrator.JobDefinition) error {
	if job == nil || !job.GKEMTCEnabled {
		return nil
	}

	mtcDocURL := "https://cloud.google.com/kubernetes-engine/docs/how-to/machine-learning/training/multi-tier-checkpointing"
	docRemediationMsg := fmt.Sprintf("Please follow the official GKE documentation to enable this feature on your cluster: %s", mtcDocURL)

	if g.clusterDesc.AddonsConfig == nil || g.clusterDesc.AddonsConfig.HighScaleCheckpointingConfig == nil || !g.clusterDesc.AddonsConfig.HighScaleCheckpointingConfig.Enabled {
		return fmt.Errorf("Multi-Tier Checkpointing (MTC) requires the HighScaleCheckpointing addon to be enabled on the target GKE cluster. With Cluster Toolkit, set enable_multi_tier_checkpointing: true (and enable_gcsfuse_csi: true) on the gke-cluster module. %s", docRemediationMsg)
	}

	if job.DryRunManifest != "" {
		return nil
	}

	if err := g.verifyCheckpointConfigurationCR(job, docRemediationMsg); err != nil {
		return err
	}
	if gsa := g.legacyMTCDriverGSA(); gsa != "" {
		logging.Warn("The MTC driver service account %s/%s still has the %s=%s annotation added by older gcluster versions. It overrides the direct Workload Identity grant (mtc_service_account_member), so checkpoint bucket mounts may fail with PermissionDenied. See \"Migrating from earlier gcluster versions\" in docs/gke-advanced-features.md.", mtcDriverNamespace, mtcDriverKSA, gcpServiceAccountAnnotation, gsa)
	}
	return nil
}
