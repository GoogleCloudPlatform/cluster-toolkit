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
  labels = merge(var.labels, { ghpc_module = "topic", ghpc_role = "pubsub" })
}

locals {
  topic_id  = var.topic_id != null ? var.topic_id : "${var.deployment_name}_topic_${random_id.resource_name_suffix.hex}"
  schema_id = var.schema_id != null ? var.schema_id : "${var.deployment_name}_schema_${random_id.resource_name_suffix.hex}"
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}

resource "google_pubsub_topic" "example" {
  name       = local.topic_id
  depends_on = [google_pubsub_schema.example]
  project    = var.project_id
  labels     = local.labels
  schema_settings {
    schema   = "projects/${var.project_id}/schemas/${local.schema_id}"
    encoding = "BINARY"
  }
}

resource "google_pubsub_schema" "example" {
  name    = local.schema_id
  project = var.project_id
  type    = "AVRO"

  definition = var.schema_json
}
