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
  # this input variable is validated to be in CIDR format
  super_global_ip_cidr_prefix = split("/", var.super_global_ip_address_range)[1]
  network_new_bits            = var.network_cidr_prefix - local.super_global_ip_cidr_prefix
  subnetwork_bits             = ceil(log(var.network_count, 2))
  additional_networks = [
    for vpc in module.vpcs :
    {
      network            = null
      subnetwork         = vpc.subnetwork_name
      subnetwork_project = var.project_id
      network_ip         = ""
      nic_type           = "GVNIC"
      stack_type         = "IPV4_ONLY"
      queue_count        = null
      access_config      = []
      ipv6_access_config = []
      alias_ip_range     = []
    }
  ]
}

resource "null_resource" "vpc_validation" {
  lifecycle {
    precondition {
      condition     = local.network_new_bits >= local.subnetwork_bits
      error_message = "network_cidr_prefix must be greater than super_global_ip_address_range's CIDR prefix by enough to accommodate the network_count"
    }
  }
}

module "vpcs" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/network/vpc?ref=v1.31.1&depth=1"

  count = var.network_count

  project_id            = var.project_id
  deployment_name       = var.deployment_name
  region                = var.region
  network_address_range = cidrsubnet(var.super_global_ip_address_range, local.network_new_bits, count.index)

  network_name                           = "${replace(var.deployment_name, "_", "-")}-${count.index}"
  subnetwork_name                        = "${replace(var.deployment_name, "_", "-")}-subnet-${count.index}"
  allowed_ssh_ip_ranges                  = var.allowed_ssh_ip_ranges
  delete_default_internet_gateway_routes = var.delete_default_internet_gateway_routes
  enable_iap_rdp_ingress                 = var.enable_iap_rdp_ingress
  enable_iap_ssh_ingress                 = var.enable_iap_ssh_ingress
  enable_iap_winrm_ingress               = var.enable_iap_winrm_ingress
  enable_internal_traffic                = var.enable_internal_traffic
  extra_iap_ports                        = var.extra_iap_ports
  firewall_rules                         = var.firewall_rules
  ips_per_nat                            = var.ips_per_nat
  mtu                                    = var.mtu
  network_description                    = var.network_description
  network_routing_mode                   = var.network_routing_mode
  secondary_ranges                       = var.secondary_ranges
  default_primary_subnetwork_size        = 0
}
