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
	"hpc-toolkit/pkg/imagebuilder"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"hpc-toolkit/pkg/telemetry"
	"os"
	"strings"
	"text/template"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"gopkg.in/yaml.v2"
	k8syaml "sigs.k8s.io/yaml"
)

const JobSetTemplate = `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: {{.WorkloadName}}
  labels:
    gcluster.google.com/workload: {{.WorkloadName}}
    kueue.x-k8s.io/queue-name: {{.KueueQueueName}}
spec:
  ttlSecondsAfterFinished: {{.TtlSecondsAfterFinished}}
  failurePolicy:
    maxRestarts: {{.MaxRestarts}}
  replicatedJobs:
    - name: main-job
      replicas: {{.NumSlices}}
      template:
        spec:
          parallelism: {{.VmsPerSlice}}
          completions: {{.VmsPerSlice}}
          backoffLimit: 0
{{- if .PodFailurePolicy }}
          podFailurePolicy:
{{.PodFailurePolicy}}
{{- end }}
          template:
            metadata:
              labels:
                gcluster.google.com/workload: {{.WorkloadName}}
{{- if or .TopologyAnnotation .GCSFuseEnabled }}
              annotations:
{{- if .TopologyAnnotation }}
{{.TopologyAnnotation}}
{{- end }}
{{- if .GCSFuseEnabled }}
                gke-gcsfuse/volumes: "true"
{{- end }}
{{- end }}
            spec:
{{- if .SchedulerName }}
              schedulerName: {{.SchedulerName}}
{{- end }}
{{- if .PriorityClassName }}
              priorityClassName: {{.PriorityClassName}}
{{- end }}
              restartPolicy: Never
              containers:
              - name: workload-container
                image: {{.FullImageName}}
{{.CommandToRun}}
                volumeMounts:
                - name: temp-storage
                  mountPath: /mnt/data
{{.VolumeMountsYAML}}
              volumes:
              - name: temp-storage
                emptyDir: {}
{{.VolumesYAML}}
{{- if .NodeSelector }}
              nodeSelector:
{{.NodeSelector}}
{{- end }}
{{- if .Affinity }}
              affinity:
{{.Affinity}}
{{- end }}
{{- if .Tolerations }}
              tolerations:
{{.Tolerations}}
{{- end }}
{{- if .ImagePullSecrets }}
              imagePullSecrets:
{{.ImagePullSecrets}}
{{- end }}
{{- if .ServiceAccountName }}
              serviceAccountName: {{.ServiceAccountName}}
{{- end }}

`

type JobProfile struct {
	IsCPUMachine  bool
	CapacityCount int
}

type ManifestOptions struct {
	WorkloadName            string
	FullImageName           string
	CommandToRun            string
	AcceleratorType         string
	ResourcesString         string
	ProjectID               string
	ClusterName             string
	ClusterLocation         string
	KueueQueueName          string
	NumSlices               int
	VmsPerSlice             int
	MaxRestarts             int
	TtlSecondsAfterFinished int
	NodeSelector            string
	Affinity                string
	PodFailurePolicy        string
	ImagePullSecrets        string
	ServiceAccountName      string
	TopologyAnnotation      string
	Topology                string
	SchedulerName           string
	Tolerations             string
	AwaitJobCompletion      bool
	PriorityClassName       string
	VolumesYAML             string
	VolumeMountsYAML        string
	GCSFuseEnabled          bool
	IsSuperSlicing          bool // True if the cluster supports and is configured for Super-slicing.
	IsCPUMachine            bool // Track if the machine type is deduced to be a CPU machine
	Pathways                orchestrator.PathwaysJobDefinition
}

type Executor interface {
	ExecuteCommand(name string, args ...string) shell.CommandResult
}

type DefaultExecutor struct{}

func (d *DefaultExecutor) ExecuteCommand(name string, args ...string) shell.CommandResult {
	return shell.ExecuteCommand(name, args...)
}

type GKEOrchestrator struct {
	executor     Executor
	clusterZones []string
	nodePoolSAs  []string
}

func NewGKEOrchestrator() (*GKEOrchestrator, error) {
	return &GKEOrchestrator{
		executor: &DefaultExecutor{},
	}, nil
}

func (g *GKEOrchestrator) SetExecutor(e Executor) {
	g.executor = e
}

