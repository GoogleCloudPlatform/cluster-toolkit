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

output "startup_script" {
  description = "Path to the Spack installation script."
  value       = local.script_content
}

output "controller_startup_script" {
  description = "Path to the Spack installation script, duplicate for SLURM controller."
  value       = local.script_content
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

output "install_spack_runner" {
  description = "Runner to install Spack using the startup-script module"
  value       = local.install_spack_runner
}

output "setup_spack_runner" {
  description = "Adds Spack setup-env.sh script to /etc/profile.d so that it is called at shell startup. Among other things this adds Spack binary to user PATH."
  value = {
    "type"        = "data"
    "destination" = "/etc/profile.d/spack.sh"
    "content"     = <<-EOT
      #!/bin/sh
      if [ -f ${var.install_dir}/share/spack/setup-env.sh ]; then
              . ${var.install_dir}/share/spack/setup-env.sh
      fi
      EOT
  }
}

output "spack_path" {
  description = "Path to the root of the spack installation"
  value       = var.install_dir
}
