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
	"os"
	"slices"
	"time"

	"hpc-toolkit/pkg/orchestrator"

	"strings"

	"github.com/spf13/cobra"
)

var (
	imageName       string
	baseImage       string
	buildContext    string
	commandToRun    string
	acceleratorType string
	dryRunManifest  string

	workloadName     string
	kueueQueueName   string
	numSlicesOrNodes int
	vmsPerSlice      int
	maxRestarts      int
	ttlAfterFinished string
	gracePeriodStr   string

	placementPolicy string
	nodeConstraint  map[string]string

	cpuAffinityStr     string
	restartOnExitCodes []int
	imagePullSecrets   string
	serviceAccountName string
	topology           string
	gkeScheduler       string
	platform           string

	awaitJobCompletion bool
	timeoutStr         string
	priorityClassName  string
	isPathwaysJob      bool
	verbose            bool

	volumeStr []string
	pathways  orchestrator.PathwaysJobDefinition
)

var SubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submits a workload on a GKE cluster using JobSet.",
	Long: `The 'submit' command deploys a workload (Kubernetes JobSet)
on a GKE cluster, integrated with Kueue. Image can be pre-built (--image)
or built on-the-fly using Crane (--base-image with --build-context).

It accepts parameters for the container image, command to execute, accelerator type,
and JobSet/Kueue specific configurations like workload name, queue, nodes, and restarts.`,
	RunE: runSubmitCmd,

	PreRunE: func(cmd *cobra.Command, args []string) error {
		if err := validateImageFlags(); err != nil {
			return err
		}

		if err := ensurePrerequisites(cmd, &projectID, location); err != nil {
			return err
		}

		priorityClassName = strings.ToLower(priorityClassName)
		if priorityClassName != "" && !slices.Contains(orchestrator.ValidPriorityClasses, priorityClassName) {
			return fmt.Errorf("invalid value for --priority: %s. Allowed values are: %s",
				priorityClassName, strings.Join(orchestrator.ValidPriorityClasses, ", "))
		}

		return nil
	},
	SilenceUsage: true,
}

