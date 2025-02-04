/**
 * Copyright 2024 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "parallelstore", ghpc_role = "file-system" })
}

locals {
  fs_type          = "daos"
  server_ip        = ""
  remote_mount     = ""
  id               = var.name != null ? var.name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}"
  access_points    = jsonencode(google_parallelstore_instance.instance.access_points)
  destination_path = var.import_destination_path == null ? "/" : var.import_destination_path

  client_install_runner = {
    "type"        = "shell"
    "source"      = "${path.module}/scripts/install-daos-client.sh"
    "destination" = "install_daos_client.sh"
  }

  mount_runner = {
    "type" = "shell"
    "content" = templatefile("${path.module}/templates/mount-daos.sh.tftpl", {
      access_points     = local.access_points
      daos_agent_config = var.daos_agent_config
      dfuse_environment = var.dfuse_environment
      local_mount       = var.local_mount
      mount_options     = join(" ", [for opt in split(",", var.mount_options) : "--${opt}"])
    })
    "destination" = "mount_filesystem${replace(var.local_mount, "/", "_")}.sh"
  }
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_parallelstore_instance" "instance" {
  project                = var.project_id
  instance_id            = local.id
  location               = var.zone
  capacity_gib           = var.size_gb
  network                = var.network_id
  file_stripe_level      = var.file_stripe
  directory_stripe_level = var.directory_stripe

  labels = local.labels

  depends_on = [var.private_vpc_connection_peering]
}

resource "null_resource" "hydration" {
  count = var.import_gcs_bucket_uri != null ? 1 : 0

  depends_on = [resource.google_parallelstore_instance.instance]
  provisioner "local-exec" {
    command = "curl -X POST   -H \"Content-Type: application/json\"   -H \"Authorization: Bearer $(gcloud auth print-access-token)\"   -d '{\"source_gcs_bucket\": {\"uri\":\"${var.import_gcs_bucket_uri}\"}, \"destination_parallelstore\": {\"path\":\"${local.destination_path}\"}}'   https://parallelstore.googleapis.com/v1beta/projects/${var.project_id}/locations/${var.zone}/instances/${local.id}:importData"
  }
}
