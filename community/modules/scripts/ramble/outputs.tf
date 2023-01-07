/**
 * Copyright 2023 Google LLC
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

output "ramble_deps_runner" {
  description = <<-EOT
    Runner to install dependencies for ramble using an ansible playbook. The
    startup-script module will automatically handle installation of ansible.
    - id: example-startup-script
      source: modules/scripts/startup-script
      settings:
        runners:
        - $(your-ramble-id.ramble_deps_runner)
    ...
    EOT
  value       = local.ramble_deps_runner
}

output "ramble_install_runner" {
  description = <<-EOT
  Runner to install Ramble using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-ramble-id.ramble_install_runner)
  ...
  EOT
  value       = local.ramble_install_runner
}

output "ramble_commands_runner" {
  description = <<-EOT
  Runner to run Ramble commands using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-ramble-id.ramble_commands_runner)
  ...
  EOT
  value       = local.ramble_commands_runner
}

output "ramble_setup_runner" {
  description = "Adds Ramble setup-env.sh script to /etc/profile.d so that it is called at shell startup. Among other things this adds Ramble binary to user PATH."
  value = {
    "type"        = "data"
    "destination" = "/etc/profile.d/ramble.sh"
    "content"     = <<-EOT
      #!/bin/sh
      . /usr/local/ghpc-venv/bin/activate
      if [ -f ${var.install_dir}/share/ramble/setup-env.sh ]; then
              . ${var.install_dir}/share/ramble/setup-env.sh
      fi
      EOT
  }
}

output "ramble_path" {
  description = "Location ramble is installed into."
  value       = var.install_dir
}
