# Copyright 2023 Google LLC
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


variable "project_id" { # tflint-ignore: terraform_unused_declarations
  type        = string
  description = "Project ID to create resources in."
}

variable "region" {
  type        = string
  description = "Region where the instances should be created."
  default     = null
}

variable "zone" {
  type        = string
  description = <<-EOD
    Zone where the instances should be created. If not specified, instances will be
    spread across available zones in the region.
    EOD
  default     = null
}

variable "name_prefix" {
  type        = string
  description = <<-EOD
    Unique name prefix for login nodes. Automatically populated by the module id if not set.
    If setting manually, ensure a unique value across all login groups.
    EOD
}

variable "num_instances" {
  type        = number
  description = "Number of instances to create. This value is ignored if static_ips is provided."
  default     = 1
}

variable "resource_manager_tags" {
  description = "(Optional) A set of key/value resource manager tag pairs to bind to the instances. Keys must be in the format tagKeys/{tag_key_id}, and values are in the format tagValues/456."
  type        = map(string)
  default     = {}
}

variable "disk_type" {
  type        = string
  description = "Boot disk type, can be either hyperdisk-balanced, pd-ssd, pd-standard, pd-balanced, or pd-extreme."
  default     = "pd-ssd"
}

variable "disk_size_gb" {
  type        = number
  description = "Boot disk size in GB."
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

variable "disk_resource_manager_tags" {
  description = "(Optional) A set of key/value resource manager tag pairs to bind to the instance disks. Keys must be in the format tagKeys/{tag_key_id}, and values are in the format tagValues/456."
  type        = map(string)
  default     = {}
  validation {
    condition     = alltrue([for value in var.disk_resource_manager_tags : can(regex("tagValues/[0-9]+", value))])
    error_message = "All Resource Manager tag values should be in the format 'tagValues/[0-9]+'"
  }
  validation {
    condition     = alltrue([for value in keys(var.disk_resource_manager_tags) : can(regex("tagKeys/[0-9]+", value))])
    error_message = "All Resource Manager tag keys should be in the format 'tagKeys/[0-9]+'"
  }
}

variable "additional_disks" {
  type = list(object({
    disk_name                  = optional(string)
    device_name                = optional(string)
    disk_size_gb               = optional(number)
    disk_type                  = optional(string)
    disk_labels                = optional(map(string))
    auto_delete                = optional(bool)
    boot                       = optional(bool)
    disk_resource_manager_tags = optional(map(string))
  }))
  description = "List of maps of disks."
  default     = []
}

variable "additional_networks" {
  description = "Additional network interface details for GCE, if any."
  default     = []
  type = list(object({
    access_config = optional(list(object({
      nat_ip       = string
      network_tier = string
    })), [])
    alias_ip_range = optional(list(object({
      ip_cidr_range         = string
      subnetwork_range_name = string
    })), [])
    ipv6_access_config = optional(list(object({
      network_tier = string
    })), [])
    network            = optional(string)
    network_ip         = optional(string, "")
    nic_type           = optional(string)
    queue_count        = optional(number)
    stack_type         = optional(string)
    subnetwork         = optional(string)
    subnetwork_project = optional(string)
  }))
  nullable = false
}

variable "advanced_machine_features" {
  description = "See https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_instance_template#nested_advanced_machine_features"
  type = object({
    enable_nested_virtualization = optional(bool)
    threads_per_core             = optional(number)
    turbo_mode                   = optional(string)
    visible_core_count           = optional(number)
    performance_monitoring_unit  = optional(string)
    enable_uefi_networking       = optional(bool)
  })
  default = {
    threads_per_core = 1 # disable SMT by default
  }
}

variable "enable_smt" { # tflint-ignore: terraform_unused_declarations
  type        = bool
  description = "DEPRECATED: Use `advanced_machine_features.threads_per_core` instead."
  default     = null
  validation {
    condition     = var.enable_smt == null
    error_message = "DEPRECATED: Use `advanced_machine_features.threads_per_core` instead."
  }
}

variable "disable_smt" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Use `advanced_machine_features.threads_per_core` instead."
  type        = bool
  default     = null
  validation {
    condition     = var.disable_smt == null
    error_message = "DEPRECATED: Use `advanced_machine_features.threads_per_core` instead."
  }
}

variable "static_ips" {
  type        = list(string)
  description = "List of static IPs for VM instances."
  default     = []
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

variable "can_ip_forward" {
  type        = bool
  description = "Enable IP forwarding, for NAT instances for example."
  default     = false
}

variable "enable_login_public_ips" {
  description = "If set to true. The login node will have a random public IP assigned to it."
  type        = bool
  default     = false
}


variable "disable_login_public_ips" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Use `enable_login_public_ips` instead."
  type        = bool
  default     = null
  validation {
    condition     = var.disable_login_public_ips == null
    error_message = "DEPRECATED: Use `enable_login_public_ips` instead."
  }
}

