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
  description = "Ramble startup script."
  value       = module.startup_script.startup_script
}

output "controller_startup_script" {
  description = "Ramble startup script, duplicate for SLURM controller."
  value       = module.startup_script.startup_script
}

output "ramble_runner" {
  description = <<-EOT
  Runner to execute Ramble commands using an ansible playbook. The startup-script module
  will automatically handle installation of ansible.
  EOT
  value       = local.combined_runner
}

output "gcs_bucket_path" {
  description = "Bucket containing the startup scripts for ramble, to be reused by ramble-execute module."
  value       = var.gcs_bucket_path
}

output "spack_profile_script_path" {
  description = "Path to Spack profile script."
  value       = var.spack_profile_script_path
}

output "ramble_profile_script_path" {
  description = "Path to Ramble profile script."
  value       = var.ramble_profile_script_path
}

output "system_user_name" {
  description = "The system user used to execute commands."
  value       = var.system_user_name
}
