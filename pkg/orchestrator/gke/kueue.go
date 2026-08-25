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
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"

	"gopkg.in/yaml.v2"
)

// -----------------------------------------------------------------------------
// 1. Kueue Package Constants
// -----------------------------------------------------------------------------

// defaultKueueVersion is the fallback version of Kueue to install.
// ATTENTION: For cluster creation the corresponding default version
// is in modules/management/kubectl-apply/variables.tf.
const defaultKueueVersion = "v0.15.2"

const (
	defaultLocalQueue    = "default"
	defaultClusterQueue  = "default"
	multisliceLocalQueue = "multislice-queue"

	// kueueAPIVersion is the GVR version used for Kueue resources.
	kueueAPIVersion = "v1beta2"

	clusterQueueManifestFile = "cluster-queue.yaml"
	localQueueManifestFile   = "local-queue.yaml"
	localQueueTemplateFile   = "local_queue.tmpl"

	pathwaysCPUNominalQuota       = "999999"
	pathwaysMemoryNominalQuota    = "999999T"
	kueueControllerRolloutTimeout = "--timeout=600s"
)

var kueuePermissionChecks = []struct {
	verb     string
	resource string
}{
	{"create", "namespaces"},
	{"create", "customresourcedefinitions.apiextensions.k8s.io"},
	{"create", "clusterroles.rbac.authorization.k8s.io"},
	{"create", "clusterrolebindings.rbac.authorization.k8s.io"},
}

var kueueCRDs = []string{
	"admissionchecks.kueue.x-k8s.io",
	"clusterqueues.kueue.x-k8s.io",
	"cohorts.kueue.x-k8s.io",
	"localqueues.kueue.x-k8s.io",
	"multikueueclusters.kueue.x-k8s.io",
	"multikueueconfigs.kueue.x-k8s.io",
	"provisioningrequestconfigs.kueue.x-k8s.io",
	"resourceflavors.kueue.x-k8s.io",
	"topologies.kueue.x-k8s.io",
	"workloadpriorityclasses.kueue.x-k8s.io",
	"workloads.kueue.x-k8s.io",
}

// -----------------------------------------------------------------------------
// 2. Kueue Job Queue Resolution & Setup (Job-Level)
// -----------------------------------------------------------------------------

// resolveKueueQueue resolves which Kueue LocalQueue to use for job submission.
// Precedence rules:
// 1. If requestedQueueName is specified explicitly by the user, return it directly.
// 2. If 0 LocalQueues are found in the namespace, fall back to defaultLocalQueue ("default").
// 3. If exactly 1 LocalQueue is found, auto-discover and return it.
// 4. If multiple LocalQueues exist, check for standard "default", then "multislice-queue".
// 5. If multiple non-standard LocalQueues exist, return an error asking the user to specify --queue.
func (g *GKEOrchestrator) resolveKueueQueue(requestedQueueName, ns string) (string, error) {
	if requestedQueueName != "" {
		logging.Info("Using provided Kueue LocalQueue: %s", requestedQueueName)
		return requestedQueueName, nil
	}

	res := g.executor.ExecuteCommand("kubectl", "get", "localqueue", "-n", ns, "-o", "jsonpath={.items[*].metadata.name}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to query LocalQueues in namespace %s: %s", ns, res.Stderr)
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

	if slices.Contains(queues, defaultLocalQueue) {
		logging.Info("Multiple LocalQueues found %v. Defaulting to standard LocalQueue '%s'.", queues, defaultLocalQueue)
		return defaultLocalQueue, nil
	}
	if slices.Contains(queues, multisliceLocalQueue) {
		logging.Info("Multiple LocalQueues found %v. Defaulting to '%s'.", queues, multisliceLocalQueue)
		return multisliceLocalQueue, nil
	}

	return "", fmt.Errorf("multiple LocalQueues found (%v) and no standard '%s' or '%s' present. Please specify which one to use using --queue flag", queues, defaultLocalQueue, multisliceLocalQueue)
}

// checkLocalQueueExists checks if a LocalQueue with the given name exists in the namespace.
func (g *GKEOrchestrator) checkLocalQueueExists(name, ns string) (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "localqueue", name, "-n", ns)
	if res.ExitCode == 0 {
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		return false, nil
	}
	return false, fmt.Errorf("failed to check localqueue status: %s", res.Stderr)
}

// checkClusterQueueExists checks if a ClusterQueue with the given name exists in the cluster.
func (g *GKEOrchestrator) checkClusterQueueExists(name string) (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "clusterqueue", name)
	if res.ExitCode == 0 {
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		return false, nil
	}
	if strings.Contains(errStr, "forbidden") {
		return false, fmt.Errorf("insufficient RBAC permissions to read ClusterQueue %q (403 Forbidden)", name)
	}
	return false, fmt.Errorf("failed to check clusterqueue status: %s", res.Stderr)
}