func init() {
	SubmitCmd.Flags().StringVarP(&imageName, "image", "i", "", "Name of the pre-built container image to run. Must include the full path including registry (e.g., us-docker.pkg.dev/my-project/my-repo/my-image:tag).")
	SubmitCmd.Flags().StringVarP(&baseImage, "base-image", "B", "", "Name of the base image for Crane to build upon (e.g., python:3.9-slim). Requires --build-context.")
	SubmitCmd.Flags().StringVarP(&buildContext, "build-context", "b", "", "Path to the build context directory for Crane (e.g., .). Required with --base-image.")
	SubmitCmd.Flags().StringVarP(&commandToRun, "command", "e", "", "Command to execute in the container (e.g., 'python train.py'). Required.")
	SubmitCmd.Flags().StringVarP(&acceleratorType, "accelerator", "a", "", "Type of accelerator to request (e.g., 'nvidia-tesla-a100').")
	SubmitCmd.Flags().StringVarP(&dryRunManifest, "dry-run-out", "o", "", "Path to output the generated Kubernetes manifest instead of applying it.")
	SubmitCmd.Flags().StringVarP(&platform, "platform", "f", "linux/amd64", "Target platform for the image build (e.g., 'linux/amd64', 'linux/arm64'). Used with --base-image.")

	SubmitCmd.Flags().StringVarP(&workloadName, "name", "n", "", "Name of the workload to create. Required.")
	SubmitCmd.Flags().StringVarP(&kueueQueueName, "queue", "q", "", "Name of the Kueue LocalQueue to submit the workload to. If empty, it will be auto-discovered.")
	SubmitCmd.Flags().IntVar(&numSlicesOrNodes, "nodes", 1, "Number of JobSet replicas (or Slices for TPUs).")
	SubmitCmd.Flags().IntVar(&vmsPerSlice, "vms-per-slice", 0, "Number of VMs (pods) per slice. Defaults to auto-calculated value for TPUs.")
	SubmitCmd.Flags().IntVar(&maxRestarts, "max-restarts", 1, "Maximum number of restarts for the JobSet before failing.")
	SubmitCmd.Flags().StringVar(&ttlAfterFinished, "gke-ttl-after-finished", "1h", "Time to retain the JobSet after it finishes (e.g. 5m, 1h).")
	SubmitCmd.Flags().StringVar(&gracePeriodStr, "grace-period", "30s", "Time to wait before forcefully terminating a pod (e.g. 30s, 2m). Gives the workload time to save checkpoints or clean up distributed state during cancellation or preemption events (like Spot VM evictions).")

	SubmitCmd.Flags().StringVar(&placementPolicy, "placement-policy", "", "Name of the GKE placement policy to use.")
	SubmitCmd.Flags().StringToStringVar(&nodeConstraint, "node-constraint", nil, "Key=value pairs for node labels to target specific nodes. Maps to nodeSelector in GKE, and to SLURM's --constraint.")
	SubmitCmd.Flags().StringVar(&cpuAffinityStr, "cpu-affinity", "", "CPU affinity rules (e.g., 'numa').")
	SubmitCmd.Flags().IntSliceVar(&restartOnExitCodes, "restart-on-exit-codes", nil, "List of exit codes that should not trigger a job failure.")
	SubmitCmd.Flags().StringVar(&imagePullSecrets, "image-pull-secret", "", "Comma-separated list of secrets for pulling images.")
	SubmitCmd.Flags().StringVar(&serviceAccountName, "service-account", "", "Service account name for the pods.")
	SubmitCmd.Flags().StringVar(&topology, "topology", "", "TPU slice topology (e.g., 2x2x1).")
	SubmitCmd.Flags().StringVar(&gkeScheduler, "gke-scheduler", "", "Kubernetes Scheduler name (e.g., gke.io/topology-aware-auto).")
	SubmitCmd.Flags().BoolVar(&awaitJobCompletion, "await-job-completion", false, "If true, gcluster will wait for the submitted job to complete.")
	SubmitCmd.Flags().StringVar(&timeoutStr, "timeout", "-1s", "Time to wait for job in seconds or string format (e.g. 1h, 10m). Default is max timeout (-1s).")
	SubmitCmd.Flags().StringVar(&priorityClassName, "priority", "medium", "A priority, one of `very-low`, `low`, `medium`, `high` or `very-high`. Defaults to `medium`.")

	SubmitCmd.Flags().BoolVar(&isPathwaysJob, "pathways", false, "If present, gcluster will generate a manifest for a Pathways job.")
	SubmitCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose logging for the workload (TPUs and GPUs).")
	SubmitCmd.Flags().StringVar(&pathways.ProxyServerImage, "pathways-proxy-server-image", "", "The image for the Pathways proxy server.")
	SubmitCmd.Flags().StringVar(&pathways.ServerImage, "pathways-server-image", "", "The image for the Pathways server.")
	SubmitCmd.Flags().StringVar(&pathways.WorkerImage, "pathways-worker-image", "", "The image for the Pathways worker.")
	SubmitCmd.Flags().BoolVar(&pathways.Headless, "pathways-headless", false, "If present, the user's workload container will not be deployed within the `pathways-head` job.")
	SubmitCmd.Flags().StringVar(&pathways.GCSLocation, "pathways-gcs-location", "", "Please provide the GCS location to store Pathways artifacts. This flag is required when using --pathways.")
	SubmitCmd.Flags().IntVar(&pathways.ElasticSlices, "pathways-elastic-slices", 0, "Configures the number of elastic slices, potentially allowing for more flexible resource allocation.")
	SubmitCmd.Flags().IntVar(&pathways.MaxSliceRestarts, "pathways-max-slice-restarts", 1, "Maximum times the workers in a slice can be restarted. Used with --pathways-elastic-slices.")
	SubmitCmd.Flags().StringVar(&pathways.ProxyArgs, "pathways-proxy-args", "", "Arbitrary additional command-line arguments to pass directly to the `pathways-proxy` executable.")
	SubmitCmd.Flags().StringVar(&pathways.ServerArgs, "pathways-server-args", "", "Arbitrary additional command-line arguments to pass directly to the `pathways-rm` (resource manager) executable.")
	SubmitCmd.Flags().StringVar(&pathways.WorkerArgs, "pathways-worker-args", "", "Arbitrary additional command-line arguments to pass directly to the `pathways-worker` executable.")
	SubmitCmd.Flags().StringVar(&pathways.ColocatedPythonSidecarImage, "pathways-colocated-python-sidecar-image", "", "Image for an optional Python-based sidecar container to run alongside the Pathways head components.")
	SubmitCmd.Flags().StringVar(&pathways.HeadNodePool, "pathways-head-np", "", "The node pool to use for the Pathways head job. If empty, it will be auto-detected (looking for 'cpu-np' or 'pathways-np').")

	SubmitCmd.Flags().StringSliceVar(&volumeStr, "mount", nil, "Volumes to mount (format: <src>:<dest>[:<mode>], mode can be 'ro' or 'rw', default 'ro').")

	_ = SubmitCmd.MarkFlagRequired("command")
	_ = SubmitCmd.MarkFlagRequired("name")
	_ = SubmitCmd.MarkFlagRequired("accelerator")
}

