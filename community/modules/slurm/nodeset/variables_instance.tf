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

// BEGIN: Boot disk
variable "disk_labels" {
  description = "Labels specific to the boot disk. These will be merged with var.labels."
  type        = map(string)
  default     = {}
}

variable "disk_size_gb" {
  description = "Size of boot disk to create for the partition compute nodes."
  type        = number
  default     = 50
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

variable "disk_auto_delete" {
  type        = bool
  description = "Whether or not the boot disk should be auto-deleted."
  default     = true
}
// END: Boot disk

variable "additional_disks" {
  description = "Configurations of additional disks to be included on the instance. (do not use \"disk_type: local-ssd\"; known issue being addressed)"
  type = list(object({
    disk_name    = optional(string)
    device_name  = optional(string)
    disk_size_gb = optional(number)
    disk_type    = optional(string)
    disk_labels  = optional(map(string), {})
    auto_delete  = optional(bool, true)
    boot         = optional(bool, false)
  }))
  default = []
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
  description = "Enable IP forwarding, for NAT instances for example."
  type        = bool
  default     = false
}

variable "enable_smt" {
  type        = bool
  description = "Enables Simultaneous Multi-Threading (SMT) on instance."
  default     = false
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

variable "enable_public_ip" {  // REVIEW_NOTE: change from V5 `disable_public_ips`
  description = "If set to true. The VM will have a random public IP assigned to it"
  type        = bool
  default     = false
}

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

variable "labels" {
  description = "Labels to add to partition compute instances. Key-value pairs."
  type        = map(string)
  default     = {}
}

variable "metadata" {
  type        = map(string)
  description = "Metadata, provided as a map."
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

variable "network_tier" { // REVIEWER_NOTE: instead of V5 access_config
  type        = string
  description = <<-EOD
    The networking tier used for configuring this instance. This field can take the following values: PREMIUM, FIXED_STANDARD or STANDARD.
    Ignored if enable_public_ip is false.
  EOD
  default     = "STANDARD"

  validation {
    condition     = var.network_tier == null ? true : contains(["PREMIUM", "FIXED_STANDARD", "STANDARD"], var.network_tier)
    error_message = "Allow values are: 'PREMIUM', 'FIXED_STANDARD', 'STANDARD'."
  }
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

variable "instance_image" {
  description = <<-EOD
    Defines the image that will be used on VM instances. 

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.

    For more information on creating custom images that comply with Slurm on GCP
    see the "Slurm on GCP Custom Images" section in docs/vm-images.md.
    EOD
  type        = map(string)
  default = { // TODO: default to null ?
    family  = "slurm-gcp-6-1-hpc-centos-7"
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

variable "subnetwork_self_link" {
  type        = string
  description = "Subnet to deploy to."
  default     = null
}

variable "tags" {
  type        = list(string)
  description = "Network tag list."
  default     = []
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
