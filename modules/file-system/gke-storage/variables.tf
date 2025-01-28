/**
 * Copyright 2024 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
*/

variable "project_id" {
  description = "The project ID to host the cluster in."
  type        = string
}

variable "cluster_id" {
  description = "An identifier for the GKE cluster in the format `projects/{{project}}/locations/{{location}}/clusters/{{cluster}}`"
  type        = string
}

variable "labels" {
  description = "GCE resource labels to be applied to resources. Key-value pairs."
  type        = map(string)
}

variable "storage_type" {
  description = <<-EOT
  The type of [GKE supported storage options](https://cloud.google.com/kubernetes-engine/docs/concepts/storage-overview)
  to used. This module currently support dynamic provisioning for the below storage options
  - Parallelstore
  - Hyperdisk-balanced
  - Hyperdisk-throughput
  - Hyperdisk-extreme
  EOT 
  type        = string
  nullable    = false
  validation {
    condition     = var.storage_type == null ? false : contains(["parallelstore", "hyperdisk-balanced", "hyperdisk-throughput", "hyperdisk-extreme"], lower(var.storage_type))
    error_message = "Allowed string values for var.storage_type are \"Parallelstore\", \"Hyperdisk-balanced\", \"Hyperdisk-throughput\", \"Hyperdisk-extreme\"."
  }
}

variable "access_mode" {
  description = <<-EOT
  The access mode that the volume can be mounted to the host/pod. More details in [Access Modes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes)
  Valid access modes:
  - ReadWriteOnce
  - ReadOnlyMany
  - ReadWriteMany
  - ReadWriteOncePod
  EOT 
  type        = string
  nullable    = false
  validation {
    condition     = var.access_mode == null ? false : contains(["readwriteonce", "readonlymany", "readwritemany", "readwriteoncepod"], lower(var.access_mode))
    error_message = "Allowed string values for var.access_mode are \"ReadWriteOnce\", \"ReadOnlyMany\", \"ReadWriteMany\", \"ReadWriteOncePod\"."
  }
}

variable "sc_volume_binding_mode" {
  description = <<-EOT
  Indicates when volume binding and dynamic provisioning should occur and how PersistentVolumeClaims should be provisioned and bound.
  Supported value:
  - Immediate
  - WaitForFirstConsumer
  EOT
  type        = string
  default     = "WaitForFirstConsumer"
  validation {
    condition     = var.sc_volume_binding_mode == null ? true : contains(["immediate", "waitforfirstconsumer"], lower(var.sc_volume_binding_mode))
    error_message = "Allowed string values for var.sc_volume_binding_mode are \"Immediate\", \"WaitForFirstConsumer\"."
  }
}

variable "sc_reclaim_policy" {
  description = <<-EOT
  Indicate whether to keep the dynamically provisioned PersistentVolumes of this storage class after the bound PersistentVolumeClaim is deleted.
  [More details about reclaiming](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#reclaiming)
  Supported value:
  - Retain
  - Delete
  EOT
  type        = string
  nullable    = false
  validation {
    condition     = var.sc_reclaim_policy == null ? true : contains(["retain", "delete"], lower(var.sc_reclaim_policy))
    error_message = "Allowed string values for var.sc_reclaim_policy are \"Retain\", \"Delete\"."
  }
}

variable "sc_topology_zones" {
  description = "Zone location that allow the volumes to be dynamically provisioned."
  type        = list(string)
  default     = null
}

variable "pvc_count" {
  description = "How many PersistentVolumeClaims that will be created"
  type        = number
  default     = 1
}

variable "pv_mount_path" {
  description = "Path within the container at which the volume should be mounted. Must not contain ':'."
  type        = string
  default     = "/data"
  validation {
    condition     = var.pv_mount_path == null ? true : !strcontains(var.pv_mount_path, ":")
    error_message = "pv_mount_path must not contain ':', please correct it and retry"
  }
}

variable "mount_options" {
  description = "Controls the mountOptions for dynamically provisioned PersistentVolumes of this storage class."
  type        = string
  default     = null
}

variable "capacity_gb" {
  description = "The storage capacity with which to create the persistent volume."
  type        = number
}

variable "private_vpc_connection_peering" {
  description = <<-EOT
    The name of the VPC Network peering connection.
    If using new VPC, please use community/modules/network/private-service-access to create private-service-access and
    If using existing VPC with private-service-access enabled, set this manually follow [user guide](https://cloud.google.com/parallelstore/docs/vpc).
    EOT
  type        = string
  default     = null
}
