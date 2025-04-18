# Copyright 2023 Google LLC
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
  nodeset_map_ell = { for x in var.nodeset : x.nodeset_name => x... }
  nodeset_map     = { for k, vs in local.nodeset_map_ell : k => vs[0] }

  nodeset_tpu_map_ell = { for x in var.nodeset_tpu : x.nodeset_name => x... }
  nodeset_tpu_map     = { for k, vs in local.nodeset_tpu_map_ell : k => vs[0] }

  nodeset_dyn_map_ell = { for x in var.nodeset_dyn : x.nodeset_name => x... }
  nodeset_dyn_map     = { for k, vs in local.nodeset_dyn_map_ell : k => vs[0] }
}

# NODESET
module "nodeset" {
  source   = "../../internal/slurm-gcp/nodeset"
  for_each = local.nodeset_map

  project_id = var.project_id

  slurm_cluster_name = local.slurm_cluster_name
  slurm_bucket_path  = module.slurm_files.slurm_bucket_path
  slurm_bucket_name  = module.slurm_files.bucket_name
  slurm_bucket_dir   = module.slurm_files.bucket_dir

  nodeset = each.value

  startup_scripts         = concat(local.common_scripts, each.value.startup_script)
  startup_scripts_timeout = var.compute_startup_scripts_timeout

  universe_domain = var.universe_domain
}

moved {
  from = module.slurm_nodeset_template
  to   = module.nodest
}

module "nodeset_cleanup" {
  source   = "./modules/cleanup_compute"
  for_each = local.nodeset_map

  nodeset                = each.value
  project_id             = var.project_id
  slurm_cluster_name     = local.slurm_cluster_name
  enable_cleanup_compute = var.enable_cleanup_compute
  universe_domain        = var.universe_domain
  endpoint_versions      = var.endpoint_versions
  gcloud_path_override   = var.gcloud_path_override
  nodeset_template       = module.nodeset[each.value.nodeset_name].instance_template
}

# NODESET TPU
module "slurm_nodeset_tpu" {
  source   = "../../internal/slurm-gcp/nodeset_tpu"
  for_each = local.nodeset_tpu_map

  project_id             = var.project_id
  node_count_dynamic_max = each.value.node_count_dynamic_max
  node_count_static      = each.value.node_count_static
  nodeset_name           = each.value.nodeset_name
  zone                   = each.value.zone
  node_type              = each.value.node_type
  accelerator_config     = each.value.accelerator_config
  tf_version             = each.value.tf_version
  preemptible            = each.value.preemptible
  preserve_tpu           = each.value.preserve_tpu
  enable_public_ip       = each.value.enable_public_ip
  service_account        = each.value.service_account
  data_disks             = each.value.data_disks
  docker_image           = each.value.docker_image
  subnetwork             = each.value.subnetwork
}

module "nodeset_cleanup_tpu" {
  source   = "./modules/cleanup_tpu"
  for_each = local.nodeset_tpu_map

  nodeset = {
    nodeset_name = each.value.nodeset_name
    zone         = each.value.zone
  }

  project_id             = var.project_id
  slurm_cluster_name     = local.slurm_cluster_name
  enable_cleanup_compute = var.enable_cleanup_compute
  universe_domain        = var.universe_domain
  endpoint_versions      = var.endpoint_versions
  gcloud_path_override   = var.gcloud_path_override

  depends_on = [
    # Depend on controller network, as a best effort to avoid
    # subnetwork resourceInUseByAnotherResource error
    var.subnetwork_self_link
  ]
}

resource "google_storage_bucket_object" "parition_config" {
  for_each = { for p in var.partitions : p.partition_name => p }

  bucket  = module.slurm_files.bucket_name
  name    = "${module.slurm_files.bucket_dir}/partition_configs/${each.key}.yaml"
  content = yamlencode(each.value)
}

moved {
  from = module.slurm_files.google_storage_bucket_object.parition_config
  to   = google_storage_bucket_object.parition_config
}
