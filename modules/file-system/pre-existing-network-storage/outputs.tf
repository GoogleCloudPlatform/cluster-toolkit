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
  # Client Install
  ddn_lustre_client_install_script = templatefile(
    "${path.module}/templates/ddn_exascaler_luster_client_install.tftpl",
    {
      server_ip    = split("@", var.server_ip)[0]
      remote_mount = var.remote_mount
      local_mount  = var.local_mount
    }
  )

  install_scripts = {
    "lustre" = local.ddn_lustre_client_install_script
  }

  # Mounting
  ddn_lustre_mount_cmd = "mount -t ${var.fs_type} ${var.server_ip}:/${var.remote_mount} ${var.local_mount}"
  mount_commands = {
    "lustre" = local.ddn_lustre_mount_cmd
  }

  mount_script = <<-EOT
    #!/bin/bash
    findmnt --source ${var.server_ip}:/${var.remote_mount} --target ${var.local_mount} &> /dev/null
    if [[ $? != 0 ]]; then
      echo "Mounting --source ${var.server_ip}:/${var.remote_mount} --target ${var.local_mount}"
      mkdir -p ${var.local_mount}
      ${lookup(local.mount_commands, var.fs_type, "exit 1")}
    else
      echo "Skipping mounting source: ${var.server_ip}:/${var.remote_mount}, already mounted to target:${var.local_mount}"
    fi
  EOT
}

output "client_install_runner" {
  description = "Runner that performs client installation needed to use file system."
  value = lookup(local.install_scripts, var.fs_type, null) == null ? null : {
    "type"        = "shell"
    "content"     = lookup(local.install_scripts, var.fs_type, "")
    "destination" = "install_filesystem_client${replace(var.local_mount, "/", "_")}.sh"
  }
}

output "mount_runner" {
  description = "Runner that mounts the file system."
  value = lookup(local.mount_commands, var.fs_type, null) == null ? null : {
    "type"        = "shell"
    "content"     = local.mount_script
    "destination" = "mount_filesystem${replace(var.local_mount, "/", "_")}.sh"
  }
}
