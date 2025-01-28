# Copyright 2024 Google LLC
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

###########
# GENERAL #
###########

variable "project_id" {
  type        = string
  description = "Project ID to create resources in."
}

variable "on_host_maintenance" {
  type        = string
  description = "Instance availability Policy"
  default     = "MIGRATE"
}

variable "labels" {
  type        = map(string)
  description = "Labels, provided as a map"
  default     = {}
}

variable "enable_oslogin" {
  type        = bool
  description = <<EOD
Enables Google Cloud os-login for user login and authentication for VMs.
See https://cloud.google.com/compute/docs/oslogin
EOD
  default     = true
}

###########
# NETWORK #
###########

variable "subnetwork_project" {
  type        = string
  description = "The ID of the project in which the subnetwork belongs. If it is not provided, the provider project is used."
  default     = null
}

variable "network" {
  type        = string
  description = <<EOD
The name or self_link of the network to attach this interface to. Use network
attribute for Legacy or Auto subnetted networks and subnetwork for custom
subnetted networks.
EOD
  default     = null
}

variable "subnetwork" {
  type        = string
  description = <<EOD
The name of the subnetwork to attach this interface to. The subnetwork must
exist in the same region this instance will be created in. Either network or
subnetwork must be provided.
EOD
  default     = null
}

variable "region" {
  type        = string
  description = "Region where the instance template should be created."
  default     = null
}

variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
}

variable "can_ip_forward" {
  type        = bool
  description = "Enable IP forwarding, for NAT instances for example."
  default     = false
}

variable "network_ip" {
  type        = string
  description = "Private IP address to assign to the instance if desired."
  default     = ""
}

variable "name_prefix" {
  type        = string
  description = "Prefix for template resource."
  default     = "default"
}

variable "bandwidth_tier" {
  description = <<-EOD
    Tier 1 bandwidth increases the maximum egress bandwidth for VMs.
    Using the `virtio_enabled` setting will only enable VirtioNet and will not enable TIER_1.
    Using the `tier_1_enabled` setting will enable both gVNIC and TIER_1 higher bandwidth networking.
    Using the `gvnic_enabled` setting will only enable gVNIC and will not enable TIER_1.
    Note that TIER_1 only works with specific machine families & shapes and must be using an image that supports gVNIC. See [official docs](https://cloud.google.com/compute/docs/networking/configure-vm-with-high-bandwidth-configuration) for more details.
  EOD
  type        = string
  default     = "platform_default"

  validation {
    condition     = contains(["platform_default", "virtio_enabled", "gvnic_enabled", "tier_1_enabled"], var.bandwidth_tier)
    error_message = "Allowed values for bandwidth_tier are 'platform_default', 'virtio_enabled', 'gvnic_enabled', or 'tier_1_enabled'."
  }
}

variable "additional_networks" {
  description = "Additional network interface details for GCE, if any."
  default     = []
  type = list(object({
    network            = string
    subnetwork         = string
    subnetwork_project = string
    network_ip         = string
    nic_type           = string
    access_config = list(object({
      nat_ip       = string
      network_tier = string
    }))
    ipv6_access_config = list(object({
      network_tier = string
    }))
  }))
}

variable "access_config" {
  description = "Access configurations, i.e. IPs via which the VM instance can be accessed via the Internet."
  type = list(object({
    nat_ip       = string
    network_tier = string
  }))
  default = []
}

############
# INSTANCE #
############

variable "machine_type" {
  type        = string
  description = "Machine type to create."
  default     = "n1-standard-1"
}

variable "min_cpu_platform" {
  type        = string
  description = <<EOD
Specifies a minimum CPU platform. Applicable values are the friendly names of
CPU platforms, such as Intel Haswell or Intel Skylake. See the complete list:
https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform
EOD
  default     = null
}

variable "gpu" {
  type = object({
    type  = string
    count = number
  })
  description = <<EOD
GPU information. Type and count of GPU to attach to the instance template. See
https://cloud.google.com/compute/docs/gpus more details.
- type : the GPU type
- count : number of GPUs
EOD
  default     = null
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = <<EOD
Service account to attach to the instances. See
'main.tf:local.service_account' for the default.
EOD
  default     = null
}

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = <<EOD
Shielded VM configuration for the instance. Note: not used unless
enable_shielded_vm is 'true'.
- enable_integrity_monitoring : Compare the most recent boot measurements to the
  integrity policy baseline and return a pair of pass/fail results depending on
  whether they match or not.
- enable_secure_boot : Verify the digital signature of all boot components, and
  halt the boot process if signature verification fails.
- enable_vtpm : Use a virtualized trusted platform module, which is a
  specialized computer chip you can use to encrypt objects like keys and
  certificates.
EOD
  default = {
    enable_integrity_monitoring = true
    enable_secure_boot          = true
    enable_vtpm                 = true
  }
}

