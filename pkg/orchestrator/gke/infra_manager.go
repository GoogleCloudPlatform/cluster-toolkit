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
	"embed"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"hpc-toolkit/pkg/orchestrator"

	"gopkg.in/yaml.v2"
)

//go:embed templates/*
var templatesFS embed.FS

// defaultKueueVersion is the fallback version of Kueue to install.
// ATTENTION: If you update this version, please also update the corresponding
// default version in modules/management/kubectl-apply/variables.tf.
const defaultKueueVersion = "v0.17.1"
const defaultJobSetVersion = "v0.10.1"

func (g *GKEOrchestrator) checkAndInstallJobSetCRD() error {
	if installed, err := g.isJobSetCRDInstalled(); err != nil {
		return err
	} else if installed {
		logging.Info("JobSet CRD found. Verifying Webhook health...")
		cmdEndpoints := g.executor.ExecuteCommand("kubectl", "get", "endpoints", "jobset-webhook-service", "-n", "jobset-system", "-o", "jsonpath={.subsets[*].addresses[*].ip}")
		if cmdEndpoints.ExitCode == 0 && strings.TrimSpace(cmdEndpoints.Stdout) != "" {
			logging.Info("JobSet Webhook is healthy.")
			return nil
		}
		logging.Info("JobSet Webhook endpoints not found. Proceeding with re-installation/fix...")
	}

	jobSetManifestsURL := fmt.Sprintf("https://github.com/kubernetes-sigs/jobset/releases/download/%s/manifests.yaml", defaultJobSetVersion)
	return g.installJobSetCRD(jobSetManifestsURL)
}

func (g *GKEOrchestrator) CheckAndInstallKueue(version string, clusterName string, clusterLocation string) error {
	kueueCRDInstalled, _ := g.isKueueInstalled()
	kueueDeploymentInstalled, _ := g.isKueueDeploymentInstalled()
	currentVersion, _ := g.GetKueueVersion()

	if version == "" {
		version = defaultKueueVersion
	}

	var reinstallReason string
	needReinstall := false

	// 1. Minimum version check for KUEUE
	if currentVersion != "" && g.isVersionBelow(currentVersion, version) {
		needReinstall = true
		reinstallReason = fmt.Sprintf("Current Kueue version %s is below target %s.", currentVersion, version)
	}

	// 2. Check if basic installation is missing
	if !needReinstall && (!kueueCRDInstalled || !kueueDeploymentInstalled) {
		needReinstall = true
		reinstallReason = "Kueue installation is incomplete (CRD or Deployment missing)."
	}

	// 3. Check if webhook is healthy (only if not already deciding to reinstall)
	if !needReinstall && g.waitForKueueWebhook() != nil {
		needReinstall = true
		reinstallReason = "Kueue webhook health check failed. Treating as broken."
	}

	if needReinstall {
		isSuperSlicing, _ := g.checkSuperSlicingViaGKE(clusterName, clusterLocation)
		if isSuperSlicing {
			return fmt.Errorf("automatic Kueue installation blocked: we detected that cluster %s is set up for Super-slicing (found 'PROVISION_ONLY' in the node pool's placementPolicy). Wiping Kueue would corrupt your custom topology configurations. Please install Kueue and the required custom CRDs manually", clusterName)
		}

		if err := g.handleKueueReinstallation(version, reinstallReason); err != nil {
			return err
		}
	}

	logging.Info("Kueue is already installed.")
	return nil
}

func (g *GKEOrchestrator) ensurePriorityClassesInstalled() error {
	priorityClassesInstalled, err := g.arePriorityClassesInstalled()
	if err != nil {
		return err
	}

	if !priorityClassesInstalled {
		logging.Info("Required PriorityClasses not found. Installing them...")
		return g.installPriorityClasses()
	}

	logging.Info("The required PriorityClasses are already installed.")
	return nil
}

