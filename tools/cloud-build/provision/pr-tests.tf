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

resource "google_cloudbuild_trigger" "pr_test" {
  for_each    = data.external.list_tests_py.result
  name        = "PR-test-${each.key}"
  description = "Runs the '${each.key}' integration test against a PR"

  filename = "tools/cloud-build/daily-tests/builds/${each.key}.yaml"
  approval_config {
    approval_required = true
  }

  github {
    owner = "GoogleCloudPlatform"
    name  = "hpc-toolkit"
    pull_request {
      branch          = ".*"
      comment_control = "COMMENTS_ENABLED_FOR_EXTERNAL_CONTRIBUTORS_ONLY"
    }
  }

}
