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
  labels = merge(var.labels, { ghpc_module = "cloud-storage-bucket", ghpc_role = "file-system" })
}

locals {
  prefix         = var.name_prefix != null ? var.name_prefix : ""
  deployment     = var.use_deployment_name_in_bucket_name ? var.deployment_name : ""
  suffix         = var.random_suffix ? random_id.resource_name_suffix.hex : ""
  first_dash     = (local.prefix != "" && (local.deployment != "" || local.suffix != "")) ? "-" : ""
  second_dash    = local.deployment != "" && local.suffix != "" ? "-" : ""
  composite_name = "${local.prefix}${local.first_dash}${local.deployment}${local.second_dash}${local.suffix}"
  name           = local.composite_name == "" ? "no-bucket-name-provided" : local.composite_name
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_storage_bucket" "bucket" {
  provider                    = google-beta
  project                     = var.project_id
  name                        = local.name
  uniform_bucket_level_access = true
  location                    = var.region
  storage_class               = "REGIONAL"
  labels                      = local.labels
  force_destroy               = var.force_destroy
  hierarchical_namespace {
    enabled = var.enable_hierarchical_namespace
  }
}

resource "google_storage_bucket_iam_binding" "viewers" {
  bucket  = google_storage_bucket.bucket.name
  role    = "roles/storage.objectViewer"
  members = var.viewers
}