func (g *GKEOrchestrator) handleKueueReinstallation(targetVersion string, reason string) error {
	promptMsg := fmt.Sprintf("%s\nKueue requires re-installation using %s.\nWARNING: This deletes all queued and suspended workloads in this cluster before proceeding.\nReplying 'no' will cause an immediate exit and you will have to do the re-installation manually. Proceed?", reason, targetVersion)
	if !shell.PromptYesNo(promptMsg) {
		logging.Fatal("User declined to re-install Kueue. Exiting.")
	}

	logging.Info("Proceeding with clean re-installation of Kueue...")
	if err := g.DeleteAllKueueResources(); err != nil {
		return fmt.Errorf("failed to delete Kueue resources: %w", err)
	}

	return g.installKueue(targetVersion)
}

func (g *GKEOrchestrator) isVersionBelow(current, target string) bool {
	curMajor, curMinor, curPatch := parseVersion(current)
	defMajor, defMinor, defPatch := parseVersion(target)

	return curMajor < defMajor || (curMajor == defMajor && curMinor < defMinor) || (curMajor == defMajor && curMinor == defMinor && curPatch < defPatch)
}

func (g *GKEOrchestrator) DeleteKueueDeployment() error {
	logging.Info("Deleting Kueue deployment...")
	res := g.executor.ExecuteCommand("kubectl", "delete", "deployment", "kueue-controller-manager", "-n", "kueue-system", "--ignore-not-found")
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to delete Kueue deployment: %s\n%s", res.Stderr, res.Stdout)
	}
	return nil
}

func (g *GKEOrchestrator) DeleteAllKueueResources() error {
	logging.Info("Deleting all Kueue resources and CRDs...")

	crds := []string{
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

	for _, crd := range crds {
		logging.Info("Deleting resources for CRD %s...", crd)
		res := g.executor.ExecuteCommand("kubectl", "delete", crd, "--all", "--ignore-not-found")
		if res.ExitCode != 0 {
			logging.Warn("Failed to delete resources for CRD %s: %s", crd, res.Stderr)
			// Continue with other CRDs even if one fails
		}
	}

	logging.Info("Deleting Kueue CRDs...")
	args := append([]string{"delete", "crd", "--ignore-not-found"}, crds...)
	res := g.executor.ExecuteCommand("kubectl", args...)
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to delete Kueue CRDs: %s\n%s", res.Stderr, res.Stdout)
	}

	return g.DeleteKueueDeployment()
}

func (g *GKEOrchestrator) isKueueInstalled() (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "crd", "clusterqueues.kueue.x-k8s.io")
	if res.ExitCode == 0 {
		logging.Info("Kueue CRD found.")
		return true, nil
	}
	if strings.Contains(res.Stderr, "not found") || strings.Contains(res.Stdout, "NotFound") {
		logging.Info("Kueue CRD not found.")
		return false, nil
	}
	return false, fmt.Errorf("failed to check for Kueue CRD: %s\n%s", res.Stderr, res.Stdout)
}

func (g *GKEOrchestrator) isKueueDeploymentInstalled() (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "deployment", "kueue-controller-manager", "-n", "kueue-system")
	if res.ExitCode == 0 {
		logging.Info("Kueue deployment found.")
		return true, nil
	}
	if strings.Contains(res.Stderr, "not found") || strings.Contains(res.Stdout, "NotFound") {
		logging.Info("Kueue deployment not found.")
		return false, nil
	}
	return false, fmt.Errorf("failed to check for Kueue deployment: %s\n%s", res.Stderr, res.Stdout)
}

func (g *GKEOrchestrator) GetKueueVersion() (string, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "deployment", "kueue-controller-manager", "-n", "kueue-system", "-o", "jsonpath={.spec.template.spec.containers[0].image}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to get Kueue version: %s\n%s", res.Stderr, res.Stdout)
	}
	image := strings.TrimSpace(res.Stdout)
	// The image string might be "registry.k8s.io/kueue/kueue:v0.6.3"
	// We want to extract "v0.6.3"
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected image format for Kueue: %s", image)
	}
	version := parts[len(parts)-1]
	return version, nil
}

