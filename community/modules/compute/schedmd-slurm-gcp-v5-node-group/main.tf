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
  labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v5-node-group", ghpc_role = "compute" })
}

locals {
  disable_automatic_updates_metadata = var.allow_automatic_updates ? {} : { google_disable_automatic_updates = "TRUE" }

  metadata = merge(
    local.disable_automatic_updates_metadata,
    var.metadata
  )

  enable_public_ip_access_config = var.disable_public_ips ? [] : [{ nat_ip = null, network_tier = null }]
  access_config                  = length(var.access_config) == 0 ? local.enable_public_ip_access_config : var.access_config

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

  node_group = {
    # Group Definition
    group_name             = var.name
    node_count_dynamic_max = var.node_count_dynamic_max
    node_count_static      = var.node_count_static
    node_conf              = var.node_conf

    # Template By Definition
    additional_disks         = local.additional_disks
    additional_networks      = var.additional_networks
    bandwidth_tier           = var.bandwidth_tier
    can_ip_forward           = var.can_ip_forward
    disable_smt              = !var.enable_smt
    disk_auto_delete         = var.disk_auto_delete
    disk_labels              = merge(local.labels, var.disk_labels)
    disk_size_gb             = var.disk_size_gb
    disk_type                = var.disk_type
    enable_confidential_vm   = var.enable_confidential_vm
    enable_oslogin           = var.enable_oslogin
    enable_shielded_vm       = var.enable_shielded_vm
    gpu                      = one(local.guest_accelerator)
    labels                   = local.labels
    machine_type             = var.machine_type
    maintenance_interval     = var.maintenance_interval
    metadata                 = local.metadata
    min_cpu_platform         = var.min_cpu_platform
    on_host_maintenance      = var.on_host_maintenance
    preemptible              = var.preemptible
    reservation_name         = var.reservation_name
    shielded_instance_config = var.shielded_instance_config
    source_image_family      = local.source_image_family             # requires source_image_logic.tf
    source_image_project     = local.source_image_project_normalized # requires source_image_logic.tf
    source_image             = local.source_image                    # requires source_image_logic.tf
    tags                     = var.tags
    access_config            = local.access_config
    service_account = var.service_account != null ? var.service_account : {
      email  = data.google_compute_default_service_account.default.email
      scopes = ["https://www.googleapis.com/auth/cloud-platform"]
    }

    # Spot VM settings
    enable_spot_vm       = var.enable_spot_vm
    spot_instance_config = var.spot_instance_config

    # Template By Source
    instance_template = var.instance_template
  }
}

data "google_compute_default_service_account" "default" {
  project = var.project_id
}
