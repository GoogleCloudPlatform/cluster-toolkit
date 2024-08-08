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

locals {
  legacy_tests = [
    ["ml-a3-highgpu-slurm", "refs/tags/v1.37.1"],
  ]
}

resource "google_cloudbuild_trigger" "legacy_test" {
  count       = length(local.legacy_tests)
  name        = "LEGACY-test-${local.legacy_tests[count.index][0]}"
  description = "Runs the '${local.legacy_tests[count.index][0]}' integration test against last supported release"
  tags        = [local.notify_chat_tag]

  git_file_source {
    path      = "tools/cloud-build/daily-tests/builds/${local.legacy_tests[count.index][0]}.yaml"
    revision  = local.legacy_tests[count.index][1]
    uri       = var.repo_uri
    repo_type = "GITHUB"
  }

  source_to_build {
    uri       = var.repo_uri
    ref       = local.legacy_tests[count.index][1]
    repo_type = "GITHUB"
  }
  # Following fields will be auto-set by CloudBuild after creation
  # Specify it explicitly to reduce discreppancy.
  ignored_files  = []
  included_files = []
  substitutions  = {}
}

# TODO: build solution for scheduling tests in sequence when we have
# more than 1 test
module "legacy_test_schedule" {
  source   = "./trigger-schedule"
  count    = length(google_cloudbuild_trigger.legacy_test)
  trigger  = google_cloudbuild_trigger.legacy_test[count.index]
  schedule = "30 5 * * MON-FRI"
}
