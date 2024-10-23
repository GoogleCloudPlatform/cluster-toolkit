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
  version = "~> 6.1"

  count = var.create_bucket ? 1 : 0

  location   = var.region
  names      = [local.synth_bucket_name]
  prefix     = "slurm"
  project_id = var.project_id

  force_destroy = {
    (local.synth_bucket_name) = true
  }

  labels = merge(local.labels, {
    slurm_cluster_name = local.slurm_cluster_name
  })
}

# BUCKET IAMs
locals {
  compute_sa     = toset(flatten([for x in module.slurm_nodeset_template : x.service_account]))
  compute_tpu_sa = toset(flatten([for x in module.slurm_nodeset_tpu : x.service_account]))
  login_sa       = toset(flatten([for x in module.slurm_login_template : x.service_account]))

  viewers = toset(flatten([
    "serviceAccount:${module.slurm_controller_template.service_account.email}",
    formatlist("serviceAccount:%s", [for x in local.compute_sa : x.email]),
    formatlist("serviceAccount:%s", [for x in local.compute_tpu_sa : x.email if x.email != null]),
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

locals {
  daos_ns = [
    for ns in var.network_storage :
    ns if ns.fs_type == "daos"
  ]

  daos_client_install_runners = [
    for ns in local.daos_ns :
    ns.client_install_runner if ns.client_install_runner != null
  ]

  daos_mount_runners = [
    for ns in local.daos_ns :
    ns.mount_runner if ns.mount_runner != null
  ]

  daos_network_storage_runners = concat(
    local.daos_client_install_runners,
    local.daos_mount_runners,
  )

  daos_install_mount_script = {
    filename = "ghpc_daos_mount.sh"
    content  = length(local.daos_ns) > 0 ? module.daos_network_storage_scripts[0].startup_script : ""
  }
}

# SLURM FILES
locals {
  ghpc_startup_controller = {
    filename = "ghpc_startup.sh"
    content  = var.controller_startup_script
  }
  ghpc_startup_script_controller = length(local.daos_ns) > 0 ? [local.daos_install_mount_script, local.ghpc_startup_controller] : [local.ghpc_startup_controller]

  ghpc_startup_login = {
    filename = "ghpc_startup.sh"
    content  = var.login_startup_script
  }
  ghpc_startup_script_login = length(local.daos_ns) > 0 ? [local.daos_install_mount_script, local.ghpc_startup_login] : [local.ghpc_startup_login]

  ghpc_startup_compute = {
    filename = "ghpc_startup.sh"
    content  = var.compute_startup_script
  }
  ghpc_startup_script_compute = length(local.daos_ns) > 0 ? [local.daos_install_mount_script, local.ghpc_startup_compute] : [local.ghpc_startup_compute]

  nodeset_startup_scripts = { for k, v in local.nodeset_map : k => v.startup_script }
}

module "daos_network_storage_scripts" {
  count = length(local.daos_ns) > 0 ? 1 : 0

  source          = "../../../../modules/scripts/startup-script"
  labels          = local.labels
  project_id      = var.project_id
  deployment_name = var.deployment_name
  region          = var.region
  runners         = local.daos_network_storage_runners
}

module "slurm_files" {
  source = "./modules/slurm_files"

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
  nodeset_startup_scripts            = local.nodeset_startup_scripts
  compute_startup_scripts            = local.ghpc_startup_script_compute
  compute_startup_scripts_timeout    = var.compute_startup_scripts_timeout
  login_startup_scripts              = local.ghpc_startup_script_login
  login_startup_scripts_timeout      = var.login_startup_scripts_timeout

  enable_debug_logging = var.enable_debug_logging
  extra_logging_flags  = var.extra_logging_flags

  enable_bigquery_load          = var.enable_bigquery_load
  enable_external_prolog_epilog = var.enable_external_prolog_epilog
  epilog_scripts                = var.epilog_scripts
  prolog_scripts                = var.prolog_scripts
  enable_slurm_gcp_plugins      = var.enable_slurm_gcp_plugins

  disable_default_mounts = !var.enable_default_mounts
  network_storage = [
    for storage in var.network_storage : {
      server_ip     = storage.server_ip,
      remote_mount  = storage.remote_mount,
      local_mount   = storage.local_mount,
      fs_type       = storage.fs_type,
      mount_options = storage.mount_options
    }
    if storage.fs_type != "daos"
  ]
  login_network_storage = var.login_network_storage

  partitions = var.partitions

  nodeset     = local.nodesets
  nodeset_dyn = values(local.nodeset_dyn_map)
  # Use legacy format for now
  nodeset_tpu = values(module.slurm_nodeset_tpu)[*]


  depends_on = [module.bucket]

  # Providers
  endpoint_versions = var.endpoint_versions
}
