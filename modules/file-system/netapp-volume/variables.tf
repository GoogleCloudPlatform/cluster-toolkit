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

variable "netapp_storage_pool_id" {
  description = "The ID of the NetApp storage pool to use for the volume."
  type        = string
  validation {
    condition     = length(split("/", var.netapp_storage_pool_id)) == 6
    error_message = "The storage pool id must be provided in the following format: projects/<project_id>/locations/<location>/storagePools/<pool_name>."
  }
}

variable "region" {
  description = "Location for NetApp storage pool."
  type        = string
}

variable "volume_name" {
  description = "The name of the volume. Needs to be unique within the storage pool."
  type        = string
  default     = null
}

variable "capacity_gib" {
  description = "The capacity of the volume in GiB."
  type        = number
  default     = 1024
  validation {
    condition     = var.capacity_gib >= 100
    error_message = "The minimum capacity for the volume is 100 GiB."
  }
}

variable "protocols" {
  description = "The protocols that the volume supports. Currently, only NFSv3 and NFSv4 is supported."
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
  default     = "rw,hard,rsize=65536,wsize=65536,tcp"
}

variable "large_capacity" {
  description = <<-EOT
    If true, the volume will be created with large capacity.
    Large capacity volumes have 6 IP addresses and a minimal size of 15 TiB.
    EOT
  type        = bool
  default     = false
}

variable "unix_permissions" {
  description = "UNIX permissions for root inode in the volume."
  type        = string
  default     = "0777"
  validation {
    condition     = length(var.unix_permissions) <= 4
    error_message = "UNIX permissions must be a 4-digit octal number."
  }
}

variable "tiering_policy" {
  description = "Define the tiering policy for the NetApp volume."
  type = object({
    tier_action            = optional(string)
    cooling_threshold_days = optional(number)
  })
  default = null
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
}
