# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

locals {
  node_roles = toset([
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter",
    "roles/monitoring.viewer",
    "roles/stackdriver.resourceMetadata.writer",
    "roles/artifactregistry.reader",
  ])
}

resource "google_service_account" "node" {
  count        = var.create_service_accounts ? 1 : 0
  project      = var.project_id
  account_id   = "${var.deployment_name}-nodes"
  display_name = "${var.deployment_name} GKE nodes"
}

resource "google_service_account" "proxy" {
  count        = var.create_service_accounts ? 1 : 0
  project      = var.project_id
  account_id   = "${var.deployment_name}-proxy"
  display_name = "${var.deployment_name} agentgateway proxy"
}

resource "google_project_iam_member" "node" {
  for_each = var.create_service_accounts ? local.node_roles : toset([])
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.node[0].email}"
}

resource "google_service_account_iam_member" "workload_identity" {
  count              = var.create_service_accounts ? 1 : 0
  service_account_id = google_service_account.proxy[0].name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[${var.namespace}/agentgateway-proxy]"
}

resource "time_sleep" "iam_ready" {
  count           = var.create_service_accounts ? 1 : 0
  create_duration = "30s"
  depends_on      = [google_project_iam_member.node, google_service_account_iam_member.workload_identity]
}
