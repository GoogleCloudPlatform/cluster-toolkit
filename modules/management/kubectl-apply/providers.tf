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

locals {
  dns_endpoint           = try(data.google_container_cluster.gke_cluster.control_plane_endpoints_config[0].dns_endpoint_config[0].allow_external_traffic, false) ? try(data.google_container_cluster.gke_cluster.control_plane_endpoints_config[0].dns_endpoint_config[0].endpoint, "") : ""
  raw_host               = local.dns_endpoint != "" ? local.dns_endpoint : (var.cluster_endpoint != null && var.cluster_endpoint != "" ? var.cluster_endpoint : data.google_container_cluster.gke_cluster.endpoint)
  host                   = startswith(local.raw_host, "http://") || startswith(local.raw_host, "https://") ? local.raw_host : "https://${local.raw_host}"
  token                  = var.access_token != null ? var.access_token : data.google_client_config.default.access_token
  is_dns_endpoint        = strcontains(local.host, ".gke.goog")
  cluster_ca_certificate = local.is_dns_endpoint ? null : (var.cluster_ca_certificate != null ? (try(base64decode(var.cluster_ca_certificate), var.cluster_ca_certificate)) : base64decode(data.google_container_cluster.gke_cluster.master_auth[0].cluster_ca_certificate))
}

provider "helm" {
  kubernetes {
    host                   = local.host
    token                  = local.token
    cluster_ca_certificate = local.cluster_ca_certificate
  }
}

provider "kubernetes" {
  host                   = local.host
  token                  = local.token
  cluster_ca_certificate = local.cluster_ca_certificate
}
