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
  description = "Region for the storage pool. Required for all service levels."
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
    condition     = contains(["STANDARD", "PREMIUM", "EXTREME", "FLEX"], var.service_level)
    error_message = "Allowed values for service_level are 'STANDARD', 'PREMIUM', 'EXTREME', or 'FLEX'."
  }
}

variable "zone" {
  description = "Zone for zonal Flex Unified pools, or active zone for regional Flex Unified pools. Must be within region when service_level is FLEX. Ignored for STANDARD, PREMIUM, and EXTREME pools."
  type        = string
  default     = null
  validation {
    condition     = var.service_level != "FLEX" || var.zone == null || startswith(var.zone, "${var.region}-")
    error_message = "zone must be within the specified region."
  }
}

variable "replica_zone" {
  description = "Replica zone for regional Flex Unified pools. Must be within region when service_level is FLEX. Omit for zonal Flex Unified pools. Ignored for STANDARD, PREMIUM, and EXTREME pools."
  type        = string
  default     = null
  validation {
    condition     = var.service_level != "FLEX" || var.replica_zone == null || startswith(var.replica_zone, "${var.region}-")
    error_message = "replica_zone must be within the specified region."
  }
}

variable "capacity_gib" {
  description = "The capacity of the storage pool in GiB. Minimum is 2048 GiB for STANDARD, PREMIUM, and EXTREME; 1024 GiB for Flex Unified; 6144 GiB for large capacity (SCALE_TYPE_SCALEOUT) pools."
  type        = number
  default     = 2048
}

variable "active_directory_policy" {
  description = <<-EOT
    The ID of the Active Directory policy to apply to the storage pool in the format:
    `projects/<project_id>/locations/<location>/activeDirectories/<name>`
    EOT
  type        = string
  default     = null
  validation {
    condition = var.active_directory_policy == null ? true : can(
      regex("^projects/[^/]+/locations/[^/]+/activeDirectories/[^/]+$", var.active_directory_policy)
    )
    error_message = "The Active Directory policy must use the format projects/<project_id>/locations/<location>/activeDirectories/<name>."
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

  validation {
    condition     = var.service_level != "FLEX" || !var.allow_auto_tiering || var.hot_tier_size_gib != null
    error_message = "hot_tier_size_gib is required when allow_auto_tiering is true for FLEX pools."
  }

  validation {
    condition     = var.allow_auto_tiering || (var.hot_tier_size_gib == null && var.enable_hot_tier_auto_resize == null)
    error_message = "hot_tier_size_gib and enable_hot_tier_auto_resize require allow_auto_tiering to be true."
  }
}

variable "hot_tier_size_gib" {
  description = "Total hot tier capacity for the storage pool in GiB. Flex-only. Requires allow_auto_tiering to be true."
  type        = number
  default     = null

  validation {
    condition     = var.service_level == "FLEX" || var.hot_tier_size_gib == null
    error_message = "hot_tier_size_gib is supported only for FLEX storage pools."
  }
}

variable "enable_hot_tier_auto_resize" {
  description = "Whether hot-tier threshold will auto-increase when it reaches 100%. Flex-only. Requires allow_auto_tiering to be true."
  type        = bool
  default     = null

  validation {
    condition     = var.service_level == "FLEX" || var.enable_hot_tier_auto_resize == null
    error_message = "enable_hot_tier_auto_resize is supported only for FLEX storage pools."
  }
}

variable "scale_type" {
  description = "Scale type of the storage pool. Flex-only. Use SCALE_TYPE_SCALEOUT for large capacity pools."
  type        = string
  default     = null
  validation {
    condition     = var.scale_type == null ? true : contains(["SCALE_TYPE_DEFAULT", "SCALE_TYPE_SCALEOUT"], var.scale_type)
    error_message = "Allowed values for scale_type are SCALE_TYPE_DEFAULT or SCALE_TYPE_SCALEOUT."
  }
  validation {
    condition     = var.scale_type != "SCALE_TYPE_SCALEOUT" || var.replica_zone == null
    error_message = "Large capacity pools (SCALE_TYPE_SCALEOUT) must be zonal Flex Unified pools. Set zone and omit replica_zone."
  }
}

variable "total_throughput_mibps" {
  description = <<-EOT
    Total pool throughput in MiB/s for Flex Unified custom performance. Omit to use the default 64 MiB/s.
  EOT
  type        = number
  default     = null
  validation {
    condition     = var.total_throughput_mibps == null || var.total_throughput_mibps > 0
    error_message = "total_throughput_mibps must be greater than 0."
  }
}

variable "total_iops" {
  description = <<-EOT
    Total pool IOPS for Flex Unified custom performance. Omit to let Google Cloud calculate IOPS from total_throughput_mibps.
  EOT
  type        = number
  default     = null
  validation {
    condition     = var.total_iops == null || var.total_iops > 0
    error_message = "total_iops must be greater than 0."
  }
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
