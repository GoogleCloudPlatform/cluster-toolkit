# Copyright 2025 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

terraform {
  required_version = ">= 1.7.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.2.0"
    }
  }
}

locals {
  cache_map = { for cache in var.caches : cache.zone => cache }
}

resource "google_storage_anywhere_cache" "cache_instances" {
  for_each = local.cache_map

  provider = google

  bucket = var.gcs_bucket_name
  zone   = each.key

  ttl              = each.value.ttl
  admission_policy = each.value.admission_policy
}
