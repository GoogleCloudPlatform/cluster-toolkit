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
  // TODO: encahnce labels with role and module name
  labels = var.labels
}

locals {

  additional_disks = var.additional_disks # TODO: add labels

  nodeset = {
    node_count_static      = var.node_count_static
    node_count_dynamic_max = var.node_count_dynamic_max
    node_conf              = var.node_conf
    nodeset_name           = var.name

    additional_disks = local.additional_disks
    bandwidth_tier   = var.bandwidth_tier
    can_ip_forward   = var.can_ip_forward
    disable_smt      = !var.enable_smt

    disk_auto_delete       = var.disk_auto_delete
    disk_labels            = merge(local.labels, var.disk_labels)
    disk_size_gb           = var.disk_size_gb
    disk_type              = var.disk_type
    enable_confidential_vm = var.enable_confidential_vm
    enable_placement       = var.enable_placement
    enable_public_ip       = var.enable_public_ip
    enable_oslogin         = var.enable_oslogin
    enable_shielded_vm     = var.enable_shielded_vm
    gpu                    = one(local.guest_accelerator)
    instance_template      = var.instance_template
    labels                 = local.labels
    machine_type           = var.machine_type
    metadata               = var.metadata
    min_cpu_platform       = var.min_cpu_platform
    network_tier           = var.network_tier
    on_host_maintenance    = var.on_host_maintenance
    preemptible            = var.preemptible
    region                 = var.region
    service_account = var.service_account != null ? var.service_account : {
      email  = data.google_compute_default_service_account.default.email
      scopes = ["https://www.googleapis.com/auth/cloud-platform"]
    }
    shielded_instance_config = var.shielded_instance_config
    source_image_family      = local.source_image_family             # requires source_image_logic.tf
    source_image_project     = local.source_image_project_normalized # requires source_image_logic.tf
    source_image             = local.source_image                    # requires source_image_logic.tf

    # subnetwork_project - omit as we use subnewtork self_link
    subnetwork         = var.subnetwork_self_link
    spot               = var.enable_spot_vm
    tags               = var.tags
    termination_action = try(var.spot_instance_config.termination_action, null)
    zones              = var.zones
    zone_target_shape  = var.zone_target_shape

  }
}

data "google_compute_default_service_account" "default" {
  project = var.project_id
}