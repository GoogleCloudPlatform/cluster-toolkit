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

output "network_name" {
  description = "The name of the network created"
  value       = module.vpc.network_name
  depends_on  = [module.firewall_rules, module.cloud_router]
}

output "network_self_link" {
  description = "The URI of the VPC being created"
  value       = module.vpc.network_self_link
  depends_on  = [module.firewall_rules, module.cloud_router]
}

output "subnetworks" {
  description = "All subnetwork resources created by this module"
  value       = module.vpc.subnets
  depends_on  = [module.firewall_rules, module.cloud_router]
}

output "subnetwork" {
  description = "The primary subnetwork object created by the input variable primary_subnetwork"
  value       = local.primary_subnetwork
  depends_on  = [module.firewall_rules, module.cloud_router]
}

output "subnetwork_name" {
  description = "The name of the primary subnetwork"
  value       = local.primary_subnetwork_name
  depends_on  = [module.firewall_rules, module.cloud_router]
}

output "subnetwork_self_link" {
  description = "The self-link to the primary subnetwork"
  value       = local.primary_subnetwork_self_link
  depends_on  = [module.firewall_rules, module.cloud_router]
}

output "subnetwork_address" {
  description = "The address range of the primary subnetwork"
  value       = local.primary_subnetwork_ip_cidr_range
  depends_on  = [module.firewall_rules, module.cloud_router]
}

output "nat_ips" {
  description = "the external IPs assigned to the NAT"
  value       = flatten([for ipmod in module.nat_ip_addresses : ipmod.addresses])
}
