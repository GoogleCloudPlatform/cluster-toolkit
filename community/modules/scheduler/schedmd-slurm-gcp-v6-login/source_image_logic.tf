/**
 * Copyright 2023 Google LLC
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
  # Currently supported images and projects
  known_project_families = {
    schedmd-slurm-public = [
      "slurm-gcp-6-8-debian-11",
      "slurm-gcp-6-8-hpc-rocky-linux-8",
      "slurm-gcp-6-8-ubuntu-2004-lts",
      "slurm-gcp-6-8-ubuntu-2204-lts-arm64"
    ]
  }

  # This approach to "hacking" the project name allows a chain of Terraform
  # calls to set the instance source_image (boot disk) with a "relative
  # resource name" that passes muster with VPC Service Control rules
  #
  # https://github.com/terraform-google-modules/terraform-google-vm/blob/735bd415fc5f034d46aa0de7922e8fada2327c0c/modules/instance_template/main.tf#L28
  # https://cloud.google.com/apis/design/resource_names#relative_resource_name
  source_image_project_normalized = (can(var.instance_image.family) ?
    "projects/${data.google_compute_image.slurm.project}/global/images/family" :
    "projects/${data.google_compute_image.slurm.project}/global/images"
  )
  source_image_family = can(var.instance_image.family) ? data.google_compute_image.slurm.family : ""
  source_image        = can(var.instance_image.name) ? data.google_compute_image.slurm.name : ""
}

data "google_compute_image" "slurm" {
  family  = try(var.instance_image.family, null)
  name    = try(var.instance_image.name, null)
  project = var.instance_image.project

  lifecycle {
    precondition {
      condition     = length(regexall("^projects/.+?/global/images/family$", var.instance_image.project)) == 0
      error_message = "The \"project\" field in var.instance_image no longer supports a long-form ending in \"family\". Specify only the project ID."
    }

    postcondition {
      condition     = var.instance_image_custom || contains(keys(local.known_project_families), self.project)
      error_message = <<-EOD
      Images in project ${self.project} are not published by SchedMD. Images must be created by compatible releases of the Terraform and Packer modules following the guidance at https://goo.gle/hpc-slurm-images. Set var.instance_image_custom to true to silence this error and acknowledge that you are using a compatible image.
      EOD
    }
    postcondition {
      condition     = !contains(keys(local.known_project_families), self.project) || try(contains(local.known_project_families[self.project], self.family), false)
      error_message = <<-EOD
      Image family ${self.family} published by SchedMD in project ${self.project} is not compatible with this release of the Terraform Slurm modules. Select from known compatible releases:
      ${join("\n", [for p in try(local.known_project_families[self.project], []) : "\t\"${p}\""])}
      EOD
    }
    postcondition {
      condition     = var.disk_size_gb >= self.disk_size_gb
      error_message = "'disk_size_gb: ${var.disk_size_gb}' is smaller than the image size (${self.disk_size_gb}GB), please increase the blueprint disk size"
    }
    postcondition {
      # Condition needs to check the suffix of the license, as prefix contains an API version which can change.
      # Example license value: https://www.googleapis.com/compute/v1/projects/cloud-hpc-image-public/global/licenses/hpc-vm-image-feature-disable-auto-updates
      condition     = var.allow_automatic_updates || anytrue([for license in self.licenses : endswith(license, "/projects/cloud-hpc-image-public/global/licenses/hpc-vm-image-feature-disable-auto-updates")])
      error_message = "Disabling automatic updates is not supported with the selected VM image.  More information: https://cloud.google.com/compute/docs/instances/create-hpc-vm#disable_automatic_updates"
    }
  }
}
