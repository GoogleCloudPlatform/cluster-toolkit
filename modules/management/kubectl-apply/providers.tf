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
  # HYBRID LOGIC: Use passed-in variable if available, otherwise fall back to container cluster data source
  host                   = var.cluster_endpoint != null ? "https://${var.cluster_endpoint}" : (length(data.google_container_cluster.gke_cluster) > 0 ? "https://${data.google_container_cluster.gke_cluster[0].endpoint}" : "")
  token                  = var.access_token != null ? var.access_token : data.google_client_config.default.access_token
  cluster_ca_certificate = var.cluster_ca_certificate != null ? base64decode(var.cluster_ca_certificate) : (length(data.google_container_cluster.gke_cluster) > 0 ? base64decode(data.google_container_cluster.gke_cluster[0].master_auth[0].cluster_ca_certificate) : "")
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
