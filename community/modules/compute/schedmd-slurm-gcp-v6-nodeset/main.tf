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
  labels = merge(var.labels, { ghpc_module = "schedmd-slurm-gcp-v6-nodeset", ghpc_role = "compute" })
}

locals {
  name = substr(replace(var.name, "/[^a-z0-9]/", ""), 0, 6)

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

  public_access_config = var.disable_public_ips ? [] : [{ nat_ip = null, network_tier = null }]
  access_config        = length(var.access_config) == 0 ? local.public_access_config : var.access_config

  nodeset = {
    node_count_static      = var.node_count_static
    node_count_dynamic_max = var.node_count_dynamic_max
    node_conf              = var.node_conf
    nodeset_name           = local.name

    disk_auto_delete = var.disk_auto_delete
    disk_labels      = merge(local.labels, var.disk_labels)
    disk_size_gb     = var.disk_size_gb
    disk_type        = var.disk_type
    additional_disks = local.additional_disks

    bandwidth_tier = var.bandwidth_tier
    can_ip_forward = var.can_ip_forward
    disable_smt    = !var.enable_smt

    enable_confidential_vm = var.enable_confidential_vm
    enable_placement       = var.enable_placement
    enable_oslogin         = var.enable_oslogin
    enable_shielded_vm     = var.enable_shielded_vm
    gpu                    = one(local.guest_accelerator)

    instance_template = var.instance_template
    labels            = local.labels
    machine_type      = var.machine_type
    metadata          = var.metadata
    min_cpu_platform  = var.min_cpu_platform

    on_host_maintenance      = var.on_host_maintenance
    preemptible              = var.preemptible
    region                   = var.region
    service_account          = var.service_account
    shielded_instance_config = var.shielded_instance_config
    source_image_family      = local.source_image_family             # requires source_image_logic.tf
    source_image_project     = local.source_image_project_normalized # requires source_image_logic.tf
    source_image             = local.source_image                    # requires source_image_logic.tf
    subnetwork_self_link     = var.subnetwork_self_link
    additional_networks      = var.additional_networks
    access_config            = local.access_config
    tags                     = var.tags
    spot                     = var.enable_spot_vm
    termination_action       = try(var.spot_instance_config.termination_action, null)
    reservation_name         = var.reservation_name

    zones             = toset(concat([var.zone], tolist(var.zones)))
    zone_target_shape = var.zone_target_shape
  }
}
