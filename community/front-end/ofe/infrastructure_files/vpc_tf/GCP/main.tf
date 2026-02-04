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


resource "random_pet" "vpc_name" {
  length    = 2
  separator = "-"
  keepers   = {}
}

locals {
  vpc_key = random_pet.vpc_name.id
}

# VPC Creation
resource "google_compute_network" "network" {
  name                    = "${local.vpc_key}-network"
  project                 = var.project
  auto_create_subnetworks = false
}


resource "google_compute_router" "network_router" {
  name    = "${local.vpc_key}-compute-router"
  region  = var.region
  project = var.project
  network = google_compute_network.network.name
}

resource "google_compute_router_nat" "network_nat" {
  name                               = "${local.vpc_key}-nat"
  project                            = var.project
  router                             = google_compute_router.network_router.name
  region                             = google_compute_router.network_router.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}



# Firewalls - allow all internal, allow ssh from external

resource "google_compute_firewall" "firewall_allow_ssh" {
  name          = "${local.vpc_key}-firewall-ssh"
  network       = google_compute_network.network.name
  project       = var.project
  source_ranges = ["0.0.0.0/0"]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}


resource "google_compute_firewall" "firewall_internal" {
  name          = "${local.vpc_key}-firewall-internal"
  network       = google_compute_network.network.name
  project       = var.project
  source_ranges = ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"]
  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "udp"
    ports    = ["0-65535"]
  }
  allow { protocol = "icmp" }
}

locals {
  # This label allows for billing report tracking based on module.
  labels = {
    created_by = "ofe"
  }
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_compute_global_address" "private_ip_alloc" {
  provider      = google-beta
  project       = var.project
  name          = "global-psconnect-ip-${random_id.resource_name_suffix.hex}"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  network       = google_compute_network.network.self_link
  prefix_length = 16
  labels        = local.labels
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.network.self_link
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_alloc.name]
}

output "vpc_id" {
  value       = google_compute_network.network.name
  description = "Name of the created VPC"
}
