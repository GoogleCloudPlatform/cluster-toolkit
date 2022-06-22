/**
 * Copyright 2022 Google LLC
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
  network_name                = var.network_name == null ? "${var.deployment_name}-net" : var.network_name
  subnetwork_name             = var.subnetwork_name == null ? "${var.deployment_name}-primary-subnet" : var.subnetwork_name
  primary_subnetwork_new_bits = try(var.primary_subnetwork.new_bits, var.subnetwork_size)
  cidr_blocks                 = cidrsubnets(var.network_address_range, local.primary_subnetwork_new_bits, var.additional_subnetworks[*].new_bits...)

  regions = distinct([for subnet in local.all_subnets : subnet.subnet_region])

  default_primary_subnet = {
    subnet_name           = local.subnetwork_name
    subnet_ip             = local.cidr_blocks[0]
    subnet_region         = var.region
    subnet_private_access = true
    subnet_flow_logs      = false
    description           = "primary subnetwork in ${local.network_name}"
    purpose               = null
    role                  = null
  }

  primary_subnet = var.primary_subnetwork == null ? local.default_primary_subnet : var.primary_subnetwork

  additional_subnets = [for index, subnet in var.additional_subnetworks :
    {
      subnet_name           = subnet.subnet_name
      subnet_ip             = local.cidr_blocks[index + 1]
      subnet_region         = subnet.subnet_region
      subnet_private_access = lookup(subnet, "subnet_private_access", true)
      subnet_flow_logs      = lookup(subnet, "subnet_flow_logs", false)
      description           = lookup(subnet, "description", null)
      purpose               = lookup(subnet, "purpose", null)
      role                  = lookup(subnet, "role", null)
    }
  ]

  all_subnets = concat([local.primary_subnet], local.additional_subnets)

  # this comprehension is guaranteed to have 1 and only 1 match
  primary_subnetwork               = one([for k, v in module.vpc.subnets : v if k == "${local.primary_subnet.subnet_region}/${local.primary_subnet.subnet_name}"])
  primary_subnetwork_name          = local.primary_subnetwork.name
  primary_subnetwork_self_link     = local.primary_subnetwork.self_link
  primary_subnetwork_ip_cidr_range = local.primary_subnetwork.ip_cidr_range
}

module "vpc" {
  source  = "terraform-google-modules/network/google"
  version = "~> 5.0"

  network_name                           = local.network_name
  project_id                             = var.project_id
  auto_create_subnetworks                = false
  subnets                                = local.all_subnets
  routing_mode                           = var.network_routing_mode
  mtu                                    = var.mtu
  description                            = var.network_description
  shared_vpc_host                        = var.shared_vpc_host
  delete_default_internet_gateway_routes = var.delete_default_internet_gateway_routes
}

module "firewall_rules" {
  source       = "terraform-google-modules/network/google//modules/firewall-rules"
  version      = "~> 5.0"
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
      ranges                  = [var.network_address_range]
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

# This use of the module may appear odd when var.ips_per_nat = 0. The module
# will be called for all regions with subnetworks but names will be set to the
# empty list. This is a perfectly valid value (the default!). In this scenario,
# no IP addresses are created and all module outputs are empty lists.
#
# https://github.com/terraform-google-modules/terraform-google-address/blob/v3.1.1/variables.tf#L27
# https://github.com/terraform-google-modules/terraform-google-address/blob/v3.1.1/outputs.tf
module "nat_ip_addresses" {
  source  = "terraform-google-modules/address/google"
  version = "~> 3.1"

  for_each = toset(local.regions)

  project_id = var.project_id
  region     = each.value
  # an external, regional (not global) IP address is suited for a regional NAT
  address_type = "EXTERNAL"
  global       = false
  names        = [for idx in range(var.ips_per_nat) : "${local.network_name}-nat-ips-${each.value}-${idx}"]
}

module "cloud_router" {
  source  = "terraform-google-modules/cloud-router/google"
  version = "~> 1.3"

  for_each = toset(local.regions)

  project = var.project_id
  name    = "${local.network_name}-router"
  region  = each.value
  network = module.vpc.network_name
  # in scenario with no NAT IPs, no NAT is created even if router is created
  # https://github.com/terraform-google-modules/terraform-google-cloud-router/blob/v1.3.0/nat.tf#L18-L20
  nats = length(module.nat_ip_addresses[each.value].self_links) == 0 ? [] : [
    {
      name : "cloud-nat-${each.value}",
      nat_ips : module.nat_ip_addresses[each.value].self_links
    },
  ]
}
