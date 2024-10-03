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
  autoname     = replace(var.deployment_name, "_", "-")
  network_name = var.network_name == null ? "${local.autoname}-net" : var.network_name

  new_bits = ceil(log(var.subnetworks_template.count, 2))
  template_subnetworks = [for i in range(var.subnetworks_template.count) :
    {
      subnet_name           = "${var.subnetworks_template.name_prefix}-${i}"
      subnet_region         = try(var.subnetworks_template.region, var.region)
      subnet_ip             = cidrsubnet(var.subnetworks_template.ip_range, local.new_bits, i)
      subnet_private_access = coalesce(var.subnetworks_template.private_access, false)
    }
  ]

  iap_ports = distinct(concat(compact([
    var.enable_iap_rdp_ingress ? "3389" : "",
    var.enable_iap_ssh_ingress ? "22" : "",
    var.enable_iap_winrm_ingress ? "5986" : "",
  ]), var.extra_iap_ports))

  firewall_log_api_values = {
    "DISABLE_LOGGING"      = null
    "INCLUDE_ALL_METADATA" = { metadata = "INCLUDE_ALL_METADATA" },
    "EXCLUDE_ALL_METADATA" = { metadata = "EXCLUDE_ALL_METADATA" },
  }
  firewall_log_config = lookup(local.firewall_log_api_values, var.firewall_log_config, null)

  allow_iap_ingress = {
    name                    = "${local.network_name}-fw-allow-iap-ingress"
    description             = "allow TCP access via Identity-Aware Proxy"
    direction               = "INGRESS"
    priority                = null
    ranges                  = ["35.235.240.0/20"]
    source_tags             = null
    source_service_accounts = null
    target_tags             = null
    target_service_accounts = null
    allow = [{
      protocol = "tcp"
      ports    = local.iap_ports
    }]
    deny       = []
    log_config = local.firewall_log_config
  }

  allow_ssh_ingress = {
    name                    = "${local.network_name}-fw-allow-ssh-ingress"
    description             = "allow SSH access"
    direction               = "INGRESS"
    priority                = null
    ranges                  = var.allowed_ssh_ip_ranges
    source_tags             = null
    source_service_accounts = null
    target_tags             = null
    target_service_accounts = null
    allow = [{
      protocol = "tcp"
      ports    = ["22"]
    }]
    deny       = []
    log_config = local.firewall_log_config
  }

  allow_internal_traffic = {
    name                    = "${local.network_name}-fw-allow-internal-traffic"
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
    deny       = []
    log_config = local.firewall_log_config
  }

  firewall_rules = concat(
    var.firewall_rules,
    length(var.allowed_ssh_ip_ranges) > 0 ? [local.allow_ssh_ingress] : [],
    var.enable_internal_traffic ? [local.allow_internal_traffic] : [],
    length(local.iap_ports) > 0 ? [local.allow_iap_ingress] : []
  )

  url_parts    = split("/", var.network_profile)
  profile_name = upper(element(local.url_parts, length(local.url_parts) - 1))
  output_subnets = [
    for subnet in module.vpc.subnets : {
      network            = local.network_name
      subnetwork         = subnet.self_link
      subnetwork_project = null # will populate from subnetwork_self_link
      network_ip         = null
      nic_type           = coalesce(var.nic_type, try(regex("IRDMA", local.profile_name), regex("MRDMA", local.profile_name), "RDMA"))
      stack_type         = null
      queue_count        = null
      access_config      = []
      ipv6_access_config = []
      alias_ip_range     = []
    }
  ]
}

module "vpc" {
  source = "./vpc-submodule"

  network_name                           = local.network_name
  project_id                             = var.project_id
  auto_create_subnetworks                = false
  subnets                                = local.template_subnetworks
  secondary_ranges                       = var.secondary_ranges
  routing_mode                           = var.network_routing_mode
  mtu                                    = var.mtu
  description                            = var.network_description
  shared_vpc_host                        = var.shared_vpc_host
  delete_default_internet_gateway_routes = var.delete_default_internet_gateway_routes
  firewall_rules                         = local.firewall_rules
  network_profile                        = var.network_profile
}
