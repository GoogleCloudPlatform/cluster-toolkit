#
# Copyright 2019 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming and slurm accounting."

  validation {
    condition     = can(regex("(^[a-z][a-z0-9]*$)", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be a match of regex '(^[a-z][a-z0-9]*$)'."
  }
}

variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "region" {
  description = "The default region for Cloud resources"
  type        = string
}

variable "partition_name" {
  description = "The name of the slurm partition"
  type        = string
}

variable "machine_type" {
  description = "Compute Platform machine type to use for this partition compute nodes"
  type        = string
  default     = "c2-standard-60"
}

variable "node_count_static" {
  description = "Number of nodes to be statically created"
  type        = number
  default     = 0
}

variable "node_count_dynamic_max" {
  description = "Maximum number of nodes allowed in this partition"
  type        = number
  default     = 10
}

variable "source_image" {
  description = "Image to be used of the compute VMs in this partition"
  type        = string
  default     = null
}

variable "source_image_project" {
  description = "Project the image is hosted in"
  type        = string
  default     = null
}

variable "disk_type" {
  description = "Type of boot disk to create for the partition compute nodes"
  type        = string
  default     = "pd-standard"
}

variable "disk_size_gb" {
  description = "Size of boot disk to create for the partition compute nodes"
  type        = number
  default     = 30
}

variable "labels" {
  description = "Labels to add to partition compute instances. List of key key, value pairs."
  type        = any
  default     = {}
}

variable "min_cpu_platform" {
  description = "The name of the minimum CPU platform that you want the instance to use."
  type        = string
  default     = null
}

variable "gpu" {
  description = "Definition of requested GPU resources"
  type = object({
    count = number,
    type  = string
  })
  default = null
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured on the partition compute nodes."
  type = list(object({
    server_ip     = string,
    remote_mount  = string,
    local_mount   = string,
    fs_type       = string,
    mount_options = string
  }))
  default = []
}

variable "preemptible" {
  description = "Should use preemptibles to burst"
  type        = string
  default     = false
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to. Only one of network or subnetwork should be specified."
  default     = ""
}

variable "exclusive" {
  description = "Exclusive job access to nodes"
  type        = bool
  default     = false
}

variable "enable_placement" {
  description = "Enable placement groups"
  type        = bool
  default     = true
}

variable "enable_spot_vm" {
  description = "Enable the partition to use spot VMs (https://cloud.google.com/spot-vms)"
  type        = bool
  default     = false
}

variable "spot_instance_config" {
  description = "Configuration for spot VMs."
  type = object({
    termination_action = string
  })
  default = null
}
