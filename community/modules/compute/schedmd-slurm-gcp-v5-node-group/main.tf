/**
 * Copyright 2022 Google LLC
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

  node_group = {
    # Group Definition
    group_name             = var.name
    node_count_dynamic_max = var.node_count_dynamic_max
    node_count_static      = var.node_count_static
    node_conf              = var.node_conf

    # Template By Definition
    additional_disks         = var.additional_disks
    bandwidth_tier           = var.bandwidth_tier
    can_ip_forward           = var.can_ip_forward
    disable_smt              = !var.enable_smt
    disk_auto_delete         = var.disk_auto_delete
    disk_labels              = merge(var.labels, var.disk_labels)
    disk_size_gb             = var.disk_size_gb
    disk_type                = var.disk_type
    enable_confidential_vm   = var.enable_confidential_vm
    enable_oslogin           = var.enable_oslogin
    enable_shielded_vm       = var.enable_shielded_vm
    gpu                      = var.gpu
    labels                   = var.labels
    machine_type             = var.machine_type
    metadata                 = var.metadata
    min_cpu_platform         = var.min_cpu_platform
    on_host_maintenance      = var.on_host_maintenance
    preemptible              = var.preemptible
    shielded_instance_config = var.shielded_instance_config
    source_image_family      = lookup(var.instance_image, "family", "")
    source_image_project     = lookup(var.instance_image, "project", "")
    source_image             = lookup(var.instance_image, "name", "")
    tags                     = var.tags
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
