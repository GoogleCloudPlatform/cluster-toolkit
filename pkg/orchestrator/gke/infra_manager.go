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
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v2"
)

//go:embed templates/*
var templatesFS embed.FS

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

	jobSetManifestsURL := "https://github.com/kubernetes-sigs/jobset/releases/download/v0.10.1/manifests.yaml"
	return g.installJobSetCRD(jobSetManifestsURL)
}

func (g *GKEOrchestrator) checkAndInstallKueue() error {
	kueueInstalled, err := g.isKueueInstalled()
	if err != nil {
		return err
	}

	if !kueueInstalled {
		logging.Info("Kueue not found. Installing Kueue...")
		return g.installKueue()
	}

	priorityClassesInstalled, err := g.arePriorityClassesInstalled()
	if err != nil {
		return err
	}

	if !priorityClassesInstalled {
		logging.Info("Required PriorityClasses not found. Installing them...")
		return g.installKueueResources()
	}

	logging.Info("Kueue and required PriorityClasses are already installed.")
	return nil
}

func (g *GKEOrchestrator) isKueueInstalled() (bool, error) {
	logging.Info("Checking for Kueue installation...")
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

func (g *GKEOrchestrator) arePriorityClassesInstalled() (bool, error) {
	logging.Info("Checking for PriorityClass installation...")
	priorityClasses := []string{"very-low", "low", "medium", "high"}
	for _, pc := range priorityClasses {
		res := g.executor.ExecuteCommand("kubectl", "get", "priorityclass", pc)
		if res.ExitCode != 0 {
			if strings.Contains(res.Stderr, "not found") || strings.Contains(res.Stdout, "NotFound") {
				logging.Info("PriorityClass %s not found.", pc)
				return false, nil
			}
			return false, fmt.Errorf("failed to check for PriorityClass %s: %s\n%s", pc, res.Stderr, res.Stdout)
		}
	}
	return true, nil
}

func (g *GKEOrchestrator) installKueue() error {
	logging.Info("Installing Kueue...")
	kueueManifestsURL := "https://github.com/kubernetes-sigs/kueue/releases/download/v0.6.3/manifests.yaml"
	manifestBytes, err := g.downloadJobSetManifests(kueueManifestsURL)
	if err != nil {
		return err
	}

	if err := g.applyJobSetManifests(manifestBytes); err != nil {
		return err
	}

	logging.Info("Kueue components applied successfully.")
	return g.installKueueResources()
}

func (g *GKEOrchestrator) installKueueResources() error {
	logging.Info("Installing Kueue resources (PriorityClasses, ClusterQueue, LocalQueue)...")

	// Install PriorityClasses
	priorityClassesTmpl, err := template.ParseFS(templatesFS, "templates/priority_classes.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse priority_classes.tmpl: %w", err)
	}
	var priorityClassesBuf bytes.Buffer
	if err := priorityClassesTmpl.Execute(&priorityClassesBuf, nil); err != nil {
		return fmt.Errorf("failed to execute priority_classes.tmpl template: %w", err)
	}
	if err := g.applyJobSetManifests(priorityClassesBuf.Bytes()); err != nil {
		return err
	}

	// Install ClusterQueue
	clusterQueueTmpl, err := template.ParseFS(templatesFS, "templates/cluster_queue.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse cluster_queue.tmpl: %w", err)
	}
	var clusterQueueBuf bytes.Buffer
	if err := clusterQueueTmpl.Execute(&clusterQueueBuf, nil); err != nil {
		return fmt.Errorf("failed to execute cluster_queue.tmpl template: %w", err)
	}
	if err := g.applyJobSetManifests(clusterQueueBuf.Bytes()); err != nil {
		return err
	}

	// Install LocalQueue
	localQueueTmpl, err := template.ParseFS(templatesFS, "templates/local_queue.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse local_queue.tmpl: %w", err)
	}
	var localQueueBuf bytes.Buffer
	if err := localQueueTmpl.Execute(&localQueueBuf, struct{ Namespace string }{"default"}); err != nil {
		return fmt.Errorf("failed to execute local_queue.tmpl template: %w", err)
	}
	if err := g.applyJobSetManifests(localQueueBuf.Bytes()); err != nil {
		return err
	}

	logging.Info("Kueue resources installed successfully.")
	return nil
}

