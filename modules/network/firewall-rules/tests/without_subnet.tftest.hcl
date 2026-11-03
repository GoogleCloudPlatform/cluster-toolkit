# Copyright 2026 "Google LLC"
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

mock_provider "google" {}

variables {
  use_subnetwork_data = false
  project_id          = "explicit-project"
  network_name        = "explicit-network"
}

run "omit_subnet_when_lookup_disabled" {
  command = plan
  assert {
    condition     = var.subnetwork_self_link == null && length(data.google_compute_subnetwork.subnetwork) == 0 && length(terraform_data.pga_check) == 0
    error_message = "Explicit network inputs must work without a subnet argument or lookup."
  }
  assert {
    condition     = local.effective_project_id == "explicit-project" && local.effective_network_name == "explicit-network"
    error_message = "Explicit project and network must be passed through."
  }
}

run "missing_project_without_lookup" {
  command = plan
  variables {
    project_id = null
  }
  expect_failures = [var.use_subnetwork_data]
}

run "empty_network_without_lookup" {
  command = plan
  variables {
    network_name = ""
  }
  expect_failures = [var.use_subnetwork_data]
}

run "subnet_required_with_lookup" {
  command = plan
  variables {
    use_subnetwork_data = true
  }
  expect_failures = [var.use_subnetwork_data]
}

run "empty_subnet_with_lookup" {
  command = plan
  variables {
    use_subnetwork_data   = true
    subnetwork_self_link = ""
  }
  expect_failures = [var.use_subnetwork_data]
}
