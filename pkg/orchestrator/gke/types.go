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
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"strings"

	compute "google.golang.org/api/compute/v1"
	"k8s.io/client-go/dynamic"
)

type Executor interface {
	ExecuteCommand(name string, args ...string) shell.CommandResult
	ExecuteCommandStream(name string, args ...string) error
}

// KubeClient defines the interface for specific Kubernetes API operations needed by the orchestrator.
type KubeClient interface {
	GetJobNamespace(workloadName string) (string, error)
	ListWorkloads(namespace string, workloadName string) ([]string, error)
	DeleteJobSet(namespace string, name string) error
	ListJobSets(labelSelector string) ([]orchestrator.JobStatus, error)
}

type MachineTypeClient interface {
	GetMachineType(project, zone, machineType string) (*compute.MachineType, error)
}

type DefaultMachineTypeClient struct{}

func (c DefaultMachineTypeClient) GetMachineType(project, zone, machineType string) (*compute.MachineType, error) {
	return config.GetMachineType(project, zone, machineType)
}

// DefaultKubeClient implements KubeClient using the actual dynamic client.
type DefaultKubeClient struct {
	dynClient dynamic.Interface
}

type DefaultExecutor struct{}

type GKEOrchestrator struct {
	executor                    Executor
	projectID                   string
	clusterZones                []string
	nodePoolSAs                 []string
	capacity                    ClusterCapacity
	clusterDesc                 gkeCluster
	dynClient                   dynamic.Interface
	kubeClient                  KubeClient
	machineTypeClient           MachineTypeClient
	acceleratorToMachineType    map[string]string
	machineCapCache             map[string]MachineTypeCap
	resolvedHeadNodePool        string
	machineTypeToThreadsPerCore map[string]string
}

// Types for GetClusterInfo unmarshaling
type gkeNodePool struct {
	Name   string `json:"name"`
	Config struct {
		MachineType string `json:"machineType"`
	} `json:"config"`
	Count  int    `json:"count"`
	Status string `json:"status"`
}

type gkeClusterDescribe struct {
	Name      string        `json:"name"`
	Location  string        `json:"location"`
	NodePools []gkeNodePool `json:"nodePools"`
}

func (c gkeClusterDescribe) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Cluster Resource Summary for: %s\n", c.Name))
	sb.WriteString(fmt.Sprintf("Location: %s\n", c.Location))
	sb.WriteString("------------------------------------------\n")
	for _, np := range c.NodePools {
		sb.WriteString(fmt.Sprintf("NodePool: %s\n", np.Name))
		sb.WriteString(fmt.Sprintf("  MachineType: %s\n", np.Config.MachineType))
		if np.Count > 0 {
			sb.WriteString(fmt.Sprintf("  Count: %d\n", np.Count))
		}
		sb.WriteString(fmt.Sprintf("  Status: %s\n", np.Status))
		sb.WriteString("\n")
	}
	return sb.String()
}

// Types for ListVolumes unmarshaling
type gkePVC struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		StorageClassName *string `json:"storageClassName"`
	} `json:"spec"`
}

type gkePVCList struct {
	Items []gkePVC `json:"items"`
}

type JobProfile struct {
	IsCPUMachine  bool
	CapacityCount int
}

type ManifestOptions struct {
	WorkloadName                  string
	FullImageName                 string
	CommandToRun                  string
	AcceleratorType               string
	ResourcesString               string
	ProjectID                     string
	ClusterName                   string
	ClusterLocation               string
	KueueQueueName                string
	NumSlices                     int
	VmsPerSlice                   int
	MaxRestarts                   int
	TtlSecondsAfterFinished       int
	TerminationGracePeriodSeconds int
	NodeSelector                  string
	Affinity                      string
	PodFailurePolicy              string
	ImagePullSecrets              string
	ServiceAccountName            string
	TopologyAnnotation            string
	Topology                      string
	PathwaysInstanceType          string
	SchedulerName                 string
	SchedulingGates               string
	Tolerations                   string
	AwaitJobCompletion            bool
	PriorityClassName             string
	VolumesYAML                   string
	VolumeMountsYAML              string
	GCSFuseEnabled                bool
	IsDynamicSlicing              bool
	IsCPUMachine                  bool
	Pathways                      orchestrator.PathwaysJobDefinition
	Verbose                       bool
}

