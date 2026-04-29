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

resource "kubernetes_secret_v1" "secret" {
  metadata {
    name      = var.secret_name
    namespace = var.namespace
  }
  data = var.data
}
locals {
  cluster_id_parts = var.cluster_id != null ? split("/", var.cluster_id) : []
  cluster_name     = length(local.cluster_id_parts) > 5 ? local.cluster_id_parts[5] : ""
  cluster_location = length(local.cluster_id_parts) > 3 ? local.cluster_id_parts[3] : ""
  kube_project_id  = length(local.cluster_id_parts) > 1 ? local.cluster_id_parts[1] : ""

  # HYBRID LOGIC: Use passed-in variable if available, otherwise fall back to data source
  host                   = var.cluster_endpoint != null ? "https://${var.cluster_endpoint}" : "https://${data.google_container_cluster.gke_cluster.endpoint}"
  token                  = var.access_token != null ? var.access_token : data.google_client_config.default.access_token
  cluster_ca_certificate = var.cluster_ca_certificate != null ? base64decode(var.cluster_ca_certificate) : base64decode(data.google_container_cluster.gke_cluster.master_auth[0].cluster_ca_certificate)
}

data "google_client_config" "default" {}
data "google_container_cluster" "gke_cluster" {
  name     = local.cluster_name
  location = local.cluster_location
  project  = local.kube_project_id
}
provider "kubernetes" {
  host                   = local.host
  token                  = local.token
  cluster_ca_certificate = local.cluster_ca_certificate
}
