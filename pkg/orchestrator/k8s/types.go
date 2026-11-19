// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// KubeconfigLoader loads the merged client-go kubeconfig; injectable for hermetic tests.
type KubeconfigLoader func() (*clientcmdapi.Config, error)

// TPUSliceAPIGroupVersion is the Kubernetes API group/version exposed by the tpu.google.com slice controller.
const TPUSliceAPIGroupVersion = "slice.tpu.google.com/v1alpha1"

// NodeGVR identifies core/v1 Node resources for dynamic Kubernetes queries.
var NodeGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "nodes",
}

// TPUSliceGVR identifies slice.tpu.google.com/v1alpha1 Slice resources.
var TPUSliceGVR = schema.GroupVersionResource{
	Group:    "slice.tpu.google.com",
	Version:  "v1alpha1",
	Resource: "slices",
}

// TPUPartitionTreeGVR identifies slice.tpu.google.com/v1alpha1 PartitionTree resources.
var TPUPartitionTreeGVR = schema.GroupVersionResource{
	Group:    "slice.tpu.google.com",
	Version:  "v1alpha1",
	Resource: "partitiontrees",
}

// SecretGVR identifies core/v1 Secret resources for dynamic Kubernetes queries.
var SecretGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "secrets",
}

// ServiceAccountGVR identifies core/v1 ServiceAccount resources for dynamic Kubernetes queries.
var ServiceAccountGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "serviceaccounts",
}

// ResourceLister lists unstructured Kubernetes resources for cluster and secret discovery.
type ResourceLister interface {
	ListResources(gvr schema.GroupVersionResource, namespace, labelSelector, fieldSelector string) ([]unstructured.Unstructured, error)
}

// ResourceReader lists and gets unstructured Kubernetes resources.
type ResourceReader interface {
	ResourceLister
	GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
}

// APIGroupChecker reports whether the Kubernetes API server serves a given API group or group/version
// (for example via the non-privileged GET /apis/<group>[/<version>] discovery endpoint).
type APIGroupChecker interface {
	HasAPIGroupVersion(groupVersion string) (bool, error)
}

// dockerConfigJSON is a .dockerconfigjson payload. authn.AuthConfig decodes "auth" with the kubelet's own rules
// (base64 of "user:pass"), so a payload the kubelet would reject fails to unmarshal here as well.
type dockerConfigJSON struct {
	Auths map[string]authn.AuthConfig `json:"auths"`
}

// LabelScheme identifies which Kubernetes node label convention a cluster uses for TPU/accelerator scheduling.
type LabelScheme string

const (
	// SchemeAuto selects automatic detection from node labels and API group discovery.
	SchemeAuto LabelScheme = ""
	// SchemeTPUSlice uses tpu.google.com/* TPU slice labels (slice.tpu.google.com/v1alpha1).
	SchemeTPUSlice LabelScheme = "tpu-slice"
	// SchemeGKE uses Google Kubernetes Engine labels (cloud.google.com/gke-*).
	SchemeGKE LabelScheme = "gke"
	// SchemeGeneric uses standard Kubernetes labels without TPU slice or GKE TPU conventions.
	SchemeGeneric LabelScheme = "generic"

	// LabelSchemeEnvVar overrides automatic label-scheme detection when --k8s-label-scheme is unset.
	LabelSchemeEnvVar = "GCLUSTER_LABEL_SCHEME"
)

// PoolCapacity summarizes per-node and aggregate allocatable resources for one node pool or machine-type group.
type PoolCapacity struct {
	MachineType   string
	NodePoolName  string
	TotalNodes    int
	ReadyNodes    int
	IsTPU         bool
	IsGPU         bool
	TPUType       string
	GPUType       string
	TPUsPerNode   int
	GPUsPerNode   int
	CPUsPerNode   int
	MemMiBPerNode int
	Topologies    []string
	SliceNames    []string
}

// SliceInfo describes a discovered slice.tpu.google.com/v1alpha1 Slice resource or node-derived slice.
type SliceInfo struct {
	Name          string
	Namespace     string
	TPUType       string
	MachineType   string
	Topology      string
	State         string
	Ready         bool
	NodesPerSlice int
	ChipsPerNode  int
	OwnerName     string
	ManagedBy     string
	PartitionTree string
	FromCRD       bool
}

// PartitionTreeInfo describes a discovered slice.tpu.google.com/v1alpha1 PartitionTree resource.
type PartitionTreeInfo struct {
	Name                string
	TPUType             string
	RootTopology        string
	SupportedTopologies []string
}

// TPUInventory summarizes the primary TPU hardware discovered in the cluster.
type TPUInventory struct {
	TPUType            string
	MachineType        string
	ChipsPerNode       int
	Topology           string
	NodesPerSlice      int
	DefaultSliceName   string
	TotalTPUNodes      int
	ReadyTPUNodes      int
	Topologies         []string
	HasPartitionLabels bool
}

// Inventory holds a pure-Kubernetes cluster discovery snapshot obtained without calling gcloud.
type Inventory struct {
	LabelScheme        LabelScheme
	ExplicitMode       bool
	TPUSliceAPIPresent bool
	TotalNodes         int
	ReadyNodes         int

	// Pools maps machine type (or disambiguated pool key) to its aggregated PoolCapacity.
	// PoolNames holds the deterministically sorted keys of Pools.
	Pools     map[string]PoolCapacity
	PoolNames []string

	// CPUPoolCap and TPUPoolCap are tracked separately so mixed CPU control-plane + TPU worker
	// clusters never let small CPU nodes override TPU worker pod sizing.
	CPUPoolCap *PoolCapacity
	TPUPoolCap *PoolCapacity

	TPU            *TPUInventory
	Slices         []SliceInfo
	PartitionTrees []PartitionTreeInfo
}

// DiscoverOptions configures pure-Kubernetes cluster inventory discovery.
// Platform label schemes are never inferred from --node-constraint or --compute-type.
type DiscoverOptions struct {
	// LabelSchemeOverride forces a specific LabelScheme when non-empty (from --k8s-label-scheme or GCLUSTER_LABEL_SCHEME).
	LabelSchemeOverride LabelScheme
}

type nodeSignals struct {
	hasTPUSliceLabels  bool
	hasGKELabels       bool
	hasPartitionLabels bool
	nodeSlices         map[string]*nodeSliceAgg
}

type nodeSliceAgg struct {
	name        string
	tpuType     string
	machineType string
	topology    string
	chips       int
	totalNodes  int
	readyNodes  int
}

type poolGroupKey struct {
	machineType  string
	class        string
	controlPlane bool
}

type poolBuilder struct {
	key        poolGroupKey
	cap        PoolCapacity
	topologies map[string]struct{}
	slices     map[string]struct{}
}

type nodeSnapshot struct {
	labels       map[string]string
	ready        bool
	cpus         int
	memMiB       int
	tpus         int
	gpus         int
	isTPU        bool
	isGPU        bool
	tpuType      string
	gpuType      string
	machineType  string
	nodePool     string
	topology     string
	sliceName    string
	hasTPUSlice  bool
	hasGKE       bool
	hasPartition bool
	controlPlane bool
}
