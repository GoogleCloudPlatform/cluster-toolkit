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

package job

import (
	"fmt"
	"net/url"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/gke"
	"strings"

	"github.com/spf13/cobra"
)

var (
	imageName       string
	baseImage       string
	buildContext    string
	commandToRun    string
	acceleratorType string
	outputManifest  string

	workloadName            string
	kueueQueueName          string
	numSlicesOrNodes        int
	vmsPerSlice             int
	maxRestarts             int
	ttlSecondsAfterFinished int

	placementPolicy string
	nodeSelector    map[string]string

	cpuAffinityStr     string
	restartOnExitCodes []int
	imagePullSecrets   string
	serviceAccountName string
	topology           string
	scheduler          string
	platform           string

	awaitJobCompletion bool
	timeoutStr         string
	priorityClassName  string
	isPathwaysJob      bool
	verbose            bool

	volumeStr []string
	pathways  orchestrator.PathwaysJobDefinition
)

var gkeOrchestratorFactory = func() (*gke.GKEOrchestrator, error) {
	return gke.NewGKEOrchestrator()
}

var SubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submits a container image workload on a Gke cluster using JobSet.",
	Long: `The 'submit' command deploys a container image as a workload (Kubernetes JobSet)
on a GKE cluster, integrated with Kueue. Image can be pre-built (--image)
or built on-the-fly using Crane (--base-image with --build-context).

It accepts parameters for the container image, command to execute, accelerator type,
and JobSet/Kueue specific configurations like workload name, queue, nodes, and restarts.`,
	RunE: runSubmitCmd,

	PreRunE: func(cmd *cobra.Command, args []string) error {
		logging.Info("Running prerequisite checks for 'gcluster job submit'...")
		err := EnsurePrerequisites(&projectID)
		if err != nil {
			return fmt.Errorf("prerequisite checks failed for 'gcluster job submit'. Please ensure your gcloud configuration and cluster context are valid: %w", err)
		}

		allowedPriorities := map[string]bool{
			"very-low":  true,
			"low":       true,
			"medium":    true,
			"high":      true,
			"very-high": true,
		}

		if priorityClassName != "" && !allowedPriorities[priorityClassName] {
			return fmt.Errorf("invalid value for --priority: %s. Allowed values are: very-low, low, medium, high, very-high", priorityClassName)
		}

		logging.Info("Prerequisite checks completed successfully.")
		return nil
	},
	SilenceUsage: true,
}