// createDefaultQueues creates a LocalQueue in the target namespace and a corresponding ClusterQueue (defaultClusterQueue).
func (g *GKEOrchestrator) createDefaultQueues(localQueueName, ns string) error {
	logging.Info("Creating default ClusterQueue and LocalQueue...")

	if err := g.EnsureResourceFlavors(); err != nil {
		return err
	}

	cqName := defaultClusterQueue
	cqExists, err := g.checkClusterQueueExists(cqName)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			return fmt.Errorf("LocalQueue '%s' does not exist in namespace '%s' and cluster permissions are restricted (403 Forbidden). Please ask your cluster administrator to create a LocalQueue in namespace '%s'", localQueueName, ns, ns)
		}
		return fmt.Errorf("failed to check if ClusterQueue %s exists: %w", cqName, err)
	}

	if !cqExists {
		// Render and apply ClusterQueue only if it does not already exist
		clusterQueueBytes, err := g.renderClusterQueue(cqName)
		if err != nil {
			return fmt.Errorf("failed to render clusterqueue: %w", err)
		}
		if err := g.applyManifests(clusterQueueBytes, clusterQueueManifestFile); err != nil {
			return fmt.Errorf("failed to apply clusterqueue: %w", err)
		}
	} else {
		logging.Info("ClusterQueue '%s' already exists. Skipping ClusterQueue creation.", cqName)
	}

	// Render and apply LocalQueue
	localQueueBytes, err := g.renderLocalQueueInNamespace(localQueueName, cqName, ns)
	if err != nil {
		return err
	}

	if err := g.applyManifests(localQueueBytes, localQueueManifestFile); err != nil {
		return fmt.Errorf("failed to apply localqueue: %w", err)
	}

	logging.Info("Default queues created successfully.")
	return nil
}

// ensureClusterQueueCoverage checks if the parent ClusterQueue covers CPU and Memory quotas, updating it if empty.
func (g *GKEOrchestrator) ensureClusterQueueCoverage(localQueueName, ns string) error {
	cqName, err := g.getClusterQueueName(localQueueName, ns)
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
		if err := g.applyManifests(clusterQueueBytes, clusterQueueManifestFile); err != nil {
			return fmt.Errorf("failed to apply clusterqueue with new capacity: %w", err)
		}
		return nil
	}

	return fmt.Errorf("ClusterQueue '%s' does not cover required resources (CPU and Memory); please configure it manually to include quotas for 'cpu' and 'memory' resources", cqName)
}

// getClusterQueueName inspects a LocalQueue to retrieve the name of its bound ClusterQueue.
func (g *GKEOrchestrator) getClusterQueueName(localQueueName, ns string) (string, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "localqueue", localQueueName, "-n", ns, "-o", "jsonpath={.spec.clusterQueue}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to find clusterqueue for %s in namespace %s: %s", localQueueName, ns, res.Stderr)
	}
	cqName := strings.TrimSpace(res.Stdout)
	if cqName == "" {
		cqName = localQueueName
	}
	return cqName, nil
}

// checkClusterQueueCoverage inspects a ClusterQueue to verify if resourceGroups cover CPU and Memory.
func (g *GKEOrchestrator) checkClusterQueueCoverage(cqName string) (bool, bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "clusterqueue", cqName, "-o", "json")
	if res.ExitCode != 0 {
		errStr := strings.ToLower(res.Stderr)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to read ClusterQueue %q (403 Forbidden). Skipping resource coverage validation.", cqName)
			return true, false, nil
		}
		return false, false, fmt.Errorf("failed to get clusterqueue %s: %s", cqName, res.Stderr)
	}

	var cq map[string]interface{}
	if err := json.Unmarshal([]byte(res.Stdout), &cq); err != nil {
		return false, false, fmt.Errorf("failed to parse clusterqueue %s JSON: %w", cqName, err)
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

// hasRequiredResources helper checks if resourceGroups list includes both CPU and Memory covered resources.
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
					switch rStr {
					case "cpu":
						hasCPU = true
					case "memory":
						hasMem = true
					}
				}
			}
		}
	}

	return hasCPU && hasMem
}

// -----------------------------------------------------------------------------
// 3. Kueue Installation & Cluster Lifecycle Management (Cluster-Level)
// -----------------------------------------------------------------------------

