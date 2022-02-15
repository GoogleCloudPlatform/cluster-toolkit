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

output "network_name" {
  description = "The name of the network created"
  value       = module.vpc.network_name
}

output "network_self_link" {
  description = "The URI of the VPC being created"
  value       = module.vpc.network_self_link

}

output "subnetwork" {
  description = "The first subnetwork found matching the primary region"
  value       = local.primary_subnetwork
}

output "subnetwork_name" {
  description = "The name of the subnetwork in the primary region"
  value       = local.primary_subnetwork_name
}

output "subnetwork_self_link" {
  description = "The subnetwork self-link in the primary region"
  value       = local.primary_subnetwork_self_link
}

output "subnetwork_address" {
  description = "The subnetwork address range in the primary region"
  value       = local.primary_subnetwork_ip_cidr_range
}

output "nat_ips" {
  description = "the external IPs assigned to the NAT"
  value       = flatten([for ipmod in module.nat_ip_addresses : ipmod.addresses])
}
