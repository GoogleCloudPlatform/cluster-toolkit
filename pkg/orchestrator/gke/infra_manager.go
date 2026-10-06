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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const defaultJobSetVersion = "v0.10.1"

func (g *GKEOrchestrator) checkAndInstallJobSetCRD() error {
	if installed, err := g.isJobSetCRDInstalled(); err != nil {
		return err
	} else if installed {
		logging.Info("JobSet CRD found. Verifying Webhook health...")
		cmdEndpoints := g.executor.ExecuteCommand("kubectl", "get", "endpointslice", "-l", "kubernetes.io/service-name=jobset-webhook-service", "-n", "jobset-system", "-o", "json")
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
	res := g.executor.ExecuteCommand("kubectl", "rollout", "status", "deployment/jobset-controller-manager", "-n", "jobset-system", "--timeout=600s")
	if res.ExitCode != 0 {
		return fmt.Errorf("jobset controller manager failed to become ready: %s\n%s", res.Stderr, res.Stdout)
	}

	logging.Info("Verifying JobSet webhook service endpoints...")
	for i := 0; i < 40; i++ {
		cmdEndpoints := g.executor.ExecuteCommand("kubectl", "get", "endpointslice", "-l", "kubernetes.io/service-name=jobset-webhook-service", "-n", "jobset-system", "-o", "json")
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
	res := g.executor.ExecuteCommand("kubectl", "get", "crd", "jobsets.jobset.x-k8s.io")
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

	replaceDeprecatedRbacProxyImage(podSpec)

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

// replaceDeprecatedRbacProxyImage replaces the deprecated image "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1"
// with "quay.io/brancz/kube-rbac-proxy:v0.13.1" in the JobSet controller pods to avoid deployment failures
// due to GCR container registry deprecation.
//
// TODO: Remove this helper function once the default JobSet version/manifest is upgraded.
func replaceDeprecatedRbacProxyImage(podSpec map[interface{}]interface{}) {
	replaceInContainerList := func(containerKey string) {
		containers, ok := podSpec[containerKey].([]interface{})
		if !ok {
			return
		}
		for _, c := range containers {
			containerMap, ok := c.(map[interface{}]interface{})
			if !ok {
				continue
			}
			img, ok := containerMap["image"].(string)
			const deprecatedProxyPrefix = "gcr.io/kubebuilder/kube-rbac-proxy"
			if ok && (img == deprecatedProxyPrefix || strings.HasPrefix(img, deprecatedProxyPrefix+":") || strings.HasPrefix(img, deprecatedProxyPrefix+"@")) {
				suffix := strings.TrimPrefix(img, deprecatedProxyPrefix)
				newImg := "quay.io/brancz/kube-rbac-proxy" + suffix
				containerMap["image"] = newImg
				logging.Info("Replaced deprecated image %s with %s in %s", img, newImg, containerKey)
			}
		}
	}

	replaceInContainerList("containers")
	replaceInContainerList("initContainers")
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

	res := g.executor.ExecuteCommand("kubectl", "apply", "-f", filePath)
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
		func() error { return g.checkClusterConnectivity(job.ClusterName) },
		func() error {
			return g.validateTargetNamespaceExists(job.ClusterName, job.ClusterLocation, job.ProjectID)
		},
		func() error { return g.CheckAndInstallKueue("", job.ClusterName, job.ClusterLocation) },
		g.checkAndInstallJobSetCRD,
	}

	if job.PriorityClassName != "" {
		validators = append(validators,
			func() error { return g.ensurePriorityClassesInstalled() },
			func() error { return g.validatePriorityClass(job.PriorityClassName) },
		)
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
// orchestrator entry point) or, if unset, resolves and caches the kubeconfig
// context namespace.
func (g *GKEOrchestrator) getCurrentNamespace(clusterName, location, projectID string) (string, error) {
	if g.namespace != "" {
		return g.namespace, nil
	}

	ns, err := g.getKubeClient().GetCurrentNamespace(clusterName, location, projectID)
	if err != nil {
		return "", err
	}
	g.namespace = ns
	return ns, nil
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
	res := g.executor.ExecuteCommand("gcloud", "container", "clusters", "describe", clusterName,
		"--location", location,
		"--project", g.projectID,
		"--format=json")
	if res.ExitCode != 0 {
		if strings.Contains(res.Stderr, "403") || strings.Contains(strings.ToLower(res.Stderr), "permission denied") {
			return "", fmt.Errorf("your account lacks the required permission to access cluster '%s' in project '%s'. Please ask your project administrator to grant you the Kubernetes Engine Viewer role (roles/container.viewer)", clusterName, g.projectID)
		}
		// If the user specified a zone (location with 3 components, e.g. us-central1-a), try to fallback to the region
		if len(strings.Split(location, "-")) == 3 {
			region := shell.ExtractRegion(location)
			logging.Info("Failed to find cluster in zone %s. Trying fallback to region %s...", location, region)
			fallbackRes := g.executor.ExecuteCommand("gcloud", "container", "clusters", "describe", clusterName,
				"--location", region,
				"--project", g.projectID,
				"--format=json")
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

	localQueue, err := g.resolveKueueQueue(job.KueueQueueName, ns)
	if err != nil {
		if job.DryRunManifest == "" {
			return err
		}
		logging.Info("Warning: Failed to auto-discover Kueue Queue Name: %v. Falling back to %s for dry-run.", err, defaultLocalQueue)
		localQueue = defaultLocalQueue
	}
	job.KueueQueueName = localQueue

	if job.DryRunManifest == "" {
		exists, err := g.checkLocalQueueExists(localQueue, ns)
		if err != nil {
			return fmt.Errorf("failed to check if LocalQueue exists: %w", err)
		}
		if !exists {
			availableQueues := g.listLocalQueues(ns)
			availableStr := ""
			if len(availableQueues) > 0 {
				availableStr = fmt.Sprintf(" Available LocalQueues in namespace '%s': %s.", ns, strings.Join(availableQueues, ", "))
			}
			promptMsg := fmt.Sprintf("LocalQueue '%s' does not exist in namespace '%s'.%s Do you want gcluster to create default Kueue resources (ClusterQueue and LocalQueue) with calculated cluster capacity?", localQueue, ns, availableStr)
			if shell.PromptYesNo(promptMsg) {
				if err := g.createDefaultQueues(localQueue, ns); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("LocalQueue '%s' does not exist in namespace '%s' and user declined to create default queues.%s Please create one manually or specify an existing queue using --queue flag", localQueue, ns, availableStr)
			}
		}

		if job.IsPathwaysJob {
			if err := g.ensureClusterQueueCoverage(localQueue, ns); err != nil {
				return err
			}
		}
	}

	return nil
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
		return fmt.Errorf("Multi-Tier Checkpointing (MTC) requires a CheckpointConfiguration resource to be deployed on the cluster. %s", docRemediationMsg)
	}

	return nil
}

func (g *GKEOrchestrator) validateMTCConfig(job *orchestrator.JobDefinition) error {
	if job == nil || !job.GKEMTCEnabled {
		return nil
	}

	mtcDocURL := "https://cloud.google.com/kubernetes-engine/docs/how-to/multi-tier-checkpointing"
	docRemediationMsg := fmt.Sprintf("Please follow the official GKE documentation to enable this feature on your cluster: %s", mtcDocURL)

	if job.GKEMTCRamdiskDirectory == "" {
		return fmt.Errorf("ramdisk directory path (--gke-mtc-ramdisk-dir) cannot be empty when Multi-Tier Checkpointing (MTC) is enabled")
	}

	sm := &StorageManager{orchestrator: g}
	if err := sm.ValidateRamdiskDir(job.GKEMTCRamdiskDirectory, job.RawMounts); err != nil {
		return err
	}

	if g.clusterDesc.AddonsConfig == nil || g.clusterDesc.AddonsConfig.HighScaleCheckpointingConfig == nil || !g.clusterDesc.AddonsConfig.HighScaleCheckpointingConfig.Enabled {
		return fmt.Errorf("Multi-Tier Checkpointing (MTC) requires the HighScaleCheckpointing addon to be enabled on the target GKE cluster. %s", docRemediationMsg)
	}

	if job.DryRunManifest != "" {
		return nil
	}

	if err := g.verifyCheckpointConfigurationCR(docRemediationMsg); err != nil {
		return err
	}

	return g.ensureMTCWorkloadIdentity()
}

// getNodeServiceAccount discovers the custom Google Service Account (GSA) used by node pools in the cluster.
func (g *GKEOrchestrator) getNodeServiceAccount() string {
	for _, np := range g.clusterDesc.NodePools {
		if np.Config.ServiceAccount != "" && np.Config.ServiceAccount != "default" {
			logging.Info("Discovered node service account '%s' (from node pool '%s') for MTC Workload Identity", np.Config.ServiceAccount, np.Name)
			return np.Config.ServiceAccount
		}
	}
	return ""
}

func isDaemonSetRolloutComplete(dsObj *unstructured.Unstructured) bool {
	if dsObj == nil || dsObj.Object == nil {
		return false
	}
	gen, _, _ := unstructured.NestedInt64(dsObj.Object, "metadata", "generation")
	obsGen, _, _ := unstructured.NestedInt64(dsObj.Object, "status", "observedGeneration")
	desired, _, _ := unstructured.NestedInt64(dsObj.Object, "status", "desiredNumberScheduled")
	ready, _, _ := unstructured.NestedInt64(dsObj.Object, "status", "numberReady")
	updated, _, _ := unstructured.NestedInt64(dsObj.Object, "status", "updatedNumberScheduled")

	if desired > 0 && obsGen >= gen && ready >= desired && updated >= desired {
		logging.Info("MTC multitier-driver DaemonSet is ready (%d/%d nodes ready).", ready, desired)
		return true
	}
	return false
}

func getMTCDaemonSet(ctx context.Context, client dynamic.Interface, namespace string) (*unstructured.Unstructured, error) {
	// First attempt direct get of "multitier-driver" in case it exists with static name.
	if dsObj, err := client.Resource(daemonsetGVR).Namespace(namespace).Get(ctx, "multitier-driver", metav1.GetOptions{}); err == nil {
		if dsObj.GetName() == "" {
			dsObj.SetName("multitier-driver")
		}
		return dsObj, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	// In GKE, HighScaleCheckpointing names the DaemonSet "multitier-driver-<instanceHandle>".
	// List DaemonSets in the namespace to find it.
	dsList, err := client.Resource(daemonsetGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		if strings.HasPrefix(ds.GetName(), "multitier-driver") || ds.GetLabels()["k8s-app"] == "high-scale-checkpointing" {
			return ds, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "daemonsets"}, "multitier-driver")
}

var daemonSetPollInterval = 2 * time.Second

// waitForDaemonSetRollout polls the DaemonSet until its rollout is complete or context times out.
func waitForDaemonSetRollout(ctx context.Context, client dynamic.Interface, namespace, dsName string) {
	waitCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	ticker := time.NewTicker(daemonSetPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			logging.Warn("Timed out or context canceled waiting for MTC multitier-driver DaemonSet %s in %s to become ready. Proceeding with job submission.", dsName, namespace)
			return
		case <-ticker.C:
			currentObj, err := client.Resource(daemonsetGVR).Namespace(namespace).Get(waitCtx, dsName, metav1.GetOptions{})
			if err != nil {
				if isForbiddenError(err) {
					logging.Warn("Insufficient RBAC permissions to get multitier-driver DaemonSet %s in %s status (403 Forbidden). Proceeding with job submission.", dsName, namespace)
					return
				}
				logging.Warn("Retrying get of multitier-driver DaemonSet %s: %v", dsName, err)
				continue
			}
			if isDaemonSetRolloutComplete(currentObj) {
				return
			}
		}
	}
}

// restartMTCDriverPods restarts the multitier-driver DaemonSet to pick up updated service account tokens.
func restartMTCDriverPods(ctx context.Context, client dynamic.Interface, namespace string) {
	dsObj, err := getMTCDaemonSet(ctx, client, namespace)
	if err != nil {
		if isForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to get multitier-driver DaemonSet in %s (403 Forbidden). Skipping driver restart.", namespace)
			return
		}
		if apierrors.IsNotFound(err) {
			logging.Warn("MTC multitier-driver DaemonSet not found in %s. Skipping driver restart.", namespace)
			return
		}
		logging.Warn("Failed to get multitier-driver DaemonSet in %s: %v. Skipping driver restart.", namespace, err)
		return
	}

	dsName := dsObj.GetName()
	patchData := fmt.Appendf(nil, `{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().UTC().Format(time.RFC3339))
	_, patchErr := client.Resource(daemonsetGVR).Namespace(namespace).Patch(ctx, dsName, types.StrategicMergePatchType, patchData, metav1.PatchOptions{})
	if patchErr == nil {
		logging.Info("Triggered rolling restart of %s DaemonSet in %s", dsName, namespace)
		waitForDaemonSetRollout(ctx, client, namespace, dsName)
		return
	}
	if isForbiddenError(patchErr) {
		logging.Warn("Insufficient RBAC permissions to restart %s DaemonSet in %s (403 Forbidden). Skipping driver restart.", dsName, namespace)
		return
	}

	logging.Warn("Failed to patch DaemonSet %s for restart: %v. Falling back to pod deletion.", dsName, patchErr)
	deleteMTCDriverPodsFallback(ctx, client, namespace)
	waitForDaemonSetRollout(ctx, client, namespace, dsName)
}

// deleteMTCDriverPodsFallback deletes MTC driver pods to trigger recreation by the DaemonSet controller
// when strategic merge patch is rejected by cluster admission webhooks. Per Pillar 33, it executes only
// after DaemonSet existence is verified and handles 403 Forbidden defensively.
func deleteMTCDriverPodsFallback(ctx context.Context, client dynamic.Interface, namespace string) {
	listOpts := metav1.ListOptions{
		LabelSelector: "k8s-app=high-scale-checkpointing",
	}
	pods, err := client.Resource(podGVR).Namespace(namespace).List(ctx, listOpts)
	if err != nil {
		if isForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to list multitier-driver pods in %s (403 Forbidden). Skipping driver pod restart.", namespace)
			return
		}
		logging.Warn("Failed to list multitier-driver pods in %s for restart: %v", namespace, err)
		return
	}
	if len(pods.Items) == 0 {
		if fallbackPods, err := client.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			pods = fallbackPods
		}
	}
	podsDeleted := 0
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.GetName(), "multitier-driver") {
			logging.Info("Restarting MTC driver pod: %s", pod.GetName())
			if err := client.Resource(podGVR).Namespace(namespace).Delete(ctx, pod.GetName(), metav1.DeleteOptions{}); err != nil {
				if isForbiddenError(err) {
					logging.Warn("Insufficient RBAC permissions to delete MTC driver pod %s (403 Forbidden).", pod.GetName())
					continue
				}
				logging.Warn("Failed to delete MTC driver pod %s: %v", pod.GetName(), err)
			} else {
				podsDeleted++
			}
		}
	}
	if podsDeleted > 0 {
		// Brief pause to allow the DaemonSet controller to observe the pod deletions
		// and decrement status.numberReady before we poll for readiness.
		time.Sleep(daemonSetPollInterval)
	}
}

// updateMTCServiceAccountAnnotation updates the MTC KSA with the node GSA Workload Identity annotation and restarts driver pods.
func updateMTCServiceAccountAnnotation(ctx context.Context, client dynamic.Interface, saObj *unstructured.Unstructured, namespace, name, nodeSA string) {
	const wiAnnotation = "iam.gke.io/gcp-service-account"
	annotations, _, _ := unstructured.NestedStringMap(saObj.Object, "metadata", "annotations")
	if annotations == nil {
		annotations = make(map[string]string)
	}

	if annotations[wiAnnotation] == nodeSA {
		logging.Info("[MTC Verification] ServiceAccount %s/%s already has Workload Identity annotation for GSA %s (provisioned during cluster creation). No annotation update or driver pod restart needed.", namespace, name, nodeSA)
		return
	}

	logging.Info("Updating MTC ServiceAccount %s/%s with Workload Identity annotation for GSA %s", namespace, name, nodeSA)
	annotations[wiAnnotation] = nodeSA
	if err := unstructured.SetNestedStringMap(saObj.Object, annotations, "metadata", "annotations"); err != nil {
		logging.Warn("Failed to set annotations on MTC ServiceAccount %s/%s: %v", namespace, name, err)
		return
	}
	if _, err := client.Resource(serviceAccountGVR).Namespace(namespace).Update(ctx, saObj, metav1.UpdateOptions{}); err != nil {
		if isForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to update MTC ServiceAccount %s/%s (403 Forbidden). Assuming Workload Identity is managed by cluster administrator.", namespace, name)
			return
		}
		logging.Warn("Failed to update MTC ServiceAccount %s/%s with Workload Identity annotation: %v", namespace, name, err)
		return
	}
	restartMTCDriverPods(ctx, client, namespace)
}

// ensureMTCWorkloadIdentity ensures the MTC Kubernetes ServiceAccount is annotated with the cluster node GSA and restarts driver pods if updated.
func (g *GKEOrchestrator) ensureMTCWorkloadIdentity() error {
	nodeSA := g.getNodeServiceAccount()
	if nodeSA == "" {
		logging.Warn("No custom GKE node service account detected. Automated Workload Identity configuration for Multi-Tier Checkpointing (MTC) will be skipped. You may need to manually configure IAM permissions for the MTC service account.")
		return nil
	}

	logging.Info("[MTC Verification] Checking Workload Identity configuration for MTC ServiceAccount in cluster against node GSA %s...", nodeSA)

	client, err := g.getDynamicClient()
	if err != nil {
		logging.Warn("Failed to get dynamic client for MTC Workload Identity verification: %v", err)
		return nil
	}

	// Allow sufficient deadline for SA mutation (15s) and subsequent DaemonSet rollout wait (90s).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const mtcNamespace = "gke-managed-checkpointing"
	const mtcKSA = "gke-checkpointing-multitier-node"

	saObj, err := client.Resource(serviceAccountGVR).Namespace(mtcNamespace).Get(ctx, mtcKSA, metav1.GetOptions{})
	if err != nil {
		if isForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to read MTC ServiceAccount %s/%s (403 Forbidden). Assuming Workload Identity is configured by cluster administrator.", mtcNamespace, mtcKSA)
			return nil
		}
		logging.Warn("Failed to get MTC ServiceAccount %s/%s: %v", mtcNamespace, mtcKSA, err)
		return nil
	}

	updateMTCServiceAccountAnnotation(ctx, client, saObj, mtcNamespace, mtcKSA, nodeSA)
	return nil
}
