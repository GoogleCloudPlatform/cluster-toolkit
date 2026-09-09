# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

output "allow_auto_tiering" {
  description = "Whether the storage pool supports auto-tiering enabled volumes."
  value       = google_netapp_storage_pool.netapp_storage_pool.allow_auto_tiering
}

output "scale_type" {
  description = "Scale type of the storage pool. Flex-only."
  value       = google_netapp_storage_pool.netapp_storage_pool.service_level == "FLEX" ? google_netapp_storage_pool.netapp_storage_pool.scale_type : null
}

output "netapp_storage_pool_id" {
  description = "An identifier for the resource with format `projects/{{project}}/locations/{{location}}/storagePools/{{name}}`"
  value       = google_netapp_storage_pool.netapp_storage_pool.id
}

output "capacity_gb" {
  description = "Storage pool capacity in GiB."
  value       = google_netapp_storage_pool.netapp_storage_pool.capacity_gib
}

output "service_level" {
  description = "Storage pool service level."
  value       = google_netapp_storage_pool.netapp_storage_pool.service_level
}

output "mode" {
  description = "Storage pool mode. Flex-only."
  value       = google_netapp_storage_pool.netapp_storage_pool.service_level == "FLEX" ? google_netapp_storage_pool.netapp_storage_pool.mode : null
}

output "type" {
  description = "Storage pool type. Flex Unified pools use UNIFIED. Omitted for STANDARD, PREMIUM, and EXTREME pools."
  value       = google_netapp_storage_pool.netapp_storage_pool.service_level == "FLEX" ? google_netapp_storage_pool.netapp_storage_pool.type : null
}
