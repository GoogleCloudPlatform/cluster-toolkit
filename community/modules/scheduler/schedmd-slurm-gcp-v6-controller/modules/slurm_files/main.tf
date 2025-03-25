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
    bucket_path           = local.bucket_path
    enable_debug_logging  = var.enable_debug_logging
    extra_logging_flags   = var.extra_logging_flags
    controller_state_disk = var.controller_state_disk

    # storage
    disable_default_mounts = var.disable_default_mounts
    network_storage        = var.network_storage
    login_network_storage  = var.enable_hybrid ? null : var.login_network_storage

    # timeouts
    controller_startup_scripts_timeout = var.controller_startup_scripts_timeout
    compute_startup_scripts_timeout    = var.compute_startup_scripts_timeout
    login_startup_scripts_timeout      = var.login_startup_scripts_timeout
    munge_mount                        = local.munge_mount

    # slurm conf
    prolog_scripts   = [for k, v in google_storage_bucket_object.prolog_scripts : k]
    epilog_scripts   = [for k, v in google_storage_bucket_object.epilog_scripts : k]
    cloud_parameters = var.cloud_parameters

    # hybrid
    hybrid                        = var.enable_hybrid
    google_app_cred_path          = var.enable_hybrid ? local.google_app_cred_path : null
    output_dir                    = var.enable_hybrid ? local.output_dir : null
    install_dir                   = var.enable_hybrid ? local.install_dir : null
    slurm_control_host            = var.enable_hybrid ? var.slurm_control_host : null
    slurm_control_host_port       = var.enable_hybrid ? local.slurm_control_host_port : null
    slurm_control_addr            = var.enable_hybrid ? var.slurm_control_addr : null
    slurm_bin_dir                 = var.enable_hybrid ? local.slurm_bin_dir : null
    slurm_log_dir                 = var.enable_hybrid ? local.slurm_log_dir : null
    controller_network_attachment = var.controller_network_attachment


    # config files templates
    slurmdbd_conf_tpl = file(coalesce(var.slurmdbd_conf_tpl, "${local.etc_dir}/slurmdbd.conf.tpl"))
    slurm_conf_tpl    = file(coalesce(var.slurm_conf_tpl, "${local.etc_dir}/slurm.conf.tpl"))
    cgroup_conf_tpl   = file(coalesce(var.cgroup_conf_tpl, "${local.etc_dir}/cgroup.conf.tpl"))

    # Providers
    endpoint_versions = var.endpoint_versions

    # Extra-files MD5 hashes. Makes config file creation to wait on those files
    controller_startup_scripts_md5 = { for k, v in local.controller_script_files : k => md5(v) }
    nodeset_startup_scripts_md5    = { for k, v in local.nodeset_script_files : k => md5(v) }
    login_startup_scripts_md5      = { for k, v in local.login_script_files : k => md5(v) }
  }

  x_nodeset         = toset(var.nodeset[*].nodeset_name)
  x_nodeset_dyn     = toset(var.nodeset_dyn[*].nodeset_name)
  x_nodeset_tpu     = toset(var.nodeset_tpu[*].nodeset.nodeset_name)
  x_nodeset_overlap = setintersection([], local.x_nodeset, local.x_nodeset_dyn, local.x_nodeset_tpu)

  etc_dir = abspath("${path.module}/etc")

  bucket_path = format("%s/%s", data.google_storage_bucket.this.url, local.bucket_dir)

  slurm_control_host_port = coalesce(var.slurm_control_host_port, "6818")

  google_app_cred_path = var.google_app_cred_path != null ? abspath(var.google_app_cred_path) : null
  slurm_bin_dir        = var.slurm_bin_dir != null ? abspath(var.slurm_bin_dir) : null
  slurm_log_dir        = var.slurm_log_dir != null ? abspath(var.slurm_log_dir) : null

  munge_mount = var.enable_hybrid ? {
    server_ip     = lookup(var.munge_mount, "server_ip", coalesce(var.slurm_control_addr, var.slurm_control_host))
    remote_mount  = lookup(var.munge_mount, "remote_mount", "/etc/munge/")
    fs_type       = lookup(var.munge_mount, "fs_type", "nfs")
    mount_options = lookup(var.munge_mount, "mount_options", "")
  } : null

  output_dir  = can(coalesce(var.output_dir)) ? abspath(var.output_dir) : abspath(".")
  install_dir = can(coalesce(var.install_dir)) ? abspath(var.install_dir) : local.output_dir
}

resource "google_storage_bucket_object" "config" {
  bucket  = data.google_storage_bucket.this.name
  name    = "${local.bucket_dir}/config.yaml"
  content = yamlencode(local.config)
}

resource "google_storage_bucket_object" "parition_config" {
  for_each = { for p in var.partitions : p.partition_name => p }

  bucket  = data.google_storage_bucket.this.name
  name    = "${local.bucket_dir}/partition_configs/${each.key}.yaml"
  content = yamlencode(each.value)
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
}

data "archive_file" "slurm_gcp_devel_zip" {
  output_path = "${local.build_dir}/${local.slurm_gcp_devel_zip}"
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

locals {
  bad_chars = "/[^a-zA-Z0-9-_]/"

  controller_script_files = {
    for x in local.controller_startup_scripts
    : format("slurm-controller-script-%s", replace(basename(x.filename), local.bad_chars, "_")) => x.content
  }

  nodeset_script_files = { for x in flatten([
    for nodeset, scripts in var.nodeset_startup_scripts
    : [for s in scripts
      : {
        content = s.content,
      name = format("slurm-nodeset-%s-script-%s", nodeset, replace(basename(s.filename), local.bad_chars, "_")) }
  ]]) : x.name => x.content }

  login_script_files = { for x in flatten([
    for group, scripts in var.login_startup_scripts
    : [for s in scripts
      : {
        content = s.content,
      name = format("slurm-login-%s-script-%s", group, replace(basename(s.filename), local.bad_chars, "_")) }
  ]]) : x.name => x.content }
}

resource "google_storage_bucket_object" "controller_startup_scripts" {
  for_each = local.controller_script_files

  bucket  = var.bucket_name
  name    = "${local.bucket_dir}/${each.key}"
  content = each.value
}

resource "google_storage_bucket_object" "nodeset_startup_scripts" {
  for_each = local.nodeset_script_files

  bucket  = var.bucket_name
  name    = "${local.bucket_dir}/${each.key}"
  content = each.value
}

resource "google_storage_bucket_object" "login_startup_scripts" {
  for_each = local.login_script_files

  bucket  = var.bucket_name
  name    = "${local.bucket_dir}/${each.key}"
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

  chs_prolog     = var.enable_chs_gpu_health_check_prolog ? local.chs_gpu_health_check : []
  ext_prolog     = var.enable_external_prolog_epilog ? local.external_prolog : []
  prolog_scripts = concat(local.chs_prolog, local.ext_prolog, var.prolog_scripts)

  chs_epilog     = var.enable_chs_gpu_health_check_epilog ? local.chs_gpu_health_check : []
  ext_epilog     = var.enable_external_prolog_epilog ? local.external_epilog : []
  epilog_scripts = concat(local.chs_epilog, local.ext_epilog, var.epilog_scripts)

  controller_startup_scripts = var.enable_external_prolog_epilog ? concat(local.setup_external, var.controller_startup_scripts) : var.controller_startup_scripts
}
