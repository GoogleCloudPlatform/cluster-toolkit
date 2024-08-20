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

output "network_names" {
  description = "Names of the new VPC networks"
  value       = module.vpcs[*].network_name
}

output "network_ids" {
  description = "IDs of the new VPC network"
  value       = module.vpcs[*].network_id
}

output "network_self_links" {
  description = "Self link of the new VPC network"
  value       = module.vpcs[*].network_self_link
}

output "additional_networks" {
  description = "Network interfaces for each subnetwork created by this module"
  value       = local.additional_networks
}

output "subnetwork_names" {
  description = "Names of the subnetwork created in each network"
  value       = module.vpcs[*].subnetwork_name
}

output "subnetwork_self_links" {
  description = "Self link of the primary subnetwork"
  value       = module.vpcs[*].subnetwork_self_link
}

output "subnetwork_addresses" {
  description = "IP address range of the primary subnetwork"
  value       = module.vpcs[*].subnetwork_address
}
