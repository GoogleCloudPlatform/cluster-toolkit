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


# BUCKET

locals {
  synt_suffix       = substr(md5("${var.project_id}${var.deployment_name}"), 0, 5)
  synth_bucket_name = "${local.slurm_cluster_name}${local.synt_suffix}"

  bucket_name = var.create_bucket ? module.bucket[0].name : var.bucket_name
}

module "bucket" {
  source  = "terraform-google-modules/cloud-storage/google"
  version = "~> 3.0"

  count = var.create_bucket ? 1 : 0

  location   = var.region
  names      = [local.synth_bucket_name]
  prefix     = "slurm"
  project_id = var.project_id

  force_destroy = {
    (local.synth_bucket_name) = true
  }

  labels = {
    slurm_cluster_name = local.slurm_cluster_name
  }
}

# BUCKET IAMs
locals {
  controller_sa  = toset(flatten([for x in module.slurm_controller_template : x.service_account]))
  compute_sa     = toset(flatten([for x in module.slurm_nodeset_template : x.service_account]))
  compute_tpu_sa = toset(flatten([for x in module.slurm_nodeset_tpu : x.service_account]))
  login_sa       = toset(flatten([for x in module.slurm_login_template : x.service_account]))

  viewers = toset(flatten([
    formatlist("serviceAccount:%s", [for x in local.controller_sa : x.email]),
    formatlist("serviceAccount:%s", [for x in local.compute_sa : x.email]),
    formatlist("serviceAccount:%s", [for x in local.compute_tpu_sa : x.email]),
    formatlist("serviceAccount:%s", [for x in local.login_sa : x.email]),
  ]))
}


resource "google_storage_bucket_iam_binding" "viewers" {
  bucket  = local.bucket_name
  role    = "roles/storage.objectViewer"
  members = compact(local.viewers)
}

resource "google_storage_bucket_iam_binding" "legacy_readers" {
  bucket  = local.bucket_name
  role    = "roles/storage.legacyBucketReader"
  members = compact(local.viewers)
}

# SLURM FILES
locals {
  ghpc_startup_script_controller = [{
    filename = "ghpc_startup.sh"
    content  = var.controller_startup_script
  }]
  ghpc_startup_script_login = [{
    filename = "ghpc_startup.sh"
    content  = var.login_startup_script
  }]
  ghpc_startup_script_compute = [{
    filename = "ghpc_startup.sh"
    content  = var.compute_startup_script
  }]
}

module "slurm_files" {
  source = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_files?ref=6.2.0"

  project_id         = var.project_id
  slurm_cluster_name = local.slurm_cluster_name
  bucket_dir         = var.bucket_dir
  bucket_name        = local.bucket_name

  slurmdbd_conf_tpl = var.slurmdbd_conf_tpl
  slurm_conf_tpl    = var.slurm_conf_tpl
  cgroup_conf_tpl   = var.cgroup_conf_tpl
  cloud_parameters  = var.cloud_parameters
  cloudsql_secret = try(
    one(google_secret_manager_secret_version.cloudsql_version[*].id),
  null)

  controller_startup_scripts         = local.ghpc_startup_script_controller
  controller_startup_scripts_timeout = var.controller_startup_scripts_timeout
  compute_startup_scripts            = local.ghpc_startup_script_compute
  compute_startup_scripts_timeout    = var.compute_startup_scripts_timeout
  login_startup_scripts              = local.ghpc_startup_script_login
  login_startup_scripts_timeout      = var.login_startup_scripts_timeout

  enable_devel         = var.enable_devel
  enable_debug_logging = var.enable_debug_logging
  extra_logging_flags  = var.extra_logging_flags

  enable_bigquery_load = var.enable_bigquery_load
  epilog_scripts       = var.epilog_scripts
  prolog_scripts       = var.prolog_scripts

  disable_default_mounts = var.disable_default_mounts
  network_storage        = var.network_storage
  login_network_storage  = var.login_network_storage

  partitions  = values(module.slurm_partition)[*]
  nodeset     = values(module.slurm_nodeset)[*]
  nodeset_tpu = values(module.slurm_nodeset_tpu)[*]

  depends_on = [module.bucket]
}
