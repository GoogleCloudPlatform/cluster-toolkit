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
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// KubeconfigLoader loads the merged client-go kubeconfig; injectable for hermetic tests.
type KubeconfigLoader func() (*clientcmdapi.Config, error)

// TPUSliceAPIGroupVersion is the group-level Kubernetes API path exposed by the tpu.google.com slice controller
// (covers both v1alpha1 and v1beta1).
const TPUSliceAPIGroupVersion = "slice.tpu.google.com"

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

// TPUSliceV1Beta1GVR identifies slice.tpu.google.com/v1beta1 Slice resources.
var TPUSliceV1Beta1GVR = schema.GroupVersionResource{
	Group:    "slice.tpu.google.com",
	Version:  "v1beta1",
	Resource: "slices",
}

// TPUPartitionTreeGVR identifies partition.tpu.google.com/v1alpha1 PartitionTree resources.
var TPUPartitionTreeGVR = schema.GroupVersionResource{
	Group:    "partition.tpu.google.com",
	Version:  "v1alpha1",
	Resource: "partitiontrees",
}

// TPUPartitionTreeV1Beta1GVR identifies partition.tpu.google.com/v1beta1 PartitionTree resources.
var TPUPartitionTreeV1Beta1GVR = schema.GroupVersionResource{
	Group:    "partition.tpu.google.com",
	Version:  "v1beta1",
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

// TPUHardwareResolver supplies optional provider-specific TPU machine-type normalization and chip-count
// lookup so pkg/orchestrator/k8s remains decoupled from GCE machine-type tables.
type TPUHardwareResolver interface {
	IsTPUMachine(machineType string) bool
	NormalizeTPUType(raw string) string
	InferTPUMachine(tpuType, rawMachine string, chipsPerNode int) string
	ChipsPerNode(machineType string) int
}

type noopHardwareResolver struct{}

// dockerConfigJSON is a .dockerconfigjson payload. authn.AuthConfig decodes "auth" with the kubelet's own rules
// (base64 of "user:pass"), so a payload the kubelet would reject fails to unmarshal here as well.
type dockerConfigJSON struct {
	Auths map[string]authn.AuthConfig `json:"auths"`
}

// ImageRef holds the parsed components of an OCI image reference ([registry/]repository[:tag][@digest]).
type ImageRef struct {
	RawName   string // Reference with tag and digest stripped, preserving original registry/repo prefix.
	Host      string // Registry host preserving case ("docker.io" for Docker Hub shorthand or exact "index.docker.io").
	Repo      string // Repository path (bare Docker Hub names become "library/<name>").
	Tag       string // Explicit tag without leading ":", or empty if absent.
	HasTag    bool   // True when a ":" tag separator appeared in the final path segment (Tag is "" for a trailing ":").
	HasDigest bool   // True when an "@" digest separator was present.
}

// LabelScheme identifies which Kubernetes node label convention a cluster uses for TPU/accelerator scheduling.
type LabelScheme string

const (
	// SchemeAuto selects automatic detection from node labels and API group discovery.
	SchemeAuto LabelScheme = ""
	// SchemeTPUSlice uses tpu.google.com/* TPU slice labels (slice.tpu.google.com).
	SchemeTPUSlice LabelScheme = "tpu-slice"
	// SchemeGKE uses Google Kubernetes Engine labels (cloud.google.com/gke-*).
	SchemeGKE LabelScheme = "gke"
	// SchemeGeneric uses standard Kubernetes labels without TPU slice or GKE TPU conventions.
	SchemeGeneric LabelScheme = "generic"

	// LabelSchemeEnvVar overrides automatic label-scheme detection.
	LabelSchemeEnvVar = "GCLUSTER_LABEL_SCHEME"
)

// PoolCapacity summarizes per-node and aggregate allocatable resources for one node pool or machine-type group.
type PoolCapacity struct {
	MachineType            string
	NodePoolName           string
	TotalNodes             int
	ReadyNodes             int
	IsControlPlane         bool
	IsSystemPool           bool
	IsTPU                  bool
	IsGPU                  bool
	TPUType                string
	GPUType                string
	GPUResourceName        string
	InstanceTypeLabel      string
	GPUProductLabel        string
	GKEAcceleratorLabel    string
	GKETPUAcceleratorLabel string
	TPUsPerNode            int
	GPUsPerNode            int
	AllocatableCPUMilli    int64
	MemMiBPerNode          int
	Topologies             []string
	SliceNames             []string
}

// SliceInfo describes a discovered slice.tpu.google.com Slice resource or node-derived slice.
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
	PartitionIDs  []string
}

