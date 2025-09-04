# Copyright 2025 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "project_id" {
  description = "The project ID to host the cluster in."
  type        = string
}

variable "cluster_id" {
  description = "projects/{{project}}/locations/{{location}}/clusters/{{cluster}}"
  type        = string
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used in slurm controller"

  validation {
    condition     = var.slurm_cluster_name != null && can(regex("^[a-z](?:[a-z0-9]{0,9})$", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be a match of regex '^[a-z](?:[a-z0-9]{0,9})$'."
  }
}

variable "slurm_controller_instance" {
  type        = any
  description = "Slurm cluster controller instance"
}

variable "image" {
  description = "The image for slurm daemon"
  type        = string
  default     = null

  validation {
    condition     = var.image != null
    error_message = "Variable 'image' must be provided."
  }
}

variable "node_pool_names" {
  description = "If set to true. The node group VMs will have a random public IP assigned to it. Ignored if access_config is set."
  type        = list(string)
  nullable    = false
}

variable "node_count" {
  description = "The number of static nodes in node-pool"
  type        = number
}

variable "subnetwork" {
  description = "Primary subnetwork object"
  type        = any
}

variable "has_gpu" {
  description = "If set to true, the nodeset template's Pod spec will contain request/limit for gpu resource."
  type        = bool
  default     = false
}

variable "has_tpu" {
  description = "If set to true, the nodeset template's Pod spec will contain request/limit for TPU resource, open port 8740 for TPU communication and add toleration for google.com/tpu."
  type        = bool
  default     = false
}

variable "tpu_chips_per_node" {
  description = "Number of TPU chips per node. Required when has_tpu=true"
  type        = number
  default     = 0
}

variable "tpu_accelerator" {
  description = "Name of the TPU accelerator (cloud.google.com/gke-tpu-accelerator annotation). Required when has_tpu=true"
  type        = string
  default     = null
}

variable "tpu_topology" {
  description = "TPU topology. Required when has_tpu=true"
  type        = string
  default     = null
}

variable "allocatable_gpu_per_node" {
  description = "Number of GPUs available for scheduling pods on each node."
  type        = number
  default     = 0
}

variable "slurm_namespace" {
  description = "slurm namespace for charts"
  type        = string
  default     = "slurm"
}

variable "nodeset_name" {
  description = "The nodeset name"
  type        = string
  default     = "gkenodeset"
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the nodes."
  type        = list(any)
  default     = []
  nullable    = false
}

variable "machine_type" {
  description = "The name of a Google Compute Engine machine type."
  type        = string
  default     = "c2-standard-60"
}

variable "pvc_name" {
  description = "An object that describes a k8s PVC created by this module."
  type        = string
}

variable "slurm_bucket_name" {
  description = "GCS Bucket name of Slurm cluster file storage."
  type        = string
  nullable    = false
}

variable "slurm_bucket_dir" {
  description = "Path directory within `bucket_name` for Slurm cluster file storage."
  type        = string
  nullable    = false
}

variable "slurm_bucket" {
  description = "GCS Bucket of Slurm cluster file storage."
  type        = any
  nullable    = true
}

variable "instance_templates" {
  description = "The URLs of Instance Templates"
  type        = list(string)
  nullable    = false
}