func init() {
	SubmitCmd.Flags().StringVarP(&imageName, "image", "i", "", "Name of the pre-built container image to run (e.g., my-project/my-image:tag).")
	SubmitCmd.Flags().StringVar(&baseImage, "base-image", "", "Name of the base image for Crane to build upon (e.g., python:3.9-slim). Requires --build-context.")
	SubmitCmd.Flags().StringVarP(&buildContext, "build-context", "c", "", "Path to the build context directory for Crane (e.g., .). Required with --base-image.")
	SubmitCmd.Flags().StringVarP(&commandToRun, "command", "e", "", "Command to execute in the container (e.g., 'python train.py'). Required.")
	SubmitCmd.Flags().StringVarP(&acceleratorType, "accelerator", "a", "", "Type of accelerator to request (e.g., 'nvidia-tesla-a100'). If empty, it will be auto-discovered.")
	SubmitCmd.Flags().StringVarP(&outputManifest, "dry-run-out", "o", "", "Path to output the generated Kubernetes manifest instead of applying it.")
	SubmitCmd.Flags().StringVarP(&platform, "platform", "f", "linux/amd64", "Target platform for the image build (e.g., 'linux/amd64', 'linux/arm64'). Used with --base-image.")

	SubmitCmd.Flags().StringVarP(&workloadName, "name", "n", "", "Name of the workload to create. Required.")
	SubmitCmd.Flags().StringVar(&kueueQueueName, "kueue-queue", "", "Name of the Kueue LocalQueue to submit the workload to. If empty, it will be auto-discovered.")
	SubmitCmd.Flags().IntVar(&numSlicesOrNodes, "nodes", 1, "Number of JobSet replicas (or Slices for TPUs).")
	SubmitCmd.Flags().IntVar(&vmsPerSlice, "vms-per-slice", 1, "Number of VMs (pods) per slice.")
	SubmitCmd.Flags().IntVar(&maxRestarts, "max-restarts", 1, "Maximum number of restarts for the JobSet before failing.")
	SubmitCmd.Flags().IntVar(&ttlSecondsAfterFinished, "ttl", 3600, "Time (in seconds) to retain the JobSet after it finishes.")

	SubmitCmd.Flags().StringVar(&placementPolicy, "placement-policy", "", "Name of the GKE placement policy to use.")
	SubmitCmd.Flags().StringToStringVar(&nodeSelector, "machine-label", nil, "Key=value pairs for node labels to target specific machine types.")
	SubmitCmd.Flags().StringVar(&cpuAffinityStr, "cpu-affinity", "", "CPU affinity rules (e.g., 'numa').")
	SubmitCmd.Flags().IntSliceVar(&restartOnExitCodes, "restart-on-exit-codes", nil, "List of exit codes that should not trigger a job failure.")
	SubmitCmd.Flags().StringVar(&imagePullSecrets, "image-pull-secret", "", "Comma-separated list of secrets for pulling images.")
	SubmitCmd.Flags().StringVar(&serviceAccountName, "service-account", "", "Service account name for the pods.")
	SubmitCmd.Flags().StringVar(&topology, "topology", "", "TPU slice topology (e.g., 2x2x1).")
	SubmitCmd.Flags().StringVar(&scheduler, "scheduler", "", "Kubernetes Scheduler name (e.g., gke.io/topology-aware-auto).")
	SubmitCmd.Flags().BoolVar(&awaitJobCompletion, "await-job-completion", false, "If true, gcluster will wait for the submitted job to complete.")
	SubmitCmd.Flags().StringVar(&timeoutStr, "timeout", "-1s", "Time to wait for job in seconds or string format (e.g. 1h, 10m). Default is max timeout (-1s).")
	SubmitCmd.Flags().StringVar(&priorityClassName, "priority", "medium", "A priority, one of `very-low`, `low`, `medium`, `high` or `very-high`. Defaults to `medium`.")

	SubmitCmd.Flags().BoolVar(&isPathwaysJob, "pathways", false, "If present, gcluster will generate a manifest for a Pathways job.")
	SubmitCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose logging for the workload (TPUs and GPUs).")
	SubmitCmd.Flags().StringVar(&pathways.ProxyServerImage, "pathways-proxy-server-image", "", "The image for the Pathways proxy server.")
	SubmitCmd.Flags().StringVar(&pathways.ServerImage, "pathways-server-image", "", "The image for the Pathways server.")
	SubmitCmd.Flags().StringVar(&pathways.WorkerImage, "pathways-worker-image", "", "The image for the Pathways worker.")
	SubmitCmd.Flags().BoolVar(&pathways.Headless, "pathways-headless", false, "If present, the user's workload container will not be deployed within the `pathways-head` job.")
	SubmitCmd.Flags().StringVar(&pathways.GCSLocation, "pathways-gcs-location", "", "A Google Cloud Storage (GCS) bucket location to be used by Pathways for temporary files, checkpoints, and inter-worker communication.")
	SubmitCmd.Flags().IntVar(&pathways.ElasticSlices, "pathways-elastic-slices", 0, "Configures the number of elastic slices, potentially allowing for more flexible resource allocation.")
	SubmitCmd.Flags().IntVar(&pathways.MaxSliceRestarts, "pathways-max-slice-restarts", 1, "Maximum times the workers in a slice can be restarted. Used with --pathways-elastic-slices.")
	SubmitCmd.Flags().StringVar(&pathways.ProxyArgs, "pathways-proxy-args", "", "Arbitrary additional command-line arguments to pass directly to the `pathways-proxy` executable.")
	SubmitCmd.Flags().StringVar(&pathways.ServerArgs, "pathways-server-args", "", "Arbitrary additional command-line arguments to pass directly to the `pathways-rm` (resource manager) executable.")
	SubmitCmd.Flags().StringVar(&pathways.WorkerArgs, "pathways-worker-args", "", "Arbitrary additional command-line arguments to pass directly to the `pathways-worker` executable.")
	SubmitCmd.Flags().StringVar(&pathways.ColocatedPythonSidecarImage, "pathways-colocated-python-sidecar-image", "", "Image for an optional Python-based sidecar container to run alongside the Pathways head components.")

	SubmitCmd.Flags().StringSliceVar(&volumeStr, "mount", nil, "Volumes to mount (format: <src>:<dest>).")

	_ = SubmitCmd.MarkFlagRequired("command")
	_ = SubmitCmd.MarkFlagRequired("cluster")
}

func runSubmitCmd(cmd *cobra.Command, args []string) error {
	logging.Info("Executing gcluster job submit command...")

	if err := validateImageFlags(); err != nil {
		return err
	}

	affinity := map[string]string{}
	if cpuAffinityStr != "" {
		affinity["cpu-affinity"] = cpuAffinityStr
	}

	vols, err := parseVolumeFlag(volumeStr)
	if err != nil {
		return fmt.Errorf("invalid volume configuration: %w", err)
	}

	if timeoutStr != "-1s" {
		awaitJobCompletion = true
	}

	jobDef := orchestrator.JobDefinition{
		ImageName:               imageName,
		BaseImage:               baseImage,
		BuildContext:            buildContext,
		Platform:                platform,
		CommandToRun:            commandToRun,
		AcceleratorType:         acceleratorType,
		OutputManifest:          outputManifest,
		ProjectID:               projectID,
		ClusterName:             clusterName,
		ClusterLocation:         clusterLocation,
		WorkloadName:            workloadName,
		KueueQueueName:          kueueQueueName,
		NumSlices:               numSlicesOrNodes,
		VmsPerSlice:             vmsPerSlice,
		MaxRestarts:             maxRestarts,
		TtlSecondsAfterFinished: ttlSecondsAfterFinished,
		PlacementPolicy:         placementPolicy,
		NodeSelector:            nodeSelector,
		Affinity:                affinity,
		RestartOnExitCodes:      restartOnExitCodes,
		ImagePullSecrets:        imagePullSecrets,
		ServiceAccountName:      serviceAccountName,
		Topology:                topology,
		Scheduler:               scheduler,
		AwaitJobCompletion:      awaitJobCompletion,
		Timeout:                 timeoutStr,
		PriorityClassName:       priorityClassName,
		IsPathwaysJob:           isPathwaysJob,
		Pathways:                pathways,
		Volumes:                 vols,
		Verbose:                 verbose,
	}

	if err := submitGKEJob(jobDef); err != nil {
		return fmt.Errorf("failed to submit job to GKE cluster '%s' in location '%s': %w", clusterName, clusterLocation, err)
	}

	if outputManifest == "" {
		printPantheonLinks(jobDef)
	}
	return nil
}

