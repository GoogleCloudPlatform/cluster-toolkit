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

mock_provider "google" {}
variables {
  project_id            = "test-project"
  deployment_name       = "agw-test"
  service_account_email = "proxy@test-project.iam.gserviceaccount.com"
}
run "create_prediction_access" {
  command = plan
  assert {
    condition     = length(google_project_iam_custom_role.predict) == 1 && length(google_project_iam_member.predict) == 1 && google_project_iam_custom_role.predict[0].permissions == toset(["aiplatform.endpoints.predict"])
    error_message = "Default mode must grant only prediction access through a custom role."
  }
}
run "existing_prediction_access" {
  command = plan
  variables { enabled = false }
  assert {
    condition     = length(google_project_iam_custom_role.predict) == 0 && length(google_project_iam_member.predict) == 0
    error_message = "Administrator-provisioned access must cause no IAM writes."
  }
}