variable "enable_confidential_vm" {
  type        = bool
  description = "Enable the Confidential VM configuration. Note: the instance image must support option."
  default     = false
}

variable "enable_shielded_vm" {
  type        = bool
  description = "Enable the Shielded VM configuration. Note: the instance image must support option."
  default     = false
}

variable "preemptible" {
  type        = bool
  description = "Allow the instance to be preempted."
  default     = false
}

variable "spot" {
  description = <<-EOD
    Provision as a SPOT preemptible instance.
    See https://cloud.google.com/compute/docs/instances/spot for more details.
  EOD
  type        = bool
  default     = false
}

variable "termination_action" {
  description = <<-EOD
    Which action to take when Compute Engine preempts the VM. Value can be: 'STOP', 'DELETE'. The default value is 'STOP'.
    See https://cloud.google.com/compute/docs/instances/spot for more details.
  EOD
  type        = string
  default     = "STOP"

  validation {
    condition     = var.termination_action == null ? true : contains(["STOP", "DELETE"], var.termination_action)
    error_message = "Allowed values are: 'STOP', 'DELETE'."
  }
}

############
# METADATA #
############

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map."
  default     = {}
}

################
# SOURCE IMAGE #
################

variable "source_image_project" {
  type        = string
  description = "Project where the source image comes from. If it is not provided, the provider project is used."
  default     = ""
}

variable "source_image_family" {
  type        = string
  description = "Source image family."
  default     = ""
}

variable "source_image" {
  type        = string
  description = "Source disk image."
  default     = ""
}

########
# DISK #
########

variable "disk_type" {
  type        = string
  description = "Boot disk type, can be either pd-ssd, local-ssd, or pd-standard."
  default     = "pd-standard"
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB."
  default     = 100
}

variable "disk_labels" {
  type        = map(string)
  description = "Labels to be assigned to boot disk, provided as a map."
  default     = {}
}

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}

variable "additional_disks" {
  type = list(object({
    disk_name    = string
    device_name  = string
    disk_type    = string
    disk_size_gb = number
    disk_labels  = map(string)
    auto_delete  = bool
    boot         = bool
  }))
  description = "List of maps of disks."
  default     = []
}

#########
# SLURM #
#########

variable "slurm_instance_role" {
  type        = string
  description = "Slurm instance type. Must be one of: controller; login; compute; or null."

  validation {
    condition     = contains(["controller", "login", "compute"], var.slurm_instance_role)
    error_message = "Must be one of: controller; login; compute."
  }
}

variable "slurm_cluster_name" {
  type        = string
  description = "Cluster name, used for resource naming."

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]{0,9})$", var.slurm_cluster_name))
    error_message = "Variable 'slurm_cluster_name' must be a match of regex '^[a-z](?:[a-z0-9]{0,9})$'."
  }
}

variable "disable_smt" {
  type        = bool
  description = "Disables Simultaneous Multi-Threading (SMT) on instance."
  default     = false
}

variable "slurm_bucket_path" {
  description = "GCS Bucket URI of Slurm cluster file storage."
  type        = string
}
