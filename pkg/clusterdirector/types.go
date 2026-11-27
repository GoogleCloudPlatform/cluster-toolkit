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

// Package clusterdirector imports resources deployed by the Cluster Toolkit
// into Cluster Director (the Hypercompute Cluster API), so that an existing
// Cluster Toolkit deployment shows up as a single cluster in Cluster Director.
//
// The payloads modelled here mirror the REST (JSON) representation of the
// google.cloud.hypercomputecluster Cluster resource. Only the subset of fields
// required to import pre-existing Cluster Toolkit resources is modelled.
package clusterdirector

import "encoding/json"

// Cluster is the Cluster Director representation of a cluster. Only the fields
// needed to import existing (Cluster Toolkit managed) resources are modelled.
type Cluster struct {
	// Name is the cluster identifier. On create it is a bare ID; on read the
	// server returns the fully qualified resource name.
	Name string `json:"name,omitempty"`
	// Description is a human-readable description of the cluster (up to 2,048
	// characters).
	Description string `json:"description,omitempty"`
	// Labels are user labels applied to the cluster resource itself.
	Labels map[string]string `json:"labels,omitempty"`
	// NetworkResources maps a user chosen key to a network imported into the
	// cluster. Keys must be RFC-1034 compliant, see ResourceKey.
	NetworkResources map[string]NetworkResource `json:"networkResources,omitempty"`
	// StorageResources maps a user chosen key to a filesystem imported into
	// the cluster. Keys must be RFC-1034 compliant, see ResourceKey.
	StorageResources map[string]StorageResource `json:"storageResources,omitempty"`
	// Orchestrator describes how the cluster's compute is scheduled. Cluster
	// Toolkit deployments are imported as a Compute Engine orchestrator.
	Orchestrator *Orchestrator `json:"orchestrator,omitempty"`
}

// NetworkResource is a VPC network attached to the cluster.
type NetworkResource struct {
	Config NetworkResourceConfig `json:"config"`
}

// NetworkResourceConfig describes how a network resource is initialized.
type NetworkResourceConfig struct {
	ExistingNetwork *ExistingNetworkConfig `json:"existingNetwork,omitempty"`
}

// ExistingNetworkConfig imports an already existing VPC network.
type ExistingNetworkConfig struct {
	// Network in the format `projects/{project}/global/networks/{network}`.
	Network string `json:"network"`
	// Subnetwork in the format
	// `projects/{project}/regions/{region}/subnetworks/{subnetwork}`.
	Subnetwork string `json:"subnetwork"`
}

// StorageResource is a filesystem attached to the cluster.
type StorageResource struct {
	Config StorageResourceConfig `json:"config"`
}

// StorageResourceConfig describes how a storage resource is initialized.
// Exactly one of the fields must be set.
type StorageResourceConfig struct {
	ExistingBucket    *ExistingBucketConfig    `json:"existingBucket,omitempty"`
	ExistingFilestore *ExistingFilestoreConfig `json:"existingFilestore,omitempty"`
	ExistingLustre    *ExistingLustreConfig    `json:"existingLustre,omitempty"`
}

// ExistingBucketConfig imports an already existing Cloud Storage bucket.
type ExistingBucketConfig struct {
	// Bucket is the bare bucket name, without the `gs://` scheme.
	Bucket string `json:"bucket"`
}

// ExistingFilestoreConfig imports an already existing Filestore instance.
type ExistingFilestoreConfig struct {
	// Filestore in the format
	// `projects/{project}/locations/{location}/instances/{instance}`.
	Filestore string `json:"filestore"`
}

// ExistingLustreConfig imports an already existing Managed Lustre instance.
type ExistingLustreConfig struct {
	// Lustre in the format
	// `projects/{project}/locations/{location}/instances/{instance}`.
	Lustre string `json:"lustre"`
}

// Orchestrator schedules and runs workloads on the cluster.
type Orchestrator struct {
	ComputeEngine *ComputeEngineOrchestrator `json:"computeEngine,omitempty"`
}

// ComputeEngineOrchestrator governs raw Compute Engine instances.
type ComputeEngineOrchestrator struct {
	// ExistingInstances maps a user chosen key to a set of instances that are
	// adopted by the orchestrator. Keys must be RFC-1034 compliant.
	ExistingInstances map[string]ExistingInstances `json:"existingInstances,omitempty"`
}

// ExistingInstances selects a group of pre-existing Compute Engine instances.
// At most one source (Project, InstanceGroupManager,
// RegionInstanceGroupManager, Reservation, ReservationBlock or
// ReservationSubBlock) may be set; Labels further filters the instances read
// from that source.
type ExistingInstances struct {
	// Project in the format `projects/{project}`.
	Project string `json:"project,omitempty"`
	// InstanceGroupManager in the format
	// `projects/{project}/zones/{zone}/instanceGroupManagers/{igm}`.
	InstanceGroupManager string `json:"instanceGroupManager,omitempty"`
	// RegionInstanceGroupManager in the format
	// `projects/{project}/regions/{region}/instanceGroupManagers/{igm}`.
	RegionInstanceGroupManager string `json:"regionInstanceGroupManager,omitempty"`
	// Reservation in the format
	// `projects/{project}/zones/{zone}/reservations/{reservation}`.
	Reservation string `json:"reservation,omitempty"`
	// ReservationBlock in the format
	// `projects/{project}/zones/{zone}/reservations/{reservation}/reservationBlocks/{reservation_block}`.
	ReservationBlock string `json:"reservationBlock,omitempty"`
	// ReservationSubBlock in the format
	// `projects/{project}/zones/{zone}/reservations/{reservation}/reservationBlocks/{reservation_block}/reservationSubBlocks/{reservation_sub_block}`.
	ReservationSubBlock string `json:"reservationSubBlock,omitempty"`
	// Labels that an instance must carry to be included.
	Labels map[string]string `json:"labels,omitempty"`
}

// Node is a compute node of a Cluster Director cluster, as returned by
// nodes.list. Only the name is used.
type Node struct {
	// Name in the format
	// `projects/{project}/locations/{location}/clusters/{cluster}/nodes/{node}`.
	Name string `json:"name,omitempty"`
}

// ListNodesResponse is the response payload from the Cluster Director
// nodes.list endpoint.
type ListNodesResponse struct {
	Nodes         []Node `json:"nodes"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// Operation is a google.longrunning.Operation as returned by the Cluster
// Director API for mutating calls.
type Operation struct {
	Name     string          `json:"name,omitempty"`
	Done     bool            `json:"done,omitempty"`
	Error    *Status         `json:"error,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
}

// Status is a google.rpc.Status returned in error responses.
type Status struct {
	Code    int               `json:"code,omitempty"`
	Message string            `json:"message,omitempty"`
	Status  string            `json:"status,omitempty"`
	Details []json.RawMessage `json:"details,omitempty"`
}
