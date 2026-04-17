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
  description = "Spack installation script."
  value       = module.startup_script.startup_script
}

output "controller_startup_script" {
  description = "Spack installation script, duplicate for SLURM controller."
  value       = module.startup_script.startup_script
}

output "spack_path" {
  description = "Path to the root of the spack installation"
  value       = var.install_dir
}

output "spack_runner" {
  description = <<-EOT
  Runner to be used with startup-script module or passed to spack-execute module.
  - installs Spack dependencies
  - installs Spack 
  - generates profile.d script to enable access to Spack
  This is safe to run in parallel by multiple machines. Use in place of deprecated `setup_spack_runner`.
  EOT
  value       = local.combined_runner
}

output "gcs_bucket_path" {
  description = "Bucket containing the startup scripts for spack, to be reused by spack-execute module."
  value       = "gs://${google_storage_bucket.bucket.name}"
}

output "spack_profile_script_path" {
  description = "Path to the Spack profile.d script."
  value       = var.spack_profile_script_path
}

output "system_user_name" {
  description = "The system user used to install Spack. It can be reused by spack-execute module to install spack packages."
  value       = var.system_user_name
}
