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
  description = "ID of project in which Filestore instance will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the HPC deployment, used as name of the filestore instance if no name is specified."
  type        = string
}

variable "zone" {
  description = "Location for Filestore instances below Enterprise tier."
  type        = string
}

variable "region" {
  description = "Location for Filestore instances at Enterprise tier."
  type        = string
}

variable "network_id" {
  description = <<-EOT
    The ID of the GCE VPC network to which the instance is connected given in the format:
    `projects/<project_id>/global/networks/<network_name>`"
    EOT
  type        = string
  validation {
    condition     = length(split("/", var.network_id)) == 5
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
}

variable "name" {
  description = "The resource name of the instance."
  type        = string
  default     = null
}

variable "filestore_share_name" {
  description = "Name of the file system share on the instance."
  type        = string
  default     = "nfsshare"
}

variable "local_mount" {
  description = "Mountpoint for this filestore instance. Note: If set to the same as the `filestore_share_name`, it will trigger a known Slurm bug ([troubleshooting](../../../docs/slurm-troubleshooting.md))."
  type        = string
  default     = "/shared"
}

variable "size_gb" {
  description = "Storage size of the filestore instance in GB."
  type        = number
  default     = 1024
  validation {
    condition     = var.size_gb >= 1024
    error_message = "No Filestore tier supports less than 1024GiB.\nSee https://cloud.google.com/filestore/docs/service-tiers."
  }
}

variable "filestore_tier" {
  description = "The service tier of the instance."
  type        = string
  default     = "BASIC_HDD"
  validation {
    condition     = var.filestore_tier != "STANDARD"
    error_message = "The preferred name for STANDARD tier is now BASIC_HDD\nhttps://cloud.google.com/filestore/docs/reference/rest/v1beta1/Tier."
  }
  validation {
    condition     = var.filestore_tier != "PREMIUM"
    error_message = "The preferred name for PREMIUM tier is now BASIC_SSD\nhttps://cloud.google.com/filestore/docs/reference/rest/v1beta1/Tier."
  }
  validation {
    condition = contains([
      "BASIC_HDD",
      "BASIC_SSD",
      "HIGH_SCALE_SSD",
      "ZONAL",
      "ENTERPRISE"
    ], var.filestore_tier)
    error_message = "Allowed values for filestore_tier are 'BASIC_HDD','BASIC_SSD','HIGH_SCALE_SSD','ZONAL','ENTERPRISE'.\nhttps://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/filestore_instance#tier\nhttps://cloud.google.com/filestore/docs/reference/rest/v1beta1/Tier."
  }
}

variable "labels" {
  description = "Labels to add to the filestore instance. Key-value pairs."
  type        = map(string)
}

variable "connect_mode" {
  description = "Used to select mode - supported values DIRECT_PEERING and PRIVATE_SERVICE_ACCESS."
  type        = string
  default     = "DIRECT_PEERING"
  nullable    = false
  validation {
    condition     = contains(["DIRECT_PEERING", "PRIVATE_SERVICE_ACCESS"], var.connect_mode)
    error_message = "Allowed values for connect_mode are \"DIRECT_PEERING\" or \"PRIVATE_SERVICE_ACCESS\"."
  }
}

variable "nfs_export_options" {
  description = "Define NFS export options."
  type = list(object({
    access_mode = optional(string)
    ip_ranges   = optional(list(string))
    squash_mode = optional(string)
  }))
  default  = []
  nullable = false
}

variable "reserved_ip_range" {
  description = <<-EOT
    Reserved IP range for Filestore instance. Users are encouraged to set to null
    for automatic selection. If supplied, it must be:

    CIDR format when var.connect_mode == "DIRECT_PEERING"
    Named IP Range when var.connect_mode == "PRIVATE_SERVICE_ACCESS"

    See Cloud documentation for more details:

    https://cloud.google.com/filestore/docs/creating-instances#configure_a_reserved_ip_address_range
  EOT
  type        = string
  default     = null
  nullable    = true
}

variable "mount_options" {
  description = "NFS mount options to mount file system."
  type        = string
  default     = "defaults,_netdev"
}

variable "deletion_protection" {
  description = "Configure Filestore instance deletion protection"
  type = object({
    enabled = optional(bool, false)
    reason  = optional(string)
  })
  default = {
    enabled = false
  }
  nullable = false

  validation {
    condition     = !can(coalesce(var.deletion_protection.reason)) || var.deletion_protection.enabled
    error_message = "Cannot set Filestore var.deletion_protection.reason unless var.deletion_protection.enabled is true"
  }
}
