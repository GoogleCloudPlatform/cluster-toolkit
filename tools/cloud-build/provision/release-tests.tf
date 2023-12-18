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

# Commented out to temporarily remove RELEASE tests

# locals {
#     ref_release_canidate = "refs/heads/release-candidate"
# }

# resource "google_cloudbuild_trigger" "release_test" {
#   for_each    = data.external.list_tests_morning.result
#   name        = "RELEASE-test-${each.key}"
#   description = "Runs the '${each.key}' integration test against `release-candidate`"
#   tags        = [local.notify_chat_tag]
#   disabled    = false

#   git_file_source {
#     path      = "tools/cloud-build/daily-tests/builds/${each.key}.yaml"
#     revision  = local.ref_release_canidate
#     uri       = var.repo_uri
#     repo_type = "GITHUB"
#   }

#   source_to_build {
#     uri       = var.repo_uri
#     ref       = local.ref_release_canidate
#     repo_type = "GITHUB"
#   }
#   # Following fields will be auto-set by CloudBuild after creation
#   # Specify it explicitly to reduce discreppancy.
#   ignored_files  = []
#   included_files = []
#   substitutions  = {}
# }

# 
# module "release_test_schedule" {
#   source   = "./trigger-schedule"
#   for_each = data.external.list_tests_morning.result
#   trigger  = google_cloudbuild_trigger.release_test[each.key]
#   schedule = each.value
# }

# data "external" "list_tests_morning" {
#   program = ["./list_tests.py", "330", "720"] # 05:30 - 12:00
# }
