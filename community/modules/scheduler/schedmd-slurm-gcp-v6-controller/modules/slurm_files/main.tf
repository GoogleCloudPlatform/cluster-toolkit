/**
 * Copyright (C) SchedMD LLC.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

locals {
  scripts_dir = abspath("${path.module}/scripts")

  bucket_dir = coalesce(var.bucket_dir, format("%s-files", var.slurm_cluster_name))
}

########
# DATA #
########

data "google_storage_bucket" "this" {
  name = var.bucket_name
}

##########
# RANDOM #
##########

resource "random_uuid" "cluster_id" {
}

##################
# CLUSTER CONFIG #
##################

locals {
  config = {
    enable_bigquery_load  = var.enable_bigquery_load
    cloudsql_secret       = var.cloudsql_secret
    cluster_id            = random_uuid.cluster_id.result
    project               = var.project_id
    slurm_cluster_name    = var.slurm_cluster_name
    enable_slurm_auth     = var.enable_slurm_auth
    bucket_path           = local.bucket_path
    enable_debug_logging  = var.enable_debug_logging
    extra_logging_flags   = var.extra_logging_flags
    controller_state_disk = var.controller_state_disk

    # storage
    disable_default_mounts = var.disable_default_mounts
    network_storage        = var.network_storage

    # timeouts
    controller_startup_scripts_timeout = var.controller_startup_scripts_timeout
    compute_startup_scripts_timeout    = var.compute_startup_scripts_timeout

    munge_mount     = local.munge_mount
    slurm_key_mount = var.slurm_key_mount

    # slurm conf
    prolog_scripts      = [for k, v in google_storage_bucket_object.prolog_scripts : k]
    epilog_scripts      = [for k, v in google_storage_bucket_object.epilog_scripts : k]
    task_prolog_scripts = [for k, v in google_storage_bucket_object.task_prolog_scripts : k]
    task_epilog_scripts = [for k, v in google_storage_bucket_object.task_epilog_scripts : k]
    cloud_parameters    = var.cloud_parameters

    # hybrid
    hybrid      = var.enable_hybrid
    hybrid_conf = var.enable_hybrid ? local.hybrid_conf : null

    controller_network_attachment = var.controller_network_attachment


    # config files templates
    slurmdbd_conf_tpl = file(coalesce(var.slurmdbd_conf_tpl, "${local.etc_dir}/slurmdbd.conf.tpl"))
    slurm_conf_tpl    = var.slurm_conf_template != null ? var.slurm_conf_template : file(coalesce(var.slurm_conf_tpl, "${local.etc_dir}/slurm.conf.tpl"))
    cgroup_conf_tpl   = file(coalesce(var.cgroup_conf_tpl, "${local.etc_dir}/cgroup.conf.tpl"))

    # Providers
    endpoint_versions = var.endpoint_versions
  }

  x_nodeset         = toset(var.nodeset[*].nodeset_name)
  x_nodeset_dyn     = toset(var.nodeset_dyn[*].nodeset_name)
  x_nodeset_tpu     = toset(var.nodeset_tpu[*].nodeset.nodeset_name)
  x_nodeset_overlap = setintersection([], local.x_nodeset, local.x_nodeset_dyn, local.x_nodeset_tpu)

  etc_dir = abspath("${path.module}/etc")

  bucket_path = format("%s/%s", data.google_storage_bucket.this.url, local.bucket_dir)
  output_dir  = try(abspath(coalesce(var.hybrid_conf.output_dir, ".")), abspath("."))
  hybrid_conf = {
    #Required params
    slurm_control_host = var.hybrid_conf != null ? var.hybrid_conf.slurm_control_host : null
    #Optional params
    output_dir  = local.output_dir
    install_dir = try(abspath(var.hybrid_conf.install_dir), local.output_dir)

    slurm_uid               = try(coalesce(var.hybrid_conf.slurm_uid, 981), 981)
    slurm_gid               = try(coalesce(var.hybrid_conf.slurm_gid, 981), 981)
    slurm_control_host_port = try(coalesce(var.hybrid_conf.slurm_control_host_port, "6817"), "6817")
    slurm_log_dir           = try(abspath(var.hybrid_conf.slurm_log_dir), null)
    slurm_bin_dir           = try(abspath(var.hybrid_conf.slurm_bin_dir), null)
    slurm_control_addr      = try(var.hybrid_conf.slurm_control_addr, null)
    google_app_cred_path    = try(abspath(var.hybrid_conf.google_app_cred_path), null)
  }
  munge_mount = var.enable_hybrid ? {
    server_ip     = lookup(var.munge_mount, "server_ip", coalesce(var.hybrid_conf.slurm_control_addr, var.hybrid_conf.slurm_control_host))
    remote_mount  = lookup(var.munge_mount, "remote_mount", "/etc/munge/")
    fs_type       = lookup(var.munge_mount, "fs_type", "nfs")
    mount_options = lookup(var.munge_mount, "mount_options", "")
  } : null

}

resource "google_storage_bucket_object" "config" {
  bucket  = data.google_storage_bucket.this.name
  name    = "${local.bucket_dir}/config.yaml"
  content = yamlencode(local.config)

  # Take dependency on all other "config artifacts" so creation of `config.yaml`
  # can be used as a signal for setup.py that "everything is ready".
  # Some of following files, particularly mount scripts for new NFSes, can take a while to be created.
  depends_on = [
    google_storage_bucket_object.controller_startup_scripts,
    google_storage_bucket_object.nodeset_startup_scripts,
    google_storage_bucket_object.prolog_scripts,
    google_storage_bucket_object.epilog_scripts,
    google_storage_bucket_object.task_prolog_scripts,
    google_storage_bucket_object.task_epilog_scripts
  ]
}

resource "google_storage_bucket_object" "nodeset_config" {
  for_each = { for ns in var.nodeset : ns.nodeset_name => merge(ns, {
    instance_properties = jsondecode(ns.instance_properties_json)
  }) }

  bucket  = data.google_storage_bucket.this.name
  name    = "${local.bucket_dir}/nodeset_configs/${each.key}.yaml"
  content = yamlencode(each.value)
}

resource "google_storage_bucket_object" "nodeset_dyn_config" {
  for_each = { for ns in var.nodeset_dyn : ns.nodeset_name => ns }

  bucket  = data.google_storage_bucket.this.name
  name    = "${local.bucket_dir}/nodeset_dyn_configs/${each.key}.yaml"
  content = yamlencode(each.value)
}

resource "google_storage_bucket_object" "nodeset_tpu_config" {
  for_each = { for n in var.nodeset_tpu[*].nodeset : n.nodeset_name => n }

  bucket  = data.google_storage_bucket.this.name
  name    = "${local.bucket_dir}/nodeset_tpu_configs/${each.key}.yaml"
  content = yamlencode(each.value)
}

#########
# DEVEL #
#########

locals {
  build_dir = abspath("${path.module}/build")

  slurm_gcp_devel_zip        = "slurm-gcp-devel.zip"
  slurm_gcp_devel_zip_bucket = format("%s/%s", local.bucket_dir, local.slurm_gcp_devel_zip)
  devel_zip_directory        = var.enable_hybrid ? local.output_dir : local.build_dir
}

data "archive_file" "slurm_gcp_devel_zip" {
  output_path = "${local.devel_zip_directory}/${local.slurm_gcp_devel_zip}"
  type        = "zip"
  source_dir  = local.scripts_dir

  excludes = flatten([
    fileset(local.scripts_dir, "tests/**"),
    # TODO: consider removing (including nested) __pycache__ and all .* files
    # Though it only affects developers
  ])

}

resource "google_storage_bucket_object" "devel" {
  bucket = var.bucket_name
  name   = local.slurm_gcp_devel_zip_bucket
  source = data.archive_file.slurm_gcp_devel_zip.output_path
}


###########
# SCRIPTS #
###########

resource "google_storage_bucket_object" "controller_startup_scripts" {
  for_each = {
    for x in local.controller_startup_scripts
    : replace(basename(x.filename), "/[^a-zA-Z0-9-_]/", "_") => x
  }

  bucket  = var.bucket_name
  name    = format("%s/slurm-controller-script-%s", local.bucket_dir, each.key)
  content = each.value.content
}

resource "google_storage_bucket_object" "nodeset_startup_scripts" {
  for_each = { for x in flatten([
    for nodeset, scripts in var.nodeset_startup_scripts
    : [for s in scripts
      : {
        content = s.content,
      name = format("slurm-nodeset-%s-script-%s", nodeset, replace(basename(s.filename), "/[^a-zA-Z0-9-_]/", "_")) }
  ]]) : x.name => x.content }

  bucket  = var.bucket_name
  name    = format("%s/%s", local.bucket_dir, each.key)
  content = each.value
}

resource "google_storage_bucket_object" "prolog_scripts" {
  for_each = {
    for x in local.prolog_scripts
    : replace(basename(x.filename), "/[^a-zA-Z0-9-_]/", "_") => x
  }

  bucket  = var.bucket_name
  name    = format("%s/slurm-prolog-script-%s", local.bucket_dir, each.key)
  content = each.value.content
  source  = each.value.source
}

resource "google_storage_bucket_object" "epilog_scripts" {
  for_each = {
    for x in local.epilog_scripts
    : replace(basename(x.filename), "/[^a-zA-Z0-9-_]/", "_") => x
  }

  bucket  = var.bucket_name
  name    = format("%s/slurm-epilog-script-%s", local.bucket_dir, each.key)
  content = each.value.content
  source  = each.value.source
}

resource "google_storage_bucket_object" "task_prolog_scripts" {
  for_each = {
    for x in local.task_prolog_scripts
    : replace(basename(x.filename), "/[^a-zA-Z0-9-_]/", "_") => x
  }

  bucket  = var.bucket_name
  name    = format("%s/slurm-task_prolog-script-%s", local.bucket_dir, each.key)
  content = each.value.content
  source  = each.value.source
}

resource "google_storage_bucket_object" "task_epilog_scripts" {
  for_each = {
    for x in local.task_epilog_scripts
    : replace(basename(x.filename), "/[^a-zA-Z0-9-_]/", "_") => x
  }

  bucket  = var.bucket_name
  name    = format("%s/slurm-task_epilog-script-%s", local.bucket_dir, each.key)
  content = each.value.content
  source  = each.value.source
}

############################
# DATA: CHS GPU HEALTH CHECK
############################

data "local_file" "chs_gpu_health_check" {
  filename = "${path.module}/scripts/tools/gpu-test"
}

################################
# DATA: EXTERNAL PROLOG/EPILOG #
################################

data "local_file" "external_epilog" {
  filename = "${path.module}/files/external_epilog.sh"
}

data "local_file" "external_prolog" {
  filename = "${path.module}/files/external_prolog.sh"
}

data "local_file" "setup_external" {
  filename = "${path.module}/files/setup_external.sh"
}

locals {
  external_epilog = [{
    filename = "z_external_epilog.sh"
    content  = data.local_file.external_epilog.content
    source   = null
  }]
  external_prolog = [{
    filename = "z_external_prolog.sh"
    content  = data.local_file.external_prolog.content
    source   = null
  }]
  setup_external = [{
    filename = "z_setup_external.sh"
    content  = data.local_file.setup_external.content
  }]
  chs_gpu_health_check = [{
    filename = "a_chs_gpu_health_check.sh"
    content  = data.local_file.chs_gpu_health_check.content
    source   = null
  }]

  chs_prolog          = var.enable_chs_gpu_health_check_prolog ? local.chs_gpu_health_check : []
  ext_prolog          = var.enable_external_prolog_epilog ? local.external_prolog : []
  prolog_scripts      = concat(local.chs_prolog, local.ext_prolog, var.prolog_scripts)
  task_prolog_scripts = var.task_prolog_scripts

  chs_epilog          = var.enable_chs_gpu_health_check_epilog ? local.chs_gpu_health_check : []
  ext_epilog          = var.enable_external_prolog_epilog ? local.external_epilog : []
  epilog_scripts      = concat(local.chs_epilog, local.ext_epilog, var.epilog_scripts)
  task_epilog_scripts = var.task_epilog_scripts

  controller_startup_scripts = var.enable_external_prolog_epilog ? concat(local.setup_external, var.controller_startup_scripts) : var.controller_startup_scripts


}
