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

  state_disk = var.controller_state_disk != null ? [{
    source      = google_compute_region_disk.disk[0].id
    device_name = google_compute_region_disk.disk[0].name
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
  replica_zones         = [data.google_compute_zones.zones.names[0], data.google_compute_zones.zones.names[length(data.google_compute_zones.zones.names) - 1]]
}

data "google_project" "controller_project" {
  project_id = local.controller_project_id
}

resource "google_compute_address" "controllers_ips" {
  count        = var.nb_controllers
  name         = "${local.slurm_cluster_name}-controller-${count.index}-ip"
  region       = var.region
  subnetwork   = var.subnetwork_self_link
  address_type = "INTERNAL"
}
data "google_compute_subnetwork" "subnet" {
  self_link = var.subnetwork_self_link
}
resource "google_dns_managed_zone" "dns-managed-zone" {
  name       = "dns-managed-zone"
  dns_name   = "${local.slurm_cluster_name}.internal."
  visibility = "private"

  private_visibility_config {
    networks {
      network_url = data.google_compute_subnetwork.subnet.network
    }
  }
}

resource "google_dns_record_set" "record" {
  count        = var.nb_controllers
  name         = "controller${count.index}.${google_dns_managed_zone.dns-managed-zone.dns_name}"
  project      = var.project_id
  type         = "A"
  ttl          = 30
  managed_zone = google_dns_managed_zone.dns-managed-zone.name
  rrdatas      = [google_compute_address.controllers_ips[count.index].address]
}
locals {
  slurm_control_hosts = [for name in google_dns_record_set.record[*].name : trim(name, ".")]
}
data "google_compute_zones" "zones" {
  region = var.region
}
resource "google_compute_region_disk" "disk" {
  count         = var.controller_state_disk != null ? 1 : 0
  project       = var.project_id
  name          = "${local.slurm_cluster_name}-controller-save-regiondisk"
  type          = var.controller_state_disk.type
  region        = var.region
  size          = var.controller_state_disk.size
  access_mode   = "READ_WRITE_MANY"
  replica_zones = local.replica_zones
}

# INSTANCE TEMPLATE
module "slurm_controller_template" {
  count  = var.nb_controllers
  source = "../../internal/slurm-gcp/instance_template"

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
  bandwidth_tier             = var.bandwidth_tier
  slurm_bucket_path          = module.slurm_files.slurm_bucket_path
  can_ip_forward             = var.can_ip_forward
  advanced_machine_features  = var.advanced_machine_features
  resource_manager_tags      = var.resource_manager_tags

  enable_confidential_vm   = var.enable_confidential_vm
  enable_oslogin           = var.enable_oslogin
  enable_shielded_vm       = var.enable_shielded_vm
  shielded_instance_config = var.shielded_instance_config

  gpu = one(module.gpu.guest_accelerator)

  machine_type     = var.machine_type
  metadata         = merge(local.metadata, { "hostname" = local.slurm_control_hosts[count.index], })
  min_cpu_platform = var.min_cpu_platform

  on_host_maintenance = var.on_host_maintenance
  preemptible         = var.preemptible
  service_account     = local.service_account

  source_image_family  = local.source_image_family             # requires source_image_logic.tf
  source_image_project = local.source_image_project_normalized # requires source_image_logic.tf
  source_image         = local.source_image                    # requires source_image_logic.tf

  subnetwork = var.subnetwork_self_link
  network_ip = length(var.static_ips) == 0 ? google_compute_address.controllers_ips[count.index].address : var.static_ips[count.index]

  tags = concat([local.slurm_cluster_name], var.tags)
  # termination_action = TODO: add support for termination_action (?)
}

# HEALTH CHECK CONTROLLERS

data "google_netblock_ip_ranges" "hcs" {
  range_type = "health-checkers"
}
resource "google_compute_firewall" "ingress_google" {
  name                    = "${local.slurm_cluster_name}-allow-google"
  network                 = data.google_compute_subnetwork.subnet.network
  source_ranges           = data.google_netblock_ip_ranges.hcs.cidr_blocks
  target_service_accounts = [local.service_account.email]
  direction               = "INGRESS"
  allow {
    protocol = "tcp"
    ports    = ["6819-6830", "6842", "8642", "8080"]
  }
}
resource "google_compute_health_check" "controller" {
  name = "${local.slurm_cluster_name}-ctrl-health-check"

  timeout_sec         = 10
  check_interval_sec  = 30
  healthy_threshold   = 1
  unhealthy_threshold = 3

  http_health_check {
    port = 8080
  }
  log_config {
    enable = true
  }
}

# MIG CONTROLLERS
resource "google_compute_region_instance_group_manager" "controller_mig" {

  name                      = "${local.slurm_cluster_name}-controller-mig"
  base_instance_name        = "${local.slurm_cluster_name}-controller"
  region                    = var.region
  target_size               = var.nb_controllers
  distribution_policy_zones = local.replica_zones

  dynamic "version" {
    for_each = module.slurm_controller_template

    content {
      name              = "controller-${version.key}"
      instance_template = version.value.self_link

      dynamic "target_size" {
        for_each = toset(version.key > 0 ? [1] : [])
        content {
          fixed = 1
        }
      }
    }
  }
  wait_for_instances = false
  auto_healing_policies {
    health_check      = google_compute_health_check.controller.id
    initial_delay_sec = 300
  }

  update_policy {
    type                         = "PROACTIVE"
    instance_redistribution_type = "PROACTIVE"
    minimal_action               = "REPLACE"
    replacement_method           = "RECREATE"
    max_surge_fixed              = 0
    max_unavailable_fixed        = 4
  }
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
