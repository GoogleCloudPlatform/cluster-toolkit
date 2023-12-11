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

data "google_compute_default_service_account" "default" {
  project = var.project_id
}

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


  login_node = {
    name_prefix      = var.name_prefix
    disk_auto_delete = var.disk_auto_delete
    disk_labels      = merge(var.disk_labels, local.labels)
    disk_size_gb     = var.disk_size_gb
    disk_type        = var.disk_type
    additional_disks = local.additional_disks

    can_ip_forward = var.can_ip_forward
    disable_smt    = var.disable_smt

    enable_confidential_vm   = var.enable_confidential_vm
    enable_public_ip         = !var.disable_login_public_ips
    enable_oslogin           = var.enable_oslogin
    enable_shielded_vm       = var.enable_shielded_vm
    shielded_instance_config = var.shielded_instance_config

    gpu                 = one(local.guest_accelerator)
    instance_template   = var.instance_template
    labels              = local.labels
    machine_type        = var.machine_type
    metadata            = var.metadata
    min_cpu_platform    = var.min_cpu_platform
    num_instances       = var.num_instances
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

    static_ips     = var.static_ips
    bandwidth_tier = var.bandwidth_tier

    subnetwork_project = var.subnetwork_project
    subnetwork         = var.subnetwork_self_link

    tags = var.tags
  }
}
