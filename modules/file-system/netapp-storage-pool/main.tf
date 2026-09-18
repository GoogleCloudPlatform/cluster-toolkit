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
  labels = merge(var.labels, { ghpc_module = "netapp-storage-pool", ghpc_role = "file-system" })
}

locals {
  is_flex_zonal    = var.service_level == "FLEX" && var.zone != null && var.replica_zone == null
  is_flex_regional = var.service_level == "FLEX" && var.zone != null && var.replica_zone != null
  pool_location    = local.is_flex_zonal ? var.zone : var.region
  min_capacity_gib = var.scale_type == "SCALE_TYPE_SCALEOUT" ? 6144 : (
    var.service_level == "FLEX" ? 1024 : 2048
  )
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

data "google_compute_network_peering" "private_peering" {
  name    = var.private_vpc_connection_peering
  network = var.network_self_link
}

resource "google_netapp_storage_pool" "netapp_storage_pool" {
  project = var.project_id

  name          = var.pool_name != null ? var.pool_name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}"
  location      = local.pool_location
  network       = var.network_id
  service_level = var.service_level
  capacity_gib  = var.capacity_gib

  active_directory   = var.active_directory_policy
  kms_config         = var.cmek_policy
  ldap_enabled       = var.ldap_enabled
  allow_auto_tiering = var.allow_auto_tiering
  mode               = var.service_level == "FLEX" ? "DEFAULT" : null
  type               = var.service_level == "FLEX" ? "UNIFIED" : null
  # zone and replica_zone are Flex regional-only API fields. Omit for all other pool types.
  zone                        = local.is_flex_regional ? var.zone : null
  replica_zone                = local.is_flex_regional ? var.replica_zone : null
  hot_tier_size_gib           = var.service_level == "FLEX" ? var.hot_tier_size_gib : null
  enable_hot_tier_auto_resize = var.service_level == "FLEX" ? var.enable_hot_tier_auto_resize : null
  scale_type                  = var.service_level == "FLEX" ? var.scale_type : null
  total_throughput_mibps      = var.service_level == "FLEX" ? var.total_throughput_mibps : null
  total_iops                  = var.service_level == "FLEX" ? var.total_iops : null

  description = var.description
  labels      = local.labels

  depends_on = [data.google_compute_network_peering.private_peering]

  lifecycle {
    # API defaults for mode, type, and scale_type differ from omitted (null) config on
    # STANDARD, PREMIUM, and EXTREME pools. Ignore drift so Terraform does not send them.
    ignore_changes = [
      mode,
      type,
      scale_type,
    ]
    precondition {
      condition     = data.google_compute_network_peering.private_peering.state == "ACTIVE"
      error_message = "The network for the storage pool must have private service access."
    }
    precondition {
      condition     = var.service_level != "FLEX" || var.zone != null
      error_message = "FLEX pools require zone. For zonal pools set zone only. For regional pools set zone and replica_zone."
    }
    precondition {
      condition     = var.service_level != "FLEX" || var.replica_zone == null || var.zone != var.replica_zone
      error_message = "zone and replica_zone must be different for regional FLEX pools."
    }
    precondition {
      condition     = var.scale_type != "SCALE_TYPE_SCALEOUT" || (var.service_level == "FLEX" && var.replica_zone == null)
      error_message = "Large capacity pools (SCALE_TYPE_SCALEOUT) must be zonal Flex Unified pools. Set zone and omit replica_zone."
    }
    precondition {
      condition     = var.service_level == "FLEX" || var.scale_type == null
      error_message = "scale_type is supported only for FLEX storage pools."
    }
    precondition {
      condition     = var.service_level == "FLEX" || (var.total_throughput_mibps == null && var.total_iops == null)
      error_message = "total_throughput_mibps and total_iops are supported only for FLEX storage pools."
    }
    precondition {
      condition = var.capacity_gib >= local.min_capacity_gib
      error_message = var.scale_type == "SCALE_TYPE_SCALEOUT" ? "Large capacity pools (SCALE_TYPE_SCALEOUT) require at least 6144 GiB." : (
        var.service_level == "FLEX" ? "Flex Unified pools require at least 1024 GiB." : "STANDARD, PREMIUM, and EXTREME pools require at least 2048 GiB."
      )
    }
  }
}
