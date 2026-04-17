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
  description = "Name of the existing VPC network"
  value       = data.google_compute_network.vpc.name
}

output "network_id" {
  description = "ID of the existing VPC network"
  value       = data.google_compute_network.vpc.id
}

output "network_self_link" {
  description = "Self link of the existing VPC network"
  value       = data.google_compute_network.vpc.self_link
}

output "subnetwork" {
  description = "Full subnetwork object in the primary region"
  value       = data.google_compute_subnetwork.primary_subnetwork
}

output "subnetwork_name" {
  description = "Name of the subnetwork in the primary region"
  value       = data.google_compute_subnetwork.primary_subnetwork.name
}

output "subnetwork_self_link" {
  description = "Subnetwork self-link in the primary region"
  value       = data.google_compute_subnetwork.primary_subnetwork.self_link
}

output "subnetwork_address" {
  description = "Subnetwork IP range in the primary region"
  value       = data.google_compute_subnetwork.primary_subnetwork.ip_cidr_range
}
