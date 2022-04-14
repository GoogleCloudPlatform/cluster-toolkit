// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

source "googlecompute" "hpc_centos_7" {
  project_id              = var.project_id
  image_name              = "example-${formatdate("YYYYMMDD't'hhmmss'z'", timestamp())}"
  image_family            = "example-v1"
  machine_type            = var.machine_type
  disk_size               = var.disk_size
  omit_external_ip        = var.omit_external_ip
  use_internal_ip         = var.omit_external_ip
  subnetwork              = var.subnetwork
  source_image            = var.source_image
  source_image_family     = var.source_image_family
  source_image_project_id = var.source_image_project_id
  ssh_username            = var.ssh_username
  tags                    = var.tags
  use_iap                 = var.use_iap
  use_os_login            = var.use_os_login
  zone                    = var.zone
}

build {
  name    = "example"
  sources = ["sources.googlecompute.hpc_centos_7"]

  provisioner "shell" {
    execute_command = "sudo -H sh -c '{{ .Vars }} {{ .Path }}'"
    script          = "scripts/install_ansible.sh"
  }

  # this will end up installing custom roles/collections from ansible-galaxy
  # under /home/packer until we modify /etc/ansible/ansible.cfg to identify
  # a directory that will remain after Packer is complete
  dynamic "provisioner" {
    # using labels this way effectively creates 'provisioner "ansible-local"' blocks
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
