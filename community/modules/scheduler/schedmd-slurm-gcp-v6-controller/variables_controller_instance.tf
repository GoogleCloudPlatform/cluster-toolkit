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

variable "enable_smt" {
  type        = bool
  description = "Enables Simultaneous Multi-Threading (SMT) on instance."
  default     = false
}

variable "disable_smt" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Use `enable_smt` instead."
  type        = bool
  default     = null
  validation {
    condition     = var.disable_smt == null
    error_message = "DEPRECATED: Use `enable_smt` instead."
  }
}

variable "static_ips" {
  type        = list(string)
  description = "List of static IPs for VM instances."
  default     = []
  validation {
    condition     = length(var.static_ips) <= 1
    error_message = "The Slurm modules supports 0 or 1 static IPs on controller instance."
  }
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

variable "enable_controller_public_ips" {
  description = "If set to true. The controller will have a random public IP assigned to it. Ignored if access_config is set."
  type        = bool
  default     = false
}

variable "disable_controller_public_ips" { # tflint-ignore: terraform_unused_declarations
  description = "DEPRECATED: Use `enable_controller_public_ips` instead."
  type        = bool
  default     = null
  validation {
    condition     = var.disable_controller_public_ips == null
    error_message = "DEPRECATED: Use `enable_controller_public_ips` instead."
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
  description = "Service account e-mail address to attach to the controller instance."
  type        = string
  default     = null
}

variable "service_account_scopes" {
  description = "Scopes to attach to the controller instance."
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
  description = "DEPRECATED: Instance template can not be specified for controller."
  type        = string
  default     = null
  validation {
    condition     = var.instance_template == null
    error_message = "DEPRECATED: Instance template can not be specified for controller."
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
    family  = "slurm-gcp-6-8-hpc-rocky-linux-8"
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
