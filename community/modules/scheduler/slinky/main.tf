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

  # Define affinity settings when node pools are specified
  node_affinity = var.node_pool_names != null
  node_pool_affinity = local.node_affinity ? {
    nodeAffinity = {
      requiredDuringSchedulingIgnoredDuringExecution = {
        nodeSelectorTerms = [{
          matchExpressions = [{
            key      = "cloud.google.com/gke-nodepool"
            operator = "In"
            values   = var.node_pool_names
          }]
        }]
      }
    }
  } : {}
}

data "google_client_config" "default" {}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

resource "helm_release" "cert_manager" {
  name             = "cert-manager"
  repository       = "https://charts.jetstack.io"
  chart            = "cert-manager"
  namespace        = "cert-manager"
  create_namespace = true

  values = [
    yamlencode(merge(var.cert_manager_values, local.node_affinity ? {
      affinity = local.node_pool_affinity
    } : {}))
  ]
}

resource "helm_release" "slurm_operator" {
  name             = "slurm-operator"
  chart            = "slurm-operator"
  repository       = "oci://ghcr.io/slinkyproject/charts"
  version          = "0.2.0"
  namespace        = "slinky"
  create_namespace = true

  # The Cert Manager webhook deployment must be running to provision the Operator
  depends_on = [
    helm_release.cert_manager
  ]

  values = [
    file("${path.module}/values/operator.yaml"),
    yamlencode(merge(var.slurm_operator_values, local.node_affinity ? {
      operator = {
        affinity = local.node_pool_affinity
      }
      webhook = {
        affinity = local.node_pool_affinity
      }
    } : {}))
  ]
}

resource "helm_release" "slurm" {
  name             = "slurm"
  chart            = "slurm"
  repository       = "oci://ghcr.io/slinkyproject/charts"
  version          = "0.2.0"
  namespace        = "slurm"
  create_namespace = true

  # The Slurm Operator must be running to provision Slurm clusters/nodesets
  depends_on = [
    helm_release.slurm_operator
  ]

  values = [
    file("${path.module}/values/slurm.yaml"),
    yamlencode(merge(var.slurm_values, local.node_affinity ? {
      controller = {
        affinity = local.node_pool_affinity
      }
      accounting = {
        affinity = local.node_pool_affinity
      }
      mariadb = {
        affinity = local.node_pool_affinity
      }
      restapi = {
        affinity = local.node_pool_affinity
      }
    } : {}))
  ]
}
