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

output "network_storage" {
  description = "Describes a filestore instance."
  value = {
    server_ip             = local.server_ip
    remote_mount          = local.remote_mount
    local_mount           = var.local_mount
    fs_type               = local.fs_type
    mount_options         = local.mount_options
    client_install_runner = local.install_nfs_client_runner
    mount_runner          = local.mount_runner
  }
}

output "install_nfs_client" {
  description = "Script for installing NFS client"
  value       = file("${path.module}/scripts/install-nfs-client.sh")
}

output "install_nfs_client_runner" {
  description = "Runner to install NFS client using the startup-script module"
  value       = local.install_nfs_client_runner
}

output "mount_runner" {
  description = <<-EOT
  Runner to mount the file-system using an ansible playbook. The startup-script
  module will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-fs-id.mount_runner)
  ...
  EOT
  value       = local.mount_runner
}

output "filestore_id" {
  description = "An identifier for the resource with format `projects/{{project}}/locations/{{location}}/instances/{{name}}`"
  value       = google_filestore_instance.filestore_instance.id
}

output "capacity_gb" {
  description = "File share capacity in GiB."
  value       = google_filestore_instance.filestore_instance.file_shares[0].capacity_gb
}