type FlavorCapacity struct {
	CPUs       int
	MemoryGi   int
	GPUs       int
	TPUs       int
	NodeLabels map[string]string
}

type ClusterCapacity struct {
	CPUs     int
	MemoryGi int
	GPUs     int
	TPUs     int
	Flavors  map[string]FlavorCapacity
}

// Types for initializeJobSubmission unmarshaling

type gkeAccelerator struct {
	AcceleratorCount json.Number `json:"acceleratorCount"`
	AcceleratorType  string      `json:"acceleratorType"`
}

type gkeAdvancedMachineFeatures struct {
	ThreadsPerCore string `json:"threadsPerCore"`
}

type gkeTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

type gkeNodePoolConfig struct {
	ServiceAccount          string                      `json:"serviceAccount"`
	MachineType             string                      `json:"machineType"`
	Accelerators            []gkeAccelerator            `json:"accelerators"`
	AdvancedMachineFeatures *gkeAdvancedMachineFeatures `json:"advancedMachineFeatures,omitempty"`
	Taints                  []gkeTaint                  `json:"taints"`
}

type gkeAutoscaling struct {
	Enabled      bool `json:"enabled"`
	MinNodeCount int  `json:"minNodeCount"`
	MaxNodeCount int  `json:"maxNodeCount"`
}

type gkePlacementPolicy struct {
	AcceleratorTopologyMode string `json:"acceleratorTopologyMode"`
	Type                    string `json:"type"`
}

type gkeJobNodePool struct {
	Name             string              `json:"name"`
	Config           gkeNodePoolConfig   `json:"config"`
	InitialNodeCount int                 `json:"initialNodeCount"`
	Autoscaling      gkeAutoscaling      `json:"autoscaling"`
	PlacementPolicy  *gkePlacementPolicy `json:"placementPolicy,omitempty"`
}

type gkeCluster struct {
	Locations []string         `json:"locations"`
	NodePools []gkeJobNodePool `json:"nodePools"`
}

// Types for JobSet status unmarshaling

type JobSetCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

type JobSetStatus struct {
	Status struct {
		Conditions []JobSetCondition `json:"conditions"`
	} `json:"status"`
}

type jobSetTemplateData struct {
	WorkloadName                  string
	ClusterName                   string
	ProjectID                     string
	KueueQueueName                string
	TtlSecondsAfterFinished       int
	TerminationGracePeriodSeconds int
	MaxRestarts                   int
	NumSlices                     int
	VmsPerSlice                   int
	WorkerBackoffLimit            int
	PathwaysInstanceType          string
	CommandToRun                  string
	ResourcesString               string
	ProxyArgsList                 []string
	ServerArgsList                []string
	WorkerArgsList                []string
	FullImageName                 string
	Command                       []string
	ResourcesYAML                 string
	AcceleratorTypeLabel          string
	NodeSelector                  string
	Affinity                      string
	PodFailurePolicy              string
	ImagePullSecrets              string
	ServiceAccountName            string
	TopologyAnnotation            string
	SchedulerName                 string
	SchedulingGates               string
	Tolerations                   string
	PriorityClassName             string
	VolumesYAML                   string
	VolumeMountsYAML              string
	GCSFuseEnabled                bool
	HostNetworkEnabled            bool
	Pathways                      orchestrator.PathwaysJobDefinition
	ExclusiveTopologyAnnotation   string
	Verbose                       bool
	IsTPU                         bool
	IsGPU                         bool
}
