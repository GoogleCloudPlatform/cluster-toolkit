/**
 * Copyright 2021 Google LLC
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

module "startup_scripts" {
  source                       = "github.com/terraform-google-modules/terraform-google-startup-scripts?ref=v1.0.0"
  enable_init_gsutil_crcmod_el = true
  enable_get_from_bucket       = true
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_storage_bucket" "configs_bucket" {
  name                        = "${var.deployment_name}-startup-scripts-${random_id.resource_name_suffix.hex}"
  uniform_bucket_level_access = true
  location                    = var.region
  storage_class               = "REGIONAL"
}

resource "google_storage_bucket_object" "scripts" {
  # this writes all scripts exactly once into GCS
  for_each = toset(var.runners[*].file)
  name     = basename(each.key)
  content  = file(each.key)
  bucket   = google_storage_bucket.configs_bucket.name
}
