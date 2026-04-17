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
  labels = merge(var.labels, { ghpc_module = "bigquery-table", ghpc_role = "database" })
}

locals {
  table_id = var.table_id != null ? var.table_id : replace("${var.deployment_name}_table_${random_id.resource_name_suffix.hex}", "-", "_")
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_bigquery_table" "pbsb" {
  deletion_protection = false
  project             = var.project_id
  table_id            = local.table_id
  dataset_id          = var.dataset_id
  schema              = var.table_schema
  labels              = local.labels
}
