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
	"strings"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (g *GKEOrchestrator) verifyCheckpointConfigurationCR(docRemediationMsg string) error {
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

	return nil
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

	if err := g.verifyCheckpointConfigurationCR(docRemediationMsg); err != nil {
		return err
	}
	if gsa := g.legacyMTCDriverGSA(); gsa != "" {
		logging.Warn("The MTC driver service account %s/%s still has the %s=%s annotation added by older gcluster versions. It overrides the direct Workload Identity grant (mtc_service_account_member), so checkpoint bucket mounts may fail with PermissionDenied. See \"Migrating from earlier gcluster versions\" in docs/gke-advanced-features.md.", mtcDriverNamespace, mtcDriverKSA, gcpServiceAccountAnnotation, gsa)
	}
	return nil
}