// PartitionTreeInfo describes a discovered partition.tpu.google.com PartitionTree resource.
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
	LabelScheme               LabelScheme
	ExplicitMode              bool
	TPUSliceAPIPresent        bool
	SliceInventoryUnavailable bool
	TotalNodes                int
	ReadyNodes                int

	// Pools maps a collision-safe deterministic pool key to its aggregated PoolCapacity.
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
	// LabelSchemeOverride forces a specific LabelScheme when non-empty (from GCLUSTER_LABEL_SCHEME).
	LabelSchemeOverride LabelScheme
	// HardwareResolver provides optional provider-specific TPU machine normalization and chip-count math.
	HardwareResolver TPUHardwareResolver
}

type nodeSignals struct {
	hasTPUSliceLabels  bool
	hasGKELabels       bool
	hasPartitionLabels bool
	poolControlPlane   map[string]bool
	nodeSlices         map[string]*nodeSliceAgg
}

type nodeSliceAgg struct {
	name             string
	tpuType          string
	readyTPUType     string
	machineType      string
	readyMachineType string
	topology         string
	readyTopology    string
	chips            int
	readyChips       int
	totalNodes       int
	readyNodes       int
}

type poolGroupKey struct {
	poolKey         string
	nodePool        string
	class           string
	accelType       string
	gpuResourceName string
	controlPlane    bool
	systemPool      bool
}

type observedLabel struct {
	seen     bool
	conflict bool
	value    string
}

func (o *observedLabel) observe(val string) {
	val = strings.TrimSpace(val)
	if !o.seen {
		o.seen = true
		o.value = val
		if val == "" {
			o.conflict = true
		}
		return
	}
	if o.value != val {
		o.conflict = true
		o.value = ""
	}
}

func (o observedLabel) resolved() string {
	if !o.seen || o.conflict {
		return ""
	}
	return o.value
}

type poolBuilder struct {
	key                   poolGroupKey
	cap                   PoolCapacity
	allMachineType        string
	readyMachineType      string
	allTPUType            string
	readyTPUType          string
	allGPUType            string
	readyGPUType          string
	allInstanceLabel      observedLabel
	readyInstanceLabel    observedLabel
	allGPUProductLabel    observedLabel
	readyGPUProductLabel  observedLabel
	allGKEAccelLabel      observedLabel
	readyGKEAccelLabel    observedLabel
	allGKETPUAccelLabel   observedLabel
	readyGKETPUAccelLabel observedLabel
	allCPUMilli           int64
	readyCPUMilli         int64
	allMemMiB             int
	readyMemMiB           int
	allTPUs               int
	readyTPUs             int
	allGPUs               int
	readyGPUs             int
	topologies            map[string]struct{}
	slices                map[string]struct{}
}

type nodeSnapshot struct {
	ready             bool
	cpuMilli          int64
	memMiB            int
	tpus              int
	gpus              int
	isTPU             bool
	isGPU             bool
	tpuType           string
	gpuType           string
	gpuResourceName   string
	instanceTypeLabel string
	gpuProductLabel   string
	gkeAccelLabel     string
	gkeTPUAccelLabel  string
	machineType       string
	poolKey           string
	nodePool          string
	topology          string
	sliceName         string
	hasTPUSlice       bool
	hasGKE            bool
	hasPartition      bool
	controlPlane      bool
	systemPool        bool
}

type crdDiscoveryResult struct {
	sliceErr   error
	sliceFound bool
	treeErr    error
	slices     []SliceInfo
	trees      []PartitionTreeInfo
}

type gpuResourceRule struct {
	key          string
	isPrefix     bool
	inferredType string
}
