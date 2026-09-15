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
mock_provider "time" {}

variables {
  project_id      = "test-project"
  deployment_name = "agw-test"
  namespace       = "agentgateway-system"
}

run "create_identities" {
  command = plan
  assert {
    condition     = length(google_service_account.node) == 1 && length(google_service_account.proxy) == 1 && length(google_project_iam_member.node) == 5 && length(google_service_account_iam_member.workload_identity) == 1
    error_message = "Default mode must provision both identities and their bindings."
  }
  assert {
    condition     = google_service_account_iam_member.workload_identity[0].member == "serviceAccount:test-project.svc.id.goog[agentgateway-system/agentgateway-proxy]"
    error_message = "The binding must reference the configured namespace and proxy KSA."
  }
}

run "existing_identities" {
  command = plan
  variables {
    create_service_accounts     = false
    node_service_account_email  = "nodes@test-project.iam.gserviceaccount.com"
    proxy_service_account_email = "proxy@test-project.iam.gserviceaccount.com"
  }
  assert {
    condition     = length(google_service_account.node) == 0 && length(google_service_account.proxy) == 0 && length(google_project_iam_member.node) == 0 && length(google_service_account_iam_member.workload_identity) == 0 && length(time_sleep.iam_ready) == 0
    error_message = "Existing mode must not create identities, modify IAM, or wait for IAM propagation."
  }
  assert {
    condition     = output.node_service_account_email == var.node_service_account_email && output.proxy_service_account_email == var.proxy_service_account_email
    error_message = "Existing account emails must flow to downstream modules unchanged."
  }
}

run "missing_existing_accounts" {
  command = plan
  variables { create_service_accounts = false }
  expect_failures = [var.node_service_account_email, var.proxy_service_account_email]
}

run "missing_proxy_account" {
  command = plan
  variables {
    create_service_accounts    = false
    node_service_account_email = "nodes@test-project.iam.gserviceaccount.com"
  }
  expect_failures = [var.proxy_service_account_email]
}

run "invalid_existing_accounts" {
  command = plan
  variables {
    create_service_accounts     = false
    node_service_account_email  = "not-an-email"
    proxy_service_account_email = "user@example.com"
  }
  expect_failures = [var.node_service_account_email, var.proxy_service_account_email]
}

run "conflicting_create_inputs" {
  command = plan
  variables {
    node_service_account_email  = "nodes@test-project.iam.gserviceaccount.com"
    proxy_service_account_email = "proxy@test-project.iam.gserviceaccount.com"
  }
  expect_failures = [var.node_service_account_email, var.proxy_service_account_email]
}

run "domain_scoped_workload_identity" {
  command = plan
  variables { project_id = "example.com:test-project" }
  assert {
    condition     = google_service_account_iam_member.workload_identity[0].member == "serviceAccount:example.com/test-project.svc.id.goog[agentgateway-system/agentgateway-proxy]"
    error_message = "Domain-scoped IAM member names must use a slash in the workload identity pool."
  }
}
