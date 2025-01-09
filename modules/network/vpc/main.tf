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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "vpc", ghpc_role = "network" })
}

locals {
  autoname        = replace(var.deployment_name, "_", "-")
  network_name    = var.network_name == null ? "${local.autoname}-net" : var.network_name
  subnetwork_name = var.subnetwork_name == null ? "${local.autoname}-primary-subnet" : var.subnetwork_name

  # define a default subnetwork for cases in which no explicit subnetworks are
  # defined in var.subnetworks
  default_primary_subnetwork_cidr_block = cidrsubnet(var.network_address_range, var.default_primary_subnetwork_size, 0)
  default_primary_subnetwork = {
    subnet_name           = local.subnetwork_name
    subnet_ip             = local.default_primary_subnetwork_cidr_block
    subnet_region         = var.region
    subnet_private_access = true
    subnet_flow_logs      = false
    description           = "primary subnetwork in ${local.network_name}"
    purpose               = null
    role                  = null
  }

  # Identify user-supplied primary subnetwork
  # (1) explicit var.subnetworks[0]
  # (2) implicit local default subnetwork
  input_primary_subnetwork = coalesce(try(var.subnetworks[0], null), local.default_primary_subnetwork)

  # Identify user-supplied additional subnetworks
  # (1) explicit var.subnetworks[1:end]
  # (2) empty list
  input_additional_subnetworks = try(slice(var.subnetworks, 1, length(var.subnetworks)), [])

  # at this point we have constructed a list of subnetworks but need to extract
  # user-provided CIDR blocks or calculate them from user-provided new_bits
  # after we complete deprecation, local.all_subnetworks can be replaced with
  # var.subnetworks (or local.default_primary_subnetwork if that is null)
  input_subnetworks = concat([local.input_primary_subnetwork], local.input_additional_subnetworks)
  subnetworks_cidr_blocks = try(
    local.input_subnetworks[*]["subnet_ip"],
    cidrsubnets(var.network_address_range, local.input_subnetworks[*]["new_bits"]...)
  )

  # merge in the CIDR blocks (even when already there) and remove new_bits
  subnetworks = [for i, subnet in local.input_subnetworks :
    merge({ for k, v in subnet : k => v if k != "new_bits" }, { "subnet_ip" = local.subnetworks_cidr_blocks[i] })
  ]

  # gather the unique regions for purposes of creating Router/NAT
  cloud_router_regions = var.enable_cloud_router ? distinct([for subnet in local.subnetworks : subnet.subnet_region]) : []
  cloud_nat_regions    = var.enable_cloud_nat ? local.cloud_router_regions : []

  # this comprehension should have 1 and only 1 match
  output_primary_subnetwork               = one([for k, v in module.vpc.subnets : v if k == "${local.subnetworks[0].subnet_region}/${local.subnetworks[0].subnet_name}"])
  output_primary_subnetwork_name          = local.output_primary_subnetwork.name
  output_primary_subnetwork_self_link     = local.output_primary_subnetwork.self_link
  output_primary_subnetwork_ip_cidr_range = local.output_primary_subnetwork.ip_cidr_range

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

  secondary_ranges_map = {
    for secondary_range in var.secondary_ranges_list :
    secondary_range.subnetwork_name => secondary_range.ranges
  }
}

module "vpc" {
  source  = "terraform-google-modules/network/google"
  version = "~> 10.0"

  network_name                           = local.network_name
  project_id                             = var.project_id
  auto_create_subnetworks                = false
  subnets                                = local.subnetworks
  secondary_ranges                       = length(local.secondary_ranges_map) > 0 ? local.secondary_ranges_map : var.secondary_ranges
  routing_mode                           = var.network_routing_mode
  mtu                                    = var.mtu
  description                            = var.network_description
  shared_vpc_host                        = var.shared_vpc_host
  delete_default_internet_gateway_routes = var.delete_default_internet_gateway_routes
  firewall_rules                         = local.firewall_rules
  network_profile                        = var.network_profile
}

resource "terraform_data" "cloud_nat_validation" {
  lifecycle {
    precondition {
      condition     = var.enable_cloud_router == true || var.enable_cloud_nat == false
      error_message = <<-EOD
        "Cannot have Cloud NAT without a Cloud Router. If you desire Cloud NAT functionality please set `enable_cloud_router` to true."
      EOD
    }
  }
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
  version = "~> 4.1"

  depends_on = [terraform_data.cloud_nat_validation]

  for_each = toset(local.cloud_nat_regions)

  project_id = var.project_id
  region     = each.value
  # an external, regional (not global) IP address is suited for a regional NAT
  address_type = "EXTERNAL"
  global       = false
  labels       = local.labels
  names        = [for idx in range(var.ips_per_nat) : "${local.network_name}-nat-ips-${each.value}-${idx}"]
}

module "cloud_router" {
  source  = "terraform-google-modules/cloud-router/google"
  version = "~> 6.0"

  depends_on = [terraform_data.cloud_nat_validation]

  for_each = toset(local.cloud_router_regions)

  project = var.project_id
  name    = "${local.network_name}-router"
  region  = each.value
  network = module.vpc.network_name
  # in scenario with no NAT IPs, no NAT is created even if router is created
  # https://github.com/terraform-google-modules/terraform-google-cloud-router/blob/v2.0.0/nat.tf#L18-L20
  nats = length(module.nat_ip_addresses[each.value].self_links) == 0 ? [] : [
    {
      name : "cloud-nat-${each.value}",
      nat_ips : module.nat_ip_addresses[each.value].self_links
    },
  ]
}
