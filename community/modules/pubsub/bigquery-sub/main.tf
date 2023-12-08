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
  labels = merge(var.labels, { ghpc_module = "bigquery-sub", ghpc_role = "pubsub" })
}

locals {
  subscription_id = var.subscription_id != null ? var.subscription_id : "${var.deployment_name}_subscription_${random_id.resource_name_suffix.hex}"
}

resource "random_id" "resource_name_suffix" {
  byte_length = 4
}
data "google_project" "project" {
  project_id = var.project_id
}

resource "google_project_iam_member" "viewer" {
  project = data.google_project.project.project_id
  role    = "roles/bigquery.metadataViewer"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_project_iam_member" "editor" {
  project = data.google_project.project.project_id
  role    = "roles/bigquery.dataEditor"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "example" {
  depends_on = [google_project_iam_member.editor, google_project_iam_member.viewer]
  name       = local.subscription_id
  topic      = var.topic_id
  project    = var.project_id
  labels     = local.labels
  bigquery_config {
    table            = "${var.project_id}.${var.dataset_id}.${var.table_id}"
    use_topic_schema = true
    write_metadata   = true
  }

}
