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
  # This label allows for billing report tracking based on module.
  labels = merge(var.labels, { ghpc_module = "verte-ai-notebook", ghpc_role = "compute" })
  project           = var.image_project_id == "" ? "deeplearning-platform-release" : var.image_project_id
  image_family      = var.image_family == "" ? "tf-latest-cpu" : var.image_family
  suffix            = random_id.resource_name_suffix.hex 
  name              = "${var.deployment_name}-notebook-${local.suffix}"
  bucket = replace(var.gcs_bucket_path, "gs://", "")
  post_script_filename = "mount.sh"

  mount_args = split(" ",var.mount_runner.args)

  unused = local.mount_args[0]
  remote_mount = local.mount_args[1]
  local_mount = local.mount_args[2]
  fs_type = local.mount_args[3]
  mount_options = "defaults,_netdev,allow_other,implicit_dirs,gid=1000,uid=1000"

  content0 = "${var.mount_runner.content}"
  content1 = replace(local.content0, "$1", local.unused)
  content2 = replace(local.content1, "$2", local.remote_mount)
  content3 = replace(local.content2, "$3", local.local_mount)
  content4 = replace(local.content3, "$4", local.fs_type)
  content5 = replace(local.content4, "$5", local.mount_options)

}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_notebooks_instance" "instance" {
  name = local.name
  location = var.zone
  machine_type = var.machine_type
  project = var.project_id
  post_startup_script = "${var.gcs_bucket_path}/${local.post_script_filename}"
  vm_image {
    project           = local.project
    image_family      = local.image_family
  }
}

resource "google_storage_bucket_object" "mount_script" {
  name   = "mount.sh"
  content = local.content5
  bucket = local.bucket
}