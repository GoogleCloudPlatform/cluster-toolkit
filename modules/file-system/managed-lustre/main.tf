/**
 * Copyright 2026 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "managed-lustre", ghpc_role = "file-system" })
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

data "google_compute_network_peering" "private_peering" {
  name    = var.private_vpc_connection_peering
  network = var.network_self_link
}

locals {
  server_ip        = split(":", google_lustre_instance.lustre_instance.mount_point)[0]
  remote_mount     = split(":", google_lustre_instance.lustre_instance.mount_point)[1]
  fs_type          = "lustre"
  mount_options    = var.mount_options
  instance_id      = var.name != null ? var.name : "${var.deployment_name}-${random_id.resource_name_suffix.hex}"
  destination_path = "/"

  install_managed_lustre_client_runner = {
    "type"        = "shell"
    "source"      = "${path.module}/scripts/install-managed-lustre-client.sh"
    "destination" = "install-managed-lustre-client${replace(var.local_mount, "/", "_")}.sh"
    "args"        = var.gke_support_enabled ? "1" : "0"
  }
  mount_runner = {
    "type"        = "shell"
    "source"      = "${path.module}/scripts/mount.sh"
    "args"        = "\"${local.server_ip}\" \"${local.remote_mount}\" \"${var.local_mount}\" \"${local.fs_type}\" \"${local.mount_options}\""
    "destination" = "mount${replace(var.local_mount, "/", "_")}.sh"
  }

  bucket_count = try(length(data.google_storage_bucket.lustre_import_bucket), 0)
}

data "google_storage_bucket" "lustre_import_bucket" {
  count = try(length(var.import_gcs_bucket_uri) > 0, false) ? 1 : 0

  name = split("//", var.import_gcs_bucket_uri)[1]
}

resource "google_lustre_instance" "lustre_instance" {
  project = var.project_id

  description = var.description
  instance_id = local.instance_id
  location    = var.zone

  filesystem                  = var.remote_mount
  capacity_gib                = var.size_gib
  per_unit_storage_throughput = var.per_unit_storage_throughput

  labels  = local.labels
  network = var.network_id

  gke_support_enabled = var.gke_support_enabled

  timeouts {
    create = "1h"
    update = "1h"
    delete = "1h"
  }

  depends_on = [var.private_vpc_connection_peering, data.google_storage_bucket.lustre_import_bucket]

  lifecycle {
    precondition {
      condition     = data.google_compute_network_peering.private_peering.state == "ACTIVE"
      error_message = "The subnetwork that the lustre instance is hosted on must have private service access."
    }
  }

  provisioner "local-exec" {
    command     = <<EOF
      if [[ ${local.bucket_count} > 0 ]]; then
        curl -X POST \
             -H "Content-Type: application/json" \
             -H "Authorization: Bearer $(gcloud auth print-access-token)" \
             -d '{"gcsPath": {"uri":"${coalesce(var.import_gcs_bucket_uri, "gs://")}"}, "lustrePath": {"path":"${local.destination_path}"}}' \
             https://lustre.googleapis.com/v1/projects/${var.project_id}/locations/${var.zone}/instances/${local.instance_id}:importData
      fi
      EOF
    interpreter = ["bash", "-c"]
  }
}
