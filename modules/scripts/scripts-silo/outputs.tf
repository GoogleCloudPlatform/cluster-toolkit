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

output "examples_hello_sh" {
  value       = "${path.module}/examples/hello.sh"
  description = "relative path to hello.sh"
}

output "examples_hello_yaml" {
  value       = "${path.module}/examples/hello.yaml"
  description = "relative path to hello.yaml"
}

output "examples_install_ansible" {
  value       = "${path.module}/examples/install_ansible.sh"
  description = "relative path to install_ansible.sh"
}

output "examples_install_cloud_ops_agent" {
  value       = "${path.module}/examples/install_cloud_ops_agent.sh"
  description = "relative path to install_cloud_ops_agent.sh"
}
