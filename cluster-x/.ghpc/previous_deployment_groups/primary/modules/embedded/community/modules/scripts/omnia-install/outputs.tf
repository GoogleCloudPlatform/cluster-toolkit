/**
 * Copyright 2022 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

output "inventory_file" {
  description = "The inventory file for the omnia cluster"
  value       = local.inventory
}

output "setup_omnia_node_script" {
  description = "An ansible script that adds the user that install omnia"
  value       = local.setup_omnia_node_file
}

output "copy_inventory_runner" {
  description = "Runner to copy the inventory to the omnia manager using the startup-script module"
  value       = local.copy_inventory_runner
}

output "setup_omnia_node_runner" {
  description = <<-EOT
  Runner to create the omnia user using an ansible playbook. The startup-script
  module will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-omnia-id.setup_omnia_node_runner)
  ...
  EOT
  value       = local.setup_omnia_node_runner
}

output "install_omnia_runner" {
  description = <<-EOT
  Runner to install Omnia using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-omnia-id.install_omnia_runner)
  ...
  EOT
  value       = local.install_omnia_runner
}

output "omnia_user_warning" {
  description = "Warn developers that the omnia user was created with sudo permissions"
  value       = "WARNING: A new user named '${var.omnia_username}' was created with sudo permissions. Remove user from all Omnia nodes if this is not desired."
}