// CheckAndInstallKueue verifies if Kueue is installed on the GKE cluster, installing or updating it if required.
func (g *GKEOrchestrator) CheckAndInstallKueue(version string, clusterName string, clusterLocation string) error {
	kueueCRDInstalled, errCRD := g.isKueueInstalled()
	if errCRD != nil {
		return fmt.Errorf("failed to verify Kueue CRD status: %w", errCRD)
	}
	kueueDeploymentInstalled, errDep := g.isKueueDeploymentInstalled()
	if errDep != nil {
		return fmt.Errorf("failed to verify Kueue deployment status: %w", errDep)
	}
	currentVersion, errVer := g.GetKueueVersion()
	if errVer != nil && (kueueCRDInstalled && kueueDeploymentInstalled) {
		logging.Info("Warning: Unable to determine installed Kueue version: %v", errVer)
	}

	if version == "" {
		version = defaultKueueVersion
	} else if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	needReinstall, reinstallReason := g.evaluateKueueReinstall(kueueCRDInstalled, kueueDeploymentInstalled, currentVersion, version)
	if needReinstall {
		if g.checkDynamicSlicingViaGKE() {
			return fmt.Errorf("automatic Kueue installation/modification blocked: cluster %s is set up for Dynamic-Slicing (found 'PROVISION_ONLY' in node pool placementPolicy). Automatic installation or modification of Kueue could conflict with custom topology configurations. Please install Kueue and required CRDs manually", clusterName)
		}

		if err := g.checkKueueInstallPermissions(version); err != nil {
			return err
		}

		if err := g.handleKueueReinstallation(version, reinstallReason); err != nil {
			return err
		}
	} else {
		if err := g.waitForKueueWebhookFast(); err != nil {
			return fmt.Errorf("kueue is installed but currently unhealthy. Please check deployment health with 'kubectl get deployment kueue-controller-manager -n kueue-system': %w", err)
		}
	}

	logging.Info("Kueue is already installed.")
	return nil
}

func (g *GKEOrchestrator) evaluateKueueReinstall(crdInstalled, depInstalled bool, currentVer, targetVer string) (bool, string) {
	if currentVer != "" && g.isVersionBelow(currentVer, targetVer) {
		return true, fmt.Sprintf("Current Kueue version %s is below target %s.", currentVer, targetVer)
	}
	if !crdInstalled || !depInstalled {
		return true, "Kueue installation is incomplete (CRD or Deployment missing)."
	}
	return false, ""
}

func (g *GKEOrchestrator) handleKueueReinstallation(targetVersion string, reason string) error {
	promptMsg := fmt.Sprintf("%s\nKueue requires re-installation using %s.\nWARNING: This deletes all queued and suspended workloads in this cluster before proceeding.\nReplying 'no' will cause an immediate exit and you will have to do the re-installation manually. Proceed?", reason, targetVersion)
	if !shell.PromptYesNo(promptMsg) {
		return fmt.Errorf("user declined to re-install Kueue")
	}

	logging.Info("Proceeding with clean re-installation of Kueue...")
	if err := g.DeleteAllKueueResources(); err != nil {
		return fmt.Errorf("failed to delete Kueue resources: %w", err)
	}

	return g.installKueue(targetVersion)
}

func (g *GKEOrchestrator) installKueue(version string) error {
	logging.Info("Installing Kueue version %s...", version)
	kueueManifestsURL := fmt.Sprintf("https://github.com/kubernetes-sigs/kueue/releases/download/%s/manifests.yaml", version)
	manifestBytes, err := g.downloadManifests(kueueManifestsURL)
	if err != nil {
		return err
	}

	cleanedManifests, err := g.cleanAndProcessManifests(manifestBytes, nil)
	if err != nil {
		return err
	}

	if err := g.applyManifests(cleanedManifests, "kueue.yaml"); err != nil {
		return err
	}

	logging.Info("Kueue components applied successfully.")

	if err := g.waitForKueueWebhook(); err != nil {
		return err
	}

	return g.installKueueResources(defaultClusterQueue, defaultLocalQueue)
}

func (g *GKEOrchestrator) checkKueueInstallPermissions(version string) error {
	logging.Info("Verifying cluster permissions for Kueue installation...")
	var missing []string
	for _, c := range kueuePermissionChecks {
		res := g.executor.ExecuteCommand("kubectl", "auth", "can-i", c.verb, c.resource)
		if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "yes" {
			missing = append(missing, fmt.Sprintf("'%s %s'", c.verb, c.resource))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("unable to re-install Kueue to version %s: missing required RBAC permissions: [%s]. Please contact your cluster administrator", version, strings.Join(missing, ", "))
	}
	return nil
}

func (g *GKEOrchestrator) isKueueInstalled() (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "crd", "clusterqueues.kueue.x-k8s.io")
	if res.ExitCode == 0 {
		logging.Info("Kueue CRD found.")
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr + " " + res.Stdout)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		logging.Info("Kueue CRD not found.")
		return false, nil
	}
	if strings.Contains(errStr, "forbidden") {
		logging.Warn("Insufficient RBAC permissions to read Kueue CRD (403 Forbidden). Assuming Kueue is installed in shared cluster.")
		return true, nil
	}
	return false, fmt.Errorf("failed to check for Kueue CRD: %s\n%s", res.Stderr, res.Stdout)
}