func runSubmitCmd(cmd *cobra.Command, args []string) error {
	ttlSeconds, err := parseDurationToSeconds(ttlAfterFinished, "--gke-ttl-after-finished")
	if err != nil {
		return err
	}

	gracePeriodSeconds, err := parseDurationToSeconds(gracePeriodStr, "--grace-period")
	if err != nil {
		return err
	}

	affinity := map[string]string{}
	if cpuAffinityStr != "" {
		affinity["cpu-affinity"] = cpuAffinityStr
	}

	vols, err := parseVolumeFlag(volumeStr)
	if err != nil {
		return err
	}

	if timeoutStr != "-1s" {
		awaitJobCompletion = true
	}

	jobDef := orchestrator.JobDefinition{
		ImageName:                     imageName,
		BaseImage:                     baseImage,
		BuildContext:                  buildContext,
		Platform:                      platform,
		CommandToRun:                  commandToRun,
		AcceleratorType:               acceleratorType,
		DryRunManifest:                dryRunManifest,
		ProjectID:                     projectID,
		ClusterName:                   clusterName,
		ClusterLocation:               location,
		WorkloadName:                  workloadName,
		KueueQueueName:                kueueQueueName,
		NumSlices:                     numSlicesOrNodes,
		VmsPerSlice:                   vmsPerSlice,
		MaxRestarts:                   maxRestarts,
		TtlSecondsAfterFinished:       ttlSeconds,
		TerminationGracePeriodSeconds: gracePeriodSeconds,
		PlacementPolicy:               placementPolicy,
		NodeConstraint:                nodeConstraint,
		Affinity:                      affinity,
		RestartOnExitCodes:            restartOnExitCodes,
		ImagePullSecrets:              imagePullSecrets,
		ServiceAccountName:            serviceAccountName,
		Topology:                      topology,
		GKEScheduler:                  gkeScheduler,
		AwaitJobCompletion:            awaitJobCompletion,
		Timeout:                       timeoutStr,
		PriorityClassName:             priorityClassName,
		IsPathwaysJob:                 isPathwaysJob,
		Pathways:                      pathways,
		Volumes:                       vols,
		Verbose:                       verbose,
	}

	return orc.SubmitJob(jobDef)
}

