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
  autoname      = replace(var.deployment_name, "_", "-")
  network_name  = var.network_name == null ? "${local.autoname}-net" : var.network_name
  subnet_prefix = var.subnetworks_template.name_prefix == null ? "${local.autoname}-subnet" : var.subnetworks_template.name_prefix

  new_bits = ceil(log(var.subnetworks_template.count, 2))
  template_subnetworks = [for i in range(var.subnetworks_template.count) :
    {
      subnet_name   = "${local.subnet_prefix}-${i}"
      subnet_region = try(var.subnetworks_template.region, var.region)
      subnet_ip     = cidrsubnet(var.subnetworks_template.ip_range, local.new_bits, i)
    }
  ]

  firewall_log_api_values = {
    "DISABLE_LOGGING"      = null
    "INCLUDE_ALL_METADATA" = { metadata = "INCLUDE_ALL_METADATA" },
    "EXCLUDE_ALL_METADATA" = { metadata = "EXCLUDE_ALL_METADATA" },
  }
  firewall_log_config = lookup(local.firewall_log_api_values, var.firewall_log_config, null)

  allow_internal_traffic = {
    name                    = "${local.network_name}-fw-allow-internal-traffic"
    priority                = null
    description             = "allow traffic between nodes of this VPC"
    direction               = "INGRESS"
    ranges                  = [var.subnetworks_template.ip_range]
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
    deny       = []
    log_config = local.firewall_log_config
  }

  firewall_rules = concat(
    var.firewall_rules,
    var.enable_internal_traffic ? [local.allow_internal_traffic] : [],
  )

  output_subnets = [
    for subnet in module.vpc.subnets : {
      network            = null
      subnetwork         = subnet.self_link
      subnetwork_project = null # will populate from subnetwork_self_link
      network_ip         = null
      nic_type           = var.nic_type
      stack_type         = null
      queue_count        = null
      access_config      = []
      ipv6_access_config = []
      alias_ip_range     = []
    }
  ]

  output_subnets_gke = [
    for i in range(length(module.vpc.subnets)) : {
      network            = local.network_name
      subnetwork         = local.template_subnetworks[i].subnet_name
      subnetwork_project = var.project_id
      network_ip         = null
      nic_type           = var.nic_type
      stack_type         = null
      queue_count        = null
      access_config      = []
      ipv6_access_config = []
      alias_ip_range     = []
    }
  ]
}

module "vpc" {
  source  = "terraform-google-modules/network/google"
  version = "~> 10.0"

  network_name                           = local.network_name
  project_id                             = var.project_id
  auto_create_subnetworks                = false
  subnets                                = local.template_subnetworks
  routing_mode                           = var.network_routing_mode
  mtu                                    = var.mtu
  description                            = var.network_description
  shared_vpc_host                        = var.shared_vpc_host
  delete_default_internet_gateway_routes = var.delete_default_internet_gateway_routes
  firewall_rules                         = local.firewall_rules
  network_profile                        = var.network_profile
}
