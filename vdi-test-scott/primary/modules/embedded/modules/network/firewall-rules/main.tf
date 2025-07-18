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

# the google_compute_network data source does not allow identification by
# self_link, which uniquely identifies subnet, project, and network
data "google_compute_subnetwork" "subnetwork" {
  self_link = var.subnetwork_self_link
}

# Module-level check for Private Google Access on the subnetwork
check "private_google_access_enabled_subnetwork" {
  assert {
    condition     = data.google_compute_subnetwork.subnetwork.private_ip_google_access
    error_message = "Private Google Access is disabled for subnetwork '${data.google_compute_subnetwork.subnetwork.name}'. This may cause connectivity issues for instances without external IPs trying to access Google APIs and services."
  }
}

module "firewall_rule" {
  source       = "terraform-google-modules/network/google//modules/firewall-rules"
  version      = "~> 9.0"
  project_id   = data.google_compute_subnetwork.subnetwork.project
  network_name = data.google_compute_subnetwork.subnetwork.network

  ingress_rules = var.ingress_rules
  egress_rules  = var.egress_rules
}
