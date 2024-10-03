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

output "pool_password_secret_id" {
  description = "Google Cloud Secret Manager ID containing HTCondor Pool Password"
  value       = google_secret_manager_secret.pool_password.secret_id
  sensitive   = true
}

output "central_manager_runner" {
  description = "Toolkit Runner to download pool secrets to an HTCondor central manager"
  value       = local.runner_cm
  depends_on = [
    google_secret_manager_secret_version.pool_password
  ]
}

output "access_point_runner" {
  description = "Toolkit Runner to download pool secrets to an HTCondor access point"
  value       = local.runner_access
  depends_on = [
    google_secret_manager_secret_version.pool_password
  ]
}

output "execute_point_runner" {
  description = "Toolkit Runner to download pool secrets to an HTCondor execute point"
  value       = local.runner_execute
  depends_on = [
    google_secret_manager_secret_version.pool_password
  ]
}

output "windows_startup_ps1" {
  description = "PowerShell script to download pool secrets to an HTCondor execute point"
  value       = local.windows_startup_ps1
}
