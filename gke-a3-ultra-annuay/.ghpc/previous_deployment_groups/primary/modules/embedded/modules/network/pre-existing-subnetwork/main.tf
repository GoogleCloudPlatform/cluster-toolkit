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


data "google_compute_subnetwork" "primary_subnetwork" {
  name      = var.subnetwork_name
  region    = var.region
  project   = var.project
  self_link = var.subnetwork_self_link

  lifecycle {
    postcondition {
      condition     = self.self_link != null
      error_message = "The subnetwork: ${coalesce(var.subnetwork_name, var.subnetwork_self_link)} could not be found."
    }
  }
}

# Module-level check for Private Google Access on the subnetwork
check "private_google_access_enabled_subnetwork" {
  assert {
    condition     = data.google_compute_subnetwork.primary_subnetwork.private_ip_google_access
    error_message = "Private Google Access is disabled for subnetwork '${data.google_compute_subnetwork.primary_subnetwork.name}'. This may cause connectivity issues for instances without external IPs trying to access Google APIs and services."
  }
}