func (g *GKEOrchestrator) installJobSetCRD(jobSetManifestsURL string) error {
	logging.Info("Installing/Fixing JobSet CRD and Webhook...")

	manifestBytes, err := g.downloadJobSetManifests(jobSetManifestsURL)
	if err != nil {
		return err
	}

	cleanedManifests, err := g.cleanJobSetManifests(manifestBytes)
	if err != nil {
		return err
	}

	logging.Info("Force-recreating JobSet Controller Manager...")
	g.executor.ExecuteCommand("kubectl", "delete", "deployment", "jobset-controller-manager", "-n", "jobset-system", "--ignore-not-found=true")

	if err := g.applyJobSetManifests(cleanedManifests); err != nil {
		return err
	}

	logging.Info("JobSet components applied successfully.")

	return g.waitForJobSetWebhook()
}

func (g *GKEOrchestrator) waitForJobSetWebhook() error {
	logging.Info("Waiting for JobSet webhook service to be ready...")
	cmd := shell.NewCommand("kubectl", "rollout", "status", "deployment/jobset-controller-manager", "-n", "jobset-system", "--timeout=300s")
	res := cmd.Execute()
	if res.ExitCode != 0 {
		return fmt.Errorf("jobset controller manager failed to become ready: %s\n%s", res.Stderr, res.Stdout)
	}

	logging.Info("Verifying JobSet webhook service endpoints...")
	for i := 0; i < 100; i++ {
		cmdEndpoints := g.executor.ExecuteCommand("kubectl", "get", "endpoints", "jobset-webhook-service", "-n", "jobset-system", "-o", "jsonpath={.subsets[*].addresses[*].ip}")
		if cmdEndpoints.ExitCode == 0 && strings.TrimSpace(cmdEndpoints.Stdout) != "" {
			logging.Info("JobSet webhook service endpoints are available.")
			return nil
		}
		g.executor.ExecuteCommand("sleep", "3")
	}

	return fmt.Errorf("timed out waiting for jobset-webhook-service endpoints to be available")
}

func (g *GKEOrchestrator) isJobSetCRDInstalled() (bool, error) {
	logging.Info("Checking for JobSet CRD installation...")
	res := g.executor.ExecuteCommand("kubectl", "get", "crd", "jobsets.jobset.x-k8s.io")
	if res.ExitCode == 0 {
		logging.Info("JobSet CRD already installed.")
		return true, nil
	}
	if strings.Contains(res.Stderr, "not found") || strings.Contains(res.Stdout, "NotFound") {
		logging.Info("JobSet CRD not found.")
		return false, nil
	}
	return false, fmt.Errorf("failed to check for JobSet CRD: %s\n%s", res.Stderr, res.Stdout)
}

func (g *GKEOrchestrator) downloadJobSetManifests(url string) ([]byte, error) {
	logging.Info("Downloading JobSet manifests from %s", url)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download JobSet manifests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download JobSet manifests: received status code %d", resp.StatusCode)
	}

	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read JobSet manifests: %w", err)
	}
	return manifestBytes, nil
}

func (g *GKEOrchestrator) cleanJobSetManifests(manifestBytes []byte) ([]byte, error) {
	logging.Info("Cleaning JobSet manifests (removing description fields)...")
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
			g.injectTolerationsAndLabels(data)
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

func (g *GKEOrchestrator) applyJobSetManifests(manifests []byte) error {
	logging.Info("Applying JobSet manifests...")
	cmd := shell.NewCommand("kubectl", "apply", "-f", "-")
	cmd.SetInput(string(manifests))
	res := cmd.Execute()
	if res.ExitCode != 0 {
		return fmt.Errorf("kubectl apply failed with exit code %d: %s\n%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	logging.Info("JobSet manifests applied successfully.")
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
