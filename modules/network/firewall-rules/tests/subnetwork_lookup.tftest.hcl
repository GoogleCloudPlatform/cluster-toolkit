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

mock_provider "google" {
  mock_data "google_compute_subnetwork" {
    defaults = {
      project                  = "test-project"
      network                  = "test-network"
      private_ip_google_access = true
    }
  }
}

variables {
  subnetwork_self_link = "https://www.googleapis.com/compute/v1/projects/test-project/regions/us-central1/subnetworks/test-subnet"
}

run "derive_network_and_project" {
  command = plan
  assert {
    condition     = length(data.google_compute_subnetwork.subnetwork) == 1 && length(terraform_data.pga_check) == 1
    error_message = "The default must look up the subnet and validate Private Google Access."
  }
  assert {
    condition     = local.effective_project_id == "test-project" && local.effective_network_name == "test-network"
    error_message = "Missing project and network must be derived from the subnet."
  }
}

run "explicit_null_uses_default_lookup" {
  command = plan
  variables {
    use_subnetwork_data = null
  }
  assert {
    condition     = var.use_subnetwork_data == true
    error_message = "Explicit null must use the default true value."
  }
  assert {
    condition     = length(data.google_compute_subnetwork.subnetwork) == 1 && length(terraform_data.pga_check) == 1
    error_message = "Explicit null must retain subnet lookup and Private Google Access validation."
  }
  assert {
    condition     = local.effective_project_id == "test-project" && local.effective_network_name == "test-network"
    error_message = "Explicit null must retain project and network derivation from the subnet."
  }
}

run "explicit_network_without_lookup" {
  command = plan
  variables {
    use_subnetwork_data  = false
    subnetwork_self_link = null
    project_id           = "explicit-project"
    network_name         = "explicit-network"
  }
  assert {
    condition     = length(data.google_compute_subnetwork.subnetwork) == 0 && length(terraform_data.pga_check) == 0
    error_message = "Disabling lookup must skip the subnet read and Private Google Access check."
  }
  assert {
    condition     = local.effective_project_id == "explicit-project" && local.effective_network_name == "explicit-network"
    error_message = "Explicit project and network must be preserved."
  }
}

run "reject_disabled_private_google_access" {
  command = plan
  override_data {
    target = data.google_compute_subnetwork.subnetwork[0]
    values = {
      project                  = "test-project"
      network                  = "test-network"
      private_ip_google_access = false
    }
  }
  expect_failures = [terraform_data.pga_check]
}

run "explicit_inputs_preserve_overrides" {
  command = plan
  variables {
    project_id   = "explicit-project"
    network_name = "explicit-network"
  }
  override_data {
    target = data.google_compute_subnetwork.subnetwork[0]
    values = {
      project                  = "derived-project"
      network                  = "derived-network"
      private_ip_google_access = false
    }
  }
  assert {
    condition     = local.effective_project_id == "explicit-project" && local.effective_network_name == "explicit-network"
    error_message = "Explicit inputs must take precedence and must not require Private Google Access."
  }
}

run "new_network_values_unknown_until_apply" {
  command = plan
  module {
    source = "./tests/fixtures/new-network"
  }
}
