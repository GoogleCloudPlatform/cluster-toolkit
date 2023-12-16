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
  description = "Spack startup script."
  value       = module.startup_script.startup_script
}

output "controller_startup_script" {
  description = "Spack startup script, duplicate for SLURM controller."
  value       = module.startup_script.startup_script
}

output "spack_runner" {
  description = "Single runner that combines scripts from this module and any previously chained spack-execute or spack-setup modules."
  value       = local.combined_runner
}

output "gcs_bucket_path" {
  description = "Bucket containing the startup scripts for spack, to be reused by spack-execute module."
  value       = var.gcs_bucket_path
}

output "spack_profile_script_path" {
  description = "Path to the Spack profile.d script."
  value       = var.spack_profile_script_path
}

output "system_user_name" {
  description = "The system user used to execute commands."
  value       = var.system_user_name
}
