/**
 * Copyright 2025 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "private-service-access", ghpc_role = "network" })
}

locals {
  split_network_id = split("/", var.network_id)
  network_name     = local.split_network_id[4]
  network_project  = local.split_network_id[1]
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_compute_global_address" "private_ip_alloc" {
  provider      = google
  name          = "global-psconnect-ip-${random_id.resource_name_suffix.hex}"
  project       = var.project_id
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  network       = var.network_id
  prefix_length = var.prefix_length
  labels        = local.labels
  address       = var.address
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = var.network_id
  service                 = var.service_name
  reserved_peering_ranges = [google_compute_global_address.private_ip_alloc.name]
  deletion_policy         = var.deletion_policy
  update_on_creation_fail = var.deletion_policy == "ABANDON" ? true : null
}

# Google Cloud NetApp Volumes need enablement of custom_route import and export
resource "google_compute_network_peering_routes_config" "private_vpc_peering_routes_gcnv" {
  count   = var.service_name == "netapp.servicenetworking.goog" ? 1 : 0
  project = local.network_project
  network = local.network_name
  peering = google_service_networking_connection.private_vpc_connection.peering

  export_custom_routes = true
  import_custom_routes = true
}
