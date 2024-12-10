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

  install_kueue         = try(var.kueue.install, false)
  install_jobset        = try(var.jobset.install, false)
  kueue_install_source  = format("${path.module}/manifests/kueue-%s.yaml", try(var.kueue.version, ""))
  jobset_install_source = format("${path.module}/manifests/jobset-%s.yaml", try(var.jobset.version, ""))
}

data "google_container_cluster" "gke_cluster" {
  project  = local.project_id
  name     = local.cluster_name
  location = local.cluster_location
}

data "google_client_config" "default" {}

module "kubectl_apply_manifests" {
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

module "install_kueue" {
  source            = "./kubectl"
  source_path       = local.install_kueue ? local.kueue_install_source : null
  server_side_apply = true

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}

module "install_jobset" {
  source            = "./kubectl"
  source_path       = local.install_jobset ? local.jobset_install_source : null
  server_side_apply = true

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}

module "configure_kueue" {
  source        = "./kubectl"
  source_path   = local.install_kueue ? try(var.kueue.config_path, "") : null
  template_vars = local.install_kueue ? try(var.kueue.config_template_vars, null) : null
  depends_on    = [module.install_kueue]

  server_side_apply = true
  wait_for_rollout  = true

  providers = {
    kubectl = kubectl
    http    = http.h
  }
}
