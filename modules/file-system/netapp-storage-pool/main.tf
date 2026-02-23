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
  location      = var.region
  network       = var.network_id
  service_level = var.service_level
  capacity_gib  = var.capacity_gib

  active_directory   = var.active_directory_policy
  kms_config         = var.cmek_policy
  ldap_enabled       = var.ldap_enabled
  allow_auto_tiering = var.allow_auto_tiering

  description = var.description
  labels      = local.labels

  depends_on = [data.google_compute_network_peering.private_peering]

  lifecycle {
    precondition {
      condition     = data.google_compute_network_peering.private_peering.state == "ACTIVE"
      error_message = "The network for the storage pool must have private service access."
    }
  }
}