func (g *GKEOrchestrator) SubmitJob(job orchestrator.JobDefinition) error {
	logging.Info("Starting gcluster job submit workflow...")

	startTime := time.Now()
	var success bool
	defer func() {
		latencySecs := time.Since(startTime).Seconds()
		profile := map[string]string{
			"accelerator_type": job.AcceleratorType,
			"nodes":            fmt.Sprintf("%d", job.NumSlices),
		}

		telemetry.RecordLocalMetrics(job.WorkloadName, latencySecs, success, profile)
	}()

	var err error
	job, err = g.initializeJobSubmission(job)

	if err != nil {
		return err
	}

	if err := g.ensureNodePoolImagePullPermissions(job); err != nil {
		logging.Info("Warning: Failed to auto-grant Artifact Registry permissions to node pool service accounts: %v", err)
	}

	if err := g.checkAndInstallJobSetCRD(); err != nil {
		return fmt.Errorf("failed to check or install JobSet CRD: %w", err)
	}

	if err := g.checkAndInstallKueue(); err != nil {
		return fmt.Errorf("failed to check or install Kueue: %w", err)
	}

	fullImageName, err := g.buildContainerImage(job.ProjectID, job.BaseImage, job.BuildContext, job.Platform, job.ImageName)
	if err != nil {
		return err
	}
	if job.IsPathwaysJob {
		manifestContent, err := g.generatePathwaysManifest(job, fullImageName)
		if err != nil {
			return err
		}
		return g.applyManifest(manifestContent, job.OutputManifest, job.WorkloadName)
	}

	manifestOpts, profile, err := g.prepareManifestOptions(job, fullImageName)

	if err != nil {
		return err
	}

	err = g.generateAndApplyManifest(manifestOpts, profile, job.OutputManifest)

	if err != nil {
		return err
	}

	if job.AwaitJobCompletion && job.OutputManifest == "" {
		err = g.waitForJobCompletion(job.WorkloadName, job.ClusterName, job.ClusterLocation, job.ProjectID)
		if err != nil {
			return err
		}
	}

	logging.Info("gcluster job submit workflow completed.")
	success = true
	return nil
}

func (g *GKEOrchestrator) generatePathwaysManifest(job orchestrator.JobDefinition, fullImageName string) (string, error) {
	// Set default values for Pathways-specific fields if not provided
	if job.Pathways.ProxyServerImage == "" {
		job.Pathways.ProxyServerImage = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest"
	}
	if job.Pathways.ServerImage == "" {
		job.Pathways.ServerImage = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest"
	}
	if job.Pathways.WorkerImage == "" {
		// WorkerImage defaults to ServerImage if not explicitly set
		job.Pathways.WorkerImage = job.Pathways.ServerImage
	}
	if job.Pathways.GCSLocation == "" {
		job.Pathways.GCSLocation = "gs://cloud-pathways-staging/tmp"
	}

	tmpl, err := template.New("pathwaysJobSet").Parse(pathwaysJobSetTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse pathways jobset template: %w", err)
	}

	opts, _, err := g.prepareManifestOptions(job, fullImageName)

	if err != nil {
		return "", err
	}

	opts.Pathways = job.Pathways

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, opts); err != nil {
		return "", fmt.Errorf("failed to execute pathways jobset template: %w", err)
	}

	return buf.String(), nil
}

func (g *GKEOrchestrator) applyManifest(manifestContent, outputManifestPath, workloadName string) error {
	if outputManifestPath != "" {
		logging.Info("Saving GKE manifest to %s", outputManifestPath)
		if err := os.WriteFile(outputManifestPath, []byte(manifestContent), 0644); err != nil {
			return fmt.Errorf("failed to write GKE manifest to file %s: %w", outputManifestPath, err)
		}
		logging.Info("GKE manifest saved successfully.")
	} else {
		logging.Info("Cleaning up any existing JobSet with name '%s'...", workloadName)
		g.executor.ExecuteCommand("kubectl", "delete", "jobset", workloadName, "--ignore-not-found=true")

		logging.Info("Applying GKE manifest to cluster...")
		err := g.applyJobSetManifests([]byte(manifestContent))
		if err != nil {
			return fmt.Errorf("failed to apply GKE manifest: %w", err)
		}
		logging.Info("GKE workload deployed successfully.")
	}
	return nil
}

func (g *GKEOrchestrator) initializeJobSubmission(job orchestrator.JobDefinition) (orchestrator.JobDefinition, error) {
	projectID, err := g.getProjectID(job.ProjectID)
	if err != nil {
		return job, err
	}
	job.ProjectID = projectID

	logging.Info("Fetching GKE cluster metadata for '%s'...", job.ClusterName)
	res := g.executor.ExecuteCommand("gcloud", "container", "clusters", "describe", job.ClusterName,
		"--location", job.ClusterLocation,
		"--project", job.ProjectID,
		"--format=json")
	if res.ExitCode != 0 {
		return job, fmt.Errorf("failed to describe GKE cluster %s: %s", job.ClusterName, res.Stderr)
	}

	var clusterDesc gkeCluster
	if err := json.Unmarshal([]byte(res.Stdout), &clusterDesc); err != nil {
		return job, fmt.Errorf("failed to parse GKE cluster description: %w", err)
	}

	g.clusterZones = clusterDesc.Locations
	if len(g.clusterZones) == 0 {
		return job, fmt.Errorf("GKE cluster %s has no locations/zones configured", job.ClusterName)
	}

	for _, np := range clusterDesc.NodePools {
		sa := strings.TrimSpace(np.Config.ServiceAccount)
		if sa != "" && sa != "default" {
			g.nodePoolSAs = append(g.nodePoolSAs, sa)
		}
	}

	logging.Info("Configuring kubectl for GKE cluster '%s'...", job.ClusterName)
	err = g.configureKubectl(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		return job, err
	}

	localQueue, err := g.resolveKueueQueue(job.KueueQueueName)
	if err != nil {
		logging.Info("Warning: Failed to auto-discover Kueue Queue Name: %v. Falling back to default-queue.", err)
		localQueue = "default-queue"
	}
	job.KueueQueueName = localQueue

	logging.Info("Ensuring Kueue ClusterQueue covers all requested resources...")
	if err := g.ensureClusterQueueCoverage(localQueue); err != nil {
		logging.Info("Warning: Could not automatically update ClusterQueue: %v. Workload might remain suspended.", err)
	}

	accelType, err := g.resolveAcceleratorType(job.AcceleratorType)
	if err != nil {
		logging.Info("Warning: Failed to auto-discover Accelerator Type: %v. Assuming CPU-only.", err)
		accelType = ""
	}
	job.AcceleratorType = accelType

	return job, nil
}

