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
  network_name          = coalesce(replace(var.network_name_prefix, "_", "-"), replace(var.deployment_name, "_", "-"))
  global_ip_cidr_prefix = split("/", var.global_ip_address_range)[0]
  global_ip_cidr_suffix = split("/", var.global_ip_address_range)[1]
  global_ip_cidr_valid  = "${local.global_ip_cidr_prefix}/${terraform_data.global_ip_cidr_suffix.output}"
  subnetwork_new_bits   = var.subnetwork_cidr_suffix - local.global_ip_cidr_suffix
  maximum_subnetworks   = pow(2, local.subnetwork_new_bits)
  additional_networks = [
    for vpc in module.vpcs :
    merge(var.network_interface_defaults, {
      network            = vpc.network_name
      subnetwork         = vpc.subnetwork_name
      subnetwork_project = var.project_id
    })
  ]
}

resource "terraform_data" "global_ip_cidr_suffix" {
  input = local.global_ip_cidr_suffix
  lifecycle {
    precondition {
      condition     = local.maximum_subnetworks >= var.network_count
      error_message = <<EOT
      Global IP range ${var.global_ip_address_range} and subnetwork CIDR suffix
      ${var.subnetwork_cidr_suffix} are incompatible with the VPC count
      ${var.network_count}. The subnetwork CIDR suffix must be at least
      ${ceil(log(var.network_count, 2))} greater than the global CIDR suffix.
      EOT
    }
  }
}

module "vpcs" {
  source = "../vpc"

  count = var.network_count

  project_id            = var.project_id
  deployment_name       = var.deployment_name
  region                = var.region
  network_address_range = cidrsubnet(local.global_ip_cidr_valid, local.subnetwork_new_bits, count.index)
  # the value 0 creates a single subnetwork that spans the entire range above
  # consider changing to explicit var.subnetworks implementation
  default_primary_subnetwork_size = 0

  network_name                           = "${local.network_name}-${count.index}"
  subnetwork_name                        = "${local.network_name}-${count.index}-subnet"
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
  network_profile                        = var.network_profile
}
