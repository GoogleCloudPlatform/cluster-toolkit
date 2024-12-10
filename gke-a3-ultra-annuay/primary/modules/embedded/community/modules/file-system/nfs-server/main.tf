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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "nfs-server", ghpc_role = "file-system" })
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

locals {
  name          = var.name != null ? var.name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}"
  server_ip     = google_compute_instance.compute_instance.network_interface[0].network_ip
  fs_type       = "nfs"
  mount_options = "defaults,hard,intr"
  install_nfs_client_runners = [for mount in var.local_mounts :
    {
      "type"        = "shell"
      "source"      = "${path.module}/scripts/install-nfs-client.sh"
      "destination" = "install-nfs${replace(mount, "/", "_")}.sh"
    }
  ]
  mount_runners = [for mount in var.local_mounts :
    {
      "type"        = "shell"
      "source"      = "${path.module}/scripts/mount.sh"
      "args"        = "\"${local.server_ip}\" \"/exports${mount}\" \"${mount}\" \"${local.fs_type}\" \"${local.mount_options}\""
      "destination" = "mount${replace(mount, "/", "_")}.sh"
    }
  ]
  ansible_mount_runner = {
    "type"        = "ansible-local"
    "source"      = "${path.module}/scripts/mount.yaml"
    "destination" = "mount.yaml"
  }
}

data "google_compute_default_service_account" "default" {}

resource "google_compute_disk" "attached_disk" {
  project = var.project_id
  name    = "${local.name}-nfs-instance-disk"
  size    = var.disk_size
  type    = var.type
  zone    = var.zone
  labels  = local.labels
}

data "google_compute_image" "compute_image" {
  family  = try(var.instance_image.family, null)
  name    = try(var.instance_image.name, null)
  project = var.instance_image.project
}

resource "null_resource" "image" {
  triggers = {
    name    = try(var.instance_image.name, null),
    family  = try(var.instance_image.family, null),
    project = var.instance_image.project
  }
}

resource "google_compute_disk" "boot_disk" {
  project = var.project_id

  name   = "${local.name}-boot-disk"
  image  = data.google_compute_image.compute_image.self_link
  labels = local.labels
  zone   = var.zone

  lifecycle {
    replace_triggered_by = [null_resource.image]
    ignore_changes = [
      image
    ]
  }
}

resource "google_compute_instance" "compute_instance" {
  project      = var.project_id
  name         = "${local.name}-nfs-instance"
  zone         = var.zone
  machine_type = var.machine_type

  boot_disk {
    auto_delete = var.auto_delete_disk
    source      = google_compute_disk.boot_disk.self_link
    device_name = google_compute_disk.boot_disk.name
  }

  attached_disk {
    source      = google_compute_disk.attached_disk.id
    device_name = "attached_disk"
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

  labels = local.labels
}
