
# Copyright 2024 Google LLC
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

resource "google_secret_manager_secret" "cloudsql" {
  count = var.cloudsql != null ? 1 : 0

  secret_id = "${local.slurm_cluster_name}-slurm-secret-cloudsql"

  replication {
    auto {}
  }

  labels = {
    slurm_cluster_name = local.slurm_cluster_name
  }
}

resource "google_secret_manager_secret_version" "cloudsql_version" {
  count = var.cloudsql != null ? 1 : 0

  secret      = google_secret_manager_secret.cloudsql[0].id
  secret_data = jsonencode(var.cloudsql)
}

resource "google_secret_manager_secret_iam_member" "cloudsql_secret_accessor" {
  count = var.cloudsql != null ? 1 : 0

  secret_id = google_secret_manager_secret.cloudsql[0].id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.service_account.email}"
}