func (g *GKEOrchestrator) isKueueDeploymentInstalled() (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "deployment", "kueue-controller-manager", "-n", "kueue-system")
	if res.ExitCode == 0 {
		logging.Info("Kueue deployment found.")
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr + " " + res.Stdout)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		logging.Info("Kueue deployment not found.")
		return false, nil
	}
	if strings.Contains(errStr, "forbidden") {
		logging.Warn("Insufficient RBAC permissions to read Kueue deployment in kueue-system (403 Forbidden). Assuming Kueue deployment is installed in shared cluster.")
		return true, nil
	}
	return false, fmt.Errorf("failed to check for Kueue deployment: %s\n%s", res.Stderr, res.Stdout)
}

// GetKueueVersion queries the cluster for the deployed version of the Kueue controller.
func (g *GKEOrchestrator) GetKueueVersion() (string, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "deployment", "kueue-controller-manager", "-n", "kueue-system", "-o", "jsonpath={.spec.template.spec.containers[0].image}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to get Kueue version: %s\n%s", res.Stderr, res.Stdout)
	}
	image := strings.TrimSpace(res.Stdout)
	if idx := strings.Index(image, "@"); idx != -1 {
		image = image[:idx]
	}
	lastColon := strings.LastIndex(image, ":")
	if lastColon == -1 {
		return "", fmt.Errorf("unexpected image format for Kueue: %s", image)
	}
	version := image[lastColon+1:]
	if strings.Contains(version, "/") {
		return "", fmt.Errorf("no tag found in Kueue image: %s", image)
	}
	return version, nil
}

func (g *GKEOrchestrator) isVersionBelow(current, target string) bool {
	curMajor, curMinor, curPatch := parseVersion(current)
	defMajor, defMinor, defPatch := parseVersion(target)

	return curMajor < defMajor || (curMajor == defMajor && curMinor < defMinor) || (curMajor == defMajor && curMinor == defMinor && curPatch < defPatch)
}

func parseVersion(v string) (int, int, int) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	patch := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		patchStr := parts[2]
		patchStr = strings.Split(patchStr, "-")[0]
		patchStr = strings.Split(patchStr, "+")[0]
		patch, _ = strconv.Atoi(patchStr)
	}
	return major, minor, patch
}

// DeleteKueueDeployment deletes the kueue-controller-manager deployment from the cluster.
func (g *GKEOrchestrator) DeleteKueueDeployment() error {
	logging.Info("Deleting Kueue deployment...")
	res := g.executor.ExecuteCommand("kubectl", "delete", "deployment", "kueue-controller-manager", "-n", "kueue-system", "--ignore-not-found")
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to delete Kueue deployment: %s\n%s", res.Stderr, res.Stdout)
	}
	return nil
}

// DeleteAllKueueResources deletes all Kueue custom resources and CRDs from the cluster.
func (g *GKEOrchestrator) DeleteAllKueueResources() error {
	resourceList := strings.Join(kueueCRDs, ",")
	logging.Info("Deleting all Kueue custom resources...")
	res := g.executor.ExecuteCommand("kubectl", "delete", resourceList, "--all", "--ignore-not-found", "--timeout=60s")
	if res.ExitCode != 0 {
		logging.Info("Warning: Non-zero exit code while deleting Kueue custom resources: %s", res.Stderr)
	}

	logging.Info("Deleting Kueue CRDs...")
	args := append([]string{"delete", "crd", "--ignore-not-found", "--timeout=60s"}, kueueCRDs...)
	res = g.executor.ExecuteCommand("kubectl", args...)
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to delete Kueue CRDs: %s\n%s", res.Stderr, res.Stdout)
	}

	return g.DeleteKueueDeployment()
}

