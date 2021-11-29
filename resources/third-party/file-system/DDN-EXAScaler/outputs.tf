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

output "private_addresses" {
  description = "Private IP addresses for all instances."
  value       = module.ddn_exascaler.private_addresses
}

output "ssh_console" {
  description = "Instructions to ssh into the instances."
  value       = module.ddn_exascaler.ssh_console
}

output "mount_command" {
  description = "Command to mount the file system."
  value       = module.ddn_exascaler.mount_command
}

output "http_console" {
  description = "HTTP address to access the system web console."
  value       = module.ddn_exascaler.http_console
}

output "network_storage" {
  description = "Describes a EXAScaler system to be mounted by other systems."
  value = {
    server_ip     = split(":", split(" ", module.ddn_exascaler.mount_command)[3])[0]
    remote_mount  = var.fsname
    local_mount   = var.local_mount != null ? var.local_mount : format("/mnt/%s", var.fsname)
    fs_type       = "lustre"
    mount_options = ""
  }
}
