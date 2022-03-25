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


variable "project_id" {
  description = "Project in which the HPC deployment will be created"
  type        = string
}

variable "deployment_name" {
  description = "Name of the deployment, used to name the cluster"
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

variable "static_node_count" {
  description = "Number of nodes to be statically created"
  type        = number
  default     = 0
}

variable "max_node_count" {
  description = "Maximum number of nodes allowed in this partition"
  type        = number
  default     = 10
}

variable "zone" {
  description = "Compute Platform zone where the notebook server will be located"
  type        = string
}

variable "image" {
  description = "Image to be used of the compute VMs in this partition"
  type        = string
  default     = "projects/schedmd-slurm-public/global/images/family/schedmd-slurm-21-08-4-hpc-centos-7"
}

variable "image_hyperthreads" {
  description = "Enable hyperthreading"
  type        = bool
  default     = false
}

variable "compute_disk_type" {
  description = "Type of boot disk to create for the partition compute nodes"
  type        = string
  default     = "pd-standard"
}

variable "compute_disk_size_gb" {
  description = "Size of boot disk to create for the partition compute nodes"
  type        = number
  default     = 20
}

variable "labels" {
  description = "Labels to add to partition compute instances. List of key key, value pairs."
  type        = any
  default     = {}
}

variable "cpu_platform" {
  description = "The name of the minimum CPU platform that you want the instance to use."
  type        = string
  default     = null
}

variable "gpu_count" {
  description = "Number of GPUs attached to the partition compute instances"
  type        = number
  default     = 0
}

variable "gpu_type" {
  description = "Type of GPUs attached to the partition compute instances"
  type        = string
  default     = null
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

variable "preemptible_bursting" {
  description = "Should use preemptibles to burst"
  type        = string
  default     = false
}

variable "subnetwork_name" {
  description = "The name of the pre-defined VPC subnet you want the nodes to attach to based on Region."
  type        = string
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

variable "regional_capacity" {
  description = "If True, then create instances in the region that has available capacity. Specify the region in the zone field."
  type        = bool
  default     = false
}

variable "regional_policy" {
  description = "locationPolicy defintion for regional bulkInsert()"
  type        = any
  default     = {}
}

variable "instance_template" {
  description = "Instance template to use to create partition instances"
  type        = string
  default     = null
}
