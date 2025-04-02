# Copyright 2025 "Google LLC"
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

locals {
  cluster_id_parts = split("/", var.cluster_id)
  cluster_name     = local.cluster_id_parts[5]
  cluster_location = local.cluster_id_parts[3]
  project_id       = var.project_id != null ? var.project_id : local.cluster_id_parts[1]

  kubernetes_config = {
    host  = "https://${data.google_container_cluster.gke_cluster.endpoint}"
    token = data.google_client_config.default.access_token
    cluster_ca_certificate = base64decode(
      data.google_container_cluster.gke_cluster.master_auth[0].cluster_ca_certificate,
    )
  }
}

data "google_client_config" "default" {}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

resource "kubernetes_namespace" "cert_manager" {
  metadata {
    name = "cert-manager"
  }
}

resource "kubernetes_namespace" "slinky" {
  metadata {
    name = "slinky"
  }
}

resource "kubernetes_namespace" "slurm" {
  metadata {
    name = "slurm"
  }
}

resource "helm_release" "cert_manager" {
  name       = "cert-manager"
  repository = "https://charts.jetstack.io"
  chart      = "cert-manager"
  namespace  = kubernetes_namespace.cert_manager.metadata[0].name

  values = [
    yamlencode(var.cert_manager_values)
  ]
}

resource "helm_release" "slurm_operator" {
  name       = "slurm-operator"
  chart      = "slurm-operator"
  repository = "oci://ghcr.io/slinkyproject/charts"
  version    = "0.2.0"
  namespace  = kubernetes_namespace.slinky.metadata[0].name

  values = [
    file("${path.module}/values/operator.yaml"),
    yamlencode(var.slurm_operator_values)
  ]
}

resource "helm_release" "slurm" {
  name       = "slurm"
  chart      = "slurm"
  repository = "oci://ghcr.io/slinkyproject/charts"
  version    = "0.2.0"
  namespace  = kubernetes_namespace.slurm.metadata[0].name

  values = [
    file("${path.module}/values/slurm.yaml"),
    yamlencode(var.slurm_values)
  ]
}
