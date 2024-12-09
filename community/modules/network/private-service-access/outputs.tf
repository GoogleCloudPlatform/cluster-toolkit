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

output "private_vpc_connection_peering" {
  description = "The name of the VPC Network peering connection that was created by the service provider."
  sensitive   = true
  value       = google_service_networking_connection.private_vpc_connection.peering
}

output "connect_mode" {
  description = <<-EOT
    Services that use Private Service Access typically specify connect_mode
    "PRIVATE_SERVICE_ACCESS". This output value sets connect_mode and additionally
    blocks terraform actions until the VPC connection has been created.
    EOT
  value       = "PRIVATE_SERVICE_ACCESS"
  depends_on = [
    google_service_networking_connection.private_vpc_connection,
  ]
}

output "reserved_ip_range" {
  description = "Named IP range to be used by services connected with Private Service Access."
  value       = google_compute_global_address.private_ip_alloc.name
}

output "cidr_range" {
  description = "CIDR range of the created google_compute_global_address"
  value       = "${google_compute_global_address.private_ip_alloc.address}/${google_compute_global_address.private_ip_alloc.prefix_length}"
}
