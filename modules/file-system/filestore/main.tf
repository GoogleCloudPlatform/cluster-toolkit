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

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

locals {
  timeouts      = var.filestore_tier == "HIGH_SCALE_SSD" ? [1] : []
  server_ip     = google_filestore_instance.filestore_instance.networks[0].ip_addresses[0]
  remote_mount  = format("/%s", google_filestore_instance.filestore_instance.file_shares[0].name)
  fs_type       = "nfs"
  mount_options = "defaults,_netdev"

  install_nfs_client_runner = {
    "type"        = "shell"
    "source"      = "${path.module}/scripts/install-nfs-client.sh"
    "destination" = "install-nfs${replace(var.local_mount, "/", "_")}.sh"
  }
  mount_runner = {
    "type"        = "shell"
    "source"      = "${path.module}/scripts/mount.sh"
    "args"        = "\"${local.server_ip}\" \"${local.remote_mount}\" \"${var.local_mount}\" \"${local.fs_type}\" \"${local.mount_options}\""
    "destination" = "mount${replace(var.local_mount, "/", "_")}.sh"
  }
}

resource "google_filestore_instance" "filestore_instance" {
  project = var.project_id

  name     = var.name != null ? var.name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}"
  location = var.filestore_tier == "ENTERPRISE" ? var.region : var.zone
  tier     = var.filestore_tier

  file_shares {
    capacity_gb = var.size_gb
    name        = var.filestore_share_name
  }

  labels = var.labels

  networks {
    network      = var.network_name
    connect_mode = var.connect_mode
    modes        = ["MODE_IPV4"]
  }

  dynamic "timeouts" {
    for_each = local.timeouts
    content {
      create = "1h"
      update = "1h"
      delete = "1h"
    }
  }

}
