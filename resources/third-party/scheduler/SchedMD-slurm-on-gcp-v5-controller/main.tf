/**
 * Copyright 2021 Google LLC
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

module "slurm_controller_instance" {
  source = "git::https://gitlab.com/SchedMD/slurm-gcp.git//terraform/modules/slurm_controller_instance?ref=dev-v5"

  count = 1

  access_config         = var.access_config
  slurm_cluster_name    = var.slurm_cluster_name
  instance_template     = module.slurm_controller_template[0].self_link
  project_id            = var.project_id
  region                = var.region
  subnetwork            = var.subnetwork_self_link
  zone                  = var.zone
  static_ips            = var.static_ips
  cgroup_conf_tpl       = var.cgroup_conf_tpl
  cloud_parameters      = var.cloud_parameters
  cloudsql              = var.cloudsql
  controller_d          = var.controller_d
  compute_d             = var.compute_d
  enable_devel          = var.enable_devel
  enable_bigquery_load  = var.enable_bigquery_load
  epilog_d              = var.epilog_d
  login_network_storage = var.login_network_storage
  network_storage       = var.network_storage
  partitions            = var.partition
  prolog_d              = var.prolog_d
  slurmdbd_conf_tpl     = var.slurmdbd_conf_tpl
  slurm_conf_tpl        = var.slurm_conf_tpl
}

module "slurm_controller_template" {
  source = "git::https://gitlab.com/SchedMD/slurm-gcp.git//terraform/modules/slurm_instance_template?ref=dev-v5"

  count = 1

  additional_disks         = var.additional_disks
  can_ip_forward           = var.can_ip_forward
  slurm_cluster_name       = var.slurm_cluster_name
  disable_smt              = var.disable_smt
  disk_auto_delete         = var.disk_auto_delete
  disk_labels              = var.disk_labels
  disk_size_gb             = var.disk_size_gb
  disk_type                = var.disk_type
  enable_confidential_vm   = var.enable_confidential_vm
  enable_oslogin           = var.enable_oslogin
  enable_shielded_vm       = var.enable_shielded_vm
  gpu                      = var.gpu
  labels                   = var.labels
  machine_type             = var.machine_type
  metadata                 = var.metadata
  min_cpu_platform         = var.min_cpu_platform
  network_ip               = var.network_ip != null ? var.network_ip : ""
  on_host_maintenance      = var.on_host_maintenance
  preemptible              = var.preemptible
  project_id               = var.project_id
  region                   = var.region
  service_account          = var.service_account
  shielded_instance_config = var.shielded_instance_config
  slurm_instance_role      = "controller"
  source_image_family      = var.source_image_family
  source_image_project     = var.source_image_project
  source_image             = var.source_image
  network                  = var.network
  subnetwork_project       = var.subnetwork_project
  subnetwork               = var.subnetwork_self_link
  tags                     = concat([var.slurm_cluster_name], var.tags)
}

