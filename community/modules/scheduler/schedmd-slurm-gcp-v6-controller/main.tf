/**
 * Copyright 2023 Google LLC
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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v6-controller", ghpc_role = "scheduler" })
}

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

  # Since deployment name may be used to create a cluster name, we remove any invalid character from the beginning
  # Also, slurm imposed a lot of restrictions to this name, so we format it to an acceptable string
  tmp_cluster_name   = substr(replace(lower(var.deployment_name), "/^[^a-z]*|[^a-z0-9]/", ""), 0, 10)
  slurm_cluster_name = coalesce(var.slurm_cluster_name, local.tmp_cluster_name)

}

locals {
  synt_suffix       = substr(md5("${var.project_id}${var.deployment_name}"), 0, 5)
  synth_bucket_name = "${local.slurm_cluster_name}${local.synt_suffix}"
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

data "google_compute_default_service_account" "default" {
  project = var.project_id
}

locals { # controller_instance_config
  additional_disks = [
    for ad in var.additional_disks : {
      disk_name    = ad.disk_name
      device_name  = ad.device_name
      disk_type    = ad.disk_type
      disk_size_gb = ad.disk_size_gb
      disk_labels  = merge(ad.disk_labels, local.labels)
      auto_delete  = ad.auto_delete
      boot         = ad.boot
    }
  ]

  controller_instance_config = {
    disk_auto_delete = var.disk_auto_delete
    disk_labels      = merge(var.disk_labels, local.labels)
    disk_size_gb     = var.disk_size_gb
    disk_type        = var.disk_type
    additional_disks = local.additional_disks

    can_ip_forward = var.can_ip_forward
    disable_smt    = var.disable_smt

    enable_confidential_vm   = var.enable_confidential_vm
    enable_public_ip         = !var.disable_controller_public_ips
    enable_oslogin           = var.enable_oslogin
    enable_shielded_vm       = var.enable_shielded_vm
    shielded_instance_config = var.shielded_instance_config

    gpu               = one(local.guest_accelerator)
    instance_template = var.instance_template
    labels            = local.labels
    machine_type      = var.machine_type
    metadata          = var.metadata
    min_cpu_platform  = var.min_cpu_platform

    on_host_maintenance = var.on_host_maintenance
    preemptible         = var.preemptible
    region              = var.region
    zone                = var.zone

    service_account = coalesce(var.service_account, {
      email  = data.google_compute_default_service_account.default.email
      scopes = ["https://www.googleapis.com/auth/cloud-platform"]
    })

    source_image_family  = local.source_image_family             # requires source_image_logic.tf
    source_image_project = local.source_image_project_normalized # requires source_image_logic.tf
    source_image         = local.source_image                    # requires source_image_logic.tf

    static_ip      = try(var.static_ips[0], null)
    bandwidth_tier = var.bandwidth_tier

    subnetwork         = var.subnetwork_self_link
    subnetwork_project = var.subnetwork_project

    tags = var.tags
  }
}

module "slurm_cluster" {
  source = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster?ref=6.1.2"

  project_id         = var.project_id
  slurm_cluster_name = local.slurm_cluster_name
  region             = var.region

  create_bucket = false
  bucket_name   = var.create_bucket ? module.bucket[0].name : var.bucket_name
  bucket_dir    = var.bucket_dir

  controller_instance_config = local.controller_instance_config

  enable_login = var.enable_login
  login_nodes  = var.login_nodes

  nodeset     = var.nodeset
  nodeset_tpu = var.nodeset_tpu

  partitions = var.partitions

  enable_devel           = var.enable_devel
  enable_debug_logging   = var.enable_debug_logging
  extra_logging_flags    = var.extra_logging_flags
  enable_cleanup_compute = var.enable_cleanup_compute
  enable_bigquery_load   = var.enable_bigquery_load
  cloud_parameters       = var.cloud_parameters
  disable_default_mounts = var.disable_default_mounts

  network_storage       = var.network_storage
  login_network_storage = var.network_storage

  slurmdbd_conf_tpl = var.slurmdbd_conf_tpl
  slurm_conf_tpl    = var.slurm_conf_tpl
  cgroup_conf_tpl   = var.cgroup_conf_tpl

  controller_startup_scripts         = local.ghpc_startup_script_controller
  controller_startup_scripts_timeout = var.controller_startup_scripts_timeout
  login_startup_scripts              = local.ghpc_startup_script_login
  login_startup_scripts_timeout      = var.login_startup_scripts_timeout
  compute_startup_scripts            = local.ghpc_startup_script_compute
  compute_startup_scripts_timeout    = var.compute_startup_scripts_timeout

  prolog_scripts = var.prolog_scripts
  epilog_scripts = var.epilog_scripts
  cloudsql       = var.cloudsql

  depends_on = [module.bucket]
}
