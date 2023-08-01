/**
 * Copyright 2023 Google LLC
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

# Most variables have been sourced and modified from the SchedMD/slurm-gcp
# github repository: https://github.com/SchedMD/slurm-gcp/tree/5.7.4

variable "project_id" {
  description = "Project in which the HPC deployment will be created."
  type        = string
}

## Node Group Definition

variable "name" {
  description = "Name of the node group."
  type        = string
  default     = "ghpc"

  validation {
    condition     = can(regex("^[a-z](?:[a-z0-9]{0,5})$", var.name))
    error_message = "Node group name (var.name) must begin with a letter, be fully alphanumeric and be 6 characters or less. Regexp: '^[a-z](?:[a-z0-9]{0,5})$'."
  }
}

variable "node_conf" {
  description = "Map of Slurm node line configuration."
  type        = map(any)
  default     = {}
}

variable "node_count_dynamic_max" {
  description = "Maximum number of dynamic nodes allowed in this partition."
  type        = number
  default     = 10
}

variable "node_count_static" {
  description = "Number of nodes to be statically created."
  type        = number
  default     = 0
}

## VM Definition

variable "instance_template" {
  description = <<-EOD
    Self link to a custom instance template. If set, other VM definition
    variables such as machine_type and instance_image will be ignored in favor
    of the provided instance template.

    For more information on creating custom images for the instance template
    that comply with Slurm on GCP see the "Slurm on GCP Custom Images" section
    in docs/vm-images.md.
    EOD
  type        = string
  default     = null
}

variable "machine_type" {
  description = "Compute Platform machine type to use for this partition compute nodes."
  type        = string
  default     = "c2-standard-60"
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map."
  default     = {}
}

variable "instance_image" {
  description = <<-EOD
    Defines the image that will be used in the node group VM instances. 

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.

    For more information on creating custom images that comply with Slurm on GCP
    see the "Slurm on GCP Custom Images" section in docs/vm-images.md.
    EOD
  type        = map(string)
  default = {
    family  = "slurm-gcp-5-7-hpc-centos-7"
    project = "schedmd-slurm-public"
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

variable "source_image_project" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image_project == null
    error_message = "Variable `source_image_project` is deprecated. Use `instance_image` instead."
  }
}

variable "source_image_family" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image_family == null
    error_message = "Variable `source_image_family` is deprecated. Use `instance_image` instead."
  }
}

variable "source_image" {
  type        = string
  description = "DEPRECATED: Use `instance_image` instead."
  default     = null
  validation {
    condition     = var.source_image == null
    error_message = "Variable `source_image` is deprecated. Use `instance_image` instead."
  }
}

variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
}

variable "disk_type" {
  description = "Boot disk type, can be either pd-ssd, pd-standard, pd-balanced, or pd-extreme."
  type        = string
  default     = "pd-standard"

  validation {
    condition     = contains(["pd-ssd", "pd-standard", "pd-balanced", "pd-extreme"], var.disk_type)
    error_message = "Variable disk_type must be one of pd-ssd, pd-standard, pd-balanced, or pd-extreme."
  }
}

variable "disk_size_gb" {
  description = "Size of boot disk to create for the partition compute nodes."
  type        = number
  default     = 50
}

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}

variable "disk_labels" {
  description = "Labels specific to the boot disk. These will be merged with var.labels."
  type        = map(string)
  default     = {}
}

variable "additional_disks" {
  description = "Configurations of additional disks to be included on the partition nodes. (do not use \"disk_type: local-ssd\"; known issue being addressed)"
  type = list(object({
    disk_name    = string
    device_name  = string
    disk_size_gb = number
    disk_type    = string
    disk_labels  = map(string)
    auto_delete  = bool
    boot         = bool
  }))
  default = []
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

variable "enable_oslogin" {
  type        = bool
  description = <<-EOD
    Enables Google Cloud os-login for user login and authentication for VMs.
    See https://cloud.google.com/compute/docs/oslogin
    EOD
  default     = true
}

variable "can_ip_forward" {
  description = "Enable IP forwarding, for NAT instances for example."
  type        = bool
  default     = false
}

variable "enable_smt" {
  type        = bool
  description = "Enables Simultaneous Multi-Threading (SMT) on instance."
  default     = false
}

variable "labels" {
  description = "Labels to add to partition compute instances. Key-value pairs."
  type        = map(string)
  default     = {}
}

variable "min_cpu_platform" {
  description = "The name of the minimum CPU platform that you want the instance to use."
  type        = string
  default     = null
}

variable "on_host_maintenance" {
  type        = string
  description = <<-EOD
    Instance availability Policy.

    Note: Placement groups are not supported when on_host_maintenance is set to
    "MIGRATE" and will be deactivated regardless of the value of
    enable_placement. To support enable_placement, ensure on_host_maintenance is
    set to "TERMINATE".
    EOD
  default     = "TERMINATE"
}

variable "gpu" {
  description = <<-EOD
    GPU information. Type and count of GPU to attach to the instance template. See
    https://cloud.google.com/compute/docs/gpus more details.
    - type : the GPU type, e.g. nvidia-tesla-t4, nvidia-a100-80gb, nvidia-tesla-a100, etc
    - count : number of GPUs

    If both 'var.gpu' and 'var.guest_accelerator' are set, 'var.gpu' will be used.
    EOD
  type = object({
    count = number,
    type  = string
  })
  default = null
}

variable "guest_accelerator" {
  description = <<-EOD
    Alternative method of providing 'var.gpu' with a consistent naming scheme to
    other HPC Toolkit modules.

    If both 'var.gpu' and 'var.guest_accelerator' are set, 'var.gpu' will be used.
    EOD
  type = list(object({
    type  = string,
    count = number
  }))
  default = null
}

variable "preemptible" {
  description = "Should use preemptibles to burst."
  type        = bool
  default     = false
}

variable "service_account" {
  type = object({
    email  = string
    scopes = set(string)
  })
  description = <<-EOD
    Service account to attach to the compute instances. If not set, the
    default compute service account for the given project will be used with the
    "https://www.googleapis.com/auth/cloud-platform" scope.
    EOD
  default     = null
}

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = <<-EOD
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

variable "enable_spot_vm" {
  description = "Enable the partition to use spot VMs (https://cloud.google.com/spot-vms)."
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

variable "access_config" {
  description = "Access configurations, i.e. IPs via which the node group instances can be accessed via the internet."
  type = list(object({
    network_tier = string
  }))
  default = []
}

variable "disable_public_ips" {
  description = "If set to false. The node group VMs will have a random public IP assigned to it. Ignored if access_config is set."
  type        = bool
  default     = true
}
