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

  have_template = var.instance_template != null && var.instance_template != ""
}

# TEMPLATE
module "slurm_controller_template" {
  source = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_instance_template?ref=6.2.0"
  count  = local.have_template ? 0 : 1

  project_id          = var.project_id
  region              = var.region
  slurm_instance_role = "controller"
  slurm_cluster_name  = local.slurm_cluster_name
  labels              = local.labels

  disk_auto_delete = var.disk_auto_delete
  disk_labels      = merge(var.disk_labels, local.labels)
  disk_size_gb     = var.disk_size_gb
  disk_type        = var.disk_type
  additional_disks = local.additional_disks

  bandwidth_tier    = var.bandwidth_tier
  slurm_bucket_path = module.slurm_files.slurm_bucket_path
  can_ip_forward    = var.can_ip_forward
  disable_smt       = var.disable_smt

  enable_confidential_vm   = var.enable_confidential_vm
  enable_oslogin           = var.enable_oslogin
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config

  gpu = one(local.guest_accelerator)

  machine_type     = var.machine_type
  metadata         = var.metadata
  min_cpu_platform = var.min_cpu_platform

  # network_ip = TODO: add support for network_ip
  on_host_maintenance = var.on_host_maintenance
  preemptible         = var.preemptible

  service_account = coalesce(var.service_account, {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  })

  source_image_family  = local.source_image_family             # requires source_image_logic.tf
  source_image_project = local.source_image_project_normalized # requires source_image_logic.tf
  source_image         = local.source_image                    # requires source_image_logic.tf

  # spot = TODO: add support for spot (?)
  subnetwork_project = var.subnetwork_project
  subnetwork         = var.subnetwork_self_link

  tags = concat([local.slurm_cluster_name], var.tags)
  # termination_action = TODO: add support for termination_action (?)
}

# INSTANCE
module "slurm_controller_instance" {
  source = "github.com/SchedMD/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_controller_instance?ref=6.2.0"

  slurm_cluster_name = local.slurm_cluster_name
  project_id         = var.project_id
  region             = var.region
  zone               = var.zone

  # cloudsql = TODO: add supoort for cloudsql
  instance_template = local.have_template ? var.instance_template : module.slurm_controller_template[0].self_link

  enable_public_ip = !var.disable_controller_public_ips

  # network_tier = TODO: add support for network_tier
  static_ips = var.static_ips
  subnetwork = var.subnetwork_self_link

  enable_cleanup_compute = var.enable_cleanup_compute
  depends_on             = [module.slurm_files]
}
