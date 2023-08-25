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
    "schedmd-slurm-public" = ["slurm-gcp-5-7-debian-11", "slurm-gcp-5-7-hpc-rocky-linux-8", "slurm-gcp-5-7-ubuntu-2004-lts",
    "slurm-gcp-5-7-ubuntu-2204-lts-arm64", "slurm-gcp-5-7-hpc-centos-7-k80", "slurm-gcp-5-7-hpc-centos-7"]
  }

  source_image         = lookup(var.instance_image, "name", "")
  source_image_project = lookup(var.instance_image, "project", "")
  source_image_project_normalized = (
    local.source_image != "" || length(regexall("/", local.source_image_project)) > 0
    ? local.source_image_project
    : "projects/${local.source_image_project}/global/images/family"
  )
}

data "google_compute_image" "slurm" {
  family  = try(var.instance_image.family, null)
  name    = try(var.instance_image.name, null)
  project = local.source_image_project

  lifecycle {
    postcondition {
      condition     = !var.instance_image_custom || try(contains(keys(local.known_project_families), self.project), false)
      error_message = <<-EOD
      '${self.project}' is not a known project with compatible Slurm image families. Use the 'instance_image_custom' flag to deploy custom images. See: https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/docs/vm-images.md#slurm-on-gcp.
      EOD
    }
    postcondition {
      condition     = !var.instance_image_custom || !try(contains(keys(local.known_project_families), self.project), false) || try(contains(local.known_project_families[self.project], self.family), false)
      error_message = <<-EOD
      '${self.family}', within project '${self.project}', is not a known family of compatible Slurm images. Use the 'instance_image_custom' flag to deploy custom images. See https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/main/docs/vm-images.md#slurm-on-gcp.
      EOD
    }
    postcondition {
      condition     = var.disk_size_gb > self.disk_size_gb
      error_message = "'disk_size_gb: ${var.disk_size_gb}' is smaller than the image size (${self.disk_size_gb}GB), please increase the blueprint disk size"
    }
  }
}