func (g *GKEOrchestrator) ensureClusterQueueCoverage(localQueueName string) error {
	cqName, err := g.getClusterQueueName(localQueueName)
	if err != nil {
		return err
	}

	hasCoverage, err := g.checkClusterQueueCoverage(cqName)
	if err != nil {
		return err
	}

	if hasCoverage {
		logging.Info("Kueue ClusterQueue '%s' already covers CPU and Memory.", cqName)
		return nil
	}

	logging.Info("Patching ClusterQueue '%s' to include CPU and Memory quotas...", cqName)
	patch := `[
		{"op": "add", "path": "/spec/resourceGroups/0/coveredResources/-", "value": "cpu"},
		{"op": "add", "path": "/spec/resourceGroups/0/coveredResources/-", "value": "memory"},
		{"op": "add", "path": "/spec/resourceGroups/0/flavors/0/resources/-", "value": {"name": "cpu", "nominalQuota": "2000"}},
		{"op": "add", "path": "/spec/resourceGroups/0/flavors/0/resources/-", "value": {"name": "memory", "nominalQuota": "20000Gi"}}
	]`

	res := g.executor.ExecuteCommand("kubectl", "patch", "clusterqueue", cqName, "--type", "json", "-p", patch)
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to patch clusterqueue: %s", res.Stderr)
	}

	logging.Info("ClusterQueue successfully updated.")
	return nil
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

func (g *GKEOrchestrator) checkClusterQueueCoverage(cqName string) (bool, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "clusterqueue", cqName, "-o", "json")
	if res.ExitCode != 0 {
		return false, fmt.Errorf("failed to get clusterqueue %s: %s", cqName, res.Stderr)
	}

	var cq map[string]interface{}
	if err := json.Unmarshal([]byte(res.Stdout), &cq); err != nil {
		return false, err
	}

	spec, ok := cq["spec"].(map[string]interface{})
	if !ok {
		return false, nil
	}
	rgList, ok := spec["resourceGroups"].([]interface{})
	if !ok || len(rgList) == 0 {
		return false, nil
	}

	return g.hasRequiredResources(rgList), nil
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
	if initialProjectID == "" {
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
	logging.Info("Using provided GCP Project ID: %s", initialProjectID)
	return initialProjectID, nil
}

type gkeCluster struct {
	Locations []string `json:"locations"`
	NodePools []struct {
		Config struct {
			ServiceAccount string `json:"serviceAccount"`
		} `json:"config"`
	} `json:"nodePools"`
}

func (g *GKEOrchestrator) ensureNodePoolImagePullPermissions(job orchestrator.JobDefinition) error {
	if len(g.nodePoolSAs) == 0 {
		logging.Info("No custom node pool service accounts found to grant Artifact Registry permissions.")
		return nil
	}

	logging.Info("Ensuring node pool service accounts have artifactregistry.reader role...")

	var uniqueSAs []string
	seen := make(map[string]bool)

	for _, sa := range g.nodePoolSAs {
		if !seen[sa] {
			seen[sa] = true
			uniqueSAs = append(uniqueSAs, sa)
		}
	}

	for _, sa := range uniqueSAs {
		logging.Info("Adding roles/artifactregistry.reader to service account %s on project %s...", sa, job.ProjectID)
		iamRes := g.executor.ExecuteCommand("gcloud", "projects", "add-iam-policy-binding", job.ProjectID,
			"--member", "serviceAccount:"+sa,
			"--role", "roles/artifactregistry.reader",
		)
		if iamRes.ExitCode != 0 {
			logging.Info("Warning: Failed to add IAM binding: %s", iamRes.Stderr)
		}
	}

	return nil
}