func (g *GKEOrchestrator) arePriorityClassesInstalled() (bool, error) {
	args := []string{"get", "priorityclass"}
	args = append(args, orchestrator.ValidPriorityClasses...)
	args = append(args, "-o", "name")

	res := g.executor.ExecuteCommand("kubectl", args...)
	if res.ExitCode != 0 {
		logging.Info("One or more PriorityClasses not found.")
		return false, nil
	}
	return true, nil
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

	if err := g.applyManifests(cleanedManifests, "jobset.yaml"); err != nil {
		return err
	}

	logging.Info("Kueue components applied successfully.")

	if err := g.waitForKueueWebhook(); err != nil {
		return err
	}

	return g.installKueueResources(defaultClusterQueue, defaultLocalQueue)
}

func (g *GKEOrchestrator) installPriorityClasses() error {
	logging.Info("Installing Kueue PriorityClasses...")
	priorityClassesTmpl, err := template.ParseFS(templatesFS, "templates/priority_classes.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse priority_classes.tmpl: %w", err)
	}
	var priorityClassesBuf bytes.Buffer
	if err := priorityClassesTmpl.Execute(&priorityClassesBuf, nil); err != nil {
		return fmt.Errorf("failed to execute priority_classes.tmpl template: %w", err)
	}
	return g.applyManifests(priorityClassesBuf.Bytes(), "priority-classes.yaml")
}

func (g *GKEOrchestrator) installKueueResources(cqName string, lqName string) error {
	logging.Info("Installing Kueue resources (ClusterQueue, LocalQueue)...")

	if err := g.installPriorityClasses(); err != nil {
		return err
	}

	// Install ClusterQueue
	clusterQueueBytes, err := g.renderClusterQueue(cqName)
	if err != nil {
		return err
	}
	if err := g.applyManifests(clusterQueueBytes, "cluster-queue.yaml"); err != nil {
		return err
	}

	// Install LocalQueue
	localQueueTmpl, err := template.ParseFS(templatesFS, "templates/local_queue.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse local_queue.tmpl: %w", err)
	}
	var localQueueBuf bytes.Buffer
	if err := localQueueTmpl.Execute(&localQueueBuf, struct {
		Namespace        string
		LocalQueueName   string
		ClusterQueueName string
	}{"default", lqName, cqName}); err != nil {
		return fmt.Errorf("failed to execute local_queue.tmpl template: %w", err)
	}
	if err := g.applyManifests(localQueueBuf.Bytes(), "local-queue.yaml"); err != nil {
		return err
	}

	logging.Info("Kueue resources installed successfully.")
	return nil
}

