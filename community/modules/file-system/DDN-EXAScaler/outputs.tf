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

output "private_addresses" {
  description = "Private IP addresses for all instances."
  value       = module.ddn_exascaler.private_addresses
}

output "ssh_console" {
  description = "Instructions to ssh into the instances."
  value       = module.ddn_exascaler.ssh_console
}

output "client_config_script" {
  description = "Script that will install DDN EXAScaler lustre client. The machine running this script must be on the same network & subnet as the EXAScaler."
  value       = module.ddn_exascaler.client_config
}

output "install_ddn_lustre_client_runner" {
  description = "Runner that encapsulates the `client_config_script` output on this module."
  value       = local.client_install_runner
}

locals {
  client_install_runner = {
    "type"        = "shell"
    "content"     = module.ddn_exascaler.client_config
    "destination" = "install_ddn_lustre_client.sh"
  }

  # Mount command provided by DDN does not support custom local mount
  split_mount_cmd               = split(" ", module.ddn_exascaler.mount_command)
  split_mount_cmd_wo_mountpoint = slice(local.split_mount_cmd, 0, length(local.split_mount_cmd) - 1)
  mount_cmd                     = "${join(" ", local.split_mount_cmd_wo_mountpoint)} ${var.local_mount}"
  mount_cmd_w_mkdir             = "mkdir -p ${var.local_mount} && ${local.mount_cmd}"
  mount_runner = {
    "type"        = "shell"
    "content"     = local.mount_cmd_w_mkdir
    "destination" = "mount-ddn-lustre.sh"
  }
}

output "mount_command" {
  description = "Command to mount the file system. `client_config_script` must be run first."
  value       = local.mount_cmd_w_mkdir
}

output "mount_runner" {
  description = "Runner to mount the DDN EXAScaler Lustre file system"
  value       = local.mount_runner
}

output "http_console" {
  description = "HTTP address to access the system web console."
  value       = module.ddn_exascaler.http_console
}

output "network_storage" {
  description = "Describes a EXAScaler system to be mounted by other systems."
  value = {
    server_ip             = split(":", split(" ", module.ddn_exascaler.mount_command)[3])[0]
    remote_mount          = length(regexall("^/.*", var.fsname)) > 0 ? var.fsname : format("/%s", var.fsname)
    local_mount           = var.local_mount != null ? var.local_mount : format("/mnt/%s", var.fsname)
    fs_type               = "lustre"
    mount_options         = ""
    client_install_runner = local.client_install_runner
    mount_runner          = local.mount_runner
  }
  depends_on = [
    module.ddn_exascaler
  ]
}
