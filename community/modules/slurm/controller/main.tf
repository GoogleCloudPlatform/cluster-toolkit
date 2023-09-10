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
  # TODO: encahnce labels with role and module name
  labels = var.labels
}


locals {
  additional_disks = var.additional_disks # TODO: add labels

  controller = {
    additional_disks = local.additional_disks
    bandwidth_tier   = var.bandwidth_tier
    can_ip_forward   = var.can_ip_forward
    enable_smt       = var.enable_smt

    disk_auto_delete = var.disk_auto_delete
    disk_labels      = merge(local.labels, var.disk_labels)
    disk_size_gb     = var.disk_size_gb
    disk_type        = var.disk_type

    enable_confidential_vm   = var.enable_confidential_vm
    enable_public_ip         = var.enable_public_ip
    enable_oslogin           = var.enable_oslogin
    enable_shielded_vm       = var.enable_shielded_vm
    guest_accelerator        = local.guest_accelerator
    instance_template        = var.instance_template
    labels                   = local.labels
    machine_type             = var.machine_type
    metadata                 = var.metadata
    min_cpu_platform         = var.min_cpu_platform
    network_ip               = var.network_ip
    network_tier             = var.network_tier
    on_host_maintenance      = var.on_host_maintenance
    preemptible              = var.preemptible
    service_account          = var.service_account
    shielded_instance_config = var.shielded_instance_config
    instance_image           = var.instance_image
    enable_spot_vm           = var.enable_spot_vm
    static_ip                = var.static_ip
    subnetwork_self_link     = var.subnetwork_self_link
    tags                     = var.tags
    spot_instance_config     = var.spot_instance_config
    zone                     = var.zone
  }
}
