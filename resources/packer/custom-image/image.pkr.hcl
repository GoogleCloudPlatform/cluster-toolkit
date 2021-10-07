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
  machine_type            = "n2d-standard-4"
  subnetwork              = var.subnetwork
  source_image_family     = "hpc-centos-7"
  source_image_project_id = ["cloud-hpc-image-public"]
  ssh_username            = "packer"
  tags                    = ["builder"]
  zone                    = "us-central1-a"
}

build {
  name    = "example"
  sources = ["sources.googlecompute.hpc_centos_7"]

  dynamic "provisioner" {
    # labels adds each element of below list to the right of dynamic block
    # i.e. this creates multiple ansible provisioners
    labels   = ["ansible"]
    for_each = var.ansible_playbook_files

    content {
      playbook_file   = provisioner.value
      extra_arguments = var.ansible_extra_arguments
      user            = var.ansible_user
    }
  }

  post-processor "manifest" {}
}