func (g *GKEOrchestrator) resolveKueueQueue(requested string) (string, error) {
	if requested != "" {
		logging.Info("Using provided Kueue LocalQueue: %s", requested)
		return requested, nil
	}

	logging.Info("Auto-discovering Kueue LocalQueue...")
	res := g.executor.ExecuteCommand("kubectl", "get", "localqueue", "-n", "default", "-o", "jsonpath={.items[*].metadata.name}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to query LocalQueues: %s", res.Stderr)
	}

	output := strings.TrimSpace(res.Stdout)
	if output == "" {
		logging.Info("No LocalQueues found. Defaulting to 'default-queue'.")
		return "default-queue", nil
	}

	queues := strings.Fields(output)
	if len(queues) == 1 {
		logging.Info("Auto-discovered Kueue LocalQueue: %s", queues[0])
		return queues[0], nil
	}

	logging.Info("Warning: Multiple LocalQueues found (%v). Defaulting to the first one: %s", queues, queues[0])
	return queues[0], nil
}

func (g *GKEOrchestrator) resolveAcceleratorType(requested string) (string, error) {
	if requested != "" {
		logging.Info("Using provided Accelerator Type: %s", requested)
		return requested, nil
	}

	logging.Info("Auto-discovering Accelerator Type...")

	output, err := g.queryAcceleratorLabels()
	if err != nil {
		return "", err
	}

	if output == "" {
		logging.Info("No accelerators found. Defaulting to CPU-only workload.")
		return "", nil
	}

	return g.parseAcceleratorOutput(output)
}

func (g *GKEOrchestrator) queryAcceleratorLabels() (string, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "resourceflavors.kueue.x-k8s.io", "-o", "jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-accelerator}{\"\\n\"}{end}")
	output := strings.TrimSpace(res.Stdout)

	if res.ExitCode != 0 || output == "" {
		res = g.executor.ExecuteCommand("kubectl", "get", "resourceflavors.kueue.x-k8s.io", "-o", "jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-accelerator}{\"\\n\"}{end}")
		if res.ExitCode == 0 {
			output = strings.TrimSpace(res.Stdout)
		}
	}

	if output == "" {
		res = g.executor.ExecuteCommand("kubectl", "get", "nodes", "-o", "jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-accelerator}{\"\\n\"}{end}")
		if res.ExitCode != 0 {
			return "", fmt.Errorf("failed to query Nodes for accelerators: %s", res.Stderr)
		}
		output = strings.TrimSpace(res.Stdout)
		if output == "" {
			res = g.executor.ExecuteCommand("kubectl", "get", "nodes", "-o", "jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-accelerator}{\"\\n\"}{end}")
			if res.ExitCode == 0 {
				output = strings.TrimSpace(res.Stdout)
			}
		}
	}
	return output, nil
}

func (g *GKEOrchestrator) parseAcceleratorOutput(output string) (string, error) {
	accelerators := make(map[string]bool)
	for _, acc := range strings.Split(output, "\n") {
		acc = strings.TrimSpace(acc)
		if acc != "" {
			accelerators[acc] = true
		}
	}

	if len(accelerators) == 0 {
		logging.Info("No hardware accelerators found. Defaulting to CPU-only workload.")
		return "", nil
	}

	uniqueAccels := make([]string, 0, len(accelerators))
	for acc := range accelerators {
		uniqueAccels = append(uniqueAccels, acc)
	}

	if len(uniqueAccels) == 1 {
		logging.Info("Auto-discovered Accelerator Type: %s", uniqueAccels[0])
		return uniqueAccels[0], nil
	}

	var sb strings.Builder
	sb.WriteString("Multiple Accelerator Types found on the cluster. Please specify which one you want to use with --accelerator.\n\n")
	sb.WriteString(fmt.Sprintf("%-30s\n", "ACCELERATOR TYPE"))
	sb.WriteString(fmt.Sprintf("%-30s\n", "----------------"))
	for _, acc := range uniqueAccels {
		sb.WriteString(fmt.Sprintf("%-30s\n", acc))
	}
	return "", fmt.Errorf("%s", sb.String())
}

