# Copyright 2023 Google LLC
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

resource "google_cloud_scheduler_job" "schedule" {
  name      = "${var.trigger.name}-schedule"
  schedule  = var.schedule
  time_zone = var.time_zone

  attempt_deadline = "180s"
  retry_config {
    max_backoff_duration = "1200s"
    max_retry_duration   = "3600s"
    max_doublings        = 2
    min_backoff_duration = "300s"
    retry_count          = var.retry_count
  }

  http_target {
    http_method = "POST"
    uri         = "https://cloudbuild.googleapis.com/v1/${var.trigger.id}:run"
    oauth_token {
      service_account_email = "cloud-build-trigger-scheduler@${var.trigger.project}.iam.gserviceaccount.com"
    }
  }
}
