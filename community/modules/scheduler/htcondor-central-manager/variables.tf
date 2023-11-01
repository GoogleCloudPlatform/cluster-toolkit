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

variable "project_id" {
  description = "Project in which HTCondor central manager will be created"
  type        = string
}

variable "deployment_name" {
  description = "HPC Toolkit deployment name. HTCondor cloud resource names will include this value."
  type        = string
}

variable "labels" {
  description = "Labels to add to resources. List key, value pairs."
  type        = map(string)
}

variable "region" {
  description = "Default region for creating resources"
  type        = string
}

variable "zones" {
  description = "Zone(s) in which central manager may be created. If not supplied, will default to all zones in var.region."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "distribution_policy_target_shape" {
  description = "Target shape for instance group managing high availability of central manager"
  type        = string
  default     = "BALANCED"
}

variable "network_self_link" {
  description = "The self link of the network in which the HTCondor central manager will be created."
  type        = string
  default     = null
}

variable "central_manager_service_account_email" {
  description = "Service account e-mail for central manager (can be supplied by htcondor-setup module)"
  type        = string
}

variable "service_account_scopes" {
  description = "Scopes by which to limit service account attached to central manager."
  type        = set(string)
  default = [
    "https://www.googleapis.com/auth/cloud-platform",
  ]
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

variable "disk_size_gb" {
  description = "Boot disk size in GB"
  type        = number
  default     = 20
  nullable    = false
}

variable "metadata" {
  description = "Metadata to add to HTCondor central managers"
  type        = map(string)
  default     = {}
}

variable "enable_oslogin" {
  description = "Enable or Disable OS Login with \"ENABLE\" or \"DISABLE\". Set to \"INHERIT\" to inherit project OS Login setting."
  type        = string
  default     = "ENABLE"
  nullable    = false
  validation {
    condition     = contains(["ENABLE", "DISABLE", "INHERIT"], var.enable_oslogin)
    error_message = "Allowed string values for var.enable_oslogin are \"ENABLE\", \"DISABLE\", or \"INHERIT\"."
  }
}

variable "subnetwork_self_link" {
  description = "The self link of the subnetwork in which the HTCondor central manager will be created."
  type        = string
  default     = null
}

variable "instance_image" {
  description = <<-EOD
    Custom VM image with HTCondor installed using the htcondor-install module."

    Expected Fields:
    name: The name of the image. Mutually exclusive with family.
    family: The image family to use. Mutually exclusive with name.
    project: The project where the image is hosted.
    EOD
  type        = map(string)

  validation {
    condition     = can(coalesce(var.instance_image.project))
    error_message = "In var.instance_image, the \"project\" field must be a string set to the Cloud project ID."
  }

  validation {
    condition     = can(coalesce(var.instance_image.name)) != can(coalesce(var.instance_image.family))
    error_message = "In var.instance_image, exactly one of \"family\" or \"name\" fields must be set to desired image family or name."
  }
}

variable "machine_type" {
  description = "Machine type to use for HTCondor central managers"
  type        = string
  default     = "c2-standard-4"
}

variable "central_manager_runner" {
  description = "A list of Toolkit runners for configuring an HTCondor central manager"
  type        = list(map(string))
  default     = []
}

variable "htcondor_bucket_name" {
  description = "Name of HTCondor configuration bucket"
  type        = string
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