func (g *GKEOrchestrator) resolveTopology(requested string, accelType string, clusterName string, clusterLocation string) (string, error) {
	if !strings.Contains(strings.ToLower(accelType), "tpu") {
		return "", nil // Rejects GPU topologies implicitly
	}

	top, handled, err := g.resolveSuperSlicingTopology(requested, clusterName, clusterLocation, accelType)
	if err != nil {
		return "", err
	}
	if handled {
		return top, nil
	}

	logging.Info("Auto-discovering Topology for %s...", accelType)

	output, err := g.queryDiscoveredTopologies()
	if err != nil {
		return "", err
	}

	if output == "" {
		return "", nil
	}

	topologies := g.parseTopologies(output)

	if len(topologies) == 0 {
		if requested != "" {
			logging.Info("Warning: No active topologies discovered from Kueue or Nodes. Fast-tracking provided topology: %s", requested)
			return requested, nil
		}
		return "", nil
	}

	if requested != "" {
		if !topologies[requested] {
			var valid []string
			for t := range topologies {
				valid = append(valid, t)
			}
			return "", fmt.Errorf("requested topology %s is not valid for cluster. Valid topologies discovered: %v", requested, valid)
		}
		logging.Info("Validated provided Topology: %s", requested)
		return requested, nil
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

func (g *GKEOrchestrator) resolveSuperSlicingTopology(requested string, clusterName string, clusterLocation string, accelType string) (string, bool, error) {
	if active, _ := g.verifySuperSlicingActive(ManifestOptions{
		ClusterName:     clusterName,
		ClusterLocation: clusterLocation,
		AcceleratorType: accelType,
	}); active {
		logging.Info("Super-slicing detected. Skipping strict physical state queries for topology.")
		if requested != "" {
			if len(strings.Split(requested, "x")) != 3 {
				return "", true, fmt.Errorf("invalid topology format %s. Must be AxBxC", requested)
			}
			logging.Info("Validated provided Topology (Super-Slicing): %s", requested)
			return requested, true, nil
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

func (g *GKEOrchestrator) queryDiscoveredTopologies() (string, error) {
	res := g.executor.ExecuteCommand("kubectl", "get", "resourceflavors.kueue.x-k8s.io", "-o", "jsonpath={range .items[*]}{.spec.nodeLabels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}")
	output := strings.TrimSpace(res.Stdout)

	if output == "" {
		res = g.executor.ExecuteCommand("kubectl", "get", "nodes", "-o", "jsonpath={range .items[*]}{.metadata.labels.cloud\\.google\\.com/gke-tpu-topology}{\"\\n\"}{end}")
		if res.ExitCode != 0 {
			return "", fmt.Errorf("failed to query Nodes for topology: %s", res.Stderr)
		}
		output = strings.TrimSpace(res.Stdout)
	}
	return output, nil
}

func (g *GKEOrchestrator) buildContainerImage(project, baseImage, buildContext, platformStr, imageName string) (string, error) {
	if baseImage != "" {
		logging.Info("Building container image using Crane (Go implementation) on top of %s...", baseImage)

		ignorePatterns := []string{
			".git", ".terraform", ".ghpc", ".ansible", "vendor", "bin", "pkg", "node_modules", "*.log", "tmp/", ".DS_Store", "__pycache__",
		}

		ignoreMatcher, err := imagebuilder.ReadDockerignorePatterns(buildContext, ignorePatterns)
		if err != nil {
			return "", fmt.Errorf("failed to read .dockerignore patterns: %w", err)
		}

		fullImageName, err := imagebuilder.BuildContainerImageFromBaseImage(
			project,
			baseImage,
			buildContext,
			platformStr,
			ignoreMatcher,
		)
		if err != nil {
			return "", fmt.Errorf("crane-based image build failed: %w", err)
		}
		logging.Info("Built image will be available at: %s", fullImageName)
		return fullImageName, nil
	} else if imageName != "" {
		logging.Info("Using pre-existing container image: %s", imageName)
		return imageName, nil
	} else {
		return "", fmt.Errorf("internal error: neither --image nor --base-image was provided, but CLI validation should have caught this")
	}
}

func (g *GKEOrchestrator) configureKubectl(clusterName, clusterLocation, projectID string) error {
	credsRes := g.executor.ExecuteCommand("gcloud", "container", "clusters", "get-credentials", clusterName, "--location", clusterLocation, "--project", projectID)
	if credsRes.ExitCode != 0 {
		if strings.Contains(strings.ToLower(credsRes.Stderr), "multiple") || strings.Contains(strings.ToLower(credsRes.Stderr), "ambiguous") {
			return fmt.Errorf("found multiple GKE clusters named %s. Please specify the exact Zone using --cluster-location to disambiguate.", clusterName)
		}
		return fmt.Errorf("failed to get GKE cluster credentials: %s\n%s", credsRes.Stderr, credsRes.Stdout)
	}
	return nil
}

func (g *GKEOrchestrator) BuildContainerImage(project, baseImage, buildContext, platformStr, imageName string) (string, error) {
	return g.buildContainerImage(project, baseImage, buildContext, platformStr, imageName)
}

func (g *GKEOrchestrator) PrepareManifestOptions(job orchestrator.JobDefinition, fullImageName string) (ManifestOptions, JobProfile, error) {
	opts, profile, err := g.prepareManifestOptions(job, fullImageName)
	return opts, profile, err
}

func (g *GKEOrchestrator) GeneratePathwaysManifest(job orchestrator.JobDefinition, fullImageName string) (string, error) {
	return g.generatePathwaysManifest(job, fullImageName)
}

func (g *GKEOrchestrator) ApplyManifest(manifestContent, outputManifestPath, workloadName string) error {
	return g.applyManifest(manifestContent, outputManifestPath, workloadName)
}

func (g *GKEOrchestrator) generateAndApplyManifest(opts ManifestOptions, profile JobProfile, outputManifestPath string) error {
	logging.Info("Generating GKE manifest...")
	gkeManifestContent, err := g.GenerateGKEManifest(opts, profile)
	if err != nil {
		return fmt.Errorf("failed to generate GKE manifest: %w", err)
	}

	return g.applyManifest(gkeManifestContent, outputManifestPath, opts.WorkloadName)
}

// Methods moved to infra_manager.go

func (g *GKEOrchestrator) GenerateGKENodeSelectorLabel(acceleratorType string) string {
	if strings.HasPrefix(acceleratorType, "v6e-") || strings.HasPrefix(acceleratorType, "v6e-slice-") {
		return "tpu-v6e-slice"
	}
	if strings.HasPrefix(acceleratorType, "v5p-") || strings.HasPrefix(acceleratorType, "v5p-slice-") {
		return "tpu-v5p-slice"
	}
	if strings.HasPrefix(acceleratorType, "l4-") {
		return "nvidia-l4"
	}
	if strings.HasPrefix(acceleratorType, "rtx-6000-") || strings.HasPrefix(acceleratorType, "rtx-pro-6000-") {
		return "nvidia-rtx-pro-6000"
	}
	if strings.Contains(acceleratorType, "tpu7x") {
		return "tpu7x"
	}
	switch acceleratorType {
	case "nvidia-tesla-a100":
		return "nvidia-tesla-a100"
	case "tpu-v4-podslice":
		return "tpu-v4-podslice"
	default:
		return acceleratorType
	}
}

// Methods moved to manifest_generator.go

func isTPUFallback(mapped string) bool {
	lower := strings.ToLower(mapped)
	return strings.Contains(lower, "tpu") || (len(lower) >= 2 && lower[0] == 'v' && lower[1] >= '0' && lower[1] <= '9')
}

// Methods moved to resource_resolver.go

func (g *GKEOrchestrator) prepareJobSetTemplateData(opts ManifestOptions, updatedCommand string) interface{} {
	return struct {
		WorkloadName            string
		KueueQueueName          string
		TtlSecondsAfterFinished int
		MaxRestarts             int
		NumSlices               int
		VmsPerSlice             int
		FullImageName           string
		CommandToRun            string
		AcceleratorTypeLabel    string
		NodeSelector            string
		Affinity                string
		PodFailurePolicy        string
		ImagePullSecrets        string
		ServiceAccountName      string
		TopologyAnnotation      string
		SchedulerName           string
		Tolerations             string
		PriorityClassName       string
		VolumesYAML             string
		VolumeMountsYAML        string
		GCSFuseEnabled          bool
		Pathways                orchestrator.PathwaysJobDefinition
	}{
		WorkloadName:            opts.WorkloadName,
		KueueQueueName:          opts.KueueQueueName,
		TtlSecondsAfterFinished: opts.TtlSecondsAfterFinished,
		MaxRestarts:             opts.MaxRestarts,
		NumSlices:               opts.NumSlices,
		VmsPerSlice:             opts.VmsPerSlice,
		FullImageName:           opts.FullImageName,
		CommandToRun:            updatedCommand,
		AcceleratorTypeLabel:    g.GenerateGKENodeSelectorLabel(opts.AcceleratorType),
		NodeSelector:            opts.NodeSelector,
		Affinity:                opts.Affinity,
		PodFailurePolicy:        opts.PodFailurePolicy,
		ImagePullSecrets:        opts.ImagePullSecrets,
		ServiceAccountName:      opts.ServiceAccountName,
		TopologyAnnotation:      opts.TopologyAnnotation,
		SchedulerName:           opts.SchedulerName,
		Tolerations:             opts.Tolerations,
		PriorityClassName:       opts.PriorityClassName,
		VolumesYAML:             opts.VolumesYAML,
		VolumeMountsYAML:        opts.VolumeMountsYAML,
		GCSFuseEnabled:          opts.GCSFuseEnabled,
		Pathways:                opts.Pathways,
	}
}

func (g *GKEOrchestrator) indentYaml(s string, indent int) string {
	lines := strings.Split(s, "\n")
	padding := strings.Repeat(" ", indent)
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, padding+line)
		}
	}
	return strings.Join(result, "\n")
}

// prepareManifestOptions moved to manifest_generator.go

func (g *GKEOrchestrator) determineIfCPUMachine(job orchestrator.JobDefinition) (bool, int, error) {
	if _, exists := acceleratorShorthandMap[job.AcceleratorType]; exists {
		return false, 0, nil
	}

	for _, realMachine := range acceleratorShorthandMap {
		if job.AcceleratorType == realMachine {
			return false, 0, nil
		}
	}

	mapped := g.GenerateGKENodeSelectorLabel(job.AcceleratorType)
	if strings.Contains(strings.ToLower(mapped), "nvidia") || isTPUFallback(mapped) {
		return false, 0, nil
	}

	if job.ClusterLocation != "" && job.AcceleratorType != "" {
		count, err := g.FetchMachineCapacity(job.AcceleratorType, job.ClusterLocation)
		if err != nil {
			return false, 0, fmt.Errorf("failed to describe machine type %s: %w", job.AcceleratorType, err)
		}
		if count > 0 {
			logging.Info("Dynamically determined %s is a CPU-only machine during manifest preparation", job.AcceleratorType)
			return true, count, nil
		}
	} else if job.ClusterLocation == "" && job.AcceleratorType != "" {
		logging.Warn("Zone is empty for machine type %s. Contextually treating it as a CPU machine for dry-run.", job.AcceleratorType)
		return true, 1, nil
	}
	return false, 0, nil
}

func (g *GKEOrchestrator) addVolumeOptions(opts *ManifestOptions, vols []orchestrator.VolumeDefinition) {
	if len(vols) == 0 {
		return
	}

	var volSpecs []map[string]interface{}
	var mountSpecs []map[string]interface{}
	gcsFuseEnabled := false

	for _, v := range vols {
		mountSpecs = append(mountSpecs, map[string]interface{}{
			"name":      v.Name,
			"mountPath": v.MountPath,
		})

		volSpec := map[string]interface{}{
			"name": v.Name,
		}

		switch v.Type {
		case "gcsfuse":
			gcsFuseEnabled = true
			volSpec["csi"] = map[string]interface{}{
				"driver":   "gcsfuse.csi.storage.gke.io",
				"readOnly": true,
				"volumeAttributes": map[string]interface{}{
					"bucketName": strings.TrimPrefix(v.Source, "gs://"),
				},
			}
		case "hostPath":
			volSpec["hostPath"] = map[string]interface{}{
				"path": v.Source,
			}
		case "pvc":
			volSpec["persistentVolumeClaim"] = map[string]interface{}{
				"claimName": v.Source,
			}
		}
		volSpecs = append(volSpecs, volSpec)
	}

	opts.GCSFuseEnabled = gcsFuseEnabled

	if b, err := yaml.Marshal(mountSpecs); err == nil {
		opts.VolumeMountsYAML = g.indentYaml(string(b), 16)
	}
	if b, err := yaml.Marshal(volSpecs); err == nil {
		opts.VolumesYAML = g.indentYaml(string(b), 14)
	}
}

func (g *GKEOrchestrator) parseJobStatus(obj map[string]interface{}) (statusStr, completionTime string) {
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

	if conditions, ok := statusMap["conditions"].([]interface{}); ok {
		for _, c := range conditions {
			cond := c.(map[string]interface{})
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)
			if condStatus == "True" {
				switch condType {
				case "Completed", "Succeeded":
					statusStr = "Succeeded"
				case "Failed":
					statusStr = "Failed"
				case "Suspended":
					statusStr = "Suspended"
				}
			}
		}
	}

	return
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
				"action": "Ignore",
				"onExitCodes": map[string]interface{}{
					"operator": "In",
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

func (g *GKEOrchestrator) ListJobs(opts orchestrator.ListOptions) ([]orchestrator.JobStatus, error) {
	logging.Info("Listing jobs in cluster '%s'...", opts.ClusterName)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return nil, err
	}

	client, err := g.getDynamicClient()
	if err != nil {
		return nil, err
	}

	gvr := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	list, err := client.Resource(gvr).Namespace("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list jobsets: %w", err)
	}

	var jobs []orchestrator.JobStatus
	for _, item := range list.Items {
		name := item.GetName()
		if opts.NameContains != "" && !strings.Contains(name, opts.NameContains) {
			continue
		}

		creationParams := item.GetCreationTimestamp()
		creationTime := creationParams.Time.Format(time.RFC3339)

		statusStr, completionTime := g.parseJobStatus(item.Object)

		if opts.Status != "" && !strings.EqualFold(statusStr, opts.Status) {
			continue
		}

		jobs = append(jobs, orchestrator.JobStatus{
			Name:           name,
			Status:         statusStr,
			CreationTime:   creationTime,
			CompletionTime: completionTime,
		})
	}

	return jobs, nil
}

func (g *GKEOrchestrator) CancelJob(name string, opts orchestrator.CancelOptions) error {
	logging.Info("Deleting job '%s' in cluster '%s'...", name, opts.ClusterName)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return err
	}

	client, err := g.getDynamicClient()
	if err != nil {
		return err
	}

	gvr := schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	err = client.Resource(gvr).Namespace("default").Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete jobset %s: %w", name, err)
	}

	logging.Info("Job '%s' deleted successfully.", name)
	return nil
}