variable "enable_oslogin" {
  type        = bool
  description = <<-EOD
    Enables Google Cloud os-login for user login and authentication for VMs.
    See https://cloud.google.com/compute/docs/oslogin
    EOD
  default     = true
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

variable "shielded_instance_config" {
  type = object({
    enable_integrity_monitoring = bool
    enable_secure_boot          = bool
    enable_vtpm                 = bool
  })
  description = <<EOD
Shielded VM configuration for the instance. Note: not used unless
enable_shielded_vm is 'true'.
  enable_integrity_monitoring : Compare the most recent boot measurements to the
  integrity policy baseline and return a pair of pass/fail results depending on
  whether they match or not.
  enable_secure_boot : Verify the digital signature of all boot components, and
  halt the boot process if signature verification fails.
  enable_vtpm : Use a virtualized trusted platform module, which is a
  specialized computer chip you can use to encrypt objects like keys and
  certificates.
EOD
  default = {
    enable_integrity_monitoring = true
    enable_secure_boot          = true
    enable_vtpm                 = true
  }
}

variable "guest_accelerator" {
  description = "List of the type and count of accelerator cards attached to the instance."
  type = list(object({
    type  = string,
    count = number
  }))
  default  = []
  nullable = false

  validation {
    condition     = length(var.guest_accelerator) <= 1
    error_message = "The Slurm modules supports 0 or 1 models of accelerator card on each node."
  }
}

variable "labels" {
  type        = map(string)
  description = "Labels, provided as a map."
  default     = {}
}

variable "machine_type" {
  type        = string
  description = "Machine type to create."
  default     = "c2-standard-4"
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map."
  default     = {}
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

variable "preemptible" {
  type        = bool
  description = "Allow the instance to be preempted."
  default     = false
}

variable "on_host_maintenance" {
  type        = string
  description = "Instance availability Policy."
  default     = "MIGRATE"
}

variable "service_account_email" {
  description = "Service account e-mail address to attach to the login instances."
  type        = string
  default     = null
}

variable "service_account_scopes" {
  description = "Scopes to attach to the login instances."
  type        = set(string)
  default     = ["https://www.googleapis.com/auth/cloud-platform"]
}

variable "service_account" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Use `service_account_email` and `service_account_scopes` instead."
  type = object({
    email  = string
    scopes = set(string)
  })
  default = null
  validation {
    condition     = var.service_account == null
    error_message = "DEPRECATED: Use `service_account_email` and `service_account_scopes` instead."
  }
}

variable "instance_template" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Instance template can not be specified for login nodes."
  type        = string
  default     = null
  validation {
    condition     = var.instance_template == null
    error_message = "DEPRECATED: Instance template can not be specified for login nodes."
  }
}

variable "instance_image" {
  description = <<-EOD
    Defines the image that will be used in the Slurm controller VM instance.

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.

    For more information on creating custom images that comply with Slurm on GCP
    see the "Slurm on GCP Custom Images" section in docs/vm-images.md.
    EOD
  type        = map(string)
  default = {
    family  = "slurm-gcp-6-10-hpc-rocky-linux-8"
    project = "schedmd-slurm-public"
  }

  validation {
    condition     = can(coalesce(var.instance_image.project))
    error_message = "In var.instance_image, the \"project\" field must be a string set to the Cloud project ID."
  }

  validation {
    condition     = can(coalesce(var.instance_image.name)) != can(coalesce(var.instance_image.family))
    error_message = "In var.instance_image, exactly one of \"family\" or \"name\" fields must be set to desired image family or name."
  }
}

variable "instance_image_custom" {
  description = <<-EOD
    A flag that designates that the user is aware that they are requesting
    to use a custom and potentially incompatible image for this Slurm on
    GCP module.

    If the field is set to false, only the compatible families and project
    names will be accepted.  The deployment will fail with any other image
    family or name.  If set to true, no checks will be done.

    See: https://goo.gle/hpc-slurm-images
    EOD
  type        = bool
  default     = false
}

variable "allow_automatic_updates" {
  description = <<-EOT
  If false, disables automatic system package updates on the created instances.  This feature is
  only available on supported images (or images derived from them).  For more details, see
  https://cloud.google.com/compute/docs/instances/create-hpc-vm#disable_automatic_updates
  EOT
  type        = bool
  default     = true
  nullable    = false
}

variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
}

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to."
}
