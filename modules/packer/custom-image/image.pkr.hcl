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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "custom-image", ghpc_role = "packer" })

  # construct a unique image name from the image family
  image_family       = var.image_family != null ? var.image_family : var.deployment_name
  image_name_default = "${local.image_family}-${formatdate("YYYYMMDD't'hhmmss'z'", timestamp())}"
  image_name         = var.image_name != null ? var.image_name : local.image_name_default

  # default to explicit var.communicator, otherwise in-order: ssh/winrm/none
  shell_script_communicator      = length(var.shell_scripts) > 0 ? "ssh" : ""
  ansible_playbook_communicator  = length(var.ansible_playbooks) > 0 ? "ssh" : ""
  powershell_script_communicator = length(var.windows_startup_ps1) > 0 ? "winrm" : ""
  communicator = coalesce(
    var.communicator,
    local.shell_script_communicator,
    local.ansible_playbook_communicator,
    local.powershell_script_communicator,
    "none"
  )

  # must not enable IAP when no communicator is in use
  use_iap = local.communicator == "none" ? false : var.use_iap

  # construct metadata from startup_script and metadata variables
  startup_script_metadata = var.startup_script == null ? {} : { startup-script = var.startup_script }

  linux_user_metadata = {
    block-project-ssh-keys = "TRUE"
    shutdown-script        = <<-EOT
      #!/bin/bash
      userdel -r ${var.ssh_username}
      sed -i '/${var.ssh_username}/d' /var/lib/google/google_users
    EOT
  }
  windows_packer_user = "packer_user"
  windows_user_metadata = {
    sysprep-specialize-script-cmd = "winrm quickconfig -quiet & net user /add ${local.windows_packer_user} & net localgroup administrators ${local.windows_packer_user} /add & winrm set winrm/config/service/auth @{Basic=\\\"true\\\"}"
    windows-shutdown-script-cmd   = <<-EOT
      net user /delete ${local.windows_packer_user}
      GCESysprep -no_shutdown
      EOT
  }
  user_metadata = local.communicator == "winrm" ? local.windows_user_metadata : local.linux_user_metadata

  # merge metadata such that var.metadata always overrides user management
  # metadata but always allow var.startup_script to override var.metadata
  metadata = merge(
    local.user_metadata,
    var.metadata,
    local.startup_script_metadata,
  )

  # determine best value for on_host_maintenance if not supplied by user
  machine_vals                = split("-", var.machine_type)
  machine_family              = local.machine_vals[0]
  gpu_attached                = contains(["a2", "g2"], local.machine_family) || var.accelerator_type != null
  on_host_maintenance_default = local.gpu_attached ? "TERMINATE" : "MIGRATE"
  on_host_maintenance = (
    var.on_host_maintenance != null
    ? var.on_host_maintenance
    : local.on_host_maintenance_default
  )

  accelerator_type = var.accelerator_type == null ? null : "projects/${var.project_id}/zones/${var.zone}/acceleratorTypes/${var.accelerator_type}"

  winrm_username = local.communicator == "winrm" ? "packer_user" : null
  winrm_insecure = local.communicator == "winrm" ? true : null
  winrm_use_ssl  = local.communicator == "winrm" ? true : null

  enable_integrity_monitoring = var.enable_shielded_vm && var.shielded_instance_config.enable_integrity_monitoring
  enable_secure_boot          = var.enable_shielded_vm && var.shielded_instance_config.enable_secure_boot
  enable_vtpm                 = var.enable_shielded_vm && var.shielded_instance_config.enable_vtpm

  image_licenses = [
    "projects/click-to-deploy-images/global/licenses/hpc-toolkit-vm-image"
  ]
}

source "googlecompute" "toolkit_image" {
  communicator                = local.communicator
  project_id                  = var.project_id
  image_name                  = local.image_name
  image_family                = local.image_family
  image_labels                = local.labels
  machine_type                = var.machine_type
  accelerator_type            = local.accelerator_type
  accelerator_count           = var.accelerator_count
  on_host_maintenance         = local.on_host_maintenance
  disk_size                   = var.disk_size
  disk_type                   = var.disk_type
  omit_external_ip            = var.omit_external_ip
  use_internal_ip             = var.omit_external_ip
  subnetwork                  = var.subnetwork_name
  network_project_id          = var.network_project_id
  scopes                      = var.scopes
  source_image                = var.source_image
  source_image_family         = var.source_image_family
  source_image_project_id     = var.source_image_project_id
  ssh_username                = var.ssh_username
  tags                        = var.tags
  use_iap                     = local.use_iap
  use_os_login                = var.use_os_login
  winrm_username              = local.winrm_username
  winrm_insecure              = local.winrm_insecure
  winrm_use_ssl               = local.winrm_use_ssl
  zone                        = var.zone
  labels                      = local.labels
  metadata                    = local.metadata
  startup_script_file         = var.startup_script_file
  wrap_startup_script         = var.wrap_startup_script
  state_timeout               = var.state_timeout
  image_storage_locations     = var.image_storage_locations
  enable_secure_boot          = local.enable_secure_boot
  enable_vtpm                 = local.enable_vtpm
  enable_integrity_monitoring = local.enable_integrity_monitoring
  image_licenses              = local.image_licenses
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

  # provisioner "powershell" blocks
  dynamic "provisioner" {
    labels   = ["powershell"]
    for_each = var.windows_startup_ps1
    content {
      inline = split("\n", provisioner.value)
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

  post-processor "manifest" {
    output     = var.manifest_file
    strip_path = true
    custom_data = {
      built-by = "cloud-hpc-toolkit"
    }
  }

  # if the jq command is present, this will print the image name to stdout
  # if jq is not present, this exits silently with code 0
  post-processor "shell-local" {
    inline = [
      "command -v jq > /dev/null || exit 0",
      "echo \"Image built: $(jq -r '.builds[-1].artifact_id' ${var.manifest_file} | cut -d ':' -f2)\"",
    ]
  }
}