func (g *GKEOrchestrator) GetJobLogs(name string, opts orchestrator.LogsOptions) (string, error) {
	logging.Info("Fetching logs for job '%s' in cluster '%s'...", name, opts.ClusterName)
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return "", err
	}

	// Check if JobSet exists
	checkRes := g.executor.ExecuteCommand("kubectl", "get", "jobset", name)
	if checkRes.ExitCode != 0 {
		if strings.Contains(strings.ToLower(checkRes.Stderr), "not found") || strings.Contains(strings.ToLower(checkRes.Stdout), "notfound") {
			return "", fmt.Errorf("job '%s' not found on cluster (it may have been cancelled or deleted)", name)
		}
		return "", fmt.Errorf("failed to verify job existence: %s", checkRes.Stderr)
	}

	// Retry loop for pulling logs, especially to handle ImagePullBackOff/waiting states
	maxRetries := 12 // 12 * 5s = 1 minute timeout
	var res shell.CommandResult
	for i := 0; i < maxRetries; i++ {
		res = g.executor.ExecuteCommand("kubectl", "logs", "-l", fmt.Sprintf("jobset.sigs.k8s.io/jobset-name=%s", name), "--all-containers")
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

	if strings.TrimSpace(res.Stdout) == "" {
		return "Job exists but has no live logs available (it may have finished or failed to start pods)", nil
	}

	return res.Stdout, nil
}

