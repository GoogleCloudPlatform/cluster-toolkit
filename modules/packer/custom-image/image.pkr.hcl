# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  metadata             = var.startup_script == null ? null : { startup-script = var.startup_script }
  no_shell_scripts     = length(var.shell_scripts) == 0
  no_ansible_playbooks = length(var.ansible_playbooks) == 0
  no_provisioners      = local.no_shell_scripts && local.no_ansible_playbooks
  communicator         = local.no_provisioners ? "none" : "ssh"
  use_iap              = local.no_provisioners ? false : var.use_iap
  image_family         = var.image_family != null ? var.image_family : var.deployment_name

  machine_vals                = split("-", var.machine_type)
  machine_family              = local.machine_vals[0]
  gpu_attached                = contains(["a2"], local.machine_family) || var.accelerator_type != null
  on_host_maintenance_default = local.gpu_attached ? "TERMINATE" : "MIGRATE"

  on_host_maintenance = (
    var.on_host_maintenance != null
    ? var.on_host_maintenance
    : local.on_host_maintenance_default
  )
}

source "googlecompute" "toolkit_image" {
  communicator            = local.communicator
  project_id              = var.project_id
  image_name              = "${local.image_family}-${formatdate("YYYYMMDD't'hhmmss'z'", timestamp())}"
  image_family            = local.image_family
  machine_type            = var.machine_type
  accelerator_type        = var.accelerator_type
  accelerator_count       = var.accelerator_count
  on_host_maintenance     = local.on_host_maintenance
  disk_size               = var.disk_size
  omit_external_ip        = var.omit_external_ip
  use_internal_ip         = var.omit_external_ip
  subnetwork              = var.subnetwork_name
  network_project_id      = var.network_project_id
  scopes                  = var.scopes
  source_image            = var.source_image
  source_image_family     = var.source_image_family
  source_image_project_id = var.source_image_project_id
  ssh_username            = var.ssh_username
  tags                    = var.tags
  use_iap                 = local.use_iap
  use_os_login            = var.use_os_login
  zone                    = var.zone
  labels                  = var.labels
  metadata                = local.metadata
  startup_script_file     = var.startup_script_file
  wrap_startup_script     = var.wrap_startup_script
}

build {
  name    = var.deployment_name
  sources = ["sources.googlecompute.toolkit_image"]

  # using dynamic blocks to create provisioners ensures that there are no
  # provisioner blocks when none are provided and we can use the none
  # communicator when using startup-script

  # provisioner "shell" blocks
  dynamic "provisioner" {
    labels   = ["shell"]
    for_each = var.shell_scripts
    content {
      execute_command = "sudo -H sh -c '{{ .Vars }} {{ .Path }}'"
      script          = provisioner.value
    }
  }

  # provisioner "ansible-local" blocks
  # this installs custom roles/collections from ansible-galaxy in /home/packer
  # which will be removed at the end; consider modifying /etc/ansible/ansible.cfg
  dynamic "provisioner" {
    labels   = ["ansible-local"]
    for_each = var.ansible_playbooks
    content {
      playbook_file   = provisioner.value.playbook_file
      galaxy_file     = provisioner.value.galaxy_file
      extra_arguments = provisioner.value.extra_arguments
    }
  }

  post-processor "manifest" {}
}
