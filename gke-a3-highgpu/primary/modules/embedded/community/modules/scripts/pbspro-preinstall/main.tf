/**
 * Copyright 2022 Google LLC
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
  labels = merge(var.labels, { ghpc_module = "pbspro-preinstall", ghpc_role = "scripts" })
}

locals {
  location = coalesce(var.location, var.region)

  bucket_name = "pbspro-packages"

  retention_policy = {
    (local.bucket_name) = var.retention_policy
  }
  versioning = {
    (local.bucket_name) = var.versioning
  }

  bucket_lifecycle_rules = {
    (local.bucket_name) = var.bucket_lifecycle_rules
  }

  force_destroy = {
    (local.bucket_name) = var.force_destroy
  }

  bucket_viewers = {
    (local.bucket_name) = join(",", var.bucket_viewers)
  }
  set_viewer_roles = length(var.bucket_viewers) > 0
}

module "pbspro_bucket" {
  source  = "terraform-google-modules/cloud-storage/google"
  version = "~> 5.0"

  project_id       = var.project_id
  location         = local.location
  prefix           = var.deployment_name
  names            = [local.bucket_name]
  randomize_suffix = true
  labels           = local.labels

  bucket_viewers   = local.bucket_viewers
  set_viewer_roles = local.set_viewer_roles

  retention_policy = local.retention_policy
  storage_class    = var.storage_class
  versioning       = local.versioning

  bucket_lifecycle_rules = local.bucket_lifecycle_rules

  force_destroy = local.force_destroy
}

resource "google_storage_bucket_object" "client_rpm" {
  name   = basename(var.client_rpm)
  source = var.client_rpm
  bucket = module.pbspro_bucket.bucket.name
}

resource "google_storage_bucket_object" "devel_rpm" {
  name   = basename(var.devel_rpm)
  source = var.devel_rpm
  bucket = module.pbspro_bucket.bucket.name
}

resource "google_storage_bucket_object" "execution_rpm" {
  name   = basename(var.execution_rpm)
  source = var.execution_rpm
  bucket = module.pbspro_bucket.bucket.name
}

resource "google_storage_bucket_object" "server_rpm" {
  name   = basename(var.server_rpm)
  source = var.server_rpm
  bucket = module.pbspro_bucket.bucket.name
}

resource "google_storage_bucket_object" "license_file" {
  count  = var.license_file == null ? 0 : 1
  name   = "${var.deployment_name}-altair_lic.dat"
  source = var.license_file
  bucket = module.pbspro_bucket.bucket.name
}