func (g *GKEOrchestrator) getDynamicClient() (dynamic.Interface, error) {

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	return dynamic.NewForConfig(config)
}

func (g *GKEOrchestrator) waitForJobCompletion(workloadName, clusterName, clusterLocation, projectID string) error {
	logging.Info("Waiting for job '%s' to complete...", workloadName)

	// kubectl wait --for jsonpath='.status.conditions[-1].type'=Finished jobset <workloadName> --timeout=1h
	waitRes := g.executor.ExecuteCommand("kubectl", "wait", "--for", "jsonpath={.status.conditions[-1].type}=Finished",
		"jobset", workloadName, "--timeout=1h")

	jobConsoleLink := fmt.Sprintf("https://console.cloud.google.com/kubernetes/workload/gke/%s/%s/details/%s?project=%s",
		clusterLocation, clusterName, workloadName, projectID)

	if waitRes.ExitCode != 0 {
		if strings.Contains(waitRes.Stderr, "timed out waiting") || strings.Contains(waitRes.Stdout, "timed out waiting") {
			logging.Error("Timed out waiting for job '%s' to finish. Check its status in the Cloud Console: %s", workloadName, jobConsoleLink)
			return fmt.Errorf("job timed out")
		}
		return fmt.Errorf("error waiting for job completion: %s\n%s", waitRes.Stderr, waitRes.Stdout)
	}

	logging.Info("Job '%s' has finished. Checking final status...", workloadName)

	// kubectl get jobset <workloadName> -o jsonpath='{.status.conditions[-1].type}'
	statusRes := g.executor.ExecuteCommand("kubectl", "get", "jobset", workloadName, "-o", "jsonpath={.status.conditions[-1].type}")

	if statusRes.ExitCode != 0 {
		return fmt.Errorf("failed to get final job status: %s\n%s", statusRes.Stderr, statusRes.Stdout)
	}

	finalStatus := strings.TrimSpace(statusRes.Stdout)
	if finalStatus != "Completed" {
		logging.Error("Job '%s' finished with status '%s'. Check details in the Cloud Console: %s", workloadName, finalStatus, jobConsoleLink)
		return fmt.Errorf("job completed unsuccessfully with status: %s", finalStatus)
	}

	logging.Info("Job '%s' completed successfully. View details in the Cloud Console: %s", workloadName, jobConsoleLink)
	return nil
}

