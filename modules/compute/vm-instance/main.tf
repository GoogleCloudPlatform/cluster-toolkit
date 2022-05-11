/**
 * Copyright 2021 Google LLC
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
  startup_script = var.startup_script != null ? (
  { startup-script = var.startup_script }) : {}
  network_storage = var.network_storage != null ? (
  { network_storage = jsonencode(var.network_storage) }) : {}

  resource_prefix = var.name_prefix != null ? var.name_prefix : var.deployment_name

  enable_gvnic  = var.bandwidth_tier != "not_enabled"
  enable_tier_1 = var.bandwidth_tier == "tier_1_enabled"

  # use Spot provisioning model (now GA) over older preemptible model
  provisioning_model = var.spot ? "SPOT" : null

  # compact_placement : true when placement policy is provided and collocation set; false if unset
  compact_placement = try(var.placement_policy.collocation, null) != null
  # both of these must be false if either compact placement or preemptible/spot instances are used
  automatic_restart                  = local.compact_placement || var.spot ? false : null
  on_host_maintenance_from_placement = local.compact_placement || var.spot ? "TERMINATE" : "MIGRATE"

  on_host_maintenance = (
    var.on_host_maintenance != null
    ? var.on_host_maintenance
    : local.on_host_maintenance_from_placement
  )
}

data "google_compute_image" "compute_image" {
  family  = var.instance_image.family
  project = var.instance_image.project
}

resource "google_compute_disk" "boot_disk" {
  project = var.project_id

  count = var.instance_count

  name   = "${local.resource_prefix}-boot-disk-${count.index}"
  image  = data.google_compute_image.compute_image.self_link
  type   = var.disk_type
  size   = var.disk_size_gb
  labels = var.labels
}

resource "google_compute_resource_policy" "placement_policy" {
  project = var.project_id

  count = var.placement_policy != null ? 1 : 0
  name  = "${local.resource_prefix}-vm-instance-placement"
  group_placement_policy {
    vm_count                  = var.placement_policy.vm_count
    availability_domain_count = var.placement_policy.availability_domain_count
    collocation               = var.placement_policy.collocation
  }
}

resource "google_compute_instance" "compute_vm" {
  project  = var.project_id
  provider = google-beta

  count = var.instance_count

  depends_on = [var.network_self_link, var.network_storage]

  name         = "${local.resource_prefix}-${count.index}"
  machine_type = var.machine_type
  zone         = var.zone

  resource_policies = google_compute_resource_policy.placement_policy[*].self_link

  labels = var.labels

  boot_disk {
    source      = google_compute_disk.boot_disk[count.index].self_link
    device_name = google_compute_disk.boot_disk[count.index].name
    auto_delete = true
  }

  network_interface {
    dynamic "access_config" {
      for_each = var.disable_public_ips == true ? [] : [1]
      content {}
    }

    network    = var.network_self_link
    subnetwork = var.subnetwork_self_link
    nic_type   = local.enable_gvnic ? "GVNIC" : null
  }

  network_performance_config {
    total_egress_bandwidth_tier = local.enable_tier_1 ? "TIER_1" : "DEFAULT"
  }

  dynamic "service_account" {
    for_each = var.service_account == null ? [] : [var.service_account]
    content {
      email  = lookup(service_account.value, "email", null)
      scopes = lookup(service_account.value, "scopes", null)
    }
  }

  guest_accelerator = var.guest_accelerator
  scheduling {
    on_host_maintenance = local.on_host_maintenance
    automatic_restart   = local.automatic_restart
    preemptible         = var.spot
    provisioning_model  = local.provisioning_model
  }

  advanced_machine_features {
    threads_per_core = var.threads_per_core
  }

  metadata = merge(local.network_storage, local.startup_script, var.metadata)
}
