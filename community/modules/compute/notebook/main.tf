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

  create_fsi_tutorial =     var.create_tutorial == "fsi" ? 1 : 0
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

## The following resources deploy files to the GCS bucket which is mounted both on the notebook and the batch VMs
## ONLY if the variable "create_tutorial" is set to "fsi"
data "template_file" "mc_run_py" {
  template = "${file("${path.module}/files/mc_run.tpl.py")}"
  vars = {
    project_id = var.project_id
    topic_id = var.topic_id
    topic_schema = var.topic_schema
    dataset_id = var.dataset_id
    table_id = var.table_id
  }
}

resource "google_storage_bucket_object" "mc_run" {
  count = local.create_fsi_tutorial

  name   = "mc_run.py"
  content = data.template_file.mc_run_py.rendered
  bucket = local.bucket
}

data "template_file" "mc_run_yaml" {
  template = "${file("${path.module}/files/mc_run.tpl.yaml")}"
  vars = {
    project_id = var.project_id
    bucket_name = local.bucket
    region = var.region
  }
}

resource "google_storage_bucket_object" "mc_obj_yaml" {
  count = local.create_fsi_tutorial
  name   = "mc_run.yaml"
  content = data.template_file.mc_run_yaml.rendered
  bucket = local.bucket
}

data "template_file" "ipynb_fsi" {
  template = "${file("${path.module}/files/FSI_MonteCarlo.ipynb")}"
  vars = {
    project_id = var.project_id
    dataset_id = var.dataset_id
    table_id = var.table_id
  }
}
resource "google_storage_bucket_object" "ipynb_obj_fsi" {
  count = local.create_fsi_tutorial
  name   = "FSI_MonteCarlo.ipynb"
  content = data.template_file.ipynb_fsi.rendered
  bucket = local.bucket
}

data "http" "batch_py" {
  url = "https://raw.githubusercontent.com/GoogleCloudPlatform/scientific-computing-examples/main/python-batch/batch.py"
}

resource "google_storage_bucket_object" "run_batch_py" {
  count = local.create_fsi_tutorial
  name   = "batch.py"
  content = data.http.batch_py.body
  bucket = local.bucket
}

data "http" "batch_requirements" {
  url = "https://raw.githubusercontent.com/GoogleCloudPlatform/scientific-computing-examples/main/python-batch/requirements.txt"
}

resource "google_storage_bucket_object" "get_iteration_sh" {
  count = local.create_fsi_tutorial

  name   = "iteration.sh"
  content = file("${path.module}/files/iteration.sh")
  bucket = local.bucket
}

resource "google_storage_bucket_object" "get_mc_reqs" {
  count = local.create_fsi_tutorial

  name   = "mc_run_reqs.txt"
  content = file("${path.module}/files/mc_run_reqs.txt")
  bucket = local.bucket
}