func parseVolumeFlag(vStrs []string) ([]orchestrator.VolumeDefinition, error) {
	var vols []orchestrator.VolumeDefinition
	for i, vStr := range vStrs {
		idx := strings.LastIndex(vStr, ":")
		if idx == -1 {
			return nil, fmt.Errorf("invalid volume format: %s. Expected format: <src>:<dest>", vStr)
		}
		src := vStr[:idx]
		dest := vStr[idx+1:]

		volType := "pvc" // Default
		if strings.HasPrefix(src, "gs://") {
			volType = "gcsfuse"
		} else if strings.HasPrefix(src, "/") {
			volType = "hostPath"
		}

		vols = append(vols, orchestrator.VolumeDefinition{
			Name:      fmt.Sprintf("vol-%d", i),
			Source:    src,
			MountPath: dest,
			Type:      volType,
		})
	}
	return vols, nil
}

func validateImageFlags() error {
	if imageName == "" && baseImage == "" {
		return fmt.Errorf("either --image or --base-image must be provided")
	}
	if imageName != "" && baseImage != "" {
		return fmt.Errorf("cannot provide both --image and --base-image")
	}
	if imageName != "" && buildContext != "" {
		return fmt.Errorf("--build-context cannot be provided when --image is used as no build is performed")
	}
	if baseImage != "" && buildContext == "" {
		return fmt.Errorf("a --build-context must be provided when --base-image is used for a Crane build")
	}
	return nil
}

func submitGKEJob(jobDef orchestrator.JobDefinition) error {
	gkeOrchestrator, err := gkeOrchestratorFactory()
	if err != nil {
		return fmt.Errorf("failed to initialize GKE orchestrator. Check if kubectl is configured and cluster '%s' is accessible: %v", jobDef.ClusterName, err)
	}

	if outputManifest == "" {
		return gkeOrchestrator.SubmitJob(jobDef)
	}

	fullImageName, err := gkeOrchestrator.BuildContainerImage(jobDef.ProjectID, jobDef.BaseImage, jobDef.BuildContext, jobDef.Platform, jobDef.ImageName)
	if err != nil {
		return fmt.Errorf("failed to build container image: %v", err)
	}

	var manifestContent string
	if jobDef.IsPathwaysJob {
		manifestContent, err = gkeOrchestrator.GeneratePathwaysManifest(jobDef, fullImageName)
		if err != nil {
			return fmt.Errorf("failed to generate pathways manifest: %v", err)
		}
	} else {
		manifestOpts, profile, err := gkeOrchestrator.PrepareManifestOptions(jobDef, fullImageName)
		if err != nil {
			return fmt.Errorf("failed to prepare manifest options: %v", err)
		}
		manifestContent, err = gkeOrchestrator.GenerateGKEManifest(manifestOpts, profile)

		if err != nil {
			return fmt.Errorf("failed to generate GKE manifest: %v", err)
		}
	}
	return gkeOrchestrator.ApplyManifest(manifestContent, outputManifest, jobDef.WorkloadName)
}

func printPantheonLinks(job orchestrator.JobDefinition) {
	gkeLink := fmt.Sprintf("https://console.cloud.google.com/kubernetes/job/%s/%s/default/%s/details?project=%s",
		job.ClusterLocation, job.ClusterName, job.WorkloadName, job.ProjectID)

	logging.Info("Follow your workload details here: %s", gkeLink)

	logFilter := fmt.Sprintf(`resource.type="k8s_container"
resource.labels.project_id="%s"
resource.labels.location="%s"
resource.labels.cluster_name="%s"
resource.labels.namespace_name="default"
resource.labels.pod_name:"%s-"
severity>=DEFAULT`, job.ProjectID, job.ClusterLocation, job.ClusterName, job.WorkloadName)

	encodedFilter := url.QueryEscape(logFilter)
	logsLink := fmt.Sprintf("https://console.cloud.google.com/logs/query;query=%s;storageScope=project;duration=P1D?project=%s",
		encodedFilter, job.ProjectID)

	logging.Info("View your workload logs in real-time here: %s", logsLink)
}
