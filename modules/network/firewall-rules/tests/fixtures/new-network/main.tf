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

terraform {
  required_version = ">= 1.12.2"
}

variable "subnetwork_self_link" {
  description = "Subnet self link used to simulate a newly created VPC output."
  type        = string
}

# terraform_data.output is unknown during a fresh plan, like new VPC outputs.
resource "terraform_data" "network" {
  input = {
    project = "test-project"
    name    = "test-network"
    subnet  = var.subnetwork_self_link
  }
}

module "firewall" {
  source               = "../../.."
  project_id           = terraform_data.network.output.project
  network_name         = terraform_data.network.output.name
  subnetwork_self_link = terraform_data.network.output.subnet
  ingress_rules = [{
    name          = "allow-test"
    source_ranges = ["10.0.0.0/8"]
    allow         = [{ protocol = "tcp", ports = ["8080"] }]
  }]
}
