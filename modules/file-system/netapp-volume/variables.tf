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
  description = "ID of project in which the NetApp volume will be created."
  type        = string
}

variable "netapp_storage_pool_id" {
  description = <<-EOT
    The ID of the NetApp storage pool to use for the volume. Volume location (region or zone) is parsed from this value.
    Inherited from the pool when using use: [netapp_pool].
    EOT
  type        = string
  validation {
    condition     = length(split("/", var.netapp_storage_pool_id)) == 6
    error_message = "The storage pool id must be provided in the following format: projects/<project_id>/locations/<location>/storagePools/<pool_name>."
  }
}

variable "service_level" {
  description = "Service level of the storage pool used by this volume. Inherited from the pool when using use: [netapp_pool]."
  type        = string
  default     = null
}

variable "type" {
  description = "Type of the storage pool used by this volume. Inherited from the pool when using use: [netapp_pool]. Flex Unified pools use UNIFIED. Null for STANDARD, PREMIUM, and EXTREME pools."
  type        = string
  default     = null
}

variable "allow_auto_tiering" {
  description = "Whether the storage pool supports auto-tiering. Inherited from the pool when using use: [netapp_pool]."
  type        = bool
  default     = null
}

variable "scale_type" {
  description = "Scale type of the storage pool. Inherited from the pool when using use: [netapp_pool]. Flex-only; null for STANDARD, PREMIUM, and EXTREME pools."
  type        = string
  default     = null
}

variable "volume_name" {
  description = <<-EOT
    The name of the volume. Needs to be unique within the storage pool.
    FLEX pools: lowercase letters, numbers, and underscores only; must start with a lowercase letter and cannot end with an underscore.
    STANDARD, PREMIUM, and EXTREME pools: hyphens are allowed; underscores are not allowed.
    EOT
  type        = string
}

variable "capacity_gib" {
  description = "The capacity of the volume in GiB. Minimum is 100 GiB for STANDARD, PREMIUM, and EXTREME; 15 TiB (15360 GiB) for STANDARD, PREMIUM, and EXTREME large capacity volumes; 1 GiB for Flex Unified; 4800 GiB for Flex Unified large capacity volumes."
  type        = number
  default     = 1024
}

variable "protocols" {
  description = "Access protocols for the volume. Only NFSv3 and NFSv4.1 (NFSV4) are supported."
  type        = list(string)
  default     = ["NFSV3"]
  validation {
    condition     = alltrue([for p in var.protocols : contains(["NFSV3", "NFSV4"], p)])
    error_message = "Allowed values for protocols are 'NFSV3' or 'NFSV4'."
  }
}

variable "description" {
  description = "A description of the NetApp volume."
  type        = string
  default     = ""
  validation {
    condition     = length(var.description) <= 2048
    error_message = "NetApp volume description must be 2048 characters or fewer"
  }
}

variable "labels" {
  description = "Labels to add to the NetApp volume. Key-value pairs."
  type        = map(string)
}

variable "local_mount" {
  description = "Mountpoint for this volume."
  type        = string
  default     = "/shared"
}

variable "mount_options" {
  description = "NFS mount options to mount file system."
  type        = string
  default     = "rw,hard,rsize=262144,wsize=262144,tcp"
}

variable "large_capacity" {
  description = <<-EOT
    If true, the volume will be created with large capacity for STANDARD/PREMIUM/EXTREME service levels.
    For FLEX service level, use large_capacity_config instead.
    EOT
  type        = bool
  default     = false
}

variable "large_capacity_config" {
  description = <<-EOT
    Configuration for a Flex Unified large capacity volume. Supported only for Flex Unified pools.
    Set constituent_count in the blueprint. The typical value for current SCALE_TYPE_SCALEOUT pools is 48.
    EOT
  type = object({
    constituent_count = number
  })
  default = null
  validation {
    condition     = var.large_capacity_config == null || var.large_capacity_config.constituent_count >= 2
    error_message = "constituent_count must be at least 2 for Flex Unified large capacity volumes."
  }
}

variable "unix_permissions" {
  description = "UNIX permissions for root inode in the volume."
  type        = string
  default     = "0770"
  validation {
    condition     = can(regex("^[0-7]{3,4}$", var.unix_permissions))
    error_message = "UNIX permissions must be a 3 or 4-digit octal number (digits 0-7)."
  }
}

variable "tiering_policy" {
  description = <<-EOT
    Define the tiering policy for the NetApp volume. Requires a pool with allow_auto_tiering enabled.
    hot_tier_bypass_mode_enabled (FLEX only): use during data migration so writes go to the cold tier instead of filling the hot tier; disable after migration completes.
    EOT
  type = object({
    tier_action                  = optional(string)
    cooling_threshold_days       = optional(number)
    hot_tier_bypass_mode_enabled = optional(bool)
  })
  default = null
  validation {
    condition = var.tiering_policy == null || contains(
      ["ENABLED", "PAUSED"],
      coalesce(var.tiering_policy.tier_action, "PAUSED")
    )
    error_message = "tier_action must be ENABLED or PAUSED."
  }
  validation {
    condition = var.tiering_policy == null || var.tiering_policy.cooling_threshold_days == null || (
      coalesce(var.tiering_policy.cooling_threshold_days, 0) >= 2 &&
      coalesce(var.tiering_policy.cooling_threshold_days, 0) <= 183
    )
    error_message = "cooling_threshold_days must be between 2 and 183."
  }
}

variable "deletion_policy" {
  description = <<-EOT
    Controls Terraform destroy behavior. Omit to use the provider default (delete the volume in Google Cloud).
    DEFAULT or DELETE: delete the volume in Google Cloud.
    FORCE: delete the volume even when nested snapshot resources exist.
    PREVENT: block Terraform from deleting the volume.
    ABANDON: remove the volume from Terraform state without deleting it in Google Cloud.
    EOT
  type        = string
  default     = null
  validation {
    condition     = var.deletion_policy == null ? true : contains(["DEFAULT", "FORCE", "PREVENT", "ABANDON", "DELETE"], var.deletion_policy)
    error_message = "Allowed values for deletion_policy are DEFAULT, FORCE, PREVENT, ABANDON, or DELETE."
  }
}

variable "export_policy_rules" {
  description = "Define NFS export policy."
  type = list(object({
    allowed_clients = optional(string)
    has_root_access = optional(bool, false)
    access_type     = optional(string, "READ_WRITE")
    nfsv3           = optional(bool)
    nfsv4           = optional(bool)
  }))
  # Permissive default if user does not specify nfs_export_options. Allow all RFC1918 CIDRS with no_root_squash
  default = [{
    allowed_clients = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16",
    has_root_access = true,
    access_type     = "READ_WRITE",
  }]
  nullable = true
  validation {
    condition = var.export_policy_rules == null ? true : alltrue([
      for rule in var.export_policy_rules : contains(["READ_ONLY", "READ_WRITE", "READ_NONE"], rule.access_type)
    ])
    error_message = "access_type must be READ_ONLY, READ_WRITE, or READ_NONE."
  }
}
