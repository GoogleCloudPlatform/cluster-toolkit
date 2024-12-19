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
  apply_manifests_map = tomap({
    for index, manifest in var.apply_manifests : index => manifest
  })

  install_kueue         = try(var.kueue.install, false)
  install_jobset        = try(var.jobset.install, false)
  kueue_install_source  = format("${path.module}/manifests/kueue-%s.yaml", try(var.kueue.version, ""))
  jobset_install_source = format("${path.module}/manifests/jobset-%s.yaml", try(var.jobset.version, ""))
}

module "kubectl_apply_manifests" {
  for_each = var.gke_cluster_exists ? local.apply_manifests_map : {}
  source   = "./kubectl"

  content           = each.value.content
  source_path       = each.value.source
  template_vars     = each.value.template_vars
  server_side_apply = each.value.server_side_apply
  wait_for_rollout  = each.value.wait_for_rollout

  providers = {
    http = http.h
  }
}

module "install_kueue" {
  count             = var.gke_cluster_exists ? 1 : 0
  source            = "./kubectl"
  source_path       = local.install_kueue ? local.kueue_install_source : null
  server_side_apply = true

  providers = {
    http = http.h
  }
}

module "install_jobset" {
  count             = var.gke_cluster_exists ? 1 : 0
  source            = "./kubectl"
  source_path       = local.install_jobset ? local.jobset_install_source : null
  server_side_apply = true

  providers = {
    http = http.h
  }
}

module "configure_kueue" {
  count         = var.gke_cluster_exists ? 1 : 0
  source        = "./kubectl"
  source_path   = local.install_kueue ? try(var.kueue.config_path, "") : null
  template_vars = local.install_kueue ? try(var.kueue.config_template_vars, null) : null
  depends_on    = [module.install_kueue]

  server_side_apply = true
  wait_for_rollout  = true

  providers = {
    http = http.h
  }
}
