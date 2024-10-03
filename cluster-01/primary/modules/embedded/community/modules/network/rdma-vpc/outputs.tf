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
  depends_on  = [module.vpc]
}

output "network_id" {
  description = "ID of the new VPC network"
  value       = module.vpc.network_id
  depends_on  = [module.vpc]
}

output "network_self_link" {
  description = "Self link of the new VPC network"
  value       = module.vpc.network_self_link
  depends_on  = [module.vpc]
}

output "subnetworks" {
  description = "Full list of subnetwork objects belonging to the new VPC network"
  value       = module.vpc.subnets
  depends_on  = [module.vpc]
}

output "subnetwork_interfaces" {
  description = "Full list of subnetwork objects belonging to the new VPC network (compatible with vm-instance)"
  value       = local.output_subnets
  depends_on  = [module.vpc]
}