func (g *GKEOrchestrator) buildNodeSelector(schedOpts SchedulingOptions, job orchestrator.JobDefinition, isSuperSlicing bool, isCPUMachine bool) (string, error) {
	nodeSelector := GetNodeSelector(schedOpts)
	accelLabel := g.GenerateGKENodeSelectorLabel(job.AcceleratorType)

	isGPU := strings.Contains(strings.ToLower(accelLabel), "nvidia")

	if accelLabel != "" && !isCPUMachine {
		if nodeSelector == nil {
			nodeSelector = make(map[string]string)
		}
		if strings.Contains(accelLabel, "tpu-v6e") || strings.Contains(accelLabel, "tpu7x") {
			nodeSelector["cloud.google.com/gke-tpu-accelerator"] = accelLabel
		} else {
			nodeSelector["cloud.google.com/gke-accelerator"] = accelLabel
		}
	}

	if schedOpts.Topology != "" {
		if isGPU {
			return "", fmt.Errorf("topology is not allowed for GPU jobs")
		}
		if nodeSelector == nil {
			nodeSelector = make(map[string]string)
		}
		if !isSuperSlicing {
			nodeSelector["cloud.google.com/gke-tpu-topology"] = schedOpts.Topology
		}
	}

	if len(nodeSelector) > 0 {
		b, err := yaml.Marshal(nodeSelector)
		if err != nil {
			return "", fmt.Errorf("failed to marshal nodeSelector: %w", err)
		}
		return g.indentYaml(string(b), 16), nil
	}
	return "", nil
}

func (g *GKEOrchestrator) buildAffinity(schedOpts SchedulingOptions) (string, error) {
	if affinity := GetAffinity(schedOpts); affinity != nil {
		b, err := k8syaml.Marshal(affinity)
		if err != nil {
			return "", fmt.Errorf("failed to marshal affinity: %w", err)
		}
		return g.indentYaml(string(b), 16), nil
	}
	return "", nil
}

func (g *GKEOrchestrator) buildTopologyAnnotation(topology string) string {
	topologyAnnotation := GetTopologyAnnotation(topology)
	if len(topologyAnnotation) > 0 {
		b, err := yaml.Marshal(topologyAnnotation)
		if err == nil {
			return g.indentYaml(string(b), 16)
		}
	}
	return ""
}
