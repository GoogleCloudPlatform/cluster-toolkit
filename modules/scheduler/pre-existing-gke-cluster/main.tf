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

data "google_container_cluster" "existing_gke_cluster" {
  name     = var.cluster_name
  project  = var.project_id
  location = var.region
}

locals {
  rdma_networks     = [for network_info in var.additional_networks : network_info if strcontains(upper(network_info.nic_type), "RDMA")]
  non_rdma_networks = [for network_info in var.additional_networks : network_info if !strcontains(upper(network_info.nic_type), "RDMA")]
  apply_manifests_rdma_networks = flatten([
    for idx, network_info in local.rdma_networks : [
      {
        source = "${path.module}/templates/gke-network-paramset.yaml.tftpl",
        template_vars = {
          name            = "${var.rdma_subnetwork_name_prefix}-${idx}",
          network_name    = network_info.network
          subnetwork_name = "${var.rdma_subnetwork_name_prefix}-${idx}",
          device_mode     = "RDMA"
        }
      },
      {
        source        = "${path.module}/templates/network-object.yaml.tftpl",
        template_vars = { name = "${var.rdma_subnetwork_name_prefix}-${idx}" }
      }
    ]
  ])

  apply_manifests_non_rdma_networks = flatten([
    for idx, network_info in local.non_rdma_networks : [
      {
        source = "${path.module}/templates/gke-network-paramset.yaml.tftpl",
        template_vars = {
          name            = network_info.subnetwork
          network_name    = network_info.network
          subnetwork_name = network_info.subnetwork
          device_mode     = "NetDevice"
        }
      },
      {
        source        = "${path.module}/templates/network-object.yaml.tftpl",
        template_vars = { name = network_info.subnetwork }
      }
    ]
  ])
}

module "kubectl_apply" {
  source = "../../management/kubectl-apply"

  cluster_id = data.google_container_cluster.existing_gke_cluster.id
  project_id = var.project_id

  apply_manifests = concat(local.apply_manifests_non_rdma_networks, local.apply_manifests_rdma_networks)
}
