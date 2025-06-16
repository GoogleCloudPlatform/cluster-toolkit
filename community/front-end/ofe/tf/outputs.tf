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

output "server_ip" {
  description = "Webserver IP Address"
  value       = google_compute_instance.server_vm.network_interface[0].access_config[0].nat_ip
}

output "oauth_enabled" {
  description = "Whether OAuth/IAP is enabled for this deployment"
  value       = length(google_iap_client.project_client) > 0
}

output "oauth_project_id" {
  description = "Project ID where OAuth/IAP resources are located"
  value       = length(google_iap_client.project_client) > 0 ? local.oauth_project : null
}

output "oauth_client_id" {
  description = "OAuth Client ID (only available when OAuth is enabled)"
  value       = length(google_iap_client.project_client) > 0 ? google_iap_client.project_client[0].client_id : null
  sensitive   = false
}

output "oauth_client_secret" {
  description = "OAuth Client Secret (only available when OAuth is enabled)"
  value       = length(google_iap_client.project_client) > 0 ? google_iap_client.project_client[0].secret : null
  sensitive   = true
}
