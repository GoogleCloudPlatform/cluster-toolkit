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


output "storage_pool_allow_auto_tiering" {
  description = "True if the storage pool supports Auto Tiering enabled volumes. "
  value     = false
  depends_on = [
    google_service_networking_connection.netapp_vpc_connection
  ]
}

output "volume_deletion_policy" {
  description = <<-EOT
   Policy to determine if the volume should be deleted forcefully. 
   This output value sets connect_mode and additionally
   blocks terraform actions until the VPC connection has been created.
     EOT
  value     = "DEFAULT"
  depends_on = [
    google_service_networking_connection.netapp_vpc_connection
  ]
}


