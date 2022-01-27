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
}

data "google_compute_image" "compute_image" {
  family  = var.instance_image.family
  project = var.instance_image.project
}

resource "google_compute_disk" "boot_disk" {
  count = var.instance_count

  name = var.name_prefix != null ? (
    "${var.name_prefix}-boot-disk-${count.index}") : (
  "${var.deployment_name}-boot-disk-${count.index}")
  image  = data.google_compute_image.compute_image.self_link
  type   = var.disk_type
  size   = var.disk_size_gb
  labels = var.labels
}

resource "google_compute_instance" "compute_vm" {
  count = var.instance_count

  depends_on = [var.network_self_link, var.network_storage]

  name         = var.name_prefix != null ? "${var.name_prefix}-${count.index}" : "${var.deployment_name}-${count.index}"
  machine_type = var.machine_type
  zone         = var.zone

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

    network = var.network_self_link
  }

  dynamic "service_account" {
    for_each = var.service_account == null ? [] : [var.service_account]
    content {
      email  = lookup(service_account.value, "email", null)
      scopes = lookup(service_account.value, "scopes", null)
    }
  }

  metadata = merge(local.network_storage, local.startup_script, var.metadata)
}
