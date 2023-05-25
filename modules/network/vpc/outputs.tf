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

output "network_self_link" {
  description = "The URI of the VPC being created"
  value       = module.vpc.network_self_link
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetwork_name" {
  description = "The name of the primary subnetwork"
  value       = local.out_primary_subnet.name
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetwork_self_link" {
  description = "The self-link to the primary subnetwork"
  value       = local.out_primary_subnet.self_link
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetwork_address" {
  description = "The address range of the primary subnetwork"
  value       = local.out_primary_subnet.ip_cidr_range
  depends_on  = [module.vpc, module.cloud_router]
}

output "nat_ips" {
  description = "the external IPs assigned to the NAT"
  value       = flatten([for ipmod in module.nat_ip_addresses : ipmod.addresses])
}

output "network" {
  description = <<-EOT
  Information about the network and its primary subnetwork.
  See: 
  https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_network
  https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork
  EOT
  value = {
    id        = module.vpc.network_id
    name      = module.vpc.network_name
    self_link = module.vpc.network_self_link
    project   = module.vpc.project_id

    primary_subnet = {
      name          = local.out_primary_subnet.name
      self_link     = local.out_primary_subnet.self_link
      project       = local.out_primary_subnet.project
      ip_cidr_range = local.out_primary_subnet.ip_cidr_range
    }
  }
  depends_on = [module.vpc, module.cloud_router]
}
