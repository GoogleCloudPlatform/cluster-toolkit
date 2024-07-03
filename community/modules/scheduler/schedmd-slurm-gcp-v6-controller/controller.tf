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

  service_account_email = coalesce(var.service_account_email, data.google_compute_default_service_account.default.email)

  # can't rely on `email=null` as it's used to instantiate `cloudsql_secret_accessor`
  service_account = {
    email  = local.service_account_email
    scopes = var.service_account_scopes
  }
}

# INSTANCE TEMPLATE
module "slurm_controller_template" {
  source = "github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/slurm_instance_template?ref=6.5.10"

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
  disable_smt       = !var.enable_smt

  enable_confidential_vm   = var.enable_confidential_vm
  enable_oslogin           = var.enable_oslogin
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config

  gpu = one(local.guest_accelerator)

  machine_type     = var.machine_type
  metadata         = merge(var.metadata, local.universe_domain)
  min_cpu_platform = var.min_cpu_platform

  # network_ip = TODO: add support for network_ip
  on_host_maintenance = var.on_host_maintenance
  preemptible         = var.preemptible
  service_account     = local.service_account

  source_image_family  = local.source_image_family             # requires source_image_logic.tf
  source_image_project = local.source_image_project_normalized # requires source_image_logic.tf
  source_image         = local.source_image                    # requires source_image_logic.tf

  # spot = TODO: add support for spot (?)
  subnetwork = var.subnetwork_self_link

  tags = concat([local.slurm_cluster_name], var.tags)
  # termination_action = TODO: add support for termination_action (?)
}

# INSTANCE
locals {
  # TODO: add support for proper access_config
  access_config = {
    nat_ip       = null
    network_tier = null
  }
}

module "slurm_controller_instance" {
  source = "github.com/GoogleCloudPlatform/slurm-gcp.git//terraform/slurm_cluster/modules/_slurm_instance?ref=6.5.10"

  access_config       = var.enable_controller_public_ips ? [local.access_config] : []
  add_hostname_suffix = false
  hostname            = "${local.slurm_cluster_name}-controller"
  instance_template   = module.slurm_controller_template.self_link

  project_id          = var.project_id
  region              = var.region
  slurm_cluster_name  = local.slurm_cluster_name
  slurm_instance_role = "controller"
  static_ips          = var.static_ips
  subnetwork          = var.subnetwork_self_link
  zone                = var.zone
  metadata            = var.metadata

  labels = merge(local.labels, local.files_cs_labels)

  depends_on = [
    # Ensure that controller is destroyed BEFORE doing cleanup
    null_resource.cleanup_compute[0],
  ]
}

# SECRETS: CLOUDSQL
resource "google_secret_manager_secret" "cloudsql" {
  count = var.cloudsql != null ? 1 : 0

  secret_id = "${local.slurm_cluster_name}-slurm-secret-cloudsql"

  replication {
    auto {}
  }

  labels = {
    slurm_cluster_name = local.slurm_cluster_name
  }
}

resource "google_secret_manager_secret_version" "cloudsql_version" {
  count = var.cloudsql != null ? 1 : 0

  secret      = google_secret_manager_secret.cloudsql[0].id
  secret_data = jsonencode(var.cloudsql)
}

resource "google_secret_manager_secret_iam_member" "cloudsql_secret_accessor" {
  count = var.cloudsql != null ? 1 : 0

  secret_id = google_secret_manager_secret.cloudsql[0].id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.service_account.email}"
}
