/**
 * Copyright 2026 Google LLC
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
  description = "ID of project in which the NetApp storage pool will be created."
  type        = string
}

variable "deployment_name" {
  description = "Name of the deployment, used as name of the NetApp storage pool if no name is specified."
  type        = string
}

variable "region" {
  description = "Location for NetApp storage pool."
  type        = string
}

variable "network_id" {
  description = <<-EOT
    The ID of the GCE VPC network to which the NetApp storage pool is connected given in the format:
    `projects/<project_id>/global/networks/<network_name>`"
    EOT
  type        = string
  validation {
    condition     = length(split("/", var.network_id)) == 5
    error_message = "The network id must be provided in the following format: projects/<project_id>/global/networks/<network_name>."
  }
}

variable "network_self_link" {
  description = "Network self-link the pool will be on, required for checking private service access"
  type        = string
  nullable    = false
}

variable "private_vpc_connection_peering" {
  description = "The name of the private VPC connection peering."
  type        = string
  default     = "sn-netapp-prod"
}

variable "pool_name" {
  description = "The name of the storage pool. Leave empty to generate name based on deployment name."
  type        = string
  default     = null
}

variable "service_level" {
  description = "The service level of the storage pool."
  type        = string
  default     = "PREMIUM"
  validation {
    condition     = contains(["STANDARD", "PREMIUM", "EXTREME"], var.service_level)
    error_message = "Allowed values for service_level are 'STANDARD', 'PREMIUM', or 'EXTREME'."
  }
}

variable "capacity_gib" {
  description = "The capacity of the storage pool in GiB."
  type        = number
  default     = 2048
  validation {
    condition     = var.capacity_gib >= 2048
    error_message = "The minimum capacity for the storage pool is 2048 GiB."
  }
}

variable "active_directory_policy" {
  description = <<-EOT
    The ID of the Active Directory policy to apply to the storage pool in the format:
    `projects/<project_id>/locations/<location>/activeDirectoryPolicies/<policy_id>`
    EOT
  type        = string
  default     = null
  validation {
    condition     = var.active_directory_policy == null ? true : length(split("/", var.active_directory_policy)) == 6
    error_message = "The active directory policy must be provided in the following format: projects/<project_id>/locations/<location>/activeDirectoryPolicies/<policy_id>."
  }
}

variable "cmek_policy" {
  description = <<-EOT
    The ID of the Customer Managed Encryption Key (CMEK) policy to apply to the storage pool in the format:
    `projects/<project>/locations/<location>/kmsConfigs/<name>`
    EOT
  type        = string
  default     = null
  validation {
    condition     = var.cmek_policy == null ? true : length(split("/", var.cmek_policy)) == 6
    error_message = "The CMEK policy must be provided in the following format: projects/<project>/locations/<location>/kmsConfigs/<name>."
  }
}

variable "ldap_enabled" {
  description = "Whether to enable LDAP for the storage pool."
  type        = bool
  default     = false
}

variable "allow_auto_tiering" {
  description = "Whether to allow automatic tiering for the storage pool."
  type        = bool
  default     = false
}

variable "description" {
  description = "A description of the NetApp storage pool."
  type        = string
  default     = ""
  validation {
    condition     = length(var.description) <= 2048
    error_message = "NetApp storage pool description must be 2048 characters or fewer"
  }
}

variable "labels" {
  description = "Labels to add to the NetApp storage pool. Key-value pairs."
  type        = map(string)
}
