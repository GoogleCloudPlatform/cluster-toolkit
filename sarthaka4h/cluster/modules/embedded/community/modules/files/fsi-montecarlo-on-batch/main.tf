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
  bucket = replace(var.gcs_bucket_path, "gs://", "")
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

data "template_file" "mc_run_py" {
  template = file("${path.module}/mc_run.tpl.py")
  vars = {
    project_id   = var.project_id
    topic_id     = var.topic_id
    topic_schema = var.topic_schema
    dataset_id   = var.dataset_id
    table_id     = var.table_id
  }
}

resource "google_storage_bucket_object" "mc_run" {
  name    = "mc_run.py"
  content = data.template_file.mc_run_py.rendered
  bucket  = local.bucket
}

data "template_file" "mc_run_yaml" {
  template = file("${path.module}/mc_run.tpl.yaml")
  vars = {
    project_id  = var.project_id
    bucket_name = local.bucket
    region      = var.region
  }
}

resource "google_storage_bucket_object" "mc_obj_yaml" {
  name    = "mc_run.yaml"
  content = data.template_file.mc_run_yaml.rendered
  bucket  = local.bucket
}

data "template_file" "ipynb_fsi" {
  template = file("${path.module}/FSI_MonteCarlo.ipynb")
  vars = {
    project_id = var.project_id
    dataset_id = var.dataset_id
    table_id   = var.table_id
  }
}
resource "google_storage_bucket_object" "ipynb_obj_fsi" {
  name    = "FSI_MonteCarlo.ipynb"
  content = data.template_file.ipynb_fsi.rendered
  bucket  = local.bucket
}

data "http" "batch_py" {
  url = "https://raw.githubusercontent.com/GoogleCloudPlatform/scientific-computing-examples/main/python-batch/batch.py"
}

resource "google_storage_bucket_object" "run_batch_py" {
  name    = "batch.py"
  content = data.http.batch_py.response_body
  bucket  = local.bucket
}

data "http" "batch_requirements" {
  url = "https://raw.githubusercontent.com/GoogleCloudPlatform/scientific-computing-examples/main/python-batch/requirements.txt"
}

resource "google_storage_bucket_object" "get_requirements" {
  name    = "requirements.txt"
  content = data.http.batch_requirements.response_body
  bucket  = local.bucket
}

resource "google_storage_bucket_object" "get_iteration_sh" {
  name    = "iteration.sh"
  content = file("${path.module}/iteration.sh")
  bucket  = local.bucket
}

resource "google_storage_bucket_object" "get_mc_reqs" {
  name    = "mc_run_reqs.txt"
  content = file("${path.module}/mc_run_reqs.txt")
  bucket  = local.bucket
}
