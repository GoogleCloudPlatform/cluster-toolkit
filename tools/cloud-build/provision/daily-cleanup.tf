# Copyright 2026 Google LLC
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

resource "google_cloudbuild_trigger" "daily_project_cleanup_filestore" {
  name        = "DAILY-project-cleanup-filestore"
  description = "A filestore cleanup script to run periodically"
  tags        = [local.notify_chat_tag]

  git_file_source {
    path      = "tools/cloud-build/project-cleanup-filestore.yaml"
    revision  = local.ref_develop
    uri       = var.repo_uri
    repo_type = "GITHUB"
  }

  source_to_build {
    uri       = var.repo_uri
    ref       = local.ref_develop
    repo_type = "GITHUB"
  }
}

module "daily_project_cleanup_filestore_schedule" {
  source      = "./trigger-schedule"
  trigger     = google_cloudbuild_trigger.daily_project_cleanup_filestore
  schedule    = "50 23 * * *"
  retry_count = 4
}

resource "google_cloudbuild_trigger" "daily_project_cleanup_slurm" {
  name        = "DAILY-project-cleanup-slurm"
  description = "A metadata & resource policies cleanup script to run periodically"
  tags        = [local.notify_chat_tag]

  git_file_source {
    path      = "tools/cloud-build/project-cleanup-slurm.yaml"
    revision  = local.ref_develop
    uri       = var.repo_uri
    repo_type = "GITHUB"
  }

  source_to_build {
    uri       = var.repo_uri
    ref       = local.ref_develop
    repo_type = "GITHUB"
  }
}

module "daily_project_cleanup_slurm_schedule" {
  source      = "./trigger-schedule"
  trigger     = google_cloudbuild_trigger.daily_project_cleanup_slurm
  schedule    = "0 0 * * MON-FRI"
  retry_count = 4
}
