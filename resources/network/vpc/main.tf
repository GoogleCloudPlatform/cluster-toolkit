/**
 * Copyright 2021 Google LLC
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
  project_id   = var.project_id
  network_name = var.network_name == null ? "${var.deployment_name}-net" : var.network_name
  region       = var.region
}

module "vpc" {
  source                  = "terraform-google-modules/network/google"
  version                 = "~> 3.0"
  network_name            = local.network_name
  project_id              = local.project_id
  auto_create_subnetworks = true
  subnets                 = []
}

module "firewall_rules" {
  source       = "terraform-google-modules/network/google//modules/firewall-rules"
  version      = "~> 3.0"
  project_id   = var.project_id
  network_name = module.vpc.network_name

  rules = [
    {
      name                    = "${local.network_name}-allow-iap-ssh-ingress"
      description             = "allow console SSH access"
      direction               = "INGRESS"
      priority                = null
      ranges                  = ["35.235.240.0/20"]
      source_tags             = null
      source_service_accounts = null
      target_tags             = null
      target_service_accounts = null
      allow = [{
        protocol = "tcp"
        ports    = ["22"]
      }]
      deny = []
      log_config = {
        metadata = "INCLUDE_ALL_METADATA"
      }
      }, {
      name                    = "${local.network_name}-allow-internal-traffic"
      priority                = null
      description             = "allow traffic between nodes of this VPC"
      direction               = "INGRESS"
      ranges                  = ["10.0.0.0/8"]
      source_tags             = null
      source_service_accounts = null
      target_tags             = null
      target_service_accounts = null
      allow = [{
        protocol = "tcp"
        ports    = ["0-65535"]
        }, {
        protocol = "udp"
        ports    = ["0-65535"]
        }, {
        protocol = "icmp"
        ports    = null
        },
      ]
      deny = []
      log_config = {
        metadata = "INCLUDE_ALL_METADATA"
      }
  }]
}

module "cloud_router" {
  source  = "terraform-google-modules/cloud-router/google"
  version = "~> 0.4"

  name    = "${local.network_name}-router"
  project = local.project_id
  region  = local.region
  network = module.vpc.network_name
}

resource "google_compute_address" "nat_ips" {
  count  = 2
  name   = "${local.network_name}-nat-ips-${count.index}"
  region = local.region
}

module "cloud_nat" {
  source     = "terraform-google-modules/cloud-nat/google"
  version    = "~> 1.4"
  project_id = local.project_id
  region     = local.region
  nat_ips    = google_compute_address.nat_ips.*.self_link
  router     = module.cloud_router.router.name
}

data "google_compute_subnetwork" "primary_subnetwork" {
  depends_on = [
    module.vpc
  ]
  name    = local.network_name
  region  = local.region
  project = local.project_id
}
