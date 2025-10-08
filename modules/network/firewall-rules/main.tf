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

locals {
  use_subnetwork_data = (var.project_id == null || var.network_name == null) && var.subnetwork_self_link != null
}

# the google_compute_network data source does not allow identification by
# self_link, which uniquely identifies subnet, project, and network
data "google_compute_subnetwork" "subnetwork" {
  # Only instantiate this data source if needed
  count     = local.use_subnetwork_data ? 1 : 0
  self_link = var.subnetwork_self_link
}

locals {
  # Derived values from data source, null if data source is not used
  derived_project_id   = local.use_subnetwork_data ? data.google_compute_subnetwork.subnetwork[0].project : null
  derived_network_name = local.use_subnetwork_data ? data.google_compute_subnetwork.subnetwork[0].network : null

  # Effective values: Use var if provided, otherwise use derived value
  effective_project_id   = coalesce(var.project_id, local.derived_project_id)
  effective_network_name = coalesce(var.network_name, local.derived_network_name)
}

# Module-level check for Private Google Access on the subnetwork.
# This check is only relevant if subnetwork_self_link was provided and used.
resource "terraform_data" "pga_check" {
  count = local.use_subnetwork_data ? 1 : 0

  lifecycle {
    precondition {
      condition     = data.google_compute_subnetwork.subnetwork[0].private_ip_google_access
      error_message = "Private Google Access is disabled for subnetwork '${data.google_compute_subnetwork.subnetwork[0].name}'. This may cause connectivity issues for instances without external IPs trying to access Google APIs and services."
    }
  }
}

module "firewall_rule" {
  source       = "terraform-google-modules/network/google//modules/firewall-rules"
  version      = "~> 12.0"
  project_id   = local.effective_project_id
  network_name = local.effective_network_name

  ingress_rules = var.ingress_rules
  egress_rules  = var.egress_rules
}
