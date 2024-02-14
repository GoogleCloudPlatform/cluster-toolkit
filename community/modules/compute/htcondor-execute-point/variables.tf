/**
 * Copyright 2022 Google LLC
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

variable "project_id" {
  description = "Project in which the HTCondor execute points will be created"
  type        = string
}

variable "region" {
  description = "The region in which HTCondor execute points will be created"
  type        = string
}

variable "zones" {
  description = "Zone(s) in which execute points may be created. If not supplied, will default to all zones in var.region."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "distribution_policy_target_shape" {
  description = "Target shape across zones for instance group managing execute points"
  type        = string
  default     = "ANY"
}

variable "deployment_name" {
  description = "HPC Toolkit deployment name. HTCondor cloud resource names will include this value."
  type        = string
}

variable "labels" {
  description = "Labels to add to HTConodr execute points"
  type        = map(string)
}

variable "machine_type" {
  description = "Machine type to use for HTCondor execute points"
  type        = string
  default     = "n2-standard-4"
}

variable "execute_point_runner" {
  description = "A list of Toolkit runners for configuring an HTCondor execute point"
  type        = list(map(string))
  default     = []
}

variable "network_storage" {
  description = "An array of network attached storage mounts to be configured"
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

variable "instance_image" {
  description = <<-EOD
    HTCondor execute point VM image

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    EOD
  type        = map(string)
  default = {
    project = "cloud-hpc-image-public"
    family  = "hpc-rocky-linux-8"
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

variable "execute_point_service_account_email" {
  description = "Service account for HTCondor execute point (e-mail format)"
  type        = string
}

variable "service_account_scopes" {
  description = "Scopes by which to limit service account attached to central manager."
  type        = set(string)
  default = [
    "https://www.googleapis.com/auth/cloud-platform",
  ]
}

variable "network_self_link" {
  description = "The self link of the network HTCondor execute points will join"
  type        = string
  default     = "default"
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork HTCondor execute points will join"
  type        = string
  default     = null
}

variable "target_size" {
  description = "Initial size of the HTCondor execute point pool; set to null (default) to avoid Terraform management of size."
  type        = number
  default     = null
}

variable "max_size" {
  description = "Maximum size of the HTCondor execute point pool."
  type        = number
  default     = 100
}

variable "min_idle" {
  description = "Minimum number of idle VMs in the HTCondor pool (if pool reaches var.max_size, this minimum is not guaranteed); set to ensure jobs beginning run more quickly."
  type        = number
  default     = 0
}

variable "metadata" {
  description = "Metadata to add to HTCondor execute points"
  type        = map(string)
  default     = {}
}

# this default is deliberately the opposite of vm-instance because of observed
# issues running HTCondor docker universe jobs with OS Login enabled and running
# jobs as a user with uid>2^31; these uids occur when users outside the GCP
# organization login to a VM and OS Login is enabled.
variable "enable_oslogin" {
  description = "Enable or Disable OS Login with \"ENABLE\" or \"DISABLE\". Set to \"INHERIT\" to inherit project OS Login setting."
  type        = string
  default     = "ENABLE"
  validation {
    condition     = var.enable_oslogin == null ? false : contains(["ENABLE", "DISABLE", "INHERIT"], var.enable_oslogin)
    error_message = "Allowed string values for var.enable_oslogin are \"ENABLE\", \"DISABLE\", or \"INHERIT\"."
  }
}

variable "spot" {
  description = "Provision VMs using discounted Spot pricing, allowing for preemption"
  type        = bool
  default     = false
}

variable "disk_size_gb" {
  description = "Boot disk size in GB"
  type        = number
  default     = 100
}

variable "windows_startup_ps1" {
  description = "Startup script to run at boot-time for Windows-based HTCondor execute points"
  type        = list(string)
  default     = []
  nullable    = false
}

variable "central_manager_ips" {
  description = "List of IP addresses of HTCondor Central Managers"
  type        = list(string)
}

variable "htcondor_bucket_name" {
  description = "Name of HTCondor configuration bucket"
  type        = string
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
    error_message = "The HTCondor module supports 0 or 1 models of accelerator card on each execute point"
  }
}

variable "name_prefix" {
  description = "Name prefix given to hostnames in this group of execute points; must be unique across all instances of this module"
  type        = string
  nullable    = false
  validation {
    condition     = length(var.name_prefix) > 0
    error_message = "var.name_prefix must be a set to a non-empty string and must also be unique across all instances of htcondor-execute-point"
  }
}

variable "enable_shielded_vm" {
  type        = bool
  default     = false
  description = "Enable the Shielded VM configuration (var.shielded_instance_config)."
}

variable "shielded_instance_config" {
  description = "Shielded VM configuration for the instance (must set var.enabled_shielded_vm)"
  type = object({
    enable_secure_boot          = bool
    enable_vtpm                 = bool
    enable_integrity_monitoring = bool
  })

  default = {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }
}