func (g *GKEOrchestrator) renderClusterQueue(name string) ([]byte, error) {
	var mainFlavors []map[string]interface{}
	mainCoveredResourcesMap := make(map[string]bool)

	var pathwaysFlavors []map[string]interface{}
	pathwaysCoveredResourcesMap := make(map[string]bool)

	for name, fc := range g.capacity.Flavors {
		resources, isPathways := g.buildFlavorResources(name, fc, mainCoveredResourcesMap, pathwaysCoveredResourcesMap)
		if len(resources) > 0 {
			flavor := map[string]interface{}{
				"name":      name,
				"resources": resources,
			}
			if isPathways {
				pathwaysFlavors = append(pathwaysFlavors, flavor)
			} else {
				mainFlavors = append(mainFlavors, flavor)
			}
		}
	}

	var resourceGroups []map[string]interface{}

	var mainCoveredResources []string
	for res := range mainCoveredResourcesMap {
		mainCoveredResources = append(mainCoveredResources, res)
	}
	if len(mainCoveredResources) > 0 {
		resourceGroups = append(resourceGroups, map[string]interface{}{
			"coveredResources": mainCoveredResources,
			"flavors":          mainFlavors,
		})
	}

	var pathwaysCoveredResources []string
	for res := range pathwaysCoveredResourcesMap {
		pathwaysCoveredResources = append(pathwaysCoveredResources, res)
	}
	if len(pathwaysCoveredResources) > 0 {
		resourceGroups = append(resourceGroups, map[string]interface{}{
			"coveredResources": pathwaysCoveredResources,
			"flavors":          pathwaysFlavors,
		})
	}

	cqMap := map[string]interface{}{
		"apiVersion": "kueue.x-k8s.io/v1beta1",
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

	cqBytes, err := yaml.Marshal(cqMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ClusterQueue to YAML: %w", err)
	}

	return cqBytes, nil
}

func (g *GKEOrchestrator) buildFlavorResources(name string, fc FlavorCapacity, mainCovered, pathwaysCovered map[string]bool) ([]map[string]interface{}, bool) {
	var resources []map[string]interface{}
	isPathways := (name == "pathways-flavor")

	if fc.CPUs > 0 {
		if isPathways {
			pathwaysCovered["cpu"] = true
		} else {
			mainCovered["cpu"] = true
		}
		resources = append(resources, map[string]interface{}{"name": "cpu", "nominalQuota": fc.CPUs})
	}
	if fc.MemoryGi > 0 {
		if isPathways {
			pathwaysCovered["memory"] = true
		} else {
			mainCovered["memory"] = true
		}
		resources = append(resources, map[string]interface{}{"name": "memory", "nominalQuota": fmt.Sprintf("%dGi", fc.MemoryGi)})
	}
	if fc.GPUs > 0 {
		mainCovered["nvidia.com/gpu"] = true
		resources = append(resources, map[string]interface{}{"name": "nvidia.com/gpu", "nominalQuota": fc.GPUs})
	}
	if fc.TPUs > 0 {
		mainCovered["google.com/tpu"] = true
		resources = append(resources, map[string]interface{}{"name": "google.com/tpu", "nominalQuota": fc.TPUs})
	}

	return resources, isPathways
}

func (g *GKEOrchestrator) renderResourceFlavor(name string, nodeLabels map[string]string) ([]byte, error) {
	rfMap := map[string]interface{}{
		"apiVersion": "kueue.x-k8s.io/v1beta1",
		"kind":       "ResourceFlavor",
		"metadata": map[string]interface{}{
			"name": name,
		},
	}
	if len(nodeLabels) > 0 {
		rfMap["spec"] = map[string]interface{}{
			"nodeLabels": nodeLabels,
		}
	}
	return yaml.Marshal(rfMap)
}

func (g *GKEOrchestrator) EnsureResourceFlavors() error {
	logging.Info("Ensuring Kueue ResourceFlavors exist...")
	for name, fc := range g.capacity.Flavors {
		logging.Info("Ensuring ResourceFlavor '%s'...", name)
		rfBytes, err := g.renderResourceFlavor(name, fc.NodeLabels)
		if err != nil {
			return fmt.Errorf("failed to render ResourceFlavor %s: %w", name, err)
		}
		if err := g.applyManifests(rfBytes, "resource-flavor.yaml"); err != nil {
			return fmt.Errorf("failed to apply ResourceFlavor %s: %w", name, err)
		}
	}
	return nil
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

	if err := g.applyManifests(cleanedManifests, "kueue.yaml"); err != nil {
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
			if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err == nil {
				for _, item := range eps.Items {
					for _, ep := range item.Endpoints {
						if ep.Conditions.Ready && len(ep.Addresses) > 0 {
							logging.Info("JobSet webhook service endpoints are available.")
							return nil
						}
					}
				}
			}
		}
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("timed out waiting for jobset-webhook-service endpoints to be available")
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
		patch, _ = strconv.Atoi(parts[2])
	}
	return major, minor, patch
}

