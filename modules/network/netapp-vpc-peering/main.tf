/**
 * Copyright 2024 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "netapp-vpc-peering", ghpc_role = "network" })
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_compute_global_address" "private_ip_alloc" {
  name          = "global-psconnect-ip-${random_id.resource_name_suffix.hex}"
  project       = var.project_id
  purpose       = var.vpc_peering_purpose
  address_type  = var.vpc_peering_address_type
  network       = var.network_name
  prefix_length = var.vpc_peering_address_prefix
  labels        = local.labels
  address       = var.vpc_address_ip_range
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = var.network_name
  service                 = var.vpc_connection_peering_service
  reserved_peering_ranges = [google_compute_global_address.private_ip_alloc.name]
  update_on_creation_fail = true
}

resource "google_compute_global_address" "netapp_private_svc_ip" {
  project       = var.project_id
  name          = "netapp-psa-ip-${random_id.resource_name_suffix.hex}"
  address_type  = var.vpc_peering_address_type
  purpose       = var.vpc_peering_purpose
  ip_version    = var.vpc_peering_ip_version
  address       = var.vpc_peering_address_ip_range
  prefix_length = var.vpc_peering_address_prefix
  network       = var.network_name
  labels        = local.labels
}

resource "google_service_networking_connection" "netapp_vpc_connection" {
  network = var.network_name
  service = var.netapp_connection_peering_service
  reserved_peering_ranges = [
    google_compute_global_address.netapp_private_svc_ip.name,
  ]
  depends_on = [
    google_service_networking_connection.private_vpc_connection
  ]
  deletion_policy = "ABANDON"
  update_on_creation_fail = true
}