func (g *GKEOrchestrator) waitForKueueWebhookFast() error {
	res := g.executor.ExecuteCommand("kubectl", "rollout", "status", "deployment/kueue-controller-manager", "-n", "kueue-system", kueueControllerRolloutTimeout)
	if res.ExitCode != 0 {
		podDetails := g.getKueuePodDetails()
		return fmt.Errorf("kueue controller manager failed to become ready: %s\n%s%s", res.Stderr, res.Stdout, podDetails)
	}

	version, err := g.GetKueueVersion()
	if err != nil {
		logging.Warn("Failed to get Kueue version, defaulting to Endpoints check: %v", err)
		version = defaultKueueVersion
	}

	major, minor, _ := parseVersion(version)
	useEndpointSlice := major > 0 || (major == 0 && minor > 13)

	return g.waitForKueueEndpoints(useEndpointSlice)
}

func (g *GKEOrchestrator) waitForKueueWebhook() error {
	if err := g.waitForKueueWebhookFast(); err != nil {
		return err
	}
	return g.probeKueueWebhookReadiness()
}

func (g *GKEOrchestrator) waitForKueueEndpoints(useEndpointSlice bool) error {
	for i := 0; i < 40; i++ {
		ready, err := g.checkKueueEndpoints(useEndpointSlice)
		if err != nil {
			return err
		}
		if ready {
			logging.Info("Kueue webhook service endpoints are available.")
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("timed out waiting for kueue-webhook-service endpoints to be available")
}

func (g *GKEOrchestrator) probeKueueWebhookReadiness() error {
	logging.Info("Probing Kueue webhook readiness...")
	probeName := fmt.Sprintf("gcluster-webhook-probe-%d", time.Now().UnixNano())
	probeManifest := fmt.Sprintf("apiVersion: kueue.x-k8s.io/%s\nkind: ResourceFlavor\nmetadata:\n  name: %s\n", kueueAPIVersion, probeName)
	f, err := os.CreateTemp("", "gcluster-webhook-probe-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create probe manifest file: %w", err)
	}
	probeFile := f.Name()
	defer func() {
		_ = f.Close()
		_ = g.executor.ExecuteCommand("kubectl", "delete", "-f", probeFile, "--ignore-not-found")
		_ = os.Remove(probeFile)
	}()

	if _, err := f.Write([]byte(probeManifest)); err != nil {
		return fmt.Errorf("failed to write probe manifest: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close probe manifest file: %w", err)
	}

	for i := 0; i < 20; i++ {
		res := g.executor.ExecuteCommand("kubectl", "apply", "-f", probeFile)
		if res.ExitCode == 0 {
			logging.Info("Kueue webhook is fully operational.")
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("timed out waiting for Kueue webhook to become operational")
}

func (g *GKEOrchestrator) getKueuePodDetails() string {
	podRes := g.executor.ExecuteCommand("kubectl", "get", "pods", "-n", "kueue-system", "-l", "control-plane=controller-manager", "-o", "json")
	var podDetails string
	if podRes.ExitCode == 0 {
		var podList struct {
			Items []struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Status struct {
					ContainerStatuses []struct {
						Name  string `json:"name"`
						State struct {
							Waiting struct {
								Reason  string `json:"reason"`
								Message string `json:"message"`
							} `json:"waiting"`
						} `json:"state"`
					} `json:"containerStatuses"`
				} `json:"status"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(podRes.Stdout), &podList); err == nil {
			for _, item := range podList.Items {
				for _, cs := range item.Status.ContainerStatuses {
					if cs.State.Waiting.Reason != "" {
						podDetails += fmt.Sprintf("\n  - Pod %s: %s (%s)", item.Metadata.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message)
					}
				}
			}
		}
	}
	return podDetails
}

func (g *GKEOrchestrator) checkKueueEndpoints(useEndpointSlice bool) (bool, error) {
	var cmdEndpoints shell.CommandResult
	if useEndpointSlice {
		cmdEndpoints = g.executor.ExecuteCommand("kubectl", "get", "endpointslice", "-l", "kubernetes.io/service-name=kueue-webhook-service", "-n", "kueue-system", "-o", "json")
	} else {
		cmdEndpoints = g.executor.ExecuteCommand("kubectl", "get", "endpoints", "kueue-webhook-service", "-n", "kueue-system", "-o", "json")
	}

	if cmdEndpoints.ExitCode != 0 {
		errStr := strings.ToLower(cmdEndpoints.Stderr)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to read kueue-webhook-service endpoints in kueue-system (403 Forbidden). Skipping endpoint readiness check.")
			return true, nil
		}
		return false, nil
	}

	if useEndpointSlice {
		var eps k8sEndpointSliceList
		if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err != nil {
			return false, fmt.Errorf("failed to unmarshal endpointslice json: %w", err)
		}
		if eps.HasReadyEndpoint() {
			return true, nil
		}
	} else {
		var eps struct {
			Subsets []struct {
				Addresses []struct {
					Ip string `json:"ip"`
				} `json:"addresses"`
			} `json:"subsets"`
		}
		if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err != nil {
			return false, fmt.Errorf("failed to unmarshal endpoints json: %w", err)
		}
		for _, subset := range eps.Subsets {
			if len(subset.Addresses) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// -----------------------------------------------------------------------------
// 4. Kueue Priority Classes
// -----------------------------------------------------------------------------

func (g *GKEOrchestrator) getClusterPriorityClasses() ([]string, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "priorityclass", "-o", "jsonpath={.items[*].metadata.name}")
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("failed to list priority classes: %s", res.Stderr)
	}
	return strings.Fields(res.Stdout), nil
}

func (g *GKEOrchestrator) hasUserPriorityClasses() (bool, error) {
	existing, err := g.getClusterPriorityClasses()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			logging.Warn("Insufficient RBAC permissions to list priority classes (403 Forbidden). Skipping default PriorityClass installation.")
			return true, nil
		}
		return false, err
	}

	for _, name := range existing {
		if strings.HasPrefix(name, "system-") || strings.HasPrefix(name, "gke-") {
			continue
		}
		logging.Info("Pre-existing PriorityClass '%s' found. Skipping default PriorityClass installation.", name)
		return true, nil
	}
	return false, nil
}

func (g *GKEOrchestrator) installPriorityClasses() error {
	logging.Info("Installing Kueue PriorityClasses...")
	tmpl, err := g.parseGKETemplate("priority_classes.tmpl")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return fmt.Errorf("failed to execute priority_classes template: %w", err)
	}

	logging.Info("Applying Kueue priority classes...")
	if err := g.applyManifests(buf.Bytes(), "kueue_priority_classes.yaml"); err != nil {
		return fmt.Errorf("failed to apply Kueue priority classes: %w", err)
	}

	return nil
}

func (g *GKEOrchestrator) ensurePriorityClassesInstalled() error {
	hasUserClasses, err := g.hasUserPriorityClasses()
	if err != nil {
		return err
	}

	if hasUserClasses {
		return nil
	}

	logging.Info("No user-defined PriorityClasses found. Installing defaults...")
	return g.installPriorityClasses()
}

// EnsurePriorityClassesInstalled ensures default priority classes are applied if no custom ones exist.
func (g *GKEOrchestrator) EnsurePriorityClassesInstalled() error {
	return g.ensurePriorityClassesInstalled()
}

func (g *GKEOrchestrator) validatePriorityClass(requestedPriority string) error {
	if requestedPriority == "" {
		return nil
	}

	existing, err := g.getClusterPriorityClasses()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			logging.Warn("Insufficient RBAC permissions to list priority classes (403 Forbidden). Skipping PriorityClass validation for %q.", requestedPriority)
			return nil
		}
		return err
	}

	if !slices.Contains(existing, requestedPriority) {
		return fmt.Errorf("priority class %q does not exist in the cluster. Available priority classes are: %v", requestedPriority, existing)
	}
	return nil
}

// -----------------------------------------------------------------------------
// 5. Kueue ResourceFlavors & Capacity Calculations
// -----------------------------------------------------------------------------

// EnsureResourceFlavors ensures that Kueue ResourceFlavors exist for all cluster capacity flavors.
func (g *GKEOrchestrator) EnsureResourceFlavors() error {
	logging.Info("Ensuring Kueue ResourceFlavors exist...")
	cmd := g.executor.ExecuteCommand("kubectl", "get", "resourceflavor", "-o", "jsonpath={.items[*].metadata.name}")
	existingFlavors := make(map[string]bool)
	if cmd.ExitCode != 0 {
		errStr := strings.ToLower(cmd.Stderr + " " + cmd.Stdout)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to list resource flavors (403 Forbidden). Skipping ResourceFlavor verification and creation.")
			return nil
		}
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "notfound") || strings.Contains(errStr, "no resources found") {
			logging.Info("No ResourceFlavors found.")
		} else {
			return fmt.Errorf("failed to list resource flavors: %s", cmd.Stderr)
		}
	}

	for _, f := range strings.Fields(cmd.Stdout) {
		existingFlavors[f] = true
	}

	names := slices.Sorted(maps.Keys(g.capacity.Flavors))
	for _, name := range names {
		fc := g.capacity.Flavors[name]
		if existingFlavors[name] {
			continue
		}
		logging.Info("Ensuring ResourceFlavor '%s'...", name)
		rfBytes, err := g.renderResourceFlavor(name, fc.NodeLabels)
		if err != nil {
			return fmt.Errorf("failed to render ResourceFlavor %s: %w", name, err)
		}
		if err := g.applyManifests(rfBytes, fmt.Sprintf("resource-flavor-%s.yaml", name)); err != nil {
			return fmt.Errorf("failed to apply ResourceFlavor %s: %w", name, err)
		}
	}
	return nil
}

func (g *GKEOrchestrator) renderResourceFlavor(name string, nodeLabels map[string]string) ([]byte, error) {
	rfMap := map[string]interface{}{
		"apiVersion": "kueue.x-k8s.io/" + kueueAPIVersion,
		"kind":       "ResourceFlavor",
		"metadata": map[string]interface{}{
			"name": name,
		},
	}
	if len(nodeLabels) > 0 {
		filteredLabels := make(map[string]string)
		// Omit instance-specific slice and topology labels to keep ResourceFlavor generic across TPU node pools.
		for k, v := range nodeLabels {
			if k != tpuTopologyLabel &&
				!strings.HasPrefix(k, "cloud.google.com/gke-tpu-slice-") &&
				!strings.HasPrefix(k, "cloud.google.com/gke-tpu-partition-") {
				filteredLabels[k] = v
			}
		}
		if len(filteredLabels) > 0 {
			rfMap["spec"] = map[string]interface{}{
				"nodeLabels": filteredLabels,
			}
		}
	}
	return yaml.Marshal(rfMap)
}

func (g *GKEOrchestrator) installKueueResources(cqName string, lqName string) error {
	logging.Info("Installing Kueue resources (ClusterQueue, LocalQueue)...")

	if err := g.EnsureResourceFlavors(); err != nil {
		return err
	}

	hasUserClasses, err := g.hasUserPriorityClasses()
	if err != nil {
		return err
	}
	if !hasUserClasses {
		if err := g.installPriorityClasses(); err != nil {
			return err
		}
	}

	// Install ClusterQueue
	clusterQueueBytes, err := g.renderClusterQueue(cqName)
	if err != nil {
		return err
	}
	if err := g.applyManifests(clusterQueueBytes, clusterQueueManifestFile); err != nil {
		return err
	}

	// Install LocalQueue
	localQueueBytes, err := g.renderLocalQueueInNamespace(lqName, cqName, "default")
	if err != nil {
		return err
	}
	if err := g.applyManifests(localQueueBytes, localQueueManifestFile); err != nil {
		return err
	}

	logging.Info("Kueue resources installed successfully.")
	return nil
}

func (g *GKEOrchestrator) renderLocalQueueInNamespace(lqName, cqName, ns string) ([]byte, error) {
	localQueueTmpl, err := g.parseGKETemplate(localQueueTemplateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", localQueueTemplateFile, err)
	}
	var localQueueBuf bytes.Buffer
	if err := localQueueTmpl.Execute(&localQueueBuf, struct {
		Namespace        string
		LocalQueueName   string
		ClusterQueueName string
	}{ns, lqName, cqName}); err != nil {
		return nil, fmt.Errorf("failed to execute %s template: %w", localQueueTemplateFile, err)
	}
	return localQueueBuf.Bytes(), nil
}

func (g *GKEOrchestrator) isResourceActive(staticAmount int, napLimitKey string) bool {
	return staticAmount > 0 || (g.napEnabled && g.napLimits[napLimitKey] > 0)
}

func (g *GKEOrchestrator) isAccelActive(staticAmount int, napLimitKey string, lowerFname string, flavorSub string) bool {
	if staticAmount > 0 {
		return true
	}
	if !g.napEnabled || g.napLimits[napLimitKey] <= 0 {
		return false
	}
	if flavorSub == "tpu" {
		return isTPUFlavor(lowerFname)
	}
	if flavorSub == "nvidia" {
		return isGPUFlavor(lowerFname)
	}
	return strings.Contains(lowerFname, flavorSub)
}

func (g *GKEOrchestrator) calculateCoveredResources() map[string]bool {
	coveredMap := make(map[string]bool)

	fnames := slices.Sorted(maps.Keys(g.capacity.Flavors))
	for _, fname := range fnames {
		fc := g.capacity.Flavors[fname]
		lowerFname := strings.ToLower(fname)
		if g.isResourceActive(fc.CPUs, "cpu") {
			coveredMap["cpu"] = true
		}
		if g.isResourceActive(fc.MemoryGi, "memory") {
			coveredMap["memory"] = true
		}
		if fname != "pathways-flavor" {
			if g.isAccelActive(fc.GPUs, "nvidia.com/gpu", lowerFname, "nvidia") {
				coveredMap["nvidia.com/gpu"] = true
			}
			if g.isAccelActive(fc.TPUs, "google.com/tpu", lowerFname, "tpu") {
				coveredMap["google.com/tpu"] = true
			}
		}
	}
	return coveredMap
}

func isTPUFlavor(fname string) bool {
	cleanName := strings.TrimPrefix(fname, "flavor-")
	if strings.EqualFold(cleanName, "ct") {
		return false
	}
	return config.IsTPU(cleanName) || strings.Contains(strings.ToLower(fname), "tpu")
}

func isGPUFlavor(fname string) bool {
	return strings.Contains(strings.ToLower(fname), "nvidia")
}

func (g *GKEOrchestrator) getNAPNominalQuota(resName string, fname string) (interface{}, bool) {
	if !g.napEnabled {
		return nil, false
	}
	lookupKey := resName
	if resName == "nvidia.com/gpu" || resName == "google.com/tpu" {
		specificKey := strings.TrimPrefix(fname, "flavor-")
		if _, ok := g.napLimits[specificKey]; ok {
			lookupKey = specificKey
		}
	}
	limit, ok := g.napLimits[lookupKey]
	if !ok {
		return nil, false
	}
	if resName == "google.com/tpu" && !isTPUFlavor(fname) {
		return 0, true
	}
	if resName == "nvidia.com/gpu" && !isGPUFlavor(fname) {
		return 0, true
	}
	if resName == "memory" {
		return fmt.Sprintf("%dG", limit), true
	}
	return limit, true
}

func getStaticNominalQuota(resName string, fc FlavorCapacity) interface{} {
	switch resName {
	case "cpu":
		return fc.CPUs
	case "memory":
		return fmt.Sprintf("%dGi", fc.MemoryGi)
	case "nvidia.com/gpu":
		return fc.GPUs
	case "google.com/tpu":
		return fc.TPUs
	default:
		return 0
	}
}

func (g *GKEOrchestrator) getNominalQuota(resName string, fc FlavorCapacity, fname string) interface{} {
	if val, ok := g.getNAPNominalQuota(resName, fname); ok {
		return val
	}
	return getStaticNominalQuota(resName, fc)
}

func (g *GKEOrchestrator) renderClusterQueue(name string) ([]byte, error) {
	coveredResourcesMap := g.calculateCoveredResources()

	var mainFlavors []map[string]interface{}
	fnames := slices.Sorted(maps.Keys(g.capacity.Flavors))

	for _, fname := range fnames {
		fc := g.capacity.Flavors[fname]
		resources := g.buildFlavorResources(coveredResourcesMap, fc, fname)
		if len(resources) > 0 {
			mainFlavors = append(mainFlavors, map[string]interface{}{
				"name":      fname,
				"resources": resources,
			})
		}
	}

	var resourceGroups []map[string]interface{}
	if rg := buildResourceGroup(coveredResourcesMap, mainFlavors); rg != nil {
		resourceGroups = append(resourceGroups, rg)
	}

	cqMap := map[string]interface{}{
		"apiVersion": "kueue.x-k8s.io/" + kueueAPIVersion,
		"kind":       "ClusterQueue",
		"metadata": map[string]interface{}{
			"name": name,
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{},
			"queueingStrategy":  "BestEffortFIFO",
			"resourceGroups":    resourceGroups,
		},
	}
	return yaml.Marshal(cqMap)
}

func (g *GKEOrchestrator) buildFlavorResources(coveredMap map[string]bool, fc FlavorCapacity, fname string) []map[string]interface{} {
	resList := slices.Sorted(maps.Keys(coveredMap))

	var resources []map[string]interface{}
	_, hasPathways := g.capacity.Flavors["pathways-flavor"]
	for _, resName := range resList {
		quota := g.getNominalQuota(resName, fc, fname)
		// For Pathways TPU flavors, set CPU and Memory nominal quotas to unconstrained values (999999)
		// so secondary resource limits do not block Pathways helper containers from co-scheduling with TPUs.
		if hasPathways && isTPUFlavor(fname) {
			switch resName {
			case "cpu":
				quota = pathwaysCPUNominalQuota
			case "memory":
				quota = pathwaysMemoryNominalQuota
			}
		}
		resources = append(resources, map[string]interface{}{
			"name":         resName,
			"nominalQuota": quota,
		})
	}
	return resources
}

func buildResourceGroup(coveredMap map[string]bool, flavors []map[string]interface{}) map[string]interface{} {
	if len(flavors) == 0 {
		return nil
	}
	coveredResources := slices.Sorted(maps.Keys(coveredMap))

	return map[string]interface{}{
		"coveredResources": coveredResources,
		"flavors":          flavors,
	}
}
