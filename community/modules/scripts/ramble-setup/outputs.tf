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

output "startup_script" {
  description = "Ramble installation script."
  value       = module.startup_script.startup_script
}

output "controller_startup_script" {
  description = "Ramble installation script, duplicate for SLURM controller."
  value       = module.startup_script.startup_script
}

output "ramble_runner" {
  description = <<-EOT
  Runner to be used with startup-script module or passed to ramble-execute module.
  - installs Ramble dependencies
  - installs Ramble
  - generates profile.d script to enable access to Ramble
  This is safe to run in parallel by multiple machines.
  EOT
  value       = local.combined_runner
}

output "ramble_path" {
  description = "Location ramble is installed into."
  value       = var.install_dir
}

output "ramble_ref" {
  description = "Git ref the ramble install is checked out to use"
  value       = var.ramble_ref
}

output "gcs_bucket_path" {
  description = "Bucket containing the startup scripts for Ramble, to be reused by ramble-execute module."
  value       = "gs://${google_storage_bucket.bucket.name}"
}

output "ramble_profile_script_path" {
  description = "Path to Ramble profile script."
  value       = var.ramble_profile_script_path
}

output "system_user_name" {
  description = "The system user used to install Ramble. It can be reused by ramble-execute module to execute Ramble commands."
  value       = var.system_user_name
}
