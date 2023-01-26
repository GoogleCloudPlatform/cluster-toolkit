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

output "spack_install_runner" {
  description = <<-EOT
  Runner to install Spack using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-spack-id.spack_install_runner)
  ...
  EOT
  value       = local.spack_install_runner
}

output "spack_commands_runner" {
  description = <<-EOT
  Runner to run Spack commands using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-spack-id.spack_commands_runner)
  ...
  EOT
  value       = local.spack_commands_runner
}

output "spack_packages_runner" {
  description = <<-EOT
  Runner to install Spack packages using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-spack-id.spack_packages_runner)
  ...
  EOT
  value       = local.spack_packages_runner
}


output "spack_compilers_runner" {
  description = <<-EOT
  Runner to install and configure compilers using Spack using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-spack-id.spack_compilers_runner)
  ...
  EOT
  value       = local.spack_compilers_runner
}

output "setup_spack_runner" {
  description = "Adds Spack setup-env.sh script to /etc/profile.d so that it is called at shell startup. Among other things this adds Spack binary to user PATH."
  value = {
    "type"        = "data"
    "destination" = "/etc/profile.d/spack.sh"
    "content"     = <<-EOT
      #!/bin/sh
      . /usr/local/ghpc-venv/bin/activate
      if [ -f ${var.install_dir}/share/spack/setup-env.sh ]; then
              . ${var.install_dir}/share/spack/setup-env.sh
      fi
      EOT
  }
}

output "install_spack_runner" {
  description = <<-EOT
  Runner that incorporates the contents of the following other runners, executed
  in this order: spack_install_runner, spack_commands_runner,
  spack_compiler_runner, spack_packages_runner.

  Usage:
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-spack-id.install_spack_runner)
  ...

  EOT
  value       = local.install_spack_runner
}

output "spack_path" {
  description = "Location spack is installed into."
  value       = var.install_dir
}

output "install_spack_deps_runner" {
  description = <<-EOT
  Runner to install dependencies for spack using an ansible playbook. The
  startup-script module will automatically handle installation of ansible.
  - id: example-startup-script
    source: modules/scripts/startup-script
    settings:
      runners:
      - $(your-spack-id.install_spack_deps_runner)
  ...
  EOT
  value       = local.install_spack_deps_runner
}
