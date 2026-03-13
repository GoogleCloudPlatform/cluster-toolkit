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

# resource "random_id" "resource_name_suffix" {
#   byte_length = 4
# }

locals {
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

  split_pool_id = split("/", var.netapp_storage_pool_id)
  pool_name     = local.split_pool_id[5]
}

resource "google_netapp_volume" "netapp_volume" {
  project = var.project_id

  name               = var.volume_name
  share_name         = var.volume_name
  location           = var.region
  protocols          = var.protocols
  capacity_gib       = var.capacity_gib
  large_capacity     = var.large_capacity
  multiple_endpoints = var.large_capacity == true ? true : null
  storage_pool       = local.pool_name
  unix_permissions   = var.unix_permissions

  dynamic "tiering_policy" {
    for_each = var.tiering_policy == null ? [] : [0]
    content {
      cooling_threshold_days = lookup(var.tiering_policy, "cooling_threshold_days", null)
      tier_action            = lookup(var.tiering_policy, "tier_action", null)
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
}
