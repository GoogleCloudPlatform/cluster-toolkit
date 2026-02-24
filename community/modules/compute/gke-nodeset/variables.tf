# Copyright 2026 Google LLC
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
    condition     = var.slurm_cluster_name != null && can(regex("^[a-z]([-a-z0-9]{0,19})$", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be a match of regex '^[a-z]([-a-z0-9]{0,19})$'."
  }
}

variable "slurm_controller_instance" {
  type        = any
  description = "Slurm cluster controller instance"
}

variable "image" {
  description = "The image for slurm daemon"
  type        = string
  nullable    = false
}

variable "node_pool_names" {
  description = "If set to true. The node group VMs will have a random public IP assigned to it. Ignored if access_config is set."
  type        = list(string)
  nullable    = false
}

variable "node_count_static" {
  description = "The number of static nodes in node-pool"
  type        = number
}

variable "subnetwork" {
  description = "Primary subnetwork object"
  type        = any
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

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on nodes."
  type = list(object({
    server_ip             = string,
    remote_mount          = string,
    local_mount           = string,
    fs_type               = string,
    mount_options         = string,
    client_install_runner = map(string)
    mount_runner          = map(string)
  }))

  validation {
    condition     = length(var.network_storage) == 1 && var.network_storage[0].local_mount == "/home"
    error_message = "The 'network_storage' variable must contain exactly one element, and that element's 'local_mount' attribute must be \"/home\"."
  }
}

variable "filestore_id" {
  description = "An array of identifier for a filestore with the format `projects/{{project}}/locations/{{location}}/instances/{{name}}`."
  type        = list(string)

  validation {
    condition     = length(var.filestore_id) == 1
    error_message = "The 'filestore_id' variable must contain exactly one element."
  }
}