func parseVolumeFlag(vStrs []string) ([]orchestrator.VolumeDefinition, error) {
	var vols []orchestrator.VolumeDefinition
	seenSources := make(map[string]bool)
	seenDestinations := make(map[string]bool)

	for i, vStr := range vStrs {
		src, dest, readOnly, err := parseSingleVolume(vStr)
		if err != nil {
			return nil, err
		}

		if seenSources[src] {
			return nil, fmt.Errorf("duplicate volume source: %s", src)
		}
		if seenDestinations[dest] {
			return nil, fmt.Errorf("duplicate volume destination: %s", dest)
		}
		seenSources[src] = true
		seenDestinations[dest] = true

		volType := "pvc"
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
			ReadOnly:  readOnly,
		})
	}
	return vols, nil
}

func parseSingleVolume(vStr string) (src, dest string, readOnly bool, err error) {
	readOnly = true
	idx := strings.LastIndex(vStr, ":")
	if idx <= 0 || idx == len(vStr)-1 {
		return "", "", false, fmt.Errorf("invalid volume format: %s. Expected format: <src>:<dest>[:<mode>]", vStr)
	}

	lastPart := vStr[idx+1:]
	srcDestPart := vStr[:idx]

	if lastPart == "ro" || lastPart == "rw" {
		readOnly = (lastPart == "ro")
		idx = strings.LastIndex(srcDestPart, ":")
		if idx <= 0 || idx == len(srcDestPart)-1 {
			return "", "", false, fmt.Errorf("invalid volume format: %s. Expected format: <src>:<dest>[:<mode>]", vStr)
		}
		src = srcDestPart[:idx]
		dest = srcDestPart[idx+1:]
	} else {
		src = srcDestPart
		dest = lastPart

		if strings.HasPrefix(vStr, "gs://") && !strings.HasPrefix(src, "gs://") {
			return "", "", false, fmt.Errorf("invalid volume format: %s. Missing destination.", vStr)
		}

		if strings.Contains(src, ":") {
			if strings.HasPrefix(src, "gs://") {
				if strings.Contains(src[5:], ":") {
					return "", "", false, fmt.Errorf("invalid volume format: %s", vStr)
				}
			} else {
				return "", "", false, fmt.Errorf("invalid volume format: %s", vStr)
			}
		}
	}
	return src, dest, readOnly, nil
}

func parseDurationToSeconds(dStr string, flagName string) (int, error) {
	d, err := time.ParseDuration(dStr)
	if err == nil {
		return int(d.Seconds()), nil
	}

	var seconds int
	if _, err := fmt.Sscanf(dStr, "%d", &seconds); err == nil {
		return seconds, nil
	}

	return 0, fmt.Errorf("invalid duration format for %s: %s. Expected formats: 1h, 30m, 3600", flagName, dStr)
}

func validateImageFlags() error {
	if err := validateImageSources(); err != nil {
		return err
	}
	return validateBuildContext()
}

func validateImageSources() error {
	if (imageName == "" && baseImage == "") || (buildContext != "" && baseImage == "") {
		return fmt.Errorf("either --image or --base-image must be provided")
	}
	if imageName != "" && buildContext != "" {
		return fmt.Errorf("--build-context cannot be provided when --image is used as no build is performed")
	}
	if baseImage != "" && buildContext == "" {
		return fmt.Errorf("a --build-context must be provided when --base-image is used for a Crane build")
	}
	return nil
}

func validateBuildContext() error {
	if buildContext == "" {
		return nil
	}
	if os.Getenv("GCLUSTER_IMAGE_REPO") == "" {
		return fmt.Errorf("GCLUSTER_IMAGE_REPO environment variable is required when using --build-context. Please set it in your environment with the repository name only (e.g., export GCLUSTER_IMAGE_REPO=gcluster-repo).")
	}
	if os.Getenv("USER") == "" && os.Getenv("USERNAME") == "" {
		return fmt.Errorf("failed to determine user identity from environment (tried USER and USERNAME). This is required to ensure unique image tagging when using --build-context")
	}
	return nil
}
