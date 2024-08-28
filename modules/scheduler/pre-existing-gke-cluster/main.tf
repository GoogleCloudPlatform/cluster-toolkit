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

module "kubectl_apply" {
  source = "../../management/kubectl-apply" # can point to github

  cluster_id = data.google_container_cluster.existing_gke_cluster.id
  project_id = var.project_id

  apply_manifests = flatten([
    for idx, network_info in var.additional_networks : [
      {
        source = "${path.module}/templates/gke-network-paramset.yaml.tftpl",
        template_vars = {
          name            = "vpc${idx + 1}",
          network_name    = network_info.network
          subnetwork_name = network_info.subnetwork
        }
      },
      {
        source        = "${path.module}/templates/network-object.yaml.tftpl",
        template_vars = { name = "vpc${idx + 1}" }
      }
    ]
  ])
}
