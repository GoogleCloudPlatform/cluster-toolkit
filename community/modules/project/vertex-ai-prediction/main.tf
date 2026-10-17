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

resource "google_project_iam_custom_role" "predict" {
  count       = var.enabled ? 1 : 0
  project     = var.project_id
  role_id     = "${replace(var.deployment_name, "-", "_")}_predict"
  title       = "${var.deployment_name} model prediction"
  description = "Invoke publisher models without Agent Platform resource administration."
  permissions = ["aiplatform.endpoints.predict"]
}

resource "google_project_iam_member" "predict" {
  count   = var.enabled ? 1 : 0
  project = var.project_id
  role    = google_project_iam_custom_role.predict[0].name
  member  = "serviceAccount:${var.service_account_email}"
}

moved {
  from = google_project_iam_custom_role.predict
  to   = google_project_iam_custom_role.predict[0]
}

moved {
  from = google_project_iam_member.predict
  to   = google_project_iam_member.predict[0]
}