func (g *GKEOrchestrator) waitForKueueWebhook() error {
	res := g.executor.ExecuteCommand("kubectl", "rollout", "status", "deployment/kueue-controller-manager", "-n", "kueue-system", "--timeout=600s")
	if res.ExitCode != 0 {
		podDetails := g.getKueuePodDetails()
		return fmt.Errorf("kueue controller manager failed to become ready: %s\n%s%s", res.Stderr, res.Stdout, podDetails)
	}

	version, err := g.GetKueueVersion()
	if err != nil {
		logging.Warn("Failed to get Kueue version, defaulting to Endpoints check: %v", err)
		version = defaultKueueVersion // Fallback to older version behavior
	}

	major, minor, _ := parseVersion(version)
	useEndpointSlice := major > 0 || (major == 0 && minor > 13)

	endpointsReady := false
	for i := 0; i < 40; i++ {
		ready, err := g.checkKueueEndpoints(useEndpointSlice)
		if err != nil {
			return err
		}
		if ready {
			logging.Info("Kueue webhook service endpoints are available.")
			endpointsReady = true
			break
		}
		time.Sleep(3 * time.Second)
	}

	if !endpointsReady {
		return fmt.Errorf("timed out waiting for kueue-webhook-service endpoints to be available")
	}

	// Active probe to ensure webhook is processing requests
	logging.Info("Probing Kueue webhook readiness...")
	probeManifest := `apiVersion: kueue.x-k8s.io/v1beta1
kind: ResourceFlavor
metadata:
  name: gcluster-webhook-probe
`
	probeFile := filepath.Join(os.TempDir(), "gcluster-webhook-probe.yaml")
	if err := os.WriteFile(probeFile, []byte(probeManifest), 0644); err != nil {
		return fmt.Errorf("failed to write probe manifest: %w", err)
	}
	defer os.Remove(probeFile)

	for i := 0; i < 20; i++ {
		res := g.executor.ExecuteCommand("kubectl", "apply", "-f", probeFile)
		if res.ExitCode == 0 {
			logging.Info("Kueue webhook is fully operational.")
			g.executor.ExecuteCommand("kubectl", "delete", "-f", probeFile, "--ignore-not-found")
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
		return false, nil
	}

	if useEndpointSlice {
		var eps k8sEndpointSliceList
		if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err == nil {
			for _, item := range eps.Items {
				for _, ep := range item.Endpoints {
					if ep.Conditions.Ready && len(ep.Addresses) > 0 {
						return true, nil
					}
				}
			}
		}
	} else {
		var eps struct {
			Subsets []struct {
				Addresses []struct {
					Ip string `json:"ip"`
				} `json:"addresses"`
			} `json:"subsets"`
		}
		if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err == nil {
			for _, subset := range eps.Subsets {
				if len(subset.Addresses) > 0 {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (g *GKEOrchestrator) isJobSetCRDInstalled() (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "crd", "jobsets.jobset.x-k8s.io")
	if res.ExitCode == 0 {
		return true, nil
	}
	if strings.Contains(res.Stderr, "not found") || strings.Contains(res.Stdout, "NotFound") {
		logging.Info("JobSet CRD not found.")
		return false, nil
	}
	return false, fmt.Errorf("failed to check for JobSet CRD: %s\n%s", res.Stderr, res.Stdout)
}

func (g *GKEOrchestrator) downloadManifests(url string) ([]byte, error) {
	logging.Info("Downloading manifests from %s", url)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
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
func (g *GKEOrchestrator) ValidateClusterState(workloadName string, clusterName string, clusterLocation string, projectID string) error {
	validators := []func() error{
		g.checkClusterConnectivity,
		func() error { return g.CheckAndInstallKueue("", clusterName, clusterLocation) },
		func() error { return g.ensurePriorityClassesInstalled() },
		g.checkAndInstallJobSetCRD,
		func() error { return g.validateJobConflicts(workloadName, clusterName, clusterLocation, projectID) },
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
	res := g.executor.ExecuteCommand("kubectl", "get", "namespace", "default", "--request-timeout=5s")
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to connect to GKE cluster. Please verify your IP is allowed in the cluster's authorized networks or that you have correct network access. Error: %s", res.Stderr)
	}
	logging.Info("Cluster connectivity verified.")
	return nil
}

func (g *GKEOrchestrator) checkSuperSlicingViaGKE(clusterName, clusterLocation string) (bool, error) {
	poolName := os.Getenv("GKE_NODE_POOL_NAME")
	if poolName == "" {
		return false, nil
	}

	result := g.executor.ExecuteCommand("gcloud", "container", "node-pools", "describe", poolName, "--cluster="+clusterName, "--location="+clusterLocation, "--format=json(placementPolicy)")
	if result.ExitCode != 0 {
		return false, fmt.Errorf("failed to describe node pool: %s", result.Stderr)
	}

	var policy map[string]interface{}
	if err := json.Unmarshal([]byte(result.Stdout), &policy); err == nil {
		if placement, ok := policy["placementPolicy"].(map[string]interface{}); ok {
			if mode, ok := placement["acceleratorTopologyMode"].(string); ok && mode == "PROVISION_ONLY" {
				return true, nil
			}
		}
	}
	return false, nil
}
