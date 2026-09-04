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

# Daily tests running in daily_tests_project_id (hpc-toolkit-dev-2)
resource "google_cloudbuild_trigger" "daily_test" {
  for_each = data.external.list_tests_midnight.result
  name     = "DAILY-test-${each.key}"
  project  = var.daily_tests_project_id
  tags     = [local.notify_chat_tag]
  # For projects with BYOSA enforced (e.g. hpc-toolkit-dev-2), export the service account environment variable before applying:
  # export TF_VAR_daily_tests_service_account="projects/MY_PROJECT/serviceAccounts/my-service-account@MY_PROJECT.iam.gserviceaccount.com"
  service_account = var.daily_tests_service_account

  git_file_source {
    path      = "tools/cloud-build/daily-tests/builds/${each.key}.yaml"
    revision  = local.ref_develop
    uri       = var.repo_uri
    repo_type = "GITHUB"
  }

  source_to_build {
    uri       = var.repo_uri
    ref       = local.ref_develop
    repo_type = "GITHUB"
  }
  # Following fields will be auto-set by CloudBuild after creation
  # Specify it explicitly to reduce discreppancy.
  ignored_files  = []
  included_files = []
  substitutions = {
    _TEST_PREFIX = "daily-"
  }
}

module "daily_test_schedule" {
  source   = "./trigger-schedule"
  for_each = data.external.list_tests_midnight.result
  trigger  = google_cloudbuild_trigger.daily_test[each.key]
  schedule = each.value
}

# Exception daily tests running in project_id (hpc-toolkit-dev)
resource "google_cloudbuild_trigger" "daily_test_exceptions" {
  for_each    = toset(var.daily_tests_dev_exceptions)
  name        = "DAILY-test-${each.key}"
  description = "Runs the '${each.key}' integration test against `develop`"
  tags        = [local.notify_chat_tag]

  git_file_source {
    path      = "tools/cloud-build/daily-tests/builds/${each.key}.yaml"
    revision  = local.ref_develop
    uri       = var.repo_uri
    repo_type = "GITHUB"
  }

  source_to_build {
    uri       = var.repo_uri
    ref       = local.ref_develop
    repo_type = "GITHUB"
  }
  # Following fields will be auto-set by CloudBuild after creation
  # Specify it explicitly to reduce discreppancy.
  ignored_files  = []
  included_files = []
  substitutions = {
    _TEST_PREFIX = "daily-"
  }
}

module "daily_test_schedule_exceptions" {
  source   = "./trigger-schedule"
  for_each = toset(var.daily_tests_dev_exceptions)
  trigger  = google_cloudbuild_trigger.daily_test_exceptions[each.key]
  schedule = data.external.list_tests_midnight.result[each.key]
}
