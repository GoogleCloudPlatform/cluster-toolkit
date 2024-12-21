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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v6-login", ghpc_role = "scheduler" })
}

module "gpu" {
  source = "../../../../modules/internal/gpu-definition"

  machine_type      = var.machine_type
  guest_accelerator = var.guest_accelerator
}

locals {
  guest_accelerator = module.gpu.guest_accelerator

  disable_automatic_updates_metadata = var.allow_automatic_updates ? {} : { google_disable_automatic_updates = "TRUE" }

  metadata = merge(
    local.disable_automatic_updates_metadata,
    var.metadata
  )

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

  public_access_config = [{ nat_ip = null, network_tier = null }]

  service_account = {
    email  = var.service_account_email
    scopes = var.service_account_scopes
  }

  # lower, replace `_` with `-`, and remove any non-alphanumeric characters
  name_prefix = replace(
    replace(
      lower(var.name_prefix),
    "_", "-"),
  "/[^-a-z0-9]/", "")


  login_node = {
    name_prefix         = local.name_prefix
    disk_auto_delete    = var.disk_auto_delete
    disk_labels         = merge(var.disk_labels, local.labels)
    disk_size_gb        = var.disk_size_gb
    disk_type           = var.disk_type
    additional_disks    = local.additional_disks
    additional_networks = var.additional_networks

    can_ip_forward = var.can_ip_forward
    disable_smt    = !var.enable_smt

    enable_confidential_vm   = var.enable_confidential_vm
    access_config            = var.enable_login_public_ips ? local.public_access_config : []
    enable_oslogin           = var.enable_oslogin
    enable_shielded_vm       = var.enable_shielded_vm
    shielded_instance_config = var.shielded_instance_config

    gpu                 = one(local.guest_accelerator)
    labels              = local.labels
    machine_type        = var.machine_type
    metadata            = local.metadata
    min_cpu_platform    = var.min_cpu_platform
    num_instances       = var.num_instances
    on_host_maintenance = var.on_host_maintenance
    preemptible         = var.preemptible
    region              = var.region
    zone                = var.zone

    service_account = local.service_account

    source_image_family  = local.source_image_family             # requires source_image_logic.tf
    source_image_project = local.source_image_project_normalized # requires source_image_logic.tf
    source_image         = local.source_image                    # requires source_image_logic.tf

    static_ips     = var.static_ips
    bandwidth_tier = var.bandwidth_tier

    subnetwork = var.subnetwork_self_link
    tags       = var.tags
  }
}
