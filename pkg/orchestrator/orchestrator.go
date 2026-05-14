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

package orchestrator

var ValidPriorityClasses = []string{"very-low", "low", "medium", "high", "very-high"}

type PathwaysJobDefinition struct {
	// Core Pathways Images
	ProxyServerImage string // Default: us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest
	ServerImage      string // Default: us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest
	WorkerImage      string // Default: us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest (or value of ServerImage)

	// Pathways specific configurations
	Headless         bool   // Default: false
	GCSLocation      string // Required for Pathways jobs.
	ElasticSlices    int    // Default: 0
	MaxSliceRestarts int    // Default: 1

	// Custom Arguments for Pathways components
	ProxyArgs  string // Default: ""
	ServerArgs string // Default: ""
	WorkerArgs string // Default: ""

	// Pathways-specific sidecars
	ColocatedPythonSidecarImage string // Default: ""

	HeadNodePool string // Resolved node pool to use for the Pathways head job.
}

type VolumeDefinition struct {
	Name      string
	Source    string // The raw <src>
	MountPath string // The <dest>
	Type      string // "gcsfuse", "hostPath", "pvc"
	ReadOnly  bool
}

type JobDefinition struct {
	ImageName       string
	BaseImage       string
	BuildContext    string
	Platform        string
	CommandToRun    string
	ComputeType     string
	MachineType     string
	DryRunManifest  string
	ProjectID       string
	ClusterName     string
	ClusterLocation string

	WorkloadName                  string
	KueueQueueName                string
	NumSlices                     int
	NodesPerSlice                 int
	MaxRestarts                   int
	TtlSecondsAfterFinished       int
	TerminationGracePeriodSeconds int

	PlacementPolicy    string
	NodeConstraint     map[string]string
	Affinity           map[string]string
	PodFailurePolicy   map[string]interface{}
	RestartOnExitCodes []int

	ImagePullSecrets      string
	ServiceAccountName    string
	Topology              string
	GKEScheduler          string
	AwaitJobCompletion    bool
	UseParallelContainers bool
	Timeout               string
	PriorityClassName     string

	// Pathways-specific fields
	IsPathwaysJob bool
	Pathways      PathwaysJobDefinition // Embedded struct for Pathways-specific args

	Volumes []VolumeDefinition

	Verbose bool
}

type JobStatus struct {
	Name           string
	Status         string
	CreationTime   string
	CompletionTime string
}

type ListOptions struct {
	ProjectID       string
	ClusterName     string
	ClusterLocation string
	// Filters
	Status       string
	NameContains string
}

type CancelOptions struct {
	ProjectID       string
	ClusterName     string
	ClusterLocation string
}

type LogsOptions struct {
	ProjectID       string
	ClusterName     string
	ClusterLocation string
	Follow          bool
}

type JobOrchestrator interface {
	SubmitJob(job JobDefinition) error
	ListJobs(opts ListOptions) ([]JobStatus, error)
	CancelJob(name string, opts CancelOptions) error
	GetJobLogs(name string, opts LogsOptions) (string, error)
}

type ClusterStatus struct {
	Name     string
	Location string
	Status   string
}

type VolumeStatus struct {
	Name    string
	Type    string
	Cluster string
}

type ClusterOrchestrator interface {
	ListEnvironments(opts ListOptions) ([]ClusterStatus, error)
	GetClusterInfo(name string, opts ListOptions) (string, error)
	DescribeEnvironment(name string, opts ListOptions) (string, error)
	ListVolumes(opts ListOptions) ([]VolumeStatus, error)
}
