/**
 * Copyright 2025 Google LLC
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
  template_args = {
    local_mount   = var.local_mount
    mount_options = var.mount_options == "" ? "" : "-o ${var.mount_options}"
    remote_mount  = var.remote_mount
    server_ip     = var.server_ip
    service_name  = "weka-mount${replace(var.local_mount, "/", "-")}"
  }
  mount_script = templatefile("${path.module}/templates/mount-weka.sh.tftpl", local.template_args)

  mount_runner_ansible = {
    type = "ansible-local"
    content = templatefile(
      "${path.module}/templates/mount-weka.yaml.tftpl",
      merge(
        local.template_args,
        { mount_weka_script = local.mount_script }
      )
    )
    destination = "mount_filesystem${replace(var.local_mount, "/", "_")}.yaml"
  }

  client_install_runner = {
    type        = "ansible-local"
    content     = templatefile("${path.module}/templates/install-weka-client.yaml.tftpl", local.template_args)
    destination = "install_filesystem${replace(var.local_mount, "/", "_")}.yaml"
  }
}

# currently WEKA mounts are not compatible with network_storage logic, as WEKA volumes needs to be mounted by
# systemd script and not /etc/fstab entry, as the mount command needs to have network configuration which may change
# between restarts
# 
#output "network_storage" {
#  description = "Describes a remote network storage to be mounted by fs-tab."
#  value = {
#    server_ip             = var.server_ip
#    remote_mount          = var.remote_mount
#    local_mount           = var.local_mount
#    fs_type               = var.fs_type
#    mount_options         = var.mount_options
#    client_install_runner = local.client_install_runner
#    mount_runner          = local.mount_runner
#  }
#}
#
output "client_install_runner" {
  description = "Ansible runner that performs client installation needed to use file system."
  value       = local.client_install_runner
}

output "mount_runner" {
  description = "Ansible runner that mounts the file system."
  value       = local.mount_runner_ansible
}
