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
  default     = 50
}

variable "zone" {
  description = "Compute Platform zone where the notebook server will be located"
  type        = string
}

variable "instance_image" {
  description = <<-EOD
    Defines the image that will be used by the compute VMs in this partition.
    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    Custom images must comply with Slurm on GCP requirements.
    EOD
  type        = map(string)
  default = {
    project = "schedmd-slurm-public"
    family  = "schedmd-slurm-21-08-8-hpc-centos-7"
  }

  validation {
    condition = length(var.instance_image) == 0 || (
    can(var.instance_image["family"]) || can(var.instance_image["name"])) == can(var.instance_image["project"])
    error_message = "The \"project\" is required if \"family\" or \"name\" are provided in var.instance_image."
  }
  validation {
    condition     = length(var.instance_image) == 0 || can(var.instance_image["family"]) != can(var.instance_image["name"])
    error_message = "Exactly one of \"family\" and \"name\" must be provided in var.instance_image."
  }
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
  description = "Labels to add to partition compute instances. Key-value pairs."
  type        = map(string)
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
    server_ip             = string,
    remote_mount          = string,
    local_mount           = string,
    fs_type               = string,
    mount_options         = string,
    client_install_runner = map(string)
    mount_runner          = map(string)
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
  default     = true
}

variable "enable_placement" {
  description = "Enable compact placement policies for jobs requiring low latency networking."
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

variable "bandwidth_tier" {
  description = <<EOT
  Configures the network interface card and the maximum egress bandwidth for VMs.
  - Setting `platform_default` respects the Google Cloud Platform API default values for networking.
  - Setting `virtio_enabled` explicitly selects the VirtioNet network adapter.
  - Setting `gvnic_enabled` selects the gVNIC network adapter (without Tier 1 high bandwidth).
  - Setting `tier_1_enabled` selects both the gVNIC adapter and Tier 1 high bandwidth networking.
  - Note: both gVNIC and Tier 1 networking require a VM image with gVNIC support as well as specific VM families and shapes.
  - See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details.
  EOT
  type        = string
  default     = "platform_default"

  validation {
    condition     = contains(["platform_default", "virtio_enabled", "gvnic_enabled", "tier_1_enabled"], var.bandwidth_tier)
    error_message = "Allowed values for bandwidth_tier are 'platform_default', 'virtio_enabled', 'gvnic_enabled', or 'tier_1_enabled'."
  }
}

variable "instance_template" {
  description = "Instance template to use to create partition instances"
  type        = string
  default     = null
}
