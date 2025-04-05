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
  #  mount_runner = {
  #    type        = "shell"
  #    content     = templatefile("${path.module}/templates/mount-weka.sh.tftpl", local.template_args)
  #    destination = "mount_filesystem${replace(var.local_mount, "/", "_")}.sh"
  #  }

  client_install_runner = {
    type        = "shell"
    content     = templatefile("${path.module}/templates/install-weka-client.sh.tftpl", local.template_args)
    destination = "install_filesystem${replace(var.local_mount, "/", "_")}.sh"
  }

  systemd_unit_service = {
    type        = "data"
    content     = templatefile("${path.module}/templates/weka.service.tftpl", local.template_args)
    destination = "/etc/systemd/system/${local.template_args.service_name}.service"
  }

  systemd_unit_service_script = {
    type        = "data"
    content     = templatefile("${path.module}/templates/mount-weka.sh.tftpl", local.template_args)
    destination = "/etc/${local.template_args.service_name}.sh"
  }

  systemd_unit_enable = {
    type        = "shell"
    content     = templatefile("${path.module}/templates/enable-weka.sh.tftpl", local.template_args)
    destination = "enable-weka-service.sh"
  }
}

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
#output "client_install_runner" {
#  description = "Runner that performs client installation needed to use file system."
#  value       = local.client_install_runner
#}
#
#output "mount_runner" {
#  description = "Runner that mounts the file system."
#  value       = local.mount_runner
#}

output "runners" {
  description = "All runners that"
  value = [
    local.client_install_runner,
    local.systemd_unit_service,
    local.systemd_unit_service_script,
    local.systemd_unit_enable
  ]
}
