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
  cluster_id_parts = split("/", var.cluster_id)
  cluster_name     = local.cluster_id_parts[5]
  cluster_location = local.cluster_id_parts[3]
  project_id       = var.project_id != null ? var.project_id : local.cluster_id_parts[1]

  apply_manifests_map = tomap({
    for index, manifest in var.apply_manifests : index => manifest
  })
}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

module "kubectl" {
  for_each = local.apply_manifests_map
  source   = "./kubectl"

  content           = each.value.content
  source_path       = each.value.source
  template_vars     = each.value.template_vars
  server_side_apply = each.value.server_side_apply
  wait_for_rollout  = each.value.wait_for_rollout

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}
