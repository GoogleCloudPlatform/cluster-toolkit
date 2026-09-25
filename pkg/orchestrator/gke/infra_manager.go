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
	"errors"
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
		g.checkClusterConnectivity,
		func() error { return g.validateTargetNamespaceExists(job) },
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

// checkClusterConnectivity verifies that we can connect to the cluster.
// It uses a short timeout to fail fast if IP is blocked by authorized networks.
func (g *GKEOrchestrator) checkClusterConnectivity() error {
	logging.Info("Checking cluster connectivity...")
	res := g.executor.ExecuteCommand("kubectl", "version", "--request-timeout=5s")
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to connect to GKE cluster. Please verify your IP is allowed in the cluster's authorized networks or that you have correct network access. Error: %s", res.Stderr)
	}
	logging.Info("Cluster connectivity verified.")
	return nil
}

// Initialize fetches GKE cluster metadata and resolves the cluster location,
// handling regional fallback if necessary.
func (g *GKEOrchestrator) Initialize(clusterName, location, projectID string) (string, error) {
	g.projectID = projectID

	timeoutDuration := 30 * time.Second

	logging.Info("Fetching GKE cluster metadata for '%s'...", clusterName)
	res := g.executor.ExecuteCommandWithTimeout(timeoutDuration, "gcloud", "container", "clusters", "describe", clusterName,
		"--location", location,
		"--project", g.projectID,
		"--format=json")

	if res.Err != nil {
		if errors.Is(res.Err, context.DeadlineExceeded) {
			return "", fmt.Errorf("timed out after %v while trying to reach GKE cluster '%s' in '%s'. Please check your network connection", timeoutDuration, clusterName, location)
		}
		if res.ExitCode == -1 {
			return "", fmt.Errorf("failed to execute gcloud: %w", res.Err)
		}
	}

	if res.ExitCode != 0 {
		if strings.Contains(res.Stderr, "403") || strings.Contains(strings.ToLower(res.Stderr), "permission denied") {
			return "", fmt.Errorf("your account lacks the required permission to access cluster '%s' in project '%s'. Please ask your project administrator to grant you the Kubernetes Engine Viewer role (roles/container.viewer)", clusterName, g.projectID)
		}
		// If the user specified a zone (location with 3 components, e.g. us-central1-a), try to fallback to the region
		if len(strings.Split(location, "-")) == 3 {
			region := shell.ExtractRegion(location)
			logging.Info("Failed to find cluster in zone %s. Trying fallback to region %s...", location, region)
			fallbackRes := g.executor.ExecuteCommandWithTimeout(timeoutDuration, "gcloud", "container", "clusters", "describe", clusterName,
				"--location", region,
				"--project", g.projectID,
				"--format=json")

			if fallbackRes.Err != nil {
				if errors.Is(fallbackRes.Err, context.DeadlineExceeded) {
					return "", fmt.Errorf("failed to describe GKE cluster %s in %s: %w", clusterName, location, res.Err)
				}
				if fallbackRes.ExitCode == -1 {
					return "", fmt.Errorf("failed to describe GKE cluster %s in %s: %w", clusterName, location, res.Err)
				}
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

func (g *GKEOrchestrator) configureKubectl(clusterName, clusterLocation, projectID string) error {
	// 1. Capture current namespace context before gcloud resets it on best effort to preserve current namespace.
	// If we can't read it, using 'default' allows gcloud setup to proceed,
	originalNamespace, err := g.getCurrentNamespace(clusterName, clusterLocation, projectID)
	if err != nil {
		logging.Warn("Could not read current namespace before gcloud (defaulting to 'default'): %v. If you want to target a specific namespace please use the --gke-namespace flag", err)
		originalNamespace = "default"
	}

	// 2. Refresh credentials via gcloud (this resets namespace to 'default')
	if err := g.refreshGKEAuth(clusterName, clusterLocation, projectID); err != nil {
		return err
	}

	// 3. Restore the original namespace context
	return g.restoreNamespaceContext(originalNamespace)
}

// shouldUseDNSEndpoint determines whether to pass --dns-endpoint to gcloud container clusters get-credentials.
// When a public IP endpoint is enabled, we prefer the IP endpoint to avoid HTTP 431 request header overflow issues on enterprise networks.
// When only external DNS access is permitted without a public IP endpoint, we use the DNS endpoint.
func shouldUseDNSEndpoint(cfg *controlPlaneEndpointsConfig) bool {
	if cfg == nil || cfg.DnsEndpointConfig == nil || !cfg.DnsEndpointConfig.AllowExternalTraffic {
		return false
	}
	return cfg.IPEndpointsConfig == nil || !cfg.IPEndpointsConfig.EnablePublicEndpoint
}

// refreshGKEAuth handles the gcloud container clusters get-credentials call.
func (g *GKEOrchestrator) refreshGKEAuth(clusterName, clusterLocation, projectID string) error {
	args := []string{"container", "clusters", "get-credentials", clusterName, "--location", clusterLocation, "--project", projectID}

	if shouldUseDNSEndpoint(g.clusterDesc.ControlPlaneEndpointsConfig) {
		args = append(args, "--dns-endpoint")
	}

	credsRes := g.executor.ExecuteCommand("gcloud", args...)
	if credsRes.ExitCode != 0 {
		if strings.Contains(strings.ToLower(credsRes.Stderr), "multiple") || strings.Contains(strings.ToLower(credsRes.Stderr), "ambiguous") {
			return fmt.Errorf("found multiple GKE clusters named %s. Please specify the exact Zone using --location to disambiguate", clusterName)
		}
		return fmt.Errorf("failed to get GKE cluster credentials: %s\n%s", credsRes.Stderr, credsRes.Stdout)
	}
	return nil
}

// restoreNamespaceContext sets the current namespace back to its original value if needed.
func (g *GKEOrchestrator) restoreNamespaceContext(namespace string) error {
	// If it was default, gcloud already set it to default, so we can skip.
	if namespace == "" || namespace == "default" {
		return nil
	}

	logging.Info("Restoring namespace context to '%s'...", namespace)
	restoreRes := g.executor.ExecuteCommand("kubectl", "config", "set-context", "--current", "--namespace="+namespace)
	if restoreRes.ExitCode != 0 {
		return fmt.Errorf("failed to restore namespace context to %s: %s", namespace, restoreRes.Stderr)
	}
	return nil
}
