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
      "slurm-gcp-6-10-debian-12",
      "slurm-gcp-6-10-hpc-rocky-linux-8",
      "slurm-gcp-6-10-ubuntu-2204-lts-nvidia-570",
      "slurm-gcp-6-10-ubuntu-2404-lts-nvidia-570",
      "slurm-gcp-6-10-ubuntu-2204-lts-arm64",
      "slurm-gcp-6-10-ubuntu-2404-lts-arm64"
    ]
  }

  # This approach to "hacking" the project name allows a chain of Terraform
  # calls to set the instance source_image (boot disk) with a "relative
  # resource name" that passes muster with VPC Service Control rules
  #
  # https://github.com/terraform-google-modules/terraform-google-vm/blob/735bd415fc5f034d46aa0de7922e8fada2327c0c/modules/instance_template/main.tf#L28
  # https://cloud.google.com/apis/design/resource_names#relative_resource_name
  source_image_project_normalized = (can(var.instance_image.family) ?
    "projects/${var.instance_image.project}/global/images/family" :
    "projects/${var.instance_image.project}/global/images"
  )
  source_image_family = try(var.instance_image.family, "")
  source_image        = try(var.instance_image.name, "")
}
