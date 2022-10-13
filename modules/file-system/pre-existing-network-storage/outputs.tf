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
  description = "Describes a remote network storage to be mounted by fs-tab."
  value = {
    server_ip     = var.server_ip
    remote_mount  = var.remote_mount
    local_mount   = var.local_mount
    fs_type       = var.fs_type
    mount_options = var.mount_options
  }
}

locals {
  remote_mount_with_slash = length(regexall("^/.*", var.remote_mount)) > 0 ? var.remote_mount : format("/%s", var.remote_mount)
  # Client Install
  ddn_lustre_client_install_script = templatefile(
    "${path.module}/templates/ddn_exascaler_luster_client_install.tftpl",
    {
      server_ip    = split("@", var.server_ip)[0]
      remote_mount = local.remote_mount_with_slash
      local_mount  = var.local_mount
    }
  )

  install_scripts = {
    "lustre" = local.ddn_lustre_client_install_script
  }

  # Mounting
  ddn_lustre_mount_cmd = "mount -t ${var.fs_type} ${var.server_ip}:${local.remote_mount_with_slash} ${var.local_mount}"
  mount_commands = {
    "lustre" = local.ddn_lustre_mount_cmd
  }
  mount_command = lookup(local.mount_commands, var.fs_type, "exit 1")
}

output "client_install_runner" {
  description = "Runner that performs client installation needed to use file system."
  value = {
    "type"        = "shell"
    "content"     = lookup(local.install_scripts, var.fs_type, "echo 'skipping: client_install_runner not yet supported for ${var.fs_type}'")
    "destination" = "install_filesystem_client${replace(var.local_mount, "/", "_")}.sh"
  }
}

output "mount_runner" {
  description = "Runner that mounts the file system."
  value = {
    "type"        = "shell"
    "destination" = "mount_filesystem${replace(var.local_mount, "/", "_")}.sh"
    "args"        = "\"${var.server_ip}\" \"${local.remote_mount_with_slash}\" \"${var.local_mount}\" \"${local.mount_command}\""
    "content" = (
      lookup(local.mount_commands, var.fs_type, null) == null ?
      "echo 'skipping: mount_runner not yet supported for ${var.fs_type}'" :
      file("${path.module}/scripts/mount.sh")
    )
  }
}
