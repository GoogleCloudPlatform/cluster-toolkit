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
  labels = merge(var.labels, { ghpc_module = "notebook", ghpc_role = "compute" })
}

locals {
  suffix = random_id.resource_name_suffix.hex
  #name                 = "thenotebook"
  name                 = "notebook-${var.deployment_name}-${local.suffix}"
  bucket               = replace(var.gcs_bucket_path, "gs://", "")
  post_script_filename = "mount-${local.suffix}.sh"

  # mount_runner_args is defined here: https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/3abddcfbd245b0e6747917a4e55b30658414ffd7/community/modules/file-system/cloud-storage-bucket/outputs.tf#L40
  mount_args = split(" ", var.mount_runner.args)

  unused       = local.mount_args[0]
  remote_mount = local.mount_args[1]
  local_mount  = local.mount_args[2]
  fs_type      = local.mount_args[3]
  # These options provide a "rw" mount of the GCS bucket
  mount_options = "defaults,_netdev,allow_other,implicit_dirs,gid=1000,uid=1000"

  content0 = var.mount_runner.content
  content1 = replace(local.content0, "$1", local.unused)
  content2 = replace(local.content1, "$2", local.remote_mount)
  content3 = replace(local.content2, "$3", local.local_mount)
  content4 = replace(local.content3, "$4", local.fs_type)
  content5 = replace(local.content4, "$5", local.mount_options)

}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_storage_bucket_object" "mount_script" {
  name    = local.post_script_filename
  content = local.content5
  bucket  = local.bucket
}

resource "google_workbench_instance" "instance" {
  name     = local.name
  location = var.zone
  project  = var.project_id
  labels   = local.labels
  gce_setup {
    machine_type = var.machine_type
    metadata = {
      post-startup-script = "${var.gcs_bucket_path}/${google_storage_bucket_object.mount_script.name}"
    }
    vm_image {
      project = var.instance_image.project
      family  = var.instance_image.family
    }
  }
}
