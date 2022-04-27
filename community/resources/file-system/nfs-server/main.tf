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

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

locals {
  name = var.name != null ? var.name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}"
  install_nfs_client_runner = {
    "type"        = "shell"
    "content"     = "${path.module}/scripts/install-nfs-client.sh"
    "destination" = "install-nfs.sh"
  }
  mount_runner = {
    "type"        = "ansible-local"
    "source"      = "${path.module}/scripts/mount.yaml"
    "destination" = "mount.yaml"
  }
}

data "google_compute_default_service_account" "default" {}

resource "google_compute_disk" "attached_disk" {
  name   = "${local.name}-nfs-instance-disk"
  image  = var.image
  size   = var.disk_size
  type   = var.type
  zone   = var.zone
  labels = var.labels
}

resource "google_compute_instance" "compute_instance" {
  name         = "${local.name}-nfs-instance"
  zone         = var.zone
  machine_type = var.machine_type

  boot_disk {
    auto_delete = var.auto_delete_disk
    initialize_params {
      image = var.image
    }
  }

  attached_disk {
    source = google_compute_disk.attached_disk.id
  }

  network_interface {
    network    = var.network_self_link
    subnetwork = var.subnetwork_self_link
  }

  service_account {
    email  = var.service_account == null ? data.google_compute_default_service_account.default.email : var.service_account
    scopes = var.scopes
  }

  metadata                = var.metadata
  metadata_startup_script = templatefile("${path.module}/scripts/install-nfs-server.sh.tpl", { local_mounts = var.local_mounts })

  labels = var.labels
}
