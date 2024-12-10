/**
 * Copyright 2023 Google LLC
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

output "network_storage" {
  description = "Describes a remote network storage to be mounted by fs-tab."
  value = {
    remote_mount          = local.name
    local_mount           = var.local_mount
    fs_type               = "gcsfuse"
    mount_options         = var.mount_options
    server_ip             = ""
    client_install_runner = local.client_install_runner
    mount_runner          = local.mount_runner
  }
}

locals {
  client_install_runner = {
    "type"        = "shell"
    "content"     = file("${path.module}/scripts/install-gcs-fuse.sh")
    "destination" = "install-gcsfuse${replace(var.local_mount, "/", "_")}.sh"
  }

  mount_runner = {
    "type"        = "shell"
    "destination" = "mount_gcs${replace(var.local_mount, "/", "_")}.sh"
    "args"        = "\"not-used\" \"${local.name}\" \"${var.local_mount}\" \"gcsfuse\" \"${var.mount_options}\""
    "content"     = file("${path.module}/scripts/mount.sh")
  }
}

output "client_install_runner" {
  description = "Runner that performs client installation needed to use gcs fuse."
  value       = local.client_install_runner
}

output "mount_runner" {
  description = "Runner that mounts the cloud storage bucket with gcs fuse."
  value       = local.mount_runner
}

output "gcs_bucket_path" {
  description = "The gsutil bucket path with format of `gs://<bucket-name>`."
  # cannot use resource attribute, will cause lookup failure in startup-script
  value = "gs://${local.name}"

  # needed to make sure bucket contents are deleted before bucket
  depends_on = [
    google_storage_bucket.bucket
  ]
}

output "gcs_bucket_name" {
  description = "Bucket name."
  value       = google_storage_bucket.bucket.name
}
