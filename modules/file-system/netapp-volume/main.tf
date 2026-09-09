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

locals {
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "netapp-volume", ghpc_role = "file-system" })
}

locals {
  split_pool_id = split("/", var.netapp_storage_pool_id)
  pool_name     = local.split_pool_id[5]
  pool_location = local.split_pool_id[3]

  # Inherited pool attributes are Flex-only; ignore API defaults on STANDARD/PREMIUM/EXTREME pools.
  pool_type       = var.service_level == "FLEX" ? var.type : null
  pool_scale_type = var.service_level == "FLEX" ? var.scale_type : null

  # Flex Unified when service level is FLEX and pool type is UNIFIED or unset (inherited from pool).
  is_flex_unified                 = var.service_level == "FLEX" && (local.pool_type == null || local.pool_type == "UNIFIED")
  non_flex_large_min_capacity_gib = 15360 # 15 TiB
  min_capacity_gib = var.large_capacity_config != null ? 4800 : (
    local.is_flex_unified ? 1 : (
      var.large_capacity ? local.non_flex_large_min_capacity_gib : 100
    )
  )

  full_path    = split(":", google_netapp_volume.netapp_volume.mount_options[0].export_full)
  server_ip    = local.full_path[0]
  remote_mount = local.full_path[1]
  # Large volumes will have 6 IPs
  server_ips    = [for ip in google_netapp_volume.netapp_volume.mount_options[*].export_full : split(":", ip)[0]]
  fs_type       = "nfs"
  mount_options = var.mount_options

  install_nfs_client_runner = {
    "type"        = "shell"
    "source"      = "${path.module}/scripts/install-nfs-client.sh"
    "destination" = "install-nfs${replace(var.local_mount, "/", "_")}.sh"
  }
  mount_runner = {
    "type"        = "shell"
    "source"      = "${path.module}/scripts/mount.sh"
    "args"        = "\"${join(",", local.server_ips)}\" \"${local.remote_mount}\" \"${var.local_mount}\" \"${local.fs_type}\" \"${local.mount_options}\""
    "destination" = "mount${replace(var.local_mount, "/", "_")}.sh"
  }
}

resource "google_netapp_volume" "netapp_volume" {
  project = var.project_id

  name               = var.volume_name
  share_name         = var.volume_name
  location           = local.pool_location
  protocols          = var.protocols
  capacity_gib       = var.capacity_gib
  large_capacity     = local.is_flex_unified ? null : var.large_capacity
  multiple_endpoints = local.is_flex_unified ? null : (var.large_capacity ? true : null)
  storage_pool       = local.pool_name
  unix_permissions   = var.unix_permissions
  security_style     = "UNIX"
  deletion_policy    = var.deletion_policy

  dynamic "large_capacity_config" {
    for_each = var.large_capacity_config != null ? [1] : []
    content {
      constituent_count = var.large_capacity_config.constituent_count
    }
  }

  dynamic "tiering_policy" {
    for_each = var.tiering_policy == null ? [] : [0]
    content {
      cooling_threshold_days       = try(var.tiering_policy.cooling_threshold_days, null)
      tier_action                  = try(var.tiering_policy.tier_action, null)
      hot_tier_bypass_mode_enabled = try(var.tiering_policy.hot_tier_bypass_mode_enabled, null)
    }
  }

  description = var.description
  labels      = local.labels

  dynamic "export_policy" {
    for_each = var.export_policy_rules == null ? [] : [0]
    content {
      dynamic "rules" {
        for_each = var.export_policy_rules
        content {
          access_type     = rules.value.access_type
          allowed_clients = rules.value.allowed_clients
          has_root_access = rules.value.has_root_access
          nfsv3           = rules.value.nfsv3 == null ? contains([for p in var.protocols : lower(p)], "nfsv3") : rules.value.nfsv3
          nfsv4           = rules.value.nfsv4 == null ? contains([for p in var.protocols : lower(p)], "nfsv4") : rules.value.nfsv4
        }
      }
    }
  }

  depends_on = [var.netapp_storage_pool_id]

  lifecycle {
    precondition {
      condition     = local.pool_scale_type != "SCALE_TYPE_SCALEOUT" || var.large_capacity_config != null
      error_message = "Large capacity pools (SCALE_TYPE_SCALEOUT) require large_capacity_config on the volume."
    }
    precondition {
      condition     = var.large_capacity_config == null || local.is_flex_unified
      error_message = "large_capacity_config is supported only with Flex Unified pools."
    }
    precondition {
      condition     = !local.is_flex_unified || !var.large_capacity
      error_message = "For Flex Unified pools, use large_capacity_config instead of large_capacity."
    }
    precondition {
      condition     = !(var.large_capacity && var.large_capacity_config != null)
      error_message = "large_capacity and large_capacity_config cannot be used together."
    }
    precondition {
      condition     = local.pool_scale_type == null || var.large_capacity_config == null || local.pool_scale_type == "SCALE_TYPE_SCALEOUT"
      error_message = "Flex Unified large capacity volumes require a storage pool with scale_type SCALE_TYPE_SCALEOUT."
    }
    precondition {
      condition = var.capacity_gib >= local.min_capacity_gib
      error_message = var.large_capacity_config != null ? "Flex Unified large capacity volumes require at least 4800 GiB." : (
        local.is_flex_unified ? "Flex Unified volumes require at least 1 GiB." : (
          var.large_capacity ? "STANDARD, PREMIUM, and EXTREME large capacity volumes require at least 15 TiB (15360 GiB)." : "STANDARD, PREMIUM, and EXTREME volumes require at least 100 GiB."
        )
      )
    }
    precondition {
      condition     = var.tiering_policy == null || try(var.tiering_policy.hot_tier_bypass_mode_enabled, null) != true || var.service_level == "FLEX"
      error_message = "hot_tier_bypass_mode_enabled is supported only for FLEX service level."
    }
    precondition {
      condition     = var.tiering_policy == null || var.allow_auto_tiering == null || var.allow_auto_tiering
      error_message = "tiering_policy requires a storage pool with allow_auto_tiering enabled."
    }
    precondition {
      condition     = var.service_level != "FLEX" || can(regex("^[a-z]([a-z0-9_]*[a-z0-9])?$", var.volume_name))
      error_message = "Flex volume names must start with a lowercase letter, contain only lowercase letters, numbers, and underscores, and cannot end with an underscore."
    }
    precondition {
      condition     = var.service_level == "FLEX" || var.service_level == null || can(regex("^[a-z]([a-z0-9-]*[a-z0-9])?$", var.volume_name))
      error_message = "STANDARD, PREMIUM, and EXTREME volume names must start with a lowercase letter, contain only lowercase letters, numbers, and hyphens, and cannot end with a hyphen."
    }
    precondition {
      condition     = var.service_level != "FLEX" || local.pool_type != "FILE"
      error_message = "Flex File pools (FLEX service level with type FILE) are not supported by this module."
    }
  }
}
