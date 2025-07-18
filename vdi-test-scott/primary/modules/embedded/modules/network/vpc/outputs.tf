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
  description = "Name of the new VPC network"
  value       = module.vpc.network_name
  depends_on  = [module.vpc, module.cloud_router]
}

output "network_id" {
  description = "ID of the new VPC network"
  value       = module.vpc.network_id
  depends_on  = [module.vpc, module.cloud_router]
}

output "network_self_link" {
  description = "Self link of the new VPC network"
  value       = module.vpc.network_self_link
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetworks" {
  description = "Full list of subnetwork objects belonging to the new VPC network"
  value       = module.vpc.subnets
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetwork" {
  description = "Primary subnetwork object"
  value       = local.output_primary_subnetwork
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetwork_name" {
  description = "Name of the primary subnetwork"
  value       = local.output_primary_subnetwork_name
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetwork_self_link" {
  description = "Self link of the primary subnetwork"
  value       = local.output_primary_subnetwork_self_link
  depends_on  = [module.vpc, module.cloud_router]
}

output "subnetwork_address" {
  description = "IP address range of the primary subnetwork"
  value       = local.output_primary_subnetwork_ip_cidr_range
  depends_on  = [module.vpc, module.cloud_router]
}

output "nat_ips" {
  description = "External IPs of the Cloud NAT from which outbound internet traffic will arrive (empty list if no NAT is used)"
  value       = flatten([for ipmod in module.nat_ip_addresses : ipmod.addresses])
}
