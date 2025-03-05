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

module "gpu" {
  source = "../../../../modules/internal/gpu-definition"

  machine_type      = var.machine_type
  guest_accelerator = var.guest_accelerator
}

locals {
  additional_disks = [
    for ad in var.additional_disks : {
      disk_name                  = ad.disk_name
      device_name                = ad.device_name
      disk_type                  = ad.disk_type
      disk_size_gb               = ad.disk_size_gb
      disk_labels                = merge(ad.disk_labels, local.labels)
      auto_delete                = ad.auto_delete
      boot                       = ad.boot
      disk_resource_manager_tags = ad.disk_resource_manager_tags
    }
  ]

  state_disk = (var.controller_state_disk != null && !var.enable_hybrid) ? [{
    source      = google_compute_disk.controller_disk[0].name
    device_name = google_compute_disk.controller_disk[0].name
    disk_labels = null
    auto_delete = false
    boot        = false
  }] : []

  synth_def_sa_email = "${data.google_project.controller_project.number}-compute@developer.gserviceaccount.com"

  service_account = {
    email  = coalesce(var.service_account_email, local.synth_def_sa_email)
    scopes = var.service_account_scopes
  }

  disable_automatic_updates_metadata = var.allow_automatic_updates ? {} : { google_disable_automatic_updates = "TRUE" }

  metadata = merge(
    local.disable_automatic_updates_metadata,
    var.metadata,
    local.universe_domain
  )

  controller_project_id = coalesce(var.controller_project_id, var.project_id)
}

data "google_project" "controller_project" {
  project_id = local.controller_project_id
}

resource "google_compute_disk" "controller_disk" {
  count = (var.controller_state_disk != null && !var.enable_hybrid) ? 1 : 0

  project = local.controller_project_id
  name    = "${local.slurm_cluster_name}-controller-save"
  type    = var.controller_state_disk.type
  size    = var.controller_state_disk.size
  zone    = var.zone
}

# INSTANCE TEMPLATE
module "slurm_controller_template" {
  source = "../../internal/slurm-gcp/instance_template"
  count  = var.enable_hybrid ? 0 : 1

  project_id          = local.controller_project_id
  region              = var.region
  slurm_instance_role = "controller"
  slurm_cluster_name  = local.slurm_cluster_name
  labels              = local.labels

  disk_auto_delete           = var.disk_auto_delete
  disk_labels                = merge(var.disk_labels, local.labels)
  disk_size_gb               = var.disk_size_gb
  disk_type                  = var.disk_type
  disk_resource_manager_tags = var.disk_resource_manager_tags
  additional_disks           = concat(local.additional_disks, local.state_disk)

  bandwidth_tier            = var.bandwidth_tier
  slurm_bucket_path         = module.slurm_files.slurm_bucket_path
  can_ip_forward            = var.can_ip_forward
  advanced_machine_features = var.advanced_machine_features
  resource_manager_tags     = var.resource_manager_tags

  enable_confidential_vm   = var.enable_confidential_vm
  enable_oslogin           = var.enable_oslogin
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config

  gpu = one(module.gpu.guest_accelerator)

  machine_type     = var.machine_type
  metadata         = local.metadata
  min_cpu_platform = var.min_cpu_platform

  on_host_maintenance = var.on_host_maintenance
  preemptible         = var.preemptible
  service_account     = local.service_account

  source_image_family  = local.source_image_family             # requires source_image_logic.tf
  source_image_project = local.source_image_project_normalized # requires source_image_logic.tf
  source_image         = local.source_image                    # requires source_image_logic.tf

  subnetwork = var.subnetwork_self_link

  tags = concat([local.slurm_cluster_name], var.tags)
  # termination_action = TODO: add support for termination_action (?)
}

# INSTANCE
resource "google_compute_instance_from_template" "controller" {
  provider = google-beta

  name                     = "${local.slurm_cluster_name}-controller"
  count                    = var.enable_hybrid ? 0 : 1
  project                  = local.controller_project_id
  zone                     = var.zone
  source_instance_template = module.slurm_controller_template[0].self_link
  # Due to https://github.com/hashicorp/terraform-provider-google/issues/21693
  # we have to explicitly override instance labels instead of inheriting them from template.
  labels = module.slurm_controller_template[0].labels

  allow_stopping_for_update = true

  # Can't rely on template to specify nics due to usage of static_ip
  network_interface {
    dynamic "access_config" {
      for_each = var.enable_controller_public_ips ? ["unit"] : []
      content {
        nat_ip       = null
        network_tier = null
      }
    }
    network_ip = length(var.static_ips) == 0 ? "" : var.static_ips[0]
    subnetwork = var.subnetwork_self_link
  }

  dynamic "network_interface" {
    for_each = var.controller_network_attachment != null ? [1] : []
    content {
      network_attachment = var.controller_network_attachment
    }
  }
}

moved {
  from = module.slurm_controller_instance.google_compute_instance_from_template.slurm_instance[0]
  to   = google_compute_instance_from_template.controller[0]
}

# SECRETS: CLOUDSQL
resource "google_secret_manager_secret" "cloudsql" {
  count = var.cloudsql != null ? 1 : 0

  secret_id = "${local.slurm_cluster_name}-slurm-secret-cloudsql"
  project   = var.project_id

  replication {
    dynamic "auto" {
      for_each = length(var.cloudsql.user_managed_replication) == 0 ? [1] : []
      content {}
    }
    dynamic "user_managed" {
      for_each = length(var.cloudsql.user_managed_replication) == 0 ? [] : [1]
      content {
        dynamic "replicas" {
          for_each = nonsensitive(var.cloudsql.user_managed_replication)
          content {
            location = replicas.value.location
            dynamic "customer_managed_encryption" {
              for_each = compact([replicas.value.kms_key_name])
              content {
                kms_key_name = customer_managed_encryption.value
              }
            }
          }
        }
      }
    }
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
