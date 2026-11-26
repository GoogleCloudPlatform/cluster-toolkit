/**
 * Copyright 2026 Google LLC
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

data "google_container_cluster" "gke_cluster" {
  name     = local.cluster_name
  location = local.cluster_location
  project  = local.project_id
}

locals {
  dns_endpoint = try(data.google_container_cluster.gke_cluster.control_plane_endpoints_config[0].dns_endpoint_config[0].allow_external_traffic, false) ? try(data.google_container_cluster.gke_cluster.control_plane_endpoints_config[0].dns_endpoint_config[0].endpoint, "") : ""
  raw_host     = local.dns_endpoint != "" ? local.dns_endpoint : (var.cluster_endpoint != null && var.cluster_endpoint != "" ? var.cluster_endpoint : data.google_container_cluster.gke_cluster.endpoint)
}